package game

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"os"
	"reflect"
	"testing"
	"time"
)

// 独立测试夹具明确采用来源子集；不向运行配置提供第二套维护数值。
func statusWorker(t *testing.T, job uint32) (*worker, *room, *capturedConnection) {
	w, r, c := skillWorker(t, job)
	f := &w.e.Store.Cfg.Fixture
	f.Buffs = []devconfig.BuffDefinition{
		{ID: 31, Kind: "magic_shield", ValueByLevel: []int64{20, 30, 40, 50}, DurationKind: "mc_source_power", StatAdd: 15, DurationSecondsByLevel: []int64{0, 0, 0, 0}, TickMS: 1000, HitDurationLossMSPerHP: 60, StackPolicy: "ignore_present_on_impact", RemoveOnPlayerDeath: true},
		{ID: 14, Kind: "max_magic_defence", ValueByLevel: []int64{0, 0, 0, 0}, TargetLevelDivisor: 7, TargetLevelAdd: 4, DurationKind: "sc_rank_linear", StatMultiplier: 4, DurationSecondsByLevel: []int64{50, 100, 150, 200}, TickMS: 1000, StackPolicy: "refresh_duration_keep_values", AllowDeadTargets: true},
		{ID: 15, Kind: "max_physical_defence", ValueByLevel: []int64{0, 0, 0, 0}, TargetLevelDivisor: 7, TargetLevelAdd: 4, DurationKind: "sc_rank_linear", StatMultiplier: 4, DurationSecondsByLevel: []int64{50, 100, 150, 200}, TickMS: 1000, StackPolicy: "refresh_duration_keep_values", AllowDeadTargets: true},
	}
	f.CastingSupply = devconfig.CastingSupplyRules{Enabled: true, Slot: 100, ItemIDs: []uint32{187}}
	f.Items = append(f.Items, devconfig.Item{ID: 187, EquipmentSlot: 100, Stackable: true, MaxStack: 10, AttackMinBonus: 999, AttackMaxBonus: 999})
	f.Pet.Level = 15
	for _, s := range []devconfig.Skill{
		{ID: 31, JobID: 1, Behavior: "magic_shield", BuffID: 31, TargetKind: "friendly_self", RangeCells: 9, MPBaseCost: 7},
		{ID: 14, JobID: 2, Behavior: "soul_shield", BuffID: 14, TargetKind: "friendly_grid", AreaRadius: 3, RangeCells: 9, MPBaseCost: 8, MaterialItemID: 187, MaterialQuantity: 1},
		{ID: 15, JobID: 2, Behavior: "blessed_armour", BuffID: 15, TargetKind: "friendly_grid", AreaRadius: 3, RangeCells: 9, MPBaseCost: 9, MaterialItemID: 187, MaterialQuantity: 1},
	} {
		s.MaxLevel = 3
		s.ActivationKind = "cast"
		s.DelayBaseMS = 500
		s.ProjectileMSPerCell = 50
		s.ActionMS = 600
		s.CooldownBaseMS = 1800
		s.GlobalSpellMS = 1800
		s.AutoPriority = 80
		s.AutoEnabled = true
		s.AutoMinTargets = 1
		found := false
		for i := range f.Skills {
			if f.Skills[i].ID == s.ID {
				f.Skills[i] = s
				found = true
				break
			}
		}
		if !found {
			f.Skills = append(f.Skills, s)
		}
		if s.JobID == job {
			if _, known := r.Role.Skill(s.ID); !known {
				r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: s.ID})
			}
		}
	}
	return w, r, c
}

