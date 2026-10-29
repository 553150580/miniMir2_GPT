package integration

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func TestLevelGrowthMigrationCapReceiptsAndEquipmentMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo成长/迁移事务")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion != 21 {
		t.Fatal("必须使用当前rule21完整成长配置")
	}
	// 仅隔离定义把原衣服门槛调到7以验证真正事务门禁，不向主目录增加装备。
	seven := uint32(7)
	for i := range cfg.Fixture.Items {
		if cfg.Fixture.Items[i].ID == 193 {
			cfg.Fixture.Items[i].RequiredLevel = &seven
		}
	}
	must(t, cfg.Fixture.Validate())
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	ctx := context.Background()
	authority, err := persistence.Open(ctx, cfg)
	must(t, err)
	owner, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, authority.DB.Drop(cleanup))
		owner.Close(cleanup)
		authority.Close(cleanup)
	})
	must(t, authority.Singleton(ctx, "account"))
	grant, err := authority.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
	must(t, err)
	owner.Epoch = grant.Epoch
	login, err := authority.Login(ctx, "growth_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	account, err := authority.Authenticate(ctx, login.AccountToken)
	must(t, err)
	create := func(slot uint32) *pb.RoleSummary {
		r, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: 0, Name: "成长事务" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())})
		must(t, err)
		return r
	}
	enter := func(key *pb.RoleKey) (domain.Role, persistence.Auth) {
		ticket, err := authority.IssueTicket(ctx, account, key)
		must(t, err)
		r, a, err := owner.Enter(ctx, ticket.Ticket)
		must(t, err)
		return r, a
	}
	key := create(1)
	r, auth := enter(key.Key)
	if r.ProgressionFormatVersion != 1 || r.ProgressionCurveVersion != cfg.Fixture.Progression.CurveVersion || r.Level != 1 {
		t.Fatal("新生未直接初始化format1")
	}
	cloth := r.Items[2]
	asset := func(q persistence.AssetRequest) (*pb.OperationResult, persistence.Operation) {
		q.Context = &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(r.EconomicSeq + 1), ExpectedRevision: uint64(r.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}
		raw, _ := json.Marshal(q)
		q.Fingerprint = persistence.Hash(raw)
		out, result, op, err := owner.AssetWithReceipt(ctx, auth, q)
		must(t, err)
		r = out
		return result, op
	}
	before := r
	result, _ := asset(persistence.AssetRequest{Kind: "equip", ItemID: cloth.ID, Slot: 1})
	if result.ErrorCode == 0 || !reflect.DeepEqual(r.Items, before.Items) || len(r.Equipped) != 0 || r.SourceSeq != before.SourceSeq {
		t.Fatal("未到装备来源需求等级仍穿戴或部分移物")
	}
	toSeven := int64(0)
	for _, xp := range cfg.Fixture.Progression.ExpPerLevel[:6] {
		toSeven += xp
	}
	r, op, err := owner.SettleWithReceipt(ctx, auth, persistence.Settlement{SourceSeq: r.SourceSeq + 1, Experience: toSeven})
	must(t, err)
	if r.Level != 7 || r.Experience != 0 || op.Settlement.Experience.ActualGranted != toSeven {
		t.Fatal("明确边界奖励未按主曲线到7")
	}
	result, _ = asset(persistence.AssetRequest{Kind: "equip", ItemID: cloth.ID, Slot: 1})
	if result.ErrorCode != 0 || len(r.Equipped) != 1 || r.Equipped[0].Item.ID != cloth.ID {
		t.Fatal("达到真实需求等级后未正常穿戴")
	}
	// 独立满级资格；这是cap事务边界，不冒充自然1→50已经耗时验证。
	poolCap := cfg.Fixture.Progression.ExpPerLevel[49]
	_, err = owner.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$set": bson.M{"level": 50, "experience": int64(0), "pending_experience": poolCap - 5}})
	must(t, err)
	r, err = owner.Role(ctx, r.ID)
	must(t, err)
	before = r
	item := domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1, Bound: true, SourceKind: "explicit_boundary_reward"}
	q := persistence.Settlement{SourceSeq: r.SourceSeq + 1, Kills: 1, Experience: 15, Gold: 30, Bound: true, Item: &item, HostingSession: domain.ID()}
	r, op, err = owner.SettleWithReceipt(ctx, auth, q)
	must(t, err)
	if r.Level != 50 || r.PendingExperience != poolCap || r.Experience != 0 || len(r.Items) != len(before.Items)+1 || r.Wallet.BoundGold != before.Wallet.BoundGold+30 || r.Report.Experience != 5 || r.Report.RawExperience != 15 || r.Report.PooledExperience != 5 || r.Report.RejectedExperience != 10 || op.Settlement.Experience.ActualGranted != 5 {
		t.Fatal("池边界未原子分账或阻止其它奖励", r, op)
	}
	replayed, replayOp, err := owner.SettleWithReceipt(ctx, auth, q)
	must(t, err)
	if replayed.Revision != r.Revision || replayed.Wallet != r.Wallet || !reflect.DeepEqual(replayOp.Settlement.Experience, op.Settlement.Experience) {
		t.Fatal("原Source重查重复发放或更换实际grant")
	}
	q.SourceSeq++
	q.Experience = math.MaxInt64
	q.Item = nil
	q.HostingSession = ""
	r, op, err = owner.SettleWithReceipt(ctx, auth, q)
	must(t, err)
	if op.Settlement.Experience.ActualGranted != 0 || op.Settlement.Experience.Rejected != math.MaxInt64 || r.Wallet.BoundGold != before.Wallet.BoundGold+60 || r.PendingExperience != poolCap {
		t.Fatal("满池大raw导致其它收益停止/整数溢出")
	}
	legacy := create(2)
	legacyBalance := poolCap + 500
	for _, xp := range cfg.Fixture.Progression.ExpPerLevel[20:49] {
		legacyBalance += xp
	}
	_, err = owner.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(legacy.Key.RoleId)}, bson.M{"$unset": bson.M{"progression_format_version": "", "progression_curve_version": ""}, "$set": bson.M{"level": 21, "experience": legacyBalance}})
	must(t, err)
	old, oldAuth := enter(legacy.Key)
	if old.Level != 50 || old.PendingExperience != poolCap || old.LegacyExperienceOverflow != 500 || old.Experience != 0 || old.SourceSeq != 0 || old.EconomicSeq != 0 || old.RebirthCount != 0 {
		t.Fatal("旧已确认余额迁移丢失或冒称转生", old)
	}
	var migration persistence.Operation
	must(t, owner.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "progression_migration:" + old.ID + ":1"}).Decode(&migration))
	if migration.Experience == nil || migration.Experience.ActualGranted != 0 || migration.Experience.MigrationBefore.Level != 21 || migration.Experience.MigrationBefore.Experience != legacyBalance || migration.Experience.MigrationAfter.LegacyExperienceOverflow != 500 {
		t.Fatal("缺原迁移事实回执", migration)
	}
	again, _ := enter(legacy.Key)
	if again.PendingExperience != old.PendingExperience || again.LegacyExperienceOverflow != 500 || again.Level != 50 {
		t.Fatal("新票重复迁移旧余额")
	}
	if _, _, err := owner.SettleWithReceipt(ctx, oldAuth, persistence.Settlement{SourceSeq: 1, Gold: 1}); persistence.Code(err) != p.CodeStaleSession {
		t.Fatal("旧附着仍可提交成长", err)
	}
	high := create(3)
	_, err = owner.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(high.Key.RoleId)}, bson.M{"$unset": bson.M{"progression_format_version": "", "progression_curve_version": ""}, "$set": bson.M{"level": 99, "experience": int64(math.MaxInt64)}})
	must(t, err)
	highRole, highAuth := enter(high.Key)
	if highRole.Level != 99 || highRole.RebirthCount != 0 || !highRole.ProgressionMigrationRequired || highRole.LegacyExperienceOverflow != math.MaxInt64 {
		t.Fatal("旧越目录等级被降级或伪授转生")
	}
	highRole, op, err = owner.SettleWithReceipt(ctx, highAuth, persistence.Settlement{SourceSeq: 1, Experience: 15, Gold: 30})
	must(t, err)
	if highRole.Level != 99 || op.Settlement.Experience.ActualGranted != 0 || highRole.Wallet.Gold != 30 {
		t.Fatal("需迁移余额阻断其它收益")
	}
	t.Log("真实Mongo：新生format1、原装备等级门槛、实际grant/池/拒收、满池仍金币物品、旧21余额无损封存、旧99保级、原Source与新票防重")
}

