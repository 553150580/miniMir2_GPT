// 验证安全区绑定请求、BSON 形状、地图摘要和永久操作身份，覆盖伪造或不一致来源。

package persistence

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// enableTeleportBindingFixture 启用瞬移会话绑定开发测试配置。
// s 记录持久层存储。
func enableTeleportBindingFixture(s *Store) {
	s.Cfg.Fixture.RuleVersion = 15
	s.Cfg.Fixture.Skills = []devconfig.Skill{{ID: 21, JobID: 1, Behavior: "teleport", LearnedLevel: 0, MaxLevel: 3}}
	s.Cfg.Fixture.Maps[0].SourceStaticBits = make([]byte, len(s.Cfg.Fixture.Maps[0].CollisionBits))
}

// safeBindingFixture 处理安全会话绑定开发测试配置。
func safeBindingFixture() (*Store, domain.Role, SafeZoneBindingRequest) {
	// enabled 保存当前处理所用的已开启。
	enabled := true
	// m 组装地图模板所需的数据。
	m := devconfig.MapTemplate{ID: 2, Width: 9, Height: 9, SceneKind: devconfig.SceneKindTown, CollisionBits: make([]byte, 11), Source: &devconfig.SourceMapIdentity{Code: "0", SHA256: strings.Repeat("a", 64), Width: 9, Height: 9}, SafeZones: []devconfig.SafeZone{{ID: 1, Center: devconfig.Point{X: 2, Y: 2}, Radius: 1}, {ID: 2, Center: devconfig.Point{X: 3, Y: 2}, Radius: 1}}}
	// f 组装开发测试配置所需的数据。
	f := devconfig.Fixture{RuleVersion: 14, Hash: strings.Repeat("b", 64), Map: devconfig.MapTemplate{ID: 1, Width: 17, Height: 17}, Maps: []devconfig.MapTemplate{m}, Binding: &devconfig.BindingRules{Enabled: false, SafeZoneEnabled: &enabled}}
	// s 保存当前处理所用的持久层存储。
	s := &Store{Cfg: devconfig.Config{Fixture: f}}
	// r 组装角色所需的数据。
	r := domain.Role{ID: strings.Repeat("c", 32), SourceSeq: 8, EconomicSeq: 4, Revision: 15}
	// q 组装安全区域会话绑定请求所需的数据。
	q := SafeZoneBindingRequest{SourceSeq: 9, ExpectedRevision: r.Revision, ExpectedRuleVersion: 14, ExpectedConfigHash: f.Hash, MapID: 2, Position: devconfig.Point{X: 4, Y: 2}, Action: "walk", EventID: strings.Repeat("d", 32)}
	return s, r, q
}

// safeBindingOperation 处理安全会话绑定操作。
// t 提供断言失败报告、子用例和清理注册。
func safeBindingOperation(t *testing.T) (*Store, domain.Role, Operation) {
	t.Helper()
	// s 接收安全会话绑定开发测试配置供后续步骤使用，r 接收安全会话绑定开发测试配置供后续步骤使用，q 接收安全会话绑定开发测试配置供后续步骤使用。
	s, r, q := safeBindingFixture()
	// binding 保存由准备安全区域会话绑定取得的会话绑定，digest 保存由准备安全区域会话绑定取得的摘要，err 接收准备安全区域会话绑定过程中可能产生的错误，供后续失败分支处理。
	binding, digest, err := s.prepareSafeZoneBinding(q)
	if err != nil {
		t.Fatal(err)
	}
	// point 保存当前处理所用的点位。
	point := binding.Point
	r.Binding, r.BindingSourceSeq, r.SourceSeq, r.Revision = &point, q.SourceSeq, q.SourceSeq, r.Revision+1
	// op 组装操作所需的数据。
	op := Operation{ID: bindingOperationID(r.ID, q.SourceSeq), RoleID: r.ID, Digest: digest, CommandSeq: r.EconomicSeq, SourceSeq: q.SourceSeq, Revision: r.Revision, Kind: safeZoneBindingKind, Created: time.Now(), Binding: binding}
	return s, r, op
}

