//go:build m4_economy_sampling

// 本文件用原 MongoDB 回执核对完整矿战窗，工具费用、矿收、战斗收益与期末钱包分别入账。
package integration

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// miningHorizonLedger 保存这个角色的墙钟及资产两套原始流水和汇总。
type miningHorizonLedger struct {
	Battle           economyYield          `json:"battle"`            // 原窗内的实际战斗收益、掉落及装备磨损。
	BoundaryBattle   economyYield          `json:"boundary_battle"`   // 迟近截止但已确认的普通结果。
	BattleRows       []economySource       `json:"battle_rows"`       // 含安全区与用药在内的原普通业务回执。
	Attempts         []miningBattleAttempt `json:"attempts"`          // 全部实际判定，含未出矿轮次。
	WindowAttempts   int                   `json:"window_attempts"`   // 原窗内实际完成的次数。
	BoundaryAttempts int                   `json:"boundary_attempts"` // 截止前已开始，截止后在正常停采前确认。
	MiningMS         int64                 `json:"mining_ms"`         // 完成的尝试在原窗内实际占用的墙钟。
	Ore              map[string]int64      `json:"ore"`               // 按原物品号和绑定区分入窗成功出矿数。
	Retained         []domain.Item         `json:"retained"`          // 实际入包的矿石，原纯度可复查。
	AutoGold         domain.Wallet         `json:"auto_gold"`         // 原窗内矿石自动回收。
	BoundaryGold     domain.Wallet         `json:"boundary_gold"`     // 与边界原判定一致的已确认回收。
	ToolSpend        domain.Wallet         `json:"tool_spend"`        // 原确切成交的整把工具开支。
	Exhausted        int                   `json:"exhausted"`         // 实际耗尽并移除的原工具数。
	RemainingTools   []domain.Item         `json:"remaining_tools"`   // 截止后仍可使用的实际库存耐久。
	PotionUse        map[uint32]int64      `json:"potion_use"`        // 本窗及边界真实红蓝药的消耗量。
	AssetNet         domain.Wallet         `json:"asset_net"`         // 四钱包到最后已确认落账的增量。
	WindowNet        domain.Wallet         `json:"window_net"`        // 原事件发生在准确墙钟内的实现增量。
}

// horizonToolState 保存某次实际交付给原角色的工具和已审计磨损水位。
type horizonToolState struct {
	item domain.Item // 当前剩余工具的预期持久状态。
	last time.Time   // 最近一轮实际出矿事件时刻。
}

