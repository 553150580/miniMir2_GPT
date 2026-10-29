package game

import (
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
	"time"
)

func TestPetResidentWorldDoesNotFollowAttachmentImplicitly(t *testing.T) {
	w, r, c, personal, town, _ := worldWorker(t)
	pet := r.Pet
	r.PetWorld = personal.Ref
	pet.Pos = devconfig.Point{X: 7, Y: 7}
	petRef := referenceIn(personal, pet)
	playerRef := referenceIn(personal, &r.Player)
	r.worldState = town // 宠物尚未执行随行/Recall，不能仅随观察图变居民。
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	if w.resolveInWorld(r, personal, petRef) != pet {
		t.Fatal("原World宠物失去真实居民身份")
	}
	if w.resolveInWorld(r, personal, playerRef) != nil || w.resolveInWorld(r, town, referenceIn(town, pet)) != nil {
		t.Fatal("另一World注册了幽灵角色或宠物")
	}
	if reference(r, pet) != petRef {
		t.Fatal("宠物引用错误使用主人当前World")
	}
	if !w.free(r, pet.Pos, &r.Player) {
		t.Fatal("旧World宠物占格阻挡新图相同坐标")
	}
	c.frames = nil
	w.baseline(r)
	for _, frame := range c.frames {
		if frame.ID != p.MsgS2CSceneBaseline {
			continue
		}
		var baseline pb.SceneBaselineChunk
		if err := proto.Unmarshal(frame.Payload, &baseline); err != nil {
			t.Fatal(err)
		}
		for _, e := range baseline.Entities {
			if e.EntityKind == 2 {
				t.Fatal("新scene显示原World宠物")
			}
		}
	}
}

func TestPetResidentTargetsAndCurrentMovementStayInWorld(t *testing.T) {
	w, r, _, personal, town, now := worldWorker(t)
	r.PetWorld = personal.Ref
	pet := r.Pet
	pet.Pos = devconfig.Point{X: 7, Y: 7}
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	healing, _ := w.e.Store.Cfg.Fixture.Skill(2)
	if w.skillTargetInWorld(r, personal, healing, pet.Handle) != pet || w.skillTargetInWorld(r, town, healing, pet.Handle) != nil {
		t.Fatal("治疗按主人附着而非宠物驻留图选目标")
	}
	mass, _ := w.e.Store.Cfg.Fixture.Skill(29)
	if got := w.areaTargetsInWorld(r, personal, mass, pet.Pos); len(got) != 1 || got[0] != pet {
		t.Fatal("原World友方区域未按居民包含宠物", got)
	}
	if got := w.areaTargetsInWorld(r, town, mass, pet.Pos); len(got) != 0 {
		t.Fatal("新World友方区域选中旧图宠物", got)
	}
	before := *pet
	if w.commitWalk(r, pet, devconfig.Point{X: 8, Y: 7}, 2, now, 1000) || pet.Pos != before.Pos || pet.MovementRevision != before.MovementRevision {
		t.Fatal("普通当前图移动跨图改写宠物")
	}
}

// 仅这组显式旧清槽合同降为历史rule15；常规观察/World/技能helper保持当前运行规则。
func legacyPetRule15(w *worker, r *room) {
	w.e.Store.Cfg.Fixture.RuleVersion, w.e.Store.Cfg.Fixture.PetRuntime = 15, nil
	if r.Pet != nil {
		r.Pet.PetRuntime, r.Pet.IsPetCorpse = nil, false
	}
}

func TestLegacyPetBirthAcrossCorpseRetirementAndEmptySlot(t *testing.T) {
	w, r, _, personal, town, _ := worldWorker(t)
	legacyPetRule15(w, r)
	r.PetWorld = personal.Ref
	corpse := r.Pet
	corpse.Life, corpse.HP, corpse.Action = 9, 0, 4
	corpse.HealAmount = 11
	corpse.Statuses = map[uint32]*combatStatus{1: {ID: 1, TargetLife: 9, RemainingMS: 100}}
	old := reference(r, corpse)
	newPet := func() *entity {
		return &entity{Handle: petHandle, Kind: 2, Def: corpse.Def, Pos: devconfig.Point{X: 633, Y: 613}, HP: 20, MaxHP: 30, MP: 3, MaxMP: 5}
	}
	if w.installPet(r, town, newPet()) || r.Pet != corpse || r.PetLifeSequence != 0 {
		t.Fatal("安装新宠暗中删除resident尸体")
	}
	stale := old
	stale.Life--
	if detached, ok := w.retirePet(r, stale); ok || detached != nil {
		t.Fatal("旧birth可以退役当前宠物")
	}
	before := *corpse
	detached, ok := w.retirePet(r, old)
	if !ok || detached != corpse || !reflect.DeepEqual(*corpse, before) || r.Pet != nil || r.PetWorld != (worldRef{}) || r.PetLifeSequence != 9 {
		t.Fatal("退役销毁尸体/状态或重置birth水位")
	}
	pet := newPet()
	if !w.installPet(r, town, pet) || r.Pet != pet || pet.Life != 10 || r.PetWorld != town.Ref || r.CellEntrySequence != pet.CellEntry {
		t.Fatal("空槽新生没有分配单调birth与真实居民World")
	}
	if !town.HasPet || town.PetLife != pet.Life || town.PetPosition != pet.Pos {
		t.Fatal("安装未记录新birth的真实位置")
	}
	if w.resolveInWorld(r, personal, old) != nil || w.resolveCredit(r, old) != nil || w.installPet(r, personal, newPet()) {
		t.Fatal("旧birth复活，或安装覆写当前存活宠")
	}
	if _, ok := w.retirePet(r, reference(r, pet)); !ok {
		t.Fatal("不能显式退役新宠")
	}
	next := newPet()
	if !w.installPet(r, town, next) || next.Life != 11 {
		t.Fatal("经过nil后birth回绕")
	}
}

