package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"time"
)

type assetBinding struct {
	room        *room
	auth        persistence.Auth
	generation  uint64
	id          string
	fingerprint string
}

func (w *worker) finishAsset(b assetBinding, q persistence.AssetRequest, f wire.Frame, reviveID string, role domain.Role, result *pb.OperationResult, receipt persistence.Operation, err error, retry *job) {
	r := b.room
	if w.rooms[b.auth.RoleID] != r || r.Auth != b.auth || r.Isolated || !r.Pending || r.AssetGeneration != b.generation || r.PendingOperationID != b.id || r.PendingFingerprint != b.fingerprint {
		return
	}
	if err != nil && (persistence.Code(err) == p.CodeOperationPending || persistence.Code(err) == p.CodeServerBusy) {
		r.PendingRetry = retry
		r.RetryAt = time.Now().Add(500 * time.Millisecond)
		result.Pending = true
		w.send(r, p.MsgS2COperationResult, f.Request, result)
		return
	}
	r.Pending = false
	r.PendingRetry = nil
	r.PendingOperationID = ""
	r.PendingFingerprint = ""
	if err == nil && role.Revision >= r.Role.Revision {
		r.Role = role
		transitionEventPrivilege(r, time.Now(), role.PrivilegeUntil)
		w.refreshPlayerStats(r)
	}
	if err != nil {
		result.ErrorCode = persistence.Code(err)
	}
	revived := err == nil && result.ErrorCode == 0 && q.Kind == "revive" && reviveID == r.DeathID && r.Player.HP <= 0
	if revived {
		w.revivePlayer(r)
	}
	if err == nil && result.ErrorCode == 0 && q.Kind == "use_item" {
		w.applyPotionReceipt(r, receipt, time.Now())
	}
	w.send(r, p.MsgS2COperationResult, f.Request, result)
	if err != nil && hardAuthorityFailure(persistence.Code(err)) {
		w.isolateRoom(r)
		return
	}
	w.assets(r)
	if err == nil && result.ErrorCode == 0 {
		w.equipmentRecycleResult(r, receipt)
	}
	w.snapshot(r)
	if revived {
		w.modes(r, time.Now())
	}
	w.after(r)
}
