// 本文件衔接角色资产事务与房间回调，按会话、场景和操作身份更新内存资产并返回回执。
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

// assetBinding 为在途资产操作固定房间、授权、代次和操作身份。
type assetBinding struct {
	room        *room            // 接受原资产请求的房间实例。
	auth        persistence.Auth // 发往持久层的原角色会话授权。
	generation  uint64           // 此房间的资产请求代次。
	id          string           // 原经济操作的唯一编号。
	fingerprint string           // 原操作的业务内容指纹。
}

// submitAsset 冻结原操作，提交事务，队列满时按原任务退避。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，q 记录资产请求，f 记录协议帧，reviveID 标识当前业务使用的复活。
func (w *worker) submitAsset(r *room, q persistence.AssetRequest, f wire.Frame, reviveID string) {
	// 首领副本只保留战斗快照，不能沿普通背包路径发起任何永久操作。
	if r.bossBattle != nil {
		return
	}
	if r.AssetGeneration == math.MaxUint64 {
		w.isolateRoom(r)
		return
	}
	r.AssetGeneration++
	if q.Kind == persistence.AssetClaimHostingTime {
		q.HostingClaimAt = time.Now()
	}
	// binding 初始化用作会话绑定的资产会话绑定。
	binding := assetBinding{room: r, auth: r.Auth, generation: r.AssetGeneration, id: domain.Hex(q.Context.OpId), fingerprint: q.Fingerprint}
	r.Pending = true
	r.PendingOperationID = binding.id
	r.PendingFingerprint = binding.fingerprint
	// store 是原房间的运行服视图，事务和重试都用它核对角色的持久围栏。
	store := w.roomStore(r)
	// j 记录当前位置的下标，使用任务或职业编号表示。
	var j job
	j = job{target: w, run: func() func(*worker) {
		// role 保存由资产带有回执取得的角色，result 保存由资产带有回执取得的结果，receipt 保存由资产带有回执取得的回执。
		// err 接收资产带有回执过程中可能产生的错误，供后续失败分支处理。
		role, result, receipt, err := store.AssetWithReceipt(w.e.ctx, binding.auth, q)
		// w 记录房间执行器。
		return func(w *worker) {
			w.finishAsset(binding, q, f, reviveID, role, result, receipt, err, &j)
		}
	}}
	if !w.submit(j) {
		r.PendingRetry = &j
		r.RetryAt = time.Now().Add(100 * time.Millisecond)
	}
}

// finishAsset 在房间分片核对回执，确认后应用持久状态和必要的临时效果。
// w 是调用该方法的房间执行器实例，b 记录b，使用资产会话绑定表示，q 记录资产请求，f 记录协议帧，reviveID 标识当前业务使用的复活，role 记录角色。
// result 记录结果，使用操作结果表示，receipt 记录回执，使用操作表示，err 保存当前操作的失败原因，供调用方检查或返回，retry 记录重试，使用任务或职业编号表示。
func (w *worker) finishAsset(b assetBinding, q persistence.AssetRequest, f wire.Frame, reviveID string, role domain.Role, result *pb.OperationResult, receipt persistence.Operation, err error, retry *job) {
	// r 保存当前处理所用的房间。
	r := b.room
	// 回调必须属于当前房间和同一在途经济操作；旧会话与旧代次直接忽略。
	if w.rooms[b.auth.RoleID] != r || r.Auth != b.auth || r.Isolated || !r.Pending || r.AssetGeneration != b.generation || r.PendingOperationID != b.id || r.PendingFingerprint != b.fingerprint {
		return
	}
	// autoPotion 保存根据类型与资产自动使用药水计算的自动药水。
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
	r.resumeAfterPending(time.Now())
	r.PendingRetry = nil
	r.PendingOperationID = ""
	r.PendingFingerprint = ""
	if autoPotion && (err != nil || result.ErrorCode != 0 || !matchesAutomaticPotion(b, q, receipt)) {
		w.isolateRoom(r)
		return
	}
	// modeLearned 保存当前处理所用的模式已学习。
	modeLearned := false
	if err == nil && role.Revision >= r.Role.Revision {
		if result.ErrorCode == 0 && q.Kind == "learn_skill" {
			// known 保存由技能取得的已知状态。
			_, known := r.Role.Skill(q.SkillID)
			// learned 保存由技能取得的已学习。
			_, learned := role.Skill(q.SkillID)
			// skill 接收技能供后续步骤使用，exists 保存由技能取得的存在。
			skill, exists := w.e.Store.Cfg.Fixture.Skill(q.SkillID)
			// activation 接收“activation”供后续步骤使用。
			activation := skill.Activation()
			modeLearned = exists && !known && learned && (activation == "toggle" || activation == "charge" || activation == "passive_proc")
			if modeLearned {
				r.Melee.Revision++
			}
		}
		previousPrivilege := r.Role.PrivilegeUntil
		observedAt := r.acceptRoleCardReceipt(role, receipt, time.Now())
		r.Role = role
		w.acceptRolePrivilege(r, observedAt, previousPrivilege)
		// 仓储只迁移背包及仓库，不改变已穿属性或任何临时身体状态。
		if q.Kind != "warehouse" && !autoPotion {
			w.refreshPlayerStats(r)
		}
	}
	if err != nil {
		result.ErrorCode = persistence.Code(err)
	}
	// revived 保存根据错误与“nil”的计算结果与错误结果码与当前数据的计算结果的计算结果与类型与当前数据的计算结果的计算结果与复活标识与死亡标识的计算结果的计算结果与生命值与当前数据的计算结果计算的“revived”。
	revived := err == nil && result.ErrorCode == 0 && q.Kind == "revive" && reviveID == r.DeathID && r.Player.HP <= 0
	// 只有原死亡标记仍匹配且永久复活已确认时才改变临时身体。
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
		if receipt.GoldMarket != nil && receipt.GoldMarket.Action == 1 {
			w.wakeGoldMarket(w.roomStore(r).Cfg.RunID)
		}
		if receipt.Warehouse != nil && (receipt.Warehouse.Scope != persistence.WarehouseShared || r.Role.PrivilegeUntil.After(time.Now())) {
			// observed 保存由协议取得的观察到的。
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
