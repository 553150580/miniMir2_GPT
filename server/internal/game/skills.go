package game

import (
	"math"
	"math/big"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"time"
)

type pendingCast struct {
	Scene       uint64
	Skill       devconfig.Skill
	Target      uint32
	TargetLife  uint64
	TargetPoint devconfig.Point
	Due         time.Time
	Amount      int64
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
	numerator := mpower*(int64(level)+1) + power*4
	whole, remainder := numerator/4, numerator%4
	if remainder > 2 || (remainder == 2 && whole%2 != 0) {
		whole++
	}
	value := big.NewInt(base + whole)
	value.Mul(value, big.NewInt(s.MultiplierBasePPM+int64(level)*s.MultiplierBonusPPM))
	value.Quo(value, big.NewInt(1000000))
	if value.Cmp(big.NewInt(math.MaxInt32)) > 0 {
		return math.MaxInt32
	}
	return value.Int64()
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

func (w *worker) cast(r *room, id, targetHandle, input uint32, now time.Time) uint32 {
	s, exists := w.e.Store.Cfg.Fixture.Skill(id)
	level, learned := r.Role.Skill(id)
	if !exists || !learned || s.JobID != r.Role.Job || level != s.LearnedLevel {
		return p.CodeSkillNotLearned
	}
	if s.Behavior == "fencing" || r.Pending || r.Player.HP <= 0 || now.Before(r.Player.NextMove) || len(r.Casts) >= 64 {
		return p.CodeSkillStateBlocked
	}
	if now.Before(r.NextSpell) || now.Before(r.SkillCooldowns[id]) {
		return p.CodeSkillCooldown
	}
	target := w.skillTarget(r, s, targetHandle)
	if target == nil || distance(r.Player.Pos, target.Pos) > s.RangeCells {
		return p.CodeSkillTarget
	}
	if s.Behavior == "fireball" && !w.lineOfSight(r.Player.Pos, target.Pos) {
		return p.CodeSkillTarget
	}
	cost := s.MPBaseCost + int64(level)*s.MPLevelCost
	if r.Player.MP < cost {
		return p.CodeSkillResource
	}
	stats := w.fighter(r, &r.Player)
	amount := int64(0)
	if s.Behavior == "fireball" {
		amount = w.magicAmount(s, level, w.interval(stats.MagicMin, stats.MagicMax))
	} else {
		amount = w.magicAmount(s, level, w.interval(stats.TaoMin, stats.TaoMax)*2) + int64(r.Role.Level)
	}
	delay := s.DelayBaseMS + int64(distance(r.Player.Pos, target.Pos))*s.ProjectileMSPerCell
	if delay > math.MaxUint32 || r.CastID == math.MaxUint64 {
		return p.CodeSkillStateBlocked
	}
	r.Player.MP -= cost
	r.Player.Action = 5
	r.Player.ActionUntil = now.Add(time.Duration(s.ActionMS) * time.Millisecond)
	r.Player.NextMove = r.Player.ActionUntil
	r.Player.NextAttack = r.Player.ActionUntil
	r.Player.CastSequence++
	r.Player.CastSkillID = id
	r.CastID++
	for i, d := range directions {
		dx, dy := target.Pos.X-r.Player.Pos.X, target.Pos.Y-r.Player.Pos.Y
		if (dx == 0 && d.X == 0 || dx > 0 && d.X > 0 || dx < 0 && d.X < 0) && (dy == 0 && d.Y == 0 || dy > 0 && d.Y > 0 || dy < 0 && d.Y < 0) {
			r.Player.Direction = uint32(i)
			break
		}
	}
	r.NextSpell = now.Add(time.Duration(s.GlobalSpellMS) * time.Millisecond)
	if r.SkillCooldowns == nil {
		r.SkillCooldowns = map[uint32]time.Time{}
	}
	r.SkillCooldowns[id] = now.Add(time.Duration(s.CooldownBaseMS-int64(level)*s.CooldownReductionMS) * time.Millisecond)
	r.Casts = append(r.Casts, pendingCast{Scene: r.Scene, Skill: s, Target: target.Handle, TargetLife: target.Life, TargetPoint: target.Pos, Due: now.Add(time.Duration(delay) * time.Millisecond), Amount: amount})
	w.send(r, p.MsgS2CCastAccepted, 0, &pb.CastAccepted{SceneEpoch: r.Scene, CastId: r.CastID, SkillId: id, CasterHandle: r.Player.Handle, TargetHandle: target.Handle, Origin: pointPB(r.Player.Pos), TargetGrid: pointPB(target.Pos), Direction: r.Player.Direction, DelayMs: uint32(delay), InputSequence: input})
	w.snapshot(r)
	return 0
}

func (w *worker) advanceCasts(r *room, now time.Time) {
	remaining := r.Casts[:0]
	for i, c := range r.Casts {
		if r.Pending {
			remaining = append(remaining, r.Casts[i:]...)
			break
		}
		if now.Before(c.Due) {
			remaining = append(remaining, c)
			continue
		}
		if c.Scene != r.Scene {
			continue
		}
		target := w.skillTarget(r, c.Skill, c.Target)
		if target == nil || target.Life != c.TargetLife {
			continue
		}
		if c.Skill.Behavior == "fireball" {
			if distance(target.Pos, c.TargetPoint) > c.Skill.TargetMoveToleranceCells {
				continue
			}
			damage := w.magicDamage(r, target, c.Amount)
			if damage > 0 {
				w.applyDamage(r, &r.Player, target, damage, now)
			}
		} else if target.HP < target.MaxHP {
			target.HealAmount = queueAdd(target.HealAmount, c.Amount, w.e.Store.Cfg.Fixture.Healing.QueueCap)
		}
	}
	r.Casts = remaining
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

func (w *worker) autoSkill(r *room, now time.Time) (*entity, bool) {
	for _, s := range w.e.Store.Cfg.Fixture.Skills {
		if s.JobID != r.Role.Job {
			continue
		}
		if _, ok := r.Role.Skill(s.ID); !ok {
			continue
		}
		if s.Behavior == "healing" {
			target := &r.Player
			if target.HP >= target.MaxHP && r.Pet != nil && r.Pet.HP > 0 && r.Pet.HP < r.Pet.MaxHP {
				target = r.Pet
			}
			if target.HP < target.MaxHP && target.HealAmount == 0 && w.cast(r, s.ID, target.Handle, 0, now) == 0 {
				return nil, true
			}
		} else if s.Behavior == "fireball" {
			target := w.nearest(r, &r.Player)
			if target == nil {
				return nil, true
			}
			if w.cast(r, s.ID, target.Handle, 0, now) == 0 {
				return nil, true
			}
			return target, true
		}
	}
	return nil, false
}
