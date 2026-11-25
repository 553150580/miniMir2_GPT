// 本文件验证原技能作用于讨伐链各职业和宠物时的范围、身份、状态数值及观察。
package game

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// dungeonSupportFixture 用原技能目录装配三职业和三个不同出生代次的临时宠物。
func dungeonSupportFixture(t *testing.T) (*worker, *dungeonArena, [3]*room, [3]*capturedConnection, time.Time) {
	t.Helper()
	w, earlier, first, _, _, now := dungeonBattleFixture(t)
	template, grant := earlier.template, earlier.grant
	template.Stages[0].PlayerSpawns = []devconfig.Point{{X: 0}, {X: 0, Y: 1}, {X: 2, Y: 1}}
	template.Stages[0].Enemies[0].Spawn = devconfig.Point{X: 2}
	template.Stages[0].Enemies[1].Spawn = devconfig.Point{X: 2, Y: 2}
	grant.Members = append(grant.Members, persistence.DungeonMember{RoleID: "角色三", AccountID: "账号三", Origin: 1, SessionEpoch: 3})
	a := newDungeonArena(grant, template, now)
	var members [3]*room
	var connections [3]*capturedConnection
	for index, id := range [...]uint32{2, 1, 0} {
		approved := grant.Members[index]
		auth := persistence.Auth{RoleID: approved.RoleID, AccountID: approved.AccountID, SessionEpoch: approved.SessionEpoch}
		role := domain.Role{ID: approved.RoleID, Name: approved.RoleID, Origin: approved.Origin, Job: id, Level: uint32(21 + index*14)}
		fighter := first.bossBattle.fighter
		fighter.Level, fighter.MP, fighter.HP, fighter.TaoMin, fighter.TaoMax = role.Level, 300, 120, 10, 10
		frozen := personalBossSnapshot{Fighter: fighter, Pet: &entity{Kind: 2, Def: 2, HP: 80, MaxHP: 80, Life: uint64(index + 7)}}
		for _, skill := range w.e.Store.Cfg.Fixture.Skills {
			if skill.JobID == role.Job {
				frozen.Skills = append(frozen.Skills, domain.LearnedSkill{SkillID: skill.ID, Level: skill.LearnedLevel})
			}
		}
		connections[index] = &capturedConnection{}
		seat := a.addSeat(role, auth, w.e.Store, frozen, uint64(index+20), uint64(index+30), connections[index])
		if seat == nil || seat.room.Pet == nil {
			t.Fatal("三人和原宠的测试落点装配不完整")
		}
		seat.started, seat.room.Ready, seat.room.Auto = true, true, false
		seat.room.Player.Life = uint64(index + 1)
		seat.room.Player.HP, seat.room.Pet.HP = 80, 40
		members[index] = seat.room
	}
	return w, a, members, connections, now
}

// dungeonTestImpacts 解析原观察者接收的命中，避免只检查同一内存中的角色数值。
func dungeonTestImpacts(t *testing.T, conn *capturedConnection) []*pb.CombatImpact {
	t.Helper()
	var events []*pb.CombatImpact
	for _, frame := range conn.frames {
		if frame.ID == p.MsgS2CCombatImpact {
			event := &pb.CombatImpact{}
			if err := proto.Unmarshal(frame.Payload, event); err != nil {
				t.Fatal(err)
			}
			events = append(events, event)
		}
	}
	return events
}

