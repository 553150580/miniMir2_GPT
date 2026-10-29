package integration

import (
	"context"
	"encoding/json"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestFullBagOfflineCombatAndChargingMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo/WS离线满包AI、原结算与扣时")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion < 21 {
		t.Fatal("必须使用当前容量政策21以上配置")
	}
	var offer devconfig.SkillShop
	for _, row := range cfg.Fixture.SkillShop {
		book, ok := cfg.Fixture.Item(row.ItemID)
		skill, found := cfg.Fixture.Skill(book.BookSkillID)
		if ok && found && skill.JobID == 0 && row.Currency == "gold" && row.RequiredRoleLevel <= 1 && row.PriceUnits > 0 {
			offer = row
			break
		}
	}
	if offer.ID == 0 {
		t.Fatal("缺少当前金币职业书原报价")
	}
	// 仅局部资格：正常Create产生满32格、怪物1HP/零伤害/零敏捷。
	// 初始绑金只取现有书报价，正常购1本花回0，填回装备腾出的格。
	// 不预置任何角色实例、收益、回执或最终报告，不改概率/保底/刷新/主Excel。
	cfg.Fixture.InitialInventory = make([]uint32, 31)
	for i := range cfg.Fixture.InitialInventory {
		cfg.Fixture.InitialInventory[i] = 218
	}
	cfg.Fixture.InitialInventory[30] = 193
	cfg.Fixture.InitialBoundGold = offer.PriceUnits
	cfg.Fixture.Drops.EquipmentItemIDs = []uint32{218}
	cfg.Fixture.Monsters[0].HP = 1
	cfg.Fixture.Monsters[0].AttackMin, cfg.Fixture.Monsters[0].AttackMax, cfg.Fixture.Monsters[0].Agility = 0, 0, 0
	raw, err := json.Marshal(cfg.Fixture)
	must(t, err)
	cfg.Fixture.Hash = persistence.Hash(raw)
	must(t, cfg.Fixture.Validate())
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// 整条场景共用80秒；不增加单项RPC、事务、租约或连接原有时限。
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	var group sync.WaitGroup
	for _, mode := range []string{"account", "game", "gm"} {
		group.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("满包离线%s:%v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		group.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, endpoint)
	}
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "full_off_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	role := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 0, Name: "满包托管" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	phase := "enter"
	var events []*pb.EquipmentRecycleResult
	historyFloor := uint64(0)
	observe := func(c *client, frame wire.Frame) {
		if frame.Error != 0 || frame.ID != p.MsgS2CEquipmentRecycleResult {
			return
		}
		var event pb.EquipmentRecycleResult
		must(t, proto.Unmarshal(frame.Payload, &event))
		if c.inventory == nil || c.inventory.Revision < event.RoleRevision || !reflect.DeepEqual(event.Role.RoleId, role.Key.RoleId) {
			t.Fatal("回收/弃物通知早于当前完整资产或不属于角色", &event)
		}
		if historyFloor > 0 && event.SourceSeq <= historyFloor {
			t.Fatal("健康新票重播已确认旧弃物通知", &event)
		}
		events = append(events, &event)
	}
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), observe)
	c.ctx = ctx
	defer func() { c.close() }()
	defer func() {
		if t.Failed() {
			stored, readErr := inspection.Role(context.Background(), roleID)
			t.Logf("失败阶段=%s event_count=%d player=%v hosting=%v read_error=%v source=%d rev=%d balance=%d full=%t report=%+v", phase, len(events), c.player(), c.hosting, readErr, stored.SourceSeq, stored.Revision, stored.HostingSeconds, stored.PickupStopped, stored.Report)
		}
	}()
	c.auto(false, 8)
	if len(c.inventory.Items) != int(c.inventory.Capacity) || c.inventory.AutoRecycle == nil || c.inventory.AutoRecycle.Enabled {
		t.Fatal("正常Create未产生预定满包且关闭回收的资格")
	}
	var wood *pb.ItemView
	for _, item := range c.inventory.Items {
		if item.ItemId == 218 {
			wood = item
			break
		}
	}
	if wood == nil {
		t.Fatal("缺少原木剑")
	}
	result := c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: wood.InstanceId, Slot: 0, Operation: c.operation()})
	c.inventoryAt(result.NewRevision)
	if result.ErrorCode != 0 {
		t.Fatal("正常装备失败", result)
	}
	result = c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
	c.inventoryAt(result.NewRevision)
	if result.ErrorCode != 0 {
		t.Fatal("正常领取托管失败", result)
	}
	if result = c.skillConfirm(c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: 1})); result.ErrorCode != 0 {
		t.Fatal("正常报价购书填格失败", result)
	}
	before, err := inspection.Role(ctx, roleID)
	must(t, err)
	if len(before.Items) != before.Capacity || before.Wallet.Gold != 0 || before.Wallet.BoundGold != 0 || before.AutoRecycle.Enabled || before.HostingSeconds <= 0 {
		t.Fatal("购书未花回0/填满，或资格没有真实托管余额", before)
	}
	t.Logf("隔离资格: 初始绑金=%d=原offer%d报价，正常Create/Equip/Claim/购书后32格且钱包0；1HP/0攻/0敏，原概率/20保底/10秒刷新保持，effective_fixture_sha=%s", offer.PriceUnits, offer.ID, cfg.Fixture.Hash)
	phase = "online_first_actual_discard"
	c.auto(true, 8)
	var first *pb.EquipmentRecycleResult
	for first == nil {
		for _, event := range events {
			if event.Trigger == 1 && event.DiscardedCount == 1 {
				first = event
				break
			}
		}
		if first == nil {
			c.read()
		}
	}
	if first.DiscardReason != "bag_full" || first.RecycledCount != 0 || c.hosting == nil || !c.hosting.AutoBattleEnabled {
		t.Fatal("满包未实际弃物或Auto被关闭", first, c.hosting)
	}
	playerLife, scene := c.player().Life, c.entry.SceneEpoch
	c.close() // 保持Auto=true和满包；仅正常断线，不清包或替换owner现场。
	phase = "offline_full_bag_continuous_combat"
	poll := time.NewTicker(30 * time.Millisecond)
	defer poll.Stop()
	var offline domain.Role
