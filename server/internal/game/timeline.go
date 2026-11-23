// 本文件用统一房间时钟处理到期攻击和施法，冻结击杀、绑定和托管事实，并从原世界结算。
package game

import (
	"fmt"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"sort"
	"time"
)

// actorRef 把实体句柄限定到原世界和生命代次。
type actorRef struct {
	World  worldRef // 实体归属的内部世界执行身份。
	Handle uint32   // 实体在该世界中的句柄。
	Life   uint64   // 具体出生代次，防止复活或重生实体接收旧事件。
}

// pendingAttack 描述已经受理、尚未到命中时刻的攻击。
type pendingAttack struct {
	Scene             uint64          // 仅观察身份，不参与World执行解析。
	Caster, Target    actorRef        // 攻击者和目标在原世界中的完整出生引用。
	Due               time.Time       // 受理后确定的命中到期时刻。
	Amount            int64           // 受理时冻结的原始攻击量。
	Sequence          uint32          // 用于通知客户端的本次攻击动作编号。
	Order             uint64          // 统一事件排序号，同一到期时刻按它决定先后。
	HostSession       string          // 命中事件所属的原离线托管会话。
	PrivilegeUntil    time.Time       // 命中事件所属权限区段的特权截止时刻。
	PrivilegeCaptured bool            // 区分已冻结的权限时钟与旧规则未记录的时间。
	SkillID           uint32          // 本次攻击使用的技能编号。
	SkillLevel        uint32          // 本次攻击使用的技能等级。
	Defence           physicalDefence // 命中时采用的防御计算模式。
}

// killFact 在怪物死亡当下保存来源归属和掉落候选，随后按原内容持久化。
type killFact struct {
	World               worldRef           // 击杀实际发生的原世界。
	Order               uint64             // 在房间内协调击杀和绑定事实的受理顺序。
	DropPoint           devconfig.Point    // 战利品在原世界显示的逻辑坐标。
	ID                  string             // 应用或核对该击杀事实时使用的唯一编号。
	Target              actorRef           // 死亡怪物的世界、句柄和生命代次。
	Caster              actorRef           // 击杀者的世界、句柄和生命代次。
	Monster             devconfig.Monster  // 击杀时冻结的怪物数值，供经验和掉落结算。
	MapID               uint64             // 实际发生击杀的地图配置号，后续换图不改归属。
	At                  time.Time          // 事件实际到期时刻，决定收益归属与绑定。
	Bound               bool               // 击杀时已经确定的资产绑定属性。
	HostSession         string             // 应该累加战报的原托管会话。
	StopReason          uint32             // 托管停止原因，非零时只结算停止状态。
	Roll                int64              // 击杀时抽取的一个百万分位随机值。
	EquipmentID, ItemID uint32             // 预先抽中的装备与普通物品候选编号。
	Gold                int64              // 已经抽好的本次金币数量。
	InstanceID          string             // 若生成物品时使用的唯一资产编号。
	Prepared            bool               // 首部事实已经转换为来源事务，防止再次准备。
	PetTraining         *petTrainingCredit // 为原代宠物冻结的训练资格。
	ProgressionActor    actorRef           // 原世界中允许接收升级恢复效果的角色出生身份。
}

// sourceTimeline 判断当前规则是否启用来源公式和统一动作时钟。
// w 是调用该方法的房间执行器实例。
func (w *worker) sourceTimeline() bool {
	return w.e.Store.Cfg.Fixture.DamageFormula == "development_source_combat" && w.e.Store.Cfg.Fixture.CombatTiming.PlayerAttackActionMS > 0
}

// hitContext 提供命中时刻以及冻结的特权和托管归属。
type hitContext struct {
	PeriodicPoison    bool      // 毒的旧效果按自身节拍扣血，不算一次解除围困的直接攻击。
	At                time.Time // 本次命中正式发生的时刻。
	HostSession       string    // 该事件所在离线托管区段的会话编号。
	PrivilegeUntil    time.Time // 用于判断 Bind 属性的归属区段特权截止时刻。
	PrivilegeCaptured bool      // 权限时钟已由受理事件快照固定时为真。
}

