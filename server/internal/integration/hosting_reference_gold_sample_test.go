//go:build m1_natural_upgrade

// 从真实新角进入统一普通战斗前态，用定长窗口和永久账本采样托管报价所需的毛金币。
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
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

// referenceGoldWarmupSlots 固定三职业到首技能的前置战斗，避免比较时混入不同成长前态。
const referenceGoldWarmupSlots = 4

// goldSampleIncome 区分直接与回收收入，并保留实际结算的绑定属性。
type goldSampleIncome struct {
	DirectBound   int64 `json:"direct_bound"`
	DirectUnbound int64 `json:"direct_unbound"`
	RecycleBound  int64 `json:"recycle_bound"`
	RecycleFree   int64 `json:"recycle_unbound"`
}

// add 汇入一条击杀的独立来源，供钱包前后核验及窗口收支计算使用。
func (out *goldSampleIncome) add(row goldSampleAudit) {
	if row.Bound {
		out.DirectBound += row.Gold
	} else {
		out.DirectUnbound += row.Gold
	}
	out.RecycleBound += row.Settlement.Recycling.BoundGold
	out.RecycleFree += row.Settlement.Recycling.Gold
}

// goldSampleAudit 保存通常只能从审计侧读取的击杀时刻和直接金币，以及双方共享的结算明细。
type goldSampleAudit struct {
	ID         string                        `bson:"_id"`
	RoleID     string                        `bson:"role_id"`
	Kind       string                        `bson:"kind"`
	Rule       uint64                        `bson:"rule_version"`
	Config     string                        `bson:"config_hash"`
	ResultCode uint32                        `bson:"result_code"`
	SourceSeq  int64                         `bson:"source_seq"`
	EventAt    time.Time                     `bson:"event_at"`
	Bound      bool                          `bson:"bound"`
	Kills      int64                         `bson:"kills"`
	Gold       int64                         `bson:"gold"`
	Item       *domain.Item                  `bson:"item"`
	Settlement persistence.SettlementReceipt `bson:"settlement"`
	Potion     *persistence.PotionReceipt    `bson:"potion"`
}

// goldSampleKill 保存复算单次收益所需的最小数据，不保存玩家账号或连接凭证。
type goldSampleKill struct {
	SourceSeq int64            `json:"source_seq"`
	EventAt   time.Time        `json:"event_at"`
	MapID     uint64           `json:"map_id"`
	MonsterID uint32           `json:"monster_id"`
	PoolID    uint32           `json:"pool_id"`
	Equipment bool             `json:"equipment"`
	Guarantee bool             `json:"guarantee"`
	Income    goldSampleIncome `json:"income"`
}

// goldSampleSummary 记录精确半开时间窗、用户可观察的停战时间及所有可复算的击杀。
type goldSampleSummary struct {
	Job               uint32           `json:"job"`
	Repeat            int              `json:"repeat"`
	RuleVersion       uint64           `json:"rule_version"`
	ConfigHash        string           `json:"config_hash"`
	RoleID            string           `json:"role_id"`
	MapID             uint64           `json:"map_id"`
	MonsterSlots      int              `json:"monster_slots"`
	WarmupSlots       int              `json:"warmup_slots"`
	Start             time.Time        `json:"window_start_inclusive"`
	End               time.Time        `json:"window_end_exclusive"`
	CutoffObservedMS  int64            `json:"cutoff_observed_lag_ms"`
	AutomaticActiveMS int64            `json:"observed_active_ms"`
	StoppedMS         int64            `json:"observed_stopped_ms"`
	Deaths            int              `json:"observed_deaths"`
	HostSeconds       int64            `json:"host_seconds_spent"`
	BeforeLevel       uint32           `json:"level_before"`
	AfterLevel        uint32           `json:"level_after"`
	StarterSkill      uint32           `json:"starter_skill_id"`
	StarterSkillRank  uint32           `json:"starter_skill_rank"`
	EquipmentIDs      []uint32         `json:"equipped_item_ids"`
	RecycleQualityIDs []uint32         `json:"recycle_quality_ids"`
	StartSource       int64            `json:"source_seq_before"`
	EndSource         int64            `json:"source_seq_after"`
	SourcePairs       int              `json:"source_pairs"`
	AutoManaPotions   int              `json:"auto_mana_potion_sources"`
	OutOfWindowKills  int              `json:"out_of_window_kills"`
	EquipmentDrops    int              `json:"equipment_drops"`
	GuaranteeHits     int              `json:"equipment_guarantee_hits"`
	Income            goldSampleIncome `json:"income"`
	Kills             []goldSampleKill `json:"kills"`
}

