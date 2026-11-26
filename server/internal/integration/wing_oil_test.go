// 本文件用真实世界首领羽毛和独立 MongoDB 校验翅膀用油、两段累计及战斗属性。
package integration

import (
	"encoding/json"
	"os"
	"reflect"
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

// TestWingOilMongoWS 从首领原羽毛激活独立翅膀，经过真实原报价、失败和逐瓶必成保底。
func TestWingOilMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	raw, err := os.ReadFile(os.Getenv("MINIMIR_WING_OIL_TEST_FIXTURE"))
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	if fixture.Wings == nil || !fixture.Wings.AllowLuckOil || len(fixture.Wings.Ranks) != 3 || fixture.WorldBoss == nil ||
		fixture.WorldBoss.TemplateID != 2 || fixture.WorldBoss.Rewards == nil || fixture.LuckOil == nil ||
		fixture.LuckOil.Steps[4] != (devconfig.LuckOilStep{SuccessPPM: 300_000, FailureLuck: 3, PityAfter: 100}) ||
		fixture.LuckOil.Steps[8] != (devconfig.LuckOilStep{SuccessPPM: 50_000, FailureLuck: 6, PityAfter: 3000}) {
		t.Fatal("翅膀试装、首领原羽毛或两段工作簿概率不符")
	}
	trial := startWingWorldTrial(t, fixture)
	c := trial.winner.client
	feather := claimWingFeathers(t, c, trial.winner.reward, fixture.Wings.FeatherItemID, false)
	wing := growWingWS(t, c, nil, feather, domain.Bytes(domain.ID()), fixture.Wings.Ranks[0].Feathers, 1, 1, false)
	roleID := domain.Hex(trial.winner.summary.Key.RoleId)
	initial, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	free := domain.Item{ID: domain.ID(), ItemID: fixture.LuckOil.ItemID, Quantity: 1, Version: 1}
	bound := domain.Item{ID: domain.ID(), ItemID: fixture.LuckOil.ItemID, Quantity: 8, Version: 1, Bound: true}
	c.close()
	// 只有羽毛和翅膀走本轮自然入口。两份油堆在隔离角色预置，真实客户端逐瓶确认。
	change, err := trial.store.DB.Collection("roles").UpdateOne(trial.ctx, bson.M{"_id": roleID, "revision": initial.Revision},
		bson.M{"$set": bson.M{"items": append(slices.Clone(initial.Items), free, bound)}, "$inc": bson.M{"revision": 1}})
	must(t, err)
	if change.ModifiedCount != 1 {
		t.Fatal("未能准备隔离角色的原油堆")
	}
	c = trial.attach(t, &trial.winner, false)
	luckyAttributes(c, 0, 0)
	quote := wingOilQuoteWS(t, c, wing, free, false, 0, 0, 1_000_000, false)
	c, wing = spendWingOilWS(t, trial, c, quote, 1, 0, 0, false, false)
	luckyAttributes(c, 20_000, 0)
	denied := c.send(p.MsgC2SWeaponOilQuote, wingOilQuoteRequest(c, wing, false))
	travelExpectError(c, p.MsgC2SWeaponOilQuote, denied, p.CodeWeaponOilInputs)

	// 只在独立测试库定位到第100瓶前。固定本瓶仍然允许的最高掷点，明确验证失败回退并重放。
	c, wing = seedWingOilBoundary(t, trial, c, wing, 4, 99, 0)
	quote = wingOilQuoteWS(t, c, wing, bound, true, 99, 100, 300_000, false)
	if quote.FailureLuck != 3 {
		t.Fatal("第一段失败须回到 +3", quote)
	}
	c, wing = spendWingOilWS(t, trial, c, quote, 3, 100, 0, true, true)
	luckyAttributes(c, 44_000, 0)
	for _, luck := range []uint32{4, 5, 6} {
		quote = wingOilQuoteWS(t, c, wing, bound, true, wing.OilToSix, 100, 1_000_000, true)
		c, wing = spendWingOilWS(t, trial, c, quote, luck, wing.OilToSix+1, 0, false, false)
	}
	luckyAttributes(c, 110_000, 0)

	// 仅设置原历史次数断点。第3000瓶仍按工作簿掷点，下一瓶起逐档到 +9。
	c, wing = seedWingOilBoundary(t, trial, c, wing, 8, 103, 2999)
	quote = wingOilQuoteWS(t, c, wing, bound, true, 2999, 3000, 50_000, false)
	if quote.FailureLuck != 6 {
		t.Fatal("第二段失败须回到 +6", quote)
	}
	c, wing = spendWingOilWS(t, trial, c, quote, 6, 103, 3000, true, false)
	for _, luck := range []uint32{7, 8, 9} {
		quote = wingOilQuoteWS(t, c, wing, bound, true, wing.OilToNine, 3000, 1_000_000, true)
		c, wing = spendWingOilWS(t, trial, c, quote, luck, 103, wing.OilToNine+1, false, false)
	}
	luckyAttributes(c, 300_000, 0)
	hidden := c.asset(p.MsgC2SWingAction, wingAppearanceRequest(c, wing, 1, true))
	syncWingInventory(c, hidden.NewRevision)
	if receipt := hidden.GetWing().GetWing(); hidden.ErrorCode != 0 || !receipt.GetHidden() || receipt.GetLuck() != 9 ||
		receipt.GetOilToSix() != 103 || receipt.GetOilToNine() != 3003 {
		t.Fatal("隐藏丢失了真实用油或分段累计", hidden)
	}
	wing = hidden.GetWing().GetWing()
	luckyAttributes(c, 300_000, 0)
	for _, stage := range fixture.Wings.Ranks[1:] {
		wing = growOiledWingWS(t, c, wing, feather.InstanceId, stage)
	}
	luckyAttributes(c, 300_000, 0)
	shown := c.asset(p.MsgC2SWingAction, wingAppearanceRequest(c, wing, 1, false))
	syncWingInventory(c, shown.NewRevision)
	luckyAttributes(c, 300_000, 0)
	after, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	var persisted domain.WingState
	must(t, trial.store.DB.Collection("role_wings").FindOne(trial.ctx, bson.M{"_id": roleID}).Decode(&persisted))
	remaining := slices.DeleteFunc(slices.Clone(initial.Items), func(item domain.Item) bool { return item.ID == domain.Hex(feather.InstanceId) })
	if shown.ErrorCode != 0 || !proto.Equal(shown.GetWing().GetWing(), c.inventory.GetWing()) ||
		after.Wing == nil || *after.Wing != persisted || persisted.ID != domain.Hex(wing.WingId) || persisted.Rank != 3 ||
		persisted.AppearanceTier != 1 || persisted.Hidden || persisted.Luck != 9 || persisted.OilToSix != 103 || persisted.OilToNine != 3003 ||
		after.EconomicSeq != initial.EconomicSeq+13 || after.SourceSeq != initial.SourceSeq || after.Wallet != initial.Wallet ||
		!slices.Equal(after.Items, remaining) || !slices.Equal(after.Equipped, initial.Equipped) {
		t.Fatal("原翅膀、羽毛、九瓶油、装备及经济序列未守恒", after.Wing, after.EconomicSeq)
	}
	t.Log("世界首领原羽毛激活后，实际确认两段各自失败与逐瓶保底，掉响应重放仅扣一瓶，三阶及显隐继续保留原幸运和累计")
}

