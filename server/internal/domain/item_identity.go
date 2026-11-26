// 本文件检查当前角色直接持有的道具实例身份。
package domain

// HasItemInstance 判断背包或穿戴中是否已经存在该原实例编号。
func (r Role) HasItemInstance(id string) bool {
	for _, item := range r.Items {
		if item.ID == id {
			return true
		}
	}
	for _, entry := range r.Equipped {
		if entry.Item.ID == id {
			return true
		}
	}
	return false
}
