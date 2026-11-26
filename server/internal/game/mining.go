// 本文件管理原角色矿区开停、锄头选择与场景内可观察状态。
package game

import (
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

const (
	miningQuery uint32 = iota // 查询原矿区及所选工具的状态。
	miningStart               // 选择一件背包锄头，以服务端时钟开采。
	miningStop                // 停止新增判定，已受理的一轮仍按原身份提交。
)

// roomMining 保存当前矿图内的运行状态。真正的耗量、纯度和资产由持久回执提供。
type roomMining struct {
	active    bool         // 当前可以继续产生新判定。
	toolID    string       // 原命令选择的背包锄头实例。
	startedAt time.Time    // 本次完整挖矿间隔的起点。
	lastOre   *domain.Item // 最近一次实际出矿的原实例与纯度。
	attempts  uint64       // 当前矿区已经确认的判定数。
	successes uint64       // 当前矿区已经确认的出矿数。
	sequence  uint64       // 当前客户端场景下的状态观察顺序。
	code      uint32       // 最近一次因容量、工具或时长停止的原因。
}

// miningCommand 只受理已确认在原矿层的角色，旧场景和无效工具不能开始挖矿。
func (w *worker) miningCommand(r *room, frame wire.Frame, now time.Time) {
	var request pb.MiningIntent
	if proto.Unmarshal(frame.Payload, &request) != nil || len(request.ProtoReflect().GetUnknown()) != 0 ||
		request.SceneEpoch != r.Scene || request.Action > miningStop ||
		len(request.ToolInstanceId) != 0 && (request.Action != miningStart || len(request.ToolInstanceId) != 16) {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	f := &w.roomStore(r).Cfg.Fixture
	layer, allowed := f.MiningLayer(r.MapID)
	if !allowed || !r.Ready || r.Player.HP <= 0 || r.Role.Level < layer.MinimumLevel {
		w.failure(r, frame, p.CodeLineForbidden)
		return
	}
	state := &r.mining
	switch request.Action {
	case miningStart:
		selected := ""
		if len(request.ToolInstanceId) != 0 {
			selected = domain.Hex(request.ToolInstanceId)
		}
		if state.active && (selected == "" || selected == state.toolID) {
			break // 重复开始不会重置本轮判定时钟。
		}
		if r.Pending {
			w.sendMiningState(r, frame.Request, p.CodeOperationPending)
			return
		}
		tool, code := miningToolAvailable(r.Role, f, r.MapID, selected)
		if code != 0 {
			w.sendMiningState(r, frame.Request, code)
			return
		}
		state.active, state.toolID, state.startedAt, state.code = true, tool.ID, now, 0
		// 开采前释放方向和地面寻路，挖矿动作固定在当前地格。
		r.Auto, r.Manual, r.Destination = false, false, nil
		r.Navigation = navigationState{}
	case miningStop:
		state.active, state.startedAt, state.code = false, time.Time{}, 0
	}
	w.sendMiningState(r, frame.Request, state.code)
}

// miningToolAvailable 在抽样前判断一件原工具及一格可用背包，事务内还会重查。
func miningToolAvailable(role domain.Role, f *devconfig.Fixture, mapID uint64, selected string) (domain.Item, uint32) {
	if len(role.Items) >= role.Capacity {
		return domain.Item{}, p.CodeEquipmentBagFull
	}
	tool, ok := persistence.MiningToolForAttempt(role, f, mapID, selected)
	if !ok {
		return domain.Item{}, p.CodeEquipmentChanged
	}
	return tool, 0
}

// sendMiningState 使用已提交角色视图投影工具，观察顺序与其他场景隔离。
func (w *worker) sendMiningState(r *room, request, code uint32) {
	f := &w.roomStore(r).Cfg.Fixture
	if _, ok := f.MiningLayer(r.MapID); !ok || r.Conn == nil || !r.Ready {
		return
	}
	state := &r.mining
	if state.sequence == math.MaxUint64 {
		w.isolateRoom(r)
		return
	}
	state.sequence++
	tool, hasTool := persistence.MiningToolForAttempt(r.Role, f, r.MapID, state.toolID)
	if !hasTool && !state.active {
		tool, hasTool = persistence.MiningToolForAttempt(r.Role, f, r.MapID, "")
	}
	result := &pb.MiningState{SceneEpoch: r.Scene, MapId: r.MapID, StateSequence: state.sequence, Active: state.active,
		CanStart: hasTool && r.Player.HP > 0 && len(r.Role.Items) < r.Role.Capacity, ErrorCode: code,
		Attempts: state.attempts, Successes: state.successes}
	if hasTool {
		result.Tool = tool.Proto()
	}
	if state.lastOre != nil {
		result.LastOre = state.lastOre.Proto()
	}
	w.send(r, p.MsgS2CMiningState, request, result)
}

// blocksMiningBattleCommand 阻止专用矿区进入战斗、首领和副本，仍保留回城、背包以及原工具操作。
func blocksMiningBattleCommand(id uint16, active bool) bool {
	switch id {
	case p.MsgC2SMoveIntent:
		return active
	case p.MsgC2SAutoBattle, p.MsgC2SAttackIntent, p.MsgC2SCastIntent, p.MsgC2SCastWithMaterial,
		p.MsgC2SSkillModeIntent, p.MsgC2SAutoTargetIntent, p.MsgC2SPersonalBossEnter,
		p.MsgC2SWorldBossEnter, p.MsgC2SMirrorArenaRequest, p.MsgC2SDungeonPartyRequest:
		return true
	default:
		return false
	}
}
