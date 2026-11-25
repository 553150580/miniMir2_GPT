// 本文件在合法施法机会仅缺魔法值时选择背包药水，并核对唯一用药回执与回魔作用域。
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

// automaticManaNeed 记录第一次遇到的合法施法所需魔法值，供本轮考虑自动用药。
type automaticManaNeed struct {
	// cost 是候选技能的完整魔法消耗，零表示本轮还未记录候选。
	cost int64
}

// ready 判断能否直接释放；仅缺魔法值且总消耗不超过上限时在 need 记录候选。
// r 提供当前角色的魔法上限，plan 提供该技能消耗，code 是准备结果。
func (need *automaticManaNeed) ready(r *room, plan *preparedCast, code uint32) bool {
	if code == p.CodeSkillResource && need.cost == 0 && plan != nil && plan.Cost <= r.Player.MaxMP {
		need.cost = plan.Cost
	}
	return code == 0
}

// castAutomatically 尝试自动受理无需材料的技能，仅缺魔法值时记录该技能消耗供本轮尝试用药。
// w 持有房间 r；id 是技能编号，handle 是目标实体句柄，direction 是施法朝向，ground 是可选目标格。
// now 用于行动时钟，need 保存当前决策中的补蓝条件。
func (w *worker) castAutomatically(r *room, id, handle, direction uint32, now time.Time, need *automaticManaNeed, ground *pb.GridPoint, input uint32) bool {
	// plan 保存自动施法准备结果，code 区分成功、仅缺魔法值和其他限制。
	plan, code := w.prepareCastRecordingMana(r, id, handle, direction, now, false, ground)
	return need.ready(r, plan, code) && w.acceptPreparedCast(r, plan, input, now) == 0
}

// castWithManaRecovery 受理手动施法。新版规则下，合法意图仅缺魔法值时可另行提交用药。
// w 持有房间 r；q 是客户端原施法意图，now 用于校验动作、施法和用药时钟。
func (w *worker) castWithManaRecovery(r *room, q *pb.CastIntent, now time.Time) uint32 {
	if w.e.Store.Cfg.Fixture.RuleVersion < 24 {
		return w.castDirected(r, q.SkillId, q.TargetHandle, q.InputSequence, q.Direction, now, q.TargetGrid)
	}
	// plan 是当前手动施法的准备结果，code 是直接返回原意图的协议结果。
	plan, code := w.prepareCastRecordingMana(r, q.SkillId, q.TargetHandle, q.Direction, now, false, q.TargetGrid)
	// need 在准备阶段已经确定仅缺魔法值时保存药水判断需要的魔法消耗门槛。
	var need automaticManaNeed
	if need.ready(r, plan, code) {
		return w.acceptPreparedCast(r, plan, q.InputSequence, now)
	}
	w.tryManaPotion(r, need, now)
	return code
}

// tryManaPotion 在合法施法仅缺魔法值、没有剩余药水恢复且可用药时，提交一笔背包药水消耗。
// w 持有房间 r；need 给出施法所需的魔法值，now 用来判断用药冷却。
func (w *worker) tryManaPotion(r *room, need automaticManaNeed, now time.Time) bool {
	if r.currentWorld().mirrorCombat != nil {
		return w.mirrorPotion(r, need, now)
	}
	if r.bossBattle != nil || r.Pending || r.Isolated || need.cost <= r.Player.MP || r.Potions.MP > 0 || potionGate(r, r.Scene, now) != 0 || r.Role.SourceSeq == math.MaxInt64 {
		return false
	}
	// fixture 提供允许的魔法药水定义和本次经济操作应匹配的规则版本。
	fixture := w.e.Store.Cfg.Fixture
	// item 是按背包顺序检查的药水候选，必须适用于角色职业。
	for _, item := range r.Role.Items {
		if !persistence.AutoManaPotionEligible(item, r.Role.Job, fixture) {
			continue
		}
		// q 固定药水实例、场景、出生代次和下一来源编号，重试继续使用这份原请求。
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

// matchesAutomaticPotion 确认回执对应原自动用药且只恢复魔法，避免把其他药水或场景的效果应用到当前房间。
// b 保存原角色授权和指纹，q 是第一次提交的药水消耗请求，receipt 是数据库返回的确认。
func matchesAutomaticPotion(b assetBinding, q persistence.AssetRequest, receipt persistence.Operation) bool {
	// effect 是持久回执绑定的药水效果，必须与原场景和药水定义相符。
	effect := receipt.Potion
	return receipt.RoleID == b.auth.RoleID && receipt.Digest == b.fingerprint && receipt.SourceSeq == q.SourceSeq &&
		receipt.Kind == persistence.AssetAutoUsePotion && receipt.Code == 0 && effect != nil &&
		effect.Scene == strconv.FormatUint(q.SceneEpoch, 10) && effect.ItemID == q.ItemDefinitionID && effect.HP == 0 && effect.MP > 0
}
