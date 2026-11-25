// 本文件处理原角色对镜像名单、发布快照、开战和格子观察的协议请求。
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
	mirrorQuery     uint32 = iota + 1 // 读取赛季、次数、系统名单与原战报。
	mirrorPublish                     // 冻结并发布当前角色的防守快照。
	mirrorEnter                       // 选择已签发的原名单项，预约一个挑战。
	mirrorReady                       // 确认原场次后启动双方自动战斗。
	mirrorSurrender                   // 已开始战斗时主动提交失败。
	mirrorObserve                     // 读取原场次的当前状态与整帧实体。
)

const (
	mirrorPreparing uint32 = iota + 1 // 已预约，等待当前连接确认镜像首帧。
	mirrorFighting                    // 两侧场景正在按服务端时钟战斗。
	mirrorSettling                    // 已冻结唯一终态，等待永久事务。
	mirrorFinished                    // 永久终态已经确认。
)

// allowMirrorLobby 保证镜像与其他挑战互斥，普通挂机及其原角色资产保持独立。
func allowMirrorLobby(r *room) bool {
	return r.personalBoss == nil && r.worldBossObservation == nil && r.dungeonMatch == nil && r.dungeonStart == nil &&
		(r.Travel == nil || r.Travel.Phase >= travelCommitted) && !pendingPersonalBossTransfer(r) && !mapTravelQuiescing(r)
}

// validMirrorRequest 拒绝携带客户端对手身份、跨场数据或半套操作编号的请求。
func validMirrorRequest(q *pb.MirrorArenaRequest) bool {
	switch q.Action {
	case mirrorQuery, mirrorPublish:
		return len(q.OfferId) == 0 && len(q.ChallengeId) == 0 && q.BattleSceneEpoch == 0
	case mirrorEnter:
		return validMirrorID(q.OfferId) && validMirrorID(q.ChallengeId) && q.BattleSceneEpoch == 0
	case mirrorReady, mirrorSurrender, mirrorObserve:
		return len(q.OfferId) == 0 && validMirrorID(q.ChallengeId) && q.BattleSceneEpoch != 0
	default:
		return false
	}
}

func validMirrorID(id []byte) bool {
	if len(id) != 16 {
		return false
	}
	return domain.Hex(id) != "00000000000000000000000000000000"
}

// mirrorArenaCommand 只接受当前已确认的普通角色连接。镜像场中没有客户端指定目标或攻击指令。
func (w *worker) mirrorArenaCommand(r *room, frame wire.Frame, now time.Time) {
	var q pb.MirrorArenaRequest
	if proto.Unmarshal(frame.Payload, &q) != nil || len(q.ProtoReflect().GetUnknown()) != 0 || q.SceneEpoch != r.Scene || !validMirrorRequest(&q) {
		w.sendMirrorError(r, frame.Request, p.CodeInvalidArgument)
		return
	}
	if !r.Ready || !allowMirrorLobby(r) {
		w.sendMirrorError(r, frame.Request, p.CodeLineRecovering)
		return
	}
	store := w.roomStore(r)
	cfg := store.Cfg.Fixture.MirrorArena
	if cfg == nil {
		code := uint32(0)
		if q.Action != mirrorQuery {
			code = p.CodeLineForbidden
		}
		w.sendMirrorError(r, frame.Request, code)
		return
	}
	switch q.Action {
	case mirrorQuery, mirrorPublish:
		w.mirrorLobbyRead(r, frame.Request, q.Action == mirrorPublish, store, cfg)
	case mirrorEnter:
		w.beginMirrorArena(r, frame.Request, domain.Hex(q.OfferId), domain.Hex(q.ChallengeId), store, cfg, now)
	default:
		w.observeMirrorArena(r, frame.Request, &q, now)
	}
}

