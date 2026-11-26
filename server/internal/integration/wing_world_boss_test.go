// 本文件用真实世界首领、领取和 WebSocket 核验羽毛到唯一翅膀及其原回执的闭环。
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

// TestWingWorldBossMongoWS 从共享首领的真实击杀取得两个角色各自的原羽毛，验证成长和外观。
func TestWingWorldBossMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	fixture := readWingWorldFixture(t)
	trial := startWingWorldTrial(t, fixture)
	first, second := trial.winner.client, trial.partner.client
	roleID, otherID := domain.Hex(trial.winner.summary.Key.RoleId), domain.Hex(trial.partner.summary.Key.RoleId)
	featherID := fixture.Wings.FeatherItemID
	free := claimWingFeathers(t, first, trial.winner.reward, featherID, false)
	bound := claimWingFeathers(t, second, trial.partner.reward, featherID, true)
	if domain.Hex(free.InstanceId) == domain.Hex(bound.InstanceId) || first.inventory.GetWing() != nil || second.inventory.GetWing() != nil {
		t.Fatal("两个原角色的真实羽毛身份串角，或未激活已经出现翅膀")
	}

	// 沿原资产请求给未授权的绑定羽毛报价，不能因为翅膀将归角色专属而补扣。
	sessionFirst, sessionSecond := domain.Bytes(domain.ID()), domain.Bytes(domain.ID())
	cost := fixture.Wings.Ranks[0].Feathers
	secondQuote := wingQuoteRequest(second, nil, bound, sessionSecond, cost, false)
	ineligible := second.send(p.MsgC2SWingQuote, secondQuote)
	travelExpectError(second, p.MsgC2SWingQuote, ineligible, p.CodeWingInputs)
	if _, ok := findWingFeather(second, bound.InstanceId); !ok || second.inventory.GetWing() != nil {
		t.Fatal("绑定授权关闭时改变了原羽毛或创建了翅膀")
	}

	// 三份报价依次只扣同一个非绑定原堆。其中第三阶自动解锁外观3，第二阶保留外观1。
	var wing *pb.WingState
	appearance := []uint32{1, 1, 3}
	for i, stage := range fixture.Wings.Ranks[:3] {
		feather, ok := findWingFeather(first, free.InstanceId)
		if !ok || feather.Bound || feather.SourceKind != "world_boss" {
			t.Fatal("原非绑世界首领羽毛在升阶途中丢失或重判绑定", feather)
		}
		previous := wing
		wing = growWingWS(t, first, previous, feather, sessionFirst, stage.Feathers, stage.Rank, appearance[i], false)
		if previous != nil && (domain.Hex(previous.WingId) != domain.Hex(wing.WingId) || wing.Version != previous.Version+1) {
			t.Fatal("提升翅膀阶位更换了角色原实例", previous, wing)
		}
	}
	if _, ok := findWingFeather(first, free.InstanceId); ok {
		t.Fatal("三阶精确扣除羽毛后原非绑堆依然占格")
	}

	// 第二个角色必须显式授权才能激活，并且获得自己的唯一实例。
	secondWing := growWingWS(t, second, nil, bound, sessionSecond, cost, 1, 1, true)
	if domain.Hex(secondWing.WingId) == domain.Hex(wing.WingId) {
		t.Fatal("两个永久角色共享同一个翅膀实例")
	}
	foreign := second.asset(p.MsgC2SWingAction, wingAppearanceRequest(second, wing, 1, false))
	if foreign.ErrorCode == 0 || foreign.GetWing() != nil {
		t.Fatal("另一角色可以操作别人的翅膀", foreign)
	}
	syncWingInventory(second, foreign.NewRevision)
	if !proto.Equal(second.inventory.GetWing(), secondWing) {
		t.Fatal("非法外观操作污染了真实角色的自己持有物", second.inventory.GetWing())
	}
	old := first.asset(p.MsgC2SWingAction, wingAppearanceRequest(first, wing, 5, false))
	if old.ErrorCode != p.CodeWingInputs || old.GetWing() != nil {
		t.Fatal("尚未解锁的五阶外观可被直接选择", old)
	}
	syncWingInventory(first, old.NewRevision)
	duplicate := first.send(p.MsgC2SWingQuote, wingQuoteRequest(first, nil, free, sessionFirst, cost, false))
	travelExpectError(first, p.MsgC2SWingQuote, duplicate, p.CodeWingQuoteChanged)

	// 合法外观1与隐藏都是针对同一角色的数据修改。故意不读隐藏响应，验证换票后原请求重放。
	roleBefore, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	selected := first.asset(p.MsgC2SWingAction, wingAppearanceRequest(first, wing, 1, false))
	wing = selected.GetWing().GetWing()
	if selected.ErrorCode != 0 || wing.GetAppearanceTier() != 1 || wing.GetHidden() || len(selected.GetWing().GetFeathers()) != 0 {
		t.Fatal("已解锁外观1未正确保存，或外观操作消费羽毛", selected)
	}
	syncWingInventory(first, selected.NewRevision)
	wantOldScene, wantOldBinding, wantLife := first.entry.SceneEpoch, first.binding, first.player().Life
	hide := wingAppearanceRequest(first, wing, 1, true)
	first.send(p.MsgC2SWingAction, hide)
	receipt := waitWingOperation(t, trial.ctx, trial.store, roleID, domain.Hex(hide.Operation.OpId))
	first.close()
	first = trial.attach(t, &trial.winner, false)
	if first.entry.SceneEpoch != wantOldScene || first.binding == wantOldBinding || first.player().Life != wantLife || !first.inventory.GetWing().GetHidden() {
		t.Fatal("健康重连没有恢复原角色、普通房间和永久隐藏状态", first.entry, first.inventory.GetWing())
	}
	replayed := first.asset(p.MsgC2SWingAction, hide)
	final := replayed.GetWing().GetWing()
	if replayed.ErrorCode != 0 || replayed.Pending || !proto.Equal(receipt.Wing.Proto(), replayed.GetWing()) ||
		!proto.Equal(first.inventory.GetWing(), final) || !final.GetHidden() || final.GetRank() != 3 || final.GetAppearanceTier() != 1 ||
		final.GetLuck() != wing.GetLuck() || final.GetOilToSix() != wing.GetOilToSix() || final.GetOilToNine() != wing.GetOilToNine() ||
		final.GetVersion() != wing.GetVersion()+1 || domain.Hex(final.GetWingId()) != domain.Hex(wing.GetWingId()) {
		t.Fatal("断线重试没有返回同一翅膀状态，或改变了培养轴", replayed, first.inventory.GetWing())
	}
	verifyWingWorldStorage(t, trial.ctx, trial.store, roleBefore, final, secondWing, roleID, otherID)
	t.Log("真实世界首领给两角色各发原羽毛，授权绑定消费后分别激活；同一非绑原堆逐件升到3阶、换外观、隐藏并在健康重连后重放原永久回执")
}

