// 本文件回归地狱火逐格推进、阻挡、所有合法格内目标与原生命和会话边界。
package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"testing"
	"time"
)

// castSteps 返回施法步骤集合供调用方使用。
// t 提供断言失败报告、子用例和清理注册，c 记录已捕获连接。
func castSteps(t *testing.T, c *capturedConnection) []*pb.CastStep {
	t.Helper()
	// out 记录输出，使用施法步骤列表表示。
	var out []*pb.CastStep
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CCastStep {
			// step 保存当前处理所用的步骤。
			step := &pb.CastStep{}
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := proto.Unmarshal(frame.Payload, step); err != nil {
				t.Fatal(err)
			}
			out = append(out, step)
		}
	}
	return out
}

// TestHellFireAllRanksRemainFourCellsAndNeverCatchUpLateFrames 核验地狱火阶应只创建单线首步，以及卡帧时追赶了旧600/700/800时刻，或应按实际处理+100创建下一格。
// t 提供断言失败报告、子用例和清理注册。
func TestHellFireAllRanksRemainFourCellsAndNeverCatchUpLateFrames(t *testing.T) {
	// rank 保存由无符号整数“32”取得的排序位次。
	for rank := uint32(0); rank <= 3; rank++ {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(string(rune('0'+rank)), func(t *testing.T) {
			// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
			w, r, c := stageBWorker(t, 1, 9)
			r.Role.Skills[0].Level = rank
			// now 记录本次处理开始时的时间。
			now := time.Now()
			// origin 保存位置，用于当前步骤中的大区或起点。
			origin := r.Player.Pos
			r.Monsters[0].Pos = stepPoint(origin, 0, 1)
			// definition 保存怪物，用于当前步骤中的定义。
			definition := r.Monsters[0].Monster
			r.Monsters = append(r.Monsters,
				&entity{Handle: 11, Kind: 3, Pos: stepPoint(origin, 7, 1), HP: 1000, MaxHP: 1000, Monster: definition},
				&entity{Handle: 12, Kind: 3, Pos: stepPoint(origin, 1, 1), HP: 1000, MaxHP: 1000, Monster: definition})
			r.MonsterCount = 3
			// code 保存由施法“directed”取得的结果码。
			if code := w.castDirected(r, 9, 0, 1, 0, now); code != 0 || len(r.Casts) != 1 || r.Casts[0].LineRemaining != 4 {
				t.Fatalf("地狱火%d阶没有只创建单线首步:%d", rank, code)
			}
			// amount 保存当前处理所用的数量。
			amount := r.Casts[0].Amount
			// late 保存由新增取得的“late”。
			late := now.Add(900 * time.Millisecond)
			w.advanceEvents(r, late)
			if len(castSteps(t, c)) != 1 || r.Monsters[0].HP != 1000-amount || len(r.Casts) != 1 || !r.Casts[0].Due.Equal(late.Add(100*time.Millisecond)) {
				t.Fatal("卡帧时追赶了旧600/700/800时刻，或没有按实际处理+100创建下一格")
			}
			// step 规定步骤的初始取值。
			for step := 2; step <= 4; step++ {
				// 同一个对象移动到下一格后可再次受击，不能按cast级目标集合去重。
				r.Monsters[0].Pos = stepPoint(origin, 0, step)
				// due 保存当前处理所用的已到期。
				due := r.Casts[0].Due
				w.advanceEvents(r, due.Add(-time.Nanosecond))
				if len(castSteps(t, c)) != step-1 {
					t.Fatal("传播格提前执行")
				}
				w.advanceEvents(r, due)
				if r.Monsters[0].HP != 1000-int64(step)*amount || len(castSteps(t, c)) != step {
					t.Fatal("后格重新抽威力、漏重复占位命中或重复步事件")
				}
			}
			if len(r.Casts) != 0 || r.Monsters[1].HP != 1000 || r.Monsters[2].HP != 1000 {
				t.Fatal("固定四格结束后仍继续传播或3阶偷偷增加两翼射线")
			}
			// i 定位施法步骤集合中正在处理的成员，step 接收遍历施法步骤集合时当前的元素。
			for i, step := range castSteps(t, c) {
				if step.StepIndex != uint32(i+1) || step.SkillLevel != rank || step.CasterLife != r.Player.Life || step.CastId != r.CastID || step.Grid.X != int32(origin.X) || step.Grid.Y != int32(origin.Y-i-1) {
					t.Fatal("CastStep缺实际格/阶数/施法与生命代次")
				}
			}
		})
	}
}

