// 本文件按原讨伐链配置确定 AI 补位费用，并把扣款与系统中断退款交给同一场次事务。
package persistence

import (
	"math"
	"slices"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// DungeonHireCharge 是原确认的雇佣付款归属及两个钱包各自实际扣款。
type DungeonHireCharge struct {
	RoleID    string `bson:"role_id"`    // 原真实队长角色，系统中断仅退给该角色。
	Gold      int64  `bson:"gold"`       // 实际扣除的非绑定金币。
	BoundGold int64  `bson:"bound_gold"` // 实际扣除的绑定金币。
}

// validateDungeonHireRequest 拒绝超额、空编号或未由原真实名单提供的付款角色。
func validateDungeonHireRequest(q DungeonBegin) error {
	if len(q.HireOptionIDs) == 0 {
		if q.LeaderRoleID != "" {
			return Fail(p.CodeInvalidArgument, "没有讨伐链 AI 席位却指定了付款角色")
		}
		return nil
	}
	if len(q.HireOptionIDs) > devconfig.DungeonMaxPlayers-len(q.Members) || slices.Contains(q.HireOptionIDs, 0) ||
		!lowerHex(q.LeaderRoleID, 16) || !slices.ContainsFunc(q.Members, func(member DungeonAdmissionMember) bool {
		return member.Auth.RoleID == q.LeaderRoleID
	}) {
		return Fail(p.CodeInvalidArgument, "讨伐链 AI 席位或付款角色无效")
	}
	return nil
}

// chargeDungeonHires 只读已校验的原版本报价，队长余额不足时整队入场不提交。
func chargeDungeonHires(q DungeonBegin, template *devconfig.DungeonConfig, roles []domain.Role) (*DungeonHireCharge, error) {
	if len(q.HireOptionIDs) == 0 {
		return nil, nil
	}
	var total int64 // 原配置中各已预约 AI 席位的总费用。
	for _, id := range q.HireOptionIDs {
		option, exists := template.HireOption(id)
		if !exists {
			return nil, Fail(p.CodeLineForbidden, "原讨伐链 AI 雇佣选项未开放")
		}
		var err error // 有任一价格非法或总额溢出时，原队不能扣次或付款。
		total, err = add(total, option.FeeGold)
		if err != nil {
			return nil, err
		}
	}
	index := dungeonHirePayer(roles, q.LeaderRoleID)
	if index < 0 || roles[index].Wallet.Gold < 0 || roles[index].Wallet.BoundGold < 0 {
		return nil, Fail(p.CodeOperationConflict, "原讨伐链付款角色或钱包不一致")
	}
	wallet := &roles[index].Wallet
	charge := &DungeonHireCharge{RoleID: q.LeaderRoleID, BoundGold: min(wallet.BoundGold, total)}
	charge.Gold = total - charge.BoundGold
	if wallet.Gold < charge.Gold {
		return nil, Fail(p.CodeVendorFunds, "讨伐链队长金币不足")
	}
	wallet.BoundGold -= charge.BoundGold
	wallet.Gold -= charge.Gold
	progress := dungeonProgress(roles[index])
	progress.RefundableHireGold, progress.RefundableHireBoundGold = charge.Gold, charge.BoundGold
	roles[index].Dungeon = &progress
	return charge, nil
}

// settleDungeonHires 仅向系统中断的原角色按原钱包退款，旧无 AI 场次仍可正常结束。
func settleDungeonHires(challenge DungeonChallenge, roles []domain.Role) error {
	charge := challenge.HireCharge
	if len(challenge.HireOptionIDs) == 0 && charge == nil {
		return nil
	}
	if len(challenge.HireOptionIDs) == 0 || len(challenge.HireOptionIDs) > devconfig.DungeonMaxPlayers-len(roles) ||
		charge == nil || charge.Gold < 0 || charge.BoundGold < 0 {
		return Fail(p.CodeOperationConflict, "原讨伐链 AI 席位和费用回执不一致")
	}
	index := dungeonHirePayer(roles, charge.RoleID)
	if index < 0 {
		return Fail(p.CodeOperationConflict, "原讨伐链费用缺少实际付款角色")
	}
	role := &roles[index]
	progress := dungeonProgress(*role)
	if progress.RefundableHireGold != charge.Gold || progress.RefundableHireBoundGold != charge.BoundGold {
		return Fail(p.CodeOperationConflict, "原讨伐链退款容量和费用回执不一致")
	}
	if challenge.Refunded {
		wallet := &role.Wallet
		if wallet.Gold < 0 || wallet.BoundGold < 0 {
			return Fail(p.CodeOperationConflict, "原讨伐链退款钱包状态无效")
		}
		gold, err := add(wallet.Gold, charge.Gold)
		if err != nil {
			return err
		}
		bound, err := add(wallet.BoundGold, charge.BoundGold)
		if err != nil {
			return err
		}
		wallet.Gold, wallet.BoundGold = gold, bound
	}
	progress.RefundableHireGold, progress.RefundableHireBoundGold = 0, 0
	role.Dungeon = &progress
	return nil
}

// validateDungeonHireWallet 保证普通、批量等入账后，两个钱包各自仍能接收原额退款。
func validateDungeonHireWallet(role domain.Role) error {
	state := role.Dungeon
	if state == nil || state.RefundableHireGold == 0 && state.RefundableHireBoundGold == 0 {
		return nil
	}
	if state.ActiveChallengeID == "" || state.RefundableHireGold < 0 || state.RefundableHireBoundGold < 0 {
		return Fail(p.CodeOperationConflict, "角色保留的讨伐链退款容量无效")
	}
	if role.Wallet.Gold < 0 || role.Wallet.Gold > math.MaxInt64-state.RefundableHireGold ||
		role.Wallet.BoundGold < 0 || role.Wallet.BoundGold > math.MaxInt64-state.RefundableHireBoundGold {
		return Fail(p.CodeInvalidArgument, "金币余额超出保留原讨伐链退款后的上限")
	}
	return nil
}

// dungeonHirePayer 定位原真实付款角色，席位序号和显示身份均不用于支付授权。
func dungeonHirePayer(roles []domain.Role, roleID string) int {
	return slices.IndexFunc(roles, func(role domain.Role) bool { return role.ID == roleID })
}
