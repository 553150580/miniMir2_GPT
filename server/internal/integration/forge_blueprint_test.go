// 本文件经实际首领奖励、采矿、WebSocket 报价和重连验证图纸打造的资产闭环。
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestForgeBlueprintMongoWS 从首领胜利取得原图纸，实际挖齐黑铁并按同一报价结算和重放。
func TestForgeBlueprintMongoWS(t *testing.T) {
	fixture, templateID := forgeJourneyFixture(t)
	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) { cfg.Fixture = fixture })
	recipe := fixture.Forge.Recipes[0]
	login := &pb.LoginResult{}
	rpc(t, account, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "forge_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	summary := createDirectoryRole(t, account, login.AccountToken, route.origin.OriginID, "打造验收")
	roleID := domain.Hex(summary.Key.RoleId)
	toolDef := fixture.Mining.Tools[0]
	tool := domain.Item{ID: domain.ID(), ItemID: toolDef.ItemID, Quantity: 1, Version: 1, SourceKind: "npc_shop", SourceID: domain.ID(),
		DurabilityU: toolDef.DurabilityU, MaxDurabilityU: toolDef.DurabilityU, OriginalDurabilityU: toolDef.DurabilityU}
	// 只准备等级、原工具和金币。图纸与用于打造的黑铁仅由实际玩法产出。
	_, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": bson.M{
		"level": fixture.Mining.Layers[0].MinimumLevel, "wallet": domain.Wallet{Gold: recipe.Gold, BoundGold: 1000},
	}, "$push": bson.M{"items": tool}})
	must(t, err)
	readRole := func() domain.Role {
		t.Helper()
		role, err := store.Role(ctx, roleID)
		must(t, err)
		return role
	}
	attach := func() *client {
		t.Helper()
		ticket := directoryTicket(t, account, login.AccountToken, summary.Key, route)
		c := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
		t.Cleanup(c.close)
		return c
	}
	c := attach()
	c.auto(false, 1)
	booking := forgeDefeatPersonalBoss(c, templateID)
	role := readRole()
	blueprintIndex := slices.IndexFunc(role.Items, func(item domain.Item) bool { return item.ItemID == recipe.BlueprintItemID })
	if blueprintIndex < 0 {
		t.Fatal("真实首领胜利没有交付候选图纸", booking)
	}
	blueprint := role.Items[blueprintIndex]
	if blueprint.SourceKind != "personal_boss" || blueprint.SourceID != booking || !blueprint.Bound {
		t.Fatal("图纸没有继承原挑战和击杀时绑定", blueprint)
	}
	ores := forgeMineBlackIron(c, &fixture, tool.ID, recipe.OreCount)
	travelToBaseline(c, 2, fixture.Map.ID)
	travelAcknowledge(c, false)
	before := readRole()
	inputs := &pb.ForgeQuoteRequest{SceneEpoch: c.entry.SceneEpoch, Blueprint: &pb.ForgeItemRef{InstanceId: domain.Bytes(blueprint.ID), Version: uint64(blueprint.Version)}}
	for _, ore := range ores {
		inputs.Ores = append(inputs.Ores, &pb.ForgeItemRef{InstanceId: ore.InstanceId, Version: ore.Version})
	}
	travelExpectError(c, p.MsgC2SForgeQuote, c.send(p.MsgC2SForgeQuote, inputs), p.CodeForgeInputs)
	inputs.AllowBound = true
	quote := forgeQuoteWS(c, inputs)
	definition, _ := fixture.Item(recipe.OutputItemID)
	if quote.SuccessPpm != 1_000_000 || quote.OutputItemId != recipe.OutputItemID || quote.Quality != recipe.Quality ||
		quote.RequiredLevel != *definition.RequiredLevel || quote.Slot != definition.EquipmentSlot || !quote.OutputBound ||
		quote.BoundGold != min(recipe.Gold, before.Wallet.BoundGold) || quote.Gold+quote.BoundGold != recipe.Gold ||
		!proto.Equal(quote.Blueprint, blueprint.Proto()) || len(quote.Ores) != len(ores) {
		t.Fatal("实际预览和原图纸、100%有效成功率或绑定金币优先分账不一致", quote)
	}
	for index, ore := range ores {
		if !proto.Equal(ore, quote.Ores[index]) {
			t.Fatal("预览改用了非本次选择的原黑铁", ore, quote.Ores[index])
		}
	}
	if current := readRole(); current.Wallet != before.Wallet || !slices.Equal(current.Items, before.Items) || current.Revision != before.Revision {
		t.Fatal("仅查看报价就修改了原投入")
	}
	// 换场景后旧报价不可首次提交，所有材料仍保留。返回时向现有角色重新报价。
	travelToBaseline(c, 3, fixture.Mining.Layers[0].Map.ID)
	travelAcknowledge(c, false)
	stale := &pb.ForgeCommitRequest{QuoteId: quote.QuoteId, Operation: c.operation()}
	if result := c.asset(p.MsgC2SForgeCommit, stale); result.ErrorCode != p.CodeForgeQuoteChanged || result.Forge != nil {
		t.Fatal("旧场景报价被首次成交", result)
	}
	if current := readRole(); current.Wallet != before.Wallet || !slices.Equal(current.Items, before.Items) {
		t.Fatal("旧报价失败仍消耗了资源")
	}
	inputs.SceneEpoch = c.entry.SceneEpoch
	quote = forgeQuoteWS(c, inputs)
	commit := &pb.ForgeCommitRequest{QuoteId: quote.QuoteId, Operation: c.operation()}
	c.send(p.MsgC2SForgeCommit, commit)
	settled := waitForgeReceipt(t, ctx, store, roleID, domain.Hex(commit.Operation.OpId))
	c.close() // 模拟服务端已提交但客户端未读取响应。
	c = attach()
	result := c.asset(p.MsgC2SForgeCommit, commit)
	if result.Pending || result.ErrorCode != 0 || result.Forge == nil || !result.Forge.Success || !proto.Equal(result.Forge, settled.Forge.Proto()) {
		t.Fatal("新票重连未重放原打造结果", result)
	}
	output := result.Forge.Output
	if output == nil || output.ItemId != recipe.OutputItemID || output.Quality != recipe.Quality || !output.Bound || output.InitialLuck > 3 ||
		result.Forge.Gold != quote.Gold || result.Forge.BoundGold != quote.BoundGold {
		t.Fatal("真实成品与报价条件不同", result)
	}
	final := readRole()
	wantWallet := before.Wallet
	wantWallet.Gold -= quote.Gold
	wantWallet.BoundGold -= quote.BoundGold
	removed := map[string]bool{blueprint.ID: true}
	for _, ore := range ores {
		removed[domain.Hex(ore.InstanceId)] = true
	}
	remaining := slices.DeleteFunc(slices.Clone(before.Items), func(item domain.Item) bool { return removed[item.ID] })
	if final.Wallet != wantWallet || final.SourceSeq != before.SourceSeq || final.EconomicSeq != before.EconomicSeq+2 ||
		len(final.Items) != len(remaining)+1 || !slices.Contains(final.Items, *settled.Forge.Output) ||
		settled.Forge.Output.ID != domain.Hex(output.InstanceId) || settled.Forge.Output.InitialLuck != output.InitialLuck {
		t.Fatal("打造原矿、图纸和钱包未在同一次原事务内核销", final.Wallet, result)
	}
	for _, item := range remaining {
		if !slices.Contains(final.Items, item) {
			t.Fatal("打造修改了未选择的其他原件", item)
		}
	}
}

