// 本文件处理火墙的原世界驻留、正交地格、原伤害公式与入格触发。
package game

import (
	"math"
	"slices"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
)

// fireWallTickInterval 是 Crystal 地格对象再次处理同格居民的固定间隔。
const fireWallTickInterval = 2 * time.Second

// fireWallField 保存一次施法实际建立的地格和原始威力，物理阻挡仍由地图与实体处理。
type fireWallField struct {
	Caster        actorRef           // 初次建场的施法者身份。
	Scene, CastID uint64             // 用于当前观察过滤和识别事件来源。
	SkillID       uint32             // 伤害事件携带的技能编号。
	Amount        int64              // 施法受理时冻结的原始威力。
	Expires       time.Time          // 到该时刻仍可命中，之后该场消失。
	Cells         [5]devconfig.Point // 从中心开始保存逐格建立的有效地面位置。
	Count         int                // Cells 中已实际建立的位置数量。
}

// fireWallLifetime 按来源的整数除法算出持续时长，拒绝无法由房间时钟表示的威力。
func fireWallLifetime(amount int64) time.Duration {
	// seconds 保存公式中的整数秒，10 为基础时长；除法使用命中防御前的冻结威力。
	if amount < 0 || amount/2 > math.MaxInt64/int64(time.Second)-10 {
		return 0
	}
	seconds := 10 + amount/2
	return time.Duration(seconds) * time.Second
}

// covers 查询该场是否包含 point，地形阻挡或同格已有火墙时该点未被写入。
func (field *fireWallField) covers(point devconfig.Point) bool {
	return slices.Contains(field.Cells[:field.Count], point)
}

// fireWallCells 投影当前实体能踏入的有效火墙格，排除尚未生效、已离图和过期地格。
func (world *worldState) fireWallCells(now time.Time) []*pb.GridPoint {
	if world == nil {
		return nil
	}
	var points []*pb.GridPoint
	for _, field := range world.FireWalls {
		if !now.After(field.Expires) {
			for _, cell := range field.Cells[:field.Count] {
				points = append(points, pointPB(cell))
			}
		}
	}
	return points
}

// addFireWall 建立中心和四邻接触格。同格旧火墙保留其原伤害、拥有者和期限。
func (w *worker) addFireWall(world *worldState, cast pendingCast, now time.Time) *fireWallField {
	// lifetime 使用受理时冻结的源威力，抵抗或防御不会缩短地格寿命。
	lifetime := fireWallLifetime(cast.Amount)
	if lifetime == 0 {
		return nil
	}
	// field 记录第一次处理时刻起算的原场期限，独立于网络延迟与施法动作长度。
	field := &fireWallField{Caster: cast.Caster, Scene: cast.Scene, CastID: cast.ID, SkillID: cast.Skill.ID, Amount: cast.Amount, Expires: now.Add(lifetime)}
	// points 是来源的中心、北、东、南、西建立顺序。
	points := [5]devconfig.Point{
		cast.TargetPoint, stepPoint(cast.TargetPoint, 0, 1), stepPoint(cast.TargetPoint, 2, 1),
		stepPoint(cast.TargetPoint, 4, 1), stepPoint(cast.TargetPoint, 6, 1),
	}
	for _, point := range points {
		if !w.worldWalkable(world, point) || world.fireWallAt(point, now) != nil ||
			world.mirrorCombat != nil && world.mirrorCombat.sides[1-world.mirrorSide].room.currentWorld().fireWallAt(point, now) != nil {
			continue
		}
		field.Cells[field.Count] = point
		field.Count++
	}
	if field.Count == 0 {
		return nil
	}
	world.FireWalls = append(world.FireWalls, field)
	return field
}

// fireWallAt 查找当前尚未过期的场；镜像两侧分别持有自己的技能后继。
func (world *worldState) fireWallAt(point devconfig.Point, now time.Time) *fireWallField {
	for _, field := range world.FireWalls {
		if !now.After(field.Expires) && field.covers(point) {
			return field
		}
	}
	return nil
}

