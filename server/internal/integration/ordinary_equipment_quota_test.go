// 本文件验证普通怪装备额度的真实 MongoDB 事务，以及手动战斗与离线托管共用额度的联机路径。
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// quotaDropSource 把验收用装备档位与原有普通地图、怪物和掉落池关联。
type quotaDropSource struct {
	mapID   uint64
	monster devconfig.Monster
	high    devconfig.Item
	low     devconfig.Item
	quota   devconfig.OrdinaryEquipmentQuota
}

// quotaFixture 只在隔离环境设置装备档位、权重及稀有供给，保留正式分类与20杀保底。
func quotaFixture(t *testing.T, fixture *devconfig.Fixture, twoStages bool) []quotaDropSource {
	t.Helper()
	if fixture.Drops.EquipmentGuarantee != 20 {
		t.Fatalf("普通击杀保底须为20次，当前为%d", fixture.Drops.EquipmentGuarantee)
	}
	type stageItems struct{ high, low uint32 }
	stages := []stageItems{{220, 218}}
	maps := []devconfig.MapTemplate{fixture.Map}
	primary, known := fixture.MonsterByID(fixture.Map.MonsterSpawnIDs[0])
	if !known {
		t.Fatal("主地图缺少原普通怪物配置")
	}
	if twoStages {
		for _, candidate := range fixture.Maps {
			if candidate.SceneKindValue() != devconfig.SceneKindPersonal {
				continue
			}
			monster, known := fixture.MonsterByID(candidate.MonsterSpawnIDs[0])
			if known && monster.DropPoolID != primary.DropPoolID {
				maps = append(maps, candidate)
				break
			}
		}
		if len(maps) != 2 {
			t.Fatal("第二阶段验收须使用另一只怪物的原普通掉落池")
		}
		stages = append(stages, stageItems{261, 195})
	}
	entry := func(id uint32, weight int64) devconfig.OrdinaryDropEntry {
		return devconfig.OrdinaryDropEntry{ItemID: id, Weight: weight, QuantityMin: 1, QuantityMax: 1}
	}
	for i := range fixture.OrdinaryDropPools {
		fixture.OrdinaryDropPools[i].Equipment = []devconfig.OrdinaryDropEntry{entry(218, 1)}
	}
	fixture.OrdinaryEquipmentQuotas = nil
	sources := make([]quotaDropSource, 0, len(stages))
	for i, ids := range stages {
		for j := range fixture.Items {
			if fixture.Items[j].ID == ids.high {
				fixture.Items[j].DropQuality = 2
				break
			}
		}
		high, highOK := fixture.Item(ids.high)
		low, lowOK := fixture.Item(ids.low)
		monster, monsterOK := fixture.MonsterByID(maps[i].MonsterSpawnIDs[0])
		if !highOK || !lowOK || !monsterOK || high.EquipmentStage != uint32(i) || low.EquipmentStage != high.EquipmentStage || low.DropQuality != 0 {
			t.Fatalf("隔离档位%d缺少原阶段装备或原怪物", i)
		}
		for j := range fixture.OrdinaryDropPools {
			pool := &fixture.OrdinaryDropPools[j]
			if pool.ID == monster.DropPoolID {
				// 极小的低档直接抽中概率保留正式加权抽样。该模板也是额度穷尽时的唯一降档选项。
				pool.Equipment = []devconfig.OrdinaryDropEntry{entry(ids.high, 1<<50), entry(ids.low, 1)}
			}
		}
		quota := devconfig.OrdinaryEquipmentQuota{
			ID: uint32(i + 1), Stage: uint32(i), Quality: high.DropQuality, SourceKind: devconfig.OrdinaryMonsterDropSource,
			WindowSeconds: 86400, QuotaUnits: 1, RefillSchedule: []devconfig.DropQuotaRefill{{OffsetSeconds: 1, Units: 1}},
			ExhaustedDowngradeWeights: map[uint32]int64{low.DropQuality: 1},
		}
		fixture.OrdinaryEquipmentQuotas = append(fixture.OrdinaryEquipmentQuotas, quota)
		sources = append(sources, quotaDropSource{mapID: maps[i].ID, monster: *monster, high: high, low: low, quota: quota})
	}
	return sources
}

