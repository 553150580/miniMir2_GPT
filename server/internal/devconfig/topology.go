// 本文件校验开发区服入口、运行服分配与游戏节点身份。初始配置供建库使用，已有运营记录由数据库裁决。

package devconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// OriginRouteSeed 保存目录展示分组及入口服初次指向的运行服。
type OriginRouteSeed struct {
	OriginID    uint64 `json:"origin_server_id"` // 角色创建后的永久入口身份。
	Name        string `json:"name"`             // 入口服展示名称。
	RegionID    string `json:"region_id"`        // 分区的稳定显示分组键。
	RegionName  string `json:"region_name"`      // 分区展示名称。
	RegionOrder int    `json:"region_order"`     // 分区的排序权重，值越小越靠前。
	Order       int    `json:"order"`            // 分区内入口服的排序权重。
	RunID       uint64 `json:"run_server_id"`    // 该入口初次指向的逻辑运行服。
}

// RunServerSeed 预设运行服归属，不把节点自行声明的地址发布给客户端。
type RunServerSeed struct {
	RunID      uint64 `json:"run_server_id"` // 运营分配对应的逻辑运行服。
	GameNodeID string `json:"game_node_id"`  // 允许运行此服的稳定游戏节点身份。
	GameWSURL  string `json:"game_ws_url"`   // 由可信配置公布的开发直连地址。
}

// stableDirectoryID 排除空白、控制字符和 singleton 租约保留分隔符。
var stableDirectoryID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidGameNodeID 判断节点身份能否安全用作持久租约的唯一键。
func ValidGameNodeID(id string) bool {
	return stableDirectoryID.MatchString(id)
}

// ValidateGameWSURL 允许同源试玩代理或显式的本机游戏 WebSocket 连接地址。
func ValidateGameWSURL(endpoint string) error {
	if endpoint == "/game/ws" {
		return nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") || parsed.User != nil ||
		(parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") ||
		parsed.Path != "/ws" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(endpoint, "#") {
		return errors.New("游戏连接地址必须为本机的 ws/wss /ws 端点")
	}
	port, err := strconv.ParseUint(parsed.Port(), 10, 16)
	if err != nil || port == 0 {
		return errors.New("游戏连接地址必须显式指定有效端口")
	}
	return nil
}

// decodeNodeConfiguration 将旧配置键归一到 NodeID，避免维护两套进程身份。
func (c *Config) decodeNodeConfiguration(raw []byte) error {
	var encoded struct {
		*Config
		LegacyNodeID string `json:"node_id"` // 旧试玩配置键，读取后只使用规范身份。
	}
	encoded.Config = c
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return err
	}
	if c.NodeID == "" {
		c.NodeID = encoded.LegacyNodeID
	} else if encoded.LegacyNodeID != "" && c.NodeID != encoded.LegacyNodeID {
		return errors.New("game_node_id 与旧 node_id 配置不一致")
	}
	return nil
}

// validateTopology 验证显式的初始目录。旧单节点配置由持久层按实际监听参数初始化。
func (c Config) validateTopology() error {
	if !ValidGameNodeID(c.NodeID) || c.OriginID == 0 || c.OriginID > math.MaxInt64 || c.RunID == 0 || c.RunID > math.MaxInt64 {
		return errors.New("游戏节点、永久入口服和运行服需要有效的稳定身份")
	}
	if c.OriginRoutes == nil && c.RunServers == nil {
		return nil
	}
	if len(c.OriginRoutes) == 0 || len(c.RunServers) == 0 {
		return errors.New("显式区服配置需要同时提供入口路由和运行服分配")
	}
	// runs 验证运行服主键唯一，并供入口服引用完整性检查。
	runs := make(map[uint64]struct{}, len(c.RunServers))
	// 公告地址需要能把连接交给唯一的部署节点。多个入口或运行服可复用同一节点地址。
	endpointOwners := make(map[string]string)
	for _, run := range c.RunServers {
		if _, duplicate := runs[run.RunID]; duplicate || run.RunID == 0 || run.RunID > math.MaxInt64 || !ValidGameNodeID(run.GameNodeID) {
			return fmt.Errorf("无效或重复的运行服分配 %d", run.RunID)
		}
		if err := ValidateGameWSURL(run.GameWSURL); err != nil {
			return fmt.Errorf("运行服 %d 端点无效: %w", run.RunID, err)
		}
		if owner := endpointOwners[run.GameWSURL]; owner != "" && owner != run.GameNodeID {
			return fmt.Errorf("游戏节点 %s 与 %s 不能共用本机公告地址", owner, run.GameNodeID)
		}
		endpointOwners[run.GameWSURL] = run.GameNodeID
		runs[run.RunID] = struct{}{}
	}
	// origins 防止同一入口登记多套路由，regions 防止同一区域出现不同的显示定义。
	origins := make(map[uint64]struct{}, len(c.OriginRoutes))
	regions := make(map[string]OriginRouteSeed)
	for _, origin := range c.OriginRoutes {
		_, duplicate := origins[origin.OriginID]
		_, targetExists := runs[origin.RunID]
		if duplicate || origin.OriginID == 0 || origin.OriginID > math.MaxInt64 || !targetExists ||
			!stableDirectoryID.MatchString(origin.RegionID) || !validDirectoryLabel(origin.Name) ||
			!validDirectoryLabel(origin.RegionName) || origin.RegionOrder < 0 || origin.Order < 0 {
			return fmt.Errorf("无效或重复的入口服路由 %d", origin.OriginID)
		}
		if previous, exists := regions[origin.RegionID]; exists && (previous.RegionName != origin.RegionName || previous.RegionOrder != origin.RegionOrder) {
			return fmt.Errorf("分区 %s 的名称或排序不一致", origin.RegionID)
		}
		origins[origin.OriginID] = struct{}{}
		regions[origin.RegionID] = origin
	}
	return nil
}

// validDirectoryLabel 接受中英文展示名，拒绝容易干扰目录布局的控制字符和外部空白。
func validDirectoryLabel(label string) bool {
	return label != "" && strings.TrimSpace(label) == label && utf8.RuneCountInString(label) <= 64 &&
		strings.IndexFunc(label, unicode.IsControl) < 0
}
