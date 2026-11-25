// 验证自动战斗在托管和手动走位之间仍锁定同一只怪物的同一生命代次。

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

// 隔离场景让两只活怪有稳定的远近关系。走格、主动点选、施法与重连都使用真实WS。
// TestAutoTargetStaysOnSelectedLifeAcrossHostingAndManualMoveMongoWS 确认手动走位或连接状态切换不会把自动战斗锁转给另一只尚未选中的怪物。
// t 提供断言失败报告、子用例和清理注册。
func TestAutoTargetStaysOnSelectedLifeAcrossHostingAndManualMoveMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启独立Mongo以验证定点锁怪、恢复射程和健康重入")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_lock_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	// m 保存当前处理所用的地图。
	m := &cfg.Fixture.Map
	m.PlayerSpawn = devconfig.Point{X: 0, Y: 8}
	m.Blocked = nil
	m.MonsterSpawns = []devconfig.Point{
		{X: m.Width - 1, Y: m.Height - 1}, {X: 1, Y: 8}, {X: 2}, {X: 3}, {X: 4}, {X: 5}, {X: 6}, {X: 7},
	}
	cfg.Fixture.Monsters[0].MoveMS, cfg.Fixture.Monsters[0].AttackMS = 120000, 120000
	cfg.Fixture.Monsters[0].AttackMin, cfg.Fixture.Monsters[0].AttackMax, cfg.Fixture.Monsters[0].HP = 0, 0, 1000
	must(t, cfg.Fixture.Validate())
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancel(context.Background())
	// store 保存由打开取得的持久层存储。
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	// nodes 记录节点集合，使用协程汇合计数器表示。
	var nodes sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		nodes.Wait()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, store.DB.Drop(cleanup))
		store.Close(cleanup)
	})
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"account", "game"} {
		nodes.Go(func() {
			// err 接收运行批次过程中可能产生的错误，供后续失败分支处理。
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("锁怪验收%s节点：%v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "lock_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	// role 保存当前处理所用的角色。
	role := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole,
		&pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 1, Name: "目标" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	// roleID 标识当前业务使用的角色。
	roleID := domain.Hex(role.Key.RoleId)
	_, err = store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"level": 7}})
	must(t, err)
	// c 接收加入供后续步骤使用。
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer func() { c.close() }()
	// learned 保存当前处理所用的已学习。
	learned := false
	// item 接收遍历道具集合时当前的元素。
	for _, item := range c.inventory.Items {
		if item.ItemId == 7 {
			// result 保存由资产取得的结果。
			result := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: item.InstanceId, SkillId: 1, Operation: c.operation()})
			if result.ErrorCode != 0 {
				t.Fatal("正常学习新手火球术失败", result)
			}
			c.inventoryAt(result.NewRevision)
			learned = true
			break
		}
	}
	if !learned {
		t.Fatal("法师新角没有收到首本火球术")
	}
	// click 封装“click”对应的后续处理，m 记录实体状态，life 记录生命代次，使用数值表示。
	click := func(m *pb.EntityState, life uint64) uint32 {
		c.input++
		return c.send(p.MsgC2SAutoTargetIntent, &pb.AutoTargetIntent{
			SceneEpoch: c.entry.SceneEpoch, InputSequence: c.input, TargetHandle: m.EntityHandle, TargetLife: life,
		})
	}
	// far 按目标类型读取当前接口值，得到“far”。
	far := proto.Clone(c.entity(10)).(*pb.EntityState)
	click(far, far.Life)
	c.auto(true, 2)
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.entity(11) != nil })
	// near 按目标类型读取当前接口值，得到邻近。
	near := proto.Clone(c.entity(11)).(*pb.EntityState)
	// rejected 保存由“click”取得的拒绝。
	rejected := click(near, near.Life+1)
	travelExpectError(c, p.MsgC2SAutoTargetIntent, rejected, p.CodeSkillTarget)
	click(near, near.Life)

	// start 按目标类型读取当前接口值，得到起点或开始时间。
	start := proto.Clone(c.player()).(*pb.EntityState)
	// wait 保存由新增取得的等待。
	wait := time.Now().Add(1200 * time.Millisecond)
	// f 记录协议帧。
	travelUntilWithin(c, 4*time.Second, func(f wire.Frame) bool {
		if f.ID == p.MsgS2CCastAccepted || f.ID == p.MsgS2CAttackAccepted {
			t.Fatal("锁定的远怪不可达时转打法术或贴脸普攻")
		}
		return f.ID == p.MsgS2CSceneBaseline && time.Now().After(wait)
	})
	if !proto.Equal(c.player().Position, start.Position) || c.player().MovementRevision != start.MovementRevision {
		t.Fatal("锁定的远怪不可达时角色离开原脚点")
	}
	c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: 1, TargetHandle: near.EntityHandle})
	// manualFrame 保存由地图切换有效截止时间“within”取得的手动输入协议帧，f 记录协议帧。
	manualFrame := travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CCastAccepted })
	// manual 记录手动，使用施法已受理表示。
	var manual pb.CastAccepted
	must(t, proto.Unmarshal(manualFrame.Payload, &manual))
	if manual.InputSequence == 0 || manual.TargetHandle != near.EntityHandle || manual.TargetLife != near.Life {
		t.Fatal("手动火球不能独立攻击另一只活怪", &manual)
	}

	// claim 保存由资产取得的领取。
	claim := c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
	if claim.ErrorCode != 0 {
		t.Fatal("正常领时失败", claim)
	}
	c.inventoryAt(claim.NewRevision)
	// before 保存由角色取得的操作前的状态。
	before, err := store.Role(ctx, roleID)
	must(t, err)
	// scene 保存场景代次，用于当前步骤中的场景。
	scene := c.entry.SceneEpoch
	c.close()
	// billed 保存当前处理所用的“billed”。
	billed := false
	// deadline 保存由新增取得的截止时间。
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
		time.Sleep(100 * time.Millisecond)
		// role 接收角色供后续步骤使用，err 接收角色过程中可能产生的错误，供后续失败分支处理。
		role, err := store.Role(ctx, roleID)
		must(t, err)
		if role.HostingSeconds < before.HostingSeconds {
			billed = true
			break
		}
	}
	if !billed {
		t.Fatal("断线未实际进入托管")
	}
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	if c.entry.SceneEpoch != scene || !proto.Equal(c.player().Position, start.Position) ||
		c.player().AttackSequence != start.AttackSequence || c.player().CastSequence != start.CastSequence+1 {
		t.Fatal("同场景托管失去首靶并攻击了近怪，或健康重连重建现场", c.entry, c.player())
	}
	// goal 保存当前处理所用的目标格。
	goal := &pb.GridPoint{X: int32(m.Width - 4), Y: int32(m.Height - 4)}
	c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: scene, Destination: goal})
	// f 记录协议帧。
	travelUntilWithin(c, 15*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.scene.GetNavigationState().GetStatus() == 2 && proto.Equal(c.player().Position, goal)
	})
	// resumeFrame 保存由地图切换有效截止时间“within”取得的恢复协议帧，f 记录协议帧。
	resumeFrame := travelUntilWithin(c, 4*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CCastAccepted })
	// resumed 记录“resumed”，使用施法已受理表示。
	var resumed pb.CastAccepted
	must(t, proto.Unmarshal(resumeFrame.Payload, &resumed))
	if resumed.InputSequence != 0 || resumed.TargetHandle != far.EntityHandle || resumed.TargetLife != far.Life || !proto.Equal(resumed.Origin, goal) {
		t.Fatal("换点恢复射程后未对原锁定生命自动施法", &resumed)
	}
	t.Log("真实WS确认：远怪点选、另一怪贴身仍等待、原地手动打近怪、托管扣时健康重入，换点后自动打原怪同一生命")
}
