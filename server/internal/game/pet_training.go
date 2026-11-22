// 本文件冻结击杀对原代宠物的训练资格，并按确认经验更新等级和临时属性。
package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	"time"
)

// petRuntimeState 保存宠物运行时状态所需的状态。
type petRuntimeState struct {
	// Definition 记录定义，使用宠物运行时定义表示。
	Definition devconfig.PetRuntimeDefinition
	// Fighter 记录战斗单位。
	Fighter devconfig.Fighter
	// Train 标记是否处于训练状态。
	Train bool
	// LegacyUntrained 标记是否处于兼容旧数据“untrained”状态。
	LegacyUntrained bool
	// FollowSuppressed 标记是否处于“follow”“suppressed”状态。
	FollowSuppressed bool
	// Level 记录等级，使用数值表示。
	// Maximum 记录最大值，使用数值表示。
	Level, Maximum uint32
	// Experience 记录经验，使用数值表示。
	Experience uint64
}

// petTrainingCredit 保存宠物训练入账所需的状态。
type petTrainingCredit struct {
	// Pet 记录宠物，使用行动实体引用表示。
	Pet actorRef
	// KillID 标识当前业务使用的击杀。
	KillID string
	// SourceSeq 记录持久收益来源序号，使用数值表示。
	SourceSeq int64
	// OwnerEXP 记录所有者经验，使用数值表示。
	OwnerEXP int64
	// ActualEXP 记录实测经验，使用数值表示。
	ActualEXP int64
	// Amount 记录数量，使用数值表示。
	Amount uint64
	// Applied 标记是否处于“applied”状态。
	Applied bool
}

// attachLegacyPetState 关联兼容旧数据宠物状态。
// w 是调用该方法的房间执行器实例，pet 记录宠物，使用实体表示。
func (w *worker) attachLegacyPetState(pet *entity) bool {
	if pet.PetRuntime != nil {
		return true
	}
	// d 接收定义“by”消息正文标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	d, ok := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByBodyID(pet.Def)
	if !ok {
		return false
	}
	pet.PetRuntime = &petRuntimeState{Definition: d, Fighter: w.e.Store.Cfg.Fixture.Pet, LegacyUntrained: true}
	return true
}

// factory不安装居民、不扣资源；now必须是实际Spawn时刻，非请求到达时刻。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，definitionID 标识当前业务使用的定义，rank 记录排序位次，使用数值表示。
// point 记录点位。
func (w *worker) newPet(r *room, definitionID, rank uint32, point devconfig.Point, now time.Time) (*entity, bool) {
	if !w.petRuntimeEnabled() {
		return nil, false
	}
	// d 接收定义“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	d, ok := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByID(definitionID)
	if !ok {
		return nil, false
	}
	return w.newPetFromDefinition(r, d, rank, point, now)
}

// newPetFromDefinition 构造宠物来源定义。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，definition 记录定义，使用宠物运行时定义表示。
// rank 记录排序位次，使用数值表示，point 记录点位，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) newPetFromDefinition(r *room, definition devconfig.PetRuntimeDefinition, rank uint32, point devconfig.Point, now time.Time) (*entity, bool) {
	// d 接收副本供后续步骤使用。
	d := definition.Clone()
	if d.OwnerJobID != r.Role.Job || uint64(rank) >= uint64(len(d.Training.InitialLevelByRank)) {
		return nil, false
	}
	// level 保存从初始值等级“by”排序位次取出的等级，maximum 保存从最大值等级“by”排序位次取出的最大值。
	level, maximum := d.Training.InitialLevelByRank[rank], d.Training.MaximumLevelByRank[rank]
	// f 接收战斗单位发生时间供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	f, ok := d.FighterAt(level, maximum)
	if !ok {
		return nil, false
	}
	// direction 保存由无符号整数“32”取得的朝向。
	direction := uint32(0)
	if d.SourceSkillID == 17 {
		direction = skeletonSpawnDirection
	}
	return &entity{Kind: 2, Def: d.Base.DefinitionID, Pos: point, Direction: direction, HP: f.HP, MaxHP: f.HP, MP: f.MP, MaxMP: f.MP,
		ActionReadyAt: now.Add(time.Duration(d.Lifecycle.SpawnReadyMS) * time.Millisecond),
		PetRuntime:    &petRuntimeState{Definition: d, Fighter: f, Train: true, Level: level, Maximum: maximum}}, true
}

// findPetBirth 查找宠物“birth”。
// r 是调用该方法的房间实例，ref 记录引用，使用行动实体引用表示。
func (r *room) findPetBirth(ref actorRef) *entity {
	if ref.World.Epoch != r.ExecutionEpoch || r.world(ref.World) == nil {
		return nil
	}
	// world 接收遍历场景世界集合时当前的元素。
	for _, world := range r.worlds() {
		// body 接收遍历宠物“bodies”时当前的元素。
		for _, body := range r.petBodies(world) {
			if body.Handle == ref.Handle && body.Life == ref.Life {
				return body
			}
		}
	}
	return nil
}

// capturePetTraining 返回“capture”宠物训练供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，killID 标识当前业务使用的击杀。
func (w *worker) capturePetTraining(r *room, killID string) *petTrainingCredit {
	// pet 保存由场景世界宠物取得的宠物。
	pet := r.worldPet(r.currentWorld())
	if !w.petRuntimeEnabled() || pet == nil || pet.HP <= 0 || pet.PetRuntime == nil || !pet.PetRuntime.Train || pet.PetRuntime.Level >= pet.PetRuntime.Maximum ||
		distance(pet.Pos, r.Player.Pos) > pet.PetRuntime.Definition.Training.OwnerRangeCells {
		return nil
	}
	return &petTrainingCredit{Pet: r.petReference(), KillID: killID}
}

