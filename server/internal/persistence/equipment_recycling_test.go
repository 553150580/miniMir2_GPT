package persistence

import (
	"encoding/json"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
)

func recycleFixture(t *testing.T) devconfig.Fixture {
	t.Helper()
	var items []devconfig.Item
	if err := json.Unmarshal([]byte(`[{"id":218,"is_equipment":true,"required_level":1,"drop_quality":0,"recycle_gold":50},{"id":193,"is_equipment":true,"required_level":7,"drop_quality":2,"recycle_gold":400},{"id":35}]`), &items); err != nil {
		t.Fatal(err)
	}
	f := devconfig.Fixture{RuleVersion: 19, Hash: "frozen-config", Items: items, EquipmentRecycling: &devconfig.EquipmentRecyclingRules{Mode: "development_gold_only", Outputs: []json.RawMessage{}}}
	f.Progression.ExpPerLevel = []int64{10, 20}
	return f
}

func recycleItem(id uint32, bound bool) domain.Item {
	quality := uint32(0)
	if id == 193 {
		quality = 2
	}
	return domain.Item{ID: domain.ID(), ItemID: id, Quantity: 1, Version: 3, Bound: bound, Quality: quality, SourceKind: "monster_drop", SourceID: "original-source"}
}

func recycleRole() domain.Role {
	return domain.Role{ID: domain.ID(), Level: 1, Revision: 10, SourceSeq: 4, EconomicSeq: 5, Capacity: 4, HostingSeconds: 20, AutoRecycle: domain.AutoRecyclePolicy{Enabled: true, QualityIDs: []uint32{0, 2}, Revision: 2}}
}

func TestAutoRecyclePolicyValidatesAndCopiesBoundedConfigurationIDs(t *testing.T) {
	f := recycleFixture(t)
	x := &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{4, 2, 0}, ProtectedItemIds: []uint32{218, 193}, Revision: 8}
	policy, code := policyFromProto(x, f)
	if code != 0 || !reflect.DeepEqual(policy.QualityIDs, []uint32{0, 2, 4}) || !reflect.DeepEqual(policy.ProtectedItemIDs, []uint32{193, 218}) {
		t.Fatal(policy, code)
	}
	x.QualityIds[0] = 1
	x.ProtectedItemIds[0] = 35
	if policy.QualityIDs[2] != 4 || policy.ProtectedItemIDs[1] != 218 {
		t.Fatal("策略引用了客户端切片")
	}
	for _, change := range []func(*pb.AutoRecyclePolicy){
		func(p *pb.AutoRecyclePolicy) { p.QualityIds = []uint32{5} }, func(p *pb.AutoRecyclePolicy) { p.QualityIds = []uint32{0, 0} }, func(p *pb.AutoRecyclePolicy) { p.QualityIds = nil },
		func(p *pb.AutoRecyclePolicy) { p.ProtectedItemIds = []uint32{35} }, func(p *pb.AutoRecyclePolicy) { p.ProtectedItemIds = []uint32{193, 193} }, func(p *pb.AutoRecyclePolicy) { p.Revision = math.MaxUint64 },
	} {
		q := &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0}}
		change(q)
		if _, code := policyFromProto(q, f); code != p.CodeInvalidArgument {
			t.Fatal("非法策略被受理", q)
		}
	}
	if _, code := policyFromProto(&pb.AutoRecyclePolicy{}, f); code != 0 {
		t.Fatal("历史关闭空策略应合法")
	}
}

