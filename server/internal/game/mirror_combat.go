// 本文件用两侧临时角色身体和同一张八方向地图推进镜像竞技，不使用普通挂机收益或角色钱包。
package game

import (
	"slices"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

const (
	mirrorDefeat           uint32 = 1 // 一侧参战身体全部失去战斗能力。
	mirrorTimeout          uint32 = 2 // 到期按原上阵生命比例判定。
	mirrorInterrupted      uint32 = 3 // 原临时身体失权或无法继续模拟，由原场次退预占。
	mirrorObservationLimit        = 64
)

type mirrorCombatResult struct {
	Winner     uint32    // 1 进攻方，2 防守方，0 系统中断。
	Cause      uint32    // 真实全灭、超时比较或系统中断。
	FinishedAt time.Time // 终态发生的原结算时刻。
}

type mirrorCombatSide struct {
	room          *room                       // 该侧唯一临时角色，保留原角色职业及技能时钟。
	potions       []domain.MirrorCombatSupply // 入场药剂数量的本场副本。
	material      domain.MirrorCombatSupply   // 临时专槽耗材数量。
	initialLife   int64                       // 冻结角色与唯一入场宠物的生命分母。
	entryPet      actorRef                    // 唯一可参与超时生命比较的原上阵宠物身份。
	births        uint32                      // 本侧已经成功出生的新宠数量。
	pendingSummon *summonPetPlan              // 已受理、尚未到时的本侧唯一出生计划。
}

type mirrorCombatObservation struct {
	ID      uint16        // 已登记的公共战斗表现消息号。
	Message proto.Message // 仍携本场唯一角色、施法和命中身份。
	bornAt  time.Time     // 出生观察独用的单调时点，供发送时计算经过时间。
}

type mirrorCombat struct {
	config       *devconfig.MirrorArenaConfig // 原场次不可变规则。
	sides        [2]mirrorCombatSide          // 固定进攻和防守身体身份。
	startedAt    time.Time                    // 已确认场次开始时刻。
	deadline     time.Time                    // 已确认时限，边界不接收新动作。
	lastTick     time.Time                    // 最后已推进的逻辑时点。
	result       mirrorCombatResult           // 一次写入的临时胜负。
	due          []sharedCombatDue            // 共享到期队列，按原动作时刻排序再统一推进。
	castID       uint64                       // 场次内唯一公共施法号。
	eventID      uint64                       // 场次内唯一公共命中流水。
	observations []mirrorCombatObservation    // 有界观测流，丢弃表现不会修改权威状态。
}

// newMirrorCombat 按服务端场次和配置重建原双方投影，不引用任何在线角色的可变资产聚合。
func (w *worker) newMirrorCombat(config *devconfig.MirrorArenaConfig, fighters [2]domain.MirrorCombatSnapshot, startedAt, deadline time.Time, scene uint64) *mirrorCombat {
	if config == nil || scene == 0 || startedAt.IsZero() || !deadline.After(startedAt) || config.Map.ID == 0 ||
		config.Map.SceneKindValue() != devconfig.SceneKindMirrorArena || config.Map.CellBudget() > devconfig.MaxPersonalGridCells {
		return nil
	}
	starts := [...]devconfig.Point{config.AttackerSpawn, config.DefenderSpawn}
	petStarts := [...]devconfig.Point{config.AttackerPetSpawn, config.DefenderPetSpawn}
	b := &mirrorCombat{config: config, startedAt: startedAt, deadline: deadline}
	ref := worldRef{Epoch: ownerExecutionEpoch.Add(1), ID: 1}
	if ref.Epoch == 0 {
		return nil
	}
	occupied := make(map[devconfig.Point]struct{}, 4)
	for index, input := range fighters {
		if !validMirrorFighter(input.Fighter) || input.Level == 0 || input.Job > 2 || !config.Map.Walkable(starts[index]) {
			return nil
		}
		if _, busy := occupied[starts[index]]; busy {
			return nil
		}
		occupied[starts[index]] = struct{}{}
		refWorld := &worldState{Ref: ref, MapID: config.Map.ID, BossMap: &config.Map, Auto: true, mirrorCombat: b, mirrorSide: index}
		shadow := &room{worldState: refWorld, ExecutionEpoch: ref.Epoch, WorldSequence: ref.ID, Scene: scene, Ready: true,
			Role: domain.Role{Name: input.Name, Job: input.Job, Gender: input.Gender, Level: input.Level,
				Skills: slices.Clone(input.Skills), SkillSlotIDs: slices.Clone(input.SkillSlotIDs)},
			bossBattle: &personalBossBattle{fighter: input.Fighter},
			Player: entity{Handle: uint32(index + 1), Kind: 1, Def: input.Job, Pos: starts[index], Life: 1,
				Direction: facing(starts[index], starts[1-index], 0), HP: input.Fighter.HP, MaxHP: input.Fighter.HP, MP: input.Fighter.MP, MaxMP: input.Fighter.MP}}
		b.sides[index] = mirrorCombatSide{room: shadow, initialLife: input.Fighter.HP, potions: slices.Clone(input.Potions)}
		if input.Material != nil {
			b.sides[index].material = *input.Material
		}
	}
	for index, input := range fighters {
		if input.Pet == nil {
			continue
		}
		pet := input.Pet
		if !validMirrorFighter(pet.Fighter) || pet.DefinitionID == 0 || !config.Map.Walkable(petStarts[index]) || mapForbidsPets(&config.Map) {
			return nil
		}
		if _, busy := occupied[petStarts[index]]; busy {
			return nil
		}
		occupied[petStarts[index]] = struct{}{}
		side := &b.sides[index]
		profiles := w.e.Store.Cfg.Fixture.PetRuntime
		definition, known := profiles.DefinitionByBodyID(pet.DefinitionID)
		if !known && config.Summoning != nil && profiles != nil && len(profiles.Definitions) != 0 {
			// 普通诱惑获得的宠物使用首个职业宠物定义的生命周期，镜像保留同一尸体期限。
			definition.Lifecycle = profiles.Definitions[0].Lifecycle
		}
		actor := &entity{Handle: uint32(101 + index), Kind: 2, Def: pet.DefinitionID, Pos: petStarts[index], Life: 1,
			Direction: facing(petStarts[index], starts[1-index], 0), HP: pet.Fighter.HP, MaxHP: pet.Fighter.HP, MP: pet.Fighter.MP, MaxMP: pet.Fighter.MP,
			PetRuntime: &petRuntimeState{Definition: definition, Fighter: pet.Fighter, Level: pet.Level}}
		side.room.Pet, side.room.PetWorld, side.room.PetLifeSequence = actor, ref, actor.Life
		side.room.recordPetPosition()
		side.initialLife += pet.Fighter.HP
		side.entryPet = referenceIn(side.room.currentWorld(), actor)
	}
	return b
}

// validMirrorFighter 拒绝损坏的生命与时钟，保证两侧生命乘积和原动作时长在整数范围内。
func validMirrorFighter(f devconfig.Fighter) bool {
	return f.HP > 0 && f.HP < 1_000_000 && f.MP >= 0 && f.MoveMS > 0 && f.MoveMS <= 60_000 && f.AttackMS > 0 && f.AttackMS <= 60_000 &&
		f.HitBonuses.ValidSnapshot() && f.MasteryBonuses.ValidSnapshot()
}

func (b *mirrorCombat) eachBody(visit func(*entity)) {
	for index := range b.sides {
		shadow := b.sides[index].room
		visit(&shadow.Player)
		if shadow.Pet != nil {
			visit(shadow.Pet)
		}
	}
}

func (b *mirrorCombat) occupied(point devconfig.Point, self *entity) bool {
	occupied := false
	b.eachBody(func(body *entity) { occupied = occupied || body != self && body.HP > 0 && body.Pos == point })
	return occupied
}

func (b *mirrorCombat) addObservation(id uint16, message proto.Message) {
	if len(b.observations) == mirrorObservationLimit {
		copy(b.observations, b.observations[1:])
		b.observations = b.observations[:mirrorObservationLimit-1]
	}
	b.observations = append(b.observations, mirrorCombatObservation{ID: id, Message: message})
}

func (b *mirrorCombat) drainObservations() []mirrorCombatObservation {
	out := b.observations
	b.observations = nil
	return out
}

// mirrorCombatEntities 保留原角色与宠物实体类型，客户端据固定句柄区分两侧。
func (w *worker) mirrorCombatEntities(b *mirrorCombat) []*pb.EntityState {
	var out []*pb.EntityState
	for _, side := range b.sides {
		owner := side.room
		avatar := w.worldBossBodyView(owner, &owner.Player)
		avatar.DisplayName = owner.Role.Name
		out = append(out, avatar)
		if owner.Pet != nil {
			out = append(out, w.worldBossBodyView(owner, owner.Pet))
		}
		for _, corpse := range owner.currentWorld().PetCorpses {
			out = append(out, w.worldBossBodyView(owner, corpse.Body))
		}
	}
	return out
}

func (b *mirrorCombat) fireWallCells(now time.Time) []*pb.GridPoint {
	points := append(b.sides[0].room.currentWorld().fireWallCells(now), b.sides[1].room.currentWorld().fireWallCells(now)...)
	if len(points) <= 1 {
		return points
	}
	// 双方的火墙可能覆盖同一格。保持源遍历顺序，每格只发一次。
	seen := make(map[[2]int32]bool, len(points))
	unique := points[:0]
	for _, point := range points {
		key := [2]int32{point.X, point.Y}
		if !seen[key] {
			seen[key] = true
			unique = append(unique, point)
		}
	}
	return unique
}
