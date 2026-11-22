// 本文件回归行走、受推、导航目标、修订号和原世界已释放动作之间的边界。
package game

import (
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"reflect"
	"testing"
	"time"
)

// pushedWorker 处理被推动位移房间执行器。
// t 提供断言失败报告、子用例和清理注册。
func pushedWorker(t *testing.T) (*worker, *room, *capturedConnection) {
	return pushedJobWorker(t, 2)
}

// pushedJobWorker 处理被推动位移任务或职业编号房间执行器。
// t 提供断言失败报告、子用例和清理注册，job 记录任务或职业编号，使用数值表示。
func pushedJobWorker(t *testing.T, job uint32) (*worker, *room, *capturedConnection) {
	t.Helper()
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用，c 接收阶段a房间执行器供后续步骤使用。
	w, r, c := stageAWorker(t, job)
	// 测试夹具采用已核来源合同，绝不提供运行配置fallback。
	// timing 保存当前处理所用的时序。
	timing := &w.e.Store.Cfg.Fixture.CombatTiming
	timing.PlayerPushActionMS, timing.MonsterPushActionMSPerStep, timing.MonsterPushMoveMSPerStep, timing.PushedStepDisplayMS = 500, 300, 500, 300
	w.e.Store.Cfg.Fixture.Pet.CanPush = true
	// definition 保存当前处理所用的定义。
	definition := *r.Monsters[0].Monster
	definition.CanPush = true
	r.Monsters[0].Monster = &definition
	r.Auto = false
	return w, r, c
}

// TestPushedKeepsAlreadyReleasedProjectileToleranceAndAttackActorRef 核对「推移篡改了原2格容差/目标跟踪规则」与「已受理普通攻击被同图推远坐标错误取消」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPushedKeepsAlreadyReleasedProjectileToleranceAndAttackActorRef(t *testing.T) {
	// steps 接收遍历数值列表时当前的元素。
	for _, steps := range []int{2, 3} {
		// w 接收被推动位移任务或职业编号房间执行器供后续步骤使用，r 接收被推动位移任务或职业编号房间执行器供后续步骤使用。
		w, r, _ := pushedJobWorker(t, 1)
		// now 保存由Unix 时间取得的当前时间。
		now := time.Unix(1800000000, 0)
		// m 保存从怪物集合取出的怪物集合中的指定项。
		m := r.Monsters[0]
		// hp 保存当前处理所用的生命值。
		hp := m.HP
		// code 保存由施法取得的结果码。
		if code := w.cast(r, 1, m.Handle, 1, now); code != 0 {
			t.Fatal(code)
		}
		// due 保存当前处理所用的已到期。
		due := r.Casts[0].Due
		// out 保存由来源被推动位移取得的输出。
		if out := w.sourcePushed(r, reference(r, m), 4, steps, now, displacementSource{}); out.Steps != steps {
			t.Fatal("投射目标推移前态失败")
		}
		w.advanceEvents(r, due)
		if (m.HP < hp) != (steps == 2) {
			t.Fatal("推移篡改了原2格容差/目标跟踪规则")
		}
	}
	// w 接收被推动位移任务或职业编号房间执行器供后续步骤使用，r 接收被推动位移任务或职业编号房间执行器供后续步骤使用。
	w, r, _ := pushedJobWorker(t, 1)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// m 保存从怪物集合取出的怪物集合中的指定项。
	m := r.Monsters[0]
	// hp 保存当前处理所用的生命值。
	hp := m.HP
	w.attack(r, &r.Player, m, now)
	if len(r.Attacks) != 1 {
		t.Fatal("普攻前态失败")
	}
	// due 保存当前处理所用的已到期。
	due := r.Attacks[0].Due
	w.sourcePushed(r, reference(r, m), 4, 3, now, displacementSource{})
	w.advanceEvents(r, due)
	if m.HP >= hp {
		t.Fatal("已受理普通攻击被同图推远坐标错误取消")
	}
	w, r, _ = pushedJobWorker(t, 1)
	m = r.Monsters[0]
	hp = m.HP
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 1, m.Handle, 1, now); code != 0 {
		t.Fatal(code)
	}
	due = r.Casts[0].Due
	// old 保存由引用取得的原状态。
	old := reference(r, m)
	m.Life++
	// out 保存由来源被推动位移取得的输出。
	if out := w.sourcePushed(r, old, 4, 1, now, displacementSource{}); out.Stop != pushStale {
		t.Fatal("原life推挤作用复生对象")
	}
	w.advanceEvents(r, due)
	if m.HP != hp {
		t.Fatal("原life法术作用复生对象")
	}
}

// displacementEvents 返回位移事件集合供调用方使用。
// t 提供断言失败报告、子用例和清理注册，c 记录已捕获连接。
func displacementEvents(t *testing.T, c *capturedConnection) []*pb.EntityDisplacement {
	t.Helper()
	// events 记录事件集合，使用实体位移列表表示。
	var events []*pb.EntityDisplacement
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CEntityDisplacement {
			// event 保存当前处理所用的事件。
			event := &pb.EntityDisplacement{}
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := proto.Unmarshal(frame.Payload, event); err != nil {
				t.Fatal(err)
			}
			events = append(events, event)
		}
	}
	return events
}

// lastMovementBaseline 返回上一次状态移动场景基线供调用方使用。
// t 提供断言失败报告、子用例和清理注册，c 记录已捕获连接。
func lastMovementBaseline(t *testing.T, c *capturedConnection) *pb.SceneBaselineChunk {
	t.Helper()
	// i 保存根据“len”与当前数据计算的当前位置的下标。
	for i := len(c.frames) - 1; i >= 0; i-- {
		if c.frames[i].ID == p.MsgS2CSceneBaseline {
			// scene 保存当前处理所用的场景。
			scene := &pb.SceneBaselineChunk{}
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := proto.Unmarshal(c.frames[i].Payload, scene); err != nil {
				t.Fatal(err)
			}
			return scene
		}
	}
	t.Fatal("没有观察基线")
	return nil
}

