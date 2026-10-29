package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"strings"
)

// 通知只投影当前合法附着；重入快照显示持久策略和钱包，不补播旧回收事件。
func (w *worker) equipmentRecycleResult(r *room, op persistence.Operation) {
	result := op.Recycling
	if w.e.Store.Cfg.Fixture.RuleVersion < 19 || r.Conn == nil || !r.Ready || r.Isolated || op.RoleID != r.Role.ID || result == nil || len(result.Recycled) == 0 && result.DiscardedCount == 0 {
		return
	}
	var opID []byte
	if strings.HasPrefix(op.ID, "asset:"+r.Role.ID+":") {
		opID = domain.Bytes(strings.TrimPrefix(op.ID, "asset:"+r.Role.ID+":"))
	}
	w.send(r, p.MsgS2CEquipmentRecycleResult, 0, &pb.EquipmentRecycleResult{Role: r.Role.Summary().Key, RoleRevision: uint64(op.Revision), SourceSeq: uint64(op.SourceSeq), OpId: opID, PolicyRevision: uint64(result.PolicyRevision), RecycledCount: uint64(len(result.Recycled)), Gold: result.Gold, BoundGold: result.BoundGold, DiscardedCount: uint64(result.DiscardedCount), DiscardReason: result.DiscardReason, Trigger: result.Trigger})
}
