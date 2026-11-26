// 本文件把镜像冻结样本组成主对照与边界矩阵，并输出可复现的独立验收报告。
package game

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// mirrorBalanceKey 以输入身份缓存原角色冷快照，配装与资源名称供报告追溯。
type mirrorBalanceKey struct {
	job, level    uint32
	gear, profile string
}

// mirrorBalanceCase 将场地与政策变化保持为显式的独立试验。
type mirrorBalanceCase struct {
	group, arena string
	sides        [2]mirrorBalanceKey
	seeds        int
}

// mirrorBalanceCaseResult 保存单向参战结果和供换边配对的逐种子胜方。
type mirrorBalanceCaseResult struct {
	Group       string                     `json:"group"`
	Arena       string                     `json:"arena"`
	AttackID    string                     `json:"attacker_snapshot"`
	DefenseID   string                     `json:"defender_snapshot"`
	LevelGap    int                        `json:"defender_minus_attacker_level"`
	Seeds       int                        `json:"seeds"`
	AttackWin   mirrorBalanceRate          `json:"attacker_win"`
	DefenseWin  mirrorBalanceRate          `json:"defender_win"`
	Defeat      mirrorBalanceRate          `json:"defeat"`
	Timeout     mirrorBalanceRate          `json:"timeout"`
	TimeoutTie  mirrorBalanceRate          `json:"timeout_tie"`
	Interrupted mirrorBalanceRate          `json:"interrupted"`
	P50MS       int64                      `json:"duration_p50_ms"`
	P90MS       int64                      `json:"duration_p90_ms"`
	SideTotals  [2]mirrorBalanceSideResult `json:"side_totals"`
	outcomes    []uint32                   // 下标是实际种子，仅用于计算换边区间。
}

// mirrorBalancePairedResult 以相同种子观察同两个快照在互换进攻与防守席位后的胜率。
type mirrorBalancePairedResult struct {
	Group     string            `json:"group"`
	Arena     string            `json:"arena"`
	FirstID   string            `json:"first_snapshot"`
	SecondID  string            `json:"second_snapshot"`
	SeedPairs int               `json:"seed_pairs"`
	FirstWins mirrorBalanceRate `json:"first_snapshot_seat_combined_win"`
}

// mirrorBalanceProvenance 将工作簿、规则和官方投影的摘要一同封入报告。
type mirrorBalanceProvenance struct {
	WorkbookSHA   string          `json:"workbook_sha256"`
	BaseSHA       string          `json:"base_fixture_sha256"`
	ProjectionSHA string          `json:"projection_fixture_sha256"`
	RuleVersion   uint64          `json:"rule_version"`
	Projection    json.RawMessage `json:"arena_source_evidence"`
}

// mirrorBalanceReport 保存重跑任何一组所需的输入和真实采样结果。
type mirrorBalanceReport struct {
	Scope      string                                  `json:"scope"`
	GoVersion  string                                  `json:"go_version"`
	Provenance mirrorBalanceProvenance                 `json:"source"`
	TickMS     int64                                   `json:"tick_ms"`
	FirstSeed  int64                                   `json:"first_seed"`
	MainSeeds  int                                     `json:"main_seeds"`
	SideSeeds  int                                     `json:"sensitivity_seeds"`
	DurationMS int64                                   `json:"battle_duration_ms"`
	ElapsedMS  int64                                   `json:"matrix_elapsed_ms"`
	Trials     int                                     `json:"battle_count"`
	Intervals  [2]string                               `json:"confidence_interval_methods"`
	Arena      map[string]*devconfig.MirrorArenaConfig `json:"arena_variants"`
	Roster     []mirrorBalanceRosterEntry              `json:"snapshots"`
	Cases      []mirrorBalanceCaseResult               `json:"cases"`
	Paired     []mirrorBalancePairedResult             `json:"seat_swapped_pairs"`
}

