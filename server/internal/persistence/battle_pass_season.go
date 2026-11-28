// 本文件保存战令原周期、资格和已领档位。历史周期独立于角色加载与战斗写入。
package persistence

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const battlePassSeasons = "battle_pass_seasons" // 按角色与周期首日唯一，已获得资格可补领。

// battlePassSeason 存储一个角色周期的冻结奖表和紧凑日期水位。
type battlePassSeason struct {
	ID           string                     `bson:"_id"`              // 角色与周期首日拼成的稳定身份。
	Role         string                     `bson:"role_id"`          // 得到资产的原角色，独立于运行服迁移。
	Origin       uint64                     `bson:"origin_server_id"` // 角色的永久入口。
	Start        string                     `bson:"start_day"`        // 周期起点，包含该上海日。
	Tiers        []devconfig.BattlePassTier `bson:"tiers"`            // 首次参与时固定的现金价和总奖励。
	ActivityGoal uint32                     `bson:"activity_goal"`    // 本周期活跃度门槛。
	Tier         uint32                     `bson:"tier"`             // 角色目前最高持有档位。
	Eligible     uint32                     `bson:"eligible_days"`    // 各位记录一个合格运营日。
	Claimed      [3]uint32                  `bson:"claimed_days"`     // 每档独占位图，存储各天实际已发最高档。
	OnceTier     uint32                     `bson:"once_tier"`        // 周期一次奖励已发到的最高档，零表示未发。
	Revision     int64                      `bson:"revision"`         // 同角色、同周期的数据库写入条件。
}

// battlePassDate 解析 ISO 运营日，历法计算统一使用不带本地 DST 的日期。
func battlePassDate(day string) (time.Time, bool) {
	at, err := time.Parse(time.DateOnly, day)
	return at, err == nil && at.Year() >= 2020 && at.Year() <= 2199 && at.Format(time.DateOnly) == day
}

// battlePassWindow 返回在当前上海日所属的有权参与周期首日。
func battlePassWindow(rules *devconfig.BattlePassRules, at time.Time) string {
	if rules == nil {
		return ""
	}
	anchor, ok := battlePassDate(rules.AnchorDay)
	today, valid := battlePassDate(shanghaiOperatingDay(at))
	if !ok || !valid || today.Before(anchor) {
		return ""
	}
	period := int(today.Sub(anchor)/(24*time.Hour)) / devconfig.BattlePassDays
	return anchor.AddDate(0, 0, period*devconfig.BattlePassDays).Format(time.DateOnly)
}

// battlePassDay 返回从一开始的相对日，窗口之外用零表示。
func battlePassDay(start, day string) uint32 {
	first, startValid := battlePassDate(start)
	current, dayValid := battlePassDate(day)
	if !startValid || !dayValid || current.Before(first) || !current.Before(first.AddDate(0, 0, devconfig.BattlePassDays)) {
		return 0
	}
	return uint32(current.Sub(first)/(24*time.Hour)) + 1
}

// battlePassMask 将合法的相对日映射到单个持久位。
func battlePassMask(day uint32) uint32 { return 1 << (day - 1) }

// tierClaimed 查一个日期或周期一次奖的原领取水位。
func (season battlePassSeason) tierClaimed(day uint32) uint32 {
	if day == 0 {
		return season.OnceTier
	}
	mask := battlePassMask(day)
	for index, days := range season.Claimed {
		if days&mask != 0 {
			return uint32(index + 1)
		}
	}
	return 0
}

// setClaimed 在发放已经可原子提交时推进原日期的最高档。
func (season *battlePassSeason) setClaimed(day, tier uint32) {
	if day == 0 {
		season.OnceTier = tier
		return
	}
	mask := battlePassMask(day)
	for index := range season.Claimed {
		season.Claimed[index] &^= mask
	}
	season.Claimed[tier-1] |= mask
}

// battlePassIndexes 分页只扫描原角色的周期记录，永久资格不设失效索引。
func battlePassIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{{Keys: bson.D{{Key: "role_id", Value: 1}, {Key: "start_day", Value: -1}},
		Options: options.Index().SetName("battle_pass_role_period").SetUnique(true)}}
}

