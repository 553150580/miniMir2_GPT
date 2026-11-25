// 验证并发创建角色的数量配额，以及共享运行区服内不同永久入口的仓库隔离。

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestWarehouseConcurrentFifthRoleAndSixthRejectedMongoHTTP 并发申请最后的合法角色槽与越界槽，检查 HTTP 回执和实际角色数。
// t 提供断言失败报告、子用例和清理注册。
func TestWarehouseConcurrentFifthRoleAndSixthRejectedMongoHTTP(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo/HTTP抢第五槽")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_wh5_" + domain.ID()
	cfg.AccountAddress = address(t)
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancel(context.Background())
	// inspection 保存由打开取得的检查。
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	// group 记录分组，使用协程汇合计数器表示。
	var group sync.WaitGroup
	group.Go(func() {
		// e 接收运行批次供后续步骤使用。
		if e := bootstrap.Run(ctx, cfg, "account"); e != nil && ctx.Err() == nil {
			t.Errorf("account:%v", e)
		}
	})
	t.Cleanup(func() {
		cancel()
		group.Wait()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，done 传递从调用方派生的上下文，供取消或控制截止时间。
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	healthy(t, cfg.AccountAddress)
	// url 保存根据当前数据与账号地址的计算结果与当前数据计算的URL。
	url := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, url, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "wh5_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	// slot 保存由无符号整数“32”取得的槽位。
	for slot := uint32(1); slot <= 4; slot++ {
		rpc(t, url, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: slot, Job: 0, Name: fmt.Sprintf("原槽%d%s", slot, domain.ID()[:5]), OpId: domain.Bytes(domain.ID())}, &pb.RoleSummary{}, 0)
	}
	// queries 建立带初始值的“create”角色请求列表，供本段继续处理。
	queries := []*pb.CreateRoleRequest{{OriginServerId: cfg.OriginID, Slot: 5, Job: 0, Name: "竞五甲" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, {OriginServerId: cfg.OriginID, Slot: 5, Job: 1, Name: "竞五乙" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, {OriginServerId: cfg.OriginID, Slot: 6, Job: 2, Name: "竞六" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}}
	// response 保存响应所需的状态。
	type response struct {
		// frame 记录协议帧。
		frame wire.Frame
		// role 记录角色，使用角色“summary”表示。
		role *pb.RoleSummary
		// err 保存当前操作的失败原因，供调用方检查或返回。
		err error
	}
	// exchange 封装“exchange”对应的后续处理，q 记录创建角色请求。
	exchange := func(q *pb.CreateRoleRequest) response {
		// req 保存由创建请求带有上下文取得的请求，e 接收创建请求带有上下文供后续步骤使用。
		req, e := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(wire.Encode(wire.Message(p.MsgC2SCreateRole, 1, 0, q))))
		if e != nil {
			return response{err: e}
		}
		req.Header.Set("Authorization", "Bearer "+login.AccountToken)
		// res 保存由“do”取得的结果。
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			return response{err: e}
		}
		defer res.Body.Close()
		// raw 保存由读取全部取得的原始值。
		raw, e := io.ReadAll(res.Body)
		if e != nil {
			return response{err: e}
		}
		// f 接收解码供后续步骤使用。
		f, e := wire.Decode(raw)
		if e != nil {
			return response{err: e}
		}
		// role 保存当前处理所用的角色。
		role := &pb.RoleSummary{}
		if f.Error == 0 {
			e = proto.Unmarshal(f.Payload, role)
		}
		return response{f, role, e}
	}
	// 让并发建角共享一个起跑门，以覆盖真实的唯一索引争用。
	// gate 创建用于维护门禁的容器或通道。
	gate := make(chan struct{})
	// answers 创建用于维护“answers”的容器或通道。
	answers := make([]response, len(queries))
	// racers 记录“racers”，使用协程汇合计数器表示。
	var racers sync.WaitGroup
	// n 定位“queries”中正在处理的成员，q 接收遍历“queries”时当前的元素。
	for n, q := range queries {
		racers.Go(func() { <-gate; answers[n] = exchange(q) })
	}
	close(gate)
	racers.Wait()
	// winner 保存当前处理所用的胜者。
	winner := -1
	// n 定位“answers”中正在处理的成员，a 接收遍历“answers”时当前的元素。
	for n, a := range answers {
		must(t, a.err)
		if n == 2 {
			if a.frame.Error != p.CodeInvalidArgument {
				t.Fatal("第六槽未拒绝", a.frame.Error)
			}
			continue
		}
		if a.frame.Error == 0 {
			if winner >= 0 {
				t.Fatal("第五槽双成功")
			}
			winner = n
		} else if a.frame.Error != p.CodeRoleSlotOrNameOccupied {
			t.Fatal("第五槽非预期终态", a.frame.Error)
		}
	}
	if winner < 0 {
		t.Fatal("第五槽没有赢家")
	}
	// n 定位“queries”中正在处理的成员，q 接收遍历“queries”时当前的元素。
	for n, q := range queries {
		// retry 保存由“exchange”取得的重试。
		retry := exchange(q)
		must(t, retry.err)
		if retry.frame.Error != answers[n].frame.Error || retry.frame.Error == 0 && !proto.Equal(retry.role, answers[n].role) {
			t.Fatal("原操作重放终态/角色身份改变")
		}
	}
	// directory 保存当前处理所用的目录。
	directory := &pb.RoleDirectoryResult{}
	rpc(t, url, login.AccountToken, p.MsgC2SRoleDirectory, &pb.RoleDirectoryRequest{OriginServerId: cfg.OriginID}, directory, 0)
	if len(directory.Roles) != 5 {
		t.Fatal("目录不是恰5", len(directory.Roles))
	}
	if directory.Roles[4].Slot != 5 {
		t.Fatal("最终槽目录错误")
	}
	if len(answers[winner].role.Key.RoleId) != 16 {
		t.Fatal("赢家无真实角色")
	}
	t.Logf("原1..4后同时发两个不同op抢5及第三请求6：赢家%d，其余为确定占用和参数错误；原op均重放稳定；目录恰5。", winner)

	// t 提供断言失败报告、子用例和清理注册。
	t.Run("已拒绝原建角在名字释放后仍不会执行", func(t *testing.T) {
		// other 保存当前处理所用的另一项。
		other := &pb.LoginResult{}
		rpc(t, url, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "wh5_" + domain.ID()[:10], Password: "test_123456"}, other, 0)
		// occupied 保存角色，用于当前步骤中的已占用。
		occupied := answers[winner].role
		// q 保存当前处理所用的创建角色请求。
		q := &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: 1, Job: 1, Name: occupied.Name, OpId: domain.Bytes(domain.ID())}
		// 首次确定拒绝的包可丢失，持久原回执不能依赖客户端收到它。
		rpc(t, url, other.AccountToken, p.MsgC2SCreateRole, q, &pb.RoleSummary{}, p.CodeRoleSlotOrNameOccupied)
		// id 区分当前步骤指向的目标对象。
		id := "create:" + domain.Hex(other.AccountId) + ":" + domain.Hex(q.OpId)
		// terminal 记录终止，使用操作表示。
		var terminal persistence.Operation
		must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": id}).Decode(&terminal))
		if terminal.Code != p.CodeRoleSlotOrNameOccupied || terminal.RoleID != "" || terminal.Kind != "create_role" {
			t.Fatalf("未保存确定拒绝的原回执: %+v", terminal)
		}

		// 仅在随机测试库释放占用，确认此前迟到的原请求也不会创建角色。
		// newName 保存根据当前数据与标识中的范围计算的创建名称。
		newName := "更名" + domain.ID()[:5]
		// updated 保存由更新“by”标识取得的“updated”，err 接收更新“by”标识过程中可能产生的错误，供后续失败分支处理。
		updated, err := inspection.DB.Collection("roles").UpdateByID(ctx, domain.Hex(occupied.Key.RoleId), bson.M{"$set": bson.M{"name": newName, "name_key": newName}})
		must(t, err)
		if updated.ModifiedCount != 1 {
			t.Fatal("测试未释放原名称")
		}
		rpc(t, url, other.AccountToken, p.MsgC2SCreateRole, q, &pb.RoleSummary{}, p.CodeRoleSlotOrNameOccupied)
		// changed 按目标类型读取当前接口值，得到“changed”。
		changed := proto.Clone(q).(*pb.CreateRoleRequest)
		changed.Job = 2
		rpc(t, url, other.AccountToken, p.MsgC2SCreateRole, changed, &pb.RoleSummary{}, p.CodeOperationConflict)
		// fresh 按目标类型读取当前接口值，得到首次装载。
		fresh := proto.Clone(q).(*pb.CreateRoleRequest)
		fresh.OpId = domain.Bytes(domain.ID())
		// created 保存当前处理所用的“created”。
		created := &pb.RoleSummary{}
		rpc(t, url, other.AccountToken, p.MsgC2SCreateRole, fresh, created, 0)
		if created.Name != q.Name || created.Slot != q.Slot || len(created.Key.GetRoleId()) != 16 {
			t.Fatalf("新操作未按预期建角: %+v", created)
		}
	})
}

