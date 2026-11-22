// 本文件回归网格坐标、八方向移动、对角阻挡和大网格搜索范围。
package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"strconv"
	"testing"
	"time"
)

// TestGridEightDirectionsAndCornerBlocking 验证 13、16、17 格三种地图的八方向移动、边界限制，以及墙角阻挡移动和攻击。
// t 提供断言失败报告、子用例和清理注册。
func TestGridEightDirectionsAndCornerBlocking(t *testing.T) {
	// size 接收遍历数值列表时当前的元素。
	for _, size := range []int{13, 16, 17} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(strconv.Itoa(size), func(t *testing.T) { testGridEightDirectionsAndCornerBlocking(t, size) })
	}
}

// testGridEightDirectionsAndCornerBlocking 为指定尺寸地图分别检查空地移动、斜角阻挡和攻击时钟是否保持。
// t 提供断言失败报告、子用例和清理注册，size 记录大小，使用数值表示。
func testGridEightDirectionsAndCornerBlocking(t *testing.T, size int) {
	// f 组装开发测试配置所需的数据。
	f := devconfig.Fixture{}
	f.Map.Width = size
	f.Map.Height = size
	// e 保存当前处理所用的游戏引擎。
	e := &Engine{Store: &persistence.Store{Cfg: devconfig.Config{Fixture: f}}}
	// w 保存当前处理所用的房间执行器。
	w := &worker{e: e}
	// i 定位朝向集合中正在处理的成员，d 接收遍历朝向集合时当前的元素。
	for i, d := range directions {
		// r 保存当前处理所用的房间。
		r := &room{Player: entity{Kind: 1, HP: 10, Pos: devconfig.Point{X: 8, Y: 8}}, worldState: &worldState{}}
		// to 建立带初始值的点位，供本段继续处理。
		to := devconfig.Point{X: 8 + d.X, Y: 8 + d.Y}
		if !w.neighbor(r, r.Player.Pos, to, &r.Player) {
			t.Errorf("方向%d被空地图错误阻挡", i)
		}
	}
	e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{{X: 9, Y: 8}}
	// r 保存当前处理所用的房间。
	r := &room{Role: domain.Role{}, Player: entity{Kind: 1, HP: 10, Pos: devconfig.Point{X: 8, Y: 8}}, worldState: &worldState{}}
	if w.neighbor(r, r.Player.Pos, devconfig.Point{X: 9, Y: 9}, &r.Player) {
		t.Fatal("斜向移动穿过墙角")
	}
	if w.walkable(r, devconfig.Point{X: -1, Y: 8}) || w.walkable(r, devconfig.Point{X: size, Y: 8}) || !w.walkable(r, devconfig.Point{X: size - 1, Y: size - 1}) {
		t.Fatal("可走范围越界")
	}
	// target 保存当前处理所用的目标。
	target := &entity{Kind: 3, HP: 10, Pos: devconfig.Point{X: 9, Y: 9}}
	w.attack(r, &r.Player, target, time.Now())
	if target.HP != 10 || !r.Player.NextAttack.IsZero() {
		t.Fatal("斜角阻挡仍造成伤害或消耗攻击CD")
	}
}

// TestSeventeenGridCanRouteThroughMoreThanOld256VisitedNodes 核对「17格有效绕行在被旧256预算截断或跨格瞬移」与「超过256节点的17格路径未能抵达(16,16)」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSeventeenGridCanRouteThroughMoreThanOld256VisitedNodes(t *testing.T) {
	// f 组装开发测试配置所需的数据。
	f := devconfig.Fixture{}
	f.Map.Width, f.Map.Height = 17, 17
	// y 规定纵坐标的初始取值。
	for y := 1; y < f.Map.Height; y++ {
		f.Map.Blocked = append(f.Map.Blocked, devconfig.Point{X: 14, Y: y})
	}
	// w 保存当前处理所用的房间执行器。
	w := &worker{e: &Engine{Store: &persistence.Store{Cfg: devconfig.Config{Fixture: f}}}}
	// r 保存当前处理所用的房间。
	r := &room{Player: entity{Kind: 1, HP: 10, Pos: devconfig.Point{X: 0, Y: 16}}, worldState: &worldState{}}
	// target 初始化用作目标的点位。
	target := devconfig.Point{X: 16, Y: 16}
	// now 记录本次处理开始时的时间。
	now := time.Now()
	// step 规定步骤的初始取值。
	for step := 0; step < f.GridCellBudget() && r.Player.Pos != target; step++ {
		// before 保存位置，用于当前步骤中的操作前的状态。
		before := r.Player.Pos
		w.move(r, &r.Player, target, now.Add(time.Duration(step)*600*time.Millisecond), 600)
		if r.Player.Pos == before || distance(r.Player.Pos, before) != 1 || !w.walkable(r, r.Player.Pos) {
			t.Fatalf("17格有效绕行在%+v被旧256预算截断或跨格瞬移", before)
		}
	}
	if r.Player.Pos != target {
		t.Fatal("超过256节点的17格路径未能抵达(16,16)")
	}
}

// TestSeventeenGridIntentsKeepLogicalCoordinatesAndRejectOutsideEdge 核对「第17行列的协议坐标被旧16边界拒绝或当成屏幕像素换算」与「地图外坐标未拒绝或破坏原导航目标」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSeventeenGridIntentsKeepLogicalCoordinatesAndRejectOutsideEdge(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 1, 23)
	w.e.Store.Cfg.Fixture.Map.Width, w.e.Store.Cfg.Fixture.Map.Height = 17, 17
	// move 封装移动对应的后续处理，sequence 记录序号，使用数值表示，x 记录数值。
	move := func(sequence uint32, x int32) {
		w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SMoveIntent, sequence, 3, &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: sequence, Destination: &pb.GridPoint{X: x, Y: 16}})})
	}
	move(1, 16)
	if r.Destination == nil || *r.Destination != (devconfig.Point{X: 16, Y: 16}) {
		t.Fatal("第17行列的协议坐标被旧16边界拒绝或当成屏幕像素换算")
	}
	move(2, 17)
	if c.frames[len(c.frames)-1].Error != p.CodeInvalidArgument || *r.Destination != (devconfig.Point{X: 16, Y: 16}) {
		t.Fatal("地图外坐标未拒绝或破坏原导航目标")
	}
	r.Player.Pos = devconfig.Point{X: 15, Y: 15}
	r.Monsters[0].Pos = devconfig.Point{X: 16, Y: 16}
	// now 记录本次处理开始时的时间。
	now := time.Now()
	// mana 保存魔法值，用于当前步骤中的魔法值。
	mana := r.Player.MP
	if w.cast(r, 23, 0, 3, now, &pb.GridPoint{X: 17, Y: 16}) != p.CodeSkillTarget || r.Player.MP != mana {
		t.Fatal("地图外技能中心受理或扣蓝")
	}
	if w.cast(r, 23, 0, 4, now, &pb.GridPoint{X: 16, Y: 16}) != 0 || r.Casts[0].TargetPoint != (devconfig.Point{X: 16, Y: 16}) {
		t.Fatal("技能未保留第17行列的逻辑格坐标")
	}
}
