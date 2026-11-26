// 本文件用真实个人首领奖励和独立 MongoDB 核验部位强化的材料、等级与原回执。
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
	"minimir/internal/wire"
)

const enhancementWeaponSlot = 0 // 原规则下武器是角色的第一个强化部位。

// TestEnhanceEquipmentSlotMongoWS 从无卡新角色实战取得绑定石，验证确定成功、换装、达级限制和失败回放。
func TestEnhanceEquipmentSlotMongoWS(t *testing.T) {
	fixture := enhancementJourneyFixture(t)
	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) { cfg.Fixture = fixture })
	login := &pb.LoginResult{}
	rpc(t, account, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "enhance_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	role := createDirectoryRole(t, account, login.AccountToken, route.origin.OriginID, "强化验收")
	roleID := domain.Hex(role.Key.RoleId)
	readRole := func() domain.Role {
		t.Helper()
		current, err := store.Role(ctx, roleID)
		must(t, err)
		return current
	}
	attach := func() *client {
		t.Helper()
		ticket := directoryTicket(t, account, login.AccountToken, role.Key, route)
		c := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
		t.Cleanup(c.close)
		return c
	}
	c := attach()
	created := readRole()
	if c.inventory.GetLevel() != 1 || len(c.inventory.GetEquipped()) != 0 || created.EquipmentEnhancement != nil ||
		!created.PrivilegeUntil.IsZero() ||
		naturalGrowthItem(c, fixture.EquipmentEnhancement.StoneItemID, "") != nil {
		t.Fatal("新角应从原一级空部位且无强化石开始", c.inventory)
	}
	assertEnhancementSlots(t, c.inventory, 0)
	c.auto(false, 1)
	firstSword := naturalGrowthItem(c, 218, "")
	if firstSword == nil {
		t.Fatal("普通新角未获得原木剑")
	}
	equipPersonalBossAutoFixture(t, c, &fixture, 218)
	firstStone := winEnhancementPersonalBoss(t, ctx, store, c, roleID, fixture.PersonalBoss)
	firstRequest := enhancementQuoteRequest(c, firstStone, false)
	before := readRole()
	travelExpectError(c, p.MsgC2SEquipmentEnhancementQuote, c.send(p.MsgC2SEquipmentEnhancementQuote, firstRequest), p.CodeEquipmentEnhancementInputs)
	firstRequest.AllowBound = true
	firstQuote := enhancementQuoteWS(t, c, firstRequest, firstStone, 0, 0, 10, 1_000_000)
	assertEnhancementAssets(t, readRole(), before, before.Items)
	firstCommit := enhancementCommitRequest(c, firstQuote)
	firstResult := c.asset(p.MsgC2SEquipmentEnhancementCommit, firstCommit)
	firstReceipt := waitEnhancementReceipt(t, ctx, store, roleID, firstCommit)
	assertEnhancementReceipt(t, firstResult, firstReceipt, firstQuote, true, 1)
	assertEnhancementSettlement(t, readRole(), before, firstStone, true)
	syncWingInventory(c, firstResult.NewRevision)
	assertEnhancementSlots(t, c.inventory, 1)
	if replay := c.asset(p.MsgC2SEquipmentEnhancementCommit, firstCommit); !proto.Equal(replay, firstResult) {
		t.Fatal("同连接重放未返回同一笔原强化确认", replay, firstResult)
	}
	assertEnhancementSettlement(t, readRole(), before, firstStone, true)

	// 用正常换图、穿戴保留第二把原木剑，强化等级继续归角色部位。
	travelToBaseline(c, 1, fixture.TownTravel.DestinationMapIDs[0])
	travelAcknowledge(c, true)
	spare := naturalGrowthItem(c, 218, "")
	if spare == nil || domain.Hex(spare.InstanceId) == domain.Hex(firstSword.InstanceId) {
		t.Fatal("实际可换装的第二把原木剑缺失", spare)
	}
	equipPersonalBossAutoFixture(t, c, &fixture, 218)
	swapped := readRole()
	if len(swapped.Equipped) != 1 || swapped.Equipped[0].Item.ID != domain.Hex(spare.InstanceId) ||
		swapped.Equipped[0].Item.Bound != spare.Bound || swapped.EnhancementLevel(enhancementWeaponSlot) != 1 ||
		!slices.ContainsFunc(swapped.Items, func(item domain.Item) bool {
			return item.ID == domain.Hex(firstSword.InstanceId) && item.Bound == firstSword.Bound
		}) {
		t.Fatal("换装没有保持两个原剑的归属和角色部位等级", swapped.Equipped, swapped.EquipmentEnhancement)
	}
	assertEnhancementSlots(t, c.inventory, 1)
	travelToBaseline(c, 2, fixture.Map.ID)
	travelAcknowledge(c, false)
	assertEnhancementSlots(t, c.inventory, 1)

	secondStone := winEnhancementPersonalBoss(t, ctx, store, c, roleID, fixture.PersonalBoss)
	if firstStone.ID == secondStone.ID || firstStone.SourceID == secondStone.SourceID {
		t.Fatal("两场使用了同一首领的原奖励身份", firstStone, secondStone)
	}
	secondRequest := enhancementQuoteRequest(c, secondStone, true)
	before = readRole()
	if before.Level != 1 {
		t.Fatal("等级拒绝阶段必须处于原一级", before.Level)
	}
	travelExpectError(c, p.MsgC2SEquipmentEnhancementQuote, c.send(p.MsgC2SEquipmentEnhancementQuote, secondRequest), p.CodeEquipmentEnhancementInputs)
	assertEnhancementAssets(t, readRole(), before, before.Items)
	grant := advanceEnhancementRoleLevel(t, c)
	before = readRole()
	var kill persistence.Operation
	must(t, store.DB.Collection("operations").FindOne(ctx,
		bson.M{"role_id": roleID, "source_seq": grant.SourceSeq, "kind": "settlement"}).Decode(&kill))
	if before.Level != 2 || before.SourceSeq < int64(grant.SourceSeq) || kill.Revision != int64(grant.RoleRevision) ||
		kill.Settlement == nil || kill.Settlement.Kills != 1 || kill.Settlement.MapID != fixture.Map.ID || kill.Settlement.Experience == nil {
		t.Fatal("实际林间击杀和二级经验回执不一致", before.Level, before.SourceSeq, grant, kill.Settlement)
	}
	syncWingInventory(c, uint64(before.Revision))
	secondRequest = enhancementQuoteRequest(c, secondStone, true)
	secondQuote := enhancementQuoteWS(t, c, secondRequest, secondStone, 1, 10, 20, 900_000)
	assertEnhancementAssets(t, readRole(), before, before.Items)
	// 只在独立库固定本次私有抽样，始终经过原 90% 规则与真实扣石事务。
	changed, err := store.DB.Collection("equipment_enhancement_quotes").UpdateOne(ctx,
		bson.M{"_id": roleID, "quote_id": domain.Hex(secondQuote.QuoteId)}, bson.M{"$set": bson.M{"success_roll": uint32(999_999)}})
	must(t, err)
	if changed.MatchedCount != 1 {
		t.Fatal("无法定位隔离角色的原失败报价")
	}
	secondCommit := enhancementCommitRequest(c, secondQuote)
	scene, binding := c.entry.SceneEpoch, c.binding
	c.send(p.MsgC2SEquipmentEnhancementCommit, secondCommit)
	secondReceipt := waitEnhancementReceipt(t, ctx, store, roleID, secondCommit)
	settled := readRole()
	assertEnhancementSettlement(t, settled, before, secondStone, false)
	c.close() // 原提交已落库，客户端尚未读取响应。
	c = attach()
	if c.binding == binding || c.entry.SceneEpoch != scene || c.entry.RoomId != fixture.Map.ID || c.inventory.GetLevel() != 2 {
		t.Fatal("健康新票没有接回原角色和地图", c.entry, c.binding)
	}
	if secondReceipt.EquipmentEnhancement.SuccessRoll != 999_999 {
		t.Fatal("实际结算未使用本次原私有掷点", secondReceipt.EquipmentEnhancement)
	}
	settled.Revision++ // 新连接正式换绑只推进一次角色修订，未增加经济命令或改变资产。
	assertEnhancementAssets(t, readRole(), settled, settled.Items)
	assertEnhancementReceipt(t, c.asset(p.MsgC2SEquipmentEnhancementCommit, secondCommit), secondReceipt, secondQuote, false, 1)
	assertEnhancementAssets(t, readRole(), settled, settled.Items)
	assertEnhancementSlots(t, c.inventory, 1)

	// 旧报价不得用新 op 再确认，原已结算的请求仍可幂等重放。
	final := readRole()
	newOperation := proto.Clone(secondCommit).(*pb.EquipmentEnhancementCommitRequest)
	newOperation.Operation = c.operation()
	if rejected := c.asset(p.MsgC2SEquipmentEnhancementCommit, newOperation); rejected.ErrorCode != p.CodeEquipmentEnhancementQuoteChanged || rejected.EquipmentEnhancement != nil {
		t.Fatal("旧报价被第二次结算或未拒绝", rejected)
	}
	current := readRole()
	if current.Wallet != final.Wallet || current.SourceSeq != final.SourceSeq || !slices.Equal(current.Items, final.Items) ||
		!slices.Equal(current.Equipped, final.Equipped) || !reflect.DeepEqual(current.EquipmentEnhancement, final.EquipmentEnhancement) {
		t.Fatal("拒绝旧报价仍改动了已确认资产")
	}
	t.Log("无卡角色两次实际胜利各直接收到十块绑定原石，武器部位升一级，真实换图换剑后保留，达二级后的概率失败仅扣一次")
}

