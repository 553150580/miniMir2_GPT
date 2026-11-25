// 本文件把讨伐链招募路由到同运行服的唯一分片，并衔接角色附着和断线。
package game

import (
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// dungeonPartyPresence 由原角色分片发布不可变连接快照，招募分片只读取。
// 连接关闭与新票确认的时刻独立于分片投递，旧连接的迟到关闭不能覆盖新票。
type dungeonPartyPresence struct {
	current atomic.Pointer[dungeonPartyAttachment] // 此角色最后一次可观察附着。
}

type dungeonPartyAttachment struct {
	conn           Connection // 场景基线已确认的当前连接，已断开时为空。
	binding        uint64     // 最近确认的会话代次，跨关闭保留。
	disconnectedAt time.Time  // 上一次有效连接的断开时刻。
	resumedAt      time.Time  // 上次断开后重新确认的基线时刻，尚未上线时为空。
}

func (a *dungeonPartyAttachment) online() bool {
	return a != nil && a.conn != nil && a.conn.Alive() && a.conn.Binding() == a.binding
}

// dungeonPartyActor 复用已有有界分片队列，每个运行服的房间由一个分片顺序推进。
func (w *worker) dungeonPartyActor(run uint64) *worker {
	return w.e.workers[run%uint64(len(w.e.workers))]
}

// dungeonPartyAttached 在首基线确认时恢复原队伍身份，基础信息只来自已确认角色。
func (w *worker) dungeonPartyAttached(r *room, now time.Time) {
	store := w.roomStore(r)
	if store == nil || len(store.Cfg.Fixture.Dungeons) == 0 || r.Conn == nil {
		return
	}
	if r.dungeonPartyPresence == nil {
		r.dungeonPartyPresence = &dungeonPartyPresence{}
	}
	binding := uint64(r.Auth.SessionEpoch)
	prior := r.dungeonPartyPresence.current.Load()
	if prior != nil && prior.conn == r.Conn && prior.binding == binding {
		return
	}
	state := &dungeonPartyAttachment{conn: r.Conn, binding: binding, resumedAt: now}
	if prior != nil {
		state.disconnectedAt = prior.disconnectedAt
	}
	r.dungeonPartyPresence.current.Store(state)
	identity := dungeonPartyIdentityFor(r)
	run := store.Cfg.RunID
	w.postWorldBoss(w.dungeonPartyActor(run), func(actor *worker) {
		if directory := actor.dungeonParties[run]; directory != nil {
			directory.observe(identity)
			directory.advance(now)
		}
	})
}

// dungeonPartyDetached 记录真实断开，包括被新连接接管的旧连接。
func dungeonPartyDetached(r *room, conn Connection, now time.Time) {
	watch := r.dungeonPartyPresence
	if watch == nil {
		return
	}
	if attachment := watch.current.Load(); attachment != nil && attachment.conn == conn && conn != nil {
		if now.IsZero() {
			now = time.Now()
		}
		watch.current.Store(&dungeonPartyAttachment{binding: attachment.binding, disconnectedAt: now})
	}
}

// dungeonPartyIdentityFor 冻结向所属招募分片转发的原角色身份。
func dungeonPartyIdentityFor(r *room) dungeonPartyIdentity {
	return dungeonPartyIdentity{id: r.Role.ID, name: r.Role.Name, job: r.Role.Job, level: r.Role.Level,
		binding: uint64(r.Auth.SessionEpoch), presence: r.dungeonPartyPresence}
}

// dungeonPartyScene 返回当前附着可查询的场景和是否仍处于普通场景。
func dungeonPartyScene(r *room) (uint64, bool) {
	if battle := r.dungeonMatch; battle != nil {
		if battle.shown && battle.sent.Load() && !battle.revoked.Load() {
			return battle.scene, false
		}
		return r.Scene, false // 首帧尚未送出时仍可在原普通场景查询，但不能修改招募。
	}
	if battle := r.worldBossObservation; battle != nil {
		if battle.shown && battle.started.Load() && !battle.revoked.Load() {
			return battle.scene, false
		}
		return 0, false
	}
	if run := r.personalBoss; run != nil {
		if run.shown && run.battle != nil && run.battle.Ready {
			return run.battle.Scene, false
		}
		return 0, false
	}
	return r.Scene, true
}

// dungeonPartyCommand 在原角色分片验证当前附着，随后只转发经确认的身份。
func (w *worker) dungeonPartyCommand(r *room, frame wire.Frame, now time.Time) {
	var request pb.DungeonPartyRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || len(request.ProtoReflect().GetUnknown()) != 0 || !validDungeonPartyRequest(&request) {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	scene, ordinary := dungeonPartyScene(r)
	if !r.Ready || scene == 0 || scene != request.SceneEpoch {
		w.failure(r, frame, p.CodeLineRecovering)
		return
	}
	if request.Action != dungeonPartyList && (!ordinary || mapTravelQuiescing(r)) {
		w.failure(r, frame, p.CodeLineForbidden)
		return
	}
	store := w.roomStore(r)
	fixture := &store.Cfg.Fixture
	if len(fixture.Dungeons) == 0 {
		w.failure(r, frame, p.CodeLineForbidden)
		return
	}
	if r.dungeonPartyPresence == nil {
		w.dungeonPartyAttached(r, now)
	}
	identity, conn, run := dungeonPartyIdentityFor(r), r.Conn, store.Cfg.RunID
	if !w.postWorldBoss(w.dungeonPartyActor(run), func(actor *worker) {
		if !conn.Alive() || conn.Binding() != identity.binding {
			return
		}
		if actor.dungeonParties == nil {
			actor.dungeonParties = make(map[uint64]*dungeonPartyDirectory)
		}
		directory := actor.dungeonParties[run]
		if directory == nil {
			directory = newDungeonPartyDirectory()
			actor.dungeonParties[run] = directory
		}
		if request.Action == dungeonPartyStart {
			actor.startDungeonParty(directory, store, identity, conn, &request, frame.Request, time.Now())
			return
		}
		state := directory.handle(identity, fixture, &request, time.Now())
		if conn.Alive() && conn.Binding() == identity.binding {
			conn.Deliver(wire.Message(p.MsgS2CDungeonPartyState, frame.Request, identity.binding, state))
		}
	}) {
		w.failure(r, frame, p.CodeServerBusy)
	}
}

// tickDungeonParties 推进开战确认，回收已到期招募，并报告停止进程前是否仍应等待回填。
func (w *worker) tickDungeonParties(now time.Time) (pending bool) {
	for run, directory := range w.dungeonParties {
		for _, party := range directory.parties {
			if party.start != nil {
				w.advanceDungeonStart(directory, party, now)
				pending = pending || party.start != nil
			}
		}
		directory.advance(now)
		if len(directory.parties) == 0 {
			delete(w.dungeonParties, run)
		}
	}
	return pending
}

// dungeonPartyTemplateViews 发布当前已装载的模板，不从客户端选择其它运行服。
func dungeonPartyTemplateViews(fixture *devconfig.Fixture) []*pb.DungeonPartyTemplate {
	result := make([]*pb.DungeonPartyTemplate, 0, len(fixture.Dungeons))
	for _, template := range fixture.Dungeons {
		result = append(result, &pb.DungeonPartyTemplate{TemplateId: template.TemplateID, DisplayName: template.DisplayName,
			MinimumLevel: template.MinimumLevel, DurationSeconds: uint32(template.Duration / time.Second), HireOptions: dungeonHireViews(template.Hires)})
	}
	return result
}
