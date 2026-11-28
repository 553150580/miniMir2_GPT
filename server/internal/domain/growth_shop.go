// 本文件保存当前上海运营日已确认的商城购买水位。
package domain

// GrowthShopDay 是随永久角色原子提交的一天材料限额。
type GrowthShopDay struct {
	Day    string            `bson:"day"`    // 服务器生成的上海年月日。
	Counts []GrowthShopCount `bson:"counts"` // 按材料编号升序记录，两种币种共用。
}

// GrowthShopCount 记录某个材料在当前运营日已买的实际数量。
type GrowthShopCount struct {
	ItemID uint32 `bson:"item_id"` // 唯一材料模板编号。
	Bought uint32 `bson:"bought"`  // 服务端成功到账的累计材料个数。
}
