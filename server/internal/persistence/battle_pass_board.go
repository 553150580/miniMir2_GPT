// 本文件查询当前与历史周期，按角色自己的档位显示现金差价和绑定总奖。
package persistence

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const battlePassHistoryPage = 8 // 单帧历史档案页至多展示八个已参与周期。

// loadBattlePassBoard 不生成领取流水，读取已冻结奖表或当前尚未参与周期的配置。
func (s *Store) loadBattlePassBoard(ctx context.Context, r domain.Role, start string, at time.Time) (battlePassSeason, error) {
	f := &s.Cfg.Fixture
	current := battlePassWindow(f.BattlePass, at)
	if start == "" {
		start = current
	}
	today := shanghaiOperatingDay(at)
	if _, valid := battlePassDate(start); !valid || start > today {
		return battlePassSeason{}, Fail(p.CodeBattlePassUnavailable, "战令周期尚未开放")
	}
	season, err := s.loadBattlePassSeason(ctx, r, start)
	if err != nil {
		return season, err
	}
	if season.Revision == 0 {
		if start != current {
			return season, Fail(p.CodeBattlePassUnavailable, "该角色没有此历史周期的资格")
		}
		season = newBattlePassSeason(r, f, current)
	}
	// 启用前已完成的当天活跃可在首次领取或升档事务补记。
	addBattlePassEligibility(&season, r, today)
	return season, nil
}

// ReadBattlePassState 查询有界历史页和每个实际合格日的原领取档位。
func (s *Store) ReadBattlePassState(ctx context.Context, r domain.Role, scene uint64, start, before string, at time.Time) (*pb.BattlePassState, error) {
	if s.Cfg.Fixture.BattlePass == nil {
		return nil, Fail(p.CodeBattlePassUnavailable, "当前配置未开放战令")
	}
	if before != "" {
		if _, valid := battlePassDate(before); !valid {
			return nil, Fail(p.CodeInvalidArgument, "战令历史游标无效")
		}
	}
	season, err := s.loadBattlePassBoard(ctx, r, start, at)
	if err != nil {
		return nil, err
	}
	end, _ := battlePassDate(season.Start)
	end = end.AddDate(0, 0, devconfig.BattlePassDays)
	today := shanghaiOperatingDay(at)
	state := &pb.BattlePassState{SceneEpoch: scene, RoleRevision: uint64(r.Revision), RuleVersion: s.Cfg.Fixture.RuleVersion,
		SeasonStart: season.Start, SeasonEnd: end.Format(time.DateOnly), OperatingDay: today, Tier: season.Tier,
		SeasonClaimedTier: season.OnceTier, ActivityTarget: season.ActivityGoal}
	open := today < state.SeasonEnd && season.Start == battlePassWindow(s.Cfg.Fixture.BattlePass, at)
	if open && r.TasksCodex != nil && r.TasksCodex.Daily.Day == today {
		state.Activity = min(r.TasksCodex.Daily.Activity, season.ActivityGoal)
	}
	for _, tier := range season.Tiers {
		view := &pb.BattlePassTierView{Tier: tier.ID, TotalCashFen: tier.CashFen, Daily: battlePassRewardView(tier.Daily), Season: battlePassRewardView(tier.Season)}
		if open && tier.ID > season.Tier {
			view.UpgradeCashFen = tier.CashFen - season.Tiers[season.Tier-1].CashFen
		}
		state.Tiers = append(state.Tiers, view)
	}
	for day := uint32(1); day <= devconfig.BattlePassDays; day++ {
		state.Days = append(state.Days, &pb.BattlePassDayView{Day: day, Qualified: season.Eligible&battlePassMask(day) != 0,
			ClaimedTier: season.tierClaimed(day)})
	}
	state.History, state.HistoryHasMore, err = s.battlePassHistory(ctx, r, before)
	return state, err
}

// battlePassHistory 只读当前永久角色的周期身份，不把历史全量读入玩家房间。
func (s *Store) battlePassHistory(ctx context.Context, r domain.Role, before string) ([]string, bool, error) {
	filter := bson.M{"role_id": r.ID, "origin_server_id": r.Origin}
	if before != "" {
		filter["start_day"] = bson.M{"$lt": before}
	}
	cursor, err := s.DB.Collection(battlePassSeasons).Find(ctx, filter, options.Find().SetLimit(battlePassHistoryPage+1).
		SetProjection(bson.M{"_id": 0, "start_day": 1}).SetSort(bson.D{{Key: "start_day", Value: -1}}))
	if err != nil {
		return nil, false, err
	}
	var rows []struct {
		Day string `bson:"start_day"` // 数据库索引的运营日键。
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, false, err
	}
	history := make([]string, 0, min(battlePassHistoryPage, len(rows)))
	for _, row := range rows[:min(battlePassHistoryPage, len(rows))] {
		history = append(history, row.Day)
	}
	return history, len(rows) > battlePassHistoryPage, nil
}

// battlePassRewardView 只把已冻结的绑定总奖励发给客户端用于展示。
func battlePassRewardView(reward devconfig.TaskCodexReward) *pb.BattlePassReward {
	view := &pb.BattlePassReward{BoundGold: reward.BoundGold, BoundYuanbaoUnits: reward.BoundYuanbaoUnits}
	for _, material := range reward.Items {
		view.Materials = append(view.Materials, &pb.BattlePassMaterial{ItemId: material.ItemID, Quantity: material.Quantity})
	}
	return view
}
