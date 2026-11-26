// 本文件核对旧装备编码兼容及已付候选仍使用原占用与词条字段。
package domain

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestEquipmentRefineItemBSONPreservesLegacyAndPaidState 防止零值词条改写历史装备和真实待选丢失。
func TestEquipmentRefineItemBSONPreservesLegacyAndPaidState(t *testing.T) {
	item := Item{ID: ID(), ItemID: 220, Quality: 1, Quantity: 1, Version: 1}
	raw, err := bson.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := bson.Raw(raw).LookupErr("refine"); present == nil || item.Proto().GetRefinePending() != nil {
		t.Fatal("历史空词条的 BSON 和协议应保持缺省")
	}
	item.Refine.Current[0] = EquipmentAffix{AttributeID: 3, Tier: 1, ValueUnits: 12}
	item.Refine.Candidate[0] = EquipmentAffix{AttributeID: 3, Tier: 5, ValueUnits: 28}
	item.PendingResultID = ID()
	raw, err = bson.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var restored Item
	if err = bson.Unmarshal(raw, &restored); err != nil || restored != item {
		t.Fatal("原装备的两个词条快照或待选编号往返改变", restored, err)
	}
	view := restored.Proto()
	candidates := view.GetRefinePending().GetAffixes()
	if len(view.RefineAffixes) != 1 || view.RefineAffixes[0].ValueUnits != 12 || len(candidates) != 1 || candidates[0].ValueUnits != 28 ||
		Hex(view.GetRefinePending().GetPendingId()) != item.PendingResultID || view.RecycleBlockFlags&2 == 0 {
		t.Fatal("协议未同时保护原件及显示原左、右词条", view)
	}
}
