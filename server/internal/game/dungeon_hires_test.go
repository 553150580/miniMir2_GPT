// 本文件核对佣兵只占共享战斗的空席，并沿真实职业动作参与四段战斗。
package game

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestDungeonHireSharedCast 用两个原真人与一位法师佣兵走同场首次确认、施法与权威命中。
func TestDungeonHireSharedCast(t *testing.T) {
	w, arena, first, second, peer, now := dungeonBattleFixture(t)
	option := devconfig.DungeonHireOption{ID: 8, DisplayName: "法师佣兵", Job: 1, Level: 15, FeeGold: 25,
		Skills: []devconfig.DungeonHireSkill{{ID: 1, Rank: 0}}}
	arena.template.Hires = []devconfig.DungeonHireOption{option}
	arena.grant.HireOptionIDs = []uint32{option.ID}
	if !w.addDungeonHires(arena) || len(arena.hireSeats) != 1 || w.addDungeonHires(arena) {
		t.Fatal("只应在原开战快照后一次装配一位佣兵")
	}
	hire := arena.hireSeats[0].room
	if len(arena.grant.Members) != 2 || len(arena.observations) != 0 || hire.Auth != (persistence.Auth{}) ||
		hire.Conn != nil || hire.Pet != nil || !hire.Auto || hire.Player.Handle != 3 || hire.Player.Def != option.Job ||
		hire.Role.Job != option.Job || hire.Role.Level != option.Level || hire.Role.ID == "" ||
		w.rooms[hire.Role.ID] != nil || !arena.occupied(hire.Player.Pos, nil) {
		t.Fatal("佣兵被当成真人、连接、角色 owner 或未占原空格")
	}
	firstFrame := &pb.SceneBaselineChunk{}
	w.appendDungeonViews(first, firstFrame)
	if len(firstFrame.Entities) != 1 || firstFrame.Entities[0].EntityHandle != hire.Player.Handle ||
		firstFrame.Entities[0].DisplayName != option.DisplayName || firstFrame.Entities[0].EntityKind != 1 {
		t.Fatal("首帧没有提供佣兵的真实原职业人形", firstFrame)
	}
	mp, origin := hire.Player.MP, hire.Player.Pos
	arena.seats[first.Role.ID].started, first.Ready = true, true
	w.stepDungeonArena(arena, now)
	if arena.readyToFight() || hire.Player.MP != mp || hire.Player.Pos != origin || len(hire.Casts) != 0 {
		t.Fatal("有真人未确认时佣兵已提前执行动作")
	}
	arena.seats[second.Role.ID].started, second.Ready = true, true
	for _, enemy := range arena.monsters {
		enemy.NextAttack, enemy.NextMove = arena.grant.ExpiresAt, arena.grant.ExpiresAt
	}
	w.stepDungeonArena(arena, now)
	if len(hire.Casts) != 1 || hire.Player.MP >= mp || hire.Player.Pos != origin || hire.Casts[0].Skill.ID != 1 {
		t.Fatal("原法师走蓝量、职业技能与动作时钟后没有正确释放火球", hire.Player, hire.Casts)
	}
	cast := &pb.CastAccepted{}
	for _, frame := range peer.frames {
		if frame.ID == p.MsgS2CCastAccepted && proto.Unmarshal(frame.Payload, cast) != nil {
			t.Fatal("无法读取同场佣兵施法")
		}
	}
	if cast.SceneEpoch != second.Scene || cast.CasterHandle != hire.Player.Handle || cast.SkillId != 1 || cast.InputSequence != 0 ||
		cast.CastId == 0 {
		t.Fatal("真人未收到绑定到自己观察场景的佣兵动作", cast)
	}
	target := arena.monsters[0]
	for _, enemy := range arena.monsters {
		if enemy.Handle == cast.TargetHandle {
			target = enemy
		}
	}
	due := hire.Casts[0].Due
	w.advanceDungeonEvents(arena, due)
	impact := &pb.CombatImpact{}
	for _, frame := range peer.frames {
		if frame.ID == p.MsgS2CCombatImpact && proto.Unmarshal(frame.Payload, impact) != nil {
			t.Fatal("无法读取同场佣兵命中")
		}
	}
	if target.HP >= target.MaxHP || impact.SceneEpoch != second.Scene || impact.CasterHandle != hire.Player.Handle ||
		impact.TargetHandle != target.Handle || impact.CastId != cast.CastId || len(hire.KillFacts) != 0 || hire.Role.SourceSeq != 0 {
		t.Fatal("佣兵命中观察或永久收益围栏错误", impact)
	}
}

