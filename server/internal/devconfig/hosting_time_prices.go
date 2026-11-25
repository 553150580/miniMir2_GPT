// 本文件限定金币补时的可选价格版本、等级档和整数报价。没有生效版本时不提供报价。
package devconfig

import (
	"fmt"
	"math"
	"slices"
	"time"
)

// MaxHostingTimePurchaseSeconds 是单次金币补时上限，也是购买前余额的排他门槛。
const MaxHostingTimePurchaseSeconds uint32 = 24 * 60 * 60

// maxHostingReferenceGold 使 24 小时全额报价仍可用有符号金币整数表示。
const maxHostingReferenceGold = math.MaxInt64 / (2 * 24)

// HostingTimePriceVersion 保存同一生效窗口中完整的五个等级档。
type HostingTimePriceVersion struct {
	Version             uint64                 `json:"version"`               // 可进入 Mongo 有符号整型的唯一价格版本。
	EffectiveFrom       time.Time              `json:"effective_from"`        // 含起点，使用绝对时间。
	EffectiveUntil      time.Time              `json:"effective_until"`       // 不含终点，零时间表示未设置结束。
	PurchaseStepSeconds uint32                 `json:"purchase_step_seconds"` // 单次允许购买的时长倍数，单位秒。
	Bands               []HostingTimePriceBand `json:"bands"`                 // 同一版本必须有完整且唯一的五档。
}

// HostingTimePriceBand 保存每小时标准毛金币，价格固定为它的两倍。
type HostingTimePriceBand struct {
	LevelMin        uint32 `json:"level_min"`           // 含起点的角色等级。
	LevelMax        uint32 `json:"level_max"`           // 含终点的角色等级，只有 50 级及以上为零。
	GRefGoldPerHour int64  `json:"g_ref_gold_per_hour"` // 未扣补时和药修的标准毛金币每小时整数值。
}

// HostingTimePrice 是一次查价使用的值快照，不引用可变的配置档位切片。
type HostingTimePrice struct {
	Version             uint64               // 本次使用的正式价格版本。
	EffectiveFrom       time.Time            // 本价格版本的含起点生效时刻。
	EffectiveUntil      time.Time            // 本价格版本的不含终点时刻。
	PurchaseStepSeconds uint32               // 本价格版本允许的购买粒度。
	Band                HostingTimePriceBand // 本角色等级对应的唯一标准毛金币档。
}

// HostingTimePrice 根据服务端时刻和角色等级找到唯一当前价格。在版本间隙返回不可报价。
func (f *Fixture) HostingTimePrice(level uint32, at time.Time) (HostingTimePrice, bool) {
	if f == nil || level == 0 || at.IsZero() {
		return HostingTimePrice{}, false
	}
	for _, version := range f.HostingTimePrices {
		if at.Before(version.EffectiveFrom) || !version.EffectiveUntil.IsZero() && !at.Before(version.EffectiveUntil) {
			continue
		}
		for _, band := range version.Bands {
			if level >= band.LevelMin && (band.LevelMax == 0 || level <= band.LevelMax) {
				return HostingTimePrice{Version: version.Version, EffectiveFrom: version.EffectiveFrom,
					EffectiveUntil: version.EffectiveUntil, PurchaseStepSeconds: version.PurchaseStepSeconds, Band: band}, true
			}
		}
	}
	return HostingTimePrice{}, false
}

// GoldPerHour 返回展示与收据使用的每小时单价。无效标准毛金币返回零。
func (p HostingTimePrice) GoldPerHour() int64 {
	if gold := p.Band.GRefGoldPerHour; gold > 0 && gold <= maxHostingReferenceGold {
		return 2 * gold
	}
	return 0
}

// Cost 对整笔购买仅向上取整一次。商余拆分避免先将小时单价乘总秒数而溢出。
func (p HostingTimePrice) Cost(seconds uint32) (int64, bool) {
	rate, step := p.GoldPerHour(), p.PurchaseStepSeconds
	if rate == 0 || step == 0 || seconds == 0 || seconds > MaxHostingTimePurchaseSeconds || seconds%step != 0 {
		return 0, false
	}
	const hour = int64(3600)
	duration := int64(seconds)
	return (rate/hour)*duration + ((rate%hour)*duration+hour-1)/hour, true
}

// validateHostingTimePrices 拒绝不完整分档、不可表示的价格及重叠的半开版本窗口。
func (f *Fixture) validateHostingTimePrices() error {
	versions := slices.Clone(f.HostingTimePrices)
	slices.SortFunc(versions, func(a, b HostingTimePriceVersion) int { return a.EffectiveFrom.Compare(b.EffectiveFrom) })
	seen := make(map[uint64]bool, len(versions))
	for i, version := range versions {
		if version.Version == 0 || version.Version > math.MaxInt64 || seen[version.Version] {
			return fmt.Errorf("托管补时价格版本 %d 无效或重复", version.Version)
		}
		seen[version.Version] = true
		if version.EffectiveFrom.IsZero() || !version.EffectiveUntil.IsZero() && !version.EffectiveUntil.After(version.EffectiveFrom) {
			return fmt.Errorf("托管补时价格版本 %d 的生效窗无效", version.Version)
		}
		if i > 0 && (versions[i-1].EffectiveUntil.IsZero() || version.EffectiveFrom.Before(versions[i-1].EffectiveUntil)) {
			return fmt.Errorf("托管补时价格版本 %d 的生效窗重叠", version.Version)
		}
		if version.PurchaseStepSeconds == 0 || version.PurchaseStepSeconds > MaxHostingTimePurchaseSeconds {
			return fmt.Errorf("托管补时价格版本 %d 的购买粒度无效", version.Version)
		}
		if err := validateHostingTimePriceBands(version.Bands); err != nil {
			return fmt.Errorf("托管补时价格版本 %d：%w", version.Version, err)
		}
	}
	return nil
}

// validateHostingTimePriceBands 核对策划分档并约束可安全表示的最高毛金。
func validateHostingTimePriceBands(bands []HostingTimePriceBand) error {
	ends := map[uint32]uint32{1: 19, 20: 29, 30: 39, 40: 49, 50: 0}
	if len(bands) != len(ends) {
		return fmt.Errorf("必须配置完整的五个补时等级档")
	}
	for _, band := range bands {
		end, ok := ends[band.LevelMin]
		if !ok || band.LevelMax != end {
			return fmt.Errorf("托管补时等级档 %d–%d 无效或重复", band.LevelMin, band.LevelMax)
		}
		delete(ends, band.LevelMin)
		if band.GRefGoldPerHour <= 0 || band.GRefGoldPerHour > maxHostingReferenceGold {
			return fmt.Errorf("托管补时等级档 %d 的每小时标准毛金币无效", band.LevelMin)
		}
	}
	return nil
}