// 权限变更只更新尚未到期的事件；已经发生的事件不借迟处理、重连或续卡改绑。
// r 指向当前执行器拥有并更新的目标房间，at 记录发生时间，使用时间点表示，until 记录有效截止时间，使用时间点表示。
func transitionEventPrivilege(r *room, at, until time.Time) {
	// world 接收遍历场景世界集合时当前的元素。
	for _, world := range r.worlds() {
		// i 标记遍历攻击集合时访问的下标。
		for i := range world.Attacks {
			if !world.Attacks[i].Due.Before(at) {
				world.Attacks[i].PrivilegeUntil = until
				world.Attacks[i].PrivilegeCaptured = true
			}
		}
		// i 标记遍历施法集合时访问的下标。
		for i := range world.Casts {
			if !world.Casts[i].Due.Before(at) {
				world.Casts[i].PrivilegeUntil = until
				world.Casts[i].PrivilegeCaptured = true
			}
		}
	}
	// event 接收遍历事件批次时当前的元素。
	for _, event := range r.EventBatch {
		if !event.At.Before(at) {
			if event.Attack != nil {
				event.Attack.PrivilegeUntil, event.Attack.PrivilegeCaptured = until, true
			}
			if event.Cast != nil {
				event.Cast.PrivilegeUntil, event.Cast.PrivilegeCaptured = until, true
			}
		}
	}
}

// 只更新转换时尚未到期的事件；晚处理的事件保留其事件时刻所在的在线/托管区段。
// r 指向当前执行器拥有并更新的目标房间，at 记录发生时间，使用时间点表示，session 记录会话，使用文本表示。
func transitionEventHosting(r *room, at time.Time, session string) {
	// world 接收遍历场景世界集合时当前的元素。
	for _, world := range r.worlds() {
		// i 标记遍历攻击集合时访问的下标。
		for i := range world.Attacks {
			if !world.Attacks[i].Due.Before(at) {
				world.Attacks[i].HostSession = session
			}
		}
		// i 标记遍历施法集合时访问的下标。
		for i := range world.Casts {
			if !world.Casts[i].Due.Before(at) {
				world.Casts[i].HostSession = session
			}
		}
	}
	// event 接收遍历事件批次时当前的元素。
	for _, event := range r.EventBatch {
		if !event.At.Before(at) {
			if event.Attack != nil {
				event.Attack.HostSession = session
			}
			if event.Cast != nil {
				event.Cast.HostSession = session
			}
		}
	}
}

// 围栏/身份硬失败不能给旧事实升级授权或重新抽奖；只隔离本房间，等待明确恢复。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) isolateRoom(r *room) {
	w.detachWorldBoss(r)
	r.PetBornReady = nil
	w.leaveSharedTown(r)
	w.cancelTownNavigation(r)
	w.cancelMapTravelOnDetach(r)
	r.Isolated = true
	r.Pending = false
	r.Auto = false
	r.Manual = false
	r.PreviewGeneration++
	r.PreviewInFlight = false
	if r.Conn != nil {
		r.Conn.Close("room authority isolated")
	}
	// waiting 接收遍历等待时当前的元素。
	for _, waiting := range r.Waiting {
		if waiting.conn != nil {
			waiting.conn.Close("room authority isolated")
		}
	}
	r.Waiting = nil
}

// dueEvent 把普攻和施法归入同一可比较的时间线。
type dueEvent struct {
	World  *worldState    // 保持原事件世界，换图后仍按该世界解析目标。
	At     time.Time      // 事件到期的逻辑时间。
	Order  uint64         // 同一到期时间内用于决胜的受理顺序。
	Attack *pendingAttack // 本条事件承载的普攻或近战结果。
	Cast   *pendingCast   // 本条事件承载的技能结果。
}

// 全部临时到期事件按统一时钟与受理次序结算，不以消息种类改变同帧命中顺序。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) advanceEvents(r *room, now time.Time) {
	w.advanceEventsUsing(r, now, w.resolveDueEvent)
}