// movementEntity 返回移动实体供调用方使用。
// scene 记录场景，使用场景场景基线分片表示，handle 记录实体句柄，使用数值表示。
func movementEntity(scene *pb.SceneBaselineChunk, handle uint32) *pb.EntityState {
	// e 接收遍历实体时当前的元素。
	for _, e := range scene.Entities {
		if e.EntityHandle == handle {
			return e
		}
	}
	return nil
}

// TestPushedEightDirectionsPreserveRealPrefixAndReverseFacing 核验最终基线与事件水位/末点应一致，用诊断「真实路径未一次有界发布到最终基线之前」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPushedEightDirectionsPreserveRealPrefixAndReverseFacing(t *testing.T) {
	// direction 接收遍历朝向集合时当前的元素。
	for direction := range directions {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(string(rune('0'+direction)), func(t *testing.T) {
			// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
			w, r, c := pushedWorker(t)
			// e 保存从怪物集合取出的怪物集合中的指定项。
			e := r.Monsters[0]
			e.Pos = devconfig.Point{X: 8, Y: 8}
			r.Player.Pos = devconfig.Point{X: 0, Y: 0}
			r.Pet.Pos = devconfig.Point{X: 16, Y: 16}
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			// source 初始化用作来源的位移来源。
			source := displacementSource{CastID: 17}
			// origin 保存位置，用于当前步骤中的大区或起点。
			origin := e.Pos
			e.AttackSequence, e.CastSequence = 11, 12
			// out 保存由来源被推动位移取得的输出。
			out := w.sourcePushed(r, reference(r, e), uint32(direction), 3, now, source)
			// events 保存由位移事件集合取得的事件集合。
			events := displacementEvents(t, c)
			if out.Steps != 3 || out.Stop != pushCompleted || !out.Postamble || len(events) != 1 || len(c.frames) != 2 || c.frames[0].ID != p.MsgS2CEntityDisplacement || c.frames[1].ID != p.MsgS2CSceneBaseline {
				t.Fatal("真实路径未一次有界发布到最终基线之前")
			}
			// event 保存从事件集合取出的事件。
			event := events[0]
			if len(event.Path) != 3 || event.MovementRevision != 1 || event.Kind != movementPush || event.MovementDirection != uint32(direction) || event.FacingDirection != (uint32(direction)+4)%8 || event.CastId != 17 || event.AttackSequence != 11 || event.CastSequence != 12 || event.StepDurationMs != 300 {
				t.Fatalf("位移观察身份/朝向/节拍错误:%v", event)
			}
			// index 定位路径中正在处理的成员，point 接收遍历路径时当前的元素。
			for index, point := range event.Path {
				// want 保存由步骤点位取得的期望值。
				want := stepPoint(origin, uint32(direction), index+1)
				if point.X != int32(want.X) || point.Y != int32(want.Y) {
					t.Fatal("路径包含推测点或丢失邻格")
				}
			}
			if e.Pos != stepPoint(origin, uint32(direction), 3) || e.Direction != (uint32(direction)+4)%8 || !e.ActionReadyAt.Equal(now.Add(900*time.Millisecond)) || !e.NextMove.Equal(now.Add(1500*time.Millisecond)) {
				t.Fatal("源方向或按成功步数赋值的时钟错误")
			}
			// state 保存由移动实体取得的状态。
			state := movementEntity(lastMovementBaseline(t, c), e.Handle)
			if state.MovementRevision != event.MovementRevision || state.MovementKind != event.Kind || !proto.Equal(state.Position, event.Path[2]) {
				t.Fatal("最终基线与事件水位/末点不一致")
			}
		})
	}
}

// TestPushedFourDecorationsKeepPrefixButSkipInvalidPostamble 核对「小物前的成功前缀/early return时钟/观察姿态错误」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPushedFourDecorationsKeepPrefixButSkipInvalidPostamble(t *testing.T) {
	// tests 初始化用作测试集合的临时结构列表。
	tests := []struct {
		// blocked 记录阻挡，使用点位表示。
		// start 记录起点或开始时间，使用点位表示。
		blocked, start devconfig.Point
		// direction 记录朝向，使用数值表示。
		direction uint32
	}{
		{devconfig.Point{X: 1, Y: 7}, devconfig.Point{X: 3, Y: 7}, 6},
		{devconfig.Point{X: 7, Y: 1}, devconfig.Point{X: 7, Y: 3}, 0},
		{devconfig.Point{X: 15, Y: 9}, devconfig.Point{X: 13, Y: 9}, 2},
		{devconfig.Point{X: 7, Y: 15}, devconfig.Point{X: 7, Y: 13}, 4},
	}
	// index 定位测试集合中正在处理的成员，test 接收遍历测试集合时当前的元素。
	for index, test := range tests {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(string(rune('0'+index)), func(t *testing.T) {
			// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
			w, r, c := pushedWorker(t)
			// 单独保留历史场景的阻挡回归；当前挂机地图已移除这些小物。
			w.e.Store.Cfg.Fixture.Map.Width, w.e.Store.Cfg.Fixture.Map.Height = 17, 17
			w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{test.blocked}
			// e 保存从怪物集合取出的怪物集合中的指定项。
			e := r.Monsters[0]
			e.Pos = test.start
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			// action 保存由新增取得的动作，move 保存由新增取得的移动。
			action, move := now.Add(7*time.Second), now.Add(9*time.Second)
			e.ActionReadyAt, e.NextMove = action, move
			e.Action, e.ActionUntil = 2, now.Add(time.Second)
			// out 保存由来源被推动位移取得的输出。
			out := w.sourcePushed(r, reference(r, e), test.direction, 3, now, displacementSource{})
			// events 保存由位移事件集合取得的事件集合。
			events := displacementEvents(t, c)
			if out.Steps != 1 || out.Stop != pushTerrain || out.Postamble || e.ActionReadyAt != action || e.NextMove != move || e.Pos != stepPoint(test.start, test.direction, 1) || len(events) != 1 || len(events[0].Path) != 1 || e.Action != 0 || e.ActionUntil != now {
				t.Fatal("小物前的成功前缀/early return时钟/观察姿态错误")
			}
		})
	}
}

