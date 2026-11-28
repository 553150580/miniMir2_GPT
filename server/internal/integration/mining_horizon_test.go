//go:build m4_economy_sampling

// 本文件以相同起点的双角色实测完整墙钟窗内，矿战与纯战斗占用的时间和实际收益。
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// miningHorizon 是一对角色共享的绝对起止时刻，购买、切图和等待都占用此窗。
type miningHorizon struct {
	Start time.Time `json:"start"` // 两臂同时具备资格后开始。
	End   time.Time `json:"end"`   // 不再发起本轮新增动作的时刻。
}

// miningHorizonCycle 保存同一把锄头获准开采的实际墙钟边界。
type miningHorizonCycle struct {
	ToolID string    `json:"tool_id"` // 原实际买入并选中的实例。
	Start  time.Time `json:"start"`   // 发起当前锄头开采请求。
	Stop   time.Time `json:"stop"`    // 收到停采或主动停止。
	Reason string    `json:"reason"`  // 耗尽、背包满或到达同一截止。
}

// miningHorizonProfile 保存本臂的真实活动边界和操作来源。
type miningHorizonProfile struct {
	RoleID      string               `json:"role_id"`       // 四钱包和原事件所属的真实角色。
	Plan        string               `json:"plan"`          // 矿战或纯战斗的固定策略。
	Before      economyCut           `json:"before"`        // 原钱包、配装、药量和账本水位。
	After       economyCut           `json:"after"`         // 停止新增动作且原结算收束后的观察。
	Cycles      []miningHorizonCycle `json:"cycles"`        // 真实锄头使用窗。
	Tools       []economyTrade       `json:"tools"`         // 每把工具的原购买与确切付款。
	Stop        string               `json:"stop_reason"`   // 矿区终止原因，纯战斗没有此项。
	CombatStart time.Time            `json:"combat_start"`  // 本臂开始普通战斗的实际时点。
	CutoffLagMS int64                `json:"cutoff_lag_ms"` // 收到停止确认距原窗结束的大小。
	Combat      economyReport        `json:"combat"`        // 同用普通挂机的在线体况和用药观察。
	Ledger      miningHorizonLedger  `json:"ledger"`        // 独立库的逐笔结果和结余。
}

// miningHorizonPair 固定可比较条件，并保留两臂原始结果用于清库后复核。
type miningHorizonPair struct {
	Selection economyScenario      `json:"selection"`      // 同等级、配装、怪槽、卡状态与墙钟长度。
	Layer     uint32               `json:"layer"`          // 矿战臂进入的矿层。
	Capital   string               `json:"capital"`        // 仅一把工具本金，或足额购足窗口工具。
	Fixture   string               `json:"fixture_sha256"` // 原规则、矿层、溢出与消费定价。
	Window    miningHorizon        `json:"window"`         // 同一绝对窗口。
	Mixed     miningHorizonProfile `json:"mixed"`          // 实际矿战臂。
	Control   miningHorizonProfile `json:"control"`        // 不进入矿区的普通战斗对照。
}

