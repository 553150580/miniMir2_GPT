// 本文件验证幸运贡献、原攻防环节、原施法快照及原装备观察回执。
package game

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

func luckFixture() *devconfig.CombatLuckConfig {
	return &devconfig.CombatLuckConfig{
		WeaponWingPPM:        []uint32{0, 20000, 30000, 44000, 60000, 80000, 110000, 150000, 210000, 300000},
		OtherPPM:             []uint32{0, 20000, 30000, 44500},
		MonsterResistancePPM: map[uint32]uint32{99: 200000},
		RoleResistancePPM:    map[uint32]uint32{1: 200000},
	}
}

func luckTestWeapon() domain.Equipped {
	return domain.Equipped{Slot: devconfig.WeaponSlot,
		Item: domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1, InitialLuck: 3}}
}

// TestCombatLuckContributionAndEffectivePieces 以原 11 部位分档核对封顶、抵抗和装备失效。
func TestCombatLuckContributionAndEffectivePieces(t *testing.T) {
	w, r, _ := equipmentWorker(t, 1)
	f := &w.e.Store.Cfg.Fixture
	f.CombatLuck = luckFixture()
	r.Role.Equipped = []domain.Equipped{luckTestWeapon()}
	r.Role.Equipped[0].Item.InitialLuck = 9
	for slot := uint32(1); slot <= 9; slot++ {
		id := 32300 + slot
		f.Items = append(f.Items, devconfig.Item{ID: id, IsEquipment: true, EquipmentSlot: slot})
		r.Role.Equipped = append(r.Role.Equipped, domain.Equipped{Slot: slot, Item: domain.Item{
			ID: domain.ID(), ItemID: id, Quantity: 1, Version: 1, InitialLuck: 3}})
	}
	if got := equipmentLuck(r, f, 0); got != 700500 {
		t.Fatalf("无翅膀的真实十部位贡献：%d", got)
	}
	if got := equipmentLuck(r, f, 9); got != luckMillion || effectiveAttackLuck(1000500, 200000) != 800000 || effectiveAttackLuck(22000, 200000) != 0 {
		t.Fatalf("100.05%% 必须先封顶，抵抗不能借用溢出值：%d", got)
	}
	// 持久破损、未入库磨损和错误槽均无贡献，重复部位只计一次。
	broken := &r.Role.Equipped[2].Item
	broken.OriginalDurabilityU, broken.MaxDurabilityU = 5, 5
	worn := r.Role.Equipped[5]
	r.Role.Equipped[5].Item.OriginalDurabilityU, r.Role.Equipped[5].Item.MaxDurabilityU, r.Role.Equipped[5].Item.DurabilityU = 5, 5, 1
	r.Wear.open = []persistence.EquipmentWear{{Slot: worn.Slot, InstanceID: worn.Item.ID, Version: worn.Item.Version, HitU: 1}}
	r.Role.Equipped[3].Item.ItemID = 218
	r.Role.Equipped = append(r.Role.Equipped, r.Role.Equipped[4])
	if got := equipmentLuck(r, f, 0); got != 567000 {
		t.Fatalf("破损、错槽或重复部位影响了实际贡献：%d", got)
	}
	r.Role.Equipped = []domain.Equipped{luckTestWeapon()}
	attrs := w.combatAttributesPB(r, &r.Player)
	if !attrs.LuckEnabled || attrs.LuckAttackPpm != 44000 || attrs.LuckResistancePpm != 200000 {
		t.Fatal("原人物面板未反映武器贡献和职业抵抗", attrs)
	}
	// 挑战/镜像入口使用同一份原角色战斗快照；卸装不回写已冻结的值。
	snapshot := w.freezeBossCombatSnapshot(r, &f.Map)
	r.Role.Equipped = nil
	if snapshot.Fighter.LuckAttackPPM != 44000 || w.fighter(r, &r.Player).LuckAttackPPM != 0 {
		t.Fatal("卸装或快照没有保持其独立观察")
	}
	f.CombatLuck = nil
	legacy := w.combatAttributesPB(r, &r.Player)
	encoded, err := proto.Marshal(legacy)
	var decoded pb.CombatAttributes
	if err != nil || proto.Unmarshal(encoded, &decoded) != nil || decoded.LuckEnabled || decoded.LuckAttackPpm != 0 || decoded.LuckResistancePpm != 0 {
		t.Fatal("默认无该配置的原协议投影仍出现幸运字段", &decoded)
	}
}