// readWingWorldFixture 读取已经生成的候选；等级、费用、羽毛和发奖渠道都来自这份输入。
func readWingWorldFixture(t *testing.T) devconfig.Fixture {
	t.Helper()
	raw, err := os.ReadFile(os.Getenv("MINIMIR_WING_TEST_FIXTURE"))
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	if fixture.Wings == nil || len(fixture.Wings.Ranks) < 3 || !slices.Contains(fixture.Wings.AppearanceTiers, uint32(3)) ||
		fixture.WorldBoss == nil || fixture.WorldBoss.TemplateID != 2 || fixture.WorldBoss.Rewards == nil {
		t.Fatal("候选没有角色翅膀前三阶或模板2真正发放羽毛的装配")
	}
	return fixture
}

// startWingWorldTrial 只缩短隔离首领的血量、时钟和回场等待，奖励保持工作簿投影。
func startWingWorldTrial(t *testing.T, fixture devconfig.Fixture) worldBossRewardTrial {
	t.Helper()
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		cfg.Fixture = fixture
		boss := *fixture.WorldBoss
		current := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60)).Add(-2 * time.Second)
		midnight := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location())
		boss.DailyOpenOffsets, boss.Window, boss.ReturnDelay = []time.Duration{current.Sub(midnight).Truncate(time.Millisecond)}, 5*time.Minute, 300*time.Millisecond
		boss.Monster.HP, boss.Monster.AttackMin, boss.Monster.AttackMax, boss.Monster.Agility = 5, 0, 0, 0
		boss.Monster.MoveMS, boss.Monster.DefenceMin, boss.Monster.DefenceMax = 60_000, 0, 0
		cfg.Fixture.WorldBoss, cfg.Fixture.WorldBossVariants = &boss, nil
		cfg.Fixture.Items = slices.Clone(fixture.Items)
		for i := range cfg.Fixture.Items {
			if item := &cfg.Fixture.Items[i]; item.ID == 218 {
				item.AttackMinBonus, item.AttackMaxBonus = 3, 3
			}
		}
		if !cfg.Fixture.ValidWorldBossRewards(&boss) {
			t.Fatal("隔离时钟下真实羽毛奖励不再满足装配")
		}
	})
	trial := worldBossRewardTrial{ctx: ctx, store: store, endpoint: endpoint, route: route}
	trial.winner = trial.create(t, "翅膀甲", time.Now().Add(time.Hour), false)
	trial.partner = trial.create(t, "翅膀乙", time.Now().Add(-time.Minute), false)
	first, second := trial.attach(t, &trial.winner, true), trial.attach(t, &trial.partner, true)
	trial.winner.client, trial.partner.client = first, second
	open := queryWorldBossTest(t, first)
	enterWorldBossTest(t, first, open)
	enterWorldBossTest(t, second, open)
	approachWingBoss(t, first, -1, 0)
	approachWingBoss(t, second, 0, -1)
	time.Sleep(650 * time.Millisecond) // 格子行走的动作锁到期才发送普攻。
	attackWorldBossTest(first)
	waitWorldBossTest(t, second, 0, func(state *pb.WorldBossState) bool { return state.State == 3 && state.Hp == 2 })
	attackWorldBossTest(second)
	sealed := waitWorldBossTest(t, second, 0, func(state *pb.WorldBossState) bool { return state.State == 4 })
	if sealed.LatestResult == nil || len(sealed.Ranks) != 2 || sealed.Ranks[0].Damage != 3 || sealed.Ranks[1].Damage != 2 {
		t.Fatal("两名原角色没有向同一首领实际造成足够的参与伤害", sealed)
	}
	trial.sealed = sealed
	var needed int64
	for _, rank := range fixture.Wings.Ranks[:3] {
		needed += rank.Feathers
	}
	for _, player := range []*worldBossRewardPlayer{&trial.winner, &trial.partner} {
		returnFromWorldBossTest(player.client, open.OccurrenceId)
		rewards := reconnectBossInfo(t, player.client).GetWorldPendingRewards()
		if len(rewards) != 1 || rewards[0].GetEarnedAtUnixMs() != sealed.LatestResult.GetFinishedAtUnixMs() || rewards[0].GetSourceKind() != "world_boss" {
			t.Fatal("原羽毛必须来自本次已确认的真实首领击杀", rewards)
		}
		var amount int64
		for _, item := range rewards[0].Items {
			if item.ItemId == fixture.Wings.FeatherItemID {
				amount += item.Quantity
			}
		}
		if amount != needed {
			t.Fatal("真实候选发放的原羽毛与前三阶隔离验收数量不同", amount, needed)
		}
		player.reward = rewards[0]
	}
	return trial
}

