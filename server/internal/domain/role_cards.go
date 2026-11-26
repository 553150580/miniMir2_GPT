// 本文件判断角色当前连续卡期内的权益及其精确生效边界。
package domain

import "time"

// PrivilegeActiveAt 判断当前连续特权期是否覆盖给定时刻。
func (r Role) PrivilegeActiveAt(at time.Time) bool {
	return r.PrivilegeUntil.After(at) && (r.PrivilegeSince.IsZero() || !at.Before(r.PrivilegeSince))
}

// HostingCardActiveAt 判断当前连续托管卡期是否覆盖给定时刻。
func (r Role) HostingCardActiveAt(at time.Time) bool {
	return r.HostingCardUntil.After(at) && (r.HostingCardSince.IsZero() || !at.Before(r.HostingCardSince))
}
