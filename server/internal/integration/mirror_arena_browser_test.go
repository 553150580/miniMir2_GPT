//go:build creator_mirror_arena

// 本文件用候选客户端、真实 WebSocket 与独立 MongoDB 验收镜像竞技全界面。
package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

type mirrorBrowserRole struct {
	account  string
	password string
	name     string
	key      *pb.RoleKey
	token    string
}

// TestMirrorArenaCreatorBrowserMongoWS 只通过触屏发布、进场、重连、认输和返回，最后核对永久回执。
func TestMirrorArenaCreatorBrowserMongoWS(t *testing.T) {
	webRoot, fixturePath := os.Getenv("MINIMIR_MIRROR_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_MIRROR_BROWSER_FIXTURE")
	if webRoot == "" || fixturePath == "" {
		t.Skip("需要独立 Creator 构建和启用镜像的同源夹具")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build := mirrorBrowserInput(t, work, webRoot)
	_, err = os.Stat(filepath.Join(build, "index.html"))
	must(t, err)
	cfg := m1FieldConfig(t)
	fixturePath = mirrorBrowserInput(t, work, fixturePath)
	raw, err := os.ReadFile(fixturePath)
	must(t, err)
	var candidate devconfig.Fixture
	must(t, json.Unmarshal(raw, &candidate))
	digest := sha256.Sum256(raw)
	candidate.Hash = hex.EncodeToString(digest[:])
	must(t, candidate.Validate())
	if candidate.MirrorArena == nil || candidate.RuleVersion != cfg.Fixture.RuleVersion {
		t.Fatal("必须使用当前规则的真实镜像候选")
	}
	cfg.FixtureFile, cfg.Fixture = fixturePath, candidate
	fire, ok := cfg.Fixture.Skill(1)
	if !ok || fire.JobID != 1 || fire.LearnLevel == 0 {
		t.Fatal("候选没有原法师火球术")
	}
	// 只延长隔离角色的生存供正常重连，普通野外缩短首只击杀时间。
	for i := range cfg.Fixture.PlayerGrowth {
		if cfg.Fixture.PlayerGrowth[i].JobID <= 1 {
			cfg.Fixture.PlayerGrowth[i].HPBase = 600
		}
	}
	spawn := cfg.Fixture.Map.PlayerSpawn
	cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: spawn.X + 1, Y: spawn.Y}
	for i := range cfg.Fixture.Monsters {
		if cfg.Fixture.Monsters[i].ID == cfg.Fixture.Map.MonsterSpawnIDs[0] {
			cfg.Fixture.Monsters[i].HP, cfg.Fixture.Monsters[i].AttackMin, cfg.Fixture.Monsters[i].AttackMax = 1, 0, 0
		}
	}
	cfg, ctx, store := m1FieldServices(t, cfg)
	accountURL := "http://" + cfg.AccountAddress + "/rpc"
	attacker := newMirrorBrowserRole(t, accountURL, cfg.OriginID, 0, "镜攻")
	defender := newMirrorBrowserRole(t, accountURL, cfg.OriginID, 1, "镜守")
	attackerID, defenderID := domain.Hex(attacker.key.RoleId), domain.Hex(defender.key.RoleId)
	_, err = store.DB.Collection("roles").UpdateByID(ctx, attackerID, bson.M{"$set": bson.M{"level": fire.LearnLevel}})
	must(t, err)
	// 已学火球、等级和技能槽是隔离前态；防守发布仍走正常界面。
	_, err = store.DB.Collection("roles").UpdateByID(ctx, defenderID, bson.M{"$set": bson.M{"level": fire.LearnLevel,
		"learned_skills": []domain.LearnedSkill{{SkillID: fire.ID, Level: fire.LearnedLevel}},
		"skill_slot_ids": []uint32{fire.ID, 0, 0, 0},
	}})
	must(t, err)
	directory := &pb.ServerDirectoryResult{}
	rpc(t, accountURL, attacker.token, p.MsgC2SServerDirectory, &pb.ServerDirectoryRequest{}, directory, 0)
	var region, server string
	for _, group := range directory.Regions {
		for _, item := range group.Servers {
			if item.OriginServerId == cfg.OriginID && item.Available {
				region, server = group.Name, item.Name
			}
		}
	}
	if region == "" || server == "" {
		t.Fatal("找不到独立账号节点已开放的区服")
	}

	upstream, err := url.Parse("http://" + cfg.AccountAddress)
	must(t, err)
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(work, "mirror-arena-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	raw, err = json.Marshal(map[string]any{"baseUrl": web.URL, "endpoint": "ws://" + cfg.GameAddress + "/ws", "region": region, "server": server,
		"map": strconv.FormatUint(cfg.Fixture.MirrorArena.Map.ID, 10), "normalMap": strconv.FormatUint(cfg.Fixture.Map.ID, 10),
		"ruleVersion": strconv.FormatUint(cfg.Fixture.RuleVersion, 10), "defender": defender.name, "attacker": attacker.name, "fireball": fire.ID})
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Logf("镜像触屏验收证据目录 %s", evidence)
	browserCtx, stop := context.WithTimeout(ctx, 230*time.Second)
	defer stop()
	command := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_mirror_arena_browser.cjs"), manifest)
	command.Dir = repo
	command.Env = append(os.Environ(), "MINIMIR_MIRROR_ATTACKER_ACCOUNT="+attacker.account,
		"MINIMIR_MIRROR_ATTACKER_PASSWORD="+attacker.password, "MINIMIR_MIRROR_DEFENDER_ACCOUNT="+defender.account,
		"MINIMIR_MIRROR_DEFENDER_PASSWORD="+defender.password)
	log, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("镜像触屏未完成：%v\n%s", err, log)
	}
	t.Logf("%s", log)

	var result persistence.MirrorChallenge
	challenges := store.DB.Collection("mirror_arena_challenges")
	filter := bson.M{"attacker_role_id": attackerID, "defender_role_id": defenderID}
	must(t, challenges.FindOne(ctx, filter).Decode(&result))
	count, err := challenges.CountDocuments(ctx, filter)
	must(t, err)
	if count != 1 || result.Outcome != persistence.MirrorLost || result.Points != 1 || result.DefenseVersion != 1 ||
		result.SeasonPoints != 1 || result.RuleVersion != cfg.Fixture.RuleVersion {
		t.Fatal("认输未形成唯一的原赛季、原防守版本回执", result, count)
	}
	role, err := store.Role(ctx, attackerID)
	must(t, err)
	if role.SourceSeq == 0 {
		t.Fatal("普通自动战斗没有产生已确认的永久收益")
	}
}

func mirrorBrowserInput(t *testing.T, work, filename string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(filename)
	must(t, err)
	resolved, err = filepath.Abs(resolved)
	must(t, err)
	relative, err := filepath.Rel(work, resolved)
	must(t, err)
	if relative == "." || !filepath.IsLocal(relative) {
		t.Fatal("验收输入必须位于独立工作目录")
	}
	return resolved
}

func newMirrorBrowserRole(t *testing.T, endpoint string, origin uint64, job uint32, prefix string) mirrorBrowserRole {
	t.Helper()
	value := mirrorBrowserRole{account: "mirror_" + domain.ID()[:10], password: domain.ID()[:16], name: prefix + domain.ID()[:5]}
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: value.account, Password: value.password}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: origin, Slot: 1, Job: job,
		Name: value.name, OpId: domain.Bytes(domain.ID())}, role, 0)
	value.key, value.token = role.Key, login.AccountToken
	return value
}
