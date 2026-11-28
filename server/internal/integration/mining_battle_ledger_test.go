// 本文件从真实角色流水与审计核对矿战各段钱包、出矿来源、时长和锄头耐久。
package integration

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// miningBattleReport 保存一个原角色的矿战轨迹与每一类实际收入和支出。
type miningBattleReport struct {
	RoleID        string                  `json:"role_id"`        // 建角后贯穿出矿、打怪、交易的永久角色键。
	Scenario      miningBattleScenario    `json:"scenario"`       // 矿层及两卡状态。
	Opening       domain.Wallet           `json:"opening"`        // 资格和时长准备完毕后开始的四钱包。
	FirstBattle   miningBattleFightLedger `json:"first_battle"`   // 以打怪筹集工具款的实际收益。
	Tool          miningBattleVendor      `json:"tool_purchase"`  // 首把锄头及实际付款。
	Mining        miningBattleMineLedger  `json:"mining"`         // 一把工具使用到耗尽的完整逐轮结果。
	NPCSales      []miningBattleVendor    `json:"npc_sales"`      // 实采矿物的实际 NPC 出售。
	SecondBattle  miningBattleFightLedger `json:"second_battle"`  // 回到原普通图的实际收益。
	Closing       domain.Wallet           `json:"closing"`        // 市场操作前的最终四钱包。
	HostingBefore int64                   `json:"hosting_before"` // 开采前累计有效余额。
	HostingAfter  int64                   `json:"hosting_after"`  // 原扣时与返场后的余额。
}

// miningBattleWindow 保存普通战斗的实际墙钟范围。
type miningBattleWindow struct {
	Start time.Time `json:"start"` // 发起该段自动攻击时刻。
	Stop  time.Time `json:"stop"`  // 客户端确认关停自动攻击时刻。
}

// miningBattleKill 保存平账所需的原战斗奖项及卡加成。
type miningBattleKill struct {
	ID         string                        `json:"id"`              // 原回执及审计共享的编号。
	SourceSeq  int64                         `json:"source_seq"`      // 原角色来源流水。
	At         time.Time                     `json:"at"`              // 原判定时刻。
	Kills      int64                         `json:"kills"`           // 已持久化普通怪击杀。
	MonsterID  uint32                        `json:"monster_id"`      // 原怪物的配置身份。
	Experience int64                         `json:"experience"`      // 实际接受的角色经验。
	Direct     domain.Wallet                 `json:"direct"`          // 原击杀直接金币。
	Recycle    domain.Wallet                 `json:"recycle"`         // 同次结算回收所得。
	Bonus      *persistence.HostingKillBonus `json:"bonus,omitempty"` // 确认卡加成时的原基础和倍率结果。
}

// miningBattleFightLedger 区分两种击杀收入，零击杀的原磨损来源一并保留。
type miningBattleFightLedger struct {
	Window     miningBattleWindow `json:"window"`     // 实际用于普通战斗的时间段。
	Kills      int64              `json:"kills"`      // 全段实际结算的击杀。
	Experience int64              `json:"experience"` // 实际接受的经验。
	Direct     domain.Wallet      `json:"direct"`     // 原直接金币及绑定分账。
	Recycle    domain.Wallet      `json:"recycle"`    // 原装备回收金币及绑定分账。
	Entries    []miningBattleKill `json:"entries"`    // 原分段的结算或磨损来源。
}

// Total 返回普通战斗实际的两种入账总和。
func (ledger miningBattleFightLedger) Total() domain.Wallet {
	return addMiningBattleWallet(ledger.Direct, ledger.Recycle)
}

// miningBattleAttempt 关联原矿石、工具及实际扣时与永久流水身份。
type miningBattleAttempt struct {
	ID        string                           `json:"id"`         // 可与主审计逐笔关联的原操作键。
	SourceSeq int64                            `json:"source_seq"` // 角色已确认的原来源序号。
	Fact      persistence.MiningAttemptReceipt `json:"fact"`       // 实际本轮判定及所有资产变化。
}

