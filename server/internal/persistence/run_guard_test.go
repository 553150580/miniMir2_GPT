// 验证同账号组互斥、不同组并发和等待截止，避免组内阻塞超出事务时间预算。

package persistence

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRunGuardSlotsSerializeGroupsAndRespectDeadline 核验同组在截止前不应获得执行权，用诊断「已超时请求不可占用释放后的槽位」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestRunGuardSlotsSerializeGroupsAndRespectDeadline(t *testing.T) {
	// slots 保存由创建运行批次围栏槽位集合取得的槽位集合。
	slots := newRunGuardSlots()
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()

	// held 保存由“acquire”取得的“held”，err 接收“acquire”过程中可能产生的错误，供后续失败分支处理。
	held, err := slots.acquire(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	// other 保存由“acquire”取得的另一项。
	other, err := slots.acquire(ctx, 1)
	if err != nil {
		t.Fatalf("不同组不可互相阻塞：%v", err)
	}
	<-other

	// wait 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	wait, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if _, err = slots.acquire(wait, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("同组在截止前不应获得执行权：%v", err)
	}
	<-held
	if _, err = slots.acquire(wait, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("已超时请求不可占用释放后的槽位：%v", err)
	}
	// next 保存由“acquire”取得的下一状态。
	next, err := slots.acquire(ctx, 0)
	if err != nil {
		t.Fatalf("同组释放后应恢复执行：%v", err)
	}
	<-next
}
