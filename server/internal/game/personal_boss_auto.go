// 本文件按托管卡的已验证战斗配置调度个人首领后台场次。
package game

import (
	"cmp"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/persistence"
)

// 自动预约使用现有数据库队列，避免每帧重新构造装备与战斗快照。
const personalBossAutoCheckInterval = time.Second

// personalBossQualification 按本场实际战斗输入和穿戴实例识别曾通关的组合。
// 钱包、普通耐久磨损和未升级的宠物经验不会影响本场战斗配置。
func personalBossQualification(owner *room, snapshot personalBossSnapshot) string {
	type equipped struct {
		Slot uint32 // 实际穿戴部位。
		ID   string // 原装备实例身份，替换同属性装备也需重新验证。
	}
	items := make([]equipped, 0, len(owner.Role.Equipped))
	for _, entry := range owner.Role.Equipped {
		if entry.Slot != devconfig.CastingSupplySlot {
			items = append(items, equipped{Slot: entry.Slot, ID: entry.Item.ID})
		}
	}
	slices.SortFunc(items, func(a, b equipped) int { return cmp.Compare(a.Slot, b.Slot) })
	if pet := snapshot.Pet; pet != nil && pet.PetRuntime != nil {
		body, runtime := *pet, *pet.PetRuntime
		runtime.Experience = 0
		body.PetRuntime = &runtime
		snapshot.Pet = &body
	}
	prefs := snapshot.autoSkills.proto()
	encoded, err := json.Marshal(struct {
		Snapshot personalBossSnapshot // 双方按此冻结可实战的角色和宠物属性。
		Items    []equipped           // 与本次装备属性对应的原实例。
		Auto     bool                 // 这套技能偏好是否允许自动施法。
		Disabled []uint32             // 已关闭的自动技能编号。
	}{Snapshot: snapshot, Items: items, Auto: prefs.Enabled, Disabled: prefs.DisabledSkillIds})
	if err != nil {
		return ""
	}
	return persistence.Hash(encoded)
}

// personalBossOfflineAvailable 只接受原普通挂机会话的可用时长，不为后台场分开扣时。
func personalBossOfflineAvailable(owner *room) bool {
	return owner.HostSession != "" && owner.currentWorld() == owner.PersonalWorld && owner.Player.HP > 0 &&
		owner.Role.HostingSeconds > owner.hostSecondsOpen
}

// maybeStartAutomaticPersonalBoss 在普通房间推进后评估一次，只提交一个已通过真实战斗的档位。
func (w *worker) maybeStartAutomaticPersonalBoss(owner *room, now time.Time) {
	progress := owner.Role.PersonalBoss
	if progress == nil || len(progress.Qualifications) == 0 || progress.ActiveChallengeID != "" || !owner.Auto ||
		!owner.Role.HostingCardUntil.After(now) || owner.Pending || owner.Isolated || owner.entryAttempt != "" ||
		owner.Player.HP <= 0 || owner.Scene == 0 || owner.Scene >= math.MaxUint64-1 || owner.PersonalWorld == nil ||
		owner.currentWorld() != owner.PersonalWorld || now.Before(owner.LastTick) || now.Before(owner.personalBossAutoCheck) ||
		w.e.stopped.Load() || owner.mirrorMatch != nil || now.Before(owner.worldBossResume.closesAt) || !allowMirrorLobby(owner) {
		return
	}
	if owner.Conn == nil {
		if !personalBossOfflineAvailable(owner) {
			return
		}
	} else if !owner.Ready || !owner.Conn.Alive() || owner.Travel != nil && !owner.Travel.Acked {
		return
	}
	fixture := &w.roomStore(owner).Cfg.Fixture
	if fixture.HostingBenefits == nil || !fixture.HostingBenefits.AutoPersonalBoss || fixture.PersonalBoss == nil {
		return
	}
	owner.personalBossAutoCheck = now.Add(personalBossAutoCheckInterval)
	var selected *devconfig.PersonalBossConfig
	var snapshot personalBossSnapshot
	var qualification string
	// 等级门槛最高的合格档优先。相同门槛按模板编号确定次序。
	for index := 0; index <= len(fixture.PersonalBossVariants); index++ {
		option := fixture.PersonalBoss
		if index > 0 {
			option = &fixture.PersonalBossVariants[index-1]
		}
		if selected != nil && (option.MinimumLevel < selected.MinimumLevel || option.MinimumLevel == selected.MinimumLevel && option.TemplateID >= selected.TemplateID) {
			continue
		}
		proof, ok := progress.Qualifications[strconv.FormatUint(uint64(option.TemplateID), 10)]
		if !ok || !fixture.AcceptsPersonalBossQualification(proof.RuleVersion, proof.ConfigHash) ||
			!persistence.CanBookPersonalBoss(owner.Role, option, now) {
			continue
		}
		frozen := w.freezePersonalBossSnapshot(owner, option)
		fingerprint := personalBossQualification(owner, frozen)
		if fingerprint != "" && fingerprint == proof.Fingerprint {
			selected, snapshot, qualification = option, frozen, fingerprint
		}
	}
	if selected != nil {
		w.reservePersonalBoss(owner, selected, snapshot, qualification, "", 0, now)
	}
}
