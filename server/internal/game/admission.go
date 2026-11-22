// 房间会话接管只接受已确认的持久结果。未知结果保持原尝试身份，直到可以继续旧会话或附着新会话。
package game

import (
	"context"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// join 为一次入场分配内部尝试身份，并占用房间的永久操作槽。
func (w *worker) join(r *room, c command) {
	r.PreviewGeneration++
	r.PreviewInFlight = false
	r.entryAttempt = domain.ID()
	r.Pending = true
	if !w.submit(w.entryJob(r, c, r.entryAttempt, false)) {
		r.Pending = false
		r.entryAttempt = ""
		rejectEntry(c.conn, c.frame.Request, p.CodeServerBusy, "admission transaction queue full")
		if r.Scene == 0 {
			delete(w.rooms, c.role)
		}
	}
}

// entryJob 保留原票据和尝试身份。断连后仍查证在途结果，且不再发起新接管。
func (w *worker) entryJob(r *room, c command, attempt string, reconciling bool) job {
	return job{target: w, admission: true, run: func() func(*worker) {
		var role domain.Role
		var auth persistence.Auth
		err := context.Canceled
		if alive := c.conn.Alive(); alive || reconciling {
			role, auth, err = w.e.Store.EnterTicket(w.e.ctx, *c.ticket, attempt, alive)
		}
		return func(w *worker) {
			w.finishEntry(r, c, attempt, reconciling, role, auth, err)
		}
	}}
}

func (w *worker) finishEntry(r *room, c command, attempt string, reconciling bool, role domain.Role, auth persistence.Auth, err error) {
	if w.rooms[c.role] != r || !r.Pending || r.entryAttempt != attempt || r.Isolated {
		return
	}
	code := persistence.Code(err)
	if code == p.CodeOperationPending || reconciling && code == p.CodeServerBusy {
		retry := w.entryJob(r, c, attempt, true)
		r.PendingRetry = &retry
		r.RetryAt = time.Now().Add(250 * time.Millisecond)
		return
	}
	now := time.Now()
	r.resumeAfterPending(now)
	r.PendingRetry = nil
	r.entryAttempt = ""
	if err == nil {
		w.attachEntry(r, c, role, auth, now)
		return
	}
	if c.conn.Alive() {
		rejectEntry(c.conn, c.frame.Request, code, "entry rejected")
	}
	// 只有持久围栏内确认旧绑定仍有效，才能让它继续推进。已失效的现场由下一张有效票重建。
	if r.Scene == 0 || reconciling && (auth != r.Auth || role.Owner != r.Role.Owner || role.RunEpoch != r.Role.RunEpoch) {
		if r.Conn != nil {
			r.Conn.Close("entry authority changed")
		}
		w.cancelMapTravelOnDetach(r)
		w.cancelTownNavigation(r)
		delete(w.rooms, c.role)
		for _, next := range r.Waiting {
			w.command(next)
		}
		return
	}
	w.after(r)
}

// attachEntry 安装确认后的新会话，健康重连继续使用原世界和战斗状态。
func (w *worker) attachEntry(r *room, c command, role domain.Role, auth persistence.Auth, now time.Time) {
	if r.Scene == 0 {
		w.initialize(r, role)
	}
	if r.Conn != nil && r.Conn != c.conn {
		r.Conn.Close("role taken over")
	}
	w.cancelMapTravelOnDetach(r)
	w.cancelTownNavigation(r)
	r.Role, r.Auth = role, auth
	r.idleSince = time.Time{}
	if w.e.Store.Cfg.Fixture.RuleVersion >= 20 {
		w.refreshPlayerStats(r)
	}
	transitionEventPrivilege(r, now, role.PrivilegeUntil)
	r.InputSequence = 0
	r.Manual = false
	r.Conn = c.conn
	r.Ready = false
	r.PetBornReady = nil
	r.Baseline = w.e.baseline.Add(1)
	r.HostSession = ""
	r.LastTick = now
	r.AwaitUntil = now.Add(10 * time.Second)
	m, known := w.currentMap(r)
	town := known && m.SceneKindValue() == devconfig.SceneKindTown
	if travel := w.e.Store.Cfg.Fixture.TownTravel; town && travel != nil {
		r.AwaitUntil = now.Add(time.Duration(travel.AckTTLMS) * time.Millisecond)
	}
	if c.conn.Alive() {
		c.conn.SetBinding(uint64(auth.SessionEpoch))
	}
	if !c.conn.Alive() {
		r.Conn = nil
		if !town {
			r.HostSession = domain.ID()
		}
		transitionEventHosting(r, now, r.HostSession)
		w.after(r)
		return
	}
	// 新会话切到在线区间；已经到期的事件仍保留原托管归属。
	transitionEventHosting(r, now, "")
	if r.Player.Handle == 0 {
		w.isolateRoom(r)
		return
	}
	entry := w.enterLineResult(r)
	if entry == nil {
		return
	}
	w.send(r, p.MsgS2CEnterLine, c.frame.Request, entry)
	w.bindingState(r, 1)
	w.baseline(r)
	w.after(r)
}
