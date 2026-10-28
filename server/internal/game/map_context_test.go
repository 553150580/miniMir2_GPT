package game

import (
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func installContextTown(w *worker, blocked ...devconfig.Point) {
	m := devconfig.MapTemplate{ID: 2, SceneKind: devconfig.SceneKindTown, Width: 700, Height: 700, PlayerSpawn: devconfig.Point{X: 633, Y: 613}, CollisionBits: make([]byte, 61250)}
	for _, point := range blocked {
		cell := point.Y*700 + point.X
		m.CollisionBits[cell/8] |= 1 << uint(cell%8)
	}
	w.e.Store.Cfg.Fixture.Map.ID = 1
	w.e.Store.Cfg.Fixture.Maps = []devconfig.MapTemplate{m}
}

func TestRoomMapContextSeparatesTerrainBoundsAndNavigationBudget(t *testing.T) {
	w, personal, _ := stageBWorker(t, 1, 1)
	installContextTown(w, devconfig.Point{X: 9, Y: 8})
	w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{{X: 8, Y: 7}}
	town := &room{MapID: 2, Player: entity{Kind: 1, HP: 10, Pos: devconfig.Point{X: 8, Y: 8}}}
	for _, personalID := range []uint64{0, 1} {
		personal.MapID = personalID
		if w.walkable(personal, devconfig.Point{X: 8, Y: 7}) || !w.walkable(town, devconfig.Point{X: 8, Y: 7}) || !w.walkable(personal, devconfig.Point{X: 9, Y: 8}) || w.walkable(town, devconfig.Point{X: 9, Y: 8}) {
			t.Fatal("相同格子的两张地图阻挡串用，或旧MapID0兼容失效")
		}
		if w.mapCellBudget(personal) != 289 || w.mapCellBudget(town) != 490000 || w.inBounds(personal, devconfig.Point{X: 633, Y: 613}) || !w.inBounds(town, devconfig.Point{X: 699, Y: 699}) {
			t.Fatal("当前图边界或寻路格子预算仍读全局个人地图")
		}
		if !w.lineOfSight(personal, devconfig.Point{X: 8, Y: 8}, devconfig.Point{X: 10, Y: 8}) || w.lineOfSight(town, devconfig.Point{X: 8, Y: 8}, devconfig.Point{X: 10, Y: 8}) {
			t.Fatal("远程阻挡未使用当前图")
		}
		if !w.neighbor(personal, personal.Player.Pos, devconfig.Point{X: 9, Y: 9}, &personal.Player) || w.neighbor(town, town.Player.Pos, devconfig.Point{X: 9, Y: 9}, &town.Player) {
			t.Fatal("斜角规则未使用当前图阻挡")
		}
	}
	town.Player.Pos = devconfig.Point{X: 633, Y: 613}
	w.move(town, &town.Player, devconfig.Point{X: 633, Y: 614}, time.Now(), 600)
	if town.Player.Pos != (devconfig.Point{X: 633, Y: 614}) {
		t.Fatal("来源原坐标未能普通行走，或被裁成17×17")
	}
}

func TestRoomMapContextSkillFlightAreaAndLineUseCurrentTerrain(t *testing.T) {
	for _, mapID := range []uint64{1, 2} {
		w, r, _ := stageBWorker(t, 1, 1, 9, 23)
		installContextTown(w)
		w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{{X: 9, Y: 8}}
		r.MapID, r.Player.Pos, r.Monsters[0].Pos = mapID, devconfig.Point{X: 8, Y: 8}, devconfig.Point{X: 9, Y: 8}
		r.Pet = nil
		// 手造对象只验证同坐标规则隔离，不是城镇刷怪入口或运行态实例。
		beforeMP := r.Player.MP
		code := w.cast(r, 1, r.Monsters[0].Handle, 1, time.Now())
		if mapID == 1 && (code != p.CodeSkillTarget || r.Player.MP != beforeMP) || mapID == 2 && (code != 0 || r.Player.MP >= beforeMP) {
			t.Fatalf("地图%d的火球阻挡受理/资源错误：%d", mapID, code)
		}
		area := w.areaTargets(r, devconfig.Skill{TargetKind: "enemy_ground", AreaRadius: 1}, r.Monsters[0].Pos)
		line := w.lineTargets(r, pendingCast{Skill: devconfig.Skill{Behavior: "hell_fire", LineCells: 4}, TargetPoint: r.Player.Pos, Direction: 2, LineRemaining: 4})
		want := 0
		if mapID == 2 {
			want = 1
		}
		if len(area) != want || len(line) != want {
			t.Fatalf("地图%d范围/直线技能仍读个人阻挡：%d/%d", mapID, len(area), len(line))
		}
	}
}

func TestRoomMapContextGroundIntentAndEntryPreserveSourceCoordinates(t *testing.T) {
	w, r, c := stageBWorker(t, 1, 23)
	installContextTown(w)
	r.MapID, r.Player.Pos = 2, devconfig.Point{X: 650, Y: 631}
	r.Pet, r.Monsters, r.MonsterCount = nil, nil, 0
	move := func(sequence uint32, x int32) {
		w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SMoveIntent, sequence, 3, &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: sequence, Destination: &pb.GridPoint{X: x, Y: 631}})})
	}
	move(1, 699)
	if r.Destination == nil || *r.Destination != (devconfig.Point{X: 699, Y: 631}) {
		t.Fatal("地面导航未保留700图原坐标")
	}
	move(2, 700)
	if c.frames[len(c.frames)-1].Error != p.CodeInvalidArgument || r.Destination.X != 699 {
		t.Fatal("城镇外坐标受理或覆盖原合法导航")
	}
	mp := r.Player.MP
	if code := w.cast(r, 23, 0, 3, time.Now(), &pb.GridPoint{X: 700, Y: 631}); code != p.CodeSkillTarget || r.Player.MP != mp {
		t.Fatal("来源图外技能坐标被受理或扣蓝")
	}
	if code := w.cast(r, 23, 0, 4, time.Now(), &pb.GridPoint{X: 650, Y: 632}); code != 0 || r.Casts[0].TargetPoint != (devconfig.Point{X: 650, Y: 632}) {
		t.Fatal("来源图内技能坐标被当作个人图越界")
	}
	entry := w.enterLineResult(r)
	if entry == nil || entry.RoomId != 2 || entry.SceneKind != 2 || entry.Position.X != 650 || entry.Position.Y != 631 {
		t.Fatal("入场目录身份、类型或原坐标不准确")
	}
	w.baseline(r)
	baseline := &pb.SceneBaselineChunk{}
	if proto.Unmarshal(c.frames[len(c.frames)-1].Payload, baseline) != nil || len(baseline.Entities) != 1 || baseline.Entities[0].Position.X != 650 || baseline.Entities[0].Position.Y != 631 {
		t.Fatal("基线伪造城镇宠物/怪物或裁剪来源坐标")
	}
}

