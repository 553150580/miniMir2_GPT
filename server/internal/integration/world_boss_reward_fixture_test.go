// 本文件以真实账号服、WebSocket 和权威战斗准备世界首领奖励测试共同前置。
package integration

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// worldBossRewardPlayer 保存一次性角色凭据、当前连接和真实战斗确认的原奖。
type worldBossRewardPlayer struct {
	account  string                     // 隔离账号名，浏览器经环境变量读取。
	password string                     // 一次性口令，不写入测试证据。
	token    string                     // 仅 Go 测试客户端使用的账号登录凭据。
	summary  *pb.RoleSummary            // 实际创建的永久角色。
	client   *client                    // 参战及返回原地图的 WebSocket 观察者。
	reward   *pb.ChallengePendingReward // 真实击杀后服务端下发、领取前未变更的附件。
}

// worldBossRewardTrial 保存同一隔离运行服、永久场次和两个已确认参战者。
type worldBossRewardTrial struct {
	ctx      context.Context    // 跟随隔离节点生命周期。
	store    *persistence.Store // 对拍开发资产和原结果的数据库句柄。
	endpoint string             // 本次隔离账号服 RPC 入口。
	route    directoryTestEntry // 区服公布的运行服和游戏服。
	sealed   *pb.WorldBossState // 原 WebSocket 返回的永久击杀和排行榜。
	winner   worldBossRewardPlayer
	partner  worldBossRewardPlayer
}

// prepareWorldBossRewardTrial 用两个协议客户端实际击杀共享首领。数据库只准备伤害之外的卡时效和背包容量。
func prepareWorldBossRewardTrial(t *testing.T, tightWinner bool) worldBossRewardTrial {
	t.Helper()
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		start := time.Now().UTC().Truncate(time.Millisecond).Add(-2 * time.Second)
		setDurableWorldBossFixture(t, &cfg.Fixture, start, start.Add(10*time.Minute), 5*time.Minute)
		cfg.Fixture.WorldBoss.Rewards = &devconfig.WorldBossRewards{
			MinimumDamage: 1, Participation: []devconfig.ChallengeRewardItem{{ItemID: 95, Quantity: 2}},
			Ranks: []devconfig.WorldBossRankReward{{FirstRank: 1, LastRank: 1, Items: []devconfig.ChallengeRewardItem{{ItemID: 96, Quantity: 1}}}},
		}
		if !cfg.Fixture.ValidWorldBossRewards(cfg.Fixture.WorldBoss) {
			t.Fatal("隔离奖励未通过正式装配校验")
		}
	})
	trial := worldBossRewardTrial{ctx: ctx, store: store, endpoint: endpoint, route: route}
	trial.winner = trial.create(t, "首领奖甲", time.Now().Add(time.Hour), tightWinner)
	// 同一张卡在真实入场时有效，并在实际最后一击前到期。
	partnerExpiry := time.Now().UTC().Add(15 * time.Second).Truncate(time.Millisecond)
	trial.partner = trial.create(t, "首领奖乙", partnerExpiry, false)
	first, second := trial.attach(t, &trial.winner, true), trial.attach(t, &trial.partner, true)
	trial.winner.client, trial.partner.client = first, second
	open := queryWorldBossTest(t, first)
	enterWorldBossTest(t, first, open)
	enterWorldBossTest(t, second, open)
	if !time.Now().Before(partnerExpiry) {
		t.Fatal("第二个玩家尚未确认入场时隔离卡已经到期")
	}
	goal := &pb.GridPoint{X: 6, Y: 4}
	second.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: second.entry.SceneEpoch, Destination: goal})
	travelUntilWithin(second, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && proto.Equal(second.player().Position, goal)
	})
	attackWorldBossTest(first)
	waitWorldBossTest(t, second, 0, func(state *pb.WorldBossState) bool { return state.State == 3 && state.Hp == 2 })
	time.Sleep(time.Duration(second.player().MoveIntervalMs)*time.Millisecond + 100*time.Millisecond)
	time.Sleep(time.Until(partnerExpiry.Add(100 * time.Millisecond)))
	attackWorldBossTest(second)
	sealed := waitWorldBossTest(t, second, 0, func(state *pb.WorldBossState) bool { return state.State == 4 })
	if sealed.LatestResult == nil || len(sealed.Ranks) != 2 || sealed.Ranks[0].DisplayName != trial.winner.summary.Name || sealed.Ranks[1].DisplayName != trial.partner.summary.Name {
		t.Fatal("参与与排行奖励需要同一份真实伤害榜", sealed)
	}
	trial.sealed = sealed
	if sealed.GetLatestResult().GetFinishedAtUnixMs() <= partnerExpiry.UnixMilli() {
		t.Fatal("服务端实际最后一击没有跨过原特权截止", sealed)
	}
	returnFromWorldBossTest(first, open.OccurrenceId)
	returnFromWorldBossTest(second, open.OccurrenceId)
	firstRewards, secondRewards := reconnectBossInfo(t, first), reconnectBossInfo(t, second)
	if len(firstRewards.WorldPendingRewards) != 1 || len(secondRewards.WorldPendingRewards) != 1 ||
		len(firstRewards.PendingRewards) != 0 || len(secondRewards.PendingRewards) != 0 {
		t.Fatal("世界奖励未独立保留，或挤占了个人队列", firstRewards, secondRewards)
	}
	firstReward, secondReward := firstRewards.WorldPendingRewards[0], secondRewards.WorldPendingRewards[0]
	if firstReward.SourceKind != "world_boss" || secondReward.SourceKind != "world_boss" || firstReward.SourceRank != 1 || secondReward.SourceRank != 2 ||
		len(firstReward.Items) != 2 || len(secondReward.Items) != 1 || firstReward.Items[0].GetBound() || firstReward.Items[1].GetBound() || !secondReward.Items[0].GetBound() ||
		firstReward.EarnedAtUnixMs != sealed.LatestResult.GetFinishedAtUnixMs() || secondReward.EarnedAtUnixMs != firstReward.EarnedAtUnixMs {
		t.Fatal("原击杀、排名、参与奖或按各角色判绑不一致", firstReward, secondReward, sealed)
	}
	trial.winner.reward, trial.partner.reward = firstReward, secondReward
	return trial
}

