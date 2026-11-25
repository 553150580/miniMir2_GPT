// 通过真实连接与数据库确认背包满时离线战斗行为、扣时和相关停止状态。

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

// setTestOrdinaryEquipmentDrop 固定历史和各怪物新池的装备选择，用于容量和回收回归。
func setTestOrdinaryEquipmentDrop(f *devconfig.Fixture, itemID uint32) {
	f.Drops.EquipmentItemIDs = []uint32{itemID}
	// 用例的白装覆盖全部装备掉落池，原配置的高品质额度在该隔离服没有来源。
	f.OrdinaryEquipmentQuotas = nil
	for i := range f.OrdinaryDropPools {
		f.OrdinaryDropPools[i].Equipment = []devconfig.OrdinaryDropEntry{{ItemID: itemID, Weight: 1, QuantityMin: 1, QuantityMax: 1}}
	}
}

// TestFullBagOfflineCombatAndChargingMongoWS 持续观察满背包角色的离线战斗和计时扣减，核对没有覆盖原物品。
// t 提供断言失败报告、子用例和清理注册。
func TestFullBagOfflineCombatAndChargingMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo/WS离线满包AI、原结算与扣时")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	if cfg.Fixture.RuleVersion < 21 {
		t.Fatal("必须使用当前容量政策21以上配置")
	}
	// offer 记录尝试提交，使用技能商店表示。
	var offer devconfig.SkillShop
	// row 接收遍历技能商店时当前的元素。
	for _, row := range cfg.Fixture.SkillShop {
		// book 保存由道具取得的技能书，ok 表明查询、校验或类型转换是否成功。
		book, ok := cfg.Fixture.Item(row.ItemID)
		// skill 接收技能供后续步骤使用，found 保存由技能取得的已找到标记。
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
	// i 标记遍历初始值背包资产时访问的下标。
	for i := range cfg.Fixture.InitialInventory {
		cfg.Fixture.InitialInventory[i] = 218
	}
	cfg.Fixture.InitialInventory[30] = 193
	cfg.Fixture.InitialBoundGold = offer.PriceUnits
	setTestOrdinaryEquipmentDrop(&cfg.Fixture, 218)
	cfg.Fixture.Monsters[0].HP = 1
	cfg.Fixture.Monsters[0].AttackMin, cfg.Fixture.Monsters[0].AttackMax, cfg.Fixture.Monsters[0].Agility = 0, 0, 0
	// raw 保存由序列化取得的原始值。
	raw, err := json.Marshal(cfg.Fixture)
	must(t, err)
	cfg.Fixture.Hash = persistence.Hash(raw)
	must(t, cfg.Fixture.Validate())
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// 整条场景共用80秒；不增加单项RPC、事务、租约或连接原有时限。
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	// inspection 保存由打开取得的检查。
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	// group 记录分组，使用协程汇合计数器表示。
	var group sync.WaitGroup
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"account", "game", "gm"} {
		group.Go(func() {
			// err 接收运行批次过程中可能产生的错误，供后续失败分支处理。
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("满包离线%s:%v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		group.Wait()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	// endpoint 接收遍历文本列表时当前的元素。
	for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, endpoint)
	}
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "full_off_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	// role 保存当前处理所用的角色。
	role := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 0, Name: "满包托管" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, role, 0)
	// roleID 标识当前业务使用的角色。
	roleID := domain.Hex(role.Key.RoleId)
	// phase 保存阶段所使用的文本。
	phase := "enter"
	// events 记录事件集合，使用装备回收结果列表表示。
	var events []*pb.EquipmentRecycleResult
	// historyFloor 保存由无符号整数“64”取得的“history”下限。
	historyFloor := uint64(0)
	// observe 封装观测对应的后续处理，c 记录客户端，frame 记录协议帧。
	observe := func(c *client, frame wire.Frame) {
		if frame.Error != 0 || frame.ID != p.MsgS2CEquipmentRecycleResult {
			return
		}
		// event 记录事件，使用装备回收结果表示。
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
	// c 接收加入供后续步骤使用。
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), observe)
	c.ctx = ctx
	defer func() { c.close() }()
	defer func() {
		if t.Failed() {
			// stored 保存由角色取得的已持久保存，readErr 保存由角色取得的读取错误。
			stored, readErr := inspection.Role(context.Background(), roleID)
			t.Logf("失败阶段=%s event_count=%d player=%v hosting=%v read_error=%v source=%d rev=%d balance=%d full=%t report=%+v", phase, len(events), c.player(), c.hosting, readErr, stored.SourceSeq, stored.Revision, stored.HostingSeconds, stored.PickupStopped, stored.Report)
		}
	}()
	c.auto(false, 8)
	if len(c.inventory.Items) != int(c.inventory.Capacity) || c.inventory.AutoRecycle == nil || c.inventory.AutoRecycle.Enabled {
		t.Fatal("正常Create未产生预定满包且关闭回收的资格")
	}
	// wood 记录“wood”，使用道具观察视图表示。
	var wood *pb.ItemView
	// item 接收遍历道具集合时当前的元素。
	for _, item := range c.inventory.Items {
		if item.ItemId == 218 {
			wood = item
			break
		}
	}
	if wood == nil {
		t.Fatal("缺少原木剑")
	}
	// result 保存由资产取得的结果。
	result := c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: wood.InstanceId, Slot: 0, ItemVersion: wood.Version, Operation: c.operation()})
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
	// before 保存由角色取得的操作前的状态。
	before, err := inspection.Role(ctx, roleID)
	must(t, err)
	if len(before.Items) != before.Capacity || before.Wallet.Gold != 0 || before.Wallet.BoundGold != 0 || before.AutoRecycle.Enabled || before.HostingSeconds <= 0 {
		t.Fatal("购书未花回0/填满，或资格没有真实托管余额", before)
	}
	t.Logf("隔离资格: 初始绑金=%d=原offer%d报价，正常Create/Equip/Claim/购书后32格且钱包0；1HP/0攻/0敏，原概率/20保底/10秒刷新保持，effective_fixture_sha=%s", offer.PriceUnits, offer.ID, cfg.Fixture.Hash)
	phase = "online_first_actual_discard"
	c.auto(true, 8)
	// first 记录第一项，使用装备回收结果表示。
	var first *pb.EquipmentRecycleResult
	for first == nil {
		// event 接收遍历事件集合时当前的元素。
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
	// playerLife 保存生命代次，用于当前步骤中的玩家生命代次，scene 保存场景代次，用于当前步骤中的场景。
	playerLife, scene := c.player().Life, c.entry.SceneEpoch
	c.close() // 保持Auto=true和满包；仅正常断线，不清包或替换owner现场。
	phase = "offline_full_bag_continuous_combat"
	// poll 保存由创建“ticker”取得的“poll”。
	poll := time.NewTicker(30 * time.Millisecond)
	defer poll.Stop()
	// offline 记录离线，使用角色表示。
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
	// cursor 保存由查找取得的游标。
	cursor, err := inspection.DB.Collection("audit_outbox").Find(ctx, bson.M{"role_id": roleID, "kind": "settlement", "hosting_session": offline.Report.SessionID, "source_seq": bson.M{"$lte": offline.SourceSeq}})
	must(t, err)
	// rows 记录“rows”，使用临时结构列表表示。
	var rows []struct {
		// ID 区分当前步骤指向的目标对象。
		ID string `bson:"_id"`
		// SourceSeq 记录持久收益来源序号，使用数值表示。
		SourceSeq int64 `bson:"source_seq"`
		// Kills 记录击杀集合，使用数值表示。
		Kills int64 `bson:"kills"`
		// Experience 记录经验，使用数值表示。
		Experience int64 `bson:"experience"`
		// Gold 记录金币，使用数值表示。
		Gold int64 `bson:"gold"`
		// Bound 标记是否处于绑定状态。
		Bound bool `bson:"bound"`
		// Item 记录道具。
		Item *domain.Item `bson:"item"`
		// HostSeconds 记录网络主机或托管秒集合，使用数值表示。
		HostSeconds int64 `bson:"host_seconds"`
		// Details 记录“details”，使用结算回执表示。
		Details persistence.SettlementReceipt `bson:"settlement"`
	}
	must(t, cursor.All(ctx, &rows))
	// expected 初始化用作预期结果的报告或战报。
	expected := domain.Report{}
	// row 接收遍历“rows”时当前的元素。
	for _, row := range rows {
		// receipt 记录回执，使用操作表示。
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
	// got 保存报告或战报，用于当前步骤中的实际结果。
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
