// 本文件回归宠物驻留、迁移、召回、原世界事件与寻路占位的身份边界。
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

// TestPetResidentWorldDoesNotFollowAttachmentImplicitly 核对「另一World注册了幽灵角色或宠物」与「宠物引用错误使用主人当前World」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetResidentWorldDoesNotFollowAttachmentImplicitly(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，c 接收场景世界房间执行器供后续步骤使用。
	// personal 保存由场景世界房间执行器取得的个人，town 保存由场景世界房间执行器取得的城镇。
	w, r, c, personal, town, _ := worldWorker(t)
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	r.PetWorld = personal.Ref
	pet.Pos = devconfig.Point{X: 7, Y: 7}
	// petRef 保存由引用输入取得的宠物引用。
	petRef := referenceIn(personal, pet)
	// playerRef 保存由引用输入取得的玩家引用。
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
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range c.frames {
		if frame.ID != p.MsgS2CSceneBaseline {
			continue
		}
		// baseline 记录场景基线，使用场景场景基线分片表示。
		var baseline pb.SceneBaselineChunk
		// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
		if err := proto.Unmarshal(frame.Payload, &baseline); err != nil {
			t.Fatal(err)
		}
		// e 接收遍历实体时当前的元素。
		for _, e := range baseline.Entities {
			if e.EntityKind == 2 {
				t.Fatal("新scene显示原World宠物")
			}
		}
	}
}

// TestPetResidentTargetsAndCurrentMovementStayInWorld 核对「治疗按主人附着而非宠物驻留图选目标」与「原World友方区域未按居民包含宠物」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetResidentTargetsAndCurrentMovementStayInWorld(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，personal 保存由场景世界房间执行器取得的个人。
	// town 保存由场景世界房间执行器取得的城镇，now 保存由场景世界房间执行器取得的当前时间。
	w, r, _, personal, town, now := worldWorker(t)
	r.PetWorld = personal.Ref
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	pet.Pos = devconfig.Point{X: 7, Y: 7}
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	// healing 保存由技能取得的治疗。
	healing, _ := w.e.Store.Cfg.Fixture.Skill(2)
	if w.skillTargetInWorld(r, personal, healing, pet.Handle) != pet || w.skillTargetInWorld(r, town, healing, pet.Handle) != nil {
		t.Fatal("治疗按主人附着而非宠物驻留图选目标")
	}
	// mass 保存由技能取得的“mass”。
	mass, _ := w.e.Store.Cfg.Fixture.Skill(29)
	// got 保存由区域目标集合输入场景世界取得的实际结果。
	if got := w.areaTargetsInWorld(r, personal, mass, pet.Pos); len(got) != 1 || got[0] != pet {
		t.Fatal("原World友方区域未按居民包含宠物", got)
	}
	// got 保存由区域目标集合输入场景世界取得的实际结果。
	if got := w.areaTargetsInWorld(r, town, mass, pet.Pos); len(got) != 0 {
		t.Fatal("新World友方区域选中旧图宠物", got)
	}
	// before 保存当前处理所用的操作前的状态。
	before := *pet
	if w.commitWalk(r, pet, devconfig.Point{X: 8, Y: 7}, 2, now, 1000) || pet.Pos != before.Pos || pet.MovementRevision != before.MovementRevision {
		t.Fatal("普通当前图移动跨图改写宠物")
	}
}

// 仅这组显式旧清槽合同降为历史rule15；常规观察/World/技能helper保持当前运行规则。
// w 记录房间执行器，r 指向当前执行器拥有并更新的目标房间。
func legacyPetRule15(w *worker, r *room) {
	w.e.Store.Cfg.Fixture.RuleVersion, w.e.Store.Cfg.Fixture.PetRuntime = 15, nil
	if r.Pet != nil {
		r.Pet.PetRuntime, r.Pet.IsPetCorpse = nil, false
	}
}

