package game

import (
	"math"
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

func (w *worker) submitAsset(r *room, q persistence.AssetRequest, f wire.Frame, reviveID string) {
	if r.AssetGeneration == math.MaxUint64 {
		w.isolateRoom(r)
		return
	}
	r.AssetGeneration++
	binding := assetBinding{room: r, auth: r.Auth, generation: r.AssetGeneration, id: domain.Hex(q.Context.OpId), fingerprint: q.Fingerprint}
	r.Pending = true
	r.PendingOperationID = binding.id
	r.PendingFingerprint = binding.fingerprint
	var j job
	j = job{target: w, run: func() func(*worker) {
		role, result, receipt, err := w.e.Store.AssetWithReceipt(w.e.ctx, binding.auth, q)
		return func(w *worker) {
			w.finishAsset(binding, q, f, reviveID, role, result, receipt, err, &j)
		}
	}}
	if !w.submit(j) {
		r.PendingRetry = &j
		r.RetryAt = time.Now().Add(100 * time.Millisecond)
	}
}

func (w *worker) finishAsset(b assetBinding, q persistence.AssetRequest, f wire.Frame, reviveID string, role domain.Role, result *pb.OperationResult, receipt persistence.Operation, err error, retry *job) {
	r := b.room
	if w.rooms[b.auth.RoleID] != r || r.Auth != b.auth || r.Isolated || !r.Pending || r.AssetGeneration != b.generation || r.PendingOperationID != b.id || r.PendingFingerprint != b.fingerprint {
		return
	}
	autoPotion := q.Kind == persistence.AssetAutoUsePotion
	if err != nil && (persistence.Code(err) == p.CodeOperationPending || persistence.Code(err) == p.CodeServerBusy) {
		r.PendingRetry = retry
		r.RetryAt = time.Now().Add(500 * time.Millisecond)
		result.Pending = true
		if !autoPotion {
			w.send(r, p.MsgS2COperationResult, f.Request, result)
		}
		return
	}
	r.Pending = false
	r.PendingRetry = nil
	r.PendingOperationID = ""
	r.PendingFingerprint = ""
	if autoPotion && (err != nil || result.ErrorCode != 0 || !matchesAutomaticPotion(b, q, receipt)) {
		w.isolateRoom(r)
		return
	}
	modeLearned := false
	if err == nil && role.Revision >= r.Role.Revision {
		if result.ErrorCode == 0 && q.Kind == "learn_skill" {
			_, known := r.Role.Skill(q.SkillID)
			_, learned := role.Skill(q.SkillID)
			skill, exists := w.e.Store.Cfg.Fixture.Skill(q.SkillID)
			activation := skill.Activation()
			modeLearned = exists && !known && learned && (activation == "toggle" || activation == "charge" || activation == "passive_proc")
			if modeLearned {
				r.Melee.Revision++
			}
		}
		r.Role = role
		transitionEventPrivilege(r, time.Now(), role.PrivilegeUntil)
		// 仓储只迁移背包及仓库，不改变已穿属性或任何临时身体状态。
		if q.Kind != "warehouse" && !autoPotion {
			w.refreshPlayerStats(r)
		}
	}
	if err != nil {
		result.ErrorCode = persistence.Code(err)
	}
	revived := err == nil && result.ErrorCode == 0 && q.Kind == "revive" && reviveID == r.DeathID && r.Player.HP <= 0
	if revived {
		w.revivePlayer(r)
	}
	if err == nil && result.ErrorCode == 0 && (q.Kind == "use_item" || autoPotion && r.Player.Life == q.CasterLife && r.Player.HP > 0) {
		w.applyPotionReceipt(r, receipt, time.Now())
	}
	if !autoPotion {
		w.send(r, p.MsgS2COperationResult, f.Request, result)
	}
	if err != nil && hardAuthorityFailure(persistence.Code(err)) {
		w.isolateRoom(r)
		return
	}
	w.assets(r)
	if err == nil && result.ErrorCode == 0 {
		w.equipmentRecycleResult(r, receipt)
		if receipt.Warehouse != nil && (receipt.Warehouse.Scope != persistence.WarehouseShared || r.Role.PrivilegeUntil.After(time.Now())) {
			if observed := receipt.Warehouse.Proto(r.Role, receipt); observed != nil {
				w.send(r, p.MsgS2CWarehouseActionResult, f.Request, observed)
			}
		}
	}
	w.snapshot(r)
	if revived || modeLearned {
		w.modes(r, time.Now())
	}
	w.after(r)
}
