// 本文件使用真实世界首领、双客户端和 MongoDB 验证角色独立勋章。
package integration

import (
	"bytes"
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

// TestMedalWorldBossMongoWS 核对资格、原绑定、报价升级、重连回执和原角色权威属性。
func TestMedalWorldBossMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	fixture := readMedalFixture(t, os.Getenv("MINIMIR_MEDAL_TEST_FIXTURE"))
	trial := startWingWorldTrial(t, fixture)
	first, second := trial.winner.client, trial.partner.client
	firstID, secondID := domain.Hex(trial.winner.summary.Key.RoleId), domain.Hex(trial.partner.summary.Key.RoleId)
	fragmentID := fixture.Medal.FragmentItemID
	free := claimWingFeathers(t, first, trial.winner.reward, fragmentID, false)
	bound := claimWingFeathers(t, second, trial.partner.reward, fragmentID, true)
	if free.GetQuantity() != 6 || bound.GetQuantity() != 6 || bytes.Equal(free.InstanceId, bound.InstanceId) ||
		first.inventory.GetMedal() != nil || second.inventory.GetMedal() != nil {
		t.Fatal("勋章碎片没有从同场合格参与者的独立原奖励进入背包")
	}
	level := fixture.Medal.Levels[0]
	firstSession, secondSession := domain.Bytes(domain.ID()), domain.Bytes(domain.ID())
	denied := second.send(p.MsgC2SMedalQuote, medalQuoteRequest(second, secondSession, bound, level.FragmentCost, false))
	travelExpectError(second, p.MsgC2SMedalQuote, denied, p.CodeMedalInputs)
	foreign := first.send(p.MsgC2SMedalQuote, medalQuoteRequest(first, firstSession, bound, level.FragmentCost, true))
	travelExpectError(first, p.MsgC2SMedalQuote, foreign, p.CodeMedalInputs)

	before := proto.CloneOf(first.player().GetCombatAttributes())
	firstRole, err := trial.store.Role(trial.ctx, firstID)
	must(t, err)
	quote := requestMedalWS(t, first, firstSession, free, level, false)
	request := medalCommitRequest(first, quote)
	scene, binding := first.entry.SceneEpoch, first.binding
	first.send(p.MsgC2SMedalCommit, request)
	committed := waitMedalOperation(t, trial.ctx, trial.store, firstID, domain.Hex(request.Operation.OpId))
	first.close()
	first = trial.attach(t, &trial.winner, false)
	if first.entry.SceneEpoch != scene || first.binding == binding {
		t.Fatal("原角色重连未恢复原场景或未换绑定代")
	}
	replayed := first.asset(p.MsgC2SMedalCommit, request)
	expectMedalReceipt(t, first, quote, replayed)
	if !proto.Equal(replayed.GetMedal(), committed.Medal.Proto()) {
		t.Fatal("断线后未返回原唯一勋章回执", replayed, committed.Medal)
	}
	checkMedalFragmentLeft(t, first, free, level.FragmentCost)
	assertMedalCombat(t, first, before, level)
	firstAfter, err := trial.store.Role(trial.ctx, firstID)
	must(t, err)
	if firstAfter.Wallet != firstRole.Wallet || firstAfter.EconomicSeq != firstRole.EconomicSeq+1 ||
		!slices.Equal(firstAfter.Equipped, firstRole.Equipped) || second.inventory.GetMedal() != nil {
		t.Fatal("重放影响钱包、普通穿戴，或改变另一角色勋章")
	}
	remaining, _ := findWingFeather(first, free.InstanceId)
	capped := first.send(p.MsgC2SMedalQuote, medalQuoteRequest(first, firstSession, remaining, fixture.Medal.Levels[1].FragmentCost, false))
	travelExpectError(first, p.MsgC2SMedalQuote, capped, p.CodeMedalInputs)

	secondQuote := requestMedalWS(t, second, secondSession, bound, level, true)
	secondResult := second.asset(p.MsgC2SMedalCommit, medalCommitRequest(second, secondQuote))
	expectMedalReceipt(t, second, secondQuote, secondResult)
	checkMedalFragmentLeft(t, second, bound, level.FragmentCost)
	for _, roleID := range []string{firstID, secondID} {
		var state bson.M
		must(t, trial.store.DB.Collection("role_medals").FindOne(trial.ctx, bson.M{"_id": roleID}).Decode(&state))
		if state["level"] != int64(1) && state["level"] != int32(1) {
			t.Fatal("每个角色必须各存一份勋章一级档案", state)
		}
		var raw bson.M
		must(t, trial.store.DB.Collection("roles").FindOne(trial.ctx, bson.M{"_id": roleID}).Decode(&raw))
		if _, embedded := raw["medal"]; embedded {
			t.Fatal("勋章不应变成普通角色库存字段")
		}
	}
	count, err := trial.store.DB.Collection("role_medals").CountDocuments(trial.ctx, bson.M{})
	must(t, err)
	if count != 2 {
		t.Fatal("原回执重放或错误报价创建了额外勋章", count)
	}
	t.Log("世界首领合格参与者实领原绑定与非绑碎片，两角色分别激活；等级上限、断线原回执和战斗主属性及物防核对通过")
}

