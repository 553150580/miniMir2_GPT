package integration

import (
	"context"
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

func TestMongoOwnershipFence(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("真实开发副本集需显式MINIMIR_MONGO_E2E=1")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	ctx := context.Background()
	authority, err := persistence.Open(ctx, cfg)
	must(t, err)
	old, err := persistence.Open(ctx, cfg)
	must(t, err)
	next, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		must(t, authority.DB.Drop(deadline))
		old.Close(deadline)
		next.Close(deadline)
		authority.Close(deadline)
	})
	must(t, authority.Singleton(ctx, "account"))
	grant, err := authority.ApproveGame(ctx, cfg.NodeID, old.Instance, cfg.RunID)
	must(t, err)
	old.Epoch = grant.Epoch
	if _, err = authority.ApproveGame(ctx, cfg.NodeID, next.Instance, cfg.RunID); persistence.Code(err) != p.CodeServerBusy {
		t.Fatalf("有效owner被抢占: %v", err)
	}
	registered, err := authority.Login(ctx, "fence_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	account, err := authority.Authenticate(ctx, registered.AccountToken)
	must(t, err)
	role, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 0, Name: "屏障" + domain.ID()[:8], OpId: domain.Bytes(domain.ID())})
	must(t, err)
	ticket, err := authority.IssueTicket(ctx, account, role.Key)
	must(t, err)
	before, oldAuth, err := old.Enter(ctx, ticket.Ticket)
	must(t, err)
	must(t, old.Release(ctx, "game"))
	grant, err = authority.ApproveGame(ctx, cfg.NodeID, next.Instance, cfg.RunID)
	must(t, err)
	next.Epoch = grant.Epoch
	if grant.Epoch != old.Epoch+1 {
		t.Fatal("交接没有递增run_epoch")
	}
	_, err = old.Settle(ctx, oldAuth, persistence.Settlement{SourceSeq: before.SourceSeq + 1, Gold: 30, Bound: true})
	if persistence.Code(err) != p.CodeRouteChanged {
		t.Fatalf("旧owner在新guard后仍可写: %v", err)
	}
	after, err := authority.Role(ctx, before.ID)
	must(t, err)
	if after.Revision != before.Revision || after.Wallet.BoundGold != before.Wallet.BoundGold {
		t.Fatal("旧owner资产写穿透了换代屏障")
	}
	newTicket, err := authority.IssueTicket(ctx, account, role.Key)
	must(t, err)
	current, newAuth, err := next.Enter(ctx, newTicket.Ticket)
	must(t, err)
	settlement := persistence.Settlement{SourceSeq: current.SourceSeq + 1, Gold: 30, Bound: true}
	committed, err := next.Settle(ctx, newAuth, settlement)
	must(t, err)
	replay, err := next.Settle(ctx, newAuth, settlement)
	must(t, err)
	if replay.Wallet.BoundGold != committed.Wallet.BoundGold || replay.Revision != committed.Revision {
		t.Fatal("原来源游标重复结算")
	}
	_, err = next.Settle(ctx, oldAuth, persistence.Settlement{SourceSeq: committed.SourceSeq + 1, Gold: 30, Bound: true})
	if persistence.Code(err) != p.CodeStaleSession {
		t.Fatalf("新owner接受旧绑定: %v", err)
	}
	guards, err := authority.DB.Collection("run_guards").CountDocuments(ctx, bson.M{"run_epoch": next.Epoch, "owner_instance": next.Instance})
	must(t, err)
	if guards != 32 {
		t.Fatalf("换代guard数=%d", guards)
	}
	// 满包不是任意按Price回收：不发币、不入袋，单列实际弃物。
	// 本例没有领取托管余额，HostingSeconds=0，终止原因应是耗尽1而非满包3。
	_, err = authority.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": replay.ID}, bson.M{"$set": bson.M{"capacity": len(replay.Items)}})
	must(t, err)
	full, err := next.Settle(ctx, newAuth, persistence.Settlement{SourceSeq: replay.SourceSeq + 1, Bound: true, Item: &domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Bound: true, Version: 1}, HostingSession: domain.ID()})
	must(t, err)
	if !full.PickupStopped || full.LastDiscardItem != 218 || !full.LastDiscardBound || full.HostingSeconds != 0 || full.Report.StopReason != 1 || full.Report.DiscardCount != 1 || full.Report.LastDiscardReason != "bag_full" || full.Report.TotalDropCount != 1 || full.Wallet != replay.Wallet || len(full.Items) != len(replay.Items) || !reflect.DeepEqual(full.Items, replay.Items) {
		t.Fatalf("满包弃物/余额0原因不符，或擅改钱包/原库存:%+v", full)
	}
	released, result, err := next.Asset(ctx, newAuth, persistence.AssetRequest{Kind: "recycle", Fingerprint: "test_full_bag_release", Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(full.EconomicSeq + 1), ExpectedRevision: uint64(full.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}, ItemIDs: []string{full.Items[0].ID}})
	must(t, err)
	if result.ErrorCode != 0 || released.PickupStopped {
		t.Fatal("清理背包后没有解除拾取阻塞状态")
	}
	must(t, next.Release(ctx, "game"))
	t.Log("通过：有效owner拒抢、停止→32组guard换代、旧owner写被拒、旧session被拒、原收益游标只提交一次")
}