func TestLevelGrowthNaturalStarterLearningRecoveryAndReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo/WS自然新手成长")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion != 21 {
		t.Fatal("必须是当前主rule21")
	}
	// 明确加速战斗资格，仅HP1/零攻击/零敏；经验15、主曲线、动作/刷新/掉率/保底均保持。
	cfg.Fixture.Monsters[0].HP = 1
	cfg.Fixture.Monsters[0].AttackMin = 0
	cfg.Fixture.Monsters[0].AttackMax = 0
	cfg.Fixture.Monsters[0].Agility = 0
	must(t, cfg.Fixture.Validate())
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	ctx, cancel := context.WithCancel(context.Background())
	var group sync.WaitGroup
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	for _, mode := range []string{"account", "game", "gm"} {
		group.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("成长%s节点:%v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		group.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, a)
	}
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	role := &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "growth_ws_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 1, Name: "自然法师" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, role, 0)
	var grants []*pb.ExperienceGrantResult
	observe := func(c *client, f wire.Frame) {
		if f.Error == 0 && f.ID == p.MsgS2CExperienceGrantResult {
			var g pb.ExperienceGrantResult
			must(t, proto.Unmarshal(f.Payload, &g))
			if c.inventory == nil || c.inventory.Revision < g.RoleRevision || c.inventory.Progression == nil || c.inventory.Progression.CurveVersion != g.CurveVersion || g.RawExperience != 15 || g.AcceptedExperience != 15 || g.RejectedExperience != 0 || g.Migrated {
				t.Fatal("原成长结果早于完整Inventory或错误混入迁移/改raw", &g)
			}
			if g.LevelAfter > g.LevelBefore && (c.player().Hp != c.player().HpMax || c.player().Mp != c.player().MpMax) {
				t.Fatal("真正新奖励升级未恢复原身体", &g, c.player())
			}
			grants = append(grants, &g)
		}
	}
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), observe)
	defer func() { c.close() }()
	c.auto(false, 8)
	if c.inventory.Level != 1 || c.inventory.Progression.FormatVersion != 1 || c.inventory.Progression.LevelCap != 50 {
		t.Fatal("新手正常入场成长状态错误")
	}
	var wood, book *pb.ItemView
	for _, i := range c.inventory.Items {
		if i.ItemId == 218 && wood == nil {
			wood = i
		}
		if i.ItemId == 7 {
			book = i
		}
	}
	if wood == nil || book == nil {
		t.Fatal("初始原武器/原职业书缺失")
	}
	res := c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: wood.InstanceId, Slot: 0, Operation: c.operation()})
	if res.ErrorCode != 0 {
		t.Fatal(res)
	}
	c.inventoryAt(res.NewRevision)
	c.auto(true, 8)
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CExperienceGrantResult && c.inventory.Level >= 7 })
	c.auto(false, 8)
	res = c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: book.InstanceId, SkillId: 1, Operation: c.operation()})
	if res.ErrorCode != 0 {
		t.Fatal("实际自然7级未正常学原书", res)
	}
	c.inventoryAt(res.NewRevision)
	var target *pb.EntityState
	for target == nil {
		for _, e := range c.scene.Entities {
			if e.EntityKind == 3 && e.Hp > 0 {
				target = e
				break
			}
		}
		if target == nil {
			c.read()
		}
	}
	mpBefore := c.player().Mp
	levelGrowthCastWhenReady(c, 1, target.EntityHandle)
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().Mp < mpBefore })
	c.auto(true, 8)
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CExperienceGrantResult && c.inventory.Level >= 8 })
	c.auto(false, 8)
	oldBody := proto.Clone(c.player()).(*pb.EntityState)
	before, err := inspection.Role(ctx, domain.Hex(role.Key.RoleId))
	must(t, err)
	seen := len(grants)
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), observe)
	c.auto(false, 8)
	after, err := inspection.Role(ctx, before.ID)
	must(t, err)
	if c.player().Life != oldBody.Life || c.inventory.Progression.CurveVersion != cfg.Fixture.Progression.CurveVersion || c.inventory.Level < 8 {
		t.Fatal("健康重入重建身体或丢永久成长")
	}
	if after.SourceSeq == before.SourceSeq && (c.player().Hp != oldBody.Hp || c.player().Mp != oldBody.Mp) {
		t.Fatal("无新来源事件的健康重入改变HP/MP")
	}
	for _, g := range grants[seen:] {
		if g.SourceSeq <= uint64(before.SourceSeq) {
			t.Fatal("入场补播历史成长")
		}
	}
	t.Logf("真实WS加速战斗夹具：自然1→%d、原书学习/耗MP→新奖励升级恢复、健康票保留；SourceSeq=%d，原曲线=%s，非正常主怪效率测量", after.Level, after.SourceSeq, after.ProgressionCurveVersion)
}

