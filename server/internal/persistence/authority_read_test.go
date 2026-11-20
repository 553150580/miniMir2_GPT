package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"
	p "minimir/internal/protocol"
)

func TestAuthorityReadDistinguishesMissingRecordsFromStorageFailure(t *testing.T) {
	for _, check := range []struct {
		name string
		code uint32
	}{
		{name: "lease", code: p.CodeRouteChanged},
		{name: "account", code: p.CodeUnauthenticated},
	} {
		t.Run(check.name, func(t *testing.T) {
			for _, err := range []error{mongo.ErrNoDocuments, fmt.Errorf("读取权限记录: %w", mongo.ErrNoDocuments)} {
				if got := Code(authorityReadError(err, check.code, "权限失效")); got != check.code {
					t.Fatalf("无匹配记录应返回失效码，得到 %d，期望 %d", got, check.code)
				}
			}
			transient := &mongo.CommandError{Code: 112, Labels: []string{"TransientTransactionError"}}
			for _, err := range []error{context.DeadlineExceeded, transient, errors.New("存档解码失败")} {
				if got := authorityReadError(err, check.code, "权限失效"); got != err || Code(got) != p.CodeServerBusy {
					t.Fatalf("读取失败被误判为权限失效: %v -> %v", err, got)
				}
			}
		})
	}
}
