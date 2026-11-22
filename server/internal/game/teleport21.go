// 本文件计算瞬移耗魔与受影响时钟，按可信绑定或候选落点申请移动，保留原世界事件归属。
package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"time"
)

const (
	// teleportSucceeded 规定瞬移“succeeded”的初始取值。
	teleportSucceeded uint32 = 1
	// teleportSourceForbidden 规定瞬移来源“forbidden”的初始取值。
	teleportSourceForbidden uint32 = 2
	// teleportNoLanding 规定瞬移“no”落点的初始取值。
	teleportNoLanding uint32 = 3
	// teleportBindingInvalid 规定瞬移会话绑定无效的初始取值。
	teleportBindingInvalid uint32 = 4
	// teleportActorInvalid 规定瞬移行动实体无效的初始取值。
	teleportActorInvalid uint32 = 5
	// teleportUnavailable 规定瞬移不可用的初始取值。
	teleportUnavailable uint32 = 6
)

// 先算基础费，再独立整数增加传送类惩罚；一般技能不因TemporalFlux涨价。
// player 记录玩家，使用实体表示，skill 记录技能，rank 记录排序位次，使用数值表示。
func teleportManaCost(player *entity, skill devconfig.Skill, rank uint32) (int64, bool) {
	if skill.MPBaseCost < 0 || skill.MPLevelCost < 0 || skill.MPLevelCost > 0 && int64(rank) > (math.MaxInt64-skill.MPBaseCost)/skill.MPLevelCost {
		return 0, false
	}
	// base 保存根据魔法值基础值消耗与整数“64”与魔法值等级消耗的计算结果计算的基础值。
	base := skill.MPBaseCost + int64(rank)*skill.MPLevelCost
	if skill.Behavior != "teleport" {
		return base, true
	}
	// percent 保存由整数“64”取得的“percent”。
	percent := int64(0)
	// status 接收遍历状态集合时当前的元素。
	for _, status := range player.Statuses {
		if status.TargetLife == player.Life && status.Kind == "teleport_mana_penalty" {
			if status.TeleportManaPenaltyPercent < 0 || status.TeleportManaPenaltyPercent > 100 {
				return 0, false
			}
			if status.TeleportManaPenaltyPercent > percent {
				percent = status.TeleportManaPenaltyPercent
			}
		}
	}
	// increase 保存根据基础值与当前数据的计算结果与“percent”的计算结果与基础值与当前数据的计算结果与“percent”的计算结果与当前数据的计算结果计算的“increase”。
	increase := (base/100)*percent + (base%100)*percent/100
	if base > math.MaxInt64-increase {
		return 0, false
	}
	return base + increase, true
}

// trustedTeleportBinding 判断可信状态瞬移会话绑定是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) trustedTeleportBinding(r *room) bool {
	// ok 表明查询、校验或类型转换是否成功。
	_, ok := w.teleportTownBinding(r)
	return ok
}

// teleportTownBinding 处理瞬移城镇会话绑定。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) teleportTownBinding(r *room) (devconfig.BindingPoint, bool) {
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	// b 保存会话绑定，用于当前步骤中的b。
	b := r.Role.Binding
	if !f.SafeZoneBindingEnabled() || b == nil || r.Role.BindingSourceSeq <= 0 || r.Role.BindingSafeZoneID == 0 {
		return devconfig.BindingPoint{}, false
	}
	return f.TownTeleportBinding(devconfig.BindingPoint{MapID: b.MapID, X: b.X, Y: b.Y}, r.Role.BindingSafeZoneID)
}

// teleportResult 处理瞬移结果。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，cast 记录施法，使用待确认施法表示，from 记录来源，使用数值表示，scene 记录场景，使用数值表示。
// target 记录目标，使用数值表示，point 记录点位，outcome 记录“outcome”，使用数值表示，transition 记录状态切换，使用文本表示。
func (w *worker) teleportResult(r *room, cast pendingCast, from, scene uint64, target uint64, point *devconfig.Point, outcome uint32, transition string) {
	// result 保存当前处理所用的结果。
	result := &pb.TeleportResult{Role: &pb.RoleKey{RoleId: domain.Bytes(r.Role.ID), OriginServerId: r.Role.Origin}, FromSceneEpoch: from, SceneEpoch: scene, CastId: cast.ID, PlayerLife: cast.Caster.Life, TargetMapId: target, MovementRevision: r.Player.MovementRevision, Outcome: outcome, TransitionId: transition}
	if point != nil {
		result.Position = pointPB(*point)
	}
	w.send(r, p.MsgS2CTeleportResult, 0, result)
}

