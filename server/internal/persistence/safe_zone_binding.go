package persistence

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const safeZoneBindingKind = "safe_zone_binding"

// 只由权威行走成功后的owner构造；Position仅供本次来源校验，不写入任何永久文档。
// Pending重试必须保留整个请求，不能换来源序号、事件ID、revision或配置身份。
type SafeZoneBindingRequest struct {
	SourceSeq           int64
	ExpectedRevision    int64
	ExpectedRuleVersion uint64
	ExpectedConfigHash  string
	MapID               uint64
	Position            devconfig.Point
	Action              string
	EventID             string
}

// 回执只保存已发生的永久绑定事实及来源身份，不保存角色当前位置或临时世界。
type BindingReceipt struct {
	Point           domain.RoleBinding `bson:"point"`
	ZoneID          uint64             `bson:"zone_id"`
	SourceMapSHA256 string             `bson:"source_map_sha256"`
	ZoneSetSHA256   string             `bson:"zone_set_sha256"`
	Action          string             `bson:"action"`
	EventID         string             `bson:"event_id"`
	RuleVersion     uint64             `bson:"rule_version"`
	ConfigHash      string             `bson:"config_hash"`
}

func lowerHex(value string, bytes int) bool {
	if len(value) != bytes*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == bytes
}

func bindingOperationID(roleID string, sourceSeq int64) string {
	return fmt.Sprintf("binding:%s:%d", roleID, sourceSeq)
}

func checkBindingSourceDocument(raw bson.Raw) error {
	fields, err := raw.Elements()
	if err != nil {
		return Fail(p.CodeLineForbidden, "绑定来源存档结构无效")
	}
	seen := map[string]bool{}
	for _, field := range fields {
		if field.Key() == "binding" || field.Key() == "binding_source_seq" {
			if seen[field.Key()] {
				return Fail(p.CodeLineForbidden, "绑定或来源序号字段重复")
			}
			seen[field.Key()] = true
		}
	}
	value := raw.Lookup("binding_source_seq")
	if value.Type == 0 {
		return nil // 旧角色不补造来源证明，完整旧绑定仍可保留。
	}
	if value.Type != bson.TypeInt32 && value.Type != bson.TypeInt64 || value.AsInt64() <= 0 {
		return Fail(p.CodeLineForbidden, "绑定来源序号必须为显式正整数，不能缺省截断")
	}
	if raw.Lookup("binding").Type != bson.TypeEmbeddedDocument {
		return Fail(p.CodeLineForbidden, "绑定来源序号缺少对应的完整绑定")
	}
	cursor := raw.Lookup("source_seq")
	if cursor.Type != bson.TypeInt32 && cursor.Type != bson.TypeInt64 || cursor.AsInt64() < value.AsInt64() {
		return Fail(p.CodeLineForbidden, "绑定来源序号超过角色完整来源水位")
	}
	return nil
}

func bindingZoneSetHash(m *devconfig.MapTemplate) (string, error) {
	if m == nil || m.SceneKindValue() != devconfig.SceneKindTown || m.Source == nil || m.Source.Code == "" || !lowerHex(m.Source.SHA256, 32) || m.Source.Width != m.Width || m.Source.Height != m.Height || len(m.SafeZones) == 0 {
		return "", Fail(p.CodeLineForbidden, "绑定必须使用完整来源城镇及来源安全区目录")
	}
	// 有序列表保留重叠及重复中心；此摘要是现有唯一配置的派生身份，不是另一份数值源。
	identity := struct {
		MapID  uint64
		Source devconfig.SourceMapIdentity
		Zones  []devconfig.SafeZone
	}{m.ID, *m.Source, m.SafeZones}
	raw, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	return Hash(raw), nil
}

