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

func spawnRuntimePet(t *testing.T, w *worker, r *room, world *worldState, rank uint32, now time.Time) *entity {
	t.Helper()
	pet, ok := w.newPet(r, 1, rank, r.Player.Pos, now)
	if !ok || !w.installPet(r, world, pet) {
		t.Fatal("显式测试定义不能安装正规训练宠")
	}
	return pet
}

func TestPetRuntimeRetains128BodiesWithOneLivingPetAndNoMasterCorpses(t *testing.T) {
	w, r, c, personal, _, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	w.e.Store.Cfg.Fixture.PetRuntime.Budgets.RetainedBodyCapacityPerOwner = 128
	old := r.Pet
	oldRef := reference(r, old)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], old, old.HP, now)
	for i := 0; i < 126; i++ {
		pet := spawnRuntimePet(t, w, r, personal, 0, now)
		w.applyDamageInWorld(r, personal, personal.Monsters[0], pet, pet.HP, now)
	}
	pet := spawnRuntimePet(t, w, r, personal, 0, now)
	if len(personal.PetCorpses) != 127 || r.retainedPetBodies() != 128 || r.Pet != pet {
		t.Fatal("128身体名额未包含活动宠与全部原尸体")
	}
	spell, _ := w.e.Store.Cfg.Fixture.Skill(14)
	for _, target := range w.buffTargets(r, pendingCast{Skill: spell, Caster: reference(r, &r.Player), TargetPoint: old.Pos}) {
		if target.IsPetCorpse {
			t.Fatal("允许死目标的14把无主尸体当友方")
		}
	}
	playerBefore, petBefore := r.Player.HP, pet.HP
	w.resolveAttackInWorld(r, personal, pendingAttack{Caster: referenceIn(personal, personal.Monsters[0]), Target: oldRef, Amount: 99999, Due: now, Defence: defenceAC}, now)
	if old.HP != 0 || r.Player.HP != playerBefore || pet.HP != petBefore || w.resolveInWorld(r, personal, oldRef) != old {
		t.Fatal("旧目标引用命中新宠/玩家或丢失旧尸体")
	}
	c.frames = nil
	w.baseline(r)
	seen, corpses, live, maxBytes := map[uint32]bool{}, 0, 0, 0
	for _, frame := range c.frames {
		if frame.ID != p.MsgS2CSceneBaseline {
			continue
		}
		if n := len(wire.Encode(frame)); n > maxBytes {
			maxBytes = n
		}
		var chunk pb.SceneBaselineChunk
		if err := proto.Unmarshal(frame.Payload, &chunk); err != nil {
			t.Fatal(err)
		}
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

func TestPetRuntimeCorpseReservationCapacityAndOriginalIdentity(t *testing.T) {
	w, r, _, personal, _, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	w.e.Store.Cfg.Fixture.PetRuntime.Budgets.RetainedBodyCapacityPerOwner = 2
	old := r.Pet
	oldRef := reference(r, old)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], old, old.HP, now)
	if len(personal.PetCorpses) != 1 || personal.PetCorpses[0].Body != old || !old.IsPetCorpse {
		t.Fatal("原身体没有转交原World")
	}
	token := petBirthToken{ID: "first", ExecutionEpoch: r.ExecutionEpoch, PlayerLife: r.Player.Life}
	if !w.reservePetBirth(r, token, now) || !w.reservePetBirth(r, token, now) || r.retainedPetBodies() != 2 {
		t.Fatal("新生预约没有一次占据未来尸体名额")
	}
	other := token
	other.ID = "late"
	if w.reservePetBirth(r, other, now) || w.cancelPetBirth(r, other) {
		t.Fatal("不同计划抢占/释放在途预约")
	}
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

func TestPetRuntimeHandleSkipsAllReservedSlotsAndRejectsOverflow(t *testing.T) {
	w, r, _, personal, _, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], r.Pet, r.Pet.HP, now)
	r.PetHandleSequence = 9
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

func TestPetRuntimeInactiveCorpseExpiresWithoutWakingOriginalMonsters(t *testing.T) {
	w, r, _, personal, town, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	pet := r.Pet
	w.applyDamageInWorld(r, personal, personal.Monsters[0], pet, pet.HP, now)
	deadline := personal.PetCorpses[0].ExpiresAt
	monster := personal.Monsters[0]
	monster.HP, monster.RespawnAt = 0, now.Add(time.Millisecond)
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

func TestPetRuntimeFactoryFreezesDefinitionAndKeepsLegacyUntrained(t *testing.T) {
	w, r, _, personal, _, now := worldWorker(t)
	d := enablePetRuntimeTest(t, w, r)
	legacy := r.Pet
	if !w.attachLegacyPetState(legacy) || legacy.PetRuntime.Train || !legacy.PetRuntime.LegacyUntrained || !reflect.DeepEqual(w.fighter(r, legacy), w.e.Store.Cfg.Fixture.Pet) {
		t.Fatal("bootstrap误用训练加成或非原统计")
	}
	w.applyDamageInWorld(r, personal, personal.Monsters[0], legacy, legacy.HP, now)
	pet := spawnRuntimePet(t, w, r, personal, 3, now)
	fighter, _ := d.FighterAt(3, 7)
	if pet.PetRuntime.Level != 3 || pet.PetRuntime.Maximum != 7 || !reflect.DeepEqual(w.fighter(r, pet), fighter) || pet.ActionReadyAt != now.Add(2*time.Second) || fighter.Level != d.Base.Level {
		t.Fatal("训练级混入模板等级，或首次Spawn动作锁/实体统计错误")
	}
	expectedBonus := d.Growth[3].HPBonus
	w.e.Store.Cfg.Fixture.PetRuntime.Definitions[0].Growth[3].HPBonus = 99999
	*w.e.Store.Cfg.Fixture.PetRuntime.Definitions[0].Growth[3].RequiredExperience = 1
	if pet.PetRuntime.Definition.Growth[3].HPBonus != expectedBonus || *pet.PetRuntime.Definition.Growth[3].RequiredExperience != 80000 || !reflect.DeepEqual(w.fighter(r, pet), fighter) {
		t.Fatal("出生定义引用了可被后续替换的配置切片/阈值")
	}
}
