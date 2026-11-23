// 本文件处理材料技能的准备、原操作扣料、未知结果重试和回执确认后的技能生效。
package game

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"time"
)

// 材料校验与临时施法准备分开；历史回执优先，不能让旧scene/input遮蔽已经发生的消耗。
// w 持有房间 r，frame 携带原材料施法请求和客户端请求号。
func (w *worker) materialCast(r *room, frame wire.Frame) {
	// request 保存解析出的施法意图、原操作上下文与指定材料实例。
	var request pb.CastWithMaterialRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || request.Operation == nil || len(request.Operation.OpId) != 16 || len(request.MaterialInstanceId) != 16 {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	// q 汇集扣料事务需要的身份、版本、材料实例与内容摘要。
	q := persistence.AssetRequest{Kind: "cast_material", Fingerprint: potionFingerprint(frame), Context: request.Operation, ItemID: domain.Hex(request.MaterialInstanceId), ItemVersion: request.MaterialVersion, SkillID: request.SkillId, SceneEpoch: request.SceneEpoch, CasterLife: request.CasterLife}
	// plan 只在新的施法通过门禁后保存冻结的执行计划。
	var plan *preparedCast
	// 新绑定的输入窗口从1开始；查询旧回执不能把旧高序号写入新窗口。
	// historical 表示指令序号已进入持久水位，应先重查原回执，避免用当前场景拒绝历史提交。
	historical := q.Context.CommandSeq > 0 && q.Context.CommandSeq <= uint64(r.Role.EconomicSeq)
	if !historical {
		switch {
		case q.Context.CommandSeq != uint64(r.Role.EconomicSeq+1) || q.Context.ExpectedRevision != uint64(r.Role.Revision) || q.Context.ExpectedRuleVersion != w.e.Store.Cfg.Fixture.RuleVersion:
			q.CastGateCode = p.CodeOperationConflict
		case request.SceneEpoch != r.Scene || request.CasterLife != r.Player.Life:
			q.CastGateCode = p.CodeStaleSession
		case !acceptInput(r, request.InputSequence):
			q.CastGateCode = p.CodeStaleInput
		default:
			plan, q.CastGateCode = w.prepareMaterialCast(r, q, &request, time.Now())
		}
	}
	w.submitMaterialCast(r, q, plan, request.InputSequence, &frame)
}

// prepareMaterialCast 校验原操作与施法作用域并冻结计划，避免费用失败留下动作。
// w 持有目标房间 r；q 固定消耗的材料身份，request 提供技能和目标，now 用于检查行动时钟。
func (w *worker) prepareMaterialCast(r *room, q persistence.AssetRequest, request *pb.CastWithMaterialRequest, now time.Time) (*preparedCast, uint32) {
	if w.isSummonSkill(request.SkillId) {
		// plan 是尚未扣费的召唤方案，code 是其准备阶段的受理结果。
		plan, code := w.prepareCast(r, request.SkillId, request.TargetHandle, request.Direction, now, true, request.TargetGrid)
		if code != 0 {
			return nil, code
		}
		// 乐观模式已过期：走正式失败回执，不耗符，不把材料RPC转为新的Recall。
		if plan.Summon.Recall {
			return nil, p.CodeSkillStateBlocked
		}
		// code 表示召唤所用材料的实例、版本及装备槽是否满足要求。
		if code := persistence.CastingSupplyGate(r.Role, q, w.e.Store.Cfg.Fixture); code != 0 {
			return nil, code
		}
		return plan, 0
	}
	// code 表示材料槽及指定实例是否满足当前技能消耗规则。
	if code := persistence.CastingSupplyGate(r.Role, q, w.e.Store.Cfg.Fixture); code != 0 {
		return nil, code
	}
	return w.prepareCast(r, request.SkillId, request.TargetHandle, request.Direction, now, true, request.TargetGrid)
}

// applyMaterialReceipt 核对持久回执和原施法生命，首次确认才接受冻结技能。
// w 把 receipt 对应的原 plan 应用到房间 r，input 关联这次施法意图，now 作为受理时钟的起点。
func (w *worker) applyMaterialReceipt(r *room, receipt persistence.Operation, plan *preparedCast, input uint32, now time.Time) bool {
	if receipt.Kind != "cast_material" || receipt.Code != 0 || receipt.Casting == nil || plan == nil || plan.Scene != r.Scene || plan.Caster.World != r.currentWorld().Ref || plan.Caster.Life != r.Player.Life || receipt.Casting.SkillID != plan.Skill.ID {
		return false
	}
	// watermark 指向对应业务已经应用的最新序号，手动和自动各有独立水位。
	watermark := &r.MaterialEconomicSeq
	// sequence 是本次确认序号，手动施法读取经济指令序号，自动施法读取来源序号。
	sequence := receipt.CommandSeq
	if receipt.Casting.Scope == "auto" {
		watermark = &r.MaterialSourceSeq
		sequence = receipt.SourceSeq
	}
	if sequence <= *watermark {
		return false
	}
	*watermark = sequence
	return w.acceptPreparedCast(r, plan, input, now) == 0
}

// 原随机持续时长和材料身份冻结在job闭包；未知提交只重查同一事务，不重新抽样或再次耗符。
// w 为房间 r 提交原资产请求 q 和冻结的 plan。input 关联施法输入，frame 用于回答手动请求，自动施法时为空。
func (w *worker) submitMaterialCast(r *room, q persistence.AssetRequest, plan *preparedCast, input uint32, frame *wire.Frame) {
	if plan != nil && (r.ScheduleID == math.MaxUint64 || r.CastID == math.MaxUint64) {
		q.CastGateCode, plan = p.CodeSkillStateBlocked, nil
	}
	if q.CastGateCode == 0 && !w.reserveSummonPet(r, plan, domain.Hex(q.Context.OpId), time.Now()) {
		q.CastGateCode, plan = p.CodeSkillStateBlocked, nil
	}
	r.MaterialGeneration++
	// binding 固定原房间、角色授权、场景、出生代次及当前操作代次。
	binding := materialCommitBinding{roleID: r.Role.ID, room: r, auth: r.Auth, scene: r.Scene, life: r.Player.Life, generation: r.MaterialGeneration, conn: r.Conn}
	r.Pending = true
	r.PendingOperationID = domain.Hex(q.Context.OpId)
	r.PendingFingerprint = q.Fingerprint
	// auth 保存持久任务应使用的原授权，重试时仍沿用它。
	auth := r.Auth
	// store 保存原施法房间的运行服视图，延迟回执仍核对同一运行服围栏。
	store := w.roomStore(r)
	// pending 是可能退避或重试的原扣料任务。
	var pending job
	pending = job{target: w, run: func() func(*worker) {
		// role、result、receipt 分别是事务后的角色视图、协议结果和持久回执，err 说明提交或确认是否失败。
		role, result, receipt, err := store.AssetWithReceipt(w.e.ctx, auth, q)
		// w 是接收回执且有权改写房间的原分片。
		return func(w *worker) {
			w.finishMaterialCast(binding, q, plan, input, frame, role, result, receipt, err, &pending)
		}
	}}
	if !w.submit(pending) {
		r.PendingRetry = &pending
		r.RetryAt = time.Now().Add(100 * time.Millisecond)
	}
}

// materialCommitBinding 保存材料消耗结果可应用的原房间、授权和操作身份。
type materialCommitBinding struct {
	// roleID 是用于在分片中查找现房间的角色编号。
	roleID string
	// room 是原提交对应的房间实例。
	room *room
	// auth 是原提交的持久角色授权。
	auth persistence.Auth
	// scene 和 life 限定原施法场景与出生，generation 限定原在途扣料操作。
	scene, life, generation uint64
	// conn 是原请求的附着连接，操作结果只发给仍匹配的原连接。
	conn Connection
}

// finishMaterialCast 对齐原回调，重试未知提交并且仅在确认后推动技能和资产。
// w 从 binding 找回原房间，q 和 plan 固定扣料与施法。input 和 frame 保留客户端原请求身份。
// role、result、receipt 是事务返回的角色视图、协议结果和回执，err 是事务错误，retry 指向同一待确认任务。
func (w *worker) finishMaterialCast(binding materialCommitBinding, q persistence.AssetRequest, plan *preparedCast, input uint32, frame *wire.Frame, role domain.Role, result *pb.OperationResult, receipt persistence.Operation, err error, retry *job) {
	// r 是扣料任务发出时的原房间。
	r := binding.room
	// current 是该角色此刻由分片持有的房间，可能已重建。
	current := w.rooms[binding.roleID]
	// owns 表示回执匹配当前房间的在途代次、操作编号和内容指纹，有权重试或释放等待状态。
	owns := current == r && r.Pending && r.MaterialGeneration == binding.generation && r.PendingOperationID == domain.Hex(q.Context.OpId) && r.PendingFingerprint == q.Fingerprint
	// respond 要求存在手动请求，且原连接与原授权仍有效，才可回复原请求号。
	respond := owns && frame != nil && r.Conn == binding.conn && r.Auth == binding.auth
	if err != nil && persistence.Code(err) == p.CodeOperationPending {
		if owns {
			r.PendingRetry = retry
			r.RetryAt = time.Now().Add(500 * time.Millisecond)
		}
		if respond {
			result.Pending = true
			w.send(r, p.MsgS2COperationResult, frame.Request, result)
		}
		return
	}
	// 迟到的永久事实仍更新同角色资产观察；旧job无权释放新一代Pending或复用新连接的request。
	if err == nil && current != nil && role.ID == binding.roleID && role.Revision >= current.Role.Revision {
		current.Role = role
		transitionEventPrivilege(current, time.Now(), role.PrivilegeUntil)
		w.refreshPlayerStats(current)
	}
	if !owns {
		if current != nil {
			w.assets(current)
			w.snapshot(current)
		}
		return
	}
	r.resumeAfterPending(time.Now())
	r.PendingRetry = nil
	r.PendingOperationID = ""
	r.PendingFingerprint = ""
	// applied 表示本次确认已实际接受冻结施法，否则撤销仍未消费的宠物出生预约。
	applied := false
	if err == nil && result.ErrorCode == 0 && r.Auth == binding.auth && r.Scene == binding.scene && r.Player.Life == binding.life {
		applied = w.applyMaterialReceipt(r, receipt, plan, input, time.Now())
	}
	if !applied {
		w.cancelSummonPet(r, plan)
	}
	if respond {
		if err != nil {
			result.ErrorCode = persistence.Code(err)
		}
		w.send(r, p.MsgS2COperationResult, frame.Request, result)
	}
	if err != nil && r.Auth == binding.auth && (frame == nil || hardAuthorityFailure(persistence.Code(err))) {
		w.isolateRoom(r)
		return
	}
	w.assets(r)
	w.snapshot(r)
	w.after(r)
}

// hardAuthorityFailure 判断错误是否意味着当前会话授权已不能继续安全执行。
// code 是事务结果码，旧路由、认证失败或过期会话意味着不能继续执行。
func hardAuthorityFailure(code uint32) bool {
	return code == p.CodeRouteChanged || code == p.CodeUnauthenticated || code == p.CodeStaleSession
}

// autoMaterialCast 在自动施法中准备合法地格和配置材料，保持统一事务约束。
// w 为房间 r 检验 skill 与目标格 grid，now 用于施法门禁，need 汇总合法技能仅缺魔法值时的消耗门槛。
func (w *worker) autoMaterialCast(r *room, skill devconfig.Skill, grid devconfig.Point, now time.Time, need *automaticManaNeed) bool {
	if !w.e.Store.Cfg.Fixture.CastingSupply.Enabled {
		return false
	}
	// q 使用下一来源序号建立自动扣料请求，并在找到材料槽后填入原实例和版本。
	q := persistence.AssetRequest{Kind: "cast_material", SkillID: skill.ID, SceneEpoch: r.Scene, CasterLife: r.Player.Life, SourceSeq: r.Role.SourceSeq + 1}
	// equipped 是当前遍历到的穿戴项，用于从中找到技能消耗材料槽。
	for _, equipped := range r.Role.Equipped {
		if equipped.Slot == devconfig.CastingSupplySlot {
			q.ItemID = equipped.Item.ID
			q.ItemVersion = uint64(equipped.Item.Version)
			break
		}
	}
	if persistence.CastingSupplyGate(r.Role, q, w.e.Store.Cfg.Fixture) != 0 {
		return false
	}
	// plan 保存通过校验的自动施法方案，code 区分可释放、仅缺魔法值和其他门禁拒绝。
	plan, code := w.prepareCastRecordingMana(r, skill.ID, 0, r.Player.Direction, now, true, pointPB(grid))
	if !need.ready(r, plan, code) {
		return false
	}
	q.Context = &pb.OperationContext{OpId: domain.Bytes(domain.ID()), ExpectedRevision: uint64(r.Role.Revision), ExpectedRuleVersion: w.e.Store.Cfg.Fixture.RuleVersion}
	q.Fingerprint = persistence.Hash([]byte(fmt.Sprintf("auto:%d:%d:%d:%d:%s:%d:%d,%d", q.SourceSeq, q.SceneEpoch, q.CasterLife, q.SkillID, q.ItemID, q.ItemVersion, grid.X, grid.Y)))
	w.submitMaterialCast(r, q, plan, 0, nil)
	return true
}
