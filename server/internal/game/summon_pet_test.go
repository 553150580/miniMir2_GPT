// 本文件回归召唤与召回的身份、资源预约、位置、延迟、尸体、禁宠地图和重试。
package game

import (
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"testing"
	"time"
)

// 隔离资格前态：主profile仍关闭17/材料，这些参数只验证正常handler及owner生命周期。
// t 提供断言失败报告、子用例和清理注册，withPet 标记是否处于带有宠物状态。
func summonWorker(t *testing.T, withPet bool) (*worker, *room, *capturedConnection, *worldState, *worldState, time.Time) {
	t.Helper()
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，c 接收场景世界房间执行器供后续步骤使用。
	// personal 保存由场景世界房间执行器取得的个人，town 保存由场景世界房间执行器取得的城镇，now 保存由场景世界房间执行器取得的当前时间。
	w, r, c, personal, town, now := worldWorker(t)
	// d 接收启用宠物运行时测试供后续步骤使用。
	d := enablePetRuntimeTest(t, w, r)
	d.RecallName = "BoneFamiliar"
	w.e.Store.Cfg.Fixture.PetRuntime.Definitions[0] = d
	w.e.Store.Cfg.Fixture.RuleVersion = 17
	// noPets 保存当前处理所用的“no”宠物集合。
	noPets := false
	w.e.Store.Cfg.Fixture.Map.NoPets = &noPets
	// m 保存当前处理所用的地图集合中的指定项。
	m := &w.e.Store.Cfg.Fixture.Maps[0]
	m.NoPets, m.SourceStaticBits, m.Source = &noPets, make([]byte, len(m.CollisionBits)), &devconfig.SourceMapIdentity{Code: "0", Width: 700, Height: 700}
	r.Role.Skills = []domain.LearnedSkill{{SkillID: 17}}
	// catalog 保存由加载召唤候选集合取得的目录，err 接收加载召唤候选集合过程中可能产生的错误，供后续失败分支处理。
	catalog, err := devconfig.LoadSummonCandidates("../../../config/development/summon-candidates.json")
	if err != nil {
		t.Fatal(err)
	}
	// skill 保存从技能集合取出的技能。
	skill := catalog.Skills[0]
	w.e.Store.Cfg.Fixture.Skills = append(w.e.Store.Cfg.Fixture.Skills, skill)
	r.Player.MP, r.Player.MaxMP = 1000, 1000
	r.Auto = false
	w.roll = func(int64) int64 { t.Fatal("召唤/Recall不应抽伤害随机数"); return 0 }
	if withPet {
		if !w.attachLegacyPetState(r.Pet) {
			t.Fatal("缺正式legacy定义")
		}
	} else {
		r.Pet, r.PetWorld = nil, worldRef{}
	}
	return w, r, c, personal, town, now
}

// paidSummonPlan 返回已支付召唤计划供调用方使用。
// t 提供断言失败报告、子用例和清理注册，w 记录房间执行器，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func paidSummonPlan(t *testing.T, w *worker, r *room, now time.Time) *preparedCast {
	t.Helper()
	// plan 保存由准备施法取得的计划，code 保存由准备施法取得的结果码。
	plan, code := w.prepareCast(r, 17, 0, r.Player.Direction, now, true)
	if code != 0 || !w.reserveSummonPet(r, plan, domain.ID(), now) {
		t.Fatalf("预留失败 code=%d", code)
	}
	if code = w.acceptPreparedCast(r, plan, 1, now); code != 0 {
		t.Fatalf("已核材料后受理失败%d", code)
	}
	return plan
}

// TestSummonPetCapturesAcceptedDirectionAndRejectNeverTurns 核对「出生前格或BoneFamiliar初始方向错误」与「未按合法请求方向冻结Front/prepare提前Turn」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetCapturesAcceptedDirectionAndRejectNeverTurns(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, _, _, now := summonWorker(t, false)
	r.Player.Direction = 4
	// code 保存由准备施法取得的结果码。
	if _, code := w.prepareCast(r, 17, 0, 2, now, false); code != p.CodeSkillMaterialRequired || r.Player.Direction != 4 {
		t.Fatal("材料拒绝提前转向", code)
	}
	// plan 保存由准备施法取得的计划，code 保存由准备施法取得的结果码。
	plan, code := w.prepareCast(r, 17, 0, 2, now, true)
	if code != 0 || plan.Summon.Front != (devconfig.Point{X: r.Player.Pos.X + 1, Y: r.Player.Pos.Y}) || r.Player.Direction != 4 {
		t.Fatal("未按合法请求方向冻结Front/prepare提前Turn", code)
	}
	if !w.reserveSummonPet(r, plan, domain.ID(), now) || w.acceptPreparedCast(r, plan, 1, now) != 0 || r.Player.Direction != 2 {
		t.Fatal("真正接受后未应用方向")
	}
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Pet == nil || r.Pet.Pos != plan.Summon.Front || r.Pet.Direction != skeletonSpawnDirection {
		t.Fatal("出生前格或BoneFamiliar初始方向错误")
	}
}

// TestSummonPetFreshRequiresMaterialAndDoesNotReserveOnCastIntent 核对「普通Fresh绕过材料或拒绝后泄漏预留」与「NoPets受理放行」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetFreshRequiresMaterialAndDoesNotReserveOnCastIntent(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, _, _, now := summonWorker(t, false)
	// mp 保存当前处理所用的魔法值。
	mp := r.Player.MP
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 17, 0, 1, now); code != p.CodeSkillMaterialRequired || r.PendingPetBirth != nil || r.Player.MP != mp || r.eventCount() != 0 {
		t.Fatal("普通Fresh绕过材料或拒绝后泄漏预留", code)
	}
	// yes 保存当前处理所用的“yes”。
	yes := true
	w.e.Store.Cfg.Fixture.Map.NoPets = &yes
	// code 保存由准备施法取得的结果码。
	if _, code := w.prepareCast(r, 17, 0, 0, now, true); code != p.CodeSkillStateBlocked {
		t.Fatal("NoPets受理放行", code)
	}
}

