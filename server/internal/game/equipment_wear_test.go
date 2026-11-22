// 本文件回归角色受伤时护甲磨损与死亡额外耐久扣减的事件边界。
package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestPlayerDamageWearsOneArmorAndDeathAddsOneFixedCost 核验有效来袭应只扣一件防具，用诊断「冻结事务期间丢失原装备扣减或接受空回执」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPlayerDamageWearsOneArmorAndDeathAddsOneFixedCost(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 0)
	w.e.Store.Cfg.Fixture.EquipmentDurability = &devconfig.EquipmentDurabilityRules{WeaponLossU: 1, ArmorHitLossU: 1, DeathLossU: 1000}
	// weapon 建立带初始值的道具，供本段继续处理。
	weapon := domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 3, OriginalDurabilityU: 4000, MaxDurabilityU: 4000, DurabilityU: 1300}
	// armor 初始化用作防具的道具。
	armor := domain.Item{ID: domain.ID(), ItemID: 193, Quantity: 1, Version: 4, OriginalDurabilityU: 10000, MaxDurabilityU: 10000, DurabilityU: 4}
	r.Role.Equipped = []domain.Equipped{{Slot: 0, Item: weapon}, {Slot: 1, Item: armor}}
	r.Player.HP = 2
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	w.applyDamage(r, r.Monsters[0], &r.Player, 0, now)
	if len(r.Wear.open) != 0 {
		t.Fatal("无实际损血事件触发了磨损")
	}
	w.applyDamage(r, r.Monsters[0], &r.Player, 1, now)
	if len(r.Wear.open) != 1 || r.Wear.loss(armor, 1) != 1 || r.Wear.loss(weapon, 0) != 0 || r.Wear.urgent {
		t.Fatal("有效来袭没有只扣一件防具", r.Wear)
	}
	w.applyDamage(r, r.Monsters[0], &r.Player, 100, now.Add(time.Millisecond))
	if r.Player.HP != 0 || r.Wear.loss(armor, 1) != 2 || r.Wear.loss(weapon, 0) != 1000 || r.Wear.deathID != r.DeathID || !r.Wear.urgent {
		t.Fatal("致死未记录且只记录一次原部位死亡成本", r.Wear)
	}
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CDeathNotice {
			t.Fatal("成本待确认时提前宣布死亡决议")
		}
	}
	w.applyDamage(r, r.Monsters[0], &r.Player, 100, now.Add(time.Second))
	if r.Wear.loss(weapon, 0) != 1000 || r.Wear.loss(armor, 1) != 2 {
		t.Fatal("重复来袭重复扣除已死亡角色")
	}
	// q 组装结算所需的数据。
	q := persistence.Settlement{SourceSeq: 1}
	r.Wear.attach(&q)
	if len(q.EquipmentWear) != 2 || q.WearDeathID != r.DeathID || len(r.Wear.open) != 0 || r.Wear.loss(weapon, 0) != 1000 || confirmedWear(q, persistence.Operation{}) {
		t.Fatal("冻结事务期间丢失原装备扣减或接受空回执")
	}
}