func TestRoomMapContextPushedAndShoulderDashDoNotUseOtherMapTerrain(t *testing.T) {
	now := time.Now()
	for _, mapID := range []uint64{1, 2} {
		w, r, _ := pushedWorker(t)
		installContextTown(w, devconfig.Point{X: 10, Y: 8})
		w.e.Store.Cfg.Fixture.Map.Blocked = nil
		r.MapID, r.Monsters[0].Pos = mapID, devconfig.Point{X: 9, Y: 8}
		out := w.sourcePushed(r, reference(r, r.Monsters[0]), 2, 1, now, displacementSource{})
		if mapID == 1 && out.Steps != 1 || mapID == 2 && (out.Steps != 0 || out.Stop != pushTerrain) {
			t.Fatalf("地图%d推移越过其阻挡或串用另一图：%+v", mapID, out)
		}
		w, r, _ = displacementSkillWorker(t, 27, 0)
		installContextTown(w, devconfig.Point{X: 9, Y: 8})
		w.e.Store.Cfg.Fixture.Map.Blocked = nil
		r.MapID = mapID
		if code := w.castDirected(r, 27, 0, 1, 2, now); code != 0 {
			t.Fatal(code)
		}
		if mapID == 1 && r.Player.Pos.X <= 8 || mapID == 2 && r.Player.Pos != (devconfig.Point{X: 8, Y: 8}) {
			t.Fatalf("地图%d冲撞未使用当前图阻挡", mapID)
		}
	}
	w, r, _ := pushedWorker(t)
	installContextTown(w)
	r.MapID, r.Player.Pos, r.Pet = 2, devconfig.Point{X: 633, Y: 613}, nil
	r.Monsters[0].Pos = devconfig.Point{X: 634, Y: 613}
	if out := w.sourcePushed(r, reference(r, r.Monsters[0]), 2, 1, now, displacementSource{}); out.Steps != 1 || r.Monsters[0].Pos != (devconfig.Point{X: 635, Y: 613}) {
		t.Fatal("推移仍受个人17×17预算或边界约束")
	}
}

