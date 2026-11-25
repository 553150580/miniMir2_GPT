// 本文件从真实战斗、原怪物掉落池和 MongoDB 回执检查规则30普通收益闭环。
package integration

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

const fieldFixtureLevel = 45 // 验收资格角色等级；不代表正常成长用时。

// fieldDropCase 保存工作簿原有地图、目标池、可装备实例及可直接用的原药品候选。
type fieldDropCase struct {
	MapID     uint64                      // 原怪物所驻留的独立普通地图。
	Monster   devconfig.Monster           // 穿透完整战斗轨迹预期确认的怪物身份和经验。
	PoolID    uint32                      // MongoDB 原掉落可追溯的池编号。
	Equipment devconfig.OrdinaryDropEntry // 普通击杀保底产生的可用装备条目。
	Potion    devconfig.OrdinaryDropEntry // 普通道具分类内的可用蓝药条目。
}

// selectFieldDropCase 从默认候选选出已有的怪物、装备和药品，不构造新资产定义。
func selectFieldDropCase(t *testing.T, fixture *devconfig.Fixture) fieldDropCase {
	t.Helper()
	for _, m := range fixture.Maps {
		if m.SceneKindValue() != devconfig.SceneKindPersonal {
			continue
		}
		for _, monsterID := range m.MonsterSpawnIDs {
			monster, ok := fixture.MonsterByID(monsterID)
			if !ok {
				continue
			}
			pool, ok := fixture.OrdinaryDropPool(monster.DropPoolID)
			if !ok {
				continue
			}
			var equipment, potion devconfig.OrdinaryDropEntry
			for _, row := range pool.Equipment {
				definition, known := fixture.Item(row.ItemID)
				if !known || !definition.IsEquipment || definition.DropQuality > 1 || definition.RequiredLevel != nil && *definition.RequiredLevel > fieldFixtureLevel ||
					len(definition.AllowedJobs) != 0 && !slices.Contains(definition.AllowedJobs, 1) {
					continue
				}
				equipment = row
				if !slices.Contains(fixture.Drops.EquipmentItemIDs, row.ItemID) {
					break
				}
			}
			for _, row := range pool.Items {
				if effect, known := fixture.Potions.Effect(row.ItemID); known && effect.MP > 0 {
					potion = row
					break
				}
			}
			if equipment.ItemID != 0 && potion.ItemID != 0 {
				return fieldDropCase{MapID: m.ID, Monster: *monster, PoolID: pool.ID, Equipment: equipment, Potion: potion}
			}
		}
	}
	t.Fatal("真实第二张野外图需同时拥有可供法师装备的普通候选和已绑定使用效果的魔法药")
	return fieldDropCase{}
}

// qualifyFieldDropCase 在独立服缩小子池、降低怪物生命并聚拢刷怪点。类别概率、保底和复活时长沿用原候选。
func qualifyFieldDropCase(t *testing.T, fixture *devconfig.Fixture, selected fieldDropCase) {
	t.Helper()
	// 这个用例固定旧普通装备来源。高品质额度由使用对应档位和账本的专项覆盖。
	fixture.OrdinaryEquipmentQuotas = nil
	for i := range fixture.OrdinaryDropPools {
		pool := &fixture.OrdinaryDropPools[i]
		if pool.ID == selected.PoolID {
			pool.Equipment = []devconfig.OrdinaryDropEntry{selected.Equipment}
			pool.Items = []devconfig.OrdinaryDropEntry{selected.Potion}
		} else {
			pool.Equipment = slices.DeleteFunc(pool.Equipment, func(entry devconfig.OrdinaryDropEntry) bool {
				item, found := fixture.Item(entry.ItemID)
				return found && item.DropQuality > 1
			})
		}
	}
	monster, _ := fixture.MonsterByID(selected.Monster.ID)
	monster.HP, monster.MoveMS, monster.AttackMin, monster.AttackMax = 1, 600000, 0, 0
	monster.MagicResist, monster.MagicDefenceMin, monster.MagicDefenceMax = 0, 0, 0
	m, _ := fixture.MapByID(selected.MapID)
	// 不覆盖主地图；默认多怪谱由独立联机用例验证。
	positions := make([]devconfig.Point, 0, len(m.MonsterSpawnIDs))
	for radius := 1; radius <= 3 && len(positions) < cap(positions); radius++ {
		for y := -radius; y <= radius; y++ {
			for x := -radius; x <= radius && len(positions) < cap(positions); x++ {
				if max(max(x, -x), max(y, -y)) != radius {
					continue
				}
				point := devconfig.Point{X: m.PlayerSpawn.X + x, Y: m.PlayerSpawn.Y + y}
				if point != m.PetSpawn && m.Walkable(point) {
					positions = append(positions, point)
				}
			}
		}
	}
	if len(positions) != len(m.MonsterSpawnIDs) {
		t.Fatal("独立掉落验收图没有足够的可选测试刷怪格")
	}
	m.MonsterSpawns = positions
	for i := range m.MonsterSpawnIDs {
		m.MonsterSpawnIDs[i] = monster.ID
	}
}