// enhancementJourneyFixture 先验证工作簿投影，随后只缩短独立首领战及本轮复战冷却。
func enhancementJourneyFixture(t *testing.T) devconfig.Fixture {
	t.Helper()
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	raw, err := os.ReadFile(os.Getenv("MINIMIR_EQUIPMENT_ENHANCEMENT_TEST_FIXTURE"))
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	rules, boss := fixture.EquipmentEnhancement, fixture.PersonalBoss
	if rules == nil || rules.StoneItemID != 900_041 || len(rules.Levels) != 50 ||
		rules.Levels[0] != (devconfig.EquipmentEnhancementLevel{StoneCost: 10, SuccessPPM: 1_000_000, BonusBP: 10}) ||
		rules.Levels[1] != (devconfig.EquipmentEnhancementLevel{CurrentLevel: 1, StoneCost: 10, SuccessPPM: 900_000, BonusBP: 10}) ||
		boss == nil || boss.TemplateID != 1 || boss.ItemReward == nil || len(boss.ItemReward.Items) != 1 ||
		boss.ItemReward.Items[0] != (devconfig.ChallengeRewardItem{ItemID: rules.StoneItemID, Quantity: 10}) ||
		fixture.TownTravel == nil || len(fixture.TownTravel.DestinationMapIDs) == 0 {
		t.Fatal("强化规则和原首领投影与工作簿及本轮隔离装配不一致")
	}
	spawn := devconfig.Point{X: boss.Map.PlayerSpawn.X + 1, Y: boss.Map.PlayerSpawn.Y}
	if !boss.Map.Walkable(spawn) {
		t.Fatal("本轮原首领出生点不可攻击")
	}
	boss.Map.MonsterSpawns = []devconfig.Point{spawn}
	boss.Monster.HP, boss.Monster.Agility, boss.Monster.AttackMin, boss.Monster.AttackMax = 1, 0, 0, 0
	boss.Monster.Defence, boss.Monster.DefenceMin, boss.Monster.DefenceMax = 0, 0, 0
	boss.SuccessCooldown = time.Millisecond
	return fixture
}

