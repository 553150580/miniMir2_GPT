package game

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"testing"
	"time"
)

// 已受理21不越过原永久Pending围栏。这里控制真实kill job的回调，不运行Mongo或伪称网络unknown注入。
func TestTeleport21PriorTickKillPendingRetainsDueAndExecutesOnlyAfterRelease(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		name := "inflight_kill"
		if unknown {
			name = "operation_pending_callback"
		}
		t.Run(name, func(t *testing.T) {
			w, r, c, teleport, _ := teleport21Worker(t)
			now := time.Now()
			personal := r.PersonalWorld
			first := personal.Monsters[0]
			second := *first
			first.Pos, first.HP = devconfig.Point{X: 8, Y: 7}, 1
			second.Handle, second.Pos, second.HP = first.Handle+1, devconfig.Point{X: 8, Y: 6}, 1
			personal.Monsters = []*entity{first, &second}
			personal.MonsterCount = 2
			if !w.worldWalkable(personal, first.Pos) || !w.worldWalkable(personal, second.Pos) {
				t.Fatal("必须使用真实个人图合法前方格，不改地图或技能时长")
			}
			draws := 0
			w.roll = func(n int64) int64 { draws++; return n / 2 }
			// 真实500ms首格和100ms后继；首kill事务拖过1800ms全局CD后，下一child仍在队列。
			spellAt := now.Add(-2 * time.Second)
			if code := w.castDirected(r, 9, 0, 1, 0, spellAt); code != 0 {
				t.Fatal("未受理真实地狱火", code)
			}
			w.advanceEvents(r, spellAt.Add(500*time.Millisecond))
			if first.HP != 0 || len(r.KillFacts) != 1 || len(personal.Casts) != 1 {
				t.Fatal("来源首格没有真实击杀并派生后继")
			}
			w.flushKillFacts(r)
			if !r.Pending || len(w.e.jobs) != 1 {
				t.Fatal("首kill未取得真实永久slot/job")
			}
			firstJob := <-w.e.jobs
			firstFact := r.KillFacts[0]
			firstQ := w.killSettlement(r, firstFact)
			firstBinding := settlementBinding{room: r, auth: r.Auth, world: personal.Ref, point: firstFact.DropPoint,
				generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
			committed := r.Role
			committed.Revision++
			committed.SourceSeq = firstQ.SourceSeq
			w.finishSettlement(firstBinding, firstQ, committed, nil, &firstJob, func() { r.KillFacts = r.KillFacts[1:] })
			if r.Pending || len(personal.Casts) != 1 {
				t.Fatal("原回调应只解永久slot，不补跑后续地图格")
			}

			mp, origin, scene, life, economic := r.Player.MP, r.Player.Pos, r.Scene, r.Player.Life, r.Role.EconomicSeq
			if code := w.cast(r, 21, 0, 2, now); code != 0 {
				t.Fatal("CD已自然释放、无Pending时应受理21", code)
			}
			castID := r.CastID
			due := now.Add(200 * time.Millisecond)
			queued := func() (pendingCast, bool) {
				for _, cast := range personal.Casts {
					if cast.Skill.ID == 21 && cast.ID == castID {
						return cast, true
					}
				}
				return pendingCast{}, false
			}
			accepted, ok := queued()
			if !ok || !accepted.Due.Equal(due) || r.Player.MP != mp-teleport.MPBaseCost {
				t.Fatal("受理必须只扣一次并保留原200ms Due")
			}
			// 下一tick先消费原地狱火child并击杀第二个原图怪；21尚未到期。
			w.step(r, now.Add(50*time.Millisecond))
			if second.HP != 0 || !r.Pending || len(r.KillFacts) != 1 || !r.KillFacts[0].Prepared || len(w.e.jobs) != 1 {
				t.Fatal("前一tick应由真实child击杀取得永久Pending，不是手填Pending")
			}
			killJob := <-w.e.jobs
			fact := r.KillFacts[0]
			q := w.killSettlement(r, fact)
			binding := settlementBinding{room: r, auth: r.Auth, world: personal.Ref, point: fact.DropPoint,
				generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
			if unknown {
				rejectedRole := domain.Role{Revision: r.Role.Revision + 100}
				w.finishSettlement(binding, q, rejectedRole, persistence.Fail(p.CodeOperationPending, "受控未定回调"), &killJob)
				if !r.Pending || r.PendingRetry != &killJob || r.Role.Revision == rejectedRole.Revision {
					t.Fatal("未定回调不得接纳out Role或更换原job")
				}
			}
			retry, operation, fingerprint, generation, frozenDraws := r.PendingRetry, r.PendingOperationID, r.PendingFingerprint, r.SettlementGeneration, draws
			for _, offset := range []time.Duration{200 * time.Millisecond, 400 * time.Millisecond} {
				w.step(r, now.Add(offset))
				cast, exists := queued()
				if !r.Pending || !exists || !cast.Due.Equal(due) || cast.ID != accepted.ID || cast.Order != accepted.Order ||
					r.PendingRetry != retry || r.PendingOperationID != operation || r.PendingFingerprint != fingerprint || r.SettlementGeneration != generation ||
					r.currentWorld() != personal || r.Player.Pos != origin || r.Scene != scene || r.Player.Life != life || r.Player.MP != mp-teleport.MPBaseCost || draws != frozenDraws || len(teleport21Results(t, c)) != 0 {
					t.Fatal("到期越过原Pending，或改原slot/Due/付费、提前随机选点")
				}
			}
			// 即便直接进入事件入口，统一Pending也应继续冻结原已付费队列。
			w.advanceEvents(r, now.Add(600*time.Millisecond))
			if draws != frozenDraws || len(teleport21Results(t, c)) != 0 {
				t.Fatal("事件入口绕过统一Pending围栏")
			}
			role := r.Role
			role.Revision++
			role.SourceSeq = q.SourceSeq
			w.finishSettlement(binding, q, role, nil, &killJob, func() { r.KillFacts = r.KillFacts[1:] })
			if r.Pending || r.PendingRetry != nil || r.PendingOperationID != "" || len(r.KillFacts) != 0 {
				t.Fatal("同原job成功回调未明确解锁")
			}
			cast, exists := queued()
			if !exists || !cast.Due.Equal(due) || len(teleport21Results(t, c)) != 0 {
				t.Fatal("解锁回调不应篡改Due或隐式重受理21")
			}
			permitted := now.Add(650 * time.Millisecond)
			w.step(r, permitted)
			result := teleport21Last(t, c)
			if result.Outcome != teleportSucceeded || r.Scene != scene+1 || r.MapID != 2 || r.Player.Pos != (devconfig.Point{X: 650, Y: 631}) ||
				r.Player.Life != life || r.Player.MP != mp-teleport.MPBaseCost || r.Role.EconomicSeq != economic || draws != frozenDraws+2 {
				t.Fatal("解除后首次可执行tick未按原施法唯一移交，或重扣/重抽", result)
			}
			if _, exists := queued(); exists {
				t.Fatal("已执行21仍留在原World队列")
			}
			status := r.Player.Statuses[teleport.BuffID]
			if status == nil || status.RemainingMS != 30000 || !status.LastProcess.Equal(permitted) {
				t.Fatal("成功状态应从实际移交时钟开始，不能回填名义Due")
			}
			w.step(r, permitted.Add(50*time.Millisecond))
			if len(teleport21Results(t, c)) != 1 || r.Player.MP != mp-teleport.MPBaseCost {
				t.Fatal("已消费21重复执行或扣费")
			}
		})
	}
}