// TestLegacyPetBirthAcrossCorpseRetirementAndEmptySlot 核验空槽新生应分配单调birth与真实居民World，以及不能显式退役新宠。
// t 提供断言失败报告、子用例和清理注册。
func TestLegacyPetBirthAcrossCorpseRetirementAndEmptySlot(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，personal 保存由场景世界房间执行器取得的个人。
	// town 保存由场景世界房间执行器取得的城镇。
	w, r, _, personal, town, _ := worldWorker(t)
	legacyPetRule15(w, r)
	r.PetWorld = personal.Ref
	// corpse 保存宠物，用于当前步骤中的尸体。
	corpse := r.Pet
	corpse.Life, corpse.HP, corpse.Action = 9, 0, 4
	corpse.HealAmount = 11
	corpse.Statuses = map[uint32]*combatStatus{1: {ID: 1, TargetLife: 9, RemainingMS: 100}}
	// old 保存由引用取得的原状态。
	old := reference(r, corpse)
	// newPet 封装创建宠物对应的后续处理。
	newPet := func() *entity {
		return &entity{Handle: petHandle, Kind: 2, Def: corpse.Def, Pos: devconfig.Point{X: 633, Y: 613}, HP: 20, MaxHP: 30, MP: 3, MaxMP: 5}
	}
	if w.installPet(r, town, newPet()) || r.Pet != corpse || r.PetLifeSequence != 0 {
		t.Fatal("安装新宠暗中删除resident尸体")
	}
	// stale 保存当前处理所用的已失效。
	stale := old
	stale.Life--
	// detached 保存由“retire”宠物取得的脱离，ok 表明查询、校验或类型转换是否成功。
	if detached, ok := w.retirePet(r, stale); ok || detached != nil {
		t.Fatal("旧birth可以退役当前宠物")
	}
	// before 保存当前处理所用的操作前的状态。
	before := *corpse
	// detached 保存由“retire”宠物取得的脱离，ok 表明查询、校验或类型转换是否成功。
	detached, ok := w.retirePet(r, old)
	if !ok || detached != corpse || !reflect.DeepEqual(*corpse, before) || r.Pet != nil || r.PetWorld != (worldRef{}) || r.PetLifeSequence != 9 {
		t.Fatal("退役销毁尸体/状态或重置birth水位")
	}
	// pet 保存由创建宠物取得的宠物。
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
	// ok 表明查询、校验或类型转换是否成功。
	if _, ok := w.retirePet(r, reference(r, pet)); !ok {
		t.Fatal("不能显式退役新宠")
	}
	// next 保存由创建宠物取得的下一状态。
	next := newPet()
	if !w.installPet(r, town, next) || next.Life != 11 {
		t.Fatal("经过nil后birth回绕")
	}
}

// TestLegacyPetBirthRejectsOverflowAndForeignWorldWithoutPartialMutation 核对「非法安装产生部分新生或身份回绕」与「fixture退役失败」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestLegacyPetBirthRejectsOverflowAndForeignWorldWithoutPartialMutation(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，personal 保存由场景世界房间执行器取得的个人。
	w, r, _, personal, _, _ := worldWorker(t)
	legacyPetRule15(w, r)
	r.PetWorld = personal.Ref
	// ok 表明查询、校验或类型转换是否成功。
	if _, ok := w.retirePet(r, reference(r, r.Pet)); !ok {
		t.Fatal("fixture退役失败")
	}
	// test 接收遍历文本列表时当前的元素。
	for _, test := range []string{"birth", "cell", "foreign"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(test, func(t *testing.T) {
			r.PetLifeSequence, r.CellEntrySequence = 4, 8
			// world 保存当前处理所用的场景世界。
			world := personal
			if test == "birth" {
				r.PetLifeSequence = math.MaxUint64
			} else if test == "cell" {
				r.CellEntrySequence = math.MaxUint64
			} else {
				// copy 保存当前处理所用的“copy”。
				copy := *personal
				world = &copy
			}
			// pet 保存当前处理所用的宠物。
			pet := &entity{Handle: petHandle, Kind: 2, Pos: r.Player.Pos, HP: 10, MaxHP: 10}
			// sequence 保存宠物生命代次序号，用于当前步骤中的序号，cell 保存地图格入场序号，用于当前步骤中的地图格，original 保存当前处理所用的原始。
			sequence, cell, original := r.PetLifeSequence, r.CellEntrySequence, *pet
			if w.installPet(r, world, pet) || r.Pet != nil || r.PetLifeSequence != sequence || r.CellEntrySequence != cell || !reflect.DeepEqual(*pet, original) {
				t.Fatal("非法安装产生部分新生或身份回绕")
			}
		})
	}
}

