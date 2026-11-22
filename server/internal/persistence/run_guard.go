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
	runGuardGroups     = 32
	transactionTimeout = 3 * time.Second
)

type runGuardSlots [runGuardGroups]chan struct{}

func newRunGuardSlots() *runGuardSlots {
	slots := new(runGuardSlots)
	for i := range slots {
		slots[i] = make(chan struct{}, 1)
	}
	return slots
}

func runGuardGroup(accountID string) uint64 {
	hash := sha256.Sum256(domain.Bytes(accountID))
	return binary.BigEndian.Uint64(hash[:8]) % runGuardGroups
}

func (slots *runGuardSlots) acquire(ctx context.Context, group uint64) (chan struct{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
func (s *Store) guardedTx(ctx context.Context, auth Auth, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, transactionTimeout)
	defer cancel()
	slot, err := s.runGuardSlots.acquire(ctx, runGuardGroup(auth.AccountID))
	if err != nil {
		return Fail(p.CodeOperationPending, "原操作提交待查证")
	}
	defer func() { <-slot }()
	return s.tx(ctx, fn)
}
