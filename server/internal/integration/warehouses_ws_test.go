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

func warehousePage(c *client, scope uint32, start uint64) *pb.WarehousePage {
	c.t.Helper()
	request := c.send(p.MsgC2SWarehouseQuery, &pb.WarehouseQuery{Scope: scope, StartSlot: start, Limit: 1})
	f := c.until(func(f wire.Frame) bool { return f.Request == request && f.ID == p.MsgS2CWarehousePage })
	out := &pb.WarehousePage{}
	must(c.t, proto.Unmarshal(f.Payload, out))
	return out
}
func warehouseTerminal(c *client, request uint32) *pb.OperationResult {
	c.t.Helper()
	for {
		f := c.until(func(f wire.Frame) bool { return f.Request == request && f.ID == p.MsgS2COperationResult })
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
func warehouseReceipt(c *client, request uint32) *pb.WarehouseActionResult {
	f := c.until(func(f wire.Frame) bool { return f.Request == request && f.ID == p.MsgS2CWarehouseActionResult })
	out := &pb.WarehouseActionResult{}
	must(c.t, proto.Unmarshal(f.Payload, out))
	return out
}
func warehouseAction(c *client, q *pb.WarehouseAction) (*pb.OperationResult, *pb.WarehouseActionResult) {
	c.t.Helper()
	if q.Operation == nil {
		q.Operation = c.operation()
	}
	request := c.send(p.MsgC2SWarehouseAction, q)
	res := warehouseTerminal(c, request)
	if res.ErrorCode != 0 {
		return res, nil
	}
	return res, warehouseReceipt(c, request)
}

// 资格仅加速战斗和已有资金/Until；共享两件争抢物由真实Auto产生，49旧槽仅容量前态。
func TestWarehousesTwoRolesAtomicTransferAndReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo/WS双仓储")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	for i := range cfg.Fixture.Monsters {
		cfg.Fixture.Monsters[i].HP = 1
		cfg.Fixture.Monsters[i].AttackMin = 0
		cfg.Fixture.Monsters[i].AttackMax = 0
		cfg.Fixture.Monsters[i].Agility = 0
	}
	must(t, cfg.Fixture.Validate())
	cfg.Database = "minimir_dev_game_test_whws_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	ctx := context.Background()
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	launch := func() func() {
		run, cancel := context.WithCancel(ctx)
		var group sync.WaitGroup
		for _, mode := range []string{"account", "game", "gm"} {
			group.Go(func() {
				if e := bootstrap.Run(run, cfg, mode); e != nil && run.Err() == nil {
					t.Errorf("仓储%s:%v", mode, e)
				}
			})
		}
		for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
			healthy(t, a)
		}
		return func() { cancel(); group.Wait() }
	}
	stop := launch()
	t.Cleanup(func() {
		stop()
		cleanup, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	url := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, url, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "whws_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	keys := make([]*pb.RoleSummary, 2)
	until := time.Now().Add(time.Hour)
	for n := range keys {
		keys[n] = &pb.RoleSummary{}
		rpc(t, url, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: uint32(n + 1), Job: 0, Name: fmt.Sprintf("双仓%d%s", n, domain.ID()[:5]), OpId: domain.Bytes(domain.ID())}, keys[n], 0)
		_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(keys[n].Key.RoleId)}, bson.M{"$set": bson.M{"privilege_until": until, "wallet.gold": int64(501), "wallet.yuanbao_units": cfg.Fixture.Warehouses.PersonalExpansionYuanbaoUnits + 19}})
		must(t, err)
	}
	role0, err := inspection.Role(ctx, domain.Hex(keys[0].Key.RoleId))
	must(t, err)
	warehouseID := fmt.Sprintf("shared:%s:%d", role0.AccountID, role0.Origin)
	meta := persistence.Warehouse{ID: warehouseID, Scope: 2, AccountID: role0.AccountID, Origin: role0.Origin, Revision: 1, Capacity: int64(cfg.Fixture.Warehouses.SharedSlots)}
	_, err = inspection.DB.Collection("warehouses").InsertOne(ctx, meta)
	must(t, err)
	qualification := []persistence.WarehouseStoredItem{}
	for n := int64(0); n < meta.Capacity-1; n++ {
		i := domain.Item{ID: domain.ID(), ItemID: 35, Quantity: 1, Version: 1, SourceKind: "development_fixture", SourceID: "warehouse-capacity-precondition"}
		v := persistence.WarehouseStoredItem{ID: i.ID, WarehouseID: warehouseID, Slot: n, Item: i}
		qualification = append(qualification, v)
		_, err = inspection.DB.Collection("warehouse_items").InsertOne(ctx, v)
		must(t, err)
	}
	_, err = inspection.DB.Collection("test_qualification_setup").InsertOne(ctx, bson.M{"_id": "warehouse_capacity", "metadata": meta, "items": qualification, "privilege_until": until, "monster_hp": 1, "monster_attack": 0, "main_profile_changed": false})
	must(t, err)
	clients := make([]*client, 2)
	for n := range clients {
		clients[n] = join(t, cfg.GameAddress, issue(t, url, login.AccountToken, keys[n].Key))
		defer func(n int) { clients[n].close() }(n)
		clients[n].auto(false, 1)
		var wood *pb.ItemView
		for _, i := range clients[n].inventory.Items {
			if i.ItemId == 218 {
				wood = i
				break
			}
		}
		if wood == nil {
			t.Fatal("正常木剑缺失")
		}
		res := clients[n].asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: wood.InstanceId, ItemVersion: wood.Version, Slot: 0, Operation: clients[n].operation()})
		if res.ErrorCode != 0 {
			t.Fatal(res)
		}
		clients[n].inventoryAt(res.NewRevision)
		clients[n].auto(true, 8)
	}
	drops := make([]*pb.ItemView, 2)
	for n, c := range clients {
		c.until(func(f wire.Frame) bool {
			if f.ID != p.MsgS2CInventory {
				return false
			}
			for _, i := range c.inventory.Items {
				if !i.Bound && i.SourceKind == "monster_drop" {
					drops[n] = proto.Clone(i).(*pb.ItemView)
					return true
				}
			}
			return false
		})
		c.auto(false, 0)
		travelToBaseline(c, 1, 2)
		travelAcknowledge(c, true)
	}
	// 同账号5角色同时附着；同一角色换票的会话围栏不得挤掉其它4角色。
	var otherClients []*client
	defer func() {
		for _, c := range otherClients {
			c.close()
		}
	}()
	for slot := uint32(3); slot <= 5; slot++ {
		key := &pb.RoleSummary{}
		rpc(t, url, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: slot, Job: 0, Name: fmt.Sprintf("并发仓%d%s", slot, domain.ID()[:5]), OpId: domain.Bytes(domain.ID())}, key, 0)
		c := join(t, cfg.GameAddress, issue(t, url, login.AccountToken, key.Key))
		otherClients = append(otherClients, c)
		c.auto(false, 1)
		travelToBaseline(c, 1, 2)
		travelAcknowledge(c, true)
	}
	// 后台仍有两个独立操作者；共享竞争不能把另一角色踢下线。
	for _, c := range clients {
		if warehousePage(c, 2, 0).Revision != 1 {
			t.Fatal("容量前态变化")
		}
	}
	before := warehouseGlobalItems(t, inspection, ctx)
	last := cfg.Fixture.Warehouses.SharedSlots - 1
	requests := make([]uint32, 2)
	for n, c := range clients {
		requests[n] = c.send(p.MsgC2SWarehouseAction, &pb.WarehouseAction{Scope: 2, Action: 1, WarehouseRevision: 1, WarehouseSlot: last, ItemInstanceId: drops[n].InstanceId, ItemVersion: drops[n].Version, Quantity: drops[n].Quantity, Operation: c.operation()})
	}
	winner := -1
	for n, c := range clients {
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
	page := warehousePage(clients[0], 2, last)
	if page.Revision != 2 || len(page.Items) != 1 || page.Items[0].Item.Bound {
		t.Fatal("共享最后格不是原非绑掉物", page)
	}
	stored := page.Items[0].Item
	for n, c := range clients {
		requests[n] = c.send(p.MsgC2SWarehouseAction, &pb.WarehouseAction{Scope: 2, Action: 2, WarehouseRevision: 2, WarehouseSlot: last, ItemInstanceId: stored.InstanceId, ItemVersion: stored.Version, Quantity: stored.Quantity, Operation: c.operation()})
	}
	wins := 0
	for n, c := range clients {
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
	a := clients[0]
	personal := warehousePage(a, 1, 0)
	if personal.Revision != 0 {
		t.Fatal("个人仓不是独立虚拟0")
	}
	var bound *pb.ItemView
	for _, i := range a.inventory.Items {
		if i.Bound {
			bound = proto.Clone(i).(*pb.ItemView)
			break
		}
	}
	if bound == nil {
		t.Fatal("正常Create绑定物缺失")
	}
	res, put := warehouseAction(a, &pb.WarehouseAction{Scope: 1, Action: 1, WarehouseSlot: 0, ItemInstanceId: bound.InstanceId, ItemVersion: bound.Version, Quantity: bound.Quantity})
	if res.ErrorCode != 0 || !put.Changes[0].After.Bound {
		t.Fatal("个人不能存原绑定物")
	}
	res, expanded := warehouseAction(a, &pb.WarehouseAction{Scope: 1, Action: 5, WarehouseRevision: 1})
	if res.ErrorCode != 0 || expanded.Capacity != cfg.Fixture.Warehouses.PersonalInitialSlots+cfg.Fixture.Warehouses.PersonalExpansionSlots {
		t.Fatal("正常个人扩页失败", res)
	}
	res, deposit := warehouseAction(a, &pb.WarehouseAction{Scope: 2, Action: 3, WarehouseRevision: 3, Currency: 2, Amount: 7})
	if res.ErrorCode != 0 || deposit.YuanbaoUnits != 7 {
		t.Fatal("共享元宝转移失败", res)
	}
	res, _ = warehouseAction(a, &pb.WarehouseAction{Scope: 2, Action: 4, WarehouseRevision: 4, Currency: 2, Amount: 7})
	if res.ErrorCode != 0 {
		t.Fatal(res)
	}
	// 同角色新票挤掉旧操作者，其它角色仍正常；新连接不补播旧40023。
	old := a
	hero := proto.Clone(old.player()).(*pb.EntityState)
	bornReplay := 0
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
	for _, c := range otherClients {
		if warehousePage(c, 1, 0).Revision != 0 {
			t.Fatal("其它角色个人仓串用或会话被挤")
		}
	}
	for _, c := range clients {
		c.close()
	}
	for _, c := range otherClients {
		c.close()
	}
	stop()
	stop = launch()
	cold := join(t, cfg.GameAddress, issue(t, url, login.AccountToken, keys[0].Key))
	clients[0] = cold
	cold.auto(false, 0)
	if warehousePage(cold, 1, 0).Capacity != expanded.Capacity || warehousePage(cold, 2, 0).Revision != 5 {
		t.Fatal("冷重建永久仓库丢失")
	}
	t.Logf("真实两角色并在线/同角色新票；两件正常Auto非绑掉物，49槽资格争第%d格一胜、同UID取出一胜；UID/数量守恒；个人绑定/扩页与共享元宝双向；健康/完整stop-launch冷建持久", last)
}

func warehouseGlobalItems(t *testing.T, s *persistence.Store, ctx context.Context) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	add := func(i domain.Item) {
		if _, ok := out[i.ID]; ok {
			t.Fatal("重复UID", i.ID)
		}
		out[i.ID] = i.Quantity
	}
	cur, e := s.DB.Collection("roles").Find(ctx, bson.M{})
	must(t, e)
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var r domain.Role
		must(t, cur.Decode(&r))
		for _, i := range r.Items {
			add(i)
		}
		for _, i := range r.Equipped {
			add(i.Item)
		}
	}
	must(t, cur.Err())
	items, e := s.DB.Collection("warehouse_items").Find(ctx, bson.M{})
	must(t, e)
	defer items.Close(ctx)
	for items.Next(ctx) {
		var i persistence.WarehouseStoredItem
		must(t, items.Decode(&i))
		add(i.Item)
	}
	must(t, items.Err())
	return out
}
