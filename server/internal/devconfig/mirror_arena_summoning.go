// 本文件声明镜像场内临时召唤策略。技能冷却、蓝耗和材料仍采用对应技能的原配置。
package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
)

// MirrorArenaMaxBirthsPerSide 限制单场新的宠物身份数，属于实体预算上界。
const MirrorArenaMaxBirthsPerSide uint32 = 8

// MirrorArenaSummoningRule 对进攻和防守使用同一套场内新生规则。
type MirrorArenaSummoningRule struct {
	AllowWithoutEntryPet bool   `json:"allow_without_entry_pet"` // 允许入场没有宠物的一方在原战斗中首次召唤。
	MaxBirthsPerSide     uint32 `json:"max_births_per_side"`     // 每侧入场后最多成功出生的宠物数量。
}

// UnmarshalJSON 要求显式选择空槽策略，避免遗漏字段时被解读成产品决定。
func (r *MirrorArenaSummoningRule) UnmarshalJSON(raw []byte) error {
	var input struct {
		AllowWithoutEntryPet *bool   `json:"allow_without_entry_pet"` // 用空指针区分省略与明确关闭。
		MaxBirthsPerSide     *uint32 `json:"max_births_per_side"`     // 用空指针区分省略与明确填写零。
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return err
	}
	if input.AllowWithoutEntryPet == nil || input.MaxBirthsPerSide == nil {
		return errors.New("镜像竞技临时召唤需明确指定空槽首召与每侧出生上限")
	}
	*r = MirrorArenaSummoningRule{AllowWithoutEntryPet: *input.AllowWithoutEntryPet, MaxBirthsPerSide: *input.MaxBirthsPerSide}
	return nil
}
