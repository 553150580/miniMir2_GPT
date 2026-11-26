// 本文件从独立普通房间的真实击杀走到分解和武器用油，以 MongoDB 核对每一步的原物品。
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestRecycleOilMongoWS 验证普通怪掉落、满包分解、原回执重连和实际耗油培养都使用同一件装备的绑定归属。
func TestRecycleOilMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	fixture, source, spell := recycleOilFieldFixture(t)
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) { cfg.Fixture = fixture })
	for _, scenario := range []struct {
		name  string
		bound bool
	}{{"绑定装备的祝福油仍绑定", true}, {"非绑装备的祝福油保持非绑", false}} {
		t.Run(scenario.name, func(t *testing.T) {
			login, summary := &pb.LoginResult{}, &pb.RoleSummary{}
			rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "rec_oil_" + domain.ID()[:8], Password: domain.ID()[:16]}, login, 0)
			rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: route.origin.OriginID,
				Slot: 1, Job: 1, Name: "分解" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, summary, 0)
			roleID := domain.Hex(summary.Key.RoleId)
			weapon := seedRecycleOilFieldRole(t, ctx, store, roleID, &fixture, spell, scenario.bound)
			attach := func() *client {
				ticket := directoryTicket(t, endpoint, login.AccountToken, summary.Key, route)
				return join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
			}
			c := attach()
			defer func() { c.close() }()
			c.auto(false, 1)
			initial, err := store.Role(ctx, roleID)
			must(t, err)
			if len(c.inventory.Items) != 0 || c.inventory.Capacity != 1 || initial.NoEquipmentKills != fixture.Drops.EquipmentGuarantee-1 || c.entry.RoomId != source.MapID {
				t.Fatal("隔离角色没有在空背包和真实保底前态入场")
			}
			// 实际的职业技能击杀才会推进装备保底与独立初始幸运抽取。
			corpse, _ := fieldKillOnce(t, c, spell, source)
			afterDrop := fieldWaitSettlement(t, ctx, store, roleID, initial.SourceSeq+1)
			kill := fieldCheckDropReceipt(t, ctx, store, afterDrop, source)
			if c.inventory.GetRevision() < uint64(afterDrop.Revision) {
				c.inventoryAt(uint64(afterDrop.Revision))
			}
			if len(afterDrop.Items) != 1 || len(c.inventory.Items) != 1 || afterDrop.NoEquipmentKills != 0 || !kill.Settlement.GrantedItem ||
				kill.Settlement.ConfigHash != fixture.Hash {
				t.Fatal("实际普通击杀没有把原装备放到最后一个背包格", afterDrop.Items, kill.Settlement)
			}
			dropped := afterDrop.Items[0]
			observed := fieldDroppedItem(c, source.Equipment.ItemID, afterDrop.SourceSeq)
			if dropped.ItemID != source.Equipment.ItemID || dropped.Bound != scenario.bound || !dropped.HasValidDurability() || dropped.DurabilityU <= 0 ||
				dropped.InitialLuck < 1 || dropped.InitialLuck > 3 || kill.Settlement.LuckyDrop == nil || *kill.Settlement.LuckyDrop != dropped ||
				!proto.Equal(observed, dropped.Proto()) {
				t.Fatal("真实怪物掉落实例、初始幸运、耐久或绑定与原库存/永久回执不符", dropped, observed, kill.Settlement.LuckyDrop)
			}
			var killAudit struct {
				EventAt      time.Time `bson:"event_at"`
				Bound        bool      `bson:"bound"`
				TargetHandle uint32    `bson:"target_handle"`
				TargetLife   uint64    `bson:"target_life"`
			}
			must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": kill.ID, "kind": "settlement"}).Decode(&killAudit))
			if killAudit.EventAt.IsZero() || killAudit.Bound != scenario.bound || initial.PrivilegeActiveAt(killAudit.EventAt) == scenario.bound ||
				killAudit.TargetHandle != corpse.EntityHandle || killAudit.TargetLife != corpse.Life {
				t.Fatal("掉落没有按实际受击生命及击杀时刻的原月卡判绑", killAudit)
			}

			// 发送原分解后仅观察数据库提交，不读取这一条 WebSocket 回包，模拟响应遗失。
			scene, binding, position, life := c.entry.SceneEpoch, c.binding, c.entry.Position, c.player().Life
			recycle := &pb.RecycleRequest{ItemInstanceIds: [][]byte{observed.InstanceId}, Operation: c.operation()}
			c.send(p.MsgC2SRecycle, recycle)
			operation := waitRecycleOilOperation(t, ctx, store, roleID, domain.Hex(recycle.Operation.OpId))
			c.close()
			c = attach()
			retainedCorpse := c.entity(corpse.EntityHandle)
			if c.binding == binding || c.entry.SceneEpoch != scene || c.entry.RoomId != source.MapID || !proto.Equal(c.entry.Position, position) ||
				c.player().Life != life || c.hosting.GetAutoBattleEnabled() || retainedCorpse == nil || retainedCorpse.Life != corpse.Life || retainedCorpse.Hp != 0 {
				t.Fatal("原房间、角色/尸体生命或停自动状态在健康重连后变化", c.entry)
			}
			replayed := c.asset(p.MsgC2SRecycle, recycle)
			oil := checkRecycleOilReceipt(t, ctx, store, replayed, operation, afterDrop, dropped, &fixture)
			afterRecycle, err := store.Role(ctx, roleID)
			must(t, err)
			if !slices.Equal(afterRecycle.Items, []domain.Item{oil}) || len(c.inventory.Items) != 1 || !proto.Equal(c.inventory.Items[0], oil.Proto()) {
				t.Fatal("满包分解未用同一格保存原油，或重放复制了产物", afterRecycle.Items, c.inventory.Items)
			}

			// 真实祝福油入口只有这一个原堆可选。关闭授权不能偷用绑定瓶。
			reference := &pb.WeaponOilItemRef{InstanceId: domain.Bytes(weapon.ID), Version: uint64(weapon.Version), Equipped: true}
			viewSession := domain.Bytes(domain.ID())
			if scenario.bound {
				closed := c.send(p.MsgC2SWeaponOilQuote, &pb.WeaponOilQuoteRequest{SceneEpoch: scene, Weapon: reference, UiSessionId: viewSession})
				travelExpectError(c, p.MsgC2SWeaponOilQuote, closed, p.CodeWeaponOilInputs)
			}
			quote := weaponOilQuoteWS(c, reference, viewSession, scenario.bound)
			if !proto.Equal(quote.Oil, oil.Proto()) || quote.OutputBound != scenario.bound || quote.CurrentLuck != 0 || quote.SuccessLuck != 1 || quote.SuccessPpm != 1_000_000 {
				t.Fatal("真实用油报价没有读取分解原油、绑定或首瓶规则", quote)
			}
			commit := weaponOilCommitWS(c, quote, reference)
			applied := c.asset(p.MsgC2SWeaponOilCommit, commit)
			useReceipt := waitWeaponOilReceipt(t, ctx, store, roleID, domain.Hex(commit.Operation.OpId)).WeaponOil
			changed := weapon
			changed.CurrentLuck, changed.Version, changed.Bound = 1, weapon.Version+1, scenario.bound
			final, err := store.Role(ctx, roleID)
			must(t, err)
			if applied.ErrorCode != 0 || !proto.Equal(applied.WeaponOil, useReceipt.Proto()) || !useReceipt.Success || !useReceipt.Plan.Equipped ||
				useReceipt.Plan.Oil != oil || useReceipt.Weapon != changed || useReceipt.OilConsumed != 1 || useReceipt.OilRemainder != nil ||
				len(final.Items) != 0 || !slices.Equal(final.Equipped, []domain.Equipped{{Slot: devconfig.WeaponSlot, Item: changed}}) ||
				final.Wallet != afterRecycle.Wallet || final.EconomicSeq != afterDrop.EconomicSeq+2 || final.SourceSeq != afterDrop.SourceSeq {
				t.Fatal("分解原油没有仅消耗一次，或未把同一把武器原地升为幸运 +1", applied, final.Equipped, final.Items)
			}
			luckyAttributes(c, fixture.CombatLuck.WeaponWingPPM[1], fixture.CombatLuck.RoleResistancePPM[1])
			t.Logf("真实装备幸运 +%d，正耐久满包分解得一瓶原油，同一场景重连和原回执重放后已穿木剑升至 +1；绑定=%t", dropped.InitialLuck, scenario.bound)
		})
	}
}