// TestSummonPetPaid500msKeepsOriginalWorldFrontAndActualSpawnClock 核对「原图出生/实际Spawn时钟/预留交接错误」与「baseline未携带owner观察」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetPaid500msKeepsOriginalWorldFrontAndActualSpawnClock(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，personal 保存由召唤房间执行器取得的个人。
	// town 保存由召唤房间执行器取得的城镇，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, personal, town, now := summonWorker(t, false)
	// plan 保存由已支付召唤计划取得的计划。
	plan := paidSummonPlan(t, w, r, now)
	if r.Pet != nil || r.PendingPetBirth == nil || r.Player.MP != 1000-plan.Cost {
		t.Fatal("预生已成居民或MP扣耗错误")
	}
	w.advanceEvents(r, now.Add(500*time.Millisecond-time.Nanosecond))
	if r.Pet != nil {
		t.Fatal("早于500ms出生")
	}
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	r.Scene++
	// spawnAt 保存由新增取得的生成或出生点发生时间。
	spawnAt := now.Add(725 * time.Millisecond)
	w.advanceEvents(r, spawnAt)
	if r.Pet == nil || r.petResidentWorld() != personal || r.Pet.Pos != plan.Summon.Front || r.PendingPetBirth != nil || r.Pet.ActionReadyAt != spawnAt.Add(2*time.Second) || !r.Pet.PetRuntime.Train {
		t.Fatal("原图出生/实际Spawn时钟/预留交接错误")
	}
	// owned 保存由“owned”宠物“pb”取得的“owned”。
	owned := w.ownedPetPB(r)
	if owned == nil || owned.MapId != 1 || len(owned.RecallSkillIds) != 1 || owned.RecallSkillIds[0] != 17 {
		t.Fatal("异图活动宠物owner观察丢失")
	}
	w.baseline(r)
	// chunk 保存当前处理所用的分片。
	chunk := &pb.SceneBaselineChunk{}
	_ = proto.Unmarshal(r.Conn.(*capturedConnection).frames[len(r.Conn.(*capturedConnection).frames)-1].Payload, chunk)
	// e 接收遍历实体时当前的元素。
	for _, e := range chunk.Entities {
		if e.EntityHandle == r.Pet.Handle {
			t.Fatal("异图宠物被注册当前场景")
		}
	}
	if chunk.OwnedPet == nil || chunk.OwnedPet.Life != r.Pet.Life {
		t.Fatal("baseline未携带owner观察")
	}
}

// TestSummonPetFrontInvalidFallsBackWithoutDueNoPetsGate 核对「无效原Front未回退到主人当前图脚点，或错误重查NoPets」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetFrontInvalidFallsBackWithoutDueNoPetsGate(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，personal 保存由召唤房间执行器取得的个人。
	// town 保存由召唤房间执行器取得的城镇，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, personal, town, now := summonWorker(t, false)
	// plan 保存由已支付召唤计划取得的计划。
	plan := paidSummonPlan(t, w, r, now)
	w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{plan.Summon.Front}
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	// yes 保存当前处理所用的“yes”。
	yes := true
	w.e.Store.Cfg.Fixture.Maps[0].NoPets = &yes
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Pet == nil || r.petResidentWorld() != town || r.Pet.Pos != r.Player.Pos || r.PendingPetBirth != nil || personal == town {
		t.Fatal("无效原Front未回退到主人当前图脚点，或错误重查NoPets")
	}
}

// TestSummonPetStaticWalkAllowsClosedDoorAndDynamicOverlap 核对「源静态Walk合法点被闭门/动态占格错误拒绝」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetStaticWalkAllowsClosedDoorAndDynamicOverlap(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，town 保存由召唤房间执行器取得的城镇。
	// now 保存由召唤房间执行器取得的当前时间。
	w, r, _, _, town, now := summonWorker(t, false)
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	r.Player.Direction = 2
	// front 保存由步骤点位取得的前端。
	front := stepPoint(r.Player.Pos, r.Player.Direction, 1)
	// m 保存当前处理所用的地图集合中的指定项。
	m := &w.e.Store.Cfg.Fixture.Maps[0]
	// cell 保存根据纵坐标与宽度的计算结果与横坐标计算的地图格。
	cell := front.Y*m.Width + front.X
	m.CollisionBits[cell/8] |= 1 << uint(cell%8)
	town.Monsters = []*entity{{Handle: 10, Kind: 3, HP: 100, Pos: front, Monster: &devconfig.Monster{Fighter: devconfig.Fighter{HP: 100}}}}
	town.MonsterCount = 1
	paidSummonPlan(t, w, r, now)
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Pet == nil || r.Pet.Pos != front {
		t.Fatal("源静态Walk合法点被闭门/动态占格错误拒绝")
	}
}

// TestSummonPetMaterialModeConflictDoesNotRequireSupplyOrStartRecall 核对「模式冲突后无符正常Recall无法受理」与「Fresh RPC模式冲突未明确拒绝/先耗符/转Recall」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetMaterialModeConflictDoesNotRequireSupplyOrStartRecall(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, _, _, now := summonWorker(t, true)
	// plan 保存由准备材料施法取得的计划，code 保存由准备材料施法取得的结果码。
	plan, code := w.prepareMaterialCast(r, persistence.AssetRequest{SkillID: 17}, &pb.CastWithMaterialRequest{SkillId: 17}, now)
	if plan != nil || code != p.CodeSkillStateBlocked || r.PendingPetBirth != nil || r.eventCount() != 0 || r.Player.MP != 1000 {
		t.Fatal("Fresh RPC模式冲突未明确拒绝/先耗符/转Recall", code)
	}
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 17, 0, 1, now); code != 0 {
		t.Fatal("模式冲突后无符正常Recall无法受理", code)
	}
}

