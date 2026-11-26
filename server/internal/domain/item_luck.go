// 本文件定义装备当前幸运与历史初始幸运的兼容读取方式。
package domain

// EffectiveLuck 优先返回已培养的当前幸运。旧物品及新生成未培养的物品沿用初始幸运。
// 合法的用油回退不会跨过原初始幸运，因而当前幸运零可以兼作未保存标记。
func (i Item) EffectiveLuck() uint32 {
	if i.CurrentLuck != 0 {
		return i.CurrentLuck
	}
	return i.InitialLuck
}