func (s *Store) prepareSafeZoneBinding(q SafeZoneBindingRequest) (*BindingReceipt, string, error) {
	f := &s.Cfg.Fixture
	if !f.SafeZoneBindingEnabled() {
		return nil, "", Fail(p.CodeLineForbidden, "来源安全区绑定入口未启用")
	}
	if q.SourceSeq <= 0 || q.ExpectedRevision <= 0 || q.ExpectedRuleVersion == 0 || q.ExpectedRuleVersion > math.MaxInt64 || !lowerHex(q.ExpectedConfigHash, 32) || q.MapID == 0 || q.MapID > math.MaxInt64 || q.Action != "walk" || !lowerHex(q.EventID, 16) {
		return nil, "", Fail(p.CodeInvalidArgument, "来源绑定缺少权威行走事件及完整身份")
	}
	if q.ExpectedRuleVersion != f.RuleVersion || q.ExpectedConfigHash != f.Hash {
		return nil, "", Fail(p.CodeOperationConflict, "绑定规则身份已变化")
	}
	m, ok := f.MapByID(q.MapID)
	if !ok || !m.Walkable(q.Position) {
		return nil, "", Fail(p.CodeLineForbidden, "绑定动作不在来源城镇可走格")
	}
	zoneHash, err := bindingZoneSetHash(m)
	if err != nil {
		return nil, "", err
	}
	zone, inside := m.SafeZoneAt(q.Position)
	if !inside || zone.ID == 0 || zone.ID > math.MaxInt64 || !m.Walkable(zone.Center) || zone.Center.X > math.MaxInt32 || zone.Center.Y > math.MaxInt32 {
		return nil, "", Fail(p.CodeLineForbidden, "绑定动作没有首匹配的合法来源安全区")
	}
	receipt := &BindingReceipt{Point: domain.RoleBinding{MapID: m.ID, X: int32(zone.Center.X), Y: int32(zone.Center.Y)}, ZoneID: zone.ID,
		SourceMapSHA256: m.Source.SHA256, ZoneSetSHA256: zoneHash, Action: q.Action, EventID: q.EventID, RuleVersion: q.ExpectedRuleVersion, ConfigHash: q.ExpectedConfigHash}
	raw, err := json.Marshal(q)
	if err != nil {
		return nil, "", err
	}
	return receipt, Hash(raw), nil
}

// BindingTrusted只用于事务/入场读取。worker依据已核验的BindingSourceSeq缓存判断，不能在tick中查库。
// 无证明的历史绑定保持原值但无资格；声明了证明却丢失/损坏/错配时拒绝，不静默降级或重写。
func (s *Store) BindingTrusted(ctx context.Context, role domain.Role) (bool, error) {
	return s.restoreBindingTrust(ctx, &role)
}

func (s *Store) restoreBindingTrust(ctx context.Context, role *domain.Role) (bool, error) {
	role.BindingSafeZoneID = 0
	if role.BindingSourceSeq == 0 {
		return false, nil
	}
	if role.BindingSourceSeq < 0 || role.Binding == nil || role.SourceSeq < role.BindingSourceSeq {
		return false, Fail(p.CodeLineForbidden, "绑定来源水位与角色不一致")
	}
	op, err := s.readBindingOperation(ctx, bindingOperationID(role.ID, role.BindingSourceSeq))
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, Fail(p.CodeLineForbidden, "绑定来源永久回执缺失")
	}
	if err != nil {
		return false, err
	}
	if err = s.validateBindingOperation(*role, op); err != nil {
		return false, err
	}
	role.BindingSafeZoneID = op.Binding.ZoneID
	return true, nil
}