// TestSummonPetAutomaticRecallPrecedesSpawnActionClockWithoutResettingClocks 核对「自动Recall受2000锁/重置原宠动作时钟」与「未先在原图出生」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetAutomaticRecallPrecedesSpawnActionClockWithoutResettingClocks(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，personal 保存由召唤房间执行器取得的个人。
	// town 保存由召唤房间执行器取得的城镇，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, personal, town, now := summonWorker(t, false)
	paidSummonPlan(t, w, r, now)
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	r.Player.Direction = 2
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Pet == nil || r.petResidentWorld() != personal {
		t.Fatal("未先在原图出生")
	}
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	pet.NextMove, pet.NextAttack = now.Add(4*time.Second), now.Add(5*time.Second)
	// action 保存动作就绪发生时间，用于当前步骤中的动作，move 保存下一状态移动，用于当前步骤中的移动，attack 保存下一状态攻击，用于当前步骤中的攻击。
	action, move, attack := pet.ActionReadyAt, pet.NextMove, pet.NextAttack
	w.tickResidentPetFollow(r, now.Add(500*time.Millisecond))
	if r.petResidentWorld() != town || pet.ActionReadyAt != action || pet.NextMove != move || pet.NextAttack != attack || !now.Add(500*time.Millisecond).Before(action) {
		t.Fatal("自动Recall受2000锁/重置原宠动作时钟")
	}
}

// TestSummonPetQueuedBirthIsBoundedAndNoPathFailureReleasesOnlyOwnToken 核对「reservation未占唯一slot」与「两个静态点都无效仍出生/泄漏预留」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetQueuedBirthIsBoundedAndNoPathFailureReleasesOnlyOwnToken(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，town 保存由召唤房间执行器取得的城镇。
	// now 保存由召唤房间执行器取得的当前时间。
	w, r, _, _, town, now := summonWorker(t, false)
	// plan 保存由已支付召唤计划取得的计划。
	plan := paidSummonPlan(t, w, r, now)
	if w.canBeginPetBirth(r, now) || w.reservePetBirth(r, petBirthToken{ID: "other", ExecutionEpoch: r.ExecutionEpoch, PlayerLife: r.Player.Life}, now) {
		t.Fatal("pending reservation未占唯一slot")
	}
	w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{plan.Summon.Front}
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	// m 保存当前处理所用的地图集合中的指定项。
	m := &w.e.Store.Cfg.Fixture.Maps[0]
	// cell 保存根据纵坐标与宽度的计算结果与横坐标计算的地图格。
	cell := r.Player.Pos.Y*m.Width + r.Player.Pos.X
	m.SourceStaticBits[cell/8] |= 1 << uint(cell%8)
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Pet != nil || r.PendingPetBirth != nil || r.eventCount() != 0 {
		t.Fatal("两个静态点都无效仍出生/泄漏预留")
	}
}

// TestSummonPetMaterialFailureOrActorChangeCannotLeakBirth 核对「失败/旧actor仍放行或遗留slot」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetMaterialFailureOrActorChangeCannotLeakBirth(t *testing.T) {
	// change 接收遍历文本列表时当前的元素。
	for _, change := range []string{"business_failure", "life", "scene", "auth"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(change, func(t *testing.T) {
			// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，c 接收召唤房间执行器供后续步骤使用。
			// now 保存由召唤房间执行器取得的当前时间。
			w, r, c, _, _, now := summonWorker(t, false)
			// plan 保存由准备施法取得的计划，code 保存由准备施法取得的结果码。
			plan, code := w.prepareCast(r, 17, 0, 0, now, true)
			if code != 0 {
				t.Fatal(code)
			}
			// q 组装资产请求所需的数据。
			q := persistence.AssetRequest{Kind: "cast_material", Fingerprint: "fixed", Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID())}}
			w.submitMaterialCast(r, q, plan, 1, nil)
			// binding 初始化用作会话绑定的材料提交会话绑定。
			binding := materialCommitBinding{roleID: r.Role.ID, room: r, auth: r.Auth, scene: r.Scene, life: r.Player.Life, generation: r.MaterialGeneration, conn: c}
			// result 保存当前处理所用的结果。
			result := &pb.OperationResult{}
			switch change {
			case "business_failure":
				result.ErrorCode = p.CodeSkillStateBlocked
			case "life":
				r.Player.Life++
			case "scene":
				r.Scene++
			case "auth":
				r.Auth.SessionEpoch++
			}
			// role 保存当前处理所用的角色。
			role := r.Role
			role.Revision++
			role.EconomicSeq++
			w.finishMaterialCast(binding, q, plan, 1, nil, role, result, persistence.Operation{Kind: "cast_material", CommandSeq: role.EconomicSeq, Casting: &persistence.CastingReceipt{Scope: "manual", SkillID: 17}}, nil, &job{})
			if r.PendingPetBirth != nil || r.eventCount() != 0 || r.Pending {
				t.Fatal("失败/旧actor仍放行或遗留slot")
			}
		})
	}
}

// TestSummonPetRecallNoMaterialSameWorldAndSameNameDifferentDefinition 核对「同图Recall搬宠或重建birth」与「Recall错误预留/扣MP」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetRecallNoMaterialSameWorldAndSameNameDifferentDefinition(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, _, _, now := summonWorker(t, true)
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	pet.PetRuntime.Definition.ID = 9
	pet.PetRuntime.Definition.MonsterTemplateID = 999
	// pos 保存当前处理所用的位置，life 保存当前处理所用的生命代次，handleSequence 保存宠物实体句柄序号，用于当前步骤中的实体句柄序号。
	pos, life, handleSequence := pet.Pos, pet.Life, r.PetHandleSequence
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 17, 0, 1, now); code != 0 {
		t.Fatal("无符同名Recall拒绝", code)
	}
	// skill 接收技能供后续步骤使用。
	skill, _ := w.e.Store.Cfg.Fixture.Skill(17)
	if r.PendingPetBirth != nil || r.Player.MP != 1000-skill.MPBaseCost {
		t.Fatal("Recall错误预留/扣MP")
	}
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Pet != pet || pet.Pos != pos || pet.Life != life || r.PetHandleSequence != handleSequence {
		t.Fatal("同图Recall搬宠或重建birth")
	}
	pet.PetRuntime.Definition.RecallName = "Other"
	r.Player.ActionReadyAt = time.Time{}
	r.NextSpell = time.Time{}
	// code 保存由准备施法取得的结果码。
	if _, code := w.prepareCast(r, 17, 0, 0, now.Add(2*time.Second), true); code != p.CodeSkillStateBlocked {
		t.Fatal("不同名活宠未按单宠拒新生")
	}
}