// miningBattleMineLedger 对一次完整工具寿命结算。
type miningBattleMineLedger struct {
	Attempts    []miningBattleAttempt `json:"attempts"`     // 含未产矿的真实三秒判定。
	Successes   int                   `json:"successes"`    // 实际成功出矿数。
	HostSeconds int64                 `json:"host_seconds"` // 仅离线待满整轮时扣费。
	AutoGold    domain.Wallet         `json:"auto_gold"`    // 自动回收实际到账。
	Retained    []domain.Item         `json:"retained"`     // 本轮实进背包并维持纯度和绑定的矿。
}

// miningBattleVendor 保存照实报价的买卖回执，买锄为支出，出售为所得。
type miningBattleVendor struct {
	OperationID string        `json:"operation_id"`    // 同一笔经济命令永久键。
	ItemID      uint32        `json:"item_id"`         // 真实买卖物品模板。
	Wallet      domain.Wallet `json:"wallet"`          // 各钱包实际发生额。
	Items       []domain.Item `json:"items,omitempty"` // 原锄头实例及出厂耐久。
	Sold        *domain.Item  `json:"sold,omitempty"`  // 实际卖出的单块矿石。
}

// sourceOperations 从同一个角色的两次资产切点读取完整的原来源序列。
func (player miningBattlePlayer) sourceOperations(t *testing.T, before, after domain.Role) []persistence.Operation {
	t.Helper()
	trial := player.trial
	query := bson.M{"role_id": before.ID, "source_seq": bson.M{"$gt": before.SourceSeq, "$lte": after.SourceSeq}}
	rows, err := trial.store.DB.Collection("operations").Find(trial.ctx, query, options.Find().SetSort(bson.D{{Key: "source_seq", Value: 1}}))
	must(t, err)
	var out []persistence.Operation
	must(t, rows.All(trial.ctx, &out))
	if int64(len(out)) != after.SourceSeq-before.SourceSeq {
		t.Fatal("原来源流水有缺漏或重号", before.SourceSeq, after.SourceSeq, len(out))
	}
	for i, op := range out {
		if op.SourceSeq != before.SourceSeq+int64(i)+1 || op.Code != 0 || op.CommandSeq != before.EconomicSeq {
			t.Fatal("角色原来源流水不连续或与经济水位混杂", op)
		}
	}
	return out
}

