// 本文件签发炼体原报价，并在角色经济事务内扣款、判定和记录原回执。
package persistence

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	AssetBodyTrainingCommit = "body_training_commit" // 永久角色经济操作类型。
	bodyTrainingQuotes      = "body_training_quotes" // 暂存每个角色最后一份报价。
)

// bodyTrainingPlan 是本次报价可复算的等级、概率、额度和钱包拆分。
type bodyTrainingPlan struct {
	Track       uint32        `bson:"track"`        // 原炼体系别。
	Before      uint32        `bson:"before"`       // 原已完成等级。
	Chance      uint32        `bson:"chance_ppm"`   // 原档位成功率。
	BeforeBonus int64         `bson:"before_bonus"` // 原累计额度。
	TargetBonus int64         `bson:"target_bonus"` // 成功时累计额度。
	Spend       domain.Wallet `bson:"spend"`        // 绑定金币优先使用后的分账。
}

// bodyTrainingStoredQuote 保留原判定样本，已受理的事务重试沿用。
type bodyTrainingStoredQuote struct {
	Binding growthQuoteBinding `bson:",inline"`      // 角色、运行服与临时界面身份。
	Plan    bodyTrainingPlan   `bson:"plan"`         // 已展示的完整费用。
	Roll    uint32             `bson:"success_roll"` // 对客户端隐藏的原均匀样本。
}

// BodyTrainingReceipt 保存已付的一次确定判定，包括失败时不降级的原档。
type BodyTrainingReceipt struct {
	QuoteID string           `bson:"quote_id"`     // 已消耗的原报价。
	Rule    uint64           `bson:"rule_version"` // 原报价规则版本。
	Config  string           `bson:"config_hash"`  // 原规则内容摘要。
	Plan    bodyTrainingPlan `bson:"plan"`         // 原条款与扣费。
	Roll    uint32           `bson:"success_roll"` // 仅供服务端审计。
	Success bool             `bson:"success"`      // 确定的判定结果。
	After   uint32           `bson:"after"`        // 当前系别确认后的等级。
}

// Proto 将已确认费用和等级恢复为原协议回执。
func (r *BodyTrainingReceipt) Proto() *pb.BodyTrainingReceipt {
	if r == nil {
		return nil
	}
	return &pb.BodyTrainingReceipt{QuoteId: domain.Bytes(r.QuoteID), Terms: r.Plan.terms(), Success: r.Success, AfterLevel: r.After}
}

// terms 发布玩家可确认的原条款。
func (plan bodyTrainingPlan) terms() *pb.BodyTrainingTerms {
	return &pb.BodyTrainingTerms{Track: plan.Track, BeforeLevel: plan.Before, TargetLevel: plan.Before + 1,
		SuccessPpm: plan.Chance, Gold: plan.Spend.Gold, BoundGold: plan.Spend.BoundGold,
		BeforeBonus: plan.BeforeBonus, TargetBonus: plan.TargetBonus}
}

// ValidBodyTrainingQuote 校验原场景、原界面和初批系别。
func ValidBodyTrainingQuote(q *pb.BodyTrainingQuoteRequest) bool {
	return q != nil && q.SceneEpoch != 0 && len(q.UiSessionId) == 16 &&
		(q.Track == devconfig.BodyHealth || q.Track == devconfig.BodyMana) && len(q.ProtoReflect().GetUnknown()) == 0
}

// ValidBodyTrainingCommit 确认正文、原报价及经济上下文完整。
func ValidBodyTrainingCommit(q *pb.BodyTrainingCommitRequest, operation *pb.OperationContext) bool {
	return q != nil && operation != nil && proto.Equal(operation, q.Operation) && len(q.QuoteId) == 16 &&
		len(q.UiSessionId) == 16 && (q.Track == devconfig.BodyHealth || q.Track == devconfig.BodyMana) &&
		len(q.ProtoReflect().GetUnknown()) == 0 && len(operation.ProtoReflect().GetUnknown()) == 0
}

// buildBodyTrainingPlan 以实际角色等级和已开档位限价，优先选绑定金币。
func buildBodyTrainingPlan(role domain.Role, fixture *devconfig.Fixture, id uint32) (bodyTrainingPlan, uint32) {
	c := fixture.BodyTraining
	track, found := c.Track(id)
	if !found {
		return bodyTrainingPlan{}, p.CodeBodyTrainingUnavailable
	}
	before := role.BodyTrainingLevels().Level(id)
	if before >= role.Level || uint64(before) >= uint64(len(c.Steps)) {
		return bodyTrainingPlan{}, p.CodeBodyTrainingInputs
	}
	step := c.Steps[before]
	total := track.GoldBase * int64(step.CostMultiplier)
	bound := min(max(role.Wallet.BoundGold, 0), total)
	spend := domain.Wallet{Gold: total - bound, BoundGold: bound}
	if !vendorCanPay(role.Wallet, spend) {
		return bodyTrainingPlan{}, p.CodeBodyTrainingInputs
	}
	return bodyTrainingPlan{Track: id, Before: before, Chance: step.SuccessPPM,
		BeforeBonus: int64(before) * track.BonusPerRank, TargetBonus: int64(before+1) * track.BonusPerRank, Spend: spend}, 0
}