// quotaFreezeFixture 确认隔离配置合法，并为收益回执采用真实快照摘要。
func quotaFreezeFixture(t *testing.T, fixture *devconfig.Fixture) {
	t.Helper()
	must(t, fixture.Validate())
	raw, err := json.Marshal(fixture)
	must(t, err)
	fixture.Hash = persistence.Hash(raw)
}

// quotaSettlement 创建一条固定战斗事实，以真实 Store 事务精确覆盖分窗和重试边界。
func quotaSettlement(role domain.Role, source quotaDropSource, at time.Time, hosted bool) persistence.Settlement {
	seq := role.SourceSeq + 1
	item := domain.Item{ID: domain.ID(), ItemID: source.high.ID, Quantity: 1, Version: 1, Quality: source.high.DropQuality,
		Bound: true, SourceKind: "monster_drop", SourceID: fmt.Sprint(seq)}
	lower := item
	lower.ItemID, lower.Quality = source.low.ID, source.low.DropQuality
	q := persistence.Settlement{SourceSeq: seq, Kills: 1, Experience: source.monster.Exp, Bound: item.Bound, Item: &item,
		MapID: source.mapID, MonsterID: source.monster.ID, DropPoolID: source.monster.DropPoolID, EventAt: at,
		OrdinaryEquipment: &persistence.OrdinaryEquipmentPlan{Fallbacks: []domain.Item{lower}}}
	if hosted {
		q.HostingSession, q.HostSeconds = "quota-isolated-hosting", 1
	}
	return q
}

// quotaCommit 保持同一请求身份重试真实 MongoDB 并发冲突。
func quotaCommit(ctx context.Context, store *persistence.Store, auth persistence.Auth, q persistence.Settlement) (domain.Role, persistence.Operation, error) {
	for range 20 {
		role, op, err := store.SettleWithReceipt(ctx, auth, q)
		if persistence.Code(err) != p.CodeOperationPending && persistence.Code(err) != p.CodeServerBusy {
			return role, op, err
		}
		select {
		case <-ctx.Done():
			return role, op, ctx.Err()
		case <-time.After(15 * time.Millisecond):
		}
	}
	return domain.Role{}, persistence.Operation{}, fmt.Errorf("收益来源%d持续等待原确认", q.SourceSeq)
}

// quotaAudit 读取一笔击杀最终写入审计的原时间、托管归属和实际装备。
type quotaAudit struct {
	ID             string                        `bson:"_id"`
	SourceSeq      int64                         `bson:"source_seq"`
	EventAt        time.Time                     `bson:"event_at"`
	HostingSession string                        `bson:"hosting_session"`
	Item           domain.Item                   `bson:"item"`
	Settlement     persistence.SettlementReceipt `bson:"settlement"`
}

// quotaCheckAudit 从 MongoDB 比对原操作回执、实际审计物品及唯一来源身份。
func quotaCheckAudit(t *testing.T, ctx context.Context, store *persistence.Store, roleID string, audit quotaAudit, source quotaDropSource) persistence.Operation {
	t.Helper()
	var op persistence.Operation
	must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": audit.ID}).Decode(&op))
	if op.RoleID != roleID || op.Kind != "settlement" || op.SourceSeq != audit.SourceSeq || op.Settlement == nil ||
		op.Settlement.Kills != 1 || op.Settlement.MonsterID != source.monster.ID || op.Settlement.MapID != source.mapID ||
		op.Settlement.DropPoolID != source.monster.DropPoolID || op.Settlement.ConfigHash != store.Cfg.Fixture.Hash ||
		op.Settlement.OrdinaryEquipment == nil || !reflect.DeepEqual(*op.Settlement, audit.Settlement) ||
		audit.Item != op.Settlement.OrdinaryEquipment.Item {
		t.Fatalf("MongoDB 审计与永久回执没有对应同一次普通击杀：audit=%+v op=%+v", audit, op)
	}
	return op
}

