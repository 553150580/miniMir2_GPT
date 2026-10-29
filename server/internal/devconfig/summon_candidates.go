package devconfig

import (
	"encoding/json"
	"errors"
	"os"
)

// 独立候选提供同源完整数据，加载它不修改运行profile或开启任何技能/材料渠道。
type SummonCandidateCatalog struct {
	SchemaVersion       int                        `json:"schema_version"`
	DevelopmentOnly     bool                       `json:"development_only"`
	RuntimeEnabled      bool                       `json:"runtime_enabled"`
	RequiredRuleVersion uint64                     `json:"required_rule_version"`
	Skills              []Skill                    `json:"skills"`
	Books               []Item                     `json:"books"`
	Upgrades            []SkillUpgrade             `json:"upgrades"`
	PetDefinitionIDs    []uint32                   `json:"pet_definition_ids"`
	SourceEvidence      map[string]json.RawMessage `json:"source_evidence"`
}

func LoadSummonCandidates(path string) (*SummonCandidateCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c SummonCandidateCatalog
	if err := decodePetContract(data, &c); err != nil {
		return nil, err
	}
	if c.SchemaVersion != 1 || !c.DevelopmentOnly || c.RuntimeEnabled || c.RequiredRuleVersion != 17 || len(c.SourceEvidence) == 0 ||
		len(c.Skills) != 1 || len(c.Books) != 1 || len(c.PetDefinitionIDs) != 1 || len(c.Upgrades) != 3 {
		return nil, errors.New("独立召唤候选须完整、关闭运行且明确rule17")
	}
	s, book := c.Skills[0], c.Books[0]
	if err := validateSummonSkill(s); err != nil {
		return nil, err
	}
	if book.ID != s.BookItemID || book.BookSkillID != s.ID || book.Name == "" || book.IsEquipment || book.IsSkillPage || book.Stackable || book.MaxStack != 1 ||
		book.EquipmentSlot != 0 || len(book.AllowedJobs) != 1 || book.AllowedJobs[0] != s.JobID || c.PetDefinitionIDs[0] != s.Summon.DefinitionID {
		return nil, errors.New("召唤候选的原书、职业及宠物模板引用不一致")
	}
	for _, value := range []int64{book.AttackBonus, book.AttackMinBonus, book.AttackMaxBonus, book.AttackSpeedBonus, book.DefenceBonus, book.RecycleGold, book.DefenceMinBonus, book.DefenceMaxBonus, book.MagicDefenceMinBonus, book.MagicDefenceMaxBonus} {
		if value != 0 {
			return nil, errors.New("召唤原书不能夹带装备属性或未实现的回收收入")
		}
	}
	seen := map[uint32]bool{}
	for _, u := range c.Upgrades {
		if u.SkillID != s.ID || u.Level <= s.LearnedLevel || u.Level > s.MaxLevel || seen[u.Level] || u.RequiredRoleLevel < s.LearnLevel ||
			u.PageItemID == 0 || u.PageCost < 0 || len(u.BookCosts) != 1 || u.BookCosts[0].ItemID != book.ID || u.BookCosts[0].Quantity != int64(u.Level)+1 {
			return nil, errors.New("召唤候选升级必须逐阶完整并只引用同一本原书")
		}
		seen[u.Level] = true
	}
	return &c, nil
}