// create 注册实际测试角色，只设置本轮要观测的卡截止和可选满包初始条件。
func (trial worldBossRewardTrial) create(t *testing.T, prefix string, privilege time.Time, tightBag bool) worldBossRewardPlayer {
	t.Helper()
	account, password := "wbreward_"+domain.ID()[:8], domain.ID()[:16]
	login := &pb.LoginResult{}
	rpc(t, trial.endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: account, Password: password}, login, 0)
	summary := createDirectoryRole(t, trial.endpoint, login.AccountToken, trial.route.origin.OriginID, prefix)
	id := domain.Hex(summary.Key.RoleId)
	updates := bson.M{"privilege_until": privilege.UTC().Truncate(time.Millisecond)}
	if tightBag {
		r, err := trial.store.Role(trial.ctx, id)
		must(t, err)
		updates["capacity"] = len(r.Items)
	}
	result, err := trial.store.DB.Collection("roles").UpdateOne(trial.ctx, bson.M{"_id": id}, bson.M{"$set": updates})
	must(t, err)
	if result.ModifiedCount != 1 {
		t.Fatal("没有建立隔离角色的权益前态")
	}
	return worldBossRewardPlayer{account: account, password: password, token: login.AccountToken, summary: summary}
}

// attach 按原登录账号申请正常区服票据，必要时通过原资产操作装备初始木剑。
func (trial worldBossRewardTrial) attach(t *testing.T, player *worldBossRewardPlayer, equip bool) *client {
	t.Helper()
	ticket := directoryTicket(t, trial.endpoint, player.token, player.summary.Key, trial.route)
	c := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
	t.Cleanup(c.close)
	c.auto(false, 1)
	if !equip {
		return c
	}
	for _, item := range c.inventory.Items {
		if item.ItemId != 218 {
			continue
		}
		res := c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: item.InstanceId, ItemVersion: item.Version, Operation: c.operation()})
		if res.ErrorCode != 0 {
			t.Fatal("未能通过普通流程穿原木剑", res)
		}
		c.inventoryAt(res.NewRevision)
		return c
	}
	t.Fatal("原角色缺少木剑")
	return c
}
