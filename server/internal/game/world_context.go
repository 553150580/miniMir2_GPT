// 本文件以执行代次与世界编号区分活动和停驻世界，管理迁移、实体归属和原世界事件推进。
package game

import (
	"math"
	"minimir/internal/devconfig"
	"sync/atomic"
	"time"
)

// 执行身份不属于客户端scene。每个owner保留个人世界与已到访的配置城镇，不因健康换图重建。
// Epoch 记录代次，使用数值表示。
// ID 区分当前步骤指向的目标对象。
type worldRef struct{ Epoch, ID uint64 }

// worldState 保存每张已访问地图独立存在的临时战斗现场。
type worldState struct {
	TrapHexagon    map[actorRef]time.Time // 有围困时才分配，按原怪物出生身份和期限保存。
	BossMap        *devconfig.MapTemplate // 私有挑战采用经过校验的独立地图，不混入普通地图表。
	Ref            worldRef               // 场景执行身份，限定事件和实体引用。
	MapID          uint64                 // 该世界对应的配置地图编号。
	PlayerPosition devconfig.Point        // 活动角色离开时保存的位置，供该世界的历史事实使用。
	PetPosition    devconfig.Point        // 宠物在世界停驻时保留的位置提示。
	HasPet         bool                   // 这个世界曾保留有效宠物位置时为真。
	PetLife        uint64                 // 历史位置提示所属birth；真实居民由room.PetWorld判定。
	Monsters       []*entity              // 当前世界的怪物槽位。
	MonsterCount   int                    // 活跃的前置怪物槽位数量。
	Auto           bool                   // 这个世界的普通自动战斗开关。
	Destination    *devconfig.Point       // 这个世界保留的玩家地面导航目标。
	Navigation     navigationState        // 向客户端同步的导航修订和进度。
	Loot           []*entity              // 当前世界暂时显示的收益对象。
	LootUntil      time.Time              // 临时收益对象的显示截止时刻。
	Casts          []pendingCast          // 原归属世界为本世界的已受理施法列表。
	FireWalls      []*fireWallField       // 已建立且可作用于入格实体的火墙，未使用技能的世界此切片为空。
	Attacks        []pendingAttack        // 原归属世界为本世界的已受理攻击列表。
	PetCorpses     []*petCorpse           // 在该世界保留的已死亡宠物身体。
	mirrorCombat   *mirrorCombat          // 只有镜像临时世界可以用同职业的另一阵营作目标。
	mirrorSide     int                    // 本世界的进攻或防守投影索引。
}

// combatOpponents 统一攻击目标入口。普通场保持原怪物槽，自身与友方宠物始终独立。
func (world *worldState) combatOpponents() []*entity {
	if world.mirrorCombat != nil {
		other := world.mirrorCombat.sides[1-world.mirrorSide].room
		if other.Pet == nil {
			return []*entity{&other.Player}
		}
		return []*entity{&other.Player, other.Pet}
	}
	return world.Monsters[:world.MonsterCount]
}

// 由到期业务选择一次目的地；核心在该事件与批后缀之间提交。无统一死亡取消门禁。
type worldTransfer struct {
	From        worldRef                        // 到期业务要求迁移的原世界身份。
	Target      *worldState                     // 迁移后的目标世界。
	PlayerLife  uint64                          // 仍应接收迁移的原角色出生代次。
	Position    devconfig.Point                 // 角色在目标世界中的实际落点。
	PetPosition *devconfig.Point                // 宠物需要同步迁移时的目标逻辑坐标。
	OnCommitted func(*worker, *room, time.Time) // 成功提交内部世界迁移后调用的后续行为。
	OnRejected  func(*worker, *room, time.Time) // 原身份失效或迁移不可提交时调用的回退行为。
}

// commitWorldTransfer 在到期事件之间核对原世界和原生命，一次性切换活动世界。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，transfer 记录转移，使用场景世界转移表示，now 提供当前时点，作为状态有效期和计时判断的基准。
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

// ownerExecutionEpoch 为本进程内冷启动的房间分配不复用的执行代次。
var ownerExecutionEpoch atomic.Uint64

// currentWorld 返回活动世界，缺失时补齐房间和世界执行身份。
// r 是调用该方法的房间实例。
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

