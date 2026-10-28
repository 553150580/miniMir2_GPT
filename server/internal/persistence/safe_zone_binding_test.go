package persistence

import (
	"context"
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func safeBindingFixture() (*Store, domain.Role, SafeZoneBindingRequest) {
	enabled := true
	m := devconfig.MapTemplate{ID: 2, Width: 9, Height: 9, SceneKind: devconfig.SceneKindTown, CollisionBits: make([]byte, 11), Source: &devconfig.SourceMapIdentity{Code: "0", SHA256: strings.Repeat("a", 64), Width: 9, Height: 9}, SafeZones: []devconfig.SafeZone{{ID: 1, Center: devconfig.Point{X: 2, Y: 2}, Radius: 1}, {ID: 2, Center: devconfig.Point{X: 3, Y: 2}, Radius: 1}}}
	f := devconfig.Fixture{RuleVersion: 14, Hash: strings.Repeat("b", 64), Map: devconfig.MapTemplate{ID: 1, Width: 17, Height: 17}, Maps: []devconfig.MapTemplate{m}, Binding: &devconfig.BindingRules{Enabled: false, SafeZoneEnabled: &enabled}}
	s := &Store{Cfg: devconfig.Config{Fixture: f}}
	r := domain.Role{ID: strings.Repeat("c", 32), SourceSeq: 8, EconomicSeq: 4, Revision: 15}
	q := SafeZoneBindingRequest{SourceSeq: 9, ExpectedRevision: r.Revision, ExpectedRuleVersion: 14, ExpectedConfigHash: f.Hash, MapID: 2, Position: devconfig.Point{X: 4, Y: 2}, Action: "walk", EventID: strings.Repeat("d", 32)}
	return s, r, q
}

func safeBindingOperation(t *testing.T) (*Store, domain.Role, Operation) {
	t.Helper()
	s, r, q := safeBindingFixture()
	binding, digest, err := s.prepareSafeZoneBinding(q)
	if err != nil {
		t.Fatal(err)
	}
	point := binding.Point
	r.Binding, r.BindingSourceSeq, r.SourceSeq, r.Revision = &point, q.SourceSeq, q.SourceSeq, r.Revision+1
	op := Operation{ID: bindingOperationID(r.ID, q.SourceSeq), RoleID: r.ID, Digest: digest, CommandSeq: r.EconomicSeq, SourceSeq: q.SourceSeq, Revision: r.Revision, Kind: safeZoneBindingKind, Created: time.Now(), Binding: binding}
	return s, r, op
}

func TestSafeZoneBindingUsesFirstMatchingActionCellAndPreservesInitializerOff(t *testing.T) {
	s, _, q := safeBindingFixture()
	binding, _, err := s.prepareSafeZoneBinding(q)
	if err != nil || binding.ZoneID != 2 || binding.Point != (domain.RoleBinding{MapID: 2, X: 3, Y: 2}) {
		t.Fatal("没有依据动作格的首匹配写中心", binding, err)
	}
	q.Position = devconfig.Point{X: 3, Y: 2}
	binding, _, err = s.prepareSafeZoneBinding(q)
	if err != nil || binding.ZoneID != 1 || binding.Point.X != 2 {
		t.Fatal("重叠安全区未保留来源顺序", binding, err)
	}
	if initial, err := s.initialBinding(); err != nil || initial != nil {
		t.Fatal("安全区写入开关擅自开启随机新生池", initial, err)
	}
}

func TestSafeZoneBindingRejectsUnsupportedActionsAndUntrustedInputs(t *testing.T) {
	for _, change := range []string{"turn", "run", "teleport", "pushed", "dash", "spawn", "revive", "join", "zero_seq", "zero_revision", "future_rule", "config", "event", "personal", "outside", "source_missing", "source_wrong_size", "disabled"} {
		t.Run(change, func(t *testing.T) {
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
			if _, _, err := s.prepareSafeZoneBinding(q); err == nil {
				t.Fatal("未认证动作或来源被接受")
			}
		})
	}
}

func TestLegacyBindingNeverBecomesTrustedFromWalkabilityOrMatchingCenter(t *testing.T) {
	s, r, _ := safeBindingFixture()
	for _, binding := range []*domain.RoleBinding{nil, {MapID: 2, X: 3, Y: 2}, {MapID: 1, X: 2, Y: 2}} {
		r.Binding, r.BindingSafeZoneID = binding, 999
		trusted, err := s.BindingTrusted(context.Background(), r)
		if err != nil || trusted || r.Binding != binding {
			t.Fatal("历史坐标被提升成来源证明或被改写", trusted, err)
		}
	}
}

func TestBindingSourceDocumentRejectsMissingTruncatedOrDuplicatedProof(t *testing.T) {
	point := domain.RoleBinding{MapID: 2, X: 3, Y: 2}
	for _, value := range []any{nil, false, 1.0, "1", int64(0), int64(-1)} {
		raw, err := bson.Marshal(bson.M{"binding": point, "binding_source_seq": value, "source_seq": int64(8)})
		if err != nil || Code(checkBindingDocument(raw)) != p.CodeLineForbidden {
			t.Fatal("非法来源序号被补零/截断", value, err)
		}
	}
	for _, doc := range []bson.D{
		{{Key: "binding_source_seq", Value: int64(1)}, {Key: "source_seq", Value: int64(1)}},
		{{Key: "binding", Value: point}, {Key: "binding_source_seq", Value: int64(2)}, {Key: "source_seq", Value: int64(1)}},
		{{Key: "binding", Value: point}, {Key: "binding_source_seq", Value: int64(1)}, {Key: "binding_source_seq", Value: int64(2)}, {Key: "source_seq", Value: int64(2)}},
		{{Key: "binding", Value: point}, {Key: "binding_source_seq", Value: int64(1)}, {Key: "source_seq", Value: 1.0}},
	} {
		raw, err := bson.Marshal(doc)
		if err != nil || Code(checkBindingDocument(raw)) != p.CodeLineForbidden {
			t.Fatal("悬空、未来或重复来源证明被接受", err)
		}
	}
	_, role, _ := safeBindingOperation(t)
	raw, err := bson.Marshal(role)
	if err != nil || checkBindingDocument(raw) != nil || bson.Raw(raw).Lookup("binding_safe_zone_id").Type != 0 {
		t.Fatal("完整证明未通过或派生安全区ID被持久化", err)
	}
}

func TestBindingProofRequiresTypedMatchingReceiptAndSourceCatalog(t *testing.T) {
	for _, change := range []string{"valid", "other_kind", "other_role", "sequence", "source_after_receipt_revision", "command_future", "revision", "code", "digest", "point", "zone", "source", "zone_order", "action", "event", "old_rule", "future_rule", "unrelated_rule_update"} {
		t.Run(change, func(t *testing.T) {
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

func TestBindingReceiptBSONCannotHideMissingIntegerFieldsOrTemporaryState(t *testing.T) {
	_, _, op := safeBindingOperation(t)
	data, err := bson.Marshal(op)
	if err != nil || checkBindingOperationDocument(data) != nil {
		t.Fatal("合法回执未通过严格检查", err)
	}
	var valid bson.D
	if err = bson.Unmarshal(data, &valid); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"source_float", "code_nonzero", "code_overflow", "revision_duplicate", "point_missing_x", "rule_float", "missing_event", "extra_position", "extra_economic", "extra_unknown"} {
		t.Run(change, func(t *testing.T) {
			var doc bson.D
			_ = bson.Unmarshal(data, &doc)
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
				binding := doc[i].Value.(bson.D)
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
			raw, err := bson.Marshal(doc)
			if err != nil || Code(checkBindingOperationDocument(raw)) != p.CodeLineForbidden {
				t.Fatal("非整数、缺字段、重复或夹带临时状态的证明被接受", err)
			}
		})
	}
}
