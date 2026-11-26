// 本文件将实际成交的卖方佣金计入两个市场共用的永久税余数。
package persistence

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	p "minimir/internal/protocol"
)

// marketSellerTax 保存角色切换、重启和合服后仍连续使用的账户入口余数。
type marketSellerTax struct {
	ID        string `bson:"_id"`              // 原卖方账号与永久入口组合。
	AccountID string `bson:"account_id"`       // 归属卖方账号。
	Origin    uint64 `bson:"origin_server_id"` // 卖方的永久入口服。
	Remainder int64  `bson:"remainder"`        // 不足一个账本单位的累计税余数，范围 0–9。
}

// marketCommission 先对成交额求商和余数，避免极值下相加溢出。
func marketCommission(gross, previous int64) (tax, remainder int64) {
	tail := gross%10 + previous
	return gross/10 + tail/10, tail % 10
}

// chargeMarketSellerTax 必须在实际成交、商品交付与原回执的同一事务内执行。
func (s *Store) chargeMarketSellerTax(ctx context.Context, party MarketParty, gross, divisor int64) (int64, error) {
	if gross <= 0 || divisor != 10 || party.Origin == 0 || !lowerHex(party.AccountID, 16) {
		return 0, Fail(p.CodeInvalidArgument, "卖方或实际成交额无效")
	}
	id := fmt.Sprintf("%s:%d", party.AccountID, party.Origin)
	collection := s.DB.Collection(marketSellerTaxRemainders)
	var old marketSellerTax
	err := collection.FindOne(ctx, bson.M{"_id": id}).Decode(&old)
	if errors.Is(err, mongo.ErrNoDocuments) {
		tax, remainder := marketCommission(gross, 0)
		_, err := collection.InsertOne(ctx, marketSellerTax{ID: id, AccountID: party.AccountID, Origin: party.Origin, Remainder: remainder})
		if mongo.IsDuplicateKeyError(err) {
			return 0, Fail(p.CodeOperationPending, "卖方税余数正在确认")
		}
		return tax, err
	}
	if err != nil {
		return 0, err
	}
	if old.AccountID != party.AccountID || old.Origin != party.Origin || old.Remainder < 0 || old.Remainder > 9 {
		return 0, Fail(p.CodeOperationConflict, "卖方原税余数无效")
	}
	tax, remainder := marketCommission(gross, old.Remainder)
	changed, err := collection.UpdateOne(ctx, bson.M{"_id": id, "remainder": old.Remainder}, bson.M{"$set": bson.M{"remainder": remainder}})
	if err != nil {
		return 0, err
	}
	if changed.MatchedCount != 1 {
		return 0, Fail(p.CodeOperationPending, "卖方税余数已变化，须按原成交重试")
	}
	return tax, nil
}
