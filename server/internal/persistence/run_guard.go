// 业务事务进程内的账号分组串行门。用固定哈希组减少共享 MongoDB 围栏竞争，并把排队与事务都限制在原截止内。

package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"time"
)

const (
	runGuardGroups     = 32              // 账号哈希使用的固定围栏组数。
	transactionTimeout = 3 * time.Second // 包含组内等待与数据库提交的总预算。
)

// runGuardSlots 以容量为一的通道表示每个围栏组的本地占用。
type runGuardSlots [runGuardGroups]chan struct{}

// newRunGuardSlots 给每个账号组初始化独立的互斥通道。
func newRunGuardSlots() *runGuardSlots {
	// slots 保存每个围栏组各自的单容量占用通道。
	slots := new(runGuardSlots)
	// i 标记遍历槽位集合时访问的下标。
	for i := range slots {
		slots[i] = make(chan struct{}, 1)
	}
	return slots
}

// runGuardGroup 把持久账号编号稳定映射到运行服的一个围栏组。
// accountID 标识当前业务使用的账号。
func runGuardGroup(accountID string) uint64 {
	// hash 是账号持久编号的散列，采用其高位计算稳定围栏组号。
	hash := sha256.Sum256(domain.Bytes(accountID))
	return binary.BigEndian.Uint64(hash[:8]) % runGuardGroups
}

// acquire 在当前截止内占用目标组，成功后把释放所需的通道还给调用者。
// slots 是调用该方法的运行批次围栏槽位集合实例，ctx 向下传递当前请求的截止时间和取消信号，group 记录分组，使用数值表示。
func (slots *runGuardSlots) acquire(ctx context.Context, group uint64) (chan struct{}, error) {
	// err 保存当前调用返回的错误，供后续失败分支处理。
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// slot 表示指定组的事务执行许可，事务完成要释放占用。
	slot := slots[group]
	select {
	case slot <- struct{}{}:
		return slot, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// 同一进程内先串行调度同组事务，减少共享 guard 文档的写冲突。
// MongoDB 围栏仍在原事务内执行，跨进程换代和封禁语义由持久层保证。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，auth 提供事务对应的账号，用来锁定正确的本地围栏分组，fn 记录“fn”，使用供当前流程调用的函数表示。
func (s *Store) guardedTx(ctx context.Context, auth Auth, fn func(context.Context) error) error {
	// cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(ctx, transactionTimeout)
	defer cancel()
	// slot 是本地匹配分组取得的放行信号，覆盖整个事务重试周期，err 接收“acquire”过程中可能产生的错误，供后续失败分支处理。
	slot, err := s.runGuardSlots.acquire(ctx, runGuardGroup(auth.AccountID))
	if err != nil {
		return Fail(p.CodeOperationPending, "原操作提交待查证")
	}
	defer func() { <-slot }()
	return s.tx(ctx, fn)
}
