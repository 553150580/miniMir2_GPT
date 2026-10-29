package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func TestAutoRecyclePermanentReceiptsProtectionAndSourceMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo回收事务验收")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion < 19 {
		t.Fatal("自动回收必须使用当前rule19或以上完整配置")
	}
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	ctx := context.Background()
	authority, err := persistence.Open(ctx, cfg)
	must(t, err)
	owner, err := persistence.Open(ctx, cfg)
	must(t, err)
	next, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, authority.DB.Drop(cleanup))
		next.Close(cleanup)
		owner.Close(cleanup)
		authority.Close(cleanup)
	})
	must(t, authority.Singleton(ctx, "account"))
	grant, err := authority.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
	must(t, err)
	owner.Epoch = grant.Epoch
	login, err := authority.Login(ctx, "recycle_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	account, err := authority.Authenticate(ctx, login.AccountToken)
	must(t, err)
	summary, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 0, Name: "回收事务" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())})
	must(t, err)
	enter := func(store *persistence.Store) (domain.Role, persistence.Auth) {
		ticket, err := authority.IssueTicket(ctx, account, summary.Key)
		must(t, err)
		r, a, err := store.Enter(ctx, ticket.Ticket)
		must(t, err)
		return r, a
	}
	r, auth := enter(owner)
	if r.AutoRecycle.Enabled || r.AutoRecycle.Revision != 0 {
		t.Fatal("新角色策略未默认关闭")
	}
	var wood []domain.Item
	for _, item := range r.Items {
		if item.ItemID == 218 {
			wood = append(wood, item)
		}
	}
	if len(wood) != 2 {
		t.Fatal("当前开发初始物品前态已变更，不能伪造实例")
	}
	asset := func(q persistence.AssetRequest) (*pb.OperationResult, persistence.Operation) {
		must(t, authority.Renew(ctx, "account"))
		must(t, owner.Renew(ctx, "game"))
		q.Context = &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(r.EconomicSeq + 1), ExpectedRevision: uint64(r.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}
		raw, err := json.Marshal(q)
		must(t, err)
		q.Fingerprint = persistence.Hash(raw)
		out, result, receipt, err := owner.AssetWithReceipt(ctx, auth, q)
		must(t, err)
		r = out
		return result, receipt
	}
	result, _ := asset(persistence.AssetRequest{Kind: "claim_hosting"})
	if result.ErrorCode != 0 {
		t.Fatal(result)
	}
	result, _ = asset(persistence.AssetRequest{Kind: "set_item_lock", ItemID: wood[0].ID, ItemVersion: uint64(wood[0].Version), Locked: true})
	if result.ErrorCode != 0 {
		t.Fatal(result)
	}
	before := r
	result, _ = asset(persistence.AssetRequest{Kind: "recycle", ItemIDs: []string{wood[0].ID, wood[1].ID}})
	if result.ErrorCode == 0 || !reflect.DeepEqual(r.Items, before.Items) || r.Wallet != before.Wallet || r.SourceSeq != before.SourceSeq {
		t.Fatal("手动批回收绕过锁或部分扣物")
	}
	policy := &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0, 1, 2, 3, 4}, ProtectedItemIds: []uint32{193}}
	result, _ = asset(persistence.AssetRequest{Kind: "set_auto_recycle", AutoRecycle: policy})
	if result.ErrorCode != 0 || r.AutoRecycle.Revision != 1 || !reflect.DeepEqual(r.Items, before.Items) || r.SourceSeq != 0 {
		t.Fatal("未来策略保存偷偷扫包或消耗SourceSeq")
	}
	policy.Revision = 1
	before = r
	q := persistence.AssetRequest{Kind: "set_auto_recycle", AutoRecycle: policy, ApplyExisting: true, Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(r.EconomicSeq + 1), ExpectedRevision: uint64(r.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}}
	raw, err := json.Marshal(q)
	must(t, err)
	q.Fingerprint = persistence.Hash(raw)
	r, result, receipt, err := owner.AssetWithReceipt(ctx, auth, q)
	must(t, err)
	if result.ErrorCode != 0 || len(r.Items) != len(before.Items)-1 || r.AutoRecycle.Revision != 2 || r.SourceSeq != before.SourceSeq || r.EconomicSeq != before.EconomicSeq+1 || r.Revision != before.Revision+1 || receipt.Recycling == nil || len(receipt.Recycling.Recycled) != 1 || receipt.Recycling.Recycled[0].ID != wood[1].ID || receipt.Recycling.BoundGold != 50 || receipt.AutoRecyclePolicy == nil || !reflect.DeepEqual(*receipt.AutoRecyclePolicy, r.AutoRecycle) {
		t.Fatal("显式应用并非同一经济事务", r, receipt)
	}
	var policyAudit struct {
		Policy domain.AutoRecyclePolicy `bson:"auto_recycle_policy"`
	}
	must(t, owner.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": receipt.ID}).Decode(&policyAudit))
	if !reflect.DeepEqual(policyAudit.Policy, r.AutoRecycle) {
		t.Fatal("设置审计未保存实际新策略全文")
	}
	replayed, replay, replayReceipt, err := owner.AssetWithReceipt(ctx, auth, q)
	must(t, err)
	if replay.ErrorCode != 0 || replayed.Revision != r.Revision || replayed.Wallet != r.Wallet || !reflect.DeepEqual(receipt.Recycling, replayReceipt.Recycling) {
		t.Fatal("旧设置原op重试重复扣物或丢原回执")
	}
	changed := q
	changed.Fingerprint = "changed-original-payload"
	if _, _, _, err := owner.AssetWithReceipt(ctx, auth, changed); persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("同op内容变化未拒绝", err)
	}
	before = r
	item := domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: fmt.Sprint(r.SourceSeq + 1)}
	settlement := persistence.Settlement{SourceSeq: r.SourceSeq + 1, Kills: 1, Experience: 15, Gold: 30, Bound: true, Item: &item, HostSeconds: 1, HostingSession: domain.ID(), EventAt: time.Now().UTC(), TargetHandle: 10, TargetLife: 9, CasterHandle: 1}
	r, receipt, err = owner.SettleWithReceipt(ctx, auth, settlement)
	must(t, err)
	if r.SourceSeq != before.SourceSeq+1 || r.EconomicSeq != before.EconomicSeq || r.Revision != before.Revision+1 || r.HostingSeconds != before.HostingSeconds-1 || r.Wallet.Gold != before.Wallet.Gold+50 || r.Wallet.BoundGold != before.Wallet.BoundGold+30 || len(r.Items) != len(before.Items) || r.Report.RecycleCount != 1 || r.Report.RecycleGold != 50 || r.Report.TotalDropCount != 1 || len(r.Report.Drops) != 0 || r.SettlementReceiptStartSeq != settlement.SourceSeq {
		t.Fatal("来源原子结算或逐原件绑定错误", r, receipt)
	}
	if receipt.Settlement == nil || receipt.Settlement.ConfigHash != cfg.Fixture.Hash || receipt.Recycling.PolicyRevision != 2 || receipt.Recycling.Recycled[0].ID != item.ID || receipt.Recycling.Recycled[0].Bound || receipt.Recycling.Policy == nil || !reflect.DeepEqual(*receipt.Recycling.Policy, r.AutoRecycle) {
		t.Fatal("永久回执未冻结原件/策略/配置", receipt)
	}
	replayed, replayReceipt, err = owner.SettleWithReceipt(ctx, auth, settlement)
	must(t, err)
	if replayed.Revision != r.Revision || replayed.Wallet != r.Wallet || !reflect.DeepEqual(replayReceipt.Recycling, receipt.Recycling) || replayed.Report.Sequence != r.Report.Sequence {
		t.Fatal("原来源重放重复回收/报告")
	}
	changedSettlement := settlement
	changedSettlement.Gold++
	if _, _, err := owner.SettleWithReceipt(ctx, auth, changedSettlement); persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("同source变化未拒绝", err)
	}
	opRaw, err := owner.DB.Collection("operations").FindOne(ctx, bson.M{"_id": receipt.ID}).Raw()
	must(t, err)
	_, err = owner.DB.Collection("operations").DeleteOne(ctx, bson.M{"_id": receipt.ID})
	must(t, err)
	if _, _, err := owner.SettleWithReceipt(ctx, auth, settlement); persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("新回执丢失被伪装为legacy水位", err)
	}
	_, err = owner.DB.Collection("operations").InsertOne(ctx, opRaw)
	must(t, err)
	for _, invalid := range []int64{-1, r.SourceSeq + 1} {
		_, err = owner.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$set": bson.M{"settlement_receipt_start_seq": invalid}})
		must(t, err)
		if _, _, err := owner.SettleWithReceipt(ctx, auth, settlement); persistence.Code(err) != p.CodeOperationConflict {
			t.Fatal("回执界限无效仍接受", invalid, err)
		}
	}
	_, err = owner.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$set": bson.M{"settlement_receipt_start_seq": r.SettlementReceiptStartSeq}})
	must(t, err)
	// 单独角色的可达极限水位；只验证守卫，不伪造负数后继续业务。
	limitSummary, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 2, Job: 0, Name: "水位极限" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())})
	must(t, err)
	limitTicket, err := authority.IssueTicket(ctx, account, limitSummary.Key)
	must(t, err)
	limitRole, limitAuth, err := owner.Enter(ctx, limitTicket.Ticket)
	must(t, err)
	_, err = owner.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": limitRole.ID}, bson.M{"$set": bson.M{"revision": int64(math.MaxInt64), "economic_command_seq": int64(math.MaxInt64)}})
	must(t, err)
	for _, kind := range []string{"set_auto_recycle", "set_item_lock"} {
		request := persistence.AssetRequest{Kind: kind, Fingerprint: kind + "-limit", AutoRecycle: &pb.AutoRecyclePolicy{}, ItemID: limitRole.Items[0].ID, ItemVersion: 1, Locked: true, Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(math.MaxInt64) + 1, ExpectedRevision: uint64(math.MaxInt64), ExpectedRuleVersion: cfg.Fixture.RuleVersion}}
		if _, _, _, err := owner.AssetWithReceipt(ctx, limitAuth, request); persistence.Code(err) != p.CodeOperationConflict {
			t.Fatal("资产水位溢出未先拒绝", kind, err)
		}
	}
	limited, err := owner.Role(ctx, limitRole.ID)
	must(t, err)
	if limited.Revision != math.MaxInt64 || limited.EconomicSeq != math.MaxInt64 || limited.Items[0].Version != 1 || limited.Items[0].Locked || limited.AutoRecycle.Revision != 0 {
		t.Fatal("极限水位请求留下部分变更")
	}
	must(t, owner.Release(ctx, "game"))
	grant, err = authority.ApproveGame(ctx, cfg.NodeID, next.Instance, cfg.RunID)
	must(t, err)
	next.Epoch = grant.Epoch
	cold, nextAuth := enter(next)
	if !reflect.DeepEqual(cold.AutoRecycle, r.AutoRecycle) || !cold.Items[0].Locked || cold.Items[0].ID != wood[0].ID || cold.Wallet != r.Wallet {
		t.Fatal("冷owner未保留策略/锁/实际收益")
	}
	if _, _, err := owner.SettleWithReceipt(ctx, auth, persistence.Settlement{SourceSeq: r.SourceSeq + 1}); persistence.Code(err) != p.CodeRouteChanged {
		t.Fatal("旧owner仍可写回收", err)
	}
	replayed, replayReceipt, err = next.SettleWithReceipt(ctx, nextAuth, settlement)
	must(t, err)
	if replayed.Revision != cold.Revision || replayed.Wallet != cold.Wallet || !reflect.DeepEqual(replayReceipt.Recycling, receipt.Recycling) {
		t.Fatal("冷owner重放未使用原资产结果")
	}
	count, err := owner.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": receipt.ID})
	must(t, err)
	if count != 1 {
		t.Fatal("原来源审计并非唯一", count)
	}
	t.Logf("真实事务通过：策略revision=%d，EconomicSeq=%d，SourceSeq=%d；原bound保留、原回执重查、typed receipt防缺失、冷owner旧围栏", cold.AutoRecycle.Revision, cold.EconomicSeq, cold.SourceSeq)
}

