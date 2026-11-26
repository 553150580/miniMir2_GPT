// 本文件使用独立 MongoDB 和真实 WebSocket 验证矿石的 NPC 报价、出售及重连对账。
package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestMiningRecycleMongoWS 从原商人正常报价并扣除原单块矿石，重连可重放已落库而未读取的成交响应。
func TestMiningRecycleMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	raw, err := os.ReadFile(os.Getenv("MINIMIR_MINING_TEST_FIXTURE"))
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	digest := sha256.Sum256(raw)
	fixture.Hash = hex.EncodeToString(digest[:])
	must(t, fixture.Validate())
	if fixture.Mining == nil || len(fixture.Mining.Tools) != 2 {
		t.Fatal("隔离回收用例须装配两层矿区")
	}
	town, npc, _ := miningVendor(t, &fixture, fixture.Mining.Tools[0].ItemID)
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) { cfg.Fixture = fixture })

	// 预置合法实例的纯度边界，避免靠随机出矿等待指定纯度和绑定组合。
	ore := func(key string, purity uint32, bound bool) domain.Item {
		t.Helper()
		for _, definition := range fixture.Mining.Ores {
			if definition.Key == key {
				return domain.Item{ID: domain.ID(), ItemID: definition.ItemID, Quantity: 1, Version: 1,
					Bound: bound, SourceKind: "mining", SourceID: domain.ID(), OrePurity: purity}
			}
		}
		t.Fatalf("隔离回收缺少矿种 %s", key)
		return domain.Item{}
	}
	retained := []domain.Item{ore("copper", 1, false), ore("black_iron", 100, true), ore("copper", 8, true)}
	retained[2].Locked = true
	sales := []struct {
		item domain.Item // 交易应移除且写入原回执的单块实例。
		gold int64       // 单块整数收入，按实际绑定落入对应钱包。
	}{
		{ore("copper", 5, true), 2}, {ore("copper", 7, true), 3}, {ore("copper", 12, false), 6},
		{ore("iron", 19, true), 19}, {ore("gold", 11, false), 22},
	}
	items := slices.Clone(retained)
	for _, sale := range sales {
		items = append(items, sale.item)
	}
	wallet := domain.Wallet{Gold: 110, BoundGold: 36}
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "minerecycle_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	role := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: route.origin.OriginID,
		Slot: 1, Job: 0, Name: "矿收" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	now := time.Now()
	_, err = store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": bson.M{
		"level": fixture.Mining.Layers[0].MinimumLevel, "items": items, "wallet": wallet,
		"hosting_card_since": now.Add(-time.Hour), "hosting_card_until": now.Add(time.Hour),
		"privilege_since": now.Add(-time.Hour), "privilege_until": now.Add(time.Hour),
	}})
	must(t, err)
	readRole := func() domain.Role {
		t.Helper()
		persisted, err := store.Role(ctx, roleID)
		must(t, err)
		return persisted
	}
	attach := func() *client {
		t.Helper()
		ticket := directoryTicket(t, endpoint, login.AccountToken, role.Key, route)
		c := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
		t.Cleanup(c.close)
		return c
	}
	c := attach()
	c.auto(false, 3)
	quoteRequest := func(item domain.Item, quantity int64) *pb.VendorQuoteRequest {
		return &pb.VendorQuoteRequest{SceneEpoch: c.entry.SceneEpoch, NpcId: npc.ID, Action: 2,
			InstanceId: domain.Bytes(item.ID), ItemVersion: uint64(item.Version), Quantity: quantity}
	}
	request := c.send(p.MsgC2SVendorQuote, quoteRequest(sales[0].item, 1))
	travelExpectError(c, p.MsgC2SVendorQuote, request, p.CodeVendorUnavailable)
	reachTownVendor(c, town, npc)
	for _, item := range retained {
		request := c.send(p.MsgC2SVendorQuote, quoteRequest(item, 1))
		travelExpectError(c, p.MsgC2SVendorQuote, request, p.CodeVendorItemBlocked)
	}
	request = c.send(p.MsgC2SVendorQuote, quoteRequest(sales[0].item, 2))
	travelExpectError(c, p.MsgC2SVendorQuote, request, p.CodeVendorItemBlocked)
	initial := readRole()
	if !slices.Equal(initial.Items, items) || initial.Wallet != wallet {
		t.Fatal("被拒绝的矿石报价改变了库存或钱包", initial)
	}

	commits := make([]*pb.VendorCommitRequest, 0, len(sales))
	for index, sale := range sales {
		request := c.send(p.MsgC2SVendorQuote, quoteRequest(sale.item, 1))
		frame := travelUntilWithin(c, 4*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CVendorQuote && f.Request == request })
		var quote pb.VendorQuoteResult
		must(t, proto.Unmarshal(frame.Payload, &quote))
		proceeds := domain.Wallet{Gold: sale.gold}
		if sale.item.Bound {
			proceeds = domain.Wallet{BoundGold: sale.gold}
		}
		if quote.ItemId != sale.item.ItemID || domain.Hex(quote.InstanceId) != sale.item.ID || quote.ItemVersion != uint64(sale.item.Version) ||
			quote.Bound != sale.item.Bound || quote.Gold != proceeds.Gold || quote.BoundGold != proceeds.BoundGold || quote.Quantity != 1 || quote.ItemQuantity != 1 {
			t.Fatal("商人未按原单块纯度和绑定提供真实报价", &quote)
		}
		commit := &pb.VendorCommitRequest{Operation: c.operation(), QuoteId: quote.QuoteId, SceneEpoch: quote.SceneEpoch, NpcId: quote.NpcId}
		commits = append(commits, commit)
		wallet.Gold += proceeds.Gold
		wallet.BoundGold += proceeds.BoundGold
		if index < len(sales)-1 {
			result := c.asset(p.MsgC2SVendorCommit, commit)
			if result.ErrorCode != 0 || result.CommandSeq != commit.Operation.CommandSeq {
				t.Fatal("矿石没有沿原出售操作结算", result)
			}
			c.inventoryAt(result.NewRevision)
			continue
		}
		// 确认真实事务已提交后丢弃最后一次网络结果，在另一个场景按原命令对账。
		c.send(p.MsgC2SVendorCommit, commit)
		deadline := time.Now().Add(5 * time.Second)
		for readRole().EconomicSeq != int64(commit.Operation.CommandSeq) {
			if time.Now().After(deadline) {
				t.Fatal("丢弃响应前没有观察到最后一笔矿石交易落库")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	c.close()
	c = attach()
	if c.entry.SceneKind == devconfig.SceneKindTown {
		travelToBaseline(c, 2, fixture.Map.ID)
		travelAcknowledge(c, false)
	}
	for index, commit := range commits {
		if repeat := c.asset(p.MsgC2SVendorCommit, commit); repeat.ErrorCode != 0 {
			t.Fatal("断线或切图后无法对账原矿石成交", repeat)
		}
		id := "asset:" + roleID + ":" + domain.Hex(commit.Operation.OpId)
		var receipt persistence.Operation
		must(t, store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": id}).Decode(&receipt))
		audits, err := store.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": id})
		must(t, err)
		if audits != 1 || receipt.Vendor == nil || receipt.Vendor.QuoteID != domain.Hex(commit.QuoteId) || receipt.Vendor.NPC != npc.ID ||
			receipt.Vendor.Plan.Sold == nil || *receipt.Vendor.Plan.Sold != sales[index].item || receipt.CommandSeq != int64(commit.Operation.CommandSeq) {
			t.Fatal("矿石的原报价、纯度、绑定、来源或唯一审计未保留", receipt)
		}
	}
	if final := readRole(); final.Wallet != wallet || !slices.Equal(final.Items, retained) || final.EconomicSeq != initial.EconomicSeq+int64(len(sales)) || final.SourceSeq != initial.SourceSeq {
		t.Fatal("单块取整、锁定矿保留、绑定分账或原操作防重失效", final.Wallet, final.Items, final.EconomicSeq)
	}
}
