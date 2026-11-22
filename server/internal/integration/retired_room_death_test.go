package integration

import (
	"context"
	"testing"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

func TestRetiredRoomDeathMarkerMongo(t *testing.T) {
	ctx, cfg, _ := receiptMigrationDB(t)
	account, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() { account.Close(context.Background()) })
	game, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() { game.Close(context.Background()) })
	must(t, account.Singleton(ctx, "account"))
	run, err := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
	must(t, err)
	game.Epoch = run.Epoch
	login, err := account.Login(ctx, "retired_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	authorization, err := account.Authenticate(ctx, login.AccountToken)
	must(t, err)
	created, err := account.CreateRole(ctx, authorization, &pb.CreateRoleRequest{
		OriginServerId: cfg.OriginID, Slot: 1, Name: "退役验证" + domain.ID()[:8], OpId: domain.Bytes(domain.ID()),
	})
	must(t, err)
	enter := func() (domain.Role, persistence.Auth) {
		ticket, issueErr := account.IssueTicket(ctx, authorization, created.Key)
		must(t, issueErr)
		role, auth, enterErr := game.Enter(ctx, ticket.Ticket)
		must(t, enterErr)
		return role, auth
	}
	original, first := enter()
	deathID := domain.ID()
	must(t, game.PreserveRetiredDeath(ctx, first, deathID))
	must(t, game.PreserveRetiredDeath(ctx, first, deathID))
	cold, current := enter()
	if cold.RetiredDeathID != deathID || cold.SourceSeq != original.SourceSeq ||
		cold.Wallet != original.Wallet || cold.EconomicSeq != original.EconomicSeq {
		t.Fatalf("退役或新票改变原死亡选择、永久资产或流水：%+v", cold)
	}
	if err = game.PreserveRetiredDeath(ctx, first, deathID); persistence.Code(err) != p.CodeOperationConflict {
		t.Fatalf("旧会话不应能写退役标记：%v", err)
	}
	revive := func(role domain.Role, requestedDeath string) (domain.Role, *pb.OperationResult) {
		out, result, reviveErr := game.Asset(ctx, current, persistence.AssetRequest{
			Kind: "revive", Fingerprint: requestedDeath, ValidRevive: true, ReviveDeathID: requestedDeath,
			Context: &pb.OperationContext{
				OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
				ExpectedRevision: uint64(role.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion,
			},
		})
		must(t, reviveErr)
		return out, result
	}
	rejected, result := revive(cold, domain.ID())
	if result.ErrorCode != p.CodeInvalidArgument || rejected.RetiredDeathID != deathID {
		t.Fatal("错误死亡标记不应清除原死亡选择")
	}
	revived, result := revive(rejected, deathID)
	if result.ErrorCode != 0 || revived.RetiredDeathID != "" {
		t.Fatal("原复活选择应随原子提交清除死亡标记")
	}
	reentered, _ := enter()
	if reentered.RetiredDeathID != "" {
		t.Fatal("确认复活后的新票不应恢复已清除的死亡标记")
	}
}
