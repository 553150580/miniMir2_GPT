// 覆盖首次注册、建角、入场、自动战斗和离线收益闭环，并提供集成包复用的请求与连接辅助件。

package integration

import (
	"bytes"
	"context"
	"fmt"
	"github.com/coder/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	"io"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

// 这是明确 opt-in 的真实Mongo/HTTP/WS集成测试，仅创建并删除随机专属测试库。
// t 提供断言失败报告、子用例和清理注册。
func TestFirstLoopMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以连接当前平台本任务专属开发副本集")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	// 独立测试库和回环端口承载完整节点装配，结束时先停节点再删除本轮数据。
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress = address(t)
	cfg.GameAddress = address(t)
	cfg.GMAddress = address(t)
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancel(context.Background())
	// inspection 保存由打开取得的检查。
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	// wg 记录“wg”，使用协程汇合计数器表示。
	var wg sync.WaitGroup
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			// e 接收运行批次供后续步骤使用。
			if e := bootstrap.Run(ctx, cfg, mode); e != nil && ctx.Err() == nil {
				t.Errorf("%s节点: %v", mode, e)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，c 传递从调用方派生的上下文，供取消或控制截止时间。
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	// a 接收遍历文本列表时当前的元素。
	for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, a)
	}
	// accountURL 保存根据当前数据与账号地址的计算结果与当前数据计算的账号URL。
	accountURL := "http://" + cfg.AccountAddress + "/rpc"
	// user 保存根据当前数据与标识中的范围计算的“user”。
	user := "test_" + domain.ID()[:10]
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, accountURL, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: user, Password: "test_123456"}, login, 0)
	rpc(t, accountURL, "", p.MsgC2SLocalLogin, &pb.LocalLoginRequest{Account: user, Password: "wrong_1234"}, &pb.LoginResult{}, p.CodeUnauthenticated)
	rpc(t, accountURL, "", p.MsgC2SLocalLogin, &pb.LocalLoginRequest{Account: user, Password: "test_123456"}, login, 0)
	// token 保存账号令牌，用于当前步骤中的令牌。
	token := login.AccountToken
	// create 保存当前处理所用的创建。
	create := &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 2, Name: "测试道士" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}
	// role 保存当前处理所用的角色。
	role := &pb.RoleSummary{}
	rpc(t, accountURL, token, p.MsgC2SCreateRole, create, role, 0)
	// 原请求重放须返回同一角色，同一操作编号下改名则属于内容冲突。
	// replay 保存当前处理所用的重放。
	replay := &pb.RoleSummary{}
	rpc(t, accountURL, token, p.MsgC2SCreateRole, create, replay, 0)
	if !bytes.Equal(role.Key.RoleId, replay.Key.RoleId) {
		t.Fatal("建角重试创建了第二角色")
	}
	// changed 按目标类型读取当前接口值，得到“changed”。
	changed := proto.Clone(create).(*pb.CreateRoleRequest)
	changed.Name += "变"
	rpc(t, accountURL, token, p.MsgC2SCreateRole, changed, &pb.RoleSummary{}, p.CodeOperationConflict)
	// mage 记录“mage”，使用角色“summary”表示。
	var mage *pb.RoleSummary
	// slot 保存由无符号整数“32”取得的槽位。
	for slot := uint32(2); slot <= 5; slot++ {
		// job 保存由无符号整数“32”取得的任务或职业编号。
		job := uint32(0)
		if slot == 2 {
			job = 1
		}
		// created 保存当前处理所用的“created”。
		created := &pb.RoleSummary{}
		rpc(t, accountURL, token, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: job, Name: fmt.Sprintf("测试%d%s", slot, domain.ID()[:5]), OpId: domain.Bytes(domain.ID())}, created, 0)
		if slot == 2 {
			mage = created
		}
	}
	rpc(t, accountURL, token, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 6, Job: 0, Name: "第六槽", OpId: domain.Bytes(domain.ID())}, &pb.RoleSummary{}, p.CodeInvalidArgument)
	// directory 保存当前处理所用的目录。
	directory := &pb.RoleDirectoryResult{}
	rpc(t, accountURL, token, p.MsgC2SRoleDirectory, &pb.RoleDirectoryRequest{OriginServerId: 1}, directory, 0)
	if len(directory.Roles) != 5 {
		t.Fatalf("角色数=%d", len(directory.Roles))
	}
	// 本作取消性别穿戴门槛；来源男装命名不阻止女法师穿同ID193。
	// female 保存由加入取得的“female”。
	female := join(t, cfg.GameAddress, issue(t, accountURL, token, mage.Key))
	defer female.close()
	female.auto(false, 0)
	if female.player().Mp != 17 || female.player().MpMax != 17 {
		t.Fatal("女法师来源MP未真实同步")
	}
	// cloth 记录“cloth”，使用道具观察视图表示。
	var cloth *pb.ItemView
	// i 接收遍历道具集合时当前的元素。
	for _, i := range female.inventory.Items {
		if i.ItemId == 193 {
			cloth = i
		}
	}
	if cloth == nil {
		t.Fatal("缺少布衣开发样例")
	}
	// equippedCloth 保存由资产取得的已穿戴装备“cloth”。
	equippedCloth := female.asset(p.MsgC2SEquip, &pb.EquipRequest{Operation: female.operation(), ItemInstanceId: cloth.InstanceId, Slot: 1, ItemVersion: cloth.Version})
	if equippedCloth.ErrorCode != 0 {
		t.Fatalf("非法引入性别穿戴门槛:%v", equippedCloth)
	}
	female.close()
	// 把测试专属角色置于19次无装备的合法前态，随后真正击杀验证第20次保底。
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{"$set": bson.M{"no_equipment_kills": 19}})
	must(t, err)
	// 网络在提交前关闭不能留下假在线/永久Pending；新票重入仍能完成ACK。
	// abandoned 保存由发起连接取得的“abandoned”。
	abandoned, _, err := websocket.Dial(ctx, "ws://"+cfg.GameAddress+"/ws", nil)
	must(t, err)
	// abandonedTicket 保存由签发取得的“abandoned”入场票据。
	abandonedTicket := issue(t, accountURL, token, role.Key)
	must(t, abandoned.Write(ctx, websocket.MessageBinary, wire.Encode(wire.Message(p.MsgC2SEnterLine, 1, 0, &pb.EnterLineRequest{AccountTicket: abandonedTicket}))))
	abandoned.CloseNow()
	// ticket 保存由签发取得的入场票据。
	ticket := issue(t, accountURL, token, role.Key)
	// c 接收加入供后续步骤使用。
	c := join(t, cfg.GameAddress, ticket)
	// entity 接收遍历实体时当前的元素。
	for _, entity := range c.scene.Entities {
		// expected 保存从按数值索引的数值映射取出的预期结果。
		expected := map[uint32]uint32{1: 600, 2: 1000, 3: 1500}[entity.EntityKind]
		if entity.MoveIntervalMs != expected {
			t.Fatalf("真实WS移动间隔kind%d=%d want%d", entity.EntityKind, entity.MoveIntervalMs, expected)
		}
	}
	defer c.close()
	if c.hosting == nil || !c.hosting.AutoBattleEnabled {
		t.Fatal("首入未同步真实自动战斗状态")
	}
	c.auto(false, 1)
	if c.hosting.RemainHostingSec != 0 || c.hosting.StateRevision != c.inventory.Revision {
		t.Fatal("自动战斗响应未包含当前余额/版本")
	}
	c.auto(true, 0)
	c.auto(false, 0)
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	// 单次票据重放绝不能建立第二个有效绑定。
	// bad 保存由发起连接取得的“bad”。
	bad, _, err := websocket.Dial(context.Background(), "ws://"+cfg.GameAddress+"/ws", nil)
	must(t, err)
	must(t, bad.Write(context.Background(), websocket.MessageBinary, wire.Encode(wire.Message(p.MsgC2SEnterLine, 1, 0, &pb.EnterLineRequest{AccountTicket: ticket}))))
	// rejectCtx 传递从调用方派生的上下文，供取消或控制截止时间，rejectCancel 传递从调用方派生的上下文，供取消或控制截止时间。
	rejectCtx, rejectCancel := context.WithTimeout(context.Background(), 3*time.Second)
	// raw 保存由读取取得的原始值，e 接收读取供后续步骤使用。
	_, raw, e := bad.Read(rejectCtx)
	rejectCancel()
	bad.CloseNow()
	if e == nil {
		// f 接收解码供后续步骤使用，de 保存由解码取得的“de”。
		f, de := wire.Decode(raw)
		must(t, de)
		if f.Error == 0 {
			t.Fatal("已消费票据重放成功")
		}
	}
	// 背包装备与分解都通过同一权威资产事务，重复op不能重复付出或领奖。
	// sword 记录“sword”，使用道具观察视图表示，spare 记录“spare”，使用道具观察视图表示。
	var sword, spare *pb.ItemView
	// i 接收遍历道具集合时当前的元素。
	for _, i := range c.inventory.Items {
		if i.ItemId == 218 {
			if sword == nil {
				sword = i
			} else {
				spare = i
			}
		}
	}
	if sword == nil || spare == nil {
		t.Fatal("缺少来源明确的开发木剑样例")
	}
	// equip 保存当前处理所用的穿戴装备。
	equip := &pb.EquipRequest{Operation: c.operation(), ItemInstanceId: sword.InstanceId, Slot: 0, ItemVersion: sword.Version}
	// res 保存由资产取得的结果。
	res := c.asset(p.MsgC2SEquip, equip)
	if res.ErrorCode != 0 {
		t.Fatalf("装备失败: %v", res)
	}
	c.inventoryAt(res.NewRevision)
	if len(c.inventory.Equipped) != 1 {
		t.Fatal("装备没有原子进入装备栏")
	}
	// replayRes 保存由资产取得的重放结果。
	replayRes := c.asset(p.MsgC2SEquip, equip)
	if replayRes.ErrorCode != 0 || replayRes.CommandSeq != res.CommandSeq {
		t.Fatalf("装备重放不一致: %v", replayRes)
	}
	// changedEquip 按目标类型读取当前接口值，得到“changed”穿戴装备。
	changedEquip := proto.Clone(equip).(*pb.EquipRequest)
	changedEquip.ItemInstanceId = spare.InstanceId
	// r 接收资产供后续步骤使用。
	if r := c.asset(p.MsgC2SEquip, changedEquip); r.ErrorCode != p.CodeOperationConflict {
		t.Fatalf("同op改变参数未拒绝: %v", r)
	}
	// recycle 保存当前处理所用的回收。
	recycle := &pb.RecycleRequest{Operation: c.operation(), ItemInstanceIds: [][]byte{spare.InstanceId}}
	res = c.asset(p.MsgC2SRecycle, recycle)
	if res.ErrorCode != 0 {
		t.Fatalf("分解失败: %v", res)
	}
	c.inventoryAt(res.NewRevision)
	// persisted 保存由角色取得的“persisted”。
	persisted, err := inspection.Role(ctx, domain.Hex(role.Key.RoleId))
	must(t, err)
	// gold 保存绑定金币，用于当前步骤中的金币。
	gold := persisted.Wallet.BoundGold
	if gold != 50 {
		t.Fatalf("候选木剑开发回收金币=%d，预期50", gold)
	}
	// r 接收资产供后续步骤使用。
	if r := c.asset(p.MsgC2SRecycle, recycle); r.ErrorCode != 0 {
		t.Fatalf("分解重放失败: %v", r)
	}
	persisted, err = inspection.Role(ctx, persisted.ID)
	must(t, err)
	if persisted.Wallet.BoundGold != gold {
		t.Fatal("分解重放重复给金币")
	}
	// claim 保存当前处理所用的领取。
	claim := &pb.ClaimHostingTimeRequest{Operation: c.operation(), BenefitKind: 0}
	res = c.asset(p.MsgC2SClaimHostingTime, claim)
	if res.ErrorCode != 0 {
		t.Fatalf("免费领时失败: %v", res)
	}
	c.inventoryAt(res.NewRevision)
	persisted, err = inspection.Role(ctx, persisted.ID)
	must(t, err)
	if persisted.HostingSeconds != 21600 {
		t.Fatalf("免费时长=%d", persisted.HostingSeconds)
	}
	// 旧回执返回当前确认watermark，不让客户端next倒退。
	// old 保存由资产取得的原状态。
	if old := c.asset(p.MsgC2SEquip, equip); old.CommandSeq != res.CommandSeq {
		t.Fatal("旧op回放使economic watermark倒退")
	}
	c.inventoryAt(res.NewRevision)
	// 在线不扣离线时长，点地后松摇杆不取消寻路；无重连生命/位置重置。
	// itemsBeforeCombat 初始化用作道具集合操作前的状态战斗的按文本索引的布尔状态映射。
	itemsBeforeCombat := map[string]bool{}
	// item 接收遍历道具集合时当前的元素。
	for _, item := range persisted.Items {
		itemsBeforeCombat[item.ID] = true
	}
	// destination 保存当前处理所用的目的地。
	destination := &pb.GridPoint{X: 6, Y: 8}
	c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: destination})
	c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, ReleaseJoystick: true})
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.player() != nil && proto.Equal(c.player().Position, destination)
	})
	// before 按目标类型读取当前接口值，得到操作前的状态。
	before := proto.Clone(c.player()).(*pb.EntityState)
	time.Sleep(1100 * time.Millisecond)
	persisted, err = inspection.Role(ctx, persisted.ID)
	must(t, err)
	if persisted.HostingSeconds != 21600 {
		t.Fatal("在线扣了托管时长")
	}
	// sceneEpoch 保存当前处理所用的场景代次。
	sceneEpoch := c.entry.SceneEpoch
	c.close()
	time.Sleep(1100 * time.Millisecond)
	// paused 保存由角色取得的“paused”。
	paused, err := inspection.Role(ctx, persisted.ID)
	must(t, err)
	if paused.HostingSeconds != 21600 {
		t.Fatal("关闭自动战斗后仍扣托管时长")
	}
	c = join(t, cfg.GameAddress, issue(t, accountURL, token, role.Key))
	defer c.close()
	if c.entry.SceneEpoch != sceneEpoch || c.player() == nil || c.player().Position.X != before.Position.X || c.player().Position.Y != before.Position.Y || c.player().Hp > before.Hp {
		t.Fatalf("健康重连重置现场 before=%v after=%v", before, c.player())
	}
	if c.hosting == nil || c.hosting.AutoBattleEnabled || c.hosting.RemainHostingSec != 21600 {
		t.Fatal("健康重连未还原关闭状态和当前余额")
	}
	// initialSource 保存持久收益来源序号，用于当前步骤中的初始值来源。
	initialSource := paused.SourceSeq
	c.auto(true, 1)
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	c.close()
	// 托管整秒可随击杀先提交。没有击杀时需等三秒检查点及数据库确认。
	for hostingDeadline := time.Now().Add(6 * time.Second); ; {
		persisted, err = inspection.Role(ctx, persisted.ID)
		must(t, err)
		if persisted.HostingSeconds < 21600 && persisted.Report.UsedSeconds >= 1 {
			break
		}
		if !time.Now().Before(hostingDeadline) {
			t.Fatalf("离线托管未实际扣时: %+v", persisted.Report)
		}
		time.Sleep(100 * time.Millisecond)
	}
	c = join(t, cfg.GameAddress, issue(t, accountURL, token, role.Key))
	defer c.close()
	if c.entry.SceneEpoch != sceneEpoch {
		t.Fatal("托管期间重新创建了临时世界")
	}

	if c.hosting == nil || !c.hosting.AutoBattleEnabled || c.hosting.RemainHostingSec >= 21600 {
		t.Fatal("离线托管重连未还原开启状态和扣时余额")
	}
	if c.report == nil || c.report.UsedSeconds < 1 {
		t.Fatal("未同步永久托管战报")
	}
	// 自动战斗实际杀怪，经验与物品来自逐击杀结算；八槽可调且道士具有宠物。
	// hasPet 保存当前处理所用的“has”宠物。
	hasPet := false
	// e 接收遍历实体时当前的元素。
	for _, e := range c.scene.Entities {
		if e.EntityKind == 2 && e.DefinitionId == 2 {
			hasPet = true
		}
	}
	if !hasPet {
		t.Fatal("道士缺少来源宠物")
	}
	c.auto(true, 1)
	// deadline 保存由新增取得的截止时间。
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		c.read()
		persisted, err = inspection.Role(ctx, persisted.ID)
		must(t, err)
		if persisted.Level > 1 {
			break
		}
	}
	// newEquipment 规定创建装备的初始取值。
	newEquipment := 0
	// item 接收遍历道具集合时当前的元素。
	for _, item := range persisted.Items {
		// definition 保存由道具取得的定义，exists 保存由道具取得的存在。
		definition, exists := cfg.Fixture.Item(item.ItemID)
		if !itemsBeforeCombat[item.ID] && exists && definition.IsEquipment {
			newEquipment++
		}
	}
	if persisted.NoEquipmentKills != 0 || len(persisted.Items) != len(itemsBeforeCombat)+1 || newEquipment != 1 {
		t.Fatalf("实际第20次保底未入背包: counter=%d items=%d", persisted.NoEquipmentKills, len(persisted.Items))
	}
	if persisted.Level <= 1 || persisted.SourceSeq <= initialSource {
		t.Fatal("真实战斗没有逐事件经验结算")
	}
	c.auto(false, 8)
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool {
		// count 规定数量的初始取值。
		count := 0
		// e 接收遍历“get”实体时当前的元素。
		for _, e := range c.scene.GetEntities() {
			if e.EntityKind == 3 {
				count++
			}
		}
		return count == 8
	})
	// observedAttack 保存当前处理所用的观察到的攻击。
	observedAttack := false
	// entity 接收遍历实体时当前的元素。
	for _, entity := range c.scene.Entities {
		if entity.AttackSequence > 0 {
			observedAttack = true
		}
	}
	if !observedAttack {
		t.Fatal("真实战斗基线没有回传独立攻击序号")
	}
	// 八怪实际击杀玩家，开发复活必须拿原death_id走资产原op提交，健康重连没有回血捷径。
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CDeathNotice })
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.player().Hp != 0 || c.player().Action != 4 {
		t.Fatal("死亡notice后没有即时HP0/Action4终态基线")
	}
	if c.death.AllowedActions != 1 {
		t.Fatalf("开发复活许可=%d", c.death.AllowedActions)
	}
	// deathID 标识当前业务使用的死亡。
	deathID := append([]byte(nil), c.death.DeathId...)
	// 玩家已经死亡，周期step不会广播；数量变更必须由命令立即推基线。
	// count 接收遍历数值列表时当前的元素。
	for _, count := range []uint32{4, 8} {
		c.auto(false, count)
		// f 记录协议帧。
		c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
		// monsters 规定怪物集合的初始取值。
		monsters := 0
		// e 接收遍历实体时当前的元素。
		for _, e := range c.scene.Entities {
			if e.EntityKind == 3 {
				monsters++
			}
		}
		if monsters != int(count) || c.player().Hp != 0 || !bytes.Equal(c.death.DeathId, deathID) {
			t.Fatalf("死亡切怪未即时同步或重置了现场: count=%d player=%v", monsters, c.player())
		}
	}
	c.close()
	c = join(t, cfg.GameAddress, issue(t, accountURL, token, role.Key))
	defer c.close()
	if c.player().Hp != 0 || c.death == nil || !bytes.Equal(c.death.DeathId, deathID) {
		t.Fatal("死亡重连假回血或丢失原death_id")
	}
	// revive 保存当前处理所用的复活。
	revive := &pb.ReviveChoice{DeathId: c.death.DeathId, Action: 1, Operation: c.operation()}
	// deadLife 保存生命代次，用于当前步骤中的死亡状态生命代次，deadModeRevision 保存修订号，用于当前步骤中的死亡状态模式修订号。
	deadLife, deadModeRevision := c.player().Life, c.scene.SkillModeState.Revision
	// revived 保存由资产取得的“revived”。
	revived := c.asset(p.MsgC2SReviveChoice, revive)
	if revived.ErrorCode != 0 {
		t.Fatalf("开发主动复活失败:%v", revived)
	}
	c.inventoryAt(revived.NewRevision)
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().Hp == c.player().HpMax })
	if c.player().Life != deadLife+1 || c.scene.SkillModeState.PlayerLife != c.player().Life || c.scene.SkillModeState.Revision <= deadModeRevision {
		t.Fatal("同连接复活未同步新生命代次和严格递增的模式版本")
	}
	// modeRevision 保存修订号，用于当前步骤中的模式修订号。
	modeRevision := c.scene.SkillModeState.Revision
	// r 接收资产供后续步骤使用。
	if r := c.asset(p.MsgC2SReviveChoice, revive); r.ErrorCode != 0 {
		t.Fatalf("复活原op重放失败:%v", r)
	}
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.player().Life != deadLife+1 || c.scene.SkillModeState.Revision != modeRevision {
		t.Fatal("复活原op重放额外推进生命/模式版本")
	}
	// 数据库不得含临时战斗现场。
	// document 记录文档，使用MongoDB BSON 文档表示。
	var document bson.M
	must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": persisted.ID}).Decode(&document))
	// key 接收遍历文本列表时当前的元素。
	for _, key := range []string{"hp", "mp", "position", "monsters", "cooldowns", "pet", "ground_items"} {
		// ok 表明映射中能否取得指定键。
		if _, ok := document[key]; ok {
			t.Fatalf("临时字段%s被持久化", key)
		}
	}
	t.Logf("通过: DB=%s；5槽、单票、ACK、移动、资产幂等、21600秒日领、离线用时%d秒、真实升级到%d级、8怪切换、健康重连", cfg.Database, c.report.UsedSeconds, persisted.Level)
}