// resolveFireWall 执行首次放置或后续地格处理。每个活场只占共同时间线的一个后继事件。
func (w *worker) resolveFireWall(r *room, world *worldState, caster *entity, cast pendingCast, now time.Time) {
	// field 为空代表初次放置。非空时必须仍持有原世界注册，拒绝处理已离图清除的旧批次事件。
	field := cast.FireWall
	if world != r.currentWorld() || caster != &r.Player || field != nil && now.After(field.Expires) {
		if field != nil {
			world.FireWalls = slices.DeleteFunc(world.FireWalls, func(existing *fireWallField) bool { return existing == field })
		}
		return
	}
	if field == nil {
		field = w.addFireWall(world, cast, now)
		if field == nil {
			return
		}
	} else if !slices.Contains(world.FireWalls, field) {
		return
	}
	if r.ScheduleID == math.MaxUint64 {
		w.isolateRoom(r)
		return
	}
	// next 从实际处理时间推进，队列迟到只处理一次。期限尾段仅安排清理，不补造一次伤害。
	next := cast
	next.FireWall, next.Due = field, now.Add(fireWallTickInterval)
	if next.Due.After(field.Expires) {
		next.Due = field.Expires.Add(time.Nanosecond)
	}
	// 后继在创建时携带下一事件的区段，未来掉线或续卡沿共同时间线改尚未到期的归属。
	next.HostSession = ""
	if r.Conn == nil {
		next.HostSession = r.HostSession
	}
	next.PrivilegeUntil, next.PrivilegeCaptured = r.Role.PrivilegeUntil, true
	r.ScheduleID++
	next.Order = r.ScheduleID
	world.Casts = append(world.Casts, next)
	// context 保留当前到期事件已经冻结的托管和绑定条件，即使它被晚推进也不重判。
	context := hitContext{At: cast.Due, HostSession: cast.HostSession, PrivilegeUntil: cast.PrivilegeUntil, PrivilegeCaptured: cast.PrivilegeCaptured}
	for _, point := range field.Cells[:field.Count] {
		for _, target := range world.combatOpponents() {
			if target != nil && target.Pos == point {
				w.fireWallHit(r, world, caster, target, field, now, context)
				if r.Pending || r.Isolated {
					return
				}
			}
		}
	}
}

// fireWallHit 以放置时的原始威力扣除当前目标魔防，沿统一命中入口记录伤害和击杀。
func (w *worker) fireWallHit(r *room, world *worldState, caster, target *entity, field *fireWallField, now time.Time, context hitContext) {
	if target.HP <= 0 || (target.Kind != 3 || target.Monster == nil) &&
		(world.mirrorCombat == nil || !slices.Contains(world.combatOpponents(), target)) {
		return
	}
	// before 记录本次处理前的HP，伤害表现仅使用实际减少的生命值。
	before := target.HP
	// damage 是根据当前防御判定得到的伤害。
	damage := w.magicDamage(r, target, field.Amount)
	w.applyDamageInWorld(r, world, caster, target, damage, now, context)
	// outcome 对应通用技能协议中的伤害成功或抵抗。
	outcome := uint32(2)
	if target.HP < before {
		outcome = 1
	}
	w.impactInWorld(r, world, field.Scene, field.SkillID, caster.Handle, target.Handle, target.Life, target.Pos,
		before-target.HP, outcome, false, field.CastID, 0)
}

// enterFireWall 在怪物真正进入有效格时处理一次，不重置当前场的周期时间。
func (w *worker) enterFireWall(r *room, target *entity, now time.Time) {
	if target.HP <= 0 || r.Pending || r.Isolated {
		return
	}
	// world 是本次移动提交的活动世界，历史世界里的火墙不参与查询。
	world := r.currentWorld()
	if b := world.mirrorCombat; b != nil {
		other := b.sides[1-world.mirrorSide].room
		original := other.currentWorld()
		if field := original.fireWallAt(target.Pos, now); field != nil {
			w.fireWallHit(other, original, &other.Player, target, field, now, hitContext{At: now})
		}
		return
	}
	if target.Kind != 3 {
		return
	}
	for _, field := range world.FireWalls {
		if now.After(field.Expires) || !field.covers(target.Pos) || field.Caster != referenceIn(world, &r.Player) {
			continue
		}
		// context 对入格立即命中冻结真实移动时间及当前附着区段。
		context := hitContext{At: now, PrivilegeUntil: r.Role.PrivilegeUntil, PrivilegeCaptured: true}
		if r.Conn == nil {
			context.HostSession = r.HostSession
		}
		w.fireWallHit(r, world, &r.Player, target, field, now, context)
		return // 同格只允许一个未过期的火墙。
	}
}

// clearFireWalls 清除已离开的地图上的火墙和各自后继，保留下单次放置及其它动作的原归属。
func (world *worldState) clearFireWalls() {
	if len(world.FireWalls) == 0 {
		return
	}
	world.FireWalls = nil
	world.Casts = slices.DeleteFunc(world.Casts, func(cast pendingCast) bool { return cast.FireWall != nil })
}
