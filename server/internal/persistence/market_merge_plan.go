// 本文件持久化道具与金币市场的合服清退计划，并在原运行服上原子关闭新请求。
package persistence

import (
	"context"
	"errors"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	p "minimir/internal/protocol"
)

const (
	marketMergePlanCollection = "market_merge_plans"
	marketMergeGateCollection = "market_merge_run_gates"
	marketMergeStepCollection = "market_merge_steps"

	MarketMergePrepared = "prepared" // 已固定路由，仍允许原服务正常运行。
	MarketMergeFrozen   = "frozen"   // 两服均维护，等待原所有者停服和市场清退。
	MarketMergeSealed   = "sealed"   // 已清退并取得市场部分的发行校验证明。
	MarketMergeRestored = "restored" // 重新开放原服，原计划不可用于合服发布。
)

// MarketMergeAssignment 固定预演时两服原节点、连接地址及部署版本。
type MarketMergeAssignment struct {
	RunID             uint64 `bson:"run_server_id" json:"run_server_id"`           // 原运行服编号。
	GameNodeID        string `bson:"game_node_id" json:"game_node_id"`             // 原获准承载节点。
	GameWSURL         string `bson:"game_ws_url" json:"game_ws_url"`               // 原公布的连接地址。
	AssignmentVersion int64  `bson:"assignment_version" json:"assignment_version"` // 原部署版本。
}

// MarketMergeOrigin 固定两个运行服下全部永久入口的归属与当前路由版本。
type MarketMergeOrigin struct {
	OriginID     uint64 `bson:"origin_server_id" json:"origin_server_id"` // 角色永久入口。
	RunID        uint64 `bson:"run_server_id" json:"run_server_id"`       // 预演时承载该入口的原运行服。
	RouteVersion int64  `bson:"route_version" json:"route_version"`       // 原入口路由版本。
}

// MarketMergePlan 是可续的原操作，清退完成并不发布实际合服路由。
type MarketMergePlan struct {
	ID          string                  `bson:"_id" json:"plan_id"`                                 // 原计划身份。
	SourceRunID uint64                  `bson:"source_run_id" json:"source_run_id"`                 // 拟合并的源运行服。
	TargetRunID uint64                  `bson:"target_run_id" json:"target_run_id"`                 // 拟承接的目标运行服。
	State       string                  `bson:"state" json:"state"`                                 // 当前计划阶段。
	Version     int64                   `bson:"version" json:"version"`                             // 持久状态与清退步骤使用的正数版本。
	Assignments []MarketMergeAssignment `bson:"assignments" json:"assignments"`                     // 两服原部署，按编号排序。
	Origins     []MarketMergeOrigin     `bson:"origins" json:"origins"`                             // 两服全部原永久入口，按编号排序。
	PreparedAt  time.Time               `bson:"prepared_at" json:"prepared_at"`                     // 原计划创建时刻。
	FrozenAt    *time.Time              `bson:"frozen_at,omitempty" json:"frozen_at,omitempty"`     // 首次原子冻结时刻。
	SealedAt    *time.Time              `bson:"sealed_at,omitempty" json:"sealed_at,omitempty"`     // 首次核对两服清退完毕的时刻。
	RestoredAt  *time.Time              `bson:"restored_at,omitempty" json:"restored_at,omitempty"` // 恢复原区服的时刻。
	Steps       int64                   `bson:"steps" json:"steps"`                                 // 本计划实际处理的原资产及退款任务数量。
	LastStep    *MarketMergeAssetStep   `bson:"last_step,omitempty" json:"last_step,omitempty"`     // 最近一笔实际处理的资产来源。
	Proof       *MarketMergeProof       `bson:"proof,omitempty" json:"proof,omitempty"`             // 当前封账的市场校验证明。
	FreezeFrom  int64                   `bson:"freeze_from,omitempty" json:"-"`                     // 首次冻结的入参版本，用于原请求重试。
	SealFrom    int64                   `bson:"seal_from,omitempty" json:"-"`                       // 首次封账的入参版本，用于原请求重试。
	RestoreFrom int64                   `bson:"restore_from,omitempty" json:"-"`                    // 首次恢复原区服的入参版本。
}

