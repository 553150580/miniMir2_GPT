// 本文件签发角色勋章短期报价，并在经济事务内确认原报价和原碎片。
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
	AssetMedalCommit     = "medal_commit"   // 沿角色经济命令序号激活或升级原勋章。
	medalQuoteCollection = "medal_quotes"   // 每个角色保留当前有效的原面板报价。
	medalQuoteDuration   = 30 * time.Second // 已展示报价首次受理的有效窗口。
	medalQuoteGrace      = 5 * time.Minute  // 延迟实际删除，供已受理的队列与数据库重试。
)

// medalStoredQuote 将原请求、当时条款及账号路由围栏保存到角色独占报价。
type medalStoredQuote struct {
	RoleID       string          `bson:"_id"`           // 原角色永久主键。
	ID           string          `bson:"quote_id"`      // 明确展示并提交的原报价编号。
	Scene        string          `bson:"scene_epoch"`   // 签发时场景代次，保持无损十进制。
	UISession    string          `bson:"ui_session_id"` // 展示原报价的面板会话。
	SessionEpoch int64           `bson:"session_epoch"` // 原角色连接的附着代次。
	RunID        uint64          `bson:"run_server_id"` // 承载原连接的运行服。
	RunEpoch     int64           `bson:"run_epoch"`     // 原运行服租约代次。
	Revision     int64           `bson:"revision"`      // 玩家看到报价时的角色资产修订。
	Rule         uint64          `bson:"rule_version"`  // 生成原价格与属性的规则版本。
	Config       string          `bson:"config_hash"`   // 同版本规则也必须采用原配置字节。
	Issued       time.Time       `bson:"issued_at"`     // 允许首次受理原确认的起点。
	Expires      time.Time       `bson:"expires_at"`    // 首次确认的排他截止。
	PurgeAt      time.Time       `bson:"purge_at"`      // 未消费过期报价的回收时间。
	Request      []byte          `bson:"request"`       // 原勋章、原碎片及绑料授权的完整协议。
	Plan         medalGrowthPlan `bson:"plan"`          // 已显示给玩家的材料原堆和目标属性。
}

// MedalCommitReceipt 保留原勋章变更与确切消费，重放不会再次升级或扣料。
type MedalCommitReceipt struct {
	QuoteID    string               `bson:"quote_id"`      // 已消费的原报价编号。
	Before     uint32               `bson:"before_level"`  // 原勋章等级，首次激活保存零。
	Medal      domain.MedalState    `bson:"medal"`         // 成交后的永久角色勋章。
	Fragments  []medalFragmentSpend `bson:"fragments"`     // 真实消耗的每个原堆及数量。
	AllowBound bool                 `bson:"allow_bound"`   // 原面板已表达的绑定材料授权。
	UISession  string               `bson:"ui_session_id"` // 原报价所在面板会话。
	Scene      string               `bson:"scene_epoch"`   // 受理原确认时的正式附着场景。
	Rule       uint64               `bson:"rule_version"`  // 首次确认所采用的规则版本。
	Config     string               `bson:"config_hash"`   // 首次确认所采用的配置摘要。
}

// Proto 返回原确认的持久结果，发生新成长后重放仍能认出原消费和等级。
func (r *MedalCommitReceipt) Proto() *pb.MedalReceipt {
	if r == nil {
		return nil
	}
	return &pb.MedalReceipt{QuoteId: domain.Bytes(r.QuoteID), BeforeLevel: r.Before, Medal: r.Medal.Proto(),
		Fragments: medalSpendProto(r.Fragments)}
}

// PreviewMedal 在原角色及租约围栏内，按当前持有的确切碎片签发逐级报价。
func (s *Store) PreviewMedal(ctx context.Context, auth Auth, request *pb.MedalQuoteRequest) (*pb.MedalQuote, error) {
	fixture := &s.Cfg.Fixture // 当前运行服提供的不可变定价和属性。
	if fixture.Medal == nil {
		return nil, Fail(p.CodeMedalUnavailable, "本规则未开放勋章")
	}
	if !ValidMedalQuoteRequest(request) {
		return nil, Fail(p.CodeInvalidArgument, "缺少勋章报价的原场景、碎片或面板会话")
	}
	payload, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	issued := time.Now().UTC().Truncate(time.Millisecond)
	quote := medalStoredQuote{RoleID: auth.RoleID, ID: domain.ID(), Scene: strconv.FormatUint(request.SceneEpoch, 10),
		UISession: domain.Hex(request.UiSessionId), SessionEpoch: auth.SessionEpoch, RunID: s.Cfg.RunID, RunEpoch: s.Epoch,
		Rule: fixture.RuleVersion, Config: fixture.Hash, Issued: issued, Expires: issued.Add(medalQuoteDuration),
		PurgeAt: issued.Add(medalQuoteDuration + medalQuoteGrace), Request: payload}
	err = s.guardedTx(ctx, auth, func(tx context.Context) error {
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
		plan, code := buildMedalGrowthPlan(role, fixture, request)
		if code != 0 {
			return Fail(code, "原勋章或选定碎片不满足升级条件")
		}
		quote.Revision, quote.Plan = role.Revision, plan
		_, err = s.DB.Collection(medalQuoteCollection).ReplaceOne(tx, bson.M{"_id": role.ID}, quote, options.Replace().SetUpsert(true))
		return err
	})
	if err != nil {
		return nil, err
	}
	plan := quote.Plan
	return &pb.MedalQuote{QuoteId: domain.Bytes(quote.ID), SceneEpoch: request.SceneEpoch, Medal: plan.Before.Proto(),
		TargetLevel: plan.Target.Level, TargetMainAttack: plan.Target.MainAttack, TargetDefense: plan.Target.Defense,
		Fragments: medalSpendProto(plan.Fragments), AllowBound: request.AllowBound, ExpiresAtUnixMs: uint64(quote.Expires.UnixMilli()),
		RoleRevision: uint64(quote.Revision), UiSessionId: domain.Bytes(quote.UISession)}, nil
}

