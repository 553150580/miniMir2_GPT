// 本文件集中提供圣言术的来源公式，战斗判定与升级预览使用同一计算。
package devconfig

// TurnUndeadBaseThreshold 返回原生阶 rank 对应的第二段基础概率阈值。
// 真实判定还须先通过独立等级检查，再叠加施法者和目标的等级差。
func TurnUndeadBaseThreshold(rank uint32) int64 {
	return (int64(rank)+1)*8 + 15
}
