package integration

import (
	"context"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"os"
	"sync"
	"testing"
	"time"
)

func (c *client) skillPreview(q *pb.SkillAssetPreviewRequest) *pb.SkillAssetPreview {
	c.t.Helper()
	request := c.send(p.MsgC2SSkillAssetPreview, q)
	f := c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSkillAssetPreview && f.Request == request })
	out := &pb.SkillAssetPreview{}
	must(c.t, proto.Unmarshal(f.Payload, out))
	return out
}
func (c *client) skillCommit(q *pb.SkillAssetCommitRequest) *pb.OperationResult {
	result := c.asset(p.MsgC2SSkillAssetCommit, q)
	c.inventoryAt(result.NewRevision)
	return result
}
func (c *client) skillConfirm(preview *pb.SkillAssetPreview) *pb.OperationResult {
	return c.skillCommit(&pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()})
}

func TestSkillAssetEconomyMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以验证购买/分解/升级真实事务")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: 4, Y: 8}
	ctx, cancel := context.WithCancel(context.Background())
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	var wg sync.WaitGroup
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("技能经济%s: %v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, a)
	}
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "skill_asset_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	skill, _ := cfg.Fixture.Skill(1)
	role := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: skill.JobID, Name: "升级事务" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	before, err := inspection.Role(ctx, roleID)
	must(t, err)
	var initial domain.Item
	for _, item := range before.Items {
		if item.ItemID == skill.BookItemID {
			initial = item
		}
	}
	if initial.SourceKind != "initial_grant" || initial.SourceID == "" {
		t.Fatal("初始职业书缺少持久来源")
	}
	var shop devconfig.SkillShop
	for _, offer := range cfg.Fixture.SkillShop {
		if offer.ItemID == skill.BookItemID && offer.Currency == "gold" {
			shop = offer
		}
	}
	var recycle devconfig.SkillBookRecycle
	for _, rule := range cfg.Fixture.SkillBookRecycle {
		if rule.ItemID == skill.BookItemID {
			recycle = rule
			break
		}
	}
	books, pages := int64(0), int64(0)
	required := skill.LearnLevel
	for _, row := range cfg.Fixture.SkillUpgrades {
		if row.SkillID == skill.ID {
			for _, cost := range row.BookCosts {
				books += cost.Quantity
			}
			pages += row.PageCost
			if row.RequiredRoleLevel > required {
				required = row.RequiredRoleLevel
			}
		}
	}
	if shop.ID == 0 || recycle.PagesPerBook <= 0 || books <= 0 || pages <= 0 {
		t.Fatal("缺少唯一Excel经济候选")
	}
	// 仅随机测试库准备合法来源前态及资金，不修改正在浏览器验收的角色。
	totalDropBooks := (pages + recycle.PagesPerBook - 1) / recycle.PagesPerBook
	drops := []domain.Item{}
	for n := int64(0); n < totalDropBooks; n++ {
		drops = append(drops, domain.Item{ID: domain.ID(), ItemID: skill.BookItemID, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: domain.ID(), Quality: recycle.BookQuality, Bound: n%2 == 0})
	}
	preparedItems := append(append([]domain.Item(nil), before.Items...), drops...)
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"level": required, "items": preparedItems, "wallet": domain.Wallet{Gold: shop.PriceUnits * books * 2, BoundGold: shop.PriceUnits - 1, Yuanbao: 1000000, BoundYuanbao: 1000000}}})
	must(t, err)
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer func() { c.close() }()
	c.auto(false, 1)
	learn := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: domain.Bytes(initial.ID), SkillId: skill.ID, Operation: c.operation()})
	c.inventoryAt(learn.NewRevision)
	if learn.ErrorCode != 0 {
		t.Fatal("准备零级学习失败")
	}
	quantity := func(itemID uint32) int64 {
		total := int64(0)
		for _, item := range c.inventory.Items {
			if item.ItemId == itemID {
				total += item.Quantity
			}
		}
		return total
	}
	stored := func() domain.Role { r, err := inspection.Role(ctx, roleID); must(t, err); return r }

	// 新预览替换旧预览；失败本身有永久回执，但不消费商品费用。
	old := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: 1})
	c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: 1})
	walletBefore := stored().Wallet
	if result := c.skillConfirm(old); result.ErrorCode != p.CodeSkillPreviewStale || stored().Wallet != walletBefore {
		t.Fatal("被替换旧预览仍可购买或已扣费")
	}
	purchase := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: 1})
	if purchase.Spend.BoundGold != shop.PriceUnits-1 || purchase.Spend.Gold != 1 {
		t.Fatal("真实预览未显示绑定优先钱包拆分")
	}
	original := &pb.SkillAssetCommitRequest{PreviewId: purchase.PreviewId, Operation: c.operation()}
	result := c.skillCommit(original)
	if result.ErrorCode != 0 {
		t.Fatalf("真实购买失败:%v", result)
	}
	walletAfter := stored().Wallet
	// 已提交回执必须先于过期/已删除预览检查，并保留真实原结果。
	_, err = inspection.DB.Collection("skill_quotes").DeleteOne(ctx, bson.M{"_id": roleID})
	must(t, err)
	if replay := c.skillCommit(original); replay.ErrorCode != 0 || stored().Wallet != walletAfter {
		t.Fatal("删除预览后原op无法核对或重复扣币")
	}
	changed := proto.Clone(original).(*pb.SkillAssetCommitRequest)
	changed.PreviewId = domain.Bytes(domain.ID())
	if r := c.skillCommit(changed); r.ErrorCode != p.CodeOperationConflict {
		t.Fatal("同op改变preview未拒绝")
	}
	count, err := inspection.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(original.Operation.OpId), "skill_plan.kind": 1})
	must(t, err)
	if count != 1 {
		t.Fatal("购买未永久保存单一精确计划审计")
	}
	var npc *pb.ItemView
	for _, item := range c.inventory.Items {
		if item.ItemId == skill.BookItemID && item.SourceKind == "npc_shop" {
			npc = item
			break
		}
	}
	if npc == nil || !npc.Bound || npc.Quality != 0 || npc.SourceId != domain.Hex(original.Operation.OpId) {
		t.Fatal("购买商品来源/绑定/品质没有持久同步")
	}
	blocked := c.send(p.MsgC2SSkillAssetPreview, &pb.SkillAssetPreviewRequest{Kind: 3, Materials: []*pb.ItemConsumption{{InstanceId: npc.InstanceId, Version: npc.Version, Quantity: 1}}})
	c.expectError(blocked, p.CodeSkillBookSource)

	// 买足每阶所需书本；商品数量按配置限额分批，价格和币种不在测试旁造另一套。
	remaining := books - 1
	for remaining > 0 {
		n := remaining
		if n > shop.MaxPurchaseQuantity {
			n = shop.MaxPurchaseQuantity
		}
		preview := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: n})
		if result := c.skillConfirm(preview); result.ErrorCode != 0 {
			t.Fatal("补购升级书失败")
		}
		remaining -= n
	}
	recycleRequest := &pb.SkillAssetPreviewRequest{Kind: 3}
	for _, item := range drops {
		recycleRequest.Materials = append(recycleRequest.Materials, &pb.ItemConsumption{InstanceId: domain.Bytes(item.ID), Version: 1, Quantity: 1})
	}
	pagePreview := c.skillPreview(recycleRequest)
	if len(pagePreview.Outputs) != len(drops) {
		t.Fatal("不同原书错误合并输出来源")
	}
	bound, unbound := false, false
	for _, item := range pagePreview.Outputs {
		bound = bound || item.Bound
		unbound = unbound || !item.Bound
	}
	if !bound || !unbound {
		t.Fatal("混合分解把不同绑定合成单一绑定")
	}
	pageOp := &pb.SkillAssetCommitRequest{PreviewId: pagePreview.PreviewId, Operation: c.operation()}
	if result := c.skillCommit(pageOp); result.ErrorCode != 0 {
		t.Fatal("来源书分解失败")
	}
	for _, item := range c.inventory.Items {
		if item.ItemId == cfg.Fixture.SkillEconomy.PageItemID && (item.SourceKind != "skill_recycle" || item.SourceId != domain.Hex(pageOp.Operation.OpId)) {
			t.Fatal("书页输出丢失来源")
		}
	}
	for target := skill.LearnedLevel + 1; target <= skill.MaxLevel; target++ {
		row, _ := cfg.Fixture.Upgrade(skill.ID, target)
		need := map[uint32]int64{}
		for _, cost := range row.BookCosts {
			need[cost.ItemID] += cost.Quantity
		}
		need[row.PageItemID] += row.PageCost
		request := &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: skill.ID}
		for _, item := range c.inventory.Items {
			n := need[item.ItemId]
			if n <= 0 {
				continue
			}
			if n > item.Quantity {
				n = item.Quantity
			}
			request.Materials = append(request.Materials, &pb.ItemConsumption{InstanceId: item.InstanceId, Version: item.Version, Quantity: n})
			need[item.ItemId] -= n
		}
		preview := c.skillPreview(request)
		if target == skill.LearnedLevel+1 {
			// 非材料资产版本变化也必须拒绝旧报价，不能默默换成新revision提交。
			claim := c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{BenefitKind: 0, Operation: c.operation()})
			c.inventoryAt(claim.NewRevision)
			if claim.ErrorCode != 0 {
				t.Fatal("构造报价旧版本前态失败")
			}
			walletBefore = stored().Wallet
			if stale := c.skillConfirm(preview); stale.ErrorCode != p.CodeSkillPreviewStale || stored().Wallet != walletBefore {
				t.Fatal("角色版本已变化仍接受旧报价")
			}
			preview = c.skillPreview(request)
		}
		if preview.FromLevel != target-1 || preview.ToLevel != target || len(preview.Stats) != 3 {
			t.Fatal("升级预览未包含真实前后等级/数值")
		}
		bookBefore, pageBefore := quantity(skill.BookItemID), quantity(row.PageItemID)
		rankOp := &pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()}
		if r := c.skillCommit(rankOp); r.ErrorCode != 0 {
			t.Fatalf("rank%d真实升级失败:%v", target, r)
		}
		if quantity(skill.BookItemID) != bookBefore-int64(target)-1 || quantity(row.PageItemID) != pageBefore-row.PageCost || c.inventory.LearnedSkills[0].Level != target {
			t.Fatal("升级没有逐阶精确扣书/扣页/保存技能等级")
		}
		if r := c.skillCommit(rankOp); r.ErrorCode != 0 || quantity(skill.BookItemID) != bookBefore-int64(target)-1 {
			t.Fatal("升级重传重复消耗")
		}
	}
	capped := c.send(p.MsgC2SSkillAssetPreview, &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: skill.ID})
	c.expectError(capped, p.CodeSkillUpgradeBlocked)
	// 同一报价的不同op并发竞争：只允许一笔真实扣款。
	concurrent := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: 1})
	walletBefore = stored().Wallet
	a := &pb.SkillAssetCommitRequest{PreviewId: concurrent.PreviewId, Operation: c.operation()}
	b := proto.Clone(a).(*pb.SkillAssetCommitRequest)
	b.Operation.OpId = domain.Bytes(domain.ID())
	ra, rb := c.send(p.MsgC2SSkillAssetCommit, a), c.send(p.MsgC2SSkillAssetCommit, b)
	results := map[uint32]*pb.OperationResult{}
	for len(results) < 2 {
		f := c.read()
		if f.ID == p.MsgS2COperationResult && (f.Request == ra || f.Request == rb) {
			r := &pb.OperationResult{}
			must(t, proto.Unmarshal(f.Payload, r))
			results[f.Request] = r
			if r.CommandSeq > c.watermark {
				c.watermark = r.CommandSeq
			}
		}
	}
	if results[ra].ErrorCode != 0 || results[rb].ErrorCode != p.CodeOperationConflict {
		t.Fatal("同报价并发没有单一成功与明确冲突")
	}
	c.inventoryAt(results[ra].NewRevision)
	if stored().Wallet.Gold != walletBefore.Gold-shop.PriceUnits {
		t.Fatal("并发购买重复扣费")
	}
	expired := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: 1})
	walletBefore = stored().Wallet
	_, err = inspection.DB.Collection("skill_quotes").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"expires_at": time.Unix(0, 0)}})
	must(t, err)
	if r := c.skillConfirm(expired); r.ErrorCode != p.CodeSkillPreviewStale || stored().Wallet != walletBefore {
		t.Fatal("过期预览扣费或未拒绝")
	}
	for _, offer := range cfg.Fixture.SkillShop {
		if offer.Currency == "gold" {
			continue
		}
		beforeWallet := stored().Wallet
		preview := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: 1})
		currencyRequest := &pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()}
		if offer.Currency == "ingot" {
			// 服务端已提交但客户端不读回执，再按原op重连核对；不冒称提交未知故障注入。
			c.send(p.MsgC2SSkillAssetCommit, currencyRequest)
			committed := false
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				n, err := inspection.DB.Collection("operations").CountDocuments(ctx, bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(currencyRequest.Operation.OpId)})
				must(t, err)
				if n == 1 {
					committed = true
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !committed {
				t.Fatal("丢回执测试未等到真实提交")
			}
			c.close()
			c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
			c.auto(false, 1)
		}
		if r := c.skillCommit(currencyRequest); r.ErrorCode != 0 {
			t.Fatal("明确币种购买书页失败")
		}
		afterWallet := stored().Wallet
		if offer.Currency == "ingot" && (afterWallet.Yuanbao != beforeWallet.Yuanbao-offer.PriceUnits || afterWallet.BoundYuanbao != beforeWallet.BoundYuanbao) {
			t.Fatal("元宝购买偷偷改扣绑元")
		}
		if offer.Currency == "bound_ingot" && (afterWallet.BoundYuanbao != beforeWallet.BoundYuanbao-offer.PriceUnits || afterWallet.Yuanbao != beforeWallet.Yuanbao) {
			t.Fatal("绑元购买偷偷改扣元宝")
		}
	}
	other := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 2, Job: skill.JobID, Name: "报价归属" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, other, 0)
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(other.Key.RoleId)}, bson.M{"$set": bson.M{"wallet": domain.Wallet{Gold: shop.PriceUnits}, "level": required}})
	must(t, err)
	otherClient := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, other.Key))
	defer otherClient.close()
	otherClient.auto(false, 1)
	foreign := otherClient.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: 1})
	walletBefore = stored().Wallet
	if r := c.skillConfirm(foreign); r.ErrorCode != p.CodeSkillPreviewStale || stored().Wallet != walletBefore {
		t.Fatal("同账号另一角色可消费外来报价")
	}
	if r := otherClient.skillConfirm(foreign); r.ErrorCode != 0 {
		t.Fatal("外来角色尝试破坏了真实归属角色的报价")
	}
	// 完成永久资产操作及关闭Auto不等于原移动/动作锁已经到期。
	// 沿真实基线观察合法StateBlocked后再请求；不修改业务时钟或补造可施法状态。
	hero := proto.Clone(c.player()).(*pb.EntityState)
	t.Logf("满级实际施法前：scene=%d stateSequence=%d auto=%v HP=%d MP=%d movementRevision=%d movementKind=%d action=%d attackSequence=%d castSequence=%d", c.entry.SceneEpoch, c.scene.StateSequence, c.hosting.AutoBattleEnabled, hero.Hp, hero.Mp, hero.MovementRevision, hero.MovementKind, hero.Action, hero.AttackSequence, hero.CastSequence)
	accepted := levelGrowthCastWhenReady(c, skill.ID, 10)
	if accepted.SceneEpoch != c.entry.SceneEpoch || accepted.SkillId != skill.ID || accepted.SkillLevel != skill.MaxLevel || accepted.CasterHandle != hero.EntityHandle || accepted.CasterLife != hero.Life || accepted.TargetHandle != 10 {
		t.Fatal("实际满级施法没有保留原附着/身体/目标身份", accepted)
	}
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	c.auto(false, 1)
	if c.inventory.LearnedSkills[0].Level != skill.MaxLevel {
		t.Fatal("重连丢失满级技能")
	}
	if r := c.skillCommit(original); r.ErrorCode != 0 {
		t.Fatal("重连后的历史购买回执不可核对")
	}
	t.Log("通过：来源与绑定、三钱包明确购买、旧预览、永久原op防重、NPC反套利、掉落书分解、0→3逐阶2/3/4本与页精确消耗、满级/施法/重连")
}
