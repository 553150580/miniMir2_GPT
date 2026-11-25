// 本文件核验上海开场时刻、运行服场次身份以及共享场的同帧终态顺序。
package game

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	"testing"
	"time"
)

// TestWorldBossWindowShanghaiBoundaries 确认每日两场、半开窗口和独立运行服场次编号。
func TestWorldBossWindowShanghaiBoundaries(t *testing.T) {
	cfg := &devconfig.WorldBossConfig{TemplateID: 3, DailyOpenOffsets: []time.Duration{12 * time.Hour, 20 * time.Hour}, Window: time.Hour}
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	noon := time.Date(2026, 11, 23, 12, 0, 0, 0, zone)
	before, open := worldBossWindow(cfg, 1, noon.Add(-time.Millisecond))
	if open || !before.opensAt.Equal(noon) {
		t.Fatal("午间开场前提前开放或预告了错误场次", before)
	}
	first, open := worldBossWindow(cfg, 1, noon)
	peer, otherOpen := worldBossWindow(cfg, 2, noon.UTC())
	if !open || !otherOpen || before.id != first.id || peer.id == first.id || !first.closesAt.Equal(noon.Add(time.Hour)) {
		t.Fatal("同服稳定场次、异服身份或真实关闭时刻错误", first, peer)
	}
	night, open := worldBossWindow(cfg, 1, noon.Add(time.Hour))
	if open || night.id == first.id || !night.opensAt.Equal(noon.Add(8*time.Hour)) {
		t.Fatal("午间结束后未切换到晚间预告", night)
	}
	tomorrow, open := worldBossWindow(cfg, 1, night.opensAt.Add(time.Hour))
	if open || !tomorrow.opensAt.Equal(noon.AddDate(0, 0, 1)) {
		t.Fatal("晚场结束后的运营日预告错误", tomorrow)
	}
}

// TestWorldBossRotationKeepsNoonIdentity 检验同服原午场编号和晚场新模板的唯一窗口。
func TestWorldBossRotationKeepsNoonIdentity(t *testing.T) {
	primary := devconfig.WorldBossConfig{TemplateID: 2, DailyOpenOffsets: []time.Duration{12 * time.Hour}, Window: time.Hour}
	evening := devconfig.WorldBossConfig{TemplateID: 5, DailyOpenOffsets: []time.Duration{20 * time.Hour}, Window: time.Hour}
	fixture := devconfig.Fixture{WorldBoss: &primary, WorldBossVariants: []devconfig.WorldBossConfig{evening}}
	noon := time.Date(2026, 11, 25, 4, 0, 0, 0, time.UTC)
	before, option, active := worldBossForFixture(&fixture, 1, noon.Add(-time.Millisecond))
	legacy := primary
	legacy.DailyOpenOffsets = []time.Duration{12 * time.Hour, 20 * time.Hour}
	original, _ := worldBossWindow(&legacy, 1, noon)
	if active || option.TemplateID != 2 || before.id != original.id || !before.opensAt.Equal(noon) {
		t.Fatal("轮换破坏了原午场预告和场次编号", before, original)
	}
	night := noon.Add(8 * time.Hour)
	waiting, option, active := worldBossForFixture(&fixture, 1, noon.Add(time.Hour))
	if active || option.TemplateID != 5 || waiting.id == original.id || !waiting.opensAt.Equal(night) {
		t.Fatal("午场结束后没有切为独立晚场预告", waiting)
	}
	opened, option, active := worldBossForFixture(&fixture, 1, night)
	peer, _, _ := worldBossForFixture(&fixture, 2, night)
	if !active || option.TemplateID != 5 || opened.id != waiting.id || opened.id == peer.id {
		t.Fatal("晚场开放或运行服隔离身份错误", opened, peer)
	}
}