func TestAutoRecycleFiltersRetainStrongProtectedAndUnknownOriginals(t *testing.T) {
	cases := []struct {
		name       string
		change     func(*domain.Role, *devconfig.Fixture)
		wantReason string
	}{
		{"锁定", func(r *domain.Role, _ *devconfig.Fixture) { r.Items[0].Locked = true }, "protected"},
		{"未确认结果", func(r *domain.Role, _ *devconfig.Fixture) { r.Items[0].PendingResultID = "result" }, "protected"},
		{"资产预留", func(r *domain.Role, _ *devconfig.Fixture) { r.Items[0].ReservationID = "reserved" }, "protected"},
		{"实际穿戴交叉保护", func(r *domain.Role, _ *devconfig.Fixture) { r.Equipped = []domain.Equipped{{Item: r.Items[0]}} }, "protected"},
		{"缺等级即使不限仍保留", func(_ *domain.Role, f *devconfig.Fixture) { f.Items[0].RequiredLevel = nil }, "required_level_unknown"},
		{"缺配方", func(_ *domain.Role, f *devconfig.Fixture) { f.EquipmentRecycling = nil }, "recipe_unknown"},
		{"null产物", func(_ *domain.Role, f *devconfig.Fixture) { f.EquipmentRecycling.Outputs = nil }, "recipe_unknown"},
		{"缺显式金币", func(_ *domain.Role, f *devconfig.Fixture) {
			var d devconfig.Item
			_ = json.Unmarshal([]byte(`{"id":218,"is_equipment":true,"required_level":1,"drop_quality":0}`), &d)
			f.Items[0] = d
		}, "recipe_unknown"},
		{"品质配置不符", func(r *domain.Role, _ *devconfig.Fixture) { r.Items[0].Quality = 2 }, "quality_unknown"},
		{"配置类型保护", func(r *domain.Role, _ *devconfig.Fixture) { r.AutoRecycle.ProtectedItemIDs = []uint32{218} }, ""},
		{"绑定保护", func(r *domain.Role, _ *devconfig.Fixture) { r.Items[0].Bound = true; r.AutoRecycle.ProtectBound = true }, ""},
		{"非绑保护", func(r *domain.Role, _ *devconfig.Fixture) { r.AutoRecycle.ProtectUnbound = true }, ""},
		{"品质未选", func(r *domain.Role, _ *devconfig.Fixture) { r.AutoRecycle.QualityIDs = []uint32{2} }, ""},
		{"等级上限", func(r *domain.Role, f *devconfig.Fixture) {
			n := uint32(2)
			f.Items[0].RequiredLevel = &n
			r.AutoRecycle.MaxRequiredLevel = 1
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, f := recycleRole(), recycleFixture(t)
			r.Items = []domain.Item{recycleItem(218, false)}
			tc.change(&r, &f)
			before := append([]domain.Item(nil), r.Items...)
			receipt := EquipmentRecyclingReceipt{}
			out, err := recycleExisting(r, f, &receipt)
			if err != nil || !reflect.DeepEqual(out.Items, before) || len(receipt.Recycled) != 0 || out.Wallet != r.Wallet || !reflect.DeepEqual(r.Items, before) {
				t.Fatal("保护原件被消耗", out, receipt, err)
			}
			if tc.wantReason != "" && (len(receipt.Skipped) != 1 || receipt.Skipped[0].Reason != tc.wantReason) {
				t.Fatal("保留原因未记录", receipt)
			}
			if _, _, code, err := recycleManual(r, []string{r.Items[0].ID}, f); tc.wantReason != "" && (code == 0 || err != nil) {
				t.Fatal("手动绕过同一强保护核", code, err)
			}
		})
	}
}

func TestAutoRecycleLevelBoundaryAndEachOriginalBinding(t *testing.T) {
	f, r := recycleFixture(t), recycleRole()
	r.Items = []domain.Item{recycleItem(218, false), recycleItem(193, true)}
	r.AutoRecycle.MaxRequiredLevel = 7
	before := append([]domain.Item(nil), r.Items...)
	receipt := EquipmentRecyclingReceipt{}
	out, err := recycleExisting(r, f, &receipt)
	if err != nil || len(out.Items) != 0 || out.Wallet.Gold != 50 || out.Wallet.BoundGold != 400 || len(receipt.Recycled) != 2 || !reflect.DeepEqual(receipt.Recycled, before) || !reflect.DeepEqual(r.Items, before) {
		t.Fatal(out, receipt, err)
	}
	// 等级上限不依角色当前等级判断，也不使用当前特权重判绑定。
	if out.Level != 1 || receipt.Gold != 50 || receipt.BoundGold != 400 {
		t.Fatal("非原件规则影响回收")
	}
}

func TestApplyExistingIsSinglePolicyChangeAndDoesNotAlterSourceOrReport(t *testing.T) {
	r, f := recycleRole(), recycleFixture(t)
	r.Capacity = 1
	r.PickupStopped = true
	r.Items = []domain.Item{recycleItem(218, true)}
	r.Report = domain.Report{SessionID: "previous", RecycleCount: 7}
	q := AssetRequest{AutoRecycle: &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0}, Revision: 2}, ApplyExisting: true}
	out, receipt, code, err := setAutoRecycle(r, q, f)
	if err != nil || code != 0 || len(out.Items) != 0 || out.AutoRecycle.Revision != 3 || out.SourceSeq != 4 || out.EconomicSeq != 5 || out.Revision != 10 || !reflect.DeepEqual(out.Report, r.Report) {
		t.Fatal(out, receipt, code, err)
	}
	// 公共Asset事务只在成功且有空位时解除拾取；此核不伪造owner Auto状态。
	if !out.PickupStopped || receipt.Trigger != 2 || receipt.BoundGold != 50 {
		t.Fatal("策略核越权改变拾取或丢失原回执")
	}
	q.AutoRecycle.Revision = 1
	if _, _, code, _ := setAutoRecycle(r, q, f); code != p.CodeOperationConflict {
		t.Fatal("旧策略版本未拒绝")
	}
	q.AutoRecycle.Revision = 2
	q.AutoRecycle.Enabled = false
	if _, _, code, _ := setAutoRecycle(r, q, f); code != p.CodeInvalidArgument {
		t.Fatal("关闭策略不能暗中应用现存背包")
	}
}

