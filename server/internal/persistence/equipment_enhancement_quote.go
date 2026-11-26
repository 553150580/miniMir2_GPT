// 本文件为角色部位签发短期强化报价，并在同一资产事务内落实原报价及结果。
package persistence

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"reflect"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	AssetEquipmentEnhancement  = "equipment_enhancement"        // 角色部位强化的经济操作类型。
	enhancementQuoteCollection = "equipment_enhancement_quotes" // 每名角色至多一条可确认的新报价。
	enhancementQuoteDuration   = 30 * time.Second               // 新确认在原界面的有效期。
	enhancementQuoteTTLGrace   = 5 * time.Minute                // 已受理写入可在超期后完成，过期报价稍后自动清理。
	enhancementRollRange       = 1_000_000                      // 成功率和抽样采用同一百万分比刻度。
)

// equipmentEnhancementStoredQuote 保存原报价、附着围栏、版本以及一次不可见的抽样。
type equipmentEnhancementStoredQuote struct {
	RoleID       string                   `bson:"_id"`           // 角色永久主键。
	ID           string                   `bson:"quote_id"`      // 返回给原界面的报价编号。
	Scene        string                   `bson:"scene_epoch"`   // 无损保存原场景代次。
	UISession    string                   `bson:"ui_session_id"` // 看过确认内容的原界面会话。
	SessionEpoch int64                    `bson:"session_epoch"` // 签发时的角色连接代次。
	RunID        uint64                   `bson:"run_server_id"` // 原运行服。
	RunEpoch     int64                    `bson:"run_epoch"`     // 原运行服租约代次。
	Revision     int64                    `bson:"revision"`      // 客户端确认报价所见的角色资产修订。
	Rule         uint64                   `bson:"rule_version"`  // 原操作采用的规则。
	Config       string                   `bson:"config_hash"`   // 同版内容修改也会使旧报价失效。
	Issued       time.Time                `bson:"issued_at"`     // 允许确认的第一个时刻。
	Expires      time.Time                `bson:"expires_at"`    // 新确认的排他终点。
	PurgeAt      time.Time                `bson:"purge_at"`      // 数据库可清理此报价的时刻。
	Request      []byte                   `bson:"request"`       // 原石堆、数量和使用绑定的明确选择。
	Plan         equipmentEnhancementPlan `bson:"plan"`          // 可复核的整笔条款。
	SuccessRoll  uint32                   `bson:"success_roll"`  // 私有抽样，事务重试不重新抽取。
}

// EquipmentEnhancementReceipt 是已经完成一次扣石判定的事实，概率失败也保存完整原消耗。
type EquipmentEnhancementReceipt struct {
	QuoteID     string                   `bson:"quote_id"`     // 原确认编号。
	Scene       string                   `bson:"scene_epoch"`  // 原提交接受时的场景。
	Rule        uint64                   `bson:"rule_version"` // 首次采用的规则。
	Config      string                   `bson:"config_hash"`  // 首次采用的精确配置。
	Plan        equipmentEnhancementPlan `bson:"plan"`         // 原部位、消耗、绑定授权和成功率。
	SuccessRoll uint32                   `bson:"success_roll"` // 审计保留原抽样，不进入公开回执。
	Success     bool                     `bson:"success"`      // 本次概率判定结果。
	AfterLevel  uint32                   `bson:"after_level"`  // 原级或上升一级。
}

// Proto 仅公布本次结算已确认的部位、等级和原石堆。
func (r *EquipmentEnhancementReceipt) Proto() *pb.EquipmentEnhancementReceipt {
	return &pb.EquipmentEnhancementReceipt{QuoteId: domain.Bytes(r.QuoteID), Slot: r.Plan.Slot, BeforeLevel: r.Plan.CurrentLevel,
		AfterLevel: r.AfterLevel, Success: r.Success, Stones: enhancementStonesProto(r.Plan.Stones)}
}

