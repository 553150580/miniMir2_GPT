// 本文件冻结报价和随机种子，原装备、原石、四钱包及已付待选在同一角色事务内变更。
package persistence

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	AssetEquipmentRefine           = "equipment_refine"        // 三个动作共用的原经济流水类型。
	equipmentRefineQuoteCollection = "equipment_refine_quotes" // 每个角色仅保留最新短期报价。
	equipmentRefineQuoteLifetime   = 30 * time.Second          // 从原界面生成到新确认的允许时间。
	equipmentRefineQuoteGrace      = 5 * time.Minute           // 给已受理操作留出重试时间，已付候选不在报价表中。
)

// EquipmentRefineReceipt 保存已提交的原操作，决定动作即使在角色换票后也沿此结果重放。
type EquipmentRefineReceipt struct {
	Action    uint32                  `bson:"action"`             // 生成、采纳或放弃。
	Before    domain.Item             `bson:"before"`             // 事务前原装备，包含被覆盖的上一份待选。
	Target    domain.Item             `bson:"target"`             // 事务后仍是同一个原装备。
	Equipped  bool                    `bson:"equipped"`           // 首次操作的原位置。
	QuoteID   string                  `bson:"quote_id,omitempty"` // 生成曾确认的原报价。
	PendingID string                  `bson:"pending_id"`         // 创建或处理的原已付编号。
	Stones    []enhancementStoneSpend `bson:"stones,omitempty"`   // 原扣石堆，决定动作为空。
	Spend     domain.Wallet           `bson:"spend"`              // 实际四类费用，决定动作全为零。
	Rule      uint64                  `bson:"rule_version"`       // 首次执行的规则版本。
	Config    string                  `bson:"config_hash"`        // 首次执行的准确配置摘要。
	Scene     string                  `bson:"scene_epoch"`        // 首次执行的场景代次。
	Seed      string                  `bson:"seed,omitempty"`     // 仅审计可复核生成结果，不向玩家公开。
}

// Proto 公开结果、原装备和已确认费用。历史回执不能给新装备覆盖更近的观察。
func (r *EquipmentRefineReceipt) Proto() *pb.EquipmentRefineReceipt {
	if r == nil {
		return nil
	}
	return &pb.EquipmentRefineReceipt{Action: r.Action, Target: r.Target.Proto(), Equipped: r.Equipped, QuoteId: domain.Bytes(r.QuoteID),
		PendingId: domain.Bytes(r.PendingID), Stones: refineStonesProto(r.Stones), Gold: r.Spend.Gold, BoundGold: r.Spend.BoundGold,
		YuanbaoUnits: r.Spend.Yuanbao, BoundYuanbaoUnits: r.Spend.BoundYuanbao}
}

// equipmentRefineStoredQuote 约束首次确认的原连接、资产水位、规则和恰一次抽样种子。
type equipmentRefineStoredQuote struct {
	RoleID       string              `bson:"_id"`           // 角色永久主键。
	ID           string              `bson:"quote_id"`      // 玩家看到并确认的原报价。
	Scene        string              `bson:"scene_epoch"`   // 原附着场景。
	UISession    string              `bson:"ui_session_id"` // 玩家打开的原洗炼界面。
	SessionEpoch int64               `bson:"session_epoch"` // 角色的原连接代次。
	RunID        uint64              `bson:"run_server_id"` // 原运行服。
	RunEpoch     int64               `bson:"run_epoch"`     // 原运行服租约代次。
	Revision     int64               `bson:"revision"`      // 报价依据的资产修订。
	Rule         uint64              `bson:"rule_version"`  // 报价使用的规则版本。
	Config       string              `bson:"config_hash"`   // 报价的全部配置摘要。
	Issued       time.Time           `bson:"issued_at"`     // 原报价的生效起点。
	Expires      time.Time           `bson:"expires_at"`    // 已展示报价允许新确认的排他终点。
	PurgeAt      time.Time           `bson:"purge_at"`      // 临时报价清理点。
	Request      []byte              `bson:"request"`       // 原材料、对象及界面授权正文。
	Plan         equipmentRefinePlan `bson:"plan"`          // 已确认前可公开的费用与绑定。
	Seed         string              `bson:"seed"`          // 对所有提交与事务重试固定的抽样种子。
}

