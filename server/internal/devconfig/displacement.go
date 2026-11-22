// 本文件加载位移基础合同并验证移动、动作和击退计时的独立来源。

package devconfig

import (
	"encoding/json"
	"errors"
	"math"
)

// 在夹具入口记录布尔字段是否显式提供，避免嵌入Fighter的Monster解码丢失外层字段。
// f 是调用该方法的开发测试配置实例，data 携带供当前流程读取或解析的原始字节。
func (f *Fixture) UnmarshalJSON(data []byte) error {
	// err 接收校验“summoning”版本“presence”过程中可能产生的错误，供后续失败分支处理。
	if err := validateSummoningVersionPresence(data); err != nil {
		return err
	}
	// err 接收校验宠物运行时“presence”过程中可能产生的错误，供后续失败分支处理。
	if err := validatePetRuntimePresence(data); err != nil {
		return err
	}
	// plainFixture 描述“plain”开发测试配置。
	type plainFixture Fixture
	// value 记录值，使用“plain”开发测试配置表示。
	var value plainFixture
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	// shapes 记录形状集合，使用临时结构表示。
	var shapes struct {
		// Players 记录玩家集合，使用按文本索引的原始值消息映射列表表示。
		Players []map[string]json.RawMessage `json:"players"`
		// Pet 记录宠物，使用按文本索引的原始值消息映射表示。
		Pet map[string]json.RawMessage `json:"pet"`
		// Monsters 记录怪物集合，使用按文本索引的原始值消息映射列表表示。
		Monsters []map[string]json.RawMessage `json:"monsters"`
	}
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal(data, &shapes); err != nil {
		return err
	}
	*f = Fixture(value)
	// explicit 封装显式对应的后续处理，fields 记录字段集合，使用按文本索引的原始值消息映射表示。
	explicit := func(fields map[string]json.RawMessage) bool {
		// v 保存从字段集合取出的字段集合中的指定项，exists 保存从字段集合取出的存在。
		v, exists := fields["can_push"]
		return exists && string(v) != "null"
	}
	// i 标记遍历玩家集合时访问的下标。
	for i := range f.Players {
		f.Players[i].canPushExplicit = explicit(shapes.Players[i])
	}
	f.Pet.canPushExplicit = explicit(shapes.Pet)
	// i 标记遍历怪物集合时访问的下标。
	for i := range f.Monsters {
		f.Monsters[i].canPushExplicit = explicit(shapes.Monsters[i])
	}
	return nil
}

// ValidateDisplacementFoundation 校验位移“foundation”。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) ValidateDisplacementFoundation() error {
	if f.RuleVersion < 10 {
		return nil
	}
	// actors 保留加入新条目后的行动实体集合。
	actors := append(append([]Fighter(nil), f.Players...), f.Pet)
	// m 接收遍历怪物集合时当前的元素。
	for _, m := range f.Monsters {
		actors = append(actors, m.Fighter)
	}
	// actor 接收遍历行动实体集合时当前的元素。
	for _, actor := range actors {
		if !actor.canPushExplicit {
			return errors.New("rule10实体can_push必须显式布尔，缺失或null不能默认成false")
		}
	}
	// skill 接收遍历技能集合时当前的元素。
	for _, skill := range f.Skills {
		if !skill.present["auto_enabled"] {
			return errors.New("rule10技能auto_enabled必须显式布尔，优先级0不表示关闭")
		}
	}
	// t 保存战斗时序，用于当前步骤中的t。
	t := f.CombatTiming
	// duration 接收遍历数值列表时当前的元素。
	for _, duration := range []int64{t.PlayerPushActionMS, t.MonsterPushActionMSPerStep, t.MonsterPushMoveMSPerStep, t.PushedStepDisplayMS} {
		if duration <= 0 || duration > math.MaxInt32 {
			return errors.New("推挤动作、移动和表现时长必须各自显式且可计时")
		}
	}
	return nil
}