// must 遇到不应出现的辅助步骤错误时立即终止该测试。
// t 提供断言失败报告、子用例和清理注册，e 记录错误。
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}

// address 获取回环系统分配的端口，降低并行测试间的监听冲突。
// t 提供断言失败报告、子用例和清理注册。
func address(t *testing.T) string {
	t.Helper()
	// l 保存由监听取得的l，e 接收监听供后续步骤使用。
	l, e := net.Listen("tcp", "127.0.0.1:0")
	must(t, e)
	// a 保存由文本取得的a。
	a := l.Addr().String()
	l.Close()
	return a
}

// healthy 等待指定测试节点完成启动并可以响应健康请求。
// t 提供断言失败报告、子用例和清理注册，a 记录a，使用文本表示。
func healthy(t *testing.T, a string) {
	t.Helper()
	// deadline 保存由新增取得的截止时间。
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		// r 接收“get”供后续步骤使用，e 接收“get”供后续步骤使用。
		r, e := http.Get("http://" + a + "/healthz")
		if e == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("节点未就绪: %s", a)
}

// rpc 通过真实二进制 HTTP 协议调用账号服，检查返回码后解码响应。
// t 提供断言失败报告、子用例和清理注册，url 记录URL，使用文本表示，token 记录令牌，使用文本表示，id 区分当前步骤指向的目标对象，in 记录输入，使用消息表示。
// out 记录输出，使用消息表示，code 记录结果码，使用数值表示。
func rpc(t *testing.T, url, token string, id uint16, in, out proto.Message, code uint32) {
	t.Helper()
	// req 保存由创建请求取得的请求，e 接收创建请求供后续步骤使用。
	req, e := http.NewRequest("POST", url, bytes.NewReader(wire.Encode(wire.Message(id, 1, 0, in))))
	must(t, e)
	req.Header.Set("Authorization", "Bearer "+token)
	// res 保存由“do”取得的结果。
	res, e := http.DefaultClient.Do(req)
	must(t, e)
	defer res.Body.Close()
	// b 保存由读取全部取得的b。
	b, e := io.ReadAll(res.Body)
	must(t, e)
	// f 接收解码供后续步骤使用。
	f, e := wire.Decode(b)
	must(t, e)
	if f.Error != code {
		t.Fatalf("RPC%d error=%d want%d", id, f.Error, code)
	}
	if f.Error == 0 {
		must(t, proto.Unmarshal(f.Payload, out))
	}
}