// approachWingBoss 只使用实际脚下坐标及真实寻路，不改变候选首领地图。
func approachWingBoss(t *testing.T, c *client, dx, dy int32) {
	t.Helper()
	for range 3 {
		boss := travelEntity(c.scene, 3)
		if boss == nil || boss.Position == nil {
			t.Fatal("真实首领基线没有可用坐标")
		}
		goal := &pb.GridPoint{X: boss.Position.X + dx, Y: boss.Position.Y + dy}
		if !proto.Equal(challengePlayerDeathBody(c).Position, goal) {
			moveChallengePlayerDeathTest(t, c, goal)
		}
		if target := travelEntity(c.scene, 3); target != nil && target.Position.X+dx == goal.X && target.Position.Y+dy == goal.Y {
			return
		}
	}
	t.Fatal("真实角色无法通过寻路接近同一个原首领")
}

// claimWingFeathers 验证已确认奖励中的原实例和绑定按原样进入背包。
func claimWingFeathers(t *testing.T, c *client, reward *pb.ChallengePendingReward, itemID uint32, bound bool) *pb.ItemView {
	t.Helper()
	var feather *pb.ItemView
	for _, item := range reward.Items {
		if item.ItemId == itemID {
			if feather != nil {
				t.Fatal("此隔离路径须用一份原世界首领奖励堆")
			}
			feather = item
		}
	}
	if feather == nil || feather.Bound != bound || feather.SourceKind != "world_boss" || feather.SourceId == "" {
		t.Fatal("原击杀时的角色特权没有正确决定羽毛绑定", feather)
	}
	accepted := c.asset(p.MsgC2SClaimChallengeReward, &pb.ClaimChallengeReward{RewardId: reward.RewardId, Operation: c.operation()})
	syncWingInventory(c, accepted.NewRevision)
	if claimed, ok := findWingFeather(c, feather.InstanceId); accepted.ErrorCode != 0 || !ok || !proto.Equal(feather, claimed) {
		t.Fatal("真实世界奖励未按原绑定和身份领取", accepted, claimed)
	}
	return feather
}

