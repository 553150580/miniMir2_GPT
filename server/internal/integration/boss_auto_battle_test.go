// 本文件在隔离真实 MongoDB 和 WebSocket 核对挑战默认攻击和原资产边界。
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

// TestPersonalBossDefaultAutoMongoWS 核对握手后仅靠自动攻击产生唯一挑战胜利回执。
func TestPersonalBossDefaultAutoMongoWS(t *testing.T) {
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		boss := cfg.Fixture.PersonalBoss
		boss.Map.PlayerSpawn, boss.Map.MonsterSpawns = devconfig.Point{X: 5, Y: 5}, []devconfig.Point{{X: 6, Y: 5}}
		boss.Monster.HP, boss.Monster.Agility, boss.Monster.AttackMin, boss.Monster.AttackMax = 1, 0, 0, 0
		boss.Monster.Defence, boss.Monster.DefenceMin, boss.Monster.DefenceMax = 0, 0, 0
		for i := range cfg.Fixture.Items {
			if cfg.Fixture.Items[i].ID == 218 {
				cfg.Fixture.Items[i].AttackMinBonus, cfg.Fixture.Items[i].AttackMaxBonus = 3, 3
			}
		}
		if !cfg.Fixture.ValidPersonalBoss() {
			t.Fatal("默认攻击的隔离首领候选无效")
		}
	})
	c, role, _ := joinWorldBossTestRole(t, endpoint, route)
	defer c.close()
	roleID := domain.Hex(role.Key.RoleId)
	before, err := store.Role(ctx, roleID)
	must(t, err)
	shard := c.entry.SceneEpoch
	request := c.send(p.MsgC2SPersonalBossEnter, &pb.PersonalBossEnter{SceneEpoch: shard,
		ChallengeId: domain.ID(), TemplateId: store.Cfg.Fixture.PersonalBoss.TemplateID})
	var booking pb.PersonalBossState
	bookingFrame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == request })
	if bookingFrame.ID != p.MsgS2CPersonalBossState || bookingFrame.Error != 0 {
		t.Fatalf("隔离首领预约失败：%+v", bookingFrame)
	}
	must(t, proto.Unmarshal(bookingFrame.Payload, &booking))
	challengeID := booking.ChallengeId // 真实场次和客户端请求用于幂等的编号分别由两端生成。
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonalBoss && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	boss := travelEntity(c.scene, 3)
	if c.entry.ServerTransitionId != challengeID || boss.GetHp() != 1 {
		t.Fatal("正确ACK前首领生命或场次身份错误", c.entry, boss)
	}
	// 错误旧场景确认和攻击设置均无权启动新场的技能或改变首领数量。
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: shard, BaselineId: c.entry.BaselineId})
	bad := c.send(p.MsgC2SAutoBattle, &pb.AutoBattleIntent{SceneEpoch: c.entry.SceneEpoch, Enabled: false})
	travelExpectError(c, p.MsgC2SAutoBattle, bad, p.CodeLineRecovering)
	travelAcknowledge(c, false)
	requireChallengeAuto(c, true)
	var result pb.PersonalBossState
	travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CPersonalBossState {
			return false
		}
		must(t, proto.Unmarshal(frame.Payload, &result))
		return result.State >= 4
	})
	if result.State != 4 || result.ChallengeId != challengeID {
		t.Fatal("首领默认攻击未产生同一场次的正常胜利", &result)
	}
	returnFromWorldBossTest(c, challengeID)
	requireChallengeAuto(c, false)
	var terminal persistence.PersonalBossChallenge
	must(t, store.DB.Collection("personal_boss_challenges").FindOne(ctx, bson.M{"_id": challengeID, "role_id": roleID}).Decode(&terminal))
	if terminal.Outcome != persistence.PersonalBossDefeated || terminal.Damage != 1 || terminal.HPRemaining != 0 || terminal.Successes != 1 {
		t.Fatal("正常攻击未用原场次真实击杀结算", terminal)
	}
	after, err := store.Role(ctx, roleID)
	must(t, err)
	travelAssertEconomy(t, before, after, "首领自动攻击")
}
