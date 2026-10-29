package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	"time"
)

type petRuntimeState struct {
	Definition       devconfig.PetRuntimeDefinition
	Fighter          devconfig.Fighter
	Train            bool
	LegacyUntrained  bool
	FollowSuppressed bool
	Level, Maximum   uint32
	Experience       uint64
}

type petTrainingCredit struct {
	Pet       actorRef
	KillID    string
	SourceSeq int64
	OwnerEXP  int64
	Amount    uint64
	Applied   bool
}

func (w *worker) attachLegacyPetState(pet *entity) bool {
	if pet.PetRuntime != nil {
		return true
	}
	d, ok := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByBodyID(pet.Def)
	if !ok {
		return false
	}
	pet.PetRuntime = &petRuntimeState{Definition: d, Fighter: w.e.Store.Cfg.Fixture.Pet, LegacyUntrained: true}
	return true
}

// factory不安装居民、不扣资源；now必须是实际Spawn时刻，非请求到达时刻。
func (w *worker) newPet(r *room, definitionID, rank uint32, point devconfig.Point, now time.Time) (*entity, bool) {
	if !w.petRuntimeEnabled() {
		return nil, false
	}
	d, ok := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByID(definitionID)
	if !ok {
		return nil, false
	}
	return w.newPetFromDefinition(r, d, rank, point, now)
}

func (w *worker) newPetFromDefinition(r *room, definition devconfig.PetRuntimeDefinition, rank uint32, point devconfig.Point, now time.Time) (*entity, bool) {
	d := definition.Clone()
	if d.OwnerJobID != r.Role.Job || uint64(rank) >= uint64(len(d.Training.InitialLevelByRank)) {
		return nil, false
	}
	level, maximum := d.Training.InitialLevelByRank[rank], d.Training.MaximumLevelByRank[rank]
	f, ok := d.FighterAt(level, maximum)
	if !ok {
		return nil, false
	}
	direction := uint32(0)
	if d.SourceSkillID == 17 {
		direction = skeletonSpawnDirection
	}
	return &entity{Kind: 2, Def: d.Base.DefinitionID, Pos: point, Direction: direction, HP: f.HP, MaxHP: f.HP, MP: f.MP, MaxMP: f.MP,
		ActionReadyAt: now.Add(time.Duration(d.Lifecycle.SpawnReadyMS) * time.Millisecond),
		PetRuntime:    &petRuntimeState{Definition: d, Fighter: f, Train: true, Level: level, Maximum: maximum}}, true
}

func (r *room) findPetBirth(ref actorRef) *entity {
	if ref.World.Epoch != r.ExecutionEpoch || r.world(ref.World) == nil {
		return nil
	}
	for _, world := range r.worlds() {
		for _, body := range r.petBodies(world) {
			if body.Handle == ref.Handle && body.Life == ref.Life {
				return body
			}
		}
	}
	return nil
}

func (w *worker) capturePetTraining(r *room, killID string) *petTrainingCredit {
	pet := r.worldPet(r.currentWorld())
	if !w.petRuntimeEnabled() || pet == nil || pet.HP <= 0 || pet.PetRuntime == nil || !pet.PetRuntime.Train || pet.PetRuntime.Level >= pet.PetRuntime.Maximum ||
		distance(pet.Pos, r.Player.Pos) > pet.PetRuntime.Definition.Training.OwnerRangeCells {
		return nil
	}
	return &petTrainingCredit{Pet: r.petReference(), KillID: killID}
}

func (w *worker) preparePetTraining(r *room, credit *petTrainingCredit, q persistence.Settlement) bool {
	if credit == nil {
		return true
	}
	body := r.findPetBirth(credit.Pet)
	if body == nil || body.PetRuntime == nil {
		return true // 原临时对象已消失，不能从永久经验回执重建。
	}
	if q.Experience < 0 {
		return false
	}
	if body.PetRuntime.Level >= body.PetRuntime.Maximum {
		// 后续credit可能在先前grant升满前捕获；来源满级不再累计，不能让无效加法阻塞RoleEXP。
		credit.SourceSeq, credit.OwnerEXP, credit.Amount = q.SourceSeq, q.Experience, 0
		return true
	}
	if uint64(q.Experience) > math.MaxUint64/body.PetRuntime.Definition.Training.ExperienceMultiplier {
		return false
	}
	amount := uint64(q.Experience) * body.PetRuntime.Definition.Training.ExperienceMultiplier
	if body.PetRuntime.Experience > math.MaxUint64-amount {
		return false
	}
	credit.SourceSeq, credit.OwnerEXP, credit.Amount = q.SourceSeq, q.Experience, amount
	return true
}

func (w *worker) awardPetTraining(r *room, credit *petTrainingCredit, q persistence.Settlement) bool {
	if credit == nil || credit.Applied || credit.SourceSeq != q.SourceSeq || credit.OwnerEXP != q.Experience || q.SourceSeq <= 0 || q.SourceSeq > r.Role.SourceSeq {
		return false
	}
	credit.Applied = true
	body := r.findPetBirth(credit.Pet)
	if body == nil || body.PetRuntime == nil || !body.PetRuntime.Train {
		return false
	}
	state := body.PetRuntime
	if credit.Amount == 0 || state.Level >= state.Maximum || state.Experience > math.MaxUint64-credit.Amount {
		return false
	}
	state.Experience += credit.Amount
	threshold := state.Definition.Growth[state.Level].RequiredExperience
	if threshold == nil || state.Experience < *threshold {
		return credit.Amount > 0
	}
	fighter, ok := state.Definition.FighterAt(state.Level+1, state.Maximum)
	if !ok {
		w.isolateRoom(r)
		return false
	}
	state.Experience -= *threshold
	state.Level++ // 来源每次grant只有一个if，不追赶超额经验。
	state.Fighter = fighter
	body.MaxHP, body.MaxMP = fighter.HP, fighter.MP
	return true
}

func petTrainingPB(e *entity) *pb.PetTrainingState {
	state := e.PetRuntime
	if state == nil || !state.Train {
		return nil
	}
	threshold := uint64(0)
	if state.Level < state.Maximum {
		if value := state.Definition.Growth[state.Level].RequiredExperience; value != nil {
			threshold = *value
		}
	}
	return &pb.PetTrainingState{TrainingLevel: state.Level, MaxTrainingLevel: state.Maximum, Experience: state.Experience, NextLevelExperience: threshold}
}
