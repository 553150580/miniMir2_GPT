// 本文件使用未改动的玩法、角色和怪物，验收规则30三职业从一级到首个技能的过程。
package integration

import (
	"bytes"
	"context"
	"fmt"
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

// TestLevelGrowthNaturalM1StarterMongoWS 走正常注册、建角、初装、定点战斗、等级门禁和首书学习。
func TestLevelGrowthNaturalM1StarterMongoWS(t *testing.T) {
	cfg, ctx, store := m1FieldServices(t, m1FieldConfig(t))
	f := cfg.Fixture
	if len(f.Map.MonsterSpawnIDs) == 0 {
		t.Fatal("默认新角地图未配置普通怪物")
	}
	monster, ok := f.MonsterByID(f.Map.MonsterSpawnIDs[0])
	if !ok || monster.Exp <= 0 || f.Map.InitialMonsterCount != 1 || f.Map.Width != 13 || f.Map.Height != 13 {
		t.Fatal("新角色普通挂机地图未使用当前默认一怪和13×13配置")
	}
	for _, job := range []struct {
		id    uint32
		name  string
		skill uint32
	}{{0, "战士", 3}, {1, "法师", 1}, {2, "道士", 2}} {
		t.Run(job.name, func(t *testing.T) {
			t.Parallel()
			skill, ok := f.Skill(job.skill)
			if !ok || skill.JobID != job.id || skill.LearnLevel <= 3 || int(skill.LearnLevel) > len(f.Progression.ExpPerLevel) {
				t.Fatalf("职业%d缺少正确的首个技能门槛", job.id)
			}
			endpoint, token, key := m1FieldRole(t, cfg, job.id)
			roleID := domain.Hex(key.RoleId)
			created, err := store.Role(ctx, roleID)
			must(t, err)
			if created.Level != 1 || created.Experience != 0 || created.SourceSeq != 0 || len(created.Skills) != 0 {
				t.Fatal("新角已有经验、来源或学习记录")
			}
			trace := newStarterGrowthTrace(t, &f, key, monster)
			c := join(t, cfg.GameAddress, issue(t, endpoint, token, key), trace.observe)
			c.stop() // 公共入场辅助的80秒时限已完成附着，实际成长有独立时限。
			c.ctx, c.stop = context.WithTimeout(ctx, 270*time.Second)
			t.Cleanup(c.close)
			if c.entry.GetResourceVersion() != f.RuleVersion || c.entry.GetRoomId() != f.Map.ID || c.inventory.GetLevel() != 1 ||
				c.inventory.GetProgression().GetCurveVersion() != f.Progression.CurveVersion || c.scene.GetOwnedPet() != nil {
				t.Fatal("新角首次入场规则、等级曲线、地图或宠物状态不符")
			}
			c.auto(false, 1)
			for _, itemID := range []uint32{218, 193} {
				item := naturalGrowthItem(c, itemID, "")
				definition, known := f.Item(itemID)
				if item == nil || !known || !item.Bound || !definition.IsEquipment {
					t.Fatalf("缺少新角原装 %d", itemID)
				}
				result := c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: item.InstanceId,
					Slot: definition.EquipmentSlot, ItemVersion: item.Version, Operation: c.operation()})
				if result.ErrorCode != 0 {
					t.Fatalf("正常穿戴初装%d失败：%v", itemID, result)
				}
				c.inventoryAt(result.NewRevision)
			}
			book := naturalGrowthItem(c, skill.BookItemID, "initial_grant")
			if book == nil || !book.Bound || book.Quantity != 1 || book.SourceId == "" {
				t.Fatal("正常新角色没有获得对应职业的原始赠书")
			}
			learn := func() *pb.OperationResult {
				return c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: book.InstanceId, SkillId: skill.ID, Operation: c.operation()})
			}
			if rejected := learn(); rejected.ErrorCode != p.CodeInvalidArgument {
				t.Fatalf("一级不应提前学习%d：%v", skill.ID, rejected)
			}
			trace.origin = proto.Clone(c.player().Position).(*pb.GridPoint)
			trace.started = time.Now()
			c.auto(true, 1)
			naturalGrowthWaitLevel(c, trace, 3)
			t.Logf("一只稻草人正常挂机到3级，共%d杀，用时%s", trace.thresholdKills, trace.reached.Sub(trace.started).Round(time.Millisecond))
			c.auto(true, 4)
			naturalGrowthWaitLevel(c, trace, skill.LearnLevel)
			c.auto(false, 4)
			confirmStarterGrowth(t, ctx, store, roleID, trace, &skill)
			beforeLearning, err := store.Role(ctx, roleID)
			must(t, err)
			if !slices.ContainsFunc(beforeLearning.Items, func(item domain.Item) bool {
				return item.ID == domain.Hex(book.InstanceId) && item.Quantity == 1 && item.SourceKind == "initial_grant"
			}) {
				t.Fatal("一级拒绝消耗了未达到学习等级的赠书")
			}
			result := learn()
			if result.ErrorCode != 0 {
				t.Fatalf("自然达到%d级后学习%s失败：%v", skill.LearnLevel, skill.Name, result)
			}
			c.inventoryAt(result.NewRevision)
			learned, err := store.Role(ctx, roleID)
			must(t, err)
			if level, ok := learned.Skill(skill.ID); !ok || level != skill.LearnedLevel ||
				!slices.ContainsFunc(c.inventory.LearnedSkills, func(entry *pb.LearnedSkill) bool {
					return entry.SkillId == skill.ID && entry.Level == skill.LearnedLevel
				}) || slices.ContainsFunc(c.inventory.Items, func(item *pb.ItemView) bool { return bytes.Equal(item.InstanceId, book.InstanceId) }) {
				t.Fatal("初阶习得、背包扣原书和永久角色的记录不一致")
			}
			logStarterResources(t, &f, beforeLearning, &skill)
			battleCount := uint32(4)
			if skill.Behavior == "healing" {
				// 原布衣能挡住初图稻草人的攻击。到正常开放的第二张野外图承受真实一怪伤害。
				c.onFrame = nil // 一级到首个技能的初图观察已结束。
				battleCount = 1
				c.auto(false, battleCount)
				travelToBaseline(c, 2, 6)
				travelAcknowledge(c, false)
			}
			naturalGrowthExerciseSkill(c, &skill, battleCount)
			c.auto(false, battleCount)
			t.Logf("规则%d %s：%s初阶%d已学习并验证正常战斗，达到%d级用时%s，初次结算%s，共%d条已验证的原击杀回执，死亡%d次",
				f.RuleVersion, job.name, skill.Name, skill.LearnedLevel, skill.LearnLevel, trace.reached.Sub(trace.started).Round(time.Millisecond),
				trace.first.Sub(trace.started).Round(time.Millisecond), trace.thresholdKills, trace.deaths)
		})
	}
}

