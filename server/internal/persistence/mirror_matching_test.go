// 本文件在隔离 MongoDB 中复核镜像候选排序、作用域、上海日界和不可变防守版本。
package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

func mirrorMatchingMongo(t *testing.T) (context.Context, *Store, time.Time) {
	t.Helper()
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("需要本任务专属回环 MongoDB 副本集")
	}
	if !regexp.MustCompile(`^mongodb://127\.0\.0\.1:[1-9][0-9]{3,4}/\?replicaSet=minimirLoad_[a-f0-9]{32}&directConnection=true$`).MatchString(uri) {
		t.Fatal("仅允许隔离副本集控制器生成的端点")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	mirrorMatchingMust(t, err)
	now := time.Now().In(shanghaiOperatingZone)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, shanghaiOperatingZone)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_mirror_match_"+domain.ID()[:14]
	cfg.Fixture.MirrorArena = &devconfig.MirrorArenaConfig{TemplateID: 700, DisplayName: "隔离匹配", SeasonAnchor: midnight.AddDate(0, 0, -2),
		SeasonDays: 15, DailyLimit: 5, OpponentLimit: 3, AttackHistoryLimit: 20, DefenseHistoryLimit: 20,
		Duration: time.Minute, WinBasePoints: 5, LossPoints: 1, MatchPolicy: devconfig.MirrorArenaMatchCrossJobLevelWindow, MaxLevelGap: 3}
	store, err := Open(ctx, cfg)
	mirrorMatchingMust(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		defer store.Close(cleanup)
		if err := store.DB.Drop(cleanup); err != nil {
			t.Error(err)
		}
	})
	return ctx, store, midnight
}

func mirrorMatchingMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// 种下可复查的隔离候选。选择器使用有效十六进制，明确控制原日旋转的前后半段。
func mirrorMatchingRow(store *Store, scope, name string, job, level uint32, selector int) mirrorPoolEntry {
	return mirrorPoolEntry{ID: scope + ":" + name, Scope: scope, RoleID: name, AccountID: "账号" + name, Name: name, Job: job, Level: level,
		DefenseID: Hash([]byte(name))[:32], Version: 1, RuleVersion: store.Cfg.Fixture.RuleVersion, ConfigHash: store.Cfg.Fixture.Hash,
		Selector: fmt.Sprintf("%064x", selector)}
}

type mirrorMatchingObservedRow struct {
	Name string `json:"name"`
	Job  uint32 `json:"job"`
	LV   uint32 `json:"level"`
	Gap  uint32 `json:"gap"`
}

func mirrorMatchingMeasure(t *testing.T, name string, role domain.Role, cfg devconfig.MirrorArenaConfig, eligible, offered []mirrorPoolEntry) {
	t.Helper()
	view := func(rows []mirrorPoolEntry) ([]mirrorMatchingObservedRow, []uint32, uint32, uint32) {
		seen := [3]bool{}
		out := make([]mirrorMatchingObservedRow, 0, len(rows))
		var high, total uint32
		for _, row := range rows {
			gap := max(row.Level, role.Level) - min(row.Level, role.Level)
			out = append(out, mirrorMatchingObservedRow{row.Name, row.Job, row.Level, gap})
			seen[row.Job] = true
			high, total = max(high, gap), total+gap
		}
		jobs := make([]uint32, 0, len(seen))
		for job, found := range seen {
			if found {
				jobs = append(jobs, uint32(job))
			}
		}
		return out, jobs, high, total
	}
	population, eligibleJobs, _, _ := view(eligible)
	picked, offeredJobs, gapMax, gapSum := view(offered)
	closest := slices.Clone(population)
	slices.SortFunc(closest, func(a, b mirrorMatchingObservedRow) int { return int(a.Gap) - int(b.Gap) })
	count := min(len(closest), int(cfg.OpponentLimit))
	var minimumSum uint32
	for _, candidate := range closest[:count] {
		minimumSum += candidate.Gap
	}
	data := map[string]any{"case": name, "viewer_job": role.Job, "viewer_level": role.Level, "policy": cfg.MatchPolicy,
		"window": cfg.MaxLevelGap, "eligible": population, "offered": picked, "eligible_jobs": eligibleJobs, "offered_jobs": offeredJobs,
		"offered_max_gap": gapMax, "offered_sum_gap": gapSum, "closest_sum_gap": minimumSum}
	raw, err := json.Marshal(data)
	mirrorMatchingMust(t, err)
	t.Logf("镜像匹配观察 %s", raw)
}

