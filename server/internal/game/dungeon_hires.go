// 本文件只装配开战时已确认的讨伐佣兵身体，使用同场的原角色战斗与寻路规则。
package game

import (
	"fmt"
	"slices"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// addDungeonHires 按原真人名单之后的空席装配佣兵。全部合法才发布阵容。
func (w *worker) addDungeonHires(arena *dungeonArena) bool {
	hireIDs := arena.grant.HireOptionIDs
	if len(hireIDs) == 0 {
		return true
	}
	players := len(arena.grant.Members)
	if arena.outcome != persistence.DungeonActive || arena.stage != 0 || len(arena.hireSeats) != 0 ||
		players == 0 || len(arena.seats) != players || players+len(hireIDs) > devconfig.DungeonMaxPlayers {
		return false
	}
	spawns := arena.template.Stages[0].PlayerSpawns
	if len(spawns) < players+len(hireIDs) || arena.store == nil {
		return false
	}
	hired := make([]*dungeonCombatSeat, 0, len(hireIDs))
	for offset, id := range hireIDs {
		option, ok := arena.template.HireOption(id)
		index := players + offset
		origin := spawns[index]
		if !ok || !arena.geometry.Walkable(origin) || slices.Contains(spawns[:index], origin) || arena.occupied(origin, nil) {
			return false
		}
		hire := w.dungeonHireSeat(arena, index, option, origin)
		if hire == nil {
			return false
		}
		hired = append(hired, hire)
	}
	for _, seat := range hired {
		arena.seats[seat.room.Role.ID] = seat
	}
	arena.hireSeats = hired
	return true
}

// dungeonHireSeat 按基础职业、原等级成长和已学技能构造有限资源的临时同伴。
func (w *worker) dungeonHireSeat(arena *dungeonArena, index int, option devconfig.DungeonHireOption, origin devconfig.Point) *dungeonCombatSeat {
	role := domain.Role{ID: fmt.Sprintf("dungeon-hire:%d", index), Name: option.DisplayName, Job: option.Job, Level: option.Level}
	slots := [devconfig.AttackSkillSlotCount]uint32{}
	count := 0
	for _, skill := range option.Skills {
		definition, ok := arena.store.Cfg.Fixture.Skill(skill.ID)
		if !ok || definition.JobID != option.Job {
			return nil
		}
		role.Skills = append(role.Skills, domain.LearnedSkill{SkillID: skill.ID, Level: skill.Rank})
		if !definition.AttackSlotEligible() {
			continue // 被动技能进入原属性或普攻判定，不占用四个主动槽。
		}
		if count == len(slots) {
			return nil
		}
		slots[count] = skill.ID
		count++
	}
	if count != 0 {
		role.SkillSlotIDs = slots[:]
	}
	ref := worldRef{Epoch: arena.grant.Execution, ID: 1}
	world := &worldState{Ref: ref, MapID: arena.geometry.ID, BossMap: &arena.geometry, Monsters: arena.monsters,
		MonsterCount: len(arena.monsters), Auto: true}
	child := &room{worldState: world, runStore: arena.store, ExecutionEpoch: arena.grant.Execution, WorldSequence: ref.ID, Role: role,
		LastTick: arena.lastTick, LastBroadcast: arena.lastTick, dungeonCombat: arena}
	fighter := w.basePlayerFighter(child)
	if child.Isolated || fighter.HP <= 0 || fighter.MoveMS <= 0 || fighter.AttackMS <= 0 {
		return nil
	}
	child.Player = entity{Handle: uint32(index + 1), Kind: 1, Def: role.Job, Pos: origin,
		HP: fighter.HP, MaxHP: fighter.HP, MP: fighter.MP, MaxMP: fighter.MP, Life: 1}
	child.bossBattle = &personalBossBattle{fighter: fighter}
	return &dungeonCombatSeat{room: child, started: true, hired: true}
}

// advanceDungeonHire 在标准技能和普攻之后接近敌人，服从执行场的占格、行动和冷却时钟。
func (w *worker) advanceDungeonHire(child *room, now time.Time) {
	if child.Isolated || !child.dungeonCombat.acceptsActions(now) || now.Before(child.Player.NextMove) || now.Before(child.Player.ActionReadyAt) {
		return
	}
	target := child.liveAutoTarget()
	if target == nil {
		target = w.nearest(child, &child.Player)
	}
	if target != nil && distance(child.Player.Pos, target.Pos) > 1 {
		w.move(child, &child.Player, target.Pos, now, w.fighter(child, &child.Player).MoveMS)
	}
}