// starterGrowthTrace 保存前期真实经验广播和定点战斗的观察状态。
type starterGrowthTrace struct {
	t              *testing.T
	fixture        *devconfig.Fixture
	key            *pb.RoleKey
	monster        *devconfig.Monster
	grants         []*pb.ExperienceGrantResult
	started        time.Time
	first          time.Time
	reached        time.Time
	thresholdKills int
	origin         *pb.GridPoint
	deaths         int
}

func newStarterGrowthTrace(t *testing.T, fixture *devconfig.Fixture, key *pb.RoleKey, monster *devconfig.Monster) *starterGrowthTrace {
	return &starterGrowthTrace{t: t, fixture: fixture, key: key, monster: monster}
}

// observe 只消费服务端定点位置和经验广播，记录相同来源在真实库中对应的唯一收益。
func (trace *starterGrowthTrace) observe(c *client, frame wire.Frame) {
	if frame.Error != 0 {
		return
	}
	if frame.ID == p.MsgS2CSceneBaseline && trace.origin != nil && !proto.Equal(c.player().GetPosition(), trace.origin) {
		trace.t.Fatalf("定点成长中角色从%v移至%v", trace.origin, c.player().GetPosition())
	}
	if frame.ID != p.MsgS2CExperienceGrantResult {
		return
	}
	var grant pb.ExperienceGrantResult
	must(trace.t, proto.Unmarshal(frame.Payload, &grant))
	// 原来源号还供装备磨损等无经验事件使用，经验序号只要求严格递增。
	var previousSource uint64
	if len(trace.grants) > 0 {
		previousSource = trace.grants[len(trace.grants)-1].SourceSeq
	}
	if !proto.Equal(grant.Role, trace.key) || grant.SourceSeq <= previousSource || grant.RuleVersion != trace.fixture.RuleVersion ||
		grant.CurveVersion != trace.fixture.Progression.CurveVersion || c.inventory.GetRevision() < grant.RoleRevision ||
		grant.RawExperience != trace.monster.Exp || grant.AcceptedExperience != trace.monster.Exp || grant.RejectedExperience != 0 || grant.Migrated {
		trace.t.Fatalf("成长广播的角色、来源、曲线或经验不符合真实原怪：%v", &grant)
	}
	if trace.first.IsZero() {
		trace.first = time.Now()
	}
	trace.grants = append(trace.grants, &grant)
}