// TestPushedCornerUsesDestinationOnlyAndNeverChainsDynamicBlocker 核验本例普通行走必须先被斜角规则阻挡，用诊断「源Pushed被错误套普通斜角两侧检查」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPushedCornerUsesDestinationOnlyAndNeverChainsDynamicBlocker(t *testing.T) {
	// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
	w, r, c := pushedWorker(t)
	// e 保存从怪物集合取出的怪物集合中的指定项。
	e := r.Monsters[0]
	e.Pos = devconfig.Point{X: 8, Y: 8}
	r.Player.Pos = devconfig.Point{X: 0, Y: 0}
	r.Pet.Pos = devconfig.Point{X: 0, Y: 1}
	w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{{X: 9, Y: 8}, {X: 8, Y: 9}}
	// destination 初始化用作目的地的点位。
	destination := devconfig.Point{X: 9, Y: 9}
	if w.neighbor(r, e.Pos, destination, e) {
		t.Fatal("本例普通行走必须先被斜角规则阻挡")
	}
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// out 保存由来源被推动位移取得的输出。
	if out := w.sourcePushed(r, reference(r, e), 3, 1, now, displacementSource{}); out.Steps != 1 || e.Pos != destination {
		t.Fatal("源Pushed被错误套普通斜角两侧检查")
	}
	r.Player.Pos = stepPoint(e.Pos, 3, 1)
	// blocker 保存玩家，用于当前步骤中的“blocker”。
	blocker := r.Player
	c.frames = nil
	// out 保存由来源被推动位移取得的输出。
	out := w.sourcePushed(r, reference(r, e), 3, 2, now, displacementSource{})
	if out.Steps != 0 || out.Stop != pushOccupied || !out.Postamble || !reflect.DeepEqual(blocker, r.Player) || len(displacementEvents(t, c)) != 0 {
		t.Fatal("动态挡路者被链推或0步伪造路径")
	}
}

// TestPushedTerrainBoundsImmuneAndStaleKeepOldClocks 核对「错误改写旧实体/时钟或发布路径」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPushedTerrainBoundsImmuneAndStaleKeepOldClocks(t *testing.T) {
	// failure 接收遍历文本列表时当前的元素。
	for _, failure := range []string{"bounds", "terrain", "immune", "old_life", "old_scene", "pending", "invalid_direction"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(failure, func(t *testing.T) {
			// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
			w, r, c := pushedWorker(t)
			// e 保存从怪物集合取出的怪物集合中的指定项。
			e := r.Monsters[0]
			e.Pos = devconfig.Point{X: 8, Y: 8}
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			e.NextMove, e.NextAttack, e.ActionReadyAt = now.Add(time.Second), now.Add(2*time.Second), now.Add(3*time.Second)
			// ref 接收引用供后续步骤使用。
			ref := reference(r, e)
			// direction 保存由无符号整数“32”取得的朝向。
			direction := uint32(0)
			switch failure {
			case "bounds":
				e.Pos.Y = 0
			case "terrain":
				w.e.Store.Cfg.Fixture.Map.Blocked = append(w.e.Store.Cfg.Fixture.Map.Blocked, devconfig.Point{X: 8, Y: 7})
			case "immune":
				e.Monster.CanPush = false
			case "old_life":
				ref.Life++
			case "old_scene":
				ref.World.ID++
			case "pending":
				r.Pending = true
			case "invalid_direction":
				direction = 8
			}
			// before 保存当前处理所用的操作前的状态。
			before := *e
			// out 保存由来源被推动位移取得的输出。
			out := w.sourcePushed(r, ref, direction, 1, now, displacementSource{})
			if out.Steps != 0 || out.Postamble || !reflect.DeepEqual(before, *e) || len(c.frames) != 0 {
				t.Fatalf("%s错误改写旧实体/时钟或发布路径", failure)
			}
		})
	}
}

// TestPushedZeroOccupiedPostambleAndHumanClockMapping 核验0步occupied应来源收尾/清了身体动作/误发路径/改了攻击CD，用诊断「Human误用CanPush门槛或未重映射旧更晚walk截止」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPushedZeroOccupiedPostambleAndHumanClockMapping(t *testing.T) {
	// kind 接收遍历数值列表时当前的元素。
	for _, kind := range []uint32{1, 2, 3} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(string(rune('0'+kind)), func(t *testing.T) {
			// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
			w, r, c := pushedWorker(t)
			// e 接收时钟行动实体集合供后续步骤使用。
			e, _ := clockActors(r, kind)
			e.Pos = devconfig.Point{X: 8, Y: 8}
			// blocker 保存当前处理所用的“blocker”。
			blocker := &r.Player
			if e == blocker {
				blocker = r.Pet
			}
			blocker.Pos = devconfig.Point{X: 8, Y: 7}
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			// attack 保存由新增取得的攻击。
			attack := now.Add(11 * time.Second)
			e.NextAttack = attack
			e.ActionReadyAt = now.Add(7 * time.Second)
			e.NextMove = now.Add(9 * time.Second)
			e.Action = 2
			e.ActionUntil = now.Add(8 * time.Second)
			e.MovementRevision = 19
			e.MovementKind = movementWalk
			// out 保存由来源被推动位移取得的输出。
			out := w.sourcePushed(r, reference(r, e), 0, 1, now, displacementSource{})
			// deadline 保存当前处理所用的截止时间。
			deadline := now
			if kind == 1 {
				deadline = now.Add(500 * time.Millisecond)
			}
			if out.Steps != 0 || out.Stop != pushOccupied || !out.Postamble || e.ActionReadyAt != deadline || e.NextMove != deadline || e.NextAttack != attack || e.Action != 2 || e.ActionUntil != now.Add(8*time.Second) || e.MovementRevision != 19 || e.MovementKind != movementWalk || len(c.frames) != 0 {
				t.Fatal("0步occupied没有来源收尾/清了身体动作/误发路径/改了攻击CD")
			}
		})
	}
	// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用。
	w, r, _ := pushedWorker(t)
	// e 保存当前处理所用的玩家。
	e := &r.Player
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	r.Pet.Pos = devconfig.Point{X: 0, Y: 0}
	r.Monsters[0].Pos = devconfig.Point{X: 0, Y: 1}
	e.NextMove = now.Add(9 * time.Second)
	e.ActionReadyAt = now.Add(7 * time.Second)
	// index 标记遍历玩家集合时访问的下标。
	for index := range w.e.Store.Cfg.Fixture.Players {
		w.e.Store.Cfg.Fixture.Players[index].CanPush = false
	}
	// out 保存由来源被推动位移取得的输出。
	out := w.sourcePushed(r, reference(r, e), 0, 1, now, displacementSource{})
	if out.Steps != 1 || !e.NextMove.Equal(now.Add(500*time.Millisecond)) || e.NextMove != e.ActionReadyAt {
		t.Fatal("Human误用CanPush门槛或未重映射旧更晚walk截止")
	}
}