func TestStatusFormulaMatchesIndependentCSharpOracle(t *testing.T) {
	file, err := os.Open("testdata/source-status-vectors.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	zipped, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer zipped.Close()
	var vectors []struct {
		Kind                            string
		Rank                            uint32
		Stat, Power                     int64
		Level                           uint32
		Expected                        int64
		Raw, Armour, Percent, Remaining int64
		ExpectedNet                     int64 `json:"expected_net"`
		ExpectedRemaining               int64 `json:"expected_remaining"`
		Shield                          int64 `json:"expected_shield_seconds"`
		Tao                             int64 `json:"expected_tao_seconds"`
	}
	if err = json.NewDecoder(zipped).Decode(&vectors); err != nil {
		t.Fatal(err)
	}
	w, _, _ := statusWorker(t, 1)
	shield, _ := w.e.Store.Cfg.Fixture.Buff(31)
	tao, _ := w.e.Store.Cfg.Fixture.Buff(14)
	skill, _ := w.e.Store.Cfg.Fixture.Skill(31)
	for index, v := range vectors {
		switch v.Kind {
		case "duration":
			if got := buffDurationSeconds(skill, shield, v.Rank, v.Stat, 0); got != v.Shield {
				t.Fatalf("%d 盾时长=%d/%d", index, got, v.Shield)
			}
			if got := buffDurationSeconds(skill, tao, v.Rank, v.Stat, 0); got != v.Tao {
				t.Fatalf("%d 道士时长=%d/%d", index, got, v.Tao)
			}
		case "target_bonus":
			if got := statusTargetBonus(v.Level, tao); got != v.Expected {
				t.Fatalf("%d 目标等级加值=%d/%d", index, got, v.Expected)
			}
		case "damage":
			e := &entity{Statuses: map[uint32]*combatStatus{31: {DamageReductionPercent: v.Percent, RemainingMS: v.Remaining, DamageWearMS: 60}}}
			got := statusDamage(e, v.Raw, v.Armour)
			wearStatuses(e, got)
			if got != v.ExpectedNet || e.Statuses[31].RemainingMS != v.ExpectedRemaining {
				t.Fatalf("%d 净伤/削时=%d/%d vs%d/%d", index, got, e.Statuses[31].RemainingMS, v.ExpectedNet, v.ExpectedRemaining)
			}
		default:
			t.Fatalf("未识别向量%s", v.Kind)
		}
	}
	if len(vectors) != 586 {
		t.Fatalf("未验证完整向量：%d", len(vectors))
	}
}

func TestStatusDiscreteTickRefreshAndDamageKeepClocksAndValues(t *testing.T) {
	w, r, _ := statusWorker(t, 2)
	now := time.Unix(1800000000, 0)
	skill, _ := w.e.Store.Cfg.Fixture.Skill(14)
	cast := pendingCast{Skill: skill, Caster: reference(r, &r.Player), BuffDurationMS: 50000}
	if !w.applyBuff(r, cast, r.Pet, now) || r.Pet.Statuses[14].MaxMagicDefenceBonus != 6 {
		t.Fatal("宠物固定加值未按定义Level15计算")
	}
	s := r.Pet.Statuses[14]
	if !s.NextProcess.IsZero() {
		t.Fatal("新状态Next必须为0")
	}
	w.tickStatuses(r, now.Add(100*time.Millisecond))
	last, next := s.LastProcess, s.NextProcess
	originalCaster := s.Caster
	r.Pet.HP = 0
	r.Role.Level = 100
	cast.BuffDurationMS = 10000
	cast.SkillLevel = 3
	cast.Caster.Life++
	w.applyBuff(r, cast, r.Pet, now.Add(200*time.Millisecond))
	if s.RemainingMS != 10000 || s.MaxMagicDefenceBonus != 6 || s.SkillLevel != 0 || s.Caster != originalCaster || s.LastProcess != last || s.NextProcess != next {
		t.Fatal("重施改写了初值/来源/时钟")
	}
	w.tickStatuses(r, next.Add(-time.Nanosecond))
	if s.RemainingMS != 10000 {
		t.Fatal("未到tick提前扣时")
	}
	w.tickStatuses(r, next)
	if s.RemainingMS != 9000 || !s.NextProcess.Equal(next.Add(time.Second)) {
		t.Fatal("equal边界或单次实际elapsed错误")
	}
	w.tickStatuses(r, next.Add(5*time.Second))
	if s.RemainingMS != 4000 || !s.NextProcess.Equal(next.Add(6*time.Second)) {
		t.Fatal("卡帧应扣实际时间且只安排下一跳")
	}
	// 展示归零与权威存在是两件事；受击削成负数也等下一次状态处理。
	shield, _ := w.e.Store.Cfg.Fixture.Skill(31)
	w.applyBuff(r, pendingCast{Skill: shield, Caster: reference(r, &r.Player), BuffDurationMS: 1000}, &r.Player, now)
	ss := r.Player.Statuses[31]
	ss.NextProcess = now.Add(10 * time.Second)
	wearStatuses(&r.Player, 20)
	if ss.RemainingMS != -200 || statusReduction(&r.Player) != 20 || statusesPB(&r.Player, now.Add(time.Second))[0].RemainingMs != 0 {
		t.Fatal("负剩余提前撤销/展示越界")
	}
	w.tickStatuses(r, now.Add(9*time.Second))
	if r.Player.Statuses[31] == nil {
		t.Fatal("未到离散移除点已失效")
	}
	w.tickStatuses(r, ss.NextProcess)
	if r.Player.Statuses[31] != nil {
		t.Fatal("到期未移除")
	}
}

func TestStatusMagicShieldDeathRecastAndLateSelfPosition(t *testing.T) {
	w, r, _ := statusWorker(t, 1)
	now := time.Unix(1800000000, 0)
	mp := r.Player.MP
	if code := w.cast(r, 31, 0, 1, now, &pb.GridPoint{X: 0, Y: 0}); code != 0 {
		t.Fatal(code)
	}
	cast := r.Casts[0]
	r.Player.Pos = devconfig.Point{X: 10, Y: 10}
	w.advanceEvents(r, cast.Due)
	if r.Player.Statuses[31] == nil || r.Player.MP != mp-7 {
		t.Fatal("自用盾错误依赖旧观察中心或未付MP")
	}
	first := r.Player.Statuses[31]
	remaining := first.RemainingMS
	later := now.Add(2 * time.Second)
	if code := w.cast(r, 31, 0, 2, later); code != 0 {
		t.Fatal(code)
	}
	w.advanceEvents(r, r.Casts[0].Due)
	if r.Player.Statuses[31] != first || first.RemainingMS != remaining || r.Player.MP != mp-14 {
		t.Fatal("重复盾应付资源但生效时忽略")
	}
	removeDeathStatuses(&r.Player)
	r.Player.HP = 0
	w.resolveCast(r, cast, cast.Due.Add(time.Second))
	if r.Player.Statuses[31] != nil {
		t.Fatal("本作31死亡迟到收紧失效")
	}
}

func TestStatusCorpseBuffReviveMigrationPetReplacementAndFaultReset(t *testing.T) {
	w, r, _ := statusWorker(t, 2)
	now := time.Unix(1800000000, 0)
	skill, _ := w.e.Store.Cfg.Fixture.Skill(15)
	r.Player.HP, r.Pet.HP = 0, 0
	cast := pendingCast{Scene: r.Scene, Skill: skill, Caster: reference(r, &r.Player), TargetPoint: r.Player.Pos, BuffDurationMS: 50000}
	w.resolveCast(r, cast, now)
	if r.Player.Statuses[15] == nil || r.Pet.Statuses[15] == nil {
		t.Fatal("14/15合法友方尸体被排除")
	}
	w.tickStatuses(r, now.Add(time.Second))
	if r.Player.Statuses[15].RemainingMS != 49000 {
		t.Fatal("尸体状态未计时")
	}
	w.revivePlayer(r)
	if r.Player.Statuses[15].TargetLife != r.Player.Life || statusFighter(&r.Player, devconfig.Fighter{}).DefenceMax == 0 {
		t.Fatal("玩家复活没有受控迁移保留状态")
	}
	old := r.Pet
	r.Pet = &entity{Handle: 2, Kind: 2, Life: old.Life + 1}
	if len(r.Pet.Statuses) != 0 {
		t.Fatal("新召唤继承旧对象状态")
	}
	fresh := &room{worldState: &worldState{}}
	w.initialize(fresh, r.Role)
	if len(fresh.Player.Statuses) != 0 || fresh.Pet != nil && len(fresh.Pet.Statuses) != 0 {
		t.Fatal("故障新现场恢复了临时状态")
	}
}

func TestStatusPendingFreezeBeforeEqualImpactAndFullBaseline(t *testing.T) {
	w, r, c := statusWorker(t, 1)
	now := time.Now()
	r.Auto = false
	r.MonsterCount = 0
	skill, _ := w.e.Store.Cfg.Fixture.Skill(31)
	w.applyBuff(r, pendingCast{Skill: skill, BuffDurationMS: 1000}, &r.Player, now)
	state := r.Player.Statuses[31]
	state.NextProcess = now.Add(time.Second)
	r.Pending = true
	w.step(r, now.Add(2*time.Second))
	if r.Player.Statuses[31] == nil {
		t.Fatal("未知永久事务没有冻结状态")
	}
	r.Pending = false
	r.LastBroadcast = now.Add(2 * time.Second)
	w.step(r, now.Add(2*time.Second))
	if r.Player.Statuses[31] != nil {
		t.Fatal("解除未决后未按实际时钟移除")
	}
	// 本作统一顺序：离散到期与命中相等时，先移除盾，再以当前属性结算伤害。
	w.applyBuff(r, pendingCast{Skill: skill, BuffDurationMS: 1000}, &r.Player, now)
	r.Player.Statuses[31].NextProcess = now.Add(time.Second)
	r.Player.HP = 100
	r.Attacks = []pendingAttack{{Caster: reference(r, &r.Player), Target: reference(r, &r.Player), Due: now.Add(time.Second), Amount: 10, Defence: defenceAgility}}
	w.step(r, now.Add(time.Second))
	if r.Player.HP != 90 || r.Player.Statuses[31] != nil {
		t.Fatal("同tick命中使用了已在本帧离散移除的盾")
	}
	w.applyBuff(r, pendingCast{Skill: skill, BuffDurationMS: 1000}, &r.Player, now)
	w.baseline(r)
	scene := &pb.SceneBaselineChunk{}
	if err := proto.Unmarshal(c.frames[len(c.frames)-1].Payload, scene); err != nil {
		t.Fatal(err)
	}
	if len(scene.Entities[0].Statuses) != 1 || scene.Entities[0].CombatAttributes.DamageReductionPercent != 20 {
		t.Fatal("基线缺少完整状态/实际属性")
	}
}

func TestStatusPositiveNetWearBeforeLimitedHPDeathAndMissNoWear(t *testing.T) {
	w, r, _ := statusWorker(t, 1)
	now := time.Now()
	shield, _ := w.e.Store.Cfg.Fixture.Skill(31)
	w.applyBuff(r, pendingCast{Skill: shield, BuffDurationMS: 5000}, &r.Player, now)
	s := r.Player.Statuses[31]
	r.Player.HP = 3
	w.applyDamage(r, r.Monsters[0], &r.Player, 20, now)
	if s.RemainingMS != 3800 || r.Player.HP != 0 || r.Player.Statuses[31] != nil {
		t.Fatal("致死削时错误按有限HP或未移除盾")
	}
	w, r, _ = statusWorker(t, 1)
	w.applyBuff(r, pendingCast{Skill: shield, BuffDurationMS: 5000}, &r.Player, now)
	s = r.Player.Statuses[31]
	w.applyDamage(r, r.Monsters[0], &r.Player, 0, now)
	if s.RemainingMS != 5000 {
		t.Fatal("闪避/零净伤错误削时")
	}
}

func TestMaterialManualAuthorityFailureIsolatesButBusinessConflictDoesNot(t *testing.T) {
	for _, code := range []uint32{p.CodeRouteChanged, p.CodeUnauthenticated, p.CodeStaleSession, p.CodeOperationConflict} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			w, r, c := statusWorker(t, 2)
			r.Role.ID = "role"
			r.Pending = true
			r.MaterialGeneration = 1
			q := persistence.AssetRequest{Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID())}, Fingerprint: "original"}
			r.PendingOperationID = domain.Hex(q.Context.OpId)
			r.PendingFingerprint = q.Fingerprint
			binding := materialCommitBinding{roleID: "role", room: r, auth: r.Auth, scene: r.Scene, life: r.Player.Life, generation: 1, conn: c}
			frame := wire.Frame{Request: 77}
			w.finishMaterialCast(binding, q, nil, 0, &frame, domain.Role{}, &pb.OperationResult{}, persistence.Operation{}, persistence.Fail(code, "terminal"), &job{})
			if r.Isolated != hardAuthorityFailure(code) {
				t.Fatal("权限失效继续战斗或普通冲突被错误隔离")
			}
		})
	}
}

