package game

import (
	"math"
	"minimir/internal/devconfig"
	"sync/atomic"
	"time"
)

// 执行身份不属于客户端scene。每个owner固定保留个人/城镇两个世界，不因换图重建。
type worldRef struct{ Epoch, ID uint64 }

type worldState struct {
	Ref            worldRef
	MapID          uint64
	PlayerPosition devconfig.Point
	PetPosition    devconfig.Point
	HasPet         bool
	PetLife        uint64 // 历史位置提示所属birth；真实居民由room.PetWorld判定。
	Monsters       []*entity
	MonsterCount   int
	Auto           bool
	Destination    *devconfig.Point
	Navigation     navigationState
	Loot           []*entity
	LootUntil      time.Time
	Casts          []pendingCast
	Attacks        []pendingAttack
}

// 由到期业务选择一次目的地；核心在该事件与批后缀之间提交。无统一死亡取消门禁。
type worldTransfer struct {
	From        worldRef
	Target      *worldState
	PlayerLife  uint64
	Position    devconfig.Point
	PetPosition *devconfig.Point
	OnCommitted func(*worker, *room, time.Time)
	OnRejected  func(*worker, *room, time.Time)
}

func (w *worker) commitWorldTransfer(r *room, transfer *worldTransfer, now time.Time) bool {
	if transfer == nil {
		return false
	}
	if r.currentWorld().Ref != transfer.From || r.Player.Life != transfer.PlayerLife || r.Pending || r.Isolated || !w.activateWorld(r, transfer.Target, transfer.Position, transfer.PetPosition) {
		if transfer.OnRejected != nil {
			transfer.OnRejected(w, r, now)
		}
		return false
	}
	if transfer.OnCommitted != nil {
		transfer.OnCommitted(w, r, now)
	}
	return true
}

var ownerExecutionEpoch atomic.Uint64

func (r *room) currentWorld() *worldState {
	if r.worldState == nil {
		r.worldState = &worldState{}
	}
	if r.ExecutionEpoch == 0 {
		r.ExecutionEpoch = ownerExecutionEpoch.Add(1)
		if r.ExecutionEpoch == 0 {
			panic("owner执行身份耗尽")
		}
	}
	if r.Ref == (worldRef{}) {
		r.WorldSequence++
		r.Ref = worldRef{Epoch: r.ExecutionEpoch, ID: r.WorldSequence}
	}
	return r.worldState
}

func (r *room) worlds() []*worldState {
	active := r.currentWorld()
	all := []*worldState{active}
	for _, world := range []*worldState{r.PersonalWorld, r.TownWorld} {
		if world != nil && world != active && (len(all) == 1 || all[1] != world) {
			all = append(all, world)
		}
	}
	return all
}

func (r *room) world(ref worldRef) *worldState {
	for _, world := range r.worlds() {
		if world.Ref == ref {
			return world
		}
	}
	return nil
}

func (r *room) newWorld(mapID uint64) *worldState {
	r.currentWorld()
	if r.WorldSequence >= 2 {
		return nil
	}
	r.WorldSequence++
	return &worldState{Ref: worldRef{Epoch: r.ExecutionEpoch, ID: r.WorldSequence}, MapID: mapID}
}

func (w *worker) worldMap(world *worldState) (*devconfig.MapTemplate, bool) {
	if world == nil {
		return nil, false
	}
	f := &w.e.Store.Cfg.Fixture
	if world.MapID == 0 { // 仅沿用旧手造内存fixture的primary兼容。
		return &f.Map, true
	}
	return f.MapByID(world.MapID)
}

func (w *worker) worldWalkable(world *worldState, point devconfig.Point) bool {
	m, ok := w.worldMap(world)
	return ok && m.Walkable(point)
}

func referenceIn(world *worldState, e *entity) actorRef {
	return actorRef{World: world.Ref, Handle: e.Handle, Life: e.Life}
}

func (w *worker) resolveInWorld(r *room, world *worldState, ref actorRef) *entity {
	if world == nil || world.Ref != ref.World || r.world(ref.World) != world {
		return nil
	}
	all := world.Monsters[:world.MonsterCount]
	if world == r.currentWorld() {
		all = append([]*entity{&r.Player}, all...)
		all = append(all, world.Loot...)
	}
	all = append(all[:len(all):len(all)], r.worldPet(world))
	for _, e := range all {
		if e != nil && e.Handle == ref.Handle && e.Life == ref.Life {
			return e
		}
	}
	return nil
}