// battleLedger 从击杀和原审计确认两种金币、经验、加成和零托管计费。
func (player miningBattlePlayer) battleLedger(t *testing.T, before, after domain.Role, scenario miningBattleScenario, window miningBattleWindow) miningBattleFightLedger {
	t.Helper()
	trial := player.trial
	f := &trial.store.Cfg.Fixture
	ledger := miningBattleFightLedger{Window: window}
	for _, op := range player.sourceOperations(t, before, after) {
		s := op.Settlement
		if op.Kind != "settlement" || s == nil || s.RuleVersion != f.RuleVersion || s.ConfigHash != f.Hash || s.HostSeconds != 0 ||
			s.Kills > 0 && (s.MapID != f.Map.ID || s.MonsterID == 0 || (s.HostingKillBonus != nil) != scenario.Hosting) {
			t.Fatal("普通战斗段混入矿石、其他地图、离线扣时或错误托管卡加成", op)
		}
		var audit struct {
			SourceSeq int64     `bson:"source_seq"` // 原审计引用的来源号。
			At        time.Time `bson:"event_at"`   // 原击杀或磨损事件发生时间。
			Bound     bool      `bson:"bound"`      // 发生击杀时该直接金币的绑定事实。
			Gold      int64     `bson:"gold"`       // 已提交的直接金币整数。
		}
		must(t, trial.store.DB.Collection("audit_outbox").FindOne(trial.ctx, bson.M{"_id": op.ID, "kind": op.Kind}).Decode(&audit))
		if audit.SourceSeq != op.SourceSeq || s.Kills > 0 && audit.Bound == scenario.Privilege {
			t.Fatal("普通击杀与原审计流水或特权绑定不符", op.ID, audit)
		}
		if bonus := s.HostingKillBonus; bonus != nil && (!bonus.CardUntil.After(audit.At) ||
			bonus.BaseDirectGold+bonus.BonusDirectGold != audit.Gold ||
			bonus.BonusDirectGold != bonus.BaseDirectGold*int64(f.HostingBenefits.DirectGoldBonusBP)/10_000 ||
			bonus.BonusExperience != bonus.BaseExperience*int64(f.HostingBenefits.KillExperienceBonusBP)/10_000) {
			t.Fatal("托管卡实付加成超出直接金币或经验规则", op.ID, bonus)
		}
		direct := domain.Wallet{Gold: audit.Gold}
		if audit.Bound {
			direct = domain.Wallet{BoundGold: audit.Gold}
		}
		var xp int64
		if s.Experience != nil {
			xp = s.Experience.ActualGranted
		}
		recycle := domain.Wallet{Gold: s.Recycling.Gold, BoundGold: s.Recycling.BoundGold}
		ledger.Kills, ledger.Experience = ledger.Kills+s.Kills, ledger.Experience+xp
		ledger.Direct, ledger.Recycle = addMiningBattleWallet(ledger.Direct, direct), addMiningBattleWallet(ledger.Recycle, recycle)
		ledger.Entries = append(ledger.Entries, miningBattleKill{ID: op.ID, SourceSeq: op.SourceSeq, At: audit.At, Kills: s.Kills,
			MonsterID: s.MonsterID, Experience: xp, Direct: direct, Recycle: recycle, Bonus: s.HostingKillBonus})
	}
	if ledger.Total() != minusMiningBattleWallet(after.Wallet, before.Wallet) || before.HostingSeconds != after.HostingSeconds ||
		before.LastMiningAt != after.LastMiningAt || before.EconomicSeq != after.EconomicSeq {
		t.Fatal("普通战斗的钱包增量、矿状态或时长无法复算", ledger)
	}
	return ledger
}