// quotaCheckAward 检查真实装备、保底、同一实例的绑定和资产去向。
func quotaCheckAward(t *testing.T, before, after domain.Role, op persistence.Operation, source quotaDropSource, wantItem uint32, disposition string, consumed bool) {
	t.Helper()
	if after.NoEquipmentKills != 0 || op.Settlement == nil || op.Settlement.OrdinaryEquipment == nil {
		t.Fatalf("第20杀没有归零保底或生成装备回执：pity=%d op=%+v", after.NoEquipmentKills, op)
	}
	actual := op.Settlement.OrdinaryEquipment
	wantQuality := source.low.DropQuality
	if wantItem == source.high.ID {
		wantQuality = source.high.DropQuality
	}
	if actual.CandidateItemID != source.high.ID || actual.Stage != source.high.EquipmentStage || actual.Item.ItemID != wantItem ||
		actual.Item.Quality != wantQuality || actual.Item.SourceKind != "monster_drop" || !actual.Item.Bound || actual.Item.Quantity != 1 ||
		actual.Item.SourceID != fmt.Sprint(op.SourceSeq) || actual.Disposition != disposition || actual.QuotaConsumed != consumed {
		t.Fatalf("实际掉落、出处或额度扣减不符：want=%d/%s/%t actual=%+v", wantItem, disposition, consumed, actual)
	}
	inBag := slices.Contains(after.Items, actual.Item)
	if op.Settlement.GrantedItem != inBag || inBag != (disposition == persistence.OrdinaryEquipmentGranted) {
		t.Fatalf("永久回执与实际背包不一致：%+v", actual)
	}
	switch disposition {
	case persistence.OrdinaryEquipmentDiscarded:
		if op.Settlement.Recycling.DiscardedCount != 1 || op.Settlement.Recycling.DiscardReason != "bag_full" || !reflect.DeepEqual(before.Items, after.Items) || before.Wallet != after.Wallet {
			t.Fatal("满包丢弃修改了保护物品或钱包", op.Settlement.Recycling)
		}
	case persistence.OrdinaryEquipmentRecycled:
		if !slices.Contains(op.Settlement.Recycling.Recycled, actual.Item) || op.Settlement.Recycling.BoundGold != source.high.RecycleGold ||
			after.Wallet.BoundGold-before.Wallet.BoundGold != source.high.RecycleGold || before.Wallet.Gold != after.Wallet.Gold || !reflect.DeepEqual(before.Items, after.Items) {
			t.Fatal("自动回收未确认为应得的绑定金币或修改了保护物品", op.Settlement.Recycling)
		}
	}
}

// quotaCheckWindow 直接确认运行服、装备阶段、品质及事件窗口中的普通怪来源用量。
func quotaCheckWindow(t *testing.T, ctx context.Context, store *persistence.Store, source quotaDropSource, at time.Time, expected int64) {
	t.Helper()
	start, end, _ := source.quota.Window(at)
	filter := bson.M{"_id": fmt.Sprintf("%d:%d:%d:%d", store.Cfg.RunID, source.quota.Stage, source.quota.Quality, start.Unix())}
	var window struct {
		Run     uint64           `bson:"run_server_id"`
		Stage   uint32           `bson:"stage"`
		Quality uint32           `bson:"quality"`
		Starts  time.Time        `bson:"starts_at"`
		Ends    time.Time        `bson:"ends_at"`
		Sources map[string]int64 `bson:"sources"`
	}
	err := store.DB.Collection("equipment_drop_quota_windows").FindOne(ctx, filter).Decode(&window)
	if expected == 0 && errors.Is(err, mongo.ErrNoDocuments) {
		return
	}
	must(t, err)
	if window.Run != store.Cfg.RunID || window.Stage != source.quota.Stage || window.Quality != source.quota.Quality ||
		!window.Starts.Equal(start) || !window.Ends.Equal(end) || window.Sources[devconfig.OrdinaryMonsterDropSource] != expected {
		t.Fatalf("额度窗口 %+v 与主键 %v、预计用量 %d 不符", window, filter, expected)
	}
}