// resolveDueEvent 按事件种类执行命中，支持召唤和跨世界迁移这一类延迟结果。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，e 记录已到期事件，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) resolveDueEvent(r *room, e dueEvent, now time.Time) *worldTransfer {
	if e.Cast != nil && e.Cast.Summon != nil {
		w.resolveSummonPet(r, e.World, *e.Cast, now)
		return nil
	}
	if e.Cast != nil && e.Cast.Skill.Behavior == "teleport" {
		return w.resolveTeleport21(r, e.World, *e.Cast, now)
	}
	if e.Attack != nil {
		w.resolveAttackInWorld(r, e.World, *e.Attack, now)
	} else {
		w.resolveCastInWorld(r, e.World, *e.Cast, now)
	}
	return nil
}

// 整个owner只有一个到期批；从队列摘取到局部批不归还共享容量。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
// resolve 处理一项到期事件并可返回世界迁移，生产路径使用原世界的普攻或施法处理。
func (w *worker) advanceEventsUsing(r *room, now time.Time, resolve func(*room, dueEvent, time.Time) *worldTransfer) {
	if r.AdvancingEvents || r.Pending || r.Isolated {
		return
	}
	r.AdvancingEvents = true
	defer func() { r.AdvancingEvents = false; r.EventBatch = nil }()
	// events 初始化用作事件集合的已到期事件列表。
	events := []dueEvent{}
	// world 接收遍历场景世界集合时当前的元素。
	for _, world := range r.worlds() {
		// attacks 保存当前处理所用的攻击集合。
		attacks := world.Attacks[:0]
		// casts 保存当前处理所用的施法集合。
		casts := world.Casts[:0]
		// a 接收遍历攻击集合时当前的元素。
		for _, a := range world.Attacks {
			if now.Before(a.Due) {
				attacks = append(attacks, a)
			} else {
				// copy 保存当前处理所用的“copy”。
				copy := a
				events = append(events, dueEvent{World: world, At: a.Due, Order: a.Order, Attack: &copy})
			}
		}
		// c 接收遍历施法集合时当前的元素。
		for _, c := range world.Casts {
			if now.Before(c.Due) {
				casts = append(casts, c)
			} else {
				// copy 保存当前处理所用的“copy”。
				copy := c
				events = append(events, dueEvent{World: world, At: c.Due, Order: c.Order, Cast: &copy})
			}
		}
		world.Attacks, world.Casts = attacks, casts
	}
	// 先按真实到期时刻排序，同一时刻再按跨世界受理顺序决定。
	// i 和 j 是待比较的两项事件下标，比较时按到期时刻和受理顺序排列。
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].At.Equal(events[j].At) {
			return events[i].Order < events[j].Order
		}
		return events[i].At.Before(events[j].At)
	})
	r.InflightEvents += len(events)
	// i 定位事件集合中正在处理的成员，e 接收遍历事件集合时当前的元素。
	for i, e := range events {
		r.EventBatch = events[i:]
		if r.Pending || r.Isolated {
			// 新出现的在途事务阻止继续推进，把整段未处理事件放回各自原世界。
			// waiting 接收遍历事件集合中的范围时当前的元素。
			for _, waiting := range events[i:] {
				if waiting.Attack != nil {
					waiting.World.Attacks = append(waiting.World.Attacks, *waiting.Attack)
				} else {
					waiting.World.Casts = append(waiting.World.Casts, *waiting.Cast)
				}
				r.InflightEvents--
			}
			break
		}
		r.InflightEvents-- // 当前slot可转交给同一地图动作下一子格。
		r.EventBatch = events[i+1:]
		// transfer 是到期事件申请的世界迁移，在推进剩余到期事件前尝试提交。
		if transfer := resolve(r, e, now); transfer != nil {
			w.commitWorldTransfer(r, transfer, now)
		}
	}
	if len(events) > 0 {
		w.snapshot(r)
	}
}

