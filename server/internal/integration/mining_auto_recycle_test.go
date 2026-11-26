// 本文件用真实联机检查矿石回收开关持久化、离线扣时和同一入账回执。
package integration

import (
	"context"
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
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestMiningAutoRecycleMongoWS 走角色确认、实际开采、断线托管、原矿层重连和旧设置重放。
func TestMiningAutoRecycleMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	raw, err := os.ReadFile(os.Getenv("MINIMIR_MINING_TEST_FIXTURE"))
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	if fixture.Mining == nil || fixture.Mining.AutoRecycle == nil {
		t.Fatal("隔离联机缺少已投影的矿石回收配置")
	}
	// 此用例固定有效判定都出矿，逐轮核对随机矿种。原三秒间隔及矿种纯度仍来自当前隔离投影。
	fixture.Mining.SuccessPPM = 1000000
	encoded, err := json.Marshal(fixture)
	must(t, err)
	digest := sha256.Sum256(encoded)
	fixture.Hash = hex.EncodeToString(digest[:])
	must(t, fixture.Validate())
	ctx, store, address, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) { cfg.Fixture = fixture })
	first, toolDefinition := fixture.Mining.Layers[0], fixture.Mining.Tools[0]
	miner := &pb.LoginResult{}
	rpc(t, address, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "mineauto_" + domain.ID()[:8], Password: "test_123456"}, miner, 0)
	role := createDirectoryRole(t, address, miner.AccountToken, route.origin.OriginID, "自动矿收")
	roleID := domain.Hex(role.Key.RoleId)
	tool := domain.Item{ID: domain.ID(), ItemID: toolDefinition.ItemID, Quantity: 1, Version: 1,
		SourceKind: "npc_shop", SourceID: domain.ID(), DurabilityU: toolDefinition.DurabilityU,
		MaxDurabilityU: toolDefinition.DurabilityU, OriginalDurabilityU: toolDefinition.DurabilityU}
	priorOre := domain.Item{ID: domain.ID(), ItemID: fixture.Mining.AutoRecycle.ItemIDs[0], Quantity: 1, Version: 1,
		SourceKind: "mining", SourceID: domain.ID(), OrePurity: 99, Bound: true}
	_, err = store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": bson.M{
		"level": first.MinimumLevel, "items": []domain.Item{tool, priorOre}, "hosting_seconds": int64(10),
	}})
	must(t, err)
	readRole := func() domain.Role {
		t.Helper()
		current, err := store.Role(ctx, roleID)
		must(t, err)
		return current
	}
	attach := func() *client {
		t.Helper()
		ticket := directoryTicket(t, address, miner.AccountToken, role.Key, route)
		c := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
		t.Cleanup(c.close)
		return c
	}
	c := attach()
	c.auto(false, 1)
	travelToBaseline(c, 3, first.Map.ID)
	travelAcknowledge(c, false)
	state := miningStateWS(c, 0, nil)
	if state.AutoRecycleEnabled || !slices.Equal(state.RecycleItemIds, fixture.Mining.AutoRecycle.ItemIDs) {
		t.Fatal("新角色没有默认关闭矿石回收，或可选矿种与配置不一致", state)
	}
	before := readRole()
	on := &pb.SetMiningAutoRecycleRequest{Enabled: true, Operation: c.operation()}
	if result := c.asset(p.MsgC2SSetMiningAutoRecycle, on); result.ErrorCode != 0 || result.Pending {
		t.Fatal("角色没有确认矿石自动回收", result)
	}
	if state = miningStateWS(c, 0, nil); !state.AutoRecycleEnabled || state.Attempts != 0 {
		t.Fatal("设置确认未反映在当前矿层，或错误触发了开采", state)
	}
	enabled := readRole()
	if !enabled.MiningAutoRecycle || enabled.SourceSeq != before.SourceSeq || enabled.Wallet != before.Wallet ||
		enabled.EconomicSeq != before.EconomicSeq+1 || !slices.Equal(enabled.Items, before.Items) {
		t.Fatal("只保存回收策略时修改了旧矿、收益或来源水位")
	}
	if state = miningStateWS(c, 1, domain.Bytes(tool.ID)); !state.Active || !state.AutoRecycleEnabled {
		t.Fatal("原锄头没有在已确认的回收策略下开启挖矿", state)
	}
	travelUntilWithin(c, 10*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CMiningState || frame.Request != 0 {
			return false
		}
		var state pb.MiningState
		must(t, proto.Unmarshal(frame.Payload, &state))
		return state.Attempts == 1
	})
	scene := c.entry.SceneEpoch
	c.close()
	deadline := time.Now().Add(20 * time.Second)
	for readRole().HostingSeconds != 1 {
		if time.Now().After(deadline) {
			t.Fatal("离线未消费三轮各三秒并保留尾数")
		}
		time.Sleep(35 * time.Millisecond)
	}
	c = attach()
	state = miningStateWS(c, 0, nil)
	after := readRole()
	lastPrice := verifyMiningAutoRecycleEntries(t, ctx, store, &fixture, enabled, after, priorOre)
	if c.entry.RoomId != first.Map.ID || c.entry.SceneEpoch != scene || state.Active || !state.AutoRecycleEnabled || state.Attempts != 4 ||
		state.Tool.GetDurabilityU() != uint64(tool.DurabilityU-4*fixture.Mining.ToolWearU) ||
		state.LastRecycleGold != lastPrice.Gold || state.LastRecycleBoundGold != lastPrice.BoundGold {
		t.Fatal("健康重连没有接回原矿层和对应的原回收及耐久结果", state, c.entry)
	}
	off := &pb.SetMiningAutoRecycleRequest{Operation: c.operation()}
	if result := c.asset(p.MsgC2SSetMiningAutoRecycle, off); result.ErrorCode != 0 || result.Pending {
		t.Fatal("关闭矿石自动回收失败", result)
	}
	if result := c.asset(p.MsgC2SSetMiningAutoRecycle, on); result.ErrorCode != 0 || result.Pending {
		t.Fatal("原开启请求的确认无法幂等重放", result)
	}
	final := readRole()
	if miningStateWS(c, 0, nil).AutoRecycleEnabled || final.MiningAutoRecycle || final.EconomicSeq != after.EconomicSeq+1 ||
		final.SourceSeq != after.SourceSeq || final.Wallet != after.Wallet || !slices.Equal(final.Items, after.Items) {
		t.Fatal("旧开关重放覆盖了较新设置或再次入账")
	}
	for _, request := range []*pb.SetMiningAutoRecycleRequest{on, off} {
		var receipt persistence.Operation
		must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(request.Operation.OpId)}).Decode(&receipt))
		if receipt.Kind != persistence.AssetSetMiningAutoRecycle || receipt.MiningAutoRecycle == nil || *receipt.MiningAutoRecycle != request.Enabled {
			t.Fatal("原开关回执缺少实际请求值", receipt.Kind, receipt.MiningAutoRecycle)
		}
	}
}

