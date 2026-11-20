package integration

import (
	"context"
	"os"
	"strconv"
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

func TestManaPotionManualAutomaticAndHostingMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以验证自动补蓝真实Mongo/WS事务")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)

	// 隔离测试调整耗蓝和怪物耐久，加快真实耗蓝过程。补药与恢复全部沿开发规则。
	skill, _ := cfg.Fixture.Skill(1)
	stats, ok := cfg.Fixture.PlayerAtLevel(skill.JobID, skill.LearnLevel)
	if !ok {
		t.Fatal("未得到法师开发学习等级的属性")
	}
	for i := range cfg.Fixture.Skills {
		if cfg.Fixture.Skills[i].ID == skill.ID {
			cfg.Fixture.Skills[i].MPBaseCost = (stats.MP+1)/2 + 2
			skill = cfg.Fixture.Skills[i]
			break
		}
	}
	cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: 8, Y: 5}
	cfg.Fixture.Monsters[0].HP = 10000
	cfg.Fixture.Monsters[0].AttackMin, cfg.Fixture.Monsters[0].AttackMax = 0, 0

	ctx, cancel := context.WithCancel(context.Background())
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	var wg sync.WaitGroup
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("补蓝测试%s节点: %v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, a)
	}

	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "mana_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: skill.JobID, Name: "补蓝" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	roleFilter := bson.M{"_id": roleID}
	red := domain.Item{ID: domain.ID(), ItemID: 35, Quantity: 2, Version: 1}
	locked := domain.Item{ID: domain.ID(), ItemID: 36, Quantity: 3, Version: 1, Locked: true}
	stack := domain.Item{ID: domain.ID(), ItemID: 36, Quantity: 2, Version: 1}
	last := domain.Item{ID: domain.ID(), ItemID: 36, Quantity: 1, Version: 1}
	book := domain.Item{ID: domain.ID(), ItemID: skill.BookItemID, Quantity: 1, Version: 1}
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, roleFilter, bson.M{"$set": bson.M{"level": skill.LearnLevel, "items": []domain.Item{red, locked, stack, last, book}}})
	must(t, err)

	checkReceipts := func(c *client, expected int, economic int64) domain.Role {
		t.Helper()
		var receipts []persistence.Operation
		cursor, err := inspection.DB.Collection("operations").Find(ctx, bson.M{"role_id": roleID, "kind": persistence.AssetAutoUsePotion})
		must(t, err)
		must(t, cursor.All(ctx, &receipts))
		if len(receipts) != expected {
			t.Fatalf("补蓝回执数=%d，期望%d", len(receipts), expected)
		}
		blueEffect, ok := cfg.Fixture.Potions.Effect(stack.ItemID)
		for _, receipt := range receipts {
			if !ok || receipt.SourceSeq <= 0 || receipt.Potion == nil || receipt.Potion.HP != 0 || receipt.Potion.MP != blueEffect.MP ||
				receipt.Potion.ItemID != stack.ItemID || receipt.Potion.Scene != strconv.FormatUint(c.entry.SceneEpoch, 10) {
				t.Fatalf("补蓝回执额度、来源序列或场景错误: %+v", receipt)
			}
		}
		audits, err := inspection.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"role_id": roleID, "kind": persistence.AssetAutoUsePotion})
		must(t, err)
		persisted, err := inspection.Role(ctx, roleID)
		must(t, err)
		if audits != int64(expected) || persisted.EconomicSeq != economic {
			t.Fatalf("审计数或手动经济序列错误: 审计=%d，经济序列=%d", audits, persisted.EconomicSeq)
		}
		return persisted
	}
	quantity := func(items []domain.Item, id string) int64 {
		for _, item := range items {
			if item.ID == id {
				return item.Quantity
			}
		}
		return 0
	}
	noPotionRPC := func(c *client, f wire.Frame) {
		if f.ID == p.MsgS2COperationResult && f.Request == 0 {
			t.Fatal("自动补蓝伪装成客户端资产请求响应")
		}
	}
	c := joinWithAutoSkills(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), &pb.AutoSkillSettings{}, noPotionRPC)
	defer c.close()
	c.auto(false, 1)
	learn := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: domain.Bytes(book.ID), SkillId: skill.ID, Operation: c.operation()})
	if learn.ErrorCode != 0 {
		t.Fatal("正常学习火球术失败", learn)
	}
	c.inventoryAt(learn.NewRevision)
	claim := c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
	if claim.ErrorCode != 0 {
		t.Fatal("正常领取托管时间失败", claim)
	}
	c.inventoryAt(claim.NewRevision)

	cast := func(target uint32) uint32 {
		return c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: skill.ID, TargetHandle: target})
	}
	waitCast := func(manual bool) {
		t.Helper()
		c.until(func(f wire.Frame) bool {
			if f.ID != p.MsgS2CCastAccepted {
				return false
			}
			var accepted pb.CastAccepted
			must(t, proto.Unmarshal(f.Payload, &accepted))
			if accepted.SkillId != skill.ID || accepted.TargetHandle != 10 || (accepted.InputSequence > 0) != manual {
				t.Fatal("恢复后施法归属或目标错误", &accepted)
			}
			return true
		})
	}
	cast(10)
	waitCast(true)
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().CastSequence == 1 })
	low, hp := c.player().Mp, c.player().Hp
	time.Sleep(time.Duration(skill.GlobalSpellMS)*time.Millisecond + 50*time.Millisecond)
	c.expectError(cast(99), p.CodeSkillTarget)
	checkReceipts(c, 0, int64(claim.CommandSeq))
	c.expectError(cast(10), p.CodeSkillResource)
	c.inventoryAt(claim.NewRevision + 1)
	persisted := checkReceipts(c, 1, int64(claim.CommandSeq))
	if quantity(persisted.Items, stack.ID) != 1 || quantity(persisted.Items, last.ID) != 1 || quantity(persisted.Items, locked.ID) != 3 || quantity(persisted.Items, red.ID) != 2 {
		t.Fatal("手动施法缺蓝后未消费第一件可用蓝药堆叠一次")
	}
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().Mp > low })
	firstTick := cfg.Fixture.Potions.BasePerTick + int64(skill.LearnLevel)/cfg.Fixture.Potions.LevelsPerStep
	if c.player().Mp != low+firstTick || c.player().Hp != hp || c.player().CastSequence != 1 {
		t.Fatal("补蓝跳过恢复时钟、改变HP或重放了手动施法", c.player())
	}
	blueEffect, _ := cfg.Fixture.Potions.Effect(stack.ItemID)
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().Mp == low+blueEffect.MP })

	request := c.send(p.MsgC2SAutoBattle, &pb.AutoBattleIntent{SceneEpoch: c.entry.SceneEpoch, Enabled: true, MonsterCount: 1, AutoSkillSettings: &pb.AutoSkillSettings{Enabled: true}})
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CHostingState && f.Request == request })
	waitCast(false)
	c.inventoryAt(claim.NewRevision + 2)
	persisted = checkReceipts(c, 2, int64(claim.CommandSeq))
	if quantity(persisted.Items, stack.ID) != 0 || quantity(persisted.Items, last.ID) != 1 {
		t.Fatal("在线自动没有用完同一可用堆叠，或多消费了其他实例")
	}
	waitCast(false)
	c.close()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		count, err := inspection.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": roleID, "kind": persistence.AssetAutoUsePotion})
		must(t, err)
		if count == 3 {
			break
		}
		time.Sleep(80 * time.Millisecond)
	}
	persisted = checkReceipts(c, 3, int64(claim.CommandSeq))
	if quantity(persisted.Items, last.ID) != 0 || quantity(persisted.Items, locked.ID) != 3 || quantity(persisted.Items, red.ID) != 2 || persisted.HostingSeconds >= 21600 {
		t.Fatal("离线托管没有消费下一件可用蓝药，误扣红药或锁定物品，或没进入真实托管")
	}

	resumed := joinWithAutoSkills(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), &pb.AutoSkillSettings{}, noPotionRPC)
	defer resumed.close()
	if resumed.entry.SceneEpoch != c.entry.SceneEpoch || resumed.entry.NextEconomicCommandSeq != claim.CommandSeq+1 || resumed.player().Hp != hp {
		t.Fatal("健康重连重置了现场或更改了手动经济水位")
	}
	resumed.auto(false, 1)
	checkReceipts(resumed, 3, int64(claim.CommandSeq))
	t.Log("真实WS验证手动缺蓝、在线自动和离线托管共用原蓝药事务、堆叠及分跳，健康重连无重复消费")
}
