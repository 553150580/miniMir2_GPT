// 验证真实装备事务的版本与回执、换装卸装状态、较旧穿戴数据及法术道术属性的往返。

package integration

import (
	"context"
	"encoding/json"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

// TestEquipmentTransactionsVersionsReceiptsMongo 验证装备操作的原实例版本、持久回执与重放约束。
// t 提供断言失败报告、子用例和清理注册。
func TestEquipmentTransactionsVersionsReceiptsMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo装备事务")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion < 23 {
		t.Fatal("必须使用已启用的装备合同")
	}
	cfg.Database = "minimir_dev_game_test_eq_" + domain.ID()
	// ctx 保存由“background”取得的上下文。
	ctx := context.Background()
	// authority 保存由打开取得的权限围栏。
	authority, err := persistence.Open(ctx, cfg)
	must(t, err)
	// owner 保存由打开取得的所有者。
	owner, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, authority.DB.Drop(cleanup))
		owner.Close(cleanup)
		authority.Close(cleanup)
	})
	must(t, authority.Singleton(ctx, "account"))
	// grant 保存由批准游戏取得的授予。
	grant, err := authority.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
	must(t, err)
	owner.Epoch = grant.Epoch
	// login 接收登录供后续步骤使用。
	login, err := authority.Login(ctx, "eq_tx_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	// account 保存由“authenticate”取得的账号。
	account, err := authority.Authenticate(ctx, login.AccountToken)
	must(t, err)
	// key 保存由创建角色取得的键。
	key, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 1, Name: "装备事务" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())})
	must(t, err)
	// ticket 保存由签发入场票据取得的入场票据。
	ticket, err := authority.IssueTicket(ctx, account, key.Key)
	must(t, err)
	// r 接收入场供后续步骤使用，auth 保存由入场取得的授权。
	r, auth, err := owner.Enter(ctx, ticket.Ticket)
	must(t, err)
	// woods 记录“woods”，使用道具列表表示。
	var woods []domain.Item
	// item 接收遍历道具集合时当前的元素。
	for _, item := range r.Items {
		if item.ItemID == 218 {
			woods = append(woods, item)
		}
	}
	if len(woods) != 2 {
		t.Fatal("必须正常Create两件原木剑")
	}
	// request 封装请求对应的后续处理，id 区分当前步骤指向的目标对象，version 记录版本，使用数值表示，unequip 标记是否处于“unequip”状态。
	request := func(id string, version uint64, unequip bool) persistence.AssetRequest {
		// q 组装资产请求所需的数据。
		q := persistence.AssetRequest{Kind: "equip", ItemID: id, Slot: 0, ItemVersion: version, Unequip: unequip, Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(r.EconomicSeq + 1), ExpectedRevision: uint64(r.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}}
		// raw 保存由序列化取得的原始值，e 接收序列化供后续步骤使用。
		raw, e := json.Marshal(q)
		must(t, e)
		q.Fingerprint = persistence.Hash(raw)
		return q
	}
	// apply 封装应用对应的后续处理，q 记录资产请求。
	apply := func(q persistence.AssetRequest) (*pb.OperationResult, persistence.Operation) {
		// out 保存由资产带有回执取得的输出，res 保存由资产带有回执取得的结果，op 保存由资产带有回执取得的操作，e 接收资产带有回执供后续步骤使用。
		out, res, op, e := owner.AssetWithReceipt(ctx, auth, q)
		must(t, e)
		r = out
		return res, op
	}
	// originalWallet 保存钱包，用于当前步骤中的原始钱包，originalSource 保存持久收益来源序号，用于当前步骤中的原始来源。
	originalWallet, originalSource := r.Wallet, r.SourceSeq
	// first 保存由请求取得的第一项。
	first := request(woods[0].ID, uint64(woods[0].Version), false)
	// res 保存由应用取得的结果，firstReceipt 保存由应用取得的第一项回执。
	res, firstReceipt := apply(first)
	if res.ErrorCode != 0 || firstReceipt.Equipment == nil || len(firstReceipt.Equipment.Moves) != 1 || firstReceipt.Equipment.Moves[0].Before.ID != woods[0].ID || firstReceipt.Equipment.Moves[0].After.Version != 2 {
		t.Fatal("正常穿入缺少实际typed原件回执", res, firstReceipt)
	}
	// audit 记录审计记录，使用临时结构表示。
	var audit struct {
		// Equipment 记录装备，使用装备回执表示。
		Equipment *persistence.EquipmentReceipt `bson:"equipment"`
	}
	must(t, owner.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": firstReceipt.ID}).Decode(&audit))
	if !reflect.DeepEqual(audit.Equipment, firstReceipt.Equipment) {
		t.Fatal("同事务audit与实际装备回执不一致")
	}
	// swapReceipt 保存由应用取得的交换回执。
	res, swapReceipt := apply(request(woods[1].ID, uint64(woods[1].Version), false))
	if res.ErrorCode != 0 || len(swapReceipt.Equipment.Moves) != 2 || r.Equipped[0].Item.ID != woods[1].ID || r.Equipped[0].Item.Version != 2 {
		t.Fatal("换件未记录双方位置变更")
	}
	// returned 记录已返回值，使用道具表示。
	var returned domain.Item
	// item 接收遍历道具集合时当前的元素。
	for _, item := range r.Items {
		if item.ID == woods[0].ID {
			returned = item
		}
	}
	if returned.Version != 3 || returned.Bound != woods[0].Bound || returned.SourceID != woods[0].SourceID {
		t.Fatal("回包旧件身份/版本改变不正确")
	}
	// beforeReplay 保存当前处理所用的操作前的状态重放。
	beforeReplay := r
	// replayed 保存由应用取得的“replayed”。
	res, replayed := apply(first)
	if res.ErrorCode != 0 || !reflect.DeepEqual(r, beforeReplay) || !reflect.DeepEqual(replayed.Equipment, firstReceipt.Equipment) {
		t.Fatal("旧op按当前所在位置重算或回执变化")
	}
	// changed 保存当前处理所用的已变化。
	changed := first
	changed.Unequip = true
	changed.Fingerprint = "changed-original-intent"
	_, _, _, err = owner.AssetWithReceipt(ctx, auth, changed)
	if persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("原op改动作未冲突")
	}
	// beforeFailure 保存当前处理所用的操作前的状态失败。
	beforeFailure := r
	// rejected 保存由应用取得的拒绝。
	res, rejected := apply(request(woods[1].ID, 1, true))
	if res.ErrorCode != p.CodeEquipmentChanged || rejected.Equipment != nil || r.Revision != beforeFailure.Revision+1 || r.EconomicSeq != beforeFailure.EconomicSeq+1 || !reflect.DeepEqual(r.Items, beforeFailure.Items) || !reflect.DeepEqual(r.Equipped, beforeFailure.Equipped) {
		t.Fatal("陈旧实例失败未遵原Asset业务失败合同")
	}
	// removed 保存由应用取得的“removed”。
	res, removed := apply(request(woods[1].ID, 2, true))
	if res.ErrorCode != 0 || !removed.Equipment.Unequip || len(r.Equipped) != 0 || r.SourceSeq != originalSource || r.Wallet != originalWallet {
		t.Fatal("真正卸下改变来源流水/钱包或没有移回原件")
	}
	if firstReceipt.Equipment.Moves[0].After.Version != 2 {
		t.Fatal("后续移物改写旧回执")
	}
	// 显式永久水位极值资格，不伪装成自然产生MaxInt64资产。
	_, err = owner.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$set": bson.M{"revision": int64(math.MaxInt64)}})
	must(t, err)
	r, err = owner.Role(ctx, r.ID)
	must(t, err)
	// q 接收请求供后续步骤使用。
	q := request(woods[0].ID, uint64(returned.Version), false)
	// countBefore 保存由数量文档取得的数量操作前的状态。
	countBefore, err := owner.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": r.ID})
	must(t, err)
	_, _, _, err = owner.AssetWithReceipt(ctx, auth, q)
	if persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("Revision满值仍提交装备")
	}
	// countAfter 保存由数量文档取得的数量操作后的状态。
	countAfter, err := owner.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": r.ID})
	must(t, err)
	if countBefore != countAfter {
		t.Fatal("满值围栏仍生成op")
	}
}