// issue 用当前账号令牌为测试角色申请一次性入场票据。
// t 提供断言失败报告、子用例和清理注册，url 记录URL，使用文本表示，token 记录令牌，使用文本表示，key 记录键，使用角色键表示。
func issue(t *testing.T, url, token string, key *pb.RoleKey) string {
	t.Helper()
	// q 保存当前处理所用的入场入场票据结果。
	q := &pb.EntryTicketResult{}
	rpc(t, url, token, p.MsgC2SEntryTicket, &pb.EntryTicketRequest{Role: key}, q, 0)
	return q.Ticket
}

// client 记录集成测试连接的请求序列、场景和资产观察状态。
type client struct {
	t              *testing.T                // 所属测试实例，供辅助方法报告失败。
	ws             *websocket.Conn           // 与游戏服建立的真实连接。
	ctx            context.Context           // 该连接所有读写共享的测试截止。
	stop           context.CancelFunc        // 结束读写等待和释放上下文的函数。
	binding        uint64                    // 最近成功消息确认的角色会话代次。
	request        uint32                    // 下次发送前递增的应用层请求序号。
	input          uint32                    // 需要自动填充时使用的场景输入水位。
	entry          *pb.EnterLineResult       // 最近入场响应及其基线、地图和规则版本。
	inventory      *pb.InventoryResult       // 最近解析的库存与角色版本快照。
	scene          *pb.SceneBaselineChunk    // 最近观察到的场景基线分片。
	report         *pb.HostingReport         // 最近收到的托管战报。
	hosting        *pb.HostingState          // 最近收到的托管余额与自动状态。
	watermark      uint64                    // 已确认的经济操作序号。
	death          *pb.DeathNotice           // 最近一次角色死亡通知。
	roleBinding    *pb.BindingState          // 最近解析的复活绑定状态。
	bindingCommits []*pb.BindingState        // 实际提交绑定时保留的独立事件快照。
	onFrame        func(*client, wire.Frame) // 测试监听全部应用帧的可选观察钩子。
}