waiting:
	for {
		select {
		case <-ctx.Done():
			t.Fatal("整体80秒内未观察离线满包>=2击杀、实际扣时及再次弃物")
		case <-poll.C:
			offline, err = inspection.Role(ctx, roleID)
			must(t, err)
			if offline.Report.SessionID != "" && offline.Report.Kills >= 2 && offline.Report.UsedSeconds >= 1 && offline.Report.DiscardCount >= 1 && offline.Report.Experience > 0 {
				break waiting
			}
		}
	}
	if !offline.PickupStopped || len(offline.Items) != offline.Capacity || offline.AutoRecycle.Enabled || offline.HostingSeconds >= before.HostingSeconds || before.HostingSeconds-offline.HostingSeconds != offline.Report.UsedSeconds || offline.Report.StopReason != 0 || offline.Report.LastDiscardReason != "bag_full" || len(offline.Report.Drops) != 0 || offline.Report.RecycleCount != 0 {
		t.Fatal("离线满包未继续原规则或报告/扣时不符", offline)
	}
	// 固定该已提交SourceSeq截面，逐原HostingSession重算，不把物品样本或钱包差额当账。
	cursor, err := inspection.DB.Collection("audit_outbox").Find(ctx, bson.M{"role_id": roleID, "kind": "settlement", "hosting_session": offline.Report.SessionID, "source_seq": bson.M{"$lte": offline.SourceSeq}})
	must(t, err)
	var rows []struct {
		ID          string                        `bson:"_id"`
		SourceSeq   int64                         `bson:"source_seq"`
		Kills       int64                         `bson:"kills"`
		Experience  int64                         `bson:"experience"`
		Gold        int64                         `bson:"gold"`
		Bound       bool                          `bson:"bound"`
		Item        *domain.Item                  `bson:"item"`
		HostSeconds int64                         `bson:"host_seconds"`
		Details     persistence.SettlementReceipt `bson:"settlement"`
	}
	must(t, cursor.All(ctx, &rows))
	expected := domain.Report{}
	for _, row := range rows {
		var receipt persistence.Operation
		must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": row.ID}).Decode(&receipt))
		if receipt.Kind != "settlement" || receipt.RoleID != roleID || receipt.SourceSeq != row.SourceSeq || receipt.Settlement == nil || !reflect.DeepEqual(*receipt.Settlement, row.Details) || row.Details.Experience == nil {
			t.Fatal("原session来源的typed回执与审计不一致", row.ID)
		}
		expected.Kills += row.Kills
		expected.UsedSeconds += row.HostSeconds
		expected.Sequence++
		expected.RawExperience += row.Experience
		expected.Experience += row.Details.Experience.ActualGranted
		expected.PooledExperience += row.Details.Experience.ToPool
		expected.RejectedExperience += row.Details.Experience.Rejected
		if row.Bound {
			expected.Earned.BoundGold += row.Gold
		} else {
			expected.Earned.Gold += row.Gold
		}
		if row.Item != nil {
			expected.TotalDropCount++
			if row.Details.GrantedItem || row.Details.Recycling.DiscardedCount != 1 || row.Details.Recycling.DiscardReason != "bag_full" {
				t.Fatal("满包原物品没有真正放弃", row.ID)
			}
		}
		expected.DiscardCount += row.Details.Recycling.DiscardedCount
	}
	got := offline.Report
	if got.Kills != expected.Kills || got.UsedSeconds != expected.UsedSeconds || got.Sequence != expected.Sequence || got.RawExperience != expected.RawExperience || got.Experience != expected.Experience || got.PooledExperience != expected.PooledExperience || got.RejectedExperience != expected.RejectedExperience || got.Earned != expected.Earned || got.DiscardCount != expected.DiscardCount || got.TotalDropCount != expected.TotalDropCount {
		t.Fatal("满包离线聚合不等于原typed回执/审计截面", got, expected)
	}
	// 不要求随机窗口一定掉金；其实际数值逐项等原audit，确定性金币分支另由Store测试。
	historyFloor = uint64(offline.SourceSeq)
	phase = "healthy_reentry_no_historical_discard_replay"
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), observe)
	c.ctx = ctx
	c.auto(false, 8)
	if c.entry.SceneEpoch != scene || c.player().Life != playerLife || c.report == nil || c.report.HostingSessionId == nil || domain.Hex(c.report.HostingSessionId) != offline.Report.SessionID || c.report.Kills < uint64(got.Kills) || c.report.UsedSeconds < uint64(got.UsedSeconds) || c.hosting.AutoBattleEnabled {
		t.Fatal("健康票重建了现场、丢失原session报告或正常关闭Auto失败")
	}
	t.Logf("真实满包离线通过: session=%s SourceSeq=%d kills=%d used=%d discard=%d EXP=%d raw=%d gold=%d bound_gold=%d sameScene=%d sameLife=%d；资金/库存/1HP仅隔离资格，非自然供给", got.SessionID, offline.SourceSeq, got.Kills, got.UsedSeconds, got.DiscardCount, got.Experience, got.RawExperience, got.Earned.Gold, got.Earned.BoundGold, scene, playerLife)
}