// TestSafeZoneBindingUsesFirstMatchingActionCellAndPreservesInitializerOff 核对「安全区写入开关擅自开启随机新生池」与「没有依据动作格的首匹配写中心」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSafeZoneBindingUsesFirstMatchingActionCellAndPreservesInitializerOff(t *testing.T) {
	// s 接收安全会话绑定开发测试配置供后续步骤使用，q 接收安全会话绑定开发测试配置供后续步骤使用。
	s, _, q := safeBindingFixture()
	// binding 保存由准备安全区域会话绑定取得的会话绑定，err 接收准备安全区域会话绑定过程中可能产生的错误，供后续失败分支处理。
	binding, _, err := s.prepareSafeZoneBinding(q)
	if err != nil || binding.ZoneID != 2 || binding.Point != (domain.RoleBinding{MapID: 2, X: 3, Y: 2}) {
		t.Fatal("没有依据动作格的首匹配写中心", binding, err)
	}
	q.Position = devconfig.Point{X: 3, Y: 2}
	binding, _, err = s.prepareSafeZoneBinding(q)
	if err != nil || binding.ZoneID != 1 || binding.Point.X != 2 {
		t.Fatal("重叠安全区未保留来源顺序", binding, err)
	}
	// initial 保存由初始值会话绑定取得的初始值，err 接收初始值会话绑定过程中可能产生的错误，供后续失败分支处理。
	if initial, err := s.initialBinding(); err != nil || initial != nil {
		t.Fatal("安全区写入开关擅自开启随机新生池", initial, err)
	}
}

// TestSafeZoneBindingRejectsUnsupportedActionsAndUntrustedInputs 核对「未认证动作或来源被接受」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSafeZoneBindingRejectsUnsupportedActionsAndUntrustedInputs(t *testing.T) {
	// change 接收遍历文本列表时当前的元素。
	for _, change := range []string{"turn", "run", "teleport", "pushed", "dash", "spawn", "revive", "join", "zero_seq", "zero_revision", "future_rule", "config", "event", "personal", "outside", "source_missing", "source_wrong_size", "disabled"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(change, func(t *testing.T) {
			// s 接收安全会话绑定开发测试配置供后续步骤使用，q 接收安全会话绑定开发测试配置供后续步骤使用。
			s, _, q := safeBindingFixture()
			switch change {
			case "zero_seq":
				q.SourceSeq = 0
			case "zero_revision":
				q.ExpectedRevision = 0
			case "future_rule":
				q.ExpectedRuleVersion++
			case "config":
				q.ExpectedConfigHash = strings.Repeat("e", 64)
			case "event":
				q.EventID = "client-coordinate"
			case "personal":
				q.MapID = 1
			case "outside":
				q.Position = devconfig.Point{X: 8, Y: 8}
			case "source_missing":
				s.Cfg.Fixture.Maps[0].Source = nil
			case "source_wrong_size":
				s.Cfg.Fixture.Maps[0].Source.Width--
			case "disabled":
				*s.Cfg.Fixture.Binding.SafeZoneEnabled = false
			default:
				q.Action = change
			}
			// err 接收准备安全区域会话绑定过程中可能产生的错误，供后续失败分支处理。
			if _, _, err := s.prepareSafeZoneBinding(q); err == nil {
				t.Fatal("未认证动作或来源被接受")
			}
		})
	}
}

// TestLegacyBindingNeverBecomesTrustedFromWalkabilityOrMatchingCenter 核对「历史坐标被提升成来源证明或被改写」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestLegacyBindingNeverBecomesTrustedFromWalkabilityOrMatchingCenter(t *testing.T) {
	// s 接收安全会话绑定开发测试配置供后续步骤使用，r 接收安全会话绑定开发测试配置供后续步骤使用。
	s, r, _ := safeBindingFixture()
	// binding 接收遍历角色会话绑定列表时当前的元素。
	for _, binding := range []*domain.RoleBinding{nil, {MapID: 2, X: 3, Y: 2}, {MapID: 1, X: 2, Y: 2}} {
		r.Binding, r.BindingSafeZoneID = binding, 999
		// trusted 保存由会话绑定可信状态取得的可信状态，err 接收会话绑定可信状态过程中可能产生的错误，供后续失败分支处理。
		trusted, err := s.BindingTrusted(context.Background(), r)
		if err != nil || trusted || r.Binding != binding {
			t.Fatal("历史坐标被提升成来源证明或被改写", trusted, err)
		}
	}
}

