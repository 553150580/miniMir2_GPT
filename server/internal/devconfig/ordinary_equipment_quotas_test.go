// 本文件核对普通怪高品质装备的工作簿字段、稳定时间窗和逐池降级合同。
package devconfig

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestOrdinaryEquipmentQuotaJSONRequiresExplicitZeroFields(t *testing.T) {
	// 编号0、T1阶段0及首批秒偏移0都是合法值，省略或空值则无对应授权。
	raw := `{"id":0,"stage":0,"quality":2,"source_kind":"ordinary_monster","window_seconds":120,"quota_units":2,` +
		`"refill_schedule":[{"offset_seconds":0,"units":1},{"offset_seconds":60,"units":1}],"exhausted_downgrade_weights":{"0":1}}`
	var row OrdinaryEquipmentQuota
	if err := json.Unmarshal([]byte(raw), &row); err != nil {
		t.Fatal(err)
	}
	if row.ID != 0 || row.Stage != 0 || row.Quality != 2 || row.SourceKind != OrdinaryMonsterDropSource ||
		row.WindowSeconds != 120 || row.QuotaUnits != 2 || len(row.RefillSchedule) != 2 || row.RefillSchedule[0].OffsetSeconds != 0 ||
		row.ExhaustedDowngradeWeights[0] != 1 {
		t.Fatalf("工作簿投影字段读回错误：%+v", row)
	}
	for _, broken := range []string{
		strings.Replace(raw, `"id":0,`, "", 1),
		strings.Replace(raw, `"stage":0,`, "", 1),
		strings.Replace(raw, `"stage":0`, `"stage":null`, 1),
		strings.Replace(raw, `"offset_seconds":0,`, "", 1),
		strings.Replace(raw, `"offset_seconds":0`, `"offset_seconds":null`, 1),
	} {
		if err := json.Unmarshal([]byte(broken), &row); err == nil {
			t.Fatalf("缺失字段被错误理解成显式零值：%s", broken)
		}
	}
}

func TestOrdinaryEquipmentQuotaWindowUsesKillTimeAndShanghaiAnchor(t *testing.T) {
	row := OrdinaryEquipmentQuota{WindowSeconds: 86400, QuotaUnits: 3,
		RefillSchedule: []DropQuotaRefill{{OffsetSeconds: 1, Units: 1}, {OffsetSeconds: 60, Units: 2}}}
	start := time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC) // 上海25日零时。
	for _, check := range []struct {
		elapsed, startShift time.Duration
		released            int64
	}{
		{-time.Nanosecond, -24 * time.Hour, 3},
		{0, 0, 0},
		{time.Second - time.Nanosecond, 0, 0},
		{time.Second, 0, 1},
		{time.Minute - time.Nanosecond, 0, 1},
		{time.Minute, 0, 3},
		{24*time.Hour - time.Nanosecond, 0, 3},
		{24 * time.Hour, 24 * time.Hour, 0},
	} {
		at := start.Add(check.elapsed).In(time.FixedZone("验收远端时区", -5*60*60))
		left, right, released := row.Window(at)
		want := start.Add(check.startShift)
		if !left.Equal(want) || !right.Equal(want.Add(24*time.Hour)) || released != check.released {
			t.Fatalf("事件%s的窗口或累计发放错误：%s 至%s，释放%d", at, left, right, released)
		}
	}
	if start, end, units := row.Window(time.Time{}); !start.IsZero() || !end.IsZero() || units != 0 {
		t.Fatal("缺少击杀时间却选择了有效配额窗")
	}
}

