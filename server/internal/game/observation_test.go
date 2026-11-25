// 本文件回归攻击表现与动作时钟、失败请求互不覆盖，以及死亡即时同步。
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

// observedWorker 返回观察到的房间执行器供调用方使用。
// t 提供断言失败报告、子用例和清理注册，r 指向当前执行器拥有并更新的目标房间。
func observedWorker(t *testing.T, r *room) *worker {
	t.Helper()
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// 本组只验证动作观察/死亡推送；源命中与零伤害另由技能回归覆盖。
	cfg.Fixture.DamageFormula = "development_range_minus_defence"
	// 手造怪沿用原全局池。专属池测试自行给出同一模板和掉落合同。
	cfg.Fixture.OrdinaryDropPools = nil
	for i := range cfg.Fixture.Monsters {
		cfg.Fixture.Monsters[i].DropPoolID = 0
	}
	// m 接收遍历怪物集合时当前的元素。
	for _, m := range r.Monsters {
		if m.Monster == nil {
			m.Monster = &cfg.Fixture.Monsters[0]
		}
	}
	// w 保存当前处理所用的房间执行器。
	w := &worker{e: &Engine{Store: &persistence.Store{Cfg: cfg}}, rooms: map[string]*room{"role": r}}
	if r.Pet != nil && w.petRuntimeEnabled() {
		if r.Pet.Def == 0 {
			r.Pet.Def = cfg.Fixture.Pet.DefinitionID
		}
		r.PetWorld = r.currentWorld().Ref
		if !w.attachLegacyPetState(r.Pet) {
			t.Fatal("手造观察room不能附着合法legacy_untrained宠物定义")
		}
	}
	return w
}

// observedRoom 处理观察到的房间。
func observedRoom() (*room, *capturedConnection) {
	// c 保存当前处理所用的已捕获连接。
	c := &capturedConnection{}
	// r 保存当前处理所用的房间。
	r := &room{Conn: c, Ready: true, Scene: 20, Baseline: 30, Auth: persistence.Auth{SessionEpoch: 3}, Player: entity{Handle: 1, Kind: 1, HP: 100, MaxHP: 100, Pos: devconfig.Point{X: 8, Y: 8}}, Pet: &entity{Handle: 2, Kind: 2, HP: 60, MaxHP: 60, Pos: devconfig.Point{X: 7, Y: 8}}, worldState: &worldState{MonsterCount: 1, Monsters: []*entity{{Handle: 10, Kind: 3, Def: 1, HP: 100, MaxHP: 100, Pos: devconfig.Point{X: 8, Y: 9}}}}}
	return r, c
}

// TestAttackObservationSurvivesStruckAndCooldownReject 核对「同tick受击覆盖了独立攻击观察」与「PB未独立回传被受击覆盖的攻击」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAttackObservationSurvivesStruckAndCooldownReject(t *testing.T) {
	// r 接收观察到的房间供后续步骤使用，c 接收观察到的房间供后续步骤使用。
	r, c := observedRoom()
	// w 接收观察到的房间执行器供后续步骤使用。
	w := observedWorker(t, r)
	// now 记录本次处理开始时的时间。
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
	// scene 保存当前处理所用的场景。
	scene := &pb.SceneBaselineChunk{}
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := proto.Unmarshal(c.frames[len(c.frames)-1].Payload, scene); err != nil {
		t.Fatal(err)
	}
	// e 接收遍历实体时当前的元素。
	for _, e := range scene.Entities {
		// expected 保存从按数值索引的数值映射取出的预期结果。
		expected := map[uint32]uint32{1: 600, 2: 1000, 3: 1500}[e.EntityKind]
		if e.MoveIntervalMs != expected {
			t.Fatalf("kind%d移动间隔=%d，预期%d", e.EntityKind, e.MoveIntervalMs, expected)
		}
		if e.EntityKind == 1 && (e.AttackSequence != 1 || e.Action != 3) {
			t.Fatal("PB未独立回传被受击覆盖的攻击")
		}
	}
	// interval 保存攻击毫秒，用于当前步骤中的周期。
	interval := w.e.Store.Cfg.Fixture.Players[0].AttackMS
	w.attack(r, &r.Player, r.Monsters[0], now.Add(time.Duration(interval)*time.Millisecond))
	if r.Player.AttackSequence != 2 {
		t.Fatal("冷却完成后的新攻击未递增序号")
	}
}

