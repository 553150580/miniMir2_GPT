//go:build m4_economy_sampling

// 本文件逐笔配对普通挂机原回执与审计，独立核算两类金币、经验、掉落、药耗和托管秒数。
package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// economyCut 保存角色账本水位和经过正常协议生效的档位前态。
type economyCut struct {
	Level          uint32                `json:"level"`           // 窗口前后的实际等级。
	Experience     int64                 `json:"experience"`      // 本级实际留下的经验。
	PoolExperience int64                 `json:"pool_experience"` // 待用经验池。
	Wallet         domain.Wallet         `json:"wallet"`          // 四个币种分别精确对账。
	Hosting        int64                 `json:"hosting"`         // 可用的绝对托管余额。
	Revision       int64                 `json:"revision"`        // 已确认的 MongoDB 角色修订。
	Source         int64                 `json:"source"`          // 服务端收益流水水位。
	Command        int64                 `json:"command"`         // 经济命令流水水位。
	Equipment      []domain.Equipped     `json:"equipment"`       // 原穿戴实例和耐久。
	Skills         []domain.LearnedSkill `json:"skills"`          // 通过原技能书实际学习的技能和阶。
	Potions        map[uint32]int64      `json:"potions"`         // 小量红蓝药的背包数量。
}

// economyYield 是某一时间段内的普通战斗原始收入。未出售道具单列。
type economyYield struct {
	Kills            int64            `json:"kills"`              // 实际确认的普通击杀。
	RawExperience    int64            `json:"raw_experience"`     // 减去经验封顶之前的实际击杀经验。
	Experience       int64            `json:"experience"`         // 进入人物进度或池的已接收经验。
	Direct           domain.Wallet    `json:"direct"`             // 含逐杀托管卡加成的直金及绑定分账。
	Recycle          domain.Wallet    `json:"recycle"`            // 实际自动回收金币及绑定分账。
	HostingBonusGold int64            `json:"hosting_bonus_gold"` // 已计入直金的托管卡部分。
	HostingBonusExp  int64            `json:"hosting_bonus_exp"`  // 已计入原经验的托管卡部分。
	HostingUsed      int64            `json:"hosting_used"`       // 本段实际提交的离线整秒。
	WearU            int64            `json:"wear_u"`             // 实际降低的所有穿戴耐久。
	WearDeaths       int64            `json:"wear_deaths"`        // 具有永久磨损标记的死亡。
	Dropped          map[string]int64 `json:"dropped"`            // 掉落模板、品质、绑定分组的原数量。
	Retained         map[string]int64 `json:"retained"`           // 实际进入背包的掉落数量。
	Discarded        int64            `json:"discarded"`          // 满包等原因没有留存的数量。
}

// economyWindowLedger 将入窗的收益和边界事务分开，资产核对始终用二者全量。
type economyWindowLedger struct {
	Inside      economyYield  // 以原事件时间进入半开窗口的收益。
	Outside     economyYield  // 已落账但在窗口之外的收益和扣时。
	Net         domain.Wallet // 样本开始至确认停止之间的四钱包实际变化。
	SourceCount int64         // 匹配且连续的服务端来源数。
	Commands    int64         // 匹配且连续的玩家命令数。
}

// economyReport 固定指标口径、原价格状态、起止资产和可复算的细账文件。
type economyReport struct {
	Scenario                economyScenario  `json:"scenario"`
	ConfigHash              string           `json:"config_hash"`
	RuleVersion             uint64           `json:"rule_version"`
	Start                   time.Time        `json:"start"`
	End                     time.Time        `json:"end"`
	CutoffLagMS             int64            `json:"cutoff_lag_ms"`
	OnlineActiveMS          int64            `json:"online_active_ms"`
	ObservedDeaths          int              `json:"observed_online_deaths"`
	StartState              economyCut       `json:"start_state"`
	EndState                economyCut       `json:"end_state"`
	Quote                   economyPrice     `json:"hosting_price"`
	Gross                   economyYield     `json:"gross"`
	Boundary                economyYield     `json:"boundary"`
	Net                     domain.Wallet    `json:"net_window_and_boundary"`
	Sources                 int64            `json:"source_pairs"`
	Commands                int64            `json:"command_pairs"`
	PotionUse               map[uint32]int64 `json:"potion_use"`
	DepositedHostingSeconds int64            `json:"seeded_prior_hosting_seconds"`
	ClaimedHostingSeconds   int64            `json:"actual_daily_claim_seconds"`
	PaidCardUnits           int64            `json:"prep_card_spend_yuanbao_units"`
	SourceLog               string           `json:"source_log"`
	Maintenance             *economyCare     `json:"post_window_maintenance,omitempty"`
}