func TestMirrorMatchingPrefersClosestMongo(t *testing.T) {
	ctx, store, midnight := mirrorMatchingMongo(t)
	cfg := *store.Cfg.Fixture.MirrorArena
	role := domain.Role{ID: "我方角色", AccountID: "我方账号", Job: 0, Level: 30, Origin: store.Cfg.OriginID}
	for i, scenario := range []struct {
		name         string
		levels, jobs []uint32
		want         []uint32
	}{
		{"同级候选在旋转断点前", []uint32{30, 30, 30, 31, 33, 27, 29}, []uint32{0, 1, 2, 0, 0, 0, 1}, []uint32{0, 0, 0}},
		{"低等级候选的接近程度", []uint32{27, 28, 29, 29, 29}, []uint32{0, 1, 0, 1, 2}, []uint32{1, 1, 1}},
		{"高低两侧交替靠近", []uint32{31, 32, 33, 29, 28, 27}, []uint32{0, 0, 0, 1, 1, 2}, []uint32{1, 1, 2}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			scope := mirrorScope(store.Cfg.RunID, role.Origin, midnight.AddDate(0, 0, -i))
			eligible := make([]mirrorPoolEntry, 0, len(scenario.levels))
			for j, level := range scenario.levels {
				name := fmt.Sprintf("候选%d·职%d·级%d", j, scenario.jobs[j], level)
				eligible = append(eligible, mirrorMatchingRow(store, scope, name, scenario.jobs[j], level, j+1))
			}
			_, err := store.DB.Collection(mirrorPoolCollection).InsertMany(ctx, eligible)
			mirrorMatchingMust(t, err)
			got, err := store.selectMirrorDefenses(ctx, role, scope, strings.Repeat("f", 64), &cfg)
			mirrorMatchingMust(t, err)
			mirrorMatchingMeasure(t, scenario.name, role, cfg, eligible, got)
			gaps := make([]uint32, 0, len(got))
			for _, row := range got {
				gaps = append(gaps, max(row.Level, role.Level)-min(row.Level, role.Level))
			}
			if !slices.Equal(gaps, scenario.want) {
				t.Errorf("候选应按绝对等级差从近到远：得到 %v，预期 %v", gaps, scenario.want)
			}
		})
	}
}

