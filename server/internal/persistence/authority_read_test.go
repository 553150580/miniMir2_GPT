// 回归权限查询的错误分类，区分记录不存在与数据库读操作失败，防止暂时故障被永久解释为失去权限。

package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"
	p "minimir/internal/protocol"
)

// TestAuthorityReadDistinguishesMissingRecordsFromStorageFailure 核对「无匹配记录应返回失效码」与「读取失败被误判为权限失效」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAuthorityReadDistinguishesMissingRecordsFromStorageFailure(t *testing.T) {
	// check 接收遍历临时结构列表时当前的元素。
	for _, check := range []struct {
		// name 记录名称，使用文本表示。
		name string
		// code 记录结果码，使用数值表示。
		code uint32
	}{
		{name: "lease", code: p.CodeRouteChanged},
		{name: "account", code: p.CodeUnauthenticated},
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(check.name, func(t *testing.T) {
			// err 接收遍历错误列表时当前的元素。
			for _, err := range []error{mongo.ErrNoDocuments, fmt.Errorf("读取权限记录: %w", mongo.ErrNoDocuments)} {
				// got 保存由结果码取得的实际结果。
				if got := Code(authorityReadError(err, check.code, "权限失效")); got != check.code {
					t.Fatalf("无匹配记录应返回失效码，得到 %d，期望 %d", got, check.code)
				}
			}
			// transient 保存当前处理所用的“transient”。
			transient := &mongo.CommandError{Code: 112, Labels: []string{"TransientTransactionError"}}
			// err 接收遍历错误列表时当前的元素。
			for _, err := range []error{context.DeadlineExceeded, transient, errors.New("存档解码失败")} {
				// got 保存由权限围栏读取错误取得的实际结果。
				if got := authorityReadError(err, check.code, "权限失效"); got != err || Code(got) != p.CodeServerBusy {
					t.Fatalf("读取失败被误判为权限失效: %v -> %v", err, got)
				}
			}
		})
	}
}
