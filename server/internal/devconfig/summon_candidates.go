// 本文件区分已接入的宠物召唤和保留来源的候选配置，校验候选字段及命名。

package devconfig

import (
	"encoding/json"
	"errors"
	"os"
)

// 独立候选提供同源完整数据，加载它不修改运行profile或开启任何技能/材料渠道。
type SummonCandidateCatalog struct {
	// SchemaVersion 记录“schema”版本，使用数值表示。
	SchemaVersion int `json:"schema_version"`
	// DevelopmentOnly 标记是否处于“development”“only”状态。
	DevelopmentOnly bool `json:"development_only"`
	// RuntimeEnabled 标记是否处于运行时已开启状态。
	RuntimeEnabled bool `json:"runtime_enabled"`
	// RequiredRuleVersion 记录“required”规则版本，使用数值表示。
	RequiredRuleVersion uint64 `json:"required_rule_version"`
	// Skills 记录技能集合，使用技能列表表示。
	Skills []Skill `json:"skills"`
	// Books 记录技能书集合，使用道具列表表示。
	Books []Item `json:"books"`
	// Upgrades 记录“upgrades”，使用技能升级列表表示。
	Upgrades []SkillUpgrade `json:"upgrades"`
	// PetDefinitionIDs 记录宠物定义i“ds”，使用数值列表表示。
	PetDefinitionIDs []uint32 `json:"pet_definition_ids"`
	// SourceEvidence 记录来源“evidence”，使用按文本索引的原始值消息映射表示。
	SourceEvidence map[string]json.RawMessage `json:"source_evidence"`
}

// LoadSummonCandidates 加载“summon”“candidates”。
// path 记录路径，使用文本表示。
func LoadSummonCandidates(path string) (*SummonCandidateCatalog, error) {
	// data 保存由读取文件取得的数据，err 接收读取文件过程中可能产生的错误，供后续失败分支处理。
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// c 记录召唤候选目录。
	var c SummonCandidateCatalog
	// err 接收解码宠物规则合同过程中可能产生的错误，供后续失败分支处理。
	if err := decodePetContract(data, &c); err != nil {
		return nil, err
	}
	if c.SchemaVersion != 1 || !c.DevelopmentOnly || c.RuntimeEnabled || c.RequiredRuleVersion != 17 || len(c.SourceEvidence) == 0 ||
		len(c.Skills) != 1 || len(c.Books) != 1 || len(c.PetDefinitionIDs) != 1 || len(c.Upgrades) != 3 {
		return nil, errors.New("独立召唤候选须完整、关闭运行且明确rule17")
	}
	// s 保存从技能集合取出的技能集合中的指定项，book 保存从技能书集合取出的技能书。
	s, book := c.Skills[0], c.Books[0]
	if s.ID != SummonSkeletonSkillID {
		return nil, errors.New("独立rule17召唤候选固定为召唤骷髅")
	}
	// err 接收校验召唤技能过程中可能产生的错误，供后续失败分支处理。
	if err := validateSummonSkill(s); err != nil {
		return nil, err
	}
	if book.ID != s.BookItemID || book.BookSkillID != s.ID || book.Name == "" || book.IsEquipment || book.IsSkillPage || book.Stackable || book.MaxStack != 1 ||
		book.EquipmentSlot != 0 || len(book.AllowedJobs) != 1 || book.AllowedJobs[0] != s.JobID || c.PetDefinitionIDs[0] != s.Summon.DefinitionID {
		return nil, errors.New("召唤候选的原书、职业及宠物模板引用不一致")
	}
	// value 接收遍历数值列表时当前的元素。
	for _, value := range []int64{book.AttackBonus, book.AttackMinBonus, book.AttackMaxBonus, book.AttackSpeedBonus, book.DefenceBonus, book.RecycleGold, book.DefenceMinBonus, book.DefenceMaxBonus, book.MagicDefenceMinBonus, book.MagicDefenceMaxBonus, book.MagicMinBonus, book.MagicMaxBonus, book.TaoMinBonus, book.TaoMaxBonus, int64(book.EquipmentStage), book.OriginalDurabilityU, book.RepairBasePrice} {
		if value != 0 {
			return nil, errors.New("召唤原书不能夹带装备属性或未实现的回收收入")
		}
	}
	// seen 建立带初始值的按数值索引的布尔状态映射，供本段继续处理。
	seen := map[uint32]bool{}
	// u 接收遍历“upgrades”时当前的元素。
	for _, u := range c.Upgrades {
		if u.SkillID != s.ID || u.Level <= s.LearnedLevel || u.Level > s.MaxLevel || seen[u.Level] || u.RequiredRoleLevel < s.LearnLevel ||
			u.PageItemID == 0 || u.PageCost < 0 || len(u.BookCosts) != 1 || u.BookCosts[0].ItemID != book.ID || u.BookCosts[0].Quantity != int64(u.Level)+1 {
			return nil, errors.New("召唤候选升级必须逐阶完整并只引用同一本原书")
		}
		seen[u.Level] = true
	}
	return &c, nil
}
