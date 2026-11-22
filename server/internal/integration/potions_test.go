// 验证真实药水操作回执、库存更新以及新场景对已确认药效的协调。

package integration

import (
	"context"
	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestPotionMongoWSReceiptAndFreshSceneReconciliation 验证用药库存与原场景回执的一致性，以及冷场景对旧效果的处理。
// t 提供断言失败报告、子用例和清理注册。
func TestPotionMongoWSReceiptAndFreshSceneReconciliation(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以验证药品真实Mongo/WS事务")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// inspection 保存由打开取得的检查。
	inspection, err := persistence.Open(context.Background(), cfg)
	must(t, err)
	// launch 封装启动对应的后续处理。
	launch := func() func() {
		// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		ctx, cancel := context.WithCancel(context.Background())
		// wg 记录“wg”，使用协程汇合计数器表示。
		var wg sync.WaitGroup
		// mode 接收遍历文本列表时当前的元素。
		for _, mode := range []string{"account", "game", "gm"} {
			wg.Go(func() {
				// err 接收运行批次过程中可能产生的错误，供后续失败分支处理。
				if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
					t.Errorf("药品测试%s节点: %v", mode, err)
				}
			})
		}
		// address 接收遍历文本列表时当前的元素。
		for _, address := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
			healthy(t, address)
		}
		return func() { cancel(); wg.Wait() }
	}
	// stop 保存由启动取得的停止。
	stop := launch()
	t.Cleanup(func() {
		stop()
		// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, inspection.DB.Drop(ctx))
		inspection.Close(ctx)
	})
	// accountURL 保存根据当前数据与账号地址的计算结果与当前数据计算的账号URL。
	accountURL := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, accountURL, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "potion_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	// role 保存当前处理所用的角色。
	role := &pb.RoleSummary{}
	rpc(t, accountURL, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 0, Name: "药品事务" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, role, 0)
	// roleID 标识当前业务使用的角色。
	roleID := domain.Hex(role.Key.RoleId)
	// red 建立带初始值的道具，供本段继续处理，blue 建立带初始值的道具，供本段继续处理。
	red, blue := domain.Item{ID: domain.ID(), ItemID: 35, Quantity: 4, Version: 1, Bound: true}, domain.Item{ID: domain.ID(), ItemID: 36, Quantity: 4, Version: 1}
	// 仅在本用例随机测试库构造堆叠前态，不触碰开发角色或来源数据库。
	_, err = inspection.DB.Collection("roles").UpdateOne(context.Background(), bson.M{"_id": roleID}, bson.M{"$set": bson.M{"items": []domain.Item{red, blue}}})
	must(t, err)
	// c 接收加入供后续步骤使用。
	c := join(t, cfg.GameAddress, issue(t, accountURL, login.AccountToken, role.Key))
	defer func() { c.close() }()
	c.auto(false, 1)
	if c.entry.ResourceVersion != cfg.Fixture.RuleVersion {
		t.Fatal("入场未返回当前药品规则版本")
	}
	// quantity 封装数量对应的后续处理，id 区分当前步骤指向的目标对象。
	quantity := func(id string) int64 {
		// item 接收遍历道具集合时当前的元素。
		for _, item := range c.inventory.Items {
			if domain.Hex(item.InstanceId) == id {
				return item.Quantity
			}
		}
		return 0
	}
	// use 封装使用对应的后续处理，q 记录使用道具请求。
	use := func(q *pb.UseItemRequest) *pb.OperationResult {
		// result 保存由资产取得的结果。
		result := c.asset(p.MsgC2SUseItem, q)
		c.inventoryAt(result.NewRevision)
		return result
	}
	// original 保存当前处理所用的原始。
	original := &pb.UseItemRequest{ItemInstanceId: domain.Bytes(red.ID), ItemId: red.ItemID, SceneEpoch: c.entry.SceneEpoch, Operation: c.operation()}
	// first 保存由使用取得的第一项。
	first := use(original)
	if first.ErrorCode != 0 || quantity(red.ID) != 3 {
		t.Fatalf("满HP药品没有原子减一:%v", first)
	}
	// 跳过共享节流，再消费蓝药；此时红药回执的原序号已落后于当前水位。
	time.Sleep(time.Duration(cfg.Fixture.Potions.UseDelayMS+cfg.Fixture.Potions.TickMS) * time.Millisecond)
	// blueUse 保存当前处理所用的“blue”使用。
	blueUse := &pb.UseItemRequest{ItemInstanceId: domain.Bytes(blue.ID), ItemId: blue.ItemID, SceneEpoch: c.entry.SceneEpoch, Operation: c.operation()}
	// result 保存由使用取得的结果。
	if result := use(blueUse); result.ErrorCode != 0 || quantity(blue.ID) != 3 {
		t.Fatalf("满MP蓝药没有被正确消费:%v", result)
	}
	// replayed 保存由使用取得的“replayed”。
	replayed := use(original)
	if replayed.ErrorCode != 0 || quantity(red.ID) != 3 || replayed.CommandSeq <= first.CommandSeq {
		t.Fatal("原药品重传重复扣除，或没有返回当前经济水位")
	}
	// receipt 记录回执，使用操作表示。
	var receipt persistence.Operation
	// opKey 保存根据当前数据与角色标识的计算结果与当前数据的计算结果与十六进制计算的操作键。
	opKey := "asset:" + roleID + ":" + domain.Hex(original.Operation.OpId)
	must(t, inspection.DB.Collection("operations").FindOne(context.Background(), bson.M{"_id": opKey}).Decode(&receipt))
	if receipt.CommandSeq != int64(first.CommandSeq) || receipt.Potion == nil || receipt.Potion.Scene != strconv.FormatUint(original.SceneEpoch, 10) {
		t.Fatal("没有保留首次提交原序号与原场景消费回执")
	}
	// count 保存由数量文档取得的数量。
	count, err := inspection.DB.Collection("audit_outbox").CountDocuments(context.Background(), bson.M{"_id": opKey})
	must(t, err)
	if count != 1 {
		t.Fatal("药品重传产生了重复审计事件")
	}
	time.Sleep(time.Duration(cfg.Fixture.Potions.UseDelayMS) * time.Millisecond)
	// wrongType 保存当前处理所用的“wrong”类型。
	wrongType := &pb.UseItemRequest{ItemInstanceId: domain.Bytes(red.ID), ItemId: blue.ItemID, SceneEpoch: c.entry.SceneEpoch, Operation: c.operation()}
	// result 保存由使用取得的结果。
	if result := use(wrongType); result.ErrorCode != p.CodeInvalidArgument || quantity(red.ID) != 3 || quantity(blue.ID) != 3 {
		t.Fatal("物品实例与类型不符未被拒绝，或失败消耗了库存")
	}
	c.close()
	stop()
	stop = launch()
	c = join(t, cfg.GameAddress, issue(t, accountURL, login.AccountToken, role.Key))
	c.auto(false, 1)
	if c.entry.SceneEpoch == original.SceneEpoch {
		t.Fatal("测试节点重启后未建立新场景")
	}
	// result 保存由使用取得的结果。
	if result := use(original); result.ErrorCode != 0 || quantity(red.ID) != 3 {
		t.Fatal("旧场景已提交药品无法沿原身份核对，或再次扣药")
	}
	// newStale 保存当前处理所用的创建已失效。
	newStale := &pb.UseItemRequest{ItemInstanceId: domain.Bytes(red.ID), ItemId: red.ItemID, SceneEpoch: original.SceneEpoch, Operation: c.operation()}
	// result 保存由使用取得的结果。
	if result := use(newStale); result.ErrorCode != p.CodeStaleSession || quantity(red.ID) != 3 {
		t.Fatal("旧scene的新操作未被拒绝或消耗库存")
	}
	// document 记录文档，使用MongoDB BSON 文档表示。
	var document bson.M
	must(t, inspection.DB.Collection("roles").FindOne(context.Background(), bson.M{"_id": roleID}).Decode(&document))
	// key 接收遍历文本列表时当前的元素。
	for _, key := range []string{"hp", "mp", "position", "cooldowns", "potions", "potion_queue", "next_potion_use"} {
		// exists 保存从文档取出的存在。
		if _, exists := document[key]; exists {
			t.Fatalf("药品临时现场%s被写入角色文档", key)
		}
	}
	t.Logf("通过：药品满值消费、库存减一、原回执防重/审计、类型拒绝、节点重启后原scene对账，规则版本%d", cfg.Fixture.RuleVersion)
}
