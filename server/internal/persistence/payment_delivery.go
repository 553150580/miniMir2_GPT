// 本文件按当前运行服承载的永久入口扫描仍需发放的已付订单。
package persistence

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	p "minimir/internal/protocol"
)

// NextVerifiedPayment 读取原核验时间和订单号之后的一笔，下一轮从零游标开始以重试未确认的订单。
func (s *Store) NextVerifiedPayment(ctx context.Context, after PaymentOrder) (PaymentOrder, error) {
	ctx, stop := context.WithTimeout(ctx, transactionTimeout)
	defer stop()
	if s.Epoch <= 0 || s.Cfg.RunID == 0 {
		return PaymentOrder{}, Fail(p.CodeRouteChanged, "支付投递需要当前游戏运行服")
	}
	cursor, err := s.DB.Collection("origin_routes").Find(ctx, bson.M{"run_server_id": s.Cfg.RunID},
		options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return PaymentOrder{}, err
	}
	var routes []originRoute
	if err := cursor.All(ctx, &routes); err != nil {
		return PaymentOrder{}, err
	}
	if len(routes) == 0 {
		return PaymentOrder{}, nil
	}
	origins := make([]uint64, len(routes))
	for i, route := range routes {
		origins[i] = route.OriginID
	}
	filter := bson.M{"state": PaymentOrderVerified, "origin_server_id": bson.M{"$in": origins},
		"verified_at": bson.M{"$gt": time.Unix(0, 0)}}
	if after.ID != "" {
		if !lowerHex(after.ID, 16) || after.VerifiedAt.IsZero() {
			return PaymentOrder{}, Fail(p.CodeInvalidArgument, "支付扫描游标无效")
		}
		filter["$or"] = bson.A{bson.M{"verified_at": bson.M{"$gt": after.VerifiedAt}},
			bson.M{"verified_at": after.VerifiedAt, "_id": bson.M{"$gt": after.ID}}}
	}
	var order PaymentOrder
	err = s.DB.Collection(paymentOrderCollection).FindOne(ctx, filter,
		options.FindOne().SetSort(bson.D{{Key: "verified_at", Value: 1}, {Key: "_id", Value: 1}})).Decode(&order)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return PaymentOrder{}, nil
	}
	return order, err
}
