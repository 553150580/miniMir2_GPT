// 开发节点与运行区服的租约。申请、续约和释放使用 MongoDB 实际时钟，并将所有权版本同步到每组写围栏。

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

// NodeLeaseDuration 是 MongoDB 签发单次节点或运行服租约的时长。
const NodeLeaseDuration = 15 * time.Second

// leaseDate 用 MongoDB 的时间表达式计算租期，避免进程间本地时钟差。
func leaseDate() bson.M {
	return bson.M{"$dateAdd": bson.M{"startDate": "$$NOW", "unit": "second", "amount": int64(NodeLeaseDuration / time.Second)}}
}

// Singleton 申请 account 或 GM 等节点类别的专属租约。
// s 提供本进程标识和数据库，ctx 限制申请时长，kind 指定需要互斥的一类节点。
func (s *Store) Singleton(ctx context.Context, kind string) error {
	// key 是这一节点类型在租约集合的固定主键。
	key := "singleton:" + kind
	// e 保存初始化未持有租约文档时的数据库错误。
	if _, e := s.DB.Collection("node_leases").UpdateOne(ctx, bson.M{"_id": key}, bson.M{"$setOnInsert": bson.M{"lease_until": time.Unix(0, 0)}}, options.UpdateOne().SetUpsert(true)); e != nil {
		return e
	}
	// result 的匹配数表明本实例是否取得租约，e 保存写入本身失败的原因。
	result, e := s.DB.Collection("node_leases").UpdateOne(ctx, bson.M{"_id": key, "$or": bson.A{bson.M{"owner_instance": s.Instance}, bson.M{"$expr": bson.M{"$lte": bson.A{"$lease_until", "$$NOW"}}}, bson.M{"lease_until": bson.M{"$exists": false}}}}, mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{"owner_instance": s.Instance, "lease_until": leaseDate(), "kind": kind}}}}, options.UpdateOne())
	if e == nil && result.MatchedCount != 1 {
		return Fail(p.CodeServerBusy, "同类节点已有有效实例")
	}
	if mongo.IsDuplicateKeyError(e) {
		return Fail(p.CodeServerBusy, "同类节点已有有效实例")
	}
	return e
}

