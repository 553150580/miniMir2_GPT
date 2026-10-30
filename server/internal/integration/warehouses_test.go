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
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// 真实事务边界：旧物、堆叠、货币是明确隔离前态；不冒充正常获取或权益购买。
func TestWarehouseTransactionsSplitsLegacyReceiptsAndFencesMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo仓储事务")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_wh_" + domain.ID()
	ctx := context.Background()
	authority, err := persistence.Open(ctx, cfg)
	must(t, err)
	owner, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		must(t, authority.DB.Drop(c))
		owner.Close(c)
		authority.Close(c)
	})
	must(t, authority.Singleton(ctx, "account"))
	grant, err := authority.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
	must(t, err)
	owner.Epoch = grant.Epoch
	login, err := authority.Login(ctx, "wh_tx_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	account, err := authority.Authenticate(ctx, login.AccountToken)
	must(t, err)
	key, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: 1, Job: 0, Name: "仓储事务" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())})
	must(t, err)
	legacy, err := authority.Role(ctx, domain.Hex(key.Key.RoleId))
	must(t, err)
	wood := legacy.Items[0]
	wood.Quality, wood.Locked = 4, true
	page := domain.Item{ID: domain.ID(), ItemID: cfg.Fixture.SkillEconomy.PageItemID, Quantity: 17, Version: 3, Bound: true, SourceKind: "skill_recycle", SourceID: "isolated-original-source"}
	legacy.Items = append(legacy.Items, page)
	legacy.Items[0] = wood
	_, err = authority.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": legacy.ID}, bson.M{"$set": bson.M{"items": legacy.Items, "wallet.yuanbao_units": cfg.Fixture.Warehouses.PersonalExpansionYuanbaoUnits, "wallet.gold": int64(101), "privilege_until": time.Now().Add(time.Hour)}})
	must(t, err)
	ticket, err := authority.IssueTicket(ctx, account, key.Key)
	must(t, err)
	r, auth, err := owner.Enter(ctx, ticket.Ticket)
	must(t, err)
	query := func(scope uint32) *pb.WarehousePage {
		must(t, owner.Renew(ctx, "game"))
		v, e := owner.QueryWarehouse(ctx, auth, &pb.WarehouseQuery{Scope: scope, Limit: cfg.Fixture.Warehouses.QueryPageLimit})
		must(t, e)
		return v
	}
	request := func(v *pb.WarehouseAction) persistence.AssetRequest {
		v.Operation = &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(r.EconomicSeq + 1), ExpectedRevision: uint64(r.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}
		raw, e := json.Marshal(v)
		must(t, e)
		return persistence.AssetRequest{Kind: "warehouse", Context: v.Operation, Warehouse: v, Fingerprint: persistence.Hash(raw)}
	}
	apply := func(q persistence.AssetRequest) (*pb.OperationResult, persistence.Operation) {
		must(t, owner.Renew(ctx, "game"))
		next, res, op, e := owner.AssetWithReceipt(ctx, auth, q)
		must(t, e)
		r = next
		return res, op
	}
	initial := query(1)
	if initial.Revision != 0 || initial.Capacity != cfg.Fixture.Warehouses.PersonalInitialSlots {
		t.Fatal("空仓非虚拟初态")
	}
	n, err := owner.DB.Collection("warehouses").CountDocuments(ctx, bson.M{})
	must(t, err)
	if n != 0 {
		t.Fatal("只读查询写了空仓")
	}
	source := r.SourceSeq
	wallet := r.Wallet
	res, old := apply(request(&pb.WarehouseAction{Scope: 1, Action: 1, WarehouseSlot: 0, ItemInstanceId: domain.Bytes(wood.ID), ItemVersion: uint64(wood.Version), Quantity: wood.Quantity}))
	if res.ErrorCode != 0 || old.Warehouse == nil {
		t.Fatal("旧品质/空来源/锁件被困", res)
	}
	res, op := apply(request(&pb.WarehouseAction{Scope: 1, Action: 2, WarehouseRevision: 1, WarehouseSlot: 0, ItemInstanceId: domain.Bytes(wood.ID), ItemVersion: uint64(wood.Version + 1), Quantity: wood.Quantity}))
	if res.ErrorCode != 0 {
		t.Fatal(res)
	}
	expected := wood
	expected.Version += 2
	if !reflect.DeepEqual(*op.Warehouse.Transferred, expected) {
		t.Fatal("旧件原状态被洗白")
	}
	res, op = apply(request(&pb.WarehouseAction{Scope: 1, Action: 1, WarehouseRevision: 2, WarehouseSlot: 1, ItemInstanceId: domain.Bytes(page.ID), ItemVersion: uint64(page.Version), Quantity: 7}))
	if res.ErrorCode != 0 || op.Warehouse.Remainder.Quantity != 10 || op.Warehouse.Transferred.ID == page.ID {
		t.Fatal("部分存入未保原余量", res)
	}
	split := *op.Warehouse.Transferred
	firstSplitReceipt := op.Warehouse
	_, err = owner.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$set": bson.M{"capacity": len(r.Items)}})
	must(t, err)
	r, err = owner.Role(ctx, r.ID)
	must(t, err)
	beforeFull := r
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
	var stored persistence.WarehouseStoredItem
	must(t, owner.DB.Collection("warehouse_items").FindOne(ctx, bson.M{"_id": split.ID}).Decode(&stored))
	if stored.Item.Quantity != 4 || stored.Item.Version != 2 {
		t.Fatal("仓内余量未原位版本更新")
	}
	res, expanded := apply(request(&pb.WarehouseAction{Scope: 1, Action: 5, WarehouseRevision: 4}))
	if res.ErrorCode != 0 || expanded.Warehouse.Capacity != int64(cfg.Fixture.Warehouses.PersonalInitialSlots+cfg.Fixture.Warehouses.PersonalExpansionSlots) || r.Wallet.Yuanbao != 0 {
		t.Fatal("个人扩页没有非绑元宝原子收费")
	}
	q := request(&pb.WarehouseAction{Scope: 2, Action: 3, Currency: 1, Amount: 37})
	res, deposit := apply(q)
	if res.ErrorCode != 0 || r.Wallet.Gold != 64 || deposit.Warehouse.GoldAfter != 37 {
		t.Fatal("共享存金未守恒")
	}
	before := r
	res, replayed := apply(q)
	if res.ErrorCode != 0 || !reflect.DeepEqual(before, r) || !reflect.DeepEqual(deposit.Warehouse, replayed.Warehouse) {
		t.Fatal("原op再扣或回执变动")
	}
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
	if _, e := owner.QueryWarehouse(ctx, auth, &pb.WarehouseQuery{Scope: 2, Limit: 1}); persistence.Code(e) != p.CodeWarehousePrivilegeRequired {
		t.Fatal("到期查询泄漏", e)
	}
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
	beforeMax := r
	res, maxReceipt := apply(request(&pb.WarehouseAction{Scope: 1, Action: 5, WarehouseRevision: uint64(1<<63 - 1)}))
	if res.ErrorCode != p.CodeInvalidArgument || maxReceipt.Warehouse != nil || r.Wallet != beforeMax.Wallet {
		t.Fatal("仓库版本上溢收费")
	}
	// 原owner失效后连只读内容也不能返回；不以本地仓缓存绕过围栏。
	must(t, owner.Release(ctx, "game"))
	if _, e := owner.QueryWarehouse(ctx, auth, &pb.WarehouseQuery{Scope: 1, Limit: 1}); persistence.Code(e) != p.CodeRouteChanged {
		t.Fatal("旧owner查询未拒", e)
	}
	t.Log("隔离旧品质4/锁定/空来源原件完整往返；17页部分存7再取3，仓余4原UID版本2；个人扩页精确非绑元宝收费、共享37金币守恒；到期原op先回查/新读写拒绝；旧owner拒绝")
}
