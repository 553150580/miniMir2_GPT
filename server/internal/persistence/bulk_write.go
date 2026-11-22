// MongoDB 跨集合批量写入错误的事务适配。保留驱动可识别的写冲突、重试标签、唯一键和写确认错误。

package persistence

import (
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// 客户端跨集合批量错误容器不实现驱动事务所查的错误标签接口。
// err 保存当前操作的失败原因，供调用方检查或返回。
func transactionBulkWriteError(err error) error {
	// bulk 记录批量，使用客户端批量写入“exception”表示。
	var bulk mongo.ClientBulkWriteException
	if !errors.As(err, &bulk) {
		return err
	}
	// top 保存写入错误，用于当前步骤中的“top”。
	if top := bulk.WriteError; top != nil {
		return mongo.CommandError{Code: int32(top.Code), Message: top.Message, Labels: bulkWriteLabels(top.Code, top.Raw), Wrapped: err, Raw: top.Raw}
	}
	// converted 建立带初始值的写入“exception”，供本段继续处理。
	converted := mongo.WriteException{}
	// failure 接收遍历写入错误集合时当前的元素。
	for _, failure := range bulk.WriteErrors {
		converted.WriteErrors = append(converted.WriteErrors, failure)
		converted.Labels = append(converted.Labels, bulkWriteLabels(failure.Code, failure.Raw)...)
	}
	// i 标记遍历写入“concern”错误集合时访问的下标。
	for i := range bulk.WriteConcernErrors {
		// failure 保存当前处理所用的失败。
		failure := &bulk.WriteConcernErrors[i]
		if converted.WriteConcernError == nil {
			converted.WriteConcernError = failure
		}
		converted.Labels = append(converted.Labels, bulkWriteLabels(failure.Code, failure.Raw)...)
	}
	return converted
}

// bulkWriteLabels 返回批量写入“labels”供调用方使用。
// code 记录结果码，使用数值表示，raw 记录原始值。
func bulkWriteLabels(code int, raw bson.Raw) []string {
	// labels 记录“labels”，使用文本列表表示。
	var labels []string
	// array 保存由“array”成功标记取得的“array”，ok 表明查询、校验或类型转换是否成功。
	if array, ok := raw.Lookup("errorLabels").ArrayOK(); ok {
		// values 接收值集合供后续步骤使用。
		values, _ := array.Values()
		// value 接收遍历值集合时当前的元素。
		for _, value := range values {
			// label 保存由文本值成功标记取得的标签，ok 表明查询、校验或类型转换是否成功。
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