func TestMaterialReceiptOneEffectOriginalWatermarkAndSceneLife(t *testing.T) {
	w, r, _ := statusWorker(t, 2)
	now := time.Now()
	skill, _ := w.e.Store.Cfg.Fixture.Skill(14)
	plan := &preparedCast{Scene: r.Scene, Skill: skill, Caster: reference(r, &r.Player), Point: r.Player.Pos, Origin: r.Player.Pos, BuffDurationMS: 50000, DelayMS: 500}
	receipt := persistence.Operation{Kind: "cast_material", CommandSeq: 4, Casting: &persistence.CastingReceipt{Scope: "manual", SkillID: 14}}
	if !w.applyMaterialReceipt(r, receipt, plan, 123, now) || w.applyMaterialReceipt(r, receipt, plan, 123, now) || len(r.Casts) != 1 {
		t.Fatal("原消耗回执重复生成施法")
	}
	receipt.SourceSeq = 8
	receipt.Casting.Scope = "auto"
	if !w.applyMaterialReceipt(r, receipt, plan, 0, now) || r.MaterialSourceSeq != 8 || r.MaterialEconomicSeq != 4 {
		t.Fatal("自动/手动水位混用")
	}
	r.Scene++
	receipt.SourceSeq++
	if w.applyMaterialReceipt(r, receipt, plan, 0, now) {
		t.Fatal("旧场景原回执补效果")
	}
	r.Scene--
	r.Player.Life++
	if w.applyMaterialReceipt(r, receipt, plan, 0, now) {
		t.Fatal("旧生命原回执补效果")
	}
}

