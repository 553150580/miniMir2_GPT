//go:build m4_economy_sampling

// 本文件在普通挂机墙钟结束后，用实际 NPC 价格补药、修理并独立核对成本。
package integration

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

const (
	economyVendorBuy    = uint32(1) // 原协议的 NPC 购买动作。
	economyVendorRepair = uint32(3) // 原协议的 NPC 普通修理动作。
)

// economyCare 是窗口收束后的实际维修与恢复起始库存支出。
type economyCare struct {
	Status        string           `json:"status"`               // 实际完成或当期角色无法进入城镇。
	PotionCost    domain.Wallet    `json:"potion_cost"`          // 实付补药费用，按币种保存。
	RepairCost    domain.Wallet    `json:"repair_cost"`          // 实付普通修理费用。
	After         economyCut       `json:"after"`                // 正常补给与修理完成后看到的角色状态。
	Net           domain.Wallet    `json:"net_with_maintenance"` // 从窗口开始到维修完成的四钱包实际增减。
	Restocked     map[uint32]int64 `json:"restocked"`            // 实际买回的库存差额。
	Transactions  []economyTrade   `json:"transactions"`         // 原 NPC 成交记录，便于逐笔重新核对。
	TravelSources []string         `json:"travel_sources"`       // 正常接触安全区产生的原绑定回执。
}

type economyTrade struct {
	ID       string        `json:"id"`       // 原操作与审计共用的标识。
	Action   uint32        `json:"action"`   // 实际购买或修理。
	Item     uint32        `json:"item"`     // 原药品或原装备模板。
	Quantity int64         `json:"quantity"` // 实际发放药品数或修理件数。
	Spend    domain.Wallet `json:"spend"`    // 原报价确认的四钱包费用。
}

// closeMaintenance 对比同一角色两端的库存差额，并沿合法商店路径实际支付。
func (a *economyActor) closeMaintenance(gate chan struct{}, before, end domain.Role, report *economyReport) {
	t := a.t
	c := a.client
	care := &economyCare{Status: "completed", Restocked: map[uint32]int64{}}
	report.Maintenance = care
	stop := sampleHeartbeat(c, time.Now().Add(25*time.Minute))
	defer stop()
	select {
	case gate <- struct{}{}:
		defer func() {
			c.close()
			<-gate
		}()
	case <-c.ctx.Done():
		t.Fatal("维修排队超出本次隔离连接")
	}
	if c.player().GetHp() <= 0 {
		if c.death == nil || c.death.GetAllowedActions()&1 == 0 {
			care.Status = "no_free_revive"
			care.After, care.Net = economyState(end), report.Net
			return
		}
		a.revive(c.death)
	}
	startPotions, endPotions := economyPotions(before), economyPotions(end)
	for _, id := range []uint32{economyHealthPotion, economyManaPotion} {
		needed := max(startPotions[id]-endPotions[id], 0)
		if needed == 0 {
			continue
		}
		town, npc, offer := naturalEquipmentVendor(t, &a.cfg.Fixture, id)
		reachTownVendor(c, town, npc)
		for needed > 0 {
			count := min(needed, offer.MaxPurchaseQuantity)
			if count <= 0 {
				t.Fatal("补药商店的正常购买数量为零", offer.ID)
			}
			trade := a.economyTrade(npc.ID, &pb.VendorQuoteRequest{Action: economyVendorBuy, OfferId: offer.ID, Quantity: count})
			if trade.Item != id || trade.Quantity != count || trade.Spend.Gold+trade.Spend.BoundGold != count*offer.PriceUnits ||
				trade.Spend.Yuanbao != 0 || trade.Spend.BoundYuanbao != 0 {
				t.Fatal("原补药成交不等于购买的差额和当前原价", trade)
			}
			care.PotionCost = economySum(care.PotionCost, trade.Spend)
			care.Restocked[id] += count
			care.Transactions = append(care.Transactions, trade)
			needed -= count
		}
	}
	for _, worn := range end.Equipped {
		item := worn.Item
		if !item.HasValidDurability() || item.DurabilityU >= item.MaxDurabilityU {
			continue
		}
		town, npc := economyRepairVendor(t, &a.cfg.Fixture)
		reachTownVendor(c, town, npc)
		trade := a.economyTrade(npc.ID, &pb.VendorQuoteRequest{Action: economyVendorRepair, InstanceId: domain.Bytes(item.ID),
			ItemVersion: uint64(item.Version), Quantity: 1})
		if trade.Item != item.ItemID || trade.Quantity != 1 || trade.Spend.Yuanbao != 0 || trade.Spend.BoundYuanbao != 0 {
			t.Fatal("修理未匹配本次穿戴中的真实装备", trade)
		}
		care.RepairCost = economySum(care.RepairCost, trade.Spend)
		care.Transactions = append(care.Transactions, trade)
	}
	after := a.readRole()
	want := economyDiff(end.Wallet, economySum(care.PotionCost, care.RepairCost))
	if after.Wallet != want || after.HostingSeconds != end.HostingSeconds || after.Level != end.Level ||
		after.Experience != end.Experience || after.PendingExperience != end.PendingExperience {
		t.Fatal("收窗补药和修理没有沿原余额、库存或离线时长提交", after.Wallet, want)
	}
	care.TravelSources = a.verifyCareSources(end, after)
	for id, count := range economyPotions(after) {
		if count != max(startPotions[id], endPotions[id]) {
			t.Fatal("补药差额没有恢复到原可比库存", id, count)
		}
	}
	for _, item := range after.Equipped {
		if item.Item.HasValidDurability() && item.Item.DurabilityU != item.Item.MaxDurabilityU {
			t.Fatal("实际普通修理未填满当前耐久上限", item.Item.ID)
		}
	}
	care.After, care.Net = economyState(after), economyDiff(after.Wallet, before.Wallet)
}

