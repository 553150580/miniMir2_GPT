// 本文件校验转生档位、费用和个人首领凭证供应。
package devconfig

import (
	"errors"
	"fmt"
)

// RebirthConfig 按角色已转次数依次定义下一档。
type RebirthConfig struct {
	Stages []RebirthStage `json:"stages"` // 第一个档位从50级起算。
}

// RebirthStage 记录本档实际等级门槛、对应挑战和费用。
type RebirthStage struct {
	RequiredLevel       uint32 `json:"required_level"`        // 成功转生前必须达到的等级上限。
	ChallengeTemplateID uint32 `json:"challenge_template_id"` // 产生对应凭证的个人首领。
	CredentialItemID    uint32 `json:"credential_item_id"`    // 本档消耗一份角色绑定凭证。
	MaterialItemID      uint32 `json:"material_item_id"`      // 本档扣除的绑定材料目录号。
	MaterialCost        int64  `json:"material_cost"`         // 可跨背包多个原堆扣除的数量。
	GoldCost            int64  `json:"gold_cost"`             // 优先使用绑定金币的总费用。
}

// validateRebirth 确认每次只开一级，且每档凭证和绑料均有真实挑战来源。
func (f *Fixture) validateRebirth() error {
	if f.Rebirth == nil {
		return nil
	}
	stages := f.Rebirth.Stages
	if f.RuleVersion < 30 || len(stages) == 0 {
		return errors.New("转生要求规则30及连续档位")
	}
	credentials := make(map[uint32]bool, len(stages)) // 独立凭证禁止提前用上档余量跨关。
	for i, stage := range stages {
		credential, found := f.Item(stage.CredentialItemID)
		if stage.RequiredLevel != f.Progression.InitialLevelCap+uint32(i) || credentials[stage.CredentialItemID] ||
			!found || credential.IsEquipment || !credential.Stackable || credential.MaxStack != 1 || stage.GoldCost <= 0 {
			return fmt.Errorf("转生第%d档的等级、凭证或金币条件无效", i+1)
		}
		credentials[stage.CredentialItemID] = true
	}
	for i, stage := range stages {
		material, found := f.Item(stage.MaterialItemID)
		boss := f.PersonalBossByTemplate(stage.ChallengeTemplateID)
		if !found || material.IsEquipment || !material.Stackable || material.MaxStack <= 0 ||
			credentials[stage.MaterialItemID] || stage.MaterialCost <= 0 || boss == nil || boss.ItemReward == nil ||
			boss.MinimumLevel > stage.RequiredLevel {
			return fmt.Errorf("转生第%d档缺少可获取的绑定材料或挑战", i+1)
		}
		var proof, supply bool // 同一批真实胜利附件含凭证与绑定材料。
		for _, reward := range boss.ItemReward.BoundItems {
			proof = proof || reward.ItemID == stage.CredentialItemID && reward.Quantity == 1
			supply = supply || reward.ItemID == stage.MaterialItemID
		}
		if !proof || !supply {
			return fmt.Errorf("转生第%d档的原个人首领缺少固定绑定奖励", i+1)
		}
	}
	return nil
}