// verifyMiningAutoRecycleEntries 把四次原判定同账本、背包、实际余额和托管快照逐项对齐。
func verifyMiningAutoRecycleEntries(t *testing.T, ctx context.Context, store *persistence.Store, f *devconfig.Fixture,
	before, after domain.Role, priorOre domain.Item) domain.Wallet {
	t.Helper()
	if after.SourceSeq != before.SourceSeq+4 || after.EconomicSeq != before.EconomicSeq || after.HostingSeconds != 1 ||
		after.Report.UsedSeconds != 9 || after.Report.Kills != 0 || !slices.Contains(after.Items, priorOre) {
		t.Fatal("原在包矿、离线时长或成功判定没有逐轮持久化", after.SourceSeq, after.HostingSeconds, after.Report)
	}
	var lastPrice domain.Wallet
	wallet := before.Wallet
	var reportCount, reportGold, reportDrops int64
	for seq := before.SourceSeq + 1; seq <= after.SourceSeq; seq++ {
		var op persistence.Operation
		must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"role_id": after.ID, "kind": "mining_attempt", "source_seq": seq}).Decode(&op))
		result := op.Mining
		hosted := seq > before.SourceSeq+1
		if result == nil || result.Ore == nil || !result.Ore.Bound || (result.HostSeconds == 3) != hosted {
			t.Fatal("原成功判定、绑定或实际扣时与回执不一致", seq, result)
		}
		ore := *result.Ore
		price, quoted := f.MiningAutoRecyclePrice(ore.ItemID)
		var amount int64
		if quoted {
			amount = int64(ore.OrePurity) * int64(price.Numerator) / int64(price.Denominator)
		}
		lastPrice = domain.Wallet{BoundGold: amount}
		if amount == 0 {
			if result.AutoRecycle != nil || !slices.Contains(after.Items, ore) {
				t.Fatal("无收购价的原矿没有保留", ore)
			}
			if hosted {
				reportDrops++
			}
			continue
		}
		if result.AutoRecycle == nil || *result.AutoRecycle != lastPrice || slices.Contains(after.Items, ore) {
			t.Fatal("有价单矿的原绑定钱包没有和实例去向同笔提交", ore, result.AutoRecycle)
		}
		wallet.BoundGold += amount
		if hosted {
			reportCount++
			reportGold += amount
		}
	}
	if reportCount == 0 || wallet != after.Wallet || after.Report.MiningRecycleCount != reportCount ||
		after.Report.MiningRecycleGold != 0 || after.Report.MiningRecycleBoundGold != reportGold ||
		after.Report.Earned != (domain.Wallet{BoundGold: reportGold}) || after.Report.TotalDropCount != reportDrops ||
		int64(len(after.Report.Drops)) != reportDrops || after.Report.RecycleCount != 0 {
		t.Fatal("矿石回收钱包、托管战报或装备回收字段不守恒", after.Wallet, after.Report)
	}
	return lastPrice
}
