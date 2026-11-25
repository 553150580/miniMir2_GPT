// 验证瞬移 21 的学习、绑定选择及健康与冷入场，区分场景期状态和永久回执。

package integration

import (
	"bytes"
	"context"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

// 独立随机库，正常购书/学习/Walk证明；仅首次Enter前设置已授权的等级/钱包前态。
// TestTeleport21LearningBindingAndHealthyColdEntryMongoWS 验证瞬移技能从学习到绑定施法的闭环，对照健康重连和冷入场。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21LearningBindingAndHealthyColdEntryMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启真实Mongo/WS后验证21学习、真实绑定、施放和冷热入场")
	}
	// cfg 保存由“historical”来源城镇配置取得的服务配置。
	cfg := historicalSourceTownConfig(t)
	// skill 接收技能供后续步骤使用，exists 保存由技能取得的存在。
	skill, exists := cfg.Fixture.Skill(21)
	if cfg.Fixture.RuleVersion < 15 || !exists || skill.BookItemID != 13 || skill.Teleport == nil || !cfg.Fixture.SafeZoneBindingEnabled() {
		t.Fatal("真实唯一Excel尚未显式选择21/正常安全区绑定合同")
	}
	cfg.Database = "minimir_dev_game_test_teleport_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// ctx 保存由“background”取得的上下文。
	ctx := context.Background()
	// inspection 保存由打开取得的检查，err 接收打开过程中可能产生的错误，供后续失败分支处理。
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	// launch 封装启动对应的后续处理。
	launch := func() func() {
		// runCtx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		runCtx, cancel := context.WithCancel(context.Background())
		// group 记录分组，使用协程汇合计数器表示。
		var group sync.WaitGroup
		// mode 接收遍历文本列表时当前的元素。
		for _, mode := range []string{"account", "game", "gm"} {
			group.Go(func() {
				// e 接收运行批次供后续步骤使用。
				if e := bootstrap.Run(runCtx, cfg, mode); e != nil && runCtx.Err() == nil {
					t.Errorf("21 %s节点: %v", mode, e)
				}
			})
		}
		// endpoint 接收遍历文本列表时当前的元素。
		for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
			healthy(t, endpoint)
		}
		return func() { cancel(); group.Wait() }
	}
	// stop 保存由启动取得的停止。
	stop := launch()
	t.Cleanup(func() {
		stop()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "teleport_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	// role 保存当前处理所用的角色。
	role := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: skill.JobID, Name: "瞬移" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())}, role, 0)
	// roleID 标识当前业务使用的角色。
	roleID := domain.Hex(role.Key.RoleId)
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"level": 19, "wallet": domain.Wallet{Gold: 1000000, BoundGold: 1000000, Yuanbao: 1000000, BoundYuanbao: 1000000}}})
	must(t, err)
	// c 接收加入供后续步骤使用。
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer func() { c.close() }()
	c.auto(false, 1)
	// f 记录协议帧。
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.roleBinding == nil || c.roleBinding.Binding != nil || c.roleBinding.Trusted {
		t.Fatal("新角色被伪造随机/出生绑定")
	}
	// learned 接收遍历已学习技能集合时当前的元素。
	for _, learned := range c.inventory.LearnedSkills {
		if learned.SkillId == 21 {
			t.Fatal("21不是通过正常购书/学习获得")
		}
	}
	// item 接收遍历道具集合时当前的元素。
	for _, item := range c.inventory.Items {
		if item.ItemId == 13 {
			t.Fatal("不得预置21原书")
		}
	}
	c.buyAndLearnSkill(cfg.Fixture, 21)
	teleportWSUpgradePreviewOnly(t, c, cfg.Fixture, inspection, ctx, roleID)
	// beforeUnbound 按目标类型读取当前接口值，得到操作前的状态未绑定。
	beforeUnbound := proto.Clone(c.player()).(*pb.EntityState)
	// unboundRole 保存由角色取得的未绑定角色。
	unboundRole, err := inspection.Role(ctx, roleID)
	must(t, err)
	// request 保存由发送取得的请求。
	request := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: 21})
	travelExpectError(c, p.MsgC2SCastIntent, request, p.CodeSkillStateBlocked)
	c.auto(false, 1)
	// f 记录协议帧。
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.player().Mp != beforeUnbound.Mp {
		t.Fatal("无可信绑定的已学21拒绝前扣MP")
	}
	// afterUnbound 保存由角色取得的操作后的状态未绑定。
	afterUnbound, err := inspection.Role(ctx, roleID)
	must(t, err)
	teleportWSAssertEconomy(t, unboundRole, afterUnbound, "无证明拒付")
	if afterUnbound.SourceSeq != unboundRole.SourceSeq || afterUnbound.Binding != nil {
		t.Fatal("拒绝21反而创建了绑定/来源序号")
	}
	travelToBaseline(c, 1, 2)
	travelAcknowledge(c, true)
	// goal 保存当前处理所用的目标格。
	goal := &pb.GridPoint{X: 650, Y: 631}
	c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: goal})
	// f 记录协议帧。
	travelUntilWithin(c, 18*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && proto.Equal(c.player().Position, goal) && c.scene.NavigationState.GetStatus() == 2
	})
	if c.roleBinding == nil || !c.roleBinding.Trusted || c.roleBinding.SafeZoneId != 106 || c.roleBinding.Binding.GetMapId() != 2 || !proto.Equal(c.roleBinding.Binding.GetCenter(), goal) || len(c.bindingCommits) != 1 {
		t.Fatal("正常Walk未产生唯一真实永久绑定", c.roleBinding)
	}
	// proofRole 保存由角色取得的“proof”角色。
	proofRole, err := inspection.Role(ctx, roleID)
	must(t, err)
	if proofRole.BindingSourceSeq <= 0 || proofRole.BindingSafeZoneID != 106 {
		t.Fatal("Store未从正式receipt恢复来源证明")
	}
	travelToBaseline(c, 2, cfg.Fixture.Map.ID)
	travelAcknowledge(c, false)
	// 健康旅行保留最后一步Walk的NextMove/ActionReadyAt；按权威步间隔等待释放，不能强清时钟。
	time.Sleep(time.Duration(c.player().MoveIntervalMs) * time.Millisecond)
	c.auto(false, 1)
	// f 记录协议帧。
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	// personalScene 保存场景代次，用于当前步骤中的个人场景，personalLife 保存生命代次，用于当前步骤中的个人生命代次。
	personalScene, personalLife := c.entry.SceneEpoch, c.player().Life
	// first 保存由瞬移WebSocket施法已受理“and”已支付取得的第一项。
	// firstPaid 保存由瞬移WebSocket施法已受理“and”已支付取得的第一项已支付。
	first, firstPaid := teleportWSCastAcceptedAndPaid(c, 10)
	// firstResult 保存由瞬移WebSocket结果取得的第一项结果。
	firstResult := teleportWSResult(c, first.CastId)
	if firstResult.Outcome != 1 || firstResult.FromSceneEpoch != personalScene || firstResult.SceneEpoch != personalScene+1 || firstResult.PlayerLife != personalLife || firstResult.TargetMapId != 2 || firstResult.TransitionId == "" || !bytes.Equal(firstResult.Role.GetRoleId(), role.Key.RoleId) {
		t.Fatal("跨图21已执行身份错误", firstResult)
	}
	// town 保存由地图“by”标识取得的城镇。
	town, _ := cfg.Fixture.MapByID(2)
	if firstResult.Position == nil || !town.TeleportWalkable(devconfig.Point{X: int(firstResult.Position.X), Y: int(firstResult.Position.Y)}) {
		t.Fatal("21落点没有通过原图静态Walk")
	}
	teleportWSEnterAndAck(c, firstResult)
	if c.player().Life != personalLife || c.player().Mp != firstPaid || !proto.Equal(c.player().Position, firstResult.Position) || travelCount(c.scene, 3) != 0 {
		t.Fatal("21重建角色/退款或Town伪造怪物")
	}
	teleportWSFlux(t, c.scene, 30)
	// afterFirst 保存由角色取得的操作后的状态第一项。
	afterFirst, err := inspection.Role(ctx, roleID)
	must(t, err)
	teleportWSAssertEconomy(t, proofRole, afterFirst, "跨图21")
	// 真实等待当前已选冷却，不改MP/CD/站位。初次成功状态仍使第二次费用为13。
	time.Sleep(time.Duration(skill.CooldownBaseMS)*time.Millisecond + 200*time.Millisecond)
	// standingScene 保存场景代次，用于当前步骤中的“standing”场景，standingLife 保存生命代次，用于当前步骤中的“standing”生命代次。
	standingScene, standingLife := c.entry.SceneEpoch, c.player().Life
	// second 保存由瞬移WebSocket施法已受理“and”已支付取得的秒。
	// secondPaid 保存由瞬移WebSocket施法已受理“and”已支付取得的秒已支付。
	second, secondPaid := teleportWSCastAcceptedAndPaid(c, 13)
	// secondResult 保存由瞬移WebSocket结果取得的秒结果。
	secondResult := teleportWSResult(c, second.CastId)
	if secondResult.Outcome != 1 || secondResult.FromSceneEpoch != standingScene || secondResult.SceneEpoch != standingScene || secondResult.TransitionId != "" || secondResult.PlayerLife != standingLife {
		t.Fatal("同Town瞬移错误换scene/生命", secondResult)
	}
	// displacement 记录位移，使用实体位移表示。
	var displacement *pb.EntityDisplacement
	// f 记录协议帧。
	travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool {
		if f.ID == p.MsgS2CEnterLine {
			t.Fatal("同图21不应再次资源入场")
		}
		if f.ID != p.MsgS2CEntityDisplacement {
			return false
		}
		// q 保存当前处理所用的实体位移。
		q := &pb.EntityDisplacement{}
		must(t, proto.Unmarshal(f.Payload, q))
		if q.CastId == second.CastId {
			displacement = q
			return true
		}
		return false
	})
	if displacement.Kind != 4 || displacement.SceneEpoch != standingScene || displacement.EntityLife != standingLife || len(displacement.Path) != 1 || !proto.Equal(displacement.Path[0], secondResult.Position) {
		t.Fatal("同图30013未对应唯一真实位移", displacement)
	}
	// f 记录协议帧。
	travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && proto.Equal(c.player().Position, secondResult.Position) && c.player().Mp == secondPaid
	})
	teleportWSFlux(t, c.scene, 30)
	// 用正常恢复达到满HP后再核健康重入，避免把合法自然恢复误判成初始化。
	if c.player().Hp < c.player().HpMax {
		// f 记录协议帧。
		travelUntilWithin(c, 8*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().Hp == c.player().HpMax })
	}
	// standing 按目标类型读取当前接口值，得到“standing”。
	standing := proto.Clone(c.player()).(*pb.EntityState)
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	if c.entry.RoomId != 2 || c.entry.SceneEpoch != standingScene || c.player().Life != standing.Life || c.player().Hp != standing.Hp || c.player().Mp != standing.Mp || !proto.Equal(c.player().Position, standing.Position) {
		t.Fatal("新票健康重入初始化了21后的Town现场")
	}
	teleportWSFlux(t, c.scene, 30)
	// latestRole 保存由角色取得的“latest”角色。
	latestRole, err := inspection.Role(ctx, roleID)
	must(t, err)
	teleportWSAssertEconomy(t, proofRole, latestRole, "两次21与健康重入")
	if c.roleBinding == nil || !c.roleBinding.Trusted || c.roleBinding.SourceSeq != uint64(latestRole.BindingSourceSeq) {
		t.Fatal("健康Enter未静默恢复当前可信绑定")
	}
	// 只有实际落入新安全区时允许新增binding来源；不能用SourceSeq变化冒充21收费。
	// bindings 保存由数量文档取得的会话绑定集合。
	bindings, err := inspection.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": roleID, "kind": "safe_zone_binding", "source_seq": bson.M{"$gt": proofRole.SourceSeq}})
	must(t, err)
	// 已存在的Enter合同每次新票同时推进SessionEpoch/Revision，不消耗SourceSeq或经济命令。
	if latestRole.SourceSeq-proofRole.SourceSeq != bindings || latestRole.SessionEpoch != proofRole.SessionEpoch+1 || latestRole.Revision-proofRole.Revision != bindings+1 {
		t.Fatalf("21产生了落区绑定及一次合法Enter以外的永久变化: source=%d revision=%d session=%d binding=%d", latestRole.SourceSeq-proofRole.SourceSeq, latestRole.Revision-proofRole.Revision, latestRole.SessionEpoch-proofRole.SessionEpoch, bindings)
	}
	c.close()
	stop()
	stop = launch()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	if c.entry.RoomId != cfg.Fixture.Map.ID || c.entry.SceneKind != devconfig.SceneKindPersonal || c.entry.Position.X != int32(cfg.Fixture.Map.PlayerSpawn.X) || c.entry.Position.Y != int32(cfg.Fixture.Map.PlayerSpawn.Y) {
		t.Fatal("完整停止后把21临时站位当永久恢复点", c.entry)
	}
	if c.roleBinding == nil || !c.roleBinding.Trusted || c.roleBinding.SourceSeq != uint64(latestRole.BindingSourceSeq) || c.roleBinding.Binding.GetMapId() != latestRole.Binding.MapID || c.roleBinding.Binding.GetCenter().X != latestRole.Binding.X || c.roleBinding.Binding.GetCenter().Y != latestRole.Binding.Y {
		t.Fatal("冷Enter没有核正式永久绑定证明")
	}
	// e 接收遍历实体时当前的元素。
	for _, e := range c.scene.Entities {
		// status 接收遍历状态集合时当前的元素。
		for _, status := range e.Statuses {
			if status.StatusId == skill.BuffID {
				t.Fatal("TemporalFlux临时现场被永久恢复")
			}
		}
	}
	// learned 保存当前处理所用的已学习。
	learned := false
	// s 接收遍历已学习技能集合时当前的元素。
	for _, s := range c.inventory.LearnedSkills {
		if s.SkillId == 21 && s.Level == 0 {
			learned = true
		}
	}
	if !learned {
		t.Fatal("正常购书学习的21冷启动丢失")
	}
	// 停止本用例的旧实例，保留随机库中通过正常行走取得的原回执，按当前规则重入。
	// current 保存由加载取得的当前状态。
	current, err := loadIntegrationConfig()
	must(t, err)
	current.Database, current.AccountAddress, current.GameAddress, current.GMAddress = cfg.Database, cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress
	// mapped 保存由城镇瞬移会话绑定取得的“mapped”，ok 表明查询、校验或类型转换是否成功。
	mapped, ok := current.Fixture.TownTeleportBinding(devconfig.BindingPoint{
		MapID: latestRole.Binding.MapID, X: latestRole.Binding.X, Y: latestRole.Binding.Y,
	}, latestRole.BindingSafeZoneID)
	if !ok || mapped.MapID == 2 || current.Fixture.RuleVersion < 24 {
		t.Fatal("原回执没有可用的当前紧凑城镇迁移目标", latestRole.Binding, latestRole.BindingSafeZoneID)
	}
	c.close()
	stop()
	cfg = current
	stop = launch()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	if c.entry.RoomId != cfg.Fixture.Map.ID || c.roleBinding == nil || !c.roleBinding.Trusted ||
		c.roleBinding.Binding.GetMapId() != 2 || c.roleBinding.SourceSeq != uint64(latestRole.BindingSourceSeq) {
		t.Fatal("升级后冷入场没有保持原回执或提前改写成新地图", c.entry, c.roleBinding)
	}
	c.auto(false, 1)
	// f 记录协议帧。
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	// upgradeCast 保存由瞬移WebSocket施法已受理“and”已支付取得的升级施法。
	// upgradeMP 保存由瞬移WebSocket施法已受理“and”已支付取得的升级魔法值。
	upgradeCast, upgradeMP := teleportWSCastAcceptedAndPaid(c, 10)
	// upgradeResult 保存由瞬移WebSocket结果取得的升级结果。
	upgradeResult := teleportWSResult(c, upgradeCast.CastId)
	// mappedTown 保存由地图“by”标识取得的“mapped”城镇。
	mappedTown, _ := cfg.Fixture.MapByID(mapped.MapID)
	if upgradeResult.Outcome != 1 || upgradeResult.TargetMapId != mapped.MapID || upgradeResult.TransitionId == "" ||
		upgradeResult.Position == nil || !mappedTown.Walkable(devconfig.Point{X: int(upgradeResult.Position.X), Y: int(upgradeResult.Position.Y)}) {
		t.Fatal("真实历史回执未通过21进入对应紧凑城镇的权威落格", upgradeResult)
	}
	teleportWSEnterAndAck(c, upgradeResult)
	if c.player().Mp != upgradeMP || c.player().Life != upgradeResult.PlayerLife {
		t.Fatal("规则升级后的21没有继续保留原生命与技能费用", c.player())
	}
	if c.roleBinding.Binding.GetMapId() != mapped.MapID {
		// f 记录协议帧。
		travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool {
			return f.ID == p.MsgS2CBindingState && c.roleBinding.Binding.GetMapId() == mapped.MapID
		})
	}
	// zone 保存从安全区域集合取出的区域。
	zone := mappedTown.SafeZones[0]
	if !c.roleBinding.Trusted || c.roleBinding.SafeZoneId != zone.ID ||
		c.roleBinding.SourceSeq != uint64(latestRole.BindingSourceSeq+1) ||
		c.roleBinding.Binding.GetCenter().X != int32(zone.Center.X) || c.roleBinding.Binding.GetCenter().Y != int32(zone.Center.Y) {
		t.Fatal("仅实际落格应生成一个当前安全区的新可信源回执", c.roleBinding)
	}
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	if c.entry.RoomId != mapped.MapID || !c.roleBinding.Trusted || c.roleBinding.Binding.GetMapId() != mapped.MapID ||
		c.roleBinding.SourceSeq != uint64(latestRole.BindingSourceSeq+1) {
		t.Fatal("新回执未通过实际Mongo冷验和当前城镇健康重连", c.entry, c.roleBinding)
	}
	t.Logf("原rule23地图2可信凭证在同一随机库通过rule24冷验，正常21进入紧凑地图%d并仅提交一次新安全区%d", mapped.MapID, zone.ID)
	t.Logf("通过: 随机库%s，正常注册Mage/买原书13/学习21、真实升级Preview的MP10→13/时长30000→30000/惩罚30→30且未升级、nil proof拒付、真实Town Walk证明、MP10与Delay200、30014/Entry/ACK、同图30013/Flux费用13、健康保持与冷proof；旧World付费事件及SourceSeq争用另由game/Store专项，不冒称本WS实际发生", cfg.Database)
}

