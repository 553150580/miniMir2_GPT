package integration

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestFullBagContinuousSourceRewardsAndHostingReceiptsMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo满包连续来源/托管账")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion < 21 {
		t.Fatal("本候选须在下一阶段唯一Excel受控升级后应用")
	}
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
	login, err := authority.Login(ctx, "full_policy_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	account, err := authority.Authenticate(ctx, login.AccountToken)
	must(t, err)
	key, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 0, Name: "满包来源" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())})
	must(t, err)
	roleID := domain.Hex(key.Key.RoleId)
	initial, err := authority.Role(ctx, roleID)
	must(t, err)
	var original domain.Item
	for _, item := range initial.Items {
		if definition, ok := cfg.Fixture.Item(item.ItemID); ok && definition.IsEquipment {
			original = item
			break
		}
	}
	if original.ID == "" {
		t.Fatal("缺少真实新生装备资格")
	}
	original.Locked = true
	// 仅隔离库入场资格：1格已满、受保护原件、3秒余额；不预置收益/回执。
	_, err = authority.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"capacity": 1, "items": []domain.Item{original}, "hosting_seconds": 3, "pickup_stopped": true}})
	must(t, err)
	ticket, err := authority.IssueTicket(ctx, account, key.Key)
	must(t, err)
	r, auth, err := owner.Enter(ctx, ticket.Ticket)
	must(t, err)
	before := r
	monster := cfg.Fixture.Monsters[0]
	if monster.Exp <= 0 || monster.GoldMin <= 0 || len(cfg.Fixture.Drops.ItemIDs) == 0 {
		t.Fatal("本例需要当前怪物真实EXP/金币与普通道具候选")
	}
	requests := []persistence.Settlement{}
	var firstReceipt persistence.Operation
	for index := 0; index < 3; index++ {
		q := persistence.Settlement{SourceSeq: r.SourceSeq + 1, Kills: 1, Experience: monster.Exp, HostSeconds: 1, HostingSession: "full-session", NoEquipmentKills: r.NoEquipmentKills + 1, EventAt: time.Now()}
		if index == 1 {
			q.Gold = monster.GoldMin // 来源分支独立：不伪造一次kill同时掉金掉物。
		} else {
			itemID := cfg.Fixture.Drops.ItemIDs[0]
			q.Item = &domain.Item{ID: domain.ID(), ItemID: itemID, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: fmt.Sprint(q.SourceSeq)}
		}
		out, receipt, err := owner.SettleWithReceipt(ctx, auth, q)
		must(t, err)
		if !reflect.DeepEqual(out.Items, before.Items) || out.SourceSeq != q.SourceSeq || out.EconomicSeq != before.EconomicSeq || out.HostingSeconds != r.HostingSeconds-1 || receipt.Settlement == nil || receipt.Settlement.Experience.ActualGranted != monster.Exp {
			t.Fatal("真实满包来源没有继续接收EXP/扣时或修改了原保护件", out, receipt)
		}
		if q.Item != nil && (receipt.Settlement.GrantedItem || receipt.Settlement.Recycling.DiscardedCount != 1 || receipt.Settlement.Recycling.DiscardReason != "bag_full") {
			t.Fatal("真实原物品没有弃掉并记录原因", receipt)
		}
		if index < 2 && out.Report.StopReason != 0 || index == 2 && out.Report.StopReason != 1 {
			t.Fatal("真实报告把满包当停止或遮住余额耗尽", out.Report)
		}
		if index == 0 {
			// 原永久事实按实际BSON字节恢复；不把Fresh.Created的纳秒/单调时钟
			// 表示与Mongo毫秒时间直接DeepEqual，当作重复扣时的证据。
			must(t, owner.DB.Collection("operations").FindOne(ctx, bson.M{"_id": receipt.ID}).Decode(&firstReceipt))
			freshNormalized := receipt
			freshNormalized.Created = firstReceipt.Created
			t.Logf("首回执存储观察: fresh_created=%s stored_created=%s excluding_created_equal=%t", receipt.Created.Format(time.RFC3339Nano), firstReceipt.Created.Format(time.RFC3339Nano), reflect.DeepEqual(freshNormalized, firstReceipt))
		}
		requests = append(requests, q)
		r = out
	}
	if r.Report.Kills != 3 || r.Report.UsedSeconds != 3 || r.Report.Experience != monster.Exp*3 || r.Report.DiscardCount != 2 || r.Report.TotalDropCount != 2 || len(r.Report.Drops) != 0 || r.Wallet.Gold != before.Wallet.Gold+monster.GoldMin {
		t.Fatal("真实EXP/金币/弃物/托管分账不符", r)
	}
	after := r
	replayed, receipt, err := owner.SettleWithReceipt(ctx, auth, requests[0])
	must(t, err)
	if !reflect.DeepEqual(receipt, firstReceipt) || replayed.Revision != after.Revision || replayed.SourceSeq != after.SourceSeq || replayed.HostingSeconds != 0 || replayed.Wallet != after.Wallet {
		t.Fatalf("旧来源核对不符: receipt_equal=%t id=%t digest=%t source=%t revision=%t created=%t settlement=%t recycling=%t role_revision=%d/%d role_source=%d/%d hosting=%d wallet=%+v/%+v", reflect.DeepEqual(receipt, firstReceipt), receipt.ID == firstReceipt.ID, receipt.Digest == firstReceipt.Digest, receipt.SourceSeq == firstReceipt.SourceSeq, receipt.Revision == firstReceipt.Revision, receipt.Created.Equal(firstReceipt.Created), reflect.DeepEqual(receipt.Settlement, firstReceipt.Settlement), reflect.DeepEqual(receipt.Recycling, firstReceipt.Recycling), replayed.Revision, after.Revision, replayed.SourceSeq, after.SourceSeq, replayed.HostingSeconds, replayed.Wallet, after.Wallet)
	}
	changed := requests[0]
	changed.Gold++
	if _, _, err := owner.SettleWithReceipt(ctx, auth, changed); persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("同来源改内容未拒绝", err)
	}
	if _, _, err := owner.SettleWithReceipt(ctx, auth, persistence.Settlement{SourceSeq: after.SourceSeq + 1, HostSeconds: 1, HostingSession: "full-session"}); persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("余额0仍允许超扣托管", err)
	}
	t.Log("真实Mongo范围：隔离满包资格，原Store逐事件连续EXP/金币/弃物/扣时、余额0与原receipt防重；实际AI/WS由另一用例验证，不伪称正常掉率采样")
}
