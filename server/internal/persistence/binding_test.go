package persistence

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"testing"
)

func TestBindingDocumentNeverFillsMissingCoordinatesOrTruncatesValues(t *testing.T) {
	for _, scenario := range []struct {
		name  string
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
		t.Run(scenario.name, func(t *testing.T) {
			raw, err := bson.Marshal(bson.M{"binding": scenario.value})
			if err != nil {
				t.Fatal(err)
			}
			if Code(checkBindingDocument(raw)) != p.CodeLineForbidden {
				t.Fatal("不完整或非整数的绑定存档被默认补齐或截断")
			}
		})
	}
	for _, value := range []bson.M{{}, {"binding": nil}, {"binding": domain.RoleBinding{MapID: 1, X: 0, Y: 0}}} {
		raw, err := bson.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := checkBindingDocument(raw); err != nil {
			t.Fatal("旧存档或显式零坐标被错误拒绝", err)
		}
	}
}

func TestInitialBindingUsesApprovedListAndNeverInfersCurrentSpawn(t *testing.T) {
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{Cfg: cfg}
	if binding, err := s.initialBinding(); err != nil || binding != nil {
		t.Fatal("旧版本未审定绑定入口被默认开启", binding, err)
	}
	s.Cfg.Fixture.RuleVersion = 12
	s.Cfg.Fixture.Binding = &devconfig.BindingRules{Enabled: true, StartPoints: []devconfig.BindingPoint{{MapID: cfg.Fixture.Map.ID, X: 0, Y: 0}}}
	binding, err := s.initialBinding()
	if err != nil || binding == nil || binding.X != 0 || binding.Y != 0 || binding.MapID != cfg.Fixture.Map.ID {
		t.Fatal("显式坐标0被出生点或其它缺省值替代", binding, err)
	}
	if int(binding.X) == cfg.Fixture.Map.PlayerSpawn.X && int(binding.Y) == cfg.Fixture.Map.PlayerSpawn.Y {
		t.Fatal("本例必须区分永久绑定和临时出生坐标")
	}
	// 已存绑定只校验配置地图与地形，不因初始起点列表改变而重设。
	saved := &domain.RoleBinding{MapID: cfg.Fixture.Map.ID, X: 2, Y: 2}
	if err := s.checkBinding(saved); err != nil {
		t.Fatal(err)
	}
	saved.MapID = math.MaxInt64
	if Code(s.checkBinding(saved)) != p.CodeLineForbidden || saved.MapID != math.MaxInt64 {
		t.Fatal("未知地图绑定未拒绝，或被自动覆盖")
	}
}
