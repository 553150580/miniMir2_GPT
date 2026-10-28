package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

func TestShoulderDashSourceSafeZoneGuardStopsAtNextCellWithoutChangingZeroStepCosts(t *testing.T) {
	for _, test := range []struct {
		name   string
		center devconfig.Point
		radius int
		steps  int
	}{{"first_cell", devconfig.Point{X: 9, Y: 8}, 0, 0}, {"after_prefix", devconfig.Point{X: 10, Y: 8}, 0, 1}, {"inclusive_corner", devconfig.Point{X: 10, Y: 9}, 1, 0}, {"leave_origin", devconfig.Point{X: 8, Y: 8}, 0, 2}} {
		t.Run(test.name, func(t *testing.T) {
			w, r, c := displacementSkillWorker(t, 27, 0)
			m, _ := w.currentMap(r)
			m.SafeZones = []devconfig.SafeZone{{ID: 700, Center: test.center, Radius: test.radius}}
			now := time.Unix(1800000000, 0)
			origin, mp, targetHP := r.Player.Pos, r.Player.MP, r.Monsters[0].HP
			skill, _ := w.e.Store.Cfg.Fixture.Skill(27)
			if code := w.castDirected(r, 27, 0, 1, 2, now); code != 0 {
				t.Fatalf("合法受理的零步/成功前缀被提前拒绝: %d", code)
			}
			if r.Player.Pos != (devconfig.Point{X: origin.X + test.steps, Y: origin.Y}) || r.Player.MP != mp-skill.MPBaseCost || !r.Player.ActionReadyAt.Equal(now.Add(time.Duration(skill.ActionMS)*time.Millisecond)) || !r.NextSpell.Equal(now.Add(time.Duration(skill.GlobalSpellMS)*time.Millisecond)) || !r.SkillCooldowns[27].Equal(now.Add(time.Duration(skill.CooldownBaseMS)*time.Millisecond)) {
				t.Fatal("安全区检查偏离下一格语义，或改变原扣费/独立动作时钟")
			}
			var movement *pb.EntityDisplacement
			for _, frame := range c.frames {
				if frame.ID == p.MsgS2CEntityDisplacement {
					value := &pb.EntityDisplacement{}
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

func TestSourcePushedCanEnterSafeZoneAndDoesNotPretendThisStageSupportsPushBinding(t *testing.T) {
	w, r, _, goal := safeBindingWorker(t)
	r.Monsters = nil
	r.MonsterCount = 0
	r.Pet = nil
	origin := r.Player.Pos
	direction := facing(origin, goal, 0)
	out := w.sourcePushed(r, reference(r, &r.Player), direction, 1, time.Unix(1800000000, 0), displacementSource{})
	if out.Steps != 1 || !out.Postamble || r.Player.Pos != goal || r.PendingBinding != nil || r.Pending || len(w.e.jobs) != 0 {
		t.Fatal("把dash安全区门禁错误推广到pushed，或替延期动作造walk绑定")
	}
}