// TestCombatLuckPhysicalKeepsHitArmourAndZeroChanceDraw 验证命中不按幸运放行，AC 正常扣减，零幸运也可正常掷到上限。
func TestCombatLuckPhysicalKeepsHitArmourAndZeroChanceDraw(t *testing.T) {
	w, r, _ := equipmentWorker(t, 0)
	f := &w.e.Store.Cfg.Fixture
	f.CombatLuck = luckFixture()
	r.Role.Equipped = []domain.Equipped{luckTestWeapon()}
	target := r.Monsters[0]
	target.Monster = &devconfig.Monster{Fighter: devconfig.Fighter{DefenceMin: 1, DefenceMax: 1}}
	stats := w.fighter(r, &r.Player)
	rolls := 0
	w.roll = func(n int64) int64 {
		if n == int64(luckMillion) {
			rolls++
		}
		return 0
	}
	if got := w.physicalDamage(r, &r.Player, target); got != stats.AttackMax-1 || rolls != 1 {
		t.Fatalf("幸运取上限后未按原 AC 计算：伤害%d，幸运抽取%d，上限%d", got, rolls, stats.AttackMax)
	}
	target.Monster.ID = 99
	if got := w.physicalDamage(r, &r.Player, target); got != max(stats.AttackMin-1, 0) || rolls != 1 {
		t.Fatal("目标抵抗覆盖贡献时仍触发或额外掷点", got, rolls)
	}
	target.Monster.ID = 0
	target.Monster.Agility = 100000
	w.roll = func(n int64) int64 {
		if n == int64(luckMillion) {
			t.Fatal("物理闪避后不应再掷幸运")
		}
		return n - 1
	}
	if w.physicalDamage(r, &r.Player, target) != 0 {
		t.Fatal("幸运绕过了原物理闪避")
	}
	r.Role.Equipped[0].Item.InitialLuck = 0
	target.Monster.Agility = 0
	if got := w.physicalDamage(r, &r.Player, target); got != stats.AttackMax-1 {
		t.Fatal("零幸运的原区间仍应可能取得上限", got)
	}
}

// TestCombatLuckMeleeSharesRollAcrossTargets 用半月真实挥刀核对共享掷点及各目标的独立抵抗。
func TestCombatLuckMeleeSharesRollAcrossTargets(t *testing.T) {
	w, r, _ := stageBWorker(t, 0, 25)
	f := &w.e.Store.Cfg.Fixture
	f.CombatLuck = luckFixture()
	r.Role.Equipped = []domain.Equipped{luckTestWeapon()}
	const direction = 0
	r.Monsters = []*entity{
		{Handle: 10, Kind: 3, HP: 1000, Pos: stepPoint(r.Player.Pos, direction, 1), Monster: &devconfig.Monster{}},
		{Handle: 11, Kind: 3, HP: 1000, Pos: stepPoint(r.Player.Pos, 1, 1), Monster: &devconfig.Monster{}},
		{Handle: 12, Kind: 3, HP: 1000, Pos: stepPoint(r.Player.Pos, 7, 1), Monster: &devconfig.Monster{ID: 99}},
	}
	r.MonsterCount = len(r.Monsters)
	rolls := 0
	w.roll = func(n int64) int64 {
		if n == int64(luckMillion) {
			rolls++
		}
		return 0
	}
	stats := w.fighter(r, &r.Player)
	skill, _ := f.Skill(25)
	now := time.Unix(1800000000, 0)
	if result := w.attackWithSkill(r, direction, 1, now, skill, skill.LearnedLevel); result != 0 || len(r.Attacks) != 1 {
		t.Fatal("隔离半月攻击未受理", result, len(r.Attacks))
	}
	power := skill.SourcePower(skill.LearnedLevel, skill.MPowerBase, skill.PowerBase)
	maxSide := skill.SourceDamage(skill.LearnedLevel, stats.AttackMax, power)
	minSide := skill.SourceDamage(skill.LearnedLevel, stats.AttackMin, power)
	if rolls != 1 || r.Attacks[0].Amount != stats.AttackMax || r.Monsters[1].HP != 1000-maxSide || r.Monsters[2].HP != 1000-minSide {
		t.Fatalf("同挥刀各目标没按原比例和抵抗分流：抽取%d、前格威力%d、两旁格实际%d/%d", rolls, r.Attacks[0].Amount, r.Monsters[1].HP, r.Monsters[2].HP)
	}
}

