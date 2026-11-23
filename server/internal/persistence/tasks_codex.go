// 本文件从已确认的服务端来源推进任务和图鉴，并在角色资产事务内生成绑定奖励。
package persistence

import (
	"maps"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"strconv"
	"time"
)

// AssetTaskCodexClaim 对应玩家主动领取，使用角色原有的经济命令水位。
const AssetTaskCodexClaim = "tasks_codex_claim"

// taskCodexEvent 是来源事务已验证的事实，同一来源的永久回执重放不会再次产生事件。
type taskCodexEvent struct {
	kind      string    // 事件在配置中使用的名称。
	count     int64     // 本来源实际数量或已达到的等级。
	monsterID uint32    // 实际击杀的怪物模板，非击杀为零。
	mapID     uint64    // 事件发生的原地图，非场景事件为零。
	at        time.Time // 事实发生时刻，用于归属上海运营日。
}

// TaskCodexClaimReceipt 冻结首次领取所用的目标身份、发布版本和完整奖励。
type TaskCodexClaimReceipt struct {
	Kind              uint32        `bson:"kind"`                          // 目标所属任务或图鉴类型。
	ID                uint32        `bson:"id"`                            // 对应该类定义编号。
	Day               string        `bson:"day,omitempty"`                 // 每日或活跃领取时绑定的上海运营日。
	BoundGold         int64         `bson:"bound_gold,omitempty"`          // 实际加到绑定金币账户的金额。
	BoundYuanbaoUnits int64         `bson:"bound_yuanbao_units,omitempty"` // 实际加到绑定元宝账户的金额。
	Items             []domain.Item `bson:"items,omitempty"`               // 实际新增的绑定物品及各实例身份。
	RuleVersion       uint64        `bson:"rule_version"`                  // 领取采用的发布规则版本。
	ConfigHash        string        `bson:"config_hash"`                   // 领取采用的不可变配置摘要。
}

// taskCodexKey 将种类与编号编码成 BSON 可安全作为字段使用的固定身份。
func taskCodexKey(kind, id uint32) string {
	return strconv.FormatUint(uint64(kind), 10) + ":" + strconv.FormatUint(uint64(id), 10)
}

// taskCodexDailyKind 表示领取需要绑定运营日的两类定义。
func taskCodexDailyKind(kind uint32) bool {
	return kind == devconfig.TaskCodexDaily || kind == devconfig.TaskCodexActivity
}

// taskCodexProgress 返回已确认进度，兼容发布前未保存进度的角色。
func taskCodexProgress(r domain.Role) domain.TaskCodexProgress {
	if r.TasksCodex == nil {
		return domain.TaskCodexProgress{}
	}
	return *r.TasksCodex
}

// taskCodexToday 读取目标运营日的进度，跨日只展示新的一天。
func taskCodexToday(state domain.TaskCodexProgress, day string) domain.TaskCodexDailyProgress {
	if state.Daily.Day == day {
		return state.Daily
	}
	return domain.TaskCodexDailyProgress{Day: day}
}

// taskCodexUnlocked 检查角色等级和主线前置领取。
func taskCodexUnlocked(r domain.Role, state domain.TaskCodexProgress, def devconfig.TaskCodexDefinition) bool {
	return r.Level >= def.MinimumLevel && (def.PrerequisiteID == 0 || state.Permanent[taskCodexKey(devconfig.TaskCodexMain, def.PrerequisiteID)].Claimed)
}

// taskCodexCounter 取指定定义的已确认进度并将活跃度映射到活跃领取条目。
func taskCodexCounter(state domain.TaskCodexProgress, today domain.TaskCodexDailyProgress, def devconfig.TaskCodexDefinition, key string) domain.TaskCodexCounter {
	if taskCodexDailyKind(def.Kind) {
		counter := today.Counters[key]
		if def.Kind == devconfig.TaskCodexActivity {
			counter.Count = int64(today.Activity)
		}
		return counter
	}
	return state.Permanent[key]
}

// taskCodexVisibleCounter 用已经持久化的等级核对主线与成就的等级目标，支持旧角色与前置刚解锁的角色。
func taskCodexVisibleCounter(r domain.Role, state domain.TaskCodexProgress, today domain.TaskCodexDailyProgress, def devconfig.TaskCodexDefinition, key string) domain.TaskCodexCounter {
	counter := taskCodexCounter(state, today, def, key)
	if def.Event == devconfig.TaskCodexLevelReached && !taskCodexDailyKind(def.Kind) && taskCodexUnlocked(r, state, def) {
		counter.Count = max(counter.Count, min(int64(r.Level), def.Target))
	}
	return counter
}

// taskCodexCanClaim 判断目标是否达成且存在对应领取资格。
func taskCodexCanClaim(r domain.Role, state domain.TaskCodexProgress, today domain.TaskCodexDailyProgress, def devconfig.TaskCodexDefinition, counter domain.TaskCodexCounter, dailyClaimLimit uint32) bool {
	return taskCodexUnlocked(r, state, def) && !counter.Claimed && counter.Count >= def.Target &&
		(def.Kind != devconfig.TaskCodexDaily || today.Claimed < dailyClaimLimit)
}

