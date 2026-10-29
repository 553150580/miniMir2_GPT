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
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestSkillPageMergeFullBagSourceReceiptsMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo/WS同源书页合并与原回执核对")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	ctx, cancel := context.WithCancel(context.Background())
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	var wg sync.WaitGroup
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("书页合并%s: %v", mode, err)
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
	for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, endpoint)
	}
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "page_merge_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	rule := cfg.Fixture.SkillBookRecycle[0]
	book, known := cfg.Fixture.Item(rule.ItemID)
	skill, learned := cfg.Fixture.Skill(book.BookSkillID)
	if !known || !learned || rule.PagesPerBook <= 0 || rule.PageItemID != cfg.Fixture.SkillEconomy.PageItemID {
		t.Fatal("必须使用当前唯一Excel的真实原书及拆页定义")
	}
	key := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: skill.JobID, Name: "合并书页" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, key, 0)
	roleID := domain.Hex(key.Key.RoleId)
	initial, err := inspection.Role(ctx, roleID)
	must(t, err)
	// 仅随机库的入场资格：四本配置允许的掉落原书和满包前态。
	// 不预置任何书页、预览、回执或合并结果；两批拆书均走正常WS。
	prepared := append([]domain.Item(nil), initial.Items...)
	books := []domain.Item{}
	for n := 0; n < 4; n++ {
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
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, key.Key))
	defer func() { c.close() }()
	c.auto(false, 1)
	stored := func() domain.Role { r, err := inspection.Role(ctx, roleID); must(t, err); return r }
	before := stored()
	recycle := func(items []domain.Item) string {
		q := &pb.SkillAssetPreviewRequest{Kind: 3}
		for _, item := range items {
			q.Materials = append(q.Materials, &pb.ItemConsumption{InstanceId: domain.Bytes(item.ID), Version: uint64(item.Version), Quantity: item.Quantity})
		}
		preview := c.skillPreview(q)
		if len(preview.Outputs) != len(items) {
			t.Fatal("拆书被隐式合并")
		}
		request := &pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()}
		if result := c.skillCommit(request); result.ErrorCode != 0 {
			t.Fatal("正常拆书失败", result)
		}
		return domain.Hex(request.Operation.OpId)
	}
	firstSource, secondSource := recycle(books[:2]), recycle(books[2:])
	pages := func(source string) []*pb.ItemView {
		out := []*pb.ItemView{}
		for _, item := range c.inventory.Items {
			if item.ItemId == rule.PageItemID && item.SourceKind == "skill_recycle" && item.SourceId == source {
				out = append(out, item)
			}
		}
		return out
	}
	requestFor := func(items []*pb.ItemView) *pb.SkillAssetPreviewRequest {
		q := &pb.SkillAssetPreviewRequest{Kind: 4}
		for _, item := range items {
			q.Materials = append(q.Materials, &pb.ItemConsumption{InstanceId: item.InstanceId, Version: item.Version, Quantity: item.Quantity})
		}
		return q
	}
	first, second := pages(firstSource), pages(secondSource)
	if len(first) != 2 || len(second) != 2 || len(c.inventory.Items) != int(c.inventory.Capacity) || first[0].Quantity != rule.PagesPerBook || first[1].Quantity != rule.PagesPerBook {
		t.Fatal("真实拆书未产生两批各自独立的满包书页")
	}
	unchanged := stored()
	blocked := c.send(p.MsgC2SSkillAssetPreview, requestFor([]*pb.ItemView{first[0], second[0]}))
	c.expectError(blocked, p.CodeSkillMaterials)
	if after := stored(); after.Revision != unchanged.Revision || !reflect.DeepEqual(after.Items, unchanged.Items) {
		t.Fatal("跨SourceID合并改写了资产")
	}
	// 正常装备锁定推进角色版本，使既有预览过期；书页没有锁定RPC。
	var equipment *pb.ItemView
	for _, item := range c.inventory.Items {
		if definition, ok := cfg.Fixture.Item(item.ItemId); ok && definition.IsEquipment {
			equipment = item
			break
		}
	}
	if equipment == nil {
		t.Fatal("缺少正常装备锁定可用的新生装备")
	}
	stale := c.skillPreview(requestFor(first))
	lock := &pb.SetItemLockRequest{ItemInstanceId: equipment.InstanceId, ItemVersion: equipment.Version, Locked: true, Operation: c.operation()}
	locked := c.asset(p.MsgC2SSetItemLock, lock)
	c.inventoryAt(locked.NewRevision)
	if locked.ErrorCode != 0 {
		t.Fatal("正常锁定失败", locked)
	}
	staleRequest := &pb.SkillAssetCommitRequest{PreviewId: stale.PreviewId, Operation: c.operation()}
	if result := c.skillCommit(staleRequest); result.ErrorCode != p.CodeSkillPreviewStale {
		t.Fatal("锁定后旧预览仍可消耗原件", result)
	}
	var staleReceipt persistence.Operation
	must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(staleRequest.Operation.OpId)}).Decode(&staleReceipt))
	if staleReceipt.Skill != nil || len(stored().Items) != initial.Capacity {
		t.Fatal("失败提交伪造成功计划或改变槽位")
	}
	partial := requestFor(pages(firstSource))
	partial.Materials[0].Quantity--
	blocked = c.send(p.MsgC2SSkillAssetPreview, partial)
	c.expectError(blocked, p.CodeSkillMaterials)
	for _, item := range c.inventory.Items {
		if domain.Hex(item.InstanceId) == domain.Hex(equipment.InstanceId) && item.Locked {
			result := c.asset(p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: item.InstanceId, ItemVersion: item.Version, Locked: false, Operation: c.operation()})
			c.inventoryAt(result.NewRevision)
			if result.ErrorCode != 0 {
				t.Fatal("正常解锁失败", result)
			}
		}
	}
	first = pages(firstSource)
	preview := c.skillPreview(requestFor(first))
	if len(preview.Materials) != 2 || len(preview.Outputs) != 1 || preview.Outputs[0].SourceId != firstSource || preview.Outputs[0].Quantity != 2*rule.PagesPerBook {
		t.Fatal("合并预览没有冻结实际完整stack或来源", preview)
	}
	preMerge := stored()
	request := &pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()}
	if result := c.skillCommit(request); result.ErrorCode != 0 {
		t.Fatal("满包同源合并失败", result)
	}
	after := stored()
	merged := pages(firstSource)
	if len(after.Items) != initial.Capacity-1 || len(merged) != 1 || merged[0].Quantity != 2*rule.PagesPerBook || merged[0].Version != 1 || !merged[0].Bound || merged[0].Quality != first[0].Quality || after.PickupStopped || after.Wallet != before.Wallet || after.SourceSeq != before.SourceSeq || after.Revision != preMerge.Revision+1 || after.EconomicSeq != preMerge.EconomicSeq+1 {
		t.Fatal("合并数量、来源、满包解除或永久水位不符", after, merged)
	}
	for _, old := range first {
		if domain.Hex(old.InstanceId) == domain.Hex(merged[0].InstanceId) {
			t.Fatal("输出复用了已完整消耗的原身份")
		}
	}
	opID := "asset:" + roleID + ":" + domain.Hex(request.Operation.OpId)
	var receipt persistence.Operation
	must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": opID}).Decode(&receipt))
	if receipt.Skill == nil || receipt.Skill.Kind != 4 || len(receipt.Skill.Materials) != 2 || len(receipt.Skill.Outputs) != 1 || receipt.Skill.Outputs[0].SourceID != firstSource || receipt.Skill.Outputs[0].ID != domain.Hex(merged[0].InstanceId) {
		t.Fatal("typed原回执缺少精确输入输出", receipt)
	}
	for _, material := range receipt.Skill.Materials {
		if material.Quantity != material.Item.Quantity || material.Item.SourceID != firstSource || material.Item.SourceKind != "skill_recycle" {
			t.Fatal("typed原回执丢失完整原堆叠", receipt)
		}
	}
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
	reentered := stored()
	if reentered.Revision != after.Revision+1 || reentered.EconomicSeq != after.EconomicSeq || reentered.SourceSeq != after.SourceSeq || !reflect.DeepEqual(reentered.Items, after.Items) || reentered.Wallet != after.Wallet {
		t.Fatal("健康新票改变原书页资产或水位不符", after, reentered)
	}
	if result := c.skillCommit(request); result.ErrorCode != 0 {
		t.Fatal("健康新票或删除quote后原op不可核对", result)
	}
	replayed := stored()
	var replayReceipt persistence.Operation
	must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": opID}).Decode(&replayReceipt))
	if !reflect.DeepEqual(reentered.Items, replayed.Items) || reentered.Revision != replayed.Revision || reentered.EconomicSeq != replayed.EconomicSeq || reentered.SourceSeq != replayed.SourceSeq || !reflect.DeepEqual(receipt, replayReceipt) {
		t.Fatal("原op重放再次消耗或改变typed事实")
	}
	changed := proto.Clone(request).(*pb.SkillAssetCommitRequest)
	changed.PreviewId = domain.Bytes(domain.ID())
	if result := c.skillCommit(changed); result.ErrorCode != p.CodeOperationConflict {
		t.Fatal("同op改内容未拒绝", result)
	}
	t.Logf("通过：隔离满包资格%d格，正常两批拆书各2本×%d页；跨来源/部分stack拒绝，装备锁推进revision使旧预览失效，满包合并释放1格且不自动开战，typed原计划/审计、删除报价后健康新票原op不重复消耗。页强保护由纯测覆盖，未证明自然掉书获取。", initial.Capacity, rule.PagesPerBook)
}
