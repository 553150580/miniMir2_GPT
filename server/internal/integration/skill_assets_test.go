// 通过正式开发协议检查技能资产报价、提交和幂等重放，读取实际数据库资产结果。

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

// skillPreview 返回技能预览供调用方使用。
// c 是调用该方法的客户端实例，q 记录技能资产预览请求。
func (c *client) skillPreview(q *pb.SkillAssetPreviewRequest) *pb.SkillAssetPreview {
	c.t.Helper()
	// request 保存由发送取得的请求。
	request := c.send(p.MsgC2SSkillAssetPreview, q)
	// f 接收有效截止时间供后续步骤使用，f 记录协议帧。
	f := c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSkillAssetPreview && f.Request == request })
	// out 保存当前处理所用的输出。
	out := &pb.SkillAssetPreview{}
	must(c.t, proto.Unmarshal(f.Payload, out))
	return out
}

// skillCommit 返回技能提交供调用方使用。
// c 是调用该方法的客户端实例，q 记录技能资产提交请求。
func (c *client) skillCommit(q *pb.SkillAssetCommitRequest) *pb.OperationResult {
	// result 保存由资产取得的结果。
	result := c.asset(p.MsgC2SSkillAssetCommit, q)
	c.inventoryAt(result.NewRevision)
	return result
}

// skillConfirm 返回技能确认供调用方使用。
// c 是调用该方法的客户端实例，preview 记录预览，使用技能资产预览表示。
func (c *client) skillConfirm(preview *pb.SkillAssetPreview) *pb.OperationResult {
	return c.skillCommit(&pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()})
}