// join 通过一次性票据完成测试客户端 WebSocket 入场。
// t 提供断言失败报告、子用例和清理注册，address 记录地址，使用文本表示，ticket 记录入场票据，使用文本表示。
// observers 记录“observers”，使用可变数量的供当前流程调用的函数表示。
func join(t *testing.T, address, ticket string, observers ...func(*client, wire.Frame)) *client {
	t.Helper()
	return joinWithAutoSkills(t, address, ticket, nil, observers...)
}

// joinWithAutoSkills 完成入场基线和确认，可在同步时附带指定自动技能设置。
// t 提供断言失败报告、子用例和清理注册，address 记录地址，使用文本表示，ticket 记录入场票据，使用文本表示。
// settings 记录“settings”，使用自动技能“settings”表示，observers 记录“observers”，使用可变数量的供当前流程调用的函数表示。
func joinWithAutoSkills(t *testing.T, address, ticket string, settings *pb.AutoSkillSettings, observers ...func(*client, wire.Frame)) *client {
	t.Helper()
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
	ctx, stop := context.WithTimeout(context.Background(), 80*time.Second)
	// ws 保存由发起连接取得的WebSocket，err 接收发起连接过程中可能产生的错误，供后续失败分支处理。
	ws, _, err := websocket.Dial(ctx, "ws://"+address+"/ws", nil)
	must(t, err)
	// c 保存当前处理所用的客户端。
	c := &client{t: t, ws: ws, ctx: ctx, stop: stop}
	if len(observers) > 0 {
		// c 记录客户端，f 记录协议帧。
		c.onFrame = func(c *client, f wire.Frame) {
			// observe 接收遍历“observers”时当前的元素。
			for _, observe := range observers {
				if observe != nil {
					observe(c, f)
				}
			}
		}
	}
	c.send(p.MsgC2SEnterLine, &pb.EnterLineRequest{AccountTicket: ticket})
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CEnterLine })
	// 必须先收到完整首基线，客户端才能完成场景加载后ACK；不能用假ACK掩盖握手死锁。
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.scene.SceneEpoch != c.entry.SceneEpoch || c.scene.BaselineId != c.entry.BaselineId || c.scene.ChunkCount != 1 || len(c.scene.Entities) == 0 {
		t.Fatal("ACK前未收到匹配的完整首基线")
	}
	if c.inventory != nil || c.report != nil {
		t.Fatal("ACK前提前发送资产或战报")
	}
	// 不匹配ACK不能激活；ACK前动作仍被拒绝。
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: c.entry.SceneEpoch, BaselineId: c.entry.BaselineId + 1})
	// rejected 保存由发送取得的拒绝。
	rejected := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 2})
	// raw 保存由读取取得的原始值。
	_, raw, err := c.ws.Read(c.ctx)
	must(t, err)
	// f 接收解码供后续步骤使用。
	f, err := wire.Decode(raw)
	must(t, err)
	if c.onFrame != nil {
		c.onFrame(c, f)
	}
	if f.Request != rejected || f.ID != p.MsgC2SMoveIntent || f.Error != p.CodeLineRecovering {
		t.Fatalf("ACK前动作没有被门槛阻挡:%v", f)
	}
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: c.entry.SceneEpoch, BaselineId: c.entry.BaselineId, AutoSkillSettings: settings})
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSyncReady })
	// 用同一actor队列内的无效方向作为屏障，验证重复ACK没有重发激活或战报。
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: c.entry.SceneEpoch, BaselineId: c.entry.BaselineId})
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: c.entry.SceneEpoch, BaselineId: c.entry.BaselineId})
	// barrier 保存由发送取得的屏障。
	barrier := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 8})
	for {
		_, raw, err = c.ws.Read(c.ctx)
		must(t, err)
		f, err = wire.Decode(raw)
		must(t, err)
		if c.onFrame != nil {
			c.onFrame(c, f)
		}
		if f.ID == p.MsgS2CSyncReady || f.ID == p.MsgS2CHostingReport {
			t.Fatal("重复ACK再次发送激活或战报")
		}
		if f.Error != 0 {
			if f.ID != p.MsgC2SMoveIntent || f.Request != barrier || f.Error != p.CodeInvalidArgument {
				t.Fatalf("actor屏障异常:%v", f)
			}
			break
		}
	}
	return c
}

