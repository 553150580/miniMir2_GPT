// 本文件回归宠物尸体容量、保留句柄、原居民身份及非当前世界的过期。
package game

import (
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"reflect"
	"testing"
	"time"
)

// spawnRuntimePet 生成运行时宠物。
// t 提供断言失败报告、子用例和清理注册，w 记录房间执行器，r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，rank 记录排序位次，使用数值表示。
// now 提供当前时点，作为状态有效期和计时判断的基准。
func spawnRuntimePet(t *testing.T, w *worker, r *room, world *worldState, rank uint32, now time.Time) *entity {
	t.Helper()
	// pet 保存由创建宠物取得的宠物，ok 表明查询、校验或类型转换是否成功。
	pet, ok := w.newPet(r, 1, rank, r.Player.Pos, now)
	if !ok || !w.installPet(r, world, pet) {
		t.Fatal("显式测试定义不能安装正规训练宠")
	}
	return pet
}

// TestPetRuntimeRetains128BodiesWithOneLivingPetAndNoMasterCorpses 核对「尸体与新生重复baseline handle」与「128身体名额未包含活动宠与全部原尸体」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeRetains128BodiesWithOneLivingPetAndNoMasterCorpses(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，c 接收场景世界房间执行器供后续步骤使用。
	// personal 保存由场景世界房间执行器取得的个人，now 保存由场景世界房间执行器取得的当前时间。
	w, r, c, personal, _, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	w.e.Store.Cfg.Fixture.PetRuntime.Budgets.RetainedBodyCapacityPerOwner = 128
	// old 保存宠物，用于当前步骤中的原状态。
	old := r.Pet
	// oldRef 保存由引用取得的原状态引用。
	oldRef := reference(r, old)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], old, old.HP, now)
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 126; i++ {
		// pet 保存由生成或出生点运行时宠物取得的宠物。
		pet := spawnRuntimePet(t, w, r, personal, 0, now)
		w.applyDamageInWorld(r, personal, personal.Monsters[0], pet, pet.HP, now)
	}
	// pet 保存由生成或出生点运行时宠物取得的宠物。
	pet := spawnRuntimePet(t, w, r, personal, 0, now)
	if len(personal.PetCorpses) != 127 || r.retainedPetBodies() != 128 || r.Pet != pet {
		t.Fatal("128身体名额未包含活动宠与全部原尸体")
	}
	// spell 保存由技能取得的法术。
	spell, _ := w.e.Store.Cfg.Fixture.Skill(14)
	// target 接收遍历状态效果目标集合时当前的元素。
	for _, target := range w.buffTargets(r, pendingCast{Skill: spell, Caster: reference(r, &r.Player), TargetPoint: old.Pos}) {
		if target.IsPetCorpse {
			t.Fatal("允许死目标的14把无主尸体当友方")
		}
	}
	// playerBefore 保存生命值，用于当前步骤中的玩家操作前的状态，petBefore 保存生命值，用于当前步骤中的宠物操作前的状态。
	playerBefore, petBefore := r.Player.HP, pet.HP
	w.resolveAttackInWorld(r, personal, pendingAttack{Caster: referenceIn(personal, personal.Monsters[0]), Target: oldRef, Amount: 99999, Due: now, Defence: defenceAC}, now)
	if old.HP != 0 || r.Player.HP != playerBefore || pet.HP != petBefore || w.resolveInWorld(r, personal, oldRef) != old {
		t.Fatal("旧目标引用命中新宠/玩家或丢失旧尸体")
	}
	c.frames = nil
	w.baseline(r)
	// seen 建立带初始值的按数值索引的布尔状态映射，供本段继续处理，corpses 规定尸体集合的初始取值，live 规定存活的初始取值，maxBytes 规定允许的最大字节数的初始取值。
	seen, corpses, live, maxBytes := map[uint32]bool{}, 0, 0, 0
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range c.frames {
		if frame.ID != p.MsgS2CSceneBaseline {
			continue
		}
		// n 记录编码中的条目数。
		if n := len(wire.Encode(frame)); n > maxBytes {
			maxBytes = n
		}
		// chunk 记录分片，使用场景场景基线分片表示。
		var chunk pb.SceneBaselineChunk
		// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
		if err := proto.Unmarshal(frame.Payload, &chunk); err != nil {
			t.Fatal(err)
		}
		// e 接收遍历实体时当前的元素。
		for _, e := range chunk.Entities {
			if seen[e.EntityHandle] {
				t.Fatal("尸体与新生重复baseline handle")
			}
			seen[e.EntityHandle] = true
			if e.IsPetCorpse {
				corpses++
				if e.Hp != 0 || e.EntityKind != 2 {
					t.Fatal("尸体身份或HP投影错误")
				}
			} else if e.EntityKind == 2 {
				live++
			}
		}
	}
	if corpses != 127 || live != 1 || maxBytes > wire.MaxFrame {
		t.Fatal("未完整显示有界原尸体与单活动宠", corpses, live, maxBytes)
	}
	t.Logf("隔离定义128身体实际baseline最大帧%d字节；不是容量/真机结论", maxBytes)
}