// TestBindingSourceDocumentRejectsMissingTruncatedOrDuplicatedProof 核对「完整证明未通过或派生安全区ID被持久化」与「悬空、未来或重复来源证明被接受」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestBindingSourceDocumentRejectsMissingTruncatedOrDuplicatedProof(t *testing.T) {
	// point 初始化用作点位的角色会话绑定。
	point := domain.RoleBinding{MapID: 2, X: 3, Y: 2}
	// value 接收遍历任意类型的数据列表时当前的元素。
	for _, value := range []any{nil, false, 1.0, "1", int64(0), int64(-1)} {
		// raw 保存由序列化取得的原始值，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
		raw, err := bson.Marshal(bson.M{"binding": point, "binding_source_seq": value, "source_seq": int64(8)})
		if err != nil || Code(checkBindingDocument(raw)) != p.CodeLineForbidden {
			t.Fatal("非法来源序号被补零/截断", value, err)
		}
	}
	// doc 接收遍历MongoDB BSON 文档列表时当前的元素。
	for _, doc := range []bson.D{
		{{Key: "binding_source_seq", Value: int64(1)}, {Key: "source_seq", Value: int64(1)}},
		{{Key: "binding", Value: point}, {Key: "binding_source_seq", Value: int64(2)}, {Key: "source_seq", Value: int64(1)}},
		{{Key: "binding", Value: point}, {Key: "binding_source_seq", Value: int64(1)}, {Key: "binding_source_seq", Value: int64(2)}, {Key: "source_seq", Value: int64(2)}},
		{{Key: "binding", Value: point}, {Key: "binding_source_seq", Value: int64(1)}, {Key: "source_seq", Value: 1.0}},
	} {
		// raw 保存由序列化取得的原始值，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
		raw, err := bson.Marshal(doc)
		if err != nil || Code(checkBindingDocument(raw)) != p.CodeLineForbidden {
			t.Fatal("悬空、未来或重复来源证明被接受", err)
		}
	}
	// role 保存由安全会话绑定操作取得的角色。
	_, role, _ := safeBindingOperation(t)
	// raw 保存由序列化取得的原始值，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	raw, err := bson.Marshal(role)
	if err != nil || checkBindingDocument(raw) != nil || bson.Raw(raw).Lookup("binding_safe_zone_id").Type != 0 {
		t.Fatal("完整证明未通过或派生安全区ID被持久化", err)
	}
}

// TestBindingProofRequiresTypedMatchingReceiptAndSourceCatalog 核对「合法历史证明被中心重新选区或无关配置更新误拒」与「伪造或错配来源证明被接受」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestBindingProofRequiresTypedMatchingReceiptAndSourceCatalog(t *testing.T) {
	// change 接收遍历文本列表时当前的元素。
	for _, change := range []string{"valid", "other_kind", "other_role", "sequence", "source_after_receipt_revision", "command_future", "revision", "code", "digest", "point", "zone", "source", "zone_order", "action", "event", "old_rule", "future_rule", "unrelated_rule_update"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(change, func(t *testing.T) {
			// s 接收安全会话绑定操作供后续步骤使用，r 接收安全会话绑定操作供后续步骤使用，op 保存由安全会话绑定操作取得的操作。
			s, r, op := safeBindingOperation(t)
			switch change {
			case "other_kind":
				op.Kind = "cast_material"
			case "other_role":
				op.RoleID = strings.Repeat("e", 32)
			case "sequence":
				op.SourceSeq++
			case "source_after_receipt_revision":
				op.Revision = op.SourceSeq - 1
			case "command_future":
				op.CommandSeq = r.EconomicSeq + 1
			case "revision":
				op.Revision++
			case "code":
				op.Code = p.CodeInvalidArgument
			case "digest":
				op.Digest = "not-a-server-receipt"
			case "point":
				r.Binding.X++
			case "zone":
				op.Binding.ZoneID = 1
			case "source":
				s.Cfg.Fixture.Maps[0].Source.SHA256 = strings.Repeat("e", 64)
			case "zone_order":
				// zones 保存安全区域集合，用于当前步骤中的区域集合。
				zones := s.Cfg.Fixture.Maps[0].SafeZones
				zones[0], zones[1] = zones[1], zones[0]
			case "action":
				op.Binding.Action = "turn"
			case "event":
				op.Binding.EventID = "input"
			case "old_rule":
				op.Binding.RuleVersion = 13
			case "future_rule":
				op.Binding.RuleVersion = s.Cfg.Fixture.RuleVersion + 1
			case "unrelated_rule_update":
				s.Cfg.Fixture.RuleVersion++
				s.Cfg.Fixture.Hash = strings.Repeat("f", 64)
			}
			// err 接收校验会话绑定操作过程中可能产生的错误，供后续失败分支处理。
			err := s.validateBindingOperation(r, op)
			if change == "valid" || change == "unrelated_rule_update" {
				if err != nil {
					t.Fatal("合法历史证明被中心重新选区或无关配置更新误拒", err)
				}
			} else if Code(err) != p.CodeLineForbidden {
				t.Fatal("伪造或错配来源证明被接受", err)
			}
		})
	}
}

