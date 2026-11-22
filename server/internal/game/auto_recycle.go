// 本文件把装备自动回收的持久回执转成客户端通知，沿当前房间与角色版本发送。
package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"strings"
)

// 通知只投影当前合法附着；重入快照显示持久策略和钱包，不补播旧回收事件。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，op 记录操作。
func (w *worker) equipmentRecycleResult(r *room, op persistence.Operation) {
	// result 保存自动回收，用于当前步骤中的结果。
	result := op.Recycling
	if w.e.Store.Cfg.Fixture.RuleVersion < 19 || r.Conn == nil || !r.Ready || r.Isolated || op.RoleID != r.Role.ID || result == nil || len(result.Recycled) == 0 && result.DiscardedCount == 0 {
		return
	}
	// opID 标识当前业务使用的操作。
	var opID []byte
	if strings.HasPrefix(op.ID, "asset:"+r.Role.ID+":") {
		opID = domain.Bytes(strings.TrimPrefix(op.ID, "asset:"+r.Role.ID+":"))
	}
	w.send(r, p.MsgS2CEquipmentRecycleResult, 0, &pb.EquipmentRecycleResult{Role: r.Role.Summary().Key, RoleRevision: uint64(op.Revision), SourceSeq: uint64(op.SourceSeq), OpId: opID, PolicyRevision: uint64(result.PolicyRevision), RecycledCount: uint64(len(result.Recycled)), Gold: result.Gold, BoundGold: result.BoundGold, DiscardedCount: uint64(result.DiscardedCount), DiscardReason: result.DiscardReason, Trigger: result.Trigger})
}