// winEnhancementPersonalBoss 等待真实首领终态，逐笔核对原场次、背包和无卡绑定材料。
func winEnhancementPersonalBoss(t *testing.T, ctx context.Context, store *persistence.Store, c *client, roleID string, boss *devconfig.PersonalBossConfig) domain.Item {
	t.Helper()
	booking, _ := enterReconnectBoss(t, c, boss.TemplateID, true)
	if c.entry.RoomId != boss.Map.ID {
		t.Fatal("正常首领预约未进入原模板场景", c.entry)
	}
	returnFromWorldBossTest(c, booking.ChallengeId)
	var terminal persistence.PersonalBossChallenge
	must(t, store.DB.Collection("personal_boss_challenges").FindOne(ctx, bson.M{"_id": booking.ChallengeId, "role_id": roleID}).Decode(&terminal))
	if terminal.Outcome != persistence.PersonalBossDefeated || terminal.Damage != boss.Monster.HP || terminal.HPRemaining != 0 ||
		terminal.TemplateID != boss.TemplateID || terminal.RewardQueued || len(terminal.RewardItems) != 1 {
		t.Fatal("实际个人首领没有胜利且直接发整批原强化石", terminal)
	}
	stone := terminal.RewardItems[0]
	if stone.ItemID != 900_041 || stone.Quantity != 10 || !stone.Bound || stone.SourceKind != "personal_boss" || stone.SourceID != booking.ChallengeId {
		t.Fatal("直接到账的石堆没有原场次、数量和无卡绑定", stone, terminal)
	}
	role, err := store.Role(ctx, roleID)
	must(t, err)
	syncWingInventory(c, uint64(role.Revision))
	if !slices.Contains(role.Items, stone) || !slices.ContainsFunc(c.inventory.Items, func(item *pb.ItemView) bool {
		return proto.Equal(item, stone.Proto())
	}) {
		t.Fatal("真实首领原石未同时进入背包档案和 WS 快照", stone)
	}
	return stone
}

