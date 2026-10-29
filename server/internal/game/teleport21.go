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
	teleportSucceeded       uint32 = 1
	teleportSourceForbidden uint32 = 2
	teleportNoLanding       uint32 = 3
	teleportBindingInvalid  uint32 = 4
	teleportActorInvalid    uint32 = 5
	teleportUnavailable     uint32 = 6
)

// 先算基础费，再独立整数增加传送类惩罚；一般技能不因TemporalFlux涨价。
func teleportManaCost(player *entity, skill devconfig.Skill, rank uint32) (int64, bool) {
	if skill.MPBaseCost < 0 || skill.MPLevelCost < 0 || skill.MPLevelCost > 0 && int64(rank) > (math.MaxInt64-skill.MPBaseCost)/skill.MPLevelCost {
		return 0, false
	}
	base := skill.MPBaseCost + int64(rank)*skill.MPLevelCost
	if skill.Behavior != "teleport" {
		return base, true
	}
	percent := int64(0)
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
	increase := (base/100)*percent + (base%100)*percent/100
	if base > math.MaxInt64-increase {
		return 0, false
	}
	return base + increase, true
}

func (w *worker) trustedTeleportBinding(r *room) bool {
	f := &w.e.Store.Cfg.Fixture
	b := r.Role.Binding
	if !f.SafeZoneBindingEnabled() || b == nil || r.Role.BindingSourceSeq <= 0 || r.Role.BindingSafeZoneID == 0 {
		return false
	}
	m, ok := f.MapByID(b.MapID)
	if !ok || m.SceneKindValue() != devconfig.SceneKindTown || m.Source == nil {
		return false
	}
	for _, zone := range m.SafeZones {
		if zone.ID == r.Role.BindingSafeZoneID && zone.Center == (devconfig.Point{X: int(b.X), Y: int(b.Y)}) && m.TeleportWalkable(zone.Center) {
			return true
		}
	}
	return false
}

func (w *worker) teleportResult(r *room, cast pendingCast, from, scene uint64, target uint64, point *devconfig.Point, outcome uint32, transition string) {
	result := &pb.TeleportResult{Role: &pb.RoleKey{RoleId: domain.Bytes(r.Role.ID), OriginServerId: r.Role.Origin}, FromSceneEpoch: from, SceneEpoch: scene, CastId: cast.ID, PlayerLife: cast.Caster.Life, TargetMapId: target, MovementRevision: r.Player.MovementRevision, Outcome: outcome, TransitionId: transition}
	if point != nil {
		result.Position = pointPB(*point)
	}
	w.send(r, p.MsgS2CTeleportResult, 0, result)
}

// 到期读取当前图、当前可信绑定和当时阶；这里只选一次意图，核心在本批序位边界提交。
func (w *worker) resolveTeleport21(r *room, origin *worldState, cast pendingCast, now time.Time) *worldTransfer {
	fromScene := r.Scene
	originPosition := r.Player.Pos
	fail := func(code uint32) *worldTransfer {
		w.teleportResult(r, cast, fromScene, r.Scene, 0, nil, code, "")
		return nil
	}
	if origin == nil || r.world(origin.Ref) != origin || w.resolveCredit(r, cast.Caster) != &r.Player || r.Player.Life != cast.Caster.Life {
		return fail(teleportActorInvalid)
	}
	f := &w.e.Store.Cfg.Fixture
	source, known := w.currentMap(r)
	if !known || source.NoTeleport == nil {
		return fail(teleportUnavailable)
	}
	if *source.NoTeleport {
		return fail(teleportSourceForbidden)
	}
	if !w.trustedTeleportBinding(r) || cast.Skill.ID != 21 || cast.Skill.JobID != r.Role.Job || cast.Skill.Teleport == nil {
		return fail(teleportBindingInvalid)
	}
	rank, learned := r.Role.Skill(21)
	if !learned || rank < cast.Skill.LearnedLevel || rank > cast.Skill.MaxLevel {
		return fail(teleportBindingInvalid)
	}
	binding := *r.Role.Binding
	originProof := r.Role.BindingSourceSeq
	targetMap, known := f.MapByID(binding.MapID)
	if !known {
		return fail(teleportBindingInvalid)
	}
	sx, sy := int64(targetMap.Width)/int64(rank+1), int64(targetMap.Height)/int64(rank+1)
	var point devconfig.Point
	found := false
	for i := 0; i < cast.Skill.Teleport.MaxAttempts; i++ {
		x := int64(binding.X) + w.draw(2*sx) - sx
		y := int64(binding.Y) + w.draw(2*sy) - sy
		if x < 0 || y < 0 || x >= int64(targetMap.Width) || y >= int64(targetMap.Height) {
			continue
		}
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
	target := r.TownWorld
	if r.currentWorld().MapID == targetMap.ID {
		target = r.currentWorld()
	}
	if target == nil {
		target = r.newWorld(targetMap.ID)
		r.TownWorld = target
	}
	if target == nil || target.MapID != targetMap.ID {
		return fail(teleportUnavailable)
	}
	cross := target != r.currentWorld()
	baseline := r.Baseline
	transition := ""
	if cross {
		if r.Scene == math.MaxUint64 || f.TownTravel == nil || f.TownTravel.AckTTLMS <= 0 {
			return fail(teleportUnavailable)
		}
		var ok bool
		baseline, ok = w.nextTravelBaseline(r.Baseline)
		if !ok {
			return fail(teleportUnavailable)
		}
		transition = domain.ID()
	}
	var petPoint *devconfig.Point
	if r.Pet != nil && !mapForbidsPets(targetMap) {
		candidate := r.Pet.Pos
		if cross {
			var ok bool
			candidate, ok = travelPetPoint(targetMap, point, nil, target.Monsters, target.MonsterCount)
			if !ok {
				return fail(teleportUnavailable)
			}
		}
		petPoint = &candidate
	}
	definition, hasFlux := f.Buff(cast.Skill.BuffID)
	if !hasFlux || definition.Kind != "teleport_mana_penalty" || definition.DurationMS <= 0 {
		return fail(teleportUnavailable)
	}
	var landing *safeZoneBindingFact
	if zone, inside := targetMap.SafeZoneAt(point); inside {
		center := domain.RoleBinding{MapID: targetMap.ID, X: int32(zone.Center.X), Y: int32(zone.Center.Y)}
		if center != binding {
			if r.BindingCandidate != nil || r.PendingBinding != nil || r.FactOrder == math.MaxUint64 || r.Role.SourceSeq == math.MaxInt64 || r.Role.Revision == math.MaxInt64 || len(r.KillFacts)+1 > f.CombatTiming.KillQueueCapacity {
				return fail(teleportUnavailable)
			}
			landing = &safeZoneBindingFact{request: persistence.SafeZoneBindingRequest{ExpectedRuleVersion: f.RuleVersion, ExpectedConfigHash: f.Hash, MapID: targetMap.ID, Position: point, Action: "teleport", EventID: domain.ID(), OriginBindingSourceSeq: originProof}, center: center, zoneID: zone.ID}
		}
	}
	return &worldTransfer{From: r.currentWorld().Ref, Target: target, PlayerLife: r.Player.Life, Position: point, PetPosition: petPoint,
		OnRejected: func(w *worker, r *room, at time.Time) {
			w.teleportResult(r, cast, fromScene, r.Scene, targetMap.ID, nil, teleportUnavailable, "")
		},
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
