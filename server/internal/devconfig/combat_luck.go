// 本文件定义幸运战斗候选，核对原分档、掉落初始幸运及抵抗目标。
package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

const maxLuckPPM = 1_000_000 // 幸运概率上限，使用整数百万分率。

// CombatLuckConfig 保存已经核对工作簿来源的幸运参数。整个字段缺省时不开放幸运战斗和掉落初始幸运。
type CombatLuckConfig struct {
	WeaponWingPPM        []uint32          `json:"weapon_wing_ppm"`                  // 武器和翅膀的幸运 0 至 9 档贡献。
	OtherPPM             []uint32          `json:"other_ppm"`                        // 其他九个装备位的幸运 0 至 3 档贡献。
	MonsterResistancePPM map[uint32]uint32 `json:"monster_resistance_ppm,omitempty"` // 按原普通或挑战怪物模板配置的抵抗。
	RoleResistancePPM    map[uint32]uint32 `json:"role_resistance_ppm,omitempty"`    // 按实际角色职业配置的抵抗。
	DropPPM              uint32            `json:"drop_ppm"`                         // 已确定装备掉落后再判定初始幸运的概率。
	InitialWeights       []uint32          `json:"initial_weights"`                  // 初始幸运 1、2、3 档的条件权重。
}

// UnmarshalJSON 拒绝拼写错误的幸运配置键。
func (c *CombatLuckConfig) UnmarshalJSON(data []byte) error {
	type fields CombatLuckConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*fields)(c))
}

// validateCombatLuck 拒绝直接向基础战斗模板塞入运行态计算结果。
func (f *Fixture) validateCombatLuck() error {
	if err := f.validateLuckProjectionFields(); err != nil {
		return err
	}
	luck := f.CombatLuck
	if luck == nil {
		return nil
	}
	if !validLuckContribution(luck.WeaponWingPPM, 10) || !validLuckContribution(luck.OtherPPM, 4) {
		return errors.New("幸运贡献须包含武器翅膀 0 至 9 档和其他装备 0 至 3 档")
	}
	if luck.DropPPM > maxLuckPPM || len(luck.InitialWeights) != 3 {
		return errors.New("装备掉落初始幸运须配置概率以及三个条件权重")
	}
	var sum uint64
	for _, weight := range luck.InitialWeights {
		if weight == 0 {
			return errors.New("初始幸运的每档权重须大于零")
		}
		sum += uint64(weight)
	}
	if sum > math.MaxUint32 {
		return errors.New("初始幸运总权重超出抽样范围")
	}
	for id, resistance := range luck.MonsterResistancePPM {
		_, ordinary := f.MonsterByID(id)
		_, challenge := f.ChallengeMonster(id)
		if id == 0 || resistance > maxLuckPPM || !ordinary && !challenge {
			return fmt.Errorf("怪物幸运一击抵抗须引用已知模板并在百万分率范围内：%d", id)
		}
	}
	for job, resistance := range luck.RoleResistancePPM {
		if _, known := f.Player(job); !known || job > 2 || resistance > maxLuckPPM {
			return fmt.Errorf("角色幸运一击抵抗须引用已知职业并在百万分率范围内：%d", job)
		}
	}
	return nil
}

// validLuckContribution 校验档位完整、零级无贡献且各级贡献单调不降。
func validLuckContribution(values []uint32, count int) bool {
	if len(values) != count || values[0] != 0 {
		return false
	}
	for i, value := range values[1:] {
		if value < values[i] || value > maxLuckPPM {
			return false
		}
	}
	return true
}

// validateLuckProjectionFields 保证角色、怪物和宠物的基础模板仅使用统一配置入口。
func (f *Fixture) validateLuckProjectionFields() error {
	hasProjection := func(fighter Fighter) bool { return fighter.LuckAttackPPM != 0 || fighter.LuckResistancePPM != 0 }
	for _, fighter := range f.Players {
		if hasProjection(fighter) {
			return errors.New("角色基础模板不能直接配置幸运战斗投影值")
		}
	}
	for _, roster := range [][]Monster{f.Monsters, f.ChallengeMonsters} {
		for _, monster := range roster {
			if hasProjection(monster.Fighter) {
				return errors.New("怪物模板的幸运抵抗必须写入统一抵抗目录")
			}
		}
	}
	if hasProjection(f.Pet) {
		return errors.New("宠物基础模板不能配置角色幸运贡献")
	}
	if f.PetRuntime != nil {
		for _, pet := range f.PetRuntime.Definitions {
			if hasProjection(pet.Base) {
				return errors.New("宠物定义不能配置角色幸运贡献")
			}
		}
	}
	return nil
}
