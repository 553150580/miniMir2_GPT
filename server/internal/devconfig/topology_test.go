// 本文件覆盖稳定游戏节点身份的兼容读取、初始区服目录完整性和客户端连接地址边界。

package devconfig

import "testing"

// TestGameNodeIDConfigAliases 保留旧试玩配置格式，并拒绝同一进程声明两种不同节点身份。
func TestGameNodeIDConfigAliases(t *testing.T) {
	// tests 指定配置文本及读取后传给游戏运行时的规范身份，空值表示应拒绝。
	tests := []struct {
		name string // 子用例报告名称。
		raw  string // 包含一至两种节点身份键的配置。
		want string // 规范身份或预期拒绝标记。
	}{
		{"规范字段", `{"game_node_id":"game-a"}`, "game-a"},
		{"原字段兼容", `{"node_id":"game-a"}`, "game-a"},
		{"同一身份", `{"game_node_id":"game-a","node_id":"game-a"}`, "game-a"},
		{"身份冲突", `{"game_node_id":"game-a","node_id":"game-b"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			err := cfg.decodeNodeConfiguration([]byte(tt.raw))
			if tt.want == "" {
				if err == nil {
					t.Fatal("冲突节点身份得到接受")
				}
				return
			}
			if err != nil || cfg.NodeID != tt.want {
				t.Fatalf("节点身份 %q，预期 %q，错误 %v", cfg.NodeID, tt.want, err)
			}
		})
	}
}

// TestTopologyValidatesOriginAndRunSeeds 验证多个永久入口可共享运行服，并拒绝破坏目录一致性的配置。
func TestTopologyValidatesOriginAndRunSeeds(t *testing.T) {
	// cfg 使用两服同分区、一个运行服和一个部署节点，明确保留入口归属与部署身份的区别。
	cfg := Config{NodeID: "game-a", OriginID: 1, RunID: 10,
		OriginRoutes: []OriginRouteSeed{
			{OriginID: 1, Name: "一服", RegionID: "zone-a", RegionName: "一区", RunID: 10},
			{OriginID: 2, Name: "二服", RegionID: "zone-a", RegionName: "一区", Order: 1, RunID: 10},
		},
		RunServers: []RunServerSeed{{RunID: 10, GameNodeID: "game-a", GameWSURL: "ws://127.0.0.1:18101/ws"}},
	}
	if err := cfg.validateTopology(); err != nil {
		t.Fatalf("多入口同一运行服应有效: %v", err)
	}
	cfg.OriginRoutes[1].RunID = 11
	if err := cfg.validateTopology(); err == nil {
		t.Fatal("入口引用不存在的运行服得到接受")
	}
	cfg.OriginRoutes[1].RunID = 10
	cfg.OriginRoutes[1].RegionName = "二区"
	if err := cfg.validateTopology(); err == nil {
		t.Fatal("同一分区拥有不同展示名称得到接受")
	}
	if ValidGameNodeID("singleton:account") || ValidGameNodeID(" game-a") {
		t.Fatal("游戏节点身份占用了保留租约键或包含空白")
	}
}

// TestGameWSURLAcceptsOnlyDevelopmentEndpoints 验证发布地址仅为允许的本机直连或固定同源代理。
func TestGameWSURLAcceptsOnlyDevelopmentEndpoints(t *testing.T) {
	// allowed 覆盖可用的同源代理和本机直连格式。
	allowed := []string{"/game/ws", "ws://127.0.0.1:18101/ws", "wss://localhost:18101/ws"}
	for _, endpoint := range allowed {
		if err := ValidateGameWSURL(endpoint); err != nil {
			t.Errorf("预期允许 %q: %v", endpoint, err)
		}
	}
	// invalid 覆盖外站、错误路径、身份信息及无效端口等禁止形式。
	invalid := []string{"//other.example/game/ws", "/ws", "ws://127.0.0.1/ws", "wss://other.example:443/ws",
		"ws://admin@localhost:80/ws", "ws://localhost:0/ws", "ws://localhost:18101/ws?", "ws://localhost:18101/ws#"}
	for _, endpoint := range invalid {
		if err := ValidateGameWSURL(endpoint); err == nil {
			t.Errorf("预期拒绝 %q", endpoint)
		}
	}
}
