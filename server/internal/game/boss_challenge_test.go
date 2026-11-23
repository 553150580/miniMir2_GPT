// 本文件核验同一首领的共享生命、归属、重放去重和互斥终态。
package game

import (
	"testing"
	"time"
)

// TestBossChallengeSharedDamageAndTerminalReplay 用两个独立参战回执验证同一个权威生命和原终态。
func TestBossChallengeSharedDamageAndTerminalReplay(t *testing.T) {
	start := time.Unix(1800000000, 0)
	identity := bossChallengeIdentity{runID: 9, challengeID: "world-2026-11-23T20:00", execution: 3, ruleVersion: 26, configHash: "rule-config"}
	battle, err := newBossChallenge(identity, "", start, start.Add(time.Hour), 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	first := bossParticipantGrant{challenge: identity, roleID: "first", reservationID: "reservation-1", combatSnapshot: "snapshot-1"}
	second := bossParticipantGrant{challenge: identity, roleID: "second", reservationID: "reservation-2", combatSnapshot: "snapshot-2"}
	if !battle.admit(first, start) || !battle.admit(first, start.Add(time.Second)) || !battle.admit(second, start.Add(time.Second)) {
		t.Fatal("同场的合法角色或原预约重放未入场")
	}
	stale := second
	stale.challenge.execution--
	if battle.admit(stale, start.Add(time.Second)) || battle.hit(bossDamageFact{grant: stale, sequence: 1, at: start.Add(time.Second), amount: 100}) != 0 {
		t.Fatal("旧执行代次错误参加了当前场次")
	}
	firstHit := bossDamageFact{grant: first, sequence: 1, at: start.Add(2 * time.Second), amount: 75}
	if battle.hit(firstHit) != 75 || battle.hit(firstHit) != 0 || battle.hp != 25 {
		t.Fatal("同一内部命中重试重复扣除了首领生命")
	}
	lastHit := bossDamageFact{grant: second, sequence: 1, at: start.Add(3 * time.Second), amount: 500}
	if battle.hit(lastHit) != 25 || battle.interrupt(identity, start.Add(4*time.Second)) {
		t.Fatal("最后一击没有截断过量伤害，或终态被取消覆盖")
	}
	battle.expire(start.Add(time.Hour))
	result, ok := battle.proposal()
	if !ok || result.kind != bossDefeated || result.finishedAt != lastHit.at || result.hpRemaining != 0 || len(result.contributions) != 2 ||
		result.contributions[0].damage != 75 || result.contributions[1].damage != 25 {
		t.Fatal("场次结局与有效参与伤害不一致", result)
	}
	result.contributions[0].damage = 999
	retried, ok := battle.proposal()
	if !ok || retried.contributions[0].damage != 75 || retried.kind != bossDefeated {
		t.Fatal("提案发布后的改动污染了重复投递的权威结果")
	}
}

// TestPersonalBossChallengeScopeAndDeadline 验证独享角色身份、独立复战和逃走时间。
func TestPersonalBossChallengeScopeAndDeadline(t *testing.T) {
	start := time.Unix(1800000000, 0)
	identity := bossChallengeIdentity{runID: 3, challengeID: "personal-1", execution: 7, ruleVersion: 26, configHash: "rule-config"}
	battle, err := newBossChallenge(identity, "owner", start, start.Add(time.Minute), 120, 1)
	if err != nil {
		t.Fatal(err)
	}
	grant := bossParticipantGrant{challenge: identity, roleID: "owner", reservationID: "first", combatSnapshot: "equipment-v1"}
	if battle.admit(grant, start.Add(-time.Second)) || !battle.admit(grant, start) {
		t.Fatal("个人挑战接受了开放前入场，或拒绝授权角色")
	}
	stranger := grant
	stranger.roleID = "stranger"
	if battle.admit(stranger, start) || battle.hit(bossDamageFact{grant: stranger, sequence: 1, at: start, amount: 500}) != 0 {
		t.Fatal("第二个角色进入了个人独享场次")
	}
	if battle.hit(bossDamageFact{grant: grant, sequence: 1, at: battle.expiresAt, amount: 500}) != 0 {
		t.Fatal("到期边界仍接受命中")
	}
	battle.expire(start.Add(2 * time.Minute))
	result, ok := battle.proposal()
	if !ok || result.kind != bossEscaped || !result.finishedAt.Equal(battle.expiresAt) || result.hpRemaining != 120 {
		t.Fatal("逃走事实错误使用轮询时点或标记为已击杀", result)
	}
	identity.challengeID, identity.execution = "personal-2", identity.execution+1
	retry, err := newBossChallenge(identity, "owner", start.Add(3*time.Minute), start.Add(4*time.Minute), 120, 1)
	if err != nil || retry.hp != 120 || retry.admit(grant, retry.startsAt) {
		t.Fatal("新挑战继承了原场次状态或旧入场预约", err)
	}
}
