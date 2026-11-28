// 本文件用四个真实角色击杀同一世界首领，核对原贡献、造册与实收材料。
package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// worldBossSupplyClaim 把一笔到账与实际赢家分组、原附件和领取操作关联。
type worldBossSupplyClaim struct {
	Group       string                        `json:"group"`        // 榜首或合格的普通参与者。
	RoleID      string                        `json:"role_id"`      // 首领奖榜和资产事务共用的永久角色键。
	OperationID string                        `json:"operation_id"` // 唯一领取操作的持久键。
	ClaimedAt   time.Time                     `json:"claimed_at"`   // 实际到包时刻。
	Reward      domain.ChallengePendingReward `json:"reward"`       // 原附件明细，按参与奖再按排名奖排列。
}

// TestMedalWorldBossSupplyMongoWS 比较候选原参与奖与隔离追加排名奖，跟踪普通参与者实际成交及培养消耗。
func TestMedalWorldBossSupplyMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	for _, scenario := range []struct {
		name      string // 原量和追加排名奖的证据分别保存。
		rankBonus bool   // 只在隔离场为榜首追加两种材料。
	}{{"participation", false}, {"ranking", true}} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := readMedalFixture(t, os.Getenv("MINIMIR_MEDAL_TEST_FIXTURE"))
			sourceHash := fixture.Hash
			directory := os.Getenv("MINIMIR_WORLD_BOSS_SUPPLY_REPORT_DIR")
			fixture, raw := worldBossSupplyFixture(t, fixture, scenario.rankBonus)
			trial := startWorldBossSupplyTrial(t, fixture)
			active, expired := time.Now().Add(time.Hour), time.Now().Add(-time.Minute)
			players := []worldBossRewardPlayer{
				trial.create(t, "供给榜首", active, false), trial.create(t, "供给普通", active, false),
				trial.create(t, "供给绑定", expired, false), trial.create(t, "供给买家", active, false),
			}
			// 第一次附着前只准备月卡与直购用资金，所有物品由服务器实战生成。
			for i, yuanbao := range []int64{40_000, 1_000} {
				changed, err := trial.store.DB.Collection("roles").UpdateByID(trial.ctx, worldBossSupplyRoleID(players[i+2]), bson.M{"$set": bson.M{"wallet.yuanbao_units": yuanbao}})
				must(t, err)
				if changed.ModifiedCount != 1 {
					t.Fatal("没有建立隔离角色的交易资金", i)
				}
			}
			for i := range players {
				players[i].client = trial.attach(t, &players[i], i < 3)
			}
			result := fightWorldBossSupply(t, trial, players)
			// 第三名在击杀后、领奖前真实购卡。市场权限改变后，原绑定材料仍应保持绑定。
			card := buyWorldBossSupplyPrivilege(t, players[2].client, result.FinishedAt)
			claims := claimWorldBossSupply(t, trial, players, result)
			market := tradeWorldBossSupply(t, trial, players, claims, result.OccurrenceID)
			if directory != "" {
				must(t, os.WriteFile(filepath.Join(directory, "world-boss-"+scenario.name+"-fixture.json"), raw, 0o644))
				report, err := json.MarshalIndent(map[string]any{
					"source_fixture_sha256": sourceHash, "effective_fixture_sha256": fixture.Hash, "result": result,
					"rules": fixture.WorldBoss.Rewards, "claims": claims, "post_kill_card": card, "market": market,
				}, "", "  ")
				must(t, err)
				must(t, os.WriteFile(filepath.Join(directory, "world-boss-"+scenario.name+".json"), append(report, '\n'), 0o644))
			}
			t.Logf("场次 %s，原榜伤害 6、3、2，零贡献无附件；%d 笔原奖仅到账一次，普通卖家完成 %d 笔同源成交", result.OccurrenceID, len(claims), len(market.Sales))
		})
	}
}

// worldBossSupplyFixture 延用真实模板、位图、材料及参与奖，仅固定样本用的战斗和日程。
func worldBossSupplyFixture(t *testing.T, fixture devconfig.Fixture, rankBonus bool) (devconfig.Fixture, []byte) {
	t.Helper()
	boss := fixture.WorldBossRelease
	if boss == nil || boss.TemplateID != 2 || boss.Rewards == nil || boss.Rewards.MinimumDamage != 1 ||
		len(boss.Rewards.Participation) != 2 || len(boss.Rewards.Ranks) != 0 || fixture.ItemMarket == nil {
		t.Fatal("采样需要模板 2 现有的羽毛、碎片参与奖和道具市场")
	}
	if rankBonus {
		boss.Rewards.Ranks = []devconfig.WorldBossRankReward{{FirstRank: 1, LastRank: 1, Items: []devconfig.ChallengeRewardItem{
			{ItemID: fixture.Wings.FeatherItemID, Quantity: 1}, {ItemID: fixture.Medal.FragmentItemID, Quantity: 2},
		}}}
	}
	local := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60)).Truncate(time.Millisecond).Add(-2 * time.Second)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	boss.DailyOpenOffsetsMS, boss.WindowMS, boss.ReturnDelayMS = []int64{local.Sub(midnight).Milliseconds()}, 300_000, 600
	fixture.WorldBossVariantReleases = nil
	for i := range fixture.ChallengeMonsters {
		if monster := &fixture.ChallengeMonsters[i]; monster.ID == boss.MonsterID {
			monster.HP, monster.AttackMin, monster.AttackMax, monster.Agility = 11, 0, 0, 0
			monster.MoveMS, monster.DefenceMin, monster.DefenceMax = 60_000, 0, 0
		}
	}
	for i := range fixture.Items {
		if sword := &fixture.Items[i]; sword.ID == 218 {
			sword.AttackMinBonus, sword.AttackMaxBonus = 3, 3
		}
	}
	must(t, fixture.Validate())
	raw, err := json.Marshal(fixture)
	must(t, err)
	fixture.Hash = persistence.Hash(raw)
	return fixture, raw
}

