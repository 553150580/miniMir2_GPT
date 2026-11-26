// 本文件签发翅膀升阶报价，并在原角色经济事务中确认激活、逐阶培养或外观。
package persistence

import (
	"context"
	"errors"
	"reflect"
	"slices"
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
	AssetWingAction     = "wing_action"    // 沿原角色经济命令序号执行的翅膀操作。
	wingQuoteCollection = "wing_quotes"    // 每个角色只保留最新的本界面升阶报价。
	wingQuoteDuration   = 30 * time.Second // 与现有改造操作相同的报价有效期。
	wingQuoteTTLGrace   = 5 * time.Minute  // 给已受理的事务与重试保留读取原报价的时间。
)

// wingStoredQuote 保存本次升阶的原角色会话、界面、配置及确切的材料来源。
type wingStoredQuote struct {
	RoleID       string         `bson:"_id"`           // 原角色永久主键。
	ID           string         `bson:"quote_id"`      // 原报价确认编号。
	Scene        string         `bson:"scene_epoch"`   // 原附着场景代次，无损保存为十进制。
	UISession    string         `bson:"ui_session_id"` // 已展示本报价的原界面会话。
	SessionEpoch int64          `bson:"session_epoch"` // 签发时的角色连接会话代次。
	RunID        uint64         `bson:"run_server_id"` // 原签发运行服。
	RunEpoch     int64          `bson:"run_epoch"`     // 原签发运行服租约代次。
	Revision     int64          `bson:"revision"`      // 客户端看到报价时的角色资产水位。
	Rule         uint64         `bson:"rule_version"`  // 所用规则版本。
	Config       string         `bson:"config_hash"`   // 同版本配置更换也使报价失效。
	Issued       time.Time      `bson:"issued_at"`     // 最早可受理原报价的时刻。
	Expires      time.Time      `bson:"expires_at"`    // 原新请求首次受理的排他终点。
	PurgeAt      time.Time      `bson:"purge_at"`      // 未被使用的过期报价实际清理点。
	Request      []byte         `bson:"request"`       // 精确的原羽毛、翅膀版本和绑料选择。
	Plan         wingGrowthPlan `bson:"plan"`          // 从原角色数据生成的完整确认条款。
}

// WingActionReceipt 是已经确认的首次翅膀操作。重放不重读原报价、重新扣包或更换实例。
type WingActionReceipt struct {
	Action     uint32             `bson:"action"`                  // 激活或升阶为 1，更新外观为 2。
	Wing       domain.WingState   `bson:"wing"`                    // 成交之后同一原翅膀的完整状态。
	Before     *domain.WingState  `bson:"before,omitempty"`        // 事务之前的完整状态，首次激活时为空。
	QuoteID    string             `bson:"quote_id,omitempty"`      // 确认的原升阶报价，外观事务没有报价。
	UISession  string             `bson:"ui_session_id,omitempty"` // 原界面会话及绑定授权来源。
	Scene      string             `bson:"scene_epoch"`             // 实际首次操作的附着场景。
	Rule       uint64             `bson:"rule_version"`            // 实际采用的规则版本。
	Config     string             `bson:"config_hash"`             // 首次采用的不可变配置。
	AllowBound bool               `bson:"allow_bound"`             // 实际执行时原报价是否允许绑定羽毛。
	Feathers   []wingFeatherSpend `bson:"feathers,omitempty"`      // 实际消耗的原羽毛堆和每堆扣量。
}

// Proto 发布原已确认实例和消耗。外观操作不携带虚构报价或材料。
func (r *WingActionReceipt) Proto() *pb.WingReceipt {
	if r == nil {
		return nil
	}
	return &pb.WingReceipt{Action: r.Action, Wing: r.Wing.Proto(), QuoteId: domain.Bytes(r.QuoteID), Feathers: wingSpendProto(r.Feathers)}
}

