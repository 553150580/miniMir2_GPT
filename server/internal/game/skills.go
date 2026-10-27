package game

import (
	"minimir/internal/devconfig"
	"time"
)

type pendingCast struct {
	Scene             uint64
	Skill             devconfig.Skill
	Target            uint32
	TargetLife        uint64
	TargetPoint       devconfig.Point
	Due               time.Time
	Amount            int64
	Caster            actorRef
	ID                uint64
	Order             uint64
	HostSession       string
	PrivilegeUntil    time.Time
	PrivilegeCaptured bool
	Direction         uint32
	LineRemaining     int
	SkillLevel        uint32
}

func acceptInput(r *room, sequence uint32) bool {
	if sequence == 0 || sequence <= r.InputSequence {
		return false
	}
	r.InputSequence = sequence
	return true
}
func (w *worker) draw(n int64) int64 {
	if n <= 1 {
		return 0
	}
	if w.roll != nil {
		return w.roll(n)
	}
	return random(n)
}
func (w *worker) interval(min, max int64) int64 {
	if min < 0 {
		min = 0
	}
	if max < min {
		max = min
	}
	return min + w.draw(max-min+1)
}

func (w *worker) refreshPlayerStats(r *room) {
	if r.Player.Kind != 1 {
		return
	}
	a := w.fighter(r, &r.Player)
	r.Player.MaxHP, r.Player.MaxMP = a.HP, a.MP
	if r.Player.HP > a.HP {
		r.Player.HP = a.HP
	}
	if r.Player.MP > a.MP {
		r.Player.MP = a.MP
	}
}

func (w *worker) physicalDamage(r *room, from, target *entity) int64 {
	a, b := w.fighter(r, from), w.fighter(r, target)
	if w.draw(b.Agility+1) > a.Accuracy {
		return 0
	}
	damage := w.interval(a.AttackMin, a.AttackMax) - w.interval(b.DefenceMin, b.DefenceMax)
	if damage < 0 {
		return 0
	}
	return damage
}
func (w *worker) magicDamage(r *room, target *entity, amount int64) int64 {
	b := w.fighter(r, target)
	if b.MagicResist > 0 && w.draw(b.MagicResistWeight) < b.MagicResist {
		return 0
	}
	amount -= w.interval(b.MagicDefenceMin, b.MagicDefenceMax)
	if amount < 0 {
		return 0
	}
	return amount
}

// Crystal MagicInfo.cs:180–211：基础区间、技能随机项、四分之一取偶舍入及倍率分开。
func (w *worker) magicAmount(s devconfig.Skill, level uint32, base int64) int64 {
	mpower := s.MPowerBase + w.draw(s.MPowerBonus)
	power := s.PowerBase + w.draw(s.PowerBonus)
	return s.SourceDamage(level, base, s.SourcePower(level, mpower, power))
}

func (w *worker) skillTarget(r *room, s devconfig.Skill, handle uint32) *entity {
	if s.Behavior == "healing" {
		if handle == 0 || handle == r.Player.Handle {
			if r.Player.HP > 0 {
				return &r.Player
			}
			return nil
		}
		if r.Pet != nil && r.Pet.Handle == handle && r.Pet.HP > 0 {
			return r.Pet
		}
		return nil
	}
	for _, m := range r.Monsters[:r.MonsterCount] {
		if m.Handle == handle && m.HP > 0 {
			return m
		}
	}
	return nil
}
func (w *worker) lineOfSight(from, to devconfig.Point) bool {
	if !w.walkable(to) {
		return false
	}
	for from != to {
		dx, dy := to.X-from.X, to.Y-from.Y
		if dx > 0 {
			from.X++
		} else if dx < 0 {
			from.X--
		}
		if dy > 0 {
			from.Y++
		} else if dy < 0 {
			from.Y--
		}
		if !w.walkable(from) {
			return false
		}
	}
	return true
}

func (w *worker) tickHealing(r *room, now time.Time) bool {
	h := w.e.Store.Cfg.Fixture.Healing
	if h.TickMS <= 0 || h.LevelsPerStep <= 0 || h.QueueFractionDivisor <= 0 {
		return false
	}
	changed := false
	for _, e := range []*entity{&r.Player, r.Pet} {
		if e == nil || e.HP <= 0 {
			continue
		}
		before := e.HP
		if now.After(e.NextHeal) {
			e.NextHeal = now.Add(time.Duration(h.TickMS) * time.Millisecond)
			rate := h.PetBasePerTick
			if e.Kind == 1 {
				rate = h.BasePerTick + int64(r.Role.Level)/h.LevelsPerStep + e.HealAmount/h.QueueFractionDivisor
			}
			e.HP, e.HealAmount = restorePool(e.HP, e.MaxHP, e.HealAmount, rate)
		}
		if e.HP >= e.MaxHP {
			e.HealAmount = 0
		}
		changed = changed || before != e.HP
	}
	return changed
}
