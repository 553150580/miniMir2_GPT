package devconfig

import (
	"encoding/json"
	"testing"
)

func TestEquipmentRecyclingRequiresExplicitGoldOnlyContract(t *testing.T) {
	for _, raw := range []string{
		`{}`, `{ "equipment_recycling": null }`,
		`{ "equipment_recycling": {"mode":"development_gold_only"} }`,
		`{ "equipment_recycling": {"mode":"development_gold_only","outputs":null} }`,
		`{ "equipment_recycling": {"mode":"formal","outputs":[]} }`,
		`{ "equipment_recycling": {"mode":"development_gold_only","outputs":[{"item_id":1}]} }`,
	} {
		var f Fixture
		if err := json.Unmarshal([]byte(raw), &f); err != nil {
			t.Fatal(err)
		}
		f.RuleVersion = 19
		if err := f.ValidateEquipmentRecycling(); err == nil {
			t.Fatalf("缺失或伪造配方被接受: %s", raw)
		}
	}
	var f Fixture
	if err := json.Unmarshal([]byte(`{"equipment_recycling":{"mode":"development_gold_only","outputs":[]},"items":[{"id":218,"is_equipment":true,"required_level":1,"recycle_gold":50,"drop_quality":0}]}`), &f); err != nil {
		t.Fatal(err)
	}
	f.RuleVersion = 19
	if err := f.ValidateEquipmentRecycling(); err != nil {
		t.Fatal(err)
	}
	if f.Items[0].RequiredLevel == nil || *f.Items[0].RequiredLevel != 1 {
		t.Fatal("来源需求等级未保留")
	}
	f.Items[0].RequiredLevel = nil
	if err := f.ValidateEquipmentRecycling(); err == nil {
		t.Fatal("缺需求等级被当成0级")
	}
	f.Items = nil
	f.RuleVersion = 18
	if err := f.ValidateEquipmentRecycling(); err == nil {
		t.Fatal("新回收合同混入旧规则版本")
	}
}

func TestEquipmentRecycleZeroValuesAreNotMissingSource(t *testing.T) {
	for _, raw := range []string{
		`{"is_equipment":true,"required_level":1,"drop_quality":0}`,
		`{"is_equipment":true,"required_level":1,"drop_quality":0,"recycle_gold":null}`,
		`{"is_equipment":true,"required_level":1,"recycle_gold":0}`,
		`{"is_equipment":true,"required_level":1,"recycle_gold":0,"drop_quality":null}`,
		`{"is_equipment":true,"required_level":1,"recycle_gold":0,"drop_quality":5}`,
		`{"is_equipment":true,"required_level":0,"recycle_gold":0,"drop_quality":0}`,
	} {
		var item Item
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			t.Fatal(err)
		}
		if item.EquipmentRecycleKnown() {
			t.Fatalf("缺失来源被默认为0: %s", raw)
		}
	}
	var item Item
	if err := json.Unmarshal([]byte(`{"is_equipment":true,"required_level":1,"recycle_gold":0,"drop_quality":0}`), &item); err != nil {
		t.Fatal(err)
	}
	if !item.EquipmentRecycleKnown() {
		t.Fatal("明确的零金币普通装备被当成未知")
	}
}
