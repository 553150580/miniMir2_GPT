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
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func TestWarehouseConcurrentFifthRoleAndSixthRejectedMongoHTTP(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo/HTTP抢第五槽")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_wh5_" + domain.ID()
	cfg.AccountAddress = address(t)
	ctx, cancel := context.WithCancel(context.Background())
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	var group sync.WaitGroup
	group.Go(func() {
		if e := bootstrap.Run(ctx, cfg, "account"); e != nil && ctx.Err() == nil {
			t.Errorf("account:%v", e)
		}
	})
	t.Cleanup(func() {
		cancel()
		group.Wait()
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	healthy(t, cfg.AccountAddress)
	url := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, url, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "wh5_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	for slot := uint32(1); slot <= 4; slot++ {
		rpc(t, url, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: slot, Job: 0, Name: fmt.Sprintf("原槽%d%s", slot, domain.ID()[:5]), OpId: domain.Bytes(domain.ID())}, &pb.RoleSummary{}, 0)
	}
	queries := []*pb.CreateRoleRequest{{OriginServerId: cfg.OriginID, Slot: 5, Job: 0, Name: "竞五甲" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, {OriginServerId: cfg.OriginID, Slot: 5, Job: 1, Name: "竞五乙" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, {OriginServerId: cfg.OriginID, Slot: 6, Job: 2, Name: "竞六" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}}
	type response struct {
		frame wire.Frame
		role  *pb.RoleSummary
		err   error
	}
	exchange := func(q *pb.CreateRoleRequest) response {
		req, e := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(wire.Encode(wire.Message(p.MsgC2SCreateRole, 1, 0, q))))
		if e != nil {
			return response{err: e}
		}
		req.Header.Set("Authorization", "Bearer "+login.AccountToken)
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			return response{err: e}
		}
		defer res.Body.Close()
		raw, e := io.ReadAll(res.Body)
		if e != nil {
			return response{err: e}
		}
		f, e := wire.Decode(raw)
		if e != nil {
			return response{err: e}
		}
		role := &pb.RoleSummary{}
		if f.Error == 0 {
			e = proto.Unmarshal(f.Payload, role)
		}
		return response{f, role, e}
	}
	gate := make(chan struct{})
	answers := make([]response, len(queries))
	var racers sync.WaitGroup
	for n, q := range queries {
		racers.Go(func() { <-gate; answers[n] = exchange(q) })
	}
	close(gate)
	racers.Wait()
	winner := -1
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
		} else if a.frame.Error != p.CodeOperationConflict {
			t.Fatal("第五槽非预期终态", a.frame.Error)
		}
	}
	if winner < 0 {
		t.Fatal("第五槽没有赢家")
	}
	for n, q := range queries {
		retry := exchange(q)
		must(t, retry.err)
		if retry.frame.Error != answers[n].frame.Error || retry.frame.Error == 0 && !proto.Equal(retry.role, answers[n].role) {
			t.Fatal("原操作重放终态/角色身份改变")
		}
	}
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
	t.Logf("原1..4后同时发两个不同op抢5及第三请求6：赢家%d，其余4301/1001；原op均重放稳定；目录恰5。", winner)
}

func TestWarehousePermanentOriginsShareRunButNotAssetsMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo永久origin隔离")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_whorg_" + domain.ID()
	ctx := context.Background()
	first, err := persistence.Open(ctx, cfg)
	must(t, err)
	cfg2 := cfg
	cfg2.OriginID++
	second, err := persistence.Open(ctx, cfg2)
	must(t, err)
	owner, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		must(t, first.DB.Drop(c))
		owner.Close(c)
		second.Close(c)
		first.Close(c)
	})
	must(t, first.Singleton(ctx, "account"))
	grant, err := first.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
	must(t, err)
	owner.Epoch = grant.Epoch
	registered, err := first.Login(ctx, "whorg_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	account, err := first.Authenticate(ctx, registered.AccountToken)
	must(t, err)
	roles := make([]domain.Role, 2)
	auths := make([]persistence.Auth, 2)
	items := make([]domain.Item, 2)
	for n, authority := range []*persistence.Store{first, second} {
		key, e := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: authority.Cfg.OriginID, Slot: 1, Job: 0, Name: fmt.Sprintf("永久仓%d%s", n, domain.ID()[:5]), OpId: domain.Bytes(domain.ID())})
		must(t, e)
		_, e = authority.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(key.Key.RoleId)}, bson.M{"$set": bson.M{"privilege_until": time.Now().Add(time.Hour)}})
		must(t, e)
		ticket, e := authority.IssueTicket(ctx, account, key.Key)
		must(t, e)
		roles[n], auths[n], e = owner.Enter(ctx, ticket.Ticket)
		must(t, e)
		items[n] = domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: "1"}
	}
	if roles[0].AccountID != roles[1].AccountID || roles[0].Origin == roles[1].Origin || roles[0].Owner != roles[1].Owner || roles[0].RunEpoch != roles[1].RunEpoch {
		t.Fatal("不同origin同run/owner资格未建立")
	}
	var routes []bson.M
	cur, e := owner.DB.Collection("origin_routes").Find(ctx, bson.M{})
	must(t, e)
	must(t, cur.All(ctx, &routes))
	if len(routes) != 2 {
		t.Fatal("两路既成合服映射未建立")
	}
	apply := func(n int, q *pb.WarehouseAction) (*pb.OperationResult, persistence.Operation) {
		must(t, owner.Renew(ctx, "game"))
		r := roles[n]
		q.Operation = &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(r.EconomicSeq + 1), ExpectedRevision: uint64(r.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}
		raw, e := json.Marshal(q)
		must(t, e)
		next, res, op, e := owner.AssetWithReceipt(ctx, auths[n], persistence.AssetRequest{Kind: "warehouse", Context: q.Operation, Warehouse: q, Fingerprint: persistence.Hash(raw)})
		must(t, e)
		roles[n] = next
		return res, op
	}
	query := func(n int) *pb.WarehousePage {
		v, e := owner.QueryWarehouse(ctx, auths[n], &pb.WarehouseQuery{Scope: 2, Limit: 1})
		must(t, e)
		return v
	}
	for n := range roles {
		bound := roles[n].Items[0]
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
	for n := range roles {
		page := query(n)
		if page.Revision != 2 || page.Gold != int64(5+n) || len(page.Items) != 1 || !bytes.Equal(page.Items[0].Item.InstanceId, domain.Bytes(items[n].ID)) {
			t.Fatal("同run永久origin内容或余额串仓", page)
		}
		before := roles[n]
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
	n, e := owner.DB.Collection("warehouses").CountDocuments(ctx, bson.M{"scope": 2})
	must(t, e)
	if n != 2 {
		t.Fatal("永久仓数量错误")
	}
	t.Logf("同account origins=%d/%d均映射run=%d/node=%s/owner=%s；正常各slot1建角/票/Enter，绑定共享拒绝；明确隔离已确认掉物两件，各仓原子存取/币守恒，跨origin UID拒绝。仅验证既成路由，不声称运营合服过程。", roles[0].Origin, roles[1].Origin, cfg.RunID, cfg.NodeID, owner.Instance)
}
