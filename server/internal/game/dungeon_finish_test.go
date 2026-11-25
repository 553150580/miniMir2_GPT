// 本文件验证讨伐链停机时选择原终态，以及返场前校验永久回执的边界。
package game

import (
	"testing"
	"time"

	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// TestDungeonDrainOutcome 不允许维护中断覆盖已判定的胜负或改变原超时边界。
func TestDungeonDrainOutcome(t *testing.T) {
	started := time.Unix(100, 0)
	deadline := started.Add(time.Minute)
	grant := persistence.DungeonChallenge{StartedAt: started, ExpiresAt: deadline, Members: []persistence.DungeonMember{{RoleID: "原角色"}}}
	active := &dungeonArena{grant: grant}
	markDungeonDrainOutcome(active, deadline.Add(-time.Millisecond))
	if active.outcome != persistence.DungeonSystemInterrupted || active.finishedAt != deadline.Add(-time.Millisecond) {
		t.Fatal("未到期且无结论才应按维护中断", active.outcome, active.finishedAt)
	}
	expired := &dungeonArena{grant: grant, seats: map[string]*dungeonCombatSeat{"原角色": {started: true, room: &room{worldState: &worldState{}}}}}
	markDungeonDrainOutcome(expired, deadline.Add(time.Minute))
	if expired.outcome != persistence.DungeonFailed || expired.finishedAt != deadline {
		t.Fatal("已到期的原场只应按原截止判普通失败", expired.outcome, expired.finishedAt)
	}
	unacknowledged := &dungeonArena{grant: grant}
	markDungeonDrainOutcome(unacknowledged, deadline)
	if unacknowledged.outcome != persistence.DungeonSystemInterrupted || unacknowledged.finishedAt != deadline {
		t.Fatal("加载未完成的原场到截止应走系统中断", unacknowledged.outcome, unacknowledged.finishedAt)
	}
	succeeded := &dungeonArena{grant: grant, outcome: persistence.DungeonSucceeded, finishedAt: deadline.Add(-time.Second)}
	markDungeonDrainOutcome(succeeded, deadline.Add(time.Minute))
	if succeeded.outcome != persistence.DungeonSucceeded || succeeded.finishedAt != deadline.Add(-time.Second) {
		t.Fatal("维护覆盖了已裁决胜负", succeeded.outcome, succeeded.finishedAt)
	}
}

// TestValidDungeonReturn 返场必须同时持有原场、原终态时刻和全员永久身份。
func TestValidDungeonReturn(t *testing.T) {
	finished := time.Unix(100, 123456789)
	grant := persistence.DungeonChallenge{ChallengeID: "原编号", ReservationID: "原预约", Execution: 3, RunID: 4, RunEpoch: 5, OwnerInstance: "原进程",
		Members: []persistence.DungeonMember{{RoleID: "原角色", AccountID: "原账号", Origin: 6}}}
	proposal := persistence.DungeonFinish{Outcome: persistence.DungeonFailed, FinishedAt: finished}
	result := grant
	result.Outcome, result.FinishedAt = proposal.Outcome, finished.Truncate(time.Millisecond)
	roles := []domain.Role{{ID: "原角色", AccountID: "原账号", Origin: 6}}
	if !validDungeonReturn(grant, proposal, result, roles) {
		t.Fatal("原回执的毫秒截断不应该阻止回场")
	}
	wrong := result
	wrong.Execution++
	if validDungeonReturn(grant, proposal, wrong, roles) || validDungeonReturn(grant, proposal, result, nil) {
		t.Fatal("同一场次编号不可替代原执行或完整成员")
	}
	wrong = result
	wrong.Members = nil
	if validDungeonReturn(grant, proposal, wrong, roles) {
		t.Fatal("返场应同时查证持久场次的完整原名单")
	}
	wrong = result
	wrong.Refunded = true
	if validDungeonReturn(grant, proposal, wrong, roles) {
		t.Fatal("普通战败不能用退款回执回场")
	}
	roles[0].Origin++
	if validDungeonReturn(grant, proposal, result, roles) {
		t.Fatal("不可向其它永久入口归属发送原场结果")
	}
}
