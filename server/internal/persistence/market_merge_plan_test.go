// 本文件用独立 MongoDB 验证合服清退计划的精确版本、原租约和封账门禁。
package persistence

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestMarketMergePlanAuthorityMongo 核对闭门、排空、清退、原版本回执和恢复都不会发布新路由。
func TestMarketMergePlanAuthorityMongo(t *testing.T) {
	ctx, account, openGame := goldMarketBackgroundSetup(t)
	game := openGame()
	source, target := account.Cfg.RunID, account.Cfg.RunID+1
	require := func(err error) { t.Helper(); goldMarketRequire(t, err) }
	_, err := account.DB.Collection("run_servers").InsertOne(ctx, runAssignment{RunID: target, GameNodeID: account.Cfg.NodeID,
		GameWSURL: "ws://" + account.Cfg.GameAddress + "/ws", AssignmentVersion: 3, State: "open"})
	require(err)
	_, err = account.DB.Collection("run_control").InsertOne(ctx, Run{ID: target, State: "stopped", LeaseUntil: time.Unix(0, 0)})
	require(err)
	extraOrigin, targetOrigin := account.Cfg.OriginID+100, account.Cfg.OriginID+200
	_, err = account.DB.Collection("origin_routes").InsertMany(ctx, []originRoute{
		{OriginID: extraOrigin, RunID: source, Version: 4}, {OriginID: targetOrigin, RunID: target, Version: 5},
	})
	require(err)
	type guardSnapshot struct {
		Owner string `bson:"owner_instance"`  // 实际原运行服写入实例。
		Epoch int64  `bson:"run_epoch"`       // 实际原组的所有权代次。
		View  int64  `bson:"control_version"` // 实际原组控制版本。
		Seq   int64  `bson:"seq"`             // 市场冻结必须实际推进，无需替换组身份。
	}
	guards := func(runID uint64) [runGuardGroups]guardSnapshot {
		t.Helper()
		var out [runGuardGroups]guardSnapshot
		for group := range out {
			require(account.DB.Collection("run_guards").FindOne(ctx, bson.M{"_id": fmt.Sprintf("%d:%d", runID, group)}).Decode(&out[group]))
		}
		return out
	}
	code := func(err error, expected uint32) {
		t.Helper()
		if Code(err) != expected {
			t.Fatalf("错误码 %d，预期 %d：%v", Code(err), expected, err)
		}
	}

	old, err := account.PrepareMarketMerge(ctx, domain.ID(), source, target)
	require(err)
	if len(old.Assignments) != 2 || len(old.Origins) != 3 || old.Version != 1 {
		t.Fatal("预演未固定两服全部永久入口", old)
	}
	unchanged, err := account.PrepareMarketMerge(ctx, old.ID, source, target)
	require(err)
	if !reflect.DeepEqual(old, unchanged) {
		t.Fatal("计划重播改变原快照", old, unchanged)
	}
	_, err = account.PrepareMarketMerge(ctx, old.ID, target, source)
	code(err, p.CodeOperationConflict)
	_, err = account.DB.Collection("origin_routes").UpdateByID(ctx, extraOrigin, bson.M{"$inc": bson.M{"route_version": 1}})
	require(err)
	before := guards(source)
	_, err = account.FreezeMarketMerge(ctx, old.ID, old.Version)
	code(err, p.CodeOperationConflict)
	gateCount, err := account.DB.Collection(marketMergeGateCollection).CountDocuments(ctx, bson.M{})
	require(err)
	if gateCount != 0 || guards(source) != before {
		t.Fatal("入口漂移拒绝没有回滚全部冻结写入", gateCount)
	}

	plan, err := account.PrepareMarketMerge(ctx, domain.ID(), source, target)
	require(err)
	other, err := account.PrepareMarketMerge(ctx, domain.ID(), source, target)
	require(err)
	owner := MarketParty{RoleID: domain.ID(), AccountID: domain.ID(), Origin: extraOrigin}
	now := bossTime(time.Now())
	order := seedGoldMarketOrder(t, ctx, game, owner, GoldMarketSell, 91, 2, 1, now.Add(-time.Minute), now.Add(time.Hour), true)
	frozen, err := account.FreezeMarketMerge(ctx, plan.ID, plan.Version)
	require(err)
	if frozen.State != MarketMergeFrozen || frozen.Version != 2 || frozen.FrozenAt == nil {
		t.Fatal("未保存首次冻结计划", frozen)
	}
	_, err = account.FreezeMarketMerge(ctx, other.ID, other.Version)
	code(err, p.CodeOperationConflict)
	frozenAgain, err := account.FreezeMarketMerge(ctx, plan.ID, plan.Version)
	require(err)
	if !reflect.DeepEqual(frozen, frozenAgain) {
		t.Fatal("冻结原版本重播没有返回原记录", frozenAgain)
	}
	after := guards(source)
	for index, oldGuard := range before {
		oldGuard.Seq++
		if after[index] != oldGuard {
			t.Fatal("冻结未推进某组序列，或改写了原所有者视图", index, before[index], after[index])
		}
	}
	for _, guard := range guards(target) {
		if guard != (guardSnapshot{Seq: 1}) {
			t.Fatal("未曾注册的目标服未写入空身份组序列", guard)
		}
	}
	require(game.Renew(ctx, "game"))
	_, err = account.liveRoute(ctx, extraOrigin)
	code(err, p.CodeLineRecovering)
	requestID := domain.ID()
	_, err = account.StepMarketMerge(ctx, plan.ID, frozen.Version, requestID)
	code(err, p.CodeLineRecovering)
	_, err = account.SealMarketMerge(ctx, plan.ID, frozen.Version)
	code(err, p.CodeLineRecovering)
	require(game.Release(ctx, "game"))
	_, err = account.SealMarketMerge(ctx, plan.ID, frozen.Version)
	code(err, p.CodeOperationConflict)

	step, err := account.StepMarketMerge(ctx, plan.ID, frozen.Version, requestID)
	require(err)
	if step.Done || step.Asset == nil || step.Asset.SourceID != order.ID || step.Version != 3 {
		t.Fatal("实际原订单余量未被同事务清退", step)
	}
	var claim GoldMarketClaim
	require(account.DB.Collection(goldMarketClaims).FindOne(ctx, bson.M{"_id": step.Asset.ClaimID}).Decode(&claim))
	if claim.Owner != owner || claim.Gold != 10_000 || claim.Yuanbao != 0 {
		t.Fatal("清退的剩余非绑金币没有原路托管", claim)
	}
	_, err = account.StepMarketMerge(ctx, plan.ID, frozen.Version, domain.ID())
	code(err, p.CodeOperationConflict)
	done, err := account.StepMarketMerge(ctx, plan.ID, step.Version, domain.ID())
	require(err)
	if !done.Done || done.Asset != nil || done.Version != 4 {
		t.Fatal("计划无法确认已处理完全部市场记录", done)
	}
	replay, err := account.StepMarketMerge(ctx, plan.ID, frozen.Version, requestID)
	require(err)
	if !reflect.DeepEqual(replay, step) {
		t.Fatal("原步骤回执被当前计划水位覆盖", replay, step)
	}
	sealed, err := account.SealMarketMerge(ctx, plan.ID, done.Version)
	require(err)
	if sealed.State != MarketMergeSealed || sealed.Proof == nil || sealed.Version != 5 || sealed.Steps != 1 {
		t.Fatal("首次封账没有原证明及实际处理数", sealed)
	}
	verified, err := account.VerifyMarketMergeRouteRelease(ctx, plan.ID, sealed.Version)
	require(err)
	if !reflect.DeepEqual(sealed, verified) {
		t.Fatal("重新核证没有返回原市场证明", verified, sealed)
	}

	newOrigin := extraOrigin + 1
	_, err = account.DB.Collection("origin_routes").InsertOne(ctx, originRoute{OriginID: newOrigin, RunID: source, Version: 1})
	require(err)
	_, err = account.VerifyMarketMergeRouteRelease(ctx, plan.ID, sealed.Version)
	code(err, p.CodeOperationConflict)
	_, err = account.RestoreMarketMerge(ctx, plan.ID, sealed.Version)
	code(err, p.CodeOperationConflict)
	_, err = account.DB.Collection("origin_routes").DeleteOne(ctx, bson.M{"_id": newOrigin})
	require(err)
	restored, err := account.RestoreMarketMerge(ctx, plan.ID, sealed.Version)
	require(err)
	if restored.State != MarketMergeRestored || restored.Proof != nil || restored.Version != 6 || restored.RestoredAt == nil {
		t.Fatal("恢复原区服后仍可误用旧发行证明", restored)
	}
	gateCount, err = account.DB.Collection(marketMergeGateCollection).CountDocuments(ctx, bson.M{})
	require(err)
	if gateCount != 0 {
		t.Fatal("原计划恢复仍留有市场门禁", gateCount)
	}
	_, err = account.VerifyMarketMergeRouteRelease(ctx, plan.ID, restored.Version)
	code(err, p.CodeOperationConflict)
	restoredAgain, err := account.RestoreMarketMerge(ctx, plan.ID, sealed.Version)
	require(err)
	if !reflect.DeepEqual(restoredAgain, restored) {
		t.Fatal("恢复原区服重播改变了已确认结果", restoredAgain)
	}
	replacement := openGame()
	if replacement.Epoch <= game.Epoch {
		t.Fatal("恢复原区服没有等待原租约换代", game.Epoch, replacement.Epoch)
	}
	defer replacement.Release(ctx, "game")
}
