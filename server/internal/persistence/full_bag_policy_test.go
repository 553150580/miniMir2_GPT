// 验证背包满时既有物品的保留、当前掉落的放弃及托管停止原因和收益账本。

package persistence

import (
	"minimir/internal/domain"
	"reflect"
	"testing"
)

// TestFullBagContinuousEventsKeepExperienceGoldAndHostingCharges 核验无法入包的新物品应真实弃掉并记录原因，用诊断「逐事件EXP/金币/弃物/实际托管账不符」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestFullBagContinuousEventsKeepExperienceGoldAndHostingCharges(t *testing.T) {
	// f 接收成长开发测试配置供后续步骤使用，r 接收回收角色供后续步骤使用。
	f, r := growthFixture(t), recycleRole()
	r.ProgressionFormatVersion, r.ProgressionCurveVersion = 1, f.Progression.CurveVersion
	r.Capacity, r.HostingSeconds, r.PickupStopped = 1, 3, true
	// protected 保存由回收道具取得的“protected”。
	protected := recycleItem(218, true)
	protected.Locked = true
	r.Items = []domain.Item{protected}
	// before 保存当前处理所用的操作前的状态。
	before := r
	// index 规定下标的初始取值。
	for index := 0; index < 3; index++ {
		// q 组装结算所需的数据。
		q := Settlement{SourceSeq: r.SourceSeq + 1, Kills: 1, Experience: 7, HostSeconds: 1, HostingSession: "full-session", NoEquipmentKills: r.NoEquipmentKills + 1}
		if index == 1 {
			q.Gold = 30 // 金币事件与物品事件分开，不造每kill同时掉金掉物的规则。
		} else {
			// incoming 保存由回收道具取得的新到达项。
			incoming := recycleItem(35, false)
			q.Item = &incoming
		}
		// out 保存由应用结算取得的输出，receipt 保存由应用结算取得的回执，err 接收应用结算过程中可能产生的错误，供后续失败分支处理。
		out, receipt, err := applySettlement(r, q, f)
		if err != nil || !reflect.DeepEqual(out.Items, before.Items) || out.SourceSeq != q.SourceSeq || out.Revision != r.Revision+1 || out.EconomicSeq != before.EconomicSeq || out.HostingSeconds != r.HostingSeconds-1 || receipt.Experience.ActualGranted != q.Experience {
			t.Fatal("满包阻断合法收益/扣时或改变保护原件", out, receipt, err)
		}
		if q.Item != nil && (receipt.GrantedItem || receipt.Recycling.DiscardedCount != 1 || receipt.Recycling.DiscardReason != "bag_full") {
			t.Fatal("无法入包的新物品没有真实弃掉并记录原因", receipt)
		}
		if q.Item == nil && receipt.Recycling.DiscardedCount != 0 {
			t.Fatal("金币事件伪造弃物")
		}
		if index < 2 && out.Report.StopReason != 0 || index == 2 && out.Report.StopReason != 1 {
			t.Fatal("满包被误记为停机或遮住真实余额耗尽", out.Report)
		}
		r = out
	}
	if r.Level != 3 || r.Experience != 1 || r.Wallet.Gold != 30 || r.Report.Kills != 3 || r.Report.Experience != 21 || r.Report.RawExperience != 21 || r.Report.DiscardCount != 2 || r.Report.TotalDropCount != 2 || len(r.Report.Drops) != 0 || r.Report.UsedSeconds != 3 || !r.PickupStopped {
		t.Fatal("逐事件EXP/金币/弃物/实际托管账不符", r)
	}
}

// TestFullBagNoDropStillSweepsCommittedPolicyBeforeCapacity 核对「旧拾取阻塞阻止了后续kill按已提交策略先回收」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestFullBagNoDropStillSweepsCommittedPolicyBeforeCapacity(t *testing.T) {
	// f 接收成长开发测试配置供后续步骤使用，r 接收回收角色供后续步骤使用。
	f, r := growthFixture(t), recycleRole()
	r.ProgressionFormatVersion, r.ProgressionCurveVersion = 1, f.Progression.CurveVersion
	r.Capacity, r.PickupStopped = 1, true
	r.Items = []domain.Item{recycleItem(218, true)}
	// out 保存由应用结算取得的输出，receipt 保存由应用结算取得的回执，err 接收应用结算过程中可能产生的错误，供后续失败分支处理。
	out, receipt, err := applySettlement(r, Settlement{SourceSeq: 5, Kills: 1, Experience: 7, HostingSession: "sweep"}, f)
	if err != nil || len(out.Items) != 0 || out.PickupStopped || out.Wallet.BoundGold != 50 || len(receipt.Recycling.Recycled) != 1 || out.Report.StopReason != 0 {
		t.Fatal("旧拾取阻塞阻止了后续kill按已提交策略先回收", out, receipt, err)
	}
}

// TestFullBagLegacyReportRemainsHistoricalUntilTrueTerminalReason 核对「旧满包状态遮住真实终态，或改写原输入历史」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestFullBagLegacyReportRemainsHistoricalUntilTrueTerminalReason(t *testing.T) {
	// death 接收遍历布尔状态列表时当前的元素。
	for _, death := range []bool{false, true} {
		// f 接收成长开发测试配置供后续步骤使用，r 接收回收角色供后续步骤使用。
		f, r := growthFixture(t), recycleRole()
		r.ProgressionFormatVersion, r.ProgressionCurveVersion = 1, f.Progression.CurveVersion
		r.HostingSeconds = 1
		r.Report = domain.Report{SessionID: "old-session", StopReason: 3}
		// q 组装结算所需的数据。
		q := Settlement{SourceSeq: 5, HostSeconds: 1, HostingSession: "old-session"}
		// want 保存由无符号整数“32”取得的期望值。
		want := uint32(1)
		if death {
			q.StopReason, want = 2, 2
		}
		// out 保存由应用结算取得的输出，err 接收应用结算过程中可能产生的错误，供后续失败分支处理。
		out, _, err := applySettlement(r, q, f)
		if err != nil || out.Report.StopReason != want || r.Report.StopReason != 3 {
			t.Fatal("旧满包状态遮住真实终态，或改写原输入历史", out.Report, err)
		}
	}
}