// mineLedger 验证每一次实际开采只能推进矿石、水位、锄头和对应币种。
func (player miningBattlePlayer) mineLedger(t *testing.T, before, after domain.Role, scenario miningBattleScenario, layer devconfig.MiningLayer, tool domain.Item) miningBattleMineLedger {
	t.Helper()
	trial := player.trial
	fixture := &trial.store.Cfg.Fixture
	rules := fixture.Mining
	operations := player.sourceOperations(t, before, after)
	var ledger miningBattleMineLedger
	remaining, hostedBalance := tool.DurabilityU, before.HostingSeconds
	var last time.Time
	var hostSession string
	var hostedCount, hostedRetained, hostedRecycled int64
	var hostedGold domain.Wallet
	for index, op := range operations {
		m := op.Mining
		if op.Kind != "mining_attempt" || m == nil || m.Layer != layer.Layer || m.MapID != layer.Map.ID || m.ToolID != tool.ID || m.ToolItemID != tool.ItemID ||
			m.RuleVersion != fixture.RuleVersion || m.ConfigHash != fixture.Hash || m.StartedAt.Before(last) ||
			m.At.Sub(m.StartedAt) < time.Duration(rules.IntervalMS)*time.Millisecond || remaining <= 0 || m.DurabilityBeforeU != remaining ||
			m.ToolVersionBefore != tool.Version+int64(index) || m.ToolVersionAfter != m.ToolVersionBefore+1 {
			t.Fatal("矿区出现战斗、重复轮次、非原工具或提前判定", op)
		}
		remaining = max(remaining-rules.ToolWearU, 0)
		last = m.At
		if m.DurabilityAfterU != remaining || m.ToolExhausted != (remaining == 0) {
			t.Fatal("原工具耐久增量和耗尽状态不符", m)
		}
		if index == 0 {
			if m.HostSeconds != 0 || m.HostingSession != "" {
				t.Fatal("在线第一轮被当作离线计费", m)
			}
		} else {
			if hostSession == "" {
				hostSession = m.HostingSession
			}
			if hostSession == "" || m.HostingSession != hostSession || m.HostSeconds != rules.IntervalMS/1000 {
				t.Fatal("离线轮次没有使用唯一原会话或完整时间", m)
			}
			hostedCount++
		}
		hostedBalance -= m.HostSeconds
		if hostedBalance != m.RemainingHostSeconds {
			t.Fatal("剩余托管余额无法从原逐轮扣时复算", m)
		}
		var audit struct {
			Fact persistence.MiningAttemptReceipt `bson:"mining_attempt"` // 同事务记录的原挖矿回执。
		}
		must(t, trial.store.DB.Collection("audit_outbox").FindOne(trial.ctx, bson.M{"_id": op.ID, "kind": op.Kind, "source_seq": op.SourceSeq}).Decode(&audit))
		if !reflect.DeepEqual(audit.Fact, *m) {
			t.Fatal("原矿石操作与独立审计不一致", op.ID)
		}
		ledger.Attempts = append(ledger.Attempts, miningBattleAttempt{ID: op.ID, SourceSeq: op.SourceSeq, Fact: *m})
		ledger.HostSeconds += m.HostSeconds
		if m.Ore == nil {
			if m.AutoRecycle != nil {
				t.Fatal("未出矿仍产生回收金币", m)
			}
			continue
		}
		ledger.Successes++
		ore := *m.Ore
		if ore.SourceKind != "mining" || ore.SourceID != m.EventID || ore.Quantity != 1 || ore.OrePurity < 1 || ore.OrePurity > 100 || ore.Bound == scenario.Privilege {
			t.Fatal("原判定的矿石纯度、绑定或来源失配", ore)
		}
		price, tradableAtNPC := fixture.MiningAutoRecyclePrice(ore.ItemID)
		value := miningBattleOreGold(price, ore)
		if !scenario.Privilege && tradableAtNPC && value != (domain.Wallet{}) {
			if m.AutoRecycle == nil || *m.AutoRecycle != value || after.HasItemInstance(ore.ID) {
				t.Fatal("回收价、原钱包或实物去向发生变化", ore, m.AutoRecycle, value)
			}
			ledger.AutoGold = addMiningBattleWallet(ledger.AutoGold, value)
			if index > 0 {
				hostedGold = addMiningBattleWallet(hostedGold, value)
				hostedRecycled++
			}
		} else {
			if m.AutoRecycle != nil || !slices.Contains(after.Items, ore) {
				t.Fatal("应保留的原矿没有原样进入背包", ore, m.AutoRecycle)
			}
			ledger.Retained = append(ledger.Retained, ore)
			if index > 0 {
				hostedRetained++
			}
		}
	}
	report := after.Report
	if remaining != 0 || len(operations) < 2 || hostedBalance != after.HostingSeconds || last != after.LastMiningAt ||
		!slices.Equal(before.Equipped, after.Equipped) || before.Level != after.Level || before.Experience != after.Experience ||
		before.PendingExperience != after.PendingExperience || before.EconomicSeq != after.EconomicSeq ||
		minusMiningBattleWallet(after.Wallet, before.Wallet) != ledger.AutoGold || report.SessionID != hostSession ||
		report.UsedSeconds != ledger.HostSeconds || report.Kills != 0 || report.Experience != 0 || report.Sequence != hostedCount ||
		report.Earned != hostedGold || report.MiningRecycleCount != hostedRecycled || report.TotalDropCount != hostedRetained ||
		report.MiningRecycleGold != hostedGold.Gold || report.MiningRecycleBoundGold != hostedGold.BoundGold {
		t.Fatal("锄头使用、托管战报或矿与战钱包尚有未解释差异", report, ledger.HostSeconds, remaining)
	}
	return ledger
}

