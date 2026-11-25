// 本文件定义普通怪高品质装备配额、事件时间窗及耗尽时的降级配置。
package devconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

// OrdinaryMonsterDropSource 是普通怪在配额账本中的来源，在线与离线托管共用。
const OrdinaryMonsterDropSource = "ordinary_monster"

const (
	// 上海运营日以零时为锚，固定秒数窗口不依赖设备时区或进程启停时间。
	shanghaiWindowOffsetSeconds = int64(8 * 60 * 60)
	// 窗长须能以 Go 时长安全表示。
	maxDropQuotaWindowSeconds = math.MaxInt64 / int64(time.Second)
)

// OrdinaryEquipmentQuota 对应工作簿中一种来源、装备阶段和品质的明确额度。
type OrdinaryEquipmentQuota struct {
	ID                        uint32            `json:"id"`                          // 工作簿配置编号。
	Stage                     uint32            `json:"stage"`                       // 按装备自身需求等级定义的阶段。
	Quality                   uint32            `json:"quality"`                     // 使用本额度的稀有或史诗品质。
	SourceKind                string            `json:"source_kind"`                 // 产出来源分账键。
	WindowSeconds             int64             `json:"window_seconds"`              // 稳定时间窗长度，单位秒。
	QuotaUnits                int64             `json:"quota_units"`                 // 当前时间窗可分批发放的总件数。
	RefillSchedule            []DropQuotaRefill `json:"refill_schedule"`             // 窗内分批释放的秒偏移与件数。
	ExhaustedDowngradeWeights map[uint32]int64  `json:"exhausted_downgrade_weights"` // 耗尽时能够选择的低品质和相对权重。
}

// DropQuotaRefill 固定相对窗口起点的秒偏移及该偏移到达后释放的件数。
type DropQuotaRefill struct {
	OffsetSeconds int64 `json:"offset_seconds"` // 从窗口起点计算的秒偏移，可显式为零。
	Units         int64 `json:"units"`          // 本批释放件数，必须为正。
}

