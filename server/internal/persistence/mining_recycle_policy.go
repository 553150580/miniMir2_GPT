// 本文件保存角色独立的矿石回收偏好，资产入口负责版本、操作号与审计事务。
package persistence

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const AssetSetMiningAutoRecycle = "set_mining_auto_recycle" // 保存矿石回收偏好的主动经济操作。

// setMiningAutoRecycle 只在规则开放时记录后续矿石的处理方式，原背包矿石和装备策略不受影响。
func setMiningAutoRecycle(role domain.Role, enabled bool, fixture *devconfig.Fixture) (domain.Role, uint32) {
	if fixture.Mining == nil || fixture.Mining.AutoRecycle == nil {
		return role, p.CodeLineForbidden
	}
	role.MiningAutoRecycle = enabled
	return role, 0
}
