// 通过真实普通击杀及手动分解回执验证洗炼石来源、扣费、候选和健康重连。
package integration

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strconv"
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

const (
	refineJourneyGenerate = 1   // 生成并持久保存最新右侧候选。
	refineJourneyAdopt    = 2   // 手动将当前候选采纳至左侧。
	refineJourneyDiscard  = 3   // 丢弃已付候选，原消费和绑定保持。
	refineJourneyBatch    = 16  // 分批回收，保留目标和原背包空间。
	refineJourneyMaxDraws = 400 // 限定真实 5% 抽样的总观察窗。
)

// TestRefineEquipmentNaturalMongoWS 核验真实获得原件和石、立即改绑、掉线恢复、放弃及后续采纳。
func TestRefineEquipmentNaturalMongoWS(t *testing.T) {
	fixture, source, spell := refineJourneyFixture(t)
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) { cfg.Fixture = fixture })
	login, summary := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "refine_ws_" + domain.ID()[:8], Password: domain.ID()[:16]}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: route.origin.OriginID,
		Slot: 1, Job: 1, Name: "洗炼验收" + domain.ID()[:4], OpId: domain.Bytes(domain.ID())}, summary, 0)
	roleID := domain.Hex(summary.Key.RoleId)
	now := time.Now().UTC()
	// 只预置职业、法术、特权资格及第一次改绑用的绑定金币。背包和每件乌木剑、洗炼石均由原路径产生。
	prepared, err := store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": bson.M{
		"level": uint32(7), "learned_skills": []domain.LearnedSkill{{SkillID: spell.ID, Level: spell.LearnedLevel}},
		"skill_slot_ids": []uint32{spell.ID, 0, 0, 0}, "wallet.bound_gold": int64(1_000),
		"privilege_since": now.Add(-time.Hour), "privilege_until": now.Add(time.Hour),
	}})
	must(t, err)
	if prepared.ModifiedCount != 1 {
		t.Fatal("隔离角色的施法、特权或绑定金币资格未写入")
	}
	attach := func() *client {
		t.Helper()
		ticket := directoryTicket(t, endpoint, login.AccountToken, summary.Key, route)
		joined := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
		joined.stop()
		joined.ctx, joined.stop = context.WithTimeout(ctx, 150*time.Second)
		t.Cleanup(joined.close)
		return joined
	}
	c := attach()
	c.auto(false, 1)
	initial := refineJourneyRole(t, ctx, store, roleID)
	if initial.Level != 7 || initial.Wallet.Gold != 0 || initial.Wallet.BoundGold != 1_000 || !initial.PrivilegeActiveAt(now) ||
		slices.ContainsFunc(initial.Items, func(item domain.Item) bool {
			return item.ItemID == fixture.EquipmentRefine.StoneItemID || item.ItemID == source.Equipment.ItemID
		}) {
		t.Fatal("本例须从无目标装备、无洗炼石和没有非绑金币的隔离角色开始", initial.Items, initial.Wallet)
	}
	stoneSource, stones, draws := collectRefineJourneyStones(t, ctx, store, c, roleID, &fixture, source)
	if len(stones) < 4 {
		t.Fatal("5% 原分解观察窗内未取得可用的四枚真实洗炼石", draws, len(stones))
	}
	t.Logf("由真实普通战斗产出原目标与分解原件，在 %d 次独立 50000/1000000 判定中取得 %d 枚原洗炼石", draws, len(stones))

	before := refineJourneyRole(t, ctx, store, roleID)
	if stoneSource.Bound || !stoneSource.Refine.IsZero() || before.Wallet.Gold < 1_000 || before.Wallet.BoundGold != 1_000 {
		t.Fatal("洗炼前应有真实非绑目标、非绑回收款和足额隔离绑金", stoneSource, before.Wallet)
	}
	scene, binding, newPanel := c.entry.SceneEpoch, c.binding, domain.Bytes(domain.ID())
	closed := refineJourneyQuote(t, c, stoneSource, stones[:2], false, newPanel)
	if closed.OutputBound || closed.Gold != 1_000 || closed.BoundGold != 0 {
		t.Fatal("关闭绑定授权后没有只使用非绑金币", closed)
	}
	first := refineJourneyQuote(t, c, stoneSource, stones[:2], true, newPanel)
	if !first.OutputBound || first.Gold != 0 || first.BoundGold != 1_000 {
		t.Fatal("原报价没有明确说明使用绑定金币会令原件立即角色绑定", first)
	}
	unchanged := refineJourneyRole(t, ctx, store, roleID)
	if unchanged.Wallet != before.Wallet || unchanged.Revision != before.Revision || !slices.Equal(unchanged.Items, before.Items) {
		t.Fatal("只读洗炼报价改动了原资产")
	}
	generate := refineJourneyGenerateRequest(c, first, stoneSource, newPanel)
	c.send(p.MsgC2SEquipmentRefineAction, generate)
	stored := waitRecycleOilOperation(t, ctx, store, roleID, domain.Hex(generate.Operation.OpId))
	committed := refineJourneyRole(t, ctx, store, roleID)
	firstPending := refineJourneyAssertGenerate(t, fixture.EquipmentRefine, committed, before, stoneSource, first, stones[:2])
	c.close() // 原付费已在 MongoDB 确认，旧客户端不读取这一条响应。
	c = attach()
	if c.binding == binding || c.entry.SceneEpoch != scene || c.entry.SceneKind != devconfig.SceneKindTown ||
		!refineJourneyItemSeen(c, firstPending) || c.hosting.AutoBattleEnabled {
		t.Fatal("掉线后新票没有恢复原场景、原已付候选和已经发生的改绑", c.entry, c.inventory)
	}
	refineJourneyCheckAction(t, ctx, store, c.asset(p.MsgC2SEquipmentRefineAction, generate), stored, firstPending, refineJourneyGenerate)
	confirmed := refineJourneyRole(t, ctx, store, roleID)
	if confirmed.Wallet != committed.Wallet || confirmed.SourceSeq != committed.SourceSeq || confirmed.EconomicSeq != committed.EconomicSeq ||
		!slices.Equal(confirmed.Items, committed.Items) {
		t.Fatal("原付费操作重放再次扣取资产或改变候选")
	}
	blocked := c.asset(p.MsgC2SRecycle, &pb.RecycleRequest{
		ItemInstanceIds: [][]byte{domain.Bytes(firstPending.ID)}, Operation: c.operation(),
	})
	if blocked.ErrorCode != p.CodeInvalidArgument || !refineJourneyItemSeen(c, firstPending) {
		t.Fatal("已付待选期间原件仍能被分解", blocked, c.inventory)
	}
	// 分解拒绝也可能推进资产命令号，下一笔以实际服务器库存修订为准。
	afterBlocked := refineJourneyRole(t, ctx, store, roleID)
	if afterBlocked.Wallet != committed.Wallet || !slices.Equal(afterBlocked.Items, committed.Items) {
		t.Fatal("已付候选没有保护原装备")
	}
	syncWingInventory(c, uint64(afterBlocked.Revision))
	discard := refineJourneyDecision(c, firstPending, refineJourneyDiscard)
	discarded := refineJourneyAction(t, ctx, store, c, afterBlocked, firstPending, discard, stoneSource.Refine.Current)
	if !discarded.Bound || discarded.Refine.Current != stoneSource.Refine.Current {
		t.Fatal("丢弃右侧后改动了左侧或撤回了已经发生的绑定", discarded)
	}

	before = refineJourneyRole(t, ctx, store, roleID)
	panel := domain.Bytes(domain.ID())
	second := refineJourneyQuote(t, c, discarded, stones[2:4], false, panel)
	if !second.OutputBound || second.Gold != 1_000 || second.BoundGold != 0 {
		t.Fatal("换用全非绑资源后不应解除同一原件的绑定", second)
	}
	nextGenerate := refineJourneyGenerateRequest(c, second, discarded, panel)
	result := c.asset(p.MsgC2SEquipmentRefineAction, nextGenerate)
	stored = waitRecycleOilOperation(t, ctx, store, roleID, domain.Hex(nextGenerate.Operation.OpId))
	committed = refineJourneyRole(t, ctx, store, roleID)
	secondPending := refineJourneyAssertGenerate(t, fixture.EquipmentRefine, committed, before, discarded, second, stones[2:4])
	if secondPending.PendingResultID == firstPending.PendingResultID {
		t.Fatal("第二次付费复用了第一次已删除的待选号")
	}
	refineJourneyCheckAction(t, ctx, store, result, stored, secondPending, refineJourneyGenerate)
	syncWingInventory(c, uint64(committed.Revision))
	adopt := refineJourneyDecision(c, secondPending, refineJourneyAdopt)
	adopted := refineJourneyAction(t, ctx, store, c, committed, secondPending, adopt, secondPending.Refine.Candidate)
	refineJourneyAssertEquipped(t, c, adopted)
	t.Log("正常选石，首次用绑定金币扣 1000 且只扣两枚原非绑石；断线未读回包后同号重放，放弃保留已发生的绑定；再用 1000 非绑金与两原石生成并采纳，同一原件实际生效")
}

