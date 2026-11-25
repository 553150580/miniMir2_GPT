// 本文件管理招募中的 AI 预选席位和开战前原报价核对。
package game

import (
	"slices"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// seatsUsed 计算真人和预选 AI 共用的三个席位。
func (party *dungeonParty) seatsUsed() int {
	return len(party.members) + len(party.hires)
}

// acceptsProfile 核对当前招募所需的职业和等级。
func (rule dungeonPartyRequirements) acceptsProfile(job, level uint32) bool {
	return job < 3 && level >= rule.minimumLevel && rule.jobsMask&(1<<job) != 0
}

// dungeonHasMemberJob 检查此职业是否已经由真人占用。
func dungeonHasMemberJob(members []dungeonPartyMember, job uint32) bool {
	return slices.ContainsFunc(members, func(member dungeonPartyMember) bool { return member.job == job })
}

// dungeonHasHireJob 检查此职业是否已经由预选 AI 占用。
func dungeonHasHireJob(hires []devconfig.DungeonHireOption, job uint32) bool {
	return slices.ContainsFunc(hires, func(hire devconfig.DungeonHireOption) bool { return hire.Job == job })
}

// acceptsHire 把 AI 作为占用本队招募条件的临时身体，不增加角色身份。
func (rule dungeonPartyRequirements) acceptsHire(hire devconfig.DungeonHireOption, members []dungeonPartyMember, earlier []devconfig.DungeonHireOption) bool {
	return rule.acceptsProfile(hire.Job, hire.Level) &&
		(rule.sameJob || !dungeonHasMemberJob(members, hire.Job) && !dungeonHasHireJob(earlier, hire.Job))
}

// changeHire 只修改已核对队长、队伍修订和阶段的原招募名单，扣费留到整队开战。
func (party *dungeonParty) changeHire(fixture *devconfig.Fixture, request *pb.DungeonPartyRequest) uint32 {
	if request.Action == dungeonPartyHire {
		template, found := fixture.DungeonTemplateByID(party.template)
		if !found {
			return p.CodeLineForbidden
		}
		option, found := template.HireOption(request.HireOptionId)
		if !found || !party.requirements.acceptsHire(option, party.members, party.hires) {
			return p.CodeLineForbidden
		}
		if party.seatsUsed() >= devconfig.DungeonMaxPlayers {
			return p.CodeLineFull
		}
		party.hires = append(party.hires, option)
	} else {
		index := int(request.HireSlot) - 1
		if index < 0 || index >= len(party.hires) {
			return p.CodeInvalidArgument
		}
		party.hires = slices.Delete(party.hires, index, index+1)
	}
	party.revision = dungeonPartyRevision.Add(1)
	return 0
}

// dungeonHireViews 只公示报价和身体身份，技能与服务器快照不随招募协议上传。
func dungeonHireViews(hires []devconfig.DungeonHireOption) []*pb.DungeonPartyHireOption {
	var views []*pb.DungeonPartyHireOption
	for _, hire := range hires {
		views = append(views, &pb.DungeonPartyHireOption{OptionId: hire.ID, DisplayName: hire.DisplayName,
			Job: hire.Job, Level: hire.Level, FeeGold: uint64(hire.FeeGold)})
	}
	return views
}

// freezeDungeonHires 验证当前模板仍等于名单所示报价，原确认窗不会替用户接受新价格。
func (party *dungeonParty) freezeDungeonHires(template *devconfig.DungeonConfig, begin *persistence.DungeonBegin) (int64, bool) {
	if len(party.hires) == 0 {
		return 0, true
	}
	ids := make([]uint32, 0, len(party.hires))
	var fee int64
	for _, selected := range party.hires {
		current, exists := template.HireOption(selected.ID)
		if !exists || current.FeeGold != selected.FeeGold || current.Job != selected.Job || current.Level != selected.Level ||
			current.DisplayName != selected.DisplayName || !slices.Equal(current.Skills, selected.Skills) {
			return 0, false
		}
		fee += current.FeeGold
		ids = append(ids, selected.ID)
	}
	begin.LeaderRoleID, begin.HireOptionIDs = party.members[0].id, ids
	return fee, true
}

// validDungeonHireReceipt 核对所有 AI 席位、原付款角色和两个原钱包的实付合计。
func validDungeonHireReceipt(begin persistence.DungeonBegin, grant persistence.DungeonChallenge, fee int64) bool {
	if !slices.Equal(grant.HireOptionIDs, begin.HireOptionIDs) {
		return false
	}
	charge := grant.HireCharge
	if len(begin.HireOptionIDs) == 0 {
		return charge == nil
	}
	return charge != nil && charge.RoleID == begin.LeaderRoleID && charge.Gold >= 0 &&
		charge.Gold <= fee && charge.BoundGold == fee-charge.Gold
}

// sameDungeonHireReceipt 确认正常回场和系统退款沿用原获准席位及实付组成。
func sameDungeonHireReceipt(prior, current persistence.DungeonChallenge) bool {
	if !slices.Equal(prior.HireOptionIDs, current.HireOptionIDs) {
		return false
	}
	if prior.HireCharge == nil || current.HireCharge == nil {
		return prior.HireCharge == current.HireCharge
	}
	return *prior.HireCharge == *current.HireCharge
}
