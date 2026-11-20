package devconfig

import (
	"errors"
	"strings"
)

// 城镇NPC是地图配置中的静态居民，通行阻挡与客户端展示取自同一格。
type TownNPC struct {
	ID       uint64 `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Position Point  `json:"position"`
	Profile  string `json:"profile"`
}

func (m *MapTemplate) validateTownNPCs(blocked map[Point]bool) error {
	if len(m.NPCs) == 0 {
		return errors.New("紧凑城镇须配置本地NPC")
	}
	ids, occupied := make(map[uint64]bool, len(m.NPCs)), make(map[Point]bool, len(m.NPCs))
	for _, npc := range m.NPCs {
		if npc.ID == 0 || ids[npc.ID] || occupied[npc.Position] || !blocked[npc.Position] ||
			strings.TrimSpace(npc.Name) == "" || strings.TrimSpace(npc.Role) == "" || strings.TrimSpace(npc.Profile) == "" {
			return errors.New("城镇NPC须有唯一身份及完整展示配置，并独占一个阻挡格")
		}
		ids[npc.ID], occupied[npc.Position] = true, true
	}
	return nil
}