// TestHostingReferenceGoldSampleMongoWS 用真实新角测一组无卡普通战斗输入，不发布价格。
func TestHostingReferenceGoldSampleMongoWS(t *testing.T) {
	cfg, ctx, store := m1FieldServices(t, m1FieldConfig(t))
	fixture := &cfg.Fixture
	window, repeats, slots := goldSampleParameters(t)
	if fixture.Map.ID != 1 || fixture.Map.Width != 13 || fixture.Map.Height != 13 || len(fixture.Map.MonsterSpawnIDs) < max(referenceGoldWarmupSlots, slots) {
		t.Fatal("当前规则已不具备林间战场及本次样本所需的怪槽")
	}
	for _, job := range []struct {
		id, skill uint32
		name      string
	}{{0, 3, "战士"}, {1, 1, "法师"}, {2, 2, "道士"}} {
		for repeat := 1; repeat <= repeats; repeat++ {
			t.Run(fmt.Sprintf("%s/%d", job.name, repeat), func(t *testing.T) {
				// 角色筹备串行走正常认证入口，战斗并行测量各自独立窗口。
				endpoint, token, key := m1FieldRole(t, cfg, job.id)
				t.Parallel()
				runReferenceGoldSample(t, ctx, store, cfg, endpoint, token, key, job.id, job.skill, repeat, slots, window)
			})
		}
	}
}

// goldSampleParameters 默认裁定 20 分钟四怪；一怪与八怪使用同一真实前态。
func goldSampleParameters(t *testing.T) (time.Duration, int, int) {
	t.Helper()
	window, repeats, slots := 20*time.Minute, 1, referenceGoldWarmupSlots
	if text := os.Getenv("MINIMIR_GOLD_SAMPLE_DURATION"); text != "" {
		duration, err := time.ParseDuration(text)
		must(t, err)
		if duration < 30*time.Second || duration > 24*time.Hour || duration%time.Millisecond != 0 {
			t.Fatal("采样长度须在30秒至24小时且以整毫秒表示")
		}
		window = duration
	}
	if text := os.Getenv("MINIMIR_GOLD_SAMPLE_REPEATS"); text != "" {
		count, err := strconv.Atoi(text)
		must(t, err)
		if count < 1 || count > 2 {
			t.Fatal("每职业只接受1或2个独立新角重复")
		}
		repeats = count
	}
	if text := os.Getenv("MINIMIR_GOLD_SAMPLE_MONSTER_SLOTS"); text != "" {
		switch text {
		case "1", "4", "8":
			slots, _ = strconv.Atoi(text)
		default:
			t.Fatal("采样怪槽必须为1、4或8")
		}
	}
	return window, repeats, slots
}

