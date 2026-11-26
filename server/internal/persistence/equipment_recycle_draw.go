// 本文件为装备分解的各种副产物隔离随机空间，并在原事务重试中保持抽样一致。
package persistence

import (
	"crypto/rand"
	"encoding/hex"
)

// equipmentRecycleAttempt 保存一次手动分解的永久操作身份和私有随机种子。
type equipmentRecycleAttempt struct {
	operationID string // 已有角色经济操作的全局主键，作为各副产物实例的生成依据。
	seed        string // 第一次需要抽样时生成，只在当前资产调用内保存。
}

// draw 使用独立的副产物命名空间与原装备号，驱动重试和请求顺序不会重抽。
func (a *equipmentRecycleAttempt) draw(channel, itemID string, denominator uint32) (uint32, error) {
	if a.seed == "" {
		var bytes [32]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return 0, err
		}
		a.seed = hex.EncodeToString(bytes[:])
	}
	return sourceDraw(channel+":draw:"+a.seed+":"+a.operationID+":"+itemID, denominator), nil
}