// reconcileMiningHorizon 同时核对角色水位、逐轮时钟、原资产与审计，拒绝未解释的账。
func (a *economyActor) reconcileMiningHorizon(before, end domain.Role, window miningHorizon, layer devconfig.MiningLayer, definition devconfig.MiningTool, profile miningHorizonProfile) miningHorizonLedger {
	t := a.t
	t.Helper()
	filter := bson.M{"role_id": before.ID, "revision": bson.M{"$gt": before.Revision, "$lte": end.Revision}}
	cursor, err := a.store.DB.Collection("operations").Find(a.ctx, filter, options.Find().SetSort(bson.D{{Key: "revision", Value: 1}}))
	must(t, err)
	var operations []persistence.Operation
	must(t, cursor.All(a.ctx, &operations))
	audits := a.readEconomyAudits(a.ctx, operations)
	ledger := miningHorizonLedger{Ore: map[string]int64{}, PotionUse: map[uint32]int64{}}
	tools := make(map[string]*horizonToolState, len(profile.Tools))
	cycles := make(map[string]miningHorizonCycle, len(profile.Cycles))
	for _, cycle := range profile.Cycles {
		if _, duplicate := cycles[cycle.ToolID]; duplicate || cycle.Start.Before(window.Start) || cycle.Stop.Before(cycle.Start) {
			t.Fatal("同一把工具的采矿区间被重复或倒置", cycle)
		}
		cycles[cycle.ToolID] = cycle
	}
	wallet := before.Wallet
	revision, source, command := before.Revision, before.SourceSeq, before.EconomicSeq
	var purchases int
	var lastMine time.Time
	for _, op := range operations {
		audit := audits[op.ID]
		if op.Revision <= revision || audit.RoleID != before.ID || audit.Kind != op.Kind || audit.Source != op.SourceSeq || audit.Code != op.Code {
			t.Fatal("原角色操作和审计存在重复、失序或跨角色", op.ID)
		}
		revision = op.Revision
		if op.SourceSeq > 0 {
			source++
			if op.SourceSeq != source || op.CommandSeq != command {
				t.Fatal("原来源流水或角色经济序号缺失", op.ID, source, command)
			}
		} else {
			command++
			if op.CommandSeq != command {
				t.Fatal("原经济流水缺失", op.ID, command)
			}
		}
		switch op.Kind {
		case persistence.AssetVendor:
			if purchases >= len(profile.Tools) || profile.Tools[purchases].ID != op.ID {
				t.Fatal("窗口收到额外或重复的锄头采购", op.ID)
			}
			item := a.verifyHorizonTool(op, audit, wallet, definition, profile.Tools[purchases], window)
			for _, previous := range tools {
				if previous.item.DurabilityU > 0 {
					t.Fatal("买了下一把工具时原工具仍有耐久", previous.item, item)
				}
			}
			tools[item.ID] = &horizonToolState{item: item, last: op.Created}
			purchases++
			ledger.ToolSpend = economySum(ledger.ToolSpend, op.Vendor.Plan.Wallet)
			wallet = economyDiff(wallet, op.Vendor.Plan.Wallet)
		case "mining_attempt":
			if definition.ItemID == 0 || op.Mining == nil || op.Mining.StartedAt.Before(lastMine) ||
				!profile.CombatStart.IsZero() && !op.Mining.At.Before(profile.CombatStart) {
				t.Fatal("同一角色出现重叠矿轮、纯战斗出矿或回普通图后出矿", op.ID)
			}
			m := op.Mining
			gold := a.addHorizonAttempt(before, window, layer, op, audit, tools[m.ToolID], cycles[m.ToolID], &ledger)
			wallet = economySum(wallet, gold)
			lastMine = m.At
		default:
			row := a.economyFact(before, op, audit)
			if row.Hosting != 0 || row.Monster != 0 && (profile.CombatStart.IsZero() || row.At.Before(profile.CombatStart) || op.Settlement.MapID != a.scenario.MapID) {
				t.Fatal("原角色在矿区结算普通击杀、串图或在线扣时", op.ID)
			}
			yield := &ledger.Battle
			if !row.At.Before(window.End) {
				yield = &ledger.BoundaryBattle
			}
			yield.add(row)
			ledger.BattleRows = append(ledger.BattleRows, row)
			delta := domain.Wallet{Gold: row.Recycle, BoundGold: row.RecycleB}
			if row.Bound {
				delta.BoundGold += row.Direct
			} else {
				delta.Gold += row.Direct
			}
			wallet = economySum(wallet, delta)
			if row.Potion != 0 {
				ledger.PotionUse[row.Potion]++
			}
		}
	}
	if purchases != len(profile.Tools) || definition.ItemID != 0 && purchases == 0 ||
		definition.ItemID == 0 && ledger.Battle.Kills == 0 ||
		source != end.SourceSeq || command != end.EconomicSeq || wallet != end.Wallet || before.HostingSeconds != end.HostingSeconds {
		t.Fatal("实际矿战窗口的原操作、余额或在线时长无法复算", purchases, source, command, wallet, end.Wallet)
	}
	for id, expected := range tools {
		at := slices.IndexFunc(end.Items, func(item domain.Item) bool { return item.ID == id })
		if expected.item.DurabilityU == 0 && at >= 0 || expected.item.DurabilityU > 0 && (at < 0 || end.Items[at] != expected.item) {
			t.Fatal("实际期末工具耗尽、修订或耐久与逐轮原回执不符", id)
		}
		if at >= 0 {
			ledger.RemainingTools = append(ledger.RemainingTools, end.Items[at])
		}
	}
	for _, ore := range ledger.Retained {
		if !slices.Contains(end.Items, ore) {
			t.Fatal("原未回收矿石、纯度或绑定未留在实际背包", ore)
		}
	}
	for _, id := range []uint32{economyHealthPotion, economyManaPotion} {
		prefix := fmt.Sprintf("%d:", id)
		var dropped int64
		for _, part := range []economyYield{ledger.Battle, ledger.BoundaryBattle} {
			for key, units := range part.Retained {
				if strings.HasPrefix(key, prefix) {
					dropped += units
				}
			}
		}
		if economyPotions(end)[id] != economyPotions(before)[id]+dropped-ledger.PotionUse[id] {
			t.Fatal("同配装前态的原药水消耗和期末实际库存不一致", id)
		}
	}
	ledger.AssetNet = economyDiff(end.Wallet, before.Wallet)
	ledger.WindowNet = economyDiff(economySum(economySum(ledger.Battle.Direct, ledger.Battle.Recycle), ledger.AutoGold), ledger.ToolSpend)
	return ledger
}

