// 账号节点对游戏启动实例和整组运行服的原子授权。

package persistence

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
)

// gameBoot 固定首次成功注册的节点和完整请求集合。旧进程失权后不能用原启动身份重新申请。
type gameBoot struct {
	Instance string   `bson:"_id"`
	NodeID   string   `bson:"game_node_id"`
	RunIDs   []uint64 `bson:"run_server_ids"`
}

// ApproveGame 保留原单运行服注册入口，仍受相同事务和启动身份校验保护。
func (s *Store) ApproveGame(ctx context.Context, node, instance string, runID uint64) (Run, error) {
	grants, err := s.ApproveGameRuns(ctx, node, instance, []uint64{runID})
	if err != nil {
		return Run{}, err
	}
	return grants[0], nil
}

// ApproveGameRuns 在一个多数派事务中申请节点、请求的全部运行服和写围栏。
// 同一启动实例仅能重放仍然有效的完整原授权。任一运行服冲突会回滚其余变更。
func (s *Store) ApproveGameRuns(ctx context.Context, node, instance string, runIDs []uint64) ([]Run, error) {
	if !devconfig.ValidGameNodeID(node) || len(instance) != 32 {
		return nil, Fail(p.CodeUnauthenticated, "游戏节点或启动身份无效")
	}
	if decoded, err := hex.DecodeString(instance); err != nil || hex.EncodeToString(decoded) != instance {
		return nil, Fail(p.CodeUnauthenticated, "游戏启动实例标识无效")
	}
	if err := devconfig.ValidateHostedRunIDs(runIDs); err != nil {
		return nil, Fail(p.CodeInvalidArgument, "请求的运行服集合无效")
	}
	requested := slices.Clone(runIDs)
	slices.Sort(requested)
	var grants []Run
	err := s.tx(ctx, func(tc context.Context) error {
		grants = nil
		// 写入账号节点授权水位，使新批准与账号节点失权或换代竞争同一控制文档。
		approval, err := s.DB.Collection("node_leases").UpdateOne(tc,
			bson.M{"_id": "singleton:account", "owner_instance": s.Instance, "$expr": leaseFilter()},
			bson.M{"$inc": bson.M{"grant_seq": 1}})
		if err != nil {
			return err
		}
		if approval.MatchedCount != 1 {
			return Fail(p.CodeRouteChanged, "account授权租约失效")
		}
		var boot gameBoot
		err = s.DB.Collection("game_boots").FindOne(tc, bson.M{"_id": instance}).Decode(&boot)
		knownBoot := err == nil
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		if knownBoot && (boot.NodeID != node || !slices.Equal(boot.RunIDs, requested)) {
			return Fail(p.CodeRouteChanged, "同一启动实例的节点或运行服集合已固定")
		}
		var existing leasedGameNode
		err = s.DB.Collection("node_leases").FindOne(tc, bson.M{"_id": node}).Decode(&existing)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		replay := knownBoot || existing.Owner == instance
		memberships := existing.runs()
		if replay {
			if existing.Owner != instance || len(memberships) != len(requested) {
				return Fail(p.CodeRouteChanged, "原启动实例失权或请求的运行服集合已变化")
			}
			for i, member := range memberships {
				if member.RunID != requested[i] || member.Epoch <= 0 {
					return Fail(p.CodeRouteChanged, "原启动实例的运行服成员不匹配")
				}
			}
			filter := exactNodeMembershipFilter(node, instance, memberships)
			filter["$expr"] = leaseFilter()
			if err = s.DB.Collection("node_leases").FindOne(tc, filter).Err(); err != nil {
				return authorityReadError(err, p.CodeRouteChanged, "原启动实例的节点租约失效")
			}
		}
		// 按固定运行服顺序锁定权威部署和控制记录，所有候选更新都在同一事务内。
		next := make([]Run, 0, len(requested))
		var endpoint string
		for i, runID := range requested {
			assignment, err := s.reserveRunAssignment(tc, node, runID)
			if err != nil {
				return err
			}
			if i == 0 {
				endpoint = assignment.GameWSURL
			} else if assignment.GameWSURL != endpoint {
				return Fail(p.CodeRouteChanged, "同一游戏节点的运行服必须公布同一连接地址")
			}
			var grant Run
			if replay {
				grant, err = s.replayRunGrant(tc, instance, assignment, memberships[i])
			} else {
				grant, err = s.acquireRunGrant(tc, instance, assignment)
			}
			if err != nil {
				return err
			}
			next = append(next, grant)
		}
		if !replay {
			if err = s.acquireGameNode(tc, node, instance, next); err != nil {
				return err
			}
			models := make([]mongo.WriteModel, 0, len(next)*runGuardGroups)
			for _, run := range next {
				models = appendGrantedGuards(models, run)
			}
			if _, err = s.DB.Collection("run_guards").BulkWrite(tc, models); err != nil {
				return err
			}
		}
		// 对旧单运行服租约的首次合法重放也保留启动身份，后续重放无需增设运行服。
		if !knownBoot {
			if _, err = s.DB.Collection("game_boots").InsertOne(tc, gameBoot{Instance: instance, NodeID: node, RunIDs: requested}); err != nil {
				if mongo.IsDuplicateKeyError(err) {
					return Fail(p.CodeRouteChanged, "游戏启动实例已登记")
				}
				return err
			}
		}
		grants = next
		return nil
	})
	if err != nil {
		return nil, err
	}
	return grants, nil
}

