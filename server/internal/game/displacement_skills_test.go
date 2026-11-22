// 本文件通过来源向量和场景样例回归推人、冲撞的距离、耗费、零步和非法目标行为。
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

// TestSourceDisplacementSkills6400Vectors 核对「本组要求当前Excel已经选入两项位移技能」与「独立来源向量数量变化，需先审阅输入」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSourceDisplacementSkills6400Vectors(t *testing.T) {
	// file 保存由打开取得的文件，err 接收打开过程中可能产生的错误，供后续失败分支处理。
	file, err := os.Open("testdata/source-displacement-skills-vectors.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	// reader 保存由创建“reader”取得的“reader”。
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	// actor 保存行动实体所需的状态。
	type actor struct {
		// ID 区分当前步骤指向的目标对象。
		ID uint32 `json:"id"`
		// Position 记录位置，使用点位表示。
		Position devconfig.Point `json:"position"`
		// Level 记录等级，使用数值表示。
		Level uint32 `json:"level"`
		// HP 记录生命值，使用数值表示。
		HP int64 `json:"hp"`
		// Hostile 标记是否处于“hostile”状态。
		Hostile bool `json:"hostile"`
		// CanPush 标记是否处于“can”推入状态。
		CanPush bool `json:"can_push"`
	}
	// vectors 记录测试向量集合，使用临时结构列表表示。
	var vectors []struct {
		// Input 记录输入，使用临时结构表示。
		Input struct {
			// Kind 记录类型，使用文本表示。
			Kind string `json:"kind"`
			// Origin 记录大区或起点，使用点位表示。
			Origin devconfig.Point `json:"origin"`
			// Direction 记录朝向，使用数值表示。
			Direction uint32 `json:"direction"`
			// Rank 记录排序位次，使用数值表示。
			Rank uint32 `json:"rank"`
			// CasterLevel 记录施法者等级，使用数值表示。
			CasterLevel uint32 `json:"caster_level"`
			// Chance 记录“chance”，使用数值表示。
			Chance int64 `json:"chance"`
			// Roll 记录随机值，使用数值表示。
			Roll int64 `json:"distance_roll"`
			// Actors 记录行动实体集合，使用行动实体列表表示。
			Actors []actor `json:"actors"`
			// Blocked 记录阻挡，使用点位列表表示。
			Blocked []devconfig.Point `json:"blocked"`
		} `json:"input"`
		// Expected 记录预期结果，使用临时结构列表表示。
		Expected []struct {
			// ID 区分当前步骤指向的目标对象。
			ID uint32 `json:"id"`
			// Position 记录位置，使用点位表示。
			Position devconfig.Point `json:"position"`
			// Direction 记录朝向，使用数值表示。
			Direction uint32 `json:"direction"`
			// HP 记录生命值，使用数值表示。
			HP int64 `json:"hp"`
			// DamageCalls 记录伤害调用集合，使用数值表示。
			DamageCalls int `json:"damage_calls"`
			// Damage 记录伤害，使用数值表示。
			Damage int64 `json:"damage"`
			// Action 记录动作，使用数值表示。
			Action int64 `json:"action_ms"`
			// Move 记录移动，使用数值表示。
			Move int64 `json:"move_ms"`
			// Attack 记录攻击，使用数值表示。
			Attack int64 `json:"attack_ms"`
			// Path 记录路径，使用点位列表表示。
			Path []devconfig.Point `json:"path"`
			// Failed 标记是否处于已失败状态。
			Failed bool `json:"dash_failed"`
		} `json:"expected"`
		// Draws 记录“draws”，使用数值列表表示。
		Draws []int64 `json:"draws"`
	}
	if err = json.NewDecoder(reader).Decode(&vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 6400 {
		t.Fatal("独立来源向量数量变化，需先审阅输入")
	}
	// cfg 保存由加载取得的服务配置。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// 来源oracle的坐标与边界按17格生成，保持其固定测试几何。
	cfg.Fixture.Map.Width, cfg.Fixture.Map.Height = 17, 17
	// now 保存由Unix 时间“milli”取得的当前时间。
	now := time.UnixMilli(10000)
	// index 定位测试向量集合中正在处理的成员，test 接收遍历测试向量集合时当前的元素。
	for index, test := range vectors {
		// input 保存当前处理所用的输入。
		input := test.Input
		// configuration 保存当前处理所用的“configuration”。
		configuration := cfg
		configuration.Fixture.Map.Blocked = input.Blocked
		// conn 保存当前处理所用的连接。
		conn := &capturedConnection{}
		// r 保存当前处理所用的房间。
		r := &room{Scene: 1, Ready: true, Conn: conn, Auth: persistence.Auth{SessionEpoch: 3}, Role: domain.Role{Level: input.CasterLevel}, CellEntrySequence: uint64(len(input.Actors)), worldState: &worldState{}}
		r.Player = entity{Handle: 1, Kind: 1, Pos: input.Origin, Direction: input.Direction, HP: 100, MaxHP: 100, ActionReadyAt: time.UnixMilli(0), NextMove: time.UnixMilli(19000), NextAttack: time.UnixMilli(23000)}
		// entities 建立带初始值的按数值索引的实体映射，供本段继续处理。
		entities := map[uint32]*entity{1: &r.Player}
		// i 定位行动实体集合中正在处理的成员，initial 接收遍历行动实体集合时当前的元素。
		for i, initial := range input.Actors {
			// definition 保存从怪物集合取出的定义。
			definition := configuration.Fixture.Monsters[0]
			definition.Level = initial.Level
			definition.CanPush = initial.CanPush
			// e 保存当前处理所用的实体。
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
		// draws 建立带初始值的数值列表，供本段继续处理。
		draws := []int64{}
		// w 保存当前处理所用的房间执行器，n 记录数值。
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
		// id 区分当前步骤指向的目标对象。
		id := uint32(8)
		if input.Kind == "shoulder_dash" {
			id = 27
			r.Role.Job = 0
		} else {
			r.Role.Job = 1
		}
		// skill 接收技能供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
		skill, ok := configuration.Fixture.Skill(id)
		if !ok {
			t.Fatal("本组要求当前Excel已经选入两项位移技能")
		}
		// amount 保存由整数“64”取得的数量。
		amount := int64(0)
		if id == 27 {
			amount = w.magicAmount(skill, input.Rank, 0)
		}
		w.resolveDisplacementSkill(r, pendingCast{Scene: 1, Skill: skill, SkillLevel: input.Rank, Caster: reference(r, &r.Player), ID: 1, Direction: input.Direction, Amount: amount}, now)
		// paths 初始化用作路径集合的按数值索引的点位列表映射。
		paths := map[uint32][]devconfig.Point{}
		// failed 初始化用作已失败的按数值索引的布尔状态映射。
		failed := map[uint32]bool{}
		// calls 初始化用作调用集合的按数值索引的数值映射。
		calls := map[uint32]int{}
		// frame 接收遍历协议帧集合时当前的元素。
		for _, frame := range conn.frames {
			if frame.ID == p.MsgS2CEntityDisplacement {
				// event 记录事件，使用实体位移表示。
				var event pb.EntityDisplacement
				if err = proto.Unmarshal(frame.Payload, &event); err != nil {
					t.Fatal(err)
				}
				if event.Kind == movementDashFailed {
					failed[event.EntityHandle] = true
				}
				// q 接收遍历路径时当前的元素。
				for _, q := range event.Path {
					paths[event.EntityHandle] = append(paths[event.EntityHandle], devconfig.Point{X: int(q.X), Y: int(q.Y)})
				}
			}
			if frame.ID == p.MsgS2CCombatImpact {
				// impact 记录命中影响，使用战斗命中影响表示。
				var impact pb.CombatImpact
				if err = proto.Unmarshal(frame.Payload, &impact); err != nil {
					t.Fatal(err)
				}
				calls[impact.TargetHandle]++
			}
		}
		// expected 接收遍历预期结果时当前的元素。
		for _, expected := range test.Expected {
			// actual 保存从实体取出的实测。
			actual := entities[expected.ID]
			// path 保存从路径集合取出的路径。
			path := paths[expected.ID]
			if actual.Pos != expected.Position || actual.Direction != expected.Direction || actual.HP != expected.HP || actual.ActionReadyAt.UnixMilli() != expected.Action || actual.NextMove.UnixMilli() != expected.Move || actual.NextAttack.UnixMilli() != expected.Attack || failed[expected.ID] != expected.Failed || calls[expected.ID] != expected.DamageCalls || len(path) != len(expected.Path) {
				t.Fatalf("向量%d %s rank%d dir%d：id%d actual=%+v expected=%+v path=%v", index, input.Kind, input.Rank, input.Direction, expected.ID, actual, expected, path)
			}
			// i 定位路径中正在处理的成员，point 接收遍历路径时当前的元素。
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

// displacementSkillWorker 处理位移技能房间执行器。
// t 提供断言失败报告、子用例和清理注册，id 区分当前步骤指向的目标对象，rank 记录排序位次，使用数值表示。
func displacementSkillWorker(t *testing.T, id, rank uint32) (*worker, *room, *capturedConnection) {
	// job 保存由无符号整数“32”取得的任务或职业编号。
	job := uint32(1)
	if id == 27 {
		job = 0
	}
	// w 接收被推动位移任务或职业编号房间执行器供后续步骤使用，r 接收被推动位移任务或职业编号房间执行器供后续步骤使用，c 接收被推动位移任务或职业编号房间执行器供后续步骤使用。
	w, r, c := pushedJobWorker(t, job)
	r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: id, Level: rank})
	r.Pet.HP = 0
	r.MonsterCount = 1
	r.Player.Pos = devconfig.Point{X: 8, Y: 8}
	r.Monsters[0].Pos = devconfig.Point{X: 9, Y: 8}
	w.roll = func(int64) int64 { return 0 }
	return w, r, c
}

// TestDisplacementSkillsAcceptedOnceAndNoDamageFromRepulsion 核对「嵌套推挤基线先确认冲撞水位，客户端会丢路径」与「抗拒伤怪、野蛮逐格伤害或清除了地面目标」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDisplacementSkillsAcceptedOnceAndNoDamageFromRepulsion(t *testing.T) {
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// id 接收遍历数值列表时当前的元素。
	for _, id := range []uint32{8, 27} {
		// rank 保存由无符号整数“32”取得的排序位次。
		for rank := uint32(0); rank < 4; rank++ {
			// w 接收位移技能房间执行器供后续步骤使用，r 接收位移技能房间执行器供后续步骤使用，c 接收位移技能房间执行器供后续步骤使用。
			w, r, c := displacementSkillWorker(t, id, rank)
			// skill 接收技能供后续步骤使用。
			skill, _ := w.e.Store.Cfg.Fixture.Skill(id)
			// hp 保存当前处理所用的生命值，mp 保存当前处理所用的魔法值。
			hp, mp := r.Monsters[0].HP, r.Player.MP
			// goal 建立带初始值的点位，供本段继续处理。
			goal := devconfig.Point{X: 14, Y: 14}
			r.Destination = &goal
			// code 保存由施法“directed”取得的结果码。
			if code := w.castDirected(r, id, 0, 7, 2, now); code != 0 {
				t.Fatal(id, rank, code)
			}
			if r.Player.MP != mp-skill.MPBaseCost-int64(rank)*skill.MPLevelCost || r.CastID != 1 || len(r.Casts) != 0 || r.NextSpell != now.Add(time.Duration(skill.GlobalSpellMS)*time.Millisecond) || r.SkillCooldowns[id] != now.Add(time.Duration(skill.CooldownBaseMS)*time.Millisecond) {
				t.Fatal("位移重复扣费、延迟或未占用独立冷却")
			}
			// damage 保存由整数“64”取得的伤害。
			damage := int64(0)
			if id == 27 {
				damage = int64(rank) + 1
			}
			if r.Monsters[0].HP != hp-damage || r.Destination != &goal {
				t.Fatal("抗拒伤怪、野蛮逐格伤害或清除了地面目标")
			}
			// accepted 规定已受理的初始取值，impacts 规定“impacts”的初始取值，baselines 规定场景基线集合的初始取值。
			accepted, impacts, baselines := 0, 0, 0
			// lastDisplacement 保存当前处理所用的上一次状态位移。
			lastDisplacement := -1
			// firstBaseline 保存当前处理所用的第一项场景基线。
			firstBaseline := -1
			// i 定位协议帧集合中正在处理的成员，frame 接收遍历协议帧集合时当前的元素。
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
			// expectedImpacts 规定预期结果“impacts”的初始取值。
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
			// after 保存魔法值，用于当前步骤中的操作后的状态。
			after := r.Player.MP
			if w.castDirected(r, id, 0, 8, 2, now) == 0 || r.Player.MP != after || r.CastID != 1 {
				t.Fatal("同次冷却重放仍受理或扣费")
			}
		}
	}
}

// TestDisplacementRejectsInvalidStateBeforeAnyCostOrMovement 核对「受理前失败仍扣费、改位移或发送成功事件」与「非法位移输入仍受理」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDisplacementRejectsInvalidStateBeforeAnyCostOrMovement(t *testing.T) {
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// name 接收遍历文本列表时当前的元素。
	for _, name := range []string{"unlearned", "wrong_job", "dead", "pending", "action", "resource", "foreign_target", "direction"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// w 接收位移技能房间执行器供后续步骤使用，r 接收位移技能房间执行器供后续步骤使用，c 接收位移技能房间执行器供后续步骤使用。
			w, r, c := displacementSkillWorker(t, 8, 0)
			// target 保存由无符号整数“32”取得的目标，direction 保存由无符号整数“32”取得的朝向。
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
			// beforeMP 保存魔法值，用于当前步骤中的操作前的状态魔法值，position 保存当前处理所用的位置，revision 保存移动修订号，用于当前步骤中的修订号。
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

// TestDashZeroStepConsumesOnceWithoutInventedFailureLock 核验零步失败应唯一权威观察事件，用诊断「零步失败资源/独立时钟不符，或添加了2秒锁」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDashZeroStepConsumesOnceWithoutInventedFailureLock(t *testing.T) {
	// w 接收位移技能房间执行器供后续步骤使用，r 接收位移技能房间执行器供后续步骤使用，c 接收位移技能房间执行器供后续步骤使用。
	w, r, c := displacementSkillWorker(t, 27, 3)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	w.e.Store.Cfg.Fixture.Map.Blocked = append(w.e.Store.Cfg.Fixture.Map.Blocked, devconfig.Point{X: 9, Y: 8})
	// mp 保存当前处理所用的魔法值，origin 保存位置，用于当前步骤中的大区或起点。
	mp, origin := r.Player.MP, r.Player.Pos
	// code 保存由施法“directed”取得的结果码。
	if code := w.castDirected(r, 27, 0, 1, 2, now); code != 0 {
		t.Fatal(code)
	}
	if r.Player.MP != mp-16 || r.Player.Pos != origin || r.Player.ActionReadyAt != now.Add(600*time.Millisecond) || r.NextSpell != now.Add(1800*time.Millisecond) || r.SkillCooldowns[27] != now.Add(2500*time.Millisecond) {
		t.Fatal("零步失败资源/独立时钟不符，或添加了2秒锁")
	}
	// count 规定数量的初始取值。
	count := 0
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CEntityDisplacement {
			// event 记录事件，使用实体位移表示。
			var event pb.EntityDisplacement
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
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

// TestRepulsionFailedChanceStillPaysButNeverDamagesOrConsumesDistanceDraw 核对「概率失败仍推进、伤怪或提前抽推距」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestRepulsionFailedChanceStillPaysButNeverDamagesOrConsumesDistanceDraw(t *testing.T) {
	// w 接收位移技能房间执行器供后续步骤使用，r 接收位移技能房间执行器供后续步骤使用。
	w, r, _ := displacementSkillWorker(t, 8, 0)
	// definition 保存当前处理所用的定义。
	definition := *r.Monsters[0].Monster
	definition.Level = r.Role.Level - 1
	r.Monsters[0].Monster = &definition
	// draws 建立带初始值的数值列表，供本段继续处理。
	draws := []int64{}
	// n 记录数值。
	w.roll = func(n int64) int64 { draws = append(draws, n); return n - 1 }
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// mp 保存当前处理所用的魔法值，hp 保存当前处理所用的生命值，position 保存当前处理所用的位置。
	mp, hp, position := r.Player.MP, r.Monsters[0].HP, r.Monsters[0].Pos
	// code 保存由施法“directed”取得的结果码。
	if code := w.castDirected(r, 8, 0, 1, 2, now); code != 0 {
		t.Fatal(code)
	}
	if r.Player.MP != mp-2 || r.Monsters[0].HP != hp || r.Monsters[0].Pos != position || !reflect.DeepEqual(draws, []int64{20}) {
		t.Fatal("概率失败仍推进、伤怪或提前抽推距")
	}
}
