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
func (w *worker) materialCast(r *room, frame wire.Frame) {
	var request pb.CastWithMaterialRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || request.Operation == nil || len(request.Operation.OpId) != 16 || len(request.MaterialInstanceId) != 16 {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	q := persistence.AssetRequest{Kind: "cast_material", Fingerprint: potionFingerprint(frame), Context: request.Operation, ItemID: domain.Hex(request.MaterialInstanceId), ItemVersion: request.MaterialVersion, SkillID: request.SkillId, SceneEpoch: request.SceneEpoch, CasterLife: request.CasterLife}
	var plan *preparedCast
	// 新绑定的输入窗口从1开始；查询旧回执不能把旧高序号写入新窗口。
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

func (w *worker) prepareMaterialCast(r *room, q persistence.AssetRequest, request *pb.CastWithMaterialRequest, now time.Time) (*preparedCast, uint32) {
	if w.isSummonSkill(request.SkillId) {
		plan, code := w.prepareCast(r, request.SkillId, request.TargetHandle, request.Direction, now, true, request.TargetGrid)
		if code != 0 {
			return nil, code
		}
		// 乐观模式已过期：走正式失败回执，不耗符，不把材料RPC转为新的Recall。
		if plan.Summon.Recall {
			return nil, p.CodeSkillStateBlocked
		}
		if code := persistence.CastingSupplyGate(r.Role, q, w.e.Store.Cfg.Fixture); code != 0 {
			return nil, code
		}
		return plan, 0
	}
	if code := persistence.CastingSupplyGate(r.Role, q, w.e.Store.Cfg.Fixture); code != 0 {
		return nil, code
	}
	return w.prepareCast(r, request.SkillId, request.TargetHandle, request.Direction, now, true, request.TargetGrid)
}

func (w *worker) applyMaterialReceipt(r *room, receipt persistence.Operation, plan *preparedCast, input uint32, now time.Time) bool {
	if receipt.Kind != "cast_material" || receipt.Code != 0 || receipt.Casting == nil || plan == nil || plan.Scene != r.Scene || plan.Caster.World != r.currentWorld().Ref || plan.Caster.Life != r.Player.Life || receipt.Casting.SkillID != plan.Skill.ID {
		return false
	}
	watermark := &r.MaterialEconomicSeq
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
func (w *worker) submitMaterialCast(r *room, q persistence.AssetRequest, plan *preparedCast, input uint32, frame *wire.Frame) {
	if plan != nil && (r.ScheduleID == math.MaxUint64 || r.CastID == math.MaxUint64) {
		q.CastGateCode, plan = p.CodeSkillStateBlocked, nil
	}
	if q.CastGateCode == 0 && !w.reserveSummonPet(r, plan, domain.Hex(q.Context.OpId), time.Now()) {
		q.CastGateCode, plan = p.CodeSkillStateBlocked, nil
	}
	r.MaterialGeneration++
	binding := materialCommitBinding{roleID: r.Role.ID, room: r, auth: r.Auth, scene: r.Scene, life: r.Player.Life, generation: r.MaterialGeneration, conn: r.Conn}
	r.Pending = true
	r.PendingOperationID = domain.Hex(q.Context.OpId)
	r.PendingFingerprint = q.Fingerprint
	auth := r.Auth
	var pending job
	pending = job{target: w, run: func() func(*worker) {
		role, result, receipt, err := w.e.Store.AssetWithReceipt(w.e.ctx, auth, q)
		return func(w *worker) {
			w.finishMaterialCast(binding, q, plan, input, frame, role, result, receipt, err, &pending)
		}
	}}
	if !w.submit(pending) {
		r.PendingRetry = &pending
		r.RetryAt = time.Now().Add(100 * time.Millisecond)
	}
}

type materialCommitBinding struct {
	roleID                  string
	room                    *room
	auth                    persistence.Auth
	scene, life, generation uint64
	conn                    Connection
}

func (w *worker) finishMaterialCast(binding materialCommitBinding, q persistence.AssetRequest, plan *preparedCast, input uint32, frame *wire.Frame, role domain.Role, result *pb.OperationResult, receipt persistence.Operation, err error, retry *job) {
	r := binding.room
	current := w.rooms[binding.roleID]
	owns := current == r && r.Pending && r.MaterialGeneration == binding.generation && r.PendingOperationID == domain.Hex(q.Context.OpId) && r.PendingFingerprint == q.Fingerprint
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
	r.Pending = false
	r.PendingRetry = nil
	r.PendingOperationID = ""
	r.PendingFingerprint = ""
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

func hardAuthorityFailure(code uint32) bool {
	return code == p.CodeRouteChanged || code == p.CodeUnauthenticated || code == p.CodeStaleSession
}

func (w *worker) autoMaterialCast(r *room, skill devconfig.Skill, grid devconfig.Point, now time.Time) bool {
	if !w.e.Store.Cfg.Fixture.CastingSupply.Enabled {
		return false
	}
	q := persistence.AssetRequest{Kind: "cast_material", SkillID: skill.ID, SceneEpoch: r.Scene, CasterLife: r.Player.Life, SourceSeq: r.Role.SourceSeq + 1}
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
	plan, code := w.prepareCast(r, skill.ID, 0, r.Player.Direction, now, true, pointPB(grid))
	if code != 0 {
		return false
	}
	q.Context = &pb.OperationContext{OpId: domain.Bytes(domain.ID()), ExpectedRevision: uint64(r.Role.Revision), ExpectedRuleVersion: w.e.Store.Cfg.Fixture.RuleVersion}
	q.Fingerprint = persistence.Hash([]byte(fmt.Sprintf("auto:%d:%d:%d:%d:%s:%d:%d,%d", q.SourceSeq, q.SceneEpoch, q.CasterLife, q.SkillID, q.ItemID, q.ItemVersion, grid.X, grid.Y)))
	w.submitMaterialCast(r, q, plan, 0, nil)
	return true
}
