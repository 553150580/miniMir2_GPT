// 本文件从规则 30 真实目录组装镜像对照阵容，沿正式入场函数冻结属性。
package game

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// mirrorBalanceRosterEntry 保留每组输入的稳定身份和实际参战快照。
type mirrorBalanceRosterEntry struct {
	ID       string                      // 用职业、角色等级、装备组和补给方案标记阵容。
	Job      uint32                      // 原职业编号。
	Level    uint32                      // 角色实际等级。
	Gear     string                      // 三套已开放的固定装备组合。
	Profile  string                      // 记录原上阵宠物以及是否携带消耗品。
	ItemIDs  []uint32                    // 实际穿戴的规则 30 装备编号，施法耗材另见快照。
	Snapshot domain.MirrorCombatSnapshot // 由生产入场路径冻结的战斗数据。
}

// newMirrorBalanceWorker 只加载真实主规则，不改原属性、战斗公式或配置表。
func newMirrorBalanceWorker(t *testing.T) *worker {
	t.Helper()
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixture.RuleVersion != 30 || cfg.Fixture.PetRuntime == nil || !cfg.Fixture.PetRuntime.Enabled || !cfg.Fixture.CastingSupply.Enabled {
		t.Fatal("镜像对照需要当前已发布的规则 30 职业、宠物与装备目录")
	}
	return &worker{e: &Engine{Store: &persistence.Store{Cfg: cfg}}}
}

// mirrorBalanceSnapshot 构造合法穿戴、已学技能和可选原宠，随后沿实际挑战逻辑冻结。
func mirrorBalanceSnapshot(t *testing.T, w *worker, job, level uint32, gear, profile string) mirrorBalanceRosterEntry {
	t.Helper()
	f := &w.e.Store.Cfg.Fixture
	if _, ok := f.Player(job); !ok || level == 0 || level > f.Progression.OperatingLevelCap {
		t.Fatalf("原规则没有职业 %d 的等级 %d 样本", job, level)
	}
	itemIDs := mirrorBalanceGear(t, gear)
	build := mirrorBalanceRosterEntry{ID: fmt.Sprintf("%d-%d-%s-%s", job, level, gear, profile),
		Job: job, Level: level, Gear: gear, Profile: profile, ItemIDs: itemIDs}
	ref := worldRef{Epoch: 1, ID: 1}
	r := &room{worldState: &worldState{Ref: ref, MapID: f.Map.ID}, ExecutionEpoch: ref.Epoch, WorldSequence: ref.ID,
		Role: domain.Role{Name: build.ID, Job: job, Level: level, Capacity: f.InventoryCapacity, SkillSlotIDs: make([]uint32, devconfig.AttackSkillSlotCount)}}
	if job == 1 {
		r.Role.Gender = 1 // 规则 30 法师为女，战士和道士为男。
	}
	for _, id := range itemIDs {
		def, ok := f.Item(id)
		if !ok || !def.IsEquipment || def.RequiredLevel == nil || level < *def.RequiredLevel || !mirrorBalanceItemForJob(def, job) ||
			!def.EquipmentStatsKnown() || !def.EquipmentRecycleKnown() || !mirrorBalanceShopSells(f, id, level) {
			t.Fatalf("装备 %d 对职业 %d 等级 %d 未开放、不可穿戴或原属性待定", id, job, level)
		}
		if slices.ContainsFunc(r.Role.Equipped, func(old domain.Equipped) bool { return old.Slot == def.EquipmentSlot }) {
			t.Fatalf("装备组 %s 重复占用部位 %d", gear, def.EquipmentSlot)
		}
		// 使用新购得的原始耐久和品质，装备增益由冻结逻辑统一折算。
		durability := def.OriginalDurabilityU
		item := domain.Item{ID: fmt.Sprintf("%032x", id), ItemID: id, Quantity: 1, Version: 1, Quality: def.DropQuality,
			DurabilityU: durability, MaxDurabilityU: durability, OriginalDurabilityU: durability}
		r.Role.Equipped = append(r.Role.Equipped, domain.Equipped{Slot: def.EquipmentSlot, Item: item})
	}
	for _, s := range f.Skills {
		if s.JobID == job && level >= s.LearnLevel && mirrorBalanceBookAvailable(f, s.BookItemID, level) {
			// 统一采用技能学会时的原始等级，以免技能升阶与角色等级差混在一起。
			r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: s.ID, Level: s.LearnedLevel})
		}
	}
	summonID, entryPet := mirrorBalancePetProfile(t, profile)
	if summonID != 0 {
		if skill, exists := f.Skill(summonID); !exists || !skill.AttackSlotSummon() {
			t.Fatalf("规则 30 未装配样本的召唤技能 %d", summonID)
		}
		if _, learned := r.Role.Skill(summonID); !learned {
			t.Fatalf("职业 %d 等级 %d 尚不可学习样本的召唤技能 %d", job, level, summonID)
		}
	}
	mirrorBalanceLoadout(t, f, &r.Role, summonID, profile == "dry")
	if entryPet {
		skill, _ := f.Skill(summonID)
		pet, ok := w.newPet(r, skill.Summon.DefinitionID, skill.LearnedLevel, f.Map.PetSpawn, time.Unix(1_800_000_000, 0))
		if !ok || !f.Map.Walkable(f.Map.PetSpawn) || !w.installPet(r, r.currentWorld(), pet) {
			t.Fatalf("样本无法按原技能和等级装配宠物 %d", summonID)
		}
	}
	build.Snapshot = w.freezeMirrorCombatSnapshot(r, &f.Map)
	if r.Isolated || !validMirrorFighter(build.Snapshot.Fighter) || (build.Snapshot.Pet != nil) != entryPet ||
		build.Snapshot.WeaponItemID != itemIDs[0] {
		t.Fatalf("样本没有得到实际入场战斗快照：%+v", build)
	}
	return build
}