// TestPushedPreservesReleasedEventsEconomyAndAllOtherClocks 核验Pushed不得重抽已释放伤害/经济随机，用诊断「推移改写已释放事件/原时刻/经济游标或其它CD」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPushedPreservesReleasedEventsEconomyAndAllOtherClocks(t *testing.T) {
	// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用。
	w, r, _ := pushedWorker(t)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// e 保存从怪物集合取出的怪物集合中的指定项。
	e := r.Monsters[0]
	e.Pos = devconfig.Point{X: 8, Y: 9}
	e.NextAttack = now.Add(11 * time.Second)
	r.NextSpell = now.Add(12 * time.Second)
	r.SkillCooldowns = map[uint32]time.Time{1: now.Add(13 * time.Second)}
	r.Attacks = []pendingAttack{{Caster: reference(r, &r.Player), Target: reference(r, e), Due: now.Add(14 * time.Second), Amount: 7}}
	r.Casts = []pendingCast{{Scene: r.Scene, Caster: reference(r, &r.Player), Target: e.Handle, TargetLife: e.Life, TargetPoint: e.Pos, Due: now.Add(15 * time.Second), Amount: 8}}
	r.KillFacts = []killFact{{ID: "already_happened", At: now.Add(-time.Second), Roll: 99}}
	// attacks 保留加入新条目后的攻击集合，casts 保留加入新条目后的施法集合，facts 保留加入新条目后的“facts”。
	attacks, casts, facts := append([]pendingAttack(nil), r.Attacks...), append([]pendingCast(nil), r.Casts...), append([]killFact(nil), r.KillFacts...)
	// role 保存当前处理所用的角色。
	role := r.Role
	// spell 保存下一状态法术，用于当前步骤中的法术，skill 保存从技能冷却集合取出的技能，attack 保存下一状态攻击，用于当前步骤中的攻击。
	spell, skill, attack := r.NextSpell, r.SkillCooldowns[1], e.NextAttack
	w.roll = func(int64) int64 { t.Fatal("Pushed不得重抽已释放伤害/经济随机"); return 0 }
	// out 保存由来源被推动位移取得的输出。
	if out := w.sourcePushed(r, reference(r, e), 4, 2, now, displacementSource{}); out.Steps != 2 {
		t.Fatal("前态无法推移")
	}
	if !reflect.DeepEqual(attacks, r.Attacks) || !reflect.DeepEqual(casts, r.Casts) || !reflect.DeepEqual(facts, r.KillFacts) || !reflect.DeepEqual(role, r.Role) || r.NextSpell != spell || r.SkillCooldowns[1] != skill || e.NextAttack != attack {
		t.Fatal("推移改写已释放事件/原时刻/经济游标或其它CD")
	}
}

// TestPushedRealPrimitiveKeepsAttackAndMoveRecoveryIndependent 核对「真实推后攻击仍被移动500ms拖迟」与「推后移动未到500ms已放行」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPushedRealPrimitiveKeepsAttackAndMoveRecoveryIndependent(t *testing.T) {
	// attack 接收遍历布尔状态列表时当前的元素。
	for _, attack := range []bool{false, true} {
		// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用。
		w, r, _ := pushedWorker(t)
		// e 保存从怪物集合取出的怪物集合中的指定项。
		e := r.Monsters[0]
		r.Pet.Pos = devconfig.Point{X: 8, Y: 11}
		// now 保存由Unix 时间取得的当前时间。
		now := time.Unix(1800000000, 0)
		// out 保存由来源被推动位移取得的输出。
		if out := w.sourcePushed(r, reference(r, e), 4, 1, now, displacementSource{}); out.Steps != 1 {
			t.Fatal("推挤前态失败")
		}
		if attack {
			w.attack(r, e, r.Pet, now.Add(300*time.Millisecond-time.Nanosecond))
			if e.AttackSequence != 0 {
				t.Fatal("动作锁未到已攻击")
			}
			w.attack(r, e, r.Pet, now.Add(300*time.Millisecond))
			if e.AttackSequence != 1 {
				t.Fatal("真实推后攻击仍被移动500ms拖迟")
			}
		} else {
			// origin 保存位置，用于当前步骤中的大区或起点。
			origin := e.Pos
			w.move(r, e, devconfig.Point{X: 9, Y: 10}, now.Add(500*time.Millisecond-time.Nanosecond), e.Monster.MoveMS)
			if e.Pos != origin {
				t.Fatal("推后移动未到500ms已放行")
			}
			w.move(r, e, devconfig.Point{X: 9, Y: 10}, now.Add(500*time.Millisecond), e.Monster.MoveMS)
			if e.Pos == origin {
				t.Fatal("推后既有>=边界未恢复移动")
			}
		}
	}
}