func TestItemLockUsesExactInstanceVersionAndProtectsManualRecycle(t *testing.T) {
	r, f := recycleRole(), recycleFixture(t)
	item := recycleItem(218, false)
	r.Items = []domain.Item{item}
	out, code := setItemLock(r, AssetRequest{ItemID: item.ID, ItemVersion: 3, Locked: true}, f)
	if code != 0 || !out.Items[0].Locked || out.Items[0].Version != 4 || r.Items[0].Locked || r.Items[0].Version != 3 {
		t.Fatal(out, code)
	}
	if _, _, code, _ := recycleManual(out, []string{item.ID}, f); code == 0 {
		t.Fatal("锁定原件被手动回收")
	}
	if _, code := setItemLock(out, AssetRequest{ItemID: item.ID, ItemVersion: 3}, f); code != p.CodeOperationConflict {
		t.Fatal("旧物品版本改变锁")
	}
	r.Items = nil
	r.Equipped = []domain.Equipped{{Slot: 0, Item: item}}
	out, code = setItemLock(r, AssetRequest{ItemID: item.ID, ItemVersion: 3, Locked: true}, f)
	if code != 0 || !out.Equipped[0].Item.Locked || r.Equipped[0].Item.Locked {
		t.Fatal("穿戴原件锁不是独立原子副本")
	}
}

func TestSettlementRecyclesBagBeforeCapacityAndIncomingDirectly(t *testing.T) {
	f, r := recycleFixture(t), recycleRole()
	r.Capacity = 1
	r.Items = []domain.Item{recycleItem(218, true)}
	incoming := recycleItem(35, false)
	q := Settlement{SourceSeq: 5, Kills: 1, Experience: 15, Gold: 30, HostSeconds: 1, HostingSession: "session", Item: &incoming, NoEquipmentKills: 19}
	out, receipt, err := applySettlement(r, q, f)
	if err != nil || len(out.Items) != 1 || out.Items[0].ID != incoming.ID || out.PickupStopped || out.SourceSeq != 5 || out.EconomicSeq != 5 || out.Revision != 11 || out.Level != 2 || out.Experience != 5 || out.Wallet.Gold != 30 || out.Wallet.BoundGold != 50 {
		t.Fatal(out, receipt, err)
	}
	if out.Report.RecycleCount != 1 || out.Report.RecycleBoundGold != 50 || out.Report.Earned.BoundGold != 50 || out.Report.Earned.Gold != 30 || out.Report.TotalDropCount != 1 || len(out.Report.Drops) != 1 || out.HostingSeconds != 19 {
		t.Fatal("原击杀结算/回收/战报不原子", out.Report)
	}
	// 背包只剩不可回收品，新装备直接回收，不产生假满包或入包样本。
	r = out
	incoming = recycleItem(193, false)
	q.SourceSeq = 6
	q.Item = &incoming
	q.Experience = 0
	q.Gold = 0
	q.HostSeconds = 0
	out, receipt, err = applySettlement(r, q, f)
	if err != nil || len(out.Items) != 1 || out.PickupStopped || out.Wallet.Gold != 430 || out.Report.RecycleCount != 2 || out.Report.TotalDropCount != 2 || len(out.Report.Drops) != 1 || receipt.GrantedItem {
		t.Fatal(out, receipt, err)
	}
}

func TestSettlementFullBagDiscardsOnlyIncomingAndRecordsExactReason(t *testing.T) {
	f, r := recycleFixture(t), recycleRole()
	r.Capacity = 1
	item := recycleItem(218, false)
	item.Locked = true
	r.Items = []domain.Item{item}
	incoming := recycleItem(35, true)
	out, receipt, err := applySettlement(r, Settlement{SourceSeq: 5, Kills: 1, HostingSession: "session", Item: &incoming}, f)
	if err != nil || !out.PickupStopped || len(out.Items) != 1 || out.Items[0].ID != item.ID || receipt.Recycling.DiscardReason != "bag_full" || out.Report.DiscardCount != 1 || out.Report.TotalDropCount != 1 || len(out.Report.Drops) != 0 || out.Report.RecycleCount != 0 || out.Report.StopReason != 0 {
		t.Fatal(out, receipt, err)
	}
}