// TestHellFireEmptyCellsEmitActualStepAndBlockedCellStops 核验有效空格应单次Step或空格提前终止链，以及无效格发出Step或应终止后格传播。
// t 提供断言失败报告、子用例和清理注册。
func TestHellFireEmptyCellsEmitActualStepAndBlockedCellStops(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 1, 9)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	// origin 保存位置，用于当前步骤中的大区或起点。
	origin := r.Player.Pos
	r.Monsters[0].Pos = stepPoint(origin, 0, 3)
	w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{stepPoint(origin, 0, 2)}
	if w.castDirected(r, 9, 0, 1, 0, now) != 0 {
		t.Fatal("受理地狱火失败")
	}
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	// steps 保存由施法步骤集合取得的步骤集合。
	steps := castSteps(t, c)
	if len(steps) != 1 || steps[0].Grid.Y != int32(origin.Y-1) || len(r.Casts) != 1 {
		t.Fatal("有效空格没有单次Step或空格提前终止链")
	}
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CCombatImpact {
			// impact 记录命中影响，使用战斗命中影响表示。
			var impact pb.CombatImpact
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := proto.Unmarshal(frame.Payload, &impact); err != nil {
				t.Fatal(err)
			}
			if impact.TargetHandle == 0 && (impact.TargetGrid.X != int32(origin.X) || impact.TargetGrid.Y != int32(origin.Y-1)) {
				t.Fatal("空格未命中的观察坐标指向上一格")
			}
		}
	}
	w.advanceEvents(r, now.Add(600*time.Millisecond))
	if len(r.Casts) != 0 || len(castSteps(t, c)) != 1 || r.Monsters[0].HP != 1000 {
		t.Fatal("无效格发出Step或没有终止后格传播")
	}
}

// TestHellFireCellHitsAllLegalOccupantsAndReadyOnlySuppressesOldSteps 核验恢复观察应只收到当前真实新步，用诊断「switch内break被误作for退出，或未就绪观察漏出旧步」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestHellFireCellHitsAllLegalOccupantsAndReadyOnlySuppressesOldSteps(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 1, 9)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	// origin 保存位置，用于当前步骤中的大区或起点。
	origin := r.Player.Pos
	// point 保存由步骤点位取得的点位。
	point := stepPoint(origin, 0, 1)
	r.Monsters[0].Pos = point
	r.Monsters = append(r.Monsters, &entity{Handle: 11, Kind: 3, Pos: point, HP: 1000, MaxHP: 1000, Monster: r.Monsters[0].Monster})
	r.MonsterCount = 2
	if w.castDirected(r, 9, 0, 1, 0, now) != 0 {
		t.Fatal("受理地狱火失败")
	}
	r.Ready = false
	c.frames = nil
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Monsters[0].HP >= 1000 || r.Monsters[1].HP != r.Monsters[0].HP || len(castSteps(t, c)) != 0 {
		t.Fatal("switch内break被误作for退出，或未就绪观察漏出旧步")
	}
	r.Ready = true
	w.snapshot(r)
	if len(castSteps(t, c)) != 0 {
		t.Fatal("基线重播已经发生的旧步")
	}
	w.advanceEvents(r, now.Add(600*time.Millisecond))
	// steps 保存由施法步骤集合取得的步骤集合。
	if steps := castSteps(t, c); len(steps) != 1 || steps[0].StepIndex != 2 {
		t.Fatal("恢复观察没有只收到当前真实新步")
	}
}

