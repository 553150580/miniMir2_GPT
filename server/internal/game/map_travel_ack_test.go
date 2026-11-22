// 本文件回归换图返回后，服务端等待新场景资源确认才开放新增战斗。
package game

import (
	"minimir/internal/devconfig"
	"testing"
	"time"
)

// TestTravelReturnDoesNotCreateNewCombatBeforeResourceACK 核对「未进入新个人场景待ACK阶段」与「资源ACK之前新增战斗事件」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTravelReturnDoesNotCreateNewCombatBeforeResourceACK(t *testing.T) {
	// w 接收地图切换房间执行器供后续步骤使用，r 接收地图切换房间执行器供后续步骤使用，now 保存由地图切换房间执行器取得的当前时间。
	w, r, _, now := travelWorker(t)
	// original 保存场景，用于当前步骤中的原始。
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
	r.Role.PickupStopped = true // 容量修复不放开新场景资源ACK前的战斗。
	r.Player.ActionReadyAt = now
	// m 保存从怪物集合取出的怪物集合中的指定项。
	m := r.Monsters[0]
	m.Pos = devconfig.Point{X: r.Player.Pos.X + 1, Y: r.Player.Pos.Y}
	m.HP = m.MaxHP
	m.NextAttack = now
	m.ActionReadyAt = now
	// playerAttack 保存攻击序号，用于当前步骤中的玩家攻击，playerCast 保存施法序号，用于当前步骤中的玩家施法。
	// monsterAttack 保存攻击序号，用于当前步骤中的怪物攻击。
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
