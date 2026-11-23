// 本文件处理隐身的当前生命、怪物冷眼、定向暴露及走动解除。
package game

// hidingStatus 返回当前生命仍有效的原隐身状态。
func hidingStatus(actor *entity) *combatStatus {
	if actor == nil {
		return nil
	}
	for _, status := range actor.Statuses {
		if status.Kind == "hiding" && status.TargetLife == actor.Life && status.RemainingMS > 0 {
			return status
		}
	}
	return nil
}

// breakHidingOnWalk 在原实体成功行走时解除隐身，强制推挤及单独改朝向不走此入口。
func breakHidingOnWalk(actor *entity) {
	if hiding := hidingStatus(actor); hiding != nil {
		delete(actor.Statuses, hiding.ID)
	}
}

// exposeHiddenAttacker 允许原本已被隐身阻隔的怪物对直接攻击自己的隐身者重新建立目标。
func exposeHiddenAttacker(world *worldState, attacker, target *entity) {
	if target.Kind != 3 || attacker == nil || attacker.Kind != 1 {
		return
	}
	// hiding 状态只在角色仍是原施法生命时存在。
	hiding := hidingStatus(attacker)
	if hiding == nil {
		return
	}
	if hiding.HiddenAware == nil {
		hiding.HiddenAware = map[uint32]actorRef{}
	}
	// 固定按句柄覆盖旧出生引用，避免复活后的怪物继承知觉或导致列表增长。
	hiding.HiddenAware[target.Handle] = referenceIn(world, target)
}

// canSeeFightTarget 判断该怪物能否在当前生命上察觉目标。冷眼还要求怪物不低于目标等级。
func canSeeFightTarget(world *worldState, monster, target *entity, targetLevel uint32) bool {
	if target == nil || target.HP <= 0 {
		return false
	}
	// hiding 不存在时仍按原地图的普通索敌策略处理。
	hiding := hidingStatus(target)
	return hiding == nil || monster.Monster != nil && monster.Monster.CoolEye && monster.Monster.Level >= targetLevel ||
		hiding.HiddenAware[monster.Handle] == referenceIn(world, monster)
}

// visibleMonsterTarget 保持原角色和原宠物之间的距离优先级，再排除当前不可见的候选。
func visibleMonsterTarget(world *worldState, monster, player, pet *entity, playerLevel uint32) *entity {
	// pVisible 表示当前怪物能看到该玩家，petLevel 保存宠物原生定义等级。
	pVisible := canSeeFightTarget(world, monster, player, playerLevel)
	petLevel := uint32(0)
	if pet != nil && pet.Monster != nil {
		petLevel = pet.Monster.Level
	}
	if canSeeFightTarget(world, monster, pet, petLevel) && (!pVisible || distance(monster.Pos, pet.Pos) < distance(monster.Pos, player.Pos)) {
		return pet
	}
	if pVisible {
		return player
	}
	return nil
}