// refineJourneyFixture 先核对真实工作簿投影，仅调整独立测试的击杀产出和时间。
func refineJourneyFixture(t *testing.T) (devconfig.Fixture, fieldDropCase, devconfig.Skill) {
	t.Helper()
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	raw, err := os.ReadFile(os.Getenv("MINIMIR_EQUIPMENT_REFINE_TEST_FIXTURE"))
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	must(t, fixture.Validate())
	original := persistence.Hash(raw)
	rules := fixture.EquipmentRefine
	if rules == nil || rules.StoneItemID != 900051 || rules.Recycle != (devconfig.EquipmentRefineRecycleRule{
		ItemID: 220, Quality: 1, SourceKind: "monster_drop", ProbabilityPPM: 50_000, Quantity: 1,
	}) || fixture.Drops.EquipmentGuarantee != 20 || fixture.CombatLuck == nil || fixture.CombatLuck.DropPPM != 10_000 ||
		fixture.TownTravel == nil || len(fixture.TownTravel.DestinationMapIDs) == 0 {
		t.Fatal("前置配置没有原精良乌木剑分解 5%、20 杀装备保底和隔离出油")
	}
	mapRow, exists := fixture.MapByID(6)
	if !exists || mapRow.SceneKindValue() != devconfig.SceneKindPersonal || len(mapRow.MonsterSpawnIDs) == 0 {
		t.Fatal("缺少原谷地南坡地图")
	}
	monster, exists := fixture.MonsterByID(mapRow.MonsterSpawnIDs[0])
	if !exists || monster.DropPoolID != 13 || monster.RespawnMS != 10_000 {
		t.Fatal("缺少原谷地怪物、原池或10秒复活")
	}
	var sword devconfig.OrdinaryDropEntry
	for index := range fixture.OrdinaryDropPools {
		pool := &fixture.OrdinaryDropPools[index]
		if pool.ID != monster.DropPoolID {
			continue
		}
		for _, item := range pool.Equipment {
			if item.ItemID == rules.Recycle.ItemID && item.QuantityMin == 1 && item.QuantityMax == 1 {
				sword = item
				pool.Equipment = []devconfig.OrdinaryDropEntry{sword}
				break
			}
		}
	}
	spell, exists := fixture.Skill(1)
	if sword.ItemID == 0 || !exists || spell.JobID != 1 || spell.Behavior != "fireball" || spell.LearnLevel != 7 {
		t.Fatal("隔离候选缺少取自当前原目录的乌木剑和法师火球术")
	}
	for index := range fixture.Skills {
		skill := &fixture.Skills[index]
		if skill.ID == spell.ID {
			skill.MPBaseCost, skill.MPLevelCost, skill.CooldownBaseMS, skill.GlobalSpellMS = 0, 0, 20, 20
			skill.ActionMS, skill.DelayBaseMS, skill.ProjectileMSPerCell, skill.PowerBase = 20, 5, 0, 100
			spell = *skill
			break
		}
	}
	// 减少候选装备试验的等待时间。仍由正常火球、复活、击杀事务和真 5% 分解完成。
	fixture.Drops.EquipmentGuarantee = 1
	fixture.CombatLuck.DropPPM = 0
	monster.HP, monster.MoveMS, monster.RespawnMS = 1, 60_000, 20
	monster.AttackMin, monster.AttackMax, monster.MagicResist, monster.MagicResistWeight = 0, 0, 0, 0
	qualified, err := json.Marshal(fixture)
	must(t, err)
	fixture.Hash = persistence.Hash(qualified)
	t.Logf("原工作簿夹具 %s；独立加快火球和复活、关闭初始幸运、限定池13为原220且每杀触发装备保底，保留怪物与物品来源；实际隔离 SHA %s，分解仍原5%%", original, fixture.Hash)
	return fixture, fieldDropCase{MapID: mapRow.ID, Monster: *monster, PoolID: monster.DropPoolID, Equipment: sword}, spell
}