// TestSummonPetActualCandidateValidatesCompleteSkillWithoutMainActivation 核对「真实完整候选未通过全部规则验证」与「真实legacy owner观察误开训练或漏Recall」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetActualCandidateValidatesCompleteSkillWithoutMainActivation(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// catalog 保存由加载召唤候选集合取得的目录。
	catalog, err := devconfig.LoadSummonCandidates("../../../config/development/summon-candidates.json")
	if err != nil {
		t.Fatal(err)
	}
	// selected 保存由技能取得的已选择。
	if _, selected := cfg.Fixture.Skill(17); selected || catalog.RuntimeEnabled || cfg.Fixture.CastingSupply.Enabled {
		t.Fatal("独立候选错误启主入口")
	}
	cfg.Fixture.Skills = append(cfg.Fixture.Skills, catalog.Skills...)
	cfg.Fixture.Items = append(cfg.Fixture.Items, catalog.Books...)
	cfg.Fixture.SkillUpgrades = append(cfg.Fixture.SkillUpgrades, catalog.Upgrades...)
	// err 接收校验过程中可能产生的错误，供后续失败分支处理。
	if err := cfg.Fixture.Validate(); err != nil {
		t.Fatal("真实完整候选未通过全部规则验证", err)
	}
	// c 保存当前处理所用的已捕获连接。
	c := &capturedConnection{}
	// id 区分当前步骤指向的目标对象。
	id := domain.ID()
	// r 保存当前处理所用的房间。
	r := &room{Scene: 20, Baseline: 30, Conn: c, Ready: true}
	// w 保存当前处理所用的房间执行器。
	w := &worker{e: &Engine{Store: &persistence.Store{Cfg: cfg}}, rooms: map[string]*room{id: r}}
	w.initialize(r, domain.Role{ID: id, Job: 2, Level: 19, Revision: 1, Skills: []domain.LearnedSkill{{SkillID: 17}}})
	// mp 保存当前处理所用的魔法值。
	mp := r.Player.MP
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 17, 0, 1, time.Now()); code != 0 || r.PendingPetBirth != nil || r.Player.MP != mp-catalog.Skills[0].MPBaseCost {
		t.Fatal("真实完整定义的legacy同名无符Recall失败", code)
	}
	// pet 保存由“owned”宠物“pb”取得的宠物。
	if pet := w.ownedPetPB(r); pet == nil || len(pet.RecallSkillIds) != 1 || pet.RecallSkillIds[0] != 17 || pet.PetTraining != nil {
		t.Fatal("真实legacy owner观察误开训练或漏Recall")
	}
}

// TestSummonPetRecallFollowsOriginalBirthAcrossWorldChangeAndBackFallback 核对「Recall旧World或Back无效回退错误」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetRecallFollowsOriginalBirthAcrossWorldChangeAndBackFallback(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，personal 保存由召唤房间执行器取得的个人。
	// town 保存由召唤房间执行器取得的城镇，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, personal, town, now := summonWorker(t, true)
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 17, 0, 1, now); code != 0 {
		t.Fatal(code)
	}
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	r.Player.Direction = 2
	// back 保存由步骤点位取得的回退。
	back := stepPoint(r.Player.Pos, 6, 1)
	// m 保存当前处理所用的地图集合中的指定项。
	m := &w.e.Store.Cfg.Fixture.Maps[0]
	// cell 保存根据纵坐标与当前数据的计算结果与横坐标计算的地图格。
	cell := back.Y*700 + back.X
	m.SourceStaticBits[cell/8] |= 1 << uint(cell%8)
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Pet != pet || r.petResidentWorld() != town || pet.Pos != r.Player.Pos || personal == town {
		t.Fatal("Recall旧World或Back无效回退错误")
	}
}

// TestSummonPetRecallNoPetsSuppressesThenAutomaticResidentFollowResumes 核对「NoPets未留原宠/冻结」与「地图允许后异图宠无法恢复」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetRecallNoPetsSuppressesThenAutomaticResidentFollowResumes(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，personal 保存由召唤房间执行器取得的个人。
	// town 保存由召唤房间执行器取得的城镇，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, personal, town, now := summonWorker(t, true)
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 17, 0, 1, now); code != 0 {
		t.Fatal(code)
	}
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	r.Player.Direction = 2
	// yes 保存当前处理所用的“yes”。
	yes := true
	w.e.Store.Cfg.Fixture.Maps[0].NoPets = &yes
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.petResidentWorld() != personal || !pet.PetRuntime.FollowSuppressed {
		t.Fatal("NoPets未留原宠/冻结")
	}
	// no 保存当前处理所用的“no”。
	no := false
	w.e.Store.Cfg.Fixture.Maps[0].NoPets = &no
	w.tickResidentPetFollow(r, now.Add(time.Second))
	if r.Pet != pet || r.petResidentWorld() != town || pet.Pos != stepPoint(r.Player.Pos, 6, 1) || pet.PetRuntime.FollowSuppressed {
		t.Fatal("地图允许后异图宠无法恢复")
	}
}

// TestSummonPetMasterDeathKillsResidentPetAndNeverCompletesBirthOrRecall 核对「主人死亡后17仍出生/Recall复活」与「主人死亡未杀异图原宠/留原尸体」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetMasterDeathKillsResidentPetAndNeverCompletesBirthOrRecall(t *testing.T) {
	// fresh 接收遍历布尔状态列表时当前的元素。
	for _, fresh := range []bool{false, true} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(map[bool]string{false: "recall", true: "fresh"}[fresh], func(t *testing.T) {
			// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，personal 保存由召唤房间执行器取得的个人。
			// town 保存由召唤房间执行器取得的城镇，now 保存由召唤房间执行器取得的当前时间。
			w, r, _, personal, town, now := summonWorker(t, !fresh)
			// pet 记录宠物，使用实体表示。
			var pet *entity
			if fresh {
				paidSummonPlan(t, w, r, now)
			} else {
				pet = r.Pet
				// code 保存由施法取得的结果码。
				if code := w.cast(r, 17, 0, 1, now); code != 0 {
					t.Fatal(code)
				}
			}
			r.worldState = town
			r.Player.Pos = devconfig.Point{X: 633, Y: 613}
			w.applyDamageInWorld(r, town, &entity{Kind: 3}, &r.Player, r.Player.HP, now.Add(time.Millisecond))
			if !fresh && (r.Pet != nil || len(personal.PetCorpses) != 1 || personal.PetCorpses[0].Body != pet) {
				t.Fatal("主人死亡未杀异图原宠/留原尸体")
			}
			w.advanceEvents(r, now.Add(500*time.Millisecond))
			if r.Pet != nil || r.PendingPetBirth != nil {
				t.Fatal("主人死亡后17仍出生/Recall复活")
			}
		})
	}
}