// TestPetRuntimeCorpseReservationCapacityAndOriginalIdentity 核验新生预约应一次占据未来尸体名额，以及原身体应转交原World。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeCorpseReservationCapacityAndOriginalIdentity(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，personal 保存由场景世界房间执行器取得的个人。
	// now 保存由场景世界房间执行器取得的当前时间。
	w, r, _, personal, _, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	w.e.Store.Cfg.Fixture.PetRuntime.Budgets.RetainedBodyCapacityPerOwner = 2
	// old 保存宠物，用于当前步骤中的原状态。
	old := r.Pet
	// oldRef 保存由引用取得的原状态引用。
	oldRef := reference(r, old)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], old, old.HP, now)
	if len(personal.PetCorpses) != 1 || personal.PetCorpses[0].Body != old || !old.IsPetCorpse {
		t.Fatal("原身体没有转交原World")
	}
	// token 初始化用作令牌的宠物出生令牌。
	token := petBirthToken{ID: "first", ExecutionEpoch: r.ExecutionEpoch, PlayerLife: r.Player.Life}
	if !w.reservePetBirth(r, token, now) || !w.reservePetBirth(r, token, now) || r.retainedPetBodies() != 2 {
		t.Fatal("新生预约没有一次占据未来尸体名额")
	}
	// other 保存当前处理所用的另一项。
	other := token
	other.ID = "late"
	if w.reservePetBirth(r, other, now) || w.cancelPetBirth(r, other) {
		t.Fatal("不同计划抢占/释放在途预约")
	}
	// pet 保存由创建宠物取得的宠物，ok 表明查询、校验或类型转换是否成功。
	pet, ok := w.newPet(r, 1, 0, r.Player.Pos, now)
	if !ok || w.installPet(r, personal, pet) || !w.installReservedPet(r, personal, pet, token) || pet.Handle == old.Handle || pet.Life <= old.Life || r.PendingPetBirth != nil {
		t.Fatal("预约未匹配就出生，或新宠复用原尸体身份")
	}
	if w.resolveInWorld(r, personal, oldRef) != old || r.Pet != pet || old.HP != 0 || old.Handle != oldRef.Handle {
		t.Fatal("新生复制/覆盖原尸体")
	}
	w.applyDamageInWorld(r, personal, personal.Monsters[0], pet, pet.HP, now)
	if r.Pet != nil || len(personal.PetCorpses) != 2 || w.canBeginPetBirth(r, now) || r.Isolated {
		t.Fatal("满额删除尸体、拒绝合法死亡或允许额外出生")
	}
	r.Pending = true
	if w.expirePetCorpses(r, now.Add(2*time.Second)) || len(personal.PetCorpses) != 2 {
		t.Fatal("Pending期间越过原冻结门禁")
	}
	r.Pending = false
	if !w.expirePetCorpses(r, now.Add(time.Second)) || len(personal.PetCorpses) != 0 || !w.canBeginPetBirth(r, now.Add(time.Second)) || w.resolveInWorld(r, personal, oldRef) != nil {
		t.Fatal("绝对TTL被延长，或原对象退役后旧引用仍可解析")
	}
}

// TestPetRuntimeHandleSkipsAllReservedSlotsAndRejectsOverflow 核对「句柄耗尽时仍接受永久消耗前的新生预约」与「新句柄侵占预分配的八怪槽」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeHandleSkipsAllReservedSlotsAndRejectsOverflow(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，personal 保存由场景世界房间执行器取得的个人。
	// now 保存由场景世界房间执行器取得的当前时间。
	w, r, _, personal, _, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], r.Pet, r.Pet.HP, now)
	r.PetHandleSequence = 9
	// pet 保存由生成或出生点运行时宠物取得的宠物。
	pet := spawnRuntimePet(t, w, r, personal, 0, now)
	if pet.Handle != 18 {
		t.Fatal("新句柄侵占预分配的八怪槽", pet.Handle)
	}
	w.applyDamageInWorld(r, personal, personal.Monsters[0], pet, pet.HP, now)
	r.PetHandleSequence = 999
	pet = spawnRuntimePet(t, w, r, personal, 0, now)
	if pet.Handle != 1001 {
		t.Fatal("新句柄侵占掉落1000", pet.Handle)
	}
	w.applyDamageInWorld(r, personal, personal.Monsters[0], pet, pet.HP, now)
	r.PetHandleSequence = math.MaxUint32
	if w.canBeginPetBirth(r, now) || w.reservePetBirth(r, petBirthToken{ID: "overflow", ExecutionEpoch: r.ExecutionEpoch, PlayerLife: r.Player.Life}, now) {
		t.Fatal("句柄耗尽时仍接受永久消耗前的新生预约")
	}
}