// collectRefineJourneyStones 用普通挂机采集原件，去城镇后批量手动分解，保护首把原装备。
func collectRefineJourneyStones(t *testing.T, ctx context.Context, store *persistence.Store, c *client, roleID string,
	fixture *devconfig.Fixture, source fieldDropCase) (domain.Item, []domain.Item, int) {
	t.Helper()
	var target domain.Item
	var stones []domain.Item
	var draws, misses int
	for len(stones) < 4 && draws < refineJourneyMaxDraws {
		travelToBaseline(c, 2, source.MapID)
		travelAcknowledge(c, false)
		c.auto(true, 1)
		needed := min(refineJourneyBatch, refineJourneyMaxDraws-draws)
		if target.ID == "" {
			needed++ // 首把作为后续培养对象，不交给分解。
		}
		travelUntilWithin(c, 14*time.Second, func(frame wire.Frame) bool {
			if frame.ID != p.MsgS2CInventory {
				return false
			}
			count := 0
			for _, item := range c.inventory.Items {
				if item.ItemId == source.Equipment.ItemID && item.SourceKind == "monster_drop" && domain.Hex(item.InstanceId) != target.ID {
					count++
				}
			}
			return count >= needed
		})
		c.auto(false, 1)
		// 正常换图等待普通图的最后一笔来源收束，避免测试误读在途库存修订。
		travelToBaseline(c, 1, fixture.TownTravel.DestinationMapIDs[0])
		travelAcknowledge(c, true)
		before := refineJourneyRole(t, ctx, store, roleID)
		var inputs []domain.Item
		for _, item := range before.Items {
			if item.ItemID != source.Equipment.ItemID || item.SourceKind != "monster_drop" || item.ID == target.ID {
				continue
			}
			if target.ID == "" {
				refineJourneyCheckDrop(t, ctx, store, before, item, source)
				target = item
			} else if len(inputs) < refineJourneyMaxDraws-draws {
				refineJourneyCheckDrop(t, ctx, store, before, item, source)
				inputs = append(inputs, item)
			}
		}
		if len(inputs) == 0 || target.ID == "" {
			t.Fatal("真实掉落没有出现原目标或可分解的独立原件", needed)
		}
		obtained := refineJourneyRecycle(t, ctx, store, c, before, inputs, fixture)
		draws += len(inputs)
		misses += len(inputs) - len(obtained)
		stones = append(stones, obtained...)
	}
	if misses == 0 {
		t.Fatal("没有观察到只有对应原件、没有洗炼石产物的失败抽样")
	}
	return target, stones, draws
}