// marketMergeRunGate 通过唯一运行服编号禁止并行计划和新的市场写入。
type marketMergeRunGate struct {
	RunID    uint64    `bson:"_id"`       // 已冻结的运行服。
	PlanID   string    `bson:"plan_id"`   // 唯一持有门禁的清退计划。
	FrozenAt time.Time `bson:"frozen_at"` // 建门时刻，恢复时验证原门仍在。
}

// PrepareMarketMerge 首次登记原计划，重试同身份返回已有计划，不会覆盖其它源和目标。
func (s *Store) PrepareMarketMerge(ctx context.Context, id string, source, target uint64) (MarketMergePlan, error) {
	if !lowerHex(id, 16) || source == 0 || target == 0 || source == target || source > math.MaxInt64 || target > math.MaxInt64 {
		return MarketMergePlan{}, Fail(p.CodeInvalidArgument, "合服市场清退的计划身份或运行服无效")
	}
	preparedAt := bossTime(time.Now())
	var out MarketMergePlan
	err := s.tx(ctx, func(tc context.Context) error {
		collection := s.DB.Collection(marketMergePlanCollection)
		var existing MarketMergePlan
		err := collection.FindOne(tc, bson.M{"_id": id}).Decode(&existing)
		if err == nil {
			if existing.SourceRunID != source || existing.TargetRunID != target {
				return Fail(p.CodeOperationConflict, "同一市场清退计划已固定原源服与目标服")
			}
			out = existing
			return nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		plan := MarketMergePlan{ID: id, SourceRunID: source, TargetRunID: target, State: MarketMergePrepared, Version: 1, PreparedAt: preparedAt}
		plan.Assignments, plan.Origins, err = s.readMarketMergeTopology(tc, plan, "open")
		if err != nil {
			return err
		}
		for _, runID := range plan.runIDs() {
			err := s.DB.Collection(marketMergeGateCollection).FindOne(tc, bson.M{"_id": runID}).Err()
			if !errors.Is(err, mongo.ErrNoDocuments) {
				if err == nil {
					return Fail(p.CodeOperationConflict, "运行服已有未释放的市场清退计划")
				}
				return err
			}
		}
		if _, err = collection.InsertOne(tc, plan); err != nil {
			return pendingMarketMergeDuplicate(err)
		}
		out = plan
		return s.writeMarketMergePlanAudit(tc, plan, "market_merge_prepared", preparedAt)
	})
	return marketMergePlanResult(out, err)
}

// FreezeMarketMerge 在同一事务维护两服、写入交易门禁并与所有现有写组形成冲突。
func (s *Store) FreezeMarketMerge(ctx context.Context, id string, expectedVersion int64) (MarketMergePlan, error) {
	if err := validateMarketMergeIdentity(id, expectedVersion); err != nil {
		return MarketMergePlan{}, err
	}
	frozenAt := bossTime(time.Now())
	var out MarketMergePlan
	err := s.tx(ctx, func(tc context.Context) error {
		plan, err := s.MarketMergeStatus(tc, id)
		if err != nil {
			return err
		}
		if plan.State != MarketMergePrepared {
			if plan.FreezeFrom == expectedVersion && plan.FrozenAt != nil {
				out = plan
				return nil
			}
			return Fail(p.CodeOperationConflict, "当前计划阶段不能冻结")
		}
		if plan.Version != expectedVersion || plan.Version == math.MaxInt64 {
			return Fail(p.CodeOperationConflict, "市场清退计划版本变化")
		}
		if err := s.confirmMarketMergeTopology(tc, plan, "open"); err != nil {
			return err
		}
		if err := s.touchMarketMergeOrigins(tc, plan); err != nil {
			return err
		}
		for _, assignment := range plan.Assignments {
			updated, err := s.DB.Collection("run_servers").UpdateOne(tc, marketMergeAssignmentFilter(assignment, "open"),
				bson.M{"$set": bson.M{"state": "maintenance"}})
			if err := requireMarketMergeMatch(updated, err, "原运行服部署已经变化"); err != nil {
				return err
			}
			if _, err = s.DB.Collection(marketMergeGateCollection).InsertOne(tc, marketMergeRunGate{RunID: assignment.RunID, PlanID: plan.ID, FrozenAt: frozenAt}); err != nil {
				if mongo.IsDuplicateKeyError(err) {
					return Fail(p.CodeOperationConflict, "原运行服已有其它市场清退门禁")
				}
				return err
			}
			if err := s.bumpMarketMergeGuards(tc, assignment.RunID); err != nil {
				return err
			}
		}
		plan.State, plan.Version, plan.FreezeFrom, plan.FrozenAt = MarketMergeFrozen, expectedVersion+1, expectedVersion, &frozenAt
		if err := s.replaceMarketMergePlan(tc, plan, expectedVersion); err != nil {
			return err
		}
		out = plan
		return s.writeMarketMergePlanAudit(tc, plan, "market_merge_frozen", frozenAt)
	})
	return marketMergePlanResult(out, err)
}

// MarketMergeStatus 读取原计划和当前进度，不推进任何市场或路由。
func (s *Store) MarketMergeStatus(ctx context.Context, id string) (MarketMergePlan, error) {
	if !lowerHex(id, 16) {
		return MarketMergePlan{}, Fail(p.CodeInvalidArgument, "市场清退计划身份无效")
	}
	var plan MarketMergePlan
	err := s.DB.Collection(marketMergePlanCollection).FindOne(ctx, bson.M{"_id": id}).Decode(&plan)
	return plan, authorityReadError(err, p.CodeOperationConflict, "市场清退原计划不存在")
}

// runIDs 始终使用同一锁顺序，避免两个相关计划反向争抢运行服。
func (p MarketMergePlan) runIDs() [2]uint64 {
	return [2]uint64{min(p.SourceRunID, p.TargetRunID), max(p.SourceRunID, p.TargetRunID)}
}

func validateMarketMergeIdentity(id string, version int64) error {
	if !lowerHex(id, 16) || version <= 0 {
		return Fail(p.CodeInvalidArgument, "市场清退计划身份或期望版本无效")
	}
	return nil
}

func requireMarketMergeMatch(result *mongo.UpdateResult, err error, message string) error {
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, message)
	}
	return nil
}

