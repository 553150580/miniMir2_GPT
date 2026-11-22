// 本文件回归法师直线与范围魔法的几何、阻挡、目标过滤、资源门禁和普攻回退。
package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"testing"
	"time"
)

// TestLightningExactSixCellsEightDirectionsAndFrontWallDoesNotStop 核对「疾光未共享一次MC/技能威力样本」与「合法朝向/任意远点击格被拒绝」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestLightningExactSixCellsEightDirectionsAndFrontWallDoesNotStop(t *testing.T) {
	// direction 保存由无符号整数“32”取得的朝向。
	for direction := uint32(0); direction < 8; direction++ {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(string(rune('0'+direction)), func(t *testing.T) {
			// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
			w, r, _ := stageBWorker(t, 1, 10)
			// 边界之外的第七格仍要在图内，用独立几何验证射程而非小图边界。
			w.e.Store.Cfg.Fixture.Map.Width, w.e.Store.Cfg.Fixture.Map.Height = 17, 17
			// now 记录本次处理开始时的时间。
			now := time.Now()
			// origin 保存位置，用于当前步骤中的大区或起点。
			origin := r.Player.Pos
			// definition 保存怪物，用于当前步骤中的定义。
			definition := r.Monsters[0].Monster
			r.Monsters = nil
			// i 标记循环或列表中的当前位置。
			for i := 1; i <= 7; i++ {
				r.Monsters = append(r.Monsters, &entity{Handle: uint32(9 + i), Kind: 3, Pos: stepPoint(origin, direction, i), HP: 1000, MaxHP: 1000, Monster: definition})
			}
			// 每格只命中稳定顺序第一敌，不能把同格叠放当成整格群伤。
			r.Monsters = append(r.Monsters, &entity{Handle: 17, Kind: 3, Pos: r.Monsters[0].Pos, HP: 1000, MaxHP: 1000, Monster: definition})
			r.MonsterCount = 8
			w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{stepPoint(origin, direction, 2)}
			// code 保存由施法“directed”取得的结果码。
			if code := w.castDirected(r, 10, 0, 1, direction, now, &pb.GridPoint{X: 0, Y: 0}); code != 0 {
				t.Fatalf("合法朝向/任意远点击格被拒绝:%d", code)
			}
			if len(r.Casts) != 1 || r.Casts[0].Amount != 20 {
				t.Fatal("疾光未共享一次MC/技能威力样本")
			}
			r.Player.Pos = devconfig.Point{X: 1, Y: 1}
			w.advanceEvents(r, now.Add(500*time.Millisecond-time.Nanosecond))
			if r.Monsters[0].HP != 1000 {
				t.Fatal("疾光提前命中")
			}
			w.advanceEvents(r, now.Add(500*time.Millisecond))
			// i 定位怪物集合中正在处理的成员，target 接收遍历怪物集合时当前的元素。
			for i, target := range r.Monsters {
				// want 保存由整数“64”取得的期望值。
				want := int64(980)
				if i == 1 || i >= 6 {
					want = 1000
				}
				if target.HP != want {
					t.Fatalf("方向%d第%d目标HP%d/%d", direction, i+1, target.HP, want)
				}
			}
		})
	}
}