// worlds 枚举该角色拥有的活动与停驻世界，每个世界只返回一次。
// r 是调用该方法的房间实例。
func (r *room) worlds() []*worldState {
	// active 保存由当前状态场景世界取得的“active”。
	active := r.currentWorld()
	// all 初始化用作全部的场景世界状态列表。
	all := []*worldState{active}
	// world 接收遍历场景世界状态列表时当前的元素。
	for _, world := range []*worldState{r.PersonalWorld, r.TownWorld} {
		if world != nil && world != active && (len(all) == 1 || all[1] != world) {
			all = append(all, world)
		}
	}
	// 交换目标只短暂存在，禁宠驻留也不持有第二个完整普通战斗场景。
	for _, world := range []*worldState{r.TravelPersonalWorld, r.PetHoldingWorld} {
		if world != nil && world != active && world != r.PersonalWorld && world != r.TownWorld {
			all = append(all, world)
		}
	}
	// world 接收遍历城镇场景世界集合时当前的元素。
	for _, world := range r.TownWorlds {
		if world != nil && world != active && world != r.PersonalWorld && world != r.TownWorld {
			all = append(all, world)
		}
	}
	return all
}

// world 通过内部执行身份定位角色保留的原世界。
// r 是调用该方法的房间实例，ref 记录引用，使用场景世界引用表示。
func (r *room) world(ref worldRef) *worldState {
	// world 接收遍历场景世界集合时当前的元素。
	for _, world := range r.worlds() {
		if world.Ref == ref {
			return world
		}
	}
	return nil
}

// newWorld 为兼容单城镇流程创建有限的下一张世界。
// r 是调用该方法的房间实例，mapID 标识当前业务使用的地图。
func (r *room) newWorld(mapID uint64) *worldState {
	return r.allocateWorld(mapID, 2)
}

// allocateWorld 在调用方限额以内分配新的独立世界。
// r 是调用该方法的房间实例，mapID 标识当前业务使用的地图，limit 记录上限，使用数值表示。
func (r *room) allocateWorld(mapID, limit uint64) *worldState {
	r.currentWorld()
	if r.WorldSequence >= limit {
		return nil
	}
	r.WorldSequence++
	return &worldState{Ref: worldRef{Epoch: r.ExecutionEpoch, ID: r.WorldSequence}, MapID: mapID}
}

// townWorld 按配置地图编号查找已经创建的城镇世界。
// r 是调用该方法的房间实例，mapID 标识当前业务使用的地图。
func (r *room) townWorld(mapID uint64) *worldState {
	if r.TownWorld != nil && r.TownWorld.MapID == mapID {
		return r.TownWorld
	}
	// world 接收遍历城镇场景世界集合时当前的元素。
	for _, world := range r.TownWorlds {
		if world.MapID == mapID {
			return world
		}
	}
	return nil
}

// ensureTownWorld 复用或创建目标城镇世界，计入该角色可保存的世界配额。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，mapID 标识当前业务使用的地图。
func (w *worker) ensureTownWorld(r *room, mapID uint64) *worldState {
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	// m 接收地图“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := f.MapByID(mapID)
	if !ok || m.SceneKindValue() != devconfig.SceneKindTown || f.RuleVersion >= 24 && !f.TownDestination(mapID) {
		return nil
	}
	// world 保存由城镇场景世界取得的场景世界。
	if world := r.townWorld(mapID); world != nil {
		return world
	}
	if f.RuleVersion < 24 {
		if r.TownWorld == nil {
			r.TownWorld = r.newWorld(mapID)
		}
		return r.townWorld(mapID)
	}
	// world 保存由“allocate”场景世界取得的场景世界。
	limit := 1 + uint64(len(f.TownTravel.DestinationMapIDs))
	if f.OrdinaryMonsterRosterEnabled() {
		// 普通图可反复换并回收；已发布城镇目录仍保持实际持有的城镇数量有界。
		limit = math.MaxUint64
	}
	world := r.allocateWorld(mapID, limit)
	if world != nil {
		// 按首次到访顺序登记。后续遍历不受Go map迭代顺序影响。
		r.TownWorlds = append(r.TownWorlds, world)
	}
	return world
}

// worldMap 为内部世界的地图编号取得相应配置模板。
// w 是调用该方法的房间执行器实例，world 记录场景世界，使用场景世界状态表示。
func (w *worker) worldMap(world *worldState) (*devconfig.MapTemplate, bool) {
	if world == nil {
		return nil, false
	}
	if world.BossMap != nil {
		kind := world.BossMap.SceneKindValue()
		return world.BossMap, world.MapID == world.BossMap.ID &&
			(kind == devconfig.SceneKindPersonalBoss || kind == devconfig.SceneKindWorldBoss || kind == devconfig.SceneKindDungeon ||
				kind == devconfig.SceneKindMirrorArena && world.mirrorCombat != nil)
	}
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	if world.MapID == 0 { // 仅沿用旧手造内存fixture的primary兼容。
		return &f.Map, true
	}
	return f.MapByID(world.MapID)
}

