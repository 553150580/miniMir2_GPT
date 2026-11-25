// 本文件统一共享场次中多个成员的原世界到期事件队列。
package game

import (
	"sort"
	"time"
)

// sharedCombatDue 保留到期动作的原执行世界与角色上下文。
type sharedCombatDue struct {
	room  *room    // 原先受理该动作的席位。
	event dueEvent // 公共攻击或施法解析器接收的原动作。
}

// collectSharedCombatEvents 只摘取已到期事件，在移出到期批次之前持续占用原队列额度。
func collectSharedCombatEvents(all []sharedCombatDue, child *room, until time.Time) []sharedCombatDue {
	world := child.currentWorld()
	attacks := world.Attacks[:0]
	for _, attack := range world.Attacks {
		if attack.Due.After(until) {
			attacks = append(attacks, attack)
			continue
		}
		copy := attack
		all = append(all, sharedCombatDue{room: child, event: dueEvent{World: world, At: copy.Due, Order: copy.Order, Attack: &copy}})
		child.InflightEvents++
	}
	casts := world.Casts[:0]
	for _, cast := range world.Casts {
		if cast.Due.After(until) {
			casts = append(casts, cast)
			continue
		}
		copy := cast
		all = append(all, sharedCombatDue{room: child, event: dueEvent{World: world, At: copy.Due, Order: copy.Order, Cast: &copy}})
		child.InflightEvents++
	}
	clear(world.Casts[len(casts):])
	world.Attacks, world.Casts = attacks, casts
	return all
}

// sortSharedCombatEvents 按到期时钟、原席位句柄和本席位受理序号决定命中顺序。
func sortSharedCombatEvents(all []sharedCombatDue) {
	sort.SliceStable(all, func(i, j int) bool {
		left, right := all[i], all[j]
		if !left.event.At.Equal(right.event.At) {
			return left.event.At.Before(right.event.At)
		}
		if left.room.Player.Handle != right.room.Player.Handle {
			return left.room.Player.Handle < right.room.Player.Handle
		}
		return left.event.Order < right.event.Order
	})
}