type economyPrice struct {
	Code      uint32 `json:"code"`         // 权威报价的结果码。
	Version   uint64 `json:"version"`      // 生效价表版本，缺省时为零。
	Reference int64  `json:"g_ref"`        // 每小时标准毛金，价格未开放时为零。
	Hourly    int64  `json:"hourly_price"` // 权威每小时售价。
}

// economyAudit 保留用于核对同一持久回执的原审计字段。
type economyAudit struct {
	ID         string                            `bson:"_id"`
	RoleID     string                            `bson:"role_id"`
	Kind       string                            `bson:"kind"`
	Code       uint32                            `bson:"result_code"`
	Source     int64                             `bson:"source_seq"`
	Rule       uint64                            `bson:"rule_version"`
	Config     string                            `bson:"config_hash"`
	EventAt    time.Time                         `bson:"event_at"`
	Bound      bool                              `bson:"bound"`
	Gold       int64                             `bson:"gold"`
	Experience int64                             `bson:"experience"`
	Item       *domain.Item                      `bson:"item"`
	Settlement *persistence.SettlementReceipt    `bson:"settlement"`
	Mining     *persistence.MiningAttemptReceipt `bson:"mining_attempt"`
	Potion     *persistence.PotionReceipt        `bson:"potion"`
	Vendor     *persistence.VendorReceipt        `bson:"npc_trade"`
	Binding    *persistence.BindingReceipt       `bson:"binding_use"`
}

// economySource 是从 MongoDB 清退后仍可逐条重新聚合的最小事实。
type economySource struct {
	ID        string    `json:"id"`
	At        time.Time `json:"at"`
	Source    int64     `json:"source,omitempty"`
	Command   int64     `json:"command,omitempty"`
	Kind      string    `json:"kind"`
	Code      uint32    `json:"code,omitempty"`
	Monster   uint32    `json:"monster,omitempty"`
	Direct    int64     `json:"direct_gold,omitempty"`
	Bound     bool      `json:"bound,omitempty"`
	Recycle   int64     `json:"recycle_gold,omitempty"`
	RecycleB  int64     `json:"recycle_bound_gold,omitempty"`
	Exp       int64     `json:"experience,omitempty"`
	RawExp    int64     `json:"raw_experience,omitempty"`
	BonusGold int64     `json:"bonus_gold,omitempty"`
	BonusExp  int64     `json:"bonus_exp,omitempty"`
	Item      string    `json:"drop_key,omitempty"`
	Quantity  int64     `json:"drop_quantity,omitempty"`
	Retained  bool      `json:"retained,omitempty"`
	Discarded int64     `json:"discarded,omitempty"`
	Hosting   int64     `json:"hosting_used,omitempty"`
	Wear      int64     `json:"wear_u,omitempty"`
	Death     bool      `json:"wear_death,omitempty"`
	Potion    uint32    `json:"potion,omitempty"`
}

func economyState(role domain.Role) economyCut {
	return economyCut{role.Level, role.Experience, role.PendingExperience, role.Wallet, role.HostingSeconds,
		role.Revision, role.SourceSeq, role.EconomicSeq, role.Equipped, role.Skills, economyPotions(role)}
}

func economyPotions(role domain.Role) map[uint32]int64 {
	out := map[uint32]int64{economyHealthPotion: 0, economyManaPotion: 0}
	for _, item := range role.Items {
		if item.ItemID == economyHealthPotion || item.ItemID == economyManaPotion {
			out[item.ItemID] += item.Quantity
		}
	}
	return out
}

func newEconomyReport(a *economyActor, before domain.Role, quote *pb.HostingQuoteResult) economyReport {
	f := &a.cfg.Fixture
	if quote.QuoteVersion != 0 && (quote.RuleVersion != f.RuleVersion || quote.ConfigHash != f.Hash ||
		quote.ReferenceGoldPerHour <= 0 || quote.GoldPerHour != 2*quote.ReferenceGoldPerHour) {
		a.t.Fatal("样本观察到的实际托管价表与原配置或两倍口径不符", quote)
	}
	return economyReport{Scenario: a.scenario, ConfigHash: f.Hash, RuleVersion: f.RuleVersion,
		StartState: economyState(before), PaidCardUnits: a.paidCard, SourceLog: a.scenario.key() + ".jsonl",
		Quote: economyPrice{quote.ErrorCode, quote.QuoteVersion, quote.ReferenceGoldPerHour, quote.GoldPerHour}}
}