// forgeJourneyFixture 保证专项时长有界，原正式概率和强度由未改动的投影保存。
func forgeJourneyFixture(t *testing.T) (devconfig.Fixture, uint32) {
	t.Helper()
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	raw, err := os.ReadFile(os.Getenv("MINIMIR_FORGE_TEST_FIXTURE"))
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	if fixture.Forge == nil || len(fixture.Forge.Recipes) != 1 || fixture.Mining == nil || len(fixture.Mining.Ores) != 4 {
		t.Fatal("联机验收须提供已投影的候选地图及单个打造配方")
	}
	fixture.Mining.SuccessPPM, fixture.Mining.IntervalMS = 1_000_000, 1000
	fixture.Mining.Purity = []devconfig.MiningPurityBand{{Min: 1, Max: 99, WeightPPM: 1}, {Min: 100, Max: 100, WeightPPM: 999_999}}
	for index := range fixture.Mining.Ores {
		ore := &fixture.Mining.Ores[index]
		ore.LayerWeights = [2]uint32{1, 1}
		if ore.ItemID == fixture.Forge.OreItemID {
			ore.LayerWeights = [2]uint32{97, 97}
		}
	}
	for index := range fixture.PersonalBossVariants {
		boss := &fixture.PersonalBossVariants[index]
		if boss.ItemReward == nil || !slices.ContainsFunc(boss.ItemReward.Items, func(line devconfig.ChallengeRewardItem) bool {
			return line.ItemID == fixture.Forge.Recipes[0].BlueprintItemID
		}) {
			continue
		}
		boss.Monster.HP, boss.Monster.Agility, boss.Monster.AttackMin, boss.Monster.AttackMax = 1, 0, 0, 0
		boss.Monster.Defence, boss.Monster.DefenceMin, boss.Monster.DefenceMax = 0, 0, 0
		for _, step := range []devconfig.Point{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}} {
			spawn := devconfig.Point{X: boss.Map.PlayerSpawn.X + step.X, Y: boss.Map.PlayerSpawn.Y + step.Y}
			if boss.Map.Walkable(spawn) {
				boss.Map.MonsterSpawns = []devconfig.Point{spawn}
				return fixture, boss.TemplateID
			}
		}
	}
	t.Fatal("候选图纸没有正常的个人首领来源和可攻击位置")
	return devconfig.Fixture{}, 0
}

