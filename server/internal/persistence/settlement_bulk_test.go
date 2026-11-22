package persistence

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestSettlementBulkWriteRetainsTransactionLabels(t *testing.T) {
	raw, err := bson.Marshal(bson.M{"errorLabels": bson.A{"TransientTransactionError"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range []mongo.ClientBulkWriteException{
		{WriteError: &mongo.WriteError{Code: 251, Raw: raw}},
		{WriteErrors: map[int]mongo.WriteError{0: {Code: 112}}},
	} {
		err := transactionBulkWriteError(failure)
		var server mongo.ServerError
		if !errors.As(err, &server) || !server.HasErrorLabel("TransientTransactionError") {
			t.Fatal("跨集合写入丢失事务重试标签", err)
		}
	}
	duplicate := transactionBulkWriteError(mongo.ClientBulkWriteException{WriteErrors: map[int]mongo.WriteError{2: {Code: 11000}}})
	if !mongo.IsDuplicateKeyError(duplicate) {
		t.Fatal("跨集合写入丢失唯一键错误", duplicate)
	}
}