// close 关闭本测试连接并取消读写使用的上下文。
// c 是调用该方法的客户端实例。
func (c *client) close() { c.ws.CloseNow(); c.stop() }

// send 用递增请求编号和连接会话身份向游戏服发送协议消息。
// c 是调用该方法的客户端实例，id 区分当前步骤指向的目标对象，q 记录消息。
func (c *client) send(id uint16, q proto.Message) uint32 {
	// message 按目标类型读取当前接口值，得到消息。
	switch message := q.(type) {
	case *pb.MoveIntent:
		if message.InputSequence == 0 {
			c.input++
			message.InputSequence = c.input
		}
	case *pb.CastIntent:
		if message.InputSequence == 0 {
			c.input++
			message.InputSequence = c.input
		}
	case *pb.CastWithMaterialRequest:
		if message.InputSequence == 0 {
			c.input++
			message.InputSequence = c.input
		}
	case *pb.AttackIntent:
		if message.InputSequence == 0 {
			c.input++
			message.InputSequence = c.input
		}
	case *pb.SkillModeIntent:
		if message.InputSequence == 0 {
			c.input++
			message.InputSequence = c.input
		}
	}
	c.request++
	must(c.t, c.ws.Write(c.ctx, websocket.MessageBinary, wire.Encode(wire.Message(id, c.request, c.binding, q))))
	return c.request
}

