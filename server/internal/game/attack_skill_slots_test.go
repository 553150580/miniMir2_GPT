// 本文件回归攻击技能槽的时钟、精确目标、原战士刀法和继承规则。
package game

import (
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	p "minimir/internal/protocol"
)

// TestAttackSkillSlotsSortsReadyTimes 验证就绪时间早者优先，同一截止按保存槽位选取。
func TestAttackSkillSlotsSortsReadyTimes(t *testing.T) {
	now := time.Unix(1800000000, 0)
	for _, test := range []struct {
		name          string
		first, second time.Duration
		want          uint32
	}{
		{"第二槽较早就绪", -time.Second, -2 * time.Second, 11},
		{"两槽时间相同", -time.Second, -time.Second, 1},
		{"首槽尚未到期", time.Nanosecond, 0, 11},
	} {
		t.Run(test.name, func(t *testing.T) {
			w, r, _ := stageBWorker(t, 1, 1, 11)
			r.Role.SkillSlotIDs = []uint32{1, 0, 11, 0}
			r.SkillCooldowns = map[uint32]time.Time{1: now.Add(test.first), 11: now.Add(test.second)}
			if !w.autoSkill(r, now) || len(r.Casts) != 1 || r.Casts[0].Skill.ID != test.want || r.Player.CastSequence != 1 {
				t.Fatal("未按就绪时间和槽位只受理一个原技能", r.Casts)
			}
		})
	}
}

// TestAttackSkillSlotsExactSelectedLife 核验初始零代次、错误场景和重生目标拒绝。
func TestAttackSkillSlotsExactSelectedLife(t *testing.T) {
	w, r, conn := stageBWorker(t, 1, 1, 11)
	now := time.Unix(1800000000, 0)
	monsters := prepareAutoTargetTest(w, r, now, stepPoint(r.Player.Pos, 4, 1), stepPoint(r.Player.Pos, 2, 2))
	focus, other := monsters[0], monsters[1]
	locked := reference(r, other)
	r.AutoTarget = locked
	r.Role.SkillSlotIDs = []uint32{11, 0, 0, 0}
	request := &pb.AttackIntent{SceneEpoch: r.Scene, Direction: 2, TargetHandle: focus.Handle, TargetLife: focus.Life + 1, InputSequence: 9}
	if code := w.attackFromConfiguredSlots(r, request, now); code != p.CodeSkillTarget || r.Player.AttackSequence != 0 || r.Player.CastSequence != 0 {
		t.Fatalf("旧生命被换成其他活怪，错误码%d", code)
	}
	request.TargetLife = focus.Life
	request.SceneEpoch++
	if code := w.attackFromConfiguredSlots(r, request, now); code != p.CodeInvalidArgument || r.Player.CastSequence != 0 {
		t.Fatalf("其他场景影响当前攻击，错误码%d", code)
	}
	request.SceneEpoch = r.Scene
	if code := w.attackFromConfiguredSlots(r, request, now); code != 0 || len(r.Casts) != 1 || r.Casts[0].Target != focus.Handle ||
		r.Casts[0].TargetLife != 0 || r.AutoTarget != locked {
		t.Fatalf("普通怪零代次失效或手动攻击覆盖原自动锁，错误码%d，事件%+v", code, r.Casts)
	}
	for _, frame := range conn.frames {
		if frame.ID == p.MsgS2CCastAccepted {
			var accepted pb.CastAccepted
			if err := proto.Unmarshal(frame.Payload, &accepted); err != nil || accepted.InputSequence != 9 || accepted.TargetHandle != focus.Handle {
				t.Fatal("队列释放没有回传原手动输入和目标", err, &accepted)
			}
			return
		}
	}
	t.Fatal("客户端未取得原施法受理")
}

