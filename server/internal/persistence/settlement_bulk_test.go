// 验证批量写入错误转换保留 MongoDB 驱动事务重试和唯一键识别所需的信息。

package persistence

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestSettlementBulkWriteRetainsTransactionLabels 核对「跨集合写入丢失事务重试标签」与「跨集合写入丢失唯一键错误」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSettlementBulkWriteRetainsTransactionLabels(t *testing.T) {
	// raw 保存由序列化取得的原始值，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	raw, err := bson.Marshal(bson.M{"errorLabels": bson.A{"TransientTransactionError"}})
	if err != nil {
		t.Fatal(err)
	}
	// failure 接收遍历客户端批量写入“exception”列表时当前的元素。
	for _, failure := range []mongo.ClientBulkWriteException{
		{WriteError: &mongo.WriteError{Code: 251, Raw: raw}},
		{WriteErrors: map[int]mongo.WriteError{0: {Code: 112}}},
	} {
		// err 接收“transaction”批量写入错误过程中可能产生的错误，供后续失败分支处理。
		err := transactionBulkWriteError(failure)
		// server 记录服务端，使用服务端错误表示。
		var server mongo.ServerError
		if !errors.As(err, &server) || !server.HasErrorLabel("TransientTransactionError") {
			t.Fatal("跨集合写入丢失事务重试标签", err)
		}
	}
	// duplicate 保存由“transaction”批量写入错误取得的“duplicate”。
	duplicate := transactionBulkWriteError(mongo.ClientBulkWriteException{WriteErrors: map[int]mongo.WriteError{2: {Code: 11000}}})
	if !mongo.IsDuplicateKeyError(duplicate) {
		t.Fatal("跨集合写入丢失唯一键错误", duplicate)
	}
}