// TestEquipmentWearSwapUnequipAndReconnectMongoWS 在真实连接执行穿入、换装与卸下，确认原装备实例可在重连后查证。
// t 提供断言失败报告、子用例和清理注册。
func TestEquipmentWearSwapUnequipAndReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实WS装备穿脱")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion < 23 {
		t.Fatal("必须使用已启用的装备合同")
	}
	// offer 记录尝试提交，使用技能商店表示。
	var offer devconfig.SkillShop
	// row 接收遍历技能商店时当前的元素。
	for _, row := range cfg.Fixture.SkillShop {
		if row.Currency == "gold" && row.RequiredRoleLevel == 1 {
			offer = row
			break
		}
	}
	if offer.ID == 0 {
		t.Fatal("缺正常金币原书报价")
	}
	// 满包和资金是明确的隔离Create资格；不预置穿戴/版本/锁/回执，不新增装备和属性。
	cfg.Fixture.InitialInventory = []uint32{218, 218, 193}
	for len(cfg.Fixture.InitialInventory) < cfg.Fixture.InventoryCapacity-1 {
		cfg.Fixture.InitialInventory = append(cfg.Fixture.InitialInventory, 35)
	}
	cfg.Fixture.InitialBoundGold = offer.PriceUnits
	must(t, cfg.Fixture.Validate())
	cfg.Database = "minimir_dev_game_test_eqws_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// ctx 保存由“background”取得的上下文。
	ctx := context.Background()
	// inspection 保存由打开取得的检查。
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	// launch 封装启动对应的后续处理。
	launch := func() func() {
		// runCtx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		runCtx, cancel := context.WithCancel(ctx)
		// wg 记录“wg”，使用协程汇合计数器表示。
		var wg sync.WaitGroup
		// mode 接收遍历文本列表时当前的元素。
		for _, mode := range []string{"account", "game", "gm"} {
			wg.Go(func() {
				// e 接收运行批次供后续步骤使用。
				if e := bootstrap.Run(runCtx, cfg, mode); e != nil && runCtx.Err() == nil {
					t.Errorf("装备%s节点:%v", mode, e)
				}
			})
		}
		// endpoint 接收遍历文本列表时当前的元素。
		for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
			healthy(t, endpoint)
		}
		return func() { cancel(); wg.Wait() }
	}
	// stop 保存由启动取得的停止。
	stop := launch()
	t.Cleanup(func() {
		stop()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "eq_ws_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	// key 保存当前处理所用的键。
	key := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 1, Name: "穿脱法师" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, key, 0)
	// c 接收加入供后续步骤使用。
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, key.Key))
	defer func() { c.close() }()
	c.auto(false, 8)
	// stored 封装已持久保存对应的后续处理，r 接收角色供后续步骤使用，e 接收角色供后续步骤使用。
	stored := func() domain.Role { r, e := inspection.Role(ctx, domain.Hex(key.Key.RoleId)); must(t, e); return r }
	if len(c.inventory.Items) != int(c.inventory.Capacity) {
		t.Fatal("正常Create未形成声明的满包资格")
	}
	// woods 记录“woods”，使用道具观察视图列表表示。
	var woods []*pb.ItemView
	// cloth 记录“cloth”，使用道具观察视图表示。
	var cloth *pb.ItemView
	// item 接收遍历道具集合时当前的元素。
	for _, item := range c.inventory.Items {
		if item.ItemId == 218 {
			woods = append(woods, item)
		}
		if item.ItemId == 193 {
			cloth = item
		}
	}
	if len(woods) != 2 || cloth == nil {
		t.Fatal("缺少正常原木剑/布衣")
	}
	// baseline 封装场景基线对应的后续处理。
	baseline := func() *pb.EntityState {
		// f 记录协议帧。
		c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
		return proto.Clone(c.player()).(*pb.EntityState)
	}
	// itemInBag 封装道具输入背包对应的后续处理，id 区分当前步骤指向的目标对象。
	itemInBag := func(id []byte) *pb.ItemView {
		// item 接收遍历道具集合时当前的元素。
		for _, item := range c.inventory.Items {
			if reflect.DeepEqual(item.InstanceId, id) {
				return item
			}
		}
		t.Fatal("原实例不在背包")
		return nil
	}
	// asset 封装资产对应的后续处理，q 记录穿戴装备请求。
	asset := func(q *pb.EquipRequest) *pb.OperationResult {
		// res 保存由资产取得的结果。
		res := c.asset(p.MsgC2SEquip, q)
		c.inventoryAt(res.NewRevision)
		baseline()
		return res
	}
	// baseAttrs 按目标类型读取当前接口值，得到基础值“attrs”。
	baseAttrs := proto.Clone(c.player().CombatAttributes).(*pb.CombatAttributes)
	// lock 保存由资产取得的锁定。
	lock := c.asset(p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: woods[0].InstanceId, ItemVersion: woods[0].Version, Locked: true, Operation: c.operation()})
	if lock.ErrorCode != 0 {
		t.Fatal(lock)
	}
	c.inventoryAt(lock.NewRevision)
	baseline()
	// wood 保存由道具输入背包取得的“wood”。
	wood := itemInBag(woods[0].InstanceId)
	// first 保存当前处理所用的第一项。
	first := &pb.EquipRequest{ItemInstanceId: wood.InstanceId, ItemVersion: wood.Version, Slot: 0, Operation: c.operation()}
	// before 保存由已持久保存取得的操作前的状态。
	before := stored()
	// res 保存由资产取得的结果。
	if res := asset(first); res.ErrorCode != 0 {
		t.Fatal("锁定原件不能正常穿戴", res)
	}
	// after 保存由已持久保存取得的操作后的状态。
	after := stored()
	if after.Wallet != before.Wallet || after.SourceSeq != before.SourceSeq || after.Revision != before.Revision+1 || after.EconomicSeq != before.EconomicSeq+1 || !after.Equipped[0].Item.Locked {
		t.Fatal("穿戴改变原钱包/来源/锁/水位")
	}
	// woodDef 保存由道具取得的“wood”“def”。
	woodDef, _ := cfg.Fixture.Item(218)
	// attrs 保存战斗“attributes”，用于当前步骤中的“attrs”。
	attrs := c.player().CombatAttributes
	if attrs.PhysicalAttackMin != baseAttrs.PhysicalAttackMin+woodDef.AttackMinBonus || attrs.PhysicalAttackMax != baseAttrs.PhysicalAttackMax+woodDef.AttackMaxBonus || attrs.MagicAttackMin != baseAttrs.MagicAttackMin || attrs.SpiritAttackMax != baseAttrs.SpiritAttackMax {
		t.Fatal("真实源零MC/SC或DC权威投影错误")
	}
	// result 保存由技能确认取得的结果。
	if result := c.skillConfirm(c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: 1})); result.ErrorCode != 0 {
		t.Fatal("正常报价购书填回原空格失败", result)
	}
	baseline()
	// spare 保存由道具输入背包取得的“spare”。
	spare := itemInBag(woods[1].InstanceId)
	if len(c.inventory.Items) != int(c.inventory.Capacity) {
		t.Fatal("正常NPC购书未填满")
	}
	// res 保存由资产取得的结果。
	if res := asset(&pb.EquipRequest{ItemInstanceId: spare.InstanceId, ItemVersion: spare.Version, Slot: 0, Operation: c.operation()}); res.ErrorCode != 0 {
		t.Fatal("满包同槽换件失败", res)
	}
	if len(c.inventory.Items) != int(c.inventory.Capacity) || len(c.inventory.Equipped) != 1 || !reflect.DeepEqual(c.inventory.Equipped[0].Item.InstanceId, spare.InstanceId) {
		t.Fatal("满包替换丢格或丢原件")
	}
	// worn 保存道具，用于当前步骤中的“worn”。
	worn := c.inventory.Equipped[0].Item
	before = stored()
	// full 保存当前处理所用的满额。
	full := &pb.EquipRequest{ItemInstanceId: worn.InstanceId, ItemVersion: worn.Version, Slot: 0, Unequip: true, Operation: c.operation()}
	// res 保存由资产取得的结果。
	if res := asset(full); res.ErrorCode != p.CodeEquipmentBagFull {
		t.Fatal("满包卸下没有明确3401", res)
	}
	after = stored()
	if !reflect.DeepEqual(before.Items, after.Items) || !reflect.DeepEqual(before.Equipped, after.Equipped) || before.Wallet != after.Wallet || before.SourceSeq != after.SourceSeq || after.Revision != before.Revision+1 || after.EconomicSeq != before.EconomicSeq+1 {
		t.Fatal("满包卸下失败部分移物或改变业务失败合同")
	}
	// res 保存由资产取得的结果。
	res := c.asset(p.MsgC2SRecycle, &pb.RecycleRequest{ItemInstanceIds: [][]byte{cloth.InstanceId}, Operation: c.operation()})
	if res.ErrorCode != 0 {
		t.Fatal("正常分解原衣服腾格失败", res)
	}
	c.inventoryAt(res.NewRevision)
	baseline()
	if res = asset(&pb.EquipRequest{ItemInstanceId: worn.InstanceId, ItemVersion: worn.Version, Slot: 0, Unequip: true, Operation: c.operation()}); res.ErrorCode != 0 {
		t.Fatal("真正卸下失败", res)
	}
	if len(c.inventory.Equipped) != 0 || !proto.Equal(c.player().CombatAttributes, baseAttrs) {
		t.Fatal("卸装没有消除装备属性")
	}
	// returned 保存由道具输入背包取得的已返回值。
	returned := itemInBag(woods[0].InstanceId)
	if res = asset(&pb.EquipRequest{ItemInstanceId: returned.InstanceId, ItemVersion: returned.Version, Slot: 0, Operation: c.operation()}); res.ErrorCode != 0 {
		t.Fatal(res)
	}
	before = stored()
	if res = asset(first); res.ErrorCode != 0 {
		t.Fatal("原已提交请求不能按原指纹重放", res)
	}
	if after = stored(); !reflect.DeepEqual(before, after) {
		t.Fatal("重放旧穿入重新改变当前装备")
	}
	// receipt 记录回执，使用操作表示。
	var receipt persistence.Operation
	must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + before.ID + ":" + domain.Hex(first.Operation.OpId)}).Decode(&receipt))
	if receipt.Equipment == nil || receipt.Equipment.Moves[0].After.Version >= before.Equipped[0].Item.Version {
		t.Fatal("回执未保留首次旧版本事实")
	}
	// oldScene 保存场景代次，用于当前步骤中的原状态场景，oldLife 保存生命代次，用于当前步骤中的原状态生命代次。
	oldScene, oldLife := c.entry.SceneEpoch, c.player().Life
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, key.Key))
	if c.entry.SceneEpoch != oldScene || c.player().Life != oldLife || !reflect.DeepEqual(stored().Equipped, before.Equipped) {
		t.Fatal("健康新票重建身体或重放旧移物")
	}
	c.close()
	stop()
	stop = launch()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, key.Key))
	if c.entry.SceneEpoch == oldScene || !reflect.DeepEqual(stored().Equipped, before.Equipped) || !c.inventory.Equipped[0].Item.Locked {
		t.Fatal("冷建丢失永久原装备/锁或错误复用旧scene")
	}
	t.Log("正常Create/锁/穿戴/NPC填满/满包换件/3401原子拒绝/正常分解腾格/真正卸下/原op重放/健康与完整冷建均走真实协议；装备MC/SC保持主来源零，非零管线由隔离纯测覆盖。")
}