// reconcileEconomy 按有序修订分批读主库，用原编号配对两边账本，成功后保留可复算细账。
func (a *economyActor) reconcileEconomy(before, after domain.Role, start, end time.Time) (economyWindowLedger, map[uint32]int64) {
	t := a.t
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Minute)
	defer cancel()
	ops := a.store.DB.Collection("operations")
	query := bson.M{"role_id": before.ID, "revision": bson.M{"$gt": before.Revision, "$lte": after.Revision}}
	cursor, err := ops.Find(ctx, query, options.Find().SetSort(bson.D{{Key: "revision", Value: 1}}))
	must(t, err)
	defer cursor.Close(ctx)
	path := filepath.Join(os.Getenv("MINIMIR_ECONOMY_SAMPLES"), a.scenario.key()+".jsonl")
	file, err := os.OpenFile(path+".partial", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	must(t, err)
	defer file.Close()
	writer := bufio.NewWriterSize(file, 32*1024)
	stream := json.NewEncoder(writer)
	var out economyWindowLedger
	used := map[uint32]int64{economyHealthPotion: 0, economyManaPotion: 0}
	source, command, revision := before.SourceSeq, before.EconomicSeq, before.Revision
	batch := make([]persistence.Operation, 0, 192)
	flush := func() {
		facts := a.readEconomyAudits(ctx, batch)
		for _, operation := range batch {
			row := a.economyFact(before, operation, facts[operation.ID])
			if operation.Revision <= revision {
				t.Fatal("角色修订无序或重复", operation.ID, operation.Revision)
			}
			revision = operation.Revision
			if row.Source > 0 {
				source++
				if row.Source != source || operation.CommandSeq != command {
					t.Fatal("服务端来源与原角色两个水位失配", operation.ID, row.Source, source, operation.CommandSeq, command)
				}
			} else {
				command++
				if row.Command != command {
					t.Fatal("经济命令流水缺失或重复", operation.ID, row.Command, command)
				}
			}
			if row.Potion != 0 {
				used[row.Potion]++
			}
			if !row.At.Before(start) && row.At.Before(end) {
				out.Inside.add(row)
			} else {
				out.Outside.add(row)
			}
			must(t, stream.Encode(row))
		}
		batch = batch[:0]
	}
	for cursor.Next(ctx) {
		var operation persistence.Operation
		must(t, cursor.Decode(&operation))
		batch = append(batch, operation)
		if len(batch) == cap(batch) {
			flush()
		}
	}
	must(t, cursor.Err())
	if len(batch) != 0 {
		flush()
	}
	out.SourceCount, out.Commands = source-before.SourceSeq, command-before.EconomicSeq
	out.Net = economyDiff(after.Wallet, before.Wallet)
	earning := economySum(economySum(out.Inside.Direct, out.Inside.Recycle), economySum(out.Outside.Direct, out.Outside.Recycle))
	hosting := out.Inside.HostingUsed + out.Outside.HostingUsed
	if source != after.SourceSeq || command != after.EconomicSeq || earning != out.Net ||
		before.HostingSeconds-after.HostingSeconds != hosting || a.scenario.Mode == "online" && hosting != 0 {
		t.Fatal("独立来源、命令、四钱包或托管时长没有对平", source, command, after.SourceSeq, after.EconomicSeq, earning, out.Net, hosting)
	}
	for _, potion := range []uint32{economyHealthPotion, economyManaPotion} {
		prefix := fmt.Sprintf("%d:", potion)
		actual := economyPotions(after)[potion]
		var kept int64
		for _, part := range []economyYield{out.Inside, out.Outside} {
			for key, count := range part.Retained {
				if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
					kept += count
				}
			}
		}
		if expected := economyPotions(before)[potion] - used[potion] + kept; actual != expected {
			t.Fatal("红蓝药库存、原使用回执和已入背包掉落没有守恒", potion, actual, expected)
		}
	}
	must(t, writer.Flush())
	must(t, file.Close())
	must(t, os.Rename(path+".partial", path))
	return out, used
}

// readEconomyAudits 用相同原主键读取审计，不能以同时间或同金额猜配对。
func (a *economyActor) readEconomyAudits(ctx context.Context, batch []persistence.Operation) map[string]economyAudit {
	a.t.Helper()
	ids := make([]string, 0, len(batch))
	for _, op := range batch {
		ids = append(ids, op.ID)
	}
	cursor, err := a.store.DB.Collection("audit_outbox").Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	must(a.t, err)
	var rows []economyAudit
	must(a.t, cursor.All(ctx, &rows))
	if len(rows) != len(batch) {
		a.t.Fatal("原回执或审计有缺失", len(rows), len(batch))
	}
	facts := make(map[string]economyAudit, len(rows))
	for _, row := range rows {
		facts[row.ID] = row
	}
	return facts
}