// naturalGrowthItem 保留业务版本和实例身份，禁止拿相同模板的其它来路替代赠书。
func naturalGrowthItem(c *client, itemID uint32, source string) *pb.ItemView {
	for _, item := range c.inventory.Items {
		if item.ItemId == itemID && (source == "" || item.SourceKind == source) {
			return proto.Clone(item).(*pb.ItemView)
		}
	}
	return nil
}

// naturalGrowthWaitLevel 按已确认经验等级推进，遇到真实死亡检查原许可并走普通复活。
func naturalGrowthWaitLevel(c *client, trace *starterGrowthTrace, level uint32) {
	c.t.Helper()
	for {
		if len(trace.grants) > 0 && trace.grants[len(trace.grants)-1].LevelAfter >= level {
			trace.reached, trace.thresholdKills = time.Now(), len(trace.grants)
			return
		}
		frame := c.read()
		if frame.ID != p.MsgS2CDeathNotice {
			continue
		}
		trace.deaths++
		c.t.Logf("自然成长期间死亡，当前等级=%d，已确认击杀=%d，复活切回一只怪", c.inventory.Level, len(trace.grants))
		if trace.deaths > 2 || c.death.GetAllowedActions()&1 == 0 {
			c.t.Fatal("新角原装未能在正常复活后继续成长")
		}
		trace.origin = nil // 本次主动复活可能回安全格，后续定点以实际复活坐标为准。
		result := c.asset(p.MsgC2SReviveChoice, &pb.ReviveChoice{DeathId: c.death.DeathId, Action: 1, Operation: c.operation()})
		if result.ErrorCode != 0 {
			c.t.Fatalf("自然死亡后正常复活失败：%v", result)
		}
		c.inventoryAt(result.NewRevision)
		c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().GetHp() > 0 })
		trace.origin = proto.Clone(c.player().Position).(*pb.GridPoint)
		c.auto(true, 1)
	}
}