// TestMirrorBalanceMatrix 需要独立候选与输出文件时才执行整套真实格战。
func TestMirrorBalanceMatrix(t *testing.T) {
	out := os.Getenv("MINIMIR_MIRROR_BALANCE_REPORT")
	if out == "" {
		t.Skip("显式提供 MINIMIR_MIRROR_BALANCE_REPORT 和 MINIMIR_MIRROR_BALANCE_FIXTURE 后运行矩阵")
	}
	if !filepath.IsAbs(out) || filepath.Ext(out) != ".json" {
		t.Fatal("镜像报告应使用绝对路径的新 JSON 文件")
	}
	seeds := 256
	if value := os.Getenv("MINIMIR_MIRROR_BALANCE_SEEDS"); value != "" {
		var err error
		seeds, err = strconv.Atoi(value)
		if err != nil || seeds < 1 || seeds > 1024 {
			t.Fatal("本机镜像对照只支持 1 至 1024 个显式种子")
		}
	}
	start := time.Now()
	base := newMirrorBalanceWorker(t)
	projection, provenance := loadMirrorBalanceProjection(t, base)
	arenas := mirrorBalanceArenaVariants(t, projection)
	sideSeeds := max(1, seeds/4)
	cases := mirrorBalanceCases(seeds, sideSeeds, base.e.Store.Cfg.Fixture.Progression.OperatingLevelCap)
	roster := make(map[mirrorBalanceKey]mirrorBalanceRosterEntry)
	report := mirrorBalanceReport{Scope: "规则30三职业隔离战斗候选，种子及配装样本用于对照", GoVersion: runtime.Version(),
		Provenance: provenance, TickMS: mirrorBalanceTick.Milliseconds(), MainSeeds: seeds, SideSeeds: sideSeeds,
		DurationMS: projection.Duration.Milliseconds(), Arena: arenas, Cases: make([]mirrorBalanceCaseResult, len(cases)),
		Intervals: [2]string{"各方向采用 Wilson 95% 区间", "按同一种子的攻守两场为抽样单位进行 1024 次成对重采样，报告 95% 百分位区间"}}
	for _, one := range cases {
		report.Trials += one.seeds
		for _, key := range one.sides {
			if _, ok := roster[key]; !ok {
				entry := mirrorBalanceSnapshot(t, base, key.job, key.level, key.gear, key.profile)
				roster[key] = entry
				report.Roster = append(report.Roster, entry)
			}
		}
	}
	verifyMirrorBalanceReplay(t, base, arenas, roster)
	t.Logf("实际冻结 %d 个角色快照，开始 %d 组共 %d 场，攻守使用相同种子", len(report.Roster), len(cases), report.Trials)
	queue := make(chan int, len(cases))
	for index := range cases {
		queue <- index
	}
	close(queue)
	errors := make([]error, len(cases))
	var workers sync.WaitGroup
	for range min(4, runtime.GOMAXPROCS(0)) {
		workers.Go(func() {
			runner := &worker{e: base.e}
			for index := range queue {
				one := cases[index]
				entries := [2]mirrorBalanceRosterEntry{roster[one.sides[0]], roster[one.sides[1]]}
				report.Cases[index], errors[index] = simulateMirrorBalanceCase(runner, arenas[one.arena], one, entries)
			}
		})
	}
	workers.Wait()
	for index, err := range errors {
		if err != nil {
			t.Fatalf("第%d组 %v: %v", index+1, cases[index], err)
		}
	}
	report.Paired = mirrorBalancePairs(report.Cases)
	report.ElapsedMS = time.Since(start).Milliseconds()
	writeMirrorBalanceReport(t, out, report)
	for _, group := range report.Cases {
		if group.Interrupted.Count != 0 {
			t.Errorf("%s %s 对 %s 有 %d 场系统中断，报告已保留原计数", group.Group, group.AttackID, group.DefenseID, group.Interrupted.Count)
		}
	}
	t.Logf("完成 %d 场，耗时 %d 毫秒，换边对照 %d 组，报告 %s", report.Trials, report.ElapsedMS, len(report.Paired), out)
}