// PreviewEquipmentEnhancement 在现有角色/运行服围栏内签发原石堆报价。
func (s *Store) PreviewEquipmentEnhancement(ctx context.Context, a Auth, q *pb.EquipmentEnhancementQuoteRequest) (*pb.EquipmentEnhancementQuote, error) {
	fixture := &s.Cfg.Fixture
	if fixture.EquipmentEnhancement == nil {
		return nil, Fail(p.CodeEquipmentEnhancementUnavailable, "当前规则未开放装备部位强化")
	}
	if !ValidEquipmentEnhancementQuoteRequest(q) {
		return nil, Fail(p.CodeInvalidArgument, "装备部位强化报价缺少原界面或原石堆")
	}
	payload, err := proto.Marshal(q)
	if err != nil {
		return nil, err
	}
	roll, err := rand.Int(rand.Reader, big.NewInt(enhancementRollRange))
	if err != nil {
		return nil, err
	}
	issued := time.Now().UTC().Truncate(time.Millisecond)
	quote := equipmentEnhancementStoredQuote{RoleID: a.RoleID, ID: domain.ID(), Scene: strconv.FormatUint(q.SceneEpoch, 10),
		UISession: domain.Hex(q.UiSessionId), SessionEpoch: a.SessionEpoch, RunID: s.Cfg.RunID, RunEpoch: s.Epoch,
		Rule: fixture.RuleVersion, Config: fixture.Hash, Issued: issued, Expires: issued.Add(enhancementQuoteDuration),
		PurgeAt: issued.Add(enhancementQuoteDuration + enhancementQuoteTTLGrace), Request: payload, SuccessRoll: uint32(roll.Uint64())}
	err = s.guardedTx(ctx, a, func(tc context.Context) error {
		if err := s.fence(tc, a); err != nil {
			return err
		}
		role, err := s.Role(tc, a.RoleID)
		if err != nil {
			return err
		}
		if err = s.checkRole(role, a); err != nil {
			return err
		}
		plan, code := buildEquipmentEnhancementPlan(role, fixture, q)
		if code != 0 {
			return Fail(code, "原角色等级或选定强化石不满足条件")
		}
		quote.Revision, quote.Plan = role.Revision, plan
		_, err = s.DB.Collection(enhancementQuoteCollection).ReplaceOne(tc, bson.M{"_id": role.ID}, quote, options.Replace().SetUpsert(true))
		return err
	})
	if err != nil {
		return nil, err
	}
	plan := quote.Plan
	return &pb.EquipmentEnhancementQuote{QuoteId: domain.Bytes(quote.ID), SceneEpoch: q.SceneEpoch, Slot: plan.Slot,
		CurrentLevel: plan.CurrentLevel, TargetLevel: plan.CurrentLevel + 1, CurrentBonusBp: plan.CurrentBonusBP, TargetBonusBp: plan.TargetBonusBP,
		SuccessPpm: plan.SuccessPPM, Stones: enhancementStonesProto(plan.Stones), AllowBound: plan.AllowBound,
		ExpiresAtUnixMs: uint64(quote.Expires.UnixMilli()), RoleRevision: uint64(quote.Revision), UiSessionId: domain.Bytes(quote.UISession)}, nil
}

// ValidEquipmentEnhancementCommitRequest 确认原报价和界面身份合法，持久层还会核对资产身份。
func ValidEquipmentEnhancementCommitRequest(input *pb.EquipmentEnhancementCommitRequest) bool {
	return input != nil && input.Operation != nil && input.Slot < domain.EquipmentEnhancementSlots && len(input.QuoteId) == 16 &&
		len(input.UiSessionId) == 16 && len(input.ProtoReflect().GetUnknown()) == 0
}