func (s *Store) validateBindingOperation(role domain.Role, op Operation) error {
	b := op.Binding
	if b == nil || role.Binding == nil || op.ID != bindingOperationID(role.ID, role.BindingSourceSeq) || op.Kind != safeZoneBindingKind || op.RoleID != role.ID {
		return Fail(p.CodeLineForbidden, "绑定来源回执身份不一致")
	}
	if op.SourceSeq != role.BindingSourceSeq || op.SourceSeq <= 0 || op.SourceSeq > role.SourceSeq || op.SourceSeq > op.Revision || op.CommandSeq < 0 || op.CommandSeq > role.EconomicSeq || op.Revision <= 0 || op.Revision > role.Revision {
		return Fail(p.CodeLineForbidden, "绑定来源回执水位超过角色聚合")
	}
	if op.Code != 0 || op.Potion != nil || op.Casting != nil || !lowerHex(op.Digest, 32) || b.Action != "walk" || !lowerHex(b.EventID, 16) || b.RuleVersion < 14 || b.RuleVersion > s.Cfg.Fixture.RuleVersion || b.RuleVersion > math.MaxInt64 || !lowerHex(b.ConfigHash, 32) || b.Point != *role.Binding {
		return Fail(p.CodeLineForbidden, "绑定来源回执身份或已提交中心不一致")
	}
	m, ok := s.Cfg.Fixture.MapByID(b.Point.MapID)
	if !ok || !m.Walkable(devconfig.Point{X: int(b.Point.X), Y: int(b.Point.Y)}) {
		return Fail(p.CodeLineForbidden, "绑定来源中心不在当前来源地图可走格")
	}
	zoneHash, err := bindingZoneSetHash(m)
	if err != nil {
		return err
	}
	if b.SourceMapSHA256 != m.Source.SHA256 || b.ZoneSetSHA256 != zoneHash {
		return Fail(p.CodeLineForbidden, "绑定来源地图或安全区目录身份已变化")
	}
	// 提交时在动作格取首匹配；冷验按已记录的源行身份核中心，不在另一格重新选区。
	for _, zone := range m.SafeZones {
		if zone.ID == b.ZoneID && zone.Center.X == int(b.Point.X) && zone.Center.Y == int(b.Point.Y) {
			return nil
		}
	}
	return Fail(p.CodeLineForbidden, "绑定来源安全区与中心不一致")
}

func (s *Store) readBindingOperation(ctx context.Context, id string) (Operation, error) {
	var op Operation
	raw, err := s.DB.Collection("operations").FindOne(ctx, bson.M{"_id": id}).Raw()
	if err != nil {
		return op, err
	}
	if err = checkBindingOperationDocument(raw); err != nil {
		return op, err
	}
	err = bson.Unmarshal(raw, &op)
	return op, err
}

// 原始BSON严格核验，避免缺字段/重复键/浮点数在Decode后伪装成完整证明。
func checkBindingOperationDocument(raw bson.Raw) error {
	fail := func() error { return Fail(p.CodeLineForbidden, "绑定来源回执BSON结构不完整或类型无效") }
	fields, err := raw.Elements()
	if err != nil || len(fields) != 10 {
		return fail()
	}
	seen := map[string]bool{}
	for _, field := range fields {
		if seen[field.Key()] {
			return fail()
		}
		seen[field.Key()] = true
		switch field.Key() {
		case "_id", "digest", "role_id", "command_seq", "source_seq", "revision", "code", "kind", "created_at", "binding_use":
		default:
			return fail() // 新typed kind无旧扩展合同；不能夹带其它经济回执或临时字段。
		}
	}
	for _, name := range []string{"_id", "digest", "role_id", "kind"} {
		if raw.Lookup(name).Type != bson.TypeString {
			return fail()
		}
	}
	for _, name := range []string{"command_seq", "source_seq", "revision", "code"} {
		value := raw.Lookup(name)
		if value.Type != bson.TypeInt32 && value.Type != bson.TypeInt64 || value.AsInt64() < 0 || (name == "source_seq" || name == "revision") && value.AsInt64() == 0 || name == "code" && value.AsInt64() != 0 {
			return fail()
		}
	}
	if raw.Lookup("created_at").Type != bson.TypeDateTime {
		return fail()
	}
	doc, ok := raw.Lookup("binding_use").DocumentOK()
	if !ok {
		return fail()
	}
	items, err := doc.Elements()
	if err != nil || len(items) != 8 {
		return fail()
	}
	seen = map[string]bool{}
	for _, item := range items {
		key, value := item.Key(), item.Value()
		if seen[key] {
			return fail()
		}
		seen[key] = true
		switch key {
		case "point":
			if value.Type != bson.TypeEmbeddedDocument {
				return fail()
			}
			wrapped, err := bson.Marshal(bson.D{{Key: "binding", Value: value.Document()}})
			if err != nil || checkBindingDocument(wrapped) != nil {
				return fail()
			}
		case "zone_id", "rule_version":
			if value.Type != bson.TypeInt32 && value.Type != bson.TypeInt64 || value.AsInt64() <= 0 {
				return fail()
			}
		case "source_map_sha256", "zone_set_sha256", "action", "event_id", "config_hash":
			if value.Type != bson.TypeString {
				return fail()
			}
		default:
			return fail()
		}
	}
	return nil
}