// TestCombatLuckSpellUsesOriginalPowerAndLife 核对火球和灵魂火符的原职业区间、延迟身份以及实际魔防。
func TestCombatLuckSpellUsesOriginalPowerAndLife(t *testing.T) {
	for _, sample := range []struct{ job, skill uint32 }{{1, 1}, {2, 13}} {
		w, r, _ := equipmentWorker(t, sample.job)
		f := &w.e.Store.Cfg.Fixture
		f.CombatLuck = luckFixture()
		r.Role.Equipped = []domain.Equipped{luckTestWeapon()}
		target := r.Monsters[0]
		target.HP, target.MaxHP = 1000, 1000
		target.Monster = &devconfig.Monster{Fighter: devconfig.Fighter{MagicDefenceMin: 2, MagicDefenceMax: 2}}
		skill, _ := f.Skill(sample.skill)
		r.Role.Skills = []domain.LearnedSkill{{SkillID: sample.skill, Level: skill.LearnedLevel}}
		r.Player.MP = 1000
		rolls := 0
		w.roll = func(n int64) int64 {
			if n == int64(luckMillion) {
				rolls++
			}
			return 0
		}
		now := time.Unix(1800000000, 0)
		plan, code := w.prepareCast(r, sample.skill, target.Handle, 0, now, true)
		if code != 0 || w.acceptPreparedCast(r, plan, 1, now) != 0 || len(r.Casts) != 1 {
			t.Fatalf("技能 %d 未按原时钟受理，错误%d", sample.skill, code)
		}
		accepted := r.Casts[0]
		stats := w.fighter(r, &r.Player)
		maxValue := stats.MagicMax
		if sample.job == 2 {
			maxValue = stats.TaoMax
		}
		want := skill.SourceDamage(skill.LearnedLevel, maxValue, skill.SourcePower(skill.LearnedLevel, skill.MPowerBase, skill.PowerBase))
		if rolls != 1 || accepted.Luck.upper != want || accepted.Luck.attack != 44000 {
			t.Fatalf("技能 %d 未冻结原职业上限威力和独立掷点：%+v", sample.skill, accepted.Luck)
		}
		r.Role.Equipped = nil
		// 换施法者生命不得维持原弹道。恢复原身份后，已受理事件也不受卸装影响。
		r.Player.Life++
		w.resolveCast(r, accepted, accepted.Due)
		if target.HP != 1000 {
			t.Fatal("原幸运弹道继承了新施法者生命")
		}
		r.Player.Life--
		w.advanceEvents(r, accepted.Due)
		if target.HP != 1000-max(want-2, 0) {
			t.Fatal("卸装后仍应沿原快照结算实际魔防", target.HP, want)
		}
		target.Life++
		w.resolveCast(r, accepted, accepted.Due)
		if target.HP != 1000-max(want-2, 0) {
			t.Fatal("原幸运施法命中到了新生命")
		}
	}
}

// TestCombatLuckPeriodicDamageKeepsOriginalDuration 验证火墙与绿毒只变更伤害，红毒不掷幸运。
func TestCombatLuckPeriodicDamageKeepsOriginalDuration(t *testing.T) {
	now := time.Unix(1800000000, 0)
	t.Run("火墙", func(t *testing.T) {
		w, r, target := fireWallTestRoom(t)
		f := &w.e.Store.Cfg.Fixture
		f.CombatLuck = luckFixture()
		weapon := luckTestWeapon()
		weapon.Item.ItemID = 32300
		f.Items = append(f.Items, devconfig.Item{ID: weapon.Item.ItemID, IsEquipment: true, EquipmentSlot: weapon.Slot, MagicMaxBonus: 20})
		r.Role.Equipped = []domain.Equipped{weapon}
		center := devconfig.Point{X: 6, Y: 5}
		target.Pos = center
		if code := w.cast(r, 22, 0, 1, now, pointPB(center)); code != 0 || len(r.Casts) != 1 {
			t.Fatal("火墙未受理", code)
		}
		cast := r.Casts[0]
		w.advanceEvents(r, cast.Due)
		world := r.currentWorld()
		if len(world.FireWalls) != 1 || cast.Luck.upper <= cast.Amount || target.HP != 1000-cast.Luck.upper ||
			world.FireWalls[0].Expires.Sub(cast.Due) != fireWallLifetime(cast.Amount) {
			t.Fatalf("火墙原伤害区间或持续时间不符，生命%d，施法 %+v", target.HP, cast)
		}
		w.advanceEvents(r, cast.Due.Add(2*time.Second))
		if target.HP != 1000-2*cast.Luck.upper {
			t.Fatal("后续火墙命中未沿原火墙威力结算", target.HP)
		}
	})
	t.Run("两色施毒", func(t *testing.T) {
		w, r, target, _, _ := soulFireBallFixture(t)
		w.e.Store.Cfg.Fixture.CombatLuck = luckFixture()
		plan := &preparedCast{Target: reference(r, target), Skill: devconfig.Skill{ID: 6, Behavior: "poisoning"},
			PoisonColor: devconfig.PoisonRed, Amount: 15, Luck: luckyDamage{attack: 44000, upper: 45}}
		rolls := 0
		w.roll = func(int64) int64 { rolls++; return 0 }
		if w.acceptCastLuck(r, plan).attack != 0 || rolls != 0 {
			t.Fatal("红毒没有职业攻击区间伤害，应保留原持续时间且不掷幸运")
		}
		plan.PoisonColor = devconfig.PoisonGreen
		cast := pendingCast{Scene: r.Scene, ID: 1, Skill: plan.Skill, Caster: reference(r, &r.Player),
			Target: target.Handle, TargetLife: target.Life, Due: now, Amount: plan.Amount, Luck: w.acceptCastLuck(r, plan), PoisonColor: plan.PoisonColor}
		w.resolveCast(r, cast, now)
		if rolls != 1 || target.HP != 996 || target.Poisons == nil || target.Poisons.Green == nil || target.Poisons.Green.Remaining != 36 {
			t.Fatal("绿毒应每跳按幸运伤害结算，仍保留原跳数", target.HP, target.Poisons)
		}
		w.advanceEvents(r, now.Add(poisonInterval))
		if target.HP != 992 || rolls != 1 {
			t.Fatal("绿毒后续跳不应重抽幸运", target.HP, rolls)
		}
	})
}