// economyFact 按规则及原归属核对同笔回执和审计，未知资产类型会使样本失败。
func (a *economyActor) economyFact(before domain.Role, op persistence.Operation, audit economyAudit) economySource {
	t := a.t
	t.Helper()
	f := &a.cfg.Fixture
	if op.ID == "" || op.Digest == "" || op.ID != audit.ID || op.RoleID != before.ID || op.RoleID != audit.RoleID ||
		op.Kind != audit.Kind || op.Code != audit.Code || op.SourceSeq != audit.Source {
		t.Fatal("原操作和审计不能对应", op.ID, op.Kind)
	}
	row := economySource{ID: op.ID, At: op.Created, Source: op.SourceSeq, Kind: op.Kind, Code: op.Code}
	if row.Source == 0 {
		row.Command = op.CommandSeq
	} else if op.Kind != "safe_zone_binding" && (audit.Rule != f.RuleVersion || audit.Config != f.Hash) {
		t.Fatal("收益来源换了原规则或配置", op.ID)
	}
	switch op.Kind {
	case "settlement":
		s := op.Settlement
		if s == nil || op.Code != 0 || !reflect.DeepEqual(s, audit.Settlement) || audit.EventAt.IsZero() ||
			s.RuleVersion != f.RuleVersion || s.ConfigHash != f.Hash || s.Kills < 0 || s.Kills > 1 {
			t.Fatal("原战斗、磨损或托管回执内容不符", op.ID)
		}
		row.At, row.Hosting = audit.EventAt, s.HostSeconds
		row.Death = s.WearDeathID != ""
		for _, wear := range s.EquipmentWear {
			row.Wear += wear.Before.DurabilityU - wear.After.DurabilityU
		}
		row.Recycle, row.RecycleB, row.Discarded = s.Recycling.Gold, s.Recycling.BoundGold, s.Recycling.DiscardedCount
		a.checkEconomyRecycling(before.SourceSeq, op.SourceSeq, s)
		if s.Kills == 1 {
			row.Monster, row.Direct, row.Bound = s.MonsterID, audit.Gold, audit.Bound
			a.checkEconomyKill(before, s, audit, &row)
		}
		if audit.Item != nil {
			item := audit.Item
			row.Item = fmt.Sprintf("%d:%d:%t", item.ItemID, item.Quality, item.Bound)
			row.Quantity, row.Retained = item.Quantity, s.GrantedItem
		}
	case persistence.AssetAutoUsePotion, "use_item":
		if op.Kind == persistence.AssetAutoUsePotion && !reflect.DeepEqual(op.Potion, audit.Potion) {
			t.Fatal("自动扣药的审计缺少同一药水回执", op.ID)
		}
		if op.Code == 0 {
			if op.Potion == nil || (op.Potion.ItemID != economyHealthPotion && op.Potion.ItemID != economyManaPotion) {
				t.Fatal("药水使用没有已选的小量红蓝药回执", op.ID)
			}
			row.Potion = op.Potion.ItemID
		}
	case "revive":
		// 隔离线上角色只使用当前免费复活入口。
	case "safe_zone_binding":
		if op.Code != 0 || op.Binding == nil || !reflect.DeepEqual(op.Binding, audit.Binding) {
			t.Fatal("收窗切换安全区的原绑定回执和审计不一致", op.ID)
		}
	default:
		t.Fatal("普通挂机窗口收到未定义的资产来源", op.ID, op.Kind)
	}
	if row.At.IsZero() {
		t.Fatal("原账本缺少事件时刻", op.ID)
	}
	return row
}