// TestSummonPetTravelNoPetsRetainsBodyWithoutPetPointOrExtraMoveBudget 核对「NoPets旅行仍搬宠或要求宠落点/额外移动预算」与「冻结宠越过NoPets自动回来」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetTravelNoPetsRetainsBodyWithoutPetPointOrExtraMoveBudget(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，personal 保存由召唤房间执行器取得的个人。
	// town 保存由召唤房间执行器取得的城镇，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, personal, town, now := summonWorker(t, true)
	// pet 保存当前处理所用的宠物，pos 保存当前处理所用的位置。
	pet, pos := r.Pet, r.Pet.Pos
	// yes 保存当前处理所用的“yes”。
	yes := true
	w.e.Store.Cfg.Fixture.Maps[0].NoPets = &yes
	r.CellEntrySequence = math.MaxUint64 - 1
	if !w.activateWorld(r, town, devconfig.Point{X: 633, Y: 613}, nil) || r.Pet != pet || r.petResidentWorld() != personal || pet.Pos != pos || !pet.PetRuntime.FollowSuppressed {
		t.Fatal("NoPets旅行仍搬宠或要求宠落点/额外移动预算")
	}
	r.Player.MovementRevision = 0
	r.CellEntrySequence = 100
	w.tickResidentPetFollow(r, now.Add(2*time.Second))
	if r.petResidentWorld() != personal {
		t.Fatal("冻结宠越过NoPets自动回来")
	}
}

// TestSummonPetMaterialUnknownPreservesReservationAndSingleEvent 核对「unknown释放预留或先放临时效果」与「成功未从原预留一次放行」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetMaterialUnknownPreservesReservationAndSingleEvent(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，c 接收召唤房间执行器供后续步骤使用。
	// now 保存由召唤房间执行器取得的当前时间。
	w, r, c, _, _, now := summonWorker(t, false)
	// plan 保存由准备施法取得的计划，code 保存由准备施法取得的结果码。
	plan, code := w.prepareCast(r, 17, 0, 0, now, true)
	if code != 0 {
		t.Fatal(code)
	}
	// q 组装资产请求所需的数据。
	q := persistence.AssetRequest{Kind: "cast_material", Fingerprint: "immutable", Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID())}}
	// frame 组装协议帧所需的数据。
	frame := wire.Frame{Request: 99}
	w.submitMaterialCast(r, q, plan, 1, &frame)
	// binding 初始化用作会话绑定的材料提交会话绑定。
	binding := materialCommitBinding{roleID: r.Role.ID, room: r, auth: r.Auth, scene: r.Scene, life: r.Player.Life, generation: r.MaterialGeneration, conn: c}
	// reservation 保存待确认宠物出生，用于当前步骤中的“reservation”。
	reservation := r.PendingPetBirth
	// result 保存当前处理所用的结果。
	result := &pb.OperationResult{Pending: true}
	w.finishMaterialCast(binding, q, plan, 1, &frame, domain.Role{}, result, persistence.Operation{}, persistence.Fail(p.CodeOperationPending, "unknown"), &job{})
	if !r.Pending || r.PendingPetBirth != reservation || r.PendingRetry == nil || r.eventCount() != 0 {
		t.Fatal("unknown释放预留或先放临时效果")
	}
	// role 保存当前处理所用的角色。
	role := r.Role
	role.Revision++
	role.EconomicSeq++
	// receipt 初始化用作回执的操作。
	receipt := persistence.Operation{Kind: "cast_material", CommandSeq: role.EconomicSeq, Casting: &persistence.CastingReceipt{Scope: "manual", SkillID: 17, Quantity: 1}}
	w.finishMaterialCast(binding, q, plan, 1, &frame, role, &pb.OperationResult{}, receipt, nil, &job{})
	if r.Pending || r.PendingPetBirth != reservation || r.eventCount() != 1 {
		t.Fatal("成功未从原预留一次放行")
	}
	w.finishMaterialCast(binding, q, plan, 1, &frame, role, &pb.OperationResult{}, receipt, nil, &job{})
	if r.eventCount() != 1 {
		t.Fatal("迟到同回调重复排召唤")
	}
}

// TestSummonPetOldRecallCannotMoveReplacementBirth 核验不能安装合法新birth，用诊断「旧Recall命中新birth/抹掉原尸体」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetOldRecallCannotMoveReplacementBirth(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，personal 保存由召唤房间执行器取得的个人。
	// town 保存由召唤房间执行器取得的城镇，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, personal, town, now := summonWorker(t, true)
	// old 保存宠物，用于当前步骤中的原状态。
	old := r.Pet
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 17, 0, 1, now); code != 0 {
		t.Fatal(code)
	}
	w.applyDamageInWorld(r, personal, personal.Monsters[0], old, old.HP, now.Add(time.Millisecond))
	// newPet 接收创建宠物供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	newPet, ok := w.newPet(r, 1, 0, devconfig.Point{X: 633, Y: 614}, now.Add(2*time.Millisecond))
	if !ok || !w.installPet(r, town, newPet) {
		t.Fatal("不能安装合法新birth")
	}
	// pos 保存当前处理所用的位置。
	pos := newPet.Pos
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Pet != newPet || r.petResidentWorld() != town || newPet.Pos != pos || len(personal.PetCorpses) != 1 || personal.PetCorpses[0].Body != old {
		t.Fatal("旧Recall命中新birth/抹掉原尸体")
	}
}

// TestSummonPetMasterLifeChangeCancelsOnlyOriginalBirth 核对「旧主人Life仍出生或泄漏预留」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetMasterLifeChangeCancelsOnlyOriginalBirth(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, _, _, now := summonWorker(t, false)
	paidSummonPlan(t, w, r, now)
	r.Player.Life++
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.PendingPetBirth != nil || r.Pet != nil {
		t.Fatal("旧主人Life仍出生或泄漏预留")
	}
}