// economyRepairVendor 从实际城镇目录选处于可行地图的原铁匠。
func economyRepairVendor(t *testing.T, fixture *devconfig.Fixture) (*devconfig.MapTemplate, devconfig.TownNPC) {
	t.Helper()
	for _, id := range fixture.TownTravel.DestinationMapIDs {
		town, found := fixture.MapByID(id)
		if !found {
			continue
		}
		for _, npc := range town.NPCs {
			shop := fixture.NPCShopByRole(npc.Role)
			if shop != nil && shop.CanRepair {
				return town, npc
			}
		}
	}
	t.Fatal("当前夹具没有开放普通修理的 NPC")
	return nil, devconfig.TownNPC{}
}

// verifyCareSources 允许实际进城触发的安全区绑定，维护阶段仍不得同时赚取额外战斗收益。
func (a *economyActor) verifyCareSources(before, after domain.Role) []string {
	t := a.t
	t.Helper()
	cursor, err := a.store.DB.Collection("operations").Find(a.ctx, bson.M{
		"role_id": before.ID, "source_seq": bson.M{"$gt": before.SourceSeq, "$lte": after.SourceSeq},
	}, options.Find().SetSort(bson.D{{Key: "source_seq", Value: 1}}))
	must(t, err)
	var operations []persistence.Operation
	must(t, cursor.All(a.ctx, &operations))
	if int64(len(operations)) != after.SourceSeq-before.SourceSeq {
		t.Fatal("维护路径的安全区回执缺失", before.SourceSeq, after.SourceSeq)
	}
	var keys []string
	for index, op := range operations {
		if op.Kind != "safe_zone_binding" || op.Binding == nil || op.Code != 0 ||
			op.SourceSeq != before.SourceSeq+int64(index)+1 {
			t.Fatal("维护期间收到额外收益或无效绑定来源", op.Kind, op.SourceSeq)
		}
		fact := a.readEconomyAudits(a.ctx, []persistence.Operation{op})[op.ID]
		if fact.RoleID != before.ID || fact.Source != op.SourceSeq || fact.Kind != op.Kind || !reflect.DeepEqual(op.Binding, fact.Binding) {
			t.Fatal("进城产生的原绑定回执与审计不一致", op.ID)
		}
		keys = append(keys, op.ID)
	}
	return keys
}

// economyTrade 从真实报价提交并对拍 MongoDB 的同一笔成交与审计内容。
func (a *economyActor) economyTrade(npc uint64, request *pb.VendorQuoteRequest) economyTrade {
	t := a.t
	t.Helper()
	c := a.client
	request.SceneEpoch, request.NpcId = c.entry.SceneEpoch, npc
	id := c.send(p.MsgC2SVendorQuote, request)
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CVendorQuote && frame.Request == id
	})
	var quote pb.VendorQuoteResult
	must(t, proto.Unmarshal(frame.Payload, &quote))
	if frame.Error != 0 || len(quote.QuoteId) != 16 || quote.Action != request.Action {
		t.Fatal("原商店维护报价失败", frame.Error, request.Action)
	}
	result := a.requireOperation(p.MsgC2SVendorCommit, &pb.VendorCommitRequest{SceneEpoch: c.entry.SceneEpoch,
		NpcId: npc, QuoteId: quote.QuoteId, Operation: c.operation()})
	roleID := domain.Hex(a.role.RoleId)
	var operation persistence.Operation
	key := "asset:" + roleID + ":" + domain.Hex(result.OpId)
	must(t, a.store.DB.Collection("operations").FindOne(a.ctx, bson.M{"_id": key}).Decode(&operation))
	facts := a.readEconomyAudits(a.ctx, []persistence.Operation{operation})
	audit := facts[key]
	spend := domain.Wallet{Gold: quote.Gold, BoundGold: quote.BoundGold, Yuanbao: quote.Ingot, BoundYuanbao: quote.BoundIngot}
	if operation.RoleID != roleID || operation.Code != 0 || operation.Kind != persistence.AssetVendor || operation.Vendor == nil ||
		operation.Vendor.NPC != npc || operation.Vendor.QuoteID != domain.Hex(quote.QuoteId) ||
		audit.RoleID != roleID || audit.Kind != operation.Kind || audit.Rule != a.cfg.Fixture.RuleVersion ||
		audit.Config != a.cfg.Fixture.Hash || !reflect.DeepEqual(operation.Vendor, audit.Vendor) ||
		operation.Vendor.Plan.Action != request.Action || operation.Vendor.Plan.Wallet != spend {
		t.Fatal("原维修或补药回执没有与报价和审计一致", key)
	}
	plan := operation.Vendor.Plan
	return economyTrade{key, plan.Action, plan.ItemID, plan.ItemQuantity, spend}
}
