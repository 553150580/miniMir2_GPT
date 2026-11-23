// 本文件把个人首领的持久资格和历史发给同一原角色附着。
package game

import (
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// personalBossQuery 只读查询使用原房间的运行服权限，不占用持久资产提交槽。
func (w *worker) personalBossQuery(owner *room, frame wire.Frame) {
	var request pb.PersonalBossQuery
	if proto.Unmarshal(frame.Payload, &request) != nil || len(request.ProtoReflect().GetUnknown()) != 0 {
		w.failure(owner, frame, p.CodeInvalidArgument)
		return
	}
	if !owner.Ready || owner.PreviewInFlight {
		w.failure(owner, frame, p.CodeLineRecovering)
		return
	}
	if owner.PreviewGeneration == math.MaxUint64 {
		w.failure(owner, frame, p.CodeServerBusy)
		return
	}
	owner.PreviewInFlight = true
	owner.PreviewGeneration++
	bound := skillPreviewBinding{room: owner, conn: owner.Conn, auth: owner.Auth, scene: owner.Scene, generation: owner.PreviewGeneration}
	store := w.roomStore(owner)
	query := job{target: w, run: func() func(*worker) {
		info, err := store.QueryPersonalBoss(w.e.ctx, bound.auth)
		return func(w *worker) { w.finishPersonalBossQuery(bound, frame, info, err) }
	}}
	if !w.submit(query) {
		owner.PreviewInFlight = false
		w.failure(owner, frame, p.CodeServerBusy)
	}
}

// finishPersonalBossQuery 回到原执行器核对附着，并结合运行状态限制入口。
func (w *worker) finishPersonalBossQuery(bound skillPreviewBinding, frame wire.Frame, info persistence.PersonalBossInfo, err error) {
	owner := bound.room
	if owner.PreviewGeneration != bound.generation || !owner.PreviewInFlight {
		return
	}
	owner.PreviewInFlight = false
	if w.rooms[bound.auth.RoleID] != owner || owner.Auth != bound.auth || owner.Conn != bound.conn || owner.Scene != bound.scene ||
		owner.Isolated || !owner.Ready || bound.conn == nil || !bound.conn.Alive() || bound.conn.Binding() != uint64(bound.auth.SessionEpoch) {
		return
	}
	if err != nil {
		w.send(owner, p.MsgS2CPersonalBossInfo, frame.Request, &pb.PersonalBossInfo{ErrorCode: persistence.Code(err)})
		return
	}
	if info.EligibilityReason == persistence.PersonalBossEligible && (owner.currentWorld() != owner.PersonalWorld || owner.Player.HP <= 0 ||
		owner.personalBoss != nil || owner.worldBossObservation != nil || owner.Travel != nil && owner.Travel.Phase < travelCommitted || pendingPersonalBossTransfer(owner) || mapTravelQuiescing(owner)) {
		info.EligibilityReason = persistence.PersonalBossSceneForbidden
	}
	out := &pb.PersonalBossInfo{Open: info.Open, TemplateId: info.TemplateID, DisplayName: info.DisplayName, MapId: info.MapID,
		MonsterId: info.MonsterID, MinimumLevel: info.MinimumLevel, Day: info.Day, Successes: info.Successes, DailyLimit: info.DailyLimit,
		CanEnter: info.EligibilityReason == persistence.PersonalBossEligible, EligibilityReason: info.EligibilityReason,
		AvailableAtUnixMs: optionalBossUnixMillis(info.AvailableAt), ActiveChallengeId: info.ActiveChallengeID, ActiveExpiresAtUnixMs: optionalBossUnixMillis(info.ActiveExpiresAt)}
	for _, previous := range info.History {
		out.History = append(out.History, &pb.PersonalBossHistoryEntry{ChallengeId: previous.ChallengeID, TemplateId: previous.TemplateID,
			RuleVersion: previous.RuleVersion, Outcome: uint32(previous.Outcome), FinishedAtUnixMs: previous.FinishedAt.UnixMilli(),
			Damage: previous.Damage, HpRemaining: previous.HPRemaining, Recovered: previous.Recovered,
			RewardGold: previous.RewardGold, RewardBound: previous.RewardBound})
	}
	w.send(owner, p.MsgS2CPersonalBossInfo, frame.Request, out)
}

// optionalBossUnixMillis 用零区分从未开始冷却或没有活动预约的时间。
func optionalBossUnixMillis(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	return at.UnixMilli()
}