// PreviewEquipmentRefine 当前角色只能给自己的原装备和真实选材取得报价。
func (s *Store) PreviewEquipmentRefine(ctx context.Context, a Auth, q *pb.EquipmentRefineQuoteRequest) (*pb.EquipmentRefineQuote, error) {
	fixture := &s.Cfg.Fixture
	if fixture.EquipmentRefine == nil {
		return nil, Fail(p.CodeEquipmentRefineUnavailable, "当前规则未开放洗炼")
	}
	if !ValidEquipmentRefineQuoteRequest(q) {
		return nil, Fail(p.CodeInvalidArgument, "原洗炼报价缺少附着或材料")
	}
	payload, err := proto.Marshal(q)
	if err != nil {
		return nil, err
	}
	var entropy [32]byte // 即使事务重复也沿用这份不可见的随机性。
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, err
	}
	issued := time.Now().UTC().Truncate(time.Millisecond)
	quote := equipmentRefineStoredQuote{RoleID: a.RoleID, ID: domain.ID(), Scene: strconv.FormatUint(q.SceneEpoch, 10),
		UISession: domain.Hex(q.UiSessionId), SessionEpoch: a.SessionEpoch, RunID: s.Cfg.RunID, RunEpoch: s.Epoch,
		Rule: fixture.RuleVersion, Config: fixture.Hash, Issued: issued, Expires: issued.Add(equipmentRefineQuoteLifetime),
		PurgeAt: issued.Add(equipmentRefineQuoteLifetime + equipmentRefineQuoteGrace), Request: payload, Seed: hex.EncodeToString(entropy[:])}
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
		plan, code := buildEquipmentRefinePlan(role, fixture, q)
		if code != 0 {
			return Fail(code, "原装备、材料或钱包不满足本次洗炼报价")
		}
		quote.Revision, quote.Plan = role.Revision, plan
		_, err = s.DB.Collection(equipmentRefineQuoteCollection).ReplaceOne(tc, bson.M{"_id": role.ID}, quote, options.Replace().SetUpsert(true))
		return err
	})
	if err != nil {
		return nil, err
	}
	plan := quote.Plan
	return &pb.EquipmentRefineQuote{QuoteId: domain.Bytes(quote.ID), SceneEpoch: q.SceneEpoch, Target: plan.Target.Proto(),
		Equipped: plan.Equipped, LockedSlots: domain.RefineLockedSlots(plan.LockedMask), Stones: refineStonesProto(plan.Stones),
		AllowBound: plan.AllowBound, Gold: plan.Spend.Gold, BoundGold: plan.Spend.BoundGold,
		YuanbaoUnits: plan.Spend.Yuanbao, BoundYuanbaoUnits: plan.Spend.BoundYuanbao, OutputBound: plan.OutputBound,
		ExpiresAtUnixMs: uint64(quote.Expires.UnixMilli()), RoleRevision: uint64(quote.Revision), UiSessionId: domain.Bytes(quote.UISession)}, nil
}

// ValidEquipmentRefineAction 区分付费确认与已付结果决定，防止携带无关授权或报价。
func ValidEquipmentRefineAction(input *pb.EquipmentRefineActionRequest) bool {
	if input == nil || input.Operation == nil || len(input.Operation.OpId) != 16 || !validRefineItemRef(input.Target) ||
		len(input.UiSessionId) != 16 || len(input.ProtoReflect().GetUnknown()) != 0 || len(input.Operation.ProtoReflect().GetUnknown()) != 0 {
		return false
	}
	switch input.Action {
	case refineGenerate:
		return len(input.QuoteId) == 16 && len(input.PendingId) == 0
	case refineAdopt, refineDiscard:
		return len(input.QuoteId) == 0 && len(input.PendingId) == 16 && !input.AllowBound
	}
	return false
}

// claimEquipmentRefineQuote 复算原合约再消耗一次性报价，随原资产事务整体回滚或确认。
func (s *Store) claimEquipmentRefineQuote(ctx context.Context, a Auth, role domain.Role, q AssetRequest) (equipmentRefineStoredQuote, uint32, error) {
	input := q.EquipmentRefine
	collection := s.DB.Collection(equipmentRefineQuoteCollection)
	filter := bson.M{"_id": role.ID, "quote_id": domain.Hex(input.QuoteId)}
	var quote equipmentRefineStoredQuote
	err := collection.FindOne(ctx, filter).Decode(&quote)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return quote, p.CodeEquipmentRefineChanged, nil
	}
	if err != nil {
		return quote, 0, err
	}
	fixture := &s.Cfg.Fixture
	if quote.SessionEpoch != a.SessionEpoch || quote.RunID != s.Cfg.RunID || quote.RunEpoch != s.Epoch ||
		quote.Scene != strconv.FormatUint(q.SceneEpoch, 10) || quote.UISession != domain.Hex(input.UiSessionId) || quote.Rule != fixture.RuleVersion ||
		quote.Config != fixture.Hash || !lowerHex(quote.Seed, 32) || q.EquipmentRefineAcceptedAt.Before(quote.Issued) ||
		!q.EquipmentRefineAcceptedAt.Before(quote.Expires) || quote.Revision < 1 || quote.Revision > role.Revision ||
		uint64(quote.Revision) != q.Context.ExpectedRevision {
		return quote, p.CodeEquipmentRefineChanged, nil
	}
	var original pb.EquipmentRefineQuoteRequest
	if proto.Unmarshal(quote.Request, &original) != nil || original.SceneEpoch != q.SceneEpoch || original.AllowBound != input.AllowBound ||
		!proto.Equal(original.Target, input.Target) || domain.Hex(original.UiSessionId) != quote.UISession {
		return quote, p.CodeEquipmentRefineChanged, nil
	}
	plan, code := buildEquipmentRefinePlan(role, fixture, &original)
	if code != 0 || !reflect.DeepEqual(plan, quote.Plan) {
		return quote, p.CodeEquipmentRefineChanged, nil
	}
	deleted, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		return quote, 0, err
	}
	if deleted.DeletedCount != 1 {
		return quote, 0, Fail(p.CodeOperationConflict, "原洗炼报价已被消费")
	}
	return quote, 0, nil
}

