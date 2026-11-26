// 本文件向原角色附着返回市场页面，并为各运行服推进已经到期的托管资产。
package game

import (
	"log/slog"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// queryItemMarket 复用有界只读槽，结果仅返回同一连接和场景。
func (w *worker) queryItemMarket(r *room, frame wire.Frame) {
	var q pb.ItemMarketQuery
	if proto.Unmarshal(frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || len(q.ProtoReflect().GetUnknown()) != 0 {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	if r.PreviewInFlight || r.PreviewGeneration == math.MaxUint64 {
		w.failure(r, frame, p.CodeOperationPending)
		return
	}
	r.PreviewGeneration++
	r.PreviewInFlight = true
	binding := skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
	store := w.roomStore(r)
	if !w.submit(job{target: w, run: func() func(*worker) {
		state, err := store.QueryItemMarket(w.e.ctx, binding.auth, &q)
		return func(owner *worker) {
			owner.finishItemMarketQuery(binding, frame, state, err)
		}
	}}) {
		r.PreviewInFlight = false
		w.failure(r, frame, p.CodeServerBusy)
	}
}

func (w *worker) finishItemMarketQuery(binding skillPreviewBinding, frame wire.Frame, state *pb.ItemMarketState, err error) {
	r := binding.room
	if r.PreviewGeneration != binding.generation || !r.PreviewInFlight {
		return
	}
	r.PreviewInFlight = false
	if w.rooms[binding.auth.RoleID] != r || r.Auth != binding.auth || r.Conn != binding.conn || r.Scene != binding.scene ||
		r.Isolated || !r.Ready || binding.conn == nil || !binding.conn.Alive() || binding.conn.Binding() != uint64(binding.auth.SessionEpoch) {
		return
	}
	if err != nil {
		w.failure(r, frame, persistence.Code(err))
		return
	}
	if state == nil || len(wire.Encode(wire.Message(p.MsgS2CItemMarketState, frame.Request, binding.conn.Binding(), state))) > wire.MaxFrame {
		w.failure(r, frame, p.CodeServerBusy)
		return
	}
	w.send(r, p.MsgS2CItemMarketState, frame.Request, state)
}

type itemMarketJob struct {
	store    *persistence.Store // 原运行服的租约、配置和写入屏障。
	next     time.Time          // 下次投递时间，由运行服分片持有。
	inFlight bool               // 同运行服最多一个到期步骤在途。
	retired  bool               // 运行服已换代时停止投递。
}

func (e *Engine) initializeItemMarketJobs() {
	for run, store := range e.runStores {
		owner := e.workers[run%uint64(len(e.workers))]
		owner.itemMarketJobs = append(owner.itemMarketJobs, itemMarketJob{store: store})
	}
}

// tickItemMarket 用原数据库队列推进少量工作，让入场和角色结算有机会执行。
func (w *worker) tickItemMarket(now time.Time) {
	if w.e.stopped.Load() {
		return
	}
	for i := range w.itemMarketJobs {
		state := &w.itemMarketJobs[i]
		if state.inFlight || state.retired || now.Before(state.next) {
			continue
		}
		state.next = now.Add(time.Second)
		state.inFlight = w.submit(job{target: w, run: func() func(*worker) {
			more, err := state.store.AdvanceItemMarket(w.e.ctx, time.Now())
			return func(*worker) {
				state.inFlight = false
				pause := 2 * time.Second
				if more {
					pause = 200 * time.Millisecond
				} else if state.store.Cfg.Fixture.ItemMarket == nil {
					pause = 15 * time.Second // 已关闭新单的旧账仍能继续清偿。
				}
				if err != nil {
					code := persistence.Code(err)
					state.retired = code == p.CodeRouteChanged
					pause = 5 * time.Second
					if !w.e.stopped.Load() {
						slog.Warn("道具市场到期步骤待确认", "run", state.store.Cfg.RunID, "code", code, "error", err)
					}
				}
				state.next = time.Now().Add(pause)
			}
		}})
	}
}