// killBudgetAvailable 确认房间尚有空间记录后续击杀事实。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) killBudgetAvailable(r *room) bool {
	// cap 是规则规定的击杀事实队列上限，非正值时关闭此限制。
	cap := w.e.Store.Cfg.Fixture.CombatTiming.KillQueueCapacity
	if cap <= 0 {
		return true
	}
	// possible 估计当前事实、候选绑定和所有活怪可能产生的事实总量。
	possible := len(r.KillFacts) + 1
	if r.BindingCandidate != nil {
		possible++
	}
	// world 接收遍历场景世界集合时当前的元素。
	for _, world := range r.worlds() {
		// m 接收遍历怪物集合中的范围时当前的元素。
		for _, m := range world.Monsters[:world.MonsterCount] {
			if m.HP > 0 {
				possible++
			}
		}
	}
	return possible <= cap
}

// reference 固化实体在当前活动世界的原始执行身份。
// r 指向当前执行器拥有并更新的目标房间，e 记录实体。
func reference(r *room, e *entity) actorRef {
	if e != nil && e == r.Pet {
		return r.petReference()
	}
	return referenceIn(r.currentWorld(), e)
}

// resolve 按完整执行身份查找事件中的角色或怪物。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，ref 记录引用，使用行动实体引用表示。
func (w *worker) resolve(r *room, ref actorRef) *entity {
	return w.resolveInWorld(r, r.currentWorld(), ref)
}

// facing 从坐标差推导网格朝向，重合时沿用指定方向。
// from 记录来源，使用点位表示，to 记录目标，使用点位表示，fallback 记录回退路径，使用数值表示。
func facing(from, to devconfig.Point, fallback uint32) uint32 {
	// dx、dy 分别是目标与来源位置的横向和纵向格差，用于确定八向朝向。
	dx, dy := to.X-from.X, to.Y-from.Y
	// i 定位朝向集合中正在处理的成员，d 接收遍历朝向集合时当前的元素。
	for i, d := range directions {
		if (dx == 0 && d.X == 0 || dx > 0 && d.X > 0 || dx < 0 && d.X < 0) && (dy == 0 && d.Y == 0 || dy > 0 && d.Y > 0 || dy < 0 && d.Y < 0) {
			return uint32(i)
		}
	}
	return fallback
}

// nextEvent 分配客户端可见的下一战斗事件流水。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) nextEvent(r *room) uint64 {
	if r.EventID < math.MaxUint64 {
		r.EventID++
	}
	return r.EventID
}

// impact 在当前活动世界发送有效战斗命中表现。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，skill 记录技能，使用数值表示，caster 记录施法者，使用数值表示，target 记录目标，使用数值表示。
// life 记录生命代次，使用数值表示，grid 记录网格，使用点位表示，amount 记录数量，使用数值表示，outcome 记录“outcome”，使用数值表示。
// healing 标记是否处于治疗状态，cast 记录施法，使用数值表示，attack 记录攻击，使用数值表示。
func (w *worker) impact(r *room, skill, caster, target uint32, life uint64, grid devconfig.Point, amount int64, outcome uint32, healing bool, cast uint64, attack uint32) {
	w.send(r, p.MsgS2CCombatImpact, 0, &pb.CombatImpact{SceneEpoch: r.Scene, EventId: w.nextEvent(r), SkillId: skill, CasterHandle: caster, TargetHandle: target, TargetLife: life, TargetGrid: pointPB(grid), Amount: amount, Outcome: outcome, Healing: healing, CastId: cast, AttackSequence: attack})
}

// impactInWorld 只向当前可观察原世界的连接发送命中表现。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，scene 记录场景，使用数值表示。
// skill 记录技能，使用数值表示，caster 记录施法者，使用数值表示，target 记录目标，使用数值表示，life 记录生命代次，使用数值表示，grid 记录网格，使用点位表示。
// amount 记录数量，使用数值表示，outcome 记录“outcome”，使用数值表示，healing 标记是否处于治疗状态，cast 记录施法，使用数值表示。
// attack 记录攻击，使用数值表示。
func (w *worker) impactInWorld(r *room, world *worldState, scene uint64, skill, caster, target uint32, life uint64, grid devconfig.Point, amount int64, outcome uint32, healing bool, cast uint64, attack uint32) {
	if r.observesWorld(world, scene) {
		w.impact(r, skill, caster, target, life, grid, amount, outcome, healing, cast, attack)
	}
}