// forgeDefeatPersonalBoss 经实际入场确认、攻击和返回原图接收挑战的固定候选附件。
func forgeDefeatPersonalBoss(c *client, templateID uint32) string {
	c.t.Helper()
	request := c.send(p.MsgC2SPersonalBossEnter, &pb.PersonalBossEnter{SceneEpoch: c.entry.SceneEpoch, ChallengeId: domain.ID(), TemplateId: templateID})
	bookingFrame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == request })
	var booking pb.PersonalBossState
	if bookingFrame.ID != p.MsgS2CPersonalBossState || bookingFrame.Error != 0 {
		c.t.Fatal("候选图纸的首领预约失败", bookingFrame.Error)
	}
	must(c.t, proto.Unmarshal(bookingFrame.Payload, &booking))
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonalBoss && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	travelAcknowledge(c, false)
	travelUntilWithin(c, 10*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CPersonalBossState {
			return false
		}
		var state pb.PersonalBossState
		must(c.t, proto.Unmarshal(frame.Payload, &state))
		return state.State == 4 && state.ChallengeId == booking.ChallengeId
	})
	returnFromWorldBossTest(c, booking.ChallengeId)
	return booking.ChallengeId
}

// forgeMineBlackIron 只选择实际出矿送来的高纯度原实例，不修改矿石库存。
func forgeMineBlackIron(c *client, fixture *devconfig.Fixture, tool string, count uint32) []*pb.ItemView {
	c.t.Helper()
	travelToBaseline(c, 3, fixture.Mining.Layers[0].Map.ID)
	travelAcknowledge(c, false)
	if state := miningStateWS(c, 1, domain.Bytes(tool)); !state.Active || state.AutoRecycleEnabled {
		c.t.Fatal("候选原锄头没有开启采矿", state)
	}
	seen := make(map[string]bool)
	ores := make([]*pb.ItemView, 0, count)
	travelUntilWithin(c, 28*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CMiningState || frame.Request != 0 {
			return false
		}
		var state pb.MiningState
		must(c.t, proto.Unmarshal(frame.Payload, &state))
		if original := state.LastOre; original != nil && !seen[domain.Hex(original.InstanceId)] && original.ItemId == fixture.Forge.OreItemID && original.OrePurity >= fixture.Forge.GuaranteedPurity {
			seen[domain.Hex(original.InstanceId)] = true
			ores = append(ores, original)
		}
		return len(ores) == int(count)
	})
	if state := miningStateWS(c, 2, nil); state.Active {
		c.t.Fatal("验收原矿足量后没有停止挖矿", state)
	}
	return ores
}

// forgeQuoteWS 只接受与本次原请求编号对应的实际服务端报价。
func forgeQuoteWS(c *client, q *pb.ForgeQuoteRequest) *pb.ForgeQuote {
	c.t.Helper()
	request := c.send(p.MsgC2SForgeQuote, q)
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == request })
	if frame.ID != p.MsgS2CForgeQuote || frame.Error != 0 {
		c.t.Fatal("服务端没有返回该次打造报价", frame.ID, frame.Error)
	}
	var quote pb.ForgeQuote
	must(c.t, proto.Unmarshal(frame.Payload, &quote))
	return &quote
}

// waitForgeReceipt 只从已提交的原资产记录确认落库，触发测试断线不修改账本。
func waitForgeReceipt(t *testing.T, ctx context.Context, store *persistence.Store, roleID, opID string) persistence.Operation {
	t.Helper()
	var receipt persistence.Operation
	deadline := time.Now().Add(7 * time.Second)
	for {
		err := store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + roleID + ":" + opID}).Decode(&receipt)
		if !errors.Is(err, mongo.ErrNoDocuments) || time.Now().After(deadline) {
			must(t, err)
			break
		}
		time.Sleep(35 * time.Millisecond)
	}
	if receipt.Kind != persistence.AssetForgeCommit || receipt.Code != 0 || receipt.Forge == nil || receipt.Forge.Output == nil {
		t.Fatal("真实打造未生成有成品的原永久回执", receipt)
	}
	return receipt
}