// worldWalkable 使用指定世界自己的地形检查逻辑坐标。
// w 是调用该方法的房间执行器实例，world 记录场景世界，使用场景世界状态表示，point 记录点位。
func (w *worker) worldWalkable(world *worldState, point devconfig.Point) bool {
	// m 接收场景世界地图供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := w.worldMap(world)
	return ok && m.Walkable(point)
}

// referenceIn 把实体指针转换成绑定原世界和出生代次的执行引用。
// world 记录场景世界，使用场景世界状态表示，e 记录实体。
func referenceIn(world *worldState, e *entity) actorRef {
	return actorRef{World: world.Ref, Handle: e.Handle, Life: e.Life}
}

// resolveInWorld 查询原世界中仍对应同一个出生身份的实体。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，ref 记录引用，使用行动实体引用表示。
func (w *worker) resolveInWorld(r *room, world *worldState, ref actorRef) *entity {
	if world == nil || world.Ref != ref.World || r.world(ref.World) != world {
		return nil
	}
	// all 保存当前处理所用的全部。
	all := world.combatOpponents()
	if world == r.currentWorld() {
		all = append([]*entity{&r.Player}, all...)
		all = append(all, world.Loot...)
	}
	all = append(all[:len(all):len(all)], r.petBodies(world)...)
	// e 接收遍历全部时当前的元素。
	for _, e := range all {
		if e != nil && !e.DeferredUntilRespawn && e.Handle == ref.Handle && e.Life == ref.Life {
			return e
		}
	}
	return nil
}

// 地图动作仍引用唯一真实角色的信用/Life，不把新图角色注册为旧图可受击居民。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，ref 记录引用，使用行动实体引用表示。
func (w *worker) resolveCredit(r *room, ref actorRef) *entity {
	if r.world(ref.World) == nil {
		return nil
	}
	// e 接收遍历实体列表时当前的元素。
	for _, e := range []*entity{&r.Player, r.Pet} {
		if e != nil && e.Handle == ref.Handle && e.Life == ref.Life {
			return e
		}
	}
	return w.resolveInWorld(r, r.world(ref.World), ref)
}

// eventCount 累加所有世界积压和当前处理中事件的总数。
// r 是调用该方法的房间实例。
func (r *room) eventCount() int {
	// n 保存当前处理所用的“inflight”事件集合。
	n := r.InflightEvents
	// world 接收遍历场景世界集合时当前的元素。
	for _, world := range r.worlds() {
		n += len(world.Casts) + len(world.Attacks)
	}
	return n
}

// 仅由受信业务在调度边界调用。目的点/宠物规则由调用方选定，核心不发明空格规则。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，target 记录目标，使用场景世界状态表示，point 记录点位，petPoint 记录宠物点位。
func (w *worker) activateWorld(r *room, target *worldState, point devconfig.Point, petPoint *devconfig.Point) bool {
	return w.activateWorldAt(r, target, point, petPoint, false)
}

// activateWorldAt 原子保留共享城镇目标格，界面进城可以选择相邻的空闲落点。
func (w *worker) activateWorldAt(r *room, target *worldState, point devconfig.Point, petPoint *devconfig.Point, alternateTownLanding bool) bool {
	// m 接收场景世界地图供后续步骤使用，known 保存由场景世界地图取得的已知状态。
	m, known := w.worldMap(target)
	// old 保存由当前状态场景世界取得的原状态。
	old := r.currentWorld()
	// mapChanged 保存根据原状态与目标计算的地图已变化。
	mapChanged := old != target
	// movePet 保存根据宠物与“nil”的计算结果与地图“forbids”宠物集合计算的移动宠物。
	movePet := r.Pet != nil && !mapForbidsPets(m)
	if !known || target == nil || r.world(target.Ref) != target || !m.Contains(point) || r.Player.MovementRevision == math.MaxUint64 ||
		r.Pet != nil && r.petResidentWorld() == nil || movePet && (petPoint == nil || !m.Contains(*petPoint) || r.Pet.MovementRevision == math.MaxUint64) {
		return false
	}
	// moves 保存由无符号整数“64”取得的移动集合。
	moves := uint64(1)
	if movePet {
		moves++
	}
	if r.CellEntrySequence > math.MaxUint64-moves {
		return false
	}
	// 目标格必须在原世界仍有效时预留，防止两个角色迁移到同一出生格。
	point, available := w.reserveSharedTown(r, m, point, petPoint, alternateTownLanding, mapChanged)
	if !available {
		return false
	}
	if mapChanged {
		w.cancelTownNavigation(r)
		r.AutoTarget = actorRef{}
		old.clearFireWalls()
	}
	old.PlayerPosition = r.Player.Pos
	old.HasPet = r.worldPet(old) != nil
	// pet 保存由场景世界宠物取得的宠物。
	if pet := r.worldPet(old); pet != nil {
		old.PetPosition, old.PetLife = pet.Pos, pet.Life
	}
	r.worldState = target
	if mapChanged && old == r.PersonalWorld && m.SceneKindValue() == devconfig.SceneKindTown {
		r.SavedPersonal = old
	}
	if target == r.PersonalWorld {
		r.SavedPersonal = nil
	}
	w.beginMovement(r, &r.Player, movementTeleport)
	w.commitRelocation(r, &r.Player, point)
	if movePet {
		w.beginMovement(r, r.Pet, movementTeleport)
		r.PetWorld = target.Ref
		w.commitRelocation(r, r.Pet, *petPoint)
	}
	r.releasePetHoldingWorld()
	if mapChanged {
		w.applyPetMapEntryPolicy(r, m)
	}
	return true
}