// commitEquipmentEnhancement 只处理首次操作，旧回执已由资产事务入口核对。
func (s *Store) commitEquipmentEnhancement(ctx context.Context, a Auth, role domain.Role, q AssetRequest) (domain.Role, uint32, *EquipmentEnhancementReceipt, error) {
	input := q.EquipmentEnhancement
	if !ValidEquipmentEnhancementCommitRequest(input) || !proto.Equal(input.Operation, q.Context) || q.SceneEpoch == 0 {
		return role, p.CodeInvalidArgument, nil, nil
	}
	if q.EquipmentEnhancementGate != 0 {
		return role, q.EquipmentEnhancementGate, nil, nil
	}
	fixture := &s.Cfg.Fixture
	if fixture.EquipmentEnhancement == nil {
		return role, p.CodeEquipmentEnhancementUnavailable, nil, nil
	}
	filter := bson.M{"_id": role.ID, "quote_id": domain.Hex(input.QuoteId)}
	collection := s.DB.Collection(enhancementQuoteCollection)
	var quote equipmentEnhancementStoredQuote // 读取时只允许当前角色确认自己的原报价。
	err := collection.FindOne(ctx, filter).Decode(&quote)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return role, p.CodeEquipmentEnhancementQuoteChanged, nil, nil
	}
	if err != nil {
		return role, 0, nil, err
	}
	if quote.Scene != strconv.FormatUint(q.SceneEpoch, 10) || quote.UISession != domain.Hex(input.UiSessionId) ||
		quote.SessionEpoch != a.SessionEpoch || quote.RunID != s.Cfg.RunID || quote.RunEpoch != s.Epoch ||
		q.EquipmentEnhancementAcceptedAt.Before(quote.Issued) || !q.EquipmentEnhancementAcceptedAt.Before(quote.Expires) ||
		quote.Rule != fixture.RuleVersion || quote.Config != fixture.Hash || quote.SuccessRoll >= enhancementRollRange ||
		quote.Revision < 1 || quote.Revision > role.Revision || q.Context.ExpectedRevision != uint64(quote.Revision) {
		return role, p.CodeEquipmentEnhancementQuoteChanged, nil, nil
	}
	var original pb.EquipmentEnhancementQuoteRequest // 重新核实确切石堆、绑定授权及原部位，允许不相干的角色修订变化。
	if proto.Unmarshal(quote.Request, &original) != nil || original.SceneEpoch != q.SceneEpoch ||
		domain.Hex(original.UiSessionId) != quote.UISession || input.Slot != original.Slot {
		return role, p.CodeEquipmentEnhancementQuoteChanged, nil, nil
	}
	plan, code := buildEquipmentEnhancementPlan(role, fixture, &original)
	if code != 0 || !reflect.DeepEqual(plan, quote.Plan) {
		return role, p.CodeEquipmentEnhancementQuoteChanged, nil, nil
	}
	receipt := &EquipmentEnhancementReceipt{QuoteID: quote.ID, Scene: quote.Scene, Rule: quote.Rule, Config: quote.Config,
		Plan: plan, SuccessRoll: quote.SuccessRoll, Success: quote.SuccessRoll < plan.SuccessPPM, AfterLevel: plan.CurrentLevel}
	next := role
	if receipt.Success {
		receipt.AfterLevel++
		state := domain.EquipmentEnhancement{RoleID: role.ID, Version: role.Revision + 1}
		if role.EquipmentEnhancement != nil {
			state.Levels = role.EquipmentEnhancement.Levels
		}
		state.Levels[plan.Slot] = receipt.AfterLevel
		next.EquipmentEnhancement = &state
		if !equipmentRoleArithmeticSafe(next, *fixture) {
			return role, p.CodeEquipmentEnhancementInputs, nil, nil
		}
		if err = s.writeRoleEquipmentEnhancement(ctx, role.EquipmentEnhancement, state); err != nil {
			return role, 0, nil, err
		}
	}
	deleted, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		return role, 0, nil, err
	}
	if deleted.DeletedCount != 1 {
		return role, 0, nil, Fail(p.CodeOperationConflict, "原部位报价已经消费")
	}
	uses := make(map[string]int64, len(plan.Stones))
	for _, material := range plan.Stones {
		uses[material.Item.ID] = material.Quantity
	}
	next.Items = spendGrowthMaterials(role.Items, uses)
	return next, 0, receipt, nil
}