// TestIceStormImpactOccupantsAndThunderStormOwnOriginUndeadMAC 核验冰咆哮中心阻挡不应拒绝施法，或来源共享威力错误，用诊断「冰咆哮未按到期合法敌人/原地范围/友方过滤结算」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestIceStormImpactOccupantsAndThunderStormOwnOriginUndeadMAC(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 1, 33, 24)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	// definition 保存怪物，用于当前步骤中的定义。
	definition := r.Monsters[0].Monster
	// center 建立带初始值的点位，供本段继续处理。
	center := devconfig.Point{X: 8, Y: 6}
	r.Monsters[0].Pos = center
	r.Monsters = append(r.Monsters, &entity{Handle: 11, Kind: 3, Pos: devconfig.Point{X: 1, Y: 1}, HP: 1000, MaxHP: 1000, Monster: definition})
	r.MonsterCount = 2
	w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{center}
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 33, 0, 1, now, pointPB(center)); code != 0 || r.Casts[0].Amount != 22 {
		t.Fatal("冰咆哮中心阻挡不应拒绝施法，或来源共享威力错误")
	}
	r.Monsters[0].Pos = devconfig.Point{X: 1, Y: 1}
	r.Monsters[1].Pos = devconfig.Point{X: 9, Y: 6}
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Monsters[0].HP != 1000 || r.Monsters[1].HP != 978 || r.Pet.HP != r.Pet.MaxHP {
		t.Fatal("冰咆哮未按到期合法敌人/原地范围/友方过滤结算")
	}
	clearAction(r)
	w.e.Store.Cfg.Fixture.Map.Blocked = nil
	// origin 保存位置，用于当前步骤中的大区或起点。
	origin := r.Player.Pos
	r.Monsters[0].Pos = stepPoint(origin, 0, 2)
	r.Monsters[1].Pos = stepPoint(origin, 2, 2)
	r.Monsters[0].Monster = &devconfig.Monster{Undead: true, Fighter: devconfig.Fighter{MagicDefenceMin: 5, MagicDefenceMax: 5, MagicResistWeight: 10}}
	r.Monsters[1].Monster = &devconfig.Monster{Undead: false, Fighter: r.Monsters[0].Monster.Fighter}
	r.Monsters[1].HP = 1000
	// code 保存由施法“directed”取得的结果码。
	if code := w.castDirected(r, 24, 0, 2, 6, now, &pb.GridPoint{X: 0, Y: 0}); code != 0 || r.Casts[0].Amount != 17 {
		t.Fatal("地狱雷光应取服务器受理瞬间自身坐标，忽略点击格/朝向")
	}
	r.Player.Pos = devconfig.Point{X: 1, Y: 1}
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Monsters[0].HP != 988 || r.Monsters[1].HP != 1000 {
		t.Fatal("地狱雷光5x5/先非不死除10再减MAC，或中心随玩家移动")
	}
}

// TestNewMageAutoCannotStallAdjacentOrdinaryFallback 核验朝向群技应按目标数/CD选择下一项，或清除了导航，用诊断「只有一个邻格敌不满足群技目标数，却阻塞普通攻击」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNewMageAutoCannotStallAdjacentOrdinaryFallback(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 1, 10, 24, 33)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	r.Player.MP = 0
	if w.autoSkill(r, now) {
		t.Fatal("缺蓝时自动技能阻塞普通攻击")
	}
	r.Player.MP = r.Player.MaxMP
	if w.autoSkill(r, now) {
		t.Fatal("只有一个邻格敌不满足群技目标数，却阻塞普通攻击")
	}
	// point 组装点位所需的数据。
	point := devconfig.Point{X: 5, Y: 5}
	r.Destination = &point
	r.Monsters[0].Pos = devconfig.Point{X: 8, Y: 4}
	r.Monsters = append(r.Monsters, &entity{Handle: 11, Kind: 3, Pos: devconfig.Point{X: 8, Y: 3}, HP: 1000, MaxHP: 1000, Monster: r.Monsters[0].Monster})
	r.MonsterCount = 2
	// 冰咆哮两敌相邻，优先级更高；预算/不足蓝时尝试疾光，方向来自权威目标格。
	// ice 保存由技能取得的“ice”。
	ice, _ := w.e.Store.Cfg.Fixture.Skill(33)
	r.SkillCooldowns = map[uint32]time.Time{ice.ID: now.Add(time.Second)}
	if !w.autoSkill(r, now) || len(r.Casts) != 1 || r.Casts[0].Skill.ID != 10 || r.Casts[0].Direction != 0 || r.Destination != &point {
		t.Fatal("朝向群技没有按目标数/CD选择下一项，或清除了导航")
	}
}

// TestAllNewCastFailuresKeepManaCooldownAndInputReadyState 核对「新技能非法目标产生副作用」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAllNewCastFailuresKeepManaCooldownAndInputReadyState(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 1, 10, 24, 33)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	// id 接收遍历数值列表时当前的元素。
	for _, id := range []uint32{10, 24, 33} {
		// mana 保存魔法值，用于当前步骤中的魔法值。
		mana := r.Player.MP
		// code 保存由施法“directed”取得的结果码。
		if code := w.castDirected(r, id, 999, 1, 0, now, &pb.GridPoint{X: -1, Y: 0}); code != p.CodeSkillTarget || r.Player.MP != mana || !r.NextSpell.IsZero() || len(r.Casts) != 0 {
			t.Fatalf("新技能%d非法目标产生副作用:%d", id, code)
		}
	}
}