// mirrorBalanceGear 固定对照组，只包含当前商店可取得的装备。
func mirrorBalanceGear(t *testing.T, gear string) []uint32 {
	t.Helper()
	switch gear {
	case "early":
		return []uint32{218, 193}
	case "middle":
		return []uint32{223, 193, 259}
	case "late":
		return []uint32{225, 195, 261}
	default:
		t.Fatalf("未知规则 30 对照装备组 %q", gear)
		return nil
	}
}

// mirrorBalanceShopSells 只承认当级可通过金币买到的 NPC 原商品。
func mirrorBalanceShopSells(f *devconfig.Fixture, id, level uint32) bool {
	for _, shop := range f.NPCShops {
		if slices.ContainsFunc(shop.Offers, func(offer devconfig.SkillShop) bool {
			return offer.ItemID == id && offer.Currency == "gold" && offer.Quantity > 0 && offer.RequiredRoleLevel <= level
		}) {
			return true
		}
	}
	return false
}

// mirrorBalanceBookAvailable 核对独立的正式技能书购买表。
func mirrorBalanceBookAvailable(f *devconfig.Fixture, id, level uint32) bool {
	return slices.ContainsFunc(f.SkillShop, func(offer devconfig.SkillShop) bool {
		return offer.ItemID == id && offer.Currency == "gold" && offer.RequiredRoleLevel <= level
	})
}

// mirrorBalanceItemForJob 按与正式穿戴一致的职业白名单确认资格。
func mirrorBalanceItemForJob(item devconfig.Item, job uint32) bool {
	return len(item.AllowedJobs) == 0 || slices.Contains(item.AllowedJobs, job)
}

// mirrorBalancePetProfile 控制召唤是否进入槽位，以及受测角色入场时是否已经有活宠。
func mirrorBalancePetProfile(t *testing.T, profile string) (skill uint32, entryPet bool) {
	t.Helper()
	switch profile {
	case "standard", "dry":
		return 0, false
	case "skeleton", "empty_skeleton":
		return devconfig.SummonSkeletonSkillID, profile == "skeleton"
	case "shinsu", "empty_shinsu":
		return devconfig.SummonShinsuSkillID, profile == "shinsu"
	default:
		t.Fatalf("未知规则 30 参战样本 %q", profile)
		return 0, false
	}
}