func TestLegacyPetBirthRejectsOverflowAndForeignWorldWithoutPartialMutation(t *testing.T) {
	w, r, _, personal, _, _ := worldWorker(t)
	legacyPetRule15(w, r)
	r.PetWorld = personal.Ref
	if _, ok := w.retirePet(r, reference(r, r.Pet)); !ok {
		t.Fatal("fixture退役失败")
	}
	for _, test := range []string{"birth", "cell", "foreign"} {
		t.Run(test, func(t *testing.T) {
			r.PetLifeSequence, r.CellEntrySequence = 4, 8
			world := personal
			if test == "birth" {
				r.PetLifeSequence = math.MaxUint64
			} else if test == "cell" {
				r.CellEntrySequence = math.MaxUint64
			} else {
				copy := *personal
				world = &copy
			}
			pet := &entity{Handle: petHandle, Kind: 2, Pos: r.Player.Pos, HP: 10, MaxHP: 10}
			sequence, cell, original := r.PetLifeSequence, r.CellEntrySequence, *pet
			if w.installPet(r, world, pet) || r.Pet != nil || r.PetLifeSequence != sequence || r.CellEntrySequence != cell || !reflect.DeepEqual(*pet, original) {
				t.Fatal("非法安装产生部分新生或身份回绕")
			}
		})
	}
}

func TestPetResidencyZeroCompatibilityNeverFallsBackFromUnknownWorld(t *testing.T) {
	w, r, _, personal, town, _ := worldWorker(t)
	r.PetWorld = worldRef{} // 显式零值旧手造前态；正式fixture由helper明确填当前World。
	if r.PetWorld != (worldRef{}) || r.worldPet(personal) != r.Pet {
		t.Fatal("旧手造fixture零身份兼容失效")
	}
	r.PetWorld = worldRef{Epoch: personal.Ref.Epoch, ID: 99}
	if r.worldPet(personal) != nil || r.worldPet(town) != nil || reference(r, r.Pet).World != r.PetWorld {
		t.Fatal("未知非零居民身份静默回退到主人图")
	}
	before, pointer := r.Player.Pos, r.Pet
	point := devconfig.Point{X: 633, Y: 613}
	if w.activateWorld(r, town, point, &point) || r.currentWorld() != personal || r.Player.Pos != before || r.Pet != pointer {
		t.Fatal("迁移默默修复未知PetWorld并改变现场")
	}
}

func TestLegacyPetMigrationKeepsUniqueCorpseAndIndependentResidency(t *testing.T) {
	w, r, _, personal, town, _ := worldWorker(t)
	legacyPetRule15(w, r)
	r.PetWorld = personal.Ref
	pet := r.Pet
	pet.HP, pet.Life, pet.HealAmount = 0, 7, 9
	pet.Action = 4
	point, petPoint := devconfig.Point{X: 633, Y: 613}, devconfig.Point{X: 632, Y: 613}
	if !w.activateWorld(r, town, point, &petPoint) || r.Pet != pet || r.PetWorld != town.Ref || pet.Life != 7 || pet.HP != 0 || pet.Action != 4 || pet.HealAmount != 9 {
		t.Fatal("普通健康旅行复制/复活尸体或遗失身份")
	}
	player, scene := r.Player, r.Scene
	ref := reference(r, pet)
	if !w.relocatePet(r, ref, personal, devconfig.Point{X: 7, Y: 7}) || r.Pet != pet || r.PetWorld != personal.Ref || r.Scene != scene || !reflect.DeepEqual(r.Player, player) {
		t.Fatal("独立宠物移交擅自移动主人或attachment")
	}
	if personal.PetPosition != pet.Pos || personal.PetLife != pet.Life || !personal.HasPet || town.PetPosition != petPoint {
		t.Fatal("宠物移交没有分开记录原/目标World的实际位置")
	}
	if w.relocatePet(r, ref, town, petPoint) {
		t.Fatal("跨图前旧居民ref能重复移交")
	}
	if w.resolveInWorld(r, personal, reference(r, pet)) != pet || w.resolveInWorld(r, personal, referenceIn(personal, &r.Player)) != nil {
		t.Fatal("独立居民缺宠物或出现ghostPlayer")
	}
	if !w.activateWorld(r, personal, personal.PlayerPosition, &personal.PetPosition) || r.Pet != pet || r.PetWorld != personal.Ref || pet.HP != 0 || pet.Life != 7 {
		t.Fatal("返回原图没有移交同一宠物现场")
	}
}