// TestHellFireChildUsesNewHostingPrivilegeWhileOverdueParentKeepsOldFact 核验未知提交恢复后追赶旧格，应按实际处理时钟+100，用诊断「未来新格克隆首格旧权益/托管，而未取当前room上下文」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestHellFireChildUsesNewHostingPrivilegeWhileOverdueParentKeepsOldFact(t *testing.T) {
	// initiallyOffline 接收遍历布尔状态列表时当前的元素。
	for _, initiallyOffline := range []bool{false, true} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(map[bool]string{false: "online_to_offline", true: "offline_to_online"}[initiallyOffline], func(t *testing.T) {
			// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
			w, r, c := stageBWorker(t, 1, 9)
			// now 记录本次处理开始时的时间。
			now := time.Now()
			r.Role.PrivilegeUntil = now.Add(550 * time.Millisecond)
			if initiallyOffline {
				r.Conn, r.HostSession = nil, "old_offline"
				r.Role.PrivilegeUntil = time.Time{}
			}
			r.Monsters[0].Pos, r.Monsters[0].HP = stepPoint(r.Player.Pos, 0, 1), 1
			r.Monsters = append(r.Monsters, &entity{Handle: 11, Kind: 3, Pos: stepPoint(r.Player.Pos, 0, 2), HP: 1, MaxHP: 1, Monster: r.Monsters[0].Monster})
			r.MonsterCount = 2
			if w.castDirected(r, 9, 0, 1, 0, now) != 0 {
				t.Fatal("受理地狱火失败")
			}
			// at 保存由新增取得的发生时间。
			at := now.Add(600 * time.Millisecond)
			if initiallyOffline {
				r.Conn, r.HostSession = c, ""
				r.Role.PrivilegeUntil = now.Add(2 * time.Second)
			} else {
				r.Conn, r.HostSession = nil, "new_offline"
				r.Role.PrivilegeUntil = time.Time{}
			}
			transitionEventHosting(r, at, r.HostSession)
			transitionEventPrivilege(r, at, r.Role.PrivilegeUntil)
			w.advanceEvents(r, now.Add(800*time.Millisecond))
			if len(r.KillFacts) != 1 || !r.KillFacts[0].At.Equal(now.Add(500*time.Millisecond)) || r.KillFacts[0].Bound != initiallyOffline {
				t.Fatal("迟处理首格被转换时刻追溯改绑")
			}
			if initiallyOffline && r.KillFacts[0].HostSession != "old_offline" || !initiallyOffline && r.KillFacts[0].HostSession != "" {
				t.Fatal("迟处理首格借重连切换托管区段")
			}
			if len(r.Casts) != 1 || !r.Casts[0].Due.Equal(now.Add(900*time.Millisecond)) || r.Casts[0].HostSession != r.HostSession || !r.Casts[0].PrivilegeUntil.Equal(r.Role.PrivilegeUntil) {
				t.Fatal("未来新格克隆首格旧权益/托管，而未取当前room上下文")
			}
			r.Pending = true // 未知永久提交期间不能推进下一格，也不再次抽首格掉落。
			// factID 标识当前业务使用的“fact”。
			factID := r.KillFacts[0].ID
			w.step(r, now.Add(2*time.Second))
			if len(r.KillFacts) != 1 || r.KillFacts[0].ID != factID || r.Monsters[1].HP != 1 || len(r.Casts) != 1 {
				t.Fatal("永久提交未知时推进/重抽了链式攻击")
			}
			r.Pending = false
			w.advanceEvents(r, now.Add(2*time.Second))
			if len(r.KillFacts) != 2 || !r.KillFacts[1].At.Equal(now.Add(900*time.Millisecond)) || r.KillFacts[1].Bound == initiallyOffline || r.KillFacts[1].HostSession != r.HostSession {
				t.Fatal("新格真实Due/绑定/托管区段错误")
			}
			if len(r.Casts) != 1 || !r.Casts[0].Due.Equal(now.Add(2100*time.Millisecond)) {
				t.Fatal("未知提交恢复后追赶旧格，没有按实际处理时钟+100")
			}
		})
	}
}

// TestHellFireSurvivesDeathButOldChildCannotSurviveReviveLife 核对「同life死亡取消了已支付释放的首格」与「旧CasterLife链在复活新life继续造成伤害/发Step」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestHellFireSurvivesDeathButOldChildCannotSurviveReviveLife(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 1, 9)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	// origin 保存位置，用于当前步骤中的大区或起点。
	origin := r.Player.Pos
	r.Monsters[0].Pos = stepPoint(origin, 0, 1)
	if w.castDirected(r, 9, 0, 1, 0, now) != 0 {
		t.Fatal("受理地狱火失败")
	}
	r.Player.HP = 0
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Monsters[0].HP >= 1000 || len(castSteps(t, c)) != 1 || len(r.Casts) != 1 {
		t.Fatal("同life死亡取消了已支付释放的首格")
	}
	// before 保存生命值，用于当前步骤中的操作前的状态。
	before := r.Monsters[0].HP
	r.Monsters[0].Pos = stepPoint(origin, 0, 2)
	w.revivePlayer(r)
	w.advanceEvents(r, now.Add(600*time.Millisecond))
	if r.Monsters[0].HP != before || len(castSteps(t, c)) != 1 || len(r.Casts) != 0 {
		t.Fatal("旧CasterLife链在复活新life继续造成伤害/发Step")
	}
}