// read 从连接读取下一应用帧，并按测试约定检查错误码。
// c 是调用该方法的客户端实例。
func (c *client) read() wire.Frame {
	// f 接收读取协议帧供后续步骤使用。
	f := c.readFrame()
	if f.Error != 0 {
		c.t.Fatalf("WS %d error%d；最近玩家状态=%v，已收到死亡通知=%t", f.ID, f.Error, c.player(), c.death != nil)
	}
	return f
}

// readFrame 解码下一帧的协议数据并将支持的消息汇入客户端观察状态。
// c 是调用该方法的客户端实例。
func (c *client) readFrame() wire.Frame {
	// b 保存由读取取得的b，e 接收读取供后续步骤使用。
	_, b, e := c.ws.Read(c.ctx)
	must(c.t, e)
	// f 接收解码供后续步骤使用。
	f, e := wire.Decode(b)
	must(c.t, e)
	defer func() {
		if c.onFrame != nil {
			c.onFrame(c, f)
		}
	}()
	if f.Error != 0 {
		return f
	}
	c.binding = f.Binding
	switch f.ID {
	case p.MsgS2CHostingState:
		c.hosting = &pb.HostingState{}
		must(c.t, proto.Unmarshal(f.Payload, c.hosting))
	case p.MsgS2CDeathNotice:
		c.death = &pb.DeathNotice{}
		must(c.t, proto.Unmarshal(f.Payload, c.death))
	case p.MsgS2CEnterLine:
		c.entry = &pb.EnterLineResult{}
		must(c.t, proto.Unmarshal(f.Payload, c.entry))
		c.watermark = c.entry.NextEconomicCommandSeq - 1
	case p.MsgS2CInventory:
		c.inventory = &pb.InventoryResult{}
		must(c.t, proto.Unmarshal(f.Payload, c.inventory))
	case p.MsgS2CBindingState:
		c.roleBinding = &pb.BindingState{}
		must(c.t, proto.Unmarshal(f.Payload, c.roleBinding))
		if c.roleBinding.Cause == 2 {
			if c.inventory == nil || c.inventory.Revision < c.roleBinding.RoleRevision {
				c.t.Fatal("绑定提交观察先于对应完整资产快照")
			}
			c.bindingCommits = append(c.bindingCommits, proto.Clone(c.roleBinding).(*pb.BindingState))
		}
	case p.MsgS2CSceneBaseline:
		c.scene = &pb.SceneBaselineChunk{}
		must(c.t, proto.Unmarshal(f.Payload, c.scene))
	case p.MsgS2CHostingReport:
		c.report = &pb.HostingReport{}
		must(c.t, proto.Unmarshal(f.Payload, c.report))
	}
	return f
}