// TestWarehousePermanentOriginsShareRunButNotAssetsMongo 让不同永久入口共用运行服，验证它们的共享仓库资产仍隔离。
// t 提供断言失败报告、子用例和清理注册。
func TestWarehousePermanentOriginsShareRunButNotAssetsMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo永久origin隔离")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_whorg_" + domain.ID()
	// ctx 保存由“background”取得的上下文。
	ctx := context.Background()
	// first 保存由打开取得的第一项。
	first, err := persistence.Open(ctx, cfg)
	must(t, err)
	// cfg2 保存当前处理所用的服务配置2。
	cfg2 := cfg
	cfg2.OriginID++
	// second 保存由打开取得的秒。
	second, err := persistence.Open(ctx, cfg2)
	must(t, err)
	// owner 保存由打开取得的所有者。
	owner, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		// c 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		must(t, first.DB.Drop(c))
		owner.Close(c)
		second.Close(c)
		first.Close(c)
	})
	must(t, first.Singleton(ctx, "account"))
	// grant 保存由批准游戏取得的授予。
	grant, err := first.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
	must(t, err)
	owner.Epoch = grant.Epoch
	// registered 保存由登录取得的“registered”。
	registered, err := first.Login(ctx, "whorg_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	// account 保存由“authenticate”取得的账号。
	account, err := first.Authenticate(ctx, registered.AccountToken)
	must(t, err)
	// roles 创建用于维护角色集合的容器或通道。
	roles := make([]domain.Role, 2)
	// auths 创建用于维护授权集合的容器或通道。
	auths := make([]persistence.Auth, 2)
	// items 创建用于维护道具集合的容器或通道。
	items := make([]domain.Item, 2)
	// n 定位持久层存储列表中正在处理的成员，authority 接收遍历持久层存储列表时当前的元素。
	for n, authority := range []*persistence.Store{first, second} {
		// key 保存由创建角色取得的键，e 接收创建角色供后续步骤使用。
		key, e := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: authority.Cfg.OriginID, Slot: 1, Job: 0, Name: fmt.Sprintf("永久仓%d%s", n, domain.ID()[:5]), OpId: domain.Bytes(domain.ID())})
		must(t, e)
		_, e = authority.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(key.Key.RoleId)}, bson.M{"$set": bson.M{"privilege_until": time.Now().Add(time.Hour)}})
		must(t, e)
		// ticket 保存由签发入场票据取得的入场票据。
		ticket, e := authority.IssueTicket(ctx, account, key.Key)
		must(t, e)
		roles[n], auths[n], e = owner.Enter(ctx, ticket.Ticket)
		must(t, e)
		items[n] = domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: "1"}
	}
	if roles[0].AccountID != roles[1].AccountID || roles[0].Origin == roles[1].Origin || roles[0].Owner != roles[1].Owner || roles[0].RunEpoch != roles[1].RunEpoch {
		t.Fatal("不同origin同run/owner资格未建立")
	}
	// routes 记录路由集合，使用MongoDB BSON 文档列表表示。
	var routes []bson.M
	// cur 保存由查找取得的“cur”，e 接收查找供后续步骤使用。
	cur, e := owner.DB.Collection("origin_routes").Find(ctx, bson.M{})
	must(t, e)
	must(t, cur.All(ctx, &routes))
	if len(routes) != 2 {
		t.Fatal("两路既成合服映射未建立")
	}
	// apply 封装应用对应的后续处理，n 记录数值，q 记录仓库动作。
	apply := func(n int, q *pb.WarehouseAction) (*pb.OperationResult, persistence.Operation) {
		must(t, owner.Renew(ctx, "game"))
		// r 保存从角色集合取出的角色集合中的指定项。
		r := roles[n]
		q.Operation = &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(r.EconomicSeq + 1), ExpectedRevision: uint64(r.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}
		// raw 保存由序列化取得的原始值，e 接收序列化供后续步骤使用。
		raw, e := json.Marshal(q)
		must(t, e)
		// next 保存由资产带有回执取得的下一状态，res 保存由资产带有回执取得的结果，op 保存由资产带有回执取得的操作。
		next, res, op, e := owner.AssetWithReceipt(ctx, auths[n], persistence.AssetRequest{Kind: "warehouse", Context: q.Operation, Warehouse: q, Fingerprint: persistence.Hash(raw)})
		must(t, e)
		roles[n] = next
		return res, op
	}
	// query 封装查询对应的后续处理，n 记录数值。
	query := func(n int) *pb.WarehousePage {
		// v 接收查询仓库供后续步骤使用，e 接收查询仓库供后续步骤使用。
		v, e := owner.QueryWarehouse(ctx, auths[n], &pb.WarehouseQuery{Scope: 2, Limit: 1})
		must(t, e)
		return v
	}
	// n 接收遍历角色集合时当前的元素。
	for n := range roles {
		// bound 保存从道具集合取出的绑定。
		bound := roles[n].Items[0]
		// res 保存由应用取得的结果，op 保存由应用取得的操作。
		res, op := apply(n, &pb.WarehouseAction{Scope: 2, Action: 1, ItemInstanceId: domain.Bytes(bound.ID), ItemVersion: uint64(bound.Version), Quantity: bound.Quantity})
		if res.ErrorCode != p.CodeWarehouseBlocked || op.Warehouse != nil || query(n).Revision != 0 {
			t.Fatal("初始绑定物进入共享或拒绝时建仓")
		}
		// 仅此来源事实为隔离的已确认奖励资格，非自然击杀证明；WS另有真实Auto非绑掉物。
		roles[n], err = owner.Settle(ctx, auths[n], persistence.Settlement{SourceSeq: roles[n].SourceSeq + 1, Kills: 1, Gold: int64(11 + n), Item: &items[n], Bound: false})
		must(t, err)
		res, op = apply(n, &pb.WarehouseAction{Scope: 2, Action: 1, ItemInstanceId: domain.Bytes(items[n].ID), ItemVersion: 1, Quantity: 1})
		if res.ErrorCode != 0 || op.Warehouse.Transferred.Bound || op.Warehouse.Transferred.Quality != 0 {
			t.Fatal("非绑普通装备被CanTrade错误阻挡", res)
		}
		res, _ = apply(n, &pb.WarehouseAction{Scope: 2, Action: 3, WarehouseRevision: 1, Currency: 1, Amount: int64(5 + n)})
		if res.ErrorCode != 0 {
			t.Fatal(res)
		}
	}
	// n 接收遍历角色集合时当前的元素。
	for n := range roles {
		// page 保存由查询取得的页面或分页。
		page := query(n)
		if page.Revision != 2 || page.Gold != int64(5+n) || len(page.Items) != 1 || !bytes.Equal(page.Items[0].Item.InstanceId, domain.Bytes(items[n].ID)) {
			t.Fatal("同run永久origin内容或余额串仓", page)
		}
		// before 保存从角色集合取出的操作前的状态。
		before := roles[n]
		// res 保存由应用取得的结果，op 保存由应用取得的操作。
		res, op := apply(n, &pb.WarehouseAction{Scope: 2, Action: 2, WarehouseRevision: 2, ItemInstanceId: domain.Bytes(items[1-n].ID), ItemVersion: 2, Quantity: 1})
		if res.ErrorCode != p.CodeWarehouseChanged || op.Warehouse != nil || !reflect.DeepEqual(before.Items, roles[n].Items) || query(n).Revision != 2 {
			t.Fatal("跨origin原UID取出或失败改仓")
		}
		res, op = apply(n, &pb.WarehouseAction{Scope: 2, Action: 2, WarehouseRevision: 2, ItemInstanceId: domain.Bytes(items[n].ID), ItemVersion: 2, Quantity: 1})
		if res.ErrorCode != 0 || op.Warehouse.Transferred.ID != items[n].ID {
			t.Fatal("本origin正常取出失败")
		}
		res, _ = apply(n, &pb.WarehouseAction{Scope: 2, Action: 4, WarehouseRevision: 3, Currency: 1, Amount: int64(5 + n)})
		if res.ErrorCode != 0 || query(n).Gold != 0 || roles[n].Wallet.Gold != int64(11+n) {
			t.Fatal("各origin金币未独立守恒")
		}
	}
	// n 接收数量文档供后续步骤使用。
	n, e := owner.DB.Collection("warehouses").CountDocuments(ctx, bson.M{"scope": 2})
	must(t, e)
	if n != 2 {
		t.Fatal("永久仓数量错误")
	}
	t.Logf("同account origins=%d/%d均映射run=%d/node=%s/owner=%s；正常各slot1建角/票/Enter，绑定共享拒绝；明确隔离已确认掉物两件，各仓原子存取/币守恒，跨origin UID拒绝。仅验证既成路由，不声称运营合服过程。", roles[0].Origin, roles[1].Origin, cfg.RunID, cfg.NodeID, owner.Instance)
}
