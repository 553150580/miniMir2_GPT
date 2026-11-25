// 本文件在同一讨伐场中解析友方玩家、驻留活宠及自动范围支援的落点。
package game

import (
	"sort"

	"minimir/internal/devconfig"
)

// dungeonAlly 把友方的原身体与其冻结战斗属性所属的席位一起返回。
type dungeonAlly struct {
	owner *room   // 该身体所属的原战斗席位。
	body  *entity // 该席位的角色或当前场内驻留宠物。
}

// dungeonAllies 只枚举同一场中曾确认入场且仍在本段的原身体，保留原自动规则的自身优先。
func (r *room) dungeonAllies() []dungeonAlly {
	a := r.dungeonCombat
	source := a.seats[r.Role.ID]
	if source == nil || source.room != r || !source.started || r.Isolated {
		return nil
	}
	allies := make([]dungeonAlly, 0, len(a.seats)*2)
	appendSeat := func(seat *dungeonCombatSeat) {
		peer := seat.room
		world := peer.currentWorld()
		if !seat.started || peer.Isolated || world.Ref != r.Ref {
			return
		}
		allies = append(allies, dungeonAlly{owner: peer, body: &peer.Player})
		if pet := peer.worldPet(world); pet != nil && !pet.IsPetCorpse {
			allies = append(allies, dungeonAlly{owner: peer, body: pet})
		}
	}
	appendSeat(source)
	a.eachSeat(func(seat *dungeonCombatSeat) {
		if seat != source {
			appendSeat(seat)
		}
	})
	return allies
}

// dungeonAreaAllies 按到期时的当前地格和原技能范围收集目标，顺序由场内句柄固定。
func (w *worker) dungeonAreaAllies(r *room, skill devconfig.Skill, center devconfig.Point, allowDead bool) []*entity {
	var targets []*entity
	for _, ally := range r.dungeonAllies() {
		body := ally.body
		if (allowDead || body.HP > 0) && distance(body.Pos, center) <= skill.AreaRadius && w.walkable(r, body.Pos) {
			targets = append(targets, body)
		}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Handle < targets[j].Handle })
	return targets
}

// dungeonSupportCenter 选择射程内覆盖最多待支援身体的友方地格，并保留技能原最低目标数。
func (w *worker) dungeonSupportCenter(r *room, skill devconfig.Skill, needsEffect func(*entity) bool) (devconfig.Point, bool) {
	allies := r.dungeonAllies()
	var center devconfig.Point
	best := 0
	for _, candidate := range allies {
		body := candidate.body
		if !needsEffect(body) || distance(r.Player.Pos, body.Pos) > skill.RangeCells || !w.walkable(r, body.Pos) ||
			skillRequiresFlight(skill) && !w.lineOfSight(r, r.Player.Pos, body.Pos) {
			continue
		}
		count := 0
		for _, ally := range allies {
			target := ally.body
			if needsEffect(target) && distance(body.Pos, target.Pos) <= skill.AreaRadius && w.walkable(r, target.Pos) {
				count++
			}
		}
		if count > best {
			center, best = body.Pos, count
		}
	}
	return center, best > 0 && best >= skill.AutoMinTargets
}