// TestMedalDefaultClosedMongoWS 原规则仍可入场与穿装备，勋章协议和状态关闭。
func TestMedalDefaultClosedMongoWS(t *testing.T) {
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		if cfg.Fixture.Medal != nil {
			t.Fatal("正式规则启用了隔离勋章")
		}
	})
	trial := worldBossRewardTrial{ctx: ctx, store: store, endpoint: endpoint, route: route}
	role := trial.create(t, "勋章默认", time.Now().Add(-time.Hour), false)
	c := trial.attach(t, &role, true)
	if c.inventory.GetMedal() != nil {
		t.Fatal("默认库存返回勋章数据")
	}
	req := c.send(p.MsgC2SMedalQuote, &pb.MedalQuoteRequest{SceneEpoch: c.entry.SceneEpoch,
		Fragments:   []*pb.MedalFragmentRef{{InstanceId: domain.Bytes(domain.ID()), Version: 1, Quantity: 1}},
		UiSessionId: domain.Bytes(domain.ID())})
	travelExpectError(c, p.MsgC2SMedalQuote, req, p.CodeMedalUnavailable)
	count, err := store.DB.Collection("role_medals").CountDocuments(ctx, bson.M{})
	must(t, err)
	if count != 0 {
		t.Fatal("默认规则的请求写入了勋章")
	}
}

// readMedalFixture 只读取候选编译器冻结的首领奖励与勋章成长装配。
func readMedalFixture(t *testing.T, path string) devconfig.Fixture {
	t.Helper()
	raw, err := os.ReadFile(path)
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	if fixture.Medal == nil || len(fixture.Medal.Levels) != 3 || fixture.Wings == nil {
		t.Fatal("勋章验收缺少隔离前三档或原世界首领奖励")
	}
	return fixture
}

// medalQuoteRequest 引用实际选择的碎片堆、观察到的版本和当次界面权限。
func medalQuoteRequest(c *client, session []byte, item *pb.ItemView, cost int64, allowBound bool) *pb.MedalQuoteRequest {
	return &pb.MedalQuoteRequest{SceneEpoch: c.entry.SceneEpoch, MedalVersion: c.inventory.GetMedal().GetVersion(),
		UiSessionId: session, AllowBound: allowBound,
		Fragments: []*pb.MedalFragmentRef{{InstanceId: item.InstanceId, Version: item.Version, Quantity: cost}}}
}