// TestPetResidencyZeroCompatibilityNeverFallsBackFromUnknownWorld 核对「迁移默默修复未知PetWorld并改变现场」与「旧手造fixture零身份兼容失效」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetResidencyZeroCompatibilityNeverFallsBackFromUnknownWorld(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，personal 保存由场景世界房间执行器取得的个人。
	// town 保存由场景世界房间执行器取得的城镇。
	w, r, _, personal, town, _ := worldWorker(t)
	r.PetWorld = worldRef{} // 显式零值旧手造前态；正式fixture由helper明确填当前World。
	if r.PetWorld != (worldRef{}) || r.worldPet(personal) != r.Pet {
		t.Fatal("旧手造fixture零身份兼容失效")
	}
	r.PetWorld = worldRef{Epoch: personal.Ref.Epoch, ID: 99}
	if r.worldPet(personal) != nil || r.worldPet(town) != nil || reference(r, r.Pet).World != r.PetWorld {
		t.Fatal("未知非零居民身份静默回退到主人图")
	}
	// before 保存位置，用于当前步骤中的操作前的状态，pointer 保存宠物，用于当前步骤中的指针。
	before, pointer := r.Player.Pos, r.Pet
	// point 组装点位所需的数据。
	point := devconfig.Point{X: 633, Y: 613}
	if w.activateWorld(r, town, point, &point) || r.currentWorld() != personal || r.Player.Pos != before || r.Pet != pointer {
		t.Fatal("迁移默默修复未知PetWorld并改变现场")
	}
}

// TestLegacyPetMigrationKeepsUniqueCorpseAndIndependentResidency 核验宠物移交应分开记录原/目标World的实际位置，以及返回原图应移交同一宠物现场。
// t 提供断言失败报告、子用例和清理注册。
func TestLegacyPetMigrationKeepsUniqueCorpseAndIndependentResidency(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，personal 保存由场景世界房间执行器取得的个人。
	// town 保存由场景世界房间执行器取得的城镇。
	w, r, _, personal, town, _ := worldWorker(t)
	legacyPetRule15(w, r)
	r.PetWorld = personal.Ref
	// pet 保存当前处理所用的宠物。
	pet := r.Pet
	pet.HP, pet.Life, pet.HealAmount = 0, 7, 9
	pet.Action = 4
	// point 组装点位所需的数据，petPoint 组装宠物点位所需的数据。
	point, petPoint := devconfig.Point{X: 633, Y: 613}, devconfig.Point{X: 632, Y: 613}
	if !w.activateWorld(r, town, point, &petPoint) || r.Pet != pet || r.PetWorld != town.Ref || pet.Life != 7 || pet.HP != 0 || pet.Action != 4 || pet.HealAmount != 9 {
		t.Fatal("普通健康旅行复制/复活尸体或遗失身份")
	}
	// player 保存当前处理所用的玩家，scene 保存当前处理所用的场景。
	player, scene := r.Player, r.Scene
	// ref 接收引用供后续步骤使用。
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

// TestPetOldWorldReleasedAttackResolvesResidentWithoutSceneProjection 核对「已释放原World命中错误依赖主人仍观察原图」与「当前图AI/攻击/推挤越界写另一World宠物」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetOldWorldReleasedAttackResolvesResidentWithoutSceneProjection(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，c 接收场景世界房间执行器供后续步骤使用。
	// personal 保存由场景世界房间执行器取得的个人，town 保存由场景世界房间执行器取得的城镇，now 保存由场景世界房间执行器取得的当前时间。
	w, r, c, personal, town, now := worldWorker(t)
	r.PetWorld = personal.Ref
	// pet 保存当前处理所用的宠物，monster 保存从怪物集合取出的怪物。
	pet, monster := r.Pet, personal.Monsters[0]
	pet.HP, pet.MaxHP = 100, 100
	// attack 初始化用作攻击的待确认攻击。
	attack := pendingAttack{Scene: r.Scene, Caster: referenceIn(personal, monster), Target: reference(r, pet), Amount: 30, Due: now, Defence: defenceAC}
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	r.Scene++
	c.frames = nil
	w.resolveAttackInWorld(r, personal, attack, now)
	if pet.HP >= 100 {
		t.Fatal("已释放原World命中错误依赖主人仍观察原图")
	}
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CCombatImpact || frame.ID == p.MsgS2CEntityDisplacement {
			t.Fatal("原World命中包装成新scene事件")
		}
	}
	// before 保存当前处理所用的操作前的状态。
	before := *pet
	w.move(r, pet, devconfig.Point{X: pet.Pos.X + 1, Y: pet.Pos.Y}, now, 1)
	w.attack(r, &r.Player, pet, now)
	// out 保存由来源被推动位移取得的输出。
	if out := w.sourcePushed(r, reference(r, pet), 2, 1, now, displacementSource{}); out.Stop != pushStale || !reflect.DeepEqual(before, *pet) {
		t.Fatal("当前图AI/攻击/推挤越界写另一World宠物", out)
	}
}

