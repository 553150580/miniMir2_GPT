// 本文件验证托管卡直接收益的整数边界、持久校验及回收分账。
package persistence

import (
	"math"
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestHostingKillBonusIntegerBounds 核对大数及逐杀取整，禁止收益静默回绕。
func TestHostingKillBonusIntegerBounds(t *testing.T) {
	const safe = math.MaxInt64 / 2 // 此基数乘万分比会越界，最后应发值仍可表示。
	if got, ok := hostingKillExtra(safe, 2000); !ok || got != safe/5 {
		t.Fatalf("直接收益整数加成错误：%d", got)
	}
	for _, entry := range []struct {
		base int64  // 待计算的原收益。
		bp   uint32 // 待核对的万分比。
	}{{math.MaxInt64, 2000}, {-1, 2000}, {1, 10001}} {
		if _, ok := hostingKillExtra(entry.base, entry.bp); ok {
			t.Fatal("无效或超过余额上界的收益被受理")
		}
	}
}

// TestHostingKillBonusSettlementKeepsRecycleSeparate 检查同一来源直接金币、装备回收及战报独立入账。
func TestHostingKillBonusSettlementKeepsRecycleSeparate(t *testing.T) {
	f, r := growthFixture(t), recycleRole()
	f.HostingBenefits = &devconfig.HostingBenefits{KillExperienceBonusBP: 2000, DirectGoldBonusBP: 2000}
	f.Map.ID, f.Map.SceneKind = 1, devconfig.SceneKindPersonal
	f.Monsters = []devconfig.Monster{{ID: 1, Exp: 17, GoldMin: 19, GoldMax: 19}}
	r.ProgressionFormatVersion, r.ProgressionCurveVersion = 1, f.Progression.CurveVersion
	r.Items = []domain.Item{recycleItem(218, false)}
	at := time.Unix(1800000000, 0)
	bonus, err := NewHostingKillBonus(at.Add(time.Second), 17, 19, f.HostingBenefits)
	if err != nil {
		t.Fatal(err)
	}
	q := Settlement{SourceSeq: 5, Kills: 1, MonsterID: 1, MapID: 1, Experience: 20, Gold: 22, Bound: true,
		HostSeconds: 1, HostingSession: "原会话", EventAt: at, NoEquipmentKills: 1, HostingKillBonus: bonus}
	out, detail, err := applySettlement(r, q, f)
	if err != nil || !sameHostingKillBonus(detail.HostingKillBonus, bonus) || out.Wallet.Gold != 50 || out.Wallet.BoundGold != 22 ||
		out.Report.Earned.Gold != 50 || out.Report.Earned.BoundGold != 22 || out.Report.RecycleGold != 50 ||
		out.Report.RawExperience != 20 || detail.Recycling.Gold != 50 || len(out.Items) != 0 || out.HostingSeconds != r.HostingSeconds-1 {
		t.Fatalf("直接金币、回收、经验或扣时分账错误：钱包=%+v、战报=%+v、回执=%+v、错误=%v", out.Wallet, out.Report, detail, err)
	}

	// 变更单个原事实时整笔拒绝，保留原角色资产。
	for _, change := range []func(*Settlement){
		func(bad *Settlement) { bad.Gold++ },
		func(bad *Settlement) { bad.EventAt = bonus.CardUntil },
		func(bad *Settlement) { bad.MonsterID++ },
		func(bad *Settlement) {
			item := recycleItem(218, false)
			bad.Item = &item
		},
	} {
		bad := q
		change(&bad)
		if unchanged, _, err := applySettlement(r, bad, f); Code(err) != p.CodeOperationConflict ||
			unchanged.Wallet != r.Wallet || unchanged.SourceSeq != r.SourceSeq || len(unchanged.Items) != len(r.Items) {
			t.Fatalf("未能完整拒绝改变后的原收益：%+v %v", bad, err)
		}
	}
}