// TestPushedCorpseDoesNotBlockAndSameLifePrimitiveDoesNotInventDeadGate 核对「尸体被误当动态Blocking」与「原语擅加源不存在的Dead门禁；技能合法目标过滤应在技能层」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPushedCorpseDoesNotBlockAndSameLifePrimitiveDoesNotInventDeadGate(t *testing.T) {
	// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用。
	w, r, _ := pushedWorker(t)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// e 保存从怪物集合取出的怪物集合中的指定项。
	e := r.Monsters[0]
	e.Pos = devconfig.Point{X: 8, Y: 8}
	r.Pet.Pos = devconfig.Point{X: 8, Y: 7}
	r.Pet.HP = 0
	r.Player.Pos = devconfig.Point{X: 0, Y: 0}
	// out 保存由来源被推动位移取得的输出。
	if out := w.sourcePushed(r, reference(r, e), 0, 1, now, displacementSource{}); out.Steps != 1 {
		t.Fatal("尸体被误当动态Blocking")
	}
	e.HP = 0
	e.Action = 4
	// out 保存由来源被推动位移取得的输出。
	if out := w.sourcePushed(r, reference(r, e), 0, 1, now, displacementSource{}); out.Steps != 1 {
		t.Fatal("原语擅加源不存在的Dead门禁；技能合法目标过滤应在技能层")
	}
	if e.Action != 4 {
		t.Fatal("尸体推移伪装成存活站立姿态")
	}
}

// TestWalkAndManualUseMonotonicRevisionWithoutForcedPackets 核对「摇杆行走未共用提交水位或伪发强制包」与「寻路行走未推进精确64位水位」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestWalkAndManualUseMonotonicRevisionWithoutForcedPackets(t *testing.T) {
	// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
	w, r, c := pushedWorker(t)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	r.MonsterCount = 0
	r.Pet.HP = 0
	r.Player.MovementRevision = uint64(1)<<53 | 7
	w.move(r, &r.Player, devconfig.Point{X: 9, Y: 8}, now, 600)
	if r.Player.MovementRevision != uint64(1)<<53|8 || r.Player.MovementKind != movementWalk {
		t.Fatal("寻路行走未推进精确64位水位")
	}
	r.Manual = true
	r.ManualDirection = 2
	r.LastTick = now.Add(time.Second)
	r.LastBroadcast = now.Add(time.Second)
	w.step(r, now.Add(time.Second))
	if r.Player.Pos != (devconfig.Point{X: 10, Y: 8}) || r.Player.MovementRevision != uint64(1)<<53|9 || r.Player.MovementKind != movementWalk || len(displacementEvents(t, c)) != 0 {
		t.Fatal("摇杆行走未共用提交水位或伪发强制包")
	}
	w.snapshot(r)
	// state 保存由移动实体取得的状态。
	state := movementEntity(lastMovementBaseline(t, c), 1)
	if state.MovementRevision != r.Player.MovementRevision || state.MovementKind != movementWalk {
		t.Fatal("普通行走基线缺水位")
	}
}

// TestMovementOverflowStopsRestOfSameOwnerStep 核对「溢出后同step状态错误 isolated= player=/ pet_changed= monster_changed= frames=」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestMovementOverflowStopsRestOfSameOwnerStep(t *testing.T) {
	// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
	w, r, c := pushedWorker(t)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	r.Player.MovementRevision = math.MaxUint64
	r.Manual = true
	r.ManualDirection = 2
	r.Auto = true
	r.LastTick, r.LastBroadcast = now, now
	r.Player.NextHeal, r.Pet.NextHeal = now.Add(time.Second), now.Add(time.Second)
	r.Monsters[0].NextAttack = now.Add(-time.Second)
	// position 保存当前处理所用的位置，pet 保存当前处理所用的宠物，monster 保存当前处理所用的怪物。
	position, pet, monster := r.Player.Pos, *r.Pet, *r.Monsters[0]
	w.step(r, now)
	if !r.Isolated || r.Player.Pos != position || !reflect.DeepEqual(pet, *r.Pet) || !reflect.DeepEqual(monster, *r.Monsters[0]) || len(c.frames) != 0 {
		t.Fatalf("溢出后同step状态错误 isolated=%v player=%v/%v pet_changed=%v monster_changed=%v frames=%v", r.Isolated, r.Player.Pos, position, !reflect.DeepEqual(pet, *r.Pet), !reflect.DeepEqual(monster, *r.Monsters[0]), c.frames)
	}
}

// TestMovement64BitObservationAndOverflowIsolationBeforeMutation 核对「revision溢出回绕/先提交位置或未隔离」与「64位移动身份丢失」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestMovement64BitObservationAndOverflowIsolationBeforeMutation(t *testing.T) {
	// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
	w, r, c := pushedWorker(t)
	// e 保存从怪物集合取出的怪物集合中的指定项。
	e := r.Monsters[0]
	r.Scene = uint64(1)<<63 | 17
	e.Life = uint64(1)<<63 | 21
	e.MovementRevision = uint64(1)<<63 | 25
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// cast 保存根据无符号整数“64”与当前数据的计算结果与当前数据计算的施法。
	cast := uint64(1)<<63 | 29
	// out 保存由来源被推动位移取得的输出。
	out := w.sourcePushed(r, reference(r, e), 4, 1, now, displacementSource{CastID: cast})
	// event 保存从位移事件集合取出的事件。
	event := displacementEvents(t, c)[0]
	if out.Steps != 1 || event.SceneEpoch != r.Scene || event.EntityLife != e.Life || event.MovementRevision != uint64(1)<<63|26 || event.CastId != cast || movementEntity(lastMovementBaseline(t, c), e.Handle).MovementRevision != event.MovementRevision {
		t.Fatal("64位移动身份丢失")
	}
	// walk 接收遍历布尔状态列表时当前的元素。
	for _, walk := range []bool{false, true} {
		w, r, c = pushedWorker(t)
		e = r.Monsters[0]
		e.MovementRevision = math.MaxUint64
		// before 保存当前处理所用的操作前的状态。
		before := *e
		if walk {
			w.move(r, e, devconfig.Point{X: 9, Y: 9}, now, 1500)
		} else {
			w.sourcePushed(r, reference(r, e), 4, 1, now, displacementSource{})
		}
		if !r.Isolated || !reflect.DeepEqual(before, *e) || len(c.frames) != 0 {
			t.Fatal("revision溢出回绕/先提交位置或未隔离")
		}
	}
}