// startWorldBossSupplyTrial 启动工作簿候选的真实账号服与游戏服，使用独立测试库和端口。
func startWorldBossSupplyTrial(t *testing.T, fixture devconfig.Fixture) worldBossRewardTrial {
	t.Helper()
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) { cfg.Fixture = fixture })
	return worldBossRewardTrial{ctx: ctx, store: store, endpoint: endpoint, route: route}
}

// fightWorldBossSupply 通过普通入场、移动与普攻形成不同伤害，并交叉验证永久结果和前端可见排行。
func fightWorldBossSupply(t *testing.T, trial worldBossRewardTrial, players []worldBossRewardPlayer) persistence.WorldBossResult {
	t.Helper()
	first, second, third := players[0].client, players[1].client, players[2].client
	open := queryWorldBossTest(t, first)
	for _, player := range players {
		enterWorldBossTest(t, player.client, open)
	}
	approachWingBoss(t, first, -1, 0)
	approachWingBoss(t, second, 0, -1)
	approachWingBoss(t, third, 0, 1)
	time.Sleep(time.Duration(third.player().MoveIntervalMs)*time.Millisecond + 100*time.Millisecond)
	attackWorldBossTest(first)
	waitWorldBossTest(t, second, 0, func(state *pb.WorldBossState) bool { return state.State == 3 && state.Hp == 8 })
	attackWorldBossTest(second)
	waitWorldBossTest(t, first, 0, func(state *pb.WorldBossState) bool { return state.State == 3 && state.Hp == 5 })
	time.Sleep(time.Duration(first.player().AttackIntervalMs)*time.Millisecond + 100*time.Millisecond)
	attackWorldBossTest(first)
	waitWorldBossTest(t, third, 0, func(state *pb.WorldBossState) bool { return state.State == 3 && state.Hp == 2 })
	attackWorldBossTest(third)
	sealed := waitWorldBossTest(t, third, 0, func(state *pb.WorldBossState) bool { return state.State == 4 })
	result := readDurableWorldBossRow(t, trial.store, trial.route.assignment.RunID, open.OccurrenceId)
	if result.Outcome != persistence.WorldBossKilled || !result.ProgressKnown || result.ConfigHash != trial.store.Cfg.Fixture.Hash ||
		result.FinishedAt.UnixMilli() != sealed.GetLatestResult().GetFinishedAtUnixMs() || len(result.Contributions) != len(players) ||
		len(sealed.GetLatestResult().GetRanks()) != 3 {
		t.Fatal("真实场次未形成唯一原击杀和有效排行", result, sealed)
	}
	for i, expected := range [...]int64{6, 3, 2, 0} {
		row := result.Contributions[i]
		if expected > 0 {
			rank := sealed.LatestResult.Ranks[i]
			if rank.Position != uint32(i+1) || rank.Damage != expected || rank.DisplayName != players[i].summary.Name {
				t.Fatal("客户端展示的名次、伤害或角色错误", rank)
			}
		}
		if row.RoleID != worldBossSupplyRoleID(players[i]) || row.Damage != expected || row.RewardEvidence == nil ||
			row.RewardEvidence.Presence != persistence.WorldBossPresenceOnline || row.RewardEvidence.Origin != trial.route.origin.OriginID ||
			row.RewardEvidence.PrivilegeUntil.After(result.FinishedAt) != (i != 2) {
			t.Fatal("MongoDB 未保存原席位、贡献和击杀时卡状态", row)
		}
		returnFromWorldBossTest(players[i].client, open.OccurrenceId)
	}
	return result
}

