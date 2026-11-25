// 本文件保存镜像竞技按原角色版本冻结的战斗投影。永久库存、装备实例及当前现场状态不进入镜像。
package domain

import "minimir/internal/devconfig"

// MirrorCombatSnapshot 为同一职业战斗规则提供不可变输入。
type MirrorCombatSnapshot struct {
	Job          uint32               `bson:"job" json:"job"`                                           // 原角色职业。
	Gender       uint32               `bson:"gender" json:"gender"`                                     // 原角色外观性别。
	Level        uint32               `bson:"level" json:"level"`                                       // 原角色已确认等级。
	Name         string               `bson:"name" json:"name"`                                         // 原角色显示名。
	Fighter      devconfig.Fighter    `bson:"fighter" json:"fighter"`                                   // 已折算全部有效装备和被动技能的战斗属性。
	WeaponItemID uint32               `bson:"weapon_item_id,omitempty" json:"weapon_item_id,omitempty"` // 原武器模板，仅用于表现。
	Skills       []LearnedSkill       `bson:"skills,omitempty" json:"skills,omitempty"`                 // 原角色已习得技能与实际等级。
	SkillSlotIDs []uint32             `bson:"skill_slot_ids,omitempty" json:"skill_slot_ids,omitempty"` // 原角色确认的可排队技能。
	Pet          *MirrorPetSnapshot   `bson:"pet,omitempty" json:"pet,omitempty"`                       // 入场时唯一合格职业宠物的投影，可空。
	Potions      []MirrorCombatSupply `bson:"potions,omitempty" json:"potions,omitempty"`               // 同模板合并的有限临时药剂。
	Material     *MirrorCombatSupply  `bson:"material,omitempty" json:"material,omitempty"`             // 原角色已装入的施法耗材，可空。
}

// MirrorPetSnapshot 只冻结本次可以参战的宠物。临时复活或换宠不改变超时分母。
type MirrorPetSnapshot struct {
	DefinitionID uint32            `bson:"definition_id" json:"definition_id"` // 原宠物身体模板，供表现及技能选择。
	Fighter      devconfig.Fighter `bson:"fighter" json:"fighter"`             // 宠物已有训练阶段的有效战斗属性。
	Level        uint32            `bson:"level" json:"level"`                 // 入场展示的原宠物等级。
}

// MirrorCombatSupply 保留模板和数量，不携永久物品编号、钱包或其它资产。
type MirrorCombatSupply struct {
	ItemID   uint32 `bson:"item_id" json:"item_id"`   // 锁定规则中的药剂或施法耗材模板。
	Quantity int64  `bson:"quantity" json:"quantity"` // 入场后仅限本场消耗的原件合计数量。
}