// runReferenceGoldSample 建立真实前态、锁定窗口，最后核对收支和双边流水。
func runReferenceGoldSample(t *testing.T, ctx context.Context, store *persistence.Store, cfg devconfig.Config, endpoint, token string, key *pb.RoleKey, job, skillID uint32, repeat, slots int, window time.Duration) {
	t.Helper()
	fixture := &cfg.Fixture
	skill, known := fixture.Skill(skillID)
	if !known || skill.JobID != job || skill.LearnLevel == 0 || skill.LearnedLevel != 0 {
		t.Fatalf("职业%d缺少可自然习得的首技能%d", job, skillID)
	}
	roleID := domain.Hex(key.RoleId)
	read := func() domain.Role {
		t.Helper()
		role, err := store.Role(ctx, roleID)
		must(t, err)
		return role
	}
	created := read()
	if created.Level != 1 || created.Experience != 0 || created.SourceSeq != 0 || len(created.Skills) != 0 ||
		created.Wallet != (domain.Wallet{}) || len(created.Equipped) != 0 || created.HostingSeconds != 0 {
		t.Fatal("样本必须从未注入资产、等级、装备或托管余额的真实新角开始")
	}
	c := joinWithAutoSkills(t, cfg.GameAddress, issue(t, endpoint, token, key), &pb.AutoSkillSettings{Enabled: true})
	c.stop() // 初次附着已经确认，后续自然前态和采样共享单独的时限。
	c.ctx, c.stop = context.WithTimeout(ctx, window+12*time.Minute)
	t.Cleanup(c.close)
	if c.entry.RoomId != fixture.Map.ID || c.entry.ResourceVersion != fixture.RuleVersion {
		t.Fatal("样本没有进入当前规则的林间战场")
	}
	setReferenceGoldSlots(c, referenceGoldWarmupSlots)
	equipNaturalUpgradeStarter(c, fixture)
	spare := naturalGrowthItem(c, 218, "")
	if spare == nil || spare.SourceKind != "" {
		t.Fatal("初赠备用木剑缺失，无法隔离掉落回收收入")
	}
	locked := c.asset(p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: spare.InstanceId, ItemVersion: spare.Version,
		Locked: true, Operation: c.operation()})
	if locked.ErrorCode != 0 {
		t.Fatalf("原备用木剑锁定失败：%v", locked)
	}
	c.inventoryAt(locked.NewRevision)
	policy := &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0, 1, 2, 3, 4}}
	setPolicy := c.asset(p.MsgC2SSetAutoRecycle, &pb.SetAutoRecycleRequest{Policy: policy, Operation: c.operation()})
	if setPolicy.ErrorCode != 0 {
		t.Fatalf("样本统一回收策略失败：%v", setPolicy)
	}
	c.inventoryAt(setPolicy.NewRevision)
	var warmKills, warmDeaths int
	c.onFrame = func(_ *client, frame wire.Frame) {
		if frame.ID == p.MsgS2CExperienceGrantResult && frame.Error == 0 {
			var grant pb.ExperienceGrantResult
			must(t, proto.Unmarshal(frame.Payload, &grant))
			if !grant.Migrated && grant.AcceptedExperience > 0 {
				warmKills++
			}
		}
	}
	c.auto(true, referenceGoldWarmupSlots)
	awaitNaturalEquipmentFunds(t, c, read, skill.LearnLevel, 0, referenceGoldWarmupSlots, &warmKills, &warmDeaths)
	c.auto(false, referenceGoldWarmupSlots)
	// 原攻防与耐久写入清完，再学习赠书并取采样前的钱包和来源水位。
	time.Sleep(4 * time.Second)
	c.onFrame = nil
	naturalEquipmentLearnStarter(t, c, fixture, job)
	if slots != referenceGoldWarmupSlots {
		setReferenceGoldSlots(c, slots)
	}
	before := read()
	rank, learned := before.Skill(skillID)
	if !learned || rank != skill.LearnedLevel || before.Level != skill.LearnLevel ||
		!before.HostingCardUntil.IsZero() || !before.PrivilegeUntil.IsZero() || before.HostingSeconds != 0 || len(before.Equipped) != 2 {
		t.Fatalf("职业%d没有从无卡原装自然到首技能的统一前态，等级%d", job, before.Level)
	}
	sample := goldSampleSummary{Job: job, Repeat: repeat, RuleVersion: fixture.RuleVersion, ConfigHash: fixture.Hash, RoleID: roleID,
		MapID: fixture.Map.ID, MonsterSlots: slots, WarmupSlots: referenceGoldWarmupSlots, BeforeLevel: before.Level, StarterSkill: skillID, StarterSkillRank: rank,
		EquipmentIDs: []uint32{218, 193}, RecycleQualityIDs: policy.QualityIds, StartSource: before.SourceSeq}
	t.Logf("职业%d重复%d进入%s独立采样，%d怪，等级%d，暖机%d杀%d死，规则%d配置%s", job, repeat, window, slots, before.Level, warmKills, warmDeaths, fixture.RuleVersion, fixture.Hash)
	observeGoldSample(t, c, &sample, window)
	time.Sleep(4 * time.Second)
	after := read()
	sample.AfterLevel, sample.EndSource = after.Level, after.SourceSeq
	sample.HostSeconds = before.HostingSeconds - after.HostingSeconds
	if sample.HostSeconds != 0 || !after.HostingCardUntil.IsZero() || !after.PrivilegeUntil.IsZero() {
		t.Fatal("线上无卡对照窗口意外发生离线扣时或权益变化")
	}
	verifyGoldSampleLedger(t, ctx, store, &sample, before, after, fixture)
	encoded, err := json.Marshal(sample)
	must(t, err)
	t.Logf("毛金币采样 %s", encoded)
}

// setReferenceGoldSlots 等到服务端新基线确实按指定槽数更新后再继续。
func setReferenceGoldSlots(c *client, slots int) {
	c.auto(false, uint32(slots))
	travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && travelCount(c.scene, 3) == slots
	})
}