func TestSettlementReportSamplesBoundedButTotalIsNotSampleLength(t *testing.T) {
	f, r := recycleFixture(t), recycleRole()
	r.AutoRecycle.Enabled = false
	r.Capacity = 200
	r.Report = domain.Report{SessionID: "session", Drops: make([]domain.Item, 100), TotalDropCount: 143}
	item := recycleItem(218, false)
	out, _, err := applySettlement(r, Settlement{SourceSeq: 5, Kills: 1, HostingSession: "session", Item: &item}, f)
	if err != nil || len(out.Report.Drops) != 100 || out.Report.TotalDropCount != 144 {
		t.Fatal(out.Report, err)
	}
	r = out
	q := Settlement{SourceSeq: 6, Kills: 1, Item: &item}
	out, _, err = applySettlement(r, q, f)
	if err != nil || !reflect.DeepEqual(out.Report, r.Report) {
		t.Fatal("在线事件污染旧托管战报", err)
	}
}

func TestRecyclingOverflowRollsBackAllOriginalItemsAndReport(t *testing.T) {
	f, r := recycleFixture(t), recycleRole()
	r.Items = []domain.Item{recycleItem(218, false), recycleItem(193, true)}
	r.Wallet.Gold = math.MaxInt64 - 49
	before := append([]domain.Item(nil), r.Items...)
	out, _, err := applySettlement(r, Settlement{SourceSeq: 5, Kills: 1}, f)
	if err == nil || !reflect.DeepEqual(out, r) || !reflect.DeepEqual(r.Items, before) {
		t.Fatal("溢出部分提交", out, err)
	}
	r.Wallet.Gold = 0
	r.Report = domain.Report{SessionID: "session", RecycleCount: math.MaxInt64}
	out, _, err = applySettlement(r, Settlement{SourceSeq: 5, Kills: 1, HostingSession: "session"}, f)
	if err == nil || !reflect.DeepEqual(out, r) {
		t.Fatal("战报溢出未回滚", out, err)
	}
}

func TestHostingOnlyChargeDoesNotApplyFuturePolicyToBag(t *testing.T) {
	f, r := recycleFixture(t), recycleRole()
	r.Items = []domain.Item{recycleItem(218, false)}
	out, receipt, err := applySettlement(r, Settlement{SourceSeq: 5, HostSeconds: 1, HostingSession: "session"}, f)
	if err != nil || len(out.Items) != 1 || out.Wallet != r.Wallet || len(receipt.Recycling.Recycled) != 0 || out.Report.TotalDropCount != 0 {
		t.Fatal(out, receipt, err)
	}
}

func TestAutoRecycleReportMayRecycleExistingBagMoreThanNewDropTotal(t *testing.T) {
	f, r := recycleFixture(t), recycleRole()
	r.Items = []domain.Item{recycleItem(218, false), recycleItem(193, true)}
	item := recycleItem(35, false)
	out, _, err := applySettlement(r, Settlement{SourceSeq: 5, Kills: 1, Item: &item, HostingSession: "session"}, f)
	if err != nil || out.Report.RecycleCount != 2 || out.Report.TotalDropCount != 1 || len(out.Report.Drops) != 1 || out.Report.DiscardCount != 0 {
		t.Fatal("不能用新掉物数限制现存背包回收数", out.Report, err)
	}
}

func TestAutoRecycleReceiptsFreezeActualPolicyAndFailureHasNoSnapshot(t *testing.T) {
	f, r := recycleFixture(t), recycleRole()
	r.AutoRecycle.ProtectedItemIDs = []uint32{193}
	_, details, err := applySettlement(r, Settlement{SourceSeq: 5, Kills: 1}, f)
	if err != nil || details.Recycling.Policy == nil || details.Recycling.Policy.Revision != 2 {
		t.Fatal(details, err)
	}
	r.AutoRecycle.QualityIDs[0] = 4
	r.AutoRecycle.ProtectedItemIDs[0] = 218
	if details.Recycling.Policy.QualityIDs[0] != 0 || details.Recycling.Policy.ProtectedItemIDs[0] != 193 {
		t.Fatal("后续策略修改污染原来源快照")
	}
	request := &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0, 2}, ProtectedItemIds: []uint32{193}, Revision: 2}
	out, receipt, code, err := setAutoRecycle(r, AssetRequest{AutoRecycle: request}, f)
	if err != nil || code != 0 || receipt == nil || receipt.Policy == nil || receipt.Policy.Revision != 3 {
		t.Fatal(receipt, code, err)
	}
	request.QualityIds[0] = 1
	request.ProtectedItemIds[0] = 218
	out.AutoRecycle.QualityIDs[0] = 4
	out.AutoRecycle.ProtectedItemIDs[0] = 218
	if receipt.Policy.QualityIDs[0] != 0 || receipt.Policy.ProtectedItemIDs[0] != 193 {
		t.Fatal("受理请求/返回Role切片修改污染原设置快照")
	}
	request.Revision = 1
	_, failed, code, err := setAutoRecycle(r, AssetRequest{AutoRecycle: request}, f)
	if err != nil || code == 0 || failed != nil {
		t.Fatal("失败策略生成成功快照", failed, code, err)
	}
}
