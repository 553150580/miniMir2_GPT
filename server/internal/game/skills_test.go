package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"testing"
	"time"
)

func skillWorker(t *testing.T, job uint32) (*worker, *room, *capturedConnection) {
	t.Helper()
	r, c := observedRoom()
	w := observedWorker(t, r)
	w.e.Store.Cfg.Fixture.DamageFormula = "development_source_combat"
	r.Role.Job, r.Role.Level = job, 7
	for _, s := range w.e.Store.Cfg.Fixture.Skills {
		if s.JobID == job {
			r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: s.ID, Level: s.LearnedLevel})
		}
	}
	w.refreshPlayerStats(r)
	r.Player.HP, r.Player.MP = r.Player.MaxHP, r.Player.MaxMP
	w.roll = func(int64) int64 { return 0 }
	return w, r, c
}

func TestSourceDamageUsesHitACMACAndZeroFloor(t *testing.T) {
	w, r, _ := skillWorker(t, 1)
	if amount := w.magicAmount(devconfig.Skill{MPowerBase: 2, MultiplierBasePPM: 1000000}, 0, 0); amount != 0 {
		t.Fatalf("0.5未取偶: %d", amount)
	}
	if amount := w.magicAmount(devconfig.Skill{MPowerBase: 6, MultiplierBasePPM: 1000000}, 0, 0); amount != 2 {
		t.Fatalf("1.5未取偶: %d", amount)
	}
	monster := r.Monsters[0]
	monster.Monster = &devconfig.Monster{Fighter: devconfig.Fighter{DefenceMin: 20, DefenceMax: 20, MagicDefenceMin: 2, MagicDefenceMax: 2, Agility: 0, MagicResistWeight: 10}}
	if w.physicalDamage(r, &r.Player, monster) != 0 || w.magicDamage(r, monster, 7) != 5 {
		t.Fatal("AC/MAC混用或强行造成1点伤害")
	}
	monster.Monster.DefenceMin, monster.Monster.DefenceMax = 0, 0
	monster.Monster.Agility = 10
	w.roll = func(n int64) int64 { return n - 1 }
	if w.physicalDamage(r, &r.Player, monster) != 0 {
		t.Fatal("物理闪避没有生效")
	}
	monster.Monster.MagicResist = 10
	if w.magicDamage(r, monster, 7) != 0 {
		t.Fatal("MR权重没有参与魔法防御")
	}
}

func TestFireballValidateBeforeSideEffectsAndDelayedLifeBoundDamage(t *testing.T) {
	w, r, c := skillWorker(t, 1)
	now := time.Unix(1800000000, 0)
	m := r.Monsters[0]
	destination := devconfig.Point{X: 6, Y: 8}
	r.Destination = &destination
	mana := r.Player.MP
	for _, test := range []struct {
		id, target uint32
		code       uint32
	}{{2, m.Handle, p.CodeSkillNotLearned}, {1, r.Pet.Handle, p.CodeSkillTarget}, {1, 999, p.CodeSkillTarget}} {
		if code := w.cast(r, test.id, test.target, 1, now); code != test.code || r.Player.MP != mana || !r.NextSpell.IsZero() || r.Player.CastSequence != 0 {
			t.Fatalf("拒绝产生施法副作用:%d", code)
		}
	}
	if code := w.cast(r, 1, m.Handle, 2, now); code != 0 {
		t.Fatalf("合法火球拒绝:%d", code)
	}
	definition, _ := w.e.Store.Cfg.Fixture.Skill(1)
	if r.Player.MP != mana-definition.MPBaseCost || r.Destination != &destination || r.Player.CastSequence != 1 || len(r.Casts) != 1 || len(c.frames) != 2 || c.frames[0].ID != p.MsgS2CCastAccepted || c.frames[1].ID != p.MsgS2CSceneBaseline {
		t.Fatal("受理未同步MP/独立观察/事件，或清除了导航")
	}
	event := &pb.CastAccepted{}
	if err := proto.Unmarshal(c.frames[0].Payload, event); err != nil {
		t.Fatal(err)
	}
	if event.DelayMs != uint32(definition.DelayBaseMS+definition.ProjectileMSPerCell) || event.InputSequence != 2 || event.CastId != 1 {
		t.Fatal("事件未携带实际施法延迟和输入身份")
	}
	hp := m.HP
	due := r.Casts[0].Due
	w.advanceCasts(r, due.Add(-time.Nanosecond))
	if m.HP != hp {
		t.Fatal("火球提前命中")
	}
	r.Player.HP = 0
	w.advanceCasts(r, due)
	if m.HP >= hp || len(r.Casts) != 0 {
		t.Fatal("已发出火球被施法者死亡取消或没有延迟命中")
	}
	r.Player.HP = r.Player.MaxHP
	r.Player.NextMove = time.Time{}
	r.NextSpell = time.Time{}
	r.SkillCooldowns = nil
	if w.cast(r, 1, m.Handle, 3, due.Add(time.Second)) != 0 {
		t.Fatal("第二发受理失败")
	}
	m.Life++
	hp = m.HP
	w.advanceCasts(r, r.Casts[0].Due)
	if hp != m.HP {
		t.Fatal("旧投射物命中了目标的新生命")
	}
}