// TestLegacyPetCorpseRemainsVisibleOnlyInResidentWorld 核对「尸体被销毁或跨World显示」与「尸体因显示而复活」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestLegacyPetCorpseRemainsVisibleOnlyInResidentWorld(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，c 接收场景世界房间执行器供后续步骤使用。
	// personal 保存由场景世界房间执行器取得的个人，town 保存由场景世界房间执行器取得的城镇。
	w, r, c, personal, town, _ := worldWorker(t)
	legacyPetRule15(w, r)
	r.PetWorld = personal.Ref
	r.Pet.HP, r.Pet.Action = 0, 4
	// world 接收遍历场景世界状态列表时当前的元素。
	for _, world := range []*worldState{personal, town, personal} {
		r.worldState = world
		c.frames = nil
		w.baseline(r)
		// count 规定数量的初始取值。
		count := 0
		// frame 接收遍历协议帧集合时当前的元素。
		for _, frame := range c.frames {
			if frame.ID != p.MsgS2CSceneBaseline {
				continue
			}
			// baseline 记录场景基线，使用场景场景基线分片表示。
			var baseline pb.SceneBaselineChunk
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := proto.Unmarshal(frame.Payload, &baseline); err != nil {
				t.Fatal(err)
			}
			// e 接收遍历实体时当前的元素。
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

// TestPetTownNavigationUsesResidentOccupancyAndReturnsRetiredTask 核对「另一World宠物占用当前Town工作槽或动态格」与「退役后旧宠仍占导航任务/工作槽」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetTownNavigationUsesResidentOccupancyAndReturnsRetiredTask(t *testing.T) {
	// w 接收导航城镇房间执行器供后续步骤使用，r 接收导航城镇房间执行器供后续步骤使用。
	w, r := navigationTownWorker(t, 8, 8, 32, 32, 1, 2)
	// town 保存由当前状态场景世界取得的城镇。
	town := r.currentWorld()
	r.TownWorld = town
	r.PersonalWorld = r.newWorld(1)
	// pet 保存当前处理所用的宠物。
	pet := &entity{Handle: petHandle, Kind: 2, Pos: devconfig.Point{X: 2, Y: 1}, HP: 10, MaxHP: 10}
	if !w.installPet(r, r.PersonalWorld, pet) {
		t.Fatal("独立World安装失败")
	}
	// target 初始化用作目标的点位。
	target := devconfig.Point{X: 6, Y: 1}
	requestTownGround(r, target)
	w.moveTown(r, &r.Player, target, time.Unix(1, 0), 1)
	// q 保存从城镇导航取出的城镇导航中的指定项。
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
	// ok 表明查询、校验或类型转换是否成功。
	if _, ok := w.retirePet(r, reference(r, pet)); !ok || r.TownNavigation[1] != nil {
		t.Fatal("退役后旧宠仍占导航任务/工作槽")
	}
	if !w.updateTownOccupancy(q) || len(q.dynamic.Blocked) != 0 {
		t.Fatal("退役未清除当前Town动态占格")
	}
}

// TestPetMapTravelReturnNeverAppliesPreviousBirthPosition 核对「新birth继承旧宠位置或覆盖实际resident」与「fixture缺少两个合法独立宠物点」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetMapTravelReturnNeverAppliesPreviousBirthPosition(t *testing.T) {
	// resident 接收遍历文本列表时当前的元素。
	for _, resident := range []string{"town", "personal"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(resident, func(t *testing.T) {
			// w 接收地图切换房间执行器供后续步骤使用，r 接收地图切换房间执行器供后续步骤使用，now 保存由地图切换房间执行器取得的当前时间。
			w, r, _, now := travelWorker(t)
			// personal 保存由当前状态场景世界取得的个人。
			personal := r.currentWorld()
			r.PetWorld = personal.Ref
			// m 接收场景世界地图供后续步骤使用。
			m, _ := w.worldMap(personal)
			// points 初始化用作点位集合的点位列表。
			points := []devconfig.Point{}
			// y 规定纵坐标的初始取值。
			for y := 0; y < m.Height && len(points) < 2; y++ {
				// x 规定横坐标的初始取值。
				for x := 0; x < m.Width && len(points) < 2; x++ {
					// point 组装点位所需的数据。
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
			// old 保存宠物，用于当前步骤中的原状态。
			old := r.Pet
			w.applyDamageInWorld(r, r.currentWorld(), &r.Player, old, old.HP, now)
			if r.Pet != nil || !old.IsPetCorpse {
				t.Fatal("旧宠未按当前规则成为World尸体")
			}
			// world 保存由当前状态场景世界取得的场景世界，position 保存当前处理所用的位置。
			world, position := r.currentWorld(), old.Pos
			// expected 保存由地图切换宠物点位取得的预期结果，ok 表明查询、校验或类型转换是否成功。
			expected, ok := travelPetPoint(m, personal.PlayerPosition, nil, personal.Monsters, personal.MonsterCount)
			if !ok {
				t.Fatal("fixture返回宠物无受控落点")
			}
			if resident == "personal" {
				world, position, expected = personal, points[1], points[1]
			}
			r.Role.Job = w.e.Store.Cfg.Fixture.Pet.JobID
			// definition 保存由定义“by”消息正文标识取得的定义。
			definition, _ := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByBodyID(old.Def)
			// pet 保存由创建宠物取得的宠物，made 保存由创建宠物取得的“made”。
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

// TestPetTownArrivalKeepsDestinationResidentPosition 核对「进Town时覆盖了目标图真实Pet位置」与「fixture缺少独立Town落点」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPetTownArrivalKeepsDestinationResidentPosition(t *testing.T) {
	// w 接收地图切换房间执行器供后续步骤使用，r 接收地图切换房间执行器供后续步骤使用，now 保存由地图切换房间执行器取得的当前时间。
	w, r, _, now := travelWorker(t)
	r.PersonalWorld = r.currentWorld()
	r.PetWorld = r.PersonalWorld.Ref
	// old 保存宠物，用于当前步骤中的原状态。
	old := r.Pet
	w.applyDamageInWorld(r, r.PersonalWorld, &r.Player, old, old.HP, now)
	if r.Pet != nil || !old.IsPetCorpse {
		t.Fatal("当前规则死亡归档失败")
	}
	r.TownWorld = r.newWorld(2)
	// m 接收场景世界地图供后续步骤使用。
	m, _ := w.worldMap(r.TownWorld)
	// position 记录位置，使用点位表示。
	var position *devconfig.Point
	// y 规定纵坐标的初始取值。
	for y := 0; y < m.Height && position == nil; y++ {
		// x 规定横坐标的初始取值。
		for x := 0; x < m.Width; x++ {
			// point 组装点位所需的数据。
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
	// definition 保存由定义“by”消息正文标识取得的定义。
	definition, _ := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByBodyID(old.Def)
	// pet 保存由创建宠物取得的宠物，made 保存由创建宠物取得的“made”。
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
