// 角色复活绑定的历史 BSON 格式校验和默认绑定。读取时识别不完整或越界的坐标，并按当前城镇安全区规则初始化。

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
// checkBindingDocument 在解码之前校验角色绑定 BSON 完整性、唯一字段和坐标范围。
// role 记录角色，使用原始值表示。
func checkBindingDocument(role bson.Raw) error {
	// err 接收“check”会话绑定来源文档过程中可能产生的错误，供后续失败分支处理。
	if err := checkBindingSourceDocument(role); err != nil {
		return err
	}
	// value 保存由查找取得的值。
	value := role.Lookup("binding")
	if value.Type == 0 || value.Type == bson.TypeNull {
		return nil
	}
	// document 保存由文档成功标记取得的文档，ok 表明查询、校验或类型转换是否成功。
	document, ok := value.DocumentOK()
	if !ok {
		return Fail(p.CodeLineForbidden, "角色绑定存档不是完整地图坐标对象")
	}
	// fields 保存由“elements”取得的字段集合，err 接收“elements”过程中可能产生的错误，供后续失败分支处理。
	fields, err := document.Elements()
	if err != nil || len(fields) != 3 {
		return Fail(p.CodeLineForbidden, "角色绑定存档字段缺失或超出当前合同")
	}
	// seen 建立带初始值的按文本索引的布尔状态映射，供本段继续处理。
	seen := map[string]bool{}
	// field 接收遍历字段集合时当前的元素。
	for _, field := range fields {
		// key 接收键供后续步骤使用，number 保存由值取得的编号。
		key, number := field.Key(), field.Value()
		if (key != "map_id" && key != "x" && key != "y") || seen[key] || number.Type != bson.TypeInt32 && number.Type != bson.TypeInt64 {
			return Fail(p.CodeLineForbidden, "角色绑定存档须为唯一地图ID及完整整数坐标")
		}
		seen[key] = true
		// n 接收“as”整数“64”供后续步骤使用。
		n := number.AsInt64()
		if key == "map_id" && n <= 0 || key != "map_id" && (n < math.MinInt32 || n > math.MaxInt32) {
			return Fail(p.CodeLineForbidden, "角色绑定存档整数越界")
		}
	}
	return nil
}

// 只为首次业务初始化挑选配置中的起点；不从角色临时位置推导，也不增加收费。
// initialBinding 按当前规则的可信默认安全区坐标构造角色首次或兼容补齐用的绑定。
// s 是调用该方法的持久层存储实例。
func (s *Store) initialBinding() (*domain.RoleBinding, error) {
	// f 保存当前处理所用的开发测试配置。
	f := &s.Cfg.Fixture
	// err 接收校验会话绑定过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateBinding(); err != nil {
		return nil, err
	}
	if f.Binding == nil || !f.Binding.Enabled {
		return nil, nil
	}
	// index 保存由整数取得的下标，err 接收整数过程中可能产生的错误，供后续失败分支处理。
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(f.Binding.StartPoints))))
	if err != nil {
		return nil, err
	}
	// point 保存从起点或开始时间点位集合取出的点位。
	point := f.Binding.StartPoints[index.Int64()]
	return &domain.RoleBinding{MapID: point.MapID, X: point.X, Y: point.Y}, nil
}

// checkBinding 核对绑定所指地图和坐标能否作为角色的静态复活落点。
// s 是调用该方法的持久层存储实例，binding 记录会话绑定，使用角色会话绑定表示。
func (s *Store) checkBinding(binding *domain.RoleBinding) error {
	if binding != nil && !s.Cfg.Fixture.ValidBinding(devconfig.BindingPoint{MapID: binding.MapID, X: binding.X, Y: binding.Y}) {
		return Fail(p.CodeLineForbidden, "角色绑定地图或地格不在当前配置目录，不自动覆盖原绑定")
	}
	return nil
}
