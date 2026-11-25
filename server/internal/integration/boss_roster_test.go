// 本文件用隔离用户和真实格子场核对已投影的候选首领选择及共享场次。
package integration

import (
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

// TestPersonalBossSelectableRosterMongoWS 按真实原怪生命进入新个人和共享地图。
func TestPersonalBossSelectableRosterMongoWS(t *testing.T) {
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		if len(cfg.Fixture.PersonalBossVariants) != 2 || len(cfg.Fixture.WorldBossVariants) != 1 {
			t.Fatal("当前开发配置没有完整投影三种个人、两种世界首领")
		}
		// 独立放宽入场等级，保留原始怪物数值及真实地图，避免将预置资格冒作自然成长。
		cfg.Fixture.PersonalBossVariants[0].MinimumLevel = 1
		cfg.Fixture.WorldBossVariants[0].MinimumLevel = 1
		local := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60))
		midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
		const day = 24 * time.Hour
		offset := (local.Sub(midnight).Truncate(time.Millisecond) + day - time.Minute) % day
		cfg.Fixture.WorldBoss.DailyOpenOffsets = []time.Duration{(offset + 3*time.Hour) % day}
		cfg.Fixture.WorldBossVariants[0].DailyOpenOffsets = []time.Duration{offset}
	})
	c, role, _ := joinWorldBossTestRole(t, endpoint, route)
	defer c.close()
	roleID := domain.Hex(role.Key.RoleId)
	before, err := store.Role(ctx, roleID)
	must(t, err)
	query := c.send(p.MsgC2SPersonalBossQuery, &pb.PersonalBossQuery{})
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == query && frame.ID == p.MsgS2CPersonalBossInfo })
	var info pb.PersonalBossInfo
	must(t, proto.Unmarshal(frame.Payload, &info))
	if len(info.Templates) != 3 || info.Templates[0].TemplateId != 1 || info.Templates[1].TemplateId != 3 ||
		!info.Templates[1].CanEnter || info.Templates[1].MapId != 82 || info.Templates[2].TemplateId != 4 || info.Templates[2].CanEnter ||
		info.Templates[2].EligibilityReason != persistence.PersonalBossLevelRequired {
		t.Fatal("可选个人首领没有按模板和真实等级返回独立资格", &info)
	}
	request := c.send(p.MsgC2SPersonalBossEnter, &pb.PersonalBossEnter{SceneEpoch: c.entry.SceneEpoch, ChallengeId: domain.ID(), TemplateId: 3})
	bookingFrame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == request })
	if bookingFrame.ID != p.MsgS2CPersonalBossState || bookingFrame.Error != 0 {
		t.Fatalf("可选个人首领预约失败：%+v", bookingFrame)
	}
	var booking pb.PersonalBossState
	must(t, proto.Unmarshal(bookingFrame.Payload, &booking))
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonalBoss && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	assertRosterBoss(c, 82, 80003, 500)
	travelAcknowledge(c, false, false)
	c.send(p.MsgC2SPersonalBossLeave, &pb.PersonalBossLeave{SceneEpoch: c.entry.SceneEpoch, ChallengeId: booking.ChallengeId})
	returnFromWorldBossTest(c, booking.ChallengeId)
	var receipt persistence.PersonalBossChallenge
	must(t, store.DB.Collection("personal_boss_challenges").FindOne(ctx, bson.M{"_id": booking.ChallengeId, "role_id": roleID}).Decode(&receipt))
	if receipt.TemplateID != 3 || receipt.Outcome != persistence.PersonalBossInterrupted || receipt.HPRemaining != 500 || receipt.Successes != 0 {
		t.Fatal("个人原挑战状态没有保留所选模板和剩余生命", receipt)
	}
	open := queryWorldBossTest(t, c)
	if open.TemplateId != 5 || open.State != 3 || open.Hp != 10_000 || open.HpMax != open.Hp {
		t.Fatal("当前运行服没有按配置选中晚场祖玛教主", open)
	}
	wrong := c.send(p.MsgC2SWorldBossEnter, &pb.WorldBossEnter{SceneEpoch: c.entry.SceneEpoch, TemplateId: 2, OccurrenceId: open.OccurrenceId})
	travelExpectError(c, p.MsgC2SWorldBossEnter, wrong, p.CodeLineForbidden)
	enterWorldBossTest(t, c, open)
	assertRosterBoss(c, 84, 80005, 10_000)
	c.send(p.MsgC2SWorldBossLeave, &pb.WorldBossLeave{SceneEpoch: c.entry.SceneEpoch, OccurrenceId: open.OccurrenceId})
	waitWorldBossTest(t, c, 0, func(state *pb.WorldBossState) bool { return state.State == 6 })
	returnFromWorldBossTest(c, open.OccurrenceId)
	after, err := store.Role(ctx, roleID)
	must(t, err)
	travelAssertEconomy(t, before, after, "首领模板选择")
}

// assertRosterBoss 校验绑定地图原首领实体和第一个场景基线的未变更生命。
func assertRosterBoss(c *client, mapID uint64, monsterID uint32, health int64) {
	c.t.Helper()
	boss := travelEntity(c.scene, 3)
	if c.entry.RoomId != mapID || travelCount(c.scene, 3) != 1 || boss.GetDefinitionId() != monsterID || boss.GetHp() != health {
		c.t.Fatal("首领场未使用对应地图、原模板和已核对生命", c.entry, boss)
	}
}