// TestBindingReceiptBSONCannotHideMissingIntegerFieldsOrTemporaryState 核对「非整数、缺字段、重复或夹带临时状态的证明被接受」与「合法回执未通过严格检查」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestBindingReceiptBSONCannotHideMissingIntegerFieldsOrTemporaryState(t *testing.T) {
	// op 保存由安全会话绑定操作取得的操作。
	_, _, op := safeBindingOperation(t)
	// data 保存由序列化取得的数据，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	data, err := bson.Marshal(op)
	if err != nil || checkBindingOperationDocument(data) != nil {
		t.Fatal("合法回执未通过严格检查", err)
	}
	// valid 记录有效，使用MongoDB BSON 文档表示。
	var valid bson.D
	if err = bson.Unmarshal(data, &valid); err != nil {
		t.Fatal(err)
	}
	// change 接收遍历文本列表时当前的元素。
	for _, change := range []string{"source_float", "code_nonzero", "code_overflow", "revision_duplicate", "point_missing_x", "rule_float", "missing_event", "extra_position", "extra_economic", "extra_unknown"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(change, func(t *testing.T) {
			// doc 记录“doc”，使用MongoDB BSON 文档表示。
			var doc bson.D
			_ = bson.Unmarshal(data, &doc)
			// i 标记遍历“doc”时访问的下标。
			for i := range doc {
				if change == "source_float" && doc[i].Key == "source_seq" {
					doc[i].Value = 9.0
				}
				if doc[i].Key == "code" && (change == "code_nonzero" || change == "code_overflow") {
					doc[i].Value = int64(1)
					if change == "code_overflow" {
						doc[i].Value = int64(math.MaxInt64)
					}
				}
				if doc[i].Key != "binding_use" {
					continue
				}
				// binding 按目标类型读取当前接口值，得到会话绑定。
				binding := doc[i].Value.(bson.D)
				// j 保存根据“len”与当前数据计算的当前位置的下标。
				for j := len(binding) - 1; j >= 0; j-- {
					switch {
					case change == "point_missing_x" && binding[j].Key == "point":
						binding[j].Value = bson.D{{Key: "map_id", Value: int64(2)}, {Key: "y", Value: int32(2)}}
					case change == "rule_float" && binding[j].Key == "rule_version":
						binding[j].Value = 14.0
					case change == "missing_event" && binding[j].Key == "event_id":
						binding = append(binding[:j], binding[j+1:]...)
					}
				}
				if change == "extra_position" {
					binding = append(binding, bson.E{Key: "position", Value: bson.M{"x": 4, "y": 2}})
				}
				doc[i].Value = binding
			}
			if change == "revision_duplicate" {
				doc = append(doc, bson.E{Key: "revision", Value: int64(math.MaxInt64)})
			}
			if change == "extra_economic" {
				doc = append(doc, bson.E{Key: "potion", Value: nil})
			}
			if change == "extra_unknown" {
				doc = append(doc, bson.E{Key: "world", Value: bson.M{"hp": 1}})
			}
			// raw 保存由序列化取得的原始值，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
			raw, err := bson.Marshal(doc)
			if err != nil || Code(checkBindingOperationDocument(raw)) != p.CodeLineForbidden {
				t.Fatal("非整数、缺字段、重复或夹带临时状态的证明被接受", err)
			}
		})
	}
}