// teleportWSUpgradePreviewOnly 处理瞬移WebSocket升级预览“only”。
// t 提供断言失败报告、子用例和清理注册，c 记录客户端，f 记录开发测试配置，store 记录持久层存储，ctx 向下传递当前请求的截止时间和取消信号，roleID 标识当前业务使用的角色。
func teleportWSUpgradePreviewOnly(t *testing.T, c *client, f devconfig.Fixture, store *persistence.Store, ctx context.Context, roleID string) {
	t.Helper()
	// row 保存由升级取得的行记录，found 保存由升级取得的已找到标记。
	row, found := f.Upgrade(21, 1)
	if !found || len(row.BookCosts) != 1 || row.BookCosts[0].ItemID != 13 || row.BookCosts[0].Quantity != 2 || row.PageCost != 5 {
		t.Fatal("实际0→1升级成本不再是本组已选原书13×2及书页×5")
	}
	// need 初始化用作所需量的按数值索引的数值映射。
	need := map[uint32]int64{13: 2, row.PageItemID: row.PageCost}
	// itemID 接收遍历数值列表时当前的元素。
	for _, itemID := range []uint32{13, row.PageItemID} {
		// offerID 标识当前业务使用的“offer”。
		offerID := uint32(0)
		// currency 保存“currency”所使用的文本。
		currency := "gold"
		if itemID == row.PageItemID {
			currency = "ingot"
		}
		// offer 接收遍历技能商店时当前的元素。
		for _, offer := range f.SkillShop {
			if offer.ItemID == itemID && offer.Currency == currency && offer.Quantity == 1 {
				offerID = offer.ID
				break
			}
		}
		if offerID == 0 {
			t.Fatal("升级预览须正常购买所选原书/书页，不能预置材料", itemID)
		}
		// result 保存由技能确认取得的结果。
		if result := c.skillConfirm(c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offerID, Quantity: need[itemID]})); result.ErrorCode != 0 {
			t.Fatal("真实购买预览材料失败", result)
		}
	}
	// request 保存当前处理所用的请求。
	request := &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: 21}
	// remaining 初始化用作剩余额度的按数值索引的数值映射。
	remaining := map[uint32]int64{13: 2, row.PageItemID: row.PageCost}
	// item 接收遍历道具集合时当前的元素。
	for _, item := range c.inventory.Items {
		// n 接收最小供后续步骤使用。
		n := min(remaining[item.ItemId], item.Quantity)
		if n > 0 {
			request.Materials = append(request.Materials, &pb.ItemConsumption{InstanceId: item.InstanceId, Version: item.Version, Quantity: n})
			remaining[item.ItemId] -= n
		}
	}
	// n 接收遍历剩余额度时当前的元素。
	for _, n := range remaining {
		if n != 0 {
			t.Fatal("正常购买后仍没有完整升级材料")
		}
	}
	// before 保存由角色取得的操作前的状态，err 接收角色过程中可能产生的错误，供后续失败分支处理。
	before, err := store.Role(ctx, roleID)
	must(t, err)
	// preview 保存由技能预览取得的预览。
	preview := c.skillPreview(request)
	if preview.Kind != 2 || preview.SkillId != 21 || preview.FromLevel != 0 || preview.ToLevel != 1 || len(preview.Stats) != 3 {
		t.Fatal("真实21升级PB预览身份或比较项数量错误", preview)
	}
	// keys 初始化用作键集合的文本列表。
	keys := []string{"mp_cost", "duration_ms", "teleport_mana_penalty_percent"}
	// beforeValues 初始化用作操作前的状态值集合的数值列表，afterValues 初始化用作操作后的状态值集合的数值列表。
	beforeValues, afterValues := []int64{10, 30000, 30}, []int64{13, 30000, 30}
	// i 定位统计或属性中正在处理的成员，comparison 接收遍历统计或属性时当前的元素。
	for i, comparison := range preview.Stats {
		if comparison.Key != keys[i] || comparison.BeforeMin != beforeValues[i] || comparison.BeforeMax != beforeValues[i] || comparison.AfterMin != afterValues[i] || comparison.AfterMax != afterValues[i] {
			t.Fatal("21升级比较混入伤害/准确/道士逐阶秒数/材料，或未读取已选固定毫秒和百分比", comparison)
		}
	}
	// 只取预览，不提交升级，后续真实21仍按rank0的10/13费用验收。
	// after 保存由角色取得的操作后的状态。
	after, err := store.Role(ctx, roleID)
	must(t, err)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("仅升级预览改变了永久角色、材料、钱包或技能阶")
	}
	// rank 保存由技能取得的排序位次，learned 保存由技能取得的已学习。
	if rank, learned := after.Skill(21); !learned || rank != 0 {
		t.Fatal("预览意外升级，破坏后续rank0真实施放前态")
	}
}