// findWingFeather 只从已确认背包投影找指定原材料。
func findWingFeather(c *client, id []byte) (*pb.ItemView, bool) {
	for _, item := range c.inventory.GetItems() {
		if domain.Hex(item.InstanceId) == domain.Hex(id) {
			return item, true
		}
	}
	return nil, false
}

// wingQuoteRequest 冻结当前实际材料引用。未激活时目标留空以申请唯一实例。
func wingQuoteRequest(c *client, wing *pb.WingState, feather *pb.ItemView, session []byte, cost int64, allowBound bool) *pb.WingQuoteRequest {
	return &pb.WingQuoteRequest{SceneEpoch: c.entry.SceneEpoch, WingId: wing.GetWingId(), WingVersion: wing.GetVersion(),
		UiSessionId: session, AllowBound: allowBound, Feathers: []*pb.WingFeatherRef{{InstanceId: feather.InstanceId, Version: feather.Version, Quantity: cost}}}
}

// growWingWS 仅提交自己当次看到的原报价，再对拍库存、阶段和材料的版本归属。
func growWingWS(t *testing.T, c *client, wing *pb.WingState, feather *pb.ItemView, session []byte, cost int64, rank, appearance uint32, allowBound bool) *pb.WingState {
	t.Helper()
	quoteRequest := wingQuoteRequest(c, wing, feather, session, cost, allowBound)
	request := c.send(p.MsgC2SWingQuote, quoteRequest)
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == request && frame.ID == p.MsgS2CWingQuote })
	var quote pb.WingQuote
	must(t, proto.Unmarshal(frame.Payload, &quote))
	if len(quote.QuoteId) != 16 || quote.SceneEpoch != quoteRequest.SceneEpoch || domain.Hex(quote.UiSessionId) != domain.Hex(session) ||
		!proto.Equal(wing, quote.Wing) || quote.TargetRank != rank || quote.TargetAppearanceTier != appearance || quote.AllowBound != allowBound ||
		len(quote.Feathers) != 1 || quote.Feathers[0].Quantity != cost || !proto.Equal(quote.Feathers[0].Item, feather) {
		t.Fatal("原报价未准确冻结角色所选材料、阶段、外观及绑定", &quote)
	}
	op := c.operation()
	op.ExpectedRevision = quote.RoleRevision
	result := c.asset(p.MsgC2SWingAction, &pb.WingActionRequest{Action: 1, QuoteId: quote.QuoteId, UiSessionId: session,
		WingId: wing.GetWingId(), WingVersion: wing.GetVersion(), AllowBound: allowBound, Operation: op})
	confirmed := result.GetWing().GetWing()
	if result.ErrorCode != 0 || result.GetWing().GetAction() != 1 || confirmed.GetRank() != rank || confirmed.GetAppearanceTier() != appearance ||
		confirmed.GetLuck() != 0 || confirmed.GetOilToSix() != 0 || confirmed.GetOilToNine() != 0 || len(confirmed.GetWingId()) != 16 ||
		!slices.EqualFunc(quote.Feathers, result.GetWing().GetFeathers(), func(a, b *pb.WingFeatherSpend) bool { return proto.Equal(a, b) }) {
		t.Fatal("真实翅膀确认错误、初始不为零或未消费原羽毛", result)
	}
	syncWingInventory(c, result.NewRevision)
	left, present := findWingFeather(c, feather.InstanceId)
	if !proto.Equal(c.inventory.GetWing(), confirmed) || (feather.Quantity > cost) != present ||
		present && (left.Quantity != feather.Quantity-cost || left.Version != feather.Version+1 || left.Bound != feather.Bound || left.SourceId != feather.SourceId) {
		t.Fatal("权威库存没有同时确认原翅膀与剩余原堆", c.inventory)
	}
	return confirmed
}