// TestLethalStatePushesBeforeBroadcastWindow 核验同一立即终态应归档原宠物尸体，以及广播窗口前致死应立即推终态。
// t 提供断言失败报告、子用例和清理注册。
func TestLethalStatePushesBeforeBroadcastWindow(t *testing.T) {
	// r 接收观察到的房间供后续步骤使用，c 接收观察到的房间供后续步骤使用。
	r, c := observedRoom()
	// w 接收观察到的房间执行器供后续步骤使用。
	w := observedWorker(t, r)
	r.Player.HP = 1
	// now 记录本次处理开始时的时间。
	now := time.Now()
	r.LastBroadcast = now.Add(-50 * time.Millisecond)
	w.attack(r, r.Monsters[0], &r.Player, now)
	if r.Player.HP != 0 || r.Player.Action != 4 || r.DeathID == "" {
		t.Fatal("攻击没有形成真实死亡状态")
	}
	if len(c.frames) != 3 || c.frames[0].ID != p.MsgS2CDeathNotice || c.frames[1].ID != p.MsgS2CSceneBaseline ||
		c.frames[2].ID != p.MsgS2CCombatImpact {
		t.Fatalf("广播窗口前致死没有立即推终态:%v", c.frames)
	}
	// scene 保存当前处理所用的场景。
	scene := &pb.SceneBaselineChunk{}
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := proto.Unmarshal(c.frames[1].Payload, scene); err != nil {
		t.Fatal(err)
	}
	if scene.Entities[0].Hp != 0 || scene.Entities[0].Action != 4 {
		t.Fatal("立即基线不含HP0/死亡动作")
	}
	// impact 记录命中影响，使用战斗命中影响表示。
	var impact pb.CombatImpact
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := proto.Unmarshal(c.frames[2].Payload, &impact); err != nil || impact.TargetHandle != r.Player.Handle ||
		impact.TargetLife != r.Player.Life || impact.Amount != 1 || impact.Outcome != 1 {
		t.Fatal("立即死亡基线之后缺少同生命的真实扣血事件", &impact, err)
	}
	if w.sourcePetLifecycle() {
		if r.Pet != nil || scene.OwnedPet != nil {
			t.Fatal("主人死亡后仍有活动宠物/owner观察")
		}
		// corpse 保存当前处理所用的尸体。
		corpse := false
		// e 接收遍历实体时当前的元素。
		for _, e := range scene.Entities {
			if e.EntityKind == 2 && e.IsPetCorpse && e.Hp == 0 {
				corpse = true
			}
		}
		if !corpse {
			t.Fatal("同一立即终态没有归档原宠物尸体")
		}
	}
	w.step(r, now.Add(100*time.Millisecond))
	if len(c.frames) != 3 {
		t.Fatal("死亡早退后的tick重复发送了致死即时基线")
	}
}

// TestMoveCompletionGatesAttackForPlayerPetMonster 核验移动恰好结束时应按原攻击间隔受理攻击，用诊断「移动未结束却伤害/递增攻击序号/改变攻击CD」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestMoveCompletionGatesAttackForPlayerPetMonster(t *testing.T) {
	// kind 接收遍历数值列表时当前的元素。
	for _, kind := range []uint32{1, 2, 3} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(map[uint32]string{1: "player", 2: "pet", 3: "monster"}[kind], func(t *testing.T) {
			// r 接收观察到的房间供后续步骤使用。
			r, _ := observedRoom()
			// w 接收观察到的房间执行器供后续步骤使用。
			w := observedWorker(t, r)
			// from 记录来源，使用实体表示，target 记录目标，使用实体表示。
			var from, target *entity
			switch kind {
			case 1:
				from = &r.Player
				target = r.Monsters[0]
			case 2:
				from = r.Pet
				target = r.Monsters[0]
			case 3:
				from = r.Monsters[0]
				target = &r.Player
			}
			// now 记录本次处理开始时的时间。
			now := time.Now()
			// stats 保存由战斗单位取得的统计或属性。
			stats := w.fighter(r, from)
			// end 保存由新增取得的结束位置或时间。
			end := now.Add(time.Duration(stats.MoveMS) * time.Millisecond)
			// originalAttack 保存由新增取得的原始攻击。
			originalAttack := now.Add(-time.Millisecond)
			from.NextAttack = originalAttack
			// destination 保存从按数值索引的点位映射取出的目的地。
			destination := map[uint32]devconfig.Point{1: {X: 9, Y: 8}, 2: {X: 7, Y: 9}, 3: {X: 9, Y: 9}}[kind]
			w.move(r, from, destination, now, stats.MoveMS)
			if from.Pos != destination || from.NextMove != end {
				t.Fatal("普通行走前态未真正建立")
			}
			if kind != 1 && from.NextAttack != end {
				t.Fatal("怪物/宠物普通行走未推进原攻击门禁")
			}
			// attackAfterWalk 保存下一状态攻击，用于当前步骤中的攻击操作后的状态行走。
			attackAfterWalk := from.NextAttack
			// beforeHP 保存生命值，用于当前步骤中的操作前的状态生命值。
			beforeHP := target.HP
			w.attack(r, from, target, end.Add(-time.Nanosecond))
			if target.HP != beforeHP || from.AttackSequence != 0 || !from.NextAttack.Equal(attackAfterWalk) {
				t.Fatal("移动未结束却伤害/递增攻击序号/改变攻击CD")
			}
			w.attack(r, from, target, end)
			if target.HP >= beforeHP || from.AttackSequence != 1 || !from.NextAttack.Equal(end.Add(time.Duration(stats.AttackMS)*time.Millisecond)) {
				t.Fatal("移动恰好结束时没有按原攻击间隔受理攻击")
			}
		})
	}
}