// resolveAttack 按已受理攻击的原世界解析命中。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，a 记录a，使用待确认攻击表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) resolveAttack(r *room, a pendingAttack, now time.Time) {
	w.resolveAttackInWorld(r, r.world(a.Caster.World), a, now)
}

// resolveAttackInWorld 寻找原出生的攻守双方，结算命中并应用实际伤害。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，a 记录a，使用待确认攻击表示。
// now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) resolveAttackInWorld(r *room, world *worldState, a pendingAttack, now time.Time) {
	if world == nil || a.Target.World != world.Ref || a.Caster.World != world.Ref {
		return
	}
	// known 保存由场景世界地图取得的已知状态。
	if _, known := w.worldMap(world); !known {
		return
	}
	// target 是在事件原世界中仍匹配句柄和生命代次的命中目标。
	target := w.resolveInWorld(r, world, a.Target)
	if target == nil || target.HP <= 0 {
		w.impactInWorld(r, world, a.Scene, a.SkillID, a.Caster.Handle, a.Target.Handle, a.Target.Life, devconfig.Point{}, 0, 3, false, 0, a.Sequence)
		return
	}
	// 来源CompleteAttack不要求施法者仍存活；伤害/防御在到期时结算，坐标不重新限制近战。
	// caster 是在事件原世界中仍匹配施法来源的实体。
	caster := w.resolveInWorld(r, world, a.Caster)
	if caster == nil {
		w.impactInWorld(r, world, a.Scene, a.SkillID, a.Caster.Handle, target.Handle, target.Life, target.Pos, 0, 3, false, 0, a.Sequence)
		return
	}
	// attacker 汇总攻击者当前的战斗属性，defender 汇总受击目标的防御属性。
	attacker, defender := w.fighter(r, caster), w.fighter(r, target)
	// damage 初始为零，命中且防御计算完成后记录应扣生命。
	damage := int64(0)
	if a.Defence == defenceAC || w.draw(defender.Agility+1) <= attacker.Accuracy {
		// armour 记录本次规则选出的护甲值，某些技能模式下保持为零。
		armour := int64(0)
		if a.Defence != defenceAgility {
			armour = w.interval(defender.DefenceMin, defender.DefenceMax)
		}
		damage = statusDamage(target, a.Amount, poisonArmour(target, armour))
	}
	if damage < 0 {
		damage = 0
	}
	// before 保存生命值，用于当前步骤中的操作前的状态。
	before := target.HP
	w.applyDamageInWorld(r, world, caster, target, damage, now, hitContext{At: a.Due, HostSession: a.HostSession, PrivilegeUntil: a.PrivilegeUntil, PrivilegeCaptured: a.PrivilegeCaptured})
	// actual 是扣血前后生命差，伤害表现使用这个实际值。
	actual := before - target.HP
	// outcome 是命中表现的协议结果，区分实际掉血和未命中。
	outcome := uint32(1)
	if actual == 0 {
		outcome = 2
	}
	w.impactInWorld(r, world, a.Scene, a.SkillID, caster.Handle, target.Handle, target.Life, target.Pos, actual, outcome, false, 0, a.Sequence)
}

// kill 标记怪物死亡及复活期限，并在原世界建立来源收益事实。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，m 记录实体，now 提供当前时点，作为状态有效期和计时判断的基准，from 记录来源，使用可变数量的实体表示。
func (w *worker) kill(r *room, m *entity, now time.Time, from ...*entity) {
	if !w.sourceTimeline() {
		w.legacyKill(r, m, now)
		return
	}
	// caster 保存当前处理所用的施法者。
	caster := &r.Player
	if len(from) > 0 && from[0] != nil {
		caster = from[0]
	}
	// session 保存会话所使用的文本。
	session := ""
	if r.Conn == nil {
		session = r.HostSession
	}
	w.enqueueKillFact(r, m, now, caster, session, r.Role.PrivilegeUntil)
}

