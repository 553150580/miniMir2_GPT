// 通过 MongoDB 和 WebSocket 检查手动蓝药、自动补蓝及离线托管的消耗与状态边界。

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

// TestManaPotionManualAutomaticAndHostingMongoWS 验证手动、在线自动与离线托管的补蓝扣药，核对魔力及库存演进。
// t 提供断言失败报告、子用例和清理注册。
func TestManaPotionManualAutomaticAndHostingMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以验证自动补蓝真实Mongo/WS事务")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)

	// 隔离测试调整耗蓝和怪物耐久，加快真实耗蓝过程。补药与恢复全部沿开发规则。
	// skill 接收技能供后续步骤使用。
	skill, _ := cfg.Fixture.Skill(1)
	// stats 保存由玩家发生时间等级取得的统计或属性，ok 表明查询、校验或类型转换是否成功。
	stats, ok := cfg.Fixture.PlayerAtLevel(skill.JobID, skill.LearnLevel)
	if !ok {
		t.Fatal("未得到法师开发学习等级的属性")
	}
	// i 标记遍历技能集合时访问的下标。
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

	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancel(context.Background())
	// inspection 保存由打开取得的检查。
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	// wg 记录“wg”，使用协程汇合计数器表示。
	var wg sync.WaitGroup
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			// err 接收运行批次过程中可能产生的错误，供后续失败分支处理。
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("补蓝测试%s节点: %v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	// a 接收遍历文本列表时当前的元素。
	for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, a)
	}

	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录，role 保存当前处理所用的角色。
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "mana_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: skill.JobID, Name: "补蓝" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	// roleID 标识当前业务使用的角色。
	roleID := domain.Hex(role.Key.RoleId)
	// roleFilter 初始化用作角色筛选条件的MongoDB BSON 文档。
	roleFilter := bson.M{"_id": roleID}
	// red 建立带初始值的道具，供本段继续处理。
	red := domain.Item{ID: domain.ID(), ItemID: 35, Quantity: 2, Version: 1}
	// locked 初始化用作已锁定的道具。
	locked := domain.Item{ID: domain.ID(), ItemID: 36, Quantity: 3, Version: 1, Locked: true}
	// stack 初始化用作堆叠的道具。
	stack := domain.Item{ID: domain.ID(), ItemID: 36, Quantity: 2, Version: 1}
	// last 初始化用作上一次状态的道具。
	last := domain.Item{ID: domain.ID(), ItemID: 36, Quantity: 1, Version: 1}
	// book 初始化用作技能书的道具。
	book := domain.Item{ID: domain.ID(), ItemID: skill.BookItemID, Quantity: 1, Version: 1}
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, roleFilter, bson.M{"$set": bson.M{"level": skill.LearnLevel, "items": []domain.Item{red, locked, stack, last, book}}})
	must(t, err)

	// checkReceipts 封装“check”回执集合对应的后续处理，c 记录客户端，expected 记录预期结果，使用数值表示，economic 记录经济操作，使用数值表示。
	checkReceipts := func(c *client, expected int, economic int64) domain.Role {
		t.Helper()
		// receipts 记录回执集合，使用操作列表表示。
		var receipts []persistence.Operation
		// cursor 保存由查找取得的游标，err 接收查找过程中可能产生的错误，供后续失败分支处理。
		cursor, err := inspection.DB.Collection("operations").Find(ctx, bson.M{"role_id": roleID, "kind": persistence.AssetAutoUsePotion})
		must(t, err)
		must(t, cursor.All(ctx, &receipts))
		if len(receipts) != expected {
			t.Fatalf("补蓝回执数=%d，期望%d", len(receipts), expected)
		}
		// blueEffect 保存由效果取得的“blue”效果，ok 表明查询、校验或类型转换是否成功。
		blueEffect, ok := cfg.Fixture.Potions.Effect(stack.ItemID)
		// receipt 接收遍历回执集合时当前的元素。
		for _, receipt := range receipts {
			if !ok || receipt.SourceSeq <= 0 || receipt.Potion == nil || receipt.Potion.HP != 0 || receipt.Potion.MP != blueEffect.MP ||
				receipt.Potion.ItemID != stack.ItemID || receipt.Potion.Scene != strconv.FormatUint(c.entry.SceneEpoch, 10) {
				t.Fatalf("补蓝回执额度、来源序列或场景错误: %+v", receipt)
			}
		}
		// audits 保存由数量文档取得的审计记录集合。
		audits, err := inspection.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"role_id": roleID, "kind": persistence.AssetAutoUsePotion})
		must(t, err)
		// persisted 保存由角色取得的“persisted”。
		persisted, err := inspection.Role(ctx, roleID)
		must(t, err)
		if audits != int64(expected) || persisted.EconomicSeq != economic {
			t.Fatalf("审计数或手动经济序列错误: 审计=%d，经济序列=%d", audits, persisted.EconomicSeq)
		}
		return persisted
	}
	// quantity 封装数量对应的后续处理，items 记录道具集合，使用道具列表表示，id 区分当前步骤指向的目标对象。
	quantity := func(items []domain.Item, id string) int64 {
		// item 接收遍历道具集合时当前的元素。
		for _, item := range items {
			if item.ID == id {
				return item.Quantity
			}
		}
		return 0
	}
	// noPotionRPC 封装“no”药水RPC对应的后续处理，c 记录客户端，f 记录协议帧。
	noPotionRPC := func(c *client, f wire.Frame) {
		if f.ID == p.MsgS2COperationResult && f.Request == 0 {
			t.Fatal("自动补蓝伪装成客户端资产请求响应")
		}
	}
	// c 接收加入带有自动技能集合供后续步骤使用。
	c := joinWithAutoSkills(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), &pb.AutoSkillSettings{}, noPotionRPC)
	defer c.close()
	c.auto(false, 1)
	// learn 保存由资产取得的学习。
	learn := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: domain.Bytes(book.ID), SkillId: skill.ID, Operation: c.operation()})
	if learn.ErrorCode != 0 {
		t.Fatal("正常学习火球术失败", learn)
	}
	c.inventoryAt(learn.NewRevision)
	// claim 保存由资产取得的领取。
	claim := c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
	if claim.ErrorCode != 0 {
		t.Fatal("正常领取托管时间失败", claim)
	}
	c.inventoryAt(claim.NewRevision)

	// cast 封装施法对应的后续处理，target 记录目标，使用数值表示。
	cast := func(target uint32) uint32 {
		return c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: skill.ID, TargetHandle: target})
	}
	// waitCast 封装等待施法对应的后续处理，manual 标记是否处于手动状态。
	waitCast := func(manual bool) {
		t.Helper()
		// f 记录协议帧。
		c.until(func(f wire.Frame) bool {
			if f.ID != p.MsgS2CCastAccepted {
				return false
			}
			// accepted 记录已受理，使用施法已受理表示。
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
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().CastSequence == 1 })
	// low 保存魔法值，用于当前步骤中的较低值，hp 保存当前处理所用的生命值。
	low, hp := c.player().Mp, c.player().Hp
	time.Sleep(time.Duration(skill.GlobalSpellMS)*time.Millisecond + 50*time.Millisecond)
	c.expectError(cast(99), p.CodeSkillTarget)
	checkReceipts(c, 0, int64(claim.CommandSeq))
	c.expectError(cast(10), p.CodeSkillResource)
	c.inventoryAt(claim.NewRevision + 1)
	// persisted 保存由“check”回执集合取得的“persisted”。
	persisted := checkReceipts(c, 1, int64(claim.CommandSeq))
	if quantity(persisted.Items, stack.ID) != 1 || quantity(persisted.Items, last.ID) != 1 || quantity(persisted.Items, locked.ID) != 3 || quantity(persisted.Items, red.ID) != 2 {
		t.Fatal("手动施法缺蓝后未消费第一件可用蓝药堆叠一次")
	}
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().Mp > low })
	// firstTick 保存根据基础值“per”模拟帧与整数“64”与等级集合“per”步骤的计算结果计算的第一项模拟帧。
	firstTick := cfg.Fixture.Potions.BasePerTick + int64(skill.LearnLevel)/cfg.Fixture.Potions.LevelsPerStep
	if c.player().Mp != low+firstTick || c.player().Hp != hp || c.player().CastSequence != 1 {
		t.Fatal("补蓝跳过恢复时钟、改变HP或重放了手动施法", c.player())
	}
	// blueEffect 保存由效果取得的“blue”效果。
	blueEffect, _ := cfg.Fixture.Potions.Effect(stack.ItemID)
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().Mp == low+blueEffect.MP })

	// request 保存由发送取得的请求。
	request := c.send(p.MsgC2SAutoBattle, &pb.AutoBattleIntent{SceneEpoch: c.entry.SceneEpoch, Enabled: true, MonsterCount: 1, AutoSkillSettings: &pb.AutoSkillSettings{Enabled: true}})
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CHostingState && f.Request == request })
	waitCast(false)
	c.inventoryAt(claim.NewRevision + 2)
	persisted = checkReceipts(c, 2, int64(claim.CommandSeq))
	if quantity(persisted.Items, stack.ID) != 0 || quantity(persisted.Items, last.ID) != 1 {
		t.Fatal("在线自动没有用完同一可用堆叠，或多消费了其他实例")
	}
	waitCast(false)
	hostingBefore := persisted.HostingSeconds
	c.close()
	// deadline 保存由新增取得的截止时间。
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		// count 保存由数量文档取得的数量，err 接收数量文档过程中可能产生的错误，供后续失败分支处理。
		count, err := inspection.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": roleID, "kind": persistence.AssetAutoUsePotion})
		must(t, err)
		if count == 3 {
			// 自动用药与托管检查点分别提交，等待两项真实持久结果。
			role, err := inspection.Role(ctx, roleID)
			must(t, err)
			if role.HostingSeconds < hostingBefore {
				break
			}
		}
		time.Sleep(80 * time.Millisecond)
	}
	persisted = checkReceipts(c, 3, int64(claim.CommandSeq))
	if quantity(persisted.Items, last.ID) != 0 || quantity(persisted.Items, locked.ID) != 3 || quantity(persisted.Items, red.ID) != 2 || persisted.HostingSeconds >= hostingBefore {
		t.Fatalf("离线托管用药或扣时未确认：蓝药=%d 锁定蓝药=%d 红药=%d，托管=%d→%d",
			quantity(persisted.Items, last.ID), quantity(persisted.Items, locked.ID), quantity(persisted.Items, red.ID), hostingBefore, persisted.HostingSeconds)
	}

	// resumed 保存由加入带有自动技能集合取得的“resumed”。
	resumed := joinWithAutoSkills(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), &pb.AutoSkillSettings{}, noPotionRPC)
	defer resumed.close()
	if resumed.entry.SceneEpoch != c.entry.SceneEpoch || resumed.entry.NextEconomicCommandSeq != claim.CommandSeq+1 || resumed.player().Hp != hp {
		t.Fatal("健康重连重置了现场或更改了手动经济水位")
	}
	resumed.auto(false, 1)
	checkReceipts(resumed, 3, int64(claim.CommandSeq))
	t.Log("真实WS验证手动缺蓝、在线自动和离线托管共用原蓝药事务、堆叠及分跳，健康重连无重复消费")
}