// refineJourneyCheckDrop 逐件对拍真实怪物生命周期、角色绑定资格和原收益编号。
func refineJourneyCheckDrop(t *testing.T, ctx context.Context, store *persistence.Store, owner domain.Role,
	item domain.Item, source fieldDropCase) {
	t.Helper()
	var audit struct {
		Item         domain.Item `bson:"item"`
		EventAt      time.Time   `bson:"event_at"`
		TargetHandle uint32      `bson:"target_handle"`
		CasterHandle uint32      `bson:"caster_handle"`
	}
	id := "settle:" + owner.ID + ":" + item.SourceID
	var operation persistence.Operation
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": id}).Decode(&operation))
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": id, "kind": "settlement"}).Decode(&audit))
	settled := operation.Settlement
	if item.Bound || item.Quality != 1 || item.InitialLuck != 0 || item.CurrentLuck != 0 || !item.HasValidDurability() ||
		item.DurabilityU <= 0 || operation.Kind != "settlement" || item.SourceID != strconv.FormatInt(operation.SourceSeq, 10) ||
		settled == nil || !settled.GrantedItem || settled.Kills != 1 || settled.MapID != source.MapID || settled.MonsterID != source.Monster.ID ||
		settled.DropPoolID != source.PoolID || settled.ConfigHash != store.Cfg.Fixture.Hash || audit.Item.ID != item.ID ||
		audit.Item.SourceKind != "monster_drop" || audit.Item.SourceID != item.SourceID || audit.TargetHandle == 0 || audit.CasterHandle == 0 ||
		audit.EventAt.IsZero() || !owner.PrivilegeActiveAt(audit.EventAt) || audit.Item.Bound {
		t.Fatal("待用乌木剑未追溯到特权有效时原普通怪物击杀和共同收益事务", settled, item, audit)
	}
}