// TestTeleportBindingRule15PreservesOldWalkDigestAndProof 核验rule15不能冷验原walk14证明，用诊断「新增可选origin字段改变旧walk摘要」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleportBindingRule15PreservesOldWalkDigestAndProof(t *testing.T) {
	// s 接收安全会话绑定开发测试配置供后续步骤使用，role 保存由安全会话绑定开发测试配置取得的角色，q 接收安全会话绑定开发测试配置供后续步骤使用。
	s, role, q := safeBindingFixture()
	// oldRequest 初始化用作原状态请求的临时结构。
	oldRequest := struct {
		// SourceSeq 记录持久收益来源序号，使用数值表示。
		// ExpectedRevision 记录预期结果修订号，使用数值表示。
		SourceSeq, ExpectedRevision int64
		// ExpectedRuleVersion 记录预期结果规则版本，使用数值表示。
		ExpectedRuleVersion uint64
		// ExpectedConfigHash 记录预期结果配置摘要，使用文本表示。
		ExpectedConfigHash string
		// MapID 标识当前业务使用的地图。
		MapID uint64
		// Position 记录位置，使用点位表示。
		Position devconfig.Point
		// Action 记录动作，使用文本表示。
		// EventID 标识当前业务使用的事件。
		Action, EventID string
	}{q.SourceSeq, q.ExpectedRevision, q.ExpectedRuleVersion, q.ExpectedConfigHash, q.MapID, q.Position, q.Action, q.EventID}
	// oldJSON 保存由序列化取得的原状态JSON。
	oldJSON, _ := json.Marshal(oldRequest)
	// newJSON 保存由序列化取得的创建JSON。
	newJSON, _ := json.Marshal(q)
	if !bytes.Equal(oldJSON, newJSON) {
		t.Fatal("新增可选origin字段改变旧walk摘要", string(oldJSON), string(newJSON))
	}
	// old 保存由安全会话绑定操作取得的原状态。
	_, role, old := safeBindingOperation(t)
	enableTeleportBindingFixture(s)
	// err 接收校验会话绑定操作过程中可能产生的错误，供后续失败分支处理。
	if err := s.validateBindingOperation(role, old); err != nil {
		t.Fatal("rule15不能冷验原walk14证明", err)
	}
	old.Binding.Action = "teleport"
	// err 接收校验会话绑定操作过程中可能产生的错误，供后续失败分支处理。
	if err := s.validateBindingOperation(role, old); Code(err) != p.CodeLineForbidden {
		t.Fatal("rule14回执被改标签后获得teleport资格", err)
	}
	old.Binding.RuleVersion = 15
	// err 接收校验会话绑定操作过程中可能产生的错误，供后续失败分支处理。
	if err := s.validateBindingOperation(role, old); err != nil {
		t.Fatal("rule15 typed teleport证明无法冷验", err)
	}
	// 后续关闭技能不撤销已经签发的永久绑定；fresh提交仍单独检查当前功能及已学资格。
	s.Cfg.Fixture.Skills = nil
	// err 接收校验会话绑定操作过程中可能产生的错误，供后续失败分支处理。
	if err := s.validateBindingOperation(role, old); err != nil {
		t.Fatal("当前技能目录变化错误撤销已提交来源证明", err)
	}
}

