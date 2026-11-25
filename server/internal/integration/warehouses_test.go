// 回归 MongoDB 仓库事务、跨库存拆分、旧回执与会话围栏，检查提交原子性。

package integration

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// 真实事务边界：旧物、堆叠、货币是明确隔离前态；不冒充正常获取或权益购买。
// TestWarehouseTransactionsSplitsLegacyReceiptsAndFencesMongo 核对仓储物品拆分的原子提交、旧回执、重放和会话围栏。
// t 提供断言失败报告、子用例和清理注册。
func TestWarehouseTransactionsSplitsLegacyReceiptsAndFencesMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo仓储事务")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_wh_" + domain.ID()
	// ctx 保存由“background”取得的上下文。
	ctx := context.Background()
	// authority 保存由打开取得的权限围栏。
	authority, err := persistence.Open(ctx, cfg)
	must(t, err)
	// owner 保存由打开取得的所有者。
	owner, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		// c 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		must(t, authority.DB.Drop(c))
		owner.Close(c)
		authority.Close(c)
	})
	must(t, authority.Singleton(ctx, "account"))
	// grant 保存由批准游戏取得的授予。
	grant, err := authority.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
	must(t, err)
	owner.Epoch = grant.Epoch
	// login 接收登录供后续步骤使用。
	login, err := authority.Login(ctx, "wh_tx_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	// account 保存由“authenticate”取得的账号。
	account, err := authority.Authenticate(ctx, login.AccountToken)
	must(t, err)
	// key 保存由创建角色取得的键。
	key, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: 1, Job: 0, Name: "仓储事务" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())})
	must(t, err)
	// legacy 保存由角色取得的兼容旧数据。
	legacy, err := authority.Role(ctx, domain.Hex(key.Key.RoleId))
	must(t, err)
	// wood 保存从道具集合取出的“wood”。
	wood := legacy.Items[0]
	wood.Quality, wood.Locked = 4, true
	// page 初始化用作页面或分页的道具。
	page := domain.Item{ID: domain.ID(), ItemID: cfg.Fixture.SkillEconomy.PageItemID, Quantity: 17, Version: 3, Bound: true, SourceKind: "skill_recycle", SourceID: "isolated-original-source"}
	legacy.Items = append(legacy.Items, page)
	legacy.Items[0] = wood
	_, err = authority.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": legacy.ID}, bson.M{"$set": bson.M{"items": legacy.Items, "wallet.yuanbao_units": cfg.Fixture.Warehouses.PersonalExpansionYuanbaoUnits, "wallet.gold": int64(101), "privilege_until": time.Now().Add(time.Hour)}})
	must(t, err)
	// ticket 保存由签发入场票据取得的入场票据。
	ticket, err := authority.IssueTicket(ctx, account, key.Key)
	must(t, err)
	// r 接收入场供后续步骤使用，auth 保存由入场取得的授权。
	r, auth, err := owner.Enter(ctx, ticket.Ticket)
	must(t, err)
	// query 封装查询对应的后续处理，scope 记录作用域，使用数值表示。
	query := func(scope uint32) *pb.WarehousePage {
		must(t, owner.Renew(ctx, "game"))
		// v 接收查询仓库供后续步骤使用，e 接收查询仓库供后续步骤使用。
		v, e := owner.QueryWarehouse(ctx, auth, &pb.WarehouseQuery{Scope: scope, Limit: cfg.Fixture.Warehouses.QueryPageLimit})
		must(t, e)
		return v
	}
	// request 封装请求对应的后续处理，v 记录仓库动作。
	request := func(v *pb.WarehouseAction) persistence.AssetRequest {
		v.Operation = &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(r.EconomicSeq + 1), ExpectedRevision: uint64(r.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}
		// raw 保存由序列化取得的原始值，e 接收序列化供后续步骤使用。
		raw, e := json.Marshal(v)
		must(t, e)
		return persistence.AssetRequest{Kind: "warehouse", Context: v.Operation, Warehouse: v, Fingerprint: persistence.Hash(raw)}
	}
	// apply 封装应用对应的后续处理，q 记录资产请求。
	apply := func(q persistence.AssetRequest) (*pb.OperationResult, persistence.Operation) {
		must(t, owner.Renew(ctx, "game"))
		// next 保存由资产带有回执取得的下一状态，res 保存由资产带有回执取得的结果，op 保存由资产带有回执取得的操作，e 接收资产带有回执供后续步骤使用。
		next, res, op, e := owner.AssetWithReceipt(ctx, auth, q)
		must(t, e)
		r = next
		return res, op
	}
	// initial 保存由查询取得的初始值。
	initial := query(1)
	if initial.Revision != 0 || initial.Capacity != cfg.Fixture.Warehouses.PersonalInitialSlots {
		t.Fatal("空仓非虚拟初态")
	}
	// n 接收数量文档供后续步骤使用。
	n, err := owner.DB.Collection("warehouses").CountDocuments(ctx, bson.M{})
	must(t, err)
	if n != 0 {
		t.Fatal("只读查询写了空仓")
	}
	// source 保存持久收益来源序号，用于当前步骤中的来源。
	source := r.SourceSeq
	// wallet 保存当前处理所用的钱包。
	wallet := r.Wallet
	// res 保存由应用取得的结果，old 保存由应用取得的原状态。
	res, old := apply(request(&pb.WarehouseAction{Scope: 1, Action: 1, WarehouseSlot: 0, ItemInstanceId: domain.Bytes(wood.ID), ItemVersion: uint64(wood.Version), Quantity: wood.Quantity}))
	if res.ErrorCode != 0 || old.Warehouse == nil {
		t.Fatal("旧品质/空来源/锁件被困", res)
	}
	// op 保存由应用取得的操作。
	res, op := apply(request(&pb.WarehouseAction{Scope: 1, Action: 2, WarehouseRevision: 1, WarehouseSlot: 0, ItemInstanceId: domain.Bytes(wood.ID), ItemVersion: uint64(wood.Version + 1), Quantity: wood.Quantity}))
	if res.ErrorCode != 0 {
		t.Fatal(res)
	}
	// expected 保存当前处理所用的预期结果。
	expected := wood
	expected.Version += 2
	if !reflect.DeepEqual(*op.Warehouse.Transferred, expected) {
		t.Fatal("旧件原状态被洗白")
	}
	res, op = apply(request(&pb.WarehouseAction{Scope: 1, Action: 1, WarehouseRevision: 2, WarehouseSlot: 1, ItemInstanceId: domain.Bytes(page.ID), ItemVersion: uint64(page.Version), Quantity: 7}))
	if res.ErrorCode != 0 || op.Warehouse.Remainder.Quantity != 10 || op.Warehouse.Transferred.ID == page.ID {
		t.Fatal("部分存入未保原余量", res)
	}
	// split 保存当前处理所用的拆分。
	split := *op.Warehouse.Transferred
	// firstSplitReceipt 保存仓库，用于当前步骤中的第一项拆分回执。
	firstSplitReceipt := op.Warehouse
	_, err = owner.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$set": bson.M{"capacity": len(r.Items)}})
	must(t, err)
	r, err = owner.Role(ctx, r.ID)
	must(t, err)
	// beforeFull 保存当前处理所用的操作前的状态完整或满额。
	beforeFull := r
	// rejectedFull 保存由应用取得的拒绝完整或满额。
	res, rejectedFull := apply(request(&pb.WarehouseAction{Scope: 1, Action: 2, WarehouseRevision: 3, WarehouseSlot: 1, ItemInstanceId: domain.Bytes(split.ID), ItemVersion: uint64(split.Version), Quantity: 3}))
	if res.ErrorCode != p.CodeWarehouseFull || rejectedFull.Warehouse != nil || !reflect.DeepEqual(r.Items, beforeFull.Items) || query(1).Revision != 3 {
		t.Fatal("满包取物丢失/改仓")
	}
	_, err = owner.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$set": bson.M{"capacity": cfg.Fixture.InventoryCapacity}})
	must(t, err)
	r, err = owner.Role(ctx, r.ID)
	must(t, err)
	res, op = apply(request(&pb.WarehouseAction{Scope: 1, Action: 2, WarehouseRevision: 3, WarehouseSlot: 1, ItemInstanceId: domain.Bytes(split.ID), ItemVersion: uint64(split.Version), Quantity: 3}))
	if res.ErrorCode != 0 || op.Warehouse.Remainder.ID != split.ID || op.Warehouse.Remainder.Quantity != 4 || op.Warehouse.Transferred.Quantity != 3 {
		t.Fatal("部分取出不是原位余量+新身份", res)
	}
	// stored 记录已持久保存，使用仓库已持久保存道具表示。
	var stored persistence.WarehouseStoredItem
	must(t, owner.DB.Collection("warehouse_items").FindOne(ctx, bson.M{"_id": split.ID}).Decode(&stored))
	if stored.Item.Quantity != 4 || stored.Item.Version != 2 {
		t.Fatal("仓内余量未原位版本更新")
	}
	// expanded 保存由应用取得的“expanded”。
	res, expanded := apply(request(&pb.WarehouseAction{Scope: 1, Action: 5, WarehouseRevision: 4}))
	if res.ErrorCode != 0 || expanded.Warehouse.Capacity != int64(cfg.Fixture.Warehouses.PersonalInitialSlots+cfg.Fixture.Warehouses.PersonalExpansionSlots) || r.Wallet.Yuanbao != 0 {
		t.Fatal("个人扩页没有非绑元宝原子收费")
	}
	// q 接收请求供后续步骤使用。
	q := request(&pb.WarehouseAction{Scope: 2, Action: 3, Currency: 1, Amount: 37})
	// deposit 保存由应用取得的“deposit”。
	res, deposit := apply(q)
	if res.ErrorCode != 0 || r.Wallet.Gold != 64 || deposit.Warehouse.GoldAfter != 37 {
		t.Fatal("共享存金未守恒")
	}
	// before 保存当前处理所用的操作前的状态。
	before := r
	// replayed 保存由应用取得的“replayed”。
	res, replayed := apply(q)
	if res.ErrorCode != 0 || !reflect.DeepEqual(before, r) || !reflect.DeepEqual(deposit.Warehouse, replayed.Warehouse) {
		t.Fatal("原op再扣或回执变动")
	}
	// changed 保存当前处理所用的已变化。
	changed := q
	changed.Fingerprint = "different"
	_, _, _, err = owner.AssetWithReceipt(ctx, auth, changed)
	if persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("原op换内容")
	}
	// 失去特权后原op仍回原终态，但新读写均不可获共享内容。
	_, err = owner.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$set": bson.M{"privilege_until": time.Now().Add(-time.Second)}})
	must(t, err)
	r, err = owner.Role(ctx, r.ID)
	must(t, err)
	res, replayed = apply(q)
	if res.ErrorCode != 0 || !reflect.DeepEqual(replayed.Warehouse, deposit.Warehouse) {
		t.Fatal("到期后已提交原op被重新判权限")
	}
	// e 接收查询仓库供后续步骤使用。
	if _, e := owner.QueryWarehouse(ctx, auth, &pb.WarehouseQuery{Scope: 2, Limit: 1}); persistence.Code(e) != p.CodeWarehousePrivilegeRequired {
		t.Fatal("到期查询泄漏", e)
	}
	// denied 保存由应用取得的“denied”。
	res, denied := apply(request(&pb.WarehouseAction{Scope: 2, Action: 4, WarehouseRevision: 1, Currency: 1, Amount: 1}))
	if res.ErrorCode != p.CodeWarehousePrivilegeRequired || denied.Warehouse != nil {
		t.Fatal("到期取款例外")
	}
	if r.SourceSeq != source || r.Wallet.Gold+deposit.Warehouse.GoldAfter != wallet.Gold || r.Wallet.BoundGold != wallet.BoundGold || r.Wallet.BoundYuanbao != wallet.BoundYuanbao || firstSplitReceipt.Transferred.Quantity != 7 {
		t.Fatal("来源水位/四钱包/旧拆分回执不守恒")
	}
	// metadata版本水位耗尽仍可读，不得回绕或改原物；手动动作没有SourceSeq出口。
	_, err = owner.DB.Collection("warehouses").UpdateOne(ctx, bson.M{"_id": expanded.Warehouse.WarehouseID}, bson.M{"$set": bson.M{"revision": int64(1<<63 - 1)}})
	must(t, err)
	if query(1).Revision != uint64(1<<63-1) {
		t.Fatal("水位耗尽后不能读原仓")
	}
	// beforeMax 保存当前处理所用的操作前的状态最大。
	beforeMax := r
	// maxReceipt 保存由应用取得的最大回执。
	res, maxReceipt := apply(request(&pb.WarehouseAction{Scope: 1, Action: 5, WarehouseRevision: uint64(1<<63 - 1)}))
	if res.ErrorCode != p.CodeInvalidArgument || maxReceipt.Warehouse != nil || r.Wallet != beforeMax.Wallet {
		t.Fatal("仓库版本上溢收费")
	}
	// 原owner失效后连只读内容也不能返回；不以本地仓缓存绕过围栏。
	must(t, owner.Release(ctx, "game"))
	// e 接收查询仓库供后续步骤使用。
	if _, e := owner.QueryWarehouse(ctx, auth, &pb.WarehouseQuery{Scope: 1, Limit: 1}); persistence.Code(e) != p.CodeRouteChanged {
		t.Fatal("旧owner查询未拒", e)
	}
	t.Log("隔离旧品质4/锁定/空来源原件完整往返；17页部分存7再取3，仓余4原UID版本2；个人扩页精确非绑元宝收费、共享37金币守恒；到期原op先回查/新读写拒绝；旧owner拒绝")
}