// TestPushedNotReadyAndHealthyBaselineNeverReplayOldPath 核对「健康首全量补播旧path或丢失当前水位」与「Ready前停止世界或发布瞬时路径」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPushedNotReadyAndHealthyBaselineNeverReplayOldPath(t *testing.T) {
	// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
	w, r, c := pushedWorker(t)
	r.Ready = false
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// e 保存从怪物集合取出的怪物集合中的指定项。
	e := r.Monsters[0]
	// out 保存由来源被推动位移取得的输出。
	out := w.sourcePushed(r, reference(r, e), 4, 2, now, displacementSource{})
	if out.Steps != 2 || e.MovementRevision != 1 || len(c.frames) != 0 {
		t.Fatal("Ready前停止世界或发布瞬时路径")
	}
	r.Conn = &capturedConnection{}
	// next 按目标类型读取当前接口值，得到下一状态。
	next := r.Conn.(*capturedConnection)
	r.Baseline++
	r.Auth.SessionEpoch++
	r.InputSequence = 0
	w.baseline(r)
	r.Ready = true
	w.snapshot(r)
	if len(displacementEvents(t, next)) != 0 || movementEntity(lastMovementBaseline(t, next), e.Handle).MovementRevision != 1 || movementEntity(lastMovementBaseline(t, next), e.Handle).MovementKind != movementPush {
		t.Fatal("健康首全量补播旧path或丢失当前水位")
	}
}

// groundTarget 处理地面目标。
// w 记录房间执行器，r 指向当前执行器拥有并更新的目标房间，c 记录已捕获连接，seq 记录序号，使用数值表示，point 记录点位。
func groundTarget(w *worker, r *room, c *capturedConnection, seq uint32, point devconfig.Point) {
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SMoveIntent, seq, uint64(r.Auth.SessionEpoch), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: seq, Destination: pointPB(point)})})
}

// TestNavigationOnlyOwnerStepCompletesCurrentTargetAcrossForcePrefixes 核对「没有从新脚点继续导航或在非到达检查处分发完成」与「step未确认到达/丢失最后目标或改变身份」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNavigationOnlyOwnerStepCompletesCurrentTargetAcrossForcePrefixes(t *testing.T) {
	// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
	w, r, c := pushedWorker(t)
	r.MonsterCount = 0
	r.Pet.HP = 0
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// goal 建立带初始值的点位，供本段继续处理。
	goal := devconfig.Point{X: 9, Y: 8}
	groundTarget(w, r, c, 10, goal)
	// revision 保存当前处理所用的修订号。
	revision := r.Navigation.Revision
	for range 2 {
		// out 保存由来源被推动位移取得的输出。
		if out := w.sourcePushed(r, reference(r, &r.Player), 2, 1, now, displacementSource{}); out.Steps != 1 {
			t.Fatal("推挤前态错误")
		}
	}
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CSceneBaseline {
			// scene 保存当前处理所用的场景。
			scene := &pb.SceneBaselineChunk{}
			if proto.Unmarshal(frame.Payload, scene) != nil {
				t.Fatal("基线解码失败")
			}
			if scene.NavigationState.Status != navigationActive || scene.NavigationState.Revision != revision || scene.NavigationState.InputSequence != 10 {
				t.Fatal("经过目标的中间force基线错误完成导航")
			}
		}
	}
	if r.Destination == nil || *r.Destination != goal || r.Player.Pos == goal {
		t.Fatal("推挤取消或伪完成了导航")
	}
	w.step(r, now.Add(500*time.Millisecond))
	if r.Player.Pos != goal || r.Navigation.Status != navigationActive {
		t.Fatal("没有从新脚点继续导航或在非到达检查处分发完成")
	}
	w.step(r, now.Add(550*time.Millisecond))
	if r.Destination != nil || r.Navigation.Status != navigationArrived || r.Navigation.Destination == nil || *r.Navigation.Destination != goal || r.Navigation.Revision != revision {
		t.Fatal("owner step未确认到达/丢失最后目标或改变身份")
	}
}

// TestNavigationSameCoordinateNewInputAndNewBindingKeepDistinctRevision 核验相同坐标/复用input应新目标身份，用诊断「主动方向接管未引用原ground身份」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNavigationSameCoordinateNewInputAndNewBindingKeepDistinctRevision(t *testing.T) {
	// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
	w, r, c := pushedWorker(t)
	r.MonsterCount = 0
	r.Pet.HP = 0
	// goal 保存位置，用于当前步骤中的目标格。
	goal := r.Player.Pos
	groundTarget(w, r, c, 1, goal)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	w.step(r, now)
	// old 保存由协议取得的原状态。
	old := r.Navigation.Proto()
	if old.Status != navigationArrived || old.Revision != 1 {
		t.Fatal("初次目标未正常到达")
	}
	// 与健康join一致：输入水位重置，目标revision保留；新目标复用input=1仍有全新身份。
	r.InputSequence = 0
	groundTarget(w, r, c, 1, goal)
	if r.Navigation.Revision != 2 || r.Navigation.InputSequence != old.InputSequence || r.Navigation.Status != navigationActive {
		t.Fatal("相同坐标/复用input没有新目标身份")
	}
	w.snapshot(r)
	// state 保存导航状态，用于当前步骤中的状态。
	state := lastMovementBaseline(t, c).NavigationState
	if state.Revision == old.Revision || state.Status != navigationActive {
		t.Fatal("旧到达状态完成了新目标")
	}
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SMoveIntent, 2, 3, &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 2, ReleaseJoystick: true})})
	if r.Navigation.Status != navigationActive || r.Navigation.Revision != 2 || r.Destination == nil {
		t.Fatal("释放摇杆终止导航")
	}
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SMoveIntent, 3, 3, &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 3, Direction: 2})})
	if r.Navigation.Status != navigationManual || r.Navigation.Revision != 2 || r.Navigation.InputSequence != 1 || r.Navigation.Destination == nil || r.Destination != nil {
		t.Fatal("主动方向接管未引用原ground身份")
	}
}