// loadBattlePassSeason 核对原角色和固定窗口，缺档时由当前有效配置提供未参与视图。
func (s *Store) loadBattlePassSeason(ctx context.Context, r domain.Role, start string) (battlePassSeason, error) {
	state := battlePassSeason{} // 当前台账，缺省时保留零修订以区分首次写入。
	id := r.ID + ":" + start
	err := s.DB.Collection(battlePassSeasons).FindOne(ctx, bson.M{"_id": id}).Decode(&state)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if state.ID != id || state.Role != r.ID || state.Origin != r.Origin || state.Start != start || !state.valid() {
		return state, Fail(p.CodeOperationConflict, "战令原周期或领取记录不一致")
	}
	return state, nil
}

// newBattlePassSeason 固定第一次有效参与所见的配置，不回写配置文件。
func newBattlePassSeason(r domain.Role, f *devconfig.Fixture, start string) battlePassSeason {
	return battlePassSeason{ID: r.ID + ":" + start, Role: r.ID, Origin: r.Origin, Start: start,
		Tiers: f.BattlePass.Tiers, ActivityGoal: f.TasksCodex.DailyActivityTarget, Tier: devconfig.BattlePassFree}
}

// valid 拒绝无主、越档、重复日期或遗漏资格的历史数据。
func (season battlePassSeason) valid() bool {
	if len(season.Tiers) != int(devconfig.BattlePassMaxTier) || season.ActivityGoal == 0 ||
		season.Tier < devconfig.BattlePassFree || season.Tier > devconfig.BattlePassMaxTier || season.OnceTier > season.Tier ||
		season.Revision <= 0 || season.Eligible >= 1<<devconfig.BattlePassDays {
		return false
	}
	var seen uint32 // 已核对的日期位，三个档位同一天只能有一个。
	for index, mask := range season.Claimed {
		if mask&^season.Eligible != 0 || mask&seen != 0 || mask != 0 && uint32(index+1) > season.Tier {
			return false
		}
		seen |= mask
	}
	return true
}

// persistBattlePassSeason 只在已有资产事务写入角色周期，失败由外层回滚全部事实。
func (s *Store) persistBattlePassSeason(ctx context.Context, state battlePassSeason) error {
	collection := s.DB.Collection(battlePassSeasons)
	if mongo.SessionFromContext(ctx) == nil {
		return Fail(p.CodeOperationConflict, "战令变更需要原资产事务")
	}
	if state.Revision == 0 {
		state.Revision = 1
		_, err := collection.InsertOne(ctx, state)
		return err
	}
	revision := state.Revision
	state.Revision++
	result, err := collection.ReplaceOne(ctx, bson.M{"_id": state.ID, "revision": revision}, state)
	if err == nil && result.MatchedCount != 1 {
		err = Fail(p.CodeOperationConflict, "战令周期已变更")
	}
	return err
}

// addBattlePassEligibility 只从角色已经确认的每日任务活跃度取得当天资格。
func addBattlePassEligibility(state *battlePassSeason, r domain.Role, today string) bool {
	day := battlePassDay(state.Start, today)
	if day == 0 || r.TasksCodex == nil || r.TasksCodex.Daily.Day != today || r.TasksCodex.Daily.Activity < state.ActivityGoal {
		return false
	}
	mask := battlePassMask(day)
	if state.Eligible&mask != 0 {
		return false
	}
	state.Eligible |= mask
	return true
}

// recordBattlePassActivity 在日常任务领取事务中保存资格，过日后原资格仍可补领。
func (s *Store) recordBattlePassActivity(ctx context.Context, r domain.Role, at time.Time) error {
	f := &s.Cfg.Fixture
	start := battlePassWindow(f.BattlePass, at)
	if start == "" || r.TasksCodex == nil {
		return nil
	}
	state, err := s.loadBattlePassSeason(ctx, r, start)
	if err != nil {
		return err
	}
	if state.Revision == 0 {
		state = newBattlePassSeason(r, f, start)
	}
	if !addBattlePassEligibility(&state, r, shanghaiOperatingDay(at)) {
		return nil
	}
	return s.persistBattlePassSeason(ctx, state)
}
