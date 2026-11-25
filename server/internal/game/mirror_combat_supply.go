// 本文件从原角色的合法资产视图提取镜像投影，并将临时资源限定在镜像战斗内。
package game

import (
	"math"
	"slices"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// freezeMirrorCombatSnapshot 取当前确认等级、装备、技能及宠物，不复制原资产实例或当前血蓝。
func (w *worker) freezeMirrorCombatSnapshot(r *room, geometry *devconfig.MapTemplate) domain.MirrorCombatSnapshot {
	base := w.freezeBossCombatSnapshot(r, geometry)
	snapshot := domain.MirrorCombatSnapshot{Job: r.Role.Job, Gender: r.Role.Gender, Level: r.Role.Level, Name: r.Role.Name,
		Fighter: base.Fighter, Skills: base.Skills, SkillSlotIDs: base.SkillSlotIDs}
	fixture := &w.e.Store.Cfg.Fixture
	for _, worn := range r.Role.Equipped {
		item := worn.Item
		switch worn.Slot {
		case devconfig.WeaponSlot:
			if fixture.EquipmentDurability == nil || item.OriginalDurabilityU == 0 || item.DurabilityU > r.Wear.loss(item, worn.Slot) {
				snapshot.WeaponItemID = item.ItemID
			}
		case devconfig.CastingSupplySlot:
			if fixture.CastingSupply.Enabled && fixture.IsCastingSupply(item.ItemID) && item.Version > 0 && item.Version < math.MaxInt64 && item.Quantity > 0 {
				snapshot.Material = &domain.MirrorCombatSupply{ItemID: item.ItemID, Quantity: item.Quantity}
			}
		}
	}
	if base.Pet != nil {
		fighter, level := fixture.Pet, uint32(0)
		if base.Pet.PetRuntime != nil {
			fighter, level = base.Pet.PetRuntime.Fighter, base.Pet.PetRuntime.Level
		}
		snapshot.Pet = &domain.MirrorPetSnapshot{DefinitionID: base.Pet.Def, Fighter: fighter, Level: level}
	}
	for _, item := range r.Role.Items {
		definition, ok := fixture.Item(item.ItemID)
		if !ok || definition.IsEquipment || len(definition.AllowedJobs) != 0 && !slices.Contains(definition.AllowedJobs, r.Role.Job) ||
			item.Quantity <= 0 || item.Locked || item.ReservationID != "" || item.PendingResultID != "" {
			continue
		}
		if _, ok := fixture.Potions.Effect(item.ItemID); !ok {
			continue
		}
		index := slices.IndexFunc(snapshot.Potions, func(supply domain.MirrorCombatSupply) bool { return supply.ItemID == item.ItemID })
		if index < 0 {
			snapshot.Potions = append(snapshot.Potions, domain.MirrorCombatSupply{ItemID: item.ItemID, Quantity: item.Quantity})
		} else if snapshot.Potions[index].Quantity <= math.MaxInt64-item.Quantity {
			snapshot.Potions[index].Quantity += item.Quantity
		}
	}
	return snapshot
}

// mirrorPotion 尽量按原自动补蓝缺口选药。生命药只在已有缺口可用完原药效时使用。
func (w *worker) mirrorPotion(r *room, need automaticManaNeed, now time.Time) bool {
	if r.Isolated || potionGate(r, r.Scene, now) != 0 {
		return false
	}
	world := r.currentWorld()
	side := &world.mirrorCombat.sides[world.mirrorSide]
	rules := w.e.Store.Cfg.Fixture.Potions
	for i := range side.potions {
		supply := &side.potions[i]
		effect, ok := rules.Effect(supply.ItemID)
		if !ok || supply.Quantity <= 0 {
			continue
		}
		mana := effect.HP == 0 && effect.MP > 0 && need.cost > r.Player.MP && r.Potions.MP == 0
		health := effect.HP > 0 && effect.MP == 0 && r.Player.MaxHP-r.Player.HP >= effect.HP && r.Potions.HP == 0
		if !mana && !health {
			continue
		}
		supply.Quantity--
		w.enqueuePotionRecovery(r, effect.HP, effect.MP, now)
		return mana // 血药和普通战斗不同资源；本帧仍可受理一次合法攻击。
	}
	return false
}

// mirrorMaterialCast 复用原技能命中、蓝耗、毒色和冷却，仅在同帧成功后扣临时副本。
func (w *worker) mirrorMaterialCast(r *room, skill devconfig.Skill, handle, direction uint32, grid *pb.GridPoint, now time.Time, need *automaticManaNeed, input uint32) bool {
	world := r.currentWorld()
	material := &world.mirrorCombat.sides[world.mirrorSide].material
	if skill.MaterialQuantity <= 0 || material.Quantity < skill.MaterialQuantity || !skill.AcceptsMaterial(material.ItemID) ||
		!w.e.Store.Cfg.Fixture.CastingSupply.Enabled || !w.e.Store.Cfg.Fixture.IsCastingSupply(material.ItemID) {
		return false
	}
	color := skill.PoisonColorForMaterial(material.ItemID)
	if skill.Behavior == "poisoning" {
		target := w.skillTarget(r, skill, handle)
		if target == nil || hasPoison(target, color) {
			return false
		}
	}
	plan, code := w.prepareCastRecordingMana(r, skill.ID, handle, direction, now, true, grid)
	if !need.ready(r, plan, code) {
		return false
	}
	plan.PoisonColor = color
	if w.acceptPreparedCast(r, plan, input, now) != 0 {
		return false
	}
	material.Quantity -= skill.MaterialQuantity
	return true
}