// TestNavigationExplicitCancelAcknowledgesOriginalIdentityWithoutCombatSideEffects 核验明确取消应仅清除导航和摇杆，或改变战斗/永久提交时钟，用诊断「取消确认未沿原请求返回基线，或改变原ground身份」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNavigationExplicitCancelAcknowledgesOriginalIdentityWithoutCombatSideEffects(t *testing.T) {
	// state 接收遍历文本列表时当前的元素。
	for _, state := range []string{"active", "arrived", "manual", "dead_pending", "no_ground"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(state, func(t *testing.T) {
			// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
			w, r, c := pushedWorker(t)
			r.MonsterCount, r.Pet.HP = 0, 0
			// goal 建立带初始值的点位，供本段继续处理。
			goal := devconfig.Point{X: 10, Y: 8}
			groundTarget(w, r, c, 7, goal)
			r.Manual = true
			switch state {
			case "arrived":
				r.Navigation.Status, r.Destination = navigationArrived, nil
			case "manual":
				r.Navigation.Status, r.Destination = navigationManual, nil
			case "dead_pending":
				r.Player.HP, r.Pending = 0, true
			case "no_ground":
				r.Navigation, r.Destination = navigationState{}, nil
			}
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			r.Player.NextMove, r.Player.NextAttack, r.Player.ActionReadyAt = now.Add(time.Second), now.Add(2*time.Second), now.Add(3*time.Second)
			r.NextSpell, r.SkillCooldowns = now.Add(4*time.Second), map[uint32]time.Time{2: now.Add(5 * time.Second)}
			// beforePlayer 保存玩家，用于当前步骤中的操作前的状态玩家，beforeNav 保存由协议取得的操作前的状态“nav”。
			// beforePending 保存待确认，用于当前步骤中的操作前的状态待确认。
			beforePlayer, beforeNav, beforePending := r.Player, r.Navigation.Proto(), r.Pending
			c.frames = nil
			w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SMoveIntent, 901, 3, &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 8, CancelNavigation: true})})
			if r.Destination != nil || r.Manual || r.InputSequence != 8 || r.Pending != beforePending || !reflect.DeepEqual(beforePlayer, r.Player) || r.NextSpell != now.Add(4*time.Second) || r.SkillCooldowns[2] != now.Add(5*time.Second) {
				t.Fatal("明确取消没有仅清除导航和摇杆，或改变战斗/永久提交时钟")
			}
			// want 按目标类型读取当前接口值，得到期望值。
			want := proto.Clone(beforeNav).(*pb.NavigationState)
			if state == "no_ground" {
				want.Status = navigationNone
			} else {
				want.Status = navigationCancelled
			}
			if !proto.Equal(want, r.Navigation.Proto()) || len(c.frames) != 1 || c.frames[0].ID != p.MsgS2CSceneBaseline || c.frames[0].Request != 901 || c.frames[0].Binding != 3 || !proto.Equal(lastMovementBaseline(t, c).NavigationState, want) {
				t.Fatal("取消确认未沿原请求返回基线，或改变原ground身份")
			}
			w.step(r, now.Add(10*time.Second))
			if r.Player.Pos != beforePlayer.Pos || r.Navigation.Status != want.Status {
				t.Fatal("取消后的旧目标仍驱动下一帧移动")
			}
		})
	}
}

// TestNavigationCancelRejectsConflictingOrStaleInputWithoutChangingTarget 核对「未受理的取消覆盖原目标/输入水位/身体状态」与「非法取消未沿原请求明确拒绝」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNavigationCancelRejectsConflictingOrStaleInputWithoutChangingTarget(t *testing.T) {
	// name 接收遍历文本列表时当前的元素。
	for _, name := range []string{"scene", "binding", "connection", "stale", "destination", "release", "direction", "not_ready"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
			w, r, c := pushedWorker(t)
			// goal 建立带初始值的点位，供本段继续处理。
			goal := devconfig.Point{X: 10, Y: 8}
			groundTarget(w, r, c, 7, goal)
			r.Manual = true
			// beforeNav 保存由协议取得的操作前的状态“nav”，beforePlayer 保存玩家，用于当前步骤中的操作前的状态玩家。
			beforeNav, beforePlayer := r.Navigation.Proto(), r.Player
			// q 保存当前处理所用的移动意图。
			q := &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 8, CancelNavigation: true}
			// binding 保存由无符号整数“64”取得的会话绑定，code 保存结果码无效“argument”，用于当前步骤中的结果码。
			binding, code := uint64(3), p.CodeInvalidArgument
			// sender 保存当前处理所用的“sender”。
			sender := c
			switch name {
			case "scene":
				q.SceneEpoch++
			case "binding":
				binding, code = 2, p.CodeStaleSession
			case "connection":
				sender, code = &capturedConnection{}, p.CodeStaleSession
			case "stale":
				q.InputSequence, code = 7, p.CodeStaleInput
			case "destination":
				q.Destination = pointPB(goal)
			case "release":
				q.ReleaseJoystick = true
			case "direction":
				q.Direction = 2
			case "not_ready":
				r.Ready, code = false, p.CodeLineRecovering
			}
			c.frames = nil
			w.command(command{role: "role", conn: sender, frame: wire.Message(p.MsgC2SMoveIntent, 902, binding, q)})
			if len(sender.frames) != 1 || sender.frames[0].ID != p.MsgC2SMoveIntent || sender.frames[0].Request != 902 || sender.frames[0].Error != code {
				t.Fatal("非法取消未沿原请求明确拒绝")
			}
			if !proto.Equal(beforeNav, r.Navigation.Proto()) || r.Destination == nil || *r.Destination != goal || !r.Manual || r.InputSequence != 7 || !reflect.DeepEqual(beforePlayer, r.Player) {
				t.Fatal("未受理的取消覆盖原目标/输入水位/身体状态")
			}
		})
	}
}

