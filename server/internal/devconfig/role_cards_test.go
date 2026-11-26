// 本文件核验角色元宝月卡的加载、旧包默认值及配置门禁。
package devconfig

import "testing"

// TestRoleCardDevelopmentProjection 验证项目正式开发夹具至运行时的实际加载。
func TestRoleCardDevelopmentProjection(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &cfg.Fixture
	if fixture.RoleCards == nil || fixture.RoleCards.DailyPrivilegeUnits != 2000 {
		t.Fatalf("特权卡每日绑元定点额度未加载：%+v", fixture.RoleCards)
	}
	for _, expected := range []RoleCardProduct{
		{ID: 7, Kind: RoleCardKindPrivilege, Days: 30, PriceYuanbaoUnits: 30000},
		{ID: 8, Kind: RoleCardKindHosting, Days: 30, PriceYuanbaoUnits: 45000},
	} {
		if got, ok := fixture.RoleCardProduct(expected.ID); !ok || got != expected {
			t.Fatalf("元宝商品 %d 未按工作簿加载：%+v", expected.ID, got)
		}
	}
	for id := uint32(1); id <= 6; id++ {
		if _, ok := fixture.RoleCardProduct(id); ok {
			t.Fatalf("现金商品 %d 不应进入元宝商品查询", id)
		}
	}
}

// TestValidateRoleCards 拒绝重复、不完整和非法金额的配置，同时兼容旧包。
func TestValidateRoleCards(t *testing.T) {
	var legacy Fixture
	if err := legacy.validateRoleCards(); err != nil {
		t.Fatal(err)
	}
	if _, ok := legacy.RoleCardProduct(7); ok {
		t.Fatal("旧配置默认关闭元宝购卡")
	}
	for name, change := range map[string]func(*RoleCardConfig){
		"无每日领额": func(c *RoleCardConfig) { c.DailyPrivilegeUnits = 0 },
		"无第二卡种": func(c *RoleCardConfig) { c.Products = c.Products[:1] },
		"重复商品":  func(c *RoleCardConfig) { c.Products[1].ID = c.Products[0].ID },
		"重复卡种":  func(c *RoleCardConfig) { c.Products[1].Kind = c.Products[0].Kind },
		"未知卡种":  func(c *RoleCardConfig) { c.Products[0].Kind = 3 },
		"非月卡":   func(c *RoleCardConfig) { c.Products[0].Days = 90 },
		"非正价格":  func(c *RoleCardConfig) { c.Products[0].PriceYuanbaoUnits = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := Fixture{RoleCards: &RoleCardConfig{DailyPrivilegeUnits: 1, Products: []RoleCardProduct{
				{ID: 7, Kind: RoleCardKindPrivilege, Days: 30, PriceYuanbaoUnits: 11},
				{ID: 8, Kind: RoleCardKindHosting, Days: 30, PriceYuanbaoUnits: 22},
			}}}
			change(fixture.RoleCards)
			if err := fixture.validateRoleCards(); err == nil {
				t.Fatal("无效角色月卡配置通过校验")
			}
		})
	}
}