// claimWorldBossSupply 按真实资格检查附件。每笔以相同操作重放并验证唯一原实例已经入包。
func claimWorldBossSupply(t *testing.T, trial worldBossRewardTrial, players []worldBossRewardPlayer, result persistence.WorldBossResult) []worldBossSupplyClaim {
	t.Helper()
	rules := trial.store.Cfg.Fixture.WorldBoss.Rewards
	claims := make([]worldBossSupplyClaim, 0, len(players)-1)
	for i, player := range players {
		c := player.client
		pending := reconnectBossInfo(t, c).GetWorldPendingRewards()
		contribution := result.Contributions[i]
		if contribution.Damage < rules.MinimumDamage {
			if len(pending) != 0 {
				t.Fatal("零贡献角色领取了原材料", pending)
			}
			continue
		}
		group, lines := "participant", rules.Participation
		if i == 0 {
			group = "head"
			if len(rules.Ranks) != 0 {
				lines = append(slices.Clone(lines), rules.Ranks[0].Items...)
			}
		}
		bound := !contribution.RewardEvidence.PrivilegeUntil.After(result.FinishedAt)
		if len(pending) != 1 || pending[0].SourceRank != uint32(i+1) || pending[0].EarnedAtUnixMs != result.FinishedAt.UnixMilli() || len(pending[0].Items) != len(lines) {
			t.Fatal("原待领与实战排行、时刻或规则不一致", group, pending)
		}
		reward := pending[0]
		for j, item := range reward.Items {
			_, alreadyInBag := findWingFeather(c, item.InstanceId)
			if alreadyInBag || item.ItemId != lines[j].ItemID || item.Quantity != lines[j].Quantity || item.Bound != bound ||
				item.SourceKind != "world_boss" || item.SourceId != result.OccurrenceID {
				t.Fatal("待领原物品已到账或来源、数量和击杀时绑定错误", item)
			}
		}
		request := &pb.ClaimChallengeReward{RewardId: reward.RewardId, Operation: c.operation()}
		claimed := roleCardAssetWS(c, p.MsgC2SClaimChallengeReward, request)
		replay := c.asset(p.MsgC2SClaimChallengeReward, request)
		if claimed.ErrorCode != 0 || claimed.Pending || !proto.Equal(claimed, replay) || len(reconnectBossInfo(t, c).GetWorldPendingRewards()) != 0 {
			t.Fatal("领取与重放未确认原回执或仍有重复待领", claimed, replay)
		}
		var stored struct {
			domain.ChallengePendingReward `bson:",inline"`
			Claimed                       bool      `bson:"claimed"`         // 原附件已在领取事务中关闭。
			ClaimedAt                     time.Time `bson:"claimed_at"`      // 原附件首次完成领取的时间。
			ClaimOperation                string    `bson:"claim_operation"` // 原附件引用同一个经济操作。
		}
		roleID := contribution.RoleID
		opID := "asset:" + roleID + ":" + domain.Hex(request.Operation.OpId)
		must(t, trial.store.DB.Collection("world_boss_rewards").FindOne(trial.ctx, bson.M{"_id": domain.Hex(reward.RewardId), "role_id": roleID}).Decode(&stored))
		var receipt persistence.Operation
		must(t, trial.store.DB.Collection("operations").FindOne(trial.ctx, bson.M{"_id": opID}).Decode(&receipt))
		if !stored.Claimed || stored.ClaimOperation != opID || stored.ClaimedAt.Before(result.FinishedAt) || stored.SourceID != result.OccurrenceID ||
			stored.SourceRunID != result.RunID || stored.ConfigHash != result.ConfigHash || receipt.Code != 0 || receipt.RoleID != roleID ||
			receipt.ChallengeReward == nil || receipt.ChallengeReward.RewardID != stored.RewardID ||
			!slices.EqualFunc(receipt.ChallengeReward.Items, stored.Items, sameWorldBossSupplyItem) || len(stored.Items) != len(lines) {
			t.Fatal("原附件、操作和实收回执无法逐笔对账", stored, receipt)
		}
		role, err := trial.store.Role(trial.ctx, roleID)
		must(t, err)
		for j, original := range stored.Items {
			count := 0
			for _, item := range role.Items {
				if item.ID == original.ID && sameWorldBossSupplyItem(original, item) {
					count++
				}
			}
			if count != 1 || !proto.Equal(original.Proto(), reward.Items[j]) {
				t.Fatal("同一原物品实例在实际背包中重复、漏入或变化", original, count)
			}
		}
		claims = append(claims, worldBossSupplyClaim{Group: group, RoleID: roleID, OperationID: opID, ClaimedAt: stored.ClaimedAt, Reward: stored.ChallengePendingReward})
	}
	count, err := trial.store.DB.Collection("world_boss_rewards").CountDocuments(trial.ctx, bson.M{"source_id": result.OccurrenceID})
	must(t, err)
	if count != int64(len(claims)) {
		t.Fatal("存在重复、未入账或不具资格的原附件", count, len(claims))
	}
	return claims
}

// worldBossSupplyRoleID 返回建角时签票和各持久账本共用的永久角色键。
func worldBossSupplyRoleID(player worldBossRewardPlayer) string {
	return domain.Hex(player.summary.Key.RoleId)
}

// sameWorldBossSupplyItem 比较资产链对外确认的完整来源和库存字段。
func sameWorldBossSupplyItem(left, right domain.Item) bool {
	return proto.Equal(left.Proto(), right.Proto())
}