// TestSummonPetSharedBatchPreservesPrefixAndSuffix 核对「首中末召唤丢批前后缀」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetSharedBatchPreservesPrefixAndSuffix(t *testing.T) {
	// order 保存由无符号整数“64”取得的顺序。
	for order := uint64(1); order <= 3; order++ {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(string(rune('0'+order)), func(t *testing.T) {
			// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，personal 保存由召唤房间执行器取得的个人。
			// now 保存由召唤房间执行器取得的当前时间。
			w, r, _, personal, _, now := summonWorker(t, false)
			paidSummonPlan(t, w, r, now)
			w.roll = func(int64) int64 { return 0 } // 原攻击仍按自身到期命中抽样；17准备时已由fatal oracle覆盖。
			personal.Casts[0].Order = order
			// monster 保存从怪物集合取出的怪物。
			monster := personal.Monsters[0]
			monster.Monster.Agility = 0
			// hp 保存当前处理所用的生命值。
			hp := r.Player.HP
			// next 保存由无符号整数“64”取得的下一状态。
			for next := uint64(1); next <= 3; next++ {
				if next != order {
					personal.Attacks = append(personal.Attacks, pendingAttack{Caster: referenceIn(personal, monster), Target: reference(r, &r.Player), Due: now.Add(500 * time.Millisecond), Order: next, Amount: 1, Defence: defenceAgility})
				}
			}
			w.advanceEvents(r, now.Add(500*time.Millisecond))
			if r.Pet == nil || r.Player.HP != hp-2 || r.eventCount() != 0 || r.InflightEvents != 0 {
				t.Fatalf("首中末召唤丢批前后缀：HP=%d/%d events=%d", r.Player.HP, hp, r.eventCount())
			}
		})
	}
}

// TestSummonPetEventCapacityAndBodyReservationRejectBeforeEffect 核对「共享事件满额仍预留/受理」与「birth耗尽仍受理」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetEventCapacityAndBodyReservationRejectBeforeEffect(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，personal 保存由召唤房间执行器取得的个人。
	// now 保存由召唤房间执行器取得的当前时间。
	w, r, _, personal, _, now := summonWorker(t, false)
	w.e.Store.Cfg.Fixture.CombatTiming.EventCapacity = 1
	personal.Casts = []pendingCast{{Due: now.Add(time.Hour)}}
	// code 保存由准备施法取得的结果码。
	if _, code := w.prepareCast(r, 17, 0, 0, now, true); code != p.CodeSkillStateBlocked || r.PendingPetBirth != nil {
		t.Fatal("共享事件满额仍预留/受理")
	}
	personal.Casts = nil
	r.PetLifeSequence = math.MaxUint64
	// code 保存由准备施法取得的结果码。
	if _, code := w.prepareCast(r, 17, 0, 0, now, true); code != p.CodeSkillStateBlocked || r.PendingPetBirth != nil {
		t.Fatal("birth耗尽仍受理")
	}
}

// TestSummonPetRecallResolvesPetAfterItsWorldChangedDuringDelay 核对「旧宠World变化使Recall静默丢失」与「原宠迁移失败」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetRecallResolvesPetAfterItsWorldChangedDuringDelay(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，personal 保存由召唤房间执行器取得的个人。
	// town 保存由召唤房间执行器取得的城镇，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, personal, town, now := summonWorker(t, true)
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 17, 0, 1, now); code != 0 {
		t.Fatal(code)
	}
	if !w.relocatePet(r, r.petReference(), town, devconfig.Point{X: 633, Y: 613}) {
		t.Fatal("原宠迁移失败")
	}
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Pet != pet || r.petResidentWorld() != personal || pet.Pos != stepPoint(r.Player.Pos, (r.Player.Direction+4)%8, 1) {
		t.Fatal("旧宠World变化使Recall静默丢失")
	}
}

// TestSummonPetOwnedMetadataIsIdenticalInAllWireChunks 核对「分块owner观察不一致或超帧」与「未形成真实wire分块」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetOwnedMetadataIsIdenticalInAllWireChunks(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用。
	w, r, _, _, _, _ := summonWorker(t, true)
	// base 保存当前处理所用的基础值。
	base := &pb.SceneBaselineChunk{SceneEpoch: r.Scene, BaselineId: r.Baseline, StateSequence: 9, OwnedPet: w.ownedPetPB(r)}
	// i 保存由无符号整数“32”取得的当前位置的下标。
	for i := uint32(1001); i < 1129; i++ {
		// body 保存当前处理所用的消息正文。
		body := &pb.EntityState{EntityHandle: i, EntityKind: 2, IsPetCorpse: true}
		// j 保存由无符号整数“32”取得的当前位置的下标。
		for j := uint32(1); j <= 64; j++ {
			body.Statuses = append(body.Statuses, &pb.EntityStatusState{StatusId: j, SourceSkillId: math.MaxUint32, RemainingMs: math.MaxUint32, SkillLevel: math.MaxUint32})
		}
		base.Entities = append(base.Entities, body)
	}
	// chunks 保存由拆分场景基线取得的“chunks”，err 接收拆分场景基线过程中可能产生的错误，供后续失败分支处理。
	chunks, err := splitBaseline(base, 88, 3)
	if err != nil || len(chunks) < 2 {
		t.Fatal("未形成真实wire分块", err)
	}
	// chunk 接收遍历“chunks”时当前的元素。
	for _, chunk := range chunks {
		if !proto.Equal(chunk.OwnedPet, base.OwnedPet) || chunk.StateSequence != 9 || len(wire.Encode(wire.Message(p.MsgS2CSceneBaseline, 88, 3, chunk))) > wire.MaxFrame {
			t.Fatal("分块owner观察不一致或超帧")
		}
	}
}