// reserveRunAssignment 写入权威部署的批准序号，避免与并发的运维改派同时批准旧分配。
func (s *Store) reserveRunAssignment(ctx context.Context, node string, runID uint64) (runAssignment, error) {
	var assignment runAssignment
	err := s.DB.Collection("run_servers").FindOneAndUpdate(ctx,
		bson.M{"_id": runID, "game_node_id": node, "state": "open", "assignment_version": bson.M{"$gt": 0}},
		bson.M{"$inc": bson.M{"grant_seq": 1}}).Decode(&assignment)
	if err != nil {
		return assignment, authorityReadError(err, p.CodeUnauthenticated, "游戏节点与运行服未获当前部署授权")
	}
	if !openAssignment(assignment) {
		return assignment, Fail(p.CodeRouteChanged, "运行服公告地址无效")
	}
	return assignment, nil
}

// replayRunGrant 重读仍有效的原运行服权利，不续租及推进代次。
func (s *Store) replayRunGrant(ctx context.Context, instance string, assignment runAssignment, membership gameRunMembership) (Run, error) {
	filter := activeRunOwnership(assignment.GameNodeID, instance, membership)
	filter["$expr"] = leaseFilter()
	var current Run
	if err := s.DB.Collection("run_control").FindOne(ctx, filter).Decode(&current); err != nil {
		return current, authorityReadError(err, p.CodeRouteChanged, "原启动实例的运行服租约失效")
	}
	if !matchingRun(assignment, current) {
		return current, Fail(p.CodeRouteChanged, "原启动实例的运行服部署授权失效")
	}
	return current, nil
}

// acquireRunGrant 仅在旧运行服停止或已到数据库时钟截止时推进独立所有权代次。
func (s *Store) acquireRunGrant(ctx context.Context, instance string, assignment runAssignment) (Run, error) {
	var old Run
	if err := s.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": assignment.RunID}).Decode(&old); err != nil {
		return Run{}, authorityReadError(err, p.CodeRouteChanged, "运行服缺少控制记录")
	}
	if old.Owner == instance || old.Epoch < 0 || old.Epoch == math.MaxInt64 || old.ControlVersion < 0 || old.ControlVersion == math.MaxInt64 {
		return Run{}, Fail(p.CodeRouteChanged, "原启动实例失效或运行服代次无效")
	}
	filter := bson.M{"_id": assignment.RunID, "run_epoch": old.Epoch, "$or": bson.A{
		bson.M{"state": "stopped"}, bson.M{"$expr": bson.M{"$lte": bson.A{"$lease_until", "$$NOW"}}},
	}}
	update := bson.M{
		"owner_instance": instance, "game_node_id": assignment.GameNodeID, "assignment_version": assignment.AssignmentVersion,
		"run_epoch": old.Epoch + 1, "control_version": old.ControlVersion + 1, "state": "active", "lease_until": leaseDate(),
	}
	var grant Run
	err := s.DB.Collection("run_control").FindOneAndUpdate(ctx, filter,
		mongo.Pipeline{bson.D{{Key: "$set", Value: update}}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&grant)
	if err != nil {
		return Run{}, authorityReadError(err, p.CodeServerBusy, "运行服尚由其它有效实例持有")
	}
	return grant, nil
}

// acquireGameNode 把完整成员集绑定给本次启动，仅可接管不存在或已过期的稳定节点身份。
func (s *Store) acquireGameNode(ctx context.Context, node, instance string, grants []Run) error {
	memberships := make([]gameRunMembership, 0, len(grants))
	for _, grant := range grants {
		memberships = append(memberships, gameRunMembership{RunID: grant.ID, Epoch: grant.Epoch})
	}
	filter := bson.M{"_id": node, "$or": bson.A{
		bson.M{"$expr": bson.M{"$lte": bson.A{"$lease_until", "$$NOW"}}}, bson.M{"lease_until": bson.M{"$exists": false}},
	}}
	update := mongo.Pipeline{
		bson.D{{Key: "$set", Value: bson.M{"owner_instance": instance, "kind": "game", "run_memberships": memberships, "lease_until": leaseDate()}}},
		bson.D{{Key: "$unset", Value: bson.A{"run_server_id", "run_epoch"}}},
	}
	// 先建立确定的空租约文档，再条件更新。这样活跃节点争抢返回业务冲突，不触发有歧义的重复主键写。
	if _, err := s.DB.Collection("node_leases").UpdateOne(ctx, bson.M{"_id": node},
		bson.M{"$setOnInsert": bson.M{"kind": "game"}}, options.UpdateOne().SetUpsert(true)); err != nil {
		return err
	}
	res, err := s.DB.Collection("node_leases").UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount != 1 {
		return Fail(p.CodeServerBusy, "节点尚由其它有效实例持有")
	}
	return nil
}

// appendGrantedGuards 为每个账号分组同步运行服原子授权的实例、代次和发布版本。
func appendGrantedGuards(models []mongo.WriteModel, run Run) []mongo.WriteModel {
	for group := 0; group < runGuardGroups; group++ {
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": fmt.Sprintf("%d:%d", run.ID, group)}).
			SetUpdate(bson.M{"$set": bson.M{
				"owner_instance": run.Owner, "run_epoch": run.Epoch, "control_version": run.ControlVersion,
			}, "$inc": bson.M{"seq": 1}}).SetUpsert(true))
	}
	return models
}
