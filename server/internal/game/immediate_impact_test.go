package game

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	p "minimir/internal/protocol"
)

func immediateAttackImpacts(t *testing.T, conn *capturedConnection) []*pb.CombatImpact {
	t.Helper()
	var impacts []*pb.CombatImpact
	for _, frame := range conn.frames {
		if frame.ID != p.MsgS2CCombatImpact {
			continue
		}
		var impact pb.CombatImpact
		if err := proto.Unmarshal(frame.Payload, &impact); err != nil {
			t.Fatal(err)
		}
		impacts = append(impacts, &impact)
	}
	return impacts
}

func TestImmediatePetAndMonsterImpactsUseOnlyActualHPLoss(t *testing.T) {
	for _, tc := range []struct {
		name              string
		fromKind, hitKind uint32
		life              uint64
	}{
		{"怪打人", 3, 1, 0},
		{"怪打宠", 3, 2, 8},
		{"宠打怪", 2, 3, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, r, c := stageBWorker(t, 2)
			now := time.Unix(1800000000, 0)
			actors := map[uint32]*entity{1: &r.Player, 2: r.Pet, 3: r.Monsters[0]}
			from, target := actors[tc.fromKind], actors[tc.hitKind]
			from.Pos, target.HP, target.Life = stepPoint(target.Pos, 4, 1), 3, tc.life
			stats := &r.Monsters[0].Monster.Fighter
			if from == r.Pet {
				stats = &r.Pet.PetRuntime.Fighter
			}
			stats.AttackMin, stats.AttackMax, stats.Accuracy = 50, 50, 100
			point, sequence := target.Pos, from.AttackSequence
			w.attack(r, from, target, now)
			impacts := immediateAttackImpacts(t, c)
			if target.HP != 0 || len(impacts) != 1 {
				t.Fatal("致死即时攻击没有且仅有一次权威命中", target.HP, impacts)
			}
			hit := impacts[0]
			if hit.SceneEpoch != r.Scene || hit.EventId == 0 || hit.TargetHandle != target.Handle || hit.TargetLife != tc.life ||
				hit.CasterHandle != from.Handle || !proto.Equal(hit.TargetGrid, pointPB(point)) ||
				hit.Amount != 3 || hit.Outcome != 1 || hit.Healing || hit.SkillId != 0 || hit.CastId != 0 || hit.AttackSequence != sequence+1 {
				t.Fatal("事件用了估算伤害、错误生命/受击格或错误攻击身份", hit)
			}
			w.attack(r, from, target, now.Add(10*time.Second))
			if len(immediateAttackImpacts(t, c)) != 1 {
				t.Fatal("死目标重复发送命中")
			}
		})
	}
}

func TestImmediateImpactWaitsForObservedCurrentRuleAndPositiveDamage(t *testing.T) {
	for _, tc := range []struct {
		name       string
		rule       uint64
		ready      bool
		attack, hp int64
	}{
		{"历史规则", 23, true, 50, 0},
		{"场景未确认", 24, false, 50, 0},
		{"伤害为零", 24, true, 0, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, r, c := stageBWorker(t, 2)
			w.e.Store.Cfg.Fixture.RuleVersion, r.Ready = tc.rule, tc.ready
			m := r.Monsters[0]
			m.Monster.AttackMin, m.Monster.AttackMax, m.Monster.Accuracy = tc.attack, tc.attack, 100
			m.Pos, r.Player.HP = stepPoint(r.Player.Pos, 4, 1), 10
			w.attack(r, m, &r.Player, time.Unix(1800000000, 0))
			if got := immediateAttackImpacts(t, c); len(got) != 0 || r.Player.HP != tc.hp {
				t.Fatal("新增观测改变原伤害，或输出未就绪/旧规则/零伤害事件", got, r.Player.HP)
			}
		})
	}
}
