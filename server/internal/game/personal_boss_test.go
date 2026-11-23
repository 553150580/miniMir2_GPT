// 本文件核验个人首领在等待确认时只接受正确场景和真实到期时钟。
package game

import (
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestPersonalBossRuntimeExpiresBeforeReady 保证无ACK时既不运行战斗，也不靠迟到ACK重开已结束场次。
func TestPersonalBossRuntimeExpiresBeforeReady(t *testing.T) {
	start := time.Unix(1800000000, 0)
	deadline := start.Add(3 * time.Second)
	owner, _ := observedRoom()
	w := observedWorker(t, owner)
	connection := &travelConnection{alive: true, binding: 3}
	owner.Conn = connection
	// 持久角色另有原操作时，终态保留等待结算，便于直接检查冻结的原始事实。
	owner.Pending = true
	identity := bossChallengeIdentity{runID: 1, challengeID: "test", execution: 2, ruleVersion: 26, configHash: "fixture"}
	challenge, err := newBossChallenge(identity, "test-role", start, deadline, 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	grant := bossParticipantGrant{challenge: identity, roleID: "test-role", reservationID: "reservation", combatSnapshot: "snapshot"}
	if !challenge.admit(grant, start) {
		t.Fatal("未批准测试角色")
	}
	battle := &room{Conn: connection, Scene: 90, Baseline: 91, AwaitUntil: deadline, LastTick: start,
		bossBattle: &personalBossBattle{challenge: challenge, grant: grant}}
	run := &personalBossRun{conn: connection, battle: battle, grant: persistence.PersonalBossChallenge{ExpiresAt: deadline}}
	owner.personalBoss = run
	w.stepPersonalBoss(owner, deadline.Add(-time.Millisecond))
	if battle.Ready || challenge.finish != nil || challenge.hp != 100 || !battle.LastTick.Equal(start) {
		t.Fatal("未确认场景进行了战斗，或提前判定超时")
	}
	w.stepPersonalBoss(owner, deadline.Add(time.Second))
	ack := wire.Message(p.MsgC2SEnterWorldAck, 1, 3, &pb.EnterWorldAck{SceneEpoch: battle.Scene, BaselineId: battle.Baseline})
	w.ackPersonalBoss(owner, run, ack, deadline.Add(time.Second))
	result, ok := challenge.proposal()
	if !ok || result.kind != bossEscaped || !result.finishedAt.Equal(deadline) || result.hpRemaining != 100 || battle.Ready {
		t.Fatal("个人场次未按规则时刻超时，或旧ACK重新启用了战斗", result)
	}
}
