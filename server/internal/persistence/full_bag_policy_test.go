package persistence

import (
	"minimir/internal/domain"
	"reflect"
	"testing"
)

func TestFullBagContinuousEventsKeepExperienceGoldAndHostingCharges(t *testing.T) {
	f, r := growthFixture(t), recycleRole()
	r.ProgressionFormatVersion, r.ProgressionCurveVersion = 1, f.Progression.CurveVersion
	r.Capacity, r.HostingSeconds, r.PickupStopped = 1, 3, true
	protected := recycleItem(218, true)
	protected.Locked = true
	r.Items = []domain.Item{protected}
	before := r
	for index := 0; index < 3; index++ {
		q := Settlement{SourceSeq: r.SourceSeq + 1, Kills: 1, Experience: 7, HostSeconds: 1, HostingSession: "full-session", NoEquipmentKills: r.NoEquipmentKills + 1}
		if index == 1 {
			q.Gold = 30 // 金币事件与物品事件分开，不造每kill同时掉金掉物的规则。
		} else {
			incoming := recycleItem(35, false)
			q.Item = &incoming
		}
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

func TestFullBagNoDropStillSweepsCommittedPolicyBeforeCapacity(t *testing.T) {
	f, r := growthFixture(t), recycleRole()
	r.ProgressionFormatVersion, r.ProgressionCurveVersion = 1, f.Progression.CurveVersion
	r.Capacity, r.PickupStopped = 1, true
	r.Items = []domain.Item{recycleItem(218, true)}
	out, receipt, err := applySettlement(r, Settlement{SourceSeq: 5, Kills: 1, Experience: 7, HostingSession: "sweep"}, f)
	if err != nil || len(out.Items) != 0 || out.PickupStopped || out.Wallet.BoundGold != 50 || len(receipt.Recycling.Recycled) != 1 || out.Report.StopReason != 0 {
		t.Fatal("旧拾取阻塞阻止了后续kill按已提交策略先回收", out, receipt, err)
	}
}

func TestFullBagLegacyReportRemainsHistoricalUntilTrueTerminalReason(t *testing.T) {
	for _, death := range []bool{false, true} {
		f, r := growthFixture(t), recycleRole()
		r.ProgressionFormatVersion, r.ProgressionCurveVersion = 1, f.Progression.CurveVersion
		r.HostingSeconds = 1
		r.Report = domain.Report{SessionID: "old-session", StopReason: 3}
		q := Settlement{SourceSeq: 5, HostSeconds: 1, HostingSession: "old-session"}
		want := uint32(1)
		if death {
			q.StopReason, want = 2, 2
		}
		out, _, err := applySettlement(r, q, f)
		if err != nil || out.Report.StopReason != want || r.Report.StopReason != 3 {
			t.Fatal("旧满包状态遮住真实终态，或改写原输入历史", out.Report, err)
		}
	}
}
