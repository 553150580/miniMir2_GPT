package integration

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func TestMonsterRespawnAvoidsOccupiedOriginalSpawnMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启独立Mongo验证真实怪物复活和格子占用")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_respawn_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	f := &cfg.Fixture
	f.RuleVersion = max(f.RuleVersion, 26)
	m := &f.Map
	m.Width, m.Height, m.Blocked = 13, 13, nil
	m.PlayerSpawn, m.PetSpawn = devconfig.Point{X: 6, Y: 6}, devconfig.Point{X: 6, Y: 7}
	seed := devconfig.Point{X: 8, Y: 6}
	m.MonsterSpawns = []devconfig.Point{seed, {X: 1, Y: 1}, {X: 11, Y: 1}, {X: 1, Y: 11}, {X: 11, Y: 11}, {X: 2, Y: 8}, {X: 9, Y: 10}, {X: 10, Y: 9}}
	// 只改独立测试怪受伤前态，确保真实火球命中且玩家可占住原出生点等待。
	mob := &f.Monsters[0]
	mob.HP, mob.MoveMS, mob.AttackMin, mob.AttackMax = 1, 60000, 0, 0
	mob.MagicResist, mob.MagicDefenceMin, mob.MagicDefenceMax = 0, 0, 0
	must(t, f.Validate())
	ctx, cancel := context.WithCancel(context.Background())
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	var nodes sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		nodes.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, store.DB.Drop(cleanup))
		store.Close(cleanup)
	})
	for _, mode := range []string{"account", "game"} {
		nodes.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("怪物复活测试%s节点：%v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "respawn_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 1, Name: "复活" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	spell, _ := f.Skill(1)
	_, err = store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{"$set": bson.M{"level": spell.LearnLevel}})
	must(t, err)
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer c.close()
	c.auto(false, 1)
	var book *pb.ItemView
	for _, item := range c.inventory.Items {
		if item.ItemId == spell.BookItemID {
			book = item
			break
		}
	}
	if book == nil {
		t.Fatal("正常建角缺少原入门技能书")
	}
	learned := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: book.InstanceId, SkillId: spell.ID, Operation: c.operation()})
	if learned.ErrorCode != 0 {
		t.Fatal("正常学习入门火球术失败", learned)
	}
	before := proto.Clone(c.entity(10)).(*pb.EntityState)
	c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: spell.ID, TargetHandle: before.EntityHandle})
	travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.entity(10).Hp == 0 })
	observedDeath := time.Now()
	goal := &pb.GridPoint{X: int32(seed.X), Y: int32(seed.Y)}
	c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: goal})
	travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.scene.GetNavigationState().GetStatus() == 2 && proto.Equal(c.player().Position, goal)
	})
	travelUntilWithin(c, 13*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.entity(10).Hp > 0 })
	after := c.entity(10)
	position := devconfig.Point{X: int(after.Position.X), Y: int(after.Position.Y)}
	if time.Since(observedDeath) < 9500*time.Millisecond || after.Life != before.Life+1 || !m.Walkable(position) || position == seed || !proto.Equal(c.player().Position, goal) {
		t.Fatal("实际复活提前、落入玩家格/地图外或重复推进生命", after, time.Since(observedDeath))
	}
	t.Log("真实WS确认普通怪经过原复活等待，同句柄新生命出现于玩家所占旧出生格之外")
}
