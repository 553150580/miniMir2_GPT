// 本文件执行连续转生和待用经验释放，复用普通升级曲线。
package domain

import "errors"

// RebirthTransition 保存等级、上限和余额的原前后状态，供报价和回执对账。
type RebirthTransition struct {
	Before      ProgressionSnapshot `bson:"before"`              // 扣费前角色事实。
	After       ProgressionSnapshot `bson:"after"`               // 转生和释放经验后的事实。
	PreviousCap uint32              `bson:"previous_cap"`        // 当前档的原等级上限。
	NewCap      uint32              `bson:"new_cap"`             // 本档只开放一级。
	ReleasedXP  int64               `bson:"released_experience"` // 从原待用池转入正常升级的数额。
}

// ApplyRebirth 只在达到原上限后前进一档，原待用经验办结于本次变更。
// 角色的等级可以按普通经验规则推进一次，其它养成和资产由调用方原样保留。
func ApplyRebirth(role Role, contract GrowthContract) (Role, RebirthTransition, error) {
	before := RebirthTransition{Before: role.ProgressionSnapshot()}
	if contract.RebirthLevelIncrement != 1 || role.ProgressionFormatVersion != contract.FormatVersion ||
		role.ProgressionCurveVersion != contract.CurveVersion || role.ProgressionMigrationRequired {
		return role, before, errors.New("角色成长格式未确认")
	}
	cap, err := contract.EffectiveCap(role.RebirthCount)
	if err != nil || role.Level != cap || cap >= contract.OperatingLevelCap || role.Experience != 0 ||
		role.PendingExperience < 0 || role.PendingExperience > contract.ExpPerLevel[cap-1] {
		return role, before, errors.New("角色未达到已开放的转生档位")
	}
	out := role
	out.RebirthCount++
	out.PendingExperience = 0
	advanced, grant, err := ApplyExperience(out, role.PendingExperience, contract)
	if err != nil || grant.EffectiveCap != cap+1 || grant.ToProgress != role.PendingExperience ||
		grant.ToPool != 0 || grant.Rejected != 0 {
		return role, before, errors.New("待用经验无法按原曲线释放")
	}
	return advanced, RebirthTransition{Before: before.Before, After: advanced.ProgressionSnapshot(),
		PreviousCap: cap, NewCap: cap + 1, ReleasedXP: grant.ToProgress}, nil
}
