// 本文件使用独立 MongoDB 与真实 WebSocket 验证矿层、商店工具、离线扣时和健康接回。
package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestMiningLoopMongoWS 从地图目录握手买工具和入场，在正常托管与重连时验证逐轮耐久和余额。
func TestMiningLoopMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	raw, err := os.ReadFile(os.Getenv("MINIMIR_MINING_TEST_FIXTURE"))
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	digest := sha256.Sum256(raw)
	fixture.Hash = hex.EncodeToString(digest[:])
	must(t, fixture.Validate())
	if fixture.Mining == nil || len(fixture.Mining.Layers) != 2 || len(fixture.Mining.Tools) != 2 {
		t.Fatal("隔离验收须装配两层矿区")
	}
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) { cfg.Fixture = fixture })
	first, second := fixture.Mining.Layers[0], fixture.Mining.Layers[1]
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "minews_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	levels := []uint32{first.MinimumLevel - 1, first.MinimumLevel, second.MinimumLevel - 1, second.MinimumLevel}
	roles := make([]*pb.RoleSummary, len(levels))
	for index, level := range levels {
		role := &pb.RoleSummary{}
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: route.origin.OriginID,
			Slot: uint32(index + 1), Job: 0, Name: "矿试" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
		_, err = store.DB.Collection("roles").UpdateByID(ctx, domain.Hex(role.Key.RoleId), bson.M{"$set": bson.M{
			"level": level, "wallet": domain.Wallet{Gold: 500}, "hosting_seconds": int64(7),
		}})
		must(t, err)
		roles[index] = role
	}
	attach := func(index int) *client {
		t.Helper()
		ticket := directoryTicket(t, endpoint, login.AccountToken, roles[index].Key, route)
		c := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
		t.Cleanup(c.close)
		return c
	}
	readMiner := func() domain.Role {
		t.Helper()
		role, err := store.Role(ctx, domain.Hex(roles[1].Key.RoleId))
		must(t, err)
		return role
	}
	rejectEntry := func(c *client, to uint64) {
		t.Helper()
		request := &pb.MapTravelPrepare{SceneEpoch: c.entry.SceneEpoch, TravelId: domain.ID(), Destination: 3, TargetMapId: to}
		travelExpectError(c, p.MsgC2SMapTravelPrepare, c.send(p.MsgC2SMapTravelPrepare, request), p.CodeLineForbidden)
	}
	below := attach(0)
	rejectEntry(below, first.Map.ID)
	below = attach(2)
	rejectEntry(below, second.Map.ID)
	deep := attach(3)
	travelToBaseline(deep, 3, second.Map.ID)
	travelAcknowledge(deep, false)
	if state := miningStateWS(deep, 1, nil); state.Active || state.ErrorCode != p.CodeEquipmentChanged {
		t.Fatal("二层无工具时错误进入开采", state)
	}
	deep.close()

	c := attach(1)
	c.auto(false, 3)
	toolDefinition := fixture.Mining.Tools[0]
	town, npc, offer := miningVendor(t, &fixture, toolDefinition.ItemID)
	travelToBaseline(c, 1, town.ID)
	travelAcknowledge(c, true)
	tool := buyMiningTool(c, town, npc, offer, toolDefinition.DurabilityU)
	travelToBaseline(c, 3, first.Map.ID)
	travelAcknowledge(c, false)
	if c.entry.SceneKind != devconfig.SceneKindMining || c.player() == nil || len(c.scene.Entities) != 1 {
		t.Fatal("矿区承担了不属于原独享场景的战斗实体", c.entry, c.scene)
	}
	if rejected := miningStateWS(c, 1, domain.Bytes(domain.ID())); rejected.Active || rejected.ErrorCode != p.CodeEquipmentChanged {
		t.Fatal("非原角色背包的锄头可以开启采矿", rejected)
	}
	travelExpectError(c, p.MsgC2SAutoBattle, c.send(p.MsgC2SAutoBattle, &pb.AutoBattleIntent{
		SceneEpoch: c.entry.SceneEpoch, Enabled: true, MonsterCount: 1,
	}), p.CodeLineForbidden)
	before := readMiner()
	if started := miningStateWS(c, 1, tool.InstanceId); !started.Active || started.ErrorCode != 0 || started.Attempts != 0 {
		t.Fatal("游戏客户端未用原锄头开启挖矿", started)
	}
	if duplicate := miningStateWS(c, 1, tool.InstanceId); !duplicate.Active || duplicate.Attempts != 0 {
		t.Fatal("重复开采请求额外触发了一轮判定", duplicate)
	}
	var sampled pb.MiningState
	travelUntilWithin(c, 10*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CMiningState || frame.Request != 0 {
			return false
		}
		must(t, proto.Unmarshal(frame.Payload, &sampled))
		return sampled.Attempts == 1
	})
	if after := readMiner(); after.SourceSeq != before.SourceSeq+1 || after.HostingSeconds != before.HostingSeconds || after.Wallet != before.Wallet ||
		after.EconomicSeq != before.EconomicSeq || sampled.Tool.GetDurabilityU()+uint64(fixture.Mining.ToolWearU) != tool.DurabilityU {
		t.Fatal("在线判定改变了托管或其他资产", sampled.Tool)
	}
	checkMiningViewOre(t, &sampled)
	scene := c.entry.SceneEpoch
	c.close()
	deadline := time.Now().Add(18 * time.Second)
	for role := readMiner(); role.HostingSeconds != 1; role = readMiner() {
		if time.Now().After(deadline) || role.HostingSeconds < 1 {
			t.Fatalf("健康掉线未仅扣两个完整挖矿间隔，剩余=%d", role.HostingSeconds)
		}
		time.Sleep(35 * time.Millisecond)
	}
	c = attach(1)
	state := miningStateWS(c, 0, nil)
	if c.entry.RoomId != first.Map.ID || c.entry.SceneEpoch != scene || state.Active || state.Attempts != 3 ||
		state.Tool.GetDurabilityU()+uint64(3*fixture.Mining.ToolWearU) != tool.DurabilityU {
		t.Fatal("健康接回未保留原矿层和已执行的耐久结果", state, c.entry)
	}
	checkMiningViewOre(t, state)
	after := readMiner()
	if after.HostingSeconds != 1 || after.SourceSeq != before.SourceSeq+3 || after.EconomicSeq != before.EconomicSeq || after.Report.UsedSeconds != before.Report.UsedSeconds+6 {
		t.Fatal("两次离线判定没有与六秒实际消耗沿原流水提交", after.HostingSeconds, after.Report.UsedSeconds, after.SourceSeq)
	}
	travelToBaseline(c, 2, fixture.Map.ID)
	travelAcknowledge(c, false)
	if c.entry.SceneKind != devconfig.SceneKindPersonal || len(c.scene.Entities) < 4 {
		t.Fatal("退出矿区没有恢复普通地图和原怪物槽数", c.entry, c.scene)
	}
}

