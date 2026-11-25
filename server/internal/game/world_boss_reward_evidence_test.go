// 本文件核对共享首领以原角色、实际命中时间和权威席位状态冻结奖励证据。
package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// TestWorldBossRewardEvidenceUsesKillTimeAndOriginalMembership 让宠物命中真正归零，并核对各类已确认席位。
func TestWorldBossRewardEvidenceUsesKillTimeAndOriginalMembership(t *testing.T) {
	workerRoom, _ := observedRoom()
	actor := observedWorker(t, workerRoom)
	start := time.Unix(1800000000, 0)
	killAt := start.Add(9 * time.Second)
	identity := bossChallengeIdentity{runID: 2, challengeID: "受控场次", execution: 3, ruleVersion: 30, configHash: "原配置"}
	challenge, err := newBossChallenge(identity, "", start, start.Add(time.Hour), 10, 5)
	if err != nil {
		t.Fatal(err)
	}
	boss := &entity{Handle: 10, Kind: 3, HP: 10, MaxHP: 10, Life: 1}
	arena := &worldBossArena{challenge: challenge, boss: boss}
	settings := devconfig.Config{RunID: identity.runID}
	store := &persistence.Store{Cfg: settings}
	joined := start.Add(time.Second)
	before := start.Add(4 * time.Second)
	until := killAt.Add(time.Hour)
	book := &actor.e.worldBossPrivileges
	type participant struct {
		id       string
		origin   uint64
		presence persistence.WorldBossPresence
	}
	members := []participant{
		{"active", 1, persistence.WorldBossPresenceOnline},
		{"left", 2, persistence.WorldBossPresenceLeft},
		{"offline", 3, persistence.WorldBossPresenceOffline},
		{"defeated", 4, persistence.WorldBossPresenceDefeated},
		{"rejoined", 5, persistence.WorldBossPresenceOnline},
	}
	var attacker, leftOwner *room
	for i, member := range members {
		grant := bossParticipantGrant{challenge: identity, roleID: member.id, reservationID: member.id, combatSnapshot: member.id}
		if !challenge.admit(grant, joined) {
			t.Fatal("测试原席位未入场", member.id)
		}
		child := &room{worldState: &worldState{}, runStore: store, Role: domain.Role{ID: member.id, Origin: member.origin},
			Player:     entity{Handle: uint32(i + 101), Kind: 1, Life: 1, HP: 10},
			bossBattle: &personalBossBattle{challenge: challenge, grant: grant, boss: boss}, worldBossCombat: arena}
		key := worldBossPrivilegeKey{runID: identity.runID, roleID: member.id}
		obs := &worldBossObservation{suspension: &worldBossSuspension{}}
		seat := &worldBossSeat{room: child, observation: obs, privilegeOffer: book.offer(key, identity.challengeID, start, killAt)}
		actor.confirmWorldBossRewardMember(arena, seat, joined)
		switch member.id {
		case "active":
			attacker = child
			attacker.Pet = &entity{Handle: 150, Kind: 2, Life: 2, HP: 20}
			book.observe(key, before, until)
			// 模拟归零逻辑延后处理，不能用之后收到的有效期撤销原奖励。
			book.observe(key, killAt.Add(time.Second), time.Time{})
		case "left":
			arena.noteRewardPresence(member.id, member.presence, before)
			// 离场后仍由普通 owner 观察已确认资产，首领子场景没有该角色身体。
			leftOwner = &room{worldState: &worldState{}, runStore: store, Role: domain.Role{ID: member.id, Origin: member.origin, PrivilegeUntil: until}}
		case "offline":
			// 原 owner 已观察断线，跨分片队列暂未把信号送到场次。
			obs.suspension.unixNano.Store(before.UnixNano())
		case "defeated":
			actor.applyPersonalBossDamage(child, boss, &child.Player, 50, before, nil)
		case "rejoined":
			arena.noteRewardPresence(member.id, persistence.WorldBossPresenceOffline, before)
			seat.observation = &worldBossObservation{}
			seat.privilegeOffer = book.offer(key, identity.challengeID, killAt.Add(-time.Second), killAt)
			actor.confirmWorldBossRewardMember(arena, seat, killAt.Add(-time.Second))
		}
	}
	// 一次普通房间接管即可刷新离场角色的原特权观察，无需还原首领席位。
	owner := &worker{e: actor.e}
	owner.observeWorldBossPrivilege(leftOwner, before.Add(time.Second))
	actor.applyPersonalBossDamage(attacker, attacker.Pet, boss, 11, killAt.Add(2*time.Second), []hitContext{{At: killAt}})
	proposal, ok := challenge.proposal()
	if !ok || proposal.kind != bossDefeated || proposal.finishedAt != killAt || len(proposal.contributions) != len(members) {
		t.Fatal("真实原终结命中没有冻结正确终态", proposal)
	}
	if proposal.contributions[0].grant.roleID != "active" || proposal.contributions[0].damage != 10 {
		t.Fatal("宠物有效扣血没有按原主人和共享剩余生命归属", proposal.contributions)
	}
	for _, member := range members {
		frozen := arena.rewardEvidence[member.id]
		wantUntil := killAt
		if member.id == "active" || member.id == "left" {
			wantUntil = until
		}
		if frozen == nil || frozen.Origin != member.origin || frozen.Presence != member.presence || !frozen.PrivilegeUntil.Equal(wantUntil) {
			t.Fatalf("原席位与原击杀时刻证据不匹配：%+v 得到 %+v", member, frozen)
		}
	}
	if len(book.entries) != 0 || len(arena.rewardMembers) != 0 {
		t.Fatal("永久终态仍挂着原参与者的临时跨分片观察")
	}
	beforeRetry := arena.rewardEvidence["left"]
	owner.observeWorldBossPrivilege(leftOwner, killAt.Add(time.Minute))
	actor.freezeWorldBossRewardEvidence(arena)
	if beforeRetry != arena.rewardEvidence["left"] || !beforeRetry.PrivilegeUntil.Equal(until) {
		t.Fatal("终态重试改写了玩家原特权证据")
	}
}

// TestWorldBossPrivilegeWatchReleaseKeepsNewOccurrence 验证未 ACK、健康会话挂接和旧场次清理的边界。
func TestWorldBossPrivilegeWatchReleaseKeepsNewOccurrence(t *testing.T) {
	var book worldBossPrivilegeBook
	key := worldBossPrivilegeKey{runID: 1, roleID: "role"}
	at := time.Unix(1800000000, 0)
	first := book.offer(key, "first", at, time.Time{})
	alsoFirst := book.offer(key, "first", at.Add(time.Second), at.Add(time.Hour))
	book.confirmOffer(first, false)
	if first != alsoFirst || book.entries[key] != first {
		t.Fatal("旧连接未确认基线移除了更新的本场申请")
	}
	book.confirmOffer(alsoFirst, true)
	second := book.offer(key, "second", at.Add(time.Minute), at.Add(time.Hour))
	if stale := book.finish(first, at); !stale.IsZero() || book.entries[key] != second {
		t.Fatal("旧场次结束覆盖新场次的临时输入", stale)
	}
	book.confirmOffer(second, false)
	if len(book.entries) != 0 {
		t.Fatal("未 ACK 的首领席位留下跨分片观察")
	}
}
