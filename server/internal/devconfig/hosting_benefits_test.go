// 本文件核验自动挑战权益在当前开发及历史配置中的加载行为。
package devconfig

import (
	"encoding/json"
	"testing"
)

// TestHostingBenefitsPersonalBossProjection 验证当前配置的布尔权益与旧包兼容。
func TestHostingBenefitsPersonalBossProjection(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixture.HostingBenefits == nil || !cfg.Fixture.HostingBenefits.AutoPersonalBoss {
		t.Fatal("当前开发配置未载入工作簿已确认的个人首领托管权益")
	}
	var legacy Fixture
	if err := json.Unmarshal([]byte(`{"hosting_benefits":{"free_daily_seconds":1,"card_daily_seconds":2}}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.HostingBenefits == nil || legacy.HostingBenefits.AutoPersonalBoss {
		t.Fatal("旧包省略该字段时应保持关闭")
	}
	var invalid Fixture
	if err := json.Unmarshal([]byte(`{"hosting_benefits":{"auto_personal_boss":1}}`), &invalid); err == nil {
		t.Fatal("配置接受了整数代替布尔值")
	}
}
