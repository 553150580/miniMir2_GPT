//go:build m4_economy_sampling

// 本文件通过真实协议准备对照角色、推进墙钟窗口并在收窗时结清拍账。
package integration

import (
	"context"
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

const (
	economyHealthPotion = uint32(35) // 当前表的小量金创药。
	economyManaPotion   = uint32(36) // 当前表的小量魔法药。
	economyStock        = int64(200) // 各角色相同的隔离启动药量。
)

// economyActor 保存真实账号与所选角色。测试账号口令只用于本隔离进程的短票续签。
type economyActor struct {
	t        *testing.T         // 当前样本断言作用域。
	ctx      context.Context    // 账号及游戏节点生命周期。
	cfg      devconfig.Config   // 实际加载的规则和节点地址。
	store    *persistence.Store // 只读对拍与单次前态准备使用的隔离库。
	scenario economyScenario    // 当前角色固定观测条件。
	endpoint string             // 隔离账号 RPC 地址。
	username string             // 测试注册用户名。
	password string             // 测试进程内的虚拟账号口令。
	token    string             // 最近账号令牌，长窗结束时重新签发。
	logins   chan struct{}      // 与其他样本共用的真实登录并发窗口。
	role     *pb.RoleKey        // 原永久入口与角色键。
	client   *client            // 当前真正入场的游戏连接。
	added    int64              // 隔离启动预置的历日累计时长。
	claimed  int64              // 从日领协议实际到账的托管时长。
	paidCard int64              // 本窗口之前，为达到实验卡状态的实付非绑元宝最小单位。
}

// prepareEconomyActor 只写入可重复的起始等级、装备物品与资金。穿戴、学书、购卡走正常协议。
func prepareEconomyActor(t *testing.T, ctx context.Context, cfg devconfig.Config, store *persistence.Store, scenario economyScenario, startWallet domain.Wallet, logins chan struct{}) *economyActor {
	t.Helper()
	actor := &economyActor{t: t, ctx: ctx, cfg: cfg, store: store, scenario: scenario, endpoint: "http://" + cfg.AccountAddress + "/rpc",
		username: "econ_" + domain.ID()[:12], password: "economy_test_8753", logins: logins}
	actor.login(true)
	summary := &pb.RoleSummary{}
	rpc(t, actor.endpoint, actor.token, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID,
		Slot: 1, Job: scenario.Job, Name: "经济样本" + domain.ID()[:8], OpId: domain.Bytes(domain.ID())}, summary, 0)
	actor.role = summary.Key
	f := &cfg.Fixture
	created := actor.readRole()
	if created.Level != 1 || created.SourceSeq != 0 || created.HostingSeconds != 0 || len(created.Equipped) != 0 {
		t.Fatal("隔离角色起点与新角身份不符", created.ID)
	}
	initial := slices.Clone(created.Items)
	for _, id := range economyKits[scenario.Kit] {
		definition, _ := f.Item(id) // 配装资格已在整个矩阵启动前核对。
		initial = append(initial, domain.Item{ID: domain.ID(), ItemID: id, Quantity: 1, Bound: true, Version: 1,
			Quality: definition.DropQuality, SourceKind: "economy_sample", DurabilityU: definition.OriginalDurabilityU,
			MaxDurabilityU: definition.OriginalDurabilityU, OriginalDurabilityU: definition.OriginalDurabilityU})
	}
	for _, id := range []uint32{economyHealthPotion, economyManaPotion} {
		item, found := f.Item(id)
		if !found || !item.Stackable || item.MaxStack < economyStock {
			t.Fatal("起始药水库存无法使用现有模板", id)
		}
		initial = append(initial, domain.Item{ID: domain.ID(), ItemID: id, Quantity: economyStock, Bound: true, Version: 1, SourceKind: "economy_sample"})
	}
	withHosting := slices.Contains(economyCardProducts[scenario.Cards], uint32(8))
	withPrivilege := slices.Contains(economyCardProducts[scenario.Cards], uint32(7))
	for _, id := range economyCardProducts[scenario.Cards] {
		product, found := f.RoleCardProduct(id)
		if !found || product.PriceYuanbaoUnits <= 0 {
			t.Fatal("隔离卡分组缺少正常商品", id)
		}
		actor.paidCard += product.PriceYuanbaoUnits
	}
	if scenario.Mode == "hosting" {
		limit := f.HostingBenefits.FreeDailySeconds
		if withHosting {
			limit = f.HostingBenefits.CardDailySeconds
		}
		actor.added = max(scenario.Seconds-limit, 0)
	}
	startWallet.Yuanbao += actor.paidCard
	matched, err := store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": created.ID, "session_epoch": created.SessionEpoch,
		"revision": created.Revision, "source_seq": created.SourceSeq}, bson.M{"$set": bson.M{
		"items": initial, "level": scenario.Level, "wallet": startWallet, "hosting_seconds": actor.added}})
	must(t, err)
	if matched.ModifiedCount != 1 {
		t.Fatal("独立角色附着前的对照前态没有落库")
	}
	actor.open(5 * time.Minute)
	client := actor.client
	client.auto(false, scenario.Slots)
	if client.entry.GetResourceVersion() != f.RuleVersion {
		t.Fatal("联机资源版本与采样夹具不一致")
	}
	for _, id := range economyKits[scenario.Kit] {
		item := naturalGrowthItem(client, id, "economy_sample")
		definition, _ := f.Item(id)
		if item == nil {
			t.Fatal("入场未同步同一原装备", id)
		}
		actor.requireOperation(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: item.InstanceId, ItemVersion: item.Version,
			Slot: definition.EquipmentSlot, Operation: client.operation()})
	}
	naturalEquipmentLearnStarter(t, client, f, scenario.Job)
	// 背包中的原新手装备在窗口前锁定，避免将赠送的原件计入实际挂机毛金。
	for _, item := range client.inventory.GetItems() {
		if definition, found := f.Item(item.ItemId); found && definition.IsEquipment && !item.Locked {
			actor.requireOperation(p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: item.InstanceId,
				ItemVersion: item.Version, Locked: true, Operation: client.operation()})
		}
	}
	actor.requireOperation(p.MsgC2SSetAutoRecycle, &pb.SetAutoRecycleRequest{Policy: &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0, 1}},
		Operation: client.operation()})
	for _, id := range economyCardProducts[scenario.Cards] {
		quote := roleCardStateWS(client, id).GetQuote()
		if quote == nil || quote.GetProduct().GetProductId() != id || len(quote.GetQuoteId()) != 16 {
			t.Fatal("角色没有收到正确的实际购卡报价", id)
		}
		receipt := actor.requireOperation(p.MsgC2SBuyRoleCard, &pb.BuyRoleCardRequest{ProductId: id, QuoteId: quote.QuoteId, Operation: client.operation()})
		if receipt.GetRoleCardPurchase() == nil {
			t.Fatal("购卡没有已确认的原回执", id)
		}
	}
	observed := actor.readRole()
	if observed.Job != scenario.Job || observed.Level != scenario.Level || observed.Wallet.Yuanbao != 0 ||
		observed.PrivilegeUntil.After(time.Now()) != withPrivilege || observed.HostingCardUntil.After(time.Now()) != withHosting ||
		len(observed.Equipped) != len(economyKits[scenario.Kit]) || len(observed.Skills) != 1 {
		t.Fatal("窗口开始之前缺少预定的原等级、穿戴、技能或月卡条件", observed.ID)
	}
	if scenario.Mode == "hosting" {
		actor.requireOperation(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: client.operation()})
		claimed := actor.readRole()
		actor.claimed = claimed.HostingSeconds - actor.added
		if actor.claimed <= 0 || claimed.ClaimedSeconds != actor.claimed || claimed.HostingSeconds < scenario.Seconds {
			t.Fatal("实际日领与隔离累计余额不足以启动原定离线窗口", claimed.HostingSeconds, claimed.ClaimedSeconds)
		}
	}
	if client.entry.RoomId != scenario.MapID {
		travelToBaseline(client, 2, scenario.MapID)
		travelAcknowledge(client, false)
	}
	client.auto(false, scenario.Slots)
	if travelCount(client.scene, 3) != int(scenario.Slots) {
		travelUntilWithin(client, 18*time.Second, func(frame wire.Frame) bool {
			return frame.ID == p.MsgS2CSceneBaseline && travelCount(client.scene, 3) == int(scenario.Slots)
		})
	}
	if client.player().GetHp() <= 0 {
		t.Fatal("普通图基线没有活着的采样角色")
	}
	return actor
}