// vendor 从正常报价提交买卖，独立读取原操作，按成交前后的钱包和库存核对。
func (player miningBattlePlayer) vendor(t *testing.T, npc uint64, request *pb.VendorQuoteRequest) miningBattleVendor {
	t.Helper()
	c := player.c
	before := player.role(t)
	request.SceneEpoch, request.NpcId = c.entry.SceneEpoch, npc
	seq := c.send(p.MsgC2SVendorQuote, request)
	frame := travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CVendorQuote && f.Request == seq })
	var quote pb.VendorQuoteResult
	must(t, proto.Unmarshal(frame.Payload, &quote))
	if len(quote.QuoteId) != 16 || quote.NpcId != npc || quote.Action != request.Action || quote.Quantity != request.Quantity {
		t.Fatal("真实 NPC 报价没有冻结原条件", &quote)
	}
	commit := &pb.VendorCommitRequest{SceneEpoch: c.entry.SceneEpoch, NpcId: npc, QuoteId: quote.QuoteId, Operation: c.operation()}
	accepted := roleCardAssetWS(c, p.MsgC2SVendorCommit, commit)
	if accepted.ErrorCode != 0 || accepted.Pending || !proto.Equal(accepted, c.asset(p.MsgC2SVendorCommit, commit)) {
		t.Fatal("交易未落地或重放改变了原确认", accepted)
	}
	var op persistence.Operation
	id := "asset:" + before.ID + ":" + domain.Hex(commit.Operation.OpId)
	must(t, player.trial.store.DB.Collection("operations").FindOne(player.trial.ctx, bson.M{"_id": id}).Decode(&op))
	if op.Vendor == nil || op.Kind != persistence.AssetVendor || op.Code != 0 || op.Vendor.Plan.Action != quote.Action ||
		op.Vendor.Plan.ItemID != quote.ItemId || op.Vendor.Plan.Wallet != (domain.Wallet{Gold: quote.Gold, BoundGold: quote.BoundGold, Yuanbao: quote.Ingot, BoundYuanbao: quote.BoundIngot}) {
		t.Fatal("交易缺少与真实报价逐项相符的原回执", op)
	}
	plan := op.Vendor.Plan
	after := player.role(t)
	var issued []domain.Item
	delta := minusMiningBattleWallet(after.Wallet, before.Wallet)
	if quote.Action == 1 {
		delta = minusMiningBattleWallet(before.Wallet, after.Wallet)
		// 新物品实例在实际购买时分配，引用原交易键，报价中的模板还没有实例 ID。
		for _, item := range after.Items {
			if item.ItemID == quote.ItemId && !before.HasItemInstance(item.ID) {
				issued = append(issued, item)
			}
		}
	} else if plan.Sold == nil || !slices.Contains(before.Items, *plan.Sold) || after.HasItemInstance(plan.Sold.ID) {
		t.Fatal("实际出售没有移除原实采矿石", plan.Sold)
	}
	if delta != plan.Wallet || after.EconomicSeq != before.EconomicSeq+1 {
		t.Fatal("商店原交易或重放影响金额和经济水位", op, delta)
	}
	return miningBattleVendor{OperationID: id, ItemID: quote.ItemId, Wallet: plan.Wallet, Items: issued, Sold: plan.Sold}
}

// miningBattleOreGold 用原配置的同一逐矿有理数向下取整，返回实际应进入的钱包。
func miningBattleOreGold(price devconfig.NPCMineralBuyback, ore domain.Item) domain.Wallet {
	if price.Denominator == 0 {
		return domain.Wallet{}
	}
	value := int64(ore.OrePurity) * int64(price.Numerator) / int64(price.Denominator)
	if ore.Bound {
		return domain.Wallet{BoundGold: value}
	}
	return domain.Wallet{Gold: value}
}

// addMiningBattleWallet 按四个独立钱包相加，用于核验全部实际增减。
func addMiningBattleWallet(a, b domain.Wallet) domain.Wallet {
	return domain.Wallet{Gold: a.Gold + b.Gold, BoundGold: a.BoundGold + b.BoundGold, Yuanbao: a.Yuanbao + b.Yuanbao, BoundYuanbao: a.BoundYuanbao + b.BoundYuanbao}
}

// minusMiningBattleWallet 返回各钱包的差额。
func minusMiningBattleWallet(a, b domain.Wallet) domain.Wallet {
	return domain.Wallet{Gold: a.Gold - b.Gold, BoundGold: a.BoundGold - b.BoundGold, Yuanbao: a.Yuanbao - b.Yuanbao, BoundYuanbao: a.BoundYuanbao - b.BoundYuanbao}
}