// TestSummonPetPersistenceQueueFullRetainsSameReservationAndJob 核对「队列满时先丢预留或生效」与「原job重排改变预留身份」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetPersistenceQueueFullRetainsSameReservationAndJob(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, _, _, now := summonWorker(t, false)
	for len(w.e.jobs) < cap(w.e.jobs) {
		w.e.jobs <- job{}
	}
	// plan 保存由准备施法取得的计划，code 保存由准备施法取得的结果码。
	plan, code := w.prepareCast(r, 17, 0, 0, now, true)
	if code != 0 {
		t.Fatal(code)
	}
	// q 组装资产请求所需的数据。
	q := persistence.AssetRequest{Kind: "cast_material", Fingerprint: "full", Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID())}}
	w.submitMaterialCast(r, q, plan, 1, nil)
	// reservation 保存待确认宠物出生，用于当前步骤中的“reservation”，retry 保存待按原身份重试的操作，用于当前步骤中的重试。
	reservation, retry := r.PendingPetBirth, r.PendingRetry
	if !r.Pending || reservation == nil || retry == nil || reservation.Token.ID != domain.Hex(q.Context.OpId) || r.eventCount() != 0 {
		t.Fatal("队列满时先丢预留或生效")
	}
	<-w.e.jobs
	if !w.submit(*retry) || r.PendingPetBirth != reservation || r.PendingRetry != retry {
		t.Fatal("原job重排改变预留身份")
	}
}

// TestSummonPetOldMaterialCallbackCannotClearNextBirthReservation 核对「旧成功/unknown回调释放新预留/覆盖新job」与「首个付费body未落地」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetOldMaterialCallbackCannotClearNextBirthReservation(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，c 接收召唤房间执行器供后续步骤使用。
	// personal 保存由召唤房间执行器取得的个人，now 保存由召唤房间执行器取得的当前时间。
	w, r, c, personal, _, now := summonWorker(t, false)
	// plan 保存由准备施法取得的计划，code 保存由准备施法取得的结果码。
	plan, code := w.prepareCast(r, 17, 0, 0, now, true)
	if code != 0 {
		t.Fatal(code)
	}
	// q 组装资产请求所需的数据。
	q := persistence.AssetRequest{Kind: "cast_material", Fingerprint: "old", Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID())}}
	w.submitMaterialCast(r, q, plan, 1, nil)
	// binding 初始化用作会话绑定的材料提交会话绑定。
	binding := materialCommitBinding{roleID: r.Role.ID, room: r, auth: r.Auth, scene: r.Scene, life: r.Player.Life, generation: r.MaterialGeneration, conn: c}
	// role 保存当前处理所用的角色。
	role := r.Role
	role.Revision++
	role.EconomicSeq++
	// receipt 初始化用作回执的操作。
	receipt := persistence.Operation{Kind: "cast_material", CommandSeq: role.EconomicSeq, Casting: &persistence.CastingReceipt{Scope: "manual", SkillID: 17}}
	w.finishMaterialCast(binding, q, plan, 1, nil, role, &pb.OperationResult{}, receipt, nil, &job{})
	w.advanceEvents(r, now.Add(time.Second))
	if r.Pet == nil {
		t.Fatal("首个付费body未落地")
	}
	w.applyDamageInWorld(r, personal, personal.Monsters[0], r.Pet, r.Pet.HP, now.Add(time.Second))
	// next 保存由准备施法取得的下一状态。
	next, code := w.prepareCast(r, 17, 0, 0, now.Add(3*time.Second), true)
	if code != 0 {
		t.Fatal(code)
	}
	// q2 建立带初始值的资产请求，供本段继续处理。
	q2 := persistence.AssetRequest{Kind: "cast_material", Fingerprint: "next", Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID())}}
	w.submitMaterialCast(r, q2, next, 2, nil)
	// reservation 保存待确认宠物出生，用于当前步骤中的“reservation”，generation 保存材料代次，用于当前步骤中的代次。
	reservation, generation := r.PendingPetBirth, r.MaterialGeneration
	w.finishMaterialCast(binding, q, plan, 1, nil, role, &pb.OperationResult{}, receipt, nil, &job{})
	w.finishMaterialCast(binding, q, plan, 1, nil, domain.Role{}, &pb.OperationResult{}, persistence.Operation{}, persistence.Fail(p.CodeOperationPending, "late unknown"), &job{})
	if !r.Pending || r.PendingPetBirth != reservation || r.MaterialGeneration != generation || r.PendingOperationID != domain.Hex(q2.Context.OpId) || r.PendingRetry != nil {
		t.Fatal("旧成功/unknown回调释放新预留/覆盖新job")
	}
}

// TestSummonPetCorpseDespawnClearsRecallInDifferentOwnerWorld 核对「尸体Despawn未清原birth异图Recall」与「Recall未在共享队列中受理」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetCorpseDespawnClearsRecallInDifferentOwnerWorld(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，personal 保存由召唤房间执行器取得的个人。
	// town 保存由召唤房间执行器取得的城镇，now 保存由召唤房间执行器取得的当前时间。
	w, r, _, personal, town, now := summonWorker(t, true)
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 17, 0, 1, now); code != 0 {
		t.Fatal(code)
	}
	if len(town.Casts) != 1 {
		t.Fatal("Recall未在共享队列中受理")
	}
	w.applyDamageInWorld(r, personal, personal.Monsters[0], pet, pet.HP, now.Add(time.Millisecond))
	// deadline 保存过期集合发生时间，用于当前步骤中的截止时间。
	deadline := personal.PetCorpses[0].ExpiresAt
	// Pending解除后的过期清理先于本轮到期批；原宠物的Recall不能残留在另一World。
	if !w.expirePetCorpses(r, deadline) || r.eventCount() != 0 || len(personal.PetCorpses) != 0 {
		t.Fatal("尸体Despawn未清原birth异图Recall")
	}
}

