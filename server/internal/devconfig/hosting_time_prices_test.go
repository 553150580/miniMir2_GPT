// 本文件核对默认关闭、价格版本切换、五档完整性和整秒报价精度。
package devconfig

import (
	"math"
	"math/big"
	"testing"
	"time"
)

// hostingTimeTestVersion 创建独立模拟表，用于价格行为验收，不使用历史候选值。
func hostingTimeTestVersion(version uint64, from, until time.Time) HostingTimePriceVersion {
	return HostingTimePriceVersion{Version: version, EffectiveFrom: from, EffectiveUntil: until, PurchaseStepSeconds: 60,
		Bands: []HostingTimePriceBand{{1, 19, 251}, {20, 29, 352}, {30, 39, 453}, {40, 49, 554}, {50, 0, 655}}}
}

// TestDefaultHostingTimePricesDisabled 从真实开发装配读取规则，确认历史候选没有变成可购买报价。
func TestDefaultHostingTimePricesDisabled(t *testing.T) {
	config, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Fixture.HostingTimePrices) != 0 {
		t.Fatal("默认运行规则出现未经验收的补时价格")
	}
	if _, ok := config.Fixture.HostingTimePrice(7, time.Now()); ok {
		t.Fatal("未发布价格时存在有效购时入口")
	}
	// 用原配置走完整校验，确保格式错误会在节点受理请求之前被拒绝。
	row := hostingTimeTestVersion(1, time.Now(), time.Time{})
	config.Fixture.HostingTimePrices = []HostingTimePriceVersion{row}
	if err = config.Fixture.Validate(); err != nil {
		t.Fatal(err)
	}
	config.Fixture.HostingTimePrices[0].Bands = row.Bands[:4]
	if err = config.Fixture.Validate(); err == nil {
		t.Fatal("节点启动校验没有拒绝缺少价格档")
	}
}

// TestHostingTimePriceWindowsAndBands 逐一确认版本边界、等级边界及查价时的独立值快照。
func TestHostingTimePriceWindowsAndBands(t *testing.T) {
	first := time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	gap := second.Add(time.Hour)
	third := gap.Add(time.Hour)
	fixture := Fixture{HostingTimePrices: []HostingTimePriceVersion{
		hostingTimeTestVersion(99, third, time.Time{}), hostingTimeTestVersion(1, first, second), hostingTimeTestVersion(22, second, gap),
	}}
	if err := fixture.validateHostingTimePrices(); err != nil {
		t.Fatal(err)
	}
	if fixture.HostingTimePrices[0].Version != 99 {
		t.Fatal("校验改变了原发布配置顺序")
	}
	for _, row := range []struct {
		at      time.Time
		version uint64
	}{{time.Time{}, 0}, {first.Add(-time.Nanosecond), 0}, {first, 1}, {second.Add(-time.Nanosecond), 1},
		{second, 22}, {gap, 0}, {third.Add(-time.Nanosecond), 0}, {third, 99}, {third.Add(48 * time.Hour), 99}} {
		quote, ok := fixture.HostingTimePrice(1, row.at)
		if ok != (row.version != 0) || quote.Version != row.version {
			t.Fatalf("时间 %s 查得版本 %d，有效=%v，期望 %d", row.at, quote.Version, ok, row.version)
		}
	}
	for _, row := range []struct {
		level, min, max uint32
	}{{1, 1, 19}, {19, 1, 19}, {20, 20, 29}, {29, 20, 29}, {30, 30, 39}, {39, 30, 39},
		{40, 40, 49}, {49, 40, 49}, {50, 50, 0}, {math.MaxUint32, 50, 0}} {
		quote, ok := fixture.HostingTimePrice(row.level, first)
		if !ok || quote.Band.LevelMin != row.min || quote.Band.LevelMax != row.max || quote.PurchaseStepSeconds != 60 {
			t.Fatalf("角色 %d 级找到的价格档无效：%+v", row.level, quote)
		}
	}
	if _, ok := fixture.HostingTimePrice(0, first); ok {
		t.Fatal("非法零级返回报价")
	}
	locked, ok := fixture.HostingTimePrice(1, first)
	fixture.HostingTimePrices[1].Bands[0].GRefGoldPerHour++
	if !ok || locked.GoldPerHour() != 502 || !locked.EffectiveFrom.Equal(first) || !locked.EffectiveUntil.Equal(second) {
		t.Fatalf("已查价快照被后来的配置存储变化覆盖：%+v", locked)
	}
}

