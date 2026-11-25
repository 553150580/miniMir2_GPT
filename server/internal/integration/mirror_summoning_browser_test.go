//go:build creator_mirror_arena

// 本文件为补召触屏验收启动独立区服，并对账原挑战终态和未变动的护符。
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
	"slices"
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

// TestMirrorSummoningCreatorBrowserMongoWS 验证道士从空席施法、新票接回、触屏认输与持久库存隔离。
func TestMirrorSummoningCreatorBrowserMongoWS(t *testing.T) {
	webRoot, fixturePath := os.Getenv("MINIMIR_MIRROR_BROWSER_WEB_ROOT"), os.Getenv("MINIMIR_MIRROR_BROWSER_FIXTURE")
	if webRoot == "" || fixturePath == "" {
		t.Skip("需要独立 Creator 构建和显式开启补召的镜像候选")
	}
	repo, err := filepath.Abs("../../..")
	must(t, err)
	work := filepath.Join(repo, ".work")
	build, fixturePath := mirrorBrowserInput(t, work, webRoot), mirrorBrowserInput(t, work, fixturePath)
	_, err = os.Stat(filepath.Join(build, "index.html"))
	must(t, err)
	cfg := m1FieldConfig(t)
	raw, err := os.ReadFile(fixturePath)
	must(t, err)
	var candidate devconfig.Fixture
	must(t, json.Unmarshal(raw, &candidate))
	digest := sha256.Sum256(raw)
	candidate.Hash = hex.EncodeToString(digest[:])
	must(t, candidate.Validate())
	arena := candidate.MirrorArena
	if arena == nil || arena.Summoning == nil || !arena.Summoning.AllowWithoutEntryPet || candidate.RuleVersion != cfg.Fixture.RuleVersion ||
		candidate.PetRuntime == nil || candidate.PetRuntime.BootstrapMode != "none" {
		t.Fatal("需要启用空槽召唤且无免费入场宠物的现行候选")
	}
	skills, bodyIDs, level := prepareMirrorSummoningBrowserFixture(t, &candidate)
	cfg.FixtureFile, cfg.Fixture = fixturePath, candidate
	cfg, ctx, store := m1FieldServices(t, cfg)
	accountURL := "http://" + cfg.AccountAddress + "/rpc"
	attacker := newMirrorBrowserRole(t, accountURL, cfg.OriginID, 2, "补召攻")
	defender := newMirrorBrowserRole(t, accountURL, cfg.OriginID, 2, "补召守")
	roles := [...]mirrorBrowserRole{attacker, defender}
	var talismans [2]domain.Item
	for index, role := range roles {
		skill := skills[index]
		// 已学习的技能、等级和护符是隔离前态。发布、入场、接回和认输均走正式界面。
		talismans[index] = domain.Item{ID: domain.ID(), ItemID: skill.MaterialItemID,
			Quantity: 2 * skill.MaterialQuantity, Version: 9, Bound: true}
		update, err := store.DB.Collection("roles").UpdateByID(ctx, domain.Hex(role.key.RoleId), bson.M{
			"$set": bson.M{"level": level, "learned_skills": []domain.LearnedSkill{{SkillID: skill.ID, Level: skill.LearnedLevel}},
				"skill_slot_ids": []uint32{skill.ID, 0, 0, 0}},
			"$push": bson.M{"equipped": domain.Equipped{Slot: devconfig.CastingSupplySlot, Item: talismans[index]}},
		})
		must(t, err)
		if update.ModifiedCount != 1 {
			t.Fatal("隔离道士前态未建立", role.name)
		}
	}
	list := &pb.ServerDirectoryResult{}
	rpc(t, accountURL, attacker.token, p.MsgC2SServerDirectory, &pb.ServerDirectoryRequest{}, list, 0)
	var region, server string
	for _, group := range list.Regions {
		for _, item := range group.Servers {
			if item.OriginServerId == cfg.OriginID && item.Available {
				region, server = group.Name, item.Name
			}
		}
	}
	if region == "" || server == "" {
		t.Fatal("隔离帐号节点未开放原赛区入口")
	}
	upstream, err := url.Parse("http://" + cfg.AccountAddress)
	must(t, err)
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(work, "mirror-summoning-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	raw, err = json.Marshal(map[string]any{
		"baseUrl": web.URL, "endpoint": "ws://" + cfg.GameAddress + "/ws", "region": region, "server": server,
		"attacker": attacker.name, "defender": defender.name, "battleMap": strconv.FormatUint(arena.Map.ID, 10),
		"normalMap": strconv.FormatUint(cfg.Fixture.Map.ID, 10), "ruleVersion": strconv.FormatUint(cfg.Fixture.RuleVersion, 10),
		"skeletonDefinition": bodyIDs[0], "shinsuDefinition": bodyIDs[1],
	})
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Logf("镜像补召触屏证据目录 %s", evidence)
	browserCtx, stop := context.WithTimeout(ctx, 230*time.Second)
	defer stop()
	command := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_mirror_summoning_browser.cjs"), manifest)
	command.Dir = repo
	command.Env = append(os.Environ(), "MINIMIR_MIRROR_ATTACKER_ACCOUNT="+attacker.account,
		"MINIMIR_MIRROR_ATTACKER_PASSWORD="+attacker.password, "MINIMIR_MIRROR_DEFENDER_ACCOUNT="+defender.account,
		"MINIMIR_MIRROR_DEFENDER_PASSWORD="+defender.password)
	log, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("镜像补召触屏未完成：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	verifyMirrorSummoningBrowserSettlement(t, ctx, store, evidence, roles, talismans)
}