// UnmarshalJSON 区分合法的编号0、阶段0以及缺项。
func (q *OrdinaryEquipmentQuota) UnmarshalJSON(data []byte) error {
	type plain OrdinaryEquipmentQuota
	var decoded struct {
		plain
		ID    *uint32 `json:"id"`
		Stage *uint32 `json:"stage"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.ID == nil || decoded.Stage == nil {
		return errors.New("高品质额度须明确填写配置编号与装备阶段")
	}
	decoded.plain.ID, decoded.plain.Stage = *decoded.ID, *decoded.Stage
	*q = OrdinaryEquipmentQuota(decoded.plain)
	return nil
}

// UnmarshalJSON 确保漏填分时偏移不会把额度提前释放到窗首。
func (r *DropQuotaRefill) UnmarshalJSON(data []byte) error {
	var fields struct {
		OffsetSeconds *int64 `json:"offset_seconds"`
		Units         int64  `json:"units"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields.OffsetSeconds == nil {
		return errors.New("高品质额度分批释放须明确填写秒偏移")
	}
	*r = DropQuotaRefill{OffsetSeconds: *fields.OffsetSeconds, Units: fields.Units}
	return nil
}

// OrdinaryEquipmentQuota 按装备阶段和品质返回普通怪的明确配额行。
func (f *Fixture) OrdinaryEquipmentQuota(stage, quality uint32) (*OrdinaryEquipmentQuota, bool) {
	for i := range f.OrdinaryEquipmentQuotas {
		row := &f.OrdinaryEquipmentQuotas[i]
		if row.Stage == stage && row.Quality == quality && row.SourceKind == OrdinaryMonsterDropSource {
			return row, true
		}
	}
	return nil, false
}

// OrdinaryEquipmentCandidates 按池中的原始顺序取同阶段同品质的装备条目。
func (f *Fixture) OrdinaryEquipmentCandidates(pool OrdinaryDropPool, stage, quality uint32) []OrdinaryDropEntry {
	var candidates []OrdinaryDropEntry
	for _, entry := range pool.Equipment {
		item, ok := f.Item(entry.ItemID)
		if ok && item.IsEquipment && item.EquipmentStage == stage && item.DropQuality == quality {
			candidates = append(candidates, entry)
		}
	}
	return candidates
}

// Window 用击杀事件时刻确定含起点、不含终点的时间窗及已释放的累计件数。
func (q OrdinaryEquipmentQuota) Window(at time.Time) (starts, ends time.Time, released int64) {
	if at.IsZero() || !q.validWindow() {
		return time.Time{}, time.Time{}, 0
	}
	second := at.Unix()
	elapsed := (second%q.WindowSeconds + shanghaiWindowOffsetSeconds) % q.WindowSeconds
	if elapsed < 0 {
		elapsed += q.WindowSeconds
	}
	starts = time.Unix(second-elapsed, 0).UTC()
	ends = starts.Add(time.Duration(q.WindowSeconds) * time.Second)
	for _, row := range q.RefillSchedule {
		if row.OffsetSeconds > elapsed {
			break
		}
		released += row.Units
	}
	return starts, ends, released
}

func (q OrdinaryEquipmentQuota) validWindow() bool {
	return q.WindowSeconds > 0 && q.WindowSeconds <= maxDropQuotaWindowSeconds
}

// ordinaryEquipmentQuotaKey 用在配置校验中，当前只允许普通怪来源。
type ordinaryEquipmentQuotaKey struct {
	stage   uint32 // 装备需求等级所属阶段。
	quality uint32 // 装备固定品质。
}

// validateOrdinaryEquipmentQuotas 校验显式行并创建待核对是否被装备池使用的索引。
func (f *Fixture) validateOrdinaryEquipmentQuotas() (map[ordinaryEquipmentQuotaKey]bool, error) {
	ids := make(map[uint32]bool, len(f.OrdinaryEquipmentQuotas))
	used := make(map[ordinaryEquipmentQuotaKey]bool, len(f.OrdinaryEquipmentQuotas))
	for _, row := range f.OrdinaryEquipmentQuotas {
		key := ordinaryEquipmentQuotaKey{stage: row.Stage, quality: row.Quality}
		if _, duplicate := used[key]; duplicate || ids[row.ID] {
			return nil, fmt.Errorf("高品质额度%d的编号或阶段品质组合重复", row.ID)
		}
		if row.SourceKind != OrdinaryMonsterDropSource || row.Quality < 2 || row.Quality > 3 {
			return nil, fmt.Errorf("高品质额度%d只支持普通怪的稀有或史诗装备", row.ID)
		}
		if err := row.validateRefills(); err != nil {
			return nil, err
		}
		if err := row.validateDowngradeWeights(); err != nil {
			return nil, err
		}
		ids[row.ID], used[key] = true, false
	}
	return used, nil
}

// validateRefills 保证所有件数由有序且完整的窗内释放规则解释。
func (q OrdinaryEquipmentQuota) validateRefills() error {
	if !q.validWindow() || q.QuotaUnits <= 0 || len(q.RefillSchedule) == 0 {
		return fmt.Errorf("高品质额度%d须配置正窗口、正额度及明确分时释放", q.ID)
	}
	previous, total := int64(-1), int64(0)
	for _, row := range q.RefillSchedule {
		if row.OffsetSeconds <= previous || row.OffsetSeconds >= q.WindowSeconds || row.Units <= 0 || row.Units > q.QuotaUnits-total {
			return fmt.Errorf("高品质额度%d的分时释放偏移或件数无效", q.ID)
		}
		previous, total = row.OffsetSeconds, total+row.Units
	}
	if total != q.QuotaUnits || (len(q.RefillSchedule) == 1 && q.RefillSchedule[0].OffsetSeconds == 0) {
		return fmt.Errorf("高品质额度%d须完整分时释放，窗首不能一次给足", q.ID)
	}
	return nil
}

// validateDowngradeWeights 校验每次耗尽只允许落到更低品质，且权重和可用于一次整数抽样。
func (q OrdinaryEquipmentQuota) validateDowngradeWeights() error {
	var total int64
	for quality, weight := range q.ExhaustedDowngradeWeights {
		if quality >= q.Quality || weight <= 0 || total > math.MaxInt64-weight {
			return fmt.Errorf("高品质额度%d的降级品质或权重无效", q.ID)
		}
		total += weight
	}
	if total == 0 {
		return fmt.Errorf("高品质额度%d缺少明确降级权重", q.ID)
	}
	return nil
}

// validateOrdinaryPoolQuotaLinks 逐池查找高档配额及每种降档对应的同阶段装备。
func (f *Fixture) validateOrdinaryPoolQuotaLinks(pool OrdinaryDropPool, used map[ordinaryEquipmentQuotaKey]bool) error {
	available := make(map[ordinaryEquipmentQuotaKey]bool, len(pool.Equipment))
	for _, row := range pool.Equipment {
		item, _ := f.Item(row.ItemID) // 装备分类校验已核对每个引用。
		available[ordinaryEquipmentQuotaKey{stage: item.EquipmentStage, quality: item.DropQuality}] = true
	}
	for key := range available {
		if key.quality < 2 {
			continue
		}
		quota, ok := f.OrdinaryEquipmentQuota(key.stage, key.quality)
		if !ok {
			return fmt.Errorf("普通掉落池%d阶段%d品质%d缺少明确额度", pool.ID, key.stage, key.quality)
		}
		for quality := range quota.ExhaustedDowngradeWeights {
			if !available[ordinaryEquipmentQuotaKey{stage: key.stage, quality: quality}] {
				return fmt.Errorf("普通掉落池%d阶段%d缺少额度%d要求的降档品质%d装备", pool.ID, key.stage, quota.ID, quality)
			}
		}
		used[key] = true
	}
	return nil
}