// mirrorBalanceCases 主矩阵九种有序职业组合全样本，等级差、反射及宠物政策用同一批较短种子作敏感性观察。
func mirrorBalanceCases(mainSeeds, sideSeeds int, levelCap uint32) []mirrorBalanceCase {
	levels := [...]uint32{18, 22, 35}
	gears := [...]string{"early", "middle", "late"}
	var cases []mirrorBalanceCase
	add := func(group, arena string, left, right mirrorBalanceKey, seeds int) {
		cases = append(cases, mirrorBalanceCase{group: group, arena: arena, sides: [2]mirrorBalanceKey{left, right}, seeds: seeds})
	}
	standard := func(job, level uint32, gear string) mirrorBalanceKey {
		return mirrorBalanceKey{job: job, level: level, gear: gear, profile: "standard"}
	}
	for leftJob := uint32(0); leftJob < 3; leftJob++ {
		for rightJob := uint32(0); rightJob < 3; rightJob++ {
			for _, level := range levels {
				for _, gear := range gears {
					add("main", "source", standard(leftJob, level, gear), standard(rightJob, level, gear), mainSeeds)
				}
				left := standard(leftJob, level, "middle")
				for delta := uint32(1); delta <= 3; delta++ {
					otherLevel := level + delta
					if level != 18 {
						// 对照刺杀 22 和神兽 35 的学习门槛前后，初级样本跨过骷髅 19。
						otherLevel = level - delta
					}
					right := standard(rightJob, otherLevel, "middle")
					add("level_window", "source", left, right, sideSeeds)
					add("level_window", "source", right, left, sideSeeds)
				}
			}
			for _, level := range [...]uint32{22, 35} {
				left, right := standard(leftJob, level, "middle"), standard(rightJob, level, "middle")
				add("terrain", "source", left, right, sideSeeds)
				add("terrain", "reflected_x", left, right, sideSeeds)
			}
			add("supplies", "source", mirrorBalanceKey{leftJob, 35, "middle", "dry"}, mirrorBalanceKey{rightJob, 35, "middle", "dry"}, sideSeeds)
		}
	}
	petCases := [...]struct {
		level   uint32
		profile string
		arenas  []string
	}{
		{22, "skeleton", []string{"summon_disabled", "replacement_only"}},
		{22, "empty_skeleton", []string{"source", "replacement_only"}},
		{35, "shinsu", []string{"summon_disabled", "replacement_only", "pet_continues"}},
		{35, "empty_shinsu", []string{"source", "replacement_only"}},
	}
	for _, pet := range petCases {
		taoist := mirrorBalanceKey{2, pet.level, "middle", pet.profile}
		for opponent := uint32(0); opponent < 3; opponent++ {
			other := standard(opponent, pet.level, "middle")
			for _, arena := range pet.arenas {
				add("pet_policy", arena, taoist, other, sideSeeds)
				add("pet_policy", arena, other, taoist, sideSeeds)
			}
		}
	}
	for _, sample := range [...]mirrorBalanceKey{{2, 22, "middle", "skeleton"}, {2, 35, "middle", "shinsu"}} {
		for opponent := uint32(0); opponent < 3; opponent++ {
			other := standard(opponent, sample.level, "middle")
			add("pet_pressure", "exposed_entry_pet", sample, other, sideSeeds)
			add("pet_pressure", "exposed_entry_pet", other, sample, sideSeeds)
		}
	}
	// 机制压力样本不在候选等级窗，只使用当前等级上限与原装备来观察实际阵亡后的补召。
	starter := mirrorBalanceKey{2, 19, "early", "skeleton"}
	for opponent := uint32(0); opponent < 3; opponent++ {
		veteran := standard(opponent, levelCap, "late")
		add("pet_pressure_outside_window", "exposed_entry_pet", starter, veteran, sideSeeds)
		add("pet_pressure_outside_window", "exposed_entry_pet", veteran, starter, sideSeeds)
	}
	return cases
}

