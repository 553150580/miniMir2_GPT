//go:build creator_mirror_arena

// 本文件用真实游戏调度器、WebSocket 和 Creator 触屏验收赛季资产与排名。
package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestMirrorSeasonCreatorBrowserMongoWS 从本服旧赛季开始封账，随后用保存界面领奖、兑换和重连。
func TestMirrorSeasonCreatorBrowserMongoWS(t *testing.T) {
	webRoot, fixturePath := os.Getenv("MINIMIR_MIRROR_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_MIRROR_BROWSER_FIXTURE")
	if webRoot == "" || fixturePath == "" {
		t.Skip("需要独立 Creator 构建和显式镜像赛季候选夹具")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build, fixturePath := mirrorBrowserInput(t, work, webRoot), mirrorBrowserInput(t, work, fixturePath)
	_, err = os.Stat(filepath.Join(build, "index.html"))
	must(t, err)
	cfg := m1FieldConfig(t)
	content, err := os.ReadFile(fixturePath)
	must(t, err)
	var candidate devconfig.Fixture
	must(t, json.Unmarshal(content, &candidate))
	digest := sha256.Sum256(content)
	candidate.Hash = hex.EncodeToString(digest[:])
	must(t, candidate.Validate())
	arena := candidate.MirrorArena
	if arena == nil || arena.Settlement == nil || arena.Exchange == nil || candidate.RuleVersion != cfg.Fixture.RuleVersion {
		t.Fatal("必须使用当前规则显式开启奖表和替代兑换商品的隔离候选")
	}
	price, quantity, itemID := arena.Exchange.Price, arena.Exchange.Quantity, arena.Exchange.ItemID
	if itemID != 35 || price != 250 || quantity != 1 {
		t.Fatal("触屏样本只接受既有物品35的明确隔离映射")
	}
	item, found := candidate.Item(itemID)
	if !found {
		t.Fatal("配置的隔离兑换物不存在")
	}
	// 放慢普通怪物战斗，避免触屏期间无关的掉落改变奖品和兑换后的资产对账。
	for i := range candidate.Monsters {
		candidate.Monsters[i].HP, candidate.Monsters[i].AttackMin, candidate.Monsters[i].AttackMax = 5000, 0, 0
	}
	cfg.Fixture, cfg.FixtureFile = candidate, fixturePath
	cfg, ctx, store := m1FieldServices(t, cfg)
	accountURL := "http://" + cfg.AccountAddress + "/rpc"
	player := newMirrorBrowserRole(t, accountURL, cfg.OriginID, 0, "赛季主")
	rival := newMirrorBrowserRole(t, accountURL, cfg.OriginID, 1, "赛季对")
	roleID, rivalID := domain.Hex(player.key.RoleId), domain.Hex(rival.key.RoleId)
	now := time.Now()
	currentStart, _, opens := arena.SeasonAt(now)
	if !opens {
		t.Fatal("候选尚未进入当前赛季")
	}
	previousStart := currentStart.AddDate(0, 0, -int(arena.SeasonDays))
	origin := player.key.OriginServerId
	previousScope := fmt.Sprintf("%d:%d:%d", cfg.RunID, origin, previousStart.Unix())
	currentScope := fmt.Sprintf("%d:%d:%d", cfg.RunID, origin, currentStart.Unix())
	const previousPoints, currentPoints, matches = int64(500), int64(11), uint32(70)
	seasonSetup := seedMirrorBrowserSeasons(t, ctx, store, arena, previousScope, currentScope, previousStart, currentStart,
		origin, roleID, rivalID, player.name, rival.name, previousPoints, currentPoints, matches)
	stored, err := store.Role(ctx, roleID)
	must(t, err)

	// 无角色上线，等待真实运行服 worker 排队关闭整榜，测试没有手动调用持久封账器。
	deadline := time.Now().Add(48 * time.Second)
	for {
		var journal struct {
			Phase    uint32 `bson:"phase"`
			Credited uint32 `bson:"credited"`
		}
		must(t, store.DB.Collection("mirror_arena_seasons").FindOne(ctx, bson.M{"_id": previousScope}).Decode(&journal))
		if journal.Phase == 4 && journal.Credited == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("游戏服未自主完成原赛季封账，最后阶段和结转人数：%+v", journal)
		}
		time.Sleep(100 * time.Millisecond)
	}
	balance, err := store.ReadMirrorExchangePoints(ctx, origin, roleID)
	must(t, err)
	oldFields, err := store.DB.Collection("mirror_arena_scores").FindOne(ctx, bson.M{"_id": previousScope + ":" + roleID}).Raw()
	must(t, err)
	if balance != previousPoints || oldFields.Lookup("points").AsInt64() != 0 {
		t.Fatal("自动封账未一对一结转、或未清除已确认原竞技分", balance)
	}
	t.Log("游戏服实际调度已在角色离线时完成旧赛季冻结、发奖、结转和原竞技分清零")

	directory := &pb.ServerDirectoryResult{}
	rpc(t, accountURL, player.token, p.MsgC2SServerDirectory, &pb.ServerDirectoryRequest{}, directory, 0)
	var region, server string
	for _, group := range directory.Regions {
		for _, item := range group.Servers {
			if item.OriginServerId == origin && item.Available {
				region, server = group.Name, item.Name
			}
		}
	}
	if region == "" || server == "" {
		t.Fatal("参赛角色没有可选入口")
	}
	upstream, err := url.Parse("http://" + cfg.AccountAddress)
	must(t, err)
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(work, "mirror-season-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	raw, err := json.Marshal(map[string]any{
		"baseUrl": web.URL, "endpoint": "ws://" + cfg.GameAddress + "/ws", "region": region, "server": server,
		"attacker": player.name, "rival": rival.name, "ruleVersion": strconv.FormatUint(cfg.Fixture.RuleVersion, 10),
		"normalMap": strconv.FormatUint(cfg.Fixture.Map.ID, 10), "itemId": itemID, "itemName": item.Name,
		"price": strconv.FormatInt(price, 10), "quantity": strconv.FormatInt(quantity, 10),
		"previousPoints": strconv.FormatInt(previousPoints, 10), "currentPoints": strconv.FormatInt(currentPoints, 10),
		"rankReward": strconv.FormatInt(seasonSetup.RankAward, 10), "participationReward": strconv.FormatInt(seasonSetup.ParticipationAward, 10),
		"initialBoundYuanbao": strconv.FormatInt(stored.Wallet.BoundYuanbao, 10),
	})
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Logf("镜像赛季触屏证据目录 %s", evidence)
	browserCtx, stop := context.WithTimeout(ctx, 155*time.Second)
	defer stop()
	command := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_mirror_season_browser.cjs"), manifest)
	command.Dir = repo
	command.Env = append(os.Environ(), "MINIMIR_MIRROR_ATTACKER_ACCOUNT="+player.account,
		"MINIMIR_MIRROR_ATTACKER_PASSWORD="+player.password)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("镜像赛季触屏未完成：%v\n%s", err, output)
	}
	t.Logf("%s", output)

	balance, err = store.ReadMirrorExchangePoints(ctx, origin, roleID)
	must(t, err)
	current, err := store.Role(ctx, roleID)
	must(t, err)
	count, err := store.DB.Collection("mirror_arena_season_rewards").CountDocuments(ctx, bson.M{"role_id": roleID, "claimed": true})
	must(t, err)
	credits, err := store.DB.Collection("mirror_arena_exchange_credits").CountDocuments(ctx, bson.M{"balance_id": fmt.Sprintf("%d:%s", origin, roleID)})
	must(t, err)
	received := 0
	for _, asset := range current.Items {
		if asset.SourceKind == persistence.AssetMirrorArenaExchange && asset.Bound && asset.ItemID == itemID && asset.Quantity == quantity {
			received++
		}
	}
	if balance != previousPoints-price || count != 2 || credits != 1 || received != 1 ||
		current.Wallet.BoundYuanbao != stored.Wallet.BoundYuanbao+seasonSetup.RankAward+seasonSetup.ParticipationAward ||
		current.Wallet.Yuanbao != stored.Wallet.Yuanbao {
		t.Fatalf("重连后没能对账一笔兑换和两种独立奖：余额 %d，奖 %d，结转 %d，绑定物 %d", balance, count, credits, received)
	}
}