// TestDungeonBattleSingleHealingFencesPetLife 检查队友既可被真实治疗，也只按原宠物生命命中。
func TestDungeonBattleSingleHealingFencesPetLife(t *testing.T) {
	w, a, members, connections, now := dungeonSupportFixture(t)
	caster, ally := members[0], members[1]
	castAt := now
	if code := w.cast(caster, 2, ally.Player.Handle, 91, castAt); code != 0 {
		t.Fatal("治疗已确认同队角色失败", code)
	}
	w.advanceDungeonEvents(a, caster.Casts[0].Due)
	if ally.Player.HealAmount <= 0 || caster.Player.HealAmount != 0 || ally.Pet.HealAmount != 0 {
		t.Fatal("单体治疗落到了错误身体")
	}
	event := dungeonTestImpacts(t, connections[2])
	if len(event) != 1 || event[0].SceneEpoch != members[2].Scene || event[0].CasterHandle != caster.Player.Handle ||
		event[0].TargetHandle != ally.Player.Handle || event[0].TargetLife != ally.Player.Life || !event[0].Healing {
		t.Fatal("第三个观察者没有收到原治疗的场景、施法者和生命身份", event)
	}
	castAt = castAt.Add(10 * time.Second)
	old := ally.Pet
	if code := w.cast(caster, 2, old.Handle, 92, castAt); code != 0 {
		t.Fatal("治疗同队原活宠失败", code)
	}
	// 只改变目标的出生代次，原动作不得借同一场内句柄治疗替代身体。
	replacement := *old
	replacement.Life++
	ally.Pet = &replacement
	w.advanceDungeonEvents(a, caster.Casts[0].Due)
	event = dungeonTestImpacts(t, connections[2])
	if replacement.HealAmount != 0 || old.HealAmount != 0 || len(event) != 2 || event[1].TargetHandle != old.Handle ||
		event[1].TargetLife != old.Life || event[1].Outcome != 3 || event[1].Amount != 0 {
		t.Fatal("原宠物治疗身份失效后仍覆盖替代身体或返回了错误事件", event)
	}
}

// TestDungeonBattleMassHealingSharesSixBodies 验证实际群疗对三个职业、三个宠物及三端各自场景的完整表现。
func TestDungeonBattleMassHealingSharesSixBodies(t *testing.T) {
	w, a, members, connections, now := dungeonSupportFixture(t)
	caster := members[0]
	if code := w.cast(caster, 29, 0, 61, now, pointPB(devconfig.Point{X: 1, Y: 1})); code != 0 {
		t.Fatal("讨伐群疗未正常受理", code)
	}
	cast := caster.Casts[0]
	w.advanceDungeonEvents(a, cast.Due)
	want := map[uint32]uint64{}
	for _, r := range members {
		for _, body := range []*entity{&r.Player, r.Pet} {
			want[body.Handle] = body.Life
			if body.HealAmount <= 0 {
				t.Fatal("范围内原身体缺少群疗队列", body.Handle)
			}
		}
		if !w.tickHealing(r, cast.Due) || r.Player.HP <= 80 || r.Pet.HP <= 40 {
			t.Fatal("各角色原时钟未消化自己和宠物的治疗")
		}
	}
	for _, enemy := range a.monsters {
		if enemy.HealAmount != 0 {
			t.Fatal("群疗错误作用同场敌人")
		}
	}
	for index, conn := range connections {
		accepts := 0
		for _, frame := range conn.frames {
			if frame.ID == p.MsgS2CCastAccepted {
				accepts++
				accepted := &pb.CastAccepted{}
				if err := proto.Unmarshal(frame.Payload, accepted); err != nil || accepted.SceneEpoch != members[index].Scene ||
					accepted.CastId != cast.ID || accepted.CasterHandle != caster.Player.Handle || accepted.CasterLife != caster.Player.Life ||
					(index == 0 && accepted.InputSequence != 61 || index != 0 && accepted.InputSequence != 0) {
					t.Fatal("群疗施法接受事实未保留真实身体或正确替换观察附着", accepted, err)
				}
			}
		}
		events := dungeonTestImpacts(t, conn)
		if accepts != 1 || len(events) != len(want) {
			t.Fatal("观察端收到重复或不完整群疗动作", accepts, len(events))
		}
		seen := map[uint32]bool{}
		for _, event := range events {
			life, exists := want[event.TargetHandle]
			if !exists || seen[event.TargetHandle] || event.SceneEpoch != members[index].Scene || event.CastId != cast.ID ||
				event.CasterHandle != caster.Player.Handle || event.TargetLife != life || !event.Healing || event.Amount <= 0 {
				t.Fatal("观察端群疗目标及出生代次错误", event)
			}
			seen[event.TargetHandle] = true
		}
	}
}

