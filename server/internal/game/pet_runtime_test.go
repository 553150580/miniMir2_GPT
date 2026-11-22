// 本文件以真实宠物规则夹具验证新旧宠物状态、寿命和死亡后的占位释放。
package game

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	"reflect"
	"testing"
	"time"
)

// 独立资格前态：显式小预算/TTL用于确定性模型测试，不是运行配置或来源数值认证。
// t 提供断言失败报告、子用例和清理注册，w 记录房间执行器，r 指向当前执行器拥有并更新的目标房间。
func enablePetRuntimeTest(t *testing.T, w *worker, r *room) devconfig.PetRuntimeDefinition {
	t.Helper()
	// base 保存宠物，用于当前步骤中的基础值。
	base := w.e.Store.Cfg.Fixture.Pet
	// d 组装宠物运行时定义所需的数据。
	d := devconfig.PetRuntimeDefinition{ID: 1, SourceSkillID: 17, MonsterTemplateID: 381, OwnerJobID: 2, ResourceKey: "skeleton", Base: base,
		Training:  devconfig.PetTrainingRules{InitialLevelByRank: []uint32{0, 1, 2, 3}, MaximumLevelByRank: []uint32{4, 5, 6, 7}, ExperienceMultiplier: 3, OwnerRangeCells: 16, ExperienceBasis: "owner_actual_granted_experience", GrantPhase: "confirmed_owner_experience", Eligibility: "alive_same_world_within_owner_range", MaxLevelUpsPerGrant: 1, ExcessExperience: "keep", AtMaximum: "do_not_accumulate", CurrentHPOnGrowth: "keep"},
		Speed:     devconfig.PetSpeedRules{Basis: "maximum_training_level", MoveReductionMSPerLevel: 130, AttackReductionMSPerLevel: 70, MinimumMoveIntervalMS: 400, MinimumAttackIntervalMS: 400},
		Lifecycle: devconfig.PetLifetimeRules{CorpseTTLMS: 1000, SpawnReadyMS: 2000, LiveExpiry: "no_tame_expiry", ColdEnter: "do_not_restore_temporary_pet"}}
	// i 标记循环或列表中的当前位置。
	for i := 0; i <= 7; i++ {
		// g 建立带初始值的宠物成长等级，供本段继续处理。
		g := devconfig.PetGrowthLevel{Level: uint32(i), HPBonus: int64(i) * 20, AttackMinBonus: int64(i), AttackMaxBonus: int64(i), DefenceMinBonus: int64(i) * 2, DefenceMaxBonus: int64(i) * 2, MagicDefenceMinBonus: int64(i) * 2, MagicDefenceMaxBonus: int64(i) * 2}
		if i < 7 {
			// n 保存根据无符号整数“64”与当前数据计算的无符号整数“64”与当前数据的计算结果。
			n := uint64(i+1) * 20000
			g.RequiredExperience = &n
		}
		d.Growth = append(d.Growth, g)
	}
	// err 接收校验过程中可能产生的错误，供后续失败分支处理。
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	w.e.Store.Cfg.Fixture.PetRuntime = &devconfig.PetRuntimeRules{Enabled: true, BootstrapMode: "legacy_untrained_pet", Budgets: devconfig.PetRuntimeBudgets{RetainedBodyCapacityPerOwner: 4, FullPolicy: "reject_birth_without_discarding_corpses"}, Definitions: []devconfig.PetRuntimeDefinition{d}}
	w.e.Store.Cfg.Fixture.RuleVersion = 16
	r.PetWorld = r.currentWorld().Ref
	r.Pet.Def, r.Role.Job = base.DefinitionID, base.JobID
	r.Pet.PetRuntime = nil // 这组显式隔离定义替换出生前态；不沿用真实fixture已冻结的180秒TTL。
	return d
}

