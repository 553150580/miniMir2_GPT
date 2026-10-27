package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"testing"
	"time"
)

func TestLightningExactSixCellsEightDirectionsAndFrontWallDoesNotStop(t *testing.T) {
	for direction := uint32(0); direction < 8; direction++ {
		t.Run(string(rune('0'+direction)), func(t *testing.T) {
			w, r, _ := stageBWorker(t, 1, 10)
			now := time.Now()
			origin := r.Player.Pos
			definition := r.Monsters[0].Monster
			r.Monsters = nil
			for i := 1; i <= 7; i++ {
				r.Monsters = append(r.Monsters, &entity{Handle: uint32(9 + i), Kind: 3, Pos: stepPoint(origin, direction, i), HP: 1000, MaxHP: 1000, Monster: definition})
			}
			// 每格只命中稳定顺序第一敌，不能把同格叠放当成整格群伤。
			r.Monsters = append(r.Monsters, &entity{Handle: 17, Kind: 3, Pos: r.Monsters[0].Pos, HP: 1000, MaxHP: 1000, Monster: definition})
			r.MonsterCount = 8
			w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{stepPoint(origin, direction, 2)}
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
			for i, target := range r.Monsters {
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

func TestIceStormImpactOccupantsAndThunderStormOwnOriginUndeadMAC(t *testing.T) {
	w, r, _ := stageBWorker(t, 1, 33, 24)
	now := time.Now()
	definition := r.Monsters[0].Monster
	center := devconfig.Point{X: 8, Y: 6}
	r.Monsters[0].Pos = center
	r.Monsters = append(r.Monsters, &entity{Handle: 11, Kind: 3, Pos: devconfig.Point{X: 1, Y: 1}, HP: 1000, MaxHP: 1000, Monster: definition})
	r.MonsterCount = 2
	w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{center}
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
	origin := r.Player.Pos
	r.Monsters[0].Pos = stepPoint(origin, 0, 2)
	r.Monsters[1].Pos = stepPoint(origin, 2, 2)
	r.Monsters[0].Monster = &devconfig.Monster{Undead: true, Fighter: devconfig.Fighter{MagicDefenceMin: 5, MagicDefenceMax: 5, MagicResistWeight: 10}}
	r.Monsters[1].Monster = &devconfig.Monster{Undead: false, Fighter: r.Monsters[0].Monster.Fighter}
	r.Monsters[1].HP = 1000
	if code := w.castDirected(r, 24, 0, 2, 6, now, &pb.GridPoint{X: 0, Y: 0}); code != 0 || r.Casts[0].Amount != 17 {
		t.Fatal("地狱雷光应取服务器受理瞬间自身坐标，忽略点击格/朝向")
	}
	r.Player.Pos = devconfig.Point{X: 1, Y: 1}
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Monsters[0].HP != 988 || r.Monsters[1].HP != 1000 {
		t.Fatal("地狱雷光5x5/先非不死除10再减MAC，或中心随玩家移动")
	}
}

func TestNewMageAutoCannotStallAdjacentOrdinaryFallback(t *testing.T) {
	w, r, _ := stageBWorker(t, 1, 10, 24, 33)
	now := time.Now()
	r.Player.MP = 0
	if _, handled := w.autoSkill(r, now); handled {
		t.Fatal("缺蓝却返回空追击，阻塞普通攻击")
	}
	r.Player.MP = r.Player.MaxMP
	if _, handled := w.autoSkill(r, now); handled {
		t.Fatal("只有一个邻格敌不满足群技目标数，却阻塞普通攻击")
	}
	point := devconfig.Point{X: 5, Y: 5}
	r.Destination = &point
	r.Monsters[0].Pos = devconfig.Point{X: 8, Y: 4}
	r.Monsters = append(r.Monsters, &entity{Handle: 11, Kind: 3, Pos: devconfig.Point{X: 8, Y: 3}, HP: 1000, MaxHP: 1000, Monster: r.Monsters[0].Monster})
	r.MonsterCount = 2
	// 冰咆哮两敌相邻，优先级更高；预算/不足蓝时尝试疾光，方向来自权威目标格。
	ice, _ := w.e.Store.Cfg.Fixture.Skill(33)
	r.SkillCooldowns = map[uint32]time.Time{ice.ID: now.Add(time.Second)}
	if _, handled := w.autoSkill(r, now); !handled || len(r.Casts) != 1 || r.Casts[0].Skill.ID != 10 || r.Casts[0].Direction != 0 || r.Destination != &point {
		t.Fatal("朝向群技没有按目标数/CD选择下一项，或清除了导航")
	}
}

func TestAllNewCastFailuresKeepManaCooldownAndInputReadyState(t *testing.T) {
	w, r, _ := stageBWorker(t, 1, 10, 24, 33)
	now := time.Now()
	for _, id := range []uint32{10, 24, 33} {
		mana := r.Player.MP
		if code := w.castDirected(r, id, 999, 1, 0, now, &pb.GridPoint{X: -1, Y: 0}); code != p.CodeSkillTarget || r.Player.MP != mana || !r.NextSpell.IsZero() || len(r.Casts) != 0 {
			t.Fatalf("新技能%d非法目标产生副作用:%d", id, code)
		}
	}
}
