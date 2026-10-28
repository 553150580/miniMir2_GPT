package game

import (
	"minimir/internal/devconfig"
	"testing"
	"time"
)

func TestTravelReturnDoesNotCreateNewCombatBeforeResourceACK(t *testing.T) {
	w, r, _, now := travelWorker(t)
	original := r.Scene
	prepareTravelAt(w, r, 201, travelToTown, now)
	confirmTravelAt(w, r, 201, original, false, now)
	if !w.finishMapTravelAck(r, now) {
		t.Fatal("城镇ACK拒绝")
	}
	r.Ready = true
	prepareTravelAt(w, r, 202, travelToPersonal, now)
	confirmTravelAt(w, r, 202, r.Scene, false, now)
	if r.MapID != 1 || r.Ready || r.Travel.Acked {
		t.Fatal("未进入新个人场景待ACK阶段")
	}
	r.Player.ActionReadyAt = now
	m := r.Monsters[0]
	m.Pos = devconfig.Point{X: r.Player.Pos.X + 1, Y: r.Player.Pos.Y}
	m.HP = m.MaxHP
	m.NextAttack = now
	m.ActionReadyAt = now
	playerAttack, playerCast, monsterAttack := r.Player.AttackSequence, r.Player.CastSequence, m.AttackSequence
	w.step(r, now.Add(100*time.Millisecond))
	if r.Player.AttackSequence != playerAttack || r.Player.CastSequence != playerCast || m.AttackSequence != monsterAttack || len(r.Casts) != 0 || len(r.Attacks) != 0 {
		t.Fatal("资源ACK之前新增战斗事件")
	}
	if !w.finishMapTravelAck(r, now.Add(200*time.Millisecond)) {
		t.Fatal("个人图ACK拒绝")
	}
	r.Ready = true
	w.step(r, now.Add(300*time.Millisecond))
	if r.Player.AttackSequence == playerAttack && r.Player.CastSequence == playerCast && m.AttackSequence == monsterAttack {
		t.Fatal("ACK后未恢复原战斗现场")
	}
}