// wingAppearanceRequest 引用原角色独享翅膀与其现有版本，外观修改不带资产报价。
func wingAppearanceRequest(c *client, wing *pb.WingState, tier uint32, hidden bool) *pb.WingActionRequest {
	return &pb.WingActionRequest{Action: 2, WingId: wing.WingId, WingVersion: wing.Version, AppearanceTier: tier, Hidden: hidden, Operation: c.operation()}
}

// syncWingInventory 等待同笔角色版本的完整快照，已读到时不再等重复帧。
func syncWingInventory(c *client, revision uint64) {
	if c.inventory.GetRevision() < revision {
		c.inventoryAt(revision)
	}
}

// waitWingOperation 模拟客户端还未读取响应时，单独向数据库查证该原操作。
func waitWingOperation(t *testing.T, ctx context.Context, store *persistence.Store, roleID, opID string) persistence.Operation {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	var op persistence.Operation
	for {
		err := store.DB.Collection("operations").FindOne(deadline, bson.M{"_id": "asset:" + roleID + ":" + opID}).Decode(&op)
		if !errors.Is(err, mongo.ErrNoDocuments) || deadline.Err() != nil {
			must(t, err)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if op.Kind != persistence.AssetWingAction || op.Code != 0 || op.Wing == nil {
		t.Fatal("翅膀外观没有产生真实持久回执", op)
	}
	return op
}

// verifyWingWorldStorage 核对角色全集合未把翅膀嵌入普通资产，以及两个所有者各自只持有一件。
func verifyWingWorldStorage(t *testing.T, ctx context.Context, store *persistence.Store, before domain.Role, final, second *pb.WingState, roleID, otherID string) {
	t.Helper()
	after, err := store.Role(ctx, roleID)
	must(t, err)
	if after.EconomicSeq != before.EconomicSeq+2 || after.SourceSeq != before.SourceSeq || after.Wallet != before.Wallet ||
		!slices.Equal(after.Items, before.Items) || !slices.Equal(after.Equipped, before.Equipped) || !proto.Equal(after.Wing.Proto(), final) {
		t.Fatal("换外观或重试消耗了材料、装备或重复推进经济流水", after.Wing)
	}
	var rows = []struct {
		role  string
		state *pb.WingState
	}{{roleID, final}, {otherID, second}}
	for _, row := range rows {
		var stored domain.WingState
		must(t, store.DB.Collection("role_wings").FindOne(ctx, bson.M{"_id": row.role}).Decode(&stored))
		if stored.OwnerRoleID != row.role || !proto.Equal(stored.Proto(), row.state) {
			t.Fatal("独立永久翅膀文档不符合各自角色原归属", stored)
		}
		var roleDocument bson.M
		must(t, store.DB.Collection("roles").FindOne(ctx, bson.M{"_id": row.role}).Decode(&roleDocument))
		if _, embedded := roleDocument["wing"]; embedded {
			t.Fatal("翅膀不应作为普通角色集合资产一同替换")
		}
	}
	count, err := store.DB.Collection("role_wings").CountDocuments(ctx, bson.M{})
	must(t, err)
	if count != 2 {
		t.Fatal("重复激活尝试或重放制造了额外的永久翅膀", count)
	}
}
