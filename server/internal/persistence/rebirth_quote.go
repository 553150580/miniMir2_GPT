// 本文件在角色和运行服的原所有权下保存转生报价与原子成交回执。
package persistence

import (
	"context"
	"errors"
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
	AssetRebirthCommit     = "rebirth_commit" // 使用现有角色经济流水。
	rebirthQuoteCollection = "rebirth_quotes" // 每个角色只保留最新报价。
	rebirthQuoteDuration   = 30 * time.Second // 报价展示后的首次受理期限。
	rebirthQuoteGrace      = 5 * time.Minute  // 已受理的队列与数据库重试保留期。
)

// rebirthStoredQuote 保留报价与初次确认必须核对的所有身份。
type rebirthStoredQuote struct {
	RoleID       string      `bson:"_id"`           // 角色唯一编号。
	ID           string      `bson:"quote_id"`      // 本次报价编号。
	Scene        string      `bson:"scene_epoch"`   // 原场景，按文本保存完整64位。
	UISession    string      `bson:"ui_session_id"` // 原面板会话。
	SessionEpoch int64       `bson:"session_epoch"` // 原登录接管代次。
	RunID        uint64      `bson:"run_server_id"` // 签发报价的运行服。
	RunEpoch     int64       `bson:"run_epoch"`     // 原运行服所有权代次。
	Revision     int64       `bson:"revision"`      // 原报价的角色修订。
	Rule         uint64      `bson:"rule_version"`  // 签发时的玩法版本。
	Config       string      `bson:"config_hash"`   // 玩法字节摘要。
	Issued       time.Time   `bson:"issued_at"`     // 允许首次确认的起点。
	Expires      time.Time   `bson:"expires_at"`    // 允许首次确认的截止。
	PurgeAt      time.Time   `bson:"purge_at"`      // 被TTL清理的时刻。
	Plan         rebirthPlan `bson:"plan"`          // 已向玩家显示的角色结果和费用。
}

// RebirthCommitReceipt 保存原成长结果和资产变更，允许后续连接原样核对。
type RebirthCommitReceipt struct {
	QuoteID   string      `bson:"quote_id"`      // 已消费的报价。
	Scene     string      `bson:"scene_epoch"`   // 原订单所属场景。
	UISession string      `bson:"ui_session_id"` // 原订单所属界面。
	Rule      uint64      `bson:"rule_version"`  // 原成交规则版本。
	Config    string      `bson:"config_hash"`   // 原成交配置字节摘要。
	Plan      rebirthPlan `bson:"plan"`          // 成交时的角色结果和原堆费用。
}

// Proto 恢复原成交协议，不从后续角色状态重新推算。
func (r *RebirthCommitReceipt) Proto() *pb.RebirthReceipt {
	if r == nil {
		return nil
	}
	return r.Plan.proto(r.QuoteID)
}

// ValidRebirthQuoteRequest 校验原场景和面板身份。
func ValidRebirthQuoteRequest(request *pb.RebirthQuoteRequest) bool {
	return request != nil && request.SceneEpoch != 0 && len(request.UiSessionId) == 16 && len(request.ProtoReflect().GetUnknown()) == 0
}

// ValidRebirthCommit 确认正文和经济处理拿到同一原操作。
func ValidRebirthCommit(request *pb.RebirthCommitRequest, operation *pb.OperationContext) bool {
	return request != nil && operation != nil && proto.Equal(request.Operation, operation) &&
		len(request.QuoteId) == 16 && len(request.UiSessionId) == 16 && len(request.ProtoReflect().GetUnknown()) == 0 &&
		len(operation.ProtoReflect().GetUnknown()) == 0
}