// TestInvalidHostingTimePrices 验证错误配置须整体拒绝，不能退回旧档或只发某些等级。
func TestInvalidHostingTimePrices(t *testing.T) {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	until := from.Add(time.Hour)
	for _, row := range []struct {
		name string
		edit func(*HostingTimePriceVersion)
	}{
		{"零版本", func(v *HostingTimePriceVersion) { v.Version = 0 }},
		{"超出 Mongo 整型", func(v *HostingTimePriceVersion) { v.Version = uint64(math.MaxInt64) + 1 }},
		{"无生效开始", func(v *HostingTimePriceVersion) { v.EffectiveFrom = time.Time{} }},
		{"零长度时间窗", func(v *HostingTimePriceVersion) { v.EffectiveUntil = v.EffectiveFrom }},
		{"倒序时间窗", func(v *HostingTimePriceVersion) { v.EffectiveUntil = from.Add(-time.Hour) }},
		{"零购买粒度", func(v *HostingTimePriceVersion) { v.PurchaseStepSeconds = 0 }},
		{"购买粒度超过24小时", func(v *HostingTimePriceVersion) { v.PurchaseStepSeconds = MaxHostingTimePurchaseSeconds + 1 }},
		{"缺档", func(v *HostingTimePriceVersion) { v.Bands = v.Bands[:4] }},
		{"重档", func(v *HostingTimePriceVersion) { v.Bands[3] = v.Bands[4] }},
		{"前档越界", func(v *HostingTimePriceVersion) { v.Bands[0].LevelMax = 20 }},
		{"末档错误封顶", func(v *HostingTimePriceVersion) { v.Bands[4].LevelMax = 99 }},
		{"毛金为零", func(v *HostingTimePriceVersion) { v.Bands[0].GRefGoldPerHour = 0 }},
		{"毛金为负", func(v *HostingTimePriceVersion) { v.Bands[0].GRefGoldPerHour = -1 }},
		{"一天收费越界", func(v *HostingTimePriceVersion) { v.Bands[0].GRefGoldPerHour = maxHostingReferenceGold + 1 }},
	} {
		t.Run(row.name, func(t *testing.T) {
			version := hostingTimeTestVersion(1, from, until)
			row.edit(&version)
			fixture := Fixture{HostingTimePrices: []HostingTimePriceVersion{version}}
			if err := fixture.validateHostingTimePrices(); err == nil {
				t.Fatal("无效价格版本通过校验")
			}
		})
	}
	for _, row := range []struct {
		first, second HostingTimePriceVersion
	}{
		{hostingTimeTestVersion(1, from, until), hostingTimeTestVersion(1, until, time.Time{})},
		{hostingTimeTestVersion(1, from, until), hostingTimeTestVersion(2, until.Add(-time.Second), time.Time{})},
		{hostingTimeTestVersion(1, from, time.Time{}), hostingTimeTestVersion(2, until, time.Time{})},
	} {
		fixture := Fixture{HostingTimePrices: []HostingTimePriceVersion{row.second, row.first}}
		if err := fixture.validateHostingTimePrices(); err == nil {
			t.Fatal("重复版本或倒序存放的重叠窗口通过校验")
		}
	}
}

// TestHostingTimePriceCost 对拍全天、安全上界和跨小时的非整除整笔取整。
func TestHostingTimePriceCost(t *testing.T) {
	quote := HostingTimePrice{PurchaseStepSeconds: 60, Band: HostingTimePriceBand{GRefGoldPerHour: 251}}
	for _, row := range []struct {
		seconds uint32
		cost    int64
	}{{0, 0}, {1, 0}, {61, 0}, {60, 9}, {120, 17}, {3600, 502}, {3660, 511}, {86400, 12048}, {86401, 0}} {
		fee, valid := quote.Cost(row.seconds)
		if fee != row.cost || valid != (row.cost > 0) {
			t.Fatalf("买 %d 秒得报价 %d，有效=%v，期望 %d", row.seconds, fee, valid, row.cost)
		}
	}
	quote.PurchaseStepSeconds = 1
	quote.Band.GRefGoldPerHour = maxHostingReferenceGold
	for _, seconds := range []uint32{1, 239, 3599, 3600, 86399, 86400} {
		exact := new(big.Int).Mul(big.NewInt(quote.GoldPerHour()), big.NewInt(int64(seconds)))
		exact.Add(exact, big.NewInt(3599)).Quo(exact, big.NewInt(3600))
		fee, valid := quote.Cost(seconds)
		if !valid || !exact.IsInt64() || fee != exact.Int64() {
			t.Fatalf("买 %d 秒的大整数报价错误：%d, %v，期望 %s", seconds, fee, valid, exact)
		}
	}
}