func TestOrdinaryEquipmentQuotaValidatesEveryPoolAndDowngradeTier(t *testing.T) {
	valid := func() Fixture {
		const stage = 1
		f := Fixture{RuleVersion: 30, DamageFormula: "development_source_combat",
			Items: []Item{{ID: 35},
				{ID: 110, IsEquipment: true, EquipmentStage: stage},
				{ID: 111, IsEquipment: true, EquipmentStage: stage, DropQuality: 1},
				{ID: 112, IsEquipment: true, EquipmentStage: stage, DropQuality: 2},
				{ID: 113, IsEquipment: true, EquipmentStage: stage, DropQuality: 3}},
			Monsters: []Monster{{ID: 1, DropPoolID: 11}},
			OrdinaryDropPools: []OrdinaryDropPool{{ID: 11,
				Items: []OrdinaryDropEntry{{ItemID: 35, Weight: 1, QuantityMin: 1, QuantityMax: 1}},
				Equipment: []OrdinaryDropEntry{{ItemID: 110, Weight: 1, QuantityMin: 1, QuantityMax: 1},
					{ItemID: 111, Weight: 1, QuantityMin: 1, QuantityMax: 1},
					{ItemID: 112, Weight: 1, QuantityMin: 1, QuantityMax: 1},
					{ItemID: 113, Weight: 1, QuantityMin: 1, QuantityMax: 1}}}},
			OrdinaryEquipmentQuotas: []OrdinaryEquipmentQuota{
				{ID: 1, Stage: stage, Quality: 2, SourceKind: OrdinaryMonsterDropSource, WindowSeconds: 120, QuotaUnits: 2,
					RefillSchedule:            []DropQuotaRefill{{OffsetSeconds: 0, Units: 1}, {OffsetSeconds: 60, Units: 1}},
					ExhaustedDowngradeWeights: map[uint32]int64{0: 1, 1: 2}},
				{ID: 2, Stage: stage, Quality: 3, SourceKind: OrdinaryMonsterDropSource, WindowSeconds: 120, QuotaUnits: 1,
					RefillSchedule: []DropQuotaRefill{{OffsetSeconds: 30, Units: 1}}, ExhaustedDowngradeWeights: map[uint32]int64{2: 1}},
			},
		}
		f.CombatTiming.PlayerAttackActionMS = 1
		return f
	}
	f := valid()
	if err := f.validateOrdinaryDropPools(); err != nil {
		t.Fatal(err)
	}
	if row, ok := f.OrdinaryEquipmentQuota(1, 3); !ok || row.ID != 2 {
		t.Fatalf("史诗额度查询错误：%+v", row)
	}
	if rows := f.OrdinaryEquipmentCandidates(f.OrdinaryDropPools[0], 1, 2); len(rows) != 1 || rows[0].ItemID != 112 {
		t.Fatalf("装备候选筛选错误：%+v", rows)
	}
	for _, check := range []struct {
		name   string
		change func(*Fixture)
	}{
		{"阶段品质重复", func(f *Fixture) { f.OrdinaryEquipmentQuotas[1].Quality = 2 }},
		{"配置编号重复", func(f *Fixture) { f.OrdinaryEquipmentQuotas[1].ID = 1 }},
		{"来源不符", func(f *Fixture) { f.OrdinaryEquipmentQuotas[0].SourceKind = "elite_monster" }},
		{"传说不得随机", func(f *Fixture) { f.Items[4].DropQuality = 4 }},
		{"窗口为零", func(f *Fixture) { f.OrdinaryEquipmentQuotas[0].WindowSeconds = 0 }},
		{"窗长不能转换为时长", func(f *Fixture) { f.OrdinaryEquipmentQuotas[0].WindowSeconds = math.MaxInt64 }},
		{"窗外发放", func(f *Fixture) { f.OrdinaryEquipmentQuotas[0].RefillSchedule[1].OffsetSeconds = 120 }},
		{"重复发放偏移", func(f *Fixture) { f.OrdinaryEquipmentQuotas[0].RefillSchedule[1].OffsetSeconds = 0 }},
		{"窗口额度透支", func(f *Fixture) { f.OrdinaryEquipmentQuotas[0].RefillSchedule[1].Units = 2 }},
		{"没有完整分配", func(f *Fixture) { f.OrdinaryEquipmentQuotas[0].QuotaUnits = 3 }},
		{"窗口一开直接给足", func(f *Fixture) { f.OrdinaryEquipmentQuotas[1].RefillSchedule[0].OffsetSeconds = 0 }},
		{"不允许升档", func(f *Fixture) { f.OrdinaryEquipmentQuotas[0].ExhaustedDowngradeWeights[3] = 1 }},
		{"降级权重必须为正", func(f *Fixture) { f.OrdinaryEquipmentQuotas[0].ExhaustedDowngradeWeights[0] = 0 }},
		{"降级权重不能溢出", func(f *Fixture) { f.OrdinaryEquipmentQuotas[0].ExhaustedDowngradeWeights[0] = math.MaxInt64 }},
		{"稀有降档同池同阶段", func(f *Fixture) { f.Items[1].EquipmentStage = 0 }},
		{"史诗降档链要求稀有额度", func(f *Fixture) { f.OrdinaryEquipmentQuotas = f.OrdinaryEquipmentQuotas[1:] }},
		{"稀有满额需有终止档", func(f *Fixture) { f.OrdinaryEquipmentQuotas[0].ExhaustedDowngradeWeights = nil }},
		{"普通池无高档却配置额度", func(f *Fixture) {
			f.OrdinaryDropPools[0].Equipment = f.OrdinaryDropPools[0].Equipment[:2]
		}},
		{"第二池不能借用第一池降档", func(f *Fixture) {
			pool := f.OrdinaryDropPools[0]
			f.OrdinaryDropPools = append(f.OrdinaryDropPools, OrdinaryDropPool{ID: 12, Items: pool.Items, Equipment: pool.Equipment[2:]})
			f.Monsters = append(f.Monsters, Monster{ID: 2, DropPoolID: 12})
		}},
	} {
		t.Run(check.name, func(t *testing.T) {
			f := valid()
			check.change(&f)
			if err := f.validateOrdinaryDropPools(); err == nil {
				t.Fatal("错误的普通装备额度配置被放行")
			}
		})
	}
}
