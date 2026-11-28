// 本文件对拍上海日界、原报价失效与满包时的资产原子性。
package persistence

import (
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestGrowthShopOperatingDayAndCapacity 用可控时钟验证临近上海午夜的原报价，以及满包后的真实分堆。
func TestGrowthShopOperatingDayAndCapacity(t *testing.T) {
	const itemID = 900001
	fixture := &devconfig.Fixture{RuleVersion: 30, Hash: "商城隔离测试", Items: []devconfig.Item{{ID: itemID, Stackable: true, MaxStack: 2}},
		GrowthShop: &devconfig.GrowthShop{Offers: []devconfig.GrowthShopOffer{{ID: 3, ItemID: itemID, Currency: devconfig.GrowthShopYuanbao,
			PriceUnits: 50, ItemPerPack: 1, MaxCount: 4, DailyLimit: 4}}}}
	boundary := time.Date(2026, 11, 29, 0, 0, 0, 0, shanghaiOperatingZone)
	before := boundary.Add(-time.Second)
	role := domain.Role{ID: domain.ID(), Origin: 1, Revision: 4, Capacity: 2, Wallet: domain.Wallet{Yuanbao: 400, BoundYuanbao: 999},
		GrowthShop: &domain.GrowthShopDay{Day: shanghaiOperatingDay(before), Counts: []domain.GrowthShopCount{{ItemID: itemID, Bought: 1}}}}
	quote, code := QuoteGrowthShop(role, fixture, 2, before, 3, 3)
	if code != 0 || !quote.ExpiresAt.Equal(boundary) || quote.Terms.DailyBought != 1 {
		t.Fatal("午夜前的报价未固定上海运营日和剩余额度", code, quote)
	}
	request := func(q GrowthShopQuote) AssetRequest {
		op := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: 1, ExpectedRevision: 4, ExpectedRuleVersion: 30}
		return AssetRequest{Context: op, SceneEpoch: 2, GrowthShopQuote: &q, GrowthShopCommit: &pb.GrowthShopCommitRequest{
			QuoteId: domain.Bytes(q.ID), ProductId: q.Terms.ProductID, Count: q.Terms.Count, Operation: op}}
	}
	if next, receipt, err := commitGrowthShop(role, fixture, request(quote), boundary); err != p.CodeGrowthShopQuoteChanged ||
		receipt != nil || next.GrowthShop != role.GrowthShop || next.Wallet != role.Wallet {
		t.Fatal("午夜边界接受了旧日原报价或改变资产", next, receipt, err)
	}
	quote, code = QuoteGrowthShop(role, fixture, 2, boundary, 3, 3)
	if code != 0 || quote.Terms.DailyBought != 0 {
		t.Fatal("新运营日未清除旧日已用数量", quote, code)
	}
	changed := role
	changed.Capacity = 1
	if next, receipt, err := commitGrowthShop(changed, fixture, request(quote), boundary); err != p.CodeEquipmentBagFull ||
		receipt != nil || len(next.Items) != 0 || next.GrowthShop != role.GrowthShop || next.Wallet != role.Wallet {
		t.Fatal("满包时部分发货或扣款", next, receipt, err)
	}
	next, receipt, code := commitGrowthShop(role, fixture, request(quote), boundary)
	if code != 0 || receipt == nil || len(receipt.Items) != 2 || receipt.Items[0].Quantity != 2 || receipt.Items[1].Quantity != 1 ||
		next.Wallet.Yuanbao != 250 || next.Wallet.BoundYuanbao != 999 || next.GrowthShop.Day != shanghaiOperatingDay(boundary) ||
		next.GrowthShop.Counts[0].Bought != 3 || role.GrowthShop.Counts[0].Bought != 1 {
		t.Fatal("新运营日的分堆到账、扣款或角色数量不守恒", next, receipt, code)
	}
}