// prepareMirrorSummoningBrowserFixture 在隔离启动时提供两侧相同的换票时间窗口。
func prepareMirrorSummoningBrowserFixture(t *testing.T, f *devconfig.Fixture) ([2]devconfig.Skill, [2]uint32, uint32) {
	t.Helper()
	var skills [2]devconfig.Skill
	var bodyIDs [2]uint32
	var level uint32
	for i, id := range [...]uint32{devconfig.SummonSkeletonSkillID, devconfig.SummonShinsuSkillID} {
		skill, exists := f.Skill(id)
		if !exists || skill.JobID != 2 || !skill.AttackSlotSummon() || !f.IsCastingSupply(skill.MaterialItemID) || skill.MaterialQuantity <= 0 {
			t.Fatal("候选缺少道士原召唤技能或护符配置", id)
		}
		pet, exists := f.PetRuntime.DefinitionByID(skill.Summon.DefinitionID)
		if !exists || pet.SourceSkillID != id || pet.Base.DefinitionID == 0 {
			t.Fatal("候选缺少与原召唤技能对应的宠物身体", id)
		}
		skills[i], bodyIDs[i], level = skill, pet.Base.DefinitionID, max(level, skill.LearnLevel)
	}
	if bodyIDs[0] == bodyIDs[1] {
		t.Fatal("骷髅和神兽需分别保留原身体")
	}
	// 普通出生图禁宠，隔离镜像仍按其独立地图开启宠物。
	noPets := true
	f.Map.NoPets = &noPets
	for i := range f.PlayerGrowth {
		if f.PlayerGrowth[i].JobID == 2 {
			f.PlayerGrowth[i].HPBase, f.PlayerGrowth[i].MPBase = 20_000, 400
		}
	}
	for i := range f.PetRuntime.Definitions {
		pet := &f.PetRuntime.Definitions[i]
		if slices.Contains(bodyIDs[:], pet.Base.DefinitionID) {
			pet.Base.HP = 20_000
		}
	}
	return skills, bodyIDs, level
}

// verifyMirrorSummoningBrowserSettlement 将浏览器原挑战号与 MongoDB 单一终态和两个永久护符逐项比对。
func verifyMirrorSummoningBrowserSettlement(t *testing.T, ctx context.Context, store *persistence.Store, evidence string,
	roles [2]mirrorBrowserRole, talismans [2]domain.Item) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(evidence, "mirror-summoning-browser-report.json"))
	must(t, err)
	var report struct {
		Passed   bool // 触屏、权威帧及出场的观察均已通过。
		Terminal struct {
			Challenge string                    // 从原权威终态取出的挑战号。
			Outcome   persistence.MirrorOutcome // 从原权威终态取出的胜负。
			Points    string                    // 保持线协议的整数精度。
		}
	}
	must(t, json.Unmarshal(raw, &report))
	if !report.Passed || len(report.Terminal.Challenge) != 32 || report.Terminal.Outcome != persistence.MirrorLost || report.Terminal.Points != "1" {
		t.Fatal("浏览器没有确认原挑战的认输和积分", report.Terminal)
	}
	attackerID, defenderID := domain.Hex(roles[0].key.RoleId), domain.Hex(roles[1].key.RoleId)
	var result persistence.MirrorChallenge
	challenges := store.DB.Collection("mirror_arena_challenges")
	filter := bson.M{"attacker_role_id": attackerID, "defender_role_id": defenderID}
	count, err := challenges.CountDocuments(ctx, filter)
	must(t, err)
	filter["_id"] = report.Terminal.Challenge
	must(t, challenges.FindOne(ctx, filter).Decode(&result))
	if count != 1 || result.Outcome != report.Terminal.Outcome || strconv.FormatInt(result.Points, 10) != report.Terminal.Points ||
		result.SeasonPoints != 1 || result.DefenseVersion != 1 || result.FinishedAt.IsZero() || result.RuleVersion != store.Cfg.Fixture.RuleVersion {
		t.Fatal("永久账本没有确认唯一的原挑战认输终态", result, count)
	}
	for index, original := range roles {
		role, err := store.Role(ctx, domain.Hex(original.key.RoleId))
		must(t, err)
		if !slices.Contains(role.Equipped, domain.Equipped{Slot: devconfig.CastingSupplySlot, Item: talismans[index]}) {
			t.Fatal("镜像召唤修改了原角色的护符实例、数量或版本", role.Name)
		}
		count, err := store.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": role.ID, "kind": "cast_material"})
		must(t, err)
		if count != 0 {
			t.Fatal("临时补召创建了永久施法扣耗回执", role.Name, count)
		}
	}
	t.Log("原场次结算唯一，攻守道士的专槽护符实例、数量和版本均保持，未产生永久扣耗回执")
}
