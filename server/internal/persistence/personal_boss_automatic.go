// 本文件校验托管卡自动预约并保存确认过的个人首领通关条件。
package persistence

import (
	"maps"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// checkAutomaticPersonalBoss 在预约事务内核对当前卡权益和原模板的真实通关证明。
func checkAutomaticPersonalBoss(r domain.Role, state domain.PersonalBossProgress, fixture *devconfig.Fixture, q PersonalBossBegin, at time.Time) error {
	if fixture.HostingBenefits == nil || !fixture.HostingBenefits.AutoPersonalBoss {
		return Fail(p.CodeLineForbidden, "托管卡自动个人首领未开放")
	}
	if !r.HostingCardActiveAt(at) {
		return Fail(p.CodeLineForbidden, "角色托管卡已过期或未生效")
	}
	proof := state.Qualifications[bossTemplateKey(q.TemplateID)]
	if proof.Fingerprint != q.Qualification || !fixture.AcceptsPersonalBossQualification(proof.RuleVersion, proof.ConfigHash) {
		return Fail(p.CodeLineForbidden, "本档个人首领需要先按当前战斗条件完成实战")
	}
	return nil
}

// recordPersonalBossQualification 用刚确认胜利的入场事实更新相应模板证明。
func recordPersonalBossQualification(state *domain.PersonalBossProgress, record *personalBossRecord) {
	if record.Qualification == "" {
		return
	}
	state.Qualifications = maps.Clone(state.Qualifications)
	if state.Qualifications == nil {
		state.Qualifications = make(map[string]domain.PersonalBossQualification)
	}
	state.Qualifications[bossTemplateKey(record.TemplateID)] = domain.PersonalBossQualification{
		Fingerprint: record.Qualification, RuleVersion: record.RuleVersion, ConfigHash: record.ConfigHash, ChallengeID: record.ChallengeID,
	}
}

// rewardIsBound 用场次记录的原特权证据判断实际击杀时的物品与金币归属。
func (r personalBossRecord) rewardIsBound() bool {
	until := r.PrivilegeUntil
	if r.Automatic {
		until = r.RewardPrivilegeUntil
	}
	return !until.After(r.FinishedAt)
}