// TestDungeonBattleSupportUsesOriginalRecipientStats 覆盖原群体效果衔接，并保留实际耗材门禁。
func TestDungeonBattleSupportUsesOriginalRecipientStats(t *testing.T) {
	w, a, members, connections, now := dungeonSupportFixture(t)
	caster := members[0]
	point := devconfig.Point{X: 1, Y: 1}
	for _, id := range []uint32{14, 15} {
		skill, known := w.e.Store.Cfg.Fixture.Skill(id)
		definition, valid := w.e.Store.Cfg.Fixture.Buff(skill.BuffID)
		before := caster.Player.MP
		if !known || !valid || skill.MaterialQuantity == 0 ||
			w.cast(caster, id, 0, 17, now, pointPB(point)) != p.CodeSkillStateBlocked || caster.Player.MP != before || len(caster.Casts) != 0 {
			t.Fatal("讨伐链未保留原角色材料消耗门禁", id)
		}
		// 单独推动已授权效果以核验群体解析、真实受益属性和基线。它不代表副本内可绕过护符。
		duration := buffDurationSeconds(skill, definition, skill.LearnedLevel, 10, 0) * int64(time.Second/time.Millisecond)
		authorized := pendingCast{Scene: caster.Scene, Skill: skill, SkillLevel: skill.LearnedLevel, Caster: reference(caster, &caster.Player),
			TargetPoint: point, Due: now, BuffDurationMS: duration}
		caster.Casts = append(caster.Casts, authorized)
		w.advanceDungeonEvents(a, now)
		for _, member := range members {
			for _, body := range []*entity{&member.Player, member.Pet} {
				status := body.Statuses[skill.BuffID]
				want := statusTargetBonus(w.fighter(member, body).Level, definition)
				if status == nil || status.Caster != authorized.Caster || status.TargetLife != body.Life ||
					status.MaxMagicDefenceBonus+status.MaxPhysicalDefenceBonus != want {
					t.Fatal("群体属性未采用受益者本人或宠物的原属性", id, body.Handle, status, want)
				}
			}
		}
		for index, conn := range connections {
			var baseline pb.SceneBaselineChunk
			for _, frame := range conn.frames {
				if frame.ID == p.MsgS2CSceneBaseline {
					if err := proto.Unmarshal(frame.Payload, &baseline); err != nil {
						t.Fatal(err)
					}
				}
			}
			seen := 0
			for _, body := range baseline.Entities {
				for _, status := range body.Statuses {
					if status.StatusId == skill.BuffID && status.CasterHandle == caster.Player.Handle && status.CasterLife == caster.Player.Life {
						seen++
					}
				}
			}
			if baseline.SceneEpoch != members[index].Scene || seen != 6 {
				t.Fatal("原群体状态未完整送达各观察者", id, index, seen)
			}
			conn.frames = nil
		}
		now = now.Add(time.Second)
	}
}

// TestDungeonBattleAutoHealingChoosesTeamCluster 让自身与队友分布在不同小区，验证自动群疗和首 ACK 门。
func TestDungeonBattleAutoHealingChoosesTeamCluster(t *testing.T) {
	w, a, members, _, now := dungeonSupportFixture(t)
	caster, first, second := members[0], members[1], members[2]
	w.advanceDungeonStage(a, now)
	caster.Pet.HP = caster.Pet.MaxHP
	first.Player.Pos, first.Pet.Pos = devconfig.Point{X: 4, Y: 1}, devconfig.Point{X: 4, Y: 2}
	second.Player.Pos, second.Pet.Pos = devconfig.Point{X: 5, Y: 1}, devconfig.Point{X: 5, Y: 2}
	a.seats[second.Role.ID].started = false
	caster.Role.SkillSlotIDs = []uint32{29, 0, 0, 0}
	if !w.autoSkill(caster, now) || len(caster.Casts) != 1 || caster.Casts[0].TargetPoint != first.Player.Pos {
		t.Fatal("自动群疗未选择满足原触发门槛的友方小区")
	}
	w.advanceDungeonEvents(a, caster.Casts[0].Due)
	if first.Player.HealAmount == 0 || first.Pet.HealAmount == 0 || caster.Player.HealAmount != 0 ||
		second.Player.HealAmount != 0 || second.Pet.HealAmount != 0 {
		t.Fatal("自动群疗作用错小区或提前计入尚未确认的角色")
	}
}