// TestPetRuntimeInactiveCorpseExpiresWithoutWakingOriginalMonsters 核验主人不能独立移交，用诊断「清理inactive尸体唤醒原怪物/改变原截止」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeInactiveCorpseExpiresWithoutWakingOriginalMonsters(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，personal 保存由场景世界房间执行器取得的个人。
	// town 保存由场景世界房间执行器取得的城镇，now 保存由场景世界房间执行器取得的当前时间。
	w, r, _, personal, town, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	w.applyDamageInWorld(r, personal, personal.Monsters[0], pet, pet.HP, now)
	// deadline 保存过期集合发生时间，用于当前步骤中的截止时间。
	deadline := personal.PetCorpses[0].ExpiresAt
	// monster 保存从怪物集合取出的怪物。
	monster := personal.Monsters[0]
	monster.HP, monster.RespawnAt = 0, now.Add(time.Millisecond)
	// originalDeadline 保存复活发生时间，用于当前步骤中的原始截止时间。
	originalDeadline := monster.RespawnAt
	if !w.activateWorld(r, town, devconfig.Point{X: 633, Y: 613}, nil) {
		t.Fatal("主人不能独立移交")
	}
	if len(personal.PetCorpses) != 1 || personal.PetCorpses[0].Body != pet || !deadline.Equal(personal.PetCorpses[0].ExpiresAt) || len(town.PetCorpses) != 0 {
		t.Fatal("尸体跟随主人或旅行刷新截止")
	}
	w.step(r, now.Add(time.Second))
	if len(personal.PetCorpses) != 0 || monster.HP != 0 || monster.RespawnAt != originalDeadline || r.Isolated {
		t.Fatal("清理inactive尸体唤醒原怪物/改变原截止")
	}
}

// TestPetRuntimeFactoryFreezesDefinitionAndKeepsLegacyUntrained 核对「出生定义引用了可被后续替换的配置切片/阈值」与「bootstrap误用训练加成或非原统计」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeFactoryFreezesDefinitionAndKeepsLegacyUntrained(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，personal 保存由场景世界房间执行器取得的个人。
	// now 保存由场景世界房间执行器取得的当前时间。
	w, r, _, personal, _, now := worldWorker(t)
	// d 接收启用宠物运行时测试供后续步骤使用。
	d := enablePetRuntimeTest(t, w, r)
	// legacy 保存宠物，用于当前步骤中的兼容旧数据。
	legacy := r.Pet
	if !w.attachLegacyPetState(legacy) || legacy.PetRuntime.Train || !legacy.PetRuntime.LegacyUntrained || !reflect.DeepEqual(w.fighter(r, legacy), w.e.Store.Cfg.Fixture.Pet) {
		t.Fatal("bootstrap误用训练加成或非原统计")
	}
	w.applyDamageInWorld(r, personal, personal.Monsters[0], legacy, legacy.HP, now)
	// pet 保存由生成或出生点运行时宠物取得的宠物。
	pet := spawnRuntimePet(t, w, r, personal, 3, now)
	// fighter 保存由战斗单位发生时间取得的战斗单位。
	fighter, _ := d.FighterAt(3, 7)
	if pet.PetRuntime.Level != 3 || pet.PetRuntime.Maximum != 7 || !reflect.DeepEqual(w.fighter(r, pet), fighter) || pet.ActionReadyAt != now.Add(2*time.Second) || fighter.Level != d.Base.Level {
		t.Fatal("训练级混入模板等级，或首次Spawn动作锁/实体统计错误")
	}
	// expectedBonus 保存生命值加成，用于当前步骤中的预期结果加成。
	expectedBonus := d.Growth[3].HPBonus
	w.e.Store.Cfg.Fixture.PetRuntime.Definitions[0].Growth[3].HPBonus = 99999
	*w.e.Store.Cfg.Fixture.PetRuntime.Definitions[0].Growth[3].RequiredExperience = 1
	if pet.PetRuntime.Definition.Growth[3].HPBonus != expectedBonus || *pet.PetRuntime.Definition.Growth[3].RequiredExperience != 80000 || !reflect.DeepEqual(w.fighter(r, pet), fighter) {
		t.Fatal("出生定义引用了可被后续替换的配置切片/阈值")
	}
}
