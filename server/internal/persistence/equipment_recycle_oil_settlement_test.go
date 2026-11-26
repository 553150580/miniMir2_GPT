// 本文件实测原装备、分解副产物、原账本和合法油瓶流通在 MongoDB 中的一致性。
package persistence

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestRecycleOilSettlementMongo 用原工作簿分母核对成功与失败、满包、破损、绑定与去重。
func TestRecycleOilSettlementMongo(t *testing.T) {
	ctx, account, game, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_RECYCLE_OIL_TEST_FIXTURE")
	fixture := &game.Cfg.Fixture
	if fixture.EquipmentRecycleOil == nil || fixture.EquipmentRecycleOil.Denominator != 30 || fixture.LuckOil == nil || fixture.ItemMarket == nil {
		t.Fatal("隔离夹具缺少分解正式概率分母、祝福油或原道具市场")
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	wood, known := fixture.Item(218)
	if !known || !wood.IsEquipment || wood.OriginalDurabilityU <= 0 || wood.RecycleGold <= 0 {
		t.Fatal("验收需要有真实持久值及原分解价的武器", wood)
	}
	// 用一批已到 +9 的隔离材料同时观察原 30% 的命中和失败。其余快照仍由实际角色事务处理。
	const eligible = 160
	items := make([]domain.Item, 0, eligible+4)
	selected := make([]string, 0, eligible+3)
	originals := make(map[string]domain.Item, eligible)
	for index := range eligible {
		item := InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: wood.ID, Quantity: 1, Version: 4, Bound: index%2 == 0,
			InitialLuck: 2, CurrentLuck: 9, LuckOilToSix: 103, LuckOilToNine: 3002, Quality: wood.DropQuality,
			SourceKind: "monster_drop", SourceID: "1"}, fixture)
		items = append(items, item)
		selected = append(selected, item.ID)
		originals[item.ID] = item
	}
	broken := items[0]
	broken.ID, broken.DurabilityU = domain.ID(), 0
	legacyBroken := broken
	legacyBroken.ID, legacyBroken.DurabilityU = domain.ID(), -1
	ordinary := items[1]
	ordinary.ID, ordinary.InitialLuck, ordinary.CurrentLuck, ordinary.LuckOilToSix, ordinary.LuckOilToNine = domain.ID(), 0, 0, 0, 0
	// 此处仅预置一个其他来源旧材料，验收不会把它分解、覆盖或合并进新油。
	kept := domain.Item{ID: domain.ID(), ItemID: fixture.LuckOil.ItemID, Quantity: 2, Version: 1, SourceKind: "task_reward", SourceID: "other-source"}
	items = append(items, broken, legacyBroken, ordinary, kept)
	selected = append(selected, broken.ID, legacyBroken.ID, ordinary.ID)
	roles := account.DB.Collection("roles")
	_, err := roles.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"items": items, "capacity": len(items),
		"privilege_until": time.Now().Add(time.Hour)}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	if len(role.Items) != role.Capacity {
		t.Fatal("真实测试没有处于满包前态", role.Capacity, len(role.Items))
	}
	operation := func() *pb.OperationContext {
		return &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
			ExpectedRevision: uint64(role.Revision), ExpectedRuleVersion: fixture.RuleVersion}
	}
	apply := func(q AssetRequest, expected uint32) (*pb.OperationResult, Operation) {
		t.Helper()
		q.Fingerprint = Hash([]byte(q.Kind + ":" + domain.Hex(q.Context.OpId)))
		next, reply, receipt, err := game.AssetWithReceipt(ctx, auth, q)
		must(err)
		if reply.ErrorCode != expected || reply.Pending {
			t.Fatal("原资产事务返回非预期结果", q.Kind, reply)
		}
		role = next
		return reply, receipt
	}
	wallet := role.Wallet
	denied, deniedReceipt := apply(AssetRequest{Kind: "recycle", Context: operation(), ItemIDs: []string{selected[0], kept.ID}}, p.CodeInvalidArgument)
	if denied.RecycleOil != nil || deniedReceipt.Recycling != nil || !slices.Equal(role.Items, items) || role.Wallet != wallet {
		t.Fatal("混入无效材料后部分销毁或写了虚假回执", deniedReceipt, role.Items)
	}
	q := AssetRequest{Kind: "recycle", Context: operation(), ItemIDs: selected}
	result, original := apply(q, 0)
	if original.Recycling == nil || len(original.Recycling.Recycled) != len(selected) || len(original.Recycling.OilRolls) != eligible ||
		len(result.RecycleOil.GetRolls()) != eligible || !proto.Equal(result.RecycleOil, original.Recycling.oilProto()) {
		t.Fatal("未落下合格原件及逐件已确认回执", result, original.Recycling)
	}
	var bound, free domain.Item
	misses, hits := 0, 0
	outputs := map[string]bool{}
	for _, fact := range original.Recycling.OilRolls {
		item, found := originals[fact.EquipmentInstanceID]
		if !found || fact.Luck != item.EffectiveLuck() || fact.Denominator != 30 || fact.Draw >= 30 || (fact.Draw < fact.Luck) != (fact.Oil != nil) {
			t.Fatal("账本中的随机判定与原当前幸运、分母或命中结果不一致", fact)
		}
		delete(originals, fact.EquipmentInstanceID)
		if fact.Oil == nil {
			misses++
			continue
		}
		oil := *fact.Oil
		if oil.SourceKind != equipmentRecycleOilSource || oil.SourceID != item.ID || oil.ID == item.ID || oil.Bound != item.Bound ||
			oil.ItemID != fixture.LuckOil.ItemID || oil.Quantity != 1 || oil.Version != 1 || outputs[oil.ID] || !slices.Contains(role.Items, oil) {
			t.Fatal("没有完整入包一瓶独立的原来源油，或归属变化", fact, item)
		}
		outputs[oil.ID] = true
		hits++
		if oil.Bound {
			bound = oil
		} else {
			free = oil
		}
	}
	if bound.ID == "" || free.ID == "" || misses == 0 || len(role.Items) != hits+1 || len(originals) != 0 || !slices.Contains(role.Items, kept) ||
		role.Wallet.Gold != wallet.Gold+int64(eligible/2+1)*wood.RecycleGold || role.Wallet.BoundGold != wallet.BoundGold+int64(eligible/2)*wood.RecycleGold {
		t.Fatal("绑定与非绑定输出、破损零金币、满包产物或原货币不一致", hits, misses, role.Wallet)
	}
	for _, id := range selected {
		if role.HasItemInstance(id) {
			t.Fatal("已分解原件仍存在", id)
		}
	}
	var audit struct {
		Receipt *EquipmentRecyclingReceipt `bson:"recycling"`
		Rule    uint64                     `bson:"rule_version"`
		Config  string                     `bson:"config_hash"`
	}
	must(game.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": original.ID, "kind": "recycle"}).Decode(&audit))
	if !reflect.DeepEqual(audit.Receipt, original.Recycling) || audit.Rule != fixture.RuleVersion || audit.Config != fixture.Hash {
		t.Fatal("分解交易中原来源、抽样和审计没有同步确认", audit)
	}
	warehouse := func(item domain.Item, action uint32, revision uint64, expected uint32) *WarehouseReceipt {
		t.Helper()
		op := operation()
		_, receipt := apply(AssetRequest{Kind: "warehouse", Context: op, Warehouse: &pb.WarehouseAction{Operation: op,
			Scope: WarehouseShared, Action: action, WarehouseRevision: revision, WarehouseSlot: 0,
			ItemInstanceId: domain.Bytes(item.ID), ItemVersion: uint64(item.Version), Quantity: 1}}, expected)
		return receipt.Warehouse
	}
	if warehouse(bound, 1, 0, p.CodeWarehouseBlocked) != nil {
		t.Fatal("绑定分解油进入共享仓")
	}
	put := warehouse(free, 1, 0, 0)
	stored := free
	stored.Version++
	if put == nil || put.Original == nil || *put.Original != free || put.Transferred == nil || *put.Transferred != stored || role.HasItemInstance(free.ID) {
		t.Fatal("共享仓没有原瓶整件转移", put)
	}
	take := warehouse(stored, 2, uint64(put.RevisionAfter), 0)
	stored.Version++
	if take.Transferred == nil || *take.Transferred != stored || !slices.Contains(role.Items, stored) {
		t.Fatal("取回时修改了祝福油原实例或装备来源", take)
	}
	market := func(req *pb.ItemMarketAction, expected uint32) *ItemMarketReceipt {
		t.Helper()
		req.Operation = operation()
		_, receipt := apply(AssetRequest{Kind: AssetItemMarket, Context: req.Operation, ItemMarket: req}, expected)
		return receipt.ItemMarket
	}
	price := fixture.ItemMarket.PriceStepYuanbaoUnits
	market(&pb.ItemMarketAction{Action: itemMarketList, ItemInstanceId: domain.Bytes(bound.ID), ItemVersion: uint64(bound.Version),
		Quantity: 1, UnitPriceYuanbaoUnits: price}, p.CodeVendorItemBlocked)
	listing := market(&pb.ItemMarketAction{Action: itemMarketList, ItemInstanceId: domain.Bytes(stored.ID), ItemVersion: uint64(stored.Version),
		Quantity: 1, UnitPriceYuanbaoUnits: price}, 0)
	var order ItemMarketOrder
	must(game.DB.Collection(itemMarketOrders).FindOne(ctx, bson.M{"_id": listing.OrderID}).Decode(&order))
	stored.Version++
	if order.Item != stored || role.HasItemInstance(stored.ID) {
		t.Fatal("真实市场托管未独占完整原油", order)
	}
	returned := market(&pb.ItemMarketAction{Action: itemMarketCancel, OrderId: domain.Bytes(order.ID), OrderVersion: uint64(order.Revision),
		Quantity: 1, UnitPriceYuanbaoUnits: price}, 0)
	market(&pb.ItemMarketAction{Action: itemMarketClaim, EscrowId: domain.Bytes(returned.EscrowID), EscrowVersion: uint64(returned.EscrowVersion)}, 0)
	stored.Version++
	if !slices.Contains(role.Items, stored) {
		t.Fatal("取消并领回市场原油时改写了来源身份", stored)
	}
	q.Fingerprint = original.Digest
	unchanged, repeat, same, err := game.AssetWithReceipt(ctx, auth, q)
	must(err)
	if !reflect.DeepEqual(unchanged, role) || repeat.ErrorCode != 0 || !proto.Equal(repeat.RecycleOil, result.RecycleOil) ||
		!reflect.DeepEqual(same.Recycling, original.Recycling) || same.ID != original.ID {
		t.Fatal("消耗原件后重放再次抽样、重复入包或改写原事实", same, repeat)
	}
	t.Logf("原 30 分母满包分解 %d 件命中、%d 件未中；破损零金，原非绑油共享仓与市场全程同一原件和来源，重放原回执。", hits, misses)
}