func TestPetOldWorldReleasedAttackResolvesResidentWithoutSceneProjection(t *testing.T) {
	w, r, c, personal, town, now := worldWorker(t)
	r.PetWorld = personal.Ref
	pet, monster := r.Pet, personal.Monsters[0]
	pet.HP, pet.MaxHP = 100, 100
	attack := pendingAttack{Scene: r.Scene, Caster: referenceIn(personal, monster), Target: reference(r, pet), Amount: 30, Due: now, Defence: defenceAC}
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	r.Scene++
	c.frames = nil
	w.resolveAttackInWorld(r, personal, attack, now)
	if pet.HP >= 100 {
		t.Fatal("已释放原World命中错误依赖主人仍观察原图")
	}
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CCombatImpact || frame.ID == p.MsgS2CEntityDisplacement {
			t.Fatal("原World命中包装成新scene事件")
		}
	}
	before := *pet
	w.move(r, pet, devconfig.Point{X: pet.Pos.X + 1, Y: pet.Pos.Y}, now, 1)
	w.attack(r, &r.Player, pet, now)
	if out := w.sourcePushed(r, reference(r, pet), 2, 1, now, displacementSource{}); out.Stop != pushStale || !reflect.DeepEqual(before, *pet) {
		t.Fatal("当前图AI/攻击/推挤越界写另一World宠物", out)
	}
}

func TestLegacyPetCorpseRemainsVisibleOnlyInResidentWorld(t *testing.T) {
	w, r, c, personal, town, _ := worldWorker(t)
	legacyPetRule15(w, r)
	r.PetWorld = personal.Ref
	r.Pet.HP, r.Pet.Action = 0, 4
	for _, world := range []*worldState{personal, town, personal} {
		r.worldState = world
		c.frames = nil
		w.baseline(r)
		count := 0
		for _, frame := range c.frames {
			if frame.ID != p.MsgS2CSceneBaseline {
				continue
			}
			var baseline pb.SceneBaselineChunk
			if err := proto.Unmarshal(frame.Payload, &baseline); err != nil {
				t.Fatal(err)
			}
			for _, e := range baseline.Entities {
				if e.EntityKind == 2 {
					count++
					if e.Hp != 0 || e.Action != 4 {
						t.Fatal("尸体因显示而复活")
					}
				}
			}
		}
		if world == personal && count != 1 || world == town && count != 0 {
			t.Fatal("尸体被销毁或跨World显示", count)
		}
	}
}

func TestPetTownNavigationUsesResidentOccupancyAndReturnsRetiredTask(t *testing.T) {
	w, r := navigationTownWorker(t, 8, 8, 32, 32, 1, 2)
	town := r.currentWorld()
	r.TownWorld = town
	r.PersonalWorld = r.newWorld(1)
	pet := &entity{Handle: petHandle, Kind: 2, Pos: devconfig.Point{X: 2, Y: 1}, HP: 10, MaxHP: 10}
	if !w.installPet(r, r.PersonalWorld, pet) {
		t.Fatal("独立World安装失败")
	}
	target := devconfig.Point{X: 6, Y: 1}
	requestTownGround(r, target)
	w.moveTown(r, &r.Player, target, time.Unix(1, 0), 1)
	q := r.TownNavigation[0]
	if q == nil || len(q.dynamic.Blocked) != 0 || townActorIndex(r, pet) != -1 {
		t.Fatal("另一World宠物占用当前Town工作槽或动态格")
	}
	if !w.relocatePet(r, reference(r, pet), town, pet.Pos) || !w.updateTownOccupancy(q) || len(q.dynamic.Blocked) != 1 {
		t.Fatal("真实入场未推进Town占位")
	}
	w.moveTown(r, pet, target, time.Unix(1, 0), 1)
	w.beginTownNavigationTick(time.Unix(1, 0))
	if r.TownNavigation[1] == nil {
		t.Fatal("fixture宠物任务未受理")
	}
	if _, ok := w.retirePet(r, reference(r, pet)); !ok || r.TownNavigation[1] != nil {
		t.Fatal("退役后旧宠仍占导航任务/工作槽")
	}
	if !w.updateTownOccupancy(q) || len(q.dynamic.Blocked) != 0 {
		t.Fatal("退役未清除当前Town动态占格")
	}
}

