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
	origins, err := s.externalDeliveryOrigins(ctx)
	if err != nil || len(origins) == 0 {
		return PaymentOrder{}, err
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