func TestSkillCooldownResourceTargetAndProjectileObstruction(t *testing.T) {
	w, r, _ := skillWorker(t, 1)
	now := time.Unix(1800000000, 0)
	s, _ := w.e.Store.Cfg.Fixture.Skill(1)
	r.Player.MP = 0
	if w.cast(r, 1, r.Monsters[0].Handle, 1, now) != p.CodeSkillResource || !r.NextSpell.IsZero() {
		t.Fatal("缺蓝改变了CD")
	}
	r.Player.MP = r.Player.MaxMP
	r.NextSpell = now.Add(time.Second)
	if w.cast(r, 1, r.Monsters[0].Handle, 2, now) != p.CodeSkillCooldown {
		t.Fatal("全局CD未校验")
	}
	r.NextSpell = time.Time{}
	r.Monsters = append(r.Monsters, &entity{Handle: 11, Kind: 3, HP: 10, Pos: r.Monsters[0].Pos})
	if w.cast(r, 1, 11, 3, now) != p.CodeSkillTarget {
		t.Fatal("未激活怪物可以成为技能目标")
	}
	r.Monsters[0].Pos = devconfig.Point{X: 8, Y: 8 - s.RangeCells - 1}
	if w.cast(r, 1, 10, 4, now) != p.CodeSkillTarget {
		t.Fatal("超出射程仍然受理")
	}
	r.Monsters[0].Pos = devconfig.Point{X: 8, Y: 6}
	w.e.Store.Cfg.Fixture.Map.Blocked = append(w.e.Store.Cfg.Fixture.Map.Blocked, devconfig.Point{X: 8, Y: 7})
	if w.cast(r, 1, 10, 5, now) != p.CodeSkillTarget {
		t.Fatal("火球无视来源直线阻挡")
	}
}

func TestPotionReachesFullBeforeCombatClearsWaitingHealingBalance(t *testing.T) {
	w, r, _ := skillWorker(t, 2)
	now := time.Unix(1800000000, 0)
	r.Player.HP = r.Player.MaxHP - 1
	r.Player.HealAmount = 20
	r.Player.NextHeal = now.Add(time.Second)
	r.Potions.HP = 20
	r.Monsters[0].Pos = devconfig.Point{X: 8, Y: 9}
	r.Pet = nil
	w.roll = func(int64) int64 { return 0 }
	w.step(r, now)
	if r.Player.HP >= r.Player.MaxHP || r.Player.HealAmount != 0 || r.Potions.HP != 0 {
		t.Fatal("药品补满后本帧受击，保留了本应清除的治疗余额")
	}
	hp := r.Player.HP
	w.tickHealing(r, now.Add(2*time.Second))
	if r.Player.HP != hp {
		t.Fatal("旧治疗余额在重新受击后复用")
	}
}

func TestHealingSeparatePlayerAndPetClockSurvivesDeathPendingAndOffline(t *testing.T) {
	w, r, _ := skillWorker(t, 2)
	now := time.Unix(1800000000, 0)
	r.Player.HP = 1
	r.Pet.HP = 1
	if code := w.cast(r, 2, r.Pet.Handle, 1, now); code != 0 {
		t.Fatal(code)
	}
	due := r.Casts[0].Due
	w.advanceCasts(r, due.Add(-time.Nanosecond))
	if r.Pet.HealAmount != 0 {
		t.Fatal("治愈提前入队")
	}
	w.advanceCasts(r, due)
	r.Player.HealAmount = 20
	h := w.e.Store.Cfg.Fixture.Healing
	beforePet := r.Pet.HP
	w.tickHealing(r, due)
	if r.Pet.HP-beforePet != h.PetBasePerTick || r.Player.HP != 1+h.BasePerTick+int64(r.Role.Level)/h.LevelsPerStep+20/h.QueueFractionDivisor {
		t.Fatal("宠物错误沿用玩家递增治疗公式")
	}
	r.Potions.HP = 10
	w.tickPotions(r, due)
	if r.Player.HealAmount == 0 || r.Potions.HP == 0 {
		t.Fatal("治疗与药品队列互相清空")
	}
	before := r.Pet.HP
	r.Pending = true
	w.step(r, due.Add(time.Second))
	if r.Pet.HP != before {
		t.Fatal("未知事务未冻结治疗")
	}
	r.Pending = false
	r.Player.HP = 0
	r.Conn = nil
	r.Role.HostingSeconds = 0
	w.step(r, due.Add(2*time.Second))
	if r.Pet.HP <= before || r.Player.HealAmount == 0 || r.HostMS != 0 {
		t.Fatal("主人死亡/无托管时已支付宠物治疗停止、玩家队列丢失或错误计费")
	}
	r.Pet.HP = r.Pet.MaxHP
	w.tickHealing(r, due.Add(3*time.Second))
	if r.Pet.HealAmount != 0 {
		t.Fatal("满血未清治疗队列")
	}
}