func TestMaterialCallbackGenerationRoomLifeAuthAndHealthyDisconnect(t *testing.T) {
	for _, mutation := range []string{"healthy_disconnect", "new_pending", "new_generation", "new_room", "new_life", "new_scene", "new_auth", "new_connection"} {
		t.Run(mutation, func(t *testing.T) {
			w, r, c := statusWorker(t, 2)
			r.Role.ID = "role"
			r.Role.Revision = 1
			w.e.ctx = context.Background()
			w.e.jobs = make(chan job, 1)
			q := persistence.AssetRequest{Kind: "cast_material", Fingerprint: "original", Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID())}}
			skill, _ := w.e.Store.Cfg.Fixture.Skill(14)
			plan := &preparedCast{Scene: r.Scene, Skill: skill, Caster: reference(r, &r.Player), Point: r.Player.Pos, BuffDurationMS: 50000}
			frame := wire.Frame{Request: 77}
			w.submitMaterialCast(r, q, plan, 1, &frame)
			binding := materialCommitBinding{roleID: "role", room: r, auth: r.Auth, scene: r.Scene, life: r.Player.Life, generation: r.MaterialGeneration, conn: c}
			switch mutation {
			case "healthy_disconnect":
				r.Conn = nil
				r.Ready = false
			case "new_pending":
				r.PendingOperationID = domain.ID()
				r.PendingFingerprint = "new"
			case "new_generation":
				r.MaterialGeneration++
			case "new_room":
				replacement := *r
				replacement.Player.Statuses = nil
				w.rooms["role"] = &replacement
			case "new_life":
				r.Player.Life++
			case "new_scene":
				r.Scene++
			case "new_auth":
				r.Auth.SessionEpoch++
			case "new_connection":
				r.Conn = &capturedConnection{}
			}
			role := r.Role
			role.Revision = 2
			role.EconomicSeq = 1
			result := &pb.OperationResult{NewRevision: 2, CommandSeq: 1}
			receipt := persistence.Operation{Kind: "cast_material", CommandSeq: 1, Casting: &persistence.CastingReceipt{Scope: "manual", SkillID: 14}}
			c.frames = nil
			w.finishMaterialCast(binding, q, plan, 1, &frame, role, result, receipt, nil, &job{})
			current := w.rooms["role"]
			if current.Role.Revision != 2 {
				t.Fatal("永久事实未对账")
			}
			shouldCast := mutation == "healthy_disconnect" || mutation == "new_connection"
			if (len(r.Casts) == 1) != shouldCast {
				t.Fatalf("临时施法隔离错误:%d", len(r.Casts))
			}
			if mutation == "new_pending" || mutation == "new_generation" || mutation == "new_room" {
				if !r.Pending {
					t.Fatal("旧callback释放新代槽")
				}
			}
			for _, response := range c.frames {
				if response.ID == p.MsgS2COperationResult && (mutation == "new_auth" || mutation == "new_connection" || mutation == "healthy_disconnect") {
					t.Fatal("旧request发至新绑定或离线连接")
				}
			}
			before := r.RetryAt
			w.finishMaterialCast(binding, q, plan, 1, &frame, domain.Role{}, result, persistence.Operation{}, persistence.Fail(p.CodeOperationPending, "unknown"), &job{})
			if r.RetryAt != before {
				t.Fatal("旧回调抢占新retry槽")
			}
		})
	}
}