// TestDungeonHireFollowsGridAndCannotContinueAfterPartyWipe 确认近战佣兵主动追击与既有真人战败规则。
func TestDungeonHireFollowsGridAndCannotContinueAfterPartyWipe(t *testing.T) {
	w, arena, first, second, _, now := dungeonBattleFixture(t)
	option := devconfig.DungeonHireOption{ID: 10, DisplayName: "战士佣兵", Job: 0, Level: 20, FeeGold: 30}
	arena.template.Hires = []devconfig.DungeonHireOption{option}
	arena.grant.HireOptionIDs = []uint32{option.ID}
	if !w.addDungeonHires(arena) {
		t.Fatal("无法装配合法佣兵")
	}
	for _, member := range arena.grant.Members {
		arena.seats[member.RoleID].started = true
	}
	for _, enemy := range arena.monsters {
		enemy.NextAttack, enemy.NextMove = arena.grant.ExpiresAt, arena.grant.ExpiresAt
	}
	hire := arena.hireSeats[0].room
	origin := hire.Player.Pos
	w.stepDungeonArena(arena, now)
	if distance(hire.Player.Pos, origin) != 1 || !arena.geometry.Walkable(hire.Player.Pos) ||
		hire.Player.MovementRevision != 1 || !hire.Player.NextMove.After(now) || arena.occupied(hire.Player.Pos, &hire.Player) {
		t.Fatal("近战佣兵应沿原占格与行动时钟追击", hire.Player)
	}
	enemy := arena.monsters[0]
	w.applyDungeonDamage(first, enemy, &first.Player, first.Player.HP, now, nil)
	if arena.outcome != persistence.DungeonActive {
		t.Fatal("其余真人仍存活时提前战败")
	}
	w.applyDungeonDamage(second, enemy, &second.Player, second.Player.HP, now, nil)
	if hire.Player.HP <= 0 || arena.outcome != persistence.DungeonFailed || !arena.finishedAt.Equal(now) {
		t.Fatal("所有真人战败后不允许佣兵继续通关")
	}
	previous := hire.Player.MovementRevision
	w.stepDungeonArena(arena, now.Add(time.Second))
	if hire.Player.MovementRevision != previous || len(hire.Attacks)+len(hire.Casts) != 0 {
		t.Fatal("战败后观察到佣兵残留行动")
	}
}

// TestDungeonHireInvalidRosterDoesNotAddSeats 拒绝原名单之外的第四席或已失效的配置编号。
func TestDungeonHireInvalidRosterDoesNotAddSeats(t *testing.T) {
	w, arena, _, _, _, _ := dungeonBattleFixture(t)
	arena.grant.HireOptionIDs = []uint32{8, 9}
	if w.addDungeonHires(arena) || len(arena.hireSeats) != 0 || len(arena.seats) != 2 {
		t.Fatal("佣兵超出三个共享空席仍然被装配")
	}
	arena.grant.HireOptionIDs = []uint32{8}
	if w.addDungeonHires(arena) || len(arena.hireSeats) != 0 || len(arena.seats) != 2 {
		t.Fatal("不属于当前模板的佣兵仍然被装配")
	}
}

// TestDungeonHireQueuesConfiguredWarriorSkill 覆盖不在原默认技能列表中的自动半月刀法。
func TestDungeonHireQueuesConfiguredWarriorSkill(t *testing.T) {
	w, arena, _, _, _, now := dungeonBattleFixture(t)
	const halfMoon = 25
	option := devconfig.DungeonHireOption{ID: 11, DisplayName: "半月佣兵", Job: 0, Level: 35, FeeGold: 40,
		Skills: []devconfig.DungeonHireSkill{{ID: halfMoon, Rank: 0}}}
	arena.template.Hires = []devconfig.DungeonHireOption{option}
	arena.grant.HireOptionIDs = []uint32{option.ID}
	if !w.addDungeonHires(arena) {
		t.Fatal("无法装配已配置刀法的佣兵")
	}
	hire := arena.hireSeats[0].room
	for _, member := range arena.grant.Members {
		arena.seats[member.RoleID].started = true
	}
	for _, enemy := range arena.monsters {
		enemy.NextAttack, enemy.NextMove = arena.grant.ExpiresAt, arena.grant.ExpiresAt
	}
	enemy := arena.monsters[0]
	enemy.Pos = devconfig.Point{X: hire.Player.Pos.X + 1, Y: hire.Player.Pos.Y}
	mp := hire.Player.MP
	w.stepDungeonArena(arena, now)
	if hire.Player.AttackSequence == 0 || hire.Player.AttackSkillID != halfMoon || hire.Player.MP >= mp ||
		len(hire.Attacks) == 0 || hire.Attacks[0].SkillID != halfMoon {
		t.Fatal("已配置的半月应正确占据技能槽并以原魔法和攻击时钟释放", hire.Player, hire.Attacks)
	}
}
