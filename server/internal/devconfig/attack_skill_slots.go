// 本文件声明可以进入攻击队列的已支持技能行为。
package devconfig

const AttackSkillSlotCount = 4 // 每个角色的固定攻击技能槽数。

// AttackSlotEligible 保留定点战斗与手动控制的边界，并允许原战士刀法。
func (s Skill) AttackSlotEligible() bool {
	switch s.Behavior {
	case "half_moon", "thrusting", "flaming_sword":
		return true
	}
	return s.AutoEnabled && s.Activation() == "cast" && s.Displacement == nil && s.Teleport == nil
}