// login 通过标准注册或登录更新短期账号令牌，长时离线结束后也能按原角色取票。
func (a *economyActor) login(register bool) {
	select {
	case a.logins <- struct{}{}:
		defer func() { <-a.logins }()
	case <-a.ctx.Done():
		a.t.Fatal("独立服务退出，原样本无法登录")
	}
	message := p.MsgC2SLocalLogin
	if register {
		message = p.MsgC2SLocalRegister
	}
	result := &pb.LoginResult{}
	rpc(a.t, a.endpoint, "", message, &pb.LocalLoginRequest{Account: a.username, Password: a.password}, result, 0)
	a.token = result.AccountToken
}

// open 完成正常入场握手。采用样本所需的独立生命周期，默认 ACK 的超时不延长协议本身。
func (a *economyActor) open(timeout time.Duration) {
	c := joinWithAutoSkills(a.t, a.cfg.GameAddress, issue(a.t, a.endpoint, a.token, a.role), &pb.AutoSkillSettings{Enabled: true})
	c.stop()
	c.ctx, c.stop = context.WithTimeout(a.ctx, timeout)
	a.client = c
	a.t.Cleanup(c.close)
}

func (a *economyActor) readRole() domain.Role {
	a.t.Helper()
	role, err := a.store.Role(a.ctx, domain.Hex(a.role.RoleId))
	must(a.t, err)
	return role
}

