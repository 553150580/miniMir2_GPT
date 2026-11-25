// 本文件使用隔离 MongoDB 和正常游戏 WebSocket 验证镜像奖励与兑换的资产边界。
package integration

import (
	"bytes"
	"fmt"
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
)

// TestMirrorArenaExchangeAtomicMongoWS 验证资金、报价、满包、绑定来源与健康新票重放。
func TestMirrorArenaExchangeAtomicMongoWS(t *testing.T) {
	cfg := m1FieldConfig(t)
	configureMirrorArenaTrial(t, &cfg.Fixture)
	// 工作簿尚未登记正式羽毛物品。专用测试用原可堆叠物品 35 验证资产通路。
	const itemID, price, quantity, earned = uint32(35), int64(250), int64(2), int64(3750)
	cfg.Fixture.MirrorArenaRelease.Settlement = &devconfig.MirrorArenaSeasonSettlement{
		RankingTiebreak: devconfig.MirrorArenaRankingEarlierLastChallenge, ParticipationMinChallenges: 10,
		Ranks:         []devconfig.MirrorArenaSeasonRank{{FirstRank: 1, LastRank: 1, Reward: devconfig.TaskCodexReward{BoundYuanbaoUnits: earned}}},
		Participation: devconfig.TaskCodexReward{BoundYuanbaoUnits: 7},
	}
	cfg.Fixture.MirrorArenaRelease.Exchange = &devconfig.MirrorArenaExchangeRule{ItemID: itemID, Price: price, Quantity: quantity}
	must(t, cfg.Fixture.ValidateMirrorArena())
	for i := range cfg.Fixture.Monsters {
		cfg.Fixture.Monsters[i].HP, cfg.Fixture.Monsters[i].AttackMin, cfg.Fixture.Monsters[i].AttackMax = 5000, 0, 0
	}
	cfg, ctx, store := m1FieldServices(t, cfg)
	endpoint, token, key := m1FieldRole(t, cfg, 0)
	_, otherToken, otherKey := m1FieldRole(t, cfg, 1)
	roleID := domain.Hex(key.RoleId)
	role, err := store.Role(ctx, roleID)
	must(t, err)
	if len(role.Items) < 3 {
		t.Fatal("独立新角色缺少用于正常清包的初始物品")
	}
	_, err = store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": bson.M{"capacity": len(role.Items)}})
	must(t, err)

	// 只准备已结转积分与原待领附件前态。所有读取、领取、兑换和回执均沿真实协议。
	reward, season := domain.ID(), domain.ID()
	now := time.Now().UTC()
	_, err = store.DB.Collection("mirror_arena_exchange_wallets").InsertOne(ctx, bson.M{
		"_id": fmt.Sprintf("%d:%s", role.Origin, roleID), "origin": role.Origin, "role_id": roleID, "points": 2 * price, "updated_at": now})
	must(t, err)
	_, err = store.DB.Collection("mirror_arena_season_rewards").InsertOne(ctx, bson.M{
		"_id": reward, "reward_id": reward, "role_id": roleID, "origin": role.Origin, "source_kind": "mirror_season_rank", "source_id": season,
		"template_id": cfg.Fixture.MirrorArena.TemplateID, "rule_version": cfg.Fixture.RuleVersion, "source_run_id": cfg.RunID,
		"source_rank": 1, "earned_at": now, "claimed": false, "mirror_assets": bson.M{"bound_ingot": earned},
	})
	must(t, err)
	player := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
	defer func() { player.close() }()
	player.auto(false, 1)
	foreign := join(t, cfg.GameAddress, issue(t, endpoint, otherToken, otherKey))
	defer foreign.close()
	foreign.auto(false, 1)
	read := func(c *client) *pb.MirrorArenaState {
		state := mirrorArenaCall(c, &pb.MirrorArenaRequest{Action: 1})
		if state.ErrorCode != 0 || state.ExchangePrice != price || state.ExchangeItemId != itemID || state.ExchangePackQuantity != quantity {
			t.Fatal("镜像大厅未提供本服核准的权威兑换条款", state)
		}
		return state
	}
	before := read(player)
	if before.ExchangePoints != price*2 || len(before.SeasonPendingRewards) != 1 ||
		before.SeasonPendingRewards[0].BoundYuanbaoUnits != earned || !bytes.Equal(before.SeasonPendingRewards[0].RewardId, domain.Bytes(reward)) {
		t.Fatal("原兑换余额或独立元宝附件查询丢失", before)
	}
	if state := read(foreign); state.ExchangePoints != 0 || len(state.SeasonPendingRewards) != 0 {
		t.Fatal("未隔离其他角色的奖励与积分", state)
	}
	exchange := func(c *client) *pb.MirrorArenaExchangeRequest {
		return &pb.MirrorArenaExchangeRequest{Operation: c.operation(), PackCount: 2,
			ExpectedPrice: price, ExpectedItemId: itemID, ExpectedPackQuantity: quantity}
	}
	claim := func(c *client) *pb.ClaimChallengeReward {
		return &pb.ClaimChallengeReward{Operation: c.operation(), RewardId: domain.Bytes(reward)}
	}
	require := func(c *client, id uint16, request proto.Message, code uint32) *pb.OperationResult {
		result := c.asset(id, request)
		if result.ErrorCode != code || result.Pending {
			t.Fatalf("经济请求 %d 原回执应为 %d：%v", id, code, result)
		}
		c.inventoryAt(result.NewRevision)
		return result
	}
	require(foreign, p.MsgC2SClaimChallengeReward, claim(foreign), p.CodeOperationConflict)
	require(foreign, p.MsgC2SMirrorArenaExchange, exchange(foreign), p.CodeMirrorExchangeFunds)
	wrongPrice := exchange(player)
	wrongPrice.ExpectedPrice--
	require(player, p.MsgC2SMirrorArenaExchange, wrongPrice, p.CodeOperationConflict)
	fullRequest := exchange(player)
	require(player, p.MsgC2SMirrorArenaExchange, fullRequest, p.CodeEquipmentBagFull)
	if points := read(player).ExchangePoints; points != 2*price {
		t.Fatal("错误报价或满包扣除了积分", points)
	}
	claimRequest := claim(player)
	require(player, p.MsgC2SClaimChallengeReward, claimRequest, 0)
	require(player, p.MsgC2SClaimChallengeReward, claimRequest, 0)
	require(player, p.MsgC2SClaimChallengeReward, claim(player), p.CodeOperationConflict)
	if state := read(player); len(state.SeasonPendingRewards) != 0 || state.ExchangePoints != 2*price {
		t.Fatal("领取绑定元宝影响兑换余额或附件未消失", state)
	}

	var filler [][]byte
	for _, item := range player.inventory.Items {
		if item.ItemId == 218 || item.ItemId == 193 {
			filler = append(filler, item.InstanceId)
			if len(filler) == 2 {
				break
			}
		}
	}
	if len(filler) != 2 {
		t.Fatal("新角色应有两件可正常回收的初始物品")
	}
	require(player, p.MsgC2SRecycle, &pb.RecycleRequest{ItemInstanceIds: filler, Operation: player.operation()}, 0)
	require(player, p.MsgC2SMirrorArenaExchange, fullRequest, p.CodeEquipmentBagFull)
	completed := exchange(player)
	// 读取报价时的较旧角色修订不应在同一经济序列及报价仍可信时阻断新操作。
	completed.Operation.ExpectedRevision--
	bought := require(player, p.MsgC2SMirrorArenaExchange, completed, 0)
	receipt := bought.MirrorArenaExchange
	if receipt == nil || receipt.SpentPoints != 2*price || receipt.RemainingPoints != 0 || receipt.ItemId != itemID ||
		receipt.Quantity != 2*quantity || len(receipt.ItemInstanceId) != 16 {
		t.Fatal("未确认原兑换价格、余量和绑定实例身份", bought)
	}
	require(player, p.MsgC2SMirrorArenaExchange, exchange(player), p.CodeMirrorExchangeFunds)
	oldBinding := player.binding
	player.close()
	player = join(t, cfg.GameAddress, issue(t, endpoint, token, key))
	if player.binding <= oldBinding {
		t.Fatal("未获得健康新票")
	}
	replayed := require(player, p.MsgC2SMirrorArenaExchange, completed, 0)
	if !proto.Equal(replayed.MirrorArenaExchange, receipt) || read(player).ExchangePoints != 0 {
		t.Fatal("健康新票重放原兑换发生改判或重复扣除", replayed)
	}
	changed := proto.Clone(completed).(*pb.MirrorArenaExchangeRequest)
	changed.PackCount = 1
	altered := player.asset(p.MsgC2SMirrorArenaExchange, changed)
	if altered.ErrorCode != p.CodeOperationConflict || altered.MirrorArenaExchange != nil {
		t.Fatal("原操作编号接受了不同兑换内容", altered)
	}
	after, err := store.Role(ctx, roleID)
	must(t, err)
	matches := 0
	for _, item := range after.Items {
		if item.SourceKind != persistence.AssetMirrorArenaExchange {
			continue
		}
		matches++
		if !item.Bound || item.ItemID != itemID || item.Version != 1 || item.Quantity != 2*quantity ||
			!slices.Equal(domain.Bytes(item.ID), receipt.ItemInstanceId) || item.SourceID != domain.Hex(completed.Operation.OpId) {
			t.Fatal("入包实例脱离首次绑定兑换及其来源", item)
		}
	}
	successes, err := store.DB.Collection("operations").CountDocuments(ctx, bson.M{
		"role_id": roleID, "kind": bson.M{"$in": []string{persistence.AssetMirrorArenaExchange, persistence.AssetChallengeRewardClaim}}, "code": 0})
	must(t, err)
	if matches != 1 || successes != 2 || after.Wallet.BoundYuanbao != role.Wallet.BoundYuanbao+earned ||
		after.Wallet.Yuanbao != role.Wallet.Yuanbao || after.SourceSeq != role.SourceSeq {
		t.Fatal("积分、一次成果和绑定钱包未对账", matches, successes, after.Wallet, after.SourceSeq)
	}
}