// TestFirstLoopM1OrdinaryDropPoolsMongoWS 验证正常击杀来源、跨图20次保证、满包和原操作重放。
func TestFirstLoopM1OrdinaryDropPoolsMongoWS(t *testing.T) {
	cfg := m1FieldConfig(t)
	selection := selectFieldDropCase(t, &cfg.Fixture)
	qualifyFieldDropCase(t, &cfg.Fixture, selection)
	must(t, cfg.Fixture.Validate())
	cfg, ctx, store := m1FieldServices(t, cfg)
	fireball, _ := cfg.Fixture.Skill(1)
	area, _ := cfg.Fixture.Skill(23)
	newHunter := func(t *testing.T, pity int, full bool) (*client, domain.Role) {
		t.Helper()
		endpoint, token, key := m1FieldRole(t, cfg, 1)
		id := domain.Hex(key.RoleId)
		original, err := store.Role(ctx, id)
		must(t, err)
		// 仅控制独立角色的战斗、保底前态和容量，不提前写收益、掉落实例或回执。
		start := bson.M{"level": fieldFixtureLevel, "learned_skills": []domain.LearnedSkill{{SkillID: fireball.ID}, {SkillID: area.ID}}, "no_equipment_kills": pity}
		if full {
			var protected domain.Item
			for _, item := range original.Items {
				if item.ItemID != selection.Equipment.ItemID {
					protected = item
					break
				}
			}
			if protected.ID == "" {
				t.Fatal("缺少可保留的原建角物品")
			}
			protected.Locked = true
			start["capacity"], start["items"] = 1, []domain.Item{protected}
		}
		_, err = store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": start})
		must(t, err)
		c := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
		t.Cleanup(c.close)
		c.auto(false, 1)
		travelToBaseline(c, 2, selection.MapID)
		travelAcknowledge(c, false)
		before, err := store.Role(ctx, id)
		must(t, err)
		if before.NoEquipmentKills != pity || before.SourceSeq != original.SourceSeq {
			t.Fatal("新普通图入场重置原角色保底或推进了收益来源", before.NoEquipmentKills, before.SourceSeq)
		}
		return c, before
	}
	guarantee := cfg.Fixture.Drops.EquipmentGuarantee

	t.Run("跨图第20杀掉新怪装备，穿戴、重放和随机复活", func(t *testing.T) {
		c, before := newHunter(t, guarantee-1, false)
		monster, shot := fieldKillOnce(t, c, fireball, selection)
		death := time.Now()
		after := fieldWaitSettlement(t, ctx, store, before.ID, before.SourceSeq+1)
		receipt := fieldCheckDropReceipt(t, ctx, store, after, selection)
		if after.NoEquipmentKills != 0 || !receipt.Settlement.GrantedItem {
			t.Fatal("第20杀未消费角色共用保底并确认装备", after.NoEquipmentKills, receipt.Settlement)
		}
		c.inventoryAt(uint64(after.Revision))
		item := fieldDroppedItem(c, selection.Equipment.ItemID, after.SourceSeq)
		if item == nil || !item.Bound || item.Quantity != 1 {
			t.Fatal("未收到本怪本池新掉落的绑定装备实例", item)
		}
		d, _ := cfg.Fixture.Item(item.ItemId)
		equip := &pb.EquipRequest{Operation: c.operation(), ItemInstanceId: item.InstanceId, ItemVersion: item.Version, Slot: d.EquipmentSlot}
		if result := c.asset(p.MsgC2SEquip, equip); result.ErrorCode != 0 {
			t.Fatal("正常穿戴实际新怪掉落装备失败", result)
		}
		beforeReplay, err := store.Role(ctx, before.ID)
		must(t, err)
		if result := c.asset(p.MsgC2SEquip, equip); result.ErrorCode != 0 {
			t.Fatal("原装备操作回执不可重放", result)
		}
		worn, err := store.Role(ctx, before.ID)
		must(t, err)
		if worn.Revision != beforeReplay.Revision || !slices.ContainsFunc(worn.Equipped, func(row domain.Equipped) bool { return row.Item.ID == domain.Hex(item.InstanceId) }) {
			t.Fatal("原装备操作重放重复修改角色，或未保留掉落的原实例")
		}
		// 重放原战斗输入序号不能重复杀同一出生。同时实际站在旧怪格等待新出生避免重叠。
		repeated := c.send(p.MsgC2SCastIntent, shot)
		travelExpectError(c, p.MsgC2SCastIntent, repeated, p.CodeStaleInput)
		c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: monster.Position})
		travelUntilWithin(c, 8*time.Second, func(frame wire.Frame) bool {
			return frame.ID == p.MsgS2CSceneBaseline && proto.Equal(c.player().Position, monster.Position) && c.scene.GetNavigationState().GetStatus() == 2
		})
		travelUntilWithin(c, 13*time.Second, func(frame wire.Frame) bool {
			return frame.ID == p.MsgS2CSceneBaseline && c.entity(monster.EntityHandle).GetHp() > 0
		})
		reborn := c.entity(monster.EntityHandle)
		m, _ := cfg.Fixture.MapByID(selection.MapID)
		point := devconfig.Point{X: int(reborn.Position.X), Y: int(reborn.Position.Y)}
		if reborn.Life != monster.Life+1 || reborn.DefinitionId != monster.DefinitionId || !m.Walkable(point) ||
			proto.Equal(reborn.Position, monster.Position) || time.Since(death) < 9*time.Second {
			t.Fatal("真实新图怪物未在正确期限、空格和原模板下复活", reborn)
		}
		final, err := store.Role(ctx, before.ID)
		must(t, err)
		if final.SourceSeq != after.SourceSeq || final.NoEquipmentKills != 0 {
			t.Fatal("已确认源在攻击重传或复活时重复结算", final.SourceSeq, final.NoEquipmentKills)
		}
	})

	t.Run("满包保底只弃新增装备，经验和来源依旧提交", func(t *testing.T) {
		c, before := newHunter(t, guarantee-1, true)
		fieldKillOnce(t, c, fireball, selection)
		after := fieldWaitSettlement(t, ctx, store, before.ID, before.SourceSeq+1)
		receipt := fieldCheckDropReceipt(t, ctx, store, after, selection)
		details := receipt.Settlement
		if details.GrantedItem || details.Recycling.DiscardedCount != 1 || details.Recycling.DiscardReason != "bag_full" ||
			after.NoEquipmentKills != 0 || after.LastDiscardItem != selection.Equipment.ItemID || len(after.Items) != 1 || after.Items[0] != before.Items[0] || !after.PickupStopped {
			t.Fatalf("满包未保留原件、弃物原因和保底归零：背包%d，弃物%d，保底%d，回执%+v", len(after.Items), after.LastDiscardItem, after.NoEquipmentKills, details)
		}
		// PickupStopped 是拾取状态，继续增加一个怪位并自动击杀确认它不会停战或吞经验。
		c.auto(true, 2)
		next := fieldWaitSettlement(t, ctx, store, before.ID, after.SourceSeq+1)
		fieldCheckDropReceipt(t, ctx, store, next, selection)
		if !next.PickupStopped || len(next.Items) != 1 || next.Items[0] != before.Items[0] || !c.hosting.GetAutoBattleEnabled() {
			t.Fatal("后续满包实际击杀未继续，或原物品发生变化")
		}
	})

	t.Run("药材和新增装备可从对应城镇商店正常购买", func(t *testing.T) {
		endpoint, token, key := m1FieldRole(t, cfg, 1)
		roleID := domain.Hex(key.RoleId)
		// 仅准备前置等级与钱包，用真实 NPC 报价及操作交付全部新商品。
		_, err := store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"level": fieldFixtureLevel, "wallet": domain.Wallet{BoundGold: 1000000}}})
		must(t, err)
		c := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
		t.Cleanup(c.close)
		c.auto(false, 1)
		materials := []uint32{95, 96, 97}
		for _, itemID := range materials {
			buyTaoistSupplyAtVendor(c, &cfg.Fixture, itemID, 2)
		}
		buyTaoistSupplyAtVendor(c, &cfg.Fixture, selection.Equipment.ItemID, 1)
		persisted, err := store.Role(ctx, roleID)
		must(t, err)
		for _, itemID := range append(materials, selection.Equipment.ItemID) {
			if !slices.ContainsFunc(persisted.Items, func(item domain.Item) bool {
				return item.ItemID == itemID && item.SourceKind == "npc_shop" && item.SourceID != "" && item.Bound
			}) {
				t.Fatalf("商品 %d 缺少真实商店来源或入包绑定", itemID)
			}
		}
	})

	t.Run("怪物池按真实概率掉蓝药，按原实例用药并核对重放", func(t *testing.T) {
		c, before := newHunter(t, 0, false)
		// 不修改10%道具分类概率。八槽自动击杀后，从带原怪来源的实物确认落账。
		// 公共 join 的标准场景时限为80秒；为低于10%的自然长尾留出独立用例时钟。
		observation, stop := context.WithTimeout(ctx, 170*time.Second)
		defer stop()
		c.ctx = observation
		c.auto(true, 8)
		var potion *pb.ItemView
		var after domain.Role
		travelUntilWithin(c, 160*time.Second, func(frame wire.Frame) bool {
			if frame.ID != p.MsgS2CInventory {
				return false
			}
			var err error
			after, err = store.Role(ctx, before.ID)
			must(t, err)
			for _, item := range c.inventory.Items {
				if item.ItemId == selection.Potion.ItemID && item.SourceKind == "monster_drop" {
					potion = proto.Clone(item).(*pb.ItemView)
					return true
				}
			}
			return false
		})
		c.auto(false, 8)
		// 自动指令已经提交的最后动作仍可结算，等实际技能动作和经济队列收束后用药。
		time.Sleep(time.Duration(fireball.GlobalSpellMS+fireball.DelayBaseMS+1000) * time.Millisecond)
		barrier := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 8})
		c.expectError(barrier, p.CodeInvalidArgument)
		if potion.Quantity < selection.Potion.QuantityMin || potion.Quantity > selection.Potion.QuantityMax || !potion.Bound || after.SourceSeq <= before.SourceSeq {
			t.Fatal("药水掉落未保留原池数量、绑定与来源", potion)
		}
		dropKey := "settle:" + before.ID + ":" + potion.SourceId
		var drop persistence.Operation
		must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": dropKey}).Decode(&drop))
		if drop.Settlement == nil || !drop.Settlement.GrantedItem || drop.Settlement.DropPoolID != selection.PoolID ||
			drop.Settlement.MapID != selection.MapID || drop.Settlement.MonsterID != selection.Monster.ID {
			t.Fatal("实际药品无法追溯所击杀的原地图与怪物掉落池", drop)
		}
		effect, _ := cfg.Fixture.Potions.Effect(potion.ItemId)
		beforeMP := c.player().Mp
		recoveredMP := min(c.player().MpMax, beforeMP+effect.MP)
		if recoveredMP <= beforeMP {
			t.Fatal("受控挂机前态未产生可回补的法力缺口")
		}
		use := &pb.UseItemRequest{Operation: c.operation(), ItemInstanceId: potion.InstanceId, ItemId: potion.ItemId, SceneEpoch: c.entry.SceneEpoch}
		used := c.asset(p.MsgC2SUseItem, use)
		if used.ErrorCode != 0 {
			t.Fatal("真实掉落的蓝药不能使用", used)
		}
		c.inventoryAt(used.NewRevision)
		gotQuantity := func() int64 {
			for _, item := range c.inventory.Items {
				if bytes.Equal(item.InstanceId, potion.InstanceId) {
					return item.Quantity
				}
			}
			return 0
		}
		if gotQuantity() != potion.Quantity-1 {
			t.Fatal("用药并未恰好扣除一个真实掉落实例")
		}
		travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
			return frame.ID == p.MsgS2CSceneBaseline && c.player().Mp == recoveredMP
		})
		if replay := c.asset(p.MsgC2SUseItem, use); replay.ErrorCode != 0 || gotQuantity() != potion.Quantity-1 {
			t.Fatal("药品原操作重传重复扣库存", replay)
		}
		var receipt persistence.Operation
		id := "asset:" + before.ID + ":" + domain.Hex(use.Operation.OpId)
		must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": id}).Decode(&receipt))
		if receipt.Potion == nil || receipt.Potion.ItemID != potion.ItemId || receipt.Potion.MP <= 0 {
			t.Fatal("实际用药未记录原掉落类型和法力恢复回执", receipt)
		}
		count, err := store.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": id})
		must(t, err)
		if count != 1 {
			t.Fatal("药品原操作重传产生重复审计", count)
		}
	})
}