// observe 使用半开墙钟窗口。暂停后再取确认水位，边界上的额外收益单列并参与全账对拍。
func (a *economyActor) observe(window time.Duration, maintain bool, shopAccess chan struct{}) economyReport {
	t := a.t
	c := a.client
	quote := hostingQuoteWS(c, 0)
	before := a.readRole()
	report := newEconomyReport(a, before, quote)
	start := time.Now()
	report.Start = start.UTC().Truncate(time.Millisecond)
	report.End = report.Start.Add(window)
	deadline := start.Add(window)
	t.Logf("实际采样开始 %s，模式 %s，起止 %s 至 %s", a.scenario.key(), a.scenario.Mode,
		report.Start.Format(time.RFC3339), report.End.Format(time.RFC3339))
	if a.scenario.Mode == "hosting" {
		c.auto(true, a.scenario.Slots)
		c.close()
		pause := time.NewTimer(time.Until(deadline))
		defer pause.Stop()
		select {
		case <-pause.C:
		case <-a.ctx.Done():
			t.Fatal("隔离服务在原托管窗口结束前退出")
		}
		a.login(false)
		a.open(30 * time.Minute)
		c = a.client
	} else {
		a.observeOnline(c, deadline, &report)
	}
	report.CutoffLagMS = max(time.Since(deadline).Milliseconds(), 0)
	// 已死亡的角色可能没有输入权。此时服务器已停战，仍核对其来源账本。
	if c.player().GetHp() > 0 {
		c.auto(false, a.scenario.Slots)
		if maintain {
			// 退出野外会结清原攻击、死亡与磨损。安全区内的最终装备才是修理报价的原件。
			town, _ := economyRepairVendor(t, &a.cfg.Fixture)
			travelToBaseline(c, 1, town.ID)
			travelAcknowledge(c, true)
		}
	}
	end := a.awaitStill()
	report.EndState = economyState(end)
	ledger, quantities := a.reconcileEconomy(before, end, report.Start, report.End)
	report.Gross, report.Boundary, report.Sources = ledger.Inside, ledger.Outside, ledger.SourceCount
	report.Commands, report.Net = ledger.Commands, ledger.Net
	report.PotionUse = quantities
	report.DepositedHostingSeconds = a.added
	report.ClaimedHostingSeconds = a.claimed
	if maintain {
		a.closeMaintenance(shopAccess, before, end, &report)
	}
	return report
}

// awaitStill 等待至少一个持久检查点周期，确保剩余磨损没有落在对拍水位之外。
func (a *economyActor) awaitStill() domain.Role {
	a.t.Helper()
	last := a.readRole()
	deadline, unchanged := time.Now().Add(15*time.Second), time.Now()
	for {
		time.Sleep(300 * time.Millisecond)
		next := a.readRole()
		if next.Revision != last.Revision {
			last, unchanged = next, time.Now()
		}
		if time.Since(unchanged) >= 4*time.Second {
			return last
		}
		if time.Now().After(deadline) {
			a.t.Fatal("样本停止战斗后，角色修订仍持续变化", last.Revision)
		}
	}
}