// ApproveGame 由 account 节点授权 game 接管运行服，在同一事务推进代次和全部分组围栏。
// s 是持有 account 租约的存储实例，ctx 限制授权事务，node 是待授权部署节点，instance 是该 game 进程的实例标识。
// runID 指定该游戏进程请求接管的运行服。
func (s *Store) ApproveGame(ctx context.Context, node, instance string, runID uint64) (Run, error) {
	// next 保存已获准的运行服状态，供 game 继续持有对应代次。
	var next Run
	if node != s.Cfg.NodeID || runID != s.Cfg.RunID || len(instance) != 32 {
		return next, Fail(p.CodeUnauthenticated, "部署身份不在许可范围")
	}
	// e 保存整个授权事务的错误，tc 把节点租约、运行服和全部分组围栏绑定到同一次提交。
	e := s.tx(ctx, func(tc context.Context) error {
		// 写入授权序列，让 account 租约换代或释放能和本次授权竞争同一文档。
		// approval 的匹配数用于确认 account 仍有签发权限，e 保存授权序号写入失败。
		approval, e := s.DB.Collection("node_leases").UpdateOne(tc, bson.M{"_id": "singleton:account", "owner_instance": s.Instance, "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}, bson.M{"$inc": bson.M{"grant_seq": 1}})
		if e != nil {
			return e
		}
		if approval.MatchedCount != 1 {
			return Fail(p.CodeRouteChanged, "account授权租约失效")
		}
		// old 保存当前运行服持有者、代次及控制版本。
		var old Run
		// e 保存原运行服控制记录的读取错误。
		if e := s.DB.Collection("run_control").FindOne(tc, bson.M{"_id": runID}).Decode(&old); e != nil {
			return e
		}
		if old.Owner == instance && old.State == "active" {
			next = old
			return nil
		}
		// res 的匹配数确认原运行服已停止或租约过期，允许接管。
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
		// nodeResult 的匹配数确认部署节点已归本 game 实例持有。
		nodeResult, e := s.DB.Collection("node_leases").UpdateOne(tc, bson.M{"_id": node, "$or": bson.A{bson.M{"owner_instance": instance}, bson.M{"$expr": bson.M{"$lte": bson.A{"$lease_until", "$$NOW"}}}, bson.M{"lease_until": bson.M{"$exists": false}}}}, mongo.Pipeline{bson.D{{Key: "$set", Value: bson.M{"owner_instance": instance, "lease_until": leaseDate(), "kind": "game", "run_epoch": old.Epoch + 1}}}}, options.UpdateOne())
		if e != nil {
			return e
		}
		if nodeResult.MatchedCount != 1 {
			return Fail(p.CodeServerBusy, "节点尚由其它有效实例持有")
		}
		// 新所有权与全部分组围栏共同提交，旧运行代次的在途事务会被持久围栏挡住。
		// group 依次定位各账号组的写入围栏。
		for group := 0; group < runGuardGroups; group++ {
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

// Renew 使用控制连接池延长仍由当前实例持有的租约，游戏服还需续约运行服记录。
// s 提供控制连接和本进程身份，ctx 限制续租时长，kind 确定需要续租的节点类别。
func (s *Store) Renew(ctx context.Context, kind string) error {
	// db 是通过独立控制连接池访问的同一业务库。
	db := s.Control.Database(s.Cfg.Database)
	// key 是租约文档主键，game 使用部署节点标识。
	key := "singleton:" + kind
	if kind == "game" {
		key = s.Cfg.NodeID
	}
	// filter 只允许同一实例延长尚未过期的租约。
	filter := bson.M{"_id": key, "owner_instance": s.Instance, "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}
	// r 的匹配数识别本节点是否仍持有租约，e 保存写入错误。
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

// Release 归还本实例租约，游戏服还会关闭运行服并让原围栏失效。
// s 提供本节点实例和运行代次，ctx 限制归还时长，kind 决定释放单节点还是同时关闭运行服。
func (s *Store) Release(ctx context.Context, kind string) error {
	if kind != "game" {
		// e 保存提前结束该非 game 节点租约时的写入错误。
		_, e := s.DB.Collection("node_leases").UpdateOne(ctx, bson.M{"_id": "singleton:" + kind, "owner_instance": s.Instance}, bson.M{"$set": bson.M{"lease_until": time.Unix(0, 0)}})
		return e
	}
	// tc 保证游戏服停运、组围栏换版与节点租约归还在同一事务。
	return s.tx(ctx, func(tc context.Context) error {
		// res 的匹配数确认仍有权关闭该运行服，e 保存控制记录更新错误。
		res, e := s.DB.Collection("run_control").UpdateOne(tc, bson.M{"_id": s.Cfg.RunID, "owner_instance": s.Instance, "run_epoch": s.Epoch}, bson.M{"$set": bson.M{"state": "stopped", "lease_until": time.Unix(0, 0)}, "$inc": bson.M{"control_version": 1}})
		if e != nil {
			return e
		}
		if res.MatchedCount == 0 {
			return nil
		}
		// g 依次定位各账号组围栏，推进版本以阻止旧授权请求。
		for g := 0; g < runGuardGroups; g++ {
			if _, e = s.DB.Collection("run_guards").UpdateOne(tc, bson.M{"_id": fmt.Sprintf("%d:%d", s.Cfg.RunID, g), "owner_instance": s.Instance, "run_epoch": s.Epoch}, bson.M{"$inc": bson.M{"control_version": 1, "seq": 1}}); e != nil {
				return e
			}
		}
		_, e = s.DB.Collection("node_leases").UpdateOne(tc, bson.M{"_id": s.Cfg.NodeID, "owner_instance": s.Instance}, bson.M{"$set": bson.M{"lease_until": time.Unix(0, 0)}})
		return e
	})
}