func pendingMarketMergeDuplicate(err error) error {
	if mongo.IsDuplicateKeyError(err) {
		return Fail(p.CodeOperationPending, "市场清退原请求正在确认，请沿原身份重试")
	}
	return err
}

func (s *Store) replaceMarketMergePlan(ctx context.Context, plan MarketMergePlan, oldVersion int64) error {
	result, err := s.DB.Collection(marketMergePlanCollection).ReplaceOne(ctx, bson.M{"_id": plan.ID, "version": oldVersion}, plan)
	if err == nil && result.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "市场清退原计划版本变化")
	}
	return err
}

func marketMergePlanResult(plan MarketMergePlan, err error) (MarketMergePlan, error) {
	if err != nil {
		return MarketMergePlan{}, err
	}
	return plan, nil
}

func (s *Store) writeMarketMergePlanAudit(ctx context.Context, plan MarketMergePlan, kind string, at time.Time) error {
	_, err := s.DB.Collection("audit_outbox").InsertOne(ctx, bson.M{
		"_id": marketFactID("market-merge-plan-audit", plan.ID, kind), "kind": kind, "plan_id": plan.ID,
		"source_run_id": plan.SourceRunID, "target_run_id": plan.TargetRunID, "version": plan.Version,
		"assignments": plan.Assignments, "origins": plan.Origins, "proof": plan.Proof, "at": at,
	})
	return err
}
