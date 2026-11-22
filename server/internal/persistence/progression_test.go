// 验证旧成长字段迁移、经验上限和降级边界，确保经验限制与金币、掉落等收益独立计算。

package persistence

import (
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"reflect"
	"testing"
)

// 显式单元合同，不代表主经验曲线或自然成长速度。
// t 提供断言失败报告、子用例和清理注册。
func growthFixture(t *testing.T) devconfig.Fixture {
	// f 接收回收开发测试配置供后续步骤使用。
	f := recycleFixture(t)
	f.RuleVersion = 20
	f.Progression = devconfig.ProgressionRules{CurveVersion: "unit-curve-20", InitialLevelCap: 50, RebirthLevelCapIncrement: 1, OperatingLevelCap: 50, CatalogMaxLevel: 50, PendingPoolPolicy: "one_next_level", NewOverflowPolicy: "reject_new_with_receipt", LegacyOverflowPolicy: "preserve_frozen", LevelUpResources: "source_full_if_alive_original_birth"}
	f.Progression.ExpPerLevel = make([]int64, 50)
	// i 标记遍历经验“per”等级时访问的下标。
	for i := range f.Progression.ExpPerLevel {
		f.Progression.ExpPerLevel[i] = 10
	}
	f.Progression.ExpPerLevel[49] = 100
	return f
}

// TestGrowthSettlementCapStillCommitsGoldItemAndExactReport 核验池满不应停止其它奖励或先加溢出。
// t 提供断言失败报告、子用例和清理注册。
func TestGrowthSettlementCapStillCommitsGoldItemAndExactReport(t *testing.T) {
	// f 接收成长开发测试配置供后续步骤使用，r 接收回收角色供后续步骤使用。
	f, r := growthFixture(t), recycleRole()
	r.AutoRecycle.Enabled = false
	r.Level = 50
	r.ProgressionFormatVersion = 1
	r.ProgressionCurveVersion = f.Progression.CurveVersion
	r.PendingExperience = 95
	// item 保存由回收道具取得的道具。
	item := recycleItem(218, true)
	// q 组装结算所需的数据。
	q := Settlement{SourceSeq: 5, Kills: 1, Experience: 15, Gold: 30, Item: &item, HostSeconds: 1, HostingSession: "session"}
	// out 保存由应用结算取得的输出，receipt 保存由应用结算取得的回执，err 接收应用结算过程中可能产生的错误，供后续失败分支处理。
	out, receipt, err := applySettlement(r, q, f)
	if err != nil || out.Level != 50 || out.PendingExperience != 100 || out.Experience != 0 || out.Wallet.Gold != 30 || len(out.Items) != 1 || out.SourceSeq != 5 || out.EconomicSeq != 5 || out.HostingSeconds != 19 || out.Report.Kills != 1 || out.Report.Experience != 5 || out.Report.RawExperience != 15 || out.Report.PooledExperience != 5 || out.Report.RejectedExperience != 10 {
		t.Fatal(out, receipt, err)
	}
	if receipt.Experience == nil || receipt.Experience.ActualGranted != 5 || receipt.Experience.ToPool != 5 || receipt.Experience.Rejected != 10 || receipt.Experience.MigrationBefore != nil {
		t.Fatal(receipt)
	}
	r = out
	q.SourceSeq = 6
	q.Item = nil
	q.Experience = math.MaxInt64
	q.HostingSession = ""
	out, receipt, err = applySettlement(r, q, f)
	if err != nil || out.Wallet.Gold != 60 || out.PendingExperience != 100 || receipt.Experience.ActualGranted != 0 || receipt.Experience.Rejected != math.MaxInt64 || out.SourceSeq != 6 {
		t.Fatal("池满不应停止其它奖励或先加溢出", out, receipt, err)
	}
}