// TestPetRuntimeActualRule16FixturePreservesLegacyStatsAndUsesProjectedLifetime 核验实际rule16与独立候选的定义投影应一致，以及真实初始化宠物应按唯一投影TTL留原尸体。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeActualRule16FixturePreservesLegacyStatsAndUsesProjectedLifetime(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// catalog 保存由加载宠物运行时候选集合取得的目录。
	catalog, err := devconfig.LoadPetRuntimeCandidates("../../../config/development/pet-runtime-candidates.json")
	if err != nil || catalog.RuntimeEnabled || cfg.Fixture.RuleVersion < 16 || !reflect.DeepEqual(catalog.Definitions, cfg.Fixture.PetRuntime.Definitions) {
		t.Fatal("实际rule16与独立候选的定义投影不一致", err)
	}
	// c 保存当前处理所用的已捕获连接。
	c := &capturedConnection{}
	// r 保存当前处理所用的房间。
	r := &room{Conn: c, Ready: true}
	// id 区分当前步骤指向的目标对象。
	id := domain.ID()
	// w 保存当前处理所用的房间执行器。
	w := &worker{e: &Engine{Store: &persistence.Store{Cfg: cfg}}, rooms: map[string]*room{id: r}}
	w.initialize(r, domain.Role{ID: id, Job: cfg.Fixture.Pet.JobID, Level: 1, Revision: 1})
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	if pet == nil || pet.Handle != 2 || pet.PetRuntime == nil || pet.PetRuntime.Train || !pet.PetRuntime.LegacyUntrained || !reflect.DeepEqual(w.fighter(r, pet), cfg.Fixture.Pet) {
		t.Fatal("真实初始化改变免费bootstrap属性或错误开启训练")
	}
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	w.applyDamageInWorld(r, r.currentWorld(), r.Monsters[0], pet, pet.HP, now)
	// d 接收定义“by”消息正文标识供后续步骤使用。
	d, _ := cfg.Fixture.PetRuntime.DefinitionByBodyID(pet.Def)
	if r.Pet != nil || len(r.PetCorpses) != 1 || r.PetCorpses[0].Body != pet || r.PetCorpses[0].ExpiresAt.Sub(now) != time.Duration(d.Lifecycle.CorpseTTLMS)*time.Millisecond {
		t.Fatal("真实初始化宠物没有按唯一投影TTL留原尸体")
	}
	// trained 保存由创建宠物取得的已训练，ok 表明查询、校验或类型转换是否成功。
	trained, ok := w.newPet(r, d.ID, 3, r.Player.Pos, now)
	if !ok || !w.installPet(r, r.currentWorld(), trained) || trained.Handle == 2 || !trained.PetRuntime.Train || r.PetCorpses[0].Body != pet {
		t.Fatal("真实定义factory不能独立安装正规新宠且保留原尸体")
	}
	t.Logf("真实fixture: rule%d retained=%d corpseTTL=%dms，旧bootstrap未训练；正规factory只在隔离测试调用", cfg.Fixture.RuleVersion, cfg.Fixture.PetRuntime.Budgets.RetainedBodyCapacityPerOwner, d.Lifecycle.CorpseTTLMS)
}

// TestPetRuntimeDeathReleasesActivityWithoutDestroyingOriginalBody 核对「真实死亡仍占活动Pet槽，无法在保留原尸体时新生」与「死亡原对象/身份/驻留丢失或被改写」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeDeathReleasesActivityWithoutDestroyingOriginalBody(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，personal 保存由场景世界房间执行器取得的个人。
	// town 保存由场景世界房间执行器取得的城镇，now 保存由场景世界房间执行器取得的当前时间。
	w, r, _, personal, town, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	// ref 接收引用供后续步骤使用。
	ref := reference(r, pet)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], pet, pet.HP, now)
	if r.Pet != nil {
		t.Fatal("真实死亡仍占活动Pet槽，无法在保留原尸体时新生")
	}
	if w.resolveInWorld(r, personal, ref) != pet || w.resolveInWorld(r, town, ref) != nil || pet.HP != 0 || pet.Life != ref.Life || pet.Handle != ref.Handle {
		t.Fatal("死亡原对象/身份/驻留丢失或被改写")
	}
}
