// 验证背包容量限制下技能页合并，核对拆分产物的来源、实例版本和永久回执。

package integration

import (
	"context"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

// TestSkillPageMergeFullBagSourceReceiptsMongoWS 在满包前态合并技能页，验证产出来源与永久操作结果。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPageMergeFullBagSourceReceiptsMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo/WS同源书页合并与原回执核对")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
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
				t.Errorf("书页合并%s: %v", mode, err)
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
	// endpoint 接收遍历文本列表时当前的元素。
	for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, endpoint)
	}
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "page_merge_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	// rule 保存从技能技能书回收取出的规则。
	rule := cfg.Fixture.SkillBookRecycle[0]
	// book 保存由道具取得的技能书，known 保存由道具取得的已知状态。
	book, known := cfg.Fixture.Item(rule.ItemID)
	// skill 接收技能供后续步骤使用，learned 保存由技能取得的已学习。
	skill, learned := cfg.Fixture.Skill(book.BookSkillID)
	if !known || !learned || rule.PagesPerBook <= 0 || rule.PageItemID != cfg.Fixture.SkillEconomy.PageItemID {
		t.Fatal("必须使用当前唯一Excel的真实原书及拆页定义")
	}
	// key 保存当前处理所用的键。
	key := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: skill.JobID, Name: "合并书页" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, key, 0)
	// roleID 标识当前业务使用的角色。
	roleID := domain.Hex(key.Key.RoleId)
	// initial 保存由角色取得的初始值。
	initial, err := inspection.Role(ctx, roleID)
	must(t, err)
	// 仅随机库的入场资格：四本配置允许的掉落原书和满包前态。
	// 不预置任何书页、预览、回执或合并结果；两批拆书均走正常WS。
	// prepared 保留加入新条目后的“prepared”。
	prepared := append([]domain.Item(nil), initial.Items...)
	// books 初始化用作技能书集合的道具列表。
	books := []domain.Item{}
	// n 规定测试技能页面或分页合并完整或满额背包来源回执集合MongoDBWebSocket的当前数值的初始取值。
	for n := 0; n < 4; n++ {
		// item 组装道具所需的数据。
		item := domain.Item{ID: domain.ID(), ItemID: rule.ItemID, Quantity: 1, Version: 1, Bound: true, SourceKind: "monster_drop", SourceID: domain.ID(), Quality: rule.BookQuality}
		books = append(books, item)
		prepared = append(prepared, item)
	}
	for len(prepared) < initial.Capacity {
		prepared = append(prepared, domain.Item{ID: domain.ID(), ItemID: rule.ItemID, Quantity: 1, Version: 1, Bound: true, SourceKind: "npc_shop", SourceID: domain.ID()})
	}
	if len(prepared) != initial.Capacity {
		t.Fatal("当前新生库存不能容纳本用例的四本原书")
	}
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"items": prepared, "pickup_stopped": true}})
	must(t, err)
	// c 接收加入供后续步骤使用。
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, key.Key))
	defer func() { c.close() }()
	c.auto(false, 1)
	// stored 封装已持久保存对应的后续处理，r 接收角色供后续步骤使用，err 接收角色过程中可能产生的错误，供后续失败分支处理。
	stored := func() domain.Role { r, err := inspection.Role(ctx, roleID); must(t, err); return r }
	// before 保存由已持久保存取得的操作前的状态。
	before := stored()
	// recycle 封装回收对应的后续处理，items 记录道具集合，使用道具列表表示。
	recycle := func(items []domain.Item) string {
		// q 保存当前处理所用的技能资产预览请求。
		q := &pb.SkillAssetPreviewRequest{Kind: 3}
		// item 接收遍历道具集合时当前的元素。
		for _, item := range items {
			q.Materials = append(q.Materials, &pb.ItemConsumption{InstanceId: domain.Bytes(item.ID), Version: uint64(item.Version), Quantity: item.Quantity})
		}
		// preview 保存由技能预览取得的预览。
		preview := c.skillPreview(q)
		if len(preview.Outputs) != len(items) {
			t.Fatal("拆书被隐式合并")
		}
		// request 保存当前处理所用的请求。
		request := &pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()}
		// result 保存由技能提交取得的结果。
		if result := c.skillCommit(request); result.ErrorCode != 0 {
			t.Fatal("正常拆书失败", result)
		}
		return domain.Hex(request.Operation.OpId)
	}
	// firstSource 保存由回收取得的第一项来源，secondSource 保存由回收取得的秒来源。
	firstSource, secondSource := recycle(books[:2]), recycle(books[2:])
	// pages 封装“pages”对应的后续处理，source 记录来源，使用文本表示。
	pages := func(source string) []*pb.ItemView {
		// out 初始化用作输出的道具观察视图列表。
		out := []*pb.ItemView{}
		// item 接收遍历道具集合时当前的元素。
		for _, item := range c.inventory.Items {
			if item.ItemId == rule.PageItemID && item.SourceKind == "skill_recycle" && item.SourceId == source {
				out = append(out, item)
			}
		}
		return out
	}
	// requestFor 封装请求“for”对应的后续处理，items 记录道具集合，使用道具观察视图列表表示。
	requestFor := func(items []*pb.ItemView) *pb.SkillAssetPreviewRequest {
		// q 保存当前处理所用的技能资产预览请求。
		q := &pb.SkillAssetPreviewRequest{Kind: 4}
		// item 接收遍历道具集合时当前的元素。
		for _, item := range items {
			q.Materials = append(q.Materials, &pb.ItemConsumption{InstanceId: item.InstanceId, Version: item.Version, Quantity: item.Quantity})
		}
		return q
	}
	// first 保存由“pages”取得的第一项，second 保存由“pages”取得的秒。
	first, second := pages(firstSource), pages(secondSource)
	if len(first) != 2 || len(second) != 2 || len(c.inventory.Items) != int(c.inventory.Capacity) || first[0].Quantity != rule.PagesPerBook || first[1].Quantity != rule.PagesPerBook {
		t.Fatal("真实拆书未产生两批各自独立的满包书页")
	}
	// unchanged 保存由已持久保存取得的“unchanged”。
	unchanged := stored()
	// blocked 保存由发送取得的阻挡。
	blocked := c.send(p.MsgC2SSkillAssetPreview, requestFor([]*pb.ItemView{first[0], second[0]}))
	c.expectError(blocked, p.CodeSkillMaterials)
	// after 保存由已持久保存取得的操作后的状态。
	if after := stored(); after.Revision != unchanged.Revision || !reflect.DeepEqual(after.Items, unchanged.Items) {
		t.Fatal("跨SourceID合并改写了资产")
	}
	// 正常装备锁定推进角色版本，使既有预览过期；书页没有锁定RPC。
	// equipment 记录装备，使用道具观察视图表示。
	var equipment *pb.ItemView
	// item 接收遍历道具集合时当前的元素。
	for _, item := range c.inventory.Items {
		// definition 保存由道具取得的定义，ok 表明查询、校验或类型转换是否成功。
		if definition, ok := cfg.Fixture.Item(item.ItemId); ok && definition.IsEquipment {
			equipment = item
			break
		}
	}
	if equipment == nil {
		t.Fatal("缺少正常装备锁定可用的新生装备")
	}
	// stale 保存由技能预览取得的已失效。
	stale := c.skillPreview(requestFor(first))
	// lock 保存当前处理所用的锁定。
	lock := &pb.SetItemLockRequest{ItemInstanceId: equipment.InstanceId, ItemVersion: equipment.Version, Locked: true, Operation: c.operation()}
	// locked 保存由资产取得的已锁定。
	locked := c.asset(p.MsgC2SSetItemLock, lock)
	c.inventoryAt(locked.NewRevision)
	if locked.ErrorCode != 0 {
		t.Fatal("正常锁定失败", locked)
	}
	// staleRequest 保存当前处理所用的已失效请求。
	staleRequest := &pb.SkillAssetCommitRequest{PreviewId: stale.PreviewId, Operation: c.operation()}
	// result 保存由技能提交取得的结果。
	if result := c.skillCommit(staleRequest); result.ErrorCode != p.CodeSkillPreviewStale {
		t.Fatal("锁定后旧预览仍可消耗原件", result)
	}
	// staleReceipt 记录已失效回执，使用操作表示。
	var staleReceipt persistence.Operation
	must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(staleRequest.Operation.OpId)}).Decode(&staleReceipt))
	if staleReceipt.Skill != nil || len(stored().Items) != initial.Capacity {
		t.Fatal("失败提交伪造成功计划或改变槽位")
	}
	// partial 保存由请求“for”取得的部分结果。
	partial := requestFor(pages(firstSource))
	partial.Materials[0].Quantity--
	blocked = c.send(p.MsgC2SSkillAssetPreview, partial)
	c.expectError(blocked, p.CodeSkillMaterials)
	// item 接收遍历道具集合时当前的元素。
	for _, item := range c.inventory.Items {
		if domain.Hex(item.InstanceId) == domain.Hex(equipment.InstanceId) && item.Locked {
			// result 保存由资产取得的结果。
			result := c.asset(p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: item.InstanceId, ItemVersion: item.Version, Locked: false, Operation: c.operation()})
			c.inventoryAt(result.NewRevision)
			if result.ErrorCode != 0 {
				t.Fatal("正常解锁失败", result)
			}
		}
	}
	first = pages(firstSource)
	// preview 保存由技能预览取得的预览。
	preview := c.skillPreview(requestFor(first))
	if len(preview.Materials) != 2 || len(preview.Outputs) != 1 || preview.Outputs[0].SourceId != firstSource || preview.Outputs[0].Quantity != 2*rule.PagesPerBook {
		t.Fatal("合并预览没有冻结实际完整stack或来源", preview)
	}
	// preMerge 保存由已持久保存取得的“pre”合并。
	preMerge := stored()
	// request 保存当前处理所用的请求。
	request := &pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()}
	// result 保存由技能提交取得的结果。
	if result := c.skillCommit(request); result.ErrorCode != 0 {
		t.Fatal("满包同源合并失败", result)
	}
	// after 保存由已持久保存取得的操作后的状态。
	after := stored()
	// merged 保存由“pages”取得的合并结果。
	merged := pages(firstSource)
	if len(after.Items) != initial.Capacity-1 || len(merged) != 1 || merged[0].Quantity != 2*rule.PagesPerBook || merged[0].Version != 1 || !merged[0].Bound || merged[0].Quality != first[0].Quality || after.PickupStopped || after.Wallet != before.Wallet || after.SourceSeq != before.SourceSeq || after.Revision != preMerge.Revision+1 || after.EconomicSeq != preMerge.EconomicSeq+1 {
		t.Fatal("合并数量、来源、满包解除或永久水位不符", after, merged)
	}
	// old 接收遍历第一项时当前的元素。
	for _, old := range first {
		if domain.Hex(old.InstanceId) == domain.Hex(merged[0].InstanceId) {
			t.Fatal("输出复用了已完整消耗的原身份")
		}
	}
	// opID 标识当前业务使用的操作。
	opID := "asset:" + roleID + ":" + domain.Hex(request.Operation.OpId)
	// receipt 记录回执，使用操作表示。
	var receipt persistence.Operation
	must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": opID}).Decode(&receipt))
	if receipt.Skill == nil || receipt.Skill.Kind != 4 || len(receipt.Skill.Materials) != 2 || len(receipt.Skill.Outputs) != 1 || receipt.Skill.Outputs[0].SourceID != firstSource || receipt.Skill.Outputs[0].ID != domain.Hex(merged[0].InstanceId) {
		t.Fatal("typed原回执缺少精确输入输出", receipt)
	}
	// material 接收遍历材料集合时当前的元素。
	for _, material := range receipt.Skill.Materials {
		if material.Quantity != material.Item.Quantity || material.Item.SourceID != firstSource || material.Item.SourceKind != "skill_recycle" {
			t.Fatal("typed原回执丢失完整原堆叠", receipt)
		}
	}
	// count 保存由数量文档取得的数量。
	count, err := inspection.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": opID, "skill_plan.kind": 4, "skill_plan.outputs.0.source_id": firstSource, "skill_plan.outputs.0.instance_id": receipt.Skill.Outputs[0].ID})
	must(t, err)
	if count != 1 {
		t.Fatal("合并未保存唯一原计划审计")
	}
	_, err = inspection.DB.Collection("skill_quotes").DeleteOne(ctx, bson.M{"_id": roleID})
	must(t, err)
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, key.Key))
	c.auto(false, 1)
	// reentered 保存由已持久保存取得的“reentered”。
	reentered := stored()
	if reentered.Revision != after.Revision+1 || reentered.EconomicSeq != after.EconomicSeq || reentered.SourceSeq != after.SourceSeq || !reflect.DeepEqual(reentered.Items, after.Items) || reentered.Wallet != after.Wallet {
		t.Fatal("健康新票改变原书页资产或水位不符", after, reentered)
	}
	// result 保存由技能提交取得的结果。
	if result := c.skillCommit(request); result.ErrorCode != 0 {
		t.Fatal("健康新票或删除quote后原op不可核对", result)
	}
	// replayed 保存由已持久保存取得的“replayed”。
	replayed := stored()
	// replayReceipt 记录重放回执，使用操作表示。
	var replayReceipt persistence.Operation
	must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": opID}).Decode(&replayReceipt))
	if !reflect.DeepEqual(reentered.Items, replayed.Items) || reentered.Revision != replayed.Revision || reentered.EconomicSeq != replayed.EconomicSeq || reentered.SourceSeq != replayed.SourceSeq || !reflect.DeepEqual(receipt, replayReceipt) {
		t.Fatal("原op重放再次消耗或改变typed事实")
	}
	// changed 按目标类型读取当前接口值，得到“changed”。
	changed := proto.Clone(request).(*pb.SkillAssetCommitRequest)
	changed.PreviewId = domain.Bytes(domain.ID())
	// result 保存由技能提交取得的结果。
	if result := c.skillCommit(changed); result.ErrorCode != p.CodeOperationConflict {
		t.Fatal("同op改内容未拒绝", result)
	}
	t.Logf("通过：隔离满包资格%d格，正常两批拆书各2本×%d页；跨来源/部分stack拒绝，装备锁推进revision使旧预览失效，满包合并释放1格且不自动开战，typed原计划/审计、删除报价后健康新票原op不重复消耗。页强保护由纯测覆盖，未证明自然掉书获取。", initial.Capacity, rule.PagesPerBook)
}