// seedWingOilBoundary 只用隔离原实例模拟已经发生的历史投入；重连从权威文档重读。
func seedWingOilBoundary(t *testing.T, trial worldBossRewardTrial, c *client, wing *pb.WingState, luck, toSix, toNine uint32) (*client, *pb.WingState) {
	t.Helper()
	c.close()
	roleID := domain.Hex(trial.winner.summary.Key.RoleId)
	change, err := trial.store.DB.Collection("role_wings").UpdateOne(trial.ctx,
		bson.M{"_id": roleID, "wing_id": domain.Hex(wing.WingId), "version": wing.Version},
		bson.M{"$set": bson.M{"luck": luck, "oil_to_six": toSix, "oil_to_nine": toNine}, "$inc": bson.M{"version": 1}})
	must(t, err)
	if change.ModifiedCount != 1 {
		t.Fatal("无法定位原翅膀的隔离累计边界")
	}
	c = trial.attach(t, &trial.winner, false)
	fresh := c.inventory.GetWing()
	if fresh.GetLuck() != luck || fresh.GetVersion() != wing.Version+1 || fresh.GetOilToSix() != toSix || fresh.GetOilToNine() != toNine {
		t.Fatal("健康换票没有恢复原翅膀培养边界", fresh)
	}
	return c, fresh
}

