// 本文件处理困魔咒的两次原地格核验、怪物主动行为暂停及解除。
package game

import (
	"time"

	"minimir/internal/devconfig"
)

// trapHexagonScope 只向普通个人战斗开放隔离围困，场次首领与共享城镇不参与。
func (w *worker) trapHexagonScope(r *room, world *worldState) bool {
	if r.bossBattle != nil || r.worldBossCombat != nil || r.worldBossObservation != nil || world == nil || world != r.currentWorld() {
		return false
	}
	m, ok := w.worldMap(world)
	return ok && m.SceneKindValue() == devconfig.SceneKindPersonal
}

// trapHexagonTarget 核对原世界活怪、等级差与围困地格。原受理只扫描3×2，到期按3×3取实际仍在格内的怪。
func trapHexagonTarget(monster *entity, point devconfig.Point, playerLevel uint32, applying bool) bool {
	if monster == nil || monster.Kind != 3 || monster.Monster == nil || monster.HP <= 0 || uint64(monster.Monster.Level) > uint64(playerLevel)+2 {
		return false
	}
	x, y := monster.Pos.X-point.X, monster.Pos.Y-point.Y
	return x >= -1 && x <= 1 && y >= -1 && (y <= 0 || applying && y == 1)
}

// hasTrapHexagonTarget 在扣料前确认来源要求的原地格确有候选活怪。
func hasTrapHexagonTarget(world *worldState, point devconfig.Point, playerLevel uint32) bool {
	for _, monster := range world.Monsters[:min(world.MonsterCount, len(world.Monsters))] {
		if trapHexagonTarget(monster, point, playerLevel, false) {
			return true
		}
	}
	return false
}

// resolveTrapHexagon 在原施法者与原世界仍有效时建立围困，不制造伤害或新收益。
func (w *worker) resolveTrapHexagon(r *room, world *worldState, caster *entity, cast pendingCast, now time.Time) {
	if caster != &r.Player || caster.HP <= 0 || cast.Due.IsZero() || now.Before(cast.Due) || !w.trapHexagonScope(r, world) {
		return
	}
	duration := devconfig.TrapHexagonDuration(cast.SkillLevel)
	until := cast.Due.Add(duration)
	if duration == 0 || !now.Before(until) {
		return
	}
	for _, monster := range world.Monsters[:min(world.MonsterCount, len(world.Monsters))] {
		if !trapHexagonTarget(monster, cast.TargetPoint, r.Role.Level, true) || !w.worldWalkable(world, monster.Pos) {
			continue
		}
		if world.TrapHexagon == nil {
			world.TrapHexagon = make(map[actorRef]time.Time)
		}
		world.TrapHexagon[referenceIn(world, monster)] = until
		w.impactInWorld(r, world, cast.Scene, cast.Skill.ID, caster.Handle, monster.Handle, monster.Life, monster.Pos, 0, 1, false, cast.ID, 0)
	}
}

// trapHexagonHeld 判断本出生代次能否继续产生新的移动和攻击事实。
// 当前房间逐帧重算怪物目标，候选在围困期间只保留已经排队的动作，不模拟来源独立的近身重新索敌时钟。
func (world *worldState) trapHexagonHeld(monster *entity, now time.Time) bool {
	return len(world.TrapHexagon) != 0 && now.Before(world.TrapHexagon[referenceIn(world, monster)])
}

// clearTrapHexagon 在原实体受到有效直接命中时解除，重生实体不继承旧围困身份。
func (world *worldState) clearTrapHexagon(monster *entity) {
	if world == nil || len(world.TrapHexagon) == 0 || monster.Kind != 3 {
		return
	}
	delete(world.TrapHexagon, referenceIn(world, monster))
	if len(world.TrapHexagon) == 0 {
		world.TrapHexagon = nil
	}
}

// expireTrapHexagons 在原世界时钟到期后归还临时空间。
func (world *worldState) expireTrapHexagons(now time.Time) {
	if len(world.TrapHexagon) == 0 {
		return
	}
	for target, until := range world.TrapHexagon {
		if !now.Before(until) {
			delete(world.TrapHexagon, target)
		}
	}
	if len(world.TrapHexagon) == 0 {
		world.TrapHexagon = nil
	}
}