// TestSkillAssetEconomyMongoWS 经真实消息执行技能资产预览及提交，并核对相同操作重放的经济结果。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillAssetEconomyMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以验证购买/分解/升级真实事务")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: 4, Y: 8}
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancel(context.Background())
	// inspection 保存由打开取得的检查。
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	// wg 记录“wg”，使用协程汇合计数器表示。
	var wg sync.WaitGroup
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			// err 接收运行批次过程中可能产生的错误，供后续失败分支处理。
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("技能经济%s: %v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	// a 接收遍历文本列表时当前的元素。
	for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, a)
	}
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "skill_asset_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	// skill 接收技能供后续步骤使用。
	skill, _ := cfg.Fixture.Skill(1)
	// role 保存当前处理所用的角色。
	role := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: skill.JobID, Name: "升级事务" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, role, 0)
	// roleID 标识当前业务使用的角色。
	roleID := domain.Hex(role.Key.RoleId)
	// before 保存由角色取得的操作前的状态。
	before, err := inspection.Role(ctx, roleID)
	must(t, err)
	// initial 记录初始值，使用道具表示。
	var initial domain.Item
	// item 接收遍历道具集合时当前的元素。
	for _, item := range before.Items {
		if item.ItemID == skill.BookItemID {
			initial = item
		}
	}
	if initial.SourceKind != "initial_grant" || initial.SourceID == "" {
		t.Fatal("初始职业书缺少持久来源")
	}
	// shop 记录商店，使用技能商店表示。
	var shop devconfig.SkillShop
	// offer 接收遍历技能商店时当前的元素。
	for _, offer := range cfg.Fixture.SkillShop {
		if offer.ItemID == skill.BookItemID && offer.Currency == "gold" {
			shop = offer
		}
	}
	// recycle 记录回收，使用技能技能书回收表示。
	var recycle devconfig.SkillBookRecycle
	// rule 接收遍历技能技能书回收时当前的元素。
	for _, rule := range cfg.Fixture.SkillBookRecycle {
		if rule.ItemID == skill.BookItemID {
			recycle = rule
			break
		}
	}
	// books 保存由整数“64”取得的技能书集合，pages 保存由整数“64”取得的“pages”。
	books, pages := int64(0), int64(0)
	// required 保存学习等级，用于当前步骤中的“required”。
	required := skill.LearnLevel
	// row 接收遍历技能“upgrades”时当前的元素。
	for _, row := range cfg.Fixture.SkillUpgrades {
		if row.SkillID == skill.ID {
			// cost 接收遍历技能书“costs”时当前的元素。
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
	// totalDropBooks 保存根据“pages”与“pages”“per”技能书的计算结果与当前数据的计算结果与“pages”“per”技能书计算的合计值掉落技能书集合。
	totalDropBooks := (pages + recycle.PagesPerBook - 1) / recycle.PagesPerBook
	// drops 初始化用作掉落集合的道具列表。
	drops := []domain.Item{}
	// n 接收整数“64”供后续步骤使用。
	for n := int64(0); n < totalDropBooks; n++ {
		drops = append(drops, domain.Item{ID: domain.ID(), ItemID: skill.BookItemID, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: domain.ID(), Quality: recycle.BookQuality, Bound: n%2 == 0})
	}
	// preparedItems 保留加入新条目后的“prepared”道具集合。
	preparedItems := append(append([]domain.Item(nil), before.Items...), drops...)
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"level": required, "items": preparedItems, "wallet": domain.Wallet{Gold: shop.PriceUnits * books * 2, BoundGold: shop.PriceUnits - 1, Yuanbao: 1000000, BoundYuanbao: 1000000}}})
	must(t, err)
	// c 接收加入供后续步骤使用。
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer func() { c.close() }()
	c.auto(false, 1)
	// learn 保存由资产取得的学习。
	learn := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: domain.Bytes(initial.ID), SkillId: skill.ID, Operation: c.operation()})
	c.inventoryAt(learn.NewRevision)
	if learn.ErrorCode != 0 {
		t.Fatal("准备零级学习失败")
	}
	// quantity 封装数量对应的后续处理，itemID 标识当前业务使用的道具。
	quantity := func(itemID uint32) int64 {
		// total 保存由整数“64”取得的合计。
		total := int64(0)
		// item 接收遍历道具集合时当前的元素。
		for _, item := range c.inventory.Items {
			if item.ItemId == itemID {
				total += item.Quantity
			}
		}
		return total
	}
	// stored 封装已持久保存对应的后续处理，r 接收角色供后续步骤使用，err 接收角色过程中可能产生的错误，供后续失败分支处理。
	stored := func() domain.Role { r, err := inspection.Role(ctx, roleID); must(t, err); return r }

	// 新预览替换旧预览；失败本身有永久回执，但不消费商品费用。
	// old 保存由技能预览取得的原状态。
	old := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: 1})
	c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: 1})
	// walletBefore 保存钱包，用于当前步骤中的钱包操作前的状态。
	walletBefore := stored().Wallet
	// result 保存由技能确认取得的结果。
	if result := c.skillConfirm(old); result.ErrorCode != p.CodeSkillPreviewStale || stored().Wallet != walletBefore {
		t.Fatal("被替换旧预览仍可购买或已扣费")
	}
	// purchase 保存由技能预览取得的“purchase”。
	purchase := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: 1})
	if purchase.Spend.BoundGold != shop.PriceUnits-1 || purchase.Spend.Gold != 1 {
		t.Fatal("真实预览未显示绑定优先钱包拆分")
	}
	// original 保存当前处理所用的原始。
	original := &pb.SkillAssetCommitRequest{PreviewId: purchase.PreviewId, Operation: c.operation()}
	// result 保存由技能提交取得的结果。
	result := c.skillCommit(original)
	if result.ErrorCode != 0 {
		t.Fatalf("真实购买失败:%v", result)
	}
	// walletAfter 保存钱包，用于当前步骤中的钱包操作后的状态。
	walletAfter := stored().Wallet
	// 已提交回执必须先于过期/已删除预览检查，并保留真实原结果。
	_, err = inspection.DB.Collection("skill_quotes").DeleteOne(ctx, bson.M{"_id": roleID})
	must(t, err)
	// replay 保存由技能提交取得的重放。
	if replay := c.skillCommit(original); replay.ErrorCode != 0 || stored().Wallet != walletAfter {
		t.Fatal("删除预览后原op无法核对或重复扣币")
	}
	// changed 按目标类型读取当前接口值，得到“changed”。
	changed := proto.Clone(original).(*pb.SkillAssetCommitRequest)
	changed.PreviewId = domain.Bytes(domain.ID())
	// r 接收技能提交供后续步骤使用。
	if r := c.skillCommit(changed); r.ErrorCode != p.CodeOperationConflict {
		t.Fatal("同op改变preview未拒绝")
	}
	// count 保存由数量文档取得的数量。
	count, err := inspection.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(original.Operation.OpId), "skill_plan.kind": 1})
	must(t, err)
	if count != 1 {
		t.Fatal("购买未永久保存单一精确计划审计")
	}
	// npc 记录NPC，使用道具观察视图表示。
	var npc *pb.ItemView
	// item 接收遍历道具集合时当前的元素。
	for _, item := range c.inventory.Items {
		if item.ItemId == skill.BookItemID && item.SourceKind == "npc_shop" {
			npc = item
			break
		}
	}
	if npc == nil || !npc.Bound || npc.Quality != 0 || npc.SourceId != domain.Hex(original.Operation.OpId) {
		t.Fatal("购买商品来源/绑定/品质没有持久同步")
	}
	// blocked 保存由发送取得的阻挡。
	blocked := c.send(p.MsgC2SSkillAssetPreview, &pb.SkillAssetPreviewRequest{Kind: 3, Materials: []*pb.ItemConsumption{{InstanceId: npc.InstanceId, Version: npc.Version, Quantity: 1}}})
	c.expectError(blocked, p.CodeSkillBookSource)

	// 买足每阶所需书本；商品数量按配置限额分批，价格和币种不在测试旁造另一套。
	// remaining 保存根据技能书集合与当前数据计算的剩余额度。
	remaining := books - 1
	for remaining > 0 {
		// n 保存当前处理所用的剩余额度。
		n := remaining
		if n > shop.MaxPurchaseQuantity {
			n = shop.MaxPurchaseQuantity
		}
		// preview 保存由技能预览取得的预览。
		preview := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: n})
		// result 保存由技能确认取得的结果。
		if result := c.skillConfirm(preview); result.ErrorCode != 0 {
			t.Fatal("补购升级书失败")
		}
		remaining -= n
	}
	// recycleRequest 保存当前处理所用的回收请求。
	recycleRequest := &pb.SkillAssetPreviewRequest{Kind: 3}
	// item 接收遍历掉落集合时当前的元素。
	for _, item := range drops {
		recycleRequest.Materials = append(recycleRequest.Materials, &pb.ItemConsumption{InstanceId: domain.Bytes(item.ID), Version: 1, Quantity: 1})
	}
	// pagePreview 保存由技能预览取得的页面或分页预览。
	pagePreview := c.skillPreview(recycleRequest)
	if len(pagePreview.Outputs) != len(drops) {
		t.Fatal("不同原书错误合并输出来源")
	}
	// bound 保存当前处理所用的绑定，unbound 保存当前处理所用的未绑定。
	bound, unbound := false, false
	// item 接收遍历“outputs”时当前的元素。
	for _, item := range pagePreview.Outputs {
		bound = bound || item.Bound
		unbound = unbound || !item.Bound
	}
	if !bound || !unbound {
		t.Fatal("混合分解把不同绑定合成单一绑定")
	}
	// pageOp 保存当前处理所用的页面或分页操作。
	pageOp := &pb.SkillAssetCommitRequest{PreviewId: pagePreview.PreviewId, Operation: c.operation()}
	// result 保存由技能提交取得的结果。
	if result := c.skillCommit(pageOp); result.ErrorCode != 0 {
		t.Fatal("来源书分解失败")
	}
	// item 接收遍历道具集合时当前的元素。
	for _, item := range c.inventory.Items {
		if item.ItemId == cfg.Fixture.SkillEconomy.PageItemID && (item.SourceKind != "skill_recycle" || item.SourceId != domain.Hex(pageOp.Operation.OpId)) {
			t.Fatal("书页输出丢失来源")
		}
	}
	// target 保存根据已学习等级与当前数据计算的目标。
	for target := skill.LearnedLevel + 1; target <= skill.MaxLevel; target++ {
		// row 保存由升级取得的行记录。
		row, _ := cfg.Fixture.Upgrade(skill.ID, target)
		// need 初始化用作所需量的按数值索引的数值映射。
		need := map[uint32]int64{}
		// cost 接收遍历技能书“costs”时当前的元素。
		for _, cost := range row.BookCosts {
			need[cost.ItemID] += cost.Quantity
		}
		need[row.PageItemID] += row.PageCost
		// request 保存当前处理所用的请求。
		request := &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: skill.ID}
		// item 接收遍历道具集合时当前的元素。
		for _, item := range c.inventory.Items {
			// n 保存从所需量取出的所需量中的指定项。
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
		// preview 保存由技能预览取得的预览。
		preview := c.skillPreview(request)
		if target == skill.LearnedLevel+1 {
			// 非材料资产版本变化也必须拒绝旧报价，不能默默换成新revision提交。
			// claim 保存由资产取得的领取。
			claim := c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{BenefitKind: 0, Operation: c.operation()})
			c.inventoryAt(claim.NewRevision)
			if claim.ErrorCode != 0 {
				t.Fatal("构造报价旧版本前态失败")
			}
			walletBefore = stored().Wallet
			// stale 保存由技能确认取得的已失效。
			if stale := c.skillConfirm(preview); stale.ErrorCode != p.CodeSkillPreviewStale || stored().Wallet != walletBefore {
				t.Fatal("角色版本已变化仍接受旧报价")
			}
			preview = c.skillPreview(request)
		}
		if preview.FromLevel != target-1 || preview.ToLevel != target || len(preview.Stats) != 3 {
			t.Fatal("升级预览未包含真实前后等级/数值")
		}
		// bookBefore 保存由数量取得的技能书操作前的状态，pageBefore 保存由数量取得的页面或分页操作前的状态。
		bookBefore, pageBefore := quantity(skill.BookItemID), quantity(row.PageItemID)
		// rankOp 保存当前处理所用的排序位次操作。
		rankOp := &pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()}
		// r 接收技能提交供后续步骤使用。
		if r := c.skillCommit(rankOp); r.ErrorCode != 0 {
			t.Fatalf("rank%d真实升级失败:%v", target, r)
		}
		if quantity(skill.BookItemID) != bookBefore-int64(target)-1 || quantity(row.PageItemID) != pageBefore-row.PageCost || c.inventory.LearnedSkills[0].Level != target {
			t.Fatal("升级没有逐阶精确扣书/扣页/保存技能等级")
		}
		// r 接收技能提交供后续步骤使用。
		if r := c.skillCommit(rankOp); r.ErrorCode != 0 || quantity(skill.BookItemID) != bookBefore-int64(target)-1 {
			t.Fatal("升级重传重复消耗")
		}
	}
	// capped 保存由发送取得的“capped”。
	capped := c.send(p.MsgC2SSkillAssetPreview, &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: skill.ID})
	c.expectError(capped, p.CodeSkillUpgradeBlocked)
	// 同一报价的不同op并发竞争：只允许一笔真实扣款。
	// concurrent 保存由技能预览取得的并发。
	concurrent := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: 1})
	walletBefore = stored().Wallet
	// a 保存当前处理所用的a。
	a := &pb.SkillAssetCommitRequest{PreviewId: concurrent.PreviewId, Operation: c.operation()}
	// b 按目标类型读取当前接口值，得到b。
	b := proto.Clone(a).(*pb.SkillAssetCommitRequest)
	b.Operation.OpId = domain.Bytes(domain.ID())
	// ra 保存由发送取得的“ra”，rb 保存由发送取得的“rb”。
	ra, rb := c.send(p.MsgC2SSkillAssetCommit, a), c.send(p.MsgC2SSkillAssetCommit, b)
	// results 初始化用作结果集合的按数值索引的操作结果映射。
	results := map[uint32]*pb.OperationResult{}
	for len(results) < 2 {
		// f 接收读取供后续步骤使用。
		f := c.read()
		if f.ID == p.MsgS2COperationResult && (f.Request == ra || f.Request == rb) {
			// r 保存当前处理所用的操作结果。
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
	// expired 保存由技能预览取得的“expired”。
	expired := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: 1})
	walletBefore = stored().Wallet
	_, err = inspection.DB.Collection("skill_quotes").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"expires_at": time.Unix(0, 0)}})
	must(t, err)
	// r 接收技能确认供后续步骤使用。
	if r := c.skillConfirm(expired); r.ErrorCode != p.CodeSkillPreviewStale || stored().Wallet != walletBefore {
		t.Fatal("过期预览扣费或未拒绝")
	}
	// offer 接收遍历技能商店时当前的元素。
	for _, offer := range cfg.Fixture.SkillShop {
		if offer.Currency == "gold" {
			continue
		}
		// beforeWallet 保存钱包，用于当前步骤中的操作前的状态钱包。
		beforeWallet := stored().Wallet
		// preview 保存由技能预览取得的预览。
		preview := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: 1})
		// currencyRequest 保存当前处理所用的“currency”请求。
		currencyRequest := &pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()}
		if offer.Currency == "ingot" {
			// 服务端已提交但客户端不读回执，再按原op重连核对；不冒称提交未知故障注入。
			c.send(p.MsgC2SSkillAssetCommit, currencyRequest)
			// committed 保存当前处理所用的已提交。
			committed := false
			// deadline 保存由新增取得的截止时间。
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				// n 接收数量文档供后续步骤使用，err 接收数量文档过程中可能产生的错误，供后续失败分支处理。
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
		// r 接收技能提交供后续步骤使用。
		if r := c.skillCommit(currencyRequest); r.ErrorCode != 0 {
			t.Fatal("明确币种购买书页失败")
		}
		// afterWallet 保存钱包，用于当前步骤中的操作后的状态钱包。
		afterWallet := stored().Wallet
		if offer.Currency == "ingot" && (afterWallet.Yuanbao != beforeWallet.Yuanbao-offer.PriceUnits || afterWallet.BoundYuanbao != beforeWallet.BoundYuanbao) {
			t.Fatal("元宝购买偷偷改扣绑元")
		}
		if offer.Currency == "bound_ingot" && (afterWallet.BoundYuanbao != beforeWallet.BoundYuanbao-offer.PriceUnits || afterWallet.Yuanbao != beforeWallet.Yuanbao) {
			t.Fatal("绑元购买偷偷改扣元宝")
		}
	}
	// other 保存当前处理所用的另一项。
	other := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 2, Job: skill.JobID, Name: "报价归属" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, other, 0)
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(other.Key.RoleId)}, bson.M{"$set": bson.M{"wallet": domain.Wallet{Gold: shop.PriceUnits}, "level": required}})
	must(t, err)
	// otherClient 保存由加入取得的另一项客户端。
	otherClient := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, other.Key))
	defer otherClient.close()
	otherClient.auto(false, 1)
	// foreign 保存由技能预览取得的关联对象。
	foreign := otherClient.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: shop.ID, Quantity: 1})
	walletBefore = stored().Wallet
	// r 接收技能确认供后续步骤使用。
	if r := c.skillConfirm(foreign); r.ErrorCode != p.CodeSkillPreviewStale || stored().Wallet != walletBefore {
		t.Fatal("同账号另一角色可消费外来报价")
	}
	// r 接收技能确认供后续步骤使用。
	if r := otherClient.skillConfirm(foreign); r.ErrorCode != 0 {
		t.Fatal("外来角色尝试破坏了真实归属角色的报价")
	}
	// 完成永久资产操作及关闭Auto不等于原移动/动作锁已经到期。
	// 沿真实基线观察合法StateBlocked后再请求；不修改业务时钟或补造可施法状态。
	// hero 按目标类型读取当前接口值，得到“hero”。
	hero := proto.Clone(c.player()).(*pb.EntityState)
	t.Logf("满级实际施法前：scene=%d stateSequence=%d auto=%v HP=%d MP=%d movementRevision=%d movementKind=%d action=%d attackSequence=%d castSequence=%d", c.entry.SceneEpoch, c.scene.StateSequence, c.hosting.AutoBattleEnabled, hero.Hp, hero.Mp, hero.MovementRevision, hero.MovementKind, hero.Action, hero.AttackSequence, hero.CastSequence)
	// accepted 保存由等级成长施法“when”就绪取得的已受理。
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
	// r 接收技能提交供后续步骤使用。
	if r := c.skillCommit(original); r.ErrorCode != 0 {
		t.Fatal("重连后的历史购买回执不可核对")
	}
	t.Log("通过：来源与绑定、三钱包明确购买、旧预览、永久原op防重、NPC反套利、掉落书分解、0→3逐阶2/3/4本与页精确消耗、满级/施法/重连")
}
