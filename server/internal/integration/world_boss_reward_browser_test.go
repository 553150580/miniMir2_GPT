// 本文件将真实双客户端击杀留下的附件交给 Creator 触屏端领取，并核对 MongoDB 原实例。
package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// worldRewardBrowserItem 是击杀时已确认的原物品，只用于对拍浏览器读取和入包。
type worldRewardBrowserItem struct {
	InstanceID string `json:"instanceId"` // 原奖励物品实例。
	ItemID     uint32 `json:"itemId"`     // 原物品模板。
	Name       string `json:"name"`       // 用于核对可见名称。
	Quantity   string `json:"quantity"`   // 十进制原数量，避免 JavaScript 数字精度歧义。
	Bound      bool   `json:"bound"`      // 原击杀时刻确认的绑定。
}

// worldRewardBrowserPlayer 保存该触屏角色对应的原附着和原发放信息，不携带口令或票据。
type worldRewardBrowserPlayer struct {
	Name     string                   `json:"name"`     // 真实角色选择槽名称。
	Identity string                   `json:"identity"` // 永久入口和角色身份。
	Binding  string                   `json:"binding"`  // 触屏领取前已断开的原战斗绑定。
	RewardID string                   `json:"rewardId"` // 等待领取的原附件。
	Template uint32                   `json:"template"` // 实际被击杀的首领模板。
	Rank     uint32                   `json:"rank"`     // 真实有效伤害榜的原名次。
	EarnedAt string                   `json:"earnedAt"` // 原击杀时刻的毫秒整数。
	Items    []worldRewardBrowserItem `json:"items"`    // 原物品标识、名和绑定。
}

// TestWorldBossRewardsBrowserMongoWS 验证真实构建经正常选角和新票领取已确认的两种世界首领奖。
func TestWorldBossRewardsBrowserMongoWS(t *testing.T) {
	webRoot := os.Getenv("MINIMIR_WORLD_BOSS_REWARDS_BROWSER_WEB_ROOT")
	if webRoot == "" {
		t.Skip("需要独立 Creator Web Mobile 构建")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build, err := filepath.EvalSymlinks(webRoot)
	must(t, err)
	build, err = filepath.Abs(build)
	must(t, err)
	relative, err := filepath.Rel(work, build)
	must(t, err)
	if relative == "." || !filepath.IsLocal(relative) {
		t.Fatal("世界首领触屏领奖只接受工作区内的独立构建")
	}
	_, err = os.Stat(filepath.Join(build, "index.html"))
	must(t, err)
	trial := prepareWorldBossRewardTrial(t, false)
	players := []*worldBossRewardPlayer{&trial.winner, &trial.partner}
	seeds := make([]worldRewardBrowserPlayer, 0, len(players))
	for _, player := range players {
		// 原连接只做真实战斗和只读查奖，浏览器领取前关闭此附着。
		original := player.reward
		identity := strconv.FormatUint(player.summary.Key.OriginServerId, 10) + ":" + domain.Hex(player.summary.Key.RoleId)
		seed := worldRewardBrowserPlayer{Name: player.summary.Name, Identity: identity, Binding: strconv.FormatUint(player.client.binding, 10),
			RewardID: domain.Hex(original.RewardId), Template: original.TemplateId, Rank: original.SourceRank, EarnedAt: strconv.FormatInt(original.EarnedAtUnixMs, 10)}
		for _, item := range original.Items {
			catalog, exists := trial.store.Cfg.Fixture.Item(item.ItemId)
			if !exists {
				t.Fatal("缺少实战奖励的原物品名称", item.ItemId)
			}
			seed.Items = append(seed.Items, worldRewardBrowserItem{InstanceID: domain.Hex(item.InstanceId), ItemID: item.ItemId,
				Name: catalog.Name, Quantity: strconv.FormatInt(item.Quantity, 10), Bound: item.Bound})
		}
		seeds = append(seeds, seed)
		player.client.close()
	}
	// 网页文件从实际构建读取，账号 RPC 转发到本次隔离节点，游戏 WebSocket 直接走区服公布的地址。
	upstream, err := url.Parse("http://" + trial.store.Cfg.AccountAddress)
	must(t, err)
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(work, "world-boss-reward-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	raw, err := json.Marshal(map[string]any{"baseUrl": web.URL, "endpoint": trial.route.assignment.GameWSURL,
		"region": trial.route.origin.RegionName, "server": trial.route.origin.Name, "version": trial.store.Cfg.Fixture.RuleVersion,
		"bossName": trial.store.Cfg.Fixture.WorldBossRelease.DisplayName, "occurrence": trial.sealed.LatestResult.OccurrenceId, "players": seeds})
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Logf("世界首领奖励触屏证据目录 %s", evidence)
	browserCtx, stop := context.WithTimeout(trial.ctx, 220*time.Second)
	defer stop()
	command := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_world_boss_rewards_browser.cjs"), manifest)
	command.Dir = repo
	command.Env = append(os.Environ(), "MINIMIR_WORLD_BOSS_REWARD_WINNER_ACCOUNT="+trial.winner.account,
		"MINIMIR_WORLD_BOSS_REWARD_WINNER_PASSWORD="+trial.winner.password, "MINIMIR_WORLD_BOSS_REWARD_PARTNER_ACCOUNT="+trial.partner.account,
		"MINIMIR_WORLD_BOSS_REWARD_PARTNER_PASSWORD="+trial.partner.password)
	log, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("世界首领奖励触屏未完成：%v\n%s", err, log)
	}
	var observations struct {
		Passed  bool // 浏览器已完成真实触屏与每个响应的对拍。
		Players []struct {
			RewardID string // 浏览器请求携带的原附件编号。
			OpID     string // 浏览器实际生成的操作编号。
		}
	}
	report, err := os.ReadFile(filepath.Join(evidence, "world-boss-reward-browser-report.json"))
	must(t, err)
	must(t, json.Unmarshal(report, &observations))
	if !observations.Passed || len(observations.Players) != 2 {
		t.Fatal("浏览器证据未覆盖两名真实参战者")
	}
	for index, player := range players {
		seen := observations.Players[index]
		if seen.RewardID != domain.Hex(player.reward.RewardId) || len(domain.Bytes(seen.OpID)) != 16 {
			t.Fatal("浏览器未沿用原附件或没有真实经济命令", seen)
		}
		verifyWorldBossBrowserClaim(t, trial, player, seen.OpID)
	}
	var result persistence.WorldBossResult // 领取后核对场次仍保存同一份原击杀证据。
	must(t, trial.store.DB.Collection("world_boss_results").FindOne(trial.ctx, bson.M{"occurrence_id": trial.sealed.LatestResult.OccurrenceId}).Decode(&result))
	if result.Outcome != persistence.WorldBossKilled || result.FinishedAt.UnixMilli() != trial.sealed.LatestResult.FinishedAtUnixMs ||
		len(result.Contributions) != 2 || result.Contributions[0].Damage != 3 || result.Contributions[1].Damage != 2 {
		t.Fatal("触屏领取与真实共享击杀不在同一个原结果", result)
	}
	t.Logf("同一真实击杀的 2 笔领取、3 个原物品实例及审计核对通过")
}