// checkEconomyKill 验证击杀归属、原怪掉落池及托管卡只额外发放直接金和经验。
func (a *economyActor) checkEconomyKill(before domain.Role, s *persistence.SettlementReceipt, audit economyAudit, row *economySource) {
	t := a.t
	t.Helper()
	f := &a.cfg.Fixture
	monster, known := f.MonsterByID(s.MonsterID)
	if !known || s.MapID != a.scenario.MapID || s.DropPoolID != monster.DropPoolID ||
		audit.Bound == before.PrivilegeUntil.After(audit.EventAt) || audit.Gold < 0 || audit.Experience < 0 ||
		s.Experience == nil || s.Experience.RawExperience != audit.Experience {
		t.Fatal("本次击杀的地图、怪物、特权绑定或原经验失配", audit.ID)
	}
	row.RawExp, row.Exp = audit.Experience, s.Experience.ActualGranted
	if audit.Item != nil && audit.Item.Bound != audit.Bound {
		t.Fatal("普通掉落没有继承原击杀时的绑定", audit.ID)
	}
	baseGold, baseExp := audit.Gold, audit.Experience
	if bonus := s.HostingKillBonus; bonus != nil {
		want, err := persistence.NewHostingKillBonus(before.HostingCardUntil, bonus.BaseExperience, bonus.BaseDirectGold, f.HostingBenefits)
		must(t, err)
		if !reflect.DeepEqual(bonus, want) || audit.Gold != bonus.BaseDirectGold+bonus.BonusDirectGold ||
			audit.Experience != bonus.BaseExperience+bonus.BonusExperience {
			t.Fatal("托管卡曾应用额外费用或更改原直接收益", audit.ID)
		}
		baseGold, baseExp = bonus.BaseDirectGold, bonus.BaseExperience
		row.BonusGold, row.BonusExp = bonus.BonusDirectGold, bonus.BonusExperience
	}
	if (s.HostingKillBonus != nil) != before.HostingCardUntil.After(audit.EventAt) || baseExp != monster.Exp ||
		baseGold > 0 && (baseGold < monster.GoldMin || baseGold > monster.GoldMax || audit.Item != nil) {
		t.Fatal("原击杀掉落、经验或卡期超出相同规则", audit.ID)
	}
}

// checkEconomyRecycling 按被回收原实例的绑定分账。之前掉落的同窗库存可随当前击杀回收。
func (a *economyActor) checkEconomyRecycling(start, source int64, settlement *persistence.SettlementReceipt) {
	t := a.t
	t.Helper()
	r := settlement.Recycling
	var bound, gold int64
	for _, item := range r.Recycled {
		original, err := strconv.ParseInt(item.SourceID, 10, 64)
		definition, known := a.cfg.Fixture.Item(item.ItemID)
		if err != nil || original <= start || original > source || item.SourceKind != "monster_drop" ||
			!known || !definition.IsEquipment || item.Quantity != 1 || definition.RecycleGold < 0 {
			t.Fatal("回收收入夹入窗口前原件或无有效来源的装备", source, item.ItemID, item.SourceKind, item.SourceID)
		}
		if item.Bound {
			bound += definition.RecycleGold
		} else {
			gold += definition.RecycleGold
		}
	}
	if bound != r.BoundGold || gold != r.Gold {
		t.Fatal("自动回收金币未按被回收原件的绑定与原价分账", source, bound, gold)
	}
}

func (out *economyYield) add(row economySource) {
	if row.Monster != 0 {
		out.Kills++
	}
	if row.Bound {
		out.Direct.BoundGold += row.Direct
	} else {
		out.Direct.Gold += row.Direct
	}
	out.RawExperience += row.RawExp
	out.Experience += row.Exp
	out.Recycle.Gold += row.Recycle
	out.Recycle.BoundGold += row.RecycleB
	out.HostingBonusGold += row.BonusGold
	out.HostingBonusExp += row.BonusExp
	out.HostingUsed += row.Hosting
	out.WearU += row.Wear
	if row.Death {
		out.WearDeaths++
	}
	out.Discarded += row.Discarded
	if row.Item != "" {
		if out.Dropped == nil {
			out.Dropped = make(map[string]int64)
		}
		out.Dropped[row.Item] += row.Quantity
		if row.Retained {
			if out.Retained == nil {
				out.Retained = make(map[string]int64)
			}
			out.Retained[row.Item] += row.Quantity
		}
	}
}

func economyDiff(after, before domain.Wallet) domain.Wallet {
	return domain.Wallet{Gold: after.Gold - before.Gold, BoundGold: after.BoundGold - before.BoundGold,
		Yuanbao: after.Yuanbao - before.Yuanbao, BoundYuanbao: after.BoundYuanbao - before.BoundYuanbao}
}

func economySum(left, right domain.Wallet) domain.Wallet {
	return domain.Wallet{Gold: left.Gold + right.Gold, BoundGold: left.BoundGold + right.BoundGold,
		Yuanbao: left.Yuanbao + right.Yuanbao, BoundYuanbao: left.BoundYuanbao + right.BoundYuanbao}
}

// prepareEconomyIndexes 只在本次隔离库加采样查询索引，避免长窗结束后全表扫描。
func prepareEconomyIndexes(ctx context.Context, store *persistence.Store) error {
	_, err := store.DB.Collection("operations").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "role_id", Value: 1}, {Key: "revision", Value: 1}}})
	return err
}