// advanceEnhancementRoleLevel 只接受原普通图实际击杀发送的经验确认，达级立即关闭自动战斗。
func advanceEnhancementRoleLevel(t *testing.T, c *client) *pb.ExperienceGrantResult {
	t.Helper()
	c.auto(true, 1)
	grant := &pb.ExperienceGrantResult{}
	travelUntilWithin(c, 40*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CExperienceGrantResult {
			return false
		}
		must(t, proto.Unmarshal(frame.Payload, grant))
		return !grant.Migrated && grant.AcceptedExperience > 0 && grant.LevelBefore == 1 && grant.LevelAfter == 2
	})
	c.auto(false, 1)
	return grant
}

// enhancementQuoteRequest 明确选定本场收到的实际石堆、版本和绑定授权。
func enhancementQuoteRequest(c *client, stone domain.Item, allowBound bool) *pb.EquipmentEnhancementQuoteRequest {
	ref := &pb.EquipmentEnhancementStoneRef{InstanceId: domain.Bytes(stone.ID), Version: uint64(stone.Version), Quantity: stone.Quantity}
	return &pb.EquipmentEnhancementQuoteRequest{SceneEpoch: c.entry.SceneEpoch, Slot: enhancementWeaponSlot,
		AllowBound: allowBound, UiSessionId: domain.Bytes(domain.ID()), Stones: []*pb.EquipmentEnhancementStoneRef{ref}}
}

// enhancementQuoteWS 对拍同请求的部位、等级、概率、加成基点及确切原料。
func enhancementQuoteWS(t *testing.T, c *client, request *pb.EquipmentEnhancementQuoteRequest, stone domain.Item, level, currentBP, targetBP, chance uint32) *pb.EquipmentEnhancementQuote {
	t.Helper()
	id := c.send(p.MsgC2SEquipmentEnhancementQuote, request)
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == id })
	if frame.ID != p.MsgS2CEquipmentEnhancementQuote || frame.Error != 0 {
		t.Fatal("实际强化报价失败", frame)
	}
	var quote pb.EquipmentEnhancementQuote
	must(t, proto.Unmarshal(frame.Payload, &quote))
	if len(quote.QuoteId) != 16 || quote.SceneEpoch != c.entry.SceneEpoch || quote.Slot != enhancementWeaponSlot ||
		quote.CurrentLevel != level || quote.TargetLevel != level+1 || quote.CurrentBonusBp != currentBP || quote.TargetBonusBp != targetBP ||
		quote.SuccessPpm != chance || quote.AllowBound != request.AllowBound || !slices.Equal(quote.UiSessionId, request.UiSessionId) ||
		quote.RoleRevision != c.inventory.Revision || quote.ExpiresAtUnixMs <= uint64(time.Now().UnixMilli()) ||
		len(quote.Stones) != 1 || !proto.Equal(quote.Stones[0].Item, stone.Proto()) || quote.Stones[0].Quantity != stone.Quantity {
		t.Fatal("报价改动了原部位、强化档位、绑定选择或石堆", &quote, stone)
	}
	return &quote
}

// enhancementCommitRequest 让原报价修订和操作编号跟随首次确认及其后所有重试。
func enhancementCommitRequest(c *client, quote *pb.EquipmentEnhancementQuote) *pb.EquipmentEnhancementCommitRequest {
	op := c.operation()
	op.ExpectedRevision = quote.RoleRevision
	return &pb.EquipmentEnhancementCommitRequest{QuoteId: quote.QuoteId, UiSessionId: quote.UiSessionId, Slot: quote.Slot, Operation: op}
}