// TestTeleportBindingRequiresSelectedLearnedSkillAndVerifiedOrigin 核对「未验证的坐标或非21来源取得teleport资格」与「合法21来源资格被拒」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleportBindingRequiresSelectedLearnedSkillAndVerifiedOrigin(t *testing.T) {
	// change 接收遍历文本列表时当前的元素。
	for _, change := range []string{"valid", "rule14", "not_selected", "wrong_behavior", "not_learned", "wrong_job", "rank", "nil_binding", "unproved_same_center", "missing_zone", "missing_origin", "stale_origin", "wrong_map"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(change, func(t *testing.T) {
			// s 接收安全会话绑定操作供后续步骤使用，role 保存由安全会话绑定操作取得的角色，old 保存由安全会话绑定操作取得的原状态。
			s, role, old := safeBindingOperation(t)
			enableTeleportBindingFixture(s)
			role.Job, role.Skills, role.BindingSafeZoneID = 1, []domain.LearnedSkill{{SkillID: 21, Level: 1}}, old.Binding.ZoneID
			// q 组装安全区域会话绑定请求所需的数据。
			q := SafeZoneBindingRequest{SourceSeq: role.SourceSeq + 1, ExpectedRevision: role.Revision, ExpectedRuleVersion: 15, ExpectedConfigHash: s.Cfg.Fixture.Hash, MapID: role.Binding.MapID, Position: devconfig.Point{X: 4, Y: 2}, Action: "teleport", EventID: strings.Repeat("e", 32), OriginBindingSourceSeq: role.BindingSourceSeq}
			switch change {
			case "rule14":
				s.Cfg.Fixture.RuleVersion = 14
			case "not_selected":
				s.Cfg.Fixture.Skills = nil
			case "wrong_behavior":
				s.Cfg.Fixture.Skills[0].Behavior = "fireball"
			case "not_learned":
				role.Skills = nil
			case "wrong_job":
				role.Job = 0
			case "rank":
				role.Skills[0].Level = 4
			case "nil_binding":
				role.Binding = nil
			case "unproved_same_center":
				role.BindingSourceSeq = 0
			case "missing_zone":
				role.BindingSafeZoneID = 0
			case "missing_origin":
				q.OriginBindingSourceSeq = 0
			case "stale_origin":
				q.OriginBindingSourceSeq--
			case "wrong_map":
				q.MapID = 1
			}
			// err 接收校验会话绑定大区或起点过程中可能产生的错误，供后续失败分支处理。
			err := s.validateBindingOrigin(role, q)
			if change == "valid" {
				if err != nil {
					t.Fatal("合法21来源资格被拒", err)
				}
			} else if err == nil {
				t.Fatal("未验证的坐标或非21来源取得teleport资格")
			}
		})
	}
}

// TestTeleportBindingUsesSourceStaticLandingAndStrictActionOrigin 核对「21落点被额外闭门过滤，或未写首匹配中心」与「动作版本或origin字段不完整仍通过」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleportBindingUsesSourceStaticLandingAndStrictActionOrigin(t *testing.T) {
	// s 接收安全会话绑定开发测试配置供后续步骤使用，q 接收安全会话绑定开发测试配置供后续步骤使用。
	s, _, q := safeBindingFixture()
	enableTeleportBindingFixture(s)
	q.ExpectedRuleVersion, q.Action, q.OriginBindingSourceSeq = 15, "teleport", 1
	// m 保存当前处理所用的地图集合中的指定项。
	m := &s.Cfg.Fixture.Maps[0]
	// index 保存根据纵坐标与宽度的计算结果与横坐标计算的下标。
	index := q.Position.Y*m.Width + q.Position.X
	m.CollisionBits[index/8] |= 1 << uint(index%8) // 本作开发闭门位；原静态位仍为Walk。
	// binding 保存由准备安全区域会话绑定取得的会话绑定，err 接收准备安全区域会话绑定过程中可能产生的错误，供后续失败分支处理。
	binding, _, err := s.prepareSafeZoneBinding(q)
	if err != nil || binding.Point != (domain.RoleBinding{MapID: 2, X: 3, Y: 2}) {
		t.Fatal("21落点被额外闭门过滤，或未写首匹配中心", binding, err)
	}
	m.SourceStaticBits[index/8] |= 1 << uint(index%8)
	if _, _, err = s.prepareSafeZoneBinding(q); Code(err) != p.CodeLineForbidden {
		t.Fatal("21穿过来源静态墙", err)
	}
	m.SourceStaticBits[index/8] = 0
	// change 接收遍历文本列表时当前的元素。
	for _, change := range []string{"teleport14", "teleport_no_origin", "walk_with_origin"} {
		// bad 保存当前处理所用的“bad”。
		bad := q
		switch change {
		case "teleport14":
			bad.ExpectedRuleVersion = 14
		case "teleport_no_origin":
			bad.OriginBindingSourceSeq = 0
		case "walk_with_origin":
			bad.Action = "walk"
		}
		// err 接收准备安全区域会话绑定过程中可能产生的错误，供后续失败分支处理。
		if _, _, err := s.prepareSafeZoneBinding(bad); err == nil {
			t.Fatal("动作版本或origin字段不完整仍通过", change)
		}
	}
}