// mirrorBalanceArenaVariants 保留官方候选，反射副本同时翻转障碍与双方的主人、宠物出生格。
func mirrorBalanceArenaVariants(t *testing.T, source *devconfig.MirrorArenaConfig) map[string]*devconfig.MirrorArenaConfig {
	t.Helper()
	reflected, disabled, replacement, continues := *source, *source, *source, *source
	reflectX := func(point devconfig.Point) devconfig.Point {
		return devconfig.Point{X: source.Map.Width - 1 - point.X, Y: point.Y}
	}
	reflected.AttackerSpawn, reflected.DefenderSpawn = reflectX(source.AttackerSpawn), reflectX(source.DefenderSpawn)
	reflected.AttackerPetSpawn, reflected.DefenderPetSpawn = reflectX(source.AttackerPetSpawn), reflectX(source.DefenderPetSpawn)
	reflected.Map.PlayerSpawn, reflected.Map.PetSpawn = reflected.AttackerSpawn, reflected.AttackerPetSpawn
	reflected.Map.Blocked = slices.Clone(source.Map.Blocked)
	for index, point := range reflected.Map.Blocked {
		reflected.Map.Blocked[index] = reflectX(point)
	}
	disabled.Summoning = nil
	policy := *source.Summoning
	policy.AllowWithoutEntryPet = false
	replacement.Summoning = &policy
	continues.Summoning = &policy
	continues.PetDeathPolicy = devconfig.MirrorArenaPetFightsAfterOwnerDeath
	pressure := replacement
	// 选择距敌方主人一格的真实空地。额外场景只改变入场宠物的地理风险。
	front := func(from, toward, reserved devconfig.Point) devconfig.Point {
		direction := int(facing(from, toward, 0))
		for _, turn := range [...]int{0, -1, 1, -2, 2, -3, 3, 4} {
			point := stepPoint(from, uint32((direction+turn+len(directions))%len(directions)), 1)
			if source.Map.Walkable(point) && point != toward && point != reserved &&
				source.Map.Walkable(devconfig.Point{X: point.X, Y: from.Y}) && source.Map.Walkable(devconfig.Point{X: from.X, Y: point.Y}) {
				return point
			}
		}
		t.Fatalf("官方候选起点 %v 周围没有合法宠物受压样本格", from)
		return devconfig.Point{}
	}
	pressure.AttackerPetSpawn = front(source.DefenderSpawn, source.AttackerSpawn, source.DefenderSpawn)
	pressure.DefenderPetSpawn = front(source.AttackerSpawn, source.DefenderSpawn, pressure.AttackerPetSpawn)
	pressure.Map.PetSpawn = pressure.AttackerPetSpawn
	return map[string]*devconfig.MirrorArenaConfig{"source": source, "reflected_x": &reflected, "summon_disabled": &disabled,
		"replacement_only": &replacement, "pet_continues": &continues, "exposed_entry_pet": &pressure}
}

// verifyMirrorBalanceReplay 用两套执行器重跑同一种子，以完整观测核对普通跨职和场内召唤。
func verifyMirrorBalanceReplay(t *testing.T, source *worker, arenas map[string]*devconfig.MirrorArenaConfig, roster map[mirrorBalanceKey]mirrorBalanceRosterEntry) {
	t.Helper()
	for _, check := range [][2]mirrorBalanceKey{
		{{0, 35, "middle", "standard"}, {1, 35, "middle", "standard"}},
		{{2, 35, "middle", "empty_shinsu"}, {0, 35, "middle", "standard"}},
	} {
		fighters := [2]domain.MirrorCombatSnapshot{roster[check[0]].Snapshot, roster[check[1]].Snapshot}
		first, err := runMirrorBalanceBattle(&worker{e: source.e}, arenas["source"], fighters, 42)
		if err != nil {
			t.Fatal("镜像固定种子首轮失败", err)
		}
		second, err := runMirrorBalanceBattle(&worker{e: source.e}, arenas["source"], fighters, 42)
		if err != nil || !reflect.DeepEqual(first, second) {
			t.Fatal("固定种子未重现原主体、资源和战斗结果", check, err)
		}
	}
}