// refineJourneyRecycle 核验保持真实 5% 的每项抽样、原石来源和原金币同事务到达。
func refineJourneyRecycle(t *testing.T, ctx context.Context, store *persistence.Store, c *client, before domain.Role,
	inputs []domain.Item, fixture *devconfig.Fixture) []domain.Item {
	t.Helper()
	syncWingInventory(c, uint64(before.Revision))
	request := &pb.RecycleRequest{Operation: c.operation()}
	for _, item := range inputs {
		request.ItemInstanceIds = append(request.ItemInstanceIds, domain.Bytes(item.ID))
	}
	out := c.asset(p.MsgC2SRecycle, request)
	operation := waitRecycleOilOperation(t, ctx, store, before.ID, domain.Hex(request.Operation.OpId))
	if out.ErrorCode != 0 || out.Pending || operation.Kind != "recycle" || operation.Recycling == nil ||
		!slices.Equal(operation.Recycling.Recycled, inputs) || len(operation.Recycling.RefineRolls) != len(inputs) ||
		len(out.GetRecycleRefine().GetRolls()) != len(inputs) || len(out.GetRecycleOil().GetRolls()) != 0 || len(operation.Recycling.OilRolls) != 0 {
		t.Fatal("手动分解没有逐原件判定洗炼石，或冲突生成油", out, operation.Recycling)
	}
	var audit struct {
		Receipt *persistence.EquipmentRecyclingReceipt `bson:"recycling"`
	}
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": operation.ID}).Decode(&audit))
	if !reflect.DeepEqual(audit.Receipt, operation.Recycling) {
		t.Fatal("原分解账本与原审计不同", audit.Receipt)
	}
	after := refineJourneyRole(t, ctx, store, before.ID)
	definition, _ := fixture.Item(inputs[0].ItemID)
	wallet := before.Wallet
	wallet.Gold += int64(len(inputs)) * definition.RecycleGold
	if after.Wallet != wallet || after.SourceSeq != before.SourceSeq || after.EconomicSeq != before.EconomicSeq+1 {
		t.Fatal("原正常扣石来源的非绑定回收金币没有同次分账", after.Wallet, wallet)
	}
	var gained []domain.Item
	for index, fact := range operation.Recycling.RefineRolls {
		original, view := inputs[index], out.RecycleRefine.Rolls[index]
		if fact.Numerator != 50_000 || fact.Denominator != 1_000_000 || fact.Draw >= fact.Denominator ||
			fact.EquipmentInstanceID != original.ID || domain.Hex(view.EquipmentInstanceId) != original.ID ||
			(fact.Draw < fact.Numerator) != (fact.Stone != nil) || (view.Stone != nil) != (fact.Stone != nil) || after.HasItemInstance(original.ID) {
			t.Fatal("实际分解原件、抽样阈值和原产物不一致", fact, view, original)
		}
		if fact.Stone != nil {
			stone := *fact.Stone
			if stone.ItemID != fixture.EquipmentRefine.StoneItemID || stone.Quantity != 1 || stone.Version != 1 || stone.Bound ||
				stone.SourceKind != "equipment_recycle_refine" || stone.SourceID != original.ID || !after.HasItemInstance(stone.ID) ||
				!proto.Equal(view.Stone, stone.Proto()) {
				t.Fatal("入包石头不对应真实普通掉落装备或丢失原绑定", stone, view, original)
			}
			gained = append(gained, stone)
		}
	}
	syncWingInventory(c, uint64(after.Revision))
	return gained
}