// teleportWSCastAcceptedAndPaid 处理瞬移WebSocket施法已受理“and”已支付。
// c 记录客户端，cost 记录消耗，使用数值表示。
func teleportWSCastAcceptedAndPaid(c *client, cost int64) (*pb.CastAccepted, int64) {
	c.t.Helper()
	// before 保存魔法值，用于当前步骤中的操作前的状态。
	before := c.player().Mp
	c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: 21})
	// accepted 保存当前处理所用的已受理。
	accepted := &pb.CastAccepted{}
	// f 记录协议帧。
	travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool {
		if f.ID != p.MsgS2CCastAccepted {
			return false
		}
		must(c.t, proto.Unmarshal(f.Payload, accepted))
		return accepted.SkillId == 21
	})
	if accepted.DelayMs != 200 || accepted.CasterLife != c.player().Life || accepted.TargetHandle != 0 {
		c.t.Fatal("21受理没有明确200ms/原Life/无客户端目标", accepted)
	}
	// f 记录协议帧。
	travelUntilWithin(c, time.Second, func(f wire.Frame) bool {
		if f.ID == p.MsgS2CTeleportResult {
			c.t.Fatal("施法即时MP快照迟于传送结果")
		}
		return f.ID == p.MsgS2CSceneBaseline
	})
	if c.player().Mp != before-cost {
		c.t.Fatalf("实际21费用=%d，期望%d", before-c.player().Mp, cost)
	}
	return accepted, c.player().Mp
}