// observeGoldSample 按协议确认启停，以服务所在同机的 UTC 毫秒裁定固定窗口。
func observeGoldSample(t *testing.T, c *client, sample *goldSampleSummary, window time.Duration) {
	t.Helper()
	start := time.Now()
	deadline := start.Add(window)
	sample.Start = start.UTC().Truncate(time.Millisecond)
	sample.End = sample.Start.Add(window)
	last, active := start, false
	var activeTime time.Duration
	setActive := func(now time.Time, enabled bool) {
		if now.After(deadline) {
			now = deadline
		}
		if active {
			activeTime += now.Sub(last)
		}
		last, active = now, enabled
	}
	c.onFrame = func(_ *client, frame wire.Frame) {
		if frame.Error != 0 {
			return
		}
		now := time.Now()
		switch frame.ID {
		case p.MsgS2CDeathNotice:
			if now.Before(deadline) {
				sample.Deaths++
			}
			setActive(now, false)
		case p.MsgS2CHostingState:
			setActive(now, c.hosting.AutoBattleEnabled)
		}
	}
	stopHeartbeat := sampleHeartbeat(c, deadline)
	defer stopHeartbeat()
	c.auto(true, uint32(sample.MonsterSlots))
	for time.Now().Before(deadline) {
		frame := c.read()
		if frame.ID != p.MsgS2CDeathNotice || !time.Now().Before(deadline) {
			continue
		}
		if c.death.GetAllowedActions()&1 == 0 {
			continue // 真实不可复活的停战时间仍计入完整毛产出分母。
		}
		result := c.asset(p.MsgC2SReviveChoice, &pb.ReviveChoice{DeathId: c.death.DeathId, Action: 1, Operation: c.operation()})
		if result.ErrorCode != 0 {
			t.Fatalf("样本中通过普通入口复活失败：%v", result)
		}
		c.inventoryAt(result.NewRevision)
		c.until(func(frame wire.Frame) bool { return frame.ID == p.MsgS2CSceneBaseline && c.player().GetHp() > 0 })
		if time.Now().Before(deadline) {
			c.auto(true, uint32(sample.MonsterSlots))
		}
	}
	sample.CutoffObservedMS = time.Since(deadline).Milliseconds()
	setActive(deadline, false)
	c.onFrame = nil
	c.auto(false, uint32(sample.MonsterSlots))
	sample.AutomaticActiveMS = activeTime.Milliseconds()
	sample.StoppedMS = window.Milliseconds() - sample.AutomaticActiveMS
}