func TestMirrorMatchingCatalogMongo(t *testing.T) {
	ctx, store, midnight := mirrorMatchingMongo(t)
	base := *store.Cfg.Fixture.MirrorArena
	role := domain.Role{ID: "本角色", AccountID: "本账号", Level: 30, Origin: store.Cfg.OriginID}
	pivot := strings.Repeat("8", 64)
	for job := uint32(0); job <= 2; job++ {
		role.Job = job
		for gap := uint32(0); gap <= 3; gap++ {
			days := int(job*4+gap) + 1
			scope := mirrorScope(store.Cfg.RunID, role.Origin, midnight.AddDate(0, 0, -days))
			same := mirrorMatchingRow(store, scope, "同职业窗上沿", role.Job, role.Level+gap, 1)
			cross := mirrorMatchingRow(store, scope, "跨职业窗下沿", (role.Job+1)%3, role.Level-gap, 2)
			other := mirrorMatchingRow(store, scope, "跨职业相邻外沿", (role.Job+2)%3, role.Level+gap+1, 3)
			sibling := mirrorMatchingRow(store, scope, "本人账号另一角色", role.Job, role.Level, 4)
			sibling.AccountID = role.AccountID
			staleRule := mirrorMatchingRow(store, scope, "规则不符", role.Job, role.Level, 5)
			staleRule.RuleVersion++
			staleHash := mirrorMatchingRow(store, scope, "摘要不符", role.Job, role.Level, 6)
			staleHash.ConfigHash = Hash([]byte("另一份同版本装配"))
			rows := []mirrorPoolEntry{same, cross, other, sibling, staleRule, staleHash,
				mirrorMatchingRow(store, scope, "同职业相邻外沿", role.Job, role.Level-gap-1, 7),
				mirrorMatchingRow(store, mirrorScope(store.Cfg.RunID+1, role.Origin, midnight.AddDate(0, 0, -days)), "另一个运行服", role.Job, role.Level, 8),
				mirrorMatchingRow(store, mirrorScope(store.Cfg.RunID, role.Origin+1, midnight.AddDate(0, 0, -days)), "另一个永久入口", role.Job, role.Level, 9),
				mirrorMatchingRow(store, mirrorScope(store.Cfg.RunID, role.Origin, midnight.AddDate(0, 0, -days-30)), "另一个赛季", role.Job, role.Level, 10)}
			_, err := store.DB.Collection(mirrorPoolCollection).InsertMany(ctx, rows)
			mirrorMatchingMust(t, err)
			for _, policy := range []string{devconfig.MirrorArenaMatchSameJobLevelWindow, devconfig.MirrorArenaMatchCrossJobLevelWindow} {
				name := fmt.Sprintf("%s/职业%d/窗%d", policy, job, gap)
				t.Run(name, func(t *testing.T) {
					cfg := base
					cfg.MatchPolicy, cfg.MaxLevelGap = policy, gap
					eligible := []mirrorPoolEntry{same}
					if policy == devconfig.MirrorArenaMatchCrossJobLevelWindow {
						eligible = append(eligible, cross)
					}
					got, err := store.selectMirrorDefenses(ctx, role, scope, pivot, &cfg)
					mirrorMatchingMust(t, err)
					mirrorMatchingMeasure(t, name, role, cfg, eligible, got)
					wantNames, actualNames := make([]string, len(eligible)), make([]string, len(got))
					for i, row := range eligible {
						wantNames[i] = row.Name
					}
					for i, row := range got {
						actualNames[i] = row.Name
					}
					slices.Sort(actualNames)
					slices.Sort(wantNames)
					if !slices.Equal(actualNames, wantNames) {
						t.Fatalf("候选越界、跨作用域或补了不存在的第三项：得到 %v，预期 %v", actualNames, wantNames)
					}
				})
			}
		}
	}
}

func TestMirrorMatchingRotatesAcrossEqualGapMongo(t *testing.T) {
	ctx, store, midnight := mirrorMatchingMongo(t)
	cfg := *store.Cfg.Fixture.MirrorArena
	role := domain.Role{ID: "平局角色", AccountID: "平局账号", Job: 0, Level: 30, Origin: store.Cfg.OriginID}
	scope := mirrorScope(store.Cfg.RunID, role.Origin, midnight)
	// 两侧都有足够候选触发各自的有界查询截断，均应按同一断点取完末级。
	var rows []mirrorPoolEntry
	for index := 0; index < 8; index++ {
		level := uint32(31 - 2*(index%2))
		rows = append(rows, mirrorMatchingRow(store, scope, fmt.Sprintf("同差候选%d", index), uint32(index%3), level, (index+1)*10))
	}
	_, err := store.DB.Collection(mirrorPoolCollection).InsertMany(ctx, rows)
	mirrorMatchingMust(t, err)
	for _, sample := range []struct {
		pivot int
		want  []string
	}{{45, []string{rows[4].Name, rows[5].Name, rows[6].Name}},
		{75, []string{rows[7].Name, rows[0].Name, rows[1].Name}}} {
		pivot := fmt.Sprintf("%064x", sample.pivot)
		got, err := store.selectMirrorDefenses(ctx, role, scope, pivot, &cfg)
		mirrorMatchingMust(t, err)
		mirrorMatchingMeasure(t, fmt.Sprintf("同差轮换断点%d", sample.pivot), role, cfg, rows, got)
		names := make([]string, 0, len(got))
		for _, row := range got {
			names = append(names, row.Name)
		}
		if !slices.Equal(names, sample.want) {
			t.Errorf("同差高低侧需共用日旋转：断点 %d 得到 %v，预期 %v", sample.pivot, names, sample.want)
		}
	}
}

