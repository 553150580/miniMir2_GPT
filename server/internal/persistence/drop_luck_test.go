// 本文件核对初始幸运只装配到实际产出、同源重放和已持久回执。
package persistence

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// TestDropInitialLuckFollowsSettlementDisposition 验证背包、战报和自动回收观察同一原装备。
func TestDropInitialLuckFollowsSettlementDisposition(t *testing.T) {
	fixture := recycleFixture(t)
	fixture.CombatLuck = &devconfig.CombatLuckConfig{DropPPM: dropLuckPPM, InitialWeights: []uint32{1, 1, 1}}
	role := recycleRole()
	item := domain.Item{ID: "a1b2c3d401234567abcdef0123456789", ItemID: 218, Quantity: 1, Version: 1,
		Bound: true, SourceKind: "monster_drop", SourceID: "5", DurabilityU: 4000, MaxDurabilityU: 4000, OriginalDurabilityU: 4000}
	q := Settlement{SourceSeq: 5, Kills: 1, Bound: true, Item: &item, HostingSession: "drop-luck-test", HostSeconds: 1}
	for _, tc := range []struct {
		name                     string
		change                   func(*domain.Role, *domain.Item, *devconfig.Fixture)
		lucky, granted, recycled bool
	}{
		{"实际入包", func(r *domain.Role, _ *domain.Item, _ *devconfig.Fixture) { r.AutoRecycle.Enabled = false }, true, true, false},
		{"原掉落立即回收", func(_ *domain.Role, _ *domain.Item, _ *devconfig.Fixture) {}, true, false, true},
		{"满包丢弃", func(r *domain.Role, _ *domain.Item, _ *devconfig.Fixture) {
			r.AutoRecycle.Enabled = false
			r.Capacity = 0
		}, false, false, false},
		{"非装备掉落", func(_ *domain.Role, i *domain.Item, _ *devconfig.Fixture) { i.ItemID = 35 }, false, true, false},
		{"原来源不匹配", func(r *domain.Role, i *domain.Item, _ *devconfig.Fixture) {
			r.AutoRecycle.Enabled = false
			i.SourceID = "6"
		}, false, true, false},
		{"历史规则未启用", func(r *domain.Role, _ *domain.Item, f *devconfig.Fixture) {
			r.AutoRecycle.Enabled = false
			f.CombatLuck = nil
		}, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current, original, rules := role, item, fixture
			tc.change(&current, &original, &rules)
			input := q
			input.Item = &original
			next, receipt, err := applySettlement(current, input, rules)
			if err != nil || receipt.GrantedItem != tc.granted || (receipt.LuckyDrop != nil) != tc.lucky ||
				(len(receipt.Recycling.Recycled) != 0) != tc.recycled || original.InitialLuck != 0 {
				t.Fatalf("初始幸运改变了原去向、回执或输入：%+v %v", receipt, err)
			}
			if !tc.lucky {
				return
			}
			result := *receipt.LuckyDrop
			if result.InitialLuck < 1 || result.InitialLuck > 3 {
				t.Fatal("超出合法初始幸运档位", result)
			}
			withoutLuck := result
			withoutLuck.InitialLuck = 0
			if withoutLuck != original {
				t.Fatal("幸运抽样修改了原品质、耐久或绑定", original, result)
			}
			if tc.granted && (len(next.Items) != 1 || next.Items[0] != result || len(next.Report.Drops) != 1 || next.Report.Drops[0] != result) {
				t.Fatal("背包或离线报告未保存该原幸运装备", next)
			}
			if tc.recycled && (len(next.Items) != 0 || len(next.Report.Drops) != 0 || receipt.Recycling.Recycled[0] != result) {
				t.Fatal("已确认回收未保留原幸运装备身份", receipt)
			}
		})
	}
}

// TestDropInitialLuckDoesNotDependOnQuotaTier 验证同一已冻结掉落在额度降档时不重抽。
func TestDropInitialLuckDoesNotDependOnQuotaTier(t *testing.T) {
	fixture, q := testOrdinaryEquipmentPlan()
	fixture.CombatLuck = &devconfig.CombatLuckConfig{DropPPM: dropLuckPPM, InitialWeights: []uint32{1, 1, 1}}
	primary := settleDropInitialLuck("isolated-role", q, &fixture, &SettlementReceipt{})
	for _, alternative := range q.OrdinaryEquipment.Fallbacks {
		q.Item = &alternative
		fallback := settleDropInitialLuck("isolated-role", q, &fixture, &SettlementReceipt{})
		if primary.InitialLuck == 0 || fallback.InitialLuck != primary.InitialLuck || fallback.ItemID != alternative.ItemID || fallback.Quality != alternative.Quality {
			t.Fatal("同来源额度降档重抽幸运或替换了最终装备", primary, fallback)
		}
	}
}