// 到期读取当前图、当前可信绑定和当时阶；这里只选一次意图，核心在本批序位边界提交。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，origin 记录大区或起点，使用场景世界状态表示，cast 记录施法，使用待确认施法表示。
// now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) resolveTeleport21(r *room, origin *worldState, cast pendingCast, now time.Time) *worldTransfer {
	// fromScene 保存场景，用于当前步骤中的来源场景。
	fromScene := r.Scene
	// originPosition 保存位置，用于当前步骤中的大区或起点位置。
	originPosition := r.Player.Pos
	// fail 封装失败对应的后续处理，code 记录结果码，使用数值表示。
	fail := func(code uint32) *worldTransfer {
		w.teleportResult(r, cast, fromScene, r.Scene, 0, nil, code, "")
		return nil
	}
	if origin == nil || r.world(origin.Ref) != origin || w.resolveCredit(r, cast.Caster) != &r.Player || r.Player.Life != cast.Caster.Life {
		return fail(teleportActorInvalid)
	}
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	// source 保存由当前状态地图取得的来源，known 保存由当前状态地图取得的已知状态。
	source, known := w.currentMap(r)
	if !known || source.NoTeleport == nil {
		return fail(teleportUnavailable)
	}
	if *source.NoTeleport {
		return fail(teleportSourceForbidden)
	}
	// binding 保存由瞬移城镇会话绑定取得的会话绑定，trusted 保存由瞬移城镇会话绑定取得的可信状态。
	binding, trusted := w.teleportTownBinding(r)
	if !trusted || cast.Skill.ID != 21 || cast.Skill.JobID != r.Role.Job || cast.Skill.Teleport == nil {
		return fail(teleportBindingInvalid)
	}
	// rank 保存由技能取得的排序位次，learned 保存由技能取得的已学习。
	rank, learned := r.Role.Skill(21)
	if !learned || rank < cast.Skill.LearnedLevel || rank > cast.Skill.MaxLevel {
		return fail(teleportBindingInvalid)
	}
	// originProof 保存会话绑定来源序号，用于当前步骤中的大区或起点“proof”。
	originProof := r.Role.BindingSourceSeq
	// targetMap 保存由地图“by”标识取得的目标地图。
	targetMap, known := f.MapByID(binding.MapID)
	if !known {
		return fail(teleportBindingInvalid)
	}
	// sx 保存根据整数“64”与整数“64”计算的“sx”，sy 保存根据整数“64”与整数“64”计算的“sy”。
	sx, sy := int64(targetMap.Width)/int64(rank+1), int64(targetMap.Height)/int64(rank+1)
	// point 记录点位。
	var point devconfig.Point
	// found 保存当前处理所用的已找到标记。
	found := false
	// i 标记循环或列表中的当前位置。
	for i := 0; i < cast.Skill.Teleport.MaxAttempts; i++ {
		// x 保存根据整数“64”与“draw”的计算结果与“sx”计算的整数“64”与“draw”的计算结果与“sx”的计算结果。
		x := int64(binding.X) + w.draw(2*sx) - sx
		// y 保存根据整数“64”与“draw”的计算结果与“sy”计算的整数“64”与“draw”的计算结果与“sy”的计算结果。
		y := int64(binding.Y) + w.draw(2*sy) - sy
		if x < 0 || y < 0 || x >= int64(targetMap.Width) || y >= int64(targetMap.Height) {
			continue
		}
		// candidate 建立带初始值的点位，供本段继续处理。
		candidate := devconfig.Point{X: int(x), Y: int(y)}
		if targetMap.TeleportWalkable(candidate) {
			point = candidate
			found = true
			break
		}
	}
	if !found {
		return fail(teleportNoLanding)
	}
	// target 保存由当前状态场景世界取得的目标。
	target := r.currentWorld()
	if target.MapID != targetMap.ID {
		target = w.ensureTownWorld(r, targetMap.ID)
	}
	if target == nil || target.MapID != targetMap.ID {
		return fail(teleportUnavailable)
	}
	// cross 保存根据目标与当前状态场景世界计算的“cross”。
	cross := target != r.currentWorld()
	// baseline 保存当前处理所用的场景基线。
	baseline := r.Baseline
	// transition 保存状态切换所使用的文本。
	transition := ""
	if cross {
		if r.Scene == math.MaxUint64 || f.TownTravel == nil || f.TownTravel.AckTTLMS <= 0 {
			return fail(teleportUnavailable)
		}
		// ok 表明查询、校验或类型转换是否成功。
		var ok bool
		baseline, ok = w.nextTravelBaseline(r.Baseline)
		if !ok {
			return fail(teleportUnavailable)
		}
		transition = domain.ID()
	}
	// petPoint 记录宠物点位。
	var petPoint *devconfig.Point
	if r.Pet != nil && !mapForbidsPets(targetMap) {
		// candidate 保存位置，用于当前步骤中的候选。
		candidate := r.Pet.Pos
		if cross {
			// ok 表明查询、校验或类型转换是否成功。
			var ok bool
			candidate, ok = travelPetPoint(targetMap, point, nil, target.Monsters, target.MonsterCount)
			if !ok {
				return fail(teleportUnavailable)
			}
		}
		petPoint = &candidate
	}
	// definition 保存由状态效果取得的定义，hasFlux 保存由状态效果取得的“has”“flux”。
	definition, hasFlux := f.Buff(cast.Skill.BuffID)
	if !hasFlux || definition.Kind != "teleport_mana_penalty" || definition.DurationMS <= 0 {
		return fail(teleportUnavailable)
	}
	// landing 记录落点，使用安全区域会话绑定已发生的事实表示。
	var landing *safeZoneBindingFact
	// zone 保存由安全区域发生时间取得的区域，inside 保存由安全区域发生时间取得的“inside”。
	if zone, inside := targetMap.SafeZoneAt(point); inside {
		// center 建立带初始值的角色会话绑定，供本段继续处理。
		center := domain.RoleBinding{MapID: targetMap.ID, X: int32(zone.Center.X), Y: int32(zone.Center.Y)}
		if center != *r.Role.Binding {
			if r.BindingCandidate != nil || r.PendingBinding != nil || r.FactOrder == math.MaxUint64 || r.Role.SourceSeq == math.MaxInt64 || r.Role.Revision == math.MaxInt64 || len(r.KillFacts)+1 > f.CombatTiming.KillQueueCapacity {
				return fail(teleportUnavailable)
			}
			landing = &safeZoneBindingFact{request: persistence.SafeZoneBindingRequest{ExpectedRuleVersion: f.RuleVersion, ExpectedConfigHash: f.Hash, MapID: targetMap.ID, Position: point, Action: "teleport", EventID: domain.ID(), OriginBindingSourceSeq: originProof}, center: center, zoneID: zone.ID}
		}
	}
	return &worldTransfer{From: r.currentWorld().Ref, Target: target, PlayerLife: r.Player.Life, Position: point, PetPosition: petPoint,
		// w 记录房间执行器，r 指向当前执行器拥有并更新的目标房间，at 记录发生时间，使用时间点表示。
		OnRejected: func(w *worker, r *room, at time.Time) {
			w.teleportResult(r, cast, fromScene, r.Scene, targetMap.ID, nil, teleportUnavailable, "")
		},
		// w 记录房间执行器，r 指向当前执行器拥有并更新的目标房间，at 记录发生时间，使用时间点表示。
		OnCommitted: func(w *worker, r *room, at time.Time) {
			if r.Travel != nil && (r.Travel.Phase == travelPrepared || r.Travel.Phase == travelQuiescing) {
				r.Travel.Phase = travelCancelled
				w.sendTravelState(r, r.Travel, r.Travel.Request)
			}
			r.Travel = nil
			if cross {
				r.Scene++
				r.Baseline = baseline
				r.TravelHistory = nil
				r.Manual = false
				r.InputSequence = 0
				target.Auto = false
				target.Destination = nil
				target.Navigation = navigationState{}
				r.PreviewInFlight = false
				if r.PreviewGeneration < math.MaxUint64 {
					r.PreviewGeneration++
				}
				r.Ready = false
				r.PetBornReady = nil
				r.LastTick = at
				r.LastBroadcast = at
				r.AwaitUntil = at.Add(time.Duration(f.TownTravel.AckTTLMS) * time.Millisecond)
			}
			// applied 保存当前处理所用的“applied”。
			applied := cast
			applied.SkillLevel = rank
			applied.BuffDurationMS = definition.DurationMS
			if !w.applyBuff(r, applied, &r.Player, at) {
				w.isolateRoom(r)
				return
			}
			if landing != nil && !w.queueBindingCandidate(r, landing) {
				w.isolateRoom(r)
				return
			}
			w.teleportResult(r, cast, fromScene, r.Scene, targetMap.ID, &point, teleportSucceeded, transition)
			if cross {
				// entry 保存由入场线路结果取得的入场。
				if entry := w.enterLineResult(r); entry != nil {
					entry.ServerTransitionId = transition
					w.send(r, p.MsgS2CEnterLine, 0, entry)
					w.bindingState(r, 1)
					w.baseline(r)
				}
			} else {
				w.send(r, p.MsgS2CEntityDisplacement, 0, &pb.EntityDisplacement{SceneEpoch: r.Scene, EntityHandle: r.Player.Handle, EntityLife: r.Player.Life, MovementRevision: r.Player.MovementRevision, Kind: movementTeleport, Origin: pointPB(originPosition), Path: []*pb.GridPoint{pointPB(point)}, FacingDirection: r.Player.Direction, MovementDirection: r.Player.Direction, SourceSkillId: 21, CastId: cast.ID, AttackSequence: r.Player.AttackSequence, CastSequence: r.Player.CastSequence})
				w.snapshot(r)
			}
		},
	}
}