// TestCombatLuckMirrorUsesOpposingSnapshot 使用镜像双方临时 Fighter 验证角色目标抵抗归属。
func TestCombatLuckMirrorUsesOpposingSnapshot(t *testing.T) {
	for _, resistance := range []uint32{0, 200000} {
		w, arena, fighters, at := mirrorCombatFixture(t)
		w.e.Store.Cfg.Fixture.CombatLuck = luckFixture()
		ball := equipMirrorSkill(t, w, &fighters[0], 1)
		fighters[0].Fighter.MagicMax = 32
		fighters[0].Fighter.LuckAttackPPM = 210000
		fighters[1].Fighter.LuckResistancePPM = resistance
		b := w.newMirrorCombat(arena, fighters, at, at.Add(time.Minute), 77)
		if b == nil {
			t.Fatal("无法构建镜像双方")
		}
		left, right := b.sides[0].room, b.sides[1].room
		w.roll = func(n int64) int64 {
			if n == int64(luckMillion) {
				return 15000
			}
			return 0
		}
		plan, code := w.prepareCast(left, ball.ID, right.Player.Handle, 0, at, true)
		if code != 0 || w.acceptPreparedCast(left, plan, 1, at) != 0 || len(left.Casts) != 1 {
			t.Fatal("原镜像攻击未受理", code)
		}
		cast := left.Casts[0]
		want := cast.Amount
		if resistance == 0 {
			want = cast.Luck.upper
		}
		if cast.Amount >= cast.Luck.upper || w.fighter(left, &right.Player).LuckResistancePPM != resistance {
			t.Fatal("镜像攻击未读取双方的独立职业战斗快照")
		}
		w.advanceMirrorCombat(b, cast.Due)
		if right.Player.HP != right.Player.MaxHP-want || left.Player.HP != left.Player.MaxHP {
			t.Fatal("镜像伤害未按原对手抵抗和身体结算", resistance, right.Player.HP, want)
		}
	}
}

// TestCombatLuckConfirmedOriginalLoot 只向原战场发布真实落库的同一件幸运装备。
func TestCombatLuckConfirmedOriginalLoot(t *testing.T) {
	item := domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1, Bound: true, SourceKind: "monster_drop", SourceID: "1"}
	q := persistence.Settlement{Item: &item, SourceSeq: 1}
	actual := item
	actual.InitialLuck = 3
	receipt := &persistence.SettlementReceipt{GrantedItem: true, LuckyDrop: &actual}
	op := persistence.Operation{Kind: "settlement", SourceSeq: 1, Settlement: receipt}
	if loot, ok := confirmedSettlementLoot(q, op); !ok || loot == nil || loot.InitialLuck != 3 || item.InitialLuck != 0 {
		t.Fatal("已确认幸运不是原掉落实例", loot)
	}
	actual.ID = domain.ID()
	if _, ok := confirmedSettlementLoot(q, op); ok {
		t.Fatal("错配实例不得作为原战利品发布")
	}
	receipt.GrantedItem = false
	if loot, ok := confirmedSettlementLoot(q, op); !ok || loot != nil {
		t.Fatal("回收或丢弃的幸运物品不得伪装成已入包")
	}
}
