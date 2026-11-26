// 本文件回归现金卡目录的缺省关闭、币种、期限与元宝商品隔离。
package devconfig

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// cashCardTestCatalog 固定对拍的六款工作簿商品。
func cashCardTestCatalog() []CashCardProduct {
	return []CashCardProduct{
		{ID: 1, Kind: RoleCardKindPrivilege, Days: 30, PriceFen: 3000},
		{ID: 2, Kind: RoleCardKindPrivilege, Days: 90, PriceFen: 7800},
		{ID: 3, Kind: RoleCardKindPrivilege, Days: 365, PriceFen: 29800},
		{ID: 4, Kind: RoleCardKindHosting, Days: 30, PriceFen: 4500},
		{ID: 5, Kind: RoleCardKindHosting, Days: 90, PriceFen: 11800},
		{ID: 6, Kind: RoleCardKindHosting, Days: 365, PriceFen: 46800},
	}
}

// TestCashCardCatalogIsOptionalAndIndependent 通过真实默认配置检验两个目录的查询边界。
func TestCashCardCatalogIsOptionalAndIndependent(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &cfg.Fixture
	if fixture.Payment != nil {
		t.Fatal("当前试玩配置提前装配了现金目录")
	}
	if _, found := fixture.CashCardProduct(1); found {
		t.Fatal("历史配置查询暴露未装配的现金商品")
	}
	products := cashCardTestCatalog()
	fixture.Payment = &PaymentConfig{CashCardProducts: products}
	if err := fixture.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range products {
		if offer, found := fixture.CashCardProduct(expected.ID); !found || offer != expected {
			t.Fatal("未找到完整的人民币商品", expected, offer)
		}
		if _, found := fixture.RoleCardProduct(expected.ID); found {
			t.Fatal("现金卡可用元宝购买", expected)
		}
	}
	for _, id := range []uint32{7, 8} {
		if _, found := fixture.CashCardProduct(id); found {
			t.Fatal("元宝卡编号可作为现金卡", id)
		}
		if offer, found := fixture.RoleCardProduct(id); !found || offer.Days != 30 {
			t.Fatal("原元宝订购目录发生变化", id, offer)
		}
	}
	// 隔离运行可另外读取真实生成文件，逐字段核对配置加载后的商品。
	if path := os.Getenv("MINIMIR_CASH_CARD_TEST_FIXTURE"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var actual Fixture
		if err := json.Unmarshal(raw, &actual); err != nil {
			t.Fatal(err)
		}
		if err := actual.Validate(); err != nil {
			t.Fatal(err)
		}
		if actual.Payment == nil || !slices.Equal(actual.Payment.CashCardProducts, products) {
			t.Fatal("工作簿投影的 Go 运行配置与六款原商品不一致", actual.Payment)
		}
	}
}

// TestCashCardCatalogRejectsInvalidPricesAndTerms 防止运行配置绕过商品形状与分币校验。
func TestCashCardCatalogRejectsInvalidPricesAndTerms(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*PaymentConfig)
	}{
		{"缺少年卡", func(p *PaymentConfig) { p.CashCardProducts = p.CashCardProducts[:5] }},
		{"重复编号", func(p *PaymentConfig) { p.CashCardProducts[0].ID = 2 }},
		{"复用元宝编号", func(p *PaymentConfig) { p.CashCardProducts[0].ID = 7 }},
		{"重复期限", func(p *PaymentConfig) { p.CashCardProducts[0].Days = 90 }},
		{"无效期限", func(p *PaymentConfig) { p.CashCardProducts[0].Days = 31 }},
		{"错误卡种", func(p *PaymentConfig) { p.CashCardProducts[0].Kind = 99 }},
		{"零价", func(p *PaymentConfig) { p.CashCardProducts[0].PriceFen = 0 }},
		{"负价", func(p *PaymentConfig) { p.CashCardProducts[0].PriceFen = -1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := Fixture{Payment: &PaymentConfig{CashCardProducts: cashCardTestCatalog()},
				RoleCards: &RoleCardConfig{Products: []RoleCardProduct{{ID: 7}}}}
			test.edit(fixture.Payment)
			if err := fixture.validatePayment(); err == nil {
				t.Fatal("无效现金卡目录通过配置校验")
			}
		})
	}
	for _, invalid := range []string{
		`{"channel":"fake","cash_card_products":[]}`,
		`{"recharge_products":[{"id":1}],"cash_card_products":[]}`,
		`{"cash_card_products":[{"id":1,"kind":1,"days":30,"price_fen":3000,"price_yuanbao_units":1}]}`,
		`{"cash_card_products":[{"id":1,"kind":1,"days":30,"price_fen":3000.5}]}`,
		`{"cash_card_products":[{"id":1,"kind":1,"days":30,"price_fen":true}]}`,
		`{"cash_card_products":[{"id":1,"kind":1,"days":30,"price_fen":9223372036854775808}]}`,
	} {
		var catalog PaymentConfig
		if err := json.Unmarshal([]byte(invalid), &catalog); err == nil {
			t.Fatal("未知字段或非整数的人民币分通过解析", invalid)
		}
	}
}
