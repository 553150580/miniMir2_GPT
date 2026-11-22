package persistence

import (
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// 客户端跨集合批量错误容器不实现驱动事务所查的错误标签接口。
func transactionBulkWriteError(err error) error {
	var bulk mongo.ClientBulkWriteException
	if !errors.As(err, &bulk) {
		return err
	}
	if top := bulk.WriteError; top != nil {
		return mongo.CommandError{Code: int32(top.Code), Message: top.Message, Labels: bulkWriteLabels(top.Code, top.Raw), Wrapped: err, Raw: top.Raw}
	}
	converted := mongo.WriteException{}
	for _, failure := range bulk.WriteErrors {
		converted.WriteErrors = append(converted.WriteErrors, failure)
		converted.Labels = append(converted.Labels, bulkWriteLabels(failure.Code, failure.Raw)...)
	}
	for i := range bulk.WriteConcernErrors {
		failure := &bulk.WriteConcernErrors[i]
		if converted.WriteConcernError == nil {
			converted.WriteConcernError = failure
		}
		converted.Labels = append(converted.Labels, bulkWriteLabels(failure.Code, failure.Raw)...)
	}
	return converted
}

func bulkWriteLabels(code int, raw bson.Raw) []string {
	var labels []string
	if array, ok := raw.Lookup("errorLabels").ArrayOK(); ok {
		values, _ := array.Values()
		for _, value := range values {
			if label, ok := value.StringValueOK(); ok {
				labels = append(labels, label)
			}
		}
	}
	// 批内事务写冲突可能只在结果游标以代码112返回，请驱动重试整个事务。
	if code == 112 {
		labels = append(labels, "TransientTransactionError")
	}
	return labels
}
