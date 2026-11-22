// 验证死亡房间回收后保留原死亡选择，并与冷重入及成功复活衔接。

package integration

import (
	"context"
	"testing"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestRetiredRoomDeathMarkerMongo 让死亡选项跨房间回收保留，并检查原复活完成后的持久标记。
// t 提供断言失败报告、子用例和清理注册。
func TestRetiredRoomDeathMarkerMongo(t *testing.T) {
	// ctx 保存由回执迁移数据库取得的上下文，cfg 保存由回执迁移数据库取得的服务配置。
	ctx, cfg, _ := receiptMigrationDB(t)
	// account 保存由打开取得的账号，err 接收打开过程中可能产生的错误，供后续失败分支处理。
	account, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() { account.Close(context.Background()) })
	// game 保存由打开取得的游戏。
	game, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() { game.Close(context.Background()) })
	must(t, account.Singleton(ctx, "account"))
	// run 保存由批准游戏取得的运行批次。
	run, err := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
	must(t, err)
	game.Epoch = run.Epoch
	// login 接收登录供后续步骤使用。
	login, err := account.Login(ctx, "retired_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	// authorization 保存由“authenticate”取得的“authorization”。
	authorization, err := account.Authenticate(ctx, login.AccountToken)
	must(t, err)
	// created 保存由创建角色取得的“created”。
	created, err := account.CreateRole(ctx, authorization, &pb.CreateRoleRequest{
		OriginServerId: cfg.OriginID, Slot: 1, Name: "退役验证" + domain.ID()[:8], OpId: domain.Bytes(domain.ID()),
	})
	must(t, err)
	// enter 封装入场对应的后续处理。
	enter := func() (domain.Role, persistence.Auth) {
		// ticket 保存由签发入场票据取得的入场票据，issueErr 保存由签发入场票据取得的签发错误。
		ticket, issueErr := account.IssueTicket(ctx, authorization, created.Key)
		must(t, issueErr)
		// role 保存由入场取得的角色，auth 保存由入场取得的授权，enterErr 保存由入场取得的入场错误。
		role, auth, enterErr := game.Enter(ctx, ticket.Ticket)
		must(t, enterErr)
		return role, auth
	}
	// original 保存由入场取得的原始，first 保存由入场取得的第一项。
	original, first := enter()
	// deathID 标识当前业务使用的死亡。
	deathID := domain.ID()
	must(t, game.PreserveRetiredDeath(ctx, first, deathID))
	must(t, game.PreserveRetiredDeath(ctx, first, deathID))
	// cold 保存由入场取得的“cold”，current 保存由入场取得的当前状态。
	cold, current := enter()
	if cold.RetiredDeathID != deathID || cold.SourceSeq != original.SourceSeq ||
		cold.Wallet != original.Wallet || cold.EconomicSeq != original.EconomicSeq {
		t.Fatalf("退役或新票改变原死亡选择、永久资产或流水：%+v", cold)
	}
	if err = game.PreserveRetiredDeath(ctx, first, deathID); persistence.Code(err) != p.CodeOperationConflict {
		t.Fatalf("旧会话不应能写退役标记：%v", err)
	}
	// revive 封装复活对应的后续处理，role 记录角色，requestedDeath 记录已请求死亡，使用文本表示。
	revive := func(role domain.Role, requestedDeath string) (domain.Role, *pb.OperationResult) {
		// out 保存由资产取得的输出，result 保存由资产取得的结果，reviveErr 保存由资产取得的复活错误。
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
	// rejected 保存由复活取得的拒绝，result 保存由复活取得的结果。
	rejected, result := revive(cold, domain.ID())
	if result.ErrorCode != p.CodeInvalidArgument || rejected.RetiredDeathID != deathID {
		t.Fatal("错误死亡标记不应清除原死亡选择")
	}
	// revived 保存由复活取得的“revived”。
	revived, result := revive(rejected, deathID)
	if result.ErrorCode != 0 || revived.RetiredDeathID != "" {
		t.Fatal("原复活选择应随原子提交清除死亡标记")
	}
	// reentered 保存由入场取得的“reentered”。
	reentered, _ := enter()
	if reentered.RetiredDeathID != "" {
		t.Fatal("确认复活后的新票不应恢复已清除的死亡标记")
	}
}