// TestMiningHorizonMongoWS 只由专项启动工具跑时钟，每个本金组覆盖两层与四种卡态。
func TestMiningHorizonMongoWS(t *testing.T) {
	cfg := m1FieldConfig(t)
	file := os.Getenv("MINIMIR_MINING_TEST_FIXTURE")
	raw, err := os.ReadFile(file)
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	must(t, fixture.Validate())
	fixture.Hash = persistence.Hash(raw)
	if fixture.Mining == nil || fixture.Mining.AutoRecycle == nil || fixture.Mining.IntervalMS != 3000 ||
		fixture.Mining.SuccessPPM != 300_000 || len(fixture.Mining.Layers) != 2 || len(fixture.Mining.Tools) != 2 {
		t.Fatal("缺少已校验的原矿层、概率、工具与回收配置")
	}
	seconds, err := strconv.ParseInt(os.Getenv("MINIMIR_MINING_HORIZON_SECONDS"), 10, 64)
	must(t, err)
	capital := os.Getenv("MINIMIR_MINING_HORIZON_CAPITAL")
	directory := os.Getenv("MINIMIR_MINING_ECONOMY_REPORT_DIR")
	if seconds < 60 || seconds > 900 || capital != "one_tool" && capital != "full" || !filepath.IsAbs(directory) {
		t.Fatal("专项需要本批输出目录、有效本金和 60 至 900 秒实际墙钟")
	}
	cfg.Fixture, cfg.FixtureFile = fixture, file
	cfg, ctx, store := m1FieldServices(t, cfg)
	must(t, prepareEconomyIndexes(ctx, store))
	logins := make(chan struct{}, 2) // 两个并行本金批次共用一个测试程序或主机。
	shops := make(chan struct{}, 1)  // 同一紧凑城镇的实际访问，用原截止限制排队。
	for i, layer := range fixture.Mining.Layers {
		tool := fixture.Mining.Tools[i]
		for _, cards := range []string{"none", "privilege", "hosting", "both"} {
			name := fmt.Sprintf("layer%d-%s", layer.Layer, cards)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				scenario := economyScenario{Job: 0, Level: layer.MinimumLevel, Slots: miningBattleMonsterSlots,
					Kit: "low", Cards: cards, Repeat: 1, MapID: fixture.Map.ID, Mode: "online", Seconds: seconds}
				validateEconomyKit(t, &fixture, scenario.Job, scenario.Level, scenario.Kit)
				price := miningHorizonCapital(t, &fixture, tool, capital, seconds)
				startWallet := domain.Wallet{BoundGold: price}
				if slices.Contains(economyCardProducts[cards], uint32(7)) {
					startWallet = domain.Wallet{Gold: price}
				}
				mixed := prepareEconomyActor(t, ctx, cfg, store, scenario, startWallet, logins)
				control := prepareEconomyActor(t, ctx, cfg, store, scenario, startWallet, logins)
				mixed.requireOperation(p.MsgC2SSetMiningAutoRecycle, &pb.SetMiningAutoRecycleRequest{Enabled: true, Operation: mixed.client.operation()})
				mixedBefore, controlBefore := mixed.readRole(), control.readRole()
				if mixedBefore.Wallet != startWallet || controlBefore.Wallet != startWallet || !sameMiningHorizonEquipment(mixedBefore, controlBefore) ||
					mixedBefore.Level != controlBefore.Level || mixedBefore.Experience != controlBefore.Experience ||
					!slices.Equal(mixedBefore.Skills, controlBefore.Skills) || mixedBefore.HostingSeconds != 0 || controlBefore.HostingSeconds != 0 {
					t.Fatal("同组起始金币、配装、经验、技能或在线状态不匹配", name)
				}
				start := time.Now().Add(700 * time.Millisecond).UTC().Truncate(time.Millisecond)
				window := miningHorizon{Start: start, End: start.Add(time.Duration(seconds) * time.Second)}
				pair := miningHorizonPair{Selection: scenario, Layer: layer.Layer, Capital: capital, Fixture: fixture.Hash, Window: window}
				t.Logf("采样窗 %s 至 %s，矿战与对照的实际本金各 %d 金币", start.Format(time.RFC3339Nano), window.End.Format(time.RFC3339Nano), price)
				if t.Run("paired", func(t *testing.T) {
					t.Run("mixed", func(t *testing.T) {
						t.Parallel()
						pair.Mixed = mixed.observeMiningHorizon(t, mixedBefore, window, layer, tool, shops)
					})
					t.Run("ordinary", func(t *testing.T) {
						t.Parallel()
						pair.Control = control.observeMiningHorizon(t, controlBefore, window, layer, devconfig.MiningTool{}, shops)
					})
				}) {
					report, err := json.MarshalIndent(pair, "", "  ")
					must(t, err)
					path := filepath.Join(directory, name+".json")
					must(t, os.WriteFile(path+".partial", append(report, '\n'), 0o644))
					must(t, os.Rename(path+".partial", path))
				}
			})
		}
	}
}