// refineJourneyRole 读取原角色用于库存、钱包和永久流水的联合验证。
func refineJourneyRole(t *testing.T, ctx context.Context, store *persistence.Store, roleID string) domain.Role {
	t.Helper()
	role, err := store.Role(ctx, roleID)
	must(t, err)
	return role
}

// refineJourneyItem 读取确切实例，不能以同模板物品替换已选装备。
func refineJourneyItem(t *testing.T, role domain.Role, id string) domain.Item {
	t.Helper()
	for _, item := range role.Items {
		if item.ID == id {
			return item
		}
	}
	t.Fatal("原洗炼装备实例不在背包", id)
	return domain.Item{}
}

// refineJourneyItemSeen 比较当前连接库存中完整原实例和候选，保留版本和原石身份。
func refineJourneyItemSeen(c *client, item domain.Item) bool {
	return slices.ContainsFunc(c.inventory.GetItems(), func(view *pb.ItemView) bool { return proto.Equal(view, item.Proto()) })
}

// refineJourneyQuote 用本次选定的真实石堆请求具体分账，确认报价不暴露候选。
func refineJourneyQuote(t *testing.T, c *client, item domain.Item, stones []domain.Item, allowBound bool, panel []byte) *pb.EquipmentRefineQuote {
	t.Helper()
	q := &pb.EquipmentRefineQuoteRequest{SceneEpoch: c.entry.SceneEpoch, Target: refineJourneyTarget(item),
		AllowBound: allowBound, UiSessionId: panel}
	for _, stone := range stones {
		q.Stones = append(q.Stones, &pb.EquipmentRefineStoneRef{InstanceId: domain.Bytes(stone.ID), Version: uint64(stone.Version), Quantity: 1})
	}
	request := c.send(p.MsgC2SEquipmentRefineQuote, q)
	frame := travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CEquipmentRefineQuote && frame.Request == request
	})
	quote := &pb.EquipmentRefineQuote{}
	must(t, proto.Unmarshal(frame.Payload, quote))
	if len(quote.QuoteId) != 16 || quote.SceneEpoch != c.entry.SceneEpoch || quote.RoleRevision != c.inventory.Revision ||
		!slices.Equal(quote.UiSessionId, panel) || quote.AllowBound != allowBound || quote.Equipped ||
		!proto.Equal(quote.Target, item.Proto()) || len(quote.Stones) != len(stones) || len(quote.LockedSlots) != 0 ||
		quote.YuanbaoUnits != 0 || quote.BoundYuanbaoUnits != 0 {
		t.Fatal("当前角色未收到精确报价和原对象", quote, item)
	}
	for index, row := range quote.Stones {
		if row.Quantity != 1 || !proto.Equal(row.Item, stones[index].Proto()) {
			t.Fatal("报价的实际原选材被替换", index, row)
		}
	}
	return quote
}