// verifyHorizonTool 核对实际采购付款时已有足额资金，并从永久回执读取该把锄头。
func (a *economyActor) verifyHorizonTool(op persistence.Operation, audit economyAudit, wallet domain.Wallet, tool devconfig.MiningTool, expected economyTrade, window miningHorizon) domain.Item {
	a.t.Helper()
	vendor := op.Vendor
	if op.Code != 0 || !reflect.DeepEqual(vendor, audit.Vendor) || vendor == nil || vendor.Plan.Action != economyVendorBuy ||
		vendor.Plan.Wallet != expected.Spend || len(vendor.Plan.Outputs) != 1 || op.Created.Before(window.Start) || !op.Created.Before(window.End) {
		a.t.Fatal("实际锄头采购没有原同窗回执或审计", op.ID)
	}
	price, item := vendor.Plan.Wallet, vendor.Plan.Outputs[0]
	sourceID, assetKey := strings.CutPrefix(op.ID, "asset:"+op.RoleID+":")
	if price.Gold > wallet.Gold || price.BoundGold > wallet.BoundGold || price.Yuanbao != 0 || price.BoundYuanbao != 0 ||
		item.ID == "" || item.ItemID != tool.ItemID || item.DurabilityU != tool.DurabilityU || item.SourceKind != "npc_shop" || !assetKey || item.SourceID != sourceID {
		a.t.Fatal("整把工具先扣款、原实例或来源不符", price, wallet, item)
	}
	return item
}

// addHorizonAttempt 检查真实时钟及原审计，再按原纯度和绑态核算实际回收。
func (a *economyActor) addHorizonAttempt(before domain.Role, window miningHorizon, layer devconfig.MiningLayer, op persistence.Operation, audit economyAudit, tool *horizonToolState, cycle miningHorizonCycle, ledger *miningHorizonLedger) domain.Wallet {
	a.t.Helper()
	f, m := &a.cfg.Fixture, op.Mining
	if tool == nil || cycle.ToolID != m.ToolID || op.Code != 0 || !reflect.DeepEqual(m, audit.Mining) || m.At != audit.EventAt ||
		m.MapID != layer.Map.ID || m.Layer != layer.Layer || m.RuleVersion != f.RuleVersion || m.ConfigHash != f.Hash ||
		m.ToolItemID != tool.item.ItemID || m.ToolVersionBefore != tool.item.Version || tool.item.DurabilityU <= 0 || m.DurabilityBeforeU != tool.item.DurabilityU ||
		m.StartedAt.Before(tool.last) || m.StartedAt.Before(cycle.Start) || m.At.After(cycle.Stop) || !m.StartedAt.Before(window.End) ||
		m.At.Sub(m.StartedAt) < time.Duration(f.Mining.IntervalMS)*time.Millisecond || m.HostSeconds != 0 || m.HostingSession != "" || m.RemainingHostSeconds != before.HostingSeconds {
		a.t.Fatal("同一采矿窗内的原工具、时钟、审计或在线扣时不符", op.ID, m)
	}
	tool.item.DurabilityU = max(tool.item.DurabilityU-f.Mining.ToolWearU, 0)
	tool.item.Version++
	tool.last = m.At
	if m.DurabilityAfterU != tool.item.DurabilityU || m.ToolVersionAfter != tool.item.Version || m.ToolExhausted != (tool.item.DurabilityU == 0) {
		a.t.Fatal("原每轮锄头损耗或耗尽不符", m)
	}
	if m.ToolExhausted {
		ledger.Exhausted++
	}
	inside := m.At.Before(window.End)
	if inside {
		ledger.WindowAttempts++
		ledger.MiningMS += m.At.Sub(m.StartedAt).Milliseconds()
	} else {
		ledger.BoundaryAttempts++
		ledger.MiningMS += window.End.Sub(m.StartedAt).Milliseconds()
	}
	ledger.Attempts = append(ledger.Attempts, miningBattleAttempt{ID: op.ID, SourceSeq: op.SourceSeq, Fact: *m})
	var paid domain.Wallet
	if m.Ore != nil {
		ore := *m.Ore
		if ore.OrePurity < 1 || ore.OrePurity > 100 || ore.SourceKind != "mining" || ore.SourceID != m.EventID || ore.Quantity != 1 || ore.Bound == before.PrivilegeUntil.After(m.At) {
			a.t.Fatal("出矿原时刻绑态、数量、纯度或来源不符", ore)
		}
		if inside {
			ledger.Ore[fmt.Sprintf("%d:%t", ore.ItemID, ore.Bound)]++
		}
		if price, ok := f.MiningAutoRecyclePrice(ore.ItemID); ok {
			paid = miningBattleOreGold(price, ore)
		}
		if paid == (domain.Wallet{}) {
			ledger.Retained = append(ledger.Retained, ore)
		}
	}
	if paid == (domain.Wallet{}) && m.AutoRecycle != nil || paid != (domain.Wallet{}) && (m.AutoRecycle == nil || *m.AutoRecycle != paid) {
		a.t.Fatal("原实际回收资金、币种或工具组合未对平", m.EventID, m.AutoRecycle, paid)
	}
	if inside {
		ledger.AutoGold = economySum(ledger.AutoGold, paid)
	} else {
		ledger.BoundaryGold = economySum(ledger.BoundaryGold, paid)
	}
	return paid
}
