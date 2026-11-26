// 本文件校验交易来源与定点报价边界。
package persistence

import (
	"math"
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

func TestItemMarketPriceKeepsIntegralUnit(t *testing.T) {
	rules := &devconfig.ItemMarketConfig{BidRaiseBP: 1000, PriceStepYuanbaoUnits: 1, MinimumRaiseYuanbaoUnits: 1}
	for _, row := range []struct{ price, want int64 }{{1, 2}, {10, 11}, {11, 13}, {199, 219}} {
		if got, ok := marketNextPrice(row.price, rules); !ok || got != row.want {
			t.Fatalf("定点加价错误，原价 %d，新价 %d", row.price, got)
		}
	}
	if _, ok := marketNextPrice(math.MaxInt64, rules); ok {
		t.Fatal("加价不能溢出并下降")
	}
	if _, ok := marketTotal(math.MaxInt64/2+1, 2); ok {
		t.Fatal("总价不能溢出并绕过钱包检查")
	}
}

func TestCanTradeRespectsSourceAndEquipmentQuality(t *testing.T) {
	item := domain.Item{ID: domain.ID(), ItemID: 1, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: "1", Quality: 1}
	definition := devconfig.Item{ID: 1, IsEquipment: true}
	if CanTrade(item, definition) || item.Bound {
		t.Fatal("非绑定精良装备不能上架，且不可因此改成绑定")
	}
	item.Quality = 2
	if !CanTrade(item, definition) {
		t.Fatal("有效来源的非绑定稀有装备应允许上架")
	}
	for _, change := range []func(*domain.Item){
		func(i *domain.Item) { i.Bound = true },
		func(i *domain.Item) { i.Locked = true },
		func(i *domain.Item) { i.ReservationID = "原操作占用" },
		func(i *domain.Item) { i.SourceID = "" },
		func(i *domain.Item) { i.SourceKind = "initial_grant" },
	} {
		other := item
		change(&other)
		if CanTrade(other, definition) {
			t.Fatal("受保护或来源无效的实例不能上架")
		}
	}
}