// refineJourneyTarget 显式引用当前背包中的原件及版本。
func refineJourneyTarget(item domain.Item) *pb.EquipmentRefineItemRef {
	return &pb.EquipmentRefineItemRef{InstanceId: domain.Bytes(item.ID), Version: uint64(item.Version)}
}

// refineJourneyGenerateRequest 只确认界面上冻结的原报价及绑定许可。
func refineJourneyGenerateRequest(c *client, quote *pb.EquipmentRefineQuote, item domain.Item, panel []byte) *pb.EquipmentRefineActionRequest {
	return &pb.EquipmentRefineActionRequest{Action: refineJourneyGenerate, Operation: c.operation(), Target: refineJourneyTarget(item),
		QuoteId: quote.QuoteId, UiSessionId: panel, AllowBound: quote.AllowBound}
}

// refineJourneyDecision 断线后新界面仍引用同一原候选，决定阶段没有重复报价与扣费。
func refineJourneyDecision(c *client, item domain.Item, action uint32) *pb.EquipmentRefineActionRequest {
	return &pb.EquipmentRefineActionRequest{Action: action, Operation: c.operation(), Target: refineJourneyTarget(item),
		UiSessionId: domain.Bytes(domain.ID()), PendingId: domain.Bytes(item.PendingResultID)}
}

// refineJourneyAssertGenerate 验证真实扣料和四钱包变化与新右侧候选同批入库。
func refineJourneyAssertGenerate(t *testing.T, rules *devconfig.EquipmentRefineConfig, after, before domain.Role, item domain.Item,
	quote *pb.EquipmentRefineQuote, stones []domain.Item) domain.Item {
	t.Helper()
	got := refineJourneyItem(t, after, item.ID)
	line := got.Refine.Candidate[0]
	wantUnits, allowed := rules.ValueUnits(line.AttributeID, 0, line.Tier)
	affix, known := rules.Affix(line.AttributeID)
	if len(domain.Bytes(got.PendingResultID)) != 16 || !allowed || !known || affix.MinQuality != 1 || line.ValueUnits != wantUnits ||
		got.Refine.Candidate != (domain.EquipmentAffixes{line}) || got.Refine.LockedMask != 0 {
		t.Fatal("已付结果没有只生成原精良一槽合法随机候选", got)
	}
	want := item
	want.Version, want.Bound, want.PendingResultID, want.Refine.Candidate = item.Version+1, quote.OutputBound, got.PendingResultID, got.Refine.Candidate
	wallet := before.Wallet
	wallet.Gold, wallet.BoundGold = wallet.Gold-quote.Gold, wallet.BoundGold-quote.BoundGold
	if got != want || after.Wallet != wallet || len(after.Items) != len(before.Items)-len(stones) ||
		after.EconomicSeq != before.EconomicSeq+1 || after.SourceSeq != before.SourceSeq || !slices.Equal(after.Equipped, before.Equipped) {
		t.Fatal("生成候选改变了原装备其他字段、原钱包账或经济水位", got, want, wallet, after.Wallet)
	}
	for _, stone := range stones {
		if after.HasItemInstance(stone.ID) {
			t.Fatal("提交后仍保留已经明确消费的真实原石", stone)
		}
	}
	return got
}

