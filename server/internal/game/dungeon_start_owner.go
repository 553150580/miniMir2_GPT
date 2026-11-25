// 本文件由各原角色分片准备固定战斗快照，并在同一处安装已确认的开战结果。
package game

import (
	"encoding/json"
	"math"
	"time"

	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// prepareDungeonStartOwner 在同一原会话内冻结本席。角色原资产未静止时仍先推进原结算。
func (w *worker) prepareDungeonStartOwner(start *dungeonStartAttempt, identity dungeonPartyIdentity, now time.Time) *dungeonStartPreparation {
	owner := w.rooms[identity.id]
	if w.e.stopped.Load() || !now.Before(start.prepareUntil) || owner == nil || w.roomStore(owner) != start.store || owner.Isolated ||
		!owner.Ready || owner.Conn == nil || !owner.Conn.Alive() || owner.Conn.Binding() != identity.binding || uint64(owner.Auth.SessionEpoch) != identity.binding {
		return &dungeonStartPreparation{code: p.CodeLineRecovering}
	}
	if identity.id == start.leader.id && owner.Scene != start.scene {
		return &dungeonStartPreparation{code: p.CodeLineRecovering}
	}
	if owner.currentWorld() != owner.PersonalWorld || owner.Player.HP <= 0 || owner.Scene >= math.MaxUint64-1 || owner.Role.Level < start.template.MinimumLevel ||
		owner.personalBoss != nil || owner.worldBossObservation != nil || owner.dungeonMatch != nil || owner.dungeonStart != nil || owner.mirrorMatch != nil ||
		owner.Role.Dungeon != nil && owner.Role.Dungeon.ActiveChallengeID != "" || owner.Role.PersonalBoss != nil && owner.Role.PersonalBoss.ActiveChallengeID != "" ||
		owner.Travel != nil && owner.Travel.Phase < travelCommitted || pendingPersonalBossTransfer(owner) {
		return &dungeonStartPreparation{code: p.CodeLineForbidden}
	}
	if owner.entryAttempt != "" || owner.Pending || w.drainRoomSettlement(owner, now) {
		return &dungeonStartPreparation{code: p.CodeOperationPending}
	}
	baseline, available := w.nextTravelBaseline(owner.Baseline)
	if !available {
		return &dungeonStartPreparation{code: p.CodeLineRecovering}
	}
	// 讨伐场次按原名单与共享占位摆放宠物，角色分片只冻结可参战的身体。
	snapshot := w.freezeBossCombatSnapshot(owner, &start.template.Map)
	encoded, err := json.Marshal(snapshot)
	if err != nil || owner.Isolated || snapshot.Fighter.HP <= 0 {
		return &dungeonStartPreparation{code: p.CodeLineRecovering}
	}
	role := owner.Role
	member := &dungeonPreparedMember{owner: w, auth: owner.Auth, conn: owner.Conn, scene: owner.Scene + 1, baseline: baseline, snapshot: snapshot,
		role: domain.Role{ID: role.ID, Origin: role.Origin, Name: role.Name, Job: role.Job, Level: role.Level, EconomicSeq: role.EconomicSeq,
			Revision: role.Revision, Skills: snapshot.Skills, HostingSeconds: role.HostingSeconds, SkillSlotIDs: snapshot.SkillSlotIDs},
		durable: persistence.DungeonAdmissionMember{Auth: owner.Auth, ExpectedRevision: role.Revision, CombatSnapshot: persistence.Hash(encoded)}}
	owner.dungeonStart, owner.Pending, owner.PendingOperationID = start, true, start.ownerOperation()
	return &dungeonStartPreparation{member: member}
}

// ownerOperation 用同一稳定挑战编号标识当前挂起槽，角色期间收不到来自另一场的释放。
func (start *dungeonStartAttempt) ownerOperation() string {
	return "dungeon:start:" + start.begin.ChallengeID
}

// completeDungeonStartOwner 在释放原待确认槽之前同步扣次版本并安装副本观察。
func (w *worker) completeDungeonStartOwner(start *dungeonStartAttempt, prepared dungeonPreparedMember, committed domain.Role, install func(*room)) {
	owner := w.rooms[prepared.auth.RoleID]
	if owner == nil || owner.dungeonStart != start || owner.Auth != prepared.auth || owner.PendingOperationID != start.ownerOperation() {
		return
	}
	if committed.ID != "" && !owner.Isolated {
		prior := owner.Role.PrivilegeUntil
		owner.Role = committed
		w.acceptRolePrivilege(owner, time.Now(), prior)
		install(owner)
	}
	owner.dungeonStart, owner.PendingOperationID = nil, ""
	owner.resumeAfterPending(time.Now())
	w.after(owner)
}
