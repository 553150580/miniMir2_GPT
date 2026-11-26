// 本文件在角色资产槽忙时保存可排队命令，区分未受理与原提交待确认。
package game

import (
	"bytes"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// waitsForAssetSlot 判断该消息可否在同一角色的资产写入之后处理。
func waitsForAssetSlot(id uint16) bool {
	switch id {
	case p.MsgC2SUseItem, p.MsgC2SLearnSkill, p.MsgC2SSkillAssetCommit,
		p.MsgC2SCastWithMaterial, p.MsgC2SSetCastingSupply, p.MsgC2SSetSkillSlots, p.MsgC2SMirrorArenaExchange,
		p.MsgC2SBuyRoleCard, p.MsgC2SClaimRoleCardDaily, p.MsgC2SItemMarketAction, p.MsgC2SGoldMarketAction,
		p.MsgC2SSetMiningAutoRecycle, p.MsgC2SForgeCommit, p.MsgC2SWeaponOilCommit:
		return true
	}
	return false
}

// assetCommandFingerprint 用消息号和正文核对同一原操作，拒绝换内容重放。
func assetCommandFingerprint(frame wire.Frame) string {
	return persistence.Hash(append([]byte{byte(frame.ID >> 8), byte(frame.ID)}, frame.Payload...))
}

// queueAssetCommand 在房间等待队列中保存原帧，交由当前操作完成后的原入口处理。
func (w *worker) queueAssetCommand(r *room, c command) {
	const waitingLimit = 2 // 与现有重连等待共用有界队列。
	op := operationContext(c.frame)
	if op == nil || len(op.OpId) != 16 {
		w.failure(r, c.frame, p.CodeInvalidArgument)
		return
	}
	id := op.OpId
	digest := assetCommandFingerprint(c.frame)
	known := r.PendingOperationID == domain.Hex(id)
	conflict := known && digest != r.PendingFingerprint
	for _, waiting := range r.Waiting {
		if economic(waiting.frame.ID) && bytes.Equal(operationID(waiting.frame), id) {
			known = true
			conflict = conflict || assetCommandFingerprint(waiting.frame) != digest
		}
	}
	if conflict {
		w.failure(r, c.frame, p.CodeOperationConflict)
		return
	}
	if len(r.Waiting) >= waitingLimit {
		historical := op.CommandSeq > 0 && r.Role.EconomicSeq >= 0 && op.CommandSeq <= uint64(r.Role.EconomicSeq)
		if known || historical {
			// 已在途或落在历史经济水位内的请求可能已经提交，须保留原回执核对。
			w.send(r, p.MsgS2COperationResult, c.frame.Request, &pb.OperationResult{
				OpId: id, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision), ErrorCode: p.CodeOperationPending, Pending: true,
			})
		} else {
			w.failure(r, c.frame, p.CodeServerBusy)
		}
		return
	}
	// 首次排队保留原身份，等待真实结果，避免把旁路结算误报为本次提交未知。
	c.frame.Payload = append([]byte(nil), c.frame.Payload...)
	r.Waiting = append(r.Waiting, c)
}