func TestMirrorMatchingDailyMongo(t *testing.T) {
	ctx, account, midnight := mirrorMatchingMongo(t)
	cfg := account.Cfg
	mirrorMatchingMust(t, account.Singleton(ctx, "account"))
	owner, err := Open(ctx, cfg)
	mirrorMatchingMust(t, err)
	t.Cleanup(func() { owner.Close(context.Background()) })
	lease, err := account.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
	mirrorMatchingMust(t, err)
	owner.Epoch = lease.Epoch
	participant := func(name string, job uint32) (domain.Role, Auth) {
		t.Helper()
		login, err := account.Login(ctx, "mmatch_"+domain.ID()[:8], "test_123456", true)
		mirrorMatchingMust(t, err)
		user, err := account.Authenticate(ctx, login.AccountToken)
		mirrorMatchingMust(t, err)
		info, err := account.CreateRole(ctx, user, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: 1, Job: job,
			Name: name, OpId: domain.Bytes(domain.ID())})
		mirrorMatchingMust(t, err)
		ticket, err := account.IssueTicket(ctx, user, info.Key)
		mirrorMatchingMust(t, err)
		role, auth, err := owner.Enter(ctx, ticket.Ticket)
		mirrorMatchingMust(t, err)
		return role, auth
	}
	snapshot := func(role domain.Role) domain.MirrorCombatSnapshot {
		fighter, ok := cfg.Fixture.PlayerAtLevel(role.Job, role.Level)
		if !ok {
			t.Fatal("原角色等级缺少属性", role.Level)
		}
		return domain.MirrorCombatSnapshot{Job: role.Job, Gender: role.Gender, Level: role.Level, Name: role.Name,
			Fighter: fighter, Skills: slices.Clone(role.Skills), SkillSlotIDs: slices.Clone(role.SkillSlotIDs)}
	}
	publish := func(role domain.Role, auth Auth) MirrorDefense {
		defense, err := owner.PublishMirrorDefense(ctx, auth, MirrorPublish{ExpectedRevision: role.Revision, Snapshot: snapshot(role)})
		mirrorMatchingMust(t, err)
		return defense
	}
	attacker, attackAuth := participant("日界匹配攻方", 0)
	defender, defenseAuth := participant("日界匹配守方", 2)
	first := publish(defender, defenseAuth)
	info, err := owner.QueryMirrorArena(ctx, attackAuth)
	mirrorMatchingMust(t, err)
	if len(info.Offers) != 1 || info.Offers[0].DefenderLevel != defender.Level || info.Offers[0].DefenseVersion != first.Version {
		t.Fatalf("不足三人时应只发原一位候选：%+v", info.Offers)
	}
	// 只在本隔离库推进已发布角色的等级和修订，正式发布入口需重新写原快照。
	_, err = owner.DB.Collection("roles").UpdateByID(ctx, defender.ID, bson.M{"$inc": bson.M{"level": 1, "revision": 1}})
	mirrorMatchingMust(t, err)
	upgraded, err := owner.Role(ctx, defender.ID)
	mirrorMatchingMust(t, err)
	later := publish(upgraded, defenseAuth)
	if later.ID == first.ID || later.Version != first.Version+1 || later.Snapshot.Level != first.Snapshot.Level+1 {
		t.Fatalf("角色升级未发布独立版本：旧=%d，新=%d", first.Version, later.Version)
	}
	other, otherAuth := participant("日界后来候选", 1)
	publish(other, otherAuth)
	fixed, err := owner.QueryMirrorArena(ctx, attackAuth)
	mirrorMatchingMust(t, err)
	if !slices.Equal(fixed.Offers, info.Offers) {
		t.Fatal("同一运营日的原候选随角色升级或新增发布变化", fixed.Offers)
	}
	grant, err := owner.BeginMirrorChallenge(ctx, attackAuth, MirrorBegin{ChallengeID: domain.ID(), OfferID: info.Offers[0].ID,
		Execution: 1, ExpectedRevision: attacker.Revision, AttackerSnapshot: snapshot(attacker)})
	mirrorMatchingMust(t, err)
	if grant.DefenseVersion != first.Version || grant.DefenseSnapshot.Level != first.Snapshot.Level || grant.DefenseSnapshot.Fighter.HP != first.Snapshot.Fighter.HP {
		t.Fatal("实际挑战采用了升级后的防守快照", grant.DefenseVersion, grant.DefenseSnapshot.Level)
	}
	_, err = owner.FinishMirrorChallenge(ctx, attackAuth, MirrorFinish{ChallengeID: grant.ChallengeID, ReservationID: grant.ReservationID,
		Execution: grant.Execution, Outcome: MirrorLost, FinishedAt: bossTime(time.Now())})
	mirrorMatchingMust(t, err)

	season := cfg.Fixture.MirrorArena.SeasonAnchor
	scope := mirrorScope(cfg.RunID, attacker.Origin, season)
	boundary := midnight.AddDate(0, 0, 1)
	prior, err := owner.ensureMirrorDay(ctx, attacker, scope, boundary.Add(-time.Millisecond).UTC(), cfg.Fixture.MirrorArena)
	mirrorMatchingMust(t, err)
	next, err := owner.ensureMirrorDay(ctx, attacker, scope, boundary.UTC(), cfg.Fixture.MirrorArena)
	mirrorMatchingMust(t, err)
	replay, err := owner.ensureMirrorDay(ctx, attacker, scope, boundary.Add(time.Hour).UTC(), cfg.Fixture.MirrorArena)
	mirrorMatchingMust(t, err)
	if prior.Entries != 1 || len(prior.Offers) != 1 || prior.Offers[0].ID != info.Offers[0].ID ||
		next.Entries != 0 || len(next.Offers) != 2 || next.ID == prior.ID || !slices.Equal(next.Offers, replay.Offers) {
		t.Fatalf("上海零点前后候选或已用次数错误：前=%+v 后=%+v", prior, next)
	}
	if !slices.ContainsFunc(next.Offers, func(offer mirrorOfferRecord) bool {
		return offer.DefenderName == defender.Name && offer.DefenseID == later.ID && offer.DefenderLevel == upgraded.Level && offer.ID != prior.Offers[0].ID
	}) || !slices.ContainsFunc(next.Offers, func(offer mirrorOfferRecord) bool { return offer.DefenderName == other.Name }) {
		t.Fatal("上海日界刷新后应使用升级防守及新增对手的新凭证", next.Offers)
	}
	t.Logf("镜像日界观察 {\"prior_day\":%q,\"next_day\":%q,\"prior_offers\":%d,\"next_offers\":%d,\"prior_entries\":%d,\"next_entries\":%d,\"frozen_version\":%d,\"new_version\":%d}",
		shanghaiOperatingDay(boundary.Add(-time.Millisecond)), shanghaiOperatingDay(boundary), len(prior.Offers), len(next.Offers), prior.Entries,
		next.Entries, grant.DefenseVersion, later.Version)
}