// TaskCodexState 只从原角色的已确认持久状态构造结果，读列表不增加角色修订版本。
func TaskCodexState(r domain.Role, f *devconfig.Fixture, at time.Time) *pb.TaskCodexStateResult {
	day := shanghaiOperatingDay(at)
	out := &pb.TaskCodexStateResult{RoleRevision: uint64(r.Revision), RuleVersion: f.RuleVersion, Day: day}
	rules := f.TasksCodex
	if rules == nil || len(rules.Definitions) == 0 {
		out.ErrorCode = p.CodeTaskCodexUnavailable
		return out
	}
	state := taskCodexProgress(r)
	today := taskCodexToday(state, day)
	out.Activity, out.DailyClaimed = today.Activity, today.Claimed
	for _, def := range rules.Definitions {
		key := taskCodexKey(def.Kind, def.ID)
		counter := taskCodexVisibleCounter(r, state, today, def, key)
		row := &pb.TaskCodexStateRow{
			Kind: def.Kind, Id: def.ID, Name: def.Name, Progress: min(counter.Count, def.Target), Target: def.Target,
			Available: taskCodexUnlocked(r, state, def), Claimed: counter.Claimed,
			Claimable: taskCodexCanClaim(r, state, today, def, counter, rules.DailyClaimLimit), MapId: def.MapID, MonsterId: def.MonsterID,
			BoundGold: def.Reward.BoundGold, BoundYuanbaoUnits: def.Reward.BoundYuanbaoUnits,
		}
		for _, item := range def.Reward.Items {
			row.RewardItems = append(row.RewardItems, &pb.TaskCodexRewardItem{ItemId: item.ItemID, Quantity: item.Quantity})
		}
		out.Rows = append(out.Rows, row)
		if def.Kind == devconfig.TaskCodexDaily {
			out.DailyClaimLimit = rules.DailyClaimLimit
		}
	}
	return out
}

// advanceTasksCodex 将单个可信事件计入适用定义，仅为发生变化的进度复制数据。
func advanceTasksCodex(r domain.Role, f *devconfig.Fixture, event taskCodexEvent) domain.Role {
	if f.TasksCodex == nil || event.count <= 0 {
		return r
	}
	state := taskCodexProgress(r)
	day := shanghaiOperatingDay(event.at)
	updatedPermanent, updatedDaily := false, false
	for _, def := range f.TasksCodex.Definitions {
		if def.Event != event.kind || def.MonsterID != 0 && def.MonsterID != event.monsterID ||
			def.Kind != devconfig.TaskCodexMonster && (def.MapID != 0 && def.MapID != event.mapID || !taskCodexUnlocked(r, state, def)) {
			continue
		}
		daily := def.Kind == devconfig.TaskCodexDaily
		if daily && (event.at.IsZero() || day < state.Daily.Day) {
			continue
		}
		key := taskCodexKey(def.Kind, def.ID)
		counter := taskCodexCounter(state, taskCodexToday(state, day), def, key)
		if counter.Claimed || counter.Count >= def.Target {
			continue
		}
		if event.kind == devconfig.TaskCodexLevelReached {
			counter.Count = min(def.Target, max(counter.Count, event.count))
		} else {
			counter.Count += min(event.count, def.Target-counter.Count)
		}
		if daily {
			if !updatedDaily {
				state.Daily = taskCodexToday(state, day)
				state.Daily.Counters = copyTaskCodexCounters(state.Daily.Counters)
				updatedDaily = true
			}
			state.Daily.Counters[key] = counter
		} else {
			if !updatedPermanent {
				state.Permanent = copyTaskCodexCounters(state.Permanent)
				updatedPermanent = true
			}
			state.Permanent[key] = counter
		}
	}
	if updatedDaily || updatedPermanent {
		r.TasksCodex = &state
	}
	return r
}

// copyTaskCodexCounters 为成功写入创建新映射，保持旧角色状态及失败回滚内容安全。
func copyTaskCodexCounters(counters map[string]domain.TaskCodexCounter) map[string]domain.TaskCodexCounter {
	if counters == nil {
		return make(map[string]domain.TaskCodexCounter)
	}
	return maps.Clone(counters)
}

// advanceAssetTasks 只把已经确定成功的学习和实际回收数量转换为任务事件。
func advanceAssetTasks(r domain.Role, f *devconfig.Fixture, kind string, recycled *EquipmentRecyclingReceipt, at time.Time) domain.Role {
	if kind == "learn_skill" {
		r = advanceTasksCodex(r, f, taskCodexEvent{kind: devconfig.TaskCodexSkillLearn, count: 1, at: at})
	}
	if recycled != nil && len(recycled.Recycled) > 0 {
		r = advanceTasksCodex(r, f, taskCodexEvent{kind: devconfig.TaskCodexItemRecycle, count: int64(len(recycled.Recycled)), at: at})
	}
	return r
}