// simulateMirrorBalanceCase 汇总真实逐种子结果，按正向和换边方向保留原胜方。
func simulateMirrorBalanceCase(w *worker, arena *devconfig.MirrorArenaConfig, one mirrorBalanceCase, entries [2]mirrorBalanceRosterEntry) (mirrorBalanceCaseResult, error) {
	result := mirrorBalanceCaseResult{Group: one.group, Arena: one.arena, AttackID: entries[0].ID, DefenseID: entries[1].ID,
		LevelGap: int(entries[1].Level) - int(entries[0].Level), Seeds: one.seeds, outcomes: make([]uint32, 0, one.seeds)}
	for index := range result.SideTotals {
		result.SideTotals[index].Potions = make(map[uint32]int64)
		result.SideTotals[index].Spells = make(map[uint32]int64)
		result.SideTotals[index].Attacks = make(map[uint32]int64)
	}
	var wins [3]int
	var causes [4]int
	ties := 0
	durations := make([]int64, 0, one.seeds)
	snaps := [2]domain.MirrorCombatSnapshot{entries[0].Snapshot, entries[1].Snapshot}
	for seed := 0; seed < one.seeds; seed++ {
		battle, err := runMirrorBalanceBattle(w, arena, snaps, int64(seed))
		if err != nil {
			return result, fmt.Errorf("种子%d: %w", seed, err)
		}
		if battle.winner > 2 || battle.cause > mirrorInterrupted {
			return result, fmt.Errorf("种子%d 战斗返回非法终态", seed)
		}
		if battle.timeoutTie {
			ties++
		}
		wins[battle.winner]++
		causes[battle.cause]++
		durations = append(durations, battle.durationMS)
		result.outcomes = append(result.outcomes, battle.winner)
		for side, values := range battle.sides {
			mirrorBalanceSideTotals(&result.SideTotals[side], values)
		}
	}
	slices.Sort(durations)
	result.P50MS, result.P90MS = mirrorBalanceElapsed(durations, 0.5), mirrorBalanceElapsed(durations, 0.9)
	result.AttackWin, result.DefenseWin = mirrorBalanceWilson(wins[1], one.seeds), mirrorBalanceWilson(wins[2], one.seeds)
	result.Defeat, result.Timeout = mirrorBalanceWilson(causes[mirrorDefeat], one.seeds), mirrorBalanceWilson(causes[mirrorTimeout], one.seeds)
	result.TimeoutTie, result.Interrupted = mirrorBalanceWilson(ties, one.seeds), mirrorBalanceWilson(causes[mirrorInterrupted], one.seeds)
	return result, nil
}

// mirrorBalancePairs 只合并同一地图与规则、相同角色样本和相同种子顺序的两场方向。
func mirrorBalancePairs(source []mirrorBalanceCaseResult) []mirrorBalancePairedResult {
	type pairedKey struct{ group, arena, first, second string }
	seen := make(map[pairedKey][]uint32)
	var out []mirrorBalancePairedResult
	for _, entry := range source {
		if entry.AttackID == entry.DefenseID {
			continue
		}
		key := pairedKey{entry.Group, entry.Arena, entry.DefenseID, entry.AttackID}
		if original, ok := seen[key]; ok {
			out = append(out, mirrorBalancePairedResult{Group: entry.Group, Arena: entry.Arena, FirstID: entry.DefenseID, SecondID: entry.AttackID,
				SeedPairs: entry.Seeds, FirstWins: mirrorBalancePairRate(original, entry.outcomes)})
			delete(seen, key)
		} else {
			seen[pairedKey{entry.Group, entry.Arena, entry.AttackID, entry.DefenseID}] = entry.outcomes
		}
	}
	return out
}