// requestMedalWS 核对服务端报价对原堆、可见属性和等级的授权。
func requestMedalWS(t *testing.T, c *client, session []byte, item *pb.ItemView, target devconfig.MedalLevelConfig, allowBound bool) *pb.MedalQuote {
	t.Helper()
	request := medalQuoteRequest(c, session, item, target.FragmentCost, allowBound)
	seq := c.send(p.MsgC2SMedalQuote, request)
	f := travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool {
		return f.Request == seq && (f.ID == p.MsgS2CMedalQuote || f.ID == p.MsgC2SMedalQuote)
	})
	var quote pb.MedalQuote
	if f.ID != p.MsgS2CMedalQuote {
		t.Fatalf("勋章报价被拒绝 %v", f)
	}
	must(t, proto.Unmarshal(f.Payload, &quote))
	if len(quote.QuoteId) != 16 || quote.SceneEpoch != request.SceneEpoch || !bytes.Equal(quote.UiSessionId, session) ||
		quote.TargetLevel != target.Level || quote.TargetMainAttack != target.MainAttack || quote.TargetDefense != target.Defense ||
		quote.AllowBound != allowBound || !proto.Equal(quote.GetMedal(), c.inventory.GetMedal()) || len(quote.Fragments) != 1 {
		t.Fatal("勋章报价未冻结原属性、勋章版本或绑定权限", &quote)
	}
	spend := quote.Fragments[0]
	if !bytes.Equal(spend.InstanceId, item.InstanceId) || spend.Version != item.Version || spend.Quantity != target.FragmentCost || spend.Bound != item.Bound {
		t.Fatal("勋章报价没有引用实际碎片堆和原绑定", spend)
	}
	return &quote
}

// medalCommitRequest 使用报价修订和原经济序号提交本次永久升级。
func medalCommitRequest(c *client, quote *pb.MedalQuote) *pb.MedalCommitRequest {
	op := c.operation()
	op.ExpectedRevision = quote.RoleRevision
	return &pb.MedalCommitRequest{QuoteId: quote.QuoteId, UiSessionId: quote.UiSessionId, Operation: op}
}

// expectMedalReceipt 检查首次和重放均只推进一次，并与库存状态一致。
func expectMedalReceipt(t *testing.T, c *client, quote *pb.MedalQuote, result *pb.OperationResult) {
	t.Helper()
	got := result.GetMedal()
	if result.ErrorCode != 0 || result.Pending || got == nil || !bytes.Equal(got.QuoteId, quote.QuoteId) ||
		got.BeforeLevel != quote.GetMedal().GetLevel() || got.GetMedal().GetLevel() != quote.TargetLevel ||
		got.GetMedal().GetVersion() != quote.GetMedal().GetVersion()+1 ||
		!slices.EqualFunc(got.Fragments, quote.Fragments, func(a, b *pb.MedalFragmentSpend) bool { return proto.Equal(a, b) }) {
		t.Fatal("勋章回执的原报价、投入或版本不一致", result)
	}
	syncWingInventory(c, result.NewRevision)
	if !proto.Equal(got.Medal, c.inventory.GetMedal()) {
		t.Fatal("勋章回执与权威库存不一致", c.inventory)
	}
}

// checkMedalFragmentLeft 确认剩余材料保留原实例、绑定和来源。
func checkMedalFragmentLeft(t *testing.T, c *client, original *pb.ItemView, spent int64) {
	t.Helper()
	left, present := findWingFeather(c, original.InstanceId)
	if !present || left.Quantity != original.Quantity-spent || left.Version != original.Version+1 ||
		left.Bound != original.Bound || left.SourceId != original.SourceId {
		t.Fatal("勋章消耗误改原碎片堆或重放再次扣除", left)
	}
}

// assertMedalCombat 在真实场景基线对拍战士主攻击与物防的两端固定增益。
func assertMedalCombat(t *testing.T, c *client, before *pb.CombatAttributes, level devconfig.MedalLevelConfig) {
	t.Helper()
	check := func(after *pb.CombatAttributes) bool {
		return after != nil && before != nil && after.PhysicalAttackMin == before.PhysicalAttackMin+level.MainAttack &&
			after.PhysicalAttackMax == before.PhysicalAttackMax+level.MainAttack &&
			after.PhysicalDefenceMin == before.PhysicalDefenceMin+level.Defense && after.PhysicalDefenceMax == before.PhysicalDefenceMax+level.Defense
	}
	if !check(c.player().GetCombatAttributes()) {
		travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool {
			return f.ID == p.MsgS2CSceneBaseline && check(c.player().GetCombatAttributes())
		})
	}
}

// waitMedalOperation 模拟客户端未读取响应时，独立确认 MongoDB 已提交原事务。
func waitMedalOperation(t *testing.T, ctx context.Context, store *persistence.Store, roleID, opID string) persistence.Operation {
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
	if op.Code != 0 || op.Medal == nil {
		t.Fatal("勋章未写入永久回执", op.Code)
	}
	return op
}