// TestLuckDropSettlementMongo 使用实际投影的一百分之一触发率挑选验收种子，对拍原回执、报告、审计和重放。
func TestLuckDropSettlementMongo(t *testing.T) {
	ctx, account, game, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_LUCK_TEST_FIXTURE")
	fixture := &game.Cfg.Fixture
	if fixture.CombatLuck == nil || fixture.CombatLuck.DropPPM != 10_000 || len(fixture.CombatLuck.InitialWeights) != 3 {
		t.Fatal("实际隔离配置没有投影掉落幸运候选")
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	weapon, ok := fixture.Item(218)
	if !ok || !weapon.IsEquipment || weapon.DropQuality > 1 || weapon.OriginalDurabilityU <= 0 || len(fixture.Map.MonsterSpawnIDs) == 0 {
		t.Fatal("验收须包含已配置的原普通掉落装备与怪物")
	}
	monster, ok := fixture.MonsterByID(fixture.Map.MonsterSpawnIDs[0])
	if !ok {
		t.Fatal("验收须包含主地图的原怪物")
	}
	// 只准备离线时长和空背包。物件必须经实际结算事务出现。
	_, err := account.DB.Collection("roles").UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"items": []domain.Item{}, "hosting_seconds": 3}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	// 使用原角色与原配置为这笔 1% 的隔离事实挑一个确定命中种子。生产抽样未被调高。
	item := domain.Item{ItemID: weapon.ID, Quantity: 1, Version: 1, Quality: weapon.DropQuality, Bound: true,
		SourceKind: ordinaryEquipmentItemSource, SourceID: fmt.Sprint(role.SourceSeq + 1)}
	q := Settlement{SourceSeq: role.SourceSeq + 1, Kills: 1, MonsterID: monster.ID, MapID: fixture.Map.ID, DropPoolID: monster.DropPoolID,
		Bound: true, Item: &item, HostingSession: "luck-isolated-hosting", HostSeconds: 1, EventAt: time.Now().UTC().Truncate(time.Millisecond)}
	var expected uint32
	for index := 1; index <= 20_000; index++ {
		item.ID = fmt.Sprintf("%032x", index)
		if expected = settleDropInitialLuck(role.ID, q, fixture, &SettlementReceipt{}).InitialLuck; expected != 0 {
			break
		}
	}
	if expected == 0 {
		t.Fatal("未能为实际配置定位命中初始幸运的隔离种子")
	}
	granted, op, err := game.SettleWithReceipt(ctx, auth, q)
	must(err)
	if op.Settlement == nil || op.Settlement.LuckyDrop == nil || !op.Settlement.GrantedItem || len(granted.Items) != 1 ||
		len(granted.Report.Drops) != 1 || granted.Report.UsedSeconds != 1 || granted.HostingSeconds != 2 {
		t.Fatal("已确认物品、托管时长或永久回执缺少原结算", granted, op)
	}
	want := InitializeItemDurability(item, fixture)
	want.InitialLuck = expected
	if *op.Settlement.LuckyDrop != want || granted.Items[0] != want || granted.Report.Drops[0] != want ||
		granted.Inventory().Items[0].InitialLuck != expected || item.InitialLuck != 0 {
		t.Fatal("实例、回执、报表或公开属性未保存同一初始幸运", want, granted.Items, op.Settlement.LuckyDrop)
	}
	var audit struct {
		Item       domain.Item       `bson:"item"`
		Settlement SettlementReceipt `bson:"settlement"`
	}
	must(account.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": op.ID}).Decode(&audit))
	if audit.Item != want || !reflect.DeepEqual(audit.Settlement, *op.Settlement) {
		t.Fatal("已保存审计和永久回执不一致", audit)
	}
	current, replay, err := game.SettleWithReceipt(ctx, auth, q)
	must(err)
	if !reflect.DeepEqual(current, granted) || !reflect.DeepEqual(replay.Settlement, op.Settlement) || replay.SourceSeq != op.SourceSeq || replay.Digest != op.Digest {
		t.Fatal("原来源重放再次抽样或改变资产", replay, current)
	}
	for _, collection := range []string{"operations", "audit_outbox"} {
		count, err := account.DB.Collection(collection).CountDocuments(ctx, bson.M{"_id": op.ID})
		must(err)
		if count != 1 {
			t.Fatalf("%s 同一原事实出现 %d 次", collection, count)
		}
	}
	t.Logf("实际配置 1%% 的指定验收种子得到幸运 +%d，原实例、耐久、报告与 audit 同笔确认。", expected)
}