// fieldKillOnce 使用正常施法和原实体句柄产生真实击杀，不通过数据库伪造死亡。
func fieldKillOnce(t *testing.T, c *client, spell devconfig.Skill, selection fieldDropCase) (*pb.EntityState, *pb.CastIntent) {
	t.Helper()
	target := m1LiveMonster(t, c, spell.RangeCells, func(e *pb.EntityState) bool { return e.DefinitionId == selection.Monster.DefinitionID })
	shot := &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: spell.ID, TargetHandle: target.EntityHandle}
	c.send(p.MsgC2SCastIntent, shot)
	travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CSceneBaseline {
			return false
		}
		body := c.entity(target.EntityHandle)
		return body != nil && body.Hp == 0 && body.Life == target.Life
	})
	return proto.Clone(c.entity(target.EntityHandle)).(*pb.EntityState), shot
}

// fieldWaitSettlement 只在限定时间内等待 MongoDB 真正确认的下一条来源。
func fieldWaitSettlement(t *testing.T, ctx context.Context, store *persistence.Store, roleID string, source int64) domain.Role {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for deadline.Err() == nil {
		role, err := store.Role(deadline, roleID)
		must(t, err)
		if role.SourceSeq >= source {
			if role.SourceSeq != source {
				t.Fatal("单次击杀生成多个收益来源", role.SourceSeq, source)
			}
			return role
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("真实 MongoDB 未确认单次击杀")
	return domain.Role{}
}

// fieldCheckDropReceipt 核对原怪、原图、原池与确认经验均进入同一笔真实回执和审计。
func fieldCheckDropReceipt(t *testing.T, ctx context.Context, store *persistence.Store, role domain.Role, selection fieldDropCase) persistence.Operation {
	t.Helper()
	id := fmt.Sprintf("settle:%s:%d", role.ID, role.SourceSeq)
	var receipt persistence.Operation
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": id}).Decode(&receipt))
	details := receipt.Settlement
	if details == nil || details.Kills != 1 || details.MonsterID != selection.Monster.ID || details.MapID != selection.MapID ||
		details.DropPoolID != selection.PoolID || details.Experience == nil || details.Experience.ActualGranted != selection.Monster.Exp {
		t.Fatal("永久回执没有按原图、原怪和掉落池确认资产事实", receipt)
	}
	count, err := store.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": id, "settlement.drop_pool_id": selection.PoolID})
	must(t, err)
	if count != 1 {
		t.Fatal("同一份确认证明未写入唯一掉落池审计", count)
	}
	return receipt
}

// fieldDroppedItem 返回带确切收益序号的新掉落实例，排除旧赠送和已存在的同模板装备。
func fieldDroppedItem(c *client, itemID uint32, source int64) *pb.ItemView {
	for _, item := range c.inventory.Items {
		if item.ItemId == itemID && item.SourceKind == "monster_drop" && item.SourceId == fmt.Sprint(source) {
			return item
		}
	}
	return nil
}
