// 本文件为外部已核验的资产发放共用运行服和原账号围栏。
package persistence

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	p "minimir/internal/protocol"
)

// externalDeliveryFence 只有原入口当前所有者可向该账号兑现已核验的权益。
func (s *Store) externalDeliveryFence(ctx context.Context, origin uint64, account string) ([]mongo.ClientBulkWrite, error) {
	current, err := s.liveRoute(ctx, origin)
	if err != nil {
		return nil, err
	}
	if current.run.ID != s.Cfg.RunID || current.run.Owner != s.Instance || current.run.Epoch != s.Epoch {
		return nil, Fail(p.CodeRouteChanged, "已核验外部权益须由原入口当前所有者发放")
	}
	runWrite, err := s.runFenceWrite(ctx, runGuardGroup(account))
	if err != nil {
		return nil, err
	}
	writes := make([]mongo.ClientBulkWrite, 1, 5)
	writes[0] = runWrite
	// 已核验的原凭据与玩家在线和重新登录独立，但仍与账号及其角色覆写竞争。
	writes = append(writes, mongo.ClientBulkWrite{Database: s.DB.Name(), Collection: "accounts", Model: mongo.NewClientUpdateOneModel().
		SetFilter(bson.M{"_id": account}).SetUpdate(bson.M{"$inc": bson.M{"auth_guard_seq": 1}})})
	return writes, nil
}

// externalDeliveryOrigins 获取当前运行服实际承载的永久入口列表。
func (s *Store) externalDeliveryOrigins(ctx context.Context) ([]uint64, error) {
	if s.Epoch <= 0 || s.Cfg.RunID == 0 {
		return nil, Fail(p.CodeRouteChanged, "外部权益投递需要当前运行服")
	}
	cursor, err := s.DB.Collection("origin_routes").Find(ctx, bson.M{"run_server_id": s.Cfg.RunID},
		options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	var routes []originRoute
	if err := cursor.All(ctx, &routes); err != nil {
		return nil, err
	}
	origins := make([]uint64, len(routes))
	for i, route := range routes {
		origins[i] = route.OriginID
	}
	return origins, nil
}