// observeOnline 按真实客户端消息确认自动战斗和复活。状态停战仍占原样本墙钟。
func (a *economyActor) observeOnline(c *client, deadline time.Time, report *economyReport) {
	var active bool
	var last = time.Now()
	var death *pb.DeathNotice
	var lastPotion time.Time
	before := func(now time.Time, next bool) {
		if now.After(deadline) {
			now = deadline
		}
		if active {
			report.OnlineActiveMS += now.Sub(last).Milliseconds()
		}
		last, active = now, next
	}
	c.onFrame = func(_ *client, frame wire.Frame) {
		now := time.Now()
		if !now.Before(deadline) || frame.Error != 0 {
			return
		}
		switch frame.ID {
		case p.MsgS2CHostingState:
			before(now, c.hosting.AutoBattleEnabled)
		case p.MsgS2CDeathNotice:
			death = c.death
			report.ObservedDeaths++
			before(now, false)
		}
	}
	defer func() {
		c.onFrame = nil
		before(deadline, false)
	}()
	c.stop()
	c.ctx, c.stop = context.WithTimeout(a.ctx, time.Until(deadline)+30*time.Minute)
	stop := sampleHeartbeat(c, deadline)
	defer stop()
	c.auto(true, a.scenario.Slots)
	for time.Now().Before(deadline) {
		frame := c.read()
		if !time.Now().Before(deadline) {
			break
		}
		if death != nil {
			thisDeath := death
			death = nil
			if thisDeath.GetAllowedActions()&1 != 0 {
				a.revive(thisDeath)
				if time.Now().Before(deadline) {
					c.auto(true, a.scenario.Slots)
				}
			}
			continue
		}
		body := c.player()
		if frame.ID == p.MsgS2CSceneBaseline && body.GetHp() > 0 && body.GetHp()*5 <= body.GetHpMax()*2 &&
			time.Since(lastPotion) >= 4*time.Second {
			lastPotion = time.Now()
			a.useHealthPotion()
		}
	}
}

// revive 沿服务端给出的免费选项复活，保留真实死亡停战与原命令回执。
func (a *economyActor) revive(death *pb.DeathNotice) {
	c := a.client
	a.requireOperation(p.MsgC2SReviveChoice, &pb.ReviveChoice{DeathId: death.DeathId, Action: 1, Operation: c.operation()})
	travelUntilWithin(c, 6*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.player().GetHp() > 0
	})
	c.death = nil
}

// useHealthPotion 表示在线玩家的固定补血策略，磨损结算造成的短暂竞态不会被误算成用药。
func (a *economyActor) useHealthPotion() {
	c := a.client
	for _, item := range c.inventory.GetItems() {
		if item.ItemId != economyHealthPotion {
			continue
		}
		result := c.asset(p.MsgC2SUseItem, &pb.UseItemRequest{ItemInstanceId: item.InstanceId, ItemId: economyHealthPotion,
			SceneEpoch: c.entry.SceneEpoch, Operation: c.operation()})
		switch result.ErrorCode {
		case 0:
			c.inventoryAt(result.NewRevision)
		case p.CodeItemUseBlocked, p.CodeItemUseCooldown, p.CodeOperationConflict, p.CodeServerBusy, p.CodeEquipmentChanged:
			// 下一次角色基线和库存允许后，仍可按正常入口操作。
		default:
			a.t.Fatal("测试角色的在线补血出现未建模的结果", result)
		}
		return
	}
}

// requireOperation 等待同一操作结果和库存版本，避免取前一笔修订报价。
func (a *economyActor) requireOperation(id uint16, message proto.Message) *pb.OperationResult {
	a.t.Helper()
	result := a.client.asset(id, message)
	if result.ErrorCode != 0 || result.Pending || result.NewRevision == 0 {
		a.t.Fatalf("正常经济命令 %d 没有提交：%v", id, result)
	}
	a.client.inventoryAt(result.NewRevision)
	return result
}