// PreviewWing 只签发当前角色、当前附着和真实持有材料的短期逐阶报价。
func (s *Store) PreviewWing(ctx context.Context, a Auth, q *pb.WingQuoteRequest) (*pb.WingQuote, error) {
	fixture := &s.Cfg.Fixture
	if fixture.Wings == nil {
		return nil, Fail(p.CodeWingUnavailable, "本规则未开放翅膀")
	}
	if !validWingQuoteRequest(q) {
		return nil, Fail(p.CodeInvalidArgument, "翅膀报价缺少原场景、羽毛或界面会话")
	}
	payload, err := proto.Marshal(q)
	if err != nil {
		return nil, err
	}
	issued := time.Now().UTC().Truncate(time.Millisecond)
	quote := wingStoredQuote{RoleID: a.RoleID, ID: domain.ID(), Scene: strconv.FormatUint(q.SceneEpoch, 10),
		UISession: domain.Hex(q.UiSessionId), SessionEpoch: a.SessionEpoch, RunID: s.Cfg.RunID, RunEpoch: s.Epoch,
		Rule: fixture.RuleVersion, Config: fixture.Hash, Issued: issued, Expires: issued.Add(wingQuoteDuration),
		PurgeAt: issued.Add(wingQuoteDuration + wingQuoteTTLGrace), Request: payload}
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
		plan, code := buildWingGrowthPlan(role, fixture, q)
		if code != 0 {
			return Fail(code, "原翅膀或选定羽毛不满足逐阶条件")
		}
		quote.Revision, quote.Plan = role.Revision, plan
		_, err = s.DB.Collection(wingQuoteCollection).ReplaceOne(tc, bson.M{"_id": role.ID}, quote, options.Replace().SetUpsert(true))
		return err
	})
	if err != nil {
		return nil, err
	}
	plan := quote.Plan
	return &pb.WingQuote{QuoteId: domain.Bytes(quote.ID), SceneEpoch: q.SceneEpoch, Wing: plan.Before.Proto(),
		TargetRank: plan.TargetRank, TargetAppearanceTier: plan.TargetAppearanceTier, Feathers: wingSpendProto(plan.Feathers),
		AllowBound: plan.AllowBound, ExpiresAtUnixMs: uint64(quote.Expires.UnixMilli()), RoleRevision: uint64(quote.Revision),
		UiSessionId: domain.Bytes(quote.UISession)}, nil
}

// validWingAction 验证不同动作使用的字段，外观不带可消费报价或绑定授权。
func validWingAction(input *pb.WingActionRequest, context *pb.OperationContext) bool {
	if input == nil || !proto.Equal(input.Operation, context) || len(input.ProtoReflect().GetUnknown()) != 0 {
		return false
	}
	switch input.Action {
	case wingActionGrow:
		return len(input.QuoteId) == 16 && len(input.UiSessionId) == 16 && validWingRef(input.WingId, input.WingVersion, true) &&
			input.AppearanceTier == 0 && !input.Hidden
	case wingActionAppearance:
		return len(input.QuoteId) == 0 && (len(input.UiSessionId) == 0 || len(input.UiSessionId) == 16) && !input.AllowBound &&
			validWingRef(input.WingId, input.WingVersion, false) && input.AppearanceTier > 0
	}
	return false
}