// quotaRole 入场前只设置第19杀、测试钱包容量及原回收资格，结果全部由正常结算产生。
func quotaRole(t *testing.T, ctx context.Context, authority, owner *persistence.Store, hosted bool, pack string) (domain.Role, persistence.Auth) {
	t.Helper()
	login, err := authority.Login(ctx, "quota_"+domain.ID()[:9], "test_123456", true)
	must(t, err)
	account, err := authority.Authenticate(ctx, login.AccountToken)
	must(t, err)
	key, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: owner.Cfg.RunID, Slot: 1, Job: 0, Name: "额度" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())})
	must(t, err)
	id := domain.Hex(key.Key.RoleId)
	qualification := bson.M{"no_equipment_kills": 19}
	if hosted {
		qualification["hosting_seconds"] = int64(10)
	}
	if pack != "" {
		original, err := authority.Role(ctx, id)
		must(t, err)
		if len(original.Items) == 0 {
			t.Fatal("新角色缺少填包的原物品")
		}
		filler := original.Items[0]
		filler.Locked = true
		qualification["capacity"], qualification["items"] = 1, []domain.Item{filler}
		if pack == persistence.OrdinaryEquipmentRecycled {
			qualification["auto_recycle"] = domain.AutoRecyclePolicy{Enabled: true, QualityIDs: []uint32{2}, Revision: 1}
		}
	}
	_, err = authority.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": qualification})
	must(t, err)
	ticket, err := authority.IssueTicket(ctx, account, key.Key)
	must(t, err)
	role, auth, err := owner.Enter(ctx, ticket.Ticket)
	must(t, err)
	return role, auth
}

