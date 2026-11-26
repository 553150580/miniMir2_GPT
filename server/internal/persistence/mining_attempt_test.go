// 本文件核验原矿由事务入包后保持纯度、来源，以及挖矿、锄头和托管扣费不重不漏。
package persistence

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestMiningAssetsKeepSourceAndPurity 核对仓库移动和市场上架规则只接受原单块矿石。
func TestMiningAssetsKeepSourceAndPurity(t *testing.T) {
	definition := devconfig.Item{ID: 910001}
	ore := domain.Item{ID: domain.ID(), ItemID: definition.ID, Quantity: 1, Version: 1, OrePurity: 77, SourceKind: "mining", SourceID: domain.ID()}
	stored, remainder, ok := warehouseSplit(ore, 1, domain.ID())
	if !ok || remainder != nil || stored.OrePurity != ore.OrePurity || stored.SourceID != ore.SourceID || stored.Proto().GetOrePurity() != 77 || !CanTrade(stored, definition) {
		t.Fatalf("原矿移动或上架改变纯度及来源 %+v", stored)
	}
	for _, modify := range []func(*domain.Item){
		func(item *domain.Item) { item.Bound = true },
		func(item *domain.Item) { item.OrePurity = 0 },
		func(item *domain.Item) { item.OrePurity = 101 },
		func(item *domain.Item) { item.Quantity = 2 },
		func(item *domain.Item) { item.SourceKind = "npc_shop" },
	} {
		invalid := ore
		modify(&invalid)
		if CanTrade(invalid, definition) {
			t.Fatalf("错误矿石获准上架 %+v", invalid)
		}
	}
}

