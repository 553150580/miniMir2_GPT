// 本文件初始化普通挂机图的怪物槽，并在切换普通地图时保留已经开始的复活时钟。
package game

import (
	"minimir/internal/devconfig"
	"time"
)

// populateOrdinaryMonsters 为每个角色创建独立实体，新目录共用只读数值模板。
// 上一张普通图的可见槽数和未到期的复活时间随同一健康角色进入下一张普通图。
func populateOrdinaryMonsters(f *devconfig.Fixture, world *worldState, target *devconfig.MapTemplate, previous *worldState, now time.Time) bool {
	slots := devconfig.OrdinaryMonsterSlots
	initialCount := 1
	if target.SceneKindValue() != devconfig.SceneKindPersonal || len(target.MonsterSpawns) < slots || len(f.Monsters) == 0 {
		return false
	}
	if f.OrdinaryMonsterRosterEnabled() {
		initialCount = target.InitialMonsterCount
		if len(target.MonsterSpawnIDs) != slots || initialCount < 1 || initialCount > slots {
			return false
		}
		if previous != nil && previous.MonsterCount >= 1 && previous.MonsterCount <= slots {
			initialCount = previous.MonsterCount
		}
	}
	monsters := make([]*entity, 0, slots)
	for i := 0; i < slots; i++ {
		definition := &f.Monsters[i%len(f.Monsters)]
		appearanceID := definition.ID
		if f.OrdinaryMonsterRosterEnabled() {
			mapped, exists := f.MonsterByID(target.MonsterSpawnIDs[i])
			if !exists || mapped.DefinitionID == 0 {
				return false
			}
			definition = mapped
			appearanceID = mapped.DefinitionID
		} else {
			// 旧规则保留每个槽原先创建独立模板副本的行为。
			legacy := *definition
			definition = &legacy
		}
		monster := &entity{Handle: uint32(10 + i), Kind: 3, Def: appearanceID, Pos: target.MonsterSpawns[i],
			HP: definition.HP, MaxHP: definition.HP, Monster: definition}
		if previous != nil && i < len(previous.Monsters) {
			old := previous.Monsters[i]
			if old != nil && old.HP <= 0 && old.RespawnAt.After(now) {
				monster.HP, monster.DeferredUntilRespawn, monster.RespawnAt = 0, true, old.RespawnAt
			}
		}
		monsters = append(monsters, monster)
	}
	world.Monsters, world.MonsterCount = monsters, initialCount
	return true
}

// normalMonsterRespawnDelay 读取普通怪的独立复活时长。旧内存测试模板允许缺少该字段。
func normalMonsterRespawnDelay(monster *devconfig.Monster) time.Duration {
	if monster != nil && monster.RespawnMS > 0 {
		return time.Duration(monster.RespawnMS) * time.Millisecond
	}
	return 10 * time.Second
}

// selectOrdinaryDestination 解析直接换普通图或从城镇返回的目标。
func (w *worker) selectOrdinaryDestination(r *room, current *devconfig.MapTemplate, requested uint64) (*devconfig.MapTemplate, devconfig.Point, bool) {
	f := &w.e.Store.Cfg.Fixture
	personal := r.PersonalWorld
	if personal == nil {
		return nil, devconfig.Point{}, false
	}
	fromPersonal := current.SceneKindValue() == devconfig.SceneKindPersonal && r.currentWorld() == personal && r.SavedPersonal == nil
	fromTown := current.SceneKindValue() == devconfig.SceneKindTown && f.TownDestination(current.ID) && r.SavedPersonal == personal
	fromMine := miningMap(f, current.ID) && r.SavedPersonal == personal
	if !fromPersonal && !fromTown && !fromMine {
		return nil, devconfig.Point{}, false
	}
	if (fromTown || fromMine) && requested == 0 {
		requested = personal.MapID
	}
	target, ok := f.MapByID(requested)
	if !ok || target.SceneKindValue() != devconfig.SceneKindPersonal || target.ID == current.ID {
		return nil, devconfig.Point{}, false
	}
	position := target.PlayerSpawn
	if target.ID == personal.MapID {
		position = personal.PlayerPosition
	}
	return target, position, true
}

// adoptOrdinaryWorld 接替旧普通图，仅在禁宠情况下保留旧主体的驻留坐标和身份。
func (r *room) adoptOrdinaryWorld(target *worldState) {
	previous := r.PersonalWorld
	r.PersonalWorld, r.SavedPersonal = target, nil
	if previous != nil && previous != target && r.Pet != nil && r.PetWorld == previous.Ref {
		*previous = worldState{Ref: previous.Ref, MapID: previous.MapID, PetPosition: previous.PetPosition}
		r.PetHoldingWorld = previous
	}
}

// releasePetHoldingWorld 在宠物迁出或留存尸体到期后释放轻量驻留世界。
func (r *room) releasePetHoldingWorld() {
	if held := r.PetHoldingWorld; held != nil && r.PetWorld != held.Ref && len(held.PetCorpses) == 0 {
		r.PetHoldingWorld = nil
	}
}
