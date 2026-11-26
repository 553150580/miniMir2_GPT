// 本文件用真实 WebSocket 对照穿脱时的角色幸运属性及实际命中金额。
package integration

import (
	"encoding/json"
	"os"
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

// TestLuckyCombatMongoWS 从正常穿戴入口确认幸运、当前目标减免和卸下后的原属性。
func TestLuckyCombatMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	raw, err := os.ReadFile(os.Getenv("MINIMIR_LUCK_TEST_FIXTURE"))
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	if fixture.CombatLuck == nil || fixture.Forge == nil || fixture.CombatLuck.WeaponWingPPM[3] != 44_000 ||
		fixture.CombatLuck.OtherPPM[3] != 44_500 || fixture.CombatLuck.RoleResistancePPM[1] != 200_000 {
		t.Fatal("真实工作簿投影缺少预期的幸运分档或隔离抵抗")
	}
	// 一次真实命中即可验证最大值分支。只在本测试强制幸运武器必成，保留命中与三点防御公式。
	fixture.CombatLuck.WeaponWingPPM[3] = 1_000_000
	delete(fixture.CombatLuck.MonsterResistancePPM, 1)
	position := devconfig.Point{X: fixture.Map.PlayerSpawn.X + 1, Y: fixture.Map.PlayerSpawn.Y}
	if !fixture.Map.Walkable(position) {
		t.Fatal("专项普通图没有向东的可攻击邻格", position)
	}
	configureMap := func(m *devconfig.MapTemplate) {
		for index, existing := range m.MonsterSpawns {
			if index > 0 && existing == position {
				m.MonsterSpawns[index] = m.MonsterSpawns[0]
				break
			}
		}
		m.MonsterSpawns[0], m.MonsterSpawnIDs[0], m.InitialMonsterCount = position, 1, 1
	}
	configureMap(&fixture.Map)
	for index := range fixture.Maps {
		if fixture.Maps[index].ID == fixture.Map.ID {
			configureMap(&fixture.Maps[index])
		}
	}
	for index := range fixture.Monsters {
		m := &fixture.Monsters[index]
		if m.ID == 1 {
			m.HP, m.Agility, m.MoveMS = 10_000, 0, 60_000
			m.AttackMin, m.AttackMax, m.DefenceMin, m.DefenceMax = 0, 0, 3, 3
		}
	}
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) { cfg.Fixture = fixture })
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "luck_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	role := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: route.origin.OriginID,
		Slot: 1, Job: 1, Name: "幸运" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	weapon := persistence.InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1,
		InitialLuck: 3, SourceKind: "monster_drop", SourceID: "1"}, &fixture)
	recipe := fixture.Forge.Recipes[0]
	necklace := persistence.InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: recipe.OutputItemID, Quantity: 1, Version: 1,
		InitialLuck: 3, Quality: recipe.Quality, SourceKind: "forge_craft", SourceID: domain.ID()}, &fixture)
	_, err = store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": bson.M{
		"level": uint32(20), "items": []domain.Item{weapon, necklace}, "equipped": []domain.Equipped{},
	}})
	must(t, err)
	ticket := directoryTicket(t, endpoint, login.AccountToken, role.Key, route)
	c := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
	defer c.close()
	t.Log("隔离角色已确认首场景")
	auto := c.send(p.MsgC2SAutoBattle, &pb.AutoBattleIntent{SceneEpoch: c.entry.SceneEpoch, Enabled: false, MonsterCount: 1})
	response := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == auto })
	if response.Error != 0 || response.ID != p.MsgS2CHostingState || c.hosting.GetAutoBattleEnabled() {
		t.Fatal("无法停止隔离场的自动攻击", response.ID, response.Error)
	}
	luckyAttributes(c, 0, 200_000)
	t.Log("无幸运装备的属性及角色抵抗已确认")
	if code := luckyEquip(c, necklace.ID, 3, 1, false); code != 0 {
		t.Fatal("项链穿戴失败", code)
	}
	luckyAttributes(c, 44_500, 200_000)
	if code := luckyEquip(c, weapon.ID, 0, 1, false); code != 0 {
		t.Fatal("武器穿戴失败", code)
	}
	luckyAttributes(c, 1_000_000, 200_000)
	var wornVersion uint64
	for _, entry := range c.inventory.Equipped {
		if entry.Slot == 0 {
			wornVersion = entry.Item.Version
		}
	}
	if code := luckyEquip(c, weapon.ID, 0, wornVersion, true); code != 0 {
		t.Fatal("已确认的武器无法正常卸下", code)
	}
	luckyAttributes(c, 44_500, 200_000)
	var storedVersion uint64
	for _, item := range c.inventory.Items {
		if domain.Hex(item.InstanceId) == weapon.ID {
			storedVersion = item.Version
		}
	}
	if code := luckyEquip(c, weapon.ID, 0, storedVersion, false); code != 0 {
		t.Fatal("使用当前背包版本穿回原武器失败", code)
	}
	maximum := luckyAttributes(c, 1_000_000, 200_000).PhysicalAttackMax
	if maximum <= 3 {
		t.Fatal("该测试攻击区间无法覆盖明确的防御差", maximum)
	}
	target := travelEntity(c.scene, 3)
	if target == nil || target.DefinitionId != 1 || target.Position.X != int32(position.X) || target.Position.Y != int32(position.Y) {
		t.Fatal("隔离场没有命中测试所需的同生命目标", target)
	}
	attack := c.send(p.MsgC2SAttackIntent, &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, InputSequence: 1, Direction: 2})
	var accepted pb.AttackAccepted
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		if frame.Request == attack && frame.Error != 0 {
			return true
		}
		if frame.ID != p.MsgS2CAttackAccepted {
			return false
		}
		must(t, proto.Unmarshal(frame.Payload, &accepted))
		return accepted.InputSequence == 1 && accepted.CasterHandle == c.entry.PlayerHandle
	})
	if frame.Error != 0 {
		t.Fatal("真实普攻没有受理", frame.Error)
	}
	var impact pb.CombatImpact
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CCombatImpact {
			return false
		}
		must(t, proto.Unmarshal(frame.Payload, &impact))
		return impact.AttackSequence == accepted.AttackSequence && impact.TargetHandle == target.EntityHandle && impact.CasterHandle == c.entry.PlayerHandle
	})
	if impact.Amount != maximum-3 || impact.TargetLife != target.Life {
		t.Fatal("真实幸运一击没有用原攻击区间上限再扣三点防御", maximum, &impact)
	}
	current, err := store.Role(ctx, roleID)
	must(t, err)
	if len(current.Equipped) != 2 || current.Equipped[0].Item.ID != necklace.ID || current.Equipped[0].Item.InitialLuck != 3 ||
		current.Equipped[1].Item.ID != weapon.ID || current.Equipped[1].Item.InitialLuck != 3 {
		t.Fatal("穿卸后未保留两件原实例和幸运", current.Equipped)
	}
	t.Logf("真实普攻区间上限 %d，目标防御 3，已确认伤害 %d；此前卸下武器后幸运一击降至 4.45%%", maximum, impact.Amount)
}

