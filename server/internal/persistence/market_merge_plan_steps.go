// 本文件按持久版本推进逐笔市场清退、封账核证以及原区服恢复。
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

// MarketMergeStepReceipt 固定一笔清退或已排空的原请求结果，不重复保存完整路由图。
type MarketMergeStepReceipt struct {
	ID              string                `bson:"_id" json:"-"`                           // 原计划与步骤请求组成的记录键。
	PlanID          string                `bson:"plan_id" json:"plan_id"`                 // 已冻结的清退计划。
	RequestID       string                `bson:"request_id" json:"request_id"`           // 原步骤的幂等编号。
	ExpectedVersion int64                 `bson:"expected_version" json:"-"`              // 原请求携带的期望计划版本。
	Version         int64                 `bson:"version" json:"version"`                 // 该次步骤提交后的原版本。
	Asset           *MarketMergeAssetStep `bson:"asset,omitempty" json:"asset,omitempty"` // 实际处理的一份原托管。
	Done            bool                  `bson:"done" json:"done"`                       // 此次步骤观察到两个运行服均无市场待处理项。
	AppliedAt       time.Time             `bson:"applied_at" json:"applied_at"`           // 原步骤提交所记录的时刻。
}

// StepMarketMerge 在原清退计划下推进最多一笔托管。重放只返回原步骤，即使之后已处理更多资产。
func (s *Store) StepMarketMerge(ctx context.Context, id string, expectedVersion int64, requestID string) (MarketMergeStepReceipt, error) {
	if err := validateMarketMergeIdentity(id, expectedVersion); err != nil {
		return MarketMergeStepReceipt{}, err
	}
	if !lowerHex(requestID, 16) {
		return MarketMergeStepReceipt{}, Fail(p.CodeInvalidArgument, "市场清退步骤请求身份无效")
	}
	key := marketFactID("market-merge-step", id, requestID)
	at := bossTime(time.Now())
	var out MarketMergeStepReceipt
	err := s.tx(ctx, func(tc context.Context) error {
		steps := s.DB.Collection(marketMergeStepCollection)
		var old MarketMergeStepReceipt
		err := steps.FindOne(tc, bson.M{"_id": key}).Decode(&old)
		if err == nil {
			if old.PlanID != id || old.RequestID != requestID || old.ExpectedVersion != expectedVersion {
				return Fail(p.CodeOperationConflict, "原市场步骤请求的内容已变化")
			}
			out = old
			return nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		plan, err := s.MarketMergeStatus(tc, id)
		if err != nil {
			return err
		}
		if plan.State != MarketMergeFrozen || plan.Version != expectedVersion || plan.Version == math.MaxInt64 {
			return Fail(p.CodeOperationConflict, "市场清退计划未冻结或步骤版本变化")
		}
		if err := s.confirmMarketMergeFrozen(tc, plan, true); err != nil {
			return err
		}
		receipt := MarketMergeStepReceipt{ID: key, PlanID: id, RequestID: requestID, ExpectedVersion: expectedVersion,
			Version: expectedVersion + 1, Done: true, AppliedAt: at}
		for _, runID := range plan.runIDs() {
			asset, err := s.drainMarketMergeRun(tc, id, runID, at)
			if err != nil {
				return err
			}
			if asset.Kind == "" {
				continue
			}
			receipt.Asset, receipt.Done = &asset, false
			plan.LastStep, plan.Steps = &asset, plan.Steps+1
			break
		}
		plan.Version = receipt.Version
		if err := s.replaceMarketMergePlan(tc, plan, expectedVersion); err != nil {
			return err
		}
		if _, err = steps.InsertOne(tc, receipt); err != nil {
			return pendingMarketMergeDuplicate(err)
		}
		out = receipt
		return nil
	})
	if err != nil {
		return MarketMergeStepReceipt{}, err
	}
	return out, nil
}

// SealMarketMerge 固定闭门、无有效所有者、无市场债务的原路由证明。
func (s *Store) SealMarketMerge(ctx context.Context, id string, expectedVersion int64) (MarketMergePlan, error) {
	if err := validateMarketMergeIdentity(id, expectedVersion); err != nil {
		return MarketMergePlan{}, err
	}
	at := bossTime(time.Now())
	var out MarketMergePlan
	err := s.tx(ctx, func(tc context.Context) error {
		plan, err := s.MarketMergeStatus(tc, id)
		if err != nil {
			return err
		}
		if plan.State == MarketMergeSealed && plan.SealFrom == expectedVersion {
			out, err = s.requireMarketMergeProof(tc, id, plan.Version, false)
			return err
		}
		if plan.State != MarketMergeFrozen || plan.Version != expectedVersion || plan.Version == math.MaxInt64 {
			return Fail(p.CodeOperationConflict, "当前市场清退阶段或版本不能封账")
		}
		if err := s.confirmMarketMergeFrozen(tc, plan, true); err != nil {
			return err
		}
		if err := s.requireNoMarketMergeDebt(tc, plan); err != nil {
			return err
		}
		digest, err := marketMergeTopologyDigest(plan)
		if err != nil {
			return err
		}
		plan.State, plan.SealFrom, plan.Version, plan.SealedAt = MarketMergeSealed, expectedVersion, expectedVersion+1, &at
		plan.Proof = &MarketMergeProof{PlanID: id, PlanVersion: plan.Version, TopologySHA256: digest, SealedAt: at}
		if err := s.replaceMarketMergePlan(tc, plan, expectedVersion); err != nil {
			return err
		}
		out = plan
		return s.writeMarketMergePlanAudit(tc, plan, "market_merge_sealed", at)
	})
	return marketMergePlanResult(out, err)
}

// VerifyMarketMergeRouteRelease 在只读事务中核对当前封账证明。仅检查市场部分，不发布合服映射。
func (s *Store) VerifyMarketMergeRouteRelease(ctx context.Context, id string, expectedVersion int64) (MarketMergePlan, error) {
	if err := validateMarketMergeIdentity(id, expectedVersion); err != nil {
		return MarketMergePlan{}, err
	}
	var out MarketMergePlan
	err := s.tx(ctx, func(tc context.Context) error {
		var err error
		out, err = s.requireMarketMergeProof(tc, id, expectedVersion, false)
		return err
	})
	return marketMergePlanResult(out, err)
}

// requireMarketMergeProof 供将来的合服发布在同一事务重新核验，写模式会竞争原部署、入口及租约文档。
func (s *Store) requireMarketMergeProof(ctx context.Context, id string, version int64, write bool) (MarketMergePlan, error) {
	plan, err := s.MarketMergeStatus(ctx, id)
	if err != nil {
		return MarketMergePlan{}, err
	}
	proof := plan.Proof
	if plan.State != MarketMergeSealed || plan.Version != version || plan.SealedAt == nil || proof == nil ||
		proof.PlanID != id || proof.PlanVersion != version || !plan.SealedAt.Equal(proof.SealedAt) {
		return MarketMergePlan{}, Fail(p.CodeOperationConflict, "当前市场清退计划没有该版本的封账证明")
	}
	digest, err := marketMergeTopologyDigest(plan)
	if err != nil {
		return MarketMergePlan{}, err
	}
	if proof.TopologySHA256 != digest {
		return MarketMergePlan{}, Fail(p.CodeOperationConflict, "市场清退封账的完整路由身份已变化")
	}
	if err := s.confirmMarketMergeFrozen(ctx, plan, write); err != nil {
		return MarketMergePlan{}, err
	}
	if err := s.requireNoMarketMergeDebt(ctx, plan); err != nil {
		return MarketMergePlan{}, err
	}
	return plan, nil
}

// RestoreMarketMerge 只重新开放本计划封账的原两个运行服，并使该计划不再出具发布证明。
func (s *Store) RestoreMarketMerge(ctx context.Context, id string, expectedVersion int64) (MarketMergePlan, error) {
	if err := validateMarketMergeIdentity(id, expectedVersion); err != nil {
		return MarketMergePlan{}, err
	}
	at := bossTime(time.Now())
	var out MarketMergePlan
	err := s.tx(ctx, func(tc context.Context) error {
		current, err := s.MarketMergeStatus(tc, id)
		if err != nil {
			return err
		}
		if current.State == MarketMergeRestored && current.RestoreFrom == expectedVersion {
			out = current
			return nil
		}
		if expectedVersion == math.MaxInt64 {
			return Fail(p.CodeOperationConflict, "市场清退计划版本无法继续递增")
		}
		plan, err := s.requireMarketMergeProof(tc, id, expectedVersion, true)
		if err != nil {
			return err
		}
		for _, assignment := range plan.Assignments {
			updated, err := s.DB.Collection("run_servers").UpdateOne(tc, marketMergeAssignmentFilter(assignment, "maintenance"),
				bson.M{"$set": bson.M{"state": "open"}})
			if err := requireMarketMergeMatch(updated, err, "原区服部署发生变化，不能恢复"); err != nil {
				return err
			}
			deleted, err := s.DB.Collection(marketMergeGateCollection).DeleteOne(tc, marketMergeGateFilter(plan, assignment.RunID))
			if err != nil {
				return err
			}
			if deleted.DeletedCount != 1 {
				return Fail(p.CodeOperationConflict, "原市场门禁发生变化，不能恢复")
			}
		}
		plan.State, plan.RestoreFrom, plan.Version, plan.RestoredAt = MarketMergeRestored, expectedVersion, expectedVersion+1, &at
		plan.Proof = nil
		if err := s.replaceMarketMergePlan(tc, plan, expectedVersion); err != nil {
			return err
		}
		out = plan
		return s.writeMarketMergePlanAudit(tc, plan, "market_merge_restored_original_runs", at)
	})
	return marketMergePlanResult(out, err)
}