// TestWorldBossSameTickUsesOneRemainingLife 验证两个原动作在同一逻辑时刻争用一个共享生命。
func TestWorldBossSameTickUsesOneRemainingLife(t *testing.T) {
	owner, _ := observedRoom()
	w := observedWorker(t, owner)
	now := time.Unix(1800000000, 0)
	identity := bossChallengeIdentity{runID: 2, challengeID: "shared", execution: 3, ruleVersion: 26, configHash: "test"}
	challenge, err := newBossChallenge(identity, "", now, now.Add(time.Hour), 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	monster := &devconfig.Monster{ID: 12, Fighter: devconfig.Fighter{HP: 3, AttackMS: 1000, MoveMS: 500}}
	boss := &entity{Handle: 10, Kind: 3, Def: 12, Life: 1, HP: 3, MaxHP: 3, Monster: monster}
	mapTemplate := &devconfig.MapTemplate{ID: 99, Width: 3, Height: 3, SceneKind: devconfig.SceneKindWorldBoss}
	arena := &worldBossArena{challenge: challenge, boss: boss, seats: make(map[string]*worldBossSeat)}
	for index, id := range []string{"first", "second"} {
		grant := bossParticipantGrant{challenge: identity, roleID: id, reservationID: id, combatSnapshot: id}
		if !challenge.admit(grant, now) {
			t.Fatal("测试角色被拒绝入场", id)
		}
		child := &room{worldState: &worldState{MapID: mapTemplate.ID, BossMap: mapTemplate, Monsters: []*entity{boss}, MonsterCount: 1},
			Scene: 20, Ready: true, Role: domain.Role{ID: id}, Player: entity{Handle: uint32(101 + index), Kind: 1, Life: 1, HP: 10, MaxHP: 10},
			bossBattle: &personalBossBattle{challenge: challenge, grant: grant, boss: boss, fighter: devconfig.Fighter{Accuracy: 10}}, worldBossCombat: arena}
		world := child.currentWorld()
		world.Attacks = []pendingAttack{{Caster: referenceIn(world, &child.Player), Target: referenceIn(world, boss), Due: now.Add(time.Second), Amount: 2, Order: 1}}
		arena.seats[id] = &worldBossSeat{observation: &worldBossObservation{}, room: child}
	}
	w.advanceWorldBossEvents(arena, now.Add(time.Second))
	proposal, ok := challenge.proposal()
	if !ok || proposal.kind != bossDefeated || boss.HP != 0 || challenge.hp != 0 || len(proposal.contributions) != 2 ||
		proposal.contributions[0].damage != 2 || proposal.contributions[1].damage != 1 {
		t.Fatal("两个原事件未共享同一份剩余生命，或重复计算溢出伤害", proposal)
	}
}

// TestWorldBossTimeoutUsesOriginalDeadline 验证无终结命中的场次以原期限逃走且保留剩余生命。
func TestWorldBossTimeoutUsesOriginalDeadline(t *testing.T) {
	owner, _ := observedRoom()
	w := observedWorker(t, owner)
	start := time.Unix(1800000000, 0)
	closeAt := start.Add(time.Minute)
	identity := bossChallengeIdentity{runID: 3, challengeID: "timeout", execution: 5, ruleVersion: 28, configHash: "test"}
	challenge, err := newBossChallenge(identity, "", start, closeAt, 6, 2)
	if err != nil {
		t.Fatal(err)
	}
	grant := bossParticipantGrant{challenge: identity, roleID: "role", reservationID: "receipt", combatSnapshot: "snapshot"}
	if !challenge.admit(grant, start) || challenge.hit(bossDamageFact{grant: grant, sequence: 1, at: start.Add(time.Second), amount: 2}) != 2 {
		t.Fatal("测试首领没有正确记下有效参与")
	}
	arena := &worldBossArena{occurrence: worldBossOccurrence{runID: 3, id: identity.challengeID, opensAt: start, closesAt: closeAt},
		config: &devconfig.WorldBossConfig{TemplateID: 1, ReturnDelay: 5 * time.Second}, challenge: challenge,
		boss: &entity{HP: 4, MaxHP: 6}, names: map[string]string{"role": "角色"}}
	w.stepWorldBoss(arena, closeAt.Add(time.Second))
	state := arena.state(10, worldBossFighting)
	result, ok := challenge.proposal()
	if !ok || result.kind != bossEscaped || !result.finishedAt.Equal(closeAt) || state.State != worldBossSettling || state.LatestResult != nil {
		t.Fatal("到期瞬间应冻结逃走提案，等待永久确认后再发送终态", state, result)
	}
	arena.result = &persistence.WorldBossResult{
		WorldBossSpec: persistence.WorldBossSpec{RunID: 3, OccurrenceID: identity.challengeID, TemplateID: 1, OpensAt: start, ClosesAt: closeAt, HPMax: 6},
		WorldBossFinish: persistence.WorldBossFinish{Outcome: persistence.WorldBossExpired, FinishedAt: closeAt, HPRemaining: 4,
			Contributions: []persistence.WorldBossContribution{{RoleID: "role", DisplayName: "角色", Damage: 2}}},
		ProgressKnown: true,
	}
	state = arena.state(10, worldBossFighting)
	if state.State != worldBossExpired || state.Hp != 4 || state.LatestResult.GetFinishedAtUnixMs() != closeAt.UnixMilli() ||
		len(state.Ranks) != 1 || state.Ranks[0].Damage != 2 {
		t.Fatal("确认的超时应使用原到期时刻和永久榜单", state)
	}
}

// TestWorldBossResumeKeepsOriginalCooldown 检查新认证只替换观察身份，不重置实际入场快照和时钟。
func TestWorldBossResumeKeepsOriginalCooldown(t *testing.T) {
	now := time.Unix(1800000000, 0)
	identity := bossChallengeIdentity{runID: 2, challengeID: "resume", execution: 3, ruleVersion: 28, configHash: "test"}
	challenge, err := newBossChallenge(identity, "", now, now.Add(time.Minute), 9, 2)
	if err != nil {
		t.Fatal(err)
	}
	grant := bossParticipantGrant{challenge: identity, roleID: "role", reservationID: "receipt", combatSnapshot: "snapshot"}
	if !challenge.admit(grant, now) {
		t.Fatal("未受理测试挑战授权")
	}
	mapTemplate := devconfig.MapTemplate{ID: 101, Width: 3, Height: 3, SceneKind: devconfig.SceneKindWorldBoss}
	position := devconfig.Point{X: 2, Y: 2}
	cooldown := now.Add(8 * time.Second)
	child := &room{worldState: &worldState{BossMap: &mapTemplate, MapID: mapTemplate.ID}, Role: domain.Role{ID: "role"},
		Player: entity{Handle: 101, Life: 2, Pos: position, HP: 4, MP: 2, NextAttack: cooldown}, NextSpell: cooldown,
		SkillCooldowns: map[uint32]time.Time{1: cooldown}, bossBattle: &personalBossBattle{challenge: challenge, grant: grant}}
	seat := &worldBossSeat{room: child, priorEpoch: 8}
	arena := &worldBossArena{config: &devconfig.WorldBossConfig{Map: mapTemplate}, challenge: challenge, boss: &entity{HP: 9},
		occurrence: worldBossOccurrence{closesAt: now.Add(time.Minute)}, seats: make(map[string]*worldBossSeat), resumable: map[string]*worldBossSeat{"role": seat}}
	obs := &worldBossObservation{auth: persistence.Auth{RoleID: "role", SessionEpoch: 8}}
	enter := worldBossEnter{observation: obs}
	if arena.resumeSeat(enter, seat, now.Add(time.Second)) {
		t.Fatal("撤销的认证代次重新接管了旧席位")
	}
	obs.auth.SessionEpoch = 9
	if !arena.resumeSeat(enter, seat, now.Add(time.Second)) || arena.seats["role"].room != child || len(arena.resumable) != 0 ||
		child.Player.Handle != 101 || child.Player.Life != 2 || child.Player.HP != 4 || child.Player.MP != 2 || child.Player.Pos != position ||
		!child.Player.NextAttack.Equal(cooldown) || !child.NextSpell.Equal(cooldown) || !child.SkillCooldowns[1].Equal(cooldown) {
		t.Fatal("新认证改变了原角色身体、已用魔法或技能冷却")
	}
}
