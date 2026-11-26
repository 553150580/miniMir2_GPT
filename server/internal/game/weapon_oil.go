// 本文件在当前房间受理武器和翅膀用油，保护穿戴武器尚未入库的磨损。
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

// luckOilAvailable 只给当前可操作的角色身体开放新用油，并独立检查翅膀开关。
func (w *worker) luckOilAvailable(r *room, wing *pb.WingOilRef) bool {
	if !r.Ready || r.Player.HP <= 0 || r.bossBattle != nil {
		return false
	}
	fixture := &w.roomStore(r).Cfg.Fixture
	return fixture.LuckOil != nil && (wing == nil || fixture.Wings != nil && fixture.Wings.AllowLuckOil)
}

// validWeaponOilRef 在进入预览或等待队列之前限制武器实例编号与版本。
func validWeaponOilRef(ref *pb.WeaponOilItemRef) bool {
	return ref != nil && len(ref.InstanceId) == 16 && ref.Version > 0 && ref.Version < math.MaxInt64 &&
		len(ref.ProtoReflect().GetUnknown()) == 0
}

// validLuckOilTarget 只允许一件原武器或一件已激活翅膀，其实际归属由持久层确认。
func validLuckOilTarget(weapon *pb.WeaponOilItemRef, wing *pb.WingOilRef) bool {
	if wing == nil {
		return validWeaponOilRef(weapon)
	}
	return weapon == nil && validWingReference(wing.WingId, wing.Version, false) &&
		len(wing.ProtoReflect().GetUnknown()) == 0
}

// weaponOilPreview 使用角色既有的只读预览槽，回调仍须属于原连接和场景。
func (w *worker) weaponOilPreview(r *room, frame wire.Frame) {
	var request pb.WeaponOilQuoteRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || request.SceneEpoch == 0 || request.SceneEpoch != r.Scene ||
		len(request.UiSessionId) != 16 || !validLuckOilTarget(request.Weapon, request.Wing) || len(request.ProtoReflect().GetUnknown()) != 0 {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	if !w.luckOilAvailable(r, request.Wing) {
		w.failure(r, frame, p.CodeWeaponOilUnavailable)
		return
	}
	if r.Pending || r.PreviewInFlight || r.PreviewGeneration == math.MaxUint64 {
		w.failure(r, frame, p.CodeOperationPending)
		return
	}
	r.PreviewGeneration++
	r.PreviewInFlight = true
	binding := skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
	store := w.roomStore(r)
	work := job{target: w, run: func() func(*worker) {
		quote, err := store.PreviewWeaponOil(w.e.ctx, binding.auth, &request)
		return func(owner *worker) {
			if !owner.claimPreview(binding) {
				return
			}
			if !owner.luckOilAvailable(r, request.Wing) {
				owner.failure(r, frame, p.CodeWeaponOilUnavailable)
				return
			}
			if err != nil {
				owner.failure(r, frame, persistence.Code(err))
				return
			}
			owner.send(r, p.MsgS2CWeaponOilQuote, frame.Request, quote)
		}
	}}
	if !w.submit(work) {
		r.PreviewInFlight = false
		w.failure(r, frame, p.CodeServerBusy)
	}
}

// readWeaponOilCommit 解析原确认消息，提交仍由存储对拍原目标、界面和绑定授权。
func readWeaponOilCommit(frame wire.Frame) (*pb.WeaponOilCommitRequest, bool) {
	var request pb.WeaponOilCommitRequest
	valid := proto.Unmarshal(frame.Payload, &request) == nil && len(request.QuoteId) == 16 &&
		len(request.UiSessionId) == 16 && validLuckOilTarget(request.Weapon, request.Wing) && len(request.ProtoReflect().GetUnknown()) == 0
	return &request, valid
}

// weaponOilWearPending 只检查首次确认所指的穿戴武器。背包物品和原操作重放无需等待这笔磨损。
func weaponOilWearPending(r *room, request *pb.WeaponOilCommitRequest) bool {
	op, weapon := request.Operation, request.Weapon
	if weapon == nil || !weapon.Equipped || op == nil || len(op.OpId) != 16 || r.Role.EconomicSeq == math.MaxInt64 ||
		op.CommandSeq != uint64(r.Role.EconomicSeq+1) || op.ExpectedRevision == 0 || op.ExpectedRevision > uint64(r.Role.Revision) {
		return false
	}
	id := domain.Hex(weapon.InstanceId)
	for _, cost := range r.Wear.open {
		if cost.Slot == devconfig.WeaponSlot && cost.InstanceID == id && cost.Version == int64(weapon.Version) {
			return true
		}
	}
	return false
}

// deferWeaponOilCommit 先落库同一版本已经产生的耐久事实，再把原提交交给权威版本核验。
func (w *worker) deferWeaponOilCommit(r *room, c command, now time.Time) bool {
	if c.frame.ID != p.MsgC2SWeaponOilCommit || r.Pending || len(r.Wear.open) == 0 || !w.luckOilAvailable(r, nil) {
		return false
	}
	request, valid := readWeaponOilCommit(c.frame)
	if !valid || !weaponOilWearPending(r, request) || !w.flushWear(r, now, true) {
		return false
	}
	if r.Pending && !r.Isolated {
		w.queueAssetCommand(r, c)
	}
	return true
}
