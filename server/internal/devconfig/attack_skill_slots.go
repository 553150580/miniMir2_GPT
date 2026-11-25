// 本文件声明可以进入攻击队列的已支持技能行为。
package devconfig

const AttackSkillSlotCount = 4 // 每个角色的固定攻击技能槽数。

// AttackSlotSummon 表示可以显式排队的新生召唤，历史自动目录仍读取 AutoEnabled。
func (s Skill) AttackSlotSummon() bool {
	return s.Behavior == "summon_pet" && s.Summon != nil && s.Activation() == "cast"
}

// AttackSlotEligible 保留定点战斗与手动控制的边界，另外允许原战士刀法和显式召唤。
func (s Skill) AttackSlotEligible() bool {
	switch s.Behavior {
	case "half_moon", "thrusting", "flaming_sword":
		return true
	}
	return s.AttackSlotSummon() || s.AutoEnabled && s.Activation() == "cast" && s.Displacement == nil && s.Teleport == nil
}