// TestSummonPetNoPetsSameWorldTransferDoesNotApplyMapEntryFreeze 核对「实际跨图未执行NoPets入图冻结/改写原宠居民」与「同图NoPets迁移被错误拒绝」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetNoPetsSameWorldTransferDoesNotApplyMapEntryFreeze(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，town 保存由召唤房间执行器取得的城镇。
	// now 保存由召唤房间执行器取得的当前时间。
	w, r, _, _, town, now := summonWorker(t, false)
	// yes 保存当前处理所用的“yes”。
	yes := true
	w.e.Store.Cfg.Fixture.Maps[0].NoPets = &yes
	// plan 保存由已支付召唤计划取得的计划。
	plan := paidSummonPlan(t, w, r, now)
	w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{plan.Summon.Front}
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	if pet == nil || r.petResidentWorld() != town || pet.PetRuntime.FollowSuppressed {
		t.Fatal("合法fallback未在NoPets当前图生成原始未Frozen宠")
	}
	// position 保存当前处理所用的位置。
	position := pet.Pos
	if !w.activateWorld(r, town, devconfig.Point{X: 634, Y: 613}, nil) {
		t.Fatal("同图NoPets迁移被错误拒绝")
	}
	w.tickResidentPetFollow(r, now.Add(500*time.Millisecond))
	if pet.PetRuntime.FollowSuppressed || r.Pet != pet || pet.Pos != position || r.petResidentWorld() != town {
		t.Fatal("同图Teleport错误重复入图NoPets冻结/移动宠物")
	}
	// 两张图政策始终不变；先正常回允许图，再真正跨入NoPets图才执行入图冻结。
	// personalPetPoint 建立带初始值的点位，供本段继续处理。
	personalPetPoint := devconfig.Point{X: 7, Y: 8}
	if !w.activateWorld(r, r.PersonalWorld, devconfig.Point{X: 8, Y: 8}, &personalPetPoint) || pet.PetRuntime.FollowSuppressed || r.petResidentWorld() != r.PersonalWorld {
		t.Fatal("回允许图未保持同一原宠/恢复")
	}
	if !w.activateWorld(r, town, devconfig.Point{X: 633, Y: 613}, nil) || !pet.PetRuntime.FollowSuppressed || r.petResidentWorld() != r.PersonalWorld || pet.Pos != personalPetPoint {
		t.Fatal("实际跨图未执行NoPets入图冻结/改写原宠居民")
	}
}

// TestSummonPetFrozenRecallSkipsOnlyPetAIInCurrentTick 核对「Frozen异图宠未恢复/回主人当前图」与「Frozen自动Recall后同tick多做宠AI，或错误停了整个owner/怪物」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetFrozenRecallSkipsOnlyPetAIInCurrentTick(t *testing.T) {
	// w 接收召唤房间执行器供后续步骤使用，r 接收召唤房间执行器供后续步骤使用，town 保存由召唤房间执行器取得的城镇。
	// now 保存由召唤房间执行器取得的当前时间。
	w, r, _, _, town, now := summonWorker(t, true)
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	// yes 保存当前处理所用的“yes”。
	yes := true
	w.e.Store.Cfg.Fixture.Maps[0].NoPets = &yes
	if !w.relocatePet(r, r.petReference(), town, devconfig.Point{X: 633, Y: 613}) {
		t.Fatal("原Frozen居民前态安装失败")
	}
	pet.PetRuntime.FollowSuppressed = true
	r.Auto = true
	r.Player.Direction = 0
	r.Player.NextAttack = now.Add(time.Hour)
	r.MonsterCount = 1
	// monster 保存从怪物集合取出的怪物。
	monster := r.Monsters[0]
	monster.Pos = devconfig.Point{X: 9, Y: 8}
	monster.Monster = &devconfig.Monster{Fighter: devconfig.Fighter{AttackMin: 1, AttackMax: 1, AttackMS: 1000, MoveMS: 1000, Accuracy: 255}}
	// petBefore 保存攻击序号，用于当前步骤中的宠物操作前的状态，monsterBefore 保存攻击序号，用于当前步骤中的怪物操作前的状态。
	petBefore, monsterBefore := pet.AttackSequence, monster.AttackSequence
	w.roll = func(int64) int64 { return 0 }
	w.step(r, now)
	if r.petResidentWorld() != r.PersonalWorld || pet.PetRuntime.FollowSuppressed {
		t.Fatal("Frozen异图宠未恢复/回主人当前图")
	}
	if pet.AttackSequence != petBefore || monster.AttackSequence != monsterBefore+1 {
		t.Fatal("Frozen自动Recall后同tick多做宠AI，或错误停了整个owner/怪物", pet.AttackSequence, monster.AttackSequence)
	}
}

// TestSummonPetInitializeNoPetsFreezesLegacyOnce 核验profile不应被改成无宠，用诊断「初始图NoPets未单次应用入图冻结/owner观察」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetInitializeNoPetsFreezesLegacyOnce(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// yes 保存当前处理所用的“yes”。
	yes := true
	cfg.Fixture.Map.NoPets = &yes
	// err 接收校验过程中可能产生的错误，供后续失败分支处理。
	if err := cfg.Fixture.Validate(); err != nil {
		t.Fatal("合法NoPets初图配置被拒绝", err)
	}
	// id 区分当前步骤指向的目标对象。
	id := domain.ID()
	// c 保存当前处理所用的已捕获连接。
	c := &capturedConnection{}
	// r 保存当前处理所用的房间。
	r := &room{Scene: 20, Baseline: 30, Conn: c, Ready: true}
	// w 保存当前处理所用的房间执行器。
	w := &worker{e: &Engine{Store: &persistence.Store{Cfg: cfg}}, rooms: map[string]*room{id: r}}
	w.initialize(r, domain.Role{ID: id, Job: 2, Level: 19, Revision: 1})
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	if pet == nil {
		t.Fatal("legacy profile不应被改成无宠")
	}
	// owned 保存由“owned”宠物“pb”取得的“owned”。
	owned := w.ownedPetPB(r)
	if !pet.PetRuntime.FollowSuppressed || owned == nil || !owned.FollowSuppressed {
		t.Error("初始图NoPets未单次应用入图冻结/owner观察")
	}
	// now 记录本次处理开始时的时间。
	now := time.Now()
	r.Player.Pos = devconfig.Point{X: 12, Y: 8}
	r.Auto = false
	// monster 接收遍历怪物集合时当前的元素。
	for _, monster := range r.Monsters {
		monster.NextMove = now.Add(time.Hour)
		monster.NextAttack = now.Add(time.Hour)
	}
	// position 保存当前处理所用的位置，attack 保存攻击序号，用于当前步骤中的攻击。
	position, attack := pet.Pos, pet.AttackSequence
	w.step(r, now)
	if pet.Pos != position || pet.AttackSequence != attack {
		t.Fatal("初图冻结宠物仍执行走攻")
	}
}
