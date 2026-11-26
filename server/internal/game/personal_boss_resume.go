// 本文件管理个人首领的健康断线和新会话附着，保持同一个战斗现场。
package game

import (
	"math"
	"time"

	p "minimir/internal/protocol"
)

// detachPersonalBoss 释放观察连接和摇杆输入。已受理的事件、地面目标和自动设置仍归原场。
func (w *worker) detachPersonalBoss(owner *room) {
	run := owner.personalBoss
	if run == nil {
		return
	}
	run.conn, run.shown = nil, false
	if battle := run.battle; battle != nil {
		battle.Conn, battle.Ready, battle.Manual, battle.PetBornReady = nil, false, false, nil
	}
}

// resumePersonalBoss 在新票完成普通场景确认后续接原预约，只轮换观察身份。
func (w *worker) resumePersonalBoss(owner *room, now time.Time) {
	run := owner.personalBoss
	if run == nil || run.begin.Automatic || run.battle == nil || run.shown || owner.Auth != run.auth || owner.Conn == nil || !owner.Conn.Alive() {
		return
	}
	w.stepPersonalBoss(owner, now)
	battle := run.battle
	if battle.Isolated || battle.bossBattle.challenge.finish != nil {
		return
	}
	baseline, ok := w.nextTravelBaseline(max(owner.Baseline, battle.Baseline))
	scene := max(owner.Scene, battle.Scene)
	if !ok || scene >= math.MaxUint64-1 {
		w.isolateRoom(owner)
		return
	}
	battle.Scene, battle.Baseline = scene+1, baseline
	w.presentPersonalBoss(owner, run, now)
}

// presentPersonalBoss 生成新观察的入场状态和完整基线，原场的实体、冷却和定时器保持原值。
func (w *worker) presentPersonalBoss(owner *room, run *personalBossRun, now time.Time) {
	battle := run.battle
	run.conn, run.shown = owner.Conn, true
	battle.Auth, battle.Conn, battle.InputSequence = owner.Auth, owner.Conn, 0
	battle.Role.EconomicSeq, battle.Role.Revision = owner.Role.EconomicSeq, owner.Role.Revision
	battle.Role.HostingSeconds = owner.Role.HostingSeconds
	battle.AwaitUntil = minBossTime(now.Add(10*time.Second), run.grant.ExpiresAt)
	w.personalBossState(owner, run, 0, bossStateEntering, 0)
	if entry := w.enterLineResult(battle); entry != nil {
		entry.ServerTransitionId = run.grant.ChallengeID
		w.send(battle, p.MsgS2CEnterLine, 0, entry)
		w.baseline(battle)
	}
}