// PreviewBodyTraining 在原角色围栏内签发一次尝试，不动角色资产。
func (s *Store) PreviewBodyTraining(ctx context.Context, auth Auth, request *pb.BodyTrainingQuoteRequest) (*pb.BodyTrainingQuote, error) {
	if !ValidBodyTrainingQuote(request) {
		return nil, Fail(p.CodeInvalidArgument, "炼体报价缺少原角色界面或系别")
	}
	fixture := &s.Cfg.Fixture
	if fixture.BodyTraining == nil {
		return nil, Fail(p.CodeBodyTrainingUnavailable, "本规则未开放炼体")
	}
	sample, err := rand.Int(rand.Reader, big.NewInt(int64(devconfig.BodyChancePPM)))
	if err != nil {
		return nil, err
	}
	quote := bodyTrainingStoredQuote{Binding: s.newGrowthQuote(auth, request.SceneEpoch, request.UiSessionId), Roll: uint32(sample.Uint64())}
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
		plan, code := buildBodyTrainingPlan(role, fixture, request.Track)
		if code != 0 {
			return Fail(code, "炼体等级或金币不足")
		}
		quote.Binding.Revision, quote.Plan = role.Revision, plan
		_, err = s.DB.Collection(bodyTrainingQuotes).ReplaceOne(tx, bson.M{"_id": role.ID}, quote, options.Replace().SetUpsert(true))
		return err
	})
	if err != nil {
		return nil, err
	}
	b := quote.Binding
	return &pb.BodyTrainingQuote{QuoteId: domain.Bytes(b.ID), Terms: quote.Plan.terms(), SceneEpoch: request.SceneEpoch,
		UiSessionId: domain.Bytes(b.UISession), RoleRevision: uint64(b.Revision), ExpiresAtUnixMs: uint64(b.Expires.UnixMilli())}, nil
}

// commitBodyTraining 首次确认在同一事务中核对原条款、消费报价、扣款及推进等级。
func (s *Store) commitBodyTraining(ctx context.Context, auth Auth, role domain.Role, request AssetRequest) (domain.Role, uint32, *BodyTrainingReceipt, error) {
	f := &s.Cfg.Fixture
	if f.BodyTraining == nil || request.BodyTrainingGate != 0 {
		return role, p.CodeBodyTrainingUnavailable, nil, nil
	}
	input := request.BodyTrainingCommit
	if !ValidBodyTrainingCommit(input, request.Context) || request.SceneEpoch == 0 {
		return role, p.CodeInvalidArgument, nil, nil
	}
	filter := bson.M{"_id": role.ID, "quote_id": domain.Hex(input.QuoteId)}
	collection := s.DB.Collection(bodyTrainingQuotes)
	var quote bodyTrainingStoredQuote
	err := collection.FindOne(ctx, filter).Decode(&quote)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return role, p.CodeBodyTrainingQuoteChanged, nil, nil
	}
	if err != nil {
		return role, 0, nil, err
	}
	b := quote.Binding
	if quote.Roll >= devconfig.BodyChancePPM || quote.Plan.Track != input.Track ||
		!s.matchesGrowthQuote(b, role, auth, request.Context.ExpectedRevision, request.SceneEpoch, input.UiSessionId, request.BodyTrainingAcceptedAt) {
		return role, p.CodeBodyTrainingQuoteChanged, nil, nil
	}
	plan, code := buildBodyTrainingPlan(role, f, input.Track)
	if code != 0 || plan != quote.Plan {
		return role, p.CodeBodyTrainingQuoteChanged, nil, nil
	}
	out := role
	out.Wallet.BoundGold -= plan.Spend.BoundGold
	out.Wallet.Gold -= plan.Spend.Gold
	receipt := &BodyTrainingReceipt{QuoteID: b.ID, Rule: b.Rule, Config: b.Config, Plan: plan,
		Roll: quote.Roll, Success: quote.Roll < plan.Chance, After: plan.Before}
	if receipt.Success {
		out.BodyTraining = role.BodyTrainingLevels().Advance(input.Track)
		receipt.After++
	}
	if validateDungeonHireWallet(out) != nil || !equipmentRoleArithmeticSafe(out, *f) {
		return role, p.CodeBodyTrainingInputs, nil, nil
	}
	deleted, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		return role, 0, nil, err
	}
	if deleted.DeletedCount != 1 {
		return role, p.CodeBodyTrainingQuoteChanged, nil, nil
	}
	return out, 0, receipt, nil
}