// miningHorizonCapital 按原整把工具的实际售价配平两臂的期初钱包。
func miningHorizonCapital(t *testing.T, f *devconfig.Fixture, tool devconfig.MiningTool, capital string, seconds int64) int64 {
	t.Helper()
	_, _, offer := miningVendor(t, f, tool.ItemID)
	if capital == "one_tool" {
		return offer.PriceUnits
	}
	rounds := (tool.DurabilityU + f.Mining.ToolWearU - 1) / f.Mining.ToolWearU
	planned := seconds * 1000 / f.Mining.IntervalMS
	return ((planned + rounds - 1) / rounds) * offer.PriceUnits
}

// sameMiningHorizonEquipment 对照两个真实穿戴的模板、属性来源和期初耐久。
func sameMiningHorizonEquipment(first, second domain.Role) bool {
	return slices.EqualFunc(first.Equipped, second.Equipped, func(left, right domain.Equipped) bool {
		return left.Slot == right.Slot && left.Item.ItemID == right.Item.ItemID && left.Item.Bound == right.Item.Bound &&
			left.Item.DurabilityU == right.Item.DurabilityU && left.Item.MaxDurabilityU == right.Item.MaxDurabilityU
	})
}

// observeMiningHorizon 在同一实际截止停止两臂，等最后受理的流水确认后比对资产。
func (a *economyActor) observeMiningHorizon(t *testing.T, before domain.Role, window miningHorizon, layer devconfig.MiningLayer, tool devconfig.MiningTool, shops chan struct{}) miningHorizonProfile {
	t.Helper()
	a.t, a.client.t = t, t
	c := a.client
	c.stop()
	c.ctx, c.stop = context.WithDeadline(a.ctx, window.End.Add(2*time.Minute))
	ready := time.NewTimer(max(time.Until(window.Start), 0))
	defer ready.Stop()
	select {
	case <-ready.C:
	case <-c.ctx.Done():
		t.Fatal("等待配对开窗期间连接退出")
	}
	result := miningHorizonProfile{RoleID: before.ID, Plan: "mining", Before: economyState(before)}
	if tool.ItemID == 0 {
		result.Plan = "ordinary"
	} else {
		stop := sampleHeartbeat(c, window.End)
		defer stop()
		a.mineIntoHorizon(window.End, layer, tool, shops, &result)
		stop()
	}
	if time.Now().Before(window.End) {
		if c.entry.SceneKind != devconfig.SceneKindPersonal {
			returnMiningBattle(c, a.scenario.MapID)
		}
		if time.Now().Before(window.End) {
			result.CombatStart = time.Now().UTC().Truncate(time.Millisecond)
			a.observeOnline(c, window.End, &result.Combat)
		}
	}
	if c.entry.SceneKind == devconfig.SceneKindPersonal && c.player().GetHp() > 0 {
		c.auto(false, a.scenario.Slots)
	}
	result.CutoffLagMS = max(time.Since(window.End).Milliseconds(), 0)
	end := a.awaitStill()
	result.After = economyState(end)
	result.Ledger = a.reconcileMiningHorizon(before, end, window, layer, tool, result)
	return result
}