// confirmStarterGrowth 核实首个学习门槛需要的每条经验都有真实原图、怪物和掉落池回执。
func confirmStarterGrowth(t *testing.T, ctx context.Context, store *persistence.Store, roleID string, trace *starterGrowthTrace, skill *devconfig.Skill) {
	t.Helper()
	needed := int64(0)
	for _, step := range trace.fixture.Progression.ExpPerLevel[:skill.LearnLevel-1] {
		needed += step
	}
	if int64(trace.thresholdKills) != (needed+trace.monster.Exp-1)/trace.monster.Exp {
		t.Fatalf("达到学习门槛的确认击杀数错误：经验门槛=%d，击杀=%d", needed, trace.thresholdKills)
	}
	for _, grant := range trace.grants[:trace.thresholdKills] {
		var record persistence.Operation
		receiptID := fmt.Sprintf("settle:%s:%d", roleID, grant.SourceSeq)
		must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": receiptID}).Decode(&record))
		details := record.Settlement
		if details == nil || details.RuleVersion != trace.fixture.RuleVersion || details.Kills != 1 || details.MonsterID != trace.monster.ID ||
			details.MapID != trace.fixture.Map.ID || details.DropPoolID != trace.monster.DropPoolID || details.Experience == nil ||
			details.Experience.ActualGranted != grant.AcceptedExperience || details.Experience.LevelBefore != grant.LevelBefore || details.Experience.LevelAfter != grant.LevelAfter {
			t.Fatalf("第%d个击杀的Mongo回执与广播不一致：%+v", grant.SourceSeq, details)
		}
	}
}

// logStarterResources 记录首个技能门槛时的自然书页供给和金币现状，供后续经济节奏校验。
func logStarterResources(t *testing.T, fixture *devconfig.Fixture, role domain.Role, skill *devconfig.Skill) {
	var pages, recyclable, copies int64
	for _, item := range role.Items {
		if item.ItemID == fixture.SkillEconomy.PageItemID {
			pages += item.Quantity
		}
		if item.SourceKind != "initial_grant" && item.ItemID == skill.BookItemID {
			copies += item.Quantity
		}
		if row, ok := fixture.BookRecycle(item.ItemID, item.Quality); ok && slices.Contains(row.AllowedSourceKinds, item.SourceKind) {
			recyclable += item.Quantity * row.PagesPerBook
		}
	}
	t.Logf("%s首次学习前：金币=%d、绑定金币=%d、额外本技能书=%d、已有书页=%d、当前合规拆书可得页=%d",
		skill.Name, role.Wallet.Gold, role.Wallet.BoundGold, copies, pages, recyclable)
}

// naturalGrowthExerciseSkill 核实零阶首技走正确的真实战斗入口，剑术随普通攻击，法术手动和自动。
func naturalGrowthExerciseSkill(c *client, skill *devconfig.Skill, battleCount uint32) {
	c.t.Helper()
	if skill.ActivationKind == "passive" {
		c.auto(true, battleCount)
		travelUntilWithin(c, 20*time.Second, func(frame wire.Frame) bool {
			if frame.ID != p.MsgS2CAttackAccepted {
				return false
			}
			var event pb.AttackAccepted
			must(c.t, proto.Unmarshal(frame.Payload, &event))
			return event.CasterHandle == c.entry.PlayerHandle && event.TargetHandle != 0 && event.InputSequence == 0
		})
		return
	}
	var target uint32
	if skill.Behavior == "healing" {
		travelUntilWithin(c, 20*time.Second, func(frame wire.Frame) bool {
			return frame.ID == p.MsgS2CSceneBaseline && c.player().GetHp() > 0 && c.player().GetHp() < c.player().GetHpMax()
		})
		target = c.entry.PlayerHandle
	} else {
		target = m1LiveMonster(c.t, c, skill.RangeCells, nil).EntityHandle
	}
	manual := levelGrowthCastWhenReady(c, skill.ID, target)
	if manual.InputSequence == 0 || manual.SkillLevel != skill.LearnedLevel {
		c.t.Fatalf("初阶手动施法记录错误：%v", manual)
	}
	if impact := c.combatImpact(skill.ID, target); impact.CastId != manual.CastId || impact.Amount <= 0 {
		c.t.Fatalf("初阶手动施法未实际命中或治疗：%v", impact)
	}
	c.auto(true, battleCount)
	travelUntilWithin(c, 25*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CCastAccepted {
			return false
		}
		var event pb.CastAccepted
		must(c.t, proto.Unmarshal(frame.Payload, &event))
		return event.SkillId == skill.ID && event.SkillLevel == skill.LearnedLevel && event.InputSequence == 0
	})
}