// recycleOilFieldFixture 先验证唯一来源投影，再仅在本例隔离加快怪物、抽样和油产出。
func recycleOilFieldFixture(t *testing.T) (devconfig.Fixture, fieldDropCase, devconfig.Skill) {
	t.Helper()
	raw, err := os.ReadFile(os.Getenv("MINIMIR_RECYCLE_OIL_TEST_FIXTURE"))
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	if fixture.EquipmentRecycleOil == nil || fixture.EquipmentRecycleOil.Denominator != 30 || fixture.LuckOil == nil ||
		fixture.CombatLuck == nil || fixture.CombatLuck.DropPPM != 10_000 || !slices.Equal(fixture.CombatLuck.InitialWeights, []uint32{1, 1, 1}) ||
		fixture.Drops.EquipmentGuarantee != 20 || len(fixture.Map.MonsterSpawnIDs) != devconfig.OrdinaryMonsterSlots {
		t.Fatal("真实工作簿投影不含分母30、初始幸运1%等权、20杀保底与八怪槽")
	}
	monster, known := fixture.MonsterByID(fixture.Map.MonsterSpawnIDs[0])
	if !known {
		t.Fatal("普通图首个怪物模板缺失")
	}
	const clothID = 193
	var original devconfig.OrdinaryDropEntry
	for index := range fixture.OrdinaryDropPools {
		pool := &fixture.OrdinaryDropPools[index]
		if pool.ID != monster.DropPoolID {
			continue
		}
		for _, candidate := range pool.Equipment {
			if candidate.ItemID == clothID {
				original = candidate
				pool.Equipment = []devconfig.OrdinaryDropEntry{original}
				break
			}
		}
	}
	spell, known := fixture.Skill(1)
	if original.ItemID == 0 || !known || spell.JobID != 1 || spell.Behavior != "fireball" {
		t.Fatal("隔离普通图缺少原布衣和法师火球")
	}
	monster.HP, monster.MoveMS, monster.RespawnMS, monster.AttackMin, monster.AttackMax = 1, 60_000, 60_000, 0, 0
	monster.MagicResist, monster.MagicResistWeight, monster.MagicDefenceMin, monster.MagicDefenceMax = 0, 0, 0, 0
	// 保留装备类别权重，由第20次原保底命中。初始幸运仍按原三档等权，分解则让正幸运档必出油。
	fixture.CombatLuck.DropPPM = 1_000_000
	fixture.EquipmentRecycleOil.Denominator = 1
	qualified, err := json.Marshal(fixture)
	must(t, err)
	fixture.Hash = persistence.Hash(qualified)
	t.Log("实际取用工作簿分母30、1%初始幸运与20杀装备保底；此闭环仅隔离把分母降至1、初始幸运提至100%，验证原资产路径而非正式概率")
	return fixture, fieldDropCase{MapID: fixture.Map.ID, Monster: *monster, PoolID: monster.DropPoolID, Equipment: original}, spell
}