// enqueueKillFact 在当前活动世界登记击杀事实及执行归属。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，m 记录实体，at 记录发生时间，使用时间点表示，caster 记录施法者，使用实体表示。
// session 记录会话，使用文本表示，privilegeUntil 记录“privilege”有效截止时间，使用时间点表示。
func (w *worker) enqueueKillFact(r *room, m *entity, at time.Time, caster *entity, session string, privilegeUntil time.Time) {
	w.enqueueKillFactInWorld(r, r.currentWorld(), m, at, caster, session, privilegeUntil)
}

// enqueueKillFactInWorld 捕获原世界的掉落位置、随机数、特权和宠物训练资格。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，m 记录实体，at 记录发生时间，使用时间点表示。
// caster 记录施法者，使用实体表示，session 记录会话，使用文本表示，privilegeUntil 记录“privilege”有效截止时间，使用时间点表示。
func (w *worker) enqueueKillFactInWorld(r *room, world *worldState, m *entity, at time.Time, caster *entity, session string, privilegeUntil time.Time) {
	// f 保存当前处理所用的开发测试配置。
	f := w.e.Store.Cfg.Fixture
	r.FactOrder++
	// point 保存玩家位置，用于当前步骤中的点位。
	point := world.PlayerPosition
	if world == r.currentWorld() {
		point = r.Player.Pos
	}
	// fact 固化原世界、击杀时刻、归属、掉落随机值和未来的物品实例编号。
	fact := killFact{ID: domain.ID(), World: world.Ref, Order: r.FactOrder, DropPoint: point, Target: referenceIn(world, m), Caster: referenceIn(world, caster), Monster: *m.Monster, MapID: world.MapID, At: at, Bound: !privilegeUntil.After(at), Roll: w.draw(1000000), InstanceID: domain.ID(), HostSession: session}
	if world == r.currentWorld() {
		fact.ProgressionActor = referenceIn(world, &r.Player)
	}
	fact.PetTraining = w.capturePetTraining(r, fact.ID)
	if len(f.Drops.EquipmentItemIDs) > 0 {
		fact.EquipmentID = f.Drops.EquipmentItemIDs[w.draw(int64(len(f.Drops.EquipmentItemIDs)))]
	}
	if len(f.Drops.ItemIDs) > 0 {
		fact.ItemID = f.Drops.ItemIDs[w.draw(int64(len(f.Drops.ItemIDs)))]
	}
	fact.Gold = m.Monster.GoldMin + w.draw(m.Monster.GoldMax-m.Monster.GoldMin+1)
	r.KillFacts = append(r.KillFacts, fact)
}

// flushKillFacts 在前一永久操作完成后按顺序提交绑定事实或击杀队首。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) flushKillFacts(r *room) {
	if r.Pending || r.Isolated {
		return
	}
	// candidate 是等待进入来源队列的安全区绑定候选。
	if candidate := r.BindingCandidate; candidate != nil && (len(r.KillFacts) == 0 || candidate.order < r.KillFacts[0].Order) {
		w.admitBindingCandidate(r, time.Now())
		return
	}
	if len(r.KillFacts) == 0 || r.KillFacts[0].Prepared {
		return
	}
	// 在队首第一次提交前锁定准备标记，未知确认时继续等待同一操作。
	// head 保存当前处理所用的头部。
	head := &r.KillFacts[0]
	head.Prepared = true
	// fact 复制冻结的击杀队首，后续回执继续关联这一原事实。
	fact := *head
	// q 接收击杀结算供后续步骤使用。
	q := w.killSettlement(r, fact)
	if !w.preparePetTraining(r, fact.PetTraining, q) {
		w.isolateRoom(r)
		return
	}
	// world 接收场景世界供后续步骤使用。
	world := r.world(fact.World)
	if world == nil && fact.World == (worldRef{}) {
		world = r.currentWorld()
	} // 仅兼容旧手造killFact，正式捕获必有原World。
	if world == nil {
		w.isolateRoom(r)
		return
	}
	// receipt 记录回执，使用操作表示。
	w.settleInWorldReceipt(r, world, fact.DropPoint, q, fact.ProgressionActor, func(receipt persistence.Operation) {
		w.finishKillFact(r, fact.ID, q, receipt)
	})
}

