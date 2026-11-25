// 本文件把已封账镜像赛季奖附加到原角色永久钱包。
package persistence

import (
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

func validMirrorSeasonAssets(grant *domain.MirrorSeasonAssets) bool {
	return grant != nil && grant.BoundIngot > 0
}

// grantMirrorSeasonAssets 在标记附件领取前检查并准备全量领域变更。
func grantMirrorSeasonAssets(role domain.Role, grant *domain.MirrorSeasonAssets) (domain.Role, error) {
	if !validMirrorSeasonAssets(grant) {
		return role, Fail(p.CodeOperationConflict, "镜像赛季的原奖励资产缺失")
	}
	balance, err := add(role.Wallet.BoundYuanbao, grant.BoundIngot)
	if err != nil {
		return role, err
	}
	role.Wallet.BoundYuanbao = balance
	return role, nil
}
