// 本文件核对自动回收设置的持久化模型和客户端位标志边界。

package domain

import (
	"testing"
)

// TestInventoryProjectsStrongProtectionAndPermanentPolicyWithoutAliasing 核对「协议切片污染永久角色策略」与「旧角色应明确投影关闭策略」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestInventoryProjectsStrongProtectionAndPermanentPolicyWithoutAliasing(t *testing.T) {
	// item 组装道具所需的数据。
	item := Item{ID: ID(), ItemID: 218, Quantity: 1, Version: 3, Locked: true, PendingResultID: "pending", ReservationID: "reservation"}
	// r 组装角色所需的数据。
	r := Role{Capacity: 32, Revision: 7, Items: []Item{item}, Equipped: []Equipped{{Slot: 0, Item: item}}, AutoRecycle: AutoRecyclePolicy{Enabled: true, QualityIDs: []uint32{0, 4}, ProtectedItemIDs: []uint32{193}, Revision: 2}}
	// v 接收背包资产供后续步骤使用。
	v := r.Inventory()
	if len(v.Items) != 1 || v.Items[0].RecycleBlockFlags != 15 || !v.Items[0].Locked || v.Equipped[0].Item.RecycleBlockFlags != 15 || v.AutoRecycle.Revision != 2 {
		t.Fatal(v)
	}
	v.AutoRecycle.QualityIds[0] = 1
	v.AutoRecycle.ProtectedItemIds[0] = 218
	if r.AutoRecycle.QualityIDs[0] != 0 || r.AutoRecycle.ProtectedItemIDs[0] != 193 {
		t.Fatal("协议切片污染永久角色策略")
	}
	// legacy 保存自动回收，用于当前步骤中的兼容旧数据。
	legacy := (Role{}).Inventory().AutoRecycle
	if legacy == nil || legacy.Enabled || legacy.Revision != 0 {
		t.Fatal("旧角色应明确投影关闭策略")
	}
}

// TestHostingReportProjectsTotalsSeparateFromBoundedDropSample 验证托管战报分别投影总掉落数、受限的掉落样本、回收货币及丢弃原因。
// t 提供断言失败报告、子用例和清理注册。
func TestHostingReportProjectsTotalsSeparateFromBoundedDropSample(t *testing.T) {
	// r 组装角色所需的数据。
	r := Role{Report: Report{Sequence: 9, Drops: make([]Item, 100), RecycleCount: 5, RecycleGold: 70, RecycleBoundGold: 80, DiscardCount: 3, LastDiscardReason: "bag_full", TotalDropCount: 180}}
	// x 接收离线托管报告或战报供后续步骤使用。
	x := r.HostingReport()
	if x.RecycleCount != 5 || x.RecycleGold != 70 || x.RecycleBoundGold != 80 || x.DiscardCount != 3 || x.LastDiscardReason != "bag_full" || x.TotalDropCount != 180 || len(x.Drops) != 100 {
		t.Fatal(x)
	}
}