func TestAutoRecyclePolicyLockAndOnlineOfflineMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo/WS自动回收闭环")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion < 19 {
		t.Fatal("自动回收要求当前rule19以上配置")
	}
	// 仅本次随机库的容量/战斗前态：正常Create产生30木剑+布衣+职业书填满32格，怪物1HP/0攻/0敏。
	// 正常注册/创角/拾取/所有设置仍经实际协议，不向角色背包注入实例或操作结果。
	cfg.Fixture.InitialInventory = make([]uint32, 31)
	for i := range cfg.Fixture.InitialInventory {
		cfg.Fixture.InitialInventory[i] = 218
	}
	cfg.Fixture.InitialInventory[30] = 193
	cfg.Fixture.Drops.EquipmentItemIDs = []uint32{218}
	cfg.Fixture.Monsters[0].HP = 1
	cfg.Fixture.Monsters[0].AttackMin = 0
	cfg.Fixture.Monsters[0].AttackMax = 0
	cfg.Fixture.Monsters[0].Agility = 0
	raw, err := json.Marshal(cfg.Fixture)
	must(t, err)
	cfg.Fixture.Hash = persistence.Hash(raw)
	must(t, cfg.Fixture.Validate())
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	ctx, cancel := context.WithCancel(context.Background())
	var group sync.WaitGroup
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	for _, mode := range []string{"account", "game", "gm"} {
		group.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("自动回收%s节点:%v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		group.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, a)
	}
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "recycle_ws_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	role := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 0, Name: "自动回收" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	var events []*pb.EquipmentRecycleResult
	phase := "enter"
	attacks, positiveImpacts := 0, 0
	pendingAttacks := map[uint32]bool{}
	observedDeaths := map[string]bool{}
	lastExperienceSource := uint64(0)
	observe := func(c *client, f wire.Frame) {
		if f.Error == 0 && f.ID == p.MsgS2CSceneBaseline && c.scene != nil {
			for _, body := range c.scene.Entities {
				if body.EntityKind == 3 && body.Hp == 0 {
					observedDeaths[fmt.Sprintf("%d:%d", body.EntityHandle, body.Life)] = true
				}
			}
		}
		if f.Error == 0 && f.ID == p.MsgS2CExperienceGrantResult {
			var grant pb.ExperienceGrantResult
			must(t, proto.Unmarshal(f.Payload, &grant))
			if grant.RawExperience > 0 && grant.SourceSeq > lastExperienceSource {
				lastExperienceSource = grant.SourceSeq
			}
		}
		if f.Error == 0 && f.ID == p.MsgS2CAttackAccepted {
			attacks++
			var attack pb.AttackAccepted
			must(t, proto.Unmarshal(f.Payload, &attack))
			if attack.CasterHandle == c.entry.GetPlayerHandle() && attack.TargetHandle != 0 {
				if len(pendingAttacks) >= cfg.Fixture.CombatTiming.EventCapacity {
					t.Fatal("有效目标攻击超过原事件容量")
				}
				pendingAttacks[attack.AttackSequence] = true
			}
		}
		if f.Error == 0 && f.ID == p.MsgS2CCombatImpact {
			var hit pb.CombatImpact
			must(t, proto.Unmarshal(f.Payload, &hit))
			if hit.CasterHandle == c.entry.GetPlayerHandle() && hit.AttackSequence != 0 {
				delete(pendingAttacks, hit.AttackSequence)
			}
			if hit.Amount > 0 {
				positiveImpacts++
			}
		}
		if f.Error == 0 && f.ID == p.MsgS2CEquipmentRecycleResult {
			var x pb.EquipmentRecycleResult
			must(t, proto.Unmarshal(f.Payload, &x))
			if c.inventory == nil || c.inventory.Revision < x.RoleRevision || !reflect.DeepEqual(x.Role.RoleId, role.Key.RoleId) {
				t.Fatal("回收结果先于完整Inventory或不属当前角色", &x)
			}
			events = append(events, &x)
		}
	}
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), observe)
	defer func() { c.close() }()
	stopAutoAndDrain := func() {
		c.auto(false, 8)
		for len(pendingAttacks) > 0 {
			c.read() // 有效目标攻击包括miss/cancel终态；空挥未加入集合。
		}
		// Impact早于事件批末的死亡基线；先用同请求基线越过此前全部帧，
		// 不把该请求本身当永久Pending屏障。
		request := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, CancelNavigation: true})
		c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && f.Request == request })
		// 此段新角色无施法/绑定来源，在线SourceSeq仅为实际kill；以刚刚
		// 完整观察到的原怪死亡数等EXP确认，不在命中后猜永久Pending已完成。
		for lastExperienceSource < uint64(len(observedDeaths)) {
			c.read()
		}
		if len(pendingAttacks) != 0 {
			t.Fatal("Auto已关闭并收束后新增攻击")
		}
	}
	defer func() {
		if t.Failed() {
			stored, e := inspection.Role(context.Background(), roleID)
			t.Logf("失败观察 phase=%s attacks=%d positive_impacts=%d recycle_events=%d player=%v hosting=%v inventory_revision=%d bag=%d policy=%v；永久role_read_error=%v rev=%d source=%d econ=%d no_equipment_kills=%d pickup_stopped=%t bag=%d report=%+v", phase, attacks, positiveImpacts, len(events), c.player(), c.hosting, c.inventory.GetRevision(), len(c.inventory.GetItems()), c.inventory.GetAutoRecycle(), e, stored.Revision, stored.SourceSeq, stored.EconomicSeq, stored.NoEquipmentKills, stored.PickupStopped, len(stored.Items), stored.Report)
		}
	}()
	c.auto(false, 8)
	if c.inventory.AutoRecycle == nil || c.inventory.AutoRecycle.Enabled || c.inventory.AutoRecycle.Revision != 0 || int(c.inventory.Capacity) != len(c.inventory.Items) {
		t.Fatal("正常新角色策略/满包前态错误", c.inventory)
	}
	// 来源level1战士空手DC=0，正常装备原木剑才具备杀怪前态；不改最低伤害。
	phase = "equip_original_wood"
	wornID := append([]byte(nil), c.inventory.Items[0].InstanceId...)
	res := c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: wornID, Slot: 0, Operation: c.operation()})
	if res.ErrorCode != 0 {
		t.Fatal(res)
	}
	c.inventoryAt(res.NewRevision)
	if len(c.inventory.Equipped) != 1 || !reflect.DeepEqual(c.inventory.Equipped[0].Item.InstanceId, wornID) || c.inventory.Equipped[0].Item.RecycleBlockFlags&8 == 0 {
		t.Fatal("正常原件未穿戴或未投影强保护")
	}
	phase = "claim_hosting"
	res = c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
	if res.ErrorCode != 0 {
		t.Fatal(res)
	}
	c.inventoryAt(res.NewRevision)
	untilResult := func(predicate func(*pb.EquipmentRecycleResult) bool) *pb.EquipmentRecycleResult {
		for {
			for _, x := range events {
				if predicate(x) {
					return x
				}
			}
			c.read()
		}
	}
	c.auto(true, 8)
	phase = "default_policy_wait_actual_full_discard"
	discard := untilResult(func(x *pb.EquipmentRecycleResult) bool { return x.Trigger == 1 && x.DiscardedCount == 1 })
	full, err := inspection.Role(ctx, roleID)
	must(t, err)
	if !full.PickupStopped || full.SourceSeq < int64(discard.SourceSeq) || discard.DiscardReason != "bag_full" || discard.RecycledCount != 0 || c.hosting == nil || !c.hosting.AutoBattleEnabled {
		t.Fatal("默认关闭未真实弃物，或满包关闭了Auto", full, discard)
	}
	phase = "full_bag_continues_actual_kill_experience"
	continued := c.until(func(f wire.Frame) bool {
		if f.ID != p.MsgS2CExperienceGrantResult || f.Error != 0 {
			return false
		}
		var grant pb.ExperienceGrantResult
		must(t, proto.Unmarshal(f.Payload, &grant))
		return grant.SourceSeq > discard.SourceSeq && grant.AcceptedExperience > 0
	})
	_ = continued
	stopAutoAndDrain()
	full, err = inspection.Role(ctx, roleID)
	must(t, err)
	if !full.PickupStopped || full.SourceSeq <= int64(discard.SourceSeq) || c.hosting.AutoBattleEnabled {
		t.Fatal("首弃物后未继续原kill/EXP，或正常关闭Auto失效", full)
	}
	// 仍未腾格，使用正常意图重新开启，必须成功且再产生真实kill确认。
	beforeRestart := full.SourceSeq
	c.auto(true, 8)
	c.until(func(f wire.Frame) bool {
		if f.ID != p.MsgS2CExperienceGrantResult || f.Error != 0 {
			return false
		}
		var grant pb.ExperienceGrantResult
		must(t, proto.Unmarshal(f.Payload, &grant))
		return grant.SourceSeq > uint64(beforeRestart) && grant.AcceptedExperience > 0
	})
	stopAutoAndDrain()
	full, err = inspection.Role(ctx, roleID)
	must(t, err)
	var wood []*pb.ItemView
	for _, item := range c.inventory.Items {
		if item.ItemId == 218 {
			wood = append(wood, item)
		}
	}
	if len(wood) < 29 || len(wood) > 30 {
		t.Fatal("正常穿戴/首件实际拾取后，容量夹具木剑数错误", len(wood))
	}
	expectedSweep := uint64(len(wood) - 1)
	phase = "lock_original_and_save_future_policy"
	lock := &pb.SetItemLockRequest{ItemInstanceId: wood[0].InstanceId, ItemVersion: wood[0].Version, Locked: true, Operation: c.operation()}
	res = c.asset(p.MsgC2SSetItemLock, lock)
	if res.ErrorCode != 0 {
		t.Fatal(res)
	}
	c.inventoryAt(res.NewRevision)
	policy := &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0}, ProtectedItemIds: []uint32{193}}
	res = c.asset(p.MsgC2SSetAutoRecycle, &pb.SetAutoRecycleRequest{Policy: policy, Operation: c.operation()})
	if res.ErrorCode != 0 {
		t.Fatal(res)
	}
	c.inventoryAt(res.NewRevision)
	unchanged, err := inspection.Role(ctx, roleID)
	must(t, err)
	if !unchanged.PickupStopped || unchanged.SourceSeq != full.SourceSeq || len(unchanged.Items) != len(full.Items) || unchanged.AutoRecycle.Revision != 1 {
		t.Fatal("保存未来策略暗中扫包或启动来源事件")
	}
	policy = proto.Clone(c.inventory.AutoRecycle).(*pb.AutoRecyclePolicy)
	phase = "explicit_apply_existing"
	apply := &pb.SetAutoRecycleRequest{Policy: policy, ApplyExisting: true, Operation: c.operation()}
	res = c.asset(p.MsgC2SSetAutoRecycle, apply)
	if res.ErrorCode != 0 {
		t.Fatal(res)
	}
	c.inventoryAt(res.NewRevision)
	applied := untilResult(func(x *pb.EquipmentRecycleResult) bool { return x.Trigger == 2 && x.RoleRevision == res.NewRevision })
	released, err := inspection.Role(ctx, roleID)
	must(t, err)
	if released.PickupStopped || c.hosting.AutoBattleEnabled || len(released.Items) != len(full.Items)-int(expectedSweep) || applied.RecycledCount != expectedSweep || applied.BoundGold != int64(expectedSweep)*50 || applied.Gold != 0 || released.SourceSeq != full.SourceSeq || released.AutoRecycle.Revision != 2 {
		t.Fatal("显式应用没有只回收非锁定原件/恢复拾取/保持Auto关闭", released, applied)
	}
	if len(released.Equipped) != 1 || released.Equipped[0].Item.ID != domain.Hex(wornID) {
		t.Fatal("自动回收消耗了穿戴原件")
	}
	for _, item := range c.inventory.Items {
		if reflect.DeepEqual(item.InstanceId, wood[0].InstanceId) && (!item.Locked || item.RecycleBlockFlags&1 == 0) {
			t.Fatal("锁定状态未投影")
		}
	}
	beforeSource := released.SourceSeq
	// 此时Auto仍关闭；正常健康票重入续同一现场，并给下一段完整观察预算。
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), observe)
	c.auto(true, 8)
	phase = "online_wait_normal_equipment_or_20_kill_guarantee"
	online := untilResult(func(x *pb.EquipmentRecycleResult) bool {
		return x.Trigger == 1 && x.RecycledCount == 1 && x.SourceSeq > uint64(beforeSource)
	})
	onlineRole, err := inspection.Role(ctx, roleID)
	must(t, err)
	if online.Gold+online.BoundGold != 50 || onlineRole.Report.Sequence != 0 || onlineRole.PickupStopped {
		t.Fatal("在线直接回收未使用原规则或污染离线报告", online, onlineRole)
	}
	beforeLife := c.player().Life
	c.close()
	phase = "offline_wait_normal_equipment_or_20_kill_guarantee"
	// 保持原65/20/10/5与20杀保底、原动作和10秒复活；这是测试整体观察上限，不改业务deadline。
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(30 * time.Millisecond)
	defer poll.Stop()
	var offline domain.Role
