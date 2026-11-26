// 本文件保管短期打造报价和不可见随机值，通过同一资产事务落地原报价。
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

const AssetForgeCommit = "forge_commit"     // 由原资产流水持久化并幂等重放。
const forgeQuoteCollection = "forge_quotes" // 每角色最多保留一条未确认报价。
const forgeQuoteDuration = 30 * time.Second // 超过这一窗口的报价须由玩家重新确认。

// forgeStoredQuote 把原输入和私有随机值固定在同一存储记录，绝不向客户端暴露随机值。
type forgeStoredQuote struct {
	RoleID         string    `bson:"_id"`              // 报价归属。
	ID             string    `bson:"quote_id"`         // 当前可确认的报价编号。
	Scene          string    `bson:"scene_epoch"`      // 首次报价时的场景代次。
	SessionEpoch   int64     `bson:"session_epoch"`    // 首次报价时的角色连接代次。
	RunID          uint64    `bson:"run_server_id"`    // 签发报价时的运行服。
	RunEpoch       int64     `bson:"run_epoch"`        // 签发报价时的实际节点租约代次。
	Revision       int64     `bson:"revision"`         // 首次报价依据的角色版本。
	Rule           uint64    `bson:"rule_version"`     // 显式规则版本。
	Config         string    `bson:"config_hash"`      // 价格或掉率变化也会更换的配置摘要。
	Expires        time.Time `bson:"expires_at"`       // 第一个不再接受新操作的时刻。
	Request        []byte    `bson:"request"`          // 原始选材请求，提交时重新计算。
	Plan           forgePlan `bson:"plan"`             // 原玩家确认条款，不含隐藏随机值。
	SuccessRoll    uint32    `bson:"success_roll"`     // 独立的成功抽样值。
	LuckRoll       uint32    `bson:"luck_roll"`        // 独立的初始幸运触发值。
	LuckWeightRoll uint64    `bson:"luck_weight_roll"` // 幸运触发后按权重选择具体档位。
}

// ForgeResultReceipt 是一次已经成交的原打造事实，包含真失败及首次成功的原实例。
type ForgeResultReceipt struct {
	QuoteID string       `bson:"quote_id"`         // 回执关联的原报价。
	Scene   string       `bson:"scene_epoch"`      // 成交发生的原场景。
	Plan    forgePlan    `bson:"plan"`             // 冻结全部输入及实际上扣的金币。
	Success bool         `bson:"success"`          // 失败同样是已结算的一次尝试。
	Output  *domain.Item `bson:"output,omitempty"` // 失败没有成品，成功只生成这个原件。
}

// Proto 将永久回执投影为有限公开字段。完整原输入保存在永久账本和审计中。
func (r *ForgeResultReceipt) Proto() *pb.ForgeReceipt {
	if r == nil {
		return nil
	}
	result := &pb.ForgeReceipt{QuoteId: domain.Bytes(r.QuoteID), Success: r.Success, Gold: r.Plan.Spend.Gold, BoundGold: r.Plan.Spend.BoundGold}
	if r.Output != nil {
		item := r.Output
		result.Output = &pb.ForgeOutput{InstanceId: domain.Bytes(item.ID), ItemId: item.ItemID, Bound: item.Bound,
			Quality: item.Quality, InitialLuck: item.InitialLuck}
	}
	return result
}

// PreviewForge 从原会话角色生成唯一短期报价。读取和写入报价沿实际租约及账号围栏。
func (s *Store) PreviewForge(ctx context.Context, a Auth, q *pb.ForgeQuoteRequest) (*pb.ForgeQuote, error) {
	fixture := &s.Cfg.Fixture
	if fixture.Forge == nil {
		return nil, Fail(p.CodeForgeUnavailable, "本规则未开放打造")
	}
	if q == nil || q.SceneEpoch == 0 || len(q.Ores) == 0 || len(q.Ores) > 8 || len(q.ProtoReflect().GetUnknown()) != 0 {
		return nil, Fail(p.CodeInvalidArgument, "打造报价缺少原场景或原黑铁实例")
	}
	payload, err := proto.Marshal(q)
	if err != nil {
		return nil, err
	}
	quote := forgeStoredQuote{RoleID: a.RoleID, ID: domain.ID(), Scene: strconv.FormatUint(q.SceneEpoch, 10),
		SessionEpoch: a.SessionEpoch, RunID: s.Cfg.RunID, RunEpoch: s.Epoch, Rule: fixture.RuleVersion, Config: fixture.Hash,
		Expires: time.Now().Add(forgeQuoteDuration).UTC().Truncate(time.Millisecond), Request: payload}
	if err := quote.prepareRolls(fixture.Forge.LuckWeights); err != nil {
		return nil, err
	}
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
		plan, code := buildForgePlan(role, fixture, q)
		if code != 0 {
			return Fail(code, "打造报价未满足实际投入条件")
		}
		quote.Revision, quote.Plan = role.Revision, plan
		_, err = s.DB.Collection(forgeQuoteCollection).ReplaceOne(tc, bson.M{"_id": role.ID}, quote, options.Replace().SetUpsert(true))
		return err
	})
	if err != nil {
		return nil, err
	}
	return quote.public(q.SceneEpoch), nil
}