// teleportWSResult 返回瞬移WebSocket结果供调用方使用。
// c 记录客户端，cast 记录施法，使用数值表示。
func teleportWSResult(c *client, cast uint64) *pb.TeleportResult {
	c.t.Helper()
	// result 保存当前处理所用的结果。
	result := &pb.TeleportResult{}
	// f 记录协议帧。
	travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool {
		if f.ID != p.MsgS2CTeleportResult {
			return false
		}
		must(c.t, proto.Unmarshal(f.Payload, result))
		return result.CastId == cast
	})
	return result
}

// teleportWSEnterAndAck 处理瞬移WebSocket入场“and”确认。
// c 记录客户端，result 记录结果，使用瞬移结果表示。
func teleportWSEnterAndAck(c *client, result *pb.TeleportResult) {
	c.t.Helper()
	// old 按目标类型读取当前接口值，得到原状态。
	old := proto.Clone(c.entry).(*pb.EnterLineResult)
	// f 记录协议帧。
	travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CEnterLine })
	if c.entry.ServerTransitionId != result.TransitionId || c.entry.SceneEpoch != result.SceneEpoch || c.entry.RoomId != result.TargetMapId || !proto.Equal(c.entry.Position, result.Position) {
		c.t.Fatal("30014与Entry未严格同id/scene/map/point配对", result, c.entry)
	}
	// f 记录协议帧。
	travelUntilWithin(c, time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.scene.SceneEpoch == c.entry.SceneEpoch && c.scene.BaselineId == c.entry.BaselineId
	})
	if c.scene.ChunkCount != 1 || c.player() == nil {
		c.t.Fatal("缺新地图完整首视口基线")
	}
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: old.SceneEpoch, BaselineId: old.BaselineId})
	// request 保存由发送取得的请求。
	request := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 2})
	travelExpectError(c, p.MsgC2SMoveIntent, request, p.CodeLineRecovering)
	// resultSeq 保存最高已推进序号，用于当前步骤中的结果序号。
	resultSeq := c.watermark
	// op 保存由资产取得的操作。
	op := c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
	if op.ErrorCode != p.CodeLineRecovering || op.CommandSeq != resultSeq {
		c.t.Fatal("21 ACK前经济越过门禁/推进命令floor", op)
	}
	travelAcknowledge(c, true)
	request = c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: old.SceneEpoch, Direction: 2})
	travelExpectError(c, p.MsgC2SMoveIntent, request, p.CodeInvalidArgument)
}