// TestAttackSkillSlotsPlainAndLegacyDefaults 验证空槽、法术缺蓝与历史手动模式是独立语义。
func TestAttackSkillSlotsPlainAndLegacyDefaults(t *testing.T) {
	now := time.Unix(1800000000, 0)
	w, r, _ := stageBWorker(t, 1, 1, 11)
	fixture := w.e.Store.Cfg.Fixture
	if got := attackSlotsForRole(r.Role, fixture); !slices.Equal(got, []uint32{11, 1, 0, 0}) {
		t.Fatalf("历史缺省未按原优先级映射为四个槽：%v", got)
	}
	r.Role.SkillSlotIDs = []uint32{11, 0, 0, 0}
	r.Player.MP = 0
	focus := r.Monsters[0]
	request := &pb.AttackIntent{SceneEpoch: r.Scene, TargetHandle: focus.Handle, TargetLife: focus.Life, InputSequence: 1}
	if code := w.attackFromConfiguredSlots(r, request, now); code != 0 || r.Player.AttackSequence != 1 || r.Player.AttackSkillID != 0 || r.Player.CastSequence != 0 || r.Player.MP != 0 {
		t.Fatalf("法术资源不足未回退到原纯普攻，错误码%d", code)
	}
	w, r, _ = stageBWorker(t, 0, 25)
	r.Melee.HalfMoon = true
	request = &pb.AttackIntent{SceneEpoch: r.Scene, Direction: 4, InputSequence: 1}
	if code := w.attackFromConfiguredSlots(r, request, now); code != 0 || r.Player.AttackSkillID != 25 {
		t.Fatalf("历史缺省丢失原半月开关，错误码%d", code)
	}
	clearAction(r)
	r.Player.NextAttack = time.Time{}
	r.Role.SkillSlotIDs = []uint32{0, 0, 0, 0}
	if got := attackSlotsForRole(r.Role, w.e.Store.Cfg.Fixture); !slices.Equal(got, r.Role.SkillSlotIDs) {
		t.Fatalf("四个显式空槽被填入缺省技能：%v", got)
	}
	if code := w.attackFromConfiguredSlots(r, request, now); code != 0 || r.Player.AttackSkillID != 0 || !r.Melee.HalfMoon || r.Player.MP != r.Player.MaxMP {
		t.Fatalf("显式空槽泄漏旧模式或额外扣蓝，错误码%d", code)
	}
}

// TestAttackSkillSlotsWarriorMeleeUsesSourceCosts 验证半月跳过二格、刺杀原二格伤害和烈火原扣蓝时机。
func TestAttackSkillSlotsWarriorMeleeUsesSourceCosts(t *testing.T) {
	w, r, _ := stageBWorker(t, 0, 12, 25, 26)
	now := time.Unix(1800000000, 0)
	focus := r.Monsters[0]
	focus.Pos = stepPoint(r.Player.Pos, 4, 2)
	r.Role.SkillSlotIDs = []uint32{25, 12, 26, 0}
	request := &pb.AttackIntent{SceneEpoch: r.Scene, TargetHandle: focus.Handle, TargetLife: focus.Life, InputSequence: 1}
	mana := r.Player.MP
	if code := w.attackFromConfiguredSlots(r, request, now); code != 0 || r.Player.AttackSkillID != 12 || focus.HP >= focus.MaxHP || r.Player.MP != mana || r.Melee.Thrusting {
		t.Fatalf("二格攻击未使用配置刺杀或错误改变旧模式/蓝量，错误码%d", code)
	}
	focus.Pos = stepPoint(r.Player.Pos, 4, 1)
	r.Role.SkillSlotIDs = []uint32{26, 0, 0, 0}
	if code := w.attackFromConfiguredSlots(r, request, now); code != p.CodeSkillStateBlocked || r.Player.MP != mana || r.Melee.Flaming || !r.Melee.FlamingUntil.IsZero() {
		t.Fatalf("未通过动作检查时错误扣烈火蓝或创建冷却，错误码%d", code)
	}
	ready := r.Player.NextAttack
	if code := w.attackFromConfiguredSlots(r, request, ready); code != 0 || r.Player.AttackSkillID != 26 || r.Player.MP != mana-7 || !r.Melee.FlamingUntil.Equal(ready.Add(10*time.Second)) || r.Melee.Flaming {
		t.Fatalf("受理烈火未按原消耗和时窗提交，错误码%d", code)
	}
	if code := w.attackFromConfiguredSlots(r, request, r.Player.NextAttack); code != 0 || r.Player.AttackSkillID != 0 || r.Player.MP != mana-7 {
		t.Fatalf("烈火仍冷却却再次消耗，错误码%d", code)
	}
}