// TestEquipmentNonZeroMagicSpiritRoundtripMongoWS 验证真实线路中的魔法、道术装备加成在穿戴与恢复后的数值。
// t 提供断言失败报告、子用例和清理注册。
func TestEquipmentNonZeroMagicSpiritRoundtripMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实WS非零MC/SC隔离资格")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion < 23 {
		t.Fatal("必须从已启用的装备合同派生非零管线资格")
	}
	// n 接收遍历道具集合时当前的元素。
	for n := range cfg.Fixture.Items {
		if cfg.Fixture.Items[n].ID == 218 {
			cfg.Fixture.Items[n].MagicMinBonus, cfg.Fixture.Items[n].MagicMaxBonus = 2, 4
			cfg.Fixture.Items[n].TaoMinBonus, cfg.Fixture.Items[n].TaoMaxBonus = 3, 5
		}
	}
	// 只有怪物受击/造成受伤的隔离资格；不改技能、动作/恢复周期、掉率、刷新或任何临时结果。
	cfg.Fixture.Monsters[0].AttackMin, cfg.Fixture.Monsters[0].AttackMax = 4, 4
	cfg.Fixture.Monsters[0].Accuracy = 100
	cfg.Fixture.Monsters[0].MagicDefenceMin, cfg.Fixture.Monsters[0].MagicDefenceMax = 0, 0
	cfg.Fixture.Monsters[0].MagicResist = 0
	cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: 8, Y: 9}
	// raw 保存由序列化取得的原始值。
	raw, err := json.Marshal(cfg.Fixture)
	must(t, err)
	// roundtrip 记录往返编码，使用开发测试配置表示。
	var roundtrip devconfig.Fixture
	must(t, json.Unmarshal(raw, &roundtrip))
	roundtrip.Hash = persistence.Hash(raw)
	cfg.Fixture = roundtrip
	must(t, cfg.Fixture.Validate())
	// definition 保存由道具取得的定义，ok 表明查询、校验或类型转换是否成功。
	definition, ok := cfg.Fixture.Item(218)
	if !ok || !definition.EquipmentStatsKnown() || definition.MagicMinBonus != 2 || definition.MagicMaxBonus != 4 || definition.TaoMinBonus != 3 || definition.TaoMaxBonus != 5 {
		t.Fatal("JSON往返丢了四端点或显式阶段")
	}
	cfg.Database = "minimir_dev_game_test_eqnz_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancel(context.Background())
	// inspection 保存由打开取得的检查。
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	// wg 记录“wg”，使用协程汇合计数器表示。
	var wg sync.WaitGroup
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			// e 接收运行批次供后续步骤使用。
			if e := bootstrap.Run(ctx, cfg, mode); e != nil && ctx.Err() == nil {
				t.Errorf("非零装备%s:%v", mode, e)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	// endpoint 接收遍历文本列表时当前的元素。
	for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, endpoint)
	}
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "eq_nz_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	// job 接收遍历数值列表时当前的元素。
	for _, job := range []uint32{1, 2} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(map[uint32]string{1: "法师MC", 2: "道士SC"}[job], func(t *testing.T) {
			// key 保存当前处理所用的键。
			key := &pb.RoleSummary{}
			rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: job, Job: job, Name: "非零装备" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, key, 0)
			// 明示首次Enter前等级7资格；不预置Equipped、技能、资源、回执或施法结果。
			// e 接收更新“one”供后续步骤使用。
			_, e := inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(key.Key.RoleId)}, bson.M{"$set": bson.M{"level": uint32(7)}})
			must(t, e)
			// c 接收加入供后续步骤使用。
			c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, key.Key))
			defer c.close()
			c.auto(false, 1)
			// baseline 封装场景基线对应的后续处理，f 记录协议帧。
			baseline := func() { c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline }) }
			// baseAttrs 按目标类型读取当前接口值，得到基础值“attrs”。
			baseAttrs := proto.Clone(c.player().CombatAttributes).(*pb.CombatAttributes)
			// skillID 标识当前业务使用的技能。
			skillID := map[uint32]uint32{1: 1, 2: 2}[job]
			// skill 接收技能供后续步骤使用，known 保存由技能取得的已知状态。
			skill, known := cfg.Fixture.Skill(skillID)
			if !known {
				t.Fatal("缺职业原技能")
			}
			// book 记录技能书，使用道具观察视图表示，wood 记录“wood”，使用道具观察视图表示。
			var book, wood *pb.ItemView
			// item 接收遍历道具集合时当前的元素。
			for _, item := range c.inventory.Items {
				if item.ItemId == 218 && wood == nil {
					wood = item
				}
				if item.ItemId == skill.BookItemID {
					book = item
				}
			}
			if wood == nil || book == nil {
				t.Fatal("正常Create原武器或原书缺失")
			}
			// res 保存由资产取得的结果。
			res := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: book.InstanceId, SkillId: skillID, Operation: c.operation()})
			if res.ErrorCode != 0 {
				t.Fatal("资格等级后正常原书学习失败", res)
			}
			c.inventoryAt(res.NewRevision)
			baseline()
			res = c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: wood.InstanceId, ItemVersion: wood.Version, Slot: 0, Operation: c.operation()})
			if res.ErrorCode != 0 {
				t.Fatal(res)
			}
			c.inventoryAt(res.NewRevision)
			baseline()
			// attrs 保存战斗“attributes”，用于当前步骤中的“attrs”。
			attrs := c.player().CombatAttributes
			if attrs.MagicAttackMin != baseAttrs.MagicAttackMin+2 || attrs.MagicAttackMax != baseAttrs.MagicAttackMax+4 || attrs.SpiritAttackMin != baseAttrs.SpiritAttackMin+3 || attrs.SpiritAttackMax != baseAttrs.SpiritAttackMax+5 {
				t.Fatalf("完整baseline丢非零四端点: before=%v after=%v", baseAttrs, attrs)
			}
			// player 接收玩家供后续步骤使用。
			player := c.player()
			if job == 2 && player.Hp == player.HpMax {
				// f 记录协议帧。
				c.until(func(f wire.Frame) bool {
					return f.ID == p.MsgS2CSceneBaseline && c.player().Hp > 0 && c.player().Hp < c.player().HpMax
				})
				player = c.player()
			}
			// target 记录目标，使用实体状态表示。
			var target *pb.EntityState
			if job == 2 {
				target = player
			} else {
				// entity 接收遍历实体时当前的元素。
				for _, entity := range c.scene.Entities {
					if entity.EntityKind == 3 && entity.Hp > 0 {
						target = entity
						break
					}
				}
			}
			if target == nil || player.Hp <= 0 {
				t.Fatal("没有合法存活施法目标")
			}
			// accepted 记录已受理，使用施法已受理表示。
			var accepted pb.CastAccepted
			// request 保存由发送取得的请求。
			request := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, InputSequence: 1, SkillId: skillID, TargetHandle: target.EntityHandle})
			// f 记录协议帧。
			c.until(func(f wire.Frame) bool {
				if f.ID != p.MsgS2CCastAccepted {
					return false
				}
				must(t, proto.Unmarshal(f.Payload, &accepted))
				return accepted.InputSequence == 1 && accepted.SkillId == skillID
			})
			if accepted.CastId == 0 || accepted.CasterLife != player.Life || accepted.TargetLife != target.Life {
				t.Fatal("新施法没有原身体/目标身份", request, &accepted)
			}
			// impact 记录命中影响，使用战斗命中影响表示。
			var impact pb.CombatImpact
			// f 记录协议帧。
			c.until(func(f wire.Frame) bool {
				if f.ID != p.MsgS2CCombatImpact {
					return false
				}
				must(t, proto.Unmarshal(f.Payload, &impact))
				return impact.CastId == accepted.CastId && impact.SkillId == skillID
			})
			// low 保存魔法攻击最小，用于当前步骤中的较低值，high 保存魔法攻击最大，用于当前步骤中的“high”。
			low, high := attrs.MagicAttackMin, attrs.MagicAttackMax
			if job == 2 {
				low, high = attrs.SpiritAttackMin*skill.HealingStatMultiplier, attrs.SpiritAttackMax*skill.HealingStatMultiplier
			}
			// minimum 保存由来源伤害取得的“minimum”。
			minimum := skill.SourceDamage(skill.LearnedLevel, low, skill.SourcePower(skill.LearnedLevel, skill.MPowerBase, skill.PowerBase))
			// maximum 保存由来源伤害取得的最大值。
			maximum := skill.SourceDamage(skill.LearnedLevel, high, skill.SourcePower(skill.LearnedLevel, skill.MPowerBase+max(int64(0), skill.MPowerBonus-1), skill.PowerBase+max(int64(0), skill.PowerBonus-1)))
			if job == 2 && skill.AddCasterLevel {
				minimum += 7
				maximum += 7
			}
			if impact.Amount < minimum || impact.Amount > maximum || impact.TargetLife != target.Life || impact.Healing != (job == 2) || impact.Outcome != map[bool]uint32{false: 1, true: 4}[job == 2] {
				t.Fatalf("非零属性未走原实际技能路径: impact=%v expected=%d..%d", &impact, minimum, maximum)
			}
			if job == 2 {
				// previousHP 保存生命值，用于当前步骤中的上一步状态生命值。
				previousHP := c.player().Hp
				// f 记录协议帧。
				c.until(func(f wire.Frame) bool {
					if f.ID != p.MsgS2CSceneBaseline {
						return false
					}
					// current 保存由玩家取得的当前状态。
					current := c.player()
					if current.Hp <= 0 {
						t.Fatal("治疗观察前死亡")
					}
					// increased 保存根据生命值与上一步状态生命值计算的“increased”。
					increased := current.Hp > previousHP
					previousHP = current.Hp
					return increased
				})
			}
			// worn 保存道具，用于当前步骤中的“worn”。
			worn := c.inventory.Equipped[0].Item
			res = c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: worn.InstanceId, ItemVersion: worn.Version, Slot: 0, Unequip: true, Operation: c.operation()})
			if res.ErrorCode != 0 {
				t.Fatal(res)
			}
			c.inventoryAt(res.NewRevision)
			baseline()
			if len(c.inventory.Equipped) != 0 || !proto.Equal(c.player().CombatAttributes, baseAttrs) {
				t.Fatal("真正卸下没有恢复四端点基础")
			}
			t.Logf("隔离218四端点JSON往返/Validate，正常原件穿卸/原书学习与实际技能%d cast%d impact=%d（源公式%d..%d）；level7仅资格非自然成长，effective_fixture_sha=%s", skillID, accepted.CastId, impact.Amount, minimum, maximum, cfg.Fixture.Hash)
		})
	}
}

