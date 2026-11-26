// 本文件校验市场清退所固定的完整路由、休眠运行服及组写屏障。
package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	p "minimir/internal/protocol"
)

// MarketMergeProof 只证明指定闭门计划的市场清退完成，不包含正式合服的其它门禁。
type MarketMergeProof struct {
	PlanID         string    `bson:"plan_id" json:"plan_id"`                 // 封账计划的永久身份。
	PlanVersion    int64     `bson:"plan_version" json:"plan_version"`       // 首次封账的计划版本。
	TopologySHA256 string    `bson:"topology_sha256" json:"topology_sha256"` // 完整原部署和入口图的确定摘要。
	SealedAt       time.Time `bson:"sealed_at" json:"sealed_at"`             // 在同一事务核对市场无债务后的时刻。
}

// readMarketMergeTopology 读取两份实际部署和两服全部永久入口，保持确定排序。
func (s *Store) readMarketMergeTopology(ctx context.Context, plan MarketMergePlan, state string) ([]MarketMergeAssignment, []MarketMergeOrigin, error) {
	runIDs := plan.runIDs()
	assignments := make([]MarketMergeAssignment, 0, len(runIDs))
	for _, id := range runIDs {
		var stored runAssignment
		err := s.DB.Collection("run_servers").FindOne(ctx, bson.M{"_id": id, "state": state}).Decode(&stored)
		if err != nil {
			return nil, nil, authorityReadError(err, p.CodeOperationConflict, "源服或目标服的原部署不可用")
		}
		// 部署形状与签票一致。维护只改变开关，原节点和地址仍须合法。
		stored.State = "open"
		if !openAssignment(stored) {
			return nil, nil, Fail(p.CodeOperationConflict, "市场清退的原部署不合法")
		}
		assignments = append(assignments, MarketMergeAssignment{RunID: stored.RunID, GameNodeID: stored.GameNodeID,
			GameWSURL: stored.GameWSURL, AssignmentVersion: stored.AssignmentVersion})
	}
	cursor, err := s.DB.Collection("origin_routes").Find(ctx, bson.M{"run_server_id": bson.M{"$in": runIDs}},
		options.Find().SetProjection(bson.M{"_id": 1, "run_server_id": 1, "route_version": 1}).SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, nil, err
	}
	var stored []originRoute
	if err := cursor.All(ctx, &stored); err != nil {
		return nil, nil, err
	}
	var source, target bool
	origins := make([]MarketMergeOrigin, 0, len(stored))
	for _, route := range stored {
		if route.OriginID == 0 || route.Version <= 0 || route.RunID != plan.SourceRunID && route.RunID != plan.TargetRunID {
			return nil, nil, Fail(p.CodeOperationConflict, "市场清退的永久入口映射不合法")
		}
		source = source || route.RunID == plan.SourceRunID
		target = target || route.RunID == plan.TargetRunID
		origins = append(origins, MarketMergeOrigin{OriginID: route.OriginID, RunID: route.RunID, RouteVersion: route.Version})
	}
	if !source || !target {
		return nil, nil, Fail(p.CodeOperationConflict, "市场清退的源服或目标服缺少永久入口")
	}
	return assignments, origins, nil
}

// confirmMarketMergeTopology 拒绝部署、全入口集合、归属或任一个版本变化。
func (s *Store) confirmMarketMergeTopology(ctx context.Context, plan MarketMergePlan, state string) error {
	assignments, origins, err := s.readMarketMergeTopology(ctx, plan, state)
	if err != nil {
		return err
	}
	if !slices.Equal(assignments, plan.Assignments) || !slices.Equal(origins, plan.Origins) {
		return Fail(p.CodeOperationConflict, "市场清退预演后的部署或永久入口发生变化")
	}
	return nil
}

func marketMergeAssignmentFilter(assignment MarketMergeAssignment, state string) bson.M {
	return bson.M{"_id": assignment.RunID, "state": state, "game_node_id": assignment.GameNodeID,
		"game_ws_url": assignment.GameWSURL, "assignment_version": assignment.AssignmentVersion}
}

// touchMarketMergeOrigins 让有写入的阶段与既有入口上的并发版本变更形成冲突。
func (s *Store) touchMarketMergeOrigins(ctx context.Context, plan MarketMergePlan) error {
	models := make([]mongo.WriteModel, 0, len(plan.Origins))
	for _, route := range plan.Origins {
		models = append(models, mongo.NewUpdateOneModel().SetFilter(bson.M{
			"_id": route.OriginID, "run_server_id": route.RunID, "route_version": route.RouteVersion,
		}).SetUpdate(bson.M{"$inc": bson.M{"market_merge_check_seq": int64(1)}}))
	}
	result, err := s.DB.Collection("origin_routes").BulkWrite(ctx, models)
	if err == nil && result.MatchedCount != int64(len(models)) {
		return Fail(p.CodeOperationConflict, "市场清退的永久入口版本已经变化")
	}
	return err
}

