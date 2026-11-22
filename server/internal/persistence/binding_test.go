// 验证默认角色绑定、BSON 读入约束和持久绑定与当前安全区规则的一致性。

package persistence

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"testing"
)

// TestBindingDocumentNeverFillsMissingCoordinatesOrTruncatesValues 核验角色绑定文档避免擅自补齐缺失坐标，避免把非法数值窄化为可通过校验的位置。
// t 提供断言失败报告、子用例和清理注册。
func TestBindingDocumentNeverFillsMissingCoordinatesOrTruncatesValues(t *testing.T) {
	// scenario 接收遍历临时结构列表时当前的元素。
	for _, scenario := range []struct {
		// name 记录名称，使用文本表示。
		name string
		// value 记录值，使用任意类型的数据表示。
		value any
	}{
		{"empty", bson.D{}},
		{"missing_x", bson.D{{Key: "map_id", Value: int64(1)}, {Key: "y", Value: int32(0)}}},
		{"null_y", bson.D{{Key: "map_id", Value: int64(1)}, {Key: "x", Value: int32(0)}, {Key: "y", Value: nil}}},
		{"fraction", bson.D{{Key: "map_id", Value: int64(1)}, {Key: "x", Value: 0.5}, {Key: "y", Value: int32(0)}}},
		{"overflow", bson.D{{Key: "map_id", Value: int64(1)}, {Key: "x", Value: int64(1) << 32}, {Key: "y", Value: int32(0)}}},
		{"duplicate", bson.D{{Key: "map_id", Value: int64(1)}, {Key: "x", Value: int32(0)}, {Key: "x", Value: int32(0)}}},
		{"extra_temporary", bson.D{{Key: "map_id", Value: int64(1)}, {Key: "x", Value: int32(0)}, {Key: "y", Value: int32(0)}, {Key: "hp", Value: int64(10)}}},
		{"wrong_type", "spawn"},
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(scenario.name, func(t *testing.T) {
			// raw 保存由序列化取得的原始值，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
			raw, err := bson.Marshal(bson.M{"binding": scenario.value})
			if err != nil {
				t.Fatal(err)
			}
			if Code(checkBindingDocument(raw)) != p.CodeLineForbidden {
				t.Fatal("不完整或非整数的绑定存档被默认补齐或截断")
			}
		})
	}
	// value 接收遍历MongoDB BSON 文档列表时当前的元素。
	for _, value := range []bson.M{{}, {"binding": nil}, {"binding": domain.RoleBinding{MapID: 1, X: 0, Y: 0}}} {
		// raw 保存由序列化取得的原始值，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
		raw, err := bson.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		// err 接收“check”会话绑定文档过程中可能产生的错误，供后续失败分支处理。
		if err := checkBindingDocument(raw); err != nil {
			t.Fatal("旧存档或显式零坐标被错误拒绝", err)
		}
	}
}

// TestInitialBindingUsesApprovedListAndNeverInfersCurrentSpawn 确认初始安全区绑定按明确允许的来源建立，当前出生点不构成可信绑定。
// t 提供断言失败报告、子用例和清理注册。
func TestInitialBindingUsesApprovedListAndNeverInfersCurrentSpawn(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// s 保存当前处理所用的持久层存储。
	s := &Store{Cfg: cfg}
	// binding 保存由初始值会话绑定取得的会话绑定，err 接收初始值会话绑定过程中可能产生的错误，供后续失败分支处理。
	if binding, err := s.initialBinding(); err != nil || binding != nil {
		t.Fatal("旧版本未审定绑定入口被默认开启", binding, err)
	}
	s.Cfg.Fixture.RuleVersion = 12
	s.Cfg.Fixture.Binding = &devconfig.BindingRules{Enabled: true, StartPoints: []devconfig.BindingPoint{{MapID: cfg.Fixture.Map.ID, X: 0, Y: 0}}}
	// binding 保存由初始值会话绑定取得的会话绑定。
	binding, err := s.initialBinding()
	if err != nil || binding == nil || binding.X != 0 || binding.Y != 0 || binding.MapID != cfg.Fixture.Map.ID {
		t.Fatal("显式坐标0被出生点或其它缺省值替代", binding, err)
	}
	if int(binding.X) == cfg.Fixture.Map.PlayerSpawn.X && int(binding.Y) == cfg.Fixture.Map.PlayerSpawn.Y {
		t.Fatal("本例必须区分永久绑定和临时出生坐标")
	}
	// 已存绑定只校验配置地图与地形，不因初始起点列表改变而重设。
	// saved 保存当前处理所用的“saved”。
	saved := &domain.RoleBinding{MapID: cfg.Fixture.Map.ID, X: 2, Y: 2}
	// err 接收“check”会话绑定过程中可能产生的错误，供后续失败分支处理。
	if err := s.checkBinding(saved); err != nil {
		t.Fatal(err)
	}
	saved.MapID = math.MaxInt64
	if Code(s.checkBinding(saved)) != p.CodeLineForbidden || saved.MapID != math.MaxInt64 {
		t.Fatal("未知地图绑定未拒绝，或被自动覆盖")
	}
}