// 地图动作仍引用唯一真实角色的信用/Life，不把新图角色注册为旧图可受击居民。
func (w *worker) resolveCredit(r *room, ref actorRef) *entity {
	if r.world(ref.World) == nil {
		return nil
	}
	for _, e := range []*entity{&r.Player, r.Pet} {
		if e != nil && e.Handle == ref.Handle && e.Life == ref.Life {
			return e
		}
	}
	return w.resolveInWorld(r, r.world(ref.World), ref)
}

func (r *room) eventCount() int {
	n := r.InflightEvents
	for _, world := range r.worlds() {
		n += len(world.Casts) + len(world.Attacks)
	}
	return n
}

// 仅由受信业务在调度边界调用。目的点/宠物规则由调用方选定，核心不发明空格规则。
func (w *worker) activateWorld(r *room, target *worldState, point devconfig.Point, petPoint *devconfig.Point) bool {
	m, known := w.worldMap(target)
	if !known || target == nil || r.world(target.Ref) != target || !m.Contains(point) || r.Player.MovementRevision == math.MaxUint64 ||
		r.Pet != nil && (r.petResidentWorld() == nil || petPoint == nil || !m.Contains(*petPoint) || r.Pet.MovementRevision == math.MaxUint64) {
		return false
	}
	moves := uint64(1)
	if r.Pet != nil {
		moves++
	}
	if r.CellEntrySequence > math.MaxUint64-moves {
		return false
	}
	old := r.currentWorld()
	if old != target {
		w.cancelTownNavigation(r)
	}
	old.PlayerPosition = r.Player.Pos
	old.HasPet = r.worldPet(old) != nil
	if pet := r.worldPet(old); pet != nil {
		old.PetPosition, old.PetLife = pet.Pos, pet.Life
	}
	r.worldState = target
	if old == r.PersonalWorld && target == r.TownWorld {
		r.SavedPersonal = old
	}
	if target == r.PersonalWorld {
		r.SavedPersonal = nil
	}
	w.beginMovement(r, &r.Player, movementTeleport)
	w.commitRelocation(r, &r.Player, point)
	if r.Pet != nil {
		w.beginMovement(r, r.Pet, movementTeleport)
		r.PetWorld = target.Ref
		w.commitRelocation(r, r.Pet, *petPoint)
	}
	return true
}

func (r *room) observesWorld(world *worldState, scene uint64) bool {
	return world == r.currentWorld() && r.Scene == scene && r.Ready && !r.Isolated
}

// 无已释放地图动作的休眠World维持原首屏/ACK语义，不因纯角色失效动作开启怪物时间。
func (w *worker) reactiveWorlds(r *room) []*worldState {
	var worlds []*worldState
	for _, world := range r.worlds() {
		if world == r.currentWorld() {
			continue
		}
		for _, cast := range world.Casts {
			if cast.Skill.Behavior == "hell_fire" && w.resolveCredit(r, cast.Caster) != nil {
				worlds = append(worlds, world)
				break
			}
		}
	}
	return worlds
}

func (w *worker) worldDeadlines(r *room, world *worldState, now time.Time) {
	m, known := w.worldMap(world)
	if !known {
		w.isolateRoom(r)
		return
	}
	if !world.LootUntil.IsZero() && !now.Before(world.LootUntil) {
		world.Loot = nil
	}
	for i, monster := range world.Monsters {
		if monster.HP == 0 && !monster.RespawnAt.IsZero() && !now.Before(monster.RespawnAt) {
			if i >= len(m.MonsterSpawns) || !m.Walkable(m.MonsterSpawns[i]) || monster.Life == math.MaxUint64 {
				w.isolateRoom(r)
				return
			}
			monster.HP, monster.Life, monster.Statuses = monster.MaxHP, monster.Life+1, nil
			if !w.commitRelocation(r, monster, m.MonsterSpawns[i]) {
				return
			}
			monster.MovementRevision, monster.MovementKind = 0, movementInitial
			monster.RespawnAt, monster.Action = time.Time{}, 0
		}
	}
}