// preparePetTraining 判断准备宠物训练是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，credit 记录入账，使用宠物训练入账表示，q 记录结算。
func (w *worker) preparePetTraining(r *room, credit *petTrainingCredit, q persistence.Settlement) bool {
	if credit == nil {
		return true
	}
	// body 保存由查找宠物出生取得的消息正文。
	body := r.findPetBirth(credit.Pet)
	if body == nil || body.PetRuntime == nil {
		return true // 原临时对象已消失，不能从永久经验回执重建。
	}
	if q.Experience < 0 {
		return false
	}
	// actual 保存经验，用于当前步骤中的实测。
	actual := q.Experience
	if w.e.Store.Cfg.Fixture.RuleVersion >= 20 {
		// contract 保存由成长规则合同取得的规则合同，err 接收成长规则合同过程中可能产生的错误，供后续失败分支处理。
		contract, err := persistence.GrowthContract(w.e.Store.Cfg.Fixture)
		if err != nil {
			return false
		}
		// grant 保存由应用经验取得的授予。
		_, grant, err := domain.ApplyExperience(r.Role, q.Experience, contract)
		if err != nil {
			return false
		}
		actual = grant.ActualGranted
	}
	credit.ActualEXP = actual
	if body.PetRuntime.Level >= body.PetRuntime.Maximum {
		// 后续credit可能在先前grant升满前捕获；来源满级不再累计，不能让无效加法阻塞RoleEXP。
		credit.SourceSeq, credit.OwnerEXP, credit.Amount = q.SourceSeq, q.Experience, 0
		return true
	}
	if uint64(actual) > math.MaxUint64/body.PetRuntime.Definition.Training.ExperienceMultiplier {
		return false
	}
	// amount 保存根据无符号整数“64”与经验倍率计算的数量。
	amount := uint64(actual) * body.PetRuntime.Definition.Training.ExperienceMultiplier
	if body.PetRuntime.Experience > math.MaxUint64-amount {
		return false
	}
	credit.SourceSeq, credit.OwnerEXP, credit.Amount = q.SourceSeq, q.Experience, amount
	return true
}

// awardPetTraining 判断“award”宠物训练是否满足要求。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，credit 记录入账，使用宠物训练入账表示，q 记录结算。
// grants 记录授予集合，使用可变数量的经验赋予道具或权限回执表示。
func (w *worker) awardPetTraining(r *room, credit *petTrainingCredit, q persistence.Settlement, grants ...*persistence.ExperienceGrantReceipt) bool {
	if credit == nil || credit.Applied || credit.SourceSeq != q.SourceSeq || credit.OwnerEXP != q.Experience || q.SourceSeq <= 0 || q.SourceSeq > r.Role.SourceSeq {
		return false
	}
	if w.e.Store.Cfg.Fixture.RuleVersion >= 20 && (len(grants) != 1 || grants[0] == nil || grants[0].RawExperience != q.Experience || grants[0].ActualGranted < 0 || grants[0].ActualGranted > q.Experience) {
		return false
	}
	credit.Applied = true
	// body 保存由查找宠物出生取得的消息正文。
	body := r.findPetBirth(credit.Pet)
	if body == nil || body.PetRuntime == nil || !body.PetRuntime.Train {
		return false
	}
	// state 保存宠物运行时，用于当前步骤中的状态。
	state := body.PetRuntime
	if w.e.Store.Cfg.Fixture.RuleVersion >= 20 {
		// actual 保存实测已授予，用于当前步骤中的实测。
		actual := grants[0].ActualGranted
		if state.Level >= state.Maximum {
			credit.Amount = 0
		} else {
			if uint64(actual) > math.MaxUint64/state.Definition.Training.ExperienceMultiplier {
				w.isolateRoom(r)
				return false
			}
			credit.Amount = uint64(actual) * state.Definition.Training.ExperienceMultiplier
		}
		credit.ActualEXP = actual
	}
	if credit.Amount == 0 || state.Level >= state.Maximum || state.Experience > math.MaxUint64-credit.Amount {
		return false
	}
	state.Experience += credit.Amount
	// threshold 保存“required”经验，用于当前步骤中的“threshold”。
	threshold := state.Definition.Growth[state.Level].RequiredExperience
	if threshold == nil || state.Experience < *threshold {
		return credit.Amount > 0
	}
	// fighter 保存由战斗单位发生时间取得的战斗单位，ok 表明查询、校验或类型转换是否成功。
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

// petTrainingPB 返回宠物训练“pb”供调用方使用。
// e 记录实体。
func petTrainingPB(e *entity) *pb.PetTrainingState {
	// state 保存宠物运行时，用于当前步骤中的状态。
	state := e.PetRuntime
	if state == nil || !state.Train {
		return nil
	}
	// threshold 保存由无符号整数“64”取得的“threshold”。
	threshold := uint64(0)
	if state.Level < state.Maximum {
		// value 保存“required”经验，用于当前步骤中的值。
		if value := state.Definition.Growth[state.Level].RequiredExperience; value != nil {
			threshold = *value
		}
	}
	return &pb.PetTrainingState{TrainingLevel: state.Level, MaxTrainingLevel: state.Maximum, Experience: state.Experience, NextLevelExperience: threshold}
}