// verifyGoldSampleLedger 对拍永久来源并复算钱包，只有落在半开窗口内的击杀进入样本。
func verifyGoldSampleLedger(t *testing.T, ctx context.Context, store *persistence.Store, sample *goldSampleSummary, before, after domain.Role, fixture *devconfig.Fixture) {
	t.Helper()
	query := bson.M{"role_id": before.ID, "source_seq": bson.M{"$gt": before.SourceSeq, "$lte": after.SourceSeq}}
	operations, err := store.DB.Collection("operations").Find(ctx, query)
	must(t, err)
	var receipts []persistence.Operation
	must(t, operations.All(ctx, &receipts))
	audits, err := store.DB.Collection("audit_outbox").Find(ctx, query, options.Find().SetSort(bson.D{{Key: "source_seq", Value: 1}}))
	must(t, err)
	var rows []goldSampleAudit
	must(t, audits.All(ctx, &rows))
	if len(receipts) != len(rows) || int64(len(rows)) != after.SourceSeq-before.SourceSeq {
		t.Fatalf("采样包围区间有漏账，原水位%d→%d、回执%d、审计%d", before.SourceSeq, after.SourceSeq, len(receipts), len(rows))
	}
	bySource := make(map[int64]persistence.Operation, len(receipts))
	for _, receipt := range receipts {
		if receipt.ID == "" || receipt.Digest == "" || bySource[receipt.SourceSeq].ID != "" {
			t.Fatalf("原来源%d回执重复或缺失", receipt.SourceSeq)
		}
		bySource[receipt.SourceSeq] = receipt
	}
	var gross goldSampleIncome
	missed := before.NoEquipmentKills
	for _, row := range rows {
		receipt, exists := bySource[row.SourceSeq]
		if !exists || receipt.ID != row.ID || receipt.RoleID != row.RoleID || receipt.Kind != row.Kind ||
			row.Rule != sample.RuleVersion || row.Config != sample.ConfigHash || receipt.Code != row.ResultCode {
			t.Fatalf("原来源%d的回执、审计、规则或托管事实不一致", row.SourceSeq)
		}
		delete(bySource, row.SourceSeq)
		if row.Kind == persistence.AssetAutoUsePotion {
			if !reflect.DeepEqual(receipt.Potion, row.Potion) || row.Kills != 0 || row.Gold != 0 || receipt.Settlement != nil ||
				row.ResultCode == 0 && (row.Potion == nil || row.Potion.HP != 0 || row.Potion.MP <= 0) {
				t.Fatalf("原自动用药来源%d与审计不符或误记金币", row.SourceSeq)
			}
			if row.ResultCode == 0 {
				sample.AutoManaPotions++
			}
			continue
		}
		if row.Kind != "settlement" || receipt.ID != fmt.Sprintf("settle:%s:%d", before.ID, receipt.SourceSeq) || receipt.Code != 0 ||
			receipt.Settlement == nil || !reflect.DeepEqual(*receipt.Settlement, row.Settlement) || row.EventAt.IsZero() ||
			row.Settlement.RuleVersion != sample.RuleVersion || row.Settlement.ConfigHash != sample.ConfigHash ||
			row.Settlement.HostSeconds != 0 || row.Settlement.HostingKillBonus != nil || row.Kills != row.Settlement.Kills {
			t.Fatalf("原来源%d不是同版本的确定无卡战斗回执：%s", row.SourceSeq, row.Kind)
		}
		verifyGoldSampleRecycling(t, row)
		if row.Kills == 0 {
			continue
		}
		monster, known := fixture.MonsterByID(row.Settlement.MonsterID)
		if row.Kills != 1 || !row.Bound || !known || row.Settlement.MapID != sample.MapID || row.Settlement.DropPoolID != monster.DropPoolID ||
			row.Gold < 0 || row.Gold > 0 && (row.Gold < monster.GoldMin || row.Gold > monster.GoldMax) {
			t.Fatalf("原来源%d含不符合无卡普通图规则的击杀收入", row.SourceSeq)
		}
		var equipment bool
		if row.Item != nil {
			definition, known := fixture.Item(row.Item.ItemID)
			if !known {
				t.Fatalf("原来源%d掉落道具未关联当前规则", row.SourceSeq)
			}
			equipment = definition.IsEquipment
		}
		missed++
		guaranteed := missed >= fixture.Drops.EquipmentGuarantee
		if guaranteed && !equipment {
			t.Fatalf("原来源%d达到保底却未选择装备", row.SourceSeq)
		}
		if equipment {
			missed = 0
		}
		gross.add(row)
		if row.EventAt.Before(sample.Start) || !row.EventAt.Before(sample.End) {
			sample.OutOfWindowKills++
			continue
		}
		sample.Income.add(row)
		kill := goldSampleKill{SourceSeq: row.SourceSeq, EventAt: row.EventAt, MapID: row.Settlement.MapID,
			MonsterID: row.Settlement.MonsterID, PoolID: row.Settlement.DropPoolID, Equipment: equipment, Guarantee: guaranteed}
		kill.Income.add(row)
		sample.Kills = append(sample.Kills, kill)
		if equipment {
			sample.EquipmentDrops++
			if guaranteed {
				sample.GuaranteeHits++
			}
		}
	}
	sample.SourcePairs = len(rows)
	if len(sample.Kills) == 0 || len(bySource) != 0 || missed != after.NoEquipmentKills || after.Wallet.BoundGold-before.Wallet.BoundGold != gross.DirectBound+gross.RecycleBound ||
		after.Wallet.Gold-before.Wallet.Gold != gross.DirectUnbound+gross.RecycleFree || gross.DirectUnbound != 0 || gross.RecycleFree != 0 {
		t.Fatalf("结算来源不完整或钱包无法由真实直金和回收复算：所得%+v 钱包前后%+v→%+v", gross, before.Wallet, after.Wallet)
	}
}

// verifyGoldSampleRecycling 排除赠品、历史库存或人工处置形成的金币。
func verifyGoldSampleRecycling(t *testing.T, row goldSampleAudit) {
	t.Helper()
	result := row.Settlement.Recycling
	if row.Kills == 0 && (row.Gold != 0 || result.BoundGold != 0 || result.Gold != 0 || len(result.Recycled) != 0) {
		t.Fatalf("非击杀来源%d意外产生金币或回收", row.SourceSeq)
	}
	for _, item := range result.Recycled {
		if !item.Bound || item.SourceKind != "monster_drop" || item.SourceID != strconv.FormatInt(row.SourceSeq, 10) {
			t.Fatalf("回收来源%d并非同次击杀绑定掉落", row.SourceSeq)
		}
	}
}