// TestFirstLoopM1OrdinaryEquipmentQuotaMongo 在真实事务验证竞争、时窗、运行服、阶段、资产去向和原来源重放。
func TestFirstLoopM1OrdinaryEquipmentQuotaMongo(t *testing.T) {
	cfg := m1FieldConfig(t)
	sources := quotaFixture(t, &cfg.Fixture, true)
	quotaFreezeFixture(t, &cfg.Fixture)
	cfg.Database = "minimir_dev_game_test_quota_" + domain.ID()
	cfg.GameAddress = address(t)
	cfg.RunServerIDs = []uint64{1, 2}
	for _, id := range cfg.RunServerIDs {
		entry := makeDirectoryEntry(id, id, cfg, "quota", "额度区", fmt.Sprintf("额度%d服", id), 0, int(id))
		cfg.OriginRoutes = append(cfg.OriginRoutes, entry.origin)
		cfg.RunServers = append(cfg.RunServers, entry.assignment)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	authority, err := persistence.Open(ctx, cfg)
	must(t, err)
	owner, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		defer authority.Close(cleanup)
		defer owner.Close(cleanup)
		must(t, authority.DB.Drop(cleanup))
	})
	must(t, authority.Singleton(ctx, "account"))
	grants, err := authority.ApproveGameRuns(ctx, cfg.NodeID, owner.Instance, cfg.RunServerIDs)
	must(t, err)
	runs, err := owner.GameRunStores(grants)
	must(t, err)
	first, second := runs[0], runs[1]
	base, higherStage := sources[0], sources[1]
	_, boundary, _ := base.quota.Window(time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC))
	event := boundary.Add(-time.Hour)
	type contender struct {
		before, after domain.Role
		auth          persistence.Auth
		q             persistence.Settlement
		op            persistence.Operation
		err           error
	}
	var players [2]contender
	for i := range players {
		players[i].before, players[i].auth = quotaRole(t, ctx, authority, first, i == 1, "")
		players[i].q = quotaSettlement(players[i].before, base, event, i == 1)
	}
	begin, done := make(chan struct{}), make(chan int, len(players))
	for i := range players {
		go func() {
			player := &players[i]
			<-begin
			player.after, player.op, player.err = quotaCommit(ctx, first, player.auth, player.q)
			done <- i
		}()
	}
	close(begin)
	for range players {
		<-done
	}
	winners := 0
	for _, player := range players {
		must(t, player.err)
		var audit quotaAudit
		must(t, first.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": player.op.ID}).Decode(&audit))
		original := quotaCheckAudit(t, ctx, first, player.before.ID, audit, base)
		actual := original.Settlement.OrdinaryEquipment
		want := base.low.ID
		if actual.QuotaConsumed {
			want = base.high.ID
			winners++
		}
		quotaCheckAward(t, player.before, player.after, original, base, want, persistence.OrdinaryEquipmentGranted, actual.QuotaConsumed)
		if !audit.EventAt.Equal(event) || audit.HostingSession != player.q.HostingSession || actual.Item.ID != player.q.Item.ID ||
			player.before.HostingSeconds-player.after.HostingSeconds != player.q.HostSeconds {
			t.Fatal("原击杀时间、托管扣时或降档实例身份改变", audit)
		}
		replayed, receipt, err := quotaCommit(ctx, first, player.auth, player.q)
		must(t, err)
		if !reflect.DeepEqual(receipt, original) || !reflect.DeepEqual(replayed, player.after) {
			t.Fatal("重放原事件改变了角色或其唯一永久回执", receipt)
		}
		changed := player.q
		changed.EventAt = boundary.Add(time.Second)
		if _, _, err = quotaCommit(ctx, first, player.auth, changed); persistence.Code(err) != p.CodeOperationConflict {
			t.Fatal("相同来源可借改写事件时间取得新窗口额度", err)
		}
		count, err := first.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"role_id": player.before.ID, "source_seq": player.q.SourceSeq})
		must(t, err)
		if count != 1 {
			t.Fatal("重放写入了重复审计", count)
		}
	}
	if winners != 1 {
		t.Fatalf("两名角色争用共窗份额产生了%d次稀有授予", winners)
	}
	quotaCheckWindow(t, ctx, first, base, event, 1)
	check := func(run *persistence.Store, source quotaDropSource, at time.Time, pack string, want uint32, disposition string, consumed bool) {
		t.Helper()
		before, auth := quotaRole(t, ctx, authority, run, false, pack)
		after, op, err := quotaCommit(ctx, run, auth, quotaSettlement(before, source, at, false))
		must(t, err)
		quotaCheckAward(t, before, after, op, source, want, disposition, consumed)
	}
	check(first, higherStage, event, "", higherStage.high.ID, persistence.OrdinaryEquipmentGranted, true)
	quotaCheckWindow(t, ctx, first, higherStage, event, 1)
	check(second, base, event, "", base.high.ID, persistence.OrdinaryEquipmentGranted, true)
	quotaCheckWindow(t, ctx, second, base, event, 1)
	check(first, base, boundary, "", base.low.ID, persistence.OrdinaryEquipmentGranted, false)
	quotaCheckWindow(t, ctx, first, base, boundary, 0)
	afterRelease := boundary.Add(time.Second)
	check(first, base, afterRelease, persistence.OrdinaryEquipmentDiscarded, base.high.ID, persistence.OrdinaryEquipmentDiscarded, false)
	quotaCheckWindow(t, ctx, first, base, afterRelease, 0)
	check(first, base, afterRelease, persistence.OrdinaryEquipmentRecycled, base.high.ID, persistence.OrdinaryEquipmentRecycled, true)
	check(first, base, afterRelease, "", base.low.ID, persistence.OrdinaryEquipmentGranted, false)
	quotaCheckWindow(t, ctx, first, base, afterRelease, 1)
	quotaCheckWindow(t, ctx, first, base, event, 1)
	t.Log("共窗仅一次稀有授予，原事件重放、独立服/阶段/新窗、满包弃物与实际回收均已核对 MongoDB 回执和额度")
}

