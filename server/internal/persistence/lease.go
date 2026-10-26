package persistence

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	p "minimir/internal/protocol"
	"time"
)

func leaseDate() bson.M {
	return bson.M{"$dateAdd": bson.M{"startDate": "$$NOW", "unit": "second", "amount": 15}}
}
func (s *Store) Singleton(ctx context.Context, kind string) error {
	key := "singleton:" + kind
	if _, e := s.DB.Collection("node_leases").UpdateOne(ctx, bson.M{"_id": key}, bson.M{"$setOnInsert": bson.M{"lease_until": time.Unix(0, 0)}}, options.UpdateOne().SetUpsert(true)); e != nil {
		return e
	}
	result, e := s.DB.Collection("node_leases").UpdateOne(ctx, bson.M{"_id": key, "$or": bson.A{bson.M{"owner_instance": s.Instance}, bson.M{"$expr": bson.M{"$lte": bson.A{"$lease_until", "$$NOW"}}}, bson.M{"lease_until": bson.M{"$exists": false}}}}, mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{"owner_instance": s.Instance, "lease_until": leaseDate(), "kind": kind}}}}, options.UpdateOne())
	if e == nil && result.MatchedCount != 1 {
		return Fail(p.CodeServerBusy, "同类节点已有有效实例")
	}
	if mongo.IsDuplicateKeyError(e) {
		return Fail(p.CodeServerBusy, "同类节点已有有效实例")
	}
	return e
}
func (s *Store) ApproveGame(ctx context.Context, node, instance string, runID uint64) (Run, error) {
	var next Run
	if node != s.Cfg.NodeID || runID != s.Cfg.RunID || len(instance) != 32 {
		return next, Fail(p.CodeUnauthenticated, "部署身份不在许可范围")
	}
	e := s.tx(ctx, func(tc context.Context) error {
		approval, e := s.DB.Collection("node_leases").UpdateOne(tc, bson.M{"_id": "singleton:account", "owner_instance": s.Instance, "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}, bson.M{"$inc": bson.M{"grant_seq": 1}})
		if e != nil {
			return e
		}
		if approval.MatchedCount != 1 {
			return Fail(p.CodeRouteChanged, "account授权租约失效")
		}
		var old Run
		if e := s.DB.Collection("run_control").FindOne(tc, bson.M{"_id": runID}).Decode(&old); e != nil {
			return e
		}
		if old.Owner == instance && old.State == "active" {
			next = old
			return nil
		}
		res, e := s.DB.Collection("run_control").UpdateOne(tc, bson.M{"_id": runID, "run_epoch": old.Epoch, "$or": bson.A{bson.M{"state": "stopped"}, bson.M{"$expr": bson.M{"$lte": bson.A{"$lease_until", "$$NOW"}}}}}, mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{"owner_instance": instance, "run_epoch": old.Epoch + 1, "control_version": old.ControlVersion + 1, "state": "active", "lease_until": leaseDate()}}}})
		if e != nil {
			return e
		}
		if res.MatchedCount != 1 {
			return Fail(p.CodeServerBusy, "运行服尚由其它有效实例持有")
		}
		if _, e = s.DB.Collection("node_leases").UpdateOne(tc, bson.M{"_id": node}, bson.M{"$setOnInsert": bson.M{"lease_until": time.Unix(0, 0)}}, options.UpdateOne().SetUpsert(true)); e != nil {
			return e
		}
		nodeResult, e := s.DB.Collection("node_leases").UpdateOne(tc, bson.M{"_id": node, "$or": bson.A{bson.M{"owner_instance": instance}, bson.M{"$expr": bson.M{"$lte": bson.A{"$lease_until", "$$NOW"}}}, bson.M{"lease_until": bson.M{"$exists": false}}}}, mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{"owner_instance": instance, "lease_until": leaseDate(), "kind": "game", "run_epoch": old.Epoch + 1}}}}, options.UpdateOne())
		if e != nil {
			return e
		}
		if nodeResult.MatchedCount != 1 {
			return Fail(p.CodeServerBusy, "节点尚由其它有效实例持有")
		}
		for group := 0; group < 32; group++ {
			_, e = s.DB.Collection("run_guards").UpdateOne(tc, bson.M{"_id": fmt.Sprintf("%d:%d", runID, group)}, bson.M{"$set": bson.M{"owner_instance": instance, "run_epoch": old.Epoch + 1, "control_version": old.ControlVersion + 1}, "$inc": bson.M{"seq": 1}}, options.UpdateOne().SetUpsert(true))
			if e != nil {
				return e
			}
		}
		next = Run{ID: runID, Owner: instance, Epoch: old.Epoch + 1, ControlVersion: old.ControlVersion + 1, State: "active"}
		return nil
	})
	return next, e
}
func (s *Store) Renew(ctx context.Context, kind string) error {
	db := s.Control.Database(s.Cfg.Database)
	key := "singleton:" + kind
	if kind == "game" {
		key = s.Cfg.NodeID
	}
	filter := bson.M{"_id": key, "owner_instance": s.Instance, "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}
	r, e := db.Collection("node_leases").UpdateOne(ctx, filter, mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{"lease_until": leaseDate()}}}})
	if e != nil {
		return e
	}
	if r.MatchedCount != 1 {
		return Fail(p.CodeRouteChanged, "节点租约失效")
	}
	if kind == "game" {
		r, e = db.Collection("run_control").UpdateOne(ctx, bson.M{"_id": s.Cfg.RunID, "owner_instance": s.Instance, "run_epoch": s.Epoch, "state": "active", "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}, mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{"lease_until": leaseDate()}}}})
		if e != nil {
			return e
		}
		if r.MatchedCount != 1 {
			return Fail(p.CodeRouteChanged, "运行服租约失效")
		}
	}
	return nil
}
func (s *Store) Release(ctx context.Context, kind string) error {
	if kind != "game" {
		_, e := s.DB.Collection("node_leases").UpdateOne(ctx, bson.M{"_id": "singleton:" + kind, "owner_instance": s.Instance}, bson.M{"$set": bson.M{"lease_until": time.Unix(0, 0)}})
		return e
	}
	return s.tx(ctx, func(tc context.Context) error {
		res, e := s.DB.Collection("run_control").UpdateOne(tc, bson.M{"_id": s.Cfg.RunID, "owner_instance": s.Instance, "run_epoch": s.Epoch}, bson.M{"$set": bson.M{"state": "stopped", "lease_until": time.Unix(0, 0)}, "$inc": bson.M{"control_version": 1}})
		if e != nil {
			return e
		}
		if res.MatchedCount == 0 {
			return nil
		}
		for g := 0; g < 32; g++ {
			if _, e = s.DB.Collection("run_guards").UpdateOne(tc, bson.M{"_id": fmt.Sprintf("%d:%d", s.Cfg.RunID, g), "owner_instance": s.Instance, "run_epoch": s.Epoch}, bson.M{"$inc": bson.M{"control_version": 1, "seq": 1}}); e != nil {
				return e
			}
		}
		_, e = s.DB.Collection("node_leases").UpdateOne(tc, bson.M{"_id": s.Cfg.NodeID, "owner_instance": s.Instance}, bson.M{"$set": bson.M{"lease_until": time.Unix(0, 0)}})
		return e
	})
}
