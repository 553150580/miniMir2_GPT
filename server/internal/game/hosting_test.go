// 本文件回归死亡角色调整挂机刷怪数量时的场景推送和生命状态。
package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"reflect"
	"testing"
	"time"
)

// capturedConnection 保存已捕获连接所需的状态。
// frames 记录协议帧集合，使用协议帧列表表示。
type capturedConnection struct{ frames []wire.Frame }

// Alive 判断存活状态是否满足要求。
// c 是调用该方法的已捕获连接实例。
func (c *capturedConnection) Alive() bool { return true }

// Deliver 判断“deliver”是否满足要求。
// c 是调用该方法的已捕获连接实例，f 记录协议帧。
func (c *capturedConnection) Deliver(f wire.Frame) bool { c.frames = append(c.frames, f); return true }

// Close 关闭当前请求或状态，并把结果交给调用方。
// c 是调用该方法的已捕获连接实例。
func (c *capturedConnection) Close(string) {}

// SetRole 设置角色。
// c 是调用该方法的已捕获连接实例。
func (c *capturedConnection) SetRole(string) {}

// RoleID 返回角色标识供调用方使用。
// c 是调用该方法的已捕获连接实例。
func (c *capturedConnection) RoleID() string { return "role" }

// SetBinding 设置会话绑定。
// c 是调用该方法的已捕获连接实例。
func (c *capturedConnection) SetBinding(uint64) {}

// Binding 返回会话绑定供调用方使用。
// c 是调用该方法的已捕获连接实例。
func (c *capturedConnection) Binding() uint64 { return 3 }

// 满包拒绝Auto的旧测试完整字节留在阶段历史；普通Auto按独立容量政策测试。

// TestDeadPlayerCountChangePushesSnapshotWithoutReset 核对「死亡切怪未沿请求seq回复hosting」与「数量切换使死亡玩家复活或位置/MP重置」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDeadPlayerCountChangePushesSnapshotWithoutReset(t *testing.T) {
	// conn 保存当前处理所用的连接。
	conn := &capturedConnection{}
	// stamp 保存由Unix 时间取得的标记时间。
	stamp := time.Unix(1800000000, 0)
	// r 保存当前处理所用的房间。
	r := &room{Conn: conn, Ready: true, Scene: 20, Baseline: 30, DeathID: "original-death", Auth: persistence.Auth{SessionEpoch: 3}, Role: domain.Role{HostingSeconds: 42, Revision: 7},
		Player: entity{Handle: 1, Kind: 1, HP: 0, MaxHP: 18, MP: 6, MaxMP: 14, Action: 4, Pos: devconfig.Point{X: 8, Y: 8}, NextMove: stamp.Add(time.Second), NextAttack: stamp.Add(2 * time.Second)},
		Pet:    &entity{Handle: 2, Kind: 2, HP: 29, MaxHP: 60, Pos: devconfig.Point{X: 8, Y: 7}, NextMove: stamp.Add(3 * time.Second), NextAttack: stamp.Add(4 * time.Second)}, worldState: &worldState{MonsterCount: 1, Auto: false},
	}
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 8; i++ {
		r.Monsters = append(r.Monsters, &entity{Handle: uint32(10 + i), Kind: 3, Def: 1, HP: int64(i), MaxHP: 15, Pos: devconfig.Point{X: i + 1, Y: 5}, Action: 3, NextMove: stamp.Add(time.Duration(i+5) * time.Second), NextAttack: stamp.Add(time.Duration(i+15) * time.Second), RespawnAt: stamp.Add(time.Duration(i+25) * time.Second)})
	}
	// w 接收观察到的房间执行器供后续步骤使用。
	w := observedWorker(t, r)
	// playerBefore 保存玩家，用于当前步骤中的玩家操作前的状态，petBefore 保存当前处理所用的宠物操作前的状态。
	playerBefore, petBefore := r.Player, *r.Pet
	// pointers 保留加入新条目后的“pointers”。
	pointers := append([]*entity(nil), r.Monsters...)
	// before 创建用于维护操作前的状态的容器或通道。
	before := make([]entity, len(r.Monsters))
	// i 定位怪物集合中正在处理的成员，m 接收遍历怪物集合时当前的元素。
	for i, m := range r.Monsters {
		before[i] = *m
	}
	// index 定位数值列表中正在处理的成员，count 接收遍历数值列表时当前的元素。
	for index, count := range []uint32{4, 8} {
		conn.frames = nil
		// request 保存根据无符号整数“32”与数量计算的请求。
		request := uint32(10) + count
		w.command(command{role: "role", conn: conn, frame: wire.Message(p.MsgC2SAutoBattle, request, 3, &pb.AutoBattleIntent{SceneEpoch: 20, Enabled: false, MonsterCount: count})})
		if len(conn.frames) != 2 || conn.frames[0].ID != p.MsgS2CHostingState || conn.frames[0].Request != request {
			t.Fatalf("死亡切怪未沿请求seq回复hosting:%v", conn.frames)
		}
		// hosting 保存当前处理所用的离线托管。
		hosting := &pb.HostingState{}
		// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
		if err := proto.Unmarshal(conn.frames[0].Payload, hosting); err != nil {
			t.Fatal(err)
		}
		if hosting.AutoBattleEnabled || hosting.RemainHostingSec != 42 || hosting.StateRevision != 7 {
			t.Fatalf("死亡切怪改变了托管状态:%v", hosting)
		}
		// frame 保存从协议帧集合取出的协议帧。
		frame := conn.frames[1]
		// scene 保存当前处理所用的场景。
		scene := &pb.SceneBaselineChunk{}
		// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
		if err := proto.Unmarshal(frame.Payload, scene); err != nil {
			t.Fatal(err)
		}
		if frame.ID != p.MsgS2CSceneBaseline || frame.Request != 0 || scene.SceneEpoch != 20 || scene.BaselineId != 30 || scene.StateSequence != uint64(index+1) {
			t.Fatalf("死亡切怪未立即同步当前基线:%v %v", frame, scene)
		}
		// monsters 规定怪物集合的初始取值。
		monsters := 0
		// e 接收遍历实体时当前的元素。
		for _, e := range scene.Entities {
			if e.EntityKind == 1 && (e.Hp != 0 || e.Mp != 6 || e.Position.X != 8 || e.Position.Y != 8) {
				t.Fatal("数量切换使死亡玩家复活或位置/MP重置")
			}
			if e.EntityKind == 3 {
				monsters++
				// original 保存从操作前的状态取出的原始。
				original := before[int(e.EntityHandle)-10]
				if e.Hp != original.HP || e.Position.X != int32(original.Pos.X) || e.Position.Y != int32(original.Pos.Y) {
					t.Fatal("基线改变了怪物HP或位置")
				}
			}
		}
		if monsters != int(count) || r.MonsterCount != int(count) {
			t.Fatalf("基线怪物数=%d，预期%d", monsters, count)
		}
		if !reflect.DeepEqual(playerBefore, r.Player) || !reflect.DeepEqual(petBefore, *r.Pet) || r.DeathID != "original-death" {
			t.Fatal("数量切换重置了玩家/宠物/死亡状态或CD")
		}
		// i 定位怪物集合中正在处理的成员，m 接收遍历怪物集合时当前的元素。
		for i, m := range r.Monsters {
			if m != pointers[i] || !reflect.DeepEqual(before[i], *m) {
				t.Fatalf("数量切换重建或重置了怪物%d及其HP/CD/复活时点", i)
			}
		}
	}
}