// claimWingGrowthQuote 在首次资产事务中核对并消耗原报价。物品、翅膀和审计可整体回滚。
func (s *Store) claimWingGrowthQuote(ctx context.Context, a Auth, role domain.Role, q AssetRequest) (wingGrowthPlan, uint32, error) {
	input := q.WingAction
	filter := bson.M{"_id": role.ID, "quote_id": domain.Hex(input.QuoteId)}
	collection := s.DB.Collection(wingQuoteCollection)
	var quote wingStoredQuote
	err := collection.FindOne(ctx, filter).Decode(&quote)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return wingGrowthPlan{}, p.CodeWingQuoteChanged, nil
	}
	if err != nil {
		return wingGrowthPlan{}, 0, err
	}
	fixture := &s.Cfg.Fixture
	if quote.Scene != strconv.FormatUint(q.SceneEpoch, 10) || quote.UISession != domain.Hex(input.UiSessionId) ||
		quote.SessionEpoch != a.SessionEpoch || quote.RunID != s.Cfg.RunID || quote.RunEpoch != s.Epoch ||
		q.WingAcceptedAt.Before(quote.Issued) || !q.WingAcceptedAt.Before(quote.Expires) || quote.Rule != fixture.RuleVersion || quote.Config != fixture.Hash ||
		quote.Revision < 1 || quote.Revision > role.Revision || q.Context.ExpectedRevision != uint64(quote.Revision) {
		return wingGrowthPlan{}, p.CodeWingQuoteChanged, nil
	}
	var original pb.WingQuoteRequest
	if proto.Unmarshal(quote.Request, &original) != nil || original.SceneEpoch != q.SceneEpoch ||
		domain.Hex(original.UiSessionId) != quote.UISession || input.AllowBound != original.AllowBound ||
		domain.Hex(input.WingId) != domain.Hex(original.WingId) || input.WingVersion != original.WingVersion {
		return wingGrowthPlan{}, p.CodeWingQuoteChanged, nil
	}
	plan, code := buildWingGrowthPlan(role, fixture, &original)
	if code != 0 || !reflect.DeepEqual(plan, quote.Plan) {
		return wingGrowthPlan{}, p.CodeWingQuoteChanged, nil
	}
	deleted, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		return wingGrowthPlan{}, 0, err
	}
	if deleted.DeletedCount != 1 {
		return wingGrowthPlan{}, p.CodeWingQuoteChanged, nil
	}
	return plan, 0, nil
}

// commitWingAction 只处理首次写入。已有回执由原经济事务在场景、版本和报价检查之前返回。
func (s *Store) commitWingAction(ctx context.Context, a Auth, role domain.Role, q AssetRequest, operationID string) (domain.Role, uint32, *WingActionReceipt, error) {
	input := q.WingAction
	if !validWingAction(input, q.Context) || q.SceneEpoch == 0 {
		return role, p.CodeInvalidArgument, nil, nil
	}
	if q.WingGate != 0 {
		return role, q.WingGate, nil, nil
	}
	fixture := &s.Cfg.Fixture
	if fixture.Wings == nil {
		return role, p.CodeWingUnavailable, nil, nil
	}
	if !wingRefMatches(role.Wing, input.WingId, input.WingVersion) {
		return role, p.CodeWingQuoteChanged, nil, nil
	}
	out := role
	var wing domain.WingState
	receipt := &WingActionReceipt{Action: input.Action, Before: role.Wing, QuoteID: domain.Hex(input.QuoteId), UISession: domain.Hex(input.UiSessionId),
		Scene: strconv.FormatUint(q.SceneEpoch, 10), Rule: fixture.RuleVersion, Config: fixture.Hash, AllowBound: input.AllowBound}
	if input.Action == wingActionGrow {
		plan, code, err := s.claimWingGrowthQuote(ctx, a, role, q)
		if err != nil || code != 0 {
			return role, code, nil, err
		}
		out, wing = applyWingGrowthPlan(role, plan, operationID)
		receipt.Feathers = plan.Feathers
	} else {
		if role.Wing == nil || input.AppearanceTier > role.Wing.Rank || !slices.Contains(fixture.Wings.AppearanceTiers, input.AppearanceTier) {
			return role, p.CodeWingInputs, nil, nil
		}
		wing = *role.Wing
		wing.AppearanceTier, wing.Hidden = input.AppearanceTier, input.Hidden
		wing.Version++
		out.Wing = &wing
	}
	if !validRoleWing(&wing, role.ID, fixture.Wings) {
		return role, 0, nil, Fail(p.CodeOperationConflict, "翅膀变更结果无效")
	}
	if err := s.writeRoleWing(ctx, role.Wing, wing); err != nil {
		return role, 0, nil, err
	}
	receipt.Wing = wing
	return out, 0, receipt, nil
}
