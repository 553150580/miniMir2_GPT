// 本文件回归冲撞、受推动作经过安全区时的限制与绑定边界。
package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// TestShoulderDashSourceSafeZoneGuardStopsAtNextCellWithoutChangingZeroStepCosts 核验成功前缀应按原技能伤害一次，用诊断「合法受理的零步/成功前缀被提前拒绝」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestShoulderDashSourceSafeZoneGuardStopsAtNextCellWithoutChangingZeroStepCosts(t *testing.T) {
	// test 接收遍历临时结构列表时当前的元素。
	for _, test := range []struct {
		// name 记录名称，使用文本表示。
		name string
		// center 记录中心格，使用点位表示。
		center devconfig.Point
		// radius 记录“radius”，使用数值表示。
		radius int
		// steps 记录步骤集合，使用数值表示。
		steps int
	}{{"first_cell", devconfig.Point{X: 9, Y: 8}, 0, 0}, {"after_prefix", devconfig.Point{X: 10, Y: 8}, 0, 1}, {"inclusive_corner", devconfig.Point{X: 10, Y: 9}, 1, 0}, {"leave_origin", devconfig.Point{X: 8, Y: 8}, 0, 2}} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(test.name, func(t *testing.T) {
			// w 接收位移技能房间执行器供后续步骤使用，r 接收位移技能房间执行器供后续步骤使用，c 接收位移技能房间执行器供后续步骤使用。
			w, r, c := displacementSkillWorker(t, 27, 0)
			// m 接收当前状态地图供后续步骤使用。
			m, _ := w.currentMap(r)
			m.SafeZones = []devconfig.SafeZone{{ID: 700, Center: test.center, Radius: test.radius}}
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			// origin 保存位置，用于当前步骤中的大区或起点，mp 保存当前处理所用的魔法值，targetHP 保存生命值，用于当前步骤中的目标生命值。
			origin, mp, targetHP := r.Player.Pos, r.Player.MP, r.Monsters[0].HP
			// skill 接收技能供后续步骤使用。
			skill, _ := w.e.Store.Cfg.Fixture.Skill(27)
			// code 保存由施法“directed”取得的结果码。
			if code := w.castDirected(r, 27, 0, 1, 2, now); code != 0 {
				t.Fatalf("合法受理的零步/成功前缀被提前拒绝: %d", code)
			}
			if r.Player.Pos != (devconfig.Point{X: origin.X + test.steps, Y: origin.Y}) || r.Player.MP != mp-skill.MPBaseCost || !r.Player.ActionReadyAt.Equal(now.Add(time.Duration(skill.ActionMS)*time.Millisecond)) || !r.NextSpell.Equal(now.Add(time.Duration(skill.GlobalSpellMS)*time.Millisecond)) || !r.SkillCooldowns[27].Equal(now.Add(time.Duration(skill.CooldownBaseMS)*time.Millisecond)) {
				t.Fatal("安全区检查偏离下一格语义，或改变原扣费/独立动作时钟")
			}
			// movement 记录移动，使用实体位移表示。
			var movement *pb.EntityDisplacement
			// frame 接收遍历协议帧集合时当前的元素。
			for _, frame := range c.frames {
				if frame.ID == p.MsgS2CEntityDisplacement {
					// value 保存当前处理所用的值。
					value := &pb.EntityDisplacement{}
					// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
					if err := proto.Unmarshal(frame.Payload, value); err != nil {
						t.Fatal(err)
					}
					if value.EntityHandle == r.Player.Handle {
						movement = value
					}
				}
			}
			if movement == nil || len(movement.Path) != test.steps {
				t.Fatal("安全区停止后未保留真实成功前缀")
			}
			if test.steps == 0 && (movement.Kind != movementDashFailed || r.Monsters[0].HP != targetHP || r.Monsters[0].Pos != (devconfig.Point{X: 9, Y: 8})) {
				t.Fatal("零步先推/伤目标或未发失败观察")
			}
			if test.steps > 0 && r.Monsters[0].HP != targetHP-1 {
				t.Fatal("成功前缀没有按原技能伤害一次")
			}
			if r.PendingBinding != nil || r.Pending {
				t.Fatal("dash被错标成来源walk绑定")
			}
		})
	}
}

// TestSourcePushedCanEnterSafeZoneAndDoesNotPretendThisStageSupportsPushBinding 核对「把dash安全区门禁错误推广到pushed，或替延期动作造walk绑定」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSourcePushedCanEnterSafeZoneAndDoesNotPretendThisStageSupportsPushBinding(t *testing.T) {
	// w 接收安全会话绑定房间执行器供后续步骤使用，r 接收安全会话绑定房间执行器供后续步骤使用，goal 保存由安全会话绑定房间执行器取得的目标格。
	w, r, _, goal := safeBindingWorker(t)
	r.Monsters = nil
	r.MonsterCount = 0
	r.Pet = nil
	// origin 保存位置，用于当前步骤中的大区或起点。
	origin := r.Player.Pos
	// direction 保存由“facing”取得的朝向。
	direction := facing(origin, goal, 0)
	// out 保存由来源被推动位移取得的输出。
	out := w.sourcePushed(r, reference(r, &r.Player), direction, 1, time.Unix(1800000000, 0), displacementSource{})
	if out.Steps != 1 || !out.Postamble || r.Player.Pos != goal || r.PendingBinding != nil || r.Pending || len(w.e.jobs) != 0 {
		t.Fatal("把dash安全区门禁错误推广到pushed，或替延期动作造walk绑定")
	}
}
