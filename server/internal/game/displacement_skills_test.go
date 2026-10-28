package game

import (
	"compress/gzip"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestSourceDisplacementSkills6400Vectors(t *testing.T) {
	file, err := os.Open("testdata/source-displacement-skills-vectors.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	type actor struct {
		ID       uint32          `json:"id"`
		Position devconfig.Point `json:"position"`
		Level    uint32          `json:"level"`
		HP       int64           `json:"hp"`
		Hostile  bool            `json:"hostile"`
		CanPush  bool            `json:"can_push"`
	}
	var vectors []struct {
		Input struct {
			Kind        string            `json:"kind"`
			Origin      devconfig.Point   `json:"origin"`
			Direction   uint32            `json:"direction"`
			Rank        uint32            `json:"rank"`
			CasterLevel uint32            `json:"caster_level"`
			Chance      int64             `json:"chance"`
			Roll        int64             `json:"distance_roll"`
			Actors      []actor           `json:"actors"`
			Blocked     []devconfig.Point `json:"blocked"`
		} `json:"input"`
		Expected []struct {
			ID          uint32            `json:"id"`
			Position    devconfig.Point   `json:"position"`
			Direction   uint32            `json:"direction"`
			HP          int64             `json:"hp"`
			DamageCalls int               `json:"damage_calls"`
			Damage      int64             `json:"damage"`
			Action      int64             `json:"action_ms"`
			Move        int64             `json:"move_ms"`
			Attack      int64             `json:"attack_ms"`
			Path        []devconfig.Point `json:"path"`
			Failed      bool              `json:"dash_failed"`
		} `json:"expected"`
		Draws []int64 `json:"draws"`
	}
	if err = json.NewDecoder(reader).Decode(&vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 6400 {
		t.Fatal("独立来源向量数量变化，需先审阅输入")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(10000)
	for index, test := range vectors {
		input := test.Input
		configuration := cfg
		configuration.Fixture.Map.Blocked = input.Blocked
		conn := &capturedConnection{}
		r := &room{Scene: 1, Ready: true, Conn: conn, Auth: persistence.Auth{SessionEpoch: 3}, Role: domain.Role{Level: input.CasterLevel}, CellEntrySequence: uint64(len(input.Actors)), worldState: &worldState{}}
		r.Player = entity{Handle: 1, Kind: 1, Pos: input.Origin, Direction: input.Direction, HP: 100, MaxHP: 100, ActionReadyAt: time.UnixMilli(0), NextMove: time.UnixMilli(19000), NextAttack: time.UnixMilli(23000)}
		entities := map[uint32]*entity{1: &r.Player}
		for i, initial := range input.Actors {
			definition := configuration.Fixture.Monsters[0]
			definition.Level = initial.Level
			definition.CanPush = initial.CanPush
			e := &entity{Handle: initial.ID, Kind: 3, Pos: initial.Position, Direction: 4, HP: initial.HP, MaxHP: initial.HP, Monster: &definition, CellEntry: uint64(i + 1),
				ActionReadyAt: time.UnixMilli(17000), NextMove: time.UnixMilli(19000), NextAttack: time.UnixMilli(23000)}
			if !initial.Hostile {
				e.Kind = 2
				r.Pet = e
			} else {
				r.Monsters = append(r.Monsters, e)
			}
			entities[e.Handle] = e
		}
		r.MonsterCount = len(r.Monsters)
		draws := []int64{}
		w := &worker{e: &Engine{Store: &persistence.Store{Cfg: configuration}}, roll: func(n int64) int64 {
			draws = append(draws, n)
			if n == 20 {
				return input.Chance
			}
			if n == 2 {
				return input.Roll
			}
			t.Fatalf("向量%d出现未知随机边界%d", index, n)
			return 0
		}}
		id := uint32(8)
		if input.Kind == "shoulder_dash" {
			id = 27
			r.Role.Job = 0
		} else {
			r.Role.Job = 1
		}
		skill, ok := configuration.Fixture.Skill(id)
		if !ok {
			t.Fatal("本组要求当前Excel已经选入两项位移技能")
		}
		amount := int64(0)
		if id == 27 {
			amount = w.magicAmount(skill, input.Rank, 0)
		}
		w.resolveDisplacementSkill(r, pendingCast{Scene: 1, Skill: skill, SkillLevel: input.Rank, Caster: reference(r, &r.Player), ID: 1, Direction: input.Direction, Amount: amount}, now)
		paths := map[uint32][]devconfig.Point{}
		failed := map[uint32]bool{}
		calls := map[uint32]int{}
		for _, frame := range conn.frames {
			if frame.ID == p.MsgS2CEntityDisplacement {
				var event pb.EntityDisplacement
				if err = proto.Unmarshal(frame.Payload, &event); err != nil {
					t.Fatal(err)
				}
				if event.Kind == movementDashFailed {
					failed[event.EntityHandle] = true
				}
				for _, q := range event.Path {
					paths[event.EntityHandle] = append(paths[event.EntityHandle], devconfig.Point{X: int(q.X), Y: int(q.Y)})
				}
			}
			if frame.ID == p.MsgS2CCombatImpact {
				var impact pb.CombatImpact
				if err = proto.Unmarshal(frame.Payload, &impact); err != nil {
					t.Fatal(err)
				}
				calls[impact.TargetHandle]++
			}
		}
		for _, expected := range test.Expected {
			actual := entities[expected.ID]
			path := paths[expected.ID]
			if actual.Pos != expected.Position || actual.Direction != expected.Direction || actual.HP != expected.HP || actual.ActionReadyAt.UnixMilli() != expected.Action || actual.NextMove.UnixMilli() != expected.Move || actual.NextAttack.UnixMilli() != expected.Attack || failed[expected.ID] != expected.Failed || calls[expected.ID] != expected.DamageCalls || len(path) != len(expected.Path) {
				t.Fatalf("向量%d %s rank%d dir%d：id%d actual=%+v expected=%+v path=%v", index, input.Kind, input.Rank, input.Direction, expected.ID, actual, expected, path)
			}
			for i, point := range path {
				if point != expected.Path[i] {
					t.Fatalf("向量%d路径顺序不符", index)
				}
			}
		}
		if !reflect.DeepEqual(draws, test.Draws) {
			t.Fatalf("向量%d随机请求不符：%v / %v", index, draws, test.Draws)
		}
	}
}

func displacementSkillWorker(t *testing.T, id, rank uint32) (*worker, *room, *capturedConnection) {
	job := uint32(1)
	if id == 27 {
		job = 0
	}
	w, r, c := pushedJobWorker(t, job)
	r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: id, Level: rank})
	r.Pet.HP = 0
	r.MonsterCount = 1
	r.Player.Pos = devconfig.Point{X: 8, Y: 8}
	r.Monsters[0].Pos = devconfig.Point{X: 9, Y: 8}
	w.roll = func(int64) int64 { return 0 }
	return w, r, c
}

func TestDisplacementSkillsAcceptedOnceAndNoDamageFromRepulsion(t *testing.T) {
	now := time.Unix(1800000000, 0)
	for _, id := range []uint32{8, 27} {
		for rank := uint32(0); rank < 4; rank++ {
			w, r, c := displacementSkillWorker(t, id, rank)
			skill, _ := w.e.Store.Cfg.Fixture.Skill(id)
			hp, mp := r.Monsters[0].HP, r.Player.MP
			goal := devconfig.Point{X: 14, Y: 14}
			r.Destination = &goal
			if code := w.castDirected(r, id, 0, 7, 2, now); code != 0 {
				t.Fatal(id, rank, code)
			}
			if r.Player.MP != mp-skill.MPBaseCost-int64(rank)*skill.MPLevelCost || r.CastID != 1 || len(r.Casts) != 0 || r.NextSpell != now.Add(time.Duration(skill.GlobalSpellMS)*time.Millisecond) || r.SkillCooldowns[id] != now.Add(time.Duration(skill.CooldownBaseMS)*time.Millisecond) {
				t.Fatal("位移重复扣费、延迟或未占用独立冷却")
			}
			damage := int64(0)
			if id == 27 {
				damage = int64(rank) + 1
			}
			if r.Monsters[0].HP != hp-damage || r.Destination != &goal {
				t.Fatal("抗拒伤怪、野蛮逐格伤害或清除了地面目标")
			}
			accepted, impacts, baselines := 0, 0, 0
			lastDisplacement := -1
			firstBaseline := -1
			for i, frame := range c.frames {
				switch frame.ID {
				case p.MsgS2CCastAccepted:
					accepted++
				case p.MsgS2CCombatImpact:
					impacts++
				case p.MsgS2CEntityDisplacement:
					lastDisplacement = i
				case p.MsgS2CSceneBaseline:
					baselines++
					if firstBaseline < 0 {
						firstBaseline = i
					}
				}
			}
			expectedImpacts := 0
			if damage > 0 {
				expectedImpacts = 1
			}
			if accepted != 1 || impacts != expectedImpacts || baselines != 1 {
				t.Fatal("受理/单次命中数量不符")
			}
			if firstBaseline < lastDisplacement {
				t.Fatal("嵌套推挤基线先确认冲撞水位，客户端会丢路径")
			}
			after := r.Player.MP
			if w.castDirected(r, id, 0, 8, 2, now) == 0 || r.Player.MP != after || r.CastID != 1 {
				t.Fatal("同次冷却重放仍受理或扣费")
			}
		}
	}
}

func TestDisplacementRejectsInvalidStateBeforeAnyCostOrMovement(t *testing.T) {
	now := time.Unix(1800000000, 0)
	for _, name := range []string{"unlearned", "wrong_job", "dead", "pending", "action", "resource", "foreign_target", "direction"} {
		t.Run(name, func(t *testing.T) {
			w, r, c := displacementSkillWorker(t, 8, 0)
			target, direction := uint32(0), uint32(2)
			switch name {
			case "unlearned":
				r.Role.Skills = nil
			case "wrong_job":
				r.Role.Job = 2
			case "dead":
				r.Player.HP = 0
			case "pending":
				r.Pending = true
			case "action":
				r.Player.ActionReadyAt = now.Add(time.Millisecond)
			case "resource":
				r.Player.MP = 1
			case "foreign_target":
				target = 1
			case "direction":
				direction = 8
			}
			beforeMP, position, revision := r.Player.MP, r.Monsters[0].Pos, r.Monsters[0].MovementRevision
			if w.castDirected(r, 8, target, 1, direction, now) == 0 {
				t.Fatal("非法位移输入仍受理")
			}
			if r.Player.MP != beforeMP || r.CastID != 0 || r.Monsters[0].Pos != position || r.Monsters[0].MovementRevision != revision || len(c.frames) != 0 {
				t.Fatal("受理前失败仍扣费、改位移或发送成功事件")
			}
		})
	}
}

func TestDashZeroStepConsumesOnceWithoutInventedFailureLock(t *testing.T) {
	w, r, c := displacementSkillWorker(t, 27, 3)
	now := time.Unix(1800000000, 0)
	w.e.Store.Cfg.Fixture.Map.Blocked = append(w.e.Store.Cfg.Fixture.Map.Blocked, devconfig.Point{X: 9, Y: 8})
	mp, origin := r.Player.MP, r.Player.Pos
	if code := w.castDirected(r, 27, 0, 1, 2, now); code != 0 {
		t.Fatal(code)
	}
	if r.Player.MP != mp-16 || r.Player.Pos != origin || r.Player.ActionReadyAt != now.Add(600*time.Millisecond) || r.NextSpell != now.Add(1800*time.Millisecond) || r.SkillCooldowns[27] != now.Add(2500*time.Millisecond) {
		t.Fatal("零步失败资源/独立时钟不符，或添加了2秒锁")
	}
	count := 0
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CEntityDisplacement {
			var event pb.EntityDisplacement
			if err := proto.Unmarshal(frame.Payload, &event); err != nil {
				t.Fatal(err)
			}
			if event.Kind != movementDashFailed || len(event.Path) != 0 {
				t.Fatal("失败伪造了成功路径")
			}
			count++
		}
	}
	if count != 1 {
		t.Fatal("零步失败没有唯一权威观察事件")
	}
}

func TestRepulsionFailedChanceStillPaysButNeverDamagesOrConsumesDistanceDraw(t *testing.T) {
	w, r, _ := displacementSkillWorker(t, 8, 0)
	definition := *r.Monsters[0].Monster
	definition.Level = r.Role.Level - 1
	r.Monsters[0].Monster = &definition
	draws := []int64{}
	w.roll = func(n int64) int64 { draws = append(draws, n); return n - 1 }
	now := time.Unix(1800000000, 0)
	mp, hp, position := r.Player.MP, r.Monsters[0].HP, r.Monsters[0].Pos
	if code := w.castDirected(r, 8, 0, 1, 2, now); code != 0 {
		t.Fatal(code)
	}
	if r.Player.MP != mp-2 || r.Monsters[0].HP != hp || r.Monsters[0].Pos != position || !reflect.DeepEqual(draws, []int64{20}) {
		t.Fatal("概率失败仍推进、伤怪或提前抽推距")
	}
}