func TestInputWatermarkSharedByMoveAndCastAndRejectsOldBinding(t *testing.T) {
	w, r, c := skillWorker(t, 1)
	send := func(id uint16, q proto.Message, binding uint64) {
		w.command(command{role: "role", conn: c, frame: wire.Message(id, 1, binding, q)})
	}
	send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: r.Scene, ReleaseJoystick: true, InputSequence: 4}, 3)
	send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: r.Scene, SkillId: 1, TargetHandle: 10, InputSequence: 4}, 3)
	if r.Player.CastSequence != 0 || c.frames[len(c.frames)-1].Error != p.CodeStaleInput {
		t.Fatal("施法重放绕过了移动共享输入水位")
	}
	send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 100}, 2)
	if r.InputSequence != 4 {
		t.Fatal("旧连接影响了新连接输入水位")
	}
	// join绑定成功后归零；新绑定从1开始，旧绑定仍由上面的session gate拒绝。
	r.InputSequence = 0
	if !acceptInput(r, 1) {
		t.Fatal("健康重连无法从1重新输入")
	}
}

func TestFencingPassiveAndEquipmentMACDoNotDoubleAddLegacyDefence(t *testing.T) {
	w, r, _ := skillWorker(t, 0)
	r.Role.Skills[0].Level = 2
	r.Role.Equipped = []domain.Equipped{{Item: domain.Item{ItemID: 193}}}
	fighter := w.fighter(r, &r.Player)
	base, _ := w.e.Store.Cfg.Fixture.PlayerAtLevel(0, 7)
	if fighter.Accuracy != base.Accuracy+6 || fighter.DefenceMin != base.DefenceMin+2 || fighter.DefenceMax != base.DefenceMax+2 || fighter.MagicDefenceMin != 0 || fighter.MagicDefenceMax != 1 {
		t.Fatalf("被动/AC/MAC聚合不符:%+v", fighter)
	}
}

func TestUpgradedSkillRanksKeepShapesAndUseSourceNumericGrowth(t *testing.T) {
	now := time.Unix(1800000000, 0)
	for _, job := range []uint32{0, 1, 2} {
		w, r, _ := skillWorker(t, job)
		s, _ := w.e.Store.Cfg.Fixture.Skill(map[uint32]uint32{0: 3, 1: 1, 2: 2}[job])
		for rank := s.LearnedLevel; rank <= s.MaxLevel; rank++ {
			r.Role.Skills[0].Level = rank
			if job == 0 {
				stats := w.fighter(r, &r.Player)
				if stats.Accuracy != 5+int64(rank)*s.AccuracyPerLevel {
					t.Fatal("基本剑术升级未真正提高准确")
				}
				continue
			}
			r.Player.MP = r.Player.MaxMP
			r.Player.NextMove = time.Time{}
			r.Player.ActionReadyAt = time.Time{}
			r.NextSpell = time.Time{}
			r.SkillCooldowns = nil
			r.Casts = nil
			target := uint32(10)
			if job == 2 {
				target = 1
				r.Player.HP = 1
			}
			mana := r.Player.MP
			if code := w.cast(r, s.ID, target, rank+1, now); code != 0 {
				t.Fatalf("job%d rank%d不能施法:%d", job, rank, code)
			}
			if r.Player.MP != mana-s.MPBaseCost-int64(rank)*s.MPLevelCost || r.Casts[0].Skill.RangeCells != s.RangeCells || r.Casts[0].Skill.Behavior != s.Behavior {
				t.Fatal("升级修改基础形状或没有使用按级耗蓝")
			}
			want := []int64{5, 7, 9, 11}[rank]
			if job == 2 {
				want = []int64{13, 16, 19, 23}[rank]
			}
			if r.Casts[0].Amount != want {
				t.Fatalf("job%d rank%d值=%d，预期%d", job, rank, r.Casts[0].Amount, want)
			}
		}
	}
}
