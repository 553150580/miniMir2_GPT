// 本文件核对用油是否指向角色专属翅膀，并限制培养对独立翅膀的变更范围。
package persistence

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// wingOilEnabled 显式要求已有祝福油和已开放培养的翅膀配置。
func wingOilEnabled(fixture *devconfig.Fixture) bool {
	return fixture.LuckOil != nil && fixture.Wings != nil && fixture.Wings.AllowLuckOil
}

// validWingOilRef 只接受已激活的原翅膀身份和能够递增的版本。
func validWingOilRef(ref *pb.WingOilRef) bool {
	return ref != nil && validWingRef(ref.WingId, ref.Version, false) && len(ref.ProtoReflect().GetUnknown()) == 0
}

// wingOilTarget 从当前角色自己的独立状态核对对象，未激活、错误归属、旧版本或满幸运均无合法目标。
func wingOilTarget(role domain.Role, rules *devconfig.WingConfig, ref *pb.WingOilRef) (*domain.WingState, bool) {
	wing := role.Wing
	if !validRoleWing(wing, role.ID, rules) || !wingRefMatches(wing, ref.WingId, ref.Version) || wing.Luck >= weaponOilMaxLuck {
		return nil, false
	}
	return wing, true
}

// nextWingOilState 只调整原翅膀自己的幸运、对应段累计和版本。阶数、外观与隐藏不参与用油。
func nextWingOilState(plan weaponOilPlan, luck, used uint32) domain.WingState {
	wing := *plan.Wing
	wing.Luck, wing.Version = luck, wing.Version+1
	if plan.PityAfter != 0 {
		if plan.BeforeLuck < 6 {
			wing.OilToSix = used
		} else {
			wing.OilToNine = used
		}
	}
	return wing
}