// seedRecycleOilFieldRole 只为独立新角色设置保底、技能、未培养武器、背包容量和绑定资格，不写任何收益。
func seedRecycleOilFieldRole(t *testing.T, ctx context.Context, store *persistence.Store, roleID string, fixture *devconfig.Fixture, spell devconfig.Skill, bound bool) domain.Item {
	t.Helper()
	weapon := persistence.InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1}, fixture)
	fields := bson.M{"level": uint32(15), "no_equipment_kills": fixture.Drops.EquipmentGuarantee - 1,
		"learned_skills": []domain.LearnedSkill{{SkillID: spell.ID, Level: spell.LearnedLevel}}, "skill_slot_ids": []uint32{0, 0, 0, 0},
		"capacity": 1, "items": []domain.Item{}, "equipped": []domain.Equipped{{Slot: devconfig.WeaponSlot, Item: weapon}}}
	if !bound {
		now := time.Now().UTC()
		fields["privilege_since"], fields["privilege_until"] = now.Add(-time.Hour), now.Add(time.Hour)
	}
	result, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": fields})
	must(t, err)
	if result.ModifiedCount != 1 {
		t.Fatal("不能准备独立新角色的原背包与保底")
	}
	return weapon
}

// waitRecycleOilOperation 在联机客户端读取响应前，从 MongoDB 查证同一个原分解事务。
func waitRecycleOilOperation(t *testing.T, ctx context.Context, store *persistence.Store, roleID, opID string) persistence.Operation {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	var operation persistence.Operation
	for {
		err := store.DB.Collection("operations").FindOne(deadline, bson.M{"_id": "asset:" + roleID + ":" + opID}).Decode(&operation)
		if !errors.Is(err, mongo.ErrNoDocuments) || deadline.Err() != nil {
			must(t, err)
			return operation
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// checkRecycleOilReceipt 独立核对该原件、抽样、提交后的物品与金币，以及同事务写出的审计。
func checkRecycleOilReceipt(t *testing.T, ctx context.Context, store *persistence.Store, replayed *pb.OperationResult, operation persistence.Operation,
	before domain.Role, dropped domain.Item, fixture *devconfig.Fixture) domain.Item {
	t.Helper()
	if operation.Kind != "recycle" || operation.Code != 0 || operation.Recycling == nil || !slices.Equal(operation.Recycling.Recycled, []domain.Item{dropped}) ||
		len(operation.Recycling.OilRolls) != 1 || replayed.Pending || replayed.ErrorCode != 0 || len(replayed.GetRecycleOil().GetRolls()) != 1 {
		t.Fatal("没收到同一个已提交的原分解回执，或原件没有独立抽样", operation, replayed)
	}
	roll, observed := operation.Recycling.OilRolls[0], replayed.RecycleOil.Rolls[0]
	if roll.EquipmentInstanceID != dropped.ID || roll.Luck != dropped.InitialLuck || roll.Denominator != 1 || roll.Draw != 0 || roll.Oil == nil ||
		domain.Hex(observed.EquipmentInstanceId) != dropped.ID || observed.Luck != roll.Luck || !proto.Equal(observed.Oil, roll.Oil.Proto()) {
		t.Fatal("重连重放没有返回原抽样、原装备及同一瓶油", roll, observed)
	}
	oil := *roll.Oil
	if oil.ID == dropped.ID || oil.ItemID != fixture.LuckOil.ItemID || oil.Bound != dropped.Bound || oil.Quantity != 1 || oil.Version != 1 ||
		oil.SourceKind != "equipment_recycle" || oil.SourceID != dropped.ID {
		t.Fatal("分解祝福油没有关联原装备，或未继承绑定", oil)
	}
	var audit struct {
		Recycling *persistence.EquipmentRecyclingReceipt `bson:"recycling"`
	}
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": operation.ID, "kind": "recycle"}).Decode(&audit))
	if !reflect.DeepEqual(audit.Recycling, operation.Recycling) {
		t.Fatal("分解产油回执没有在同一事务写入原审计", audit.Recycling)
	}
	definition, _ := fixture.Item(dropped.ItemID)
	wantWallet := before.Wallet
	if dropped.Bound {
		wantWallet.BoundGold += definition.RecycleGold
	} else {
		wantWallet.Gold += definition.RecycleGold
	}
	after, err := store.Role(ctx, before.ID)
	must(t, err)
	if after.Wallet != wantWallet || after.EconomicSeq != before.EconomicSeq+1 || after.SourceSeq != before.SourceSeq {
		t.Fatal("分解金币、绑定分账或原操作水位重复扣记", after.Wallet, wantWallet)
	}
	return oil
}
