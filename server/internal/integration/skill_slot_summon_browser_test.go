//go:build creator_summon_slots

// 本文件从受控角色前态启动真实 Creator 触屏、WebSocket 和 MongoDB 技能槽验收。
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
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestSkillAttackSlotsSummonBrowserMongoWS 从触屏滚动、学习、配置到重连核对原书与四槽。
func TestSkillAttackSlotsSummonBrowserMongoWS(t *testing.T) {
	webRoot := os.Getenv("MINIMIR_SUMMON_SLOTS_BROWSER_WEB_ROOT")
	if webRoot == "" {
		t.Skip("必须提供工作区内实际 Creator Web Mobile 候选")
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
		t.Fatal("候选必须是独立 Creator 构建目录")
	}
	_, err = os.Stat(filepath.Join(build, "index.html"))
	must(t, err)
	cfg, ctx, store := m1FieldServices(t, m1FieldConfig(t))
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	account, password, name := "slots_"+domain.ID()[:10], domain.ID()[:16], "触屏道士"+domain.ID()[:5]
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: account, Password: password}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: 1, Job: 2, Name: name,
		OpId: domain.Bytes(domain.ID())}, role, 0)
	list := &pb.ServerDirectoryResult{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SServerDirectory, &pb.ServerDirectoryRequest{}, list, 0)
	var region, server string
	for _, group := range list.Regions {
		for _, item := range group.Servers {
			if item.OriginServerId == cfg.OriginID {
				region, server = group.Name, item.Name
			}
		}
	}
	if region == "" || server == "" {
		t.Fatal("已建道士没有通常可选的正式目录入口")
	}
	roleID := domain.Hex(role.Key.RoleId)
	slotFirst := os.Getenv("MINIMIR_SLOTS_BROWSER_SLOT_FIRST") == "1"
	// 只在本独立库准备适龄角色和两本原书，浏览器仍需真正学习后才能配置。
	var level uint32
	books := make([]domain.Item, 0, 2)
	for _, id := range []uint32{devconfig.SummonSkeletonSkillID, devconfig.SummonShinsuSkillID} {
		skill, known := cfg.Fixture.Skill(id)
		if !known {
			t.Fatal("现行配置缺少召唤技能", id)
		}
		level = max(level, skill.LearnLevel)
		books = append(books, domain.Item{ID: domain.ID(), ItemID: skill.BookItemID, Quantity: 1, Bound: true, Version: 1})
	}
	learned := []domain.LearnedSkill{{SkillID: 2}}
	if slotFirst {
		// 使用已学骷髅、治愈术在第四槽的老角色前态，避免只有刚学书才能通过验收。
		level = 32
		learned = append(learned, domain.LearnedSkill{SkillID: devconfig.SummonSkeletonSkillID})
		books = books[:0]
	}
	_, err = store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{
		"$set":  bson.M{"level": level, "learned_skills": learned, "skill_slot_ids": []uint32{0, 0, 0, 2}},
		"$push": bson.M{"items": bson.M{"$each": books}},
	})
	must(t, err)
	upstream, err := url.Parse("http://" + cfg.AccountAddress)
	must(t, err)
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", httputil.NewSingleHostReverseProxy(upstream)))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()
	evidence, err := os.MkdirTemp(work, "summon-slots-browser-")
	must(t, err)
	manifest := filepath.Join(evidence, "manifest.json")
	// 凭据通过临时环境变量传递，文本证据只保留临时路由和角色显示名。
	raw, err := json.Marshal(map[string]any{"baseUrl": web.URL, "endpoint": "ws://" + cfg.GameAddress + "/ws", "region": region,
		"server": server, "roleName": name, "ruleVersion": cfg.Fixture.RuleVersion})
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Logf("召唤槽触屏验收证据目录 %s", evidence)
	browserCtx, stop := context.WithTimeout(ctx, 150*time.Second)
	defer stop()
	command := exec.CommandContext(browserCtx, "node", filepath.Join(repo, "tools/client/verify_summon_slots_browser.cjs"), manifest)
	command.Dir = repo
	command.Env = append(os.Environ(), "MINIMIR_SLOTS_BROWSER_ACCOUNT="+account, "MINIMIR_SLOTS_BROWSER_PASSWORD="+password)
	log, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("召唤槽触屏未完成：%v\n%s", err, log)
	}
	t.Logf("%s", log)
	stored, err := store.Role(ctx, roleID)
	must(t, err)
	for _, book := range books {
		if slices.ContainsFunc(stored.Items, func(item domain.Item) bool { return item.ID == book.ID }) {
			t.Fatal("浏览器学习未消耗对应原书", book.ItemID)
		}
	}
	for _, id := range []uint32{devconfig.SummonSkeletonSkillID, devconfig.SummonShinsuSkillID} {
		if _, known := stored.Skill(id); known != (!slotFirst || id == devconfig.SummonSkeletonSkillID) {
			t.Fatal("浏览器学习未持久化技能", id)
		}
	}
	wantSlots := []uint32{devconfig.SummonSkeletonSkillID, devconfig.SummonShinsuSkillID, 0, 2}
	wantCommands := int64(4)
	if slotFirst {
		wantSlots = []uint32{0, 0, devconfig.SummonSkeletonSkillID, 2}
		wantCommands = 2
	}
	if !slices.Equal(stored.SkillSlotIDs, wantSlots) {
		t.Fatal("触屏两次配置未持久化四槽", stored.SkillSlotIDs)
	}
	count, err := store.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": roleID, "kind": persistence.AssetSetSkillSlots, "code": 0})
	must(t, err)
	if count != 2 || stored.EconomicSeq != wantCommands {
		t.Fatalf("学书、槽位点击或健康重连产生非预期永久操作次数：技能槽%d、角色流水%d", count, stored.EconomicSeq)
	}
}
