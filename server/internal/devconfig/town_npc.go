// 本文件加载紧凑城镇中的 NPC 摆放、朝向、交互及来源标识。

package devconfig

import (
	"errors"
	"strings"
)

// 城镇NPC是地图配置中的静态居民，通行阻挡与客户端展示取自同一格。
type TownNPC struct {
	// ID 区分当前步骤指向的目标对象。
	ID uint64 `json:"id"`
	// Name 记录名称，使用文本表示。
	Name string `json:"name"`
	// Role 记录角色，使用文本表示。
	Role string `json:"role"`
	// Position 记录位置，使用点位表示。
	Position Point `json:"position"`
	// Profile 记录“profile”，使用文本表示。
	Profile string `json:"profile"`
}

// validateTownNPCs 校验城镇“np”“cs”。
// m 是调用该方法的地图模板实例，blocked 记录阻挡，使用按点位索引的布尔状态映射表示。
func (m *MapTemplate) validateTownNPCs(blocked map[Point]bool) error {
	if len(m.NPCs) == 0 {
		return errors.New("紧凑城镇须配置本地NPC")
	}
	// ids 创建用于维护标识集合的容器或通道，occupied 创建用于维护已占用的容器或通道。
	ids, occupied := make(map[uint64]bool, len(m.NPCs)), make(map[Point]bool, len(m.NPCs))
	// npc 接收遍历“np”“cs”时当前的元素。
	for _, npc := range m.NPCs {
		if npc.ID == 0 || ids[npc.ID] || occupied[npc.Position] || !blocked[npc.Position] ||
			strings.TrimSpace(npc.Name) == "" || strings.TrimSpace(npc.Role) == "" || strings.TrimSpace(npc.Profile) == "" {
			return errors.New("城镇NPC须有唯一身份及完整展示配置，并独占一个阻挡格")
		}
		ids[npc.ID], occupied[npc.Position] = true, true
	}
	return nil
}