// bumpMarketMergeGuards 实际写入每组序列，使冻结与并发交易和入场确立提交次序。
// 原控制版本、租约、组所有者及运行代次全部保留供原 game 正常排空。
func (s *Store) bumpMarketMergeGuards(ctx context.Context, runID uint64) error {
	var control Run
	err := s.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": runID}).Decode(&control)
	if err != nil {
		return authorityReadError(err, p.CodeOperationConflict, "缺少原运行服控制记录")
	}
	neverStarted := control.Epoch == 0 && control.Owner == "" && control.State == "stopped" && control.ControlVersion == 0
	if !neverStarted && (control.Epoch <= 0 || control.ControlVersion <= 0 || !lowerHex(control.Owner, 16) ||
		control.State != "active" && control.State != "stopped") {
		return Fail(p.CodeOperationConflict, "原运行服的组写屏障身份不合法")
	}
	models := make([]mongo.WriteModel, 0, runGuardGroups)
	for group := 0; group < runGuardGroups; group++ {
		filter := bson.M{"_id": fmt.Sprintf("%d:%d", runID, group), "owner_instance": control.Owner,
			"run_epoch": control.Epoch, "control_version": control.ControlVersion}
		if neverStarted {
			// 从未注册的服没有组记录，初始化空身份只为留下真实写序列。
			filter["$or"] = bson.A{bson.M{"seq": bson.M{"$lt": int64(math.MaxInt64)}}, bson.M{"seq": bson.M{"$exists": false}}}
		} else {
			filter["seq"] = bson.M{"$lt": int64(math.MaxInt64)}
		}
		models = append(models, mongo.NewUpdateOneModel().SetFilter(filter).SetUpsert(neverStarted).
			SetUpdate(bson.M{"$inc": bson.M{"seq": int64(1)}}))
	}
	result, err := s.DB.Collection("run_guards").BulkWrite(ctx, models)
	if err == nil && result.MatchedCount+result.UpsertedCount != int64(len(models)) {
		return Fail(p.CodeOperationConflict, "市场冻结未覆盖全部原组写屏障")
	}
	return err
}

// marketMergeIdleFilter 采用数据库时钟，失效旧租约与已正常停服都不再是可写所有者。
func marketMergeIdleFilter(runID uint64) bson.M {
	return bson.M{"_id": runID, "$or": bson.A{
		bson.M{"state": "stopped"},
		bson.M{"state": "active", "$expr": bson.M{"$lte": bson.A{"$lease_until", "$$NOW"}}},
	}}
}

// marketMergeGateFilter 固定两服关闭时原子创建的计划、运行服和建门时刻。
func marketMergeGateFilter(plan MarketMergePlan, runID uint64) bson.M {
	return bson.M{"_id": runID, "plan_id": plan.ID, "frozen_at": plan.FrozenAt}
}

// confirmMarketMergeFrozen 校验原两服维护与无有效 game 租约。写入阶段同时钉住拓扑和租约文档。
func (s *Store) confirmMarketMergeFrozen(ctx context.Context, plan MarketMergePlan, write bool) error {
	if plan.FrozenAt == nil {
		return Fail(p.CodeOperationConflict, "市场清退计划尚未冻结")
	}
	if err := s.confirmMarketMergeTopology(ctx, plan, "maintenance"); err != nil {
		return err
	}
	for _, assignment := range plan.Assignments {
		runID := assignment.RunID
		if err := s.DB.Collection(marketMergeGateCollection).FindOne(ctx, marketMergeGateFilter(plan, runID)).Err(); err != nil {
			return authorityReadError(err, p.CodeOperationConflict, "原市场清退门禁已变化")
		}
		if err := s.DB.Collection("run_control").FindOne(ctx, marketMergeIdleFilter(runID)).Err(); err != nil {
			return authorityReadError(err, p.CodeLineRecovering, "源服或目标服仍有有效游戏所有者")
		}
		if !write {
			continue
		}
		// 与迟到续租、停机释放和其它部署操作竞争，而不改写所有权。
		update := bson.M{"$inc": bson.M{"market_merge_check_seq": int64(1)}}
		result, err := s.DB.Collection("run_control").UpdateOne(ctx, marketMergeIdleFilter(runID), update)
		if err := requireMarketMergeMatch(result, err, "市场清退时运行服所有权发生变化"); err != nil {
			return err
		}
		result, err = s.DB.Collection("run_servers").UpdateOne(ctx, marketMergeAssignmentFilter(assignment, "maintenance"), update)
		if err := requireMarketMergeMatch(result, err, "市场清退期间原部署发生变化"); err != nil {
			return err
		}
		result, err = s.DB.Collection(marketMergeGateCollection).UpdateOne(ctx, marketMergeGateFilter(plan, runID), update)
		if err := requireMarketMergeMatch(result, err, "市场清退期间原门禁发生变化"); err != nil {
			return err
		}
	}
	if write {
		return s.touchMarketMergeOrigins(ctx, plan)
	}
	return nil
}

func (s *Store) requireNoMarketMergeDebt(ctx context.Context, plan MarketMergePlan) error {
	for _, runID := range plan.runIDs() {
		debt, err := s.marketMergeRunHasDebt(ctx, runID)
		if err != nil {
			return err
		}
		if debt {
			return Fail(p.CodeOperationConflict, "原运行服还有委托、预购冻结或待执行退款")
		}
	}
	return nil
}

// marketMergeTopologyDigest 固定清退方向、两份部署和完整永久入口图。
func marketMergeTopologyDigest(plan MarketMergePlan) (string, error) {
	body, err := json.Marshal(struct {
		Source      uint64
		Target      uint64
		Assignments []MarketMergeAssignment
		Origins     []MarketMergeOrigin
	}{plan.SourceRunID, plan.TargetRunID, plan.Assignments, plan.Origins})
	if err != nil {
		return "", err
	}
	return Hash(body), nil
}