// BindSafeZoneWithReceipt不消耗客户端经济序号。原ID/摘要先查回执，绝不把旧SourceSeq直接当成功。
func (s *Store) BindSafeZoneWithReceipt(ctx context.Context, auth Auth, q SafeZoneBindingRequest) (domain.Role, Operation, error) {
	var out domain.Role
	var receipt Operation
	if q.SourceSeq <= 0 {
		return out, receipt, Fail(p.CodeInvalidArgument, "绑定来源序号无效")
	}
	opID := bindingOperationID(auth.RoleID, q.SourceSeq)
	raw, err := json.Marshal(q)
	if err != nil {
		return out, receipt, err
	}
	digest := Hash(raw)
	err = s.tx(ctx, func(tc context.Context) error {
		receipt = Operation{}
		if err := s.fence(tc, auth); err != nil {
			return err
		}
		role, err := s.Role(tc, auth.RoleID)
		if err != nil {
			return err
		}
		if err = s.checkRole(role, auth); err != nil {
			return err
		}
		old, err := s.readBindingOperation(tc, opID)
		if err == nil {
			if old.Kind != safeZoneBindingKind || old.RoleID != role.ID || old.SourceSeq != q.SourceSeq || old.Digest != digest || old.Code != 0 || old.Binding == nil {
				return Fail(p.CodeOperationConflict, "同绑定来源ID的请求内容或回执改变")
			}
			historical := role
			point := old.Binding.Point
			historical.Binding, historical.BindingSourceSeq = &point, old.SourceSeq
			if err = s.validateBindingOperation(historical, old); err != nil {
				return err
			}
			// 返回当前聚合及原回执，旧回执不能覆盖后来已提交的另一绑定。
			out, receipt = role, old
			return nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		if role.SourceSeq == math.MaxInt64 || role.Revision == math.MaxInt64 || role.SourceSeq+1 != q.SourceSeq || role.Revision != q.ExpectedRevision {
			return Fail(p.CodeOperationConflict, "绑定来源序列或角色revision变化，保留原事件对账")
		}
		binding, fingerprint, err := s.prepareSafeZoneBinding(q)
		if err != nil {
			return err
		}
		if fingerprint != digest {
			return Fail(p.CodeOperationConflict, "绑定请求身份不一致")
		}
		before := role.Revision
		point := binding.Point
		role.Binding, role.BindingSourceSeq = &point, q.SourceSeq
		role.BindingSafeZoneID = binding.ZoneID
		role.SourceSeq, role.Revision = q.SourceSeq, role.Revision+1
		if err = s.replace(tc, role, before); err != nil {
			return err
		}
		op := Operation{ID: opID, Digest: digest, RoleID: role.ID, CommandSeq: role.EconomicSeq, SourceSeq: q.SourceSeq, Revision: role.Revision, Kind: safeZoneBindingKind, Created: time.Now(), Binding: binding}
		if _, err = s.DB.Collection("operations").InsertOne(tc, op); err != nil {
			return err
		}
		if _, err = s.DB.Collection("audit_outbox").InsertOne(tc, bson.M{"_id": opID, "kind": safeZoneBindingKind, "role_id": role.ID, "account_id": auth.AccountID, "source_seq": q.SourceSeq, "revision": role.Revision, "binding_use": binding, "at": op.Created}); err != nil {
			return err
		}
		out, receipt = role, op
		return nil
	})
	return out, receipt, err
}
