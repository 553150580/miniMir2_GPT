// 通过真实 WebSocket 验证同账号两角色仓库原子转移、另一角色可见性及重连。

package integration

import (
	"context"
	"fmt"
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

// warehousePage 返回仓库页面或分页供调用方使用。
// c 记录客户端，scope 记录作用域，使用数值表示，start 记录起点或开始时间，使用数值表示。
func warehousePage(c *client, scope uint32, start uint64) *pb.WarehousePage {
	c.t.Helper()
	// request 保存由发送取得的请求。
	request := c.send(p.MsgC2SWarehouseQuery, &pb.WarehouseQuery{Scope: scope, StartSlot: start, Limit: 1})
	// f 接收有效截止时间供后续步骤使用，f 记录协议帧。
	f := c.until(func(f wire.Frame) bool { return f.Request == request && f.ID == p.MsgS2CWarehousePage })
	// out 保存当前处理所用的输出。
	out := &pb.WarehousePage{}
	must(c.t, proto.Unmarshal(f.Payload, out))
	return out
}

// warehouseTerminal 返回仓库终止供调用方使用。
// c 记录客户端，request 关联客户端请求与服务端的对应响应。
func warehouseTerminal(c *client, request uint32) *pb.OperationResult {
	c.t.Helper()
	for {
		// f 接收有效截止时间供后续步骤使用，f 记录协议帧。
		f := c.until(func(f wire.Frame) bool { return f.Request == request && f.ID == p.MsgS2COperationResult })
		// res 保存当前处理所用的结果。
		res := &pb.OperationResult{}
		must(c.t, proto.Unmarshal(f.Payload, res))
		if res.Pending {
			continue
		}
		if res.CommandSeq > c.watermark {
			c.watermark = res.CommandSeq
		}
		c.inventoryAt(res.NewRevision)
		return res
	}
}

// warehouseReceipt 返回仓库回执供调用方使用。
// c 记录客户端，request 关联客户端请求与服务端的对应响应。
func warehouseReceipt(c *client, request uint32) *pb.WarehouseActionResult {
	// f 接收有效截止时间供后续步骤使用，f 记录协议帧。
	f := c.until(func(f wire.Frame) bool { return f.Request == request && f.ID == p.MsgS2CWarehouseActionResult })
	// out 保存当前处理所用的输出。
	out := &pb.WarehouseActionResult{}
	must(c.t, proto.Unmarshal(f.Payload, out))
	return out
}

// warehouseAction 处理仓库动作。
// c 记录客户端，q 记录仓库动作。
func warehouseAction(c *client, q *pb.WarehouseAction) (*pb.OperationResult, *pb.WarehouseActionResult) {
	c.t.Helper()
	if q.Operation == nil {
		q.Operation = c.operation()
	}
	// request 保存由发送取得的请求。
	request := c.send(p.MsgC2SWarehouseAction, q)
	// res 保存由仓库终止取得的结果。
	res := warehouseTerminal(c, request)
	if res.ErrorCode != 0 {
		return res, nil
	}
	return res, warehouseReceipt(c, request)
}

// 资格仅加速战斗和已有资金/Until；共享两件争抢物由真实Auto产生，49旧槽仅容量前态。
// TestWarehousesTwoRolesAtomicTransferAndReconnectMongoWS 验证同账号两角色存取共享仓库、原子资产变化和重连读取。
// t 提供断言失败报告、子用例和清理注册。
func TestWarehousesTwoRolesAtomicTransferAndReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo/WS双仓储")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	// i 标记遍历怪物集合时访问的下标。
	for i := range cfg.Fixture.Monsters {
		cfg.Fixture.Monsters[i].HP = 1
		cfg.Fixture.Monsters[i].AttackMin = 0
		cfg.Fixture.Monsters[i].AttackMax = 0
		cfg.Fixture.Monsters[i].Agility = 0
	}
	must(t, cfg.Fixture.Validate())
	cfg.Database = "minimir_dev_game_test_whws_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// ctx 保存由“background”取得的上下文。
	ctx := context.Background()
	// inspection 保存由打开取得的检查。
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	// launch 封装启动对应的后续处理。
	launch := func() func() {
		// run 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		run, cancel := context.WithCancel(ctx)
		// group 记录分组，使用协程汇合计数器表示。
		var group sync.WaitGroup
		// mode 接收遍历文本列表时当前的元素。
		for _, mode := range []string{"account", "game", "gm"} {
			group.Go(func() {
				// e 接收运行批次供后续步骤使用。
				if e := bootstrap.Run(run, cfg, mode); e != nil && run.Err() == nil {
					t.Errorf("仓储%s:%v", mode, e)
				}
			})
		}
		// a 接收遍历文本列表时当前的元素。
		for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
			healthy(t, a)
		}
		return func() { cancel(); group.Wait() }
	}
	// stop 保存由启动取得的停止。
	stop := launch()
	t.Cleanup(func() {
		stop()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		cleanup, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	// url 保存根据当前数据与账号地址的计算结果与当前数据计算的URL。
	url := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, url, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "whws_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	// keys 创建用于维护键集合的容器或通道。
	keys := make([]*pb.RoleSummary, 2)
	// until 保存由新增取得的有效截止时间。
	until := time.Now().Add(time.Hour)
	// n 接收遍历键集合时当前的元素。
	for n := range keys {
		keys[n] = &pb.RoleSummary{}
		rpc(t, url, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: uint32(n + 1), Job: 0, Name: fmt.Sprintf("双仓%d%s", n, domain.ID()[:5]), OpId: domain.Bytes(domain.ID())}, keys[n], 0)
		_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(keys[n].Key.RoleId)}, bson.M{"$set": bson.M{"privilege_until": until, "wallet.gold": int64(501), "wallet.yuanbao_units": cfg.Fixture.Warehouses.PersonalExpansionYuanbaoUnits + 19}})
		must(t, err)
	}
	// role0 保存由角色取得的角色0。
	role0, err := inspection.Role(ctx, domain.Hex(keys[0].Key.RoleId))
	must(t, err)
	// warehouseID 标识当前业务使用的仓库。
	warehouseID := fmt.Sprintf("shared:%s:%d", role0.AccountID, role0.Origin)
	// meta 初始化用作元数据的仓库。
	meta := persistence.Warehouse{ID: warehouseID, Scope: 2, AccountID: role0.AccountID, Origin: role0.Origin, Revision: 1, Capacity: int64(cfg.Fixture.Warehouses.SharedSlots)}
	_, err = inspection.DB.Collection("warehouses").InsertOne(ctx, meta)
	must(t, err)
	// qualification 建立带初始值的仓库“stored”道具列表，供本段继续处理。
	qualification := []persistence.WarehouseStoredItem{}
	// n 接收整数“64”供后续步骤使用。
	for n := int64(0); n < meta.Capacity-1; n++ {
		// i 初始化用作当前位置的下标的道具。
		i := domain.Item{ID: domain.ID(), ItemID: 35, Quantity: 1, Version: 1, SourceKind: "development_fixture", SourceID: "warehouse-capacity-precondition"}
		// v 组装仓库已持久保存道具所需的数据。
		v := persistence.WarehouseStoredItem{ID: i.ID, WarehouseID: warehouseID, Slot: n, Item: i}
		qualification = append(qualification, v)
		_, err = inspection.DB.Collection("warehouse_items").InsertOne(ctx, v)
		must(t, err)
	}
	_, err = inspection.DB.Collection("test_qualification_setup").InsertOne(ctx, bson.M{"_id": "warehouse_capacity", "metadata": meta, "items": qualification, "privilege_until": until, "monster_hp": 1, "monster_attack": 0, "main_profile_changed": false})
	must(t, err)
	// clients 创建用于维护客户端集合的容器或通道。
	clients := make([]*client, 2)
	// n 接收遍历客户端集合时当前的元素。
	for n := range clients {
		clients[n] = join(t, cfg.GameAddress, issue(t, url, login.AccountToken, keys[n].Key))
		// n 记录数值。
		defer func(n int) { clients[n].close() }(n)
		clients[n].auto(false, 1)
		// wood 记录“wood”，使用道具观察视图表示。
		var wood *pb.ItemView
		// i 接收遍历道具集合时当前的元素。
		for _, i := range clients[n].inventory.Items {
			if i.ItemId == 218 {
				wood = i
				break
			}
		}
		if wood == nil {
			t.Fatal("正常木剑缺失")
		}
		// res 保存由资产取得的结果。
		res := clients[n].asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: wood.InstanceId, ItemVersion: wood.Version, Slot: 0, Operation: clients[n].operation()})
		if res.ErrorCode != 0 {
			t.Fatal(res)
		}
		clients[n].inventoryAt(res.NewRevision)
		clients[n].auto(true, 8)
	}
	// drops 创建用于维护掉落集合的容器或通道。
	drops := make([]*pb.ItemView, 2)
	// n 定位客户端集合中正在处理的成员，c 接收遍历客户端集合时当前的元素。
	for n, c := range clients {
		// f 记录协议帧。
		c.until(func(f wire.Frame) bool {
			if f.ID != p.MsgS2CInventory {
				return false
			}
			// i 接收遍历道具集合时当前的元素。
			for _, i := range c.inventory.Items {
				if !i.Bound && i.SourceKind == "monster_drop" {
					drops[n] = proto.Clone(i).(*pb.ItemView)
					return true
				}
			}
			return false
		})
		c.auto(false, 0)
		travelToBaseline(c, 1, 3)
		travelAcknowledge(c, true)
	}
	// 同账号5角色同时附着；同一角色换票的会话围栏不得挤掉其它4角色。
	// otherClients 记录另一项客户端集合，使用客户端列表表示。
	var otherClients []*client
	defer func() {
		// c 接收遍历另一项客户端集合时当前的元素。
		for _, c := range otherClients {
			c.close()
		}
	}()
	// slot 保存由无符号整数“32”取得的槽位。
	for slot := uint32(3); slot <= 5; slot++ {
		// key 保存当前处理所用的键。
		key := &pb.RoleSummary{}
		rpc(t, url, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: slot, Job: 0, Name: fmt.Sprintf("并发仓%d%s", slot, domain.ID()[:5]), OpId: domain.Bytes(domain.ID())}, key, 0)
		// c 接收加入供后续步骤使用。
		c := join(t, cfg.GameAddress, issue(t, url, login.AccountToken, key.Key))
		otherClients = append(otherClients, c)
		c.auto(false, 1)
		travelToBaseline(c, 1, 3)
		travelAcknowledge(c, true)
	}
	// 后台仍有两个独立操作者；共享竞争不能把另一角色踢下线。
	// c 接收遍历客户端集合时当前的元素。
	for _, c := range clients {
		if warehousePage(c, 2, 0).Revision != 1 {
			t.Fatal("容量前态变化")
		}
	}
	// before 保存由仓库全局道具集合取得的操作前的状态。
	before := warehouseGlobalItems(t, inspection, ctx)
	// last 保存根据共享槽位集合与当前数据计算的上一次状态。
	last := cfg.Fixture.Warehouses.SharedSlots - 1
	// requests 创建用于维护请求集合的容器或通道。
	requests := make([]uint32, 2)
	// n 定位客户端集合中正在处理的成员，c 接收遍历客户端集合时当前的元素。
	for n, c := range clients {
		requests[n] = c.send(p.MsgC2SWarehouseAction, &pb.WarehouseAction{Scope: 2, Action: 1, WarehouseRevision: 1, WarehouseSlot: last, ItemInstanceId: drops[n].InstanceId, ItemVersion: drops[n].Version, Quantity: drops[n].Quantity, Operation: c.operation()})
	}
	// winner 保存当前处理所用的胜者。
	winner := -1
	// n 定位客户端集合中正在处理的成员，c 接收遍历客户端集合时当前的元素。
	for n, c := range clients {
		// res 保存由仓库终止取得的结果。
		res := warehouseTerminal(c, requests[n])
		if res.ErrorCode == 0 {
			if winner != -1 {
				t.Fatal("最后格双成功")
			}
			winner = n
			warehouseReceipt(c, requests[n])
		} else if res.ErrorCode != p.CodeWarehouseChanged && res.ErrorCode != p.CodeWarehouseFull {
			t.Fatal("竞争异常", res)
		}
	}
	if winner < 0 {
		t.Fatal("最后格无人提交")
	}
	if !reflect.DeepEqual(before, warehouseGlobalItems(t, inspection, ctx)) {
		t.Fatal("抢最后格丢失/复制UID或数量")
	}
	// page 保存由仓库页面或分页取得的页面或分页。
	page := warehousePage(clients[0], 2, last)
	if page.Revision != 2 || len(page.Items) != 1 || page.Items[0].Item.Bound {
		t.Fatal("共享最后格不是原非绑掉物", page)
	}
	// stored 保存道具，用于当前步骤中的已持久保存。
	stored := page.Items[0].Item
	// n 定位客户端集合中正在处理的成员，c 接收遍历客户端集合时当前的元素。
	for n, c := range clients {
		requests[n] = c.send(p.MsgC2SWarehouseAction, &pb.WarehouseAction{Scope: 2, Action: 2, WarehouseRevision: 2, WarehouseSlot: last, ItemInstanceId: stored.InstanceId, ItemVersion: stored.Version, Quantity: stored.Quantity, Operation: c.operation()})
	}
	// wins 规定“wins”的初始取值。
	wins := 0
	// n 定位客户端集合中正在处理的成员，c 接收遍历客户端集合时当前的元素。
	for n, c := range clients {
		// res 保存由仓库终止取得的结果。
		res := warehouseTerminal(c, requests[n])
		if res.ErrorCode == 0 {
			wins++
			warehouseReceipt(c, requests[n])
		} else if res.ErrorCode != p.CodeWarehouseChanged {
			t.Fatal(res)
		}
	}
	if wins != 1 || !reflect.DeepEqual(before, warehouseGlobalItems(t, inspection, ctx)) {
		t.Fatal("两角色取同物双成功或资产不守恒")
	}
	// a 保存从客户端集合取出的a。
	a := clients[0]
	// personal 保存由仓库页面或分页取得的个人。
	personal := warehousePage(a, 1, 0)
	if personal.Revision != 0 {
		t.Fatal("个人仓不是独立虚拟0")
	}
	// bound 记录绑定，使用道具观察视图表示。
	var bound *pb.ItemView
	// i 接收遍历道具集合时当前的元素。
	for _, i := range a.inventory.Items {
		if i.Bound {
			bound = proto.Clone(i).(*pb.ItemView)
			break
		}
	}
	if bound == nil {
		t.Fatal("正常Create绑定物缺失")
	}
	// res 保存由仓库动作取得的结果，put 保存由仓库动作取得的“put”。
	res, put := warehouseAction(a, &pb.WarehouseAction{Scope: 1, Action: 1, WarehouseSlot: 0, ItemInstanceId: bound.InstanceId, ItemVersion: bound.Version, Quantity: bound.Quantity})
	if res.ErrorCode != 0 || !put.Changes[0].After.Bound {
		t.Fatal("个人不能存原绑定物")
	}
	// expanded 保存由仓库动作取得的“expanded”。
	res, expanded := warehouseAction(a, &pb.WarehouseAction{Scope: 1, Action: 5, WarehouseRevision: 1})
	if res.ErrorCode != 0 || expanded.Capacity != cfg.Fixture.Warehouses.PersonalInitialSlots+cfg.Fixture.Warehouses.PersonalExpansionSlots {
		t.Fatal("正常个人扩页失败", res)
	}
	// deposit 保存由仓库动作取得的“deposit”。
	res, deposit := warehouseAction(a, &pb.WarehouseAction{Scope: 2, Action: 3, WarehouseRevision: 3, Currency: 2, Amount: 7})
	if res.ErrorCode != 0 || deposit.YuanbaoUnits != 7 {
		t.Fatal("共享元宝转移失败", res)
	}
	res, _ = warehouseAction(a, &pb.WarehouseAction{Scope: 2, Action: 4, WarehouseRevision: 4, Currency: 2, Amount: 7})
	if res.ErrorCode != 0 {
		t.Fatal(res)
	}
	// 同角色新票挤掉旧操作者，其它角色仍正常；新连接不补播旧40023。
	// old 保存当前处理所用的原状态。
	old := a
	// hero 按目标类型读取当前接口值，得到“hero”。
	hero := proto.Clone(old.player()).(*pb.EntityState)
	// bornReplay 规定出生重放的初始取值。
	bornReplay := 0
	// fresh 保存由加入取得的首次装载，f 记录协议帧。
	fresh := join(t, cfg.GameAddress, issue(t, url, login.AccountToken, keys[0].Key), func(_ *client, f wire.Frame) {
		if f.ID == p.MsgS2CWarehouseActionResult {
			bornReplay++
		}
	})
	clients[0] = fresh
	if fresh.player().Life != hero.Life || fresh.player().Hp != hero.Hp || fresh.player().Mp != hero.Mp || fresh.entry.SceneEpoch != old.entry.SceneEpoch {
		t.Fatal("健康新票仓储重置身体")
	}
	old.close()
	if warehousePage(fresh, 1, 0).Revision != 2 || warehousePage(clients[1], 2, 0).Revision != 5 || bornReplay != 0 {
		t.Fatal("健康重入仓储丢失、其它角色被挤线或重播历史")
	}
	// c 接收遍历另一项客户端集合时当前的元素。
	for _, c := range otherClients {
		if warehousePage(c, 1, 0).Revision != 0 {
			t.Fatal("其它角色个人仓串用或会话被挤")
		}
	}
	// c 接收遍历客户端集合时当前的元素。
	for _, c := range clients {
		c.close()
	}
	// c 接收遍历另一项客户端集合时当前的元素。
	for _, c := range otherClients {
		c.close()
	}
	stop()
	stop = launch()
	// cold 保存由加入取得的“cold”。
	cold := join(t, cfg.GameAddress, issue(t, url, login.AccountToken, keys[0].Key))
	clients[0] = cold
	cold.auto(false, 0)
	if warehousePage(cold, 1, 0).Capacity != expanded.Capacity || warehousePage(cold, 2, 0).Revision != 5 {
		t.Fatal("冷重建永久仓库丢失")
	}
	t.Logf("真实两角色并在线/同角色新票；两件正常Auto非绑掉物，49槽资格争第%d格一胜、同UID取出一胜；UID/数量守恒；个人绑定/扩页与共享元宝双向；健康/完整stop-launch冷建持久", last)
}

