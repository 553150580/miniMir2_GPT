// 本文件用两个真实 Web Mobile 客户端验收共享首领的跨职业表现和窗口变化。
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
	"regexp"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
)

// TestWorldBossTwoClientBrowserMongoWS 仅接受本轮隔离副本集和工作目录内的真实构建。
func TestWorldBossTwoClientBrowserMongoWS(t *testing.T) {
	webRoot := os.Getenv("MINIMIR_WORLD_BOSS_BROWSER_WEB_ROOT")
	coldEntryOnly := os.Getenv("MINIMIR_WORLD_BOSS_COLD_ENTRY_ONLY") == "1"
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || webRoot == "" {
		t.Skip("由独立世界首领双客户端验收启动")
	}
	if os.Getenv("MINIMIR_SERVER_DIRECTORY_TEST_MONGO_URI") == "" {
		t.Fatal("世界首领双端验收需要本轮独立副本集")
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
	if !filepath.IsLocal(relative) || relative == "." {
		t.Fatal("世界首领双端验收只接受独立 Web Mobile 构建")
	}
	_, err = os.Stat(filepath.Join(build, "index.html"))
	must(t, err)
	atlasTexturePaths := worldBossAtlasTexturePaths(t, build)

	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg = directoryMongoTarget(t, cfg)
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	entry := makeDirectoryEntry(cfg.OriginID, cfg.RunID, cfg, "world-browser", "首领验收区", "双客户端一服", 0, 0)
	cfg.OriginRoutes = []devconfig.OriginRouteSeed{entry.origin}
	cfg.RunServers = []devconfig.RunServerSeed{entry.assignment}
	// 使用真实服务端时钟。隔离场次给两端冷加载原帧预留时间，线上十秒基线确认门槛保持不变。
	const shanghaiDay = 24 * time.Hour
	openAt := time.Now().Add(70 * time.Second).Truncate(time.Millisecond)
	local := openAt.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	world := cfg.Fixture.WorldBoss
	if world == nil {
		t.Fatal("首领双端验收需要工作区规则启用世界首领")
	}
	world.DailyOpenOffsets = []time.Duration{local.Sub(midnight) % shanghaiDay}
	world.Window = 220 * time.Second
	world.Map.MonsterSpawns = []devconfig.Point{{X: 6, Y: 4}}
	world.Monster.HP = 10_000
	world.Monster.Accuracy = 100
	world.Monster.AttackMin, world.Monster.AttackMax = 20, 20
	// 固定每十秒十点有效伤害，保证另一端完成冷加载后先观察存活宠物，再由首领正常击杀。
	world.Monster.AttackMS, world.Monster.MoveMS = 10_000, 30_000
	for i := range cfg.Fixture.PlayerGrowth {
		cfg.Fixture.PlayerGrowth[i].HPBase = 500
	}
	// 本双端投影回归从一个已拥有骷髅的测试账号起步。
	cfg.Fixture.PetRuntime.BootstrapMode = "legacy_untrained_pet"
	// 隔离新角色以一级验证正常购取之外的初始书学习、施法和旁观帧，正式等级要求由技能专项覆盖。
	if cfg.Fixture.RuleVersion >= 30 {
		for i := range cfg.Fixture.Skills {
			if cfg.Fixture.Skills[i].ID == 2 {
				cfg.Fixture.Skills[i].LearnLevel = 1
				break
			}
		}
	}
	if !cfg.Fixture.ValidWorldBoss() || cfg.Fixture.Pet.JobID != 2 {
		t.Fatal("首领双端隔离配置无效")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	var nodes sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		nodes.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		defer inspection.Close(cleanup)
		must(t, inspection.DB.Drop(cleanup))
	})
	for _, mode := range []string{"account", "game"} {
		nodes.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("隔离 %s：%v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)

	upstream, err := url.Parse("http://" + cfg.AccountAddress)
	must(t, err)
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()

	evidence, err := os.MkdirTemp(work, "world-boss-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	raw, err := json.Marshal(map[string]any{
		"baseUrl": web.URL, "endpoint": entry.assignment.GameWSURL, "region": entry.origin.RegionName,
		"server": entry.origin.Name, "version": cfg.Fixture.RuleVersion, "map": world.Map.ID,
		"opensAt": openAt.UnixMilli(), "expiresAt": openAt.Add(world.Window).UnixMilli(),
		"petDefinition": cfg.Fixture.Pet.DefinitionID, "coldEntryOnly": coldEntryOnly, "atlasTexturePaths": atlasTexturePaths,
	})
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Logf("世界首领双端触屏证据目录 %s", evidence)
	browserCtx, stop := context.WithTimeout(ctx, 325*time.Second)
	defer stop()
	command := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_world_boss_browser.cjs"), manifest)
	command.Dir = repo
	log, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("世界首领双端触屏未完成: %v\n%s", err, log)
	}
	t.Logf("%s", log)
}

// worldBossAtlasTexturePaths 从当前 Creator 包映射逻辑图集页到浏览器实际请求的纹理路径。
func worldBossAtlasTexturePaths(t *testing.T, build string) []string {
	t.Helper()
	assets := filepath.Join("assets", "resources")
	raw, err := os.ReadFile(filepath.Join(build, assets, "config.json"))
	must(t, err)
	var catalog struct {
		NativeBase string                       `json:"nativeBase"`
		Paths      map[string][]json.RawMessage `json:"paths"`
		UUIDs      []string                     `json:"uuids"`
	}
	must(t, json.Unmarshal(raw, &catalog))
	if catalog.NativeBase != "native" || len(catalog.Paths) == 0 {
		t.Fatal("Creator 资源目录缺少原生纹理路径索引")
	}
	page := regexp.MustCompile(`^development/world-boss-atlas/page-[0-9]+$`)
	fullUUID := regexp.MustCompile(`^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$`)
	paths := []string{}
	for key, fields := range catalog.Paths {
		if len(fields) == 0 {
			continue
		}
		var logicalPath string
		must(t, json.Unmarshal(fields[0], &logicalPath))
		if !page.MatchString(logicalPath) {
			continue
		}
		uuid := key
		if index, parseErr := strconv.Atoi(key); parseErr == nil && index >= 0 && index < len(catalog.UUIDs) {
			uuid = catalog.UUIDs[index]
		}
		if !fullUUID.MatchString(uuid) {
			t.Fatalf("图集页 %s 缺少可追溯 Creator 纹理标识", logicalPath)
		}
		candidates, globErr := filepath.Glob(filepath.Join(build, assets, catalog.NativeBase, uuid[:2], uuid+".*"))
		must(t, globErr)
		if len(candidates) != 1 || filepath.Ext(candidates[0]) != ".png" {
			t.Fatalf("图集页 %s 缺少唯一的 PNG 原生输出", logicalPath)
		}
		relative, relativeErr := filepath.Rel(build, candidates[0])
		must(t, relativeErr)
		paths = append(paths, "/"+filepath.ToSlash(relative))
	}
	sort.Strings(paths)
	return paths
}