// wingOilQuoteRequest 沿相同消息只引用原翅膀，武器字段保持缺省。
func wingOilQuoteRequest(c *client, wing *pb.WingState, allowBound bool) *pb.WeaponOilQuoteRequest {
	return &pb.WeaponOilQuoteRequest{SceneEpoch: c.entry.SceneEpoch, UiSessionId: domain.Bytes(domain.ID()),
		Wing: &pb.WingOilRef{WingId: wing.WingId, Version: wing.Version}, AllowBound: allowBound}
}

// wingOilQuoteWS 核对前态翼、实际油堆、有效概率和原场景的发回报价。
func wingOilQuoteWS(t *testing.T, c *client, wing *pb.WingState, oil domain.Item, allowBound bool, used, threshold, chance uint32, guaranteed bool) *pb.WeaponOilQuote {
	t.Helper()
	q := wingOilQuoteRequest(c, wing, allowBound)
	id := c.send(p.MsgC2SWeaponOilQuote, q)
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == id })
	if frame.ID != p.MsgS2CWeaponOilQuote || frame.Error != 0 {
		t.Fatal("当前真实角色未得到原翅膀用油报价", frame)
	}
	var quote pb.WeaponOilQuote
	must(t, proto.Unmarshal(frame.Payload, &quote))
	if quote.Weapon != nil || quote.Equipped || !quote.OutputBound || !proto.Equal(quote.Wing, wing) ||
		quote.AllowBound != allowBound || quote.SceneEpoch != c.entry.SceneEpoch || !slices.Equal(quote.UiSessionId, q.UiSessionId) ||
		quote.CurrentLuck != wing.Luck || quote.SuccessLuck != wing.Luck+1 || quote.PityUsed != used || quote.PityAfter != threshold ||
		quote.SuccessPpm != chance || quote.Guaranteed != guaranteed || domain.Hex(quote.Oil.GetInstanceId()) != oil.ID || quote.Oil.GetBound() != oil.Bound {
		t.Fatal("实际报价不符合翅膀独占目标、原油身份或分段规则", &quote)
	}
	return &quote
}