// warehouseGlobalItems 返回仓库全局道具集合供调用方使用。
// t 提供断言失败报告、子用例和清理注册，s 记录持久层存储，ctx 向下传递当前请求的截止时间和取消信号。
func warehouseGlobalItems(t *testing.T, s *persistence.Store, ctx context.Context) map[string]int64 {
	t.Helper()
	// out 初始化用作输出的按文本索引的数值映射。
	out := map[string]int64{}
	// add 封装新增对应的后续处理，i 记录当前位置的下标，使用道具表示。
	add := func(i domain.Item) {
		// ok 表明映射中能否取得指定键。
		if _, ok := out[i.ID]; ok {
			t.Fatal("重复UID", i.ID)
		}
		out[i.ID] = i.Quantity
	}
	// cur 保存由查找取得的“cur”，e 接收查找供后续步骤使用。
	cur, e := s.DB.Collection("roles").Find(ctx, bson.M{})
	must(t, e)
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		// r 记录角色。
		var r domain.Role
		must(t, cur.Decode(&r))
		// i 接收遍历道具集合时当前的元素。
		for _, i := range r.Items {
			add(i)
		}
		// i 接收遍历已穿戴装备时当前的元素。
		for _, i := range r.Equipped {
			add(i.Item)
		}
	}
	must(t, cur.Err())
	// items 保存由查找取得的道具集合。
	items, e := s.DB.Collection("warehouse_items").Find(ctx, bson.M{})
	must(t, e)
	defer items.Close(ctx)
	for items.Next(ctx) {
		// i 记录当前位置的下标，使用仓库已持久保存道具表示。
		var i persistence.WarehouseStoredItem
		must(t, items.Decode(&i))
		add(i.Item)
	}
	must(t, items.Err())
	return out
}