// 关闭Auto只停止新增自动意图，原移动/普通攻击锁仍按真实时钟结束。
// 仅合法StateBlocked等待下一权威baseline后重试；不改时限、不私写现场或固定sleep。
func levelGrowthCastWhenReady(c *client, skill, target uint32) *pb.CastAccepted {
	c.t.Helper()
	for {
		if c.hosting == nil || c.hosting.AutoBattleEnabled {
			c.t.Fatal("手动施法前尚未权威关闭Auto")
		}
		beforeMP, beforeCast, sequence := c.player().Mp, c.player().CastSequence, c.scene.StateSequence
		request := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: skill, TargetHandle: target})
		blocked := false
		for {
			f := c.readFrame()
			if f.Error != 0 {
				if f.ID != p.MsgC2SCastIntent || f.Request != request || f.Error != p.CodeSkillStateBlocked {
					c.t.Fatalf("成长手动施法非预期错误:%v", f)
				}
				c.t.Logf("正常施法等待原动作锁: request=%d scene=%d baselineSequence=%d HP=%d MP=%d attackSequence=%d castSequence=%d", request, c.entry.SceneEpoch, c.scene.StateSequence, c.player().Hp, c.player().Mp, c.player().AttackSequence, c.player().CastSequence)
				blocked = true
				break
			}
			if f.ID == p.MsgS2CCastAccepted {
				var accepted pb.CastAccepted
				must(c.t, proto.Unmarshal(f.Payload, &accepted))
				if accepted.SkillId == skill {
					return &accepted
				}
			}
		}
		if blocked {
			c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.scene.StateSequence > sequence })
			if c.player().Mp != beforeMP || c.player().CastSequence != beforeCast {
				c.t.Fatal("StateBlocked仍扣MP或创建施法")
			}
		}
	}
}