// mirrorBalanceLoadout 选择固定的四槽顺序。消耗品只来自初始背包和原商店专槽白名单。
func mirrorBalanceLoadout(t *testing.T, f *devconfig.Fixture, role *domain.Role, summonID uint32, dry bool) {
	t.Helper()
	var priority []uint32
	var materialID uint32
	switch role.Job {
	case 0:
		priority = []uint32{26, 12, 25}
	case 1:
		priority = []uint32{33, 31, 10, 23, 11, 5, 9, 1}
	case 2:
		priority = []uint32{13, 2, 14, 15}
		power, _ := f.Skill(13)
		if _, learned := role.Skill(power.ID); learned {
			materialID = power.MaterialItemID
		} else if _, learned := role.Skill(6); learned {
			poison, _ := f.Skill(6)
			if poison.Poison == nil || len(poison.Poison.GreenItemIDs) == 0 {
				t.Fatal("原施毒术缺少已装配的灰色药粉")
			}
			priority = []uint32{6, 2}
			materialID = poison.Poison.GreenItemIDs[0]
		}
	}
	if summonID != 0 {
		skill, _ := f.Skill(summonID)
		priority = append([]uint32{summonID}, priority...)
		materialID = skill.MaterialItemID
	}
	var count int
	for _, id := range priority {
		skill, exists := f.Skill(id)
		_, learned := role.Skill(id)
		if !exists || !learned || !skill.AttackSlotEligible() || skill.MaterialQuantity > 0 && !skill.AcceptsMaterial(materialID) {
			continue
		}
		role.SkillSlotIDs[count] = id
		count++
		if count == len(role.SkillSlotIDs) {
			break
		}
	}
	if dry {
		return // 保留相同技能槽，只移除补给，用来量化资源敏感性。
	}
	for index, id := range f.InitialInventory {
		if _, exists := f.Potions.Effect(id); exists {
			role.Items = append(role.Items, domain.Item{ID: fmt.Sprintf("%032x", index+1), ItemID: id, Quantity: 1, Version: 1})
		}
	}
	if materialID != 0 {
		item, available := f.Item(materialID)
		if !available || !f.IsCastingSupply(materialID) || !item.Stackable || item.MaxStack <= 0 || !mirrorBalanceItemForJob(item, role.Job) ||
			!mirrorBalanceShopSells(f, materialID, role.Level) {
			t.Fatalf("当前道士样本不能安装原施法材料 %d", materialID)
		}
		// 满堆只是对照初态，容量取规则原上限，单次用量仍由真实技能决定。
		role.Equipped = append(role.Equipped, domain.Equipped{Slot: devconfig.CastingSupplySlot,
			Item: domain.Item{ID: fmt.Sprintf("%032x", materialID), ItemID: materialID, Quantity: item.MaxStack, Version: 1}})
	}
}

// TestMirrorBalanceRosterUsesReleaseGearSkillsAndPets 核对主矩阵和等级差两端所需的合法样本。
func TestMirrorBalanceRosterUsesReleaseGearSkillsAndPets(t *testing.T) {
	w := newMirrorBalanceWorker(t)
	f := &w.e.Store.Cfg.Fixture
	for job := uint32(0); job <= 2; job++ {
		for _, level := range []uint32{15, 18, 19, 22, 35, 38} {
			for _, gear := range []string{"early", "middle", "late"} {
				standard := mirrorBalanceSnapshot(t, w, job, level, gear, "standard").Snapshot
				dry := mirrorBalanceSnapshot(t, w, job, level, gear, "dry").Snapshot
				if standard.Fighter != dry.Fighter || !slices.Equal(standard.SkillSlotIDs, dry.SkillSlotIDs) || len(standard.Potions) != 2 ||
					dry.Material != nil || len(dry.Potions) != 0 || standard.Pet != nil || len(standard.SkillSlotIDs) != devconfig.AttackSkillSlotCount {
					t.Fatalf("职业 %d 等级 %d 装备 %s 的标准组和无补给组混入额外变量", job, level, gear)
				}
				for _, learned := range standard.Skills {
					skill, ok := f.Skill(learned.SkillID)
					if !ok || skill.JobID != job || level < skill.LearnLevel || learned.Level != skill.LearnedLevel {
						t.Fatalf("职业 %d 等级 %d 选入了不合规技能 %+v", job, level, learned)
					}
				}
			}
		}
	}
	for _, p := range []struct {
		level   uint32 // 当前原技能的初次学习等级。
		skillID uint32 // 该组宠物对应的原召唤技能。
		name    string // 入场有宠时使用的样本名。
	}{
		{19, devconfig.SummonSkeletonSkillID, "skeleton"},
		{35, devconfig.SummonShinsuSkillID, "shinsu"},
	} {
		entry := mirrorBalanceSnapshot(t, w, 2, p.level, "middle", p.name).Snapshot
		empty := mirrorBalanceSnapshot(t, w, 2, p.level, "middle", "empty_"+p.name).Snapshot
		skill, _ := f.Skill(p.skillID)
		definition, _ := f.PetRuntime.DefinitionByID(skill.Summon.DefinitionID)
		if entry.Pet == nil || entry.Pet.DefinitionID != definition.Base.DefinitionID || empty.Pet != nil ||
			entry.SkillSlotIDs[0] != p.skillID || !slices.Equal(entry.SkillSlotIDs, empty.SkillSlotIDs) ||
			entry.Material == nil || empty.Material == nil || *entry.Material != *empty.Material || !skill.AcceptsMaterial(entry.Material.ItemID) {
			t.Fatal("有宠和空槽首召的对应样本错误", p.name)
		}
	}
}