// prepareRolls 在收到报价时生成互相独立的服务器抽样，不在同一原报价重试时重新抽取。
func (q *forgeStoredQuote) prepareRolls(luckWeights []uint32) error {
	var total uint64
	for _, weight := range luckWeights {
		total += uint64(weight)
	}
	if total == 0 {
		return Fail(p.CodeForgeUnavailable, "打造幸运权重不可用")
	}
	success, err := rand.Int(rand.Reader, big.NewInt(int64(forgeMillion)))
	if err != nil {
		return err
	}
	luck, err := rand.Int(rand.Reader, big.NewInt(int64(forgeMillion)))
	if err != nil {
		return err
	}
	weight, err := rand.Int(rand.Reader, new(big.Int).SetUint64(total))
	if err != nil {
		return err
	}
	q.SuccessRoll, q.LuckRoll, q.LuckWeightRoll = uint32(success.Uint64()), uint32(luck.Uint64()), weight.Uint64()
	return nil
}

// public 仅返回原输入、钱包分账、实际概率、目标装备及原报价确认边界。
func (q forgeStoredQuote) public(scene uint64) *pb.ForgeQuote {
	plan := q.Plan
	result := &pb.ForgeQuote{QuoteId: domain.Bytes(q.ID), SceneEpoch: scene, Blueprint: plan.Blueprint.Proto(),
		AllowBound: plan.AllowBound, OutputItemId: plan.OutputItemID, Quality: plan.Quality, RequiredLevel: plan.RequiredLevel,
		Slot: plan.Slot, SuccessPpm: plan.SuccessPPM, Gold: plan.Spend.Gold, BoundGold: plan.Spend.BoundGold, OutputBound: plan.OutputBound,
		ExpiresAtUnixMs: uint64(q.Expires.UnixMilli()), RoleRevision: uint64(q.Revision)}
	for _, ore := range plan.Ores {
		result.Ores = append(result.Ores, ore.Proto())
	}
	return result
}

// commitForge 在查询原操作回执之后核对首次提交的报价条件。原材料、金币、产物和审计一起提交。
func (s *Store) commitForge(ctx context.Context, a Auth, role domain.Role, q AssetRequest, operationID string) (domain.Role, uint32, *ForgeResultReceipt, error) {
	if q.ForgeCommit == nil || !proto.Equal(q.ForgeCommit.Operation, q.Context) || len(q.ForgeCommit.QuoteId) != 16 ||
		len(q.ForgeCommit.ProtoReflect().GetUnknown()) != 0 {
		return role, p.CodeInvalidArgument, nil, nil
	}
	if q.ForgeGate != 0 {
		return role, q.ForgeGate, nil, nil
	}
	fixture := &s.Cfg.Fixture
	if fixture.Forge == nil {
		return role, p.CodeForgeUnavailable, nil, nil
	}
	filter := bson.M{"_id": role.ID, "quote_id": domain.Hex(q.ForgeCommit.QuoteId)}
	var quote forgeStoredQuote
	err := s.DB.Collection(forgeQuoteCollection).FindOne(ctx, filter).Decode(&quote)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return role, p.CodeForgeQuoteChanged, nil, nil
	}
	if err != nil {
		return role, 0, nil, err
	}
	if q.SceneEpoch == 0 || quote.Scene != strconv.FormatUint(q.SceneEpoch, 10) || quote.SessionEpoch != a.SessionEpoch ||
		quote.RunID != s.Cfg.RunID || quote.RunEpoch != s.Epoch || !q.ForgeAcceptedAt.Before(quote.Expires) ||
		quote.Revision != role.Revision || q.Context.ExpectedRevision != uint64(quote.Revision) ||
		quote.Rule != fixture.RuleVersion || quote.Config != fixture.Hash {
		return role, p.CodeForgeQuoteChanged, nil, nil
	}
	var request pb.ForgeQuoteRequest
	if proto.Unmarshal(quote.Request, &request) != nil || request.SceneEpoch != q.SceneEpoch {
		return role, p.CodeForgeQuoteChanged, nil, nil
	}
	plan, code := buildForgePlan(role, fixture, &request)
	if code != 0 || !reflect.DeepEqual(plan, quote.Plan) {
		return role, p.CodeForgeQuoteChanged, nil, nil
	}
	deleted, err := s.DB.Collection(forgeQuoteCollection).DeleteOne(ctx, filter)
	if err != nil {
		return role, 0, nil, err
	}
	if deleted.DeletedCount != 1 {
		return role, p.CodeForgeQuoteChanged, nil, nil
	}
	out, receipt := applyForgePlan(role, fixture, quote, operationID)
	return out, 0, &receipt, nil
}