// finishKillFact 确认队首回执并应用原代宠物训练，然后释放该事实。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，id 区分当前步骤指向的目标对象，q 记录结算，receipt 记录回执，使用可变数量的操作表示。
func (w *worker) finishKillFact(r *room, id string, q persistence.Settlement, receipt ...persistence.Operation) {
	if len(r.KillFacts) == 0 || r.KillFacts[0].ID != id {
		return
	}
	// credit 保存宠物训练，用于当前步骤中的入账。
	credit := r.KillFacts[0].PetTraining
	if credit != nil && credit.KillID != id {
		w.isolateRoom(r)
		return
	}
	if w.e.Store.Cfg.Fixture.RuleVersion >= 20 {
		if len(receipt) != 1 {
			w.isolateRoom(r)
			return
		}
		// grant 保存由经验回执取得的授予，ok 表明查询、校验或类型转换是否成功。
		grant, ok := experienceReceipt(q, receipt[0])
		if !ok {
			w.isolateRoom(r)
			return
		}
		w.awardPetTraining(r, credit, q, grant)
	} else {
		w.awardPetTraining(r, credit, q)
	}
	r.KillFacts[0] = killFact{}
	r.KillFacts = r.KillFacts[1:]
}

// killSettlement 将原击杀内容转换为下一来源事务，按原掷点及当前确认保底水位确定奖励。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，fact 记录已发生的事实，使用击杀已发生的事实表示。
func (w *worker) killSettlement(r *room, fact killFact) persistence.Settlement {
	// q 组装结算所需的数据。
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, HostingSession: fact.HostSession, StopReason: fact.StopReason, EventAt: fact.At}
	q.TargetHandle, q.TargetLife, q.CasterHandle = fact.Target.Handle, fact.Target.Life, fact.Caster.Handle
	if fact.StopReason == 0 {
		// f 保存当前处理所用的开发测试配置。
		f := w.e.Store.Cfg.Fixture
		q.Kills = 1
		q.MonsterID, q.MapID = fact.Monster.ID, fact.MapID
		q.Experience = fact.Monster.Exp
		q.Bound = fact.Bound
		q.NoEquipmentKills = r.Role.NoEquipmentKills + 1
		// itemID 标识当前业务使用的道具。
		itemID := uint32(0)
		// goldEnd 是百万分位掷点中空掉落区与金币区的累计上界。
		goldEnd := int64(f.Drops.EmptyPPM + f.Drops.GoldPPM)
		// itemEnd 是再累加普通物品区后的上界，其上属于装备掉落分支。
		itemEnd := goldEnd + int64(f.Drops.ItemPPM)
		if q.NoEquipmentKills >= f.Drops.EquipmentGuarantee || fact.Roll >= itemEnd {
			itemID = fact.EquipmentID
			q.NoEquipmentKills = 0
		} else if fact.Roll >= int64(f.Drops.EmptyPPM) && fact.Roll < goldEnd {
			q.Gold = fact.Gold
		} else if fact.Roll >= goldEnd {
			itemID = fact.ItemID
		}
		if itemID != 0 {
			// definition 是掉落物品的配置定义，提供本次资产的品质。
			definition, _ := f.Item(itemID)
			q.Item = &domain.Item{ID: fact.InstanceID, ItemID: itemID, Quantity: 1, Version: 1, Bound: fact.Bound, SourceKind: "monster_drop", SourceID: fmt.Sprint(q.SourceSeq), Quality: definition.DropQuality}
		}
	}
	return q
}