// ValidMedalCommit 检查原确认携带同一个经济上下文及界面会话。
func ValidMedalCommit(request *pb.MedalCommitRequest, operation *pb.OperationContext) bool {
	return request != nil && operation != nil && proto.Equal(request.Operation, operation) &&
		len(request.QuoteId) == 16 && len(request.UiSessionId) == 16 &&
		len(request.ProtoReflect().GetUnknown()) == 0 && len(operation.ProtoReflect().GetUnknown()) == 0
}

// commitMedal 在已有回执检查以后执行首次确认；原报价、碎片、档案和审计同进退。
func (s *Store) commitMedal(ctx context.Context, auth Auth, role domain.Role, request AssetRequest) (domain.Role, uint32, *MedalCommitReceipt, error) {
	fixture := &s.Cfg.Fixture
	if fixture.Medal == nil || request.MedalGate != 0 {
		return role, p.CodeMedalUnavailable, nil, nil
	}
	input := request.MedalCommit
	if !ValidMedalCommit(input, request.Context) {
		return role, p.CodeInvalidArgument, nil, nil
	}
	collection := s.DB.Collection(medalQuoteCollection)
	filter := bson.M{"_id": role.ID, "quote_id": domain.Hex(input.QuoteId)}
	var quote medalStoredQuote // 从当前角色的报价文档恢复原场景与确切投入。
	err := collection.FindOne(ctx, filter).Decode(&quote)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return role, p.CodeMedalQuoteChanged, nil, nil
	}
	if err != nil {
		return role, 0, nil, err
	}
	if quote.Scene != strconv.FormatUint(request.SceneEpoch, 10) || quote.UISession != domain.Hex(input.UiSessionId) ||
		quote.SessionEpoch != auth.SessionEpoch || quote.RunID != s.Cfg.RunID || quote.RunEpoch != s.Epoch ||
		request.MedalAcceptedAt.Before(quote.Issued) || !request.MedalAcceptedAt.Before(quote.Expires) ||
		quote.Rule != fixture.RuleVersion || quote.Config != fixture.Hash ||
		quote.Revision < 1 || quote.Revision > role.Revision || request.Context.ExpectedRevision != uint64(quote.Revision) {
		return role, p.CodeMedalQuoteChanged, nil, nil
	}
	var original pb.MedalQuoteRequest // 重新验证确切原堆，拒绝换料、换数量或换绑料授权。
	if proto.Unmarshal(quote.Request, &original) != nil || original.SceneEpoch != request.SceneEpoch ||
		domain.Hex(original.UiSessionId) != quote.UISession {
		return role, p.CodeMedalQuoteChanged, nil, nil
	}
	plan, code := buildMedalGrowthPlan(role, fixture, &original)
	if code != 0 || !reflect.DeepEqual(plan, quote.Plan) {
		return role, p.CodeMedalQuoteChanged, nil, nil
	}
	out := applyMedalGrowthPlan(role, plan)
	if !equipmentRoleArithmeticSafe(out, *fixture) {
		return role, p.CodeMedalInputs, nil, nil
	}
	deleted, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		return role, 0, nil, err
	}
	if deleted.DeletedCount != 1 {
		return role, p.CodeMedalQuoteChanged, nil, nil
	}
	if err := s.writeRoleMedal(ctx, role.Medal, *out.Medal); err != nil {
		return role, 0, nil, err
	}
	receipt := &MedalCommitReceipt{QuoteID: quote.ID, Medal: *out.Medal, Fragments: plan.Fragments, AllowBound: original.AllowBound,
		UISession: quote.UISession, Scene: quote.Scene, Rule: quote.Rule, Config: quote.Config}
	if role.Medal != nil {
		receipt.Before = role.Medal.Level
	}
	return out, 0, receipt, nil
}