// teleportWSFlux 处理瞬移WebSocket“flux”。
// t 提供断言失败报告、子用例和清理注册，scene 记录场景，使用场景场景基线分片表示，percent 记录“percent”，使用数值表示。
func teleportWSFlux(t *testing.T, scene *pb.SceneBaselineChunk, percent uint32) {
	t.Helper()
	// e 接收遍历实体时当前的元素。
	for _, e := range scene.Entities {
		if e.EntityKind == 1 {
			// s 接收遍历状态集合时当前的元素。
			for _, s := range e.Statuses {
				if s.StatusId == 21 && s.TeleportManaPenaltyPercent == percent && s.RemainingMs > 0 && s.RemainingMs <= 30000 {
					return
				}
			}
		}
	}
	t.Fatal("实际权威基线缺成功30秒TemporalFlux观察")
}

// teleportWSAssertEconomy 处理瞬移WebSocket“assert”经济。
// t 提供断言失败报告、子用例和清理注册，before 记录操作前的状态，使用角色表示，after 记录操作后的状态，使用角色表示，phase 记录阶段，使用文本表示。
func teleportWSAssertEconomy(t *testing.T, before, after domain.Role, phase string) {
	t.Helper()
	if before.EconomicSeq != after.EconomicSeq || before.Wallet != after.Wallet || before.HostingSeconds != after.HostingSeconds || before.Level != after.Level || before.Experience != after.Experience || !reflect.DeepEqual(before.Items, after.Items) || !reflect.DeepEqual(before.Skills, after.Skills) {
		t.Fatalf("%s产生非MP费用/新收益或更改所学技能", phase)
	}
}
