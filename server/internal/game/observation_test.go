package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"testing"
	"time"
)

func observedWorker(t *testing.T, r *room) *worker {
	t.Helper()
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range r.Monsters {
		if m.Monster == nil {
			m.Monster = &cfg.Fixture.Monsters[0]
		}
	}
	return &worker{e: &Engine{Store: &persistence.Store{Cfg: cfg}}, rooms: map[string]*room{"role": r}}
}
func observedRoom() (*room, *capturedConnection) {
	c := &capturedConnection{}
	r := &room{Conn: c, Ready: true, Scene: 20, Baseline: 30, MonsterCount: 1, Auth: persistence.Auth{SessionEpoch: 3}, Player: entity{Handle: 1, Kind: 1, HP: 100, MaxHP: 100, Pos: devconfig.Point{X: 8, Y: 8}}, Pet: &entity{Handle: 2, Kind: 2, HP: 60, MaxHP: 60, Pos: devconfig.Point{X: 7, Y: 8}}, Monsters: []*entity{{Handle: 10, Kind: 3, Def: 1, HP: 100, MaxHP: 100, Pos: devconfig.Point{X: 8, Y: 9}}}}
	return r, c
}
func TestAttackObservationSurvivesStruckAndCooldownReject(t *testing.T) {
	r, c := observedRoom()
	w := observedWorker(t, r)
	now := time.Now()
	w.attack(r, &r.Player, r.Monsters[0], now)
	if r.Player.AttackSequence != 1 {
		t.Fatal("已受理攻击未递增观察序号")
	}
	w.attack(r, &r.Player, r.Monsters[0], now.Add(time.Millisecond))
	if r.Player.AttackSequence != 1 {
		t.Fatal("冷却拒绝错误递增攻击序号")
	}
	// 同一tick怪物反击覆盖action，但不能覆盖玩家攻击序号。
	w.attack(r, r.Monsters[0], &r.Player, now)
	if r.Player.Action != 3 || r.Player.AttackSequence != 1 || r.Monsters[0].AttackSequence != 1 {
		t.Fatal("同tick受击覆盖了独立攻击观察")
	}
	w.snapshot(r)
	scene := &pb.SceneBaselineChunk{}
	if err := proto.Unmarshal(c.frames[len(c.frames)-1].Payload, scene); err != nil {
		t.Fatal(err)
	}
	for _, e := range scene.Entities {
		expected := map[uint32]uint32{1: 600, 2: 1000, 3: 1500}[e.EntityKind]
		if e.MoveIntervalMs != expected {
			t.Fatalf("kind%d移动间隔=%d，预期%d", e.EntityKind, e.MoveIntervalMs, expected)
		}
		if e.EntityKind == 1 && (e.AttackSequence != 1 || e.Action != 3) {
			t.Fatal("PB未独立回传被受击覆盖的攻击")
		}
	}
	interval := w.e.Store.Cfg.Fixture.Players[0].AttackMS
	w.attack(r, &r.Player, r.Monsters[0], now.Add(time.Duration(interval)*time.Millisecond))
	if r.Player.AttackSequence != 2 {
		t.Fatal("冷却完成后的新攻击未递增序号")
	}
}
func TestLethalStatePushesBeforeBroadcastWindow(t *testing.T) {
	r, c := observedRoom()
	w := observedWorker(t, r)
	r.Player.HP = 1
	now := time.Now()
	r.LastBroadcast = now.Add(-50 * time.Millisecond)
	w.attack(r, r.Monsters[0], &r.Player, now)
	if r.Player.HP != 0 || r.Player.Action != 4 || r.DeathID == "" {
		t.Fatal("攻击没有形成真实死亡状态")
	}
	if len(c.frames) != 2 || c.frames[0].ID != p.MsgS2CDeathNotice || c.frames[1].ID != p.MsgS2CSceneBaseline {
		t.Fatalf("广播窗口前致死没有立即推终态:%v", c.frames)
	}
	scene := &pb.SceneBaselineChunk{}
	if err := proto.Unmarshal(c.frames[1].Payload, scene); err != nil {
		t.Fatal(err)
	}
	if scene.Entities[0].Hp != 0 || scene.Entities[0].Action != 4 {
		t.Fatal("立即基线不含HP0/死亡动作")
	}
	w.step(r, now.Add(100*time.Millisecond))
	if len(c.frames) != 2 {
		t.Fatal("死亡早退后的tick重复发送了致死即时基线")
	}
}

func TestMoveCompletionGatesAttackForPlayerPetMonster(t *testing.T) {
	for _, kind := range []uint32{1, 2, 3} {
		t.Run(map[uint32]string{1: "player", 2: "pet", 3: "monster"}[kind], func(t *testing.T) {
			r, _ := observedRoom()
			w := observedWorker(t, r)
			var from, target *entity
			switch kind {
			case 1:
				from = &r.Player
				target = r.Monsters[0]
			case 2:
				from = r.Pet
				target = r.Monsters[0]
				target.Pos = devconfig.Point{X: 7, Y: 9}
			case 3:
				from = r.Monsters[0]
				target = &r.Player
			}
			now := time.Now()
			stats := w.fighter(r, from)
			end := now.Add(time.Duration(stats.MoveMS) * time.Millisecond)
			from.NextMove = end
			originalAttack := now.Add(-time.Millisecond)
			from.NextAttack = originalAttack
			beforeHP := target.HP
			w.attack(r, from, target, end.Add(-time.Nanosecond))
			if target.HP != beforeHP || from.AttackSequence != 0 || !from.NextAttack.Equal(originalAttack) {
				t.Fatal("移动未结束却伤害/递增攻击序号/改变攻击CD")
			}
			w.attack(r, from, target, end)
			if target.HP >= beforeHP || from.AttackSequence != 1 || !from.NextAttack.Equal(end.Add(time.Duration(stats.AttackMS)*time.Millisecond)) {
				t.Fatal("移动恰好结束时没有按原攻击间隔受理攻击")
			}
		})
	}
}