// TestMiningSettlementMongo 使用编译候选和独立副本集验证来源事务及审计。
func TestMiningSettlementMongo(t *testing.T) {
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	fixturePath := os.Getenv("MINIMIR_MINING_TEST_FIXTURE")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" || fixturePath == "" {
		t.Skip("需要专属 MongoDB 副本集和已编译的隔离挖矿候选")
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	checkCode := func(err error, code uint32) {
		t.Helper()
		if Code(err) != code {
			t.Fatalf("错误码 %d，预期 %d，原错误 %v", Code(err), code, err)
		}
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(err)
	raw, err := os.ReadFile(fixturePath)
	must(err)
	var fixture devconfig.Fixture
	must(json.Unmarshal(raw, &fixture))
	fixture.Hash = Hash(raw)
	must(fixture.Validate())
	if fixture.Mining == nil || len(fixture.Mining.Layers) == 0 {
		t.Fatal("实测夹具未装配矿层")
	}
	t.Logf("挖矿持久化实际加载候选 SHA-256=%s", fixture.Hash)
	cfg.MongoURI, cfg.Database, cfg.Fixture = uri, "minimir_dev_game_test_mining_"+domain.ID()[:12], fixture
	ctx, stop := context.WithTimeout(t.Context(), 35*time.Second)
	t.Cleanup(stop)
	account, err := Open(ctx, cfg)
	must(err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		defer account.Close(cleanup)
		must(account.DB.Drop(cleanup))
	})
	must(account.Singleton(ctx, "account"))
	game, err := Open(ctx, cfg)
	must(err)
	t.Cleanup(func() { game.Close(context.Background()) })
	lease, err := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
	must(err)
	game.Epoch = lease.Epoch

	name := "miner_" + domain.ID()[:8]
	login, err := account.Login(ctx, name, "mining_test_123456", true)
	must(err)
	auth, err := account.Authenticate(ctx, login.AccountToken)
	must(err)
	created, err := account.CreateRole(ctx, auth, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: 1, Name: name, OpId: domain.Bytes(domain.ID())})
	must(err)
	ticket, err := account.IssueTicket(ctx, auth, created.Key)
	must(err)
	role, auth, err := game.Enter(ctx, ticket.Ticket)
	must(err)
	layer := fixture.Mining.Layers[0]
	toolDef := fixture.Mining.Tools[0]
	if layer.Layer != toolDef.Layer || fixture.Mining.IntervalMS != 3000 || fixture.Mining.ToolWearU != 1 || toolDef.DurabilityU < 3 {
		t.Fatal("候选配置不符合此专项的三个间隔和工具消耗口径")
	}
	interval := 3 * time.Second
	firstAt := bossTime(time.Now().Add(-15 * time.Second))
	tool := InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: toolDef.ItemID, Quantity: 1, Version: 1}, &fixture)
	if tool.DurabilityU != toolDef.DurabilityU || !tool.HasValidDurability() {
		t.Fatalf("真实锄头定义未初始化耐久 %+v", tool)
	}
	tool.DurabilityU = 3
	roles := account.DB.Collection("roles")
	_, err = roles.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"items": []domain.Item{tool}, "capacity": 2,
		"hosting_seconds": 5, "privilege_since": firstAt.Add(-time.Minute), "privilege_until": firstAt.Add(2 * interval)}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	q := MiningAttemptRequest{EventID: miningTestEvent(t, role.ID, layer.Layer, &fixture, true), SourceSeq: role.SourceSeq + 1,
		ExpectedRevision: role.Revision, RuleVersion: fixture.RuleVersion, ConfigHash: fixture.Hash, MapID: layer.Map.ID,
		ToolID: tool.ID, ToolVersion: tool.Version, StartedAt: firstAt.Add(-interval), At: firstAt}
	_, _, err = game.ResolveMiningAttempt(ctx, auth, q)
	checkCode(err, p.CodeLineForbidden)
	_, err = roles.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"level": layer.MinimumLevel}})
	must(err)
	premature := q
	premature.StartedAt = q.StartedAt.Add(time.Millisecond)
	_, _, err = game.ResolveMiningAttempt(ctx, auth, premature)
	checkCode(err, p.CodeOperationConflict)

	// 允许并发重试同一来源，两份回包读取同一永久结果。
	type attemptOutput struct {
		role domain.Role // 事务确认后的角色。
		op   Operation   // 该次来源固定回执。
		err  error       // 事务提交状态。
	}
	results := make(chan attemptOutput, 2)
	for range 2 {
		go func() {
			next, op, err := game.ResolveMiningAttempt(ctx, auth, q)
			results <- attemptOutput{next, op, err}
		}()
	}
	first, second := <-results, <-results
	must(first.err)
	must(second.err)
	if first.op.Mining == nil || first.op.Mining.Ore == nil || first.op.Mining.Ore.Bound || first.op.Mining.Ore.OrePurity < 1 ||
		first.op.Mining.Ore.OrePurity > 100 || first.op.Mining.DurabilityAfterU != 2 || !reflect.DeepEqual(first.op, second.op) ||
		first.role.HostingSeconds != 5 || first.role.SourceSeq != q.SourceSeq || len(first.role.Items) != 2 || first.role.Wallet != role.Wallet {
		t.Fatalf("并发出矿未返回原事件或特权判绑不正确 %+v / %+v", first.op, second.op)
	}
	conflicting := q
	conflicting.ToolVersion++
	_, _, err = game.ResolveMiningAttempt(ctx, auth, conflicting)
	checkCode(err, p.CodeOperationConflict)

	q2 := q
	q2.EventID, q2.SourceSeq, q2.ExpectedRevision, q2.ToolVersion = miningTestEvent(t, role.ID, layer.Layer, &fixture, false), q.SourceSeq+1, first.role.Revision, 2
	q2.StartedAt, q2.At, q2.HostingSession, q2.HostSeconds = q.At, q.At.Add(interval), domain.ID(), 3
	_, _, err = game.ResolveMiningAttempt(ctx, auth, q2)
	checkCode(err, p.CodeEquipmentBagFull)
	full, err := game.Role(ctx, role.ID)
	must(err)
	if full.SourceSeq != first.role.SourceSeq || full.HostingSeconds != 5 || !reflect.DeepEqual(full.Items, first.role.Items) {
		t.Fatalf("满包判定已扣源序、锄头或托管时间 %+v", full)
	}
	_, err = roles.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"capacity": 4}})
	must(err)
	failed, noOre, err := game.ResolveMiningAttempt(ctx, auth, q2)
	must(err)
	if noOre.Mining == nil || noOre.Mining.Ore != nil || noOre.Mining.DurabilityAfterU != 1 || noOre.Mining.HostSeconds != 3 ||
		failed.HostingSeconds != 2 || failed.Report.UsedSeconds != 3 || failed.Report.Kills != 0 || failed.Report.TotalDropCount != 0 {
		t.Fatalf("离线未出矿消费与战报不一致 %+v / %+v", noOre.Mining, failed.Report)
	}
	q3 := q2
	q3.EventID, q3.SourceSeq, q3.ExpectedRevision, q3.ToolVersion = miningTestEvent(t, role.ID, layer.Layer, &fixture, true), q2.SourceSeq+1, failed.Revision, 3
	q3.StartedAt, q3.At = q2.At, q2.At.Add(interval)
	_, _, err = game.ResolveMiningAttempt(ctx, auth, q3)
	checkCode(err, p.CodeOperationConflict)
	q3.HostingSession, q3.HostSeconds = "", 0
	last, exhaustion, err := game.ResolveMiningAttempt(ctx, auth, q3)
	must(err)
	if exhaustion.Mining == nil || !exhaustion.Mining.ToolExhausted || exhaustion.Mining.Ore == nil || !exhaustion.Mining.Ore.Bound ||
		last.HostingSeconds != 2 || len(last.Items) != 2 || last.Items[0].ID != first.op.Mining.Ore.ID ||
		last.Items[1].ID != exhaustion.Mining.Ore.ID || last.Items[0].ID == last.Items[1].ID || last.LastMiningAt != q3.At || last.Wallet != role.Wallet {
		t.Fatalf("耗尽、同包独立矿石或特权到期判绑不正确 %+v", exhaustion.Mining)
	}
	for _, collection := range []string{"operations", "audit_outbox"} {
		count, err := account.DB.Collection(collection).CountDocuments(ctx, bson.M{"kind": miningAttemptKind, "role_id": role.ID})
		must(err)
		if count != 3 {
			t.Fatalf("%s 挖矿账本 %d 条，预期只记录三次成功提交的判定", collection, count)
		}
	}
}

// miningTestEvent 只在验收中挑选一个确实会出矿或判定失败的随机原事件。
func miningTestEvent(t *testing.T, roleID string, layer uint32, fixture *devconfig.Fixture, wantOre bool) string {
	t.Helper()
	for range 128 {
		id := domain.ID()
		ore, err := sampleMiningOre(roleID, id, layer, fixture)
		if err != nil {
			t.Fatal(err)
		}
		if (ore != nil) == wantOre {
			return id
		}
	}
	t.Fatal("未能生成预期隔离挖矿事件")
	return ""
}
