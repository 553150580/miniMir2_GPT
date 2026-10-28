package persistence

import (
	"crypto/rand"
	"go.mongodb.org/mongo-driver/v2/bson"
	"math"
	"math/big"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// 先检查原始存档结构，避免BSON解码把缺少的x/y补为0后当成有效绑定。
func checkBindingDocument(role bson.Raw) error {
	value := role.Lookup("binding")
	if value.Type == 0 || value.Type == bson.TypeNull {
		return nil
	}
	document, ok := value.DocumentOK()
	if !ok {
		return Fail(p.CodeLineForbidden, "角色绑定存档不是完整地图坐标对象")
	}
	fields, err := document.Elements()
	if err != nil || len(fields) != 3 {
		return Fail(p.CodeLineForbidden, "角色绑定存档字段缺失或超出当前合同")
	}
	seen := map[string]bool{}
	for _, field := range fields {
		key, number := field.Key(), field.Value()
		if (key != "map_id" && key != "x" && key != "y") || seen[key] || number.Type != bson.TypeInt32 && number.Type != bson.TypeInt64 {
			return Fail(p.CodeLineForbidden, "角色绑定存档须为唯一地图ID及完整整数坐标")
		}
		seen[key] = true
		n := number.AsInt64()
		if key == "map_id" && n <= 0 || key != "map_id" && (n < math.MinInt32 || n > math.MaxInt32) {
			return Fail(p.CodeLineForbidden, "角色绑定存档整数越界")
		}
	}
	return nil
}

// 只为首次业务初始化挑选配置中的起点；不从角色临时位置推导，也不增加收费。
func (s *Store) initialBinding() (*domain.RoleBinding, error) {
	f := &s.Cfg.Fixture
	if err := f.ValidateBinding(); err != nil {
		return nil, err
	}
	if f.Binding == nil || !f.Binding.Enabled {
		return nil, nil
	}
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(f.Binding.StartPoints))))
	if err != nil {
		return nil, err
	}
	point := f.Binding.StartPoints[index.Int64()]
	return &domain.RoleBinding{MapID: point.MapID, X: point.X, Y: point.Y}, nil
}

func (s *Store) checkBinding(binding *domain.RoleBinding) error {
	if binding != nil && !s.Cfg.Fixture.ValidBinding(devconfig.BindingPoint{MapID: binding.MapID, X: binding.X, Y: binding.Y}) {
		return Fail(p.CodeLineForbidden, "角色绑定地图或地格不在当前配置目录，不自动覆盖原绑定")
	}
	return nil
}