// TestNavigationDeathSameMapRevivePreservesGroundAndRejectsOldLifePush 核对「同图复活未保留原目标/重置生命观察水位」与「旧life迟到force作用于复活实体」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNavigationDeathSameMapRevivePreservesGroundAndRejectsOldLifePush(t *testing.T) {
	// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
	w, r, c := pushedWorker(t)
	r.MonsterCount = 0
	r.Pet.HP = 0
	// goal 建立带初始值的点位，供本段继续处理。
	goal := devconfig.Point{X: 10, Y: 8}
	groundTarget(w, r, c, 7, goal)
	// identity 保存由协议取得的身份。
	identity := r.Navigation.Proto()
	// oldRef 保存由引用取得的原状态引用。
	oldRef := reference(r, &r.Player)
	r.Player.HP = 0
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	w.step(r, now)
	if !proto.Equal(identity, r.Navigation.Proto()) || r.Destination == nil {
		t.Fatal("死亡丢弃地面导航")
	}
	r.Manual = true
	w.revivePlayer(r)
	if r.Manual || r.Destination == nil || *r.Destination != goal || !proto.Equal(identity, r.Navigation.Proto()) || r.Player.MovementRevision != 0 || r.Player.MovementKind != movementInitial {
		t.Fatal("同图复活未保留原目标/重置生命观察水位")
	}
	// position 保存当前处理所用的位置。
	position := r.Player.Pos
	// out 保存由来源被推动位移取得的输出。
	if out := w.sourcePushed(r, oldRef, 2, 1, now, displacementSource{}); out.Stop != pushStale || r.Player.Pos != position {
		t.Fatal("旧life迟到force作用于复活实体")
	}
	// i 标记循环或列表中的当前位置。
	for i := 1; i <= 6 && r.Navigation.Status != navigationArrived; i++ {
		w.step(r, now.Add(time.Duration(i)*time.Second))
	}
	if r.Player.Pos != goal || r.Navigation.Status != navigationArrived || r.Navigation.Revision != identity.Revision {
		t.Fatal("复活后未从新脚点继续到原目标")
	}
}

// TestNavigationOverflowPreservesOriginalTargetAndNewWorldClears 核对「目标身份溢出回绕/覆盖旧目标」与「新世界继承旧导航/移动身份」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNavigationOverflowPreservesOriginalTargetAndNewWorldClears(t *testing.T) {
	// w 接收被推动位移房间执行器供后续步骤使用，r 接收被推动位移房间执行器供后续步骤使用，c 接收被推动位移房间执行器供后续步骤使用。
	w, r, c := pushedWorker(t)
	// goal 建立带初始值的点位，供本段继续处理。
	goal := devconfig.Point{X: 10, Y: 8}
	groundTarget(w, r, c, 1, goal)
	r.Navigation.Revision = math.MaxUint64
	// old 保存由协议取得的原状态。
	old := r.Navigation.Proto()
	groundTarget(w, r, c, 2, devconfig.Point{X: 11, Y: 8})
	if !r.Isolated || !proto.Equal(old, r.Navigation.Proto()) || r.Destination == nil || *r.Destination != goal {
		t.Fatal("目标身份溢出回绕/覆盖旧目标")
	}
	// fresh 保存当前处理所用的首次装载。
	fresh := &room{Manual: true, worldState: &worldState{Destination: &goal, Navigation: r.Navigation}}
	w.initialize(fresh, r.Role)
	if fresh.Navigation.Revision != 0 || fresh.Navigation.Status != navigationNone || fresh.Destination != nil || fresh.Manual || fresh.Player.MovementRevision != 0 {
		t.Fatal("新世界继承旧导航/移动身份")
	}
}

// TestAutoEnabledFalseAndZeroPriorityAreIndependentFromFlaming 核对「priority0被错误当显式关闭而非排序值」与「主动selector门禁误停独立烈火自动」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAutoEnabledFalseAndZeroPriorityAreIndependentFromFlaming(t *testing.T) {
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
	w, r, _ := stageAWorker(t, 1)
	r.Player.MP = r.Player.MaxMP
	// index 标记遍历技能集合时访问的下标。
	for index := range w.e.Store.Cfg.Fixture.Skills {
		w.e.Store.Cfg.Fixture.Skills[index].AutoEnabled = false
	}
	if w.autoSkill(r, time.Now()) || len(r.Casts) != 0 {
		t.Fatal("显式auto关闭仍选择主动技")
	}
	// index 标记遍历技能集合时访问的下标。
	for index := range w.e.Store.Cfg.Fixture.Skills {
		// s 保存当前处理所用的技能集合中的指定项。
		s := &w.e.Store.Cfg.Fixture.Skills[index]
		if s.ID == 1 {
			s.AutoEnabled = true
			s.AutoPriority = 0
		}
	}
	if !w.autoSkill(r, time.Now()) || len(r.Casts) != 1 || r.Casts[0].Skill.ID != 1 {
		t.Fatal("priority0被错误当显式关闭而非排序值")
	}
	w, r, _ = stageBWorker(t, 0, 26)
	// index 标记遍历技能集合时访问的下标。
	for index := range w.e.Store.Cfg.Fixture.Skills {
		w.e.Store.Cfg.Fixture.Skills[index].AutoEnabled = false
	}
	r.Auto = true
	r.Player.MP = r.Player.MaxMP
	// code 保存由设置技能模式取得的结果码。
	if code := w.setSkillMode(r, 26, true, time.Now()); code != 0 {
		t.Fatalf("独立烈火充能被auto门禁拒绝:%d", code)
	}
	// code 保存由攻击“facing”取得的结果码。
	if code := w.attackFacing(r, 4, 0, 1, time.Now()); code != 0 || r.Player.AttackSkillID != 26 {
		t.Fatalf("主动selector门禁误停独立烈火自动:%d/%d", code, r.Player.AttackSkillID)
	}
}