// luckyEquip 通过普通资产命令穿戴或卸下选中的原实例。
func luckyEquip(c *client, instance string, slot uint32, version uint64, unequip bool) uint32 {
	c.t.Helper()
	request := c.send(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: domain.Bytes(instance), ItemVersion: version,
		Slot: slot, Unequip: unequip, Operation: c.operation()})
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == request })
	if frame.Error != 0 || frame.ID != p.MsgS2COperationResult {
		c.t.Fatal("装备入口没有受理", frame.ID, frame.Error)
	}
	var result pb.OperationResult
	must(c.t, proto.Unmarshal(frame.Payload, &result))
	if result.Pending {
		c.t.Fatal("角色装备确认失败", &result)
	}
	c.watermark = max(c.watermark, result.CommandSeq)
	if c.inventory.GetRevision() < result.NewRevision {
		travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
			return frame.ID == p.MsgS2CInventory && c.inventory.GetRevision() >= result.NewRevision
		})
	}
	return result.ErrorCode
}

// luckyAttributes 等待同一附着的真实属性基线，忽略先前场景或旧资源观察。
func luckyAttributes(c *client, chance, resist uint32) *pb.CombatAttributes {
	c.t.Helper()
	current := func() *pb.CombatAttributes {
		for _, entity := range c.scene.GetEntities() {
			if entity.EntityHandle == c.entry.PlayerHandle {
				return entity.CombatAttributes
			}
		}
		return nil
	}
	matches := func() bool {
		value := current()
		return value.GetLuckEnabled() && value.GetLuckAttackPpm() == chance && value.GetLuckResistancePpm() == resist
	}
	if !matches() {
		travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.ID == p.MsgS2CSceneBaseline && matches() })
	}
	return current()
}