// PreviewRebirth 在原角色授权和租约内签发只读费用与成长结果。
func (s *Store) PreviewRebirth(ctx context.Context, auth Auth, request *pb.RebirthQuoteRequest) (*pb.RebirthQuote, error) {
	f := &s.Cfg.Fixture
	if !f.Progression.RebirthEnabled {
		return nil, Fail(p.CodeRebirthUnavailable, "本规则未开放转生")
	}
	if !ValidRebirthQuoteRequest(request) {
		return nil, Fail(p.CodeInvalidArgument, "转生报价缺少原场景或面板身份")
	}
	issued := bossTime(time.Now())
	quote := rebirthStoredQuote{RoleID: auth.RoleID, ID: domain.ID(), Scene: strconv.FormatUint(request.SceneEpoch, 10),
		UISession: domain.Hex(request.UiSessionId), SessionEpoch: auth.SessionEpoch, RunID: s.Cfg.RunID, RunEpoch: s.Epoch,
		Rule: f.RuleVersion, Config: f.Hash, Issued: issued, Expires: issued.Add(rebirthQuoteDuration),
		PurgeAt: issued.Add(rebirthQuoteDuration + rebirthQuoteGrace)}
	err := s.guardedTx(ctx, auth, func(tx context.Context) error {
		if err := s.fence(tx, auth); err != nil {
			return err
		}
		role, err := s.Role(tx, auth.RoleID)
		if err != nil {
			return err
		}
		if err := s.checkRole(role, auth); err != nil {
			return err
		}
		_, plan, code, err := s.buildRebirthPlan(tx, role)
		if err != nil {
			return err
		}
		if code != 0 {
			return Fail(code, "转生档位、挑战凭证或投入条件不符")
		}
		quote.Revision, quote.Plan = role.Revision, plan
		_, err = s.DB.Collection(rebirthQuoteCollection).ReplaceOne(tx, bson.M{"_id": role.ID}, quote, options.Replace().SetUpsert(true))
		return err
	})
	if err != nil {
		return nil, err
	}
	return &pb.RebirthQuote{Preview: quote.Plan.proto(quote.ID), SceneEpoch: request.SceneEpoch,
		UiSessionId: domain.Bytes(quote.UISession), RoleRevision: uint64(quote.Revision),
		ExpiresAtUnixMs: uint64(quote.Expires.UnixMilli())}, nil
}

// commitRebirth 应用首次确认。原回执由资产事务在场景和资格检查前处理。
func (s *Store) commitRebirth(ctx context.Context, auth Auth, role domain.Role, request AssetRequest) (domain.Role, uint32, *RebirthCommitReceipt, error) {
	f := &s.Cfg.Fixture
	if !f.Progression.RebirthEnabled || request.RebirthGate != 0 {
		return role, p.CodeRebirthUnavailable, nil, nil
	}
	input := request.RebirthCommit
	if !ValidRebirthCommit(input, request.Context) {
		return role, p.CodeInvalidArgument, nil, nil
	}
	collection := s.DB.Collection(rebirthQuoteCollection)
	filter := bson.M{"_id": role.ID, "quote_id": domain.Hex(input.QuoteId)}
	var quote rebirthStoredQuote
	err := collection.FindOne(ctx, filter).Decode(&quote)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return role, p.CodeRebirthQuoteChanged, nil, nil
	}
	if err != nil {
		return role, 0, nil, err
	}
	if quote.Scene != strconv.FormatUint(request.SceneEpoch, 10) || quote.UISession != domain.Hex(input.UiSessionId) ||
		quote.SessionEpoch != auth.SessionEpoch || quote.RunID != s.Cfg.RunID || quote.RunEpoch != s.Epoch ||
		request.RebirthAcceptedAt.Before(quote.Issued) || !request.RebirthAcceptedAt.Before(quote.Expires) ||
		quote.Rule != f.RuleVersion || quote.Config != f.Hash || quote.Revision < 1 || quote.Revision > role.Revision ||
		uint64(quote.Revision) != request.Context.ExpectedRevision {
		return role, p.CodeRebirthQuoteChanged, nil, nil
	}
	out, plan, code, err := s.buildRebirthPlan(ctx, role)
	if err != nil {
		return role, 0, nil, err
	}
	if code != 0 || !reflect.DeepEqual(plan, quote.Plan) {
		return role, p.CodeRebirthQuoteChanged, nil, nil
	}
	deleted, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		return role, 0, nil, err
	}
	if deleted.DeletedCount != 1 {
		return role, p.CodeRebirthQuoteChanged, nil, nil
	}
	return out, 0, &RebirthCommitReceipt{QuoteID: quote.ID, Scene: quote.Scene, UISession: quote.UISession,
		Rule: quote.Rule, Config: quote.Config, Plan: plan}, nil
}