// refineJourneyCheckAction 核对实际联机结果、原唯一回执与同事务审计。
func refineJourneyCheckAction(t *testing.T, ctx context.Context, store *persistence.Store, out *pb.OperationResult,
	operation persistence.Operation, item domain.Item, action uint32) {
	t.Helper()
	receipt := operation.EquipmentRefine
	if out.ErrorCode != 0 || out.Pending || operation.Kind != persistence.AssetEquipmentRefine || operation.Code != 0 ||
		receipt == nil || receipt.Action != action || receipt.Target != item || !proto.Equal(receipt.Proto(), out.EquipmentRefine) {
		t.Fatal("联机动作没有返回实际持久的原实例及原操作回执", out, operation)
	}
	var audit struct {
		Refine *persistence.EquipmentRefineReceipt `bson:"equipment_refine"`
	}
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": operation.ID, "kind": persistence.AssetEquipmentRefine}).Decode(&audit))
	if !reflect.DeepEqual(audit.Refine, receipt) {
		t.Fatal("完整实际消费或决定没有写入同事务审计", audit.Refine, receipt)
	}
}

// refineJourneyAction 采纳或丢弃的决定只处理对应原待选，不付新费用。
func refineJourneyAction(t *testing.T, ctx context.Context, store *persistence.Store, c *client, before domain.Role, item domain.Item,
	request *pb.EquipmentRefineActionRequest, expected domain.EquipmentAffixes) domain.Item {
	t.Helper()
	result := c.asset(p.MsgC2SEquipmentRefineAction, request)
	stored := waitRecycleOilOperation(t, ctx, store, before.ID, domain.Hex(request.Operation.OpId))
	after := refineJourneyRole(t, ctx, store, before.ID)
	target := refineJourneyItem(t, after, item.ID)
	want := item
	want.PendingResultID, want.Refine, want.Version = "", domain.EquipmentRefineState{Current: expected}, item.Version+1
	if target != want || after.Wallet != before.Wallet || len(after.Items) != len(before.Items) ||
		after.SourceSeq != before.SourceSeq || after.EconomicSeq != before.EconomicSeq+1 ||
		len(result.GetEquipmentRefine().GetStones()) != 0 || result.GetEquipmentRefine().GetBoundGold() != 0 ||
		result.GetEquipmentRefine().GetGold() != 0 || !slices.Equal(result.GetEquipmentRefine().GetPendingId(), request.PendingId) {
		t.Fatal("候选决定不应修改费用、其他资产或原装备属性", target, want, result)
	}
	refineJourneyCheckAction(t, ctx, store, result, stored, target, request.Action)
	syncWingInventory(c, uint64(after.Revision))
	return target
}

// refineJourneyAssertEquipped 确认可采纳的真实词条随装备进入权威角色属性。
func refineJourneyAssertEquipped(t *testing.T, c *client, item domain.Item) {
	t.Helper()
	before := proto.Clone(c.player()).(*pb.EntityState)
	line := item.Refine.Current[0]
	delta := line.ValueUnits / 10
	q := &pb.EquipRequest{ItemInstanceId: domain.Bytes(item.ID), ItemVersion: uint64(item.Version), Slot: devconfig.WeaponSlot,
		Operation: c.operation()}
	result := c.asset(p.MsgC2SEquip, q)
	if result.ErrorCode != 0 || delta <= 0 {
		t.Fatal("真实培养原件没有正常穿戴或词条配置无可见效果", result, line)
	}
	syncWingInventory(c, result.NewRevision)
	travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CSceneBaseline || c.player() == nil || c.player().CombatAttributes == nil {
			return false
		}
		player := c.player()
		switch line.AttributeID {
		case devconfig.RefineAttributeDefence:
			return player.CombatAttributes.PhysicalDefenceMin == before.CombatAttributes.PhysicalDefenceMin+delta &&
				player.CombatAttributes.PhysicalDefenceMax == before.CombatAttributes.PhysicalDefenceMax+delta
		case devconfig.RefineAttributeMaxHP:
			return player.HpMax == before.HpMax+delta
		case devconfig.RefineAttributeMaxMP:
			return player.MpMax == before.MpMax+delta
		default:
			t.Fatal("精良隔离目录产生了不受该品质支持的词条", line)
			return false
		}
	})
}