func TestRoomMapContextUnknownIDFailsWithoutPrimaryFallback(t *testing.T) {
	w, r, _ := pushedJobWorker(t, 1)
	installContextTown(w)
	r.MapID = math.MaxUint64
	origin, mp, life := r.Player.Pos, r.Player.MP, r.Player.Life
	if _, ok := w.currentMap(r); ok || w.mapCellBudget(r) != 0 || w.walkable(r, origin) || w.inBounds(r, origin) || w.lineOfSight(r, origin, origin) {
		t.Fatal("显式未知地图退回了个人地图")
	}
	if code := w.cast(r, 1, r.Monsters[0].Handle, 1, time.Now()); code != p.CodeSkillStateBlocked || r.Player.MP != mp || len(r.Casts) != 0 {
		t.Fatal("未知地图技能受理或扣费")
	}
	if out := w.sourcePushed(r, reference(r, &r.Player), 2, 1, time.Now(), displacementSource{}); out.Stop != pushInvalid || out.Steps != 0 {
		t.Fatal("未知地图推移回退个人地图")
	}
	if w.commitWalk(r, &r.Player, devconfig.Point{X: 9, Y: 8}, 2, time.Now(), 600) || r.Player.Pos != origin || r.Player.MovementRevision != 0 {
		t.Fatal("未知地图普通移动改变现场或观察水位")
	}
	w.revivePlayer(r)
	if !r.Isolated || r.Player.Life != life || r.Player.Pos != origin {
		t.Fatal("未知地图复活未拒绝或修改了生命现场")
	}
	if entry := w.enterLineResult(r); entry != nil {
		t.Fatal("未知地图发布了伪造的个人图入场")
	}
}

func TestRoomMapContextColdInitializationAndRespawnUseCorrectTemplate(t *testing.T) {
	w, r, _ := stageBWorker(t, 1, 1)
	installContextTown(w)
	fresh := &room{MapID: 2}
	w.initialize(fresh, r.Role)
	if fresh.MapID != 1 || fresh.Player.Pos != w.e.Store.Cfg.Fixture.Map.PlayerSpawn || len(fresh.Monsters) != 8 {
		t.Fatal("冷初始化没有明确创建原个人地图")
	}
	r.MapID, r.Player.HP, r.MonsterCount, r.Pet = 2, 0, 0, nil
	r.Monsters = nil
	w.revivePlayer(r)
	if r.Player.Pos != (devconfig.Point{X: 633, Y: 613}) {
		t.Fatal("当前图复活错误读回个人图出生点")
	}
	// 城镇无怪物生成配置；异常残留的死亡对象不得从个人图偷取重生格。
	r.Auto = false
	r.Monsters = []*entity{{Kind: 3, HP: 0, MaxHP: 10, RespawnAt: time.Now().Add(-time.Second)}}
	w.step(r, time.Now())
	if !r.Isolated || r.Monsters[0].HP != 0 || r.Monsters[0].Life != 0 {
		t.Fatal("无怪槽城镇从个人图生成了怪物")
	}
}
