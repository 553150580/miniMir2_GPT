// 本文件复用真实镜像格战，比较同一名 35 级法师更换单体攻击配招后的结果。
package game

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// TestMirrorBalanceMageLoadoutSensitivity 固定等级、装备、工作簿、攻守换边与种子，只改法师的四个槽。
func TestMirrorBalanceMageLoadoutSensitivity(t *testing.T) {
	out := os.Getenv("MINIMIR_MIRROR_LOADOUT_REPORT")
	if out == "" {
		t.Skip("显式提供 MINIMIR_MIRROR_LOADOUT_REPORT 后运行法师配招复测")
	}
	if !filepath.IsAbs(out) || filepath.Ext(out) != ".json" {
		t.Fatal("法师配招报告应使用新的绝对 JSON 路径")
	}
	started := time.Now()
	base := newMirrorBalanceWorker(t)
	arena, provenance := loadMirrorBalanceProjection(t, base)
	const seeds = 256
	report := mirrorBalanceReport{Scope: "规则30法师单体配招对照，固定三套公共装备和原镜像政策", GoVersion: runtime.Version(),
		Provenance: provenance, TickMS: mirrorBalanceTick.Milliseconds(), MainSeeds: seeds, DurationMS: arena.Duration.Milliseconds(),
		Arena: map[string]*devconfig.MirrorArenaConfig{"source": arena}, Intervals: [2]string{"各方向采用 Wilson 95% 区间", "按同一种子的双方换边做 1024 次成对重采样，报告 95% 区间"}}
	fixture := &base.e.Store.Cfg.Fixture
	for _, gear := range []string{"early", "middle", "late"} {
		warrior := mirrorBalanceSnapshot(t, base, 0, 35, gear, "standard")
		mage := mirrorBalanceSnapshot(t, base, 1, 35, gear, "standard")
		taoist := mirrorBalanceSnapshot(t, base, 2, 35, gear, "standard")
		mixed := mirrorBalanceMageSlots(t, fixture, mage, "lightning-in-fourth", [4]uint32{33, 31, 10, 11})
		single := mirrorBalanceMageSlots(t, fixture, mage, "single-target", [4]uint32{31, 11, 5, 1})
		report.Roster = append(report.Roster, warrior, mage, taoist, mixed, single)
		for _, opponent := range []mirrorBalanceRosterEntry{warrior, taoist} {
			for _, loadout := range []mirrorBalanceRosterEntry{mage, mixed, single} {
				for mageSeat := range 2 {
					entries := [2]mirrorBalanceRosterEntry{}
					entries[mageSeat], entries[1-mageSeat] = loadout, opponent
					one := mirrorBalanceCase{group: "mage_loadout", arena: "source", seeds: seeds}
					row, err := simulateMirrorBalanceCase(base, arena, one, entries)
					if err != nil || row.Interrupted.Count != 0 {
						t.Fatalf("法师配招对照失败 %s 对 %s: %v", entries[0].ID, entries[1].ID, err)
					}
					spells := row.SideTotals[mageSeat].Spells
					if loadout.Profile == "standard" && (len(spells) != 1 || spells[31] == 0) {
						t.Fatalf("原法师组合有魔法盾以外的受理记录: %v", spells)
					}
					if loadout.Profile != "standard" && spells[11] == 0 {
						t.Fatal("新增的合法雷电术没有成功受理", gear, opponent.Job, mageSeat)
					}
					report.Cases = append(report.Cases, row)
					report.Trials += seeds
				}
			}
		}
	}
	report.Paired = mirrorBalancePairs(report.Cases)
	report.ElapsedMS = time.Since(started).Milliseconds()
	writeMirrorBalanceReport(t, out, report)
	t.Logf("完成 %d 场法师配招对照，攻守换边 %d 组，报告 %s", report.Trials, len(report.Paired), out)
}

// mirrorBalanceMageSlots 检查书、职业和原槽资格，只在生产冻结快照上替换玩家可持久保存的四槽。
func mirrorBalanceMageSlots(t *testing.T, f *devconfig.Fixture, source mirrorBalanceRosterEntry, profile string, slots [4]uint32) mirrorBalanceRosterEntry {
	t.Helper()
	role := domain.Role{Job: source.Job, Level: source.Level, Skills: source.Snapshot.Skills}
	for index, id := range slots {
		skill, ok := f.Skill(id)
		_, learned := role.Skill(id)
		if !ok || !learned || skill.JobID != role.Job || !skill.AttackSlotEligible() || skill.MaterialQuantity != 0 ||
			!mirrorBalanceBookAvailable(f, skill.BookItemID, role.Level) {
			t.Fatalf("法师候选 %s 技能槽 %d 的技能 %d 不符合正式编排资格", profile, index+1, id)
		}
	}
	result := source
	result.Profile = profile
	result.ID = source.ID + "-" + profile
	result.Snapshot.SkillSlotIDs = slots[:]
	return result
}
