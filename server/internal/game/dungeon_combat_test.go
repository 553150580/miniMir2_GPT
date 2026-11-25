// 本文件以公共攻击和施法入口校验讨伐链同场占格、换段及唯一终态。
package game

import (
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// dungeonBattleFixture 装配四段可达小区和两个不同的观察场景。
func dungeonBattleFixture(t *testing.T) (*worker, *dungeonArena, *room, *room, *capturedConnection, time.Time) {
	t.Helper()
	w, origin, _ := stageAWorker(t, 1)
	now := time.Unix(1800000000, 0)
	template := &devconfig.DungeonConfig{TemplateID: 7, Duration: time.Minute,
		Map: devconfig.MapTemplate{ID: 700, SceneKind: devconfig.SceneKindDungeon, Width: 12, Height: 3}}
	monster := *origin.Monsters[0].Monster
	monster.HP, monster.Exp, monster.GoldMin, monster.GoldMax = 1000, 0, 0, 0
	for index, kind := range [...]devconfig.DungeonStageKind{devconfig.DungeonStageMobsFirst, devconfig.DungeonStageMobsSecond,
		devconfig.DungeonStageElite, devconfig.DungeonStageFinalBoss} {
		x := index * 3
		stage := devconfig.DungeonStage{Kind: kind,
			Area:         devconfig.DungeonArea{Min: devconfig.Point{X: x}, Max: devconfig.Point{X: x + 2, Y: 2}},
			PlayerSpawns: []devconfig.Point{{X: x}, {X: x, Y: 1}, {X: x, Y: 2}},
			Enemies:      []devconfig.DungeonEnemy{{Monster: monster, Spawn: devconfig.Point{X: x + 1}}}}
		if index == 0 {
			stage.Enemies = append(stage.Enemies, devconfig.DungeonEnemy{Monster: monster, Spawn: devconfig.Point{X: x + 2, Y: 1}})
		}
		template.Stages = append(template.Stages, stage)
	}
	grant := persistence.DungeonChallenge{ChallengeID: "讨伐链", Execution: 12, TemplateID: template.TemplateID, StartedAt: now,
		ExpiresAt: now.Add(template.Duration), Members: []persistence.DungeonMember{
			{RoleID: "角色一", AccountID: "账号一", Origin: 1, SessionEpoch: 3},
			{RoleID: "角色二", AccountID: "账号二", Origin: 2, SessionEpoch: 3},
		}}
	a := newDungeonArena(grant, template, now)
	first, second := &capturedConnection{}, &capturedConnection{}
	var rooms []*room
	for index, conn := range []*capturedConnection{first, second} {
		member := grant.Members[index]
		role := domain.Role{ID: member.RoleID, Name: member.RoleID, Origin: member.Origin, Job: origin.Role.Job, Level: origin.Role.Level}
		auth := persistence.Auth{RoleID: role.ID, AccountID: member.AccountID, SessionEpoch: member.SessionEpoch}
		frozen := personalBossSnapshot{Fighter: w.basePlayerFighter(origin), Skills: origin.Role.Skills}
		seat := a.addSeat(role, auth, w.e.Store, frozen, uint64(index+10), uint64(index+20), conn)
		if seat == nil {
			t.Fatal("已批准角色未装配到原席位")
		}
		seat.room.Auto = false
		rooms = append(rooms, seat.room)
	}
	return w, a, rooms[0], rooms[1], second, now
}

// TestDungeonBattleTransitionsFromSharedEnemyDeaths 验证每段只能由当前的全部真实敌人死亡推进。
func TestDungeonBattleTransitionsFromSharedEnemyDeaths(t *testing.T) {
	w, a, first, second, _, now := dungeonBattleFixture(t)
	a.captureKills = true
	firstEnemy, lastEnemy := a.monsters[0], a.monsters[1]
	firstEnemy.NextMove, lastEnemy.NextMove = a.grant.ExpiresAt, a.grant.ExpiresAt
	firstEnemy.NextAttack, lastEnemy.NextAttack = a.grant.ExpiresAt, a.grant.ExpiresAt
	nextArea := a.template.Stages[1].PlayerSpawns[0]
	if w.walkable(first, nextArea) || w.free(first, second.Player.Pos, &first.Player) ||
		w.commitRelocation(first, &first.Player, second.Player.Pos) || first.Monsters[0] != second.Monsters[0] {
		t.Fatal("共享场景未阻止未来小区或同队活体占格")
	}
	w.applyDamage(first, &first.Player, firstEnemy, firstEnemy.HP, now)
	if firstEnemy.HP != firstEnemy.MaxHP {
		t.Fatal("首次确认前的攻击改变了共享敌人")
	}
	for _, seat := range a.seats {
		seat.started = true
	}
	w.applyDamage(first, &second.Player, firstEnemy, firstEnemy.HP, now)
	if firstEnemy.HP != firstEnemy.MaxHP {
		t.Fatal("接受了其它席位的冒名攻击")
	}
	w.applyDamage(first, &first.Player, firstEnemy, firstEnemy.HP, now)
	w.applyDungeonDamage(first, &first.Player, firstEnemy, firstEnemy.MaxHP, now, nil) // 原身体已经死亡，重复命中不得再次生成证据。
	w.stepDungeonArena(a, now)
	if a.stage != 0 || second.Monsters[0].HP != 0 || w.walkable(second, nextArea) {
		t.Fatal("第一只敌人死亡就错误推进或同场观察到不同生命值")
	}
	stale := reference(second, lastEnemy)
	staleAttack := pendingAttack{Caster: reference(second, &second.Player), Target: stale, Due: now.Add(time.Second), Amount: 5000}
	second.Attacks = append(second.Attacks, staleAttack)
	w.applyDamage(second, &second.Player, lastEnemy, lastEnemy.HP, now.Add(time.Millisecond))
	w.stepDungeonArena(a, now.Add(time.Millisecond))
	if a.stage != 1 || !w.walkable(first, nextArea) || !w.walkable(second, first.Player.Pos) ||
		w.resolve(second, stale) != nil || len(second.Attacks) != 0 || second.Monsters[0].Handle == lastEnemy.Handle {
		t.Fatal("未正确开放新小区、保留旧小区或隔离原段已经受理的动作")
	}
	hp := a.monsters[0].HP
	w.resolveAttackInWorld(second, second.currentWorld(), staleAttack, now.Add(time.Second))
	if a.monsters[0].HP != hp {
		t.Fatal("旧段攻击击中了新生身体")
	}
	for index := 1; index < 4; index++ {
		at := now.Add(time.Duration(index+1) * time.Second)
		enemy := a.monsters[0]
		w.applyDamage(first, &first.Player, enemy, enemy.HP, at)
		w.stepDungeonArena(a, at)
	}
	finished := now.Add(4 * time.Second)
	a.end(persistence.DungeonSystemInterrupted, finished.Add(time.Second))
	if a.stage != 3 || a.outcome != persistence.DungeonSucceeded || !a.finishedAt.Equal(finished) ||
		len(first.KillFacts)+len(second.KillFacts) != 0 || first.Role.SourceSeq != 0 || second.Role.SourceSeq != 0 {
		t.Fatal("最终段未生成唯一通关终态，或临时伤害泄漏为普通结算")
	}
	expected := []persistence.DungeonKillEvidence{
		{Stage: 0, At: now}, {Stage: 0, Enemy: 1, At: now.Add(time.Millisecond)},
		{Stage: 1, At: now.Add(2 * time.Second)}, {Stage: 2, At: now.Add(3 * time.Second)}, {Stage: 3, At: finished},
	}
	if !a.rewardAt.IsZero() || !slices.Equal(a.killEvidence, expected) {
		t.Fatal("没有奖表也应逐只保存首次死亡，不采纳冒名、未确认或旧段伤害", a.killEvidence)
	}
}

// TestDungeonBattleSharesOfflineCastImpact 确认手动施法和迟到命中通过两位不同观察场景同步。
func TestDungeonBattleSharesOfflineCastImpact(t *testing.T) {
	w, a, first, second, peer, now := dungeonBattleFixture(t)
	for _, seat := range a.seats {
		seat.started, seat.room.Ready = true, true
	}
	other := &pb.SceneBaselineChunk{}
	w.appendDungeonViews(second, other)
	if len(other.Entities) != 1 || other.Entities[0].EntityHandle != first.Player.Handle || other.Entities[0].DisplayName != first.Role.Name {
		t.Fatal("第二位成员的基线缺少第一位真实战斗身体")
	}
	target := a.monsters[0]
	if code := w.cast(first, 1, target.Handle, 37, now); code != 0 {
		t.Fatal("共同施法入口拒绝已学的本场魔法", code)
	}
	start := &pb.CastAccepted{}
	for _, frame := range peer.frames {
		if frame.ID == p.MsgS2CCastAccepted && proto.Unmarshal(frame.Payload, start) != nil {
			t.Fatal("无法解析第二位成员接收的施法")
		}
	}
	if start.SceneEpoch != second.Scene || start.CasterHandle != first.Player.Handle || start.InputSequence != 0 || start.CastId == 0 {
		t.Fatal("同场施法观察泄漏原输入身份或缺少权威动作")
	}
	due := first.Casts[0].Due
	first.Conn = nil
	peer.frames = nil
	w.advanceEvents(first, due)
	if target.HP != target.MaxHP || len(first.Casts) != 1 {
		t.Fatal("单席位路径抢先解析了同场到期动作")
	}
	w.advanceDungeonEvents(a, due)
	impact := &pb.CombatImpact{}
	for _, frame := range peer.frames {
		if frame.ID == p.MsgS2CCombatImpact && proto.Unmarshal(frame.Payload, impact) != nil {
			t.Fatal("无法解析第二位成员接收的命中")
		}
	}
	if target.HP >= target.MaxHP || impact.SceneEpoch != second.Scene || impact.CastId != start.CastId ||
		impact.TargetHandle != target.Handle || impact.TargetLife != target.Life {
		t.Fatal("掉线角色原法术没有通过同场战斗时钟命中或同步")
	}
}

// TestDungeonBattleOrdersDueHitsAcrossSeats 验证后席位更早到期的普攻优先，且不会保留刚清场段的后缀。
func TestDungeonBattleOrdersDueHitsAcrossSeats(t *testing.T) {
	w, a, first, second, _, now := dungeonBattleFixture(t)
	for _, seat := range a.seats {
		seat.started = true
	}
	firstEnemy, target := a.monsters[0], a.monsters[1]
	w.applyDamage(first, &first.Player, firstEnemy, firstEnemy.HP, now)
	first.Player.Pos, second.Player.Pos = devconfig.Point{X: 1, Y: 1}, devconfig.Point{X: 1, Y: 2}
	target.HP = 1
	w.attack(first, &first.Player, target, now.Add(3*time.Millisecond))
	w.attack(second, &second.Player, target, now.Add(time.Millisecond))
	if len(first.Attacks) != 1 || len(second.Attacks) != 1 {
		t.Fatal("公共普攻受理未产生原时间线动作")
	}
	earlier, later := second.Attacks[0].Due, first.Attacks[0].Due
	w.advanceDungeonEvents(a, later)
	if target.HP != 0 || !a.clearedAt.Equal(earlier) || a.stage != 0 || first.InflightEvents != 0 || second.InflightEvents != 0 {
		t.Fatal("未按共享到期顺序裁决清场或未归还动作队列额度")
	}
	w.stepDungeonArena(a, later)
	if a.stage != 1 {
		t.Fatal("合并到期动作之后没有推进下一段")
	}
}

// TestDungeonBattleOfflineAutomaticAttack 在无传输连接时继续原席位的定点和技能队列。
func TestDungeonBattleOfflineAutomaticAttack(t *testing.T) {
	w, a, first, _, _, now := dungeonBattleFixture(t)
	first.Ready, first.Conn, first.Auto = true, nil, true
	first.Role.SkillSlotIDs = []uint32{1, 0, 0, 0}
	origin := first.Player.Pos
	w.stepDungeonArena(a, now)
	if len(first.Casts) != 1 || first.Casts[0].Skill.ID != 1 || first.Player.Pos != origin {
		t.Fatal("已确认离线席位未按保留的技能槽定点发出原技能")
	}
	target := a.monsters[0]
	w.stepDungeonArena(a, first.Casts[0].Due)
	if target.HP >= target.MaxHP || first.Player.Pos != origin || first.Role.SourceSeq != 0 {
		t.Fatal("自动施法没有在同一场命中，或改变了挂机点与永久来源游标")
	}
}

// TestDungeonBattleExpiryAndCompleteDefeat 区分裁决时刻与原超时边界，并保留未死亡的队员。
func TestDungeonBattleExpiryAndCompleteDefeat(t *testing.T) {
	w, a, first, second, _, now := dungeonBattleFixture(t)
	for _, seat := range a.seats {
		seat.started = true
	}
	enemy := a.monsters[0]
	w.applyDamage(first, enemy, &first.Player, first.Player.HP, now)
	if a.outcome != persistence.DungeonActive || first.Player.HP != 0 || second.Player.HP <= 0 {
		t.Fatal("单人死亡不该使全队立即结束")
	}
	w.applyDamage(second, enemy, &second.Player, second.Player.HP, a.grant.ExpiresAt)
	if second.Player.HP <= 0 {
		t.Fatal("挑战截止时刻已经不接受伤害")
	}
	w.applyDamage(second, enemy, &second.Player, second.Player.HP, now.Add(time.Second))
	if a.outcome != persistence.DungeonFailed || !a.finishedAt.Equal(now.Add(time.Second)) {
		t.Fatal("全员失去生命后未按原命中时刻立即确定战败")
	}
	w, a, _, _, _, _ = dungeonBattleFixture(t)
	w.stepDungeonArena(a, a.grant.ExpiresAt.Add(time.Second))
	if a.outcome != persistence.DungeonFailed || !a.finishedAt.Equal(a.grant.ExpiresAt) {
		t.Fatal("超时应保留原截止时刻")
	}
}
