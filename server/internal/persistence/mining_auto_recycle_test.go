// 本文件核对自动回收的原矿去向、满溢保护和实际 MongoDB 来源回执。
package persistence

import (
	"math"
	"reflect"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// TestMiningAutoRecycleKeepsIneligibleOre 保障无价、未开放、关闭及余额溢出时保留本次矿石。
func TestMiningAutoRecycleKeepsIneligibleOre(t *testing.T) {
	fixture := devconfig.Fixture{
		Mining: &devconfig.MiningConfig{ToolWearU: 1,
			AutoRecycle: &devconfig.MiningAutoRecycleConfig{ShopKey: "test", ItemIDs: []uint32{1}}},
		NPCShops: []devconfig.NPCShop{{Key: "test", MineralBuyback: []devconfig.NPCMineralBuyback{
			{ItemID: 1, Numerator: 1, Denominator: 2, Rounding: "floor_per_ore"},
			{ItemID: 2, Numerator: 4, Denominator: 1, Rounding: "exact"},
		}}},
		Items: []devconfig.Item{{ID: 1}, {ID: 2}},
	}
	tool := domain.Item{ID: domain.ID(), ItemID: 3, Quantity: 1, Version: 7, DurabilityU: 4}
	ore := domain.Item{ID: domain.ID(), ItemID: 1, Quantity: 1, Version: 1, OrePurity: 13, SourceKind: "mining", SourceID: domain.ID(), Bound: true}
	session := domain.ID()
	role := domain.Role{Items: []domain.Item{tool}, Capacity: 3, Revision: 8, HostingSeconds: 5, MiningAutoRecycle: true,
		Wallet: domain.Wallet{Gold: 9, BoundGold: 11}, Report: domain.Report{SessionID: session}}
	now := time.Now()
	request := MiningAttemptRequest{EventID: ore.SourceID, SourceSeq: role.SourceSeq + 1, At: now,
		StartedAt: now.Add(-3 * time.Second), HostingSession: session, HostSeconds: 3}
	for _, row := range []struct {
		name string
		edit func(*domain.Role, *domain.Item)
	}{
		{"当前角色关闭", func(r *domain.Role, _ *domain.Item) { r.MiningAutoRecycle = false }},
		{"原矿纯度一且回收价为零", func(_ *domain.Role, ore *domain.Item) { ore.OrePurity = 1 }},
		{"未开放自动处理的黑铁", func(_ *domain.Role, ore *domain.Item) { ore.ItemID = 2 }},
		{"对应钱包已满", func(r *domain.Role, _ *domain.Item) { r.Wallet.BoundGold = math.MaxInt64 }},
		{"当次托管已累计到收益上限", func(r *domain.Role, _ *domain.Item) { r.Report.Earned.BoundGold = math.MaxInt64 }},
		{"当次托管已累计到计数上限", func(r *domain.Role, _ *domain.Item) { r.Report.MiningRecycleCount = math.MaxInt64 }},
	} {
		t.Run(row.name, func(t *testing.T) {
			original, produced := role, ore
			row.edit(&original, &produced)
			next, receipt, err := applyMiningAttempt(original, request, 1, 3*time.Second, tool, &fixture, &produced)
			if err != nil || receipt.AutoRecycle != nil || receipt.Ore == nil || *receipt.Ore != produced ||
				!slices.Contains(next.Items, produced) || next.Wallet != original.Wallet || next.HostingSeconds != 2 ||
				next.Report.UsedSeconds != 3 || next.Report.TotalDropCount != 1 || !slices.Contains(next.Report.Drops, produced) ||
				next.Report.Earned != original.Report.Earned || next.Report.MiningRecycleCount != original.Report.MiningRecycleCount {
				t.Fatalf("不能成功回收时未保全原矿或改变余额 %+v / %+v / %v", next, receipt, err)
			}
		})
	}
}

// TestMiningAutoRecycleSettlementMongo 验证特权到期前后的两轮回收、托管扣费与原事件重放。
func TestMiningAutoRecycleSettlementMongo(t *testing.T) {
	ctx, account, game, role, auth := startMiningStoreTest(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	fixture := &game.Cfg.Fixture
	layer, toolDef := fixture.Mining.Layers[0], fixture.Mining.Tools[0]
	var copperID uint32
	for _, ore := range fixture.Mining.Ores {
		if ore.Key == "copper" {
			copperID = ore.ItemID
		}
	}
	price, ok := fixture.MiningAutoRecyclePrice(copperID)
	if !ok || price.Numerator != 1 || price.Denominator != 2 || price.Rounding != "floor_per_ore" ||
		fixture.Mining.ToolWearU != 1 || fixture.Mining.IntervalMS != 3000 || toolDef.DurabilityU < 2 {
		t.Fatal("隔离夹具缺少两轮工具和逐块取整的铜矿自动回收")
	}
	tool := InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: toolDef.ItemID, Quantity: 1, Version: 1}, fixture)
	tool.DurabilityU = 2
	existing := domain.Item{ID: domain.ID(), ItemID: copperID, Quantity: 1, Version: 1, OrePurity: 99,
		SourceKind: "mining", SourceID: domain.ID()}
	interval := 3 * time.Second
	firstAt := bossTime(time.Now().Add(-12 * time.Second))
	roles := account.DB.Collection("roles")
	_, err := roles.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{
		"items": []domain.Item{tool, existing}, "capacity": 3, "level": layer.MinimumLevel,
		"hosting_seconds": 7, "mining_auto_recycle": true,
		"privilege_since": firstAt.Add(-time.Minute), "privilege_until": firstAt.Add(interval),
		"hosting_card_since": firstAt.Add(-time.Minute), "hosting_card_until": firstAt.Add(time.Minute),
	}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	hostSession := domain.ID()
	wantWallet := role.Wallet
	var requests []MiningAttemptRequest
	var operations []Operation
	for round := range 2 {
		at := firstAt.Add(time.Duration(round) * interval)
		request := MiningAttemptRequest{
			EventID: miningAutoRecycleEvent(t, role.ID, layer.Layer, fixture, copperID), SourceSeq: role.SourceSeq + 1,
			ExpectedRevision: role.Revision, RuleVersion: fixture.RuleVersion, ConfigHash: fixture.Hash, MapID: layer.Map.ID,
			ToolID: tool.ID, ToolVersion: tool.Version + int64(round), StartedAt: at.Add(-interval), At: at,
			HostingSession: hostSession, HostSeconds: 3,
		}
		bound := round == 1
		const copperGold = 6 // 纯度 13 时逐块按一半取整。
		wantIncome := domain.Wallet{Gold: copperGold}
		if bound {
			wantIncome = domain.Wallet{BoundGold: copperGold}
		}
		wantWallet.Gold += wantIncome.Gold
		wantWallet.BoundGold += wantIncome.BoundGold
		next, op, err := game.ResolveMiningAttempt(ctx, auth, request)
		must(err)
		result := op.Mining
		if result == nil || result.Ore == nil || result.Ore.SourceID != request.EventID || result.Ore.ItemID != copperID || result.Ore.OrePurity != 13 ||
			result.Ore.Bound != bound || result.AutoRecycle == nil || *result.AutoRecycle != wantIncome || result.DurabilityAfterU != 1-int64(round) ||
			result.ToolExhausted != bound || result.EventID != request.EventID || result.HostSeconds != 3 ||
			next.SourceSeq != request.SourceSeq || next.HostingSeconds != 4-3*int64(round) || next.Wallet != wantWallet ||
			len(next.Items) != 2-round || !slices.Contains(next.Items, existing) ||
			slices.ContainsFunc(next.Items, func(item domain.Item) bool { return item.ID == result.Ore.ID }) {
			t.Fatalf("原矿、有效权益绑定、回收金额或扣时在第 %d 轮不一致 %+v / %+v", round, next, result)
		}
		role = next
		requests = append(requests, request)
		operations = append(operations, op)
	}
	report := role.Report
	if report.SessionID != hostSession || report.Sequence != 2 || report.UsedSeconds != 6 || report.StopReason != 1 ||
		report.Kills != 0 || report.TotalDropCount != 0 || len(report.Drops) != 0 || report.RecycleCount != 0 ||
		report.MiningRecycleCount != 2 || report.MiningRecycleGold != 6 || report.MiningRecycleBoundGold != 6 ||
		report.Earned != (domain.Wallet{Gold: 6, BoundGold: 6}) {
		t.Fatalf("原托管战报未区分矿石回收、装备回收和实际入包 %+v", report)
	}
	// 关闭当前角色的设置后仍可用原事件查回原成交，避免按新配置或当前权益重新判定。
	_, err = roles.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"mining_auto_recycle": false}})
	must(err)
	for i, request := range requests {
		replayed, original, err := game.ResolveMiningAttempt(ctx, auth, request)
		must(err)
		if !reflect.DeepEqual(original, operations[i]) || replayed.Wallet != role.Wallet || replayed.SourceSeq != role.SourceSeq ||
			replayed.HostingSeconds != role.HostingSeconds || replayed.Report.MiningRecycleCount != 2 {
			t.Fatalf("重放改变回执或重复回收 %+v / %+v", original.Mining, replayed)
		}
		var audit struct {
			Result *MiningAttemptReceipt `bson:"mining_attempt"`
		}
		must(account.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": original.ID}).Decode(&audit))
		if !reflect.DeepEqual(audit.Result, original.Mining) {
			t.Fatalf("同事务审计丢失原矿或回收钱包 %+v / %+v", audit.Result, original.Mining)
		}
	}
	for _, name := range []string{"operations", "audit_outbox"} {
		count, err := account.DB.Collection(name).CountDocuments(ctx, bson.M{"role_id": role.ID, "kind": miningAttemptKind})
		must(err)
		if count != 2 {
			t.Fatalf("%s 记录了 %d 次结算，期望两条", name, count)
		}
	}
}

// miningAutoRecycleEvent 有界选出可归因的一块奇数纯度铜矿，便于精确对拍实库。
func miningAutoRecycleEvent(t *testing.T, roleID string, layer uint32, fixture *devconfig.Fixture, copperID uint32) string {
	t.Helper()
	for range 4096 {
		id := domain.ID()
		ore, err := sampleMiningOre(roleID, id, layer, fixture)
		if err != nil {
			t.Fatal(err)
		}
		if ore != nil && ore.ItemID == copperID && ore.OrePurity == 13 {
			return id
		}
	}
	t.Fatal("未抽到纯度为 13 的隔离铜矿")
	return ""
}