// TestMirrorMatchingBoundaryAndLevelLimitMongo 覆盖已有近级截断末层以及极端等级域。
func TestMirrorMatchingBoundaryAndLevelLimitMongo(t *testing.T) {
	ctx, store, midnight := mirrorMatchingMongo(t)
	cfg := *store.Cfg.Fixture.MirrorArena
	role := domain.Role{ID: "原申请角色", AccountID: "原申请账号", Job: 0, Level: 30, Origin: store.Cfg.OriginID}
	collection := store.DB.Collection(mirrorPoolCollection)
	pivot := fmt.Sprintf("%064x", 50)
	assert := func(name, scope string, rows []mirrorPoolEntry, wantNames []string) {
		t.Helper()
		_, err := collection.InsertMany(ctx, rows)
		mirrorMatchingMust(t, err)
		got, err := store.selectMirrorDefenses(ctx, role, scope, pivot, &cfg)
		mirrorMatchingMust(t, err)
		var names []string
		for _, row := range got {
			names = append(names, row.Name)
		}
		if !slices.Equal(names, wantNames) {
			t.Fatalf("%s：实际候选 %v，预期 %v", name, names, wantNames)
		}
		t.Logf("镜像边界 %s 结果=%v", name, names)
	}
	cfg.OpponentLimit = 5
	scope := mirrorScope(store.Cfg.RunID, role.Origin, midnight)
	rows := []mirrorPoolEntry{
		mirrorMatchingRow(store, scope, "同级·05", 0, 30, 5),
		mirrorMatchingRow(store, scope, "近低·90", 0, 29, 90),
		mirrorMatchingRow(store, scope, "近低·30", 1, 29, 30),
		mirrorMatchingRow(store, scope, "近高·70", 2, 31, 70),
		mirrorMatchingRow(store, scope, "次近低·01", 0, 28, 1),
		mirrorMatchingRow(store, scope, "次近低·51", 1, 28, 51),
		mirrorMatchingRow(store, scope, "次近低·55", 1, 28, 55),
		mirrorMatchingRow(store, scope, "次近低·120", 2, 28, 120),
		mirrorMatchingRow(store, scope, "次近低·200", 2, 28, 200),
		mirrorMatchingRow(store, scope, "次近高·48", 0, 32, 48),
		mirrorMatchingRow(store, scope, "次近高·60", 1, 32, 60),
		mirrorMatchingRow(store, scope, "次近高·65", 1, 32, 65),
		mirrorMatchingRow(store, scope, "次近高·80", 2, 32, 80),
	}
	assert("同级先绕环、近级整层合并、两侧部分末层重新选择", scope, rows,
		[]string{"同级·05", "近高·70", "近低·90", "近低·30", "次近低·51"})
	maxLevel := ^uint32(0)
	role.Level, cfg.MaxLevelGap, cfg.OpponentLimit = maxLevel-1, 3, 5
	scope = mirrorScope(store.Cfg.RunID, role.Origin, midnight.AddDate(0, 0, -1))
	rows = []mirrorPoolEntry{
		mirrorMatchingRow(store, scope, "最高·50", 0, maxLevel, 50),
		mirrorMatchingRow(store, scope, "同级·01", 1, maxLevel-1, 1),
		mirrorMatchingRow(store, scope, "近低·60", 2, maxLevel-2, 60),
		mirrorMatchingRow(store, scope, "窗内低沿·20", 2, maxLevel-4, 20),
		mirrorMatchingRow(store, scope, "窗外低沿·21", 2, maxLevel-5, 21),
	}
	assert("上沿 uint32 饱和并接受等于轮换点的选择器", scope, rows,
		[]string{"同级·01", "最高·50", "近低·60", "窗内低沿·20"})
	role.Level, cfg.MaxLevelGap = 1, maxLevel
	scope = mirrorScope(store.Cfg.RunID, role.Origin, midnight.AddDate(0, 0, -2))
	rows = []mirrorPoolEntry{
		mirrorMatchingRow(store, scope, "零级·60", 0, 0, 60),
		mirrorMatchingRow(store, scope, "起始·01", 1, 1, 1),
		mirrorMatchingRow(store, scope, "次级·50", 2, 2, 50),
		mirrorMatchingRow(store, scope, "高端·20", 2, maxLevel, 20),
	}
	assert("下沿不回绕至零，无需遍历缺席等级", scope, rows, []string{"起始·01", "次级·50", "高端·20"})
}