// spendWingOilWS 逐笔对拍原持久事务、同事务审计和已投影到背包的独立翅膀。
func spendWingOilWS(t *testing.T, trial worldBossRewardTrial, c *client, quote *pb.WeaponOilQuote, luck, toSix, toNine uint32, fail, reconnect bool) (*client, *pb.WingState) {
	t.Helper()
	roleID := domain.Hex(trial.winner.summary.Key.RoleId)
	if fail {
		fixed, err := trial.store.DB.Collection("weapon_oil_quotes").UpdateOne(trial.ctx,
			bson.M{"_id": roleID, "quote_id": domain.Hex(quote.QuoteId)}, bson.M{"$set": bson.M{"roll": uint32(999_999)}})
		must(t, err)
		if fixed.MatchedCount != 1 {
			t.Fatal("只可固定同一隔离报价的原掷点")
		}
	}
	op := c.operation()
	op.ExpectedRevision = quote.RoleRevision
	request := &pb.WeaponOilCommitRequest{QuoteId: quote.QuoteId, UiSessionId: quote.UiSessionId, AllowBound: quote.AllowBound,
		Wing: &pb.WingOilRef{WingId: quote.Wing.WingId, Version: quote.Wing.Version}, Operation: op}
	if reconnect {
		pendingScene, binding := c.entry.SceneEpoch, c.binding
		c.send(p.MsgC2SWeaponOilCommit, request)
		waitWeaponOilReceipt(t, trial.ctx, trial.store, roleID, domain.Hex(op.OpId))
		c.close()
		c = trial.attach(t, &trial.winner, false)
		if c.binding == binding || c.entry.SceneEpoch != pendingScene {
			t.Fatal("用油响应丢失后没有重接原房间")
		}
	}
	result := c.asset(p.MsgC2SWeaponOilCommit, request)
	original := waitWeaponOilReceipt(t, trial.ctx, trial.store, roleID, domain.Hex(op.OpId))
	receipt := result.GetWeaponOil()
	wing := receipt.GetWing()
	used := quote.PityUsed
	if quote.PityAfter != 0 {
		used++
	}
	if result.ErrorCode != 0 || result.Pending || receipt.GetWeapon() != nil || receipt.GetEquipped() || receipt.GetSuccess() == fail ||
		original.WeaponOil.OilConsumed != 1 || !proto.Equal(receipt, original.WeaponOil.Proto()) || wing.GetLuck() != luck ||
		wing.GetOilToSix() != toSix || wing.GetOilToNine() != toNine || wing.GetVersion() != quote.Wing.Version+1 ||
		domain.Hex(wing.GetWingId()) != domain.Hex(quote.Wing.WingId) || wing.GetRank() != quote.Wing.Rank ||
		receipt.GetPityUsed() != used || receipt.GetPityAfter() != quote.PityAfter || receipt.GetGuaranteed() != quote.Guaranteed ||
		!proto.Equal(quote.Wing, original.WeaponOil.Plan.Wing.Proto()) || !proto.Equal(quote.Oil, original.WeaponOil.Plan.Oil.Proto()) {
		t.Fatal("用油永久确认没有原翅膀、原油及同次独立结果", result)
	}
	var audit struct {
		WeaponOil *persistence.WeaponOilResultReceipt `bson:"weapon_oil"`
	}
	must(t, trial.store.DB.Collection("audit_outbox").FindOne(trial.ctx, bson.M{"_id": original.ID}).Decode(&audit))
	if !reflect.DeepEqual(audit.WeaponOil, original.WeaponOil) {
		t.Fatal("原用油结果未随事务写入审计")
	}
	syncWingInventory(c, result.NewRevision)
	if !proto.Equal(c.inventory.GetWing(), wing) {
		t.Fatal("确认后原独立翅膀没有更新角色库存", c.inventory.GetWing())
	}
	return c, wing
}

// growOiledWingWS 实际消耗原羽毛升阶，保留隐藏和已经形成的原幸运及累计。
func growOiledWingWS(t *testing.T, c *client, wing *pb.WingState, featherID []byte, stage devconfig.WingRankConfig) *pb.WingState {
	t.Helper()
	feather, exists := findWingFeather(c, featherID)
	if !exists || feather.Bound || feather.SourceKind != "world_boss" {
		t.Fatal("升阶只能消费隔离用例原首领奖励的非绑定羽毛")
	}
	session := domain.Bytes(domain.ID())
	request := c.send(p.MsgC2SWingQuote, wingQuoteRequest(c, wing, feather, session, stage.Feathers, false))
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == request })
	if frame.ID != p.MsgS2CWingQuote || frame.Error != 0 {
		t.Fatal("尚有原羽毛的翅膀升阶报价失败", frame)
	}
	var quote pb.WingQuote
	must(t, proto.Unmarshal(frame.Payload, &quote))
	op := c.operation()
	op.ExpectedRevision = quote.RoleRevision
	result := c.asset(p.MsgC2SWingAction, &pb.WingActionRequest{Action: 1, QuoteId: quote.QuoteId, Operation: op,
		UiSessionId: session, WingId: wing.WingId, WingVersion: wing.Version})
	got := result.GetWing().GetWing()
	syncWingInventory(c, result.NewRevision)
	if result.ErrorCode != 0 || !proto.Equal(c.inventory.GetWing(), got) || !got.GetHidden() || got.GetRank() != stage.Rank ||
		got.GetAppearanceTier() != quote.TargetAppearanceTier || domain.Hex(got.GetWingId()) != domain.Hex(wing.WingId) ||
		got.GetVersion() != wing.Version+1 || got.GetLuck() != wing.Luck || got.GetOilToSix() != wing.OilToSix || got.GetOilToNine() != wing.OilToNine {
		t.Fatal("换阶、解锁外观或保持隐藏时失去了原翼的油计数", result)
	}
	return got
}