// until 消费连接消息直到遇到满足断言条件的帧。
// c 是调用该方法的客户端实例，predicate 记录“predicate”，使用供当前流程调用的函数表示。
func (c *client) until(predicate func(wire.Frame) bool) wire.Frame {
	c.t.Helper()
	for {
		// f 接收读取供后续步骤使用。
		f := c.read()
		if predicate(f) {
			return f
		}
	}
}

// player 在最近缓存的基线分片里查找玩家类型实体。
// c 是调用该方法的客户端实例。
func (c *client) player() *pb.EntityState {
	// e 接收遍历“get”实体时当前的元素。
	for _, e := range c.scene.GetEntities() {
		if e.EntityKind == 1 {
			return e
		}
	}
	return nil
}

// operation 以最新角色版本、规则和经济序号生成新的操作上下文。
// c 是调用该方法的客户端实例。
func (c *client) operation() *pb.OperationContext {
	return &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: c.watermark + 1, ExpectedRevision: c.inventory.Revision, ExpectedRuleVersion: c.entry.ResourceVersion}
}

// asset 提交指定资产请求并等待匹配该请求编号的操作结果。
// c 是调用该方法的客户端实例，id 区分当前步骤指向的目标对象，q 记录消息。
func (c *client) asset(id uint16, q proto.Message) *pb.OperationResult {
	// req 保存由发送取得的请求。
	req := c.send(id, q)
	// f 接收有效截止时间供后续步骤使用，f 记录协议帧。
	f := c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2COperationResult && f.Request == req })
	// r 保存当前处理所用的操作结果。
	r := &pb.OperationResult{}
	must(c.t, proto.Unmarshal(f.Payload, r))
	if r.CommandSeq > c.watermark {
		c.watermark = r.CommandSeq
	}
	return r
}

// inventoryAt 等待库存消息推进到指定资产版本。
// c 是调用该方法的客户端实例，rev 记录修订号，使用数值表示。
func (c *client) inventoryAt(rev uint64) {
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CInventory && c.inventory.Revision >= rev })
}

// auto 通过网络切换自动战斗及怪物数，等待同一请求的状态确认。
// c 是调用该方法的客户端实例，enabled 标记是否处于已开启状态，count 记录数量，使用数值表示。
func (c *client) auto(enabled bool, count uint32) {
	// request 保存由发送取得的请求。
	request := c.send(p.MsgC2SAutoBattle, &pb.AutoBattleIntent{SceneEpoch: c.entry.SceneEpoch, Enabled: enabled, MonsterCount: count})
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CHostingState && f.Request == request })
	if c.hosting == nil || c.hosting.AutoBattleEnabled != enabled {
		c.t.Fatalf("自动战斗响应没有返回真实状态: enabled=%v state=%v", enabled, c.hosting)
	}
}