// observesWorld 判断客户端是否仍能观察指定世界和场景代次。
// r 是调用该方法的房间实例，world 记录场景世界，使用场景世界状态表示，scene 记录场景，使用数值表示。
func (r *room) observesWorld(world *worldState, scene uint64) bool {
	if arena := r.dungeonCombat; arena != nil {
		seat := arena.seats[r.Role.ID]
		return world == r.currentWorld() && seat != nil && seat.room == r && seat.started && !r.Isolated
	}
	return world == r.currentWorld() && r.Scene == scene && r.Ready && !r.Isolated
}

// 无已释放地图动作的休眠World维持原首屏/ACK语义，不因纯角色失效动作开启怪物时间。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) reactiveWorlds(r *room) []*worldState {
	// worlds 记录场景世界集合，使用场景世界状态列表表示。
	var worlds []*worldState
	// world 接收遍历场景世界集合时当前的元素。
	for _, world := range r.worlds() {
		if world == r.currentWorld() {
			continue
		}
		// cast 接收遍历施法集合时当前的元素。
		for _, cast := range world.Casts {
			if (cast.Skill.Behavior == "hell_fire" || cast.Poison != nil) && w.resolveCredit(r, cast.Caster) != nil {
				worlds = append(worlds, world)
				break
			}
		}
	}
	return worlds
}

// worldDeadlines 在原世界处理怪物复活、临时收益消失和宠物尸体回收。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) worldDeadlines(r *room, world *worldState, now time.Time) {
	// m 接收场景世界地图供后续步骤使用，known 保存由场景世界地图取得的已知状态。
	m, known := w.worldMap(world)
	if !known {
		w.isolateRoom(r)
		return
	}
	if !world.LootUntil.IsZero() && !now.Before(world.LootUntil) {
		world.Loot = nil
	}
	world.expireTrapHexagons(now)
	// i 定位怪物集合中正在处理的成员，monster 接收遍历怪物集合时当前的元素。
	for i, monster := range world.Monsters {
		if monster.HP == 0 && !monster.RespawnAt.IsZero() && !now.Before(monster.RespawnAt) {
			if i >= len(m.MonsterSpawns) || monster.Life == math.MaxUint64 {
				w.isolateRoom(r)
				return
			}
			// point 保存从怪物生成或出生点集合取出的点位。
			point := m.MonsterSpawns[i]
			if w.e.Store.Cfg.Fixture.RuleVersion >= 26 && m.SceneKindValue() == devconfig.SceneKindPersonal {
				// ok 表明查询、校验或类型转换是否成功。
				var ok bool
				point, ok = w.randomMonsterRespawnPoint(r, world, m)
				if !ok {
					continue // 保留原截止，下一帧有空格就复活，无需再等十秒。
				}
			} else if !m.Walkable(point) {
				w.isolateRoom(r)
				return
			}
			if !w.commitRelocation(r, monster, point) {
				return
			}
			monster.HP, monster.Life, monster.Statuses = monster.MaxHP, monster.Life+1, nil
			monster.MovementRevision, monster.MovementKind = 0, movementInitial
			monster.RespawnAt, monster.Action = time.Time{}, 0
			monster.DeferredUntilRespawn, monster.ShockUntilMS = false, 0
		}
	}
}