func TestMaterialHistoricalRequestNeverPollutesNewBindingInput(t *testing.T) {
	w, r, _ := statusWorker(t, 2)
	w.e.jobs = make(chan job, 1)
	r.Role.EconomicSeq = 12
	r.InputSequence = 0
	request := &pb.CastWithMaterialRequest{SceneEpoch: r.Scene - 1, CasterLife: r.Player.Life, InputSequence: 9000, SkillId: 14, MaterialInstanceId: domain.Bytes(domain.ID()), MaterialVersion: 1, Operation: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: 10}}
	w.materialCast(r, wire.Message(p.MsgC2SCastWithMaterial, 1, 3, request))
	if r.InputSequence != 0 || !r.Pending || len(w.e.jobs) != 1 {
		t.Fatal("历史回执未进Store核查或污染新输入窗口")
	}
}

func TestMaterialUnknownKeepsOriginalJobAndRandomPlanUntilKnownOnce(t *testing.T) {
	w, r, c := statusWorker(t, 2)
	r.Role.ID = "role"
	r.Role.Revision = 1
	r.MaterialGeneration = 1
	r.Pending = true
	q := persistence.AssetRequest{Kind: "cast_material", Fingerprint: "frozen", Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID())}}
	r.PendingOperationID = domain.Hex(q.Context.OpId)
	r.PendingFingerprint = q.Fingerprint
	skill, _ := w.e.Store.Cfg.Fixture.Skill(14)
	plan := &preparedCast{Scene: r.Scene, Skill: skill, Caster: reference(r, &r.Player), Point: r.Player.Pos, BuffDurationMS: 51000}
	binding := materialCommitBinding{roleID: "role", room: r, auth: r.Auth, scene: r.Scene, life: r.Player.Life, generation: 1, conn: c}
	retry := &job{}
	frame := wire.Frame{Request: 7}
	for i := 0; i < 2; i++ {
		w.finishMaterialCast(binding, q, plan, 1, &frame, domain.Role{}, &pb.OperationResult{}, persistence.Operation{}, persistence.Fail(p.CodeOperationPending, "unknown"), retry)
	}
	if !r.Pending || r.PendingRetry != retry || len(r.Casts) != 0 || plan.BuffDurationMS != 51000 {
		t.Fatal("未知结果更换原job/重抽计划/提前施法")
	}
	role := r.Role
	role.Revision = 2
	receipt := persistence.Operation{Kind: "cast_material", CommandSeq: 1, Casting: &persistence.CastingReceipt{Scope: "manual", SkillID: 14}}
	w.finishMaterialCast(binding, q, plan, 1, &frame, role, &pb.OperationResult{}, receipt, nil, retry)
	w.finishMaterialCast(binding, q, plan, 1, &frame, role, &pb.OperationResult{}, receipt, nil, retry)
	if r.Pending || r.PendingRetry != nil || len(r.Casts) != 1 || r.Casts[0].BuffDurationMS != 51000 {
		t.Fatal("已知原回执没有保留首次随机持续时长或重复应用")
	}
}