// waitEnhancementReceipt 读取独立持久回执和同事务审计，等待期间不读取客户端响应。
func waitEnhancementReceipt(t *testing.T, ctx context.Context, store *persistence.Store, roleID string, request *pb.EquipmentEnhancementCommitRequest) persistence.Operation {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	filter := bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(request.Operation.OpId)}
	var receipt persistence.Operation
	for {
		err := store.DB.Collection("operations").FindOne(deadline, filter).Decode(&receipt)
		if !errors.Is(err, mongo.ErrNoDocuments) || deadline.Err() != nil {
			must(t, err)
			break
		}
		time.Sleep(35 * time.Millisecond)
	}
	if receipt.Kind != persistence.AssetEquipmentEnhancement || receipt.Code != 0 || receipt.EquipmentEnhancement == nil ||
		receipt.RoleID != roleID || uint64(receipt.CommandSeq) != request.Operation.CommandSeq {
		t.Fatal("实际确认缺少原部位强化回执", receipt)
	}
	var audit struct {
		Enhancement *persistence.EquipmentEnhancementReceipt `bson:"equipment_enhancement"`
	}
	must(t, store.DB.Collection("audit_outbox").FindOne(ctx, filter).Decode(&audit))
	if !reflect.DeepEqual(audit.Enhancement, receipt.EquipmentEnhancement) {
		t.Fatal("强化实际消费与原回执未随同一事务写审计", audit.Enhancement, receipt.EquipmentEnhancement)
	}
	return receipt
}

// assertEnhancementReceipt 对拍 40057 的实际概率结果、原扣量及永久类型回执。
func assertEnhancementReceipt(t *testing.T, actual *pb.OperationResult, stored persistence.Operation, quote *pb.EquipmentEnhancementQuote, success bool, level uint32) {
	t.Helper()
	receipt := actual.GetEquipmentEnhancement()
	if actual.ErrorCode != 0 || actual.Pending || receipt == nil || !proto.Equal(receipt, stored.EquipmentEnhancement.Proto()) ||
		!slices.Equal(receipt.QuoteId, quote.QuoteId) || receipt.Slot != quote.Slot || receipt.BeforeLevel != quote.CurrentLevel ||
		receipt.AfterLevel != level || receipt.Success != success || len(receipt.Stones) != 1 || !proto.Equal(receipt.Stones[0], quote.Stones[0]) {
		t.Fatal("原强化结果、等级或直接到账的扣料不符", actual, stored.EquipmentEnhancement)
	}
}

// assertEnhancementSlots 核对新角色和已强化角色都始终能看到十个常规角色部位。
func assertEnhancementSlots(t *testing.T, inventory *pb.InventoryResult, level uint32) {
	t.Helper()
	slots := inventory.GetEquipmentEnhancementSlots()
	if len(slots) != domain.EquipmentEnhancementSlots {
		t.Fatal("部位强化快照需要十个拥有明确编号的常规部位", slots)
	}
	for index, slot := range slots {
		var want uint32
		if index == enhancementWeaponSlot {
			want = level
		}
		if slot == nil || slot.Slot != uint32(index) || slot.Level != want {
			t.Fatal("部位快照缺失或训练了非武器部位", index, slot, want)
		}
	}
}

// assertEnhancementAssets 保证报价和拒绝不会改动角色版本、钱包、装备或所选原石。
func assertEnhancementAssets(t *testing.T, after, before domain.Role, items []domain.Item) {
	t.Helper()
	if after.Wallet != before.Wallet || after.Level != before.Level || after.SourceSeq != before.SourceSeq ||
		after.EconomicSeq != before.EconomicSeq || after.Revision != before.Revision || !slices.Equal(after.Items, items) ||
		!slices.Equal(after.Equipped, before.Equipped) || !reflect.DeepEqual(after.EquipmentEnhancement, before.EquipmentEnhancement) {
		t.Fatal("原报价或拒绝改动永久资产", before.Revision, after.Revision)
	}
}

// assertEnhancementSettlement 比对本轮只扣一份原石，结果只改变对应的部位等级和经济水位。
func assertEnhancementSettlement(t *testing.T, after, before domain.Role, stone domain.Item, success bool) {
	t.Helper()
	remaining := slices.DeleteFunc(slices.Clone(before.Items), func(item domain.Item) bool { return item.ID == stone.ID })
	want := before
	want.Revision++
	want.EconomicSeq++
	if success {
		state := domain.EquipmentEnhancement{RoleID: before.ID, Version: want.Revision}
		state.Levels[enhancementWeaponSlot] = 1
		want.EquipmentEnhancement = &state
	}
	assertEnhancementAssets(t, after, want, remaining)
}
