// 本文件回归瞬移生效时若有先到期的击杀仍待结算，完整保留原事件直到可执行。
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
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21PriorTickKillPendingRetainsDueAndExecutesOnlyAfterRelease(t *testing.T) {
	// unknown 接收遍历布尔状态列表时当前的元素。
	for _, unknown := range []bool{false, true} {
		// name 保存名称所使用的文本。
		name := "inflight_kill"
		if unknown {
			name = "operation_pending_callback"
		}
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用。
			// teleport 保存由瞬移“21”房间执行器取得的瞬移。
			w, r, c, teleport, _ := teleport21Worker(t)
			// now 记录本次处理开始时的时间。
			now := time.Now()
			// personal 保存个人场景世界，用于当前步骤中的个人。
			personal := r.PersonalWorld
			// first 保存从怪物集合取出的第一项。
			first := personal.Monsters[0]
			// second 保存当前处理所用的秒。
			second := *first
			first.Pos, first.HP = devconfig.Point{X: 8, Y: 7}, 1
			second.Handle, second.Pos, second.HP = first.Handle+1, devconfig.Point{X: 8, Y: 6}, 1
			personal.Monsters = []*entity{first, &second}
			personal.MonsterCount = 2
			if !w.worldWalkable(personal, first.Pos) || !w.worldWalkable(personal, second.Pos) {
				t.Fatal("必须使用真实个人图合法前方格，不改地图或技能时长")
			}
			// draws 规定“draws”的初始取值。
			draws := 0
			// n 记录数值。
			w.roll = func(n int64) int64 { draws++; return n / 2 }
			// 真实500ms首格和100ms后继；首kill事务拖过1800ms全局CD后，下一child仍在队列。
			// spellAt 保存由新增取得的法术发生时间。
			spellAt := now.Add(-2 * time.Second)
			// code 保存由施法“directed”取得的结果码。
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
			// firstJob 接收任务或职业编号集合通道送来的第一项任务或职业编号。
			firstJob := <-w.e.jobs
			// firstFact 保存从击杀“facts”取出的第一项已发生的事实。
			firstFact := r.KillFacts[0]
			// firstQ 保存由击杀结算取得的第一项q。
			firstQ := w.killSettlement(r, firstFact)
			// firstBinding 初始化用作第一项会话绑定的结算会话绑定。
			firstBinding := settlementBinding{room: r, auth: r.Auth, world: personal.Ref, point: firstFact.DropPoint,
				generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
			// committed 保存角色，用于当前步骤中的已提交。
			committed := r.Role
			committed.Revision++
			committed.SourceSeq = firstQ.SourceSeq
			w.finishSettlement(firstBinding, firstQ, committed, nil, &firstJob, func() { r.KillFacts = r.KillFacts[1:] })
			if r.Pending || len(personal.Casts) != 1 {
				t.Fatal("原回调应只解永久slot，不补跑后续地图格")
			}

			// mp 保存当前处理所用的魔法值，origin 保存位置，用于当前步骤中的大区或起点，scene 保存当前处理所用的场景，life 保存当前处理所用的生命代次。
			// economic 保存经济操作序号，用于当前步骤中的经济操作。
			mp, origin, scene, life, economic := r.Player.MP, r.Player.Pos, r.Scene, r.Player.Life, r.Role.EconomicSeq
			// code 保存由施法取得的结果码。
			if code := w.cast(r, 21, 0, 2, now); code != 0 {
				t.Fatal("CD已自然释放、无Pending时应受理21", code)
			}
			// castID 标识当前业务使用的施法。
			castID := r.CastID
			// due 保存由新增取得的已到期。
			due := now.Add(200 * time.Millisecond)
			// queued 封装已排队对应的后续处理。
			queued := func() (pendingCast, bool) {
				// cast 接收遍历施法集合时当前的元素。
				for _, cast := range personal.Casts {
					if cast.Skill.ID == 21 && cast.ID == castID {
						return cast, true
					}
				}
				return pendingCast{}, false
			}
			// accepted 保存由已排队取得的已受理，ok 表明查询、校验或类型转换是否成功。
			accepted, ok := queued()
			if !ok || !accepted.Due.Equal(due) || r.Player.MP != mp-teleport.MPBaseCost {
				t.Fatal("受理必须只扣一次并保留原200ms Due")
			}
			// 下一tick先消费原地狱火child并击杀第二个原图怪；21尚未到期。
			w.step(r, now.Add(50*time.Millisecond))
			if second.HP != 0 || !r.Pending || len(r.KillFacts) != 1 || !r.KillFacts[0].Prepared || len(w.e.jobs) != 1 {
				t.Fatal("前一tick应由真实child击杀取得永久Pending，不是手填Pending")
			}
			// killJob 接收任务或职业编号集合通道送来的击杀任务或职业编号。
			killJob := <-w.e.jobs
			// fact 保存从击杀“facts”取出的已发生的事实。
			fact := r.KillFacts[0]
			// q 接收击杀结算供后续步骤使用。
			q := w.killSettlement(r, fact)
			// binding 初始化用作会话绑定的结算会话绑定。
			binding := settlementBinding{room: r, auth: r.Auth, world: personal.Ref, point: fact.DropPoint,
				generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
			if unknown {
				// rejectedRole 组装拒绝角色所需的数据。
				rejectedRole := domain.Role{Revision: r.Role.Revision + 100}
				w.finishSettlement(binding, q, rejectedRole, persistence.Fail(p.CodeOperationPending, "受控未定回调"), &killJob)
				if !r.Pending || r.PendingRetry != &killJob || r.Role.Revision == rejectedRole.Revision {
					t.Fatal("未定回调不得接纳out Role或更换原job")
				}
			}
			// retry 保存待按原身份重试的操作，用于当前步骤中的重试，operation 保存待确认操作标识，用于当前步骤中的操作。
			// fingerprint 保存待确认“fingerprint”，用于当前步骤中的“fingerprint”，generation 保存结算代次，用于当前步骤中的代次。
			// frozenDraws 保存当前处理所用的已冻结状态“draws”。
			retry, operation, fingerprint, generation, frozenDraws := r.PendingRetry, r.PendingOperationID, r.PendingFingerprint, r.SettlementGeneration, draws
			// offset 接收遍历时长列表时当前的元素。
			for _, offset := range []time.Duration{200 * time.Millisecond, 400 * time.Millisecond} {
				w.step(r, now.Add(offset))
				// cast 保存由已排队取得的施法，exists 保存由已排队取得的存在。
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
			// role 保存当前处理所用的角色。
			role := r.Role
			role.Revision++
			role.SourceSeq = q.SourceSeq
			w.finishSettlement(binding, q, role, nil, &killJob, func() { r.KillFacts = r.KillFacts[1:] })
			if r.Pending || r.PendingRetry != nil || r.PendingOperationID != "" || len(r.KillFacts) != 0 {
				t.Fatal("同原job成功回调未明确解锁")
			}
			// cast 保存由已排队取得的施法，exists 保存由已排队取得的存在。
			cast, exists := queued()
			if !exists || !cast.Due.Equal(due) || len(teleport21Results(t, c)) != 0 {
				t.Fatal("解锁回调不应篡改Due或隐式重受理21")
			}
			// permitted 保存由新增取得的“permitted”。
			permitted := now.Add(650 * time.Millisecond)
			w.step(r, permitted)
			// result 保存由瞬移“21”上一次状态取得的结果。
			result := teleport21Last(t, c)
			if result.Outcome != teleportSucceeded || r.Scene != scene+1 || r.MapID != 2 || r.Player.Pos != (devconfig.Point{X: 650, Y: 631}) ||
				r.Player.Life != life || r.Player.MP != mp-teleport.MPBaseCost || r.Role.EconomicSeq != economic || draws != frozenDraws+2 {
				t.Fatal("解除后首次可执行tick未按原施法唯一移交，或重扣/重抽", result)
			}
			// exists 保存由已排队取得的存在。
			if _, exists := queued(); exists {
				t.Fatal("已执行21仍留在原World队列")
			}
			// status 保存从状态集合取出的状态。
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
