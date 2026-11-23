// 开发节点与运行服的租约续期与释放，始终使用 MongoDB 实际时钟。

package persistence

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	p "minimir/internal/protocol"
)

// NodeLeaseDuration 是 MongoDB 签发单次节点或运行服租约的时长。
const NodeLeaseDuration = 15 * time.Second

// leaseDate 用 MongoDB 的时间表达式计算租期，避免进程间本地时钟差。
func leaseDate() bson.M {
	return bson.M{"$dateAdd": bson.M{"startDate": "$$NOW", "unit": "second", "amount": int64(NodeLeaseDuration / time.Second)}}
}

// Singleton 申请 account 或 GM 等节点类别的专属租约。
func (s *Store) Singleton(ctx context.Context, kind string) error {
	key := "singleton:" + kind
	if _, err := s.DB.Collection("node_leases").UpdateOne(ctx, bson.M{"_id": key}, bson.M{"$setOnInsert": bson.M{"lease_until": time.Unix(0, 0)}}, options.UpdateOne().SetUpsert(true)); err != nil {
		return err
	}
	filter := bson.M{"_id": key, "$or": bson.A{
		bson.M{"owner_instance": s.Instance}, bson.M{"$expr": bson.M{"$lte": bson.A{"$lease_until", "$$NOW"}}},
		bson.M{"lease_until": bson.M{"$exists": false}},
	}}
	update := mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{"owner_instance": s.Instance, "lease_until": leaseDate(), "kind": kind}}}}
	result, err := s.DB.Collection("node_leases").UpdateOne(ctx, filter, update)
	if mongo.IsDuplicateKeyError(err) || err == nil && result.MatchedCount != 1 {
		return Fail(p.CodeServerBusy, "同类节点已有有效实例")
	}
	return err
}

// Renew 使用独立控制连接。多个运行服与节点租约在同一事务续约，任何失权都回滚其余续约。
func (s *Store) Renew(ctx context.Context, kind string) error {
	db := s.Control.Database(s.Cfg.Database)
	update := mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{"lease_until": leaseDate()}}}}
	if kind != "game" {
		filter := bson.M{"_id": "singleton:" + kind, "owner_instance": s.Instance, "$expr": leaseFilter()}
		return renewLease(ctx, db.Collection("node_leases"), filter, update, "节点租约失效")
	}
	runs, err := s.ownedGameRuns()
	if err != nil {
		return err
	}
	node := exactNodeMembershipFilter(s.Cfg.NodeID, s.Instance, runs)
	node["$expr"] = leaseFilter()
	if len(runs) == 1 {
		// 旧单运行服保持原有双回执检查和独立的失权确认行为。
		if err = renewLease(ctx, db.Collection("node_leases"), node, update, "节点租约失效"); err != nil {
			return err
		}
		control := activeRunOwnership(s.Cfg.NodeID, s.Instance, runs[0])
		control["$expr"] = leaseFilter()
		return renewLease(ctx, db.Collection("run_control"), control, update, "运行服租约失效")
	}
	return s.controlLeaseTransaction(ctx, func(tc context.Context) error {
		if err := renewLease(tc, db.Collection("node_leases"), node, update, "节点租约失效"); err != nil {
			return err
		}
		writes := make([]mongo.WriteModel, 0, len(runs))
		for _, run := range runs {
			control := activeRunOwnership(s.Cfg.NodeID, s.Instance, run)
			control["$expr"] = leaseFilter()
			writes = append(writes, mongo.NewUpdateOneModel().SetFilter(control).SetUpdate(update))
		}
		result, err := db.Collection("run_control").BulkWrite(tc, writes)
		if err != nil {
			return err
		}
		if result.MatchedCount != int64(len(runs)) {
			return Fail(p.CodeRouteChanged, "部分运行服租约失效")
		}
		return nil
	})
}

// renewLease 以极短控制命令确认当前查询条件仍然匹配相应租约。
func renewLease(ctx context.Context, collection *mongo.Collection, filter bson.M, update mongo.Pipeline, revoked string) error {
	result, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return Fail(p.CodeRouteChanged, revoked)
	}
	return nil
}

// controlLeaseTransaction 保留驱动的原始超时、暂态和未知提交错误供续租监视器判断。
func (s *Store) controlLeaseTransaction(ctx context.Context, callback func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, transactionTimeout)
	defer cancel()
	session, err := s.Control.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())
	_, err = session.WithTransaction(ctx, func(tc context.Context) (any, error) {
		return nil, callback(tc)
	}, options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))
	return err
}

// Release 释放节点租约，并在同一事务停止本实例仍拥有的所有运行服及其写围栏。
func (s *Store) Release(ctx context.Context, kind string) error {
	expired := time.Unix(0, 0)
	if kind != "game" {
		_, err := s.DB.Collection("node_leases").UpdateOne(ctx, bson.M{"_id": "singleton:" + kind, "owner_instance": s.Instance},
			bson.M{"$set": bson.M{"lease_until": expired}})
		return err
	}
	runs, err := s.ownedGameRuns()
	if err != nil {
		// 从未取得控制授权的启动失败无需释放任何运行服。
		if !s.borrowedRunView && Code(err) == p.CodeRouteChanged {
			return nil
		}
		return err
	}
	return s.tx(ctx, func(tc context.Context) error {
		node := exactNodeMembershipFilter(s.Cfg.NodeID, s.Instance, runs)
		result, err := s.DB.Collection("node_leases").UpdateOne(tc, node, bson.M{"$set": bson.M{"lease_until": expired}})
		if err != nil {
			return err
		}
		if result.MatchedCount == 0 {
			return nil
		}
		writes := make([]mongo.WriteModel, 0, len(runs))
		for _, run := range runs {
			filter := activeRunOwnership(s.Cfg.NodeID, s.Instance, run)
			update := bson.M{"$set": bson.M{"state": "stopped", "lease_until": expired}, "$inc": bson.M{"control_version": 1}}
			stopped, err := s.DB.Collection("run_control").UpdateOne(tc, filter, update)
			if err != nil {
				return err
			}
			if stopped.MatchedCount == 1 {
				writes = append(writes, releasedGuardWrites(run, s.Instance))
			}
		}
		if len(writes) != 0 {
			_, err = s.DB.Collection("run_guards").BulkWrite(tc, writes)
		}
		return err
	})
}

// releasedGuardWrites 仅推进原实例与原运行服代次对应的组围栏。
func releasedGuardWrites(run gameRunMembership, instance string) mongo.WriteModel {
	keys := make([]string, 0, runGuardGroups)
	for group := 0; group < runGuardGroups; group++ {
		keys = append(keys, fmt.Sprintf("%d:%d", run.RunID, group))
	}
	return mongo.NewUpdateManyModel().
		SetFilter(bson.M{"_id": bson.M{"$in": keys}, "owner_instance": instance, "run_epoch": run.Epoch}).
		SetUpdate(bson.M{"$inc": bson.M{"control_version": 1, "seq": 1}})
}