// mirrorLobbyRead 在同一有界任务中确认发布和查询，回包前检查原角色附着仍然有效。
func (w *worker) mirrorLobbyRead(r *room, request uint32, publish bool, store *persistence.Store, cfg *devconfig.MirrorArenaConfig) {
	if r.PreviewInFlight || r.PreviewGeneration == math.MaxUint64 || publish && (r.Pending || r.Player.HP <= 0) {
		w.sendMirrorError(r, request, p.CodeServerBusy)
		return
	}
	r.PreviewGeneration++
	r.PreviewInFlight = true
	bound := skillPreviewBinding{room: r, auth: r.Auth, conn: r.Conn, scene: r.Scene, generation: r.PreviewGeneration}
	var publication persistence.MirrorPublish
	if publish {
		publication = persistence.MirrorPublish{ExpectedRevision: r.Role.Revision, Snapshot: w.freezeMirrorCombatSnapshot(r, &cfg.Map)}
	}
	recoverCold := r.mirrorMatch == nil
	if !w.submit(job{target: w, run: func() func(*worker) {
		var info persistence.MirrorInfo
		var err error
		if recoverCold {
			_, err = store.RecoverMirrorChallenge(w.e.ctx, bound.auth)
		}
		if err == nil && publish {
			_, err = store.PublishMirrorDefense(w.e.ctx, bound.auth, publication)
		}
		if err == nil {
			info, err = store.QueryMirrorArena(w.e.ctx, bound.auth)
		}
		return func(w *worker) { w.finishMirrorLobbyRead(bound, request, info, err) }
	}}) {
		r.PreviewInFlight = false
		w.sendMirrorError(r, request, p.CodeServerBusy)
	}
}

func (w *worker) finishMirrorLobbyRead(bound skillPreviewBinding, request uint32, info persistence.MirrorInfo, err error) {
	r := bound.room
	if !r.PreviewInFlight || r.PreviewGeneration != bound.generation {
		return
	}
	r.PreviewInFlight = false
	if w.rooms[bound.auth.RoleID] != r || r.Isolated || !r.Ready || r.Auth != bound.auth || r.Conn != bound.conn || r.Scene != bound.scene ||
		bound.conn == nil || !bound.conn.Alive() || bound.conn.Binding() != uint64(bound.auth.SessionEpoch) {
		return
	}
	if err != nil {
		w.sendMirrorError(r, request, persistence.Code(err))
		return
	}
	fixture := &w.roomStore(r).Cfg.Fixture
	state := mirrorLobbyState(r, info, fixture)
	if run := r.mirrorMatch; run != nil && run.grant.ChallengeID != "" && run.auth == r.Auth {
		mergeMirrorRun(state, run)
	}
	w.send(r, p.MsgS2CMirrorArenaState, request, state)
}

// observeMirrorArena 确认当前来源场景、预约和战斗标识，避免早期加载或旧票触发开战。
func (w *worker) observeMirrorArena(r *room, request uint32, q *pb.MirrorArenaRequest, now time.Time) {
	run := r.mirrorMatch
	if run == nil || run.grant.ChallengeID != domain.Hex(q.ChallengeId) || q.BattleSceneEpoch != run.scene || run.auth != r.Auth {
		w.sendMirrorError(r, request, p.CodeInvalidArgument)
		return
	}
	switch q.Action {
	case mirrorReady:
		if run.phase == mirrorPreparing {
			w.openMirrorCombat(r, run, now)
		}
	case mirrorSurrender:
		if run.phase != mirrorFighting {
			w.sendMirrorError(r, request, p.CodeLineForbidden)
			return
		}
		// 先兑现已经到期的真实命中。玩家不能把已裁决胜负改成主动认输。
		w.advanceMirrorRun(r, run, now)
		if run.phase == mirrorFighting {
			run.finish(persistence.MirrorLost, now)
		}
	}
	w.sendMirrorRun(r, request, run)
	if run.battle != nil {
		w.sendMirrorCombatFrame(r, run, now)
	}
}

func (w *worker) openMirrorCombat(r *room, run *mirrorRun, now time.Time) {
	if !now.Before(run.prepareUntil) || w.e.stopped.Load() {
		run.finish(persistence.MirrorInterrupted, now)
		return
	}
	deadline := minBossTime(now.Add(run.grant.Duration), run.grant.ExpiresAt)
	seats := [2]domain.MirrorCombatSnapshot{run.grant.AttackerSnapshot, run.grant.DefenseSnapshot}
	if run.battle = w.newMirrorCombat(run.cfg, seats, now, deadline, run.scene); run.battle == nil {
		run.finish(persistence.MirrorInterrupted, now)
		return
	}
	run.phase, run.deadline = mirrorFighting, deadline
}