// mineIntoHorizon 仅在旧锄头耗尽后才买下一把，真实钱包不足或满包则结束采矿。
func (a *economyActor) mineIntoHorizon(end time.Time, layer devconfig.MiningLayer, tool devconfig.MiningTool, gate chan struct{}, out *miningHorizonProfile) {
	a.t.Helper()
	c := a.client
	town, npc, offer := miningVendor(a.t, &a.cfg.Fixture, tool.ItemID)
	out.Stop = "window"
	for time.Until(end) >= 10*time.Second {
		bought, reason := a.buyHorizonTool(end, town, npc, offer, gate)
		if reason != "" {
			out.Stop = reason
			return
		}
		out.Tools = append(out.Tools, bought)
		index := slices.IndexFunc(c.inventory.Items, func(item *pb.ItemView) bool { return item.ItemId == tool.ItemID })
		if index < 0 || c.inventory.Items[index].DurabilityU != uint64(tool.DurabilityU) {
			a.t.Fatal("本轮真实采购缺少原锄头实例", bought)
		}
		owned := c.inventory.Items[index]
		if time.Until(end) < time.Duration(a.cfg.Fixture.Mining.IntervalMS)*time.Millisecond {
			return
		}
		travelToBaseline(c, 3, layer.Map.ID)
		travelAcknowledge(c, false)
		if !time.Now().Before(end) {
			return
		}
		cycle := miningHorizonCycle{ToolID: domain.Hex(owned.InstanceId), Start: time.Now().UTC().Truncate(time.Millisecond), Reason: "window"}
		state := miningStateWS(c, 1, owned.InstanceId)
		if !state.Active || !state.AutoRecycleEnabled || !proto.Equal(state.Tool, owned) {
			a.t.Fatal("刚买的锄头没有通过正常动作开采", state)
		}
		for state.Active && time.Now().Before(end) {
			frame := c.read()
			if frame.ID == p.MsgS2CMiningState && frame.Request == 0 {
				state = &pb.MiningState{}
				must(a.t, proto.Unmarshal(frame.Payload, state))
			}
		}
		if state.Active {
			if stopped := miningStateWS(c, 2, nil); stopped.Active {
				a.t.Fatal("到窗未结束新开采", stopped)
			}
		} else {
			switch state.ErrorCode {
			case p.CodeEquipmentChanged:
				cycle.Reason = "exhausted"
			case p.CodeEquipmentBagFull:
				cycle.Reason, out.Stop = "bag_full", "bag_full"
			default:
				a.t.Fatal("窗口内有未建模的停采结果", state)
			}
		}
		cycle.Stop = time.Now().UTC().Truncate(time.Millisecond)
		out.Cycles = append(out.Cycles, cycle)
		if cycle.Reason != "exhausted" {
			return
		}
	}
}

// buyHorizonTool 用同一 NPC 原报价检验不足并核对成功的原回执，截止后不再开启新交易。
func (a *economyActor) buyHorizonTool(end time.Time, town *devconfig.MapTemplate, npc devconfig.TownNPC, offer devconfig.SkillShop, gate chan struct{}) (economyTrade, string) {
	a.t.Helper()
	c := a.client
	deadline := time.NewTimer(max(time.Until(end.Add(-5*time.Second)), 0))
	defer deadline.Stop()
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-deadline.C:
		return economyTrade{}, "window"
	case <-c.ctx.Done():
		a.t.Fatal("买锄排队期间连接被关闭")
	}
	reachTownVendor(c, town, npc)
	if time.Until(end) < 4*time.Second {
		return economyTrade{}, "window"
	}
	q := &pb.VendorQuoteRequest{SceneEpoch: c.entry.SceneEpoch, NpcId: npc.ID, Action: economyVendorBuy, OfferId: offer.ID, Quantity: 1}
	wallet := a.readRole().Wallet
	if wallet.Gold+wallet.BoundGold < offer.PriceUnits {
		travelExpectError(c, p.MsgC2SVendorQuote, c.send(p.MsgC2SVendorQuote, q), p.CodeVendorFunds)
		return economyTrade{}, "funds"
	}
	if c.inventory.GetCapacity() <= uint32(len(c.inventory.GetItems())) {
		return economyTrade{}, "bag_full"
	}
	purchase := a.economyTrade(npc.ID, q)
	if purchase.Item != offer.ItemID || purchase.Quantity != 1 || purchase.Spend.Gold+purchase.Spend.BoundGold != offer.PriceUnits {
		a.t.Fatal("原锄头费用没有实际成交或币种不符", purchase)
	}
	return purchase, ""
}