// loadMirrorBalanceProjection 核对候选正由当前唯一工作簿和未改写的正式开发底稿投影。
func loadMirrorBalanceProjection(t *testing.T, w *worker) (*devconfig.MirrorArenaConfig, mirrorBalanceProvenance) {
	t.Helper()
	path := os.Getenv("MINIMIR_MIRROR_BALANCE_FIXTURE")
	if !filepath.IsAbs(path) {
		t.Fatal("镜像竞技候选投影须采用显式绝对路径")
	}
	cfg := w.e.Store.Cfg
	defaultPath := cfg.FixtureFile
	if !filepath.IsAbs(defaultPath) {
		defaultPath = filepath.Join(cfg.Root, defaultPath)
	}
	base, defaultDigest := readMirrorBalanceInput(t, defaultPath)
	raw, projectionDigest := readMirrorBalanceInput(t, path)
	type inputEvidence struct {
		Evidence struct {
			Workbook struct {
				Path   string `json:"path"`
				Digest string `json:"sha256"`
			} `json:"workbook"`
			Arena json.RawMessage `json:"mirror_arena"`
		} `json:"_source_evidence"`
	}
	var baseline, recorded inputEvidence
	var candidate devconfig.Fixture
	for _, input := range []struct {
		data []byte
		into any
	}{{base, &baseline}, {raw, &recorded}, {raw, &candidate}} {
		if err := json.Unmarshal(input.data, input.into); err != nil {
			t.Fatal("镜像规则输入解析失败", err)
		}
	}
	var arenaEvidence struct {
		WorkbookSHA string `json:"workbook_sha256"`
		AssemblySHA string `json:"assembly_sha256"`
		CompilerSHA string `json:"compiler_sha256"`
	}
	if err := json.Unmarshal(recorded.Evidence.Arena, &arenaEvidence); err != nil {
		t.Fatal("缺少镜像官方投影出处", err)
	}
	_, workbookSHA := readMirrorBalanceInput(t, filepath.Join(cfg.Root, baseline.Evidence.Workbook.Path))
	if defaultDigest != cfg.Fixture.Hash || workbookSHA != baseline.Evidence.Workbook.Digest ||
		workbookSHA != recorded.Evidence.Workbook.Digest || workbookSHA != arenaEvidence.WorkbookSHA ||
		len(arenaEvidence.AssemblySHA) != 64 || len(arenaEvidence.CompilerSHA) != 64 ||
		!bytes.Equal(mirrorBalanceBaseJSON(t, base), mirrorBalanceBaseJSON(t, raw)) {
		t.Fatal("镜像投影与当前唯一工作簿或规则底稿不一致")
	}
	candidate.Hash = projectionDigest
	if err := candidate.Validate(); err != nil || candidate.MirrorArena == nil || candidate.MirrorArena.Summoning == nil ||
		!candidate.DevelopmentOnly || candidate.RuntimeReady || candidate.RuleVersion != cfg.Fixture.RuleVersion {
		t.Fatal("镜像投影非有效的同版本开发候选", err)
	}
	return candidate.MirrorArena, mirrorBalanceProvenance{WorkbookSHA: workbookSHA, BaseSHA: defaultDigest, ProjectionSHA: projectionDigest,
		RuleVersion: candidate.RuleVersion, Projection: recorded.Evidence.Arena}
}

// mirrorBalanceBaseJSON 仅允许官方投影增加镜像规则与投影出处，比较底稿保留原整数精度。
func mirrorBalanceBaseJSON(t *testing.T, raw []byte) []byte {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal("原规则 JSON 解析失败", err)
	}
	delete(value, "mirror_arena")
	delete(value, "_source_evidence")
	canonical, err := json.Marshal(value)
	if err != nil {
		t.Fatal("原规则无法重新编码", err)
	}
	return canonical
}

// readMirrorBalanceInput 读取一次原输入并记录逐字节摘要。
func readMirrorBalanceInput(t *testing.T, path string) ([]byte, string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取镜像对照文件 %s: %v", path, err)
	}
	return data, fmt.Sprintf("%x", sha256.Sum256(data))
}

// writeMirrorBalanceReport 使用独立文件收集验收，避免覆盖先前一轮真实报告。
func writeMirrorBalanceReport(t *testing.T, path string, report mirrorBalanceReport) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatal("打开镜像矩阵报告失败", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(report)
	closeErr := file.Close()
	if encodeErr != nil || closeErr != nil {
		t.Fatal("写入镜像矩阵报告失败", encodeErr, closeErr)
	}
}