// TestEquipmentLegacyWearCanBeRemovedAndReplacedMongo 用旧格式穿戴前态验证卸下与替换兼容。
// t 提供断言失败报告、子用例和清理注册。
func TestEquipmentLegacyWearCanBeRemovedAndReplacedMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo旧穿戴物解除资格")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion < 23 {
		t.Fatal("必须使用已启用的装备合同")
	}
	// n 接收遍历道具集合时当前的元素。
	for n := range cfg.Fixture.Items {
		// i 保存当前处理所用的当前位置的下标。
		i := &cfg.Fixture.Items[n]
		if i.ID == 218 {
			i.RequiredLevel = new(uint32(19))
			i.EquipmentStage = 1
			i.AllowedJobs = []uint32{0}
		}
		// 同测试槽的合格替换件资格，仅复用已选布衣定义，不扩目录/数值/回收配方。
		if i.ID == 193 {
			i.EquipmentSlot = 0
		}
	}
	must(t, cfg.Fixture.Validate())
	cfg.Database = "minimir_dev_game_test_eqold_" + domain.ID()
	// ctx 保存由“background”取得的上下文。
	ctx := context.Background()
	// authority 保存由打开取得的权限围栏。
	authority, err := persistence.Open(ctx, cfg)
	must(t, err)
	// owner 保存由打开取得的所有者。
	owner, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, authority.DB.Drop(cleanup))
		owner.Close(cleanup)
		authority.Close(cleanup)
	})
	must(t, authority.Singleton(ctx, "account"))
	// grant 保存由批准游戏取得的授予。
	grant, err := authority.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
	must(t, err)
	owner.Epoch = grant.Epoch
	// login 接收登录供后续步骤使用。
	login, err := authority.Login(ctx, "eq_old_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	// account 保存由“authenticate”取得的账号。
	account, err := authority.Authenticate(ctx, login.AccountToken)
	must(t, err)
	// replace 接收遍历布尔状态列表时当前的元素。
	for _, replace := range []bool{false, true} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(map[bool]string{false: "卸下", true: "替换"}[replace], func(t *testing.T) {
			// slot 保存由无符号整数“32”取得的槽位。
			slot := uint32(1)
			if replace {
				slot = 2
			}
			// key 保存由创建角色取得的键，e 接收创建角色供后续步骤使用。
			key, e := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: 1, Name: "旧装解除" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())})
			must(t, e)
			// legacy 保存由角色取得的兼容旧数据。
			legacy, e := authority.Role(ctx, domain.Hex(key.Key.RoleId))
			must(t, e)
			// old 记录原状态，使用道具表示。
			var old domain.Item
			// replacement 记录“replacement”，使用道具表示。
			var replacement domain.Item
			// items 创建用于维护道具集合的容器或通道。
			items := make([]domain.Item, 0, len(legacy.Items))
			// item 接收遍历道具集合时当前的元素。
			for _, item := range legacy.Items {
				if item.ItemID == 218 && old.ID == "" {
					old = item
					continue
				}
				items = append(items, item)
				if item.ItemID == 193 {
					replacement = item
				}
			}
			if old.ID == "" || replacement.ID == "" {
				t.Fatal("旧存档资格缺正常Create原件")
			}
			old.Version, old.Quality, old.Locked = 3, 4, true
			// 明确隔离旧存档：仅首次Enter前安放原已穿戴物。它不符合新定义的level19/战士，但不伪装成一次正常新穿。
			_, e = authority.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": legacy.ID}, bson.M{"$set": bson.M{"items": items, "equipped": []domain.Equipped{{Slot: 0, Item: old}}}})
			must(t, e)
			// ticket 保存由签发入场票据取得的入场票据。
			ticket, e := authority.IssueTicket(ctx, account, key.Key)
			must(t, e)
			// r 接收入场供后续步骤使用，auth 保存由入场取得的授权。
			r, auth, e := owner.Enter(ctx, ticket.Ticket)
			must(t, e)
			// makeRequest 封装“make”请求对应的后续处理，id 区分当前步骤指向的目标对象，version 记录版本，使用数值表示，unequip 标记是否处于“unequip”状态。
			makeRequest := func(id string, version uint64, unequip bool) persistence.AssetRequest {
				// q 组装资产请求所需的数据。
				q := persistence.AssetRequest{Kind: "equip", ItemID: id, ItemVersion: version, Slot: 0, Unequip: unequip, Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(r.EconomicSeq + 1), ExpectedRevision: uint64(r.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}}
				// data 保存由序列化取得的数据，e 接收序列化供后续步骤使用。
				data, e := json.Marshal(q)
				must(t, e)
				q.Fingerprint = persistence.Hash(data)
				return q
			}
			// q 接收“make”请求供后续步骤使用。
			q := makeRequest(old.ID, 3, true)
			if replace {
				q = makeRequest(replacement.ID, uint64(replacement.Version), false)
			}
			// before 保存当前处理所用的操作前的状态。
			before := r
			// out 保存由资产带有回执取得的输出，result 保存由资产带有回执取得的结果，op 保存由资产带有回执取得的操作。
			out, result, op, e := owner.AssetWithReceipt(ctx, auth, q)
			must(t, e)
			r = out
			if result.ErrorCode != 0 || op.Equipment == nil || r.Wallet != before.Wallet || r.SourceSeq != before.SourceSeq || r.Revision != before.Revision+1 || r.EconomicSeq != before.EconomicSeq+1 {
				t.Fatal("旧等级/职业/合法旧品质被困住或永久水位错误", result)
			}
			// returned 记录已返回值，使用道具表示。
			var returned domain.Item
			// item 接收遍历道具集合时当前的元素。
			for _, item := range r.Items {
				if item.ID == old.ID {
					returned = item
				}
			}
			// wanted 保存当前处理所用的期望状态。
			wanted := old
			wanted.Version++
			if !reflect.DeepEqual(returned, wanted) {
				t.Fatal("解除旧穿戴改变原ID/品质/绑定/来源/锁")
			}
			before = r
			// rejected 保存由资产带有回执取得的拒绝。
			out, result, rejected, e := owner.AssetWithReceipt(ctx, auth, makeRequest(old.ID, uint64(returned.Version), false))
			must(t, e)
			r = out
			if result.ErrorCode != p.CodeEquipmentChanged || rejected.Equipment != nil || !reflect.DeepEqual(r.Items, before.Items) || !reflect.DeepEqual(r.Equipped, before.Equipped) {
				t.Fatal("旧物解除后获得不符合当前职业/等级/品质的新穿资格")
			}
			before = r
			// replayed 保存由资产带有回执取得的“replayed”。
			out, result, replayed, e := owner.AssetWithReceipt(ctx, auth, q)
			must(t, e)
			if result.ErrorCode != 0 || !reflect.DeepEqual(out, before) || !reflect.DeepEqual(replayed.Equipment, op.Equipment) {
				t.Fatal("旧解除回执重放再次移动或被当前新穿资格拒绝")
			}
			t.Logf("明确旧存档资格原件%s quality4/version3：解除或替换→原样返包v4；当前level1法师不满足新level19战士定义，重新穿入仍3402；未扩主目录或回收资格", old.ID)
		})
	}
}
