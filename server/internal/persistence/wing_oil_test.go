// 本文件核对翅膀和武器互斥、当前角色归属、用油阶段和对外报价的形状。
package persistence

import (
	"testing"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// wingOilQuoteRequest 构造一份绑定于原翅膀的用油请求。
func wingOilQuoteRequest(wing domain.WingState, allowBound bool) *pb.WeaponOilQuoteRequest {
	return &pb.WeaponOilQuoteRequest{SceneEpoch: 327, UiSessionId: domain.Bytes(domain.ID()), AllowBound: allowBound,
		Wing: &pb.WingOilRef{WingId: domain.Bytes(wing.ID), Version: uint64(wing.Version)}}
}

// TestWingOilTargetAndTerms 检查原状态核验和武器共用规则的原始报价。
func TestWingOilTargetAndTerms(t *testing.T) {
	fixture := testWeaponOilFixture()
	fixture.Wings = &devconfig.WingConfig{AllowLuckOil: true, MaxRank: 10, AppearanceTiers: []uint32{1, 3, 5, 7, 10}}
	wing := domain.WingState{ID: domain.ID(), OwnerRoleID: domain.ID(), Rank: 5, AppearanceTier: 3, Hidden: true,
		Luck: 5, OilToSix: 99, Version: 13}
	weapon := domain.Item{ID: domain.ID(), ItemID: 20, Quantity: 1, Version: 8, CurrentLuck: 7, LuckOilToSix: 101, LuckOilToNine: 21}
	free := domain.Item{ID: domain.ID(), ItemID: fixture.LuckOil.ItemID, Quantity: 1, Version: 4}
	bound := domain.Item{ID: domain.ID(), ItemID: fixture.LuckOil.ItemID, Quantity: 3, Version: 5, Bound: true}
	role := domain.Role{ID: wing.OwnerRoleID, Wing: &wing, Items: []domain.Item{free, bound}, Equipped: []domain.Equipped{{Slot: 0, Item: weapon}}}
	for _, tc := range []struct {
		name   string                                        // 场景名称。
		change func(*domain.Role, *pb.WeaponOilQuoteRequest) // 对原角色或请求实施的一处变化。
		code   uint32                                        // 对该错误的既有协议结果。
	}{
		{"同时选择武器", func(_ *domain.Role, q *pb.WeaponOilQuoteRequest) {
			q.Weapon = weaponOilQuoteRequest(weapon, true, false).Weapon
		}, p.CodeInvalidArgument},
		{"跳过目标", func(_ *domain.Role, q *pb.WeaponOilQuoteRequest) { q.Wing = nil }, p.CodeInvalidArgument},
		{"角色没有翅膀", func(role *domain.Role, _ *pb.WeaponOilQuoteRequest) { role.Wing = nil }, p.CodeWeaponOilInputs},
		{"他人翅膀", func(role *domain.Role, _ *pb.WeaponOilQuoteRequest) { role.ID = domain.ID() }, p.CodeWeaponOilInputs},
		{"错误实例", func(_ *domain.Role, q *pb.WeaponOilQuoteRequest) { q.Wing.WingId = domain.Bytes(domain.ID()) }, p.CodeWeaponOilInputs},
		{"版本已变化", func(_ *domain.Role, q *pb.WeaponOilQuoteRequest) { q.Wing.Version++ }, p.CodeWeaponOilInputs},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := role
			q := wingOilQuoteRequest(wing, false)
			tc.change(&changed, q)
			if _, code := buildWeaponOilPlan(changed, &fixture, q); code != tc.code {
				t.Fatal("用油目标围栏未返回原拒绝", code, tc.code)
			}
		})
	}
	q := wingOilQuoteRequest(wing, false)
	plan, code := buildWeaponOilPlan(role, &fixture, q)
	if code != 0 || plan.Wing == nil || *plan.Wing != wing || plan.Oil != free || plan.Equipped || !plan.OutputBound ||
		plan.BeforeLuck != 5 || plan.SuccessPPM != 200_000 || plan.PityUsed != 99 || plan.PityAfter != 100 || plan.Guaranteed {
		t.Fatal("专属翅膀报价未使用非绑原油和自己的第一段累计", code, plan)
	}
	frozen := weaponOilStoredQuote{Plan: plan, Roll: weaponOilPPM - 1}
	public := frozen.public(q.SceneEpoch)
	if public.Weapon != nil || public.Wing == nil || public.Wing.Hidden != true || public.Equipped || !public.OutputBound {
		t.Fatal("报价应公开原翅膀，而非伪装为背包武器", public)
	}
	after, receipt := applyWeaponOilPlan(role, frozen)
	if len(after.Items) != 1 || after.Items[0] != bound || after.Equipped[0].Item != weapon || receipt.OilRemainder != nil ||
		receipt.Wing == nil || receipt.Proto().Weapon != nil || receipt.Proto().Wing == nil || receipt.PityUsed != 100 || receipt.Success {
		t.Fatal("用油失败也只扣一瓶真实非绑油，不改变穿戴原武器", after, receipt)
	}
	want := wing
	want.Luck, want.OilToSix, want.Version = 3, 100, wing.Version+1
	if *after.Wing != want || *role.Wing != wing {
		t.Fatal("翅膀用油改动了原等级、外观或第二段计数", after.Wing, role.Wing)
	}
	fixture.Wings.AllowLuckOil = false
	if _, code = buildWeaponOilPlan(role, &fixture, q); code != p.CodeWeaponOilUnavailable {
		t.Fatal("翅膀功能须单独授权", code)
	}
	if current, code := buildWeaponOilPlan(role, &fixture, weaponOilQuoteRequest(weapon, true, false)); code != 0 || current.Wing != nil || current.Weapon != weapon {
		t.Fatal("关闭翅膀培养不影响原武器报价", code, current)
	}
}