func TestStatusAreaFriendlyValidCellsAndCapacityRejectBeforeResources(t *testing.T) {
	w, r, _ := statusWorker(t, 2)
	now := time.Now()
	s, _ := w.e.Store.Cfg.Fixture.Skill(14)
	r.Player.HP = 0
	r.Pet.HP = 0
	r.Monsters[0].Pos = r.Player.Pos
	cast := pendingCast{Skill: s, TargetPoint: r.Player.Pos}
	targets := w.buffTargets(r, cast)
	if len(targets) != 2 {
		t.Fatal("范围未按友方尸体筛选")
	}
	w.e.Store.Cfg.Fixture.Map.Blocked = append(w.e.Store.Cfg.Fixture.Map.Blocked, r.Pet.Pos)
	if got := w.buffTargets(r, cast); len(got) != 1 || got[0] != &r.Player {
		t.Fatal("范围错误包含阻挡格目标/敌方")
	}
	r.Player.HP = r.Player.MaxHP
	beforeMP := r.Player.MP
	if _, code := w.prepareCast(r, 14, 0, 8, now, true, pointPB(r.Player.Pos)); code != p.CodeSkillTarget || r.Player.MP != beforeMP || !r.NextSpell.IsZero() {
		t.Fatal("非法八向材料施法没有在消费/随机抽样前拒绝")
	}
	r.Attacks = make([]pendingAttack, w.eventCapacity())
	if _, code := w.prepareCast(r, 14, 0, r.Player.Direction, now, true, pointPB(r.Player.Pos)); code != p.CodeSkillStateBlocked || r.Player.MP != beforeMP || !r.NextSpell.IsZero() {
		t.Fatal("事件预算已满仍扣资源/准备新状态")
	}
}

func TestStatusMissingSupplyAutoFallsThroughAndSlotNeverAddsGearStats(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		w, r, _ := statusWorker(t, 2)
		w.e.Store.Cfg.Fixture.CastingSupply.Enabled = enabled
		r.Player.HP--
		r.Pet.HP = r.Pet.MaxHP
		_, handled := w.autoSkill(r, time.Now())
		if !handled || len(r.Casts) != 1 || r.Casts[0].Skill.Behavior != "healing" || r.Pending {
			t.Fatal("缺护符/未开放材料饿死后续治愈")
		}
	}
	w, r, _ := statusWorker(t, 2)
	before := w.fighter(r, &r.Player)
	r.Role.Equipped = append(r.Role.Equipped, domain.Equipped{Slot: 100, Item: domain.Item{ItemID: 187}})
	if !reflect.DeepEqual(before, w.fighter(r, &r.Player)) {
		t.Fatal("耗材槽误进入装备属性聚合")
	}
}
