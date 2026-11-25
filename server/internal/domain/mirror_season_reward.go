// 本文件定义已确认镜像赛季奖的绑定元宝。
package domain

// MirrorSeasonAssets 是赛季封账时冻结的附件。
type MirrorSeasonAssets struct {
	BoundIngot int64 `bson:"bound_ingot,omitempty" json:"bound_ingot,omitempty"` // 与绑定元宝钱包同一定点单位。
}