// advanceSettlementTasks 从同一结算内已计算成功的击杀、升级和回收提取任务进度。
func advanceSettlementTasks(before, next domain.Role, f *devconfig.Fixture, q Settlement, receipt SettlementReceipt) domain.Role {
	if f.TasksCodex == nil {
		return next
	}
	if q.Kills > 0 && q.MonsterID != 0 {
		next = advanceTasksCodex(next, f, taskCodexEvent{kind: devconfig.TaskCodexMonsterKill, count: q.Kills, monsterID: q.MonsterID, mapID: q.MapID, at: q.EventAt})
	}
	if next.Level > before.Level {
		next = advanceTasksCodex(next, f, taskCodexEvent{kind: devconfig.TaskCodexLevelReached, count: int64(next.Level), at: q.EventAt})
	}
	return advanceAssetTasks(next, f, "settlement", &receipt.Recycling, q.EventAt)
}

// claimTaskCodex 核对当日或永久资格，在现有资产事务内同时提交进度和所有奖励。
func claimTaskCodex(r domain.Role, f *devconfig.Fixture, q *pb.TaskCodexClaimRequest, at time.Time, opID string) (domain.Role, *TaskCodexClaimReceipt, uint32) {
	if q.Kind < devconfig.TaskCodexMain || q.Kind > devconfig.TaskCodexActivity || q.Id == 0 || !taskCodexDailyKind(q.Kind) && q.Day != "" {
		return r, nil, p.CodeInvalidArgument
	}
	rules := f.TasksCodex
	if rules == nil {
		return r, nil, p.CodeTaskCodexUnavailable
	}
	day := shanghaiOperatingDay(at)
	state := taskCodexProgress(r)
	today := taskCodexToday(state, day)
	if taskCodexDailyKind(q.Kind) && (q.Day != day || state.Daily.Day > day) {
		return r, nil, p.CodeTaskCodexDayChanged
	}
	for _, def := range rules.Definitions {
		if def.Kind != q.Kind || def.ID != q.Id {
			continue
		}
		key := taskCodexKey(def.Kind, def.ID)
		counter := taskCodexVisibleCounter(r, state, today, def, key)
		if !taskCodexCanClaim(r, state, today, def, counter, rules.DailyClaimLimit) {
			break
		}
		receipt := &TaskCodexClaimReceipt{Kind: def.Kind, ID: def.ID, BoundGold: def.Reward.BoundGold,
			BoundYuanbaoUnits: def.Reward.BoundYuanbaoUnits, RuleVersion: f.RuleVersion, ConfigHash: f.Hash}
		next, code := taskCodexReward(r, f, def.Reward, receipt, opID)
		if code != 0 {
			return r, nil, code
		}
		counter.Claimed = true
		if taskCodexDailyKind(def.Kind) {
			receipt.Day = day
			today.Counters = copyTaskCodexCounters(today.Counters)
			if def.Kind == devconfig.TaskCodexDaily {
				today.Claimed++
				today.Activity = uint32(min(uint64(rules.DailyActivityTarget), uint64(today.Activity)+uint64(def.Activity)))
			}
			today.Counters[key] = counter
			state.Daily = today
		} else {
			state.Permanent = copyTaskCodexCounters(state.Permanent)
			state.Permanent[key] = counter
		}
		next.TasksCodex = &state
		return next, receipt, 0
	}
	return r, nil, p.CodeTaskCodexUnavailable
}

// taskCodexReward 检查全部货币与物品空间，成功后才替换原钱包和背包。
func taskCodexReward(r domain.Role, f *devconfig.Fixture, reward devconfig.TaskCodexReward, receipt *TaskCodexClaimReceipt, opID string) (domain.Role, uint32) {
	next := r
	gold, err := add(r.Wallet.BoundGold, reward.BoundGold)
	if err != nil {
		return r, Code(err)
	}
	yuanbao, err := add(r.Wallet.BoundYuanbao, reward.BoundYuanbaoUnits)
	if err != nil {
		return r, Code(err)
	}
	source := "task_reward"
	if receipt.Kind == devconfig.TaskCodexMonster {
		source = "codex_reward"
	}
	for _, grant := range reward.Items {
		itemDef, exists := f.Item(grant.ItemID)
		if !exists || grant.Quantity <= 0 {
			return r, p.CodeInvalidArgument
		}
		stack := int64(1)
		if itemDef.Stackable && itemDef.MaxStack > 1 {
			stack = itemDef.MaxStack
		}
		if (grant.Quantity-1)/stack+1 > int64(r.Capacity-len(r.Items)-len(receipt.Items)) {
			return r, p.CodeEquipmentBagFull
		}
		for left := grant.Quantity; left > 0; left -= min(left, stack) {
			item := domain.Item{ID: domain.ID(), ItemID: grant.ItemID, Quantity: min(left, stack), Bound: true, Version: 1,
				SourceKind: source, SourceID: opID, Quality: itemDef.DropQuality}
			receipt.Items = append(receipt.Items, InitializeItemDurability(item, f))
		}
	}
	if len(receipt.Items) > 0 {
		next.Items = append(append([]domain.Item(nil), r.Items...), receipt.Items...)
	}
	next.Wallet.BoundGold, next.Wallet.BoundYuanbao = gold, yuanbao
	return next, 0
}
