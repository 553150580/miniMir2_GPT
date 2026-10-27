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

func TestPotionMongoWSReceiptAndFreshSceneReconciliation(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以验证药品真实Mongo/WS事务")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	inspection, err := persistence.Open(context.Background(), cfg)
	must(t, err)
	launch := func() func() {
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		for _, mode := range []string{"account", "game", "gm"} {
			wg.Go(func() {
				if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
					t.Errorf("药品测试%s节点: %v", mode, err)
				}
			})
		}
		for _, address := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
			healthy(t, address)
		}
		return func() { cancel(); wg.Wait() }
	}
	stop := launch()
	t.Cleanup(func() {
		stop()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, inspection.DB.Drop(ctx))
		inspection.Close(ctx)
	})
	accountURL := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, accountURL, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "potion_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	role := &pb.RoleSummary{}
	rpc(t, accountURL, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 0, Name: "药品事务" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	red, blue := domain.Item{ID: domain.ID(), ItemID: 35, Quantity: 4, Version: 1, Bound: true}, domain.Item{ID: domain.ID(), ItemID: 36, Quantity: 4, Version: 1}
	// 仅在本用例随机测试库构造堆叠前态，不触碰开发角色或来源数据库。
	_, err = inspection.DB.Collection("roles").UpdateOne(context.Background(), bson.M{"_id": roleID}, bson.M{"$set": bson.M{"items": []domain.Item{red, blue}}})
	must(t, err)
	c := join(t, cfg.GameAddress, issue(t, accountURL, login.AccountToken, role.Key))
	defer func() { c.close() }()
	c.auto(false, 1)
	if c.entry.ResourceVersion != cfg.Fixture.RuleVersion {
		t.Fatal("入场未返回当前药品规则版本")
	}
	quantity := func(id string) int64 {
		for _, item := range c.inventory.Items {
			if domain.Hex(item.InstanceId) == id {
				return item.Quantity
			}
		}
		return 0
	}
	use := func(q *pb.UseItemRequest) *pb.OperationResult {
		result := c.asset(p.MsgC2SUseItem, q)
		c.inventoryAt(result.NewRevision)
		return result
	}
	original := &pb.UseItemRequest{ItemInstanceId: domain.Bytes(red.ID), ItemId: red.ItemID, SceneEpoch: c.entry.SceneEpoch, Operation: c.operation()}
	first := use(original)
	if first.ErrorCode != 0 || quantity(red.ID) != 3 {
		t.Fatalf("满HP药品没有原子减一:%v", first)
	}
	// 跳过共享节流，再消费蓝药；此时红药回执的原序号已落后于当前水位。
	time.Sleep(time.Duration(cfg.Fixture.Potions.UseDelayMS+cfg.Fixture.Potions.TickMS) * time.Millisecond)
	blueUse := &pb.UseItemRequest{ItemInstanceId: domain.Bytes(blue.ID), ItemId: blue.ItemID, SceneEpoch: c.entry.SceneEpoch, Operation: c.operation()}
	if result := use(blueUse); result.ErrorCode != 0 || quantity(blue.ID) != 3 {
		t.Fatalf("满MP蓝药没有被正确消费:%v", result)
	}
	replayed := use(original)
	if replayed.ErrorCode != 0 || quantity(red.ID) != 3 || replayed.CommandSeq <= first.CommandSeq {
		t.Fatal("原药品重传重复扣除，或没有返回当前经济水位")
	}
	var receipt persistence.Operation
	opKey := "asset:" + roleID + ":" + domain.Hex(original.Operation.OpId)
	must(t, inspection.DB.Collection("operations").FindOne(context.Background(), bson.M{"_id": opKey}).Decode(&receipt))
	if receipt.CommandSeq != int64(first.CommandSeq) || receipt.Potion == nil || receipt.Potion.Scene != strconv.FormatUint(original.SceneEpoch, 10) {
		t.Fatal("没有保留首次提交原序号与原场景消费回执")
	}
	count, err := inspection.DB.Collection("audit_outbox").CountDocuments(context.Background(), bson.M{"_id": opKey})
	must(t, err)
	if count != 1 {
		t.Fatal("药品重传产生了重复审计事件")
	}
	time.Sleep(time.Duration(cfg.Fixture.Potions.UseDelayMS) * time.Millisecond)
	wrongType := &pb.UseItemRequest{ItemInstanceId: domain.Bytes(red.ID), ItemId: blue.ItemID, SceneEpoch: c.entry.SceneEpoch, Operation: c.operation()}
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
	if result := use(original); result.ErrorCode != 0 || quantity(red.ID) != 3 {
		t.Fatal("旧场景已提交药品无法沿原身份核对，或再次扣药")
	}
	newStale := &pb.UseItemRequest{ItemInstanceId: domain.Bytes(red.ID), ItemId: red.ItemID, SceneEpoch: original.SceneEpoch, Operation: c.operation()}
	if result := use(newStale); result.ErrorCode != p.CodeStaleSession || quantity(red.ID) != 3 {
		t.Fatal("旧scene的新操作未被拒绝或消耗库存")
	}
	var document bson.M
	must(t, inspection.DB.Collection("roles").FindOne(context.Background(), bson.M{"_id": roleID}).Decode(&document))
	for _, key := range []string{"hp", "mp", "position", "cooldowns", "potions", "potion_queue", "next_potion_use"} {
		if _, exists := document[key]; exists {
			t.Fatalf("药品临时现场%s被写入角色文档", key)
		}
	}
	t.Logf("通过：药品满值消费、库存减一、原回执防重/审计、类型拒绝、节点重启后原scene对账，规则版本%d", cfg.Fixture.RuleVersion)
}