func miningStateWS(c *client, action uint32, tool []byte) *pb.MiningState {
	c.t.Helper()
	request := c.send(p.MsgC2SMiningIntent, &pb.MiningIntent{SceneEpoch: c.entry.SceneEpoch, Action: action, ToolInstanceId: tool})
	frame := travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CMiningState && f.Request == request })
	var state pb.MiningState
	must(c.t, proto.Unmarshal(frame.Payload, &state))
	return &state
}

func checkMiningViewOre(t *testing.T, state *pb.MiningState) {
	t.Helper()
	if ore := state.LastOre; ore != nil && (ore.OrePurity < 1 || ore.OrePurity > 100 || !ore.Bound || ore.SourceKind != "mining") {
		t.Fatal("真实出矿没有同步原纯度、来源和绑定", ore)
	}
}

func miningVendor(t *testing.T, f *devconfig.Fixture, itemID uint32) (*devconfig.MapTemplate, devconfig.TownNPC, devconfig.SkillShop) {
	t.Helper()
	for i := range f.Maps {
		town := &f.Maps[i]
		for _, npc := range town.NPCs {
			shop := f.NPCShopByRole(npc.Role)
			if shop == nil {
				continue
			}
			if index := slices.IndexFunc(shop.Offers, func(offer devconfig.SkillShop) bool { return offer.ItemID == itemID }); index >= 0 {
				return town, npc, shop.Offers[index]
			}
		}
	}
	t.Fatal("隔离矿区没有正常商店锄头来源")
	return nil, devconfig.TownNPC{}, devconfig.SkillShop{}
}

func buyMiningTool(c *client, town *devconfig.MapTemplate, npc devconfig.TownNPC, offer devconfig.SkillShop, durability int64) *pb.ItemView {
	c.t.Helper()
	reachTownVendor(c, town, npc)
	request := c.send(p.MsgC2SVendorQuote, &pb.VendorQuoteRequest{SceneEpoch: c.entry.SceneEpoch, NpcId: npc.ID, Action: 1, OfferId: offer.ID, Quantity: 1})
	frame := travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CVendorQuote && frame.Request == request })
	var quote pb.VendorQuoteResult
	must(c.t, proto.Unmarshal(frame.Payload, &quote))
	if quote.ItemId != offer.ItemID || quote.Gold != offer.PriceUnits {
		c.t.Fatal("原商店没有按隔离配置给出锄头报价", &quote)
	}
	result := c.asset(p.MsgC2SVendorCommit, &pb.VendorCommitRequest{SceneEpoch: c.entry.SceneEpoch, NpcId: npc.ID, QuoteId: quote.QuoteId, Operation: c.operation()})
	if result.ErrorCode != 0 {
		c.t.Fatal("现有 NPC 购买没有实际交付锄头", result)
	}
	c.inventoryAt(result.NewRevision)
	for _, item := range c.inventory.Items {
		if item.ItemId == offer.ItemID && item.DurabilityU == uint64(durability) && item.Quantity == 1 && item.SourceKind == "npc_shop" {
			return item
		}
	}
	c.t.Fatal("NPC 锄头没有独立实例、原耐久或来源")
	return nil
}