waiting:
	for {
		select {
		case <-deadline.C:
			t.Fatal("整体90秒观察内未观察原规则离线击杀与保底回收")
		case <-poll.C:
			offline, err = inspection.Role(ctx, roleID)
			must(t, err)
			if offline.Report.Kills > 0 && offline.Report.RecycleCount > 0 {
				break waiting
			}
		}
	}
	if offline.Report.UsedSeconds <= 0 || offline.Report.TotalDropCount > offline.Report.Kills || offline.Report.TotalDropCount < offline.Report.RecycleCount || offline.Report.DiscardCount != 0 || offline.Report.RecycleGold+offline.Report.RecycleBoundGold != 50*offline.Report.RecycleCount || offline.Report.Earned.Gold < offline.Report.RecycleGold || offline.Report.Earned.BoundGold < offline.Report.RecycleBoundGold || offline.PickupStopped {
		t.Fatal("离线未逐事件共用回收或报告把样本当总量", offline.Report)
	}
	// 按该原HostingSession和已提交SourceSeq截面重算真实审计，不能把kill数猜成物品数。
	cursor, err := inspection.DB.Collection("audit_outbox").Find(ctx, bson.M{"role_id": roleID, "kind": "settlement", "hosting_session": offline.Report.SessionID, "source_seq": bson.M{"$lte": offline.SourceSeq}})
	must(t, err)
	var rows []struct {
		Kills       int64                         `bson:"kills"`
		Experience  int64                         `bson:"experience"`
		Gold        int64                         `bson:"gold"`
		Bound       bool                          `bson:"bound"`
		Item        *domain.Item                  `bson:"item"`
		HostSeconds int64                         `bson:"host_seconds"`
		Details     persistence.SettlementReceipt `bson:"settlement"`
	}
	must(t, cursor.All(ctx, &rows))
	expected := domain.Report{}
	for _, row := range rows {
		expected.Kills += row.Kills
		expected.Experience += row.Experience
		expected.UsedSeconds += row.HostSeconds
		expected.Sequence++
		if row.Item != nil {
			expected.TotalDropCount++
		}
		rec := row.Details.Recycling
		expected.RecycleCount += int64(len(rec.Recycled))
		expected.RecycleGold += rec.Gold
		expected.RecycleBoundGold += rec.BoundGold
		expected.DiscardCount += rec.DiscardedCount
		expected.Earned.Gold += rec.Gold
		expected.Earned.BoundGold += rec.BoundGold
		if row.Bound {
			expected.Earned.BoundGold += row.Gold
		} else {
			expected.Earned.Gold += row.Gold
		}
		if row.Details.GrantedItem {
			expected.Drops = append(expected.Drops, *row.Item)
		}
	}
	got := offline.Report
	if got.Kills != expected.Kills || got.Experience != expected.Experience || got.UsedSeconds != expected.UsedSeconds || got.Sequence != expected.Sequence || got.TotalDropCount != expected.TotalDropCount || got.RecycleCount != expected.RecycleCount || got.RecycleGold != expected.RecycleGold || got.RecycleBoundGold != expected.RecycleBoundGold || got.DiscardCount != expected.DiscardCount || got.Earned != expected.Earned || len(got.Drops) != len(expected.Drops) {
		t.Fatal("战报不等于原来源已提交审计截面", got, expected)
	}
	previousEvents := len(events)
	phase = "healthy_reentry_no_historical_replay"
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), observe)
	c.auto(false, 8)
	if c.player().Life != beforeLife || c.inventory.AutoRecycle.Revision != 2 || c.report.RecycleCount < uint64(offline.Report.RecycleCount) {
		t.Fatal("健康重入丢策略/生命/报告或补播历史回收")
	}
	for _, event := range events[previousEvents:] {
		if event.SourceSeq <= uint64(offline.SourceSeq) {
			t.Fatal("健康重入补播旧回收", event)
		}
	}
	healthyRole, err := inspection.Role(ctx, roleID)
	must(t, err)
	res = c.asset(p.MsgC2SSetItemLock, lock)
	if res.ErrorCode != 0 {
		t.Fatal("原锁op健康票重放失败", res)
	}
	c.inventoryAt(res.NewRevision)
	final, err := inspection.Role(ctx, roleID)
	must(t, err)
	if final.AutoRecycle.Revision != 2 || final.Items[0].Version != 2 || !final.Items[0].Locked || final.Wallet != healthyRole.Wallet {
		t.Fatal("旧锁回执改变新策略/实例/钱包")
	}
	phase = "done"
	t.Logf("正常WS通过：默认满包真丢弃→正常锁定/保存/应用→在线回收→离线%d杀%d回收/总掉物%d→新票保持；policy=%d SourceSeq=%d EconomicSeq=%d", offline.Report.Kills, offline.Report.RecycleCount, offline.Report.TotalDropCount, final.AutoRecycle.Revision, final.SourceSeq, final.EconomicSeq)
}
