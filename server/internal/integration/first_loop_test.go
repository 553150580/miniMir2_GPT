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
	"minimir/internal/devconfig"
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
func TestFirstLoopMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以连接当前平台本任务专属开发副本集")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress = address(t)
	cfg.GameAddress = address(t)
	cfg.GMAddress = address(t)
	ctx, cancel := context.WithCancel(context.Background())
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	var wg sync.WaitGroup
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			if e := bootstrap.Run(ctx, cfg, mode); e != nil && ctx.Err() == nil {
				t.Errorf("%s节点: %v", mode, e)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, a)
	}
	accountURL := "http://" + cfg.AccountAddress + "/rpc"
	user := "test_" + domain.ID()[:10]
	login := &pb.LoginResult{}
	rpc(t, accountURL, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: user, Password: "test_123456"}, login, 0)
	rpc(t, accountURL, "", p.MsgC2SLocalLogin, &pb.LocalLoginRequest{Account: user, Password: "wrong_1234"}, &pb.LoginResult{}, p.CodeUnauthenticated)
	rpc(t, accountURL, "", p.MsgC2SLocalLogin, &pb.LocalLoginRequest{Account: user, Password: "test_123456"}, login, 0)
	token := login.AccountToken
	create := &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 2, Name: "测试道士" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}
	role := &pb.RoleSummary{}
	rpc(t, accountURL, token, p.MsgC2SCreateRole, create, role, 0)
	replay := &pb.RoleSummary{}
	rpc(t, accountURL, token, p.MsgC2SCreateRole, create, replay, 0)
	if !bytes.Equal(role.Key.RoleId, replay.Key.RoleId) {
		t.Fatal("建角重试创建了第二角色")
	}
	changed := proto.Clone(create).(*pb.CreateRoleRequest)
	changed.Name += "变"
	rpc(t, accountURL, token, p.MsgC2SCreateRole, changed, &pb.RoleSummary{}, p.CodeOperationConflict)
	var mage *pb.RoleSummary
	for slot := uint32(2); slot <= 5; slot++ {
		job := uint32(0)
		if slot == 2 {
			job = 1
		}
		created := &pb.RoleSummary{}
		rpc(t, accountURL, token, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: job, Name: fmt.Sprintf("测试%d%s", slot, domain.ID()[:5]), OpId: domain.Bytes(domain.ID())}, created, 0)
		if slot == 2 {
			mage = created
		}
	}
	rpc(t, accountURL, token, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 6, Job: 0, Name: "第六槽", OpId: domain.Bytes(domain.ID())}, &pb.RoleSummary{}, p.CodeInvalidArgument)
	directory := &pb.RoleDirectoryResult{}
	rpc(t, accountURL, token, p.MsgC2SRoleDirectory, &pb.RoleDirectoryRequest{OriginServerId: 1}, directory, 0)
	if len(directory.Roles) != 5 {
		t.Fatalf("角色数=%d", len(directory.Roles))
	}
	// 本作取消性别穿戴门槛；来源男装命名不阻止女法师穿同ID193。
	female := join(t, cfg.GameAddress, issue(t, accountURL, token, mage.Key))
	defer female.close()
	female.auto(false, 0)
	if female.player().Mp != 17 || female.player().MpMax != 17 {
		t.Fatal("女法师来源MP未真实同步")
	}
	var cloth *pb.ItemView
	for _, i := range female.inventory.Items {
		if i.ItemId == 193 {
			cloth = i
		}
	}
	if cloth == nil {
		t.Fatal("缺少布衣开发样例")
	}
	equippedCloth := female.asset(p.MsgC2SEquip, &pb.EquipRequest{Operation: female.operation(), ItemInstanceId: cloth.InstanceId, Slot: 1})
	if equippedCloth.ErrorCode != 0 {
		t.Fatalf("非法引入性别穿戴门槛:%v", equippedCloth)
	}
	female.close()
	// 把测试专属角色置于19次无装备的合法前态，随后真正击杀验证第20次保底。
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{"$set": bson.M{"no_equipment_kills": 19}})
	must(t, err)
	// 网络在提交前关闭不能留下假在线/永久Pending；新票重入仍能完成ACK。
	abandoned, _, err := websocket.Dial(ctx, "ws://"+cfg.GameAddress+"/ws", nil)
	must(t, err)
	abandonedTicket := issue(t, accountURL, token, role.Key)
	must(t, abandoned.Write(ctx, websocket.MessageBinary, wire.Encode(wire.Message(p.MsgC2SEnterLine, 1, 0, &pb.EnterLineRequest{AccountTicket: abandonedTicket}))))
	abandoned.CloseNow()
	ticket := issue(t, accountURL, token, role.Key)
	c := join(t, cfg.GameAddress, ticket)
	for _, entity := range c.scene.Entities {
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
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	// 单次票据重放绝不能建立第二个有效绑定。
	bad, _, err := websocket.Dial(context.Background(), "ws://"+cfg.GameAddress+"/ws", nil)
	must(t, err)
	must(t, bad.Write(context.Background(), websocket.MessageBinary, wire.Encode(wire.Message(p.MsgC2SEnterLine, 1, 0, &pb.EnterLineRequest{AccountTicket: ticket}))))
	rejectCtx, rejectCancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, raw, e := bad.Read(rejectCtx)
	rejectCancel()
	bad.CloseNow()
	if e == nil {
		f, de := wire.Decode(raw)
		must(t, de)
		if f.Error == 0 {
			t.Fatal("已消费票据重放成功")
		}
	}
	// 背包装备与分解都通过同一权威资产事务，重复op不能重复付出或领奖。
	var sword, spare *pb.ItemView
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
	equip := &pb.EquipRequest{Operation: c.operation(), ItemInstanceId: sword.InstanceId, Slot: 0}
	res := c.asset(p.MsgC2SEquip, equip)
	if res.ErrorCode != 0 {
		t.Fatalf("装备失败: %v", res)
	}
	c.inventoryAt(res.NewRevision)
	if len(c.inventory.Equipped) != 1 {
		t.Fatal("装备没有原子进入装备栏")
	}
	replayRes := c.asset(p.MsgC2SEquip, equip)
	if replayRes.ErrorCode != 0 || replayRes.CommandSeq != res.CommandSeq {
		t.Fatalf("装备重放不一致: %v", replayRes)
	}
	changedEquip := proto.Clone(equip).(*pb.EquipRequest)
	changedEquip.ItemInstanceId = spare.InstanceId
	if r := c.asset(p.MsgC2SEquip, changedEquip); r.ErrorCode != p.CodeOperationConflict {
		t.Fatalf("同op改变参数未拒绝: %v", r)
	}
	recycle := &pb.RecycleRequest{Operation: c.operation(), ItemInstanceIds: [][]byte{spare.InstanceId}}
	res = c.asset(p.MsgC2SRecycle, recycle)
	if res.ErrorCode != 0 {
		t.Fatalf("分解失败: %v", res)
	}
	c.inventoryAt(res.NewRevision)
	persisted, err := inspection.Role(ctx, domain.Hex(role.Key.RoleId))
	must(t, err)
	gold := persisted.Wallet.BoundGold
	if gold != 50 {
		t.Fatalf("候选木剑开发回收金币=%d，预期50", gold)
	}
	if r := c.asset(p.MsgC2SRecycle, recycle); r.ErrorCode != 0 {
		t.Fatalf("分解重放失败: %v", r)
	}
	persisted, err = inspection.Role(ctx, persisted.ID)
	must(t, err)
	if persisted.Wallet.BoundGold != gold {
		t.Fatal("分解重放重复给金币")
	}
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
	if old := c.asset(p.MsgC2SEquip, equip); old.CommandSeq != res.CommandSeq {
		t.Fatal("旧op回放使economic watermark倒退")
	}
	c.inventoryAt(res.NewRevision)
	// 在线不扣离线时长，点地后松摇杆不取消寻路；无重连生命/位置重置。
	itemsBeforeCombat := map[string]bool{}
	for _, item := range persisted.Items {
		itemsBeforeCombat[item.ID] = true
	}
	c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: &pb.GridPoint{X: 6, Y: 8}})
	c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, ReleaseJoystick: true})
	c.until(func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.player() != nil && c.player().Position.X == 6
	})
	before := proto.Clone(c.player()).(*pb.EntityState)
	time.Sleep(1100 * time.Millisecond)
	persisted, err = inspection.Role(ctx, persisted.ID)
	must(t, err)
	if persisted.HostingSeconds != 21600 {
		t.Fatal("在线扣了托管时长")
	}
	sceneEpoch := c.entry.SceneEpoch
	c.close()
	time.Sleep(1100 * time.Millisecond)
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
	initialSource := paused.SourceSeq
	c.auto(true, 1)
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	c.close()
	time.Sleep(2300 * time.Millisecond)
	persisted, err = inspection.Role(ctx, persisted.ID)
	must(t, err)
	if persisted.HostingSeconds >= 21600 || persisted.Report.UsedSeconds < 1 {
		t.Fatalf("离线托管未实际扣时: %+v", persisted.Report)
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
	hasPet := false
	for _, e := range c.scene.Entities {
		if e.EntityKind == 2 && e.DefinitionId == 2 {
			hasPet = true
		}
	}
	if !hasPet {
		t.Fatal("道士缺少来源宠物")
	}
	c.auto(true, 1)
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		c.read()
		persisted, err = inspection.Role(ctx, persisted.ID)
		must(t, err)
		if persisted.Level > 1 {
			break
		}
	}
	newEquipment := 0
	for _, item := range persisted.Items {
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
	c.until(func(f wire.Frame) bool {
		count := 0
		for _, e := range c.scene.GetEntities() {
			if e.EntityKind == 3 {
				count++
			}
		}
		return count == 8
	})
	observedAttack := false
	for _, entity := range c.scene.Entities {
		if entity.AttackSequence > 0 {
			observedAttack = true
		}
	}
	if !observedAttack {
		t.Fatal("真实战斗基线没有回传独立攻击序号")
	}
	// 八怪实际击杀玩家，开发复活必须拿原death_id走资产原op提交，健康重连没有回血捷径。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CDeathNotice })
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.player().Hp != 0 || c.player().Action != 4 {
		t.Fatal("死亡notice后没有即时HP0/Action4终态基线")
	}
	if c.death.AllowedActions != 1 {
		t.Fatalf("开发复活许可=%d", c.death.AllowedActions)
	}
	deathID := append([]byte(nil), c.death.DeathId...)
	// 玩家已经死亡，周期step不会广播；数量变更必须由命令立即推基线。
	for _, count := range []uint32{4, 8} {
		c.auto(false, count)
		c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
		monsters := 0
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
	revive := &pb.ReviveChoice{DeathId: c.death.DeathId, Action: 1, Operation: c.operation()}
	deadLife, deadModeRevision := c.player().Life, c.scene.SkillModeState.Revision
	revived := c.asset(p.MsgC2SReviveChoice, revive)
	if revived.ErrorCode != 0 {
		t.Fatalf("开发主动复活失败:%v", revived)
	}
	c.inventoryAt(revived.NewRevision)
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().Hp == c.player().HpMax })
	if c.player().Life != deadLife+1 || c.scene.SkillModeState.PlayerLife != c.player().Life || c.scene.SkillModeState.Revision <= deadModeRevision {
		t.Fatal("同连接复活未同步新生命代次和严格递增的模式版本")
	}
	modeRevision := c.scene.SkillModeState.Revision
	if r := c.asset(p.MsgC2SReviveChoice, revive); r.ErrorCode != 0 {
		t.Fatalf("复活原op重放失败:%v", r)
	}
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.player().Life != deadLife+1 || c.scene.SkillModeState.Revision != modeRevision {
		t.Fatal("复活原op重放额外推进生命/模式版本")
	}
	// 数据库不得含临时战斗现场。
	var document bson.M
	must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": persisted.ID}).Decode(&document))
	for _, key := range []string{"hp", "mp", "position", "monsters", "cooldowns", "pet", "ground_items"} {
		if _, ok := document[key]; ok {
			t.Fatalf("临时字段%s被持久化", key)
		}
	}
	t.Logf("通过: DB=%s；5槽、单票、ACK、移动、资产幂等、21600秒日领、离线用时%d秒、真实升级到%d级、8怪切换、健康重连", cfg.Database, c.report.UsedSeconds, persisted.Level)
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func address(t *testing.T) string {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	must(t, e)
	a := l.Addr().String()
	l.Close()
	return a
}
func healthy(t *testing.T, a string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
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
func rpc(t *testing.T, url, token string, id uint16, in, out proto.Message, code uint32) {
	t.Helper()
	req, e := http.NewRequest("POST", url, bytes.NewReader(wire.Encode(wire.Message(id, 1, 0, in))))
	must(t, e)
	req.Header.Set("Authorization", "Bearer "+token)
	res, e := http.DefaultClient.Do(req)
	must(t, e)
	defer res.Body.Close()
	b, e := io.ReadAll(res.Body)
	must(t, e)
	f, e := wire.Decode(b)
	must(t, e)
	if f.Error != code {
		t.Fatalf("RPC%d error=%d want%d", id, f.Error, code)
	}
	if f.Error == 0 {
		must(t, proto.Unmarshal(f.Payload, out))
	}
}
func issue(t *testing.T, url, token string, key *pb.RoleKey) string {
	t.Helper()
	q := &pb.EntryTicketResult{}
	rpc(t, url, token, p.MsgC2SEntryTicket, &pb.EntryTicketRequest{Role: key}, q, 0)
	return q.Ticket
}

type client struct {
	t         *testing.T
	ws        *websocket.Conn
	ctx       context.Context
	stop      context.CancelFunc
	binding   uint64
	request   uint32
	input     uint32
	entry     *pb.EnterLineResult
	inventory *pb.InventoryResult
	scene     *pb.SceneBaselineChunk
	report    *pb.HostingReport
	hosting   *pb.HostingState
	watermark uint64
	death     *pb.DeathNotice
}

func join(t *testing.T, address, ticket string) *client {
	t.Helper()
	ctx, stop := context.WithTimeout(context.Background(), 80*time.Second)
	ws, _, err := websocket.Dial(ctx, "ws://"+address+"/ws", nil)
	must(t, err)
	c := &client{t: t, ws: ws, ctx: ctx, stop: stop}
	c.send(p.MsgC2SEnterLine, &pb.EnterLineRequest{AccountTicket: ticket})
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CEnterLine })
	// 必须先收到完整首基线，客户端才能完成场景加载后ACK；不能用假ACK掩盖握手死锁。
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.scene.SceneEpoch != c.entry.SceneEpoch || c.scene.BaselineId != c.entry.BaselineId || c.scene.ChunkCount != 1 || len(c.scene.Entities) == 0 {
		t.Fatal("ACK前未收到匹配的完整首基线")
	}
	if c.inventory != nil || c.report != nil {
		t.Fatal("ACK前提前发送资产或战报")
	}
	// 不匹配ACK不能激活；ACK前动作仍被拒绝。
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: c.entry.SceneEpoch, BaselineId: c.entry.BaselineId + 1})
	rejected := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 2})
	_, raw, err := c.ws.Read(c.ctx)
	must(t, err)
	f, err := wire.Decode(raw)
	must(t, err)
	if f.Request != rejected || f.ID != p.MsgC2SMoveIntent || f.Error != p.CodeLineRecovering {
		t.Fatalf("ACK前动作没有被门槛阻挡:%v", f)
	}
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: c.entry.SceneEpoch, BaselineId: c.entry.BaselineId})
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSyncReady })
	// 用同一actor队列内的无效方向作为屏障，验证重复ACK没有重发激活或战报。
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: c.entry.SceneEpoch, BaselineId: c.entry.BaselineId})
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: c.entry.SceneEpoch, BaselineId: c.entry.BaselineId})
	barrier := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 8})
	for {
		_, raw, err = c.ws.Read(c.ctx)
		must(t, err)
		f, err = wire.Decode(raw)
		must(t, err)
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
func (c *client) close() { c.ws.CloseNow(); c.stop() }
func (c *client) send(id uint16, q proto.Message) uint32 {
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
func (c *client) read() wire.Frame {
	f := c.readFrame()
	if f.Error != 0 {
		c.t.Fatalf("WS %d error%d；最近玩家状态=%v，已收到死亡通知=%t", f.ID, f.Error, c.player(), c.death != nil)
	}
	return f
}
func (c *client) readFrame() wire.Frame {
	_, b, e := c.ws.Read(c.ctx)
	must(c.t, e)
	f, e := wire.Decode(b)
	must(c.t, e)
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
	case p.MsgS2CSceneBaseline:
		c.scene = &pb.SceneBaselineChunk{}
		must(c.t, proto.Unmarshal(f.Payload, c.scene))
	case p.MsgS2CHostingReport:
		c.report = &pb.HostingReport{}
		must(c.t, proto.Unmarshal(f.Payload, c.report))
	}
	return f
}
func (c *client) until(predicate func(wire.Frame) bool) wire.Frame {
	c.t.Helper()
	for {
		f := c.read()
		if predicate(f) {
			return f
		}
	}
}
func (c *client) player() *pb.EntityState {
	for _, e := range c.scene.GetEntities() {
		if e.EntityKind == 1 {
			return e
		}
	}
	return nil
}
func (c *client) operation() *pb.OperationContext {
	return &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: c.watermark + 1, ExpectedRevision: c.inventory.Revision, ExpectedRuleVersion: c.entry.ResourceVersion}
}
func (c *client) asset(id uint16, q proto.Message) *pb.OperationResult {
	req := c.send(id, q)
	f := c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2COperationResult && f.Request == req })
	r := &pb.OperationResult{}
	must(c.t, proto.Unmarshal(f.Payload, r))
	if r.CommandSeq > c.watermark {
		c.watermark = r.CommandSeq
	}
	return r
}
func (c *client) inventoryAt(rev uint64) {
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CInventory && c.inventory.Revision >= rev })
}

func (c *client) auto(enabled bool, count uint32) {
	request := c.send(p.MsgC2SAutoBattle, &pb.AutoBattleIntent{SceneEpoch: c.entry.SceneEpoch, Enabled: enabled, MonsterCount: count})
	c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CHostingState && f.Request == request })
	if c.hosting == nil || c.hosting.AutoBattleEnabled != enabled {
		c.t.Fatalf("自动战斗响应没有返回真实状态: enabled=%v state=%v", enabled, c.hosting)
	}
}
