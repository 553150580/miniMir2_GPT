package persistence

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunGuardSlotsSerializeGroupsAndRespectDeadline(t *testing.T) {
	slots := newRunGuardSlots()
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()

	held, err := slots.acquire(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	other, err := slots.acquire(ctx, 1)
	if err != nil {
		t.Fatalf("不同组不可互相阻塞：%v", err)
	}
	<-other

	wait, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if _, err = slots.acquire(wait, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("同组在截止前不应获得执行权：%v", err)
	}
	<-held
	if _, err = slots.acquire(wait, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("已超时请求不可占用释放后的槽位：%v", err)
	}
	next, err := slots.acquire(ctx, 0)
	if err != nil {
		t.Fatalf("同组释放后应恢复执行：%v", err)
	}
	<-next
}