// TestGrowthSettlementLegacyBalanceIsSeparateFromNewActual 验证旧经验按规则迁移、溢出封存，本场报告仅记录新经验的申请和实际拒绝，金币仍可入账。
// t 提供断言失败报告、子用例和清理注册。
func TestGrowthSettlementLegacyBalanceIsSeparateFromNewActual(t *testing.T) {
	// f 接收成长开发测试配置供后续步骤使用，r 接收回收角色供后续步骤使用。
	f, r := growthFixture(t), recycleRole()
	r.Level = 21
	r.Experience = 2000
	// out 保存由应用结算取得的输出，receipt 保存由应用结算取得的回执，err 接收应用结算过程中可能产生的错误，供后续失败分支处理。
	out, receipt, err := applySettlement(r, Settlement{SourceSeq: 5, Kills: 1, Experience: 15, Gold: 30, HostingSession: "session"}, f)
	if err != nil || out.Level != 50 || out.Experience != 0 || out.PendingExperience != 100 || out.LegacyExperienceOverflow != 1610 || out.Wallet.Gold != 30 || out.Report.Experience != 0 || out.Report.RawExperience != 15 || out.Report.RejectedExperience != 15 || receipt.Experience.LevelBefore != 50 || receipt.Experience.LevelAfter != 50 || receipt.Experience.ActualGranted != 0 || receipt.Experience.MigrationBefore.Experience != 2000 || receipt.Experience.MigrationBefore.Level != 21 {
		t.Fatal(out, receipt, err)
	}
}

// TestGrowthCurveMismatchPreservesAllBalancesAndProjectsUnknownLimits 核验旧余额不能按新曲线伪造比例或拒整份Inventory。
// t 提供断言失败报告、子用例和清理注册。
func TestGrowthCurveMismatchPreservesAllBalancesAndProjectsUnknownLimits(t *testing.T) {
	// f 接收成长开发测试配置供后续步骤使用，r 接收回收角色供后续步骤使用。
	f, r := growthFixture(t), recycleRole()
	r.Level = 21
	r.ProgressionFormatVersion = 1
	r.ProgressionCurveVersion = "old-balance-curve"
	r.Experience = 900
	r.PendingExperience = 800
	r.LegacyExperienceOverflow = 700
	// out 保存由应用结算取得的输出，receipt 保存由应用结算取得的回执，err 接收应用结算过程中可能产生的错误，供后续失败分支处理。
	out, receipt, err := applySettlement(r, Settlement{SourceSeq: 5, Kills: 1, Experience: 15, Gold: 30}, f)
	if err != nil || out.Level != 21 || out.Experience != 900 || out.PendingExperience != 800 || out.LegacyExperienceOverflow != 700 || out.ProgressionCurveVersion != "old-balance-curve" || !out.ProgressionMigrationRequired || receipt.Experience.ActualGranted != 0 || out.Wallet.Gold != 30 {
		t.Fatal(out, receipt, err)
	}
	// state 保存由成长状态取得的状态。
	state, err := ProgressionState(out, f)
	if err != nil || !state.MigrationRequired || state.CurveVersion != f.Progression.CurveVersion || state.NextLevelExperience != 0 || state.PendingExperienceLimit != 0 || state.PendingExperience != 800 {
		t.Fatal("旧余额不能按新曲线伪造比例或拒整份Inventory", state, err)
	}
}

// TestGrowthPureMigrationKeepsReportHistoricalAcceptedExperience 核对「入场重试再次迁移」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestGrowthPureMigrationKeepsReportHistoricalAcceptedExperience(t *testing.T) {
	// f 接收成长开发测试配置供后续步骤使用，r 接收回收角色供后续步骤使用。
	f, r := growthFixture(t), recycleRole()
	r.Report = domain.Report{SessionID: "old-session", Experience: 123, Kills: 9}
	// c 接收成长规则合同供后续步骤使用，err 接收成长规则合同过程中可能产生的错误，供后续失败分支处理。
	c, err := GrowthContract(f)
	if err != nil {
		t.Fatal(err)
	}
	// out 保存由经验带有规则合同取得的输出，g 保存经验带有规则合同返回的结果。
	out, g, err := experienceWithContract(r, 0, c)
	if err != nil || g.ActualGranted != 0 || g.MigrationBefore == nil || out.Report.RawExperience != 123 || out.Report.Experience != 123 || out.Report.Kills != 9 || r.Report.RawExperience != 0 {
		t.Fatal(out, g, err)
	}
	// before 保存当前处理所用的操作前的状态。
	before := out
	out, g, err = experienceWithContract(out, 0, c)
	if err != nil || !reflect.DeepEqual(out, before) || g.MigrationBefore != nil {
		t.Fatal("入场重试再次迁移", out, g, err)
	}
}