// verifyWorldBossBrowserClaim 对拍原附件、资产事务、审计和快照中的实例，同一角色只成交一次。
func verifyWorldBossBrowserClaim(t *testing.T, trial worldBossRewardTrial, player *worldBossRewardPlayer, opID string) {
	t.Helper()
	store, ctx := trial.store, trial.ctx
	roleID := domain.Hex(player.summary.Key.RoleId)
	operationKey := "asset:" + roleID + ":" + opID
	var reward struct {
		domain.ChallengePendingReward `bson:",inline"`
		RoleID                        string `bson:"role_id"`         // Mongo 原参战者。
		Claimed                       bool   `bson:"claimed"`         // 一次性原附件标记。
		ClaimOperation                string `bson:"claim_operation"` // 关联触屏原操作。
	}
	must(t, store.DB.Collection("world_boss_rewards").FindOne(ctx, bson.M{"_id": domain.Hex(player.reward.RewardId)}).Decode(&reward))
	if !reward.Claimed || reward.RoleID != roleID || reward.ClaimOperation != operationKey || reward.SourceID != trial.sealed.LatestResult.OccurrenceId ||
		reward.SourceRank != player.reward.SourceRank || reward.EarnedAt.UnixMilli() != player.reward.EarnedAtUnixMs {
		t.Fatal("永久奖励未关联真实触屏的原操作和场次", reward)
	}
	var receipt persistence.Operation // 已到账原资产回执。
	var audit struct {
		Claim *domain.ChallengePendingReward `bson:"challenge_reward_claim"` // 与资产事务同提交的审计附件。
	}
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": operationKey}).Decode(&receipt))
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": operationKey}).Decode(&audit))
	if receipt.Kind != persistence.AssetChallengeRewardClaim || receipt.Code != 0 || receipt.ChallengeReward == nil || audit.Claim == nil ||
		receipt.ChallengeReward.RewardID != reward.RewardID || audit.Claim.RewardID != reward.RewardID || len(reward.Items) != len(player.reward.Items) ||
		len(receipt.ChallengeReward.Items) != len(reward.Items) || len(audit.Claim.Items) != len(reward.Items) {
		t.Fatal("资产回执和审计未保留原附件", receipt, audit)
	}
	count, err := store.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": roleID, "kind": persistence.AssetChallengeRewardClaim, "code": 0})
	must(t, err)
	stored, err := store.Role(ctx, roleID)
	must(t, err)
	if count != 1 {
		t.Fatal("同一角色的触屏领取重复到账", roleID, count)
	}
	for index, original := range player.reward.Items {
		copies := 0
		for _, item := range stored.Items {
			if proto.Equal(item.Proto(), original) {
				copies++
			}
		}
		for _, item := range []*pb.ItemView{reward.Items[index].Proto(), receipt.ChallengeReward.Items[index].Proto(), audit.Claim.Items[index].Proto()} {
			if !proto.Equal(item, original) {
				t.Fatal("领取改变了原物品实例、数量或绑定", item, original)
			}
		}
		if copies != 1 {
			t.Fatal("角色背包未精确收到一次原实例", original, copies)
		}
	}
}