type mirrorBrowserSeasonAmounts struct {
	RankAward          int64 // 原隔离榜首的绑定元宝数。
	ParticipationAward int64 // 原隔离参与奖的绑定元宝数。
}

// seedMirrorBrowserSeasons 模拟自然跨十五天后已有的原成绩和结算游标，不预置任何已发放的资产。
func seedMirrorBrowserSeasons(t *testing.T, ctx context.Context, store *persistence.Store, arena *devconfig.MirrorArenaConfig,
	previousScope, currentScope string, previousStart, currentStart time.Time, origin uint64,
	roleID, rivalID, name, rivalName string, previousPoints, currentPoints int64, matches uint32) mirrorBrowserSeasonAmounts {
	t.Helper()
	policy := arena.Settlement
	var rankAward int64
	ranks := bson.A{}
	for _, tier := range policy.Ranks {
		ranks = append(ranks, bson.M{"first": tier.FirstRank, "last": tier.LastRank, "bound_ingot": tier.Reward.BoundYuanbaoUnits})
		if tier.FirstRank == 1 {
			rankAward = tier.Reward.BoundYuanbaoUnits
		}
	}
	if rankAward <= 0 || policy.Participation.BoundYuanbaoUnits <= 0 || matches < policy.ParticipationMinChallenges {
		t.Fatal("隔离奖表未覆盖榜首和参与两种独立奖")
	}
	scores := store.DB.Collection("mirror_arena_scores")
	oldPlayerAt, oldRivalAt := currentStart.Add(-3*time.Hour), currentStart.Add(-time.Hour)
	_, err := scores.InsertMany(ctx, []any{
		bson.M{"_id": previousScope + ":" + roleID, "scope": previousScope, "role_id": roleID, "name": name, "points": previousPoints, "matches": matches, "streak": 3, "last_match_at": oldPlayerAt},
		bson.M{"_id": previousScope + ":" + rivalID, "scope": previousScope, "role_id": rivalID, "name": rivalName, "points": previousPoints, "matches": matches, "streak": 2, "last_match_at": oldRivalAt},
		bson.M{"_id": currentScope + ":" + roleID, "scope": currentScope, "role_id": roleID, "name": name, "points": currentPoints, "matches": 3, "streak": 2, "last_match_at": currentStart.Add(4 * time.Second)},
		bson.M{"_id": currentScope + ":" + rivalID, "scope": currentScope, "role_id": rivalID, "name": rivalName, "points": currentPoints, "matches": 3, "streak": 2, "last_match_at": currentStart.Add(3 * time.Second)},
	})
	must(t, err)
	_, err = store.DB.Collection("mirror_arena_challenges").InsertOne(ctx, bson.M{"_id": domain.ID(),
		"run_server_id": store.Cfg.RunID, "origin": origin, "season_start": previousStart, "season_end": currentStart,
		"attacker_role_id": roleID, "defender_role_id": rivalID, "attacker_name": name, "defender_name": rivalName,
		"defense_version": 1, "outcome": persistence.MirrorWon, "points": 5, "started_at": oldPlayerAt.Add(-time.Minute), "finished_at": oldPlayerAt})
	must(t, err)
	_, err = store.DB.Collection("mirror_arena_seasons").UpdateOne(ctx, bson.M{"_id": previousScope}, bson.M{"$setOnInsert": bson.M{
		"_id": previousScope, "run_server_id": store.Cfg.RunID, "origin": origin, "season_start": previousStart, "season_end": currentStart,
		"template_id": arena.TemplateID, "rule_version": store.Cfg.Fixture.RuleVersion, "config_hash": store.Cfg.Fixture.Hash,
		"phase": 1, "next_check_at": currentStart, "policy": bson.M{
			"participation_min_challenges": policy.ParticipationMinChallenges, "participation_bound_ingot": policy.Participation.BoundYuanbaoUnits, "ranks": ranks},
	}}, options.UpdateOne().SetUpsert(true))
	must(t, err)
	return mirrorBrowserSeasonAmounts{RankAward: rankAward, ParticipationAward: policy.Participation.BoundYuanbaoUnits}
}
