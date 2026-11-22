// 本文件回归即时攻击伤害表现使用实际扣血量，并按当前场景和观察条件推送。
package game

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	p "minimir/internal/protocol"
)

// immediateAttackImpacts 返回“immediate”攻击“impacts”供调用方使用。
// t 提供断言失败报告、子用例和清理注册，conn 记录连接，使用已捕获连接表示。
func immediateAttackImpacts(t *testing.T, conn *capturedConnection) []*pb.CombatImpact {
	t.Helper()
	// impacts 记录“impacts”，使用战斗命中影响列表表示。
	var impacts []*pb.CombatImpact
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range conn.frames {
		if frame.ID != p.MsgS2CCombatImpact {
			continue
		}
		// impact 记录命中影响，使用战斗命中影响表示。
		var impact pb.CombatImpact
		// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
		if err := proto.Unmarshal(frame.Payload, &impact); err != nil {
			t.Fatal(err)
		}
		impacts = append(impacts, &impact)
	}
	return impacts
}

// TestImmediatePetAndMonsterImpactsUseOnlyActualHPLoss 核验致死即时攻击应且仅有一次权威命中，用诊断「事件用了估算伤害、错误生命/受击格或错误攻击身份」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestImmediatePetAndMonsterImpactsUseOnlyActualHPLoss(t *testing.T) {
	// tc 接收遍历临时结构列表时当前的元素。
	for _, tc := range []struct {
		// name 记录名称，使用文本表示。
		name string
		// fromKind 记录来源类型，使用数值表示。
		// hitKind 记录命中类型，使用数值表示。
		fromKind, hitKind uint32
		// life 记录生命代次，使用数值表示。
		life uint64
	}{
		{"怪打人", 3, 1, 0},
		{"怪打宠", 3, 2, 8},
		{"宠打怪", 2, 3, 7},
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(tc.name, func(t *testing.T) {
			// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
			w, r, c := stageBWorker(t, 2)
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			// actors 初始化用作行动实体集合的按数值索引的实体映射。
			actors := map[uint32]*entity{1: &r.Player, 2: r.Pet, 3: r.Monsters[0]}
			// from 保存从行动实体集合取出的来源，target 保存从行动实体集合取出的目标。
			from, target := actors[tc.fromKind], actors[tc.hitKind]
			from.Pos, target.HP, target.Life = stepPoint(target.Pos, 4, 1), 3, tc.life
			// stats 保存当前处理所用的统计或属性。
			stats := &r.Monsters[0].Monster.Fighter
			if from == r.Pet {
				stats = &r.Pet.PetRuntime.Fighter
			}
			stats.AttackMin, stats.AttackMax, stats.Accuracy = 50, 50, 100
			// point 保存位置，用于当前步骤中的点位，sequence 保存攻击序号，用于当前步骤中的序号。
			point, sequence := target.Pos, from.AttackSequence
			w.attack(r, from, target, now)
			// impacts 保存由“immediate”攻击“impacts”取得的“impacts”。
			impacts := immediateAttackImpacts(t, c)
			if target.HP != 0 || len(impacts) != 1 {
				t.Fatal("致死即时攻击没有且仅有一次权威命中", target.HP, impacts)
			}
			// hit 保存从“impacts”取出的命中。
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

// TestImmediateImpactWaitsForObservedCurrentRuleAndPositiveDamage 核对「新增观测改变原伤害，或输出未就绪/旧规则/零伤害事件」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestImmediateImpactWaitsForObservedCurrentRuleAndPositiveDamage(t *testing.T) {
	// tc 接收遍历临时结构列表时当前的元素。
	for _, tc := range []struct {
		// name 记录名称，使用文本表示。
		name string
		// rule 记录规则，使用数值表示。
		rule uint64
		// ready 标记是否处于就绪状态。
		ready bool
		// attack 记录攻击，使用数值表示。
		// hp 记录生命值，使用数值表示。
		attack, hp int64
	}{
		{"历史规则", 23, true, 50, 0},
		{"场景未确认", 24, false, 50, 0},
		{"伤害为零", 24, true, 0, 10},
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(tc.name, func(t *testing.T) {
			// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
			w, r, c := stageBWorker(t, 2)
			w.e.Store.Cfg.Fixture.RuleVersion, r.Ready = tc.rule, tc.ready
			// m 保存从怪物集合取出的怪物集合中的指定项。
			m := r.Monsters[0]
			m.Monster.AttackMin, m.Monster.AttackMax, m.Monster.Accuracy = tc.attack, tc.attack, 100
			m.Pos, r.Player.HP = stepPoint(r.Player.Pos, 4, 1), 10
			w.attack(r, m, &r.Player, time.Unix(1800000000, 0))
			// got 保存由“immediate”攻击“impacts”取得的实际结果。
			if got := immediateAttackImpacts(t, c); len(got) != 0 || r.Player.HP != tc.hp {
				t.Fatal("新增观测改变原伤害，或输出未就绪/旧规则/零伤害事件", got, r.Player.HP)
			}
		})
	}
}