// commitEquipmentRefine 首次生成时支付并立即绑定，决定阶段只处理原待选及词条。
func (s *Store) commitEquipmentRefine(ctx context.Context, a Auth, role domain.Role, q AssetRequest, operationID string) (domain.Role, uint32, *EquipmentRefineReceipt, error) {
	input := q.EquipmentRefine
	if !ValidEquipmentRefineAction(input) || !proto.Equal(input.Operation, q.Context) || q.SceneEpoch == 0 || !equipmentRoleUnique(role) {
		return role, p.CodeInvalidArgument, nil, nil
	}
	if q.EquipmentRefineGate != 0 {
		return role, q.EquipmentRefineGate, nil, nil
	}
	fixture := &s.Cfg.Fixture
	if fixture.EquipmentRefine == nil {
		return role, p.CodeEquipmentRefineUnavailable, nil, nil
	}
	target, _, found := ownedRefineTarget(role, fixture, input.Target)
	if !found {
		return role, p.CodeEquipmentRefineChanged, nil, nil
	}
	receipt := &EquipmentRefineReceipt{Action: input.Action, Before: target, Equipped: input.Target.Equipped,
		Scene: strconv.FormatUint(q.SceneEpoch, 10), Rule: fixture.RuleVersion, Config: fixture.Hash}
	next := role
	if input.Action == refineGenerate {
		quote, code, err := s.claimEquipmentRefineQuote(ctx, a, role, q)
		if code != 0 || err != nil {
			return role, code, nil, err
		}
		candidate, ok := rollEquipmentRefine(quote.Plan, fixture.EquipmentRefine, quote.Seed)
		if !ok {
			return role, 0, nil, Fail(p.CodeEquipmentRefineInputs, "锁槽后的原词条池无法生成完整候选")
		}
		pendingID := Hash([]byte("refine-pending:" + operationID))[:32]
		target.PendingResultID, target.Refine.Candidate = pendingID, candidate
		target.Refine.LockedMask, target.Bound = quote.Plan.LockedMask, quote.Plan.OutputBound
		receipt.QuoteID, receipt.PendingID, receipt.Seed = quote.ID, pendingID, quote.Seed
		receipt.Stones, receipt.Spend = quote.Plan.Stones, quote.Plan.Spend
		uses := make(map[string]int64, len(receipt.Stones))
		for _, material := range receipt.Stones {
			uses[material.Item.ID] = material.Quantity
		}
		next.Items = spendGrowthMaterials(role.Items, uses)
		next.Wallet.Gold -= receipt.Spend.Gold
		next.Wallet.BoundGold -= receipt.Spend.BoundGold
		next.Wallet.Yuanbao -= receipt.Spend.Yuanbao
		next.Wallet.BoundYuanbao -= receipt.Spend.BoundYuanbao
	} else {
		if target.PendingResultID != domain.Hex(input.PendingId) {
			return role, p.CodeEquipmentRefineChanged, nil, nil
		}
		receipt.PendingID = target.PendingResultID
		if input.Action == refineAdopt {
			target.Refine.Current = target.Refine.Candidate
		}
		target.PendingResultID, target.Refine.Candidate, target.Refine.LockedMask = "", domain.EquipmentAffixes{}, 0
	}
	target.Version++
	next = replaceRefineTarget(next, target, input.Target.Equipped)
	if input.Action == refineAdopt && input.Target.Equipped && !equipmentRoleArithmeticSafe(next, *fixture) {
		return role, p.CodeEquipmentRefineInputs, nil, nil
	}
	receipt.Target = target
	return next, 0, receipt, nil
}

// replaceRefineTarget 替换已核验当前位置的同一原件。独立切片不污染回滚时的角色快照。
func replaceRefineTarget(role domain.Role, item domain.Item, equipped bool) domain.Role {
	if equipped {
		role.Equipped = slices.Clone(role.Equipped)
		for i := range role.Equipped {
			if role.Equipped[i].Item.ID == item.ID {
				role.Equipped[i].Item = item
				break
			}
		}
		return role
	}
	role.Items = slices.Clone(role.Items)
	for i := range role.Items {
		if role.Items[i].ID == item.ID {
			role.Items[i] = item
			break
		}
	}
	return role
}