// quotaWaitKill 只等待真实游戏服写入的击杀审计，避免把离线整秒扣时当成击杀。
func quotaWaitKill(t *testing.T, ctx context.Context, store *persistence.Store, id string) quotaAudit {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	for deadline.Err() == nil {
		var audit quotaAudit
		err := store.DB.Collection("audit_outbox").FindOne(deadline, bson.M{"role_id": id, "kind": "settlement", "kills": 1}).Decode(&audit)
		if err == nil {
			return audit
		}
		if !errors.Is(err, mongo.ErrNoDocuments) && deadline.Err() == nil {
			must(t, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("角色 %s 的真实击杀未确认", id)
	return quotaAudit{}
}

// TestFirstLoopM1OrdinaryEquipmentQuotaOnlineHostingMongoWS 验证真实在线手打和断线托管争夺同一额度，且第20杀降档后归零。
func TestFirstLoopM1OrdinaryEquipmentQuotaOnlineHostingMongoWS(t *testing.T) {
	cfg := m1FieldConfig(t)
	source := quotaFixture(t, &cfg.Fixture, false)[0]
	m := &cfg.Fixture.Map
	// 新房间默认开启自动攻击，令原怪在首入场时超出普攻范围。
	m.MonsterSpawns[0] = devconfig.Point{X: m.PlayerSpawn.X + 2, Y: m.PlayerSpawn.Y}
	for i := range cfg.Fixture.Monsters {
		monster := &cfg.Fixture.Monsters[i]
		if monster.ID == source.monster.ID {
			monster.HP, monster.MoveMS, monster.AttackMin, monster.AttackMax = 1, 600000, 0, 0
			monster.Agility, monster.DefenceMin, monster.DefenceMax = 0, 0, 0
		}
	}
	// 让托管角色有时间在真实命中前关断 WebSocket，命中和结算仍由正式时间线推进。
	cfg.Fixture.CombatTiming.PlayerAttackActionMS = 850
	cfg.Fixture.CombatTiming.PlayerAttackImpactMS = 700
	quotaFreezeFixture(t, &cfg.Fixture)
	cfg, ctx, store := m1FieldServices(t, cfg)
	type hunter struct {
		client   *client
		before   domain.Role
		endpoint string
		token    string
		key      *pb.RoleKey
	}
	newHunter := func() hunter {
		endpoint, token, key := m1FieldRole(t, cfg, 0)
		id := domain.Hex(key.RoleId)
		_, err := store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"no_equipment_kills": 19}})
		must(t, err)
		c := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
		t.Cleanup(c.close)
		c.auto(false, 1)
		for _, item := range c.inventory.Items {
			if item.ItemId != 218 {
				continue
			}
			result := c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: item.InstanceId, ItemVersion: item.Version, Slot: 0, Operation: c.operation()})
			if result.ErrorCode != 0 {
				t.Fatal("原木剑穿戴失败", result)
			}
			c.inventoryAt(result.NewRevision)
			standing := &pb.GridPoint{X: int32(m.PlayerSpawn.X + 1), Y: int32(m.PlayerSpawn.Y)}
			c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: standing})
			travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
				position := c.player().Position
				return frame.ID == p.MsgS2CSceneBaseline && position.X == standing.X && position.Y == standing.Y
			})
			before, err := store.Role(ctx, id)
			must(t, err)
			return hunter{client: c, before: before, endpoint: endpoint, token: token, key: key}
		}
		t.Fatal("新角色缺少原木剑")
		return hunter{}
	}
	manual, hosted := newHunter(), newHunter()
	result := hosted.client.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: hosted.client.operation()})
	if result.ErrorCode != 0 {
		t.Fatal("原离线托管余额领取失败", result)
	}
	hosted.client.inventoryAt(result.NewRevision)
	// 行走的原动作锁包含完整移动周期，两个角色仍保持自动战斗关闭。
	time.Sleep(time.Duration(hosted.client.player().MoveIntervalMs) * time.Millisecond)
	// 仅跨零点时等待数秒，令两个真实击杀落在同一个已释放的日窗。
	_, nearEnd, released := source.quota.Window(time.Now())
	if until := time.Until(nearEnd); until < 4*time.Second {
		time.Sleep(until + 1100*time.Millisecond)
	} else if released == 0 {
		time.Sleep(1100 * time.Millisecond)
	}
	hosted.client.auto(true, 1)
	hosted.client.close()
	monster := m1LiveMonster(t, manual.client, 1, func(e *pb.EntityState) bool { return e.DefinitionId == source.monster.DefinitionID })
	attack := &pb.AttackIntent{SceneEpoch: manual.client.entry.SceneEpoch, Direction: m1AdjacentDirection(manual.client.player().Position, monster.Position)}
	manual.client.send(p.MsgC2SAttackIntent, attack)
	travelUntilWithin(manual.client, 4*time.Second, func(frame wire.Frame) bool {
		corpse := manual.client.entity(monster.EntityHandle)
		return frame.ID == p.MsgS2CSceneBaseline && corpse != nil && corpse.Hp == 0 && corpse.Life == monster.Life
	})
	replay := manual.client.send(p.MsgC2SAttackIntent, attack) // 重放相同输入序号，不生成第二次击杀或第二笔收益。
	travelExpectError(manual.client, p.MsgC2SAttackIntent, replay, p.CodeStaleInput)
	manualAudit := quotaWaitKill(t, ctx, store, manual.before.ID)
	hostedAudit := quotaWaitKill(t, ctx, store, hosted.before.ID)
	manualStart, _, _ := source.quota.Window(manualAudit.EventAt)
	hostedStart, _, _ := source.quota.Window(hostedAudit.EventAt)
	if manualAudit.HostingSession != "" || hostedAudit.HostingSession == "" || manualStart != hostedStart {
		t.Fatalf("在线手动和实际离线托管未落在同一事件窗：在线=%+v 托管=%+v", manualAudit, hostedAudit)
	}
	winners := 0
	for i, player := range []hunter{manual, hosted} {
		audit := []quotaAudit{manualAudit, hostedAudit}[i]
		op := quotaCheckAudit(t, ctx, store, player.before.ID, audit, source)
		after, err := store.Role(ctx, player.before.ID)
		must(t, err)
		want := source.low.ID
		consumed := op.Settlement.OrdinaryEquipment.QuotaConsumed
		if consumed {
			want = source.high.ID
			winners++
		}
		quotaCheckAward(t, player.before, after, op, source, want, persistence.OrdinaryEquipmentGranted, consumed)
		if i == 1 && (after.Report.SessionID != audit.HostingSession || after.Report.Kills != 1) {
			t.Fatal("真实托管报告未接收原装备击杀", after.Report)
		}
		// 实际重连后，客户端资产与原永久回执应展示同一档位和实例，且不会重复产生收益。
		player.client.close()
		rejoined := join(t, cfg.GameAddress, issue(t, player.endpoint, player.token, player.key))
		t.Cleanup(rejoined.close)
		item := fieldDroppedItem(rejoined, want, op.SourceSeq)
		if item == nil || domain.Hex(item.InstanceId) != op.Settlement.OrdinaryEquipment.Item.ID {
			t.Fatal("健康重连未恢复结算实际装备实例", want, item)
		}
		count, err := store.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"role_id": player.before.ID, "kind": "settlement", "kills": 1})
		must(t, err)
		if count != 1 {
			t.Fatal("一次击杀或输入重放产生了多笔收益审计", count)
		}
	}
	if winners != 1 {
		t.Fatalf("真实在线和托管竞争产生%d次稀有授予", winners)
	}
	quotaCheckWindow(t, ctx, store, source, manualAudit.EventAt, 1)
	t.Logf("真实手打与断线托管同窗各确认一次击杀，稀有发放%d件，托管原会话=%s", winners, hostedAudit.HostingSession)
}
