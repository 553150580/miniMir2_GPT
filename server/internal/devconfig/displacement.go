package devconfig

import (
	"encoding/json"
	"errors"
	"math"
)

// 在夹具入口记录布尔字段是否显式提供，避免嵌入Fighter的Monster解码丢失外层字段。
func (f *Fixture) UnmarshalJSON(data []byte) error {
	if err := validatePetRuntimePresence(data); err != nil {
		return err
	}
	type plainFixture Fixture
	var value plainFixture
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var shapes struct {
		Players  []map[string]json.RawMessage `json:"players"`
		Pet      map[string]json.RawMessage   `json:"pet"`
		Monsters []map[string]json.RawMessage `json:"monsters"`
	}
	if err := json.Unmarshal(data, &shapes); err != nil {
		return err
	}
	*f = Fixture(value)
	explicit := func(fields map[string]json.RawMessage) bool {
		v, exists := fields["can_push"]
		return exists && string(v) != "null"
	}
	for i := range f.Players {
		f.Players[i].canPushExplicit = explicit(shapes.Players[i])
	}
	f.Pet.canPushExplicit = explicit(shapes.Pet)
	for i := range f.Monsters {
		f.Monsters[i].canPushExplicit = explicit(shapes.Monsters[i])
	}
	return nil
}

func (f *Fixture) ValidateDisplacementFoundation() error {
	if f.RuleVersion < 10 {
		return nil
	}
	actors := append(append([]Fighter(nil), f.Players...), f.Pet)
	for _, m := range f.Monsters {
		actors = append(actors, m.Fighter)
	}
	for _, actor := range actors {
		if !actor.canPushExplicit {
			return errors.New("rule10实体can_push必须显式布尔，缺失或null不能默认成false")
		}
	}
	for _, skill := range f.Skills {
		if !skill.present["auto_enabled"] {
			return errors.New("rule10技能auto_enabled必须显式布尔，优先级0不表示关闭")
		}
	}
	t := f.CombatTiming
	for _, duration := range []int64{t.PlayerPushActionMS, t.MonsterPushActionMSPerStep, t.MonsterPushMoveMSPerStep, t.PushedStepDisplayMS} {
		if duration <= 0 || duration > math.MaxInt32 {
			return errors.New("推挤动作、移动和表现时长必须各自显式且可计时")
		}
	}
	return nil
}
