package game

import (
	"fmt"
	"math"
	"strconv"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

type automaticManaNeed struct {
	cost int64
}

func (need *automaticManaNeed) ready(r *room, plan *preparedCast, code uint32) bool {
	if code == p.CodeSkillResource && need.cost == 0 && plan != nil && plan.Cost <= r.Player.MaxMP {
		need.cost = plan.Cost
	}
	return code == 0
}

func (w *worker) castAutomatically(r *room, id, handle, direction uint32, now time.Time, need *automaticManaNeed, ground *pb.GridPoint) bool {
	plan, code := w.prepareCastRecordingMana(r, id, handle, direction, now, false, ground)
	return need.ready(r, plan, code) && w.acceptPreparedCast(r, plan, 0, now) == 0
}

func (w *worker) castWithManaRecovery(r *room, q *pb.CastIntent, now time.Time) uint32 {
	if w.e.Store.Cfg.Fixture.RuleVersion < 24 {
		return w.castDirected(r, q.SkillId, q.TargetHandle, q.InputSequence, q.Direction, now, q.TargetGrid)
	}
	plan, code := w.prepareCastRecordingMana(r, q.SkillId, q.TargetHandle, q.Direction, now, false, q.TargetGrid)
	var need automaticManaNeed
	if need.ready(r, plan, code) {
		return w.acceptPreparedCast(r, plan, q.InputSequence, now)
	}
	w.tryManaPotion(r, need, now)
	return code
}

func (w *worker) tryManaPotion(r *room, need automaticManaNeed, now time.Time) bool {
	if r.Pending || r.Isolated || need.cost <= r.Player.MP || r.Potions.MP > 0 || potionGate(r, r.Scene, now) != 0 || r.Role.SourceSeq == math.MaxInt64 {
		return false
	}
	fixture := w.e.Store.Cfg.Fixture
	for _, item := range r.Role.Items {
		if !persistence.AutoManaPotionEligible(item, r.Role.Job, fixture) {
			continue
		}
		q := persistence.AssetRequest{
			Kind: persistence.AssetAutoUsePotion, ItemID: item.ID, ItemDefinitionID: item.ItemID, ItemVersion: uint64(item.Version),
			SceneEpoch: r.Scene, CasterLife: r.Player.Life, SourceSeq: r.Role.SourceSeq + 1,
			Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), ExpectedRevision: uint64(r.Role.Revision), ExpectedRuleVersion: fixture.RuleVersion},
		}
		q.Fingerprint = persistence.Hash([]byte(fmt.Sprintf("%d:%d:%d:%s:%d:%d", q.SourceSeq, q.SceneEpoch, q.CasterLife, q.ItemID, q.ItemDefinitionID, q.ItemVersion)))
		// 消耗确认前冻结原操作身份，恢复时重新选技能和目标，不续发旧意图。
		w.submitAsset(r, q, wire.Frame{}, "")
		return true
	}
	return false
}

func matchesAutomaticPotion(b assetBinding, q persistence.AssetRequest, receipt persistence.Operation) bool {
	effect := receipt.Potion
	return receipt.RoleID == b.auth.RoleID && receipt.Digest == b.fingerprint && receipt.SourceSeq == q.SourceSeq &&
		receipt.Kind == persistence.AssetAutoUsePotion && receipt.Code == 0 && effect != nil &&
		effect.Scene == strconv.FormatUint(q.SceneEpoch, 10) && effect.ItemID == q.ItemDefinitionID && effect.HP == 0 && effect.MP > 0
}
