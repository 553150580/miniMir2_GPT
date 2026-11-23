// 本文件提供服务端对账与每日资格共用的上海运营日。
package persistence

import "time"

// shanghaiOperatingZone 采用当前运营地区的 UTC+8 日界。
var shanghaiOperatingZone = time.FixedZone("Asia/Shanghai", 8*60*60)

// shanghaiOperatingDay 返回服务端事件时刻所属运营日的年月日键。
func shanghaiOperatingDay(at time.Time) string {
	return at.In(shanghaiOperatingZone).Format("2006-01-02")
}
