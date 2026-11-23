// 本文件记录角色的任务成就、怪物图鉴与运营日进度。
package domain

// TaskCodexProgress 属于角色聚合，和来源水位、已领取资产一起原子确认。
type TaskCodexProgress struct {
	Permanent map[string]TaskCodexCounter `bson:"permanent,omitempty" json:"permanent,omitempty"` // 主线、成就和图鉴以种类与定义编号为键。
	Daily     TaskCodexDailyProgress      `bson:"daily,omitempty" json:"daily,omitempty"`         // 当前保留的最近运营日进度。
}

// TaskCodexCounter 保存定义已确认的达成数量和领取事实。
type TaskCodexCounter struct {
	Count   int64 `bson:"count" json:"count"`                         // 已确认的目标数量，达到目标后停止增长。
	Claimed bool  `bson:"claimed,omitempty" json:"claimed,omitempty"` // 该定义对应的奖励是否已写入。
}

// TaskCodexDailyProgress 记录一个运营日的任务及活跃资格。
type TaskCodexDailyProgress struct {
	Day      string                      `bson:"day" json:"day"`                               // 上海运营日，格式为 YYYY-MM-DD。
	Counters map[string]TaskCodexCounter `bson:"counters,omitempty" json:"counters,omitempty"` // 当日任务与活跃奖励的领取记录。
	Claimed  uint32                      `bson:"claimed,omitempty" json:"claimed,omitempty"`   // 当日已领奖的每日任务数量。
	Activity uint32                      `bson:"activity,omitempty" json:"activity,omitempty"` // 当日已通过确认领取取得的活跃度。
}
