// 本文件用独立 MongoDB 回归洗炼石原分解、幂等、审计回滚与市场仓储流转。
package persistence

import (
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestRefineRecycleSettlementMongo 校验分解和发石属于同一个原操作，已绑定石禁止进入共享流通。
func TestRefineRecycleSettlementMongo(t *testing.T) {
	ctx, _, game, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_EQUIPMENT_REFINE_TEST_FIXTURE")
	fixture := &game.Cfg.Fixture
	rules := fixture.EquipmentRefine
	if rules == nil || rules.Recycle.Quantity != 1 || rules.Recycle.ProbabilityPPM != 50_000 || fixture.ItemMarket == nil {
		t.Fatal("隔离配置缺少原分解概率、单位数量或实际市场")
	}
	weapon, found := fixture.Item(rules.Recycle.ItemID)
	if !found || weapon.ID != 220 || weapon.DropQuality != 1 || weapon.RecycleGold <= 0 {
		t.Fatal("隔离配置缺少实际精良乌木剑目录和原分解金币")
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	const perBinding = 180
	roles := game.DB.Collection("roles")
	operations, audits := game.DB.Collection("operations"), game.DB.Collection("audit_outbox")
	operation := func() *pb.OperationContext {
		return &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
			ExpectedRevision: uint64(role.Revision), ExpectedRuleVersion: fixture.RuleVersion}
	}
	apply := func(q AssetRequest, expected uint32) (*pb.OperationResult, Operation) {
		t.Helper()
		q.Fingerprint = Hash(append([]byte(q.Kind+":"), q.Context.OpId...))
		after, reply, fact, err := game.AssetWithReceipt(ctx, auth, q)
		must(err)
		if reply.ErrorCode != expected || reply.Pending {
			t.Fatal("原资产事务返回非预期结果", q.Kind, reply)
		}
		role = after
		return reply, fact
	}
	var boundStone, freeStone domain.Item
	// 大批实物只为在真实 5% 概率下同时观察命中与失手。击杀自然流程另由联机用例验证。
	for batch := 0; batch < 2 && (boundStone.ID == "" || freeStone.ID == ""); batch++ {
		items := slices.Clone(role.Items)
		selected := make([]string, 0, perBinding*2)
		originals := make(map[string]domain.Item, perBinding*2)
		for index := range perBinding * 2 {
			item := InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: weapon.ID, Bound: index%2 == 0,
				Quantity: 1, Version: 1, Quality: weapon.DropQuality, SourceKind: ordinaryEquipmentItemSource,
				SourceID: strconv.Itoa(index + batch*perBinding*2 + 1)}, fixture)
			items, selected = append(items, item), append(selected, item.ID)
			originals[item.ID] = item
		}
		_, err := roles.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"items": items, "capacity": len(items), "privilege_until": time.Now().Add(time.Hour)}})
		must(err)
		role, err = game.Role(ctx, role.ID)
		must(err)
		before := role
		q := AssetRequest{Kind: "recycle", Context: operation(), ItemIDs: selected}
		q.Fingerprint = Hash(append([]byte(q.Kind+":"), q.Context.OpId...))
		key := bson.M{"_id": "asset:" + role.ID + ":" + domain.Hex(q.Context.OpId)}
		if batch == 0 {
			// 审计最后写入，临时重键验证前面的删除、发石、分账和原回执全部回滚。
			_, err = audits.InsertOne(ctx, key)
			must(err)
			_, _, _, err = game.AssetWithReceipt(ctx, auth, q)
			if !mongo.IsDuplicateKeyError(err) {
				t.Fatal("应由隔离审计原键触发真实事务回滚", err)
			}
			rolledBack, err := game.Role(ctx, role.ID)
			must(err)
			count, err := operations.CountDocuments(ctx, key)
			must(err)
			if !reflect.DeepEqual(before, rolledBack) || count != 0 {
				t.Fatal("审计失败后原背包、金币、修订或回执发生变动", count)
			}
			_, err = audits.DeleteOne(ctx, key)
			must(err)
		}
		result, fact := apply(q, p.CodeOK)
		recycling := fact.Recycling
		if recycling == nil || len(recycling.Recycled) != len(selected) || len(recycling.RefineRolls) != len(selected) ||
			len(recycling.OilRolls) != 0 || !proto.Equal(result.RecycleRefine, recycling.refineProto()) || result.RecycleOil != nil ||
			role.Wallet.Gold-before.Wallet.Gold != weapon.RecycleGold*perBinding || role.Wallet.BoundGold-before.Wallet.BoundGold != weapon.RecycleGold*perBinding {
			t.Fatal("分解的原料、金币和洗炼石回执不一致")
		}
		hits := 0
		for _, roll := range recycling.RefineRolls {
			original, exists := originals[roll.EquipmentInstanceID]
			if !exists || roll.Numerator != rules.Recycle.ProbabilityPPM || roll.Denominator != equipmentRecycleRefineDenominator ||
				role.HasItemInstance(original.ID) || roll.Draw >= roll.Denominator || (roll.Draw < roll.Numerator) != (roll.Stone != nil) {
				t.Fatal("原装备、随机范围、消耗与产石事实不一致", roll)
			}
			delete(originals, original.ID)
			if roll.Stone == nil {
				continue
			}
			stone := *roll.Stone
			if stone.ItemID != rules.StoneItemID || stone.Quantity != 1 || stone.Version != 1 || stone.SourceKind != equipmentRecycleRefineSource ||
				stone.SourceID != original.ID || stone.Bound != original.Bound || !slices.Contains(role.Items, stone) {
				t.Fatal("已入包的洗炼石丢失原绑定、数量或来源", stone, original)
			}
			hits++
			if stone.Bound {
				boundStone = stone
			} else {
				freeStone = stone
			}
		}
		if len(originals) != 0 || len(role.Items) != len(before.Items)-len(selected)+hits || hits == len(selected) {
			t.Fatal("原物品总量不守恒或没有保留实际失手", len(originals), hits)
		}
		confirmed := role
		reply, replay := apply(q, p.CodeOK)
		var audit struct {
			Recycling *EquipmentRecyclingReceipt `bson:"recycling"`    // 同事务冻结的原分解和副产物事实。
			Rule      uint64                     `bson:"rule_version"` // 实际采用的规则版本。
			Config    string                     `bson:"config_hash"`  // 实际采用的规则内容摘要。
		}
		must(audits.FindOne(ctx, key).Decode(&audit))
		if !proto.Equal(reply, result) || !reflect.DeepEqual(replay.Recycling, recycling) || !reflect.DeepEqual(audit.Recycling, recycling) ||
			!reflect.DeepEqual(role, confirmed) || audit.Rule != fixture.RuleVersion || audit.Config != fixture.Hash {
			t.Fatal("同号重试再次结算或审计未保留完整原分解结果")
		}
	}
	if boundStone.ID == "" || freeStone.ID == "" {
		t.Fatal("隔离样本未实际观察到两种绑定命中")
	}
	warehouse := func(item domain.Item, scope, action uint32, revision uint64, expected uint32) *WarehouseReceipt {
		t.Helper()
		op := operation()
		_, fact := apply(AssetRequest{Kind: "warehouse", Context: op, Warehouse: &pb.WarehouseAction{Operation: op,
			Scope: scope, Action: action, WarehouseRevision: revision, WarehouseSlot: 0,
			ItemInstanceId: domain.Bytes(item.ID), ItemVersion: uint64(item.Version), Quantity: 1}}, expected)
		return fact.Warehouse
	}
	// 单枚原石可以完整跨库存移动，绑定仅允许同角色私仓。
	if warehouse(boundStone, WarehouseShared, 1, 0, p.CodeWarehouseBlocked) != nil {
		t.Fatal("绑定洗炼石进入共享仓")
	}
	for _, row := range []struct {
		item  *domain.Item // 移入、移出时持续追踪同一原石。
		scope uint32       // 当前移动采用的仓库作用域。
	}{{&boundStone, WarehousePersonal}, {&freeStone, WarehouseShared}} {
		original := *row.item
		put := warehouse(original, row.scope, 1, 0, p.CodeOK)
		row.item.Version++
		if put == nil || put.Original == nil || *put.Original != original || put.Transferred == nil || *put.Transferred != *row.item || role.HasItemInstance(original.ID) {
			t.Fatal("仓库移动没有保存完整原石", put)
		}
		take := warehouse(*row.item, row.scope, 2, uint64(put.RevisionAfter), p.CodeOK)
		row.item.Version++
		if take == nil || take.Transferred == nil || *take.Transferred != *row.item || !slices.Contains(role.Items, *row.item) {
			t.Fatal("领回原石时来源、绑定或版本发生异常", take)
		}
	}
	price := fixture.ItemMarket.PriceStepYuanbaoUnits
	market := func(req *pb.ItemMarketAction, expected uint32) *ItemMarketReceipt {
		t.Helper()
		req.Operation = operation()
		_, fact := apply(AssetRequest{Kind: AssetItemMarket, Context: req.Operation, ItemMarket: req}, expected)
		return fact.ItemMarket
	}
	market(&pb.ItemMarketAction{Action: itemMarketList, ItemInstanceId: domain.Bytes(boundStone.ID), ItemVersion: uint64(boundStone.Version),
		Quantity: 1, UnitPriceYuanbaoUnits: price}, p.CodeVendorItemBlocked)
	listing := market(&pb.ItemMarketAction{Action: itemMarketList, ItemInstanceId: domain.Bytes(freeStone.ID), ItemVersion: uint64(freeStone.Version),
		Quantity: 1, UnitPriceYuanbaoUnits: price}, p.CodeOK)
	var order ItemMarketOrder
	must(game.DB.Collection(itemMarketOrders).FindOne(ctx, bson.M{"_id": listing.OrderID}).Decode(&order))
	freeStone.Version++
	if order.Item != freeStone || role.HasItemInstance(freeStone.ID) {
		t.Fatal("市场托管改写了合法非绑定原石", order)
	}
	returned := market(&pb.ItemMarketAction{Action: itemMarketCancel, OrderId: domain.Bytes(order.ID), OrderVersion: uint64(order.Revision),
		Quantity: 1, UnitPriceYuanbaoUnits: price}, p.CodeOK)
	market(&pb.ItemMarketAction{Action: itemMarketClaim, EscrowId: domain.Bytes(returned.EscrowID), EscrowVersion: uint64(returned.EscrowVersion)}, p.CodeOK)
	freeStone.Version++
	if !slices.Contains(role.Items, freeStone) {
		t.Fatal("取消上架并领回时原石身份、绑定或来源出现变化", freeStone)
	}
}