func TestPetMapTravelReturnNeverAppliesPreviousBirthPosition(t *testing.T) {
	for _, resident := range []string{"town", "personal"} {
		t.Run(resident, func(t *testing.T) {
			w, r, _, now := travelWorker(t)
			personal := r.currentWorld()
			r.PetWorld = personal.Ref
			m, _ := w.worldMap(personal)
			points := []devconfig.Point{}
			for y := 0; y < m.Height && len(points) < 2; y++ {
				for x := 0; x < m.Width && len(points) < 2; x++ {
					point := devconfig.Point{X: x, Y: y}
					if m.Walkable(point) && distance(point, r.Player.Pos) > 2 && !travelOccupied(r.Monsters, r.MonsterCount, point) {
						points = append(points, point)
					}
				}
			}
			if len(points) != 2 {
				t.Fatal("fixture缺少两个合法独立宠物点")
			}
			r.Pet.Pos = points[0]
			prepareTravelAt(w, r, 1, travelToTown, now)
			confirmTravelAt(w, r, 1, r.Scene, false, now)
			if r.currentWorld() == personal || !w.finishMapTravelAck(r, now) {
				t.Fatal("fixture进城失败")
			}
			r.Ready = true
			old := r.Pet
			w.applyDamageInWorld(r, r.currentWorld(), &r.Player, old, old.HP, now)
			if r.Pet != nil || !old.IsPetCorpse {
				t.Fatal("旧宠未按当前规则成为World尸体")
			}
			world, position := r.currentWorld(), old.Pos
			expected, ok := travelPetPoint(m, personal.PlayerPosition, nil, personal.Monsters, personal.MonsterCount)
			if !ok {
				t.Fatal("fixture返回宠物无受控落点")
			}
			if resident == "personal" {
				world, position, expected = personal, points[1], points[1]
			}
			r.Role.Job = w.e.Store.Cfg.Fixture.Pet.JobID
			definition, _ := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByBodyID(old.Def)
			pet, made := w.newPet(r, definition.ID, 0, position, now)
			if !made || !w.installPet(r, world, pet) || pet.Life <= old.Life {
				t.Fatal("新生安装失败")
			}
			prepareTravelAt(w, r, 2, travelToPersonal, now.Add(time.Second))
			confirmTravelAt(w, r, 2, r.Scene, false, now.Add(time.Second))
			if r.currentWorld() != personal || r.Pet != pet || r.Pet.Pos != expected || r.Pet.Pos == points[0] {
				t.Fatalf("新birth继承旧宠位置或覆盖实际resident: old=%v actual=%v expected=%v", points[0], pet.Pos, expected)
			}
		})
	}
}

func TestPetTownArrivalKeepsDestinationResidentPosition(t *testing.T) {
	w, r, _, now := travelWorker(t)
	r.PersonalWorld = r.currentWorld()
	r.PetWorld = r.PersonalWorld.Ref
	old := r.Pet
	w.applyDamageInWorld(r, r.PersonalWorld, &r.Player, old, old.HP, now)
	if r.Pet != nil || !old.IsPetCorpse {
		t.Fatal("当前规则死亡归档失败")
	}
	r.TownWorld = r.newWorld(2)
	m, _ := w.worldMap(r.TownWorld)
	var position *devconfig.Point
	for y := 0; y < m.Height && position == nil; y++ {
		for x := 0; x < m.Width; x++ {
			point := devconfig.Point{X: x, Y: y}
			if m.Walkable(point) && distance(point, m.PlayerSpawn) > 2 {
				position = &point
				break
			}
		}
	}
	if position == nil {
		t.Fatal("fixture缺少独立Town落点")
	}
	r.Role.Job = w.e.Store.Cfg.Fixture.Pet.JobID
	definition, _ := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByBodyID(old.Def)
	pet, made := w.newPet(r, definition.ID, 0, *position, now)
	if !made || !w.installPet(r, r.TownWorld, pet) {
		t.Fatal("目的图安装失败")
	}
	prepareTravelAt(w, r, 1, travelToTown, now)
	confirmTravelAt(w, r, 1, r.Scene, false, now)
	if r.currentWorld() != r.TownWorld || r.Pet != pet || r.Pet.Pos != *position || r.PetWorld != r.TownWorld.Ref {
		t.Fatal("进Town时覆盖了目标图真实Pet位置")
	}
}
