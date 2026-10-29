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
func TestTeleport21LearningBindingAndHealthyColdEntryMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启真实Mongo/WS后验证21学习、真实绑定、施放和冷热入场")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	skill, exists := cfg.Fixture.Skill(21)
	if cfg.Fixture.RuleVersion < 15 || !exists || skill.BookItemID != 13 || skill.Teleport == nil || !cfg.Fixture.SafeZoneBindingEnabled() {
		t.Fatal("真实唯一Excel尚未显式选择21/正常安全区绑定合同")
	}
	cfg.Database = "minimir_dev_game_test_teleport_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	ctx := context.Background()
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	launch := func() func() {
		runCtx, cancel := context.WithCancel(context.Background())
		var group sync.WaitGroup
		for _, mode := range []string{"account", "game", "gm"} {
			group.Go(func() {
				if e := bootstrap.Run(runCtx, cfg, mode); e != nil && runCtx.Err() == nil {
					t.Errorf("21 %s节点: %v", mode, e)
				}
			})
		}
		for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
			healthy(t, endpoint)
		}
		return func() { cancel(); group.Wait() }
	}
	stop := launch()
	t.Cleanup(func() {
		stop()
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "teleport_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	role := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: skill.JobID, Name: "瞬移" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"level": 19, "wallet": domain.Wallet{Gold: 1000000, BoundGold: 1000000, Yuanbao: 1000000, BoundYuanbao: 1000000}}})
	must(t, err)
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	defer func() { c.close() }()
	c.auto(false, 1)
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.roleBinding == nil || c.roleBinding.Binding != nil || c.roleBinding.Trusted {
		t.Fatal("新角色被伪造随机/出生绑定")
	}
	for _, learned := range c.inventory.LearnedSkills {
		if learned.SkillId == 21 {
			t.Fatal("21不是通过正常购书/学习获得")
		}
	}
	for _, item := range c.inventory.Items {
		if item.ItemId == 13 {
			t.Fatal("不得预置21原书")
		}
	}
	c.buyAndLearnSkill(cfg.Fixture, 21)
	teleportWSUpgradePreviewOnly(t, c, cfg.Fixture, inspection, ctx, roleID)
	beforeUnbound := proto.Clone(c.player()).(*pb.EntityState)
	unboundRole, err := inspection.Role(ctx, roleID)
	must(t, err)
	request := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: 21})
	travelExpectError(c, p.MsgC2SCastIntent, request, p.CodeSkillStateBlocked)
	c.auto(false, 1)
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.player().Mp != beforeUnbound.Mp {
		t.Fatal("无可信绑定的已学21拒绝前扣MP")
	}
	afterUnbound, err := inspection.Role(ctx, roleID)
	must(t, err)
	teleportWSAssertEconomy(t, unboundRole, afterUnbound, "无证明拒付")
	if afterUnbound.SourceSeq != unboundRole.SourceSeq || afterUnbound.Binding != nil {
		t.Fatal("拒绝21反而创建了绑定/来源序号")
	}
	travelToBaseline(c, 1, 2)
	travelAcknowledge(c, true)
	goal := &pb.GridPoint{X: 650, Y: 631}
	c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: goal})
	travelUntilWithin(c, 18*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && proto.Equal(c.player().Position, goal) && c.scene.NavigationState.GetStatus() == 2
	})
	if c.roleBinding == nil || !c.roleBinding.Trusted || c.roleBinding.SafeZoneId != 106 || c.roleBinding.Binding.GetMapId() != 2 || !proto.Equal(c.roleBinding.Binding.GetCenter(), goal) || len(c.bindingCommits) != 1 {
		t.Fatal("正常Walk未产生唯一真实永久绑定", c.roleBinding)
	}
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
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	personalScene, personalLife := c.entry.SceneEpoch, c.player().Life
	first, firstPaid := teleportWSCastAcceptedAndPaid(c, 10)
	firstResult := teleportWSResult(c, first.CastId)
	if firstResult.Outcome != 1 || firstResult.FromSceneEpoch != personalScene || firstResult.SceneEpoch != personalScene+1 || firstResult.PlayerLife != personalLife || firstResult.TargetMapId != 2 || firstResult.TransitionId == "" || !bytes.Equal(firstResult.Role.GetRoleId(), role.Key.RoleId) {
		t.Fatal("跨图21已执行身份错误", firstResult)
	}
	town, _ := cfg.Fixture.MapByID(2)
	if firstResult.Position == nil || !town.TeleportWalkable(devconfig.Point{X: int(firstResult.Position.X), Y: int(firstResult.Position.Y)}) {
		t.Fatal("21落点没有通过原图静态Walk")
	}
	teleportWSEnterAndAck(c, firstResult)
	if c.player().Life != personalLife || c.player().Mp != firstPaid || !proto.Equal(c.player().Position, firstResult.Position) || travelCount(c.scene, 3) != 0 {
		t.Fatal("21重建角色/退款或Town伪造怪物")
	}
	teleportWSFlux(t, c.scene, 30)
	afterFirst, err := inspection.Role(ctx, roleID)
	must(t, err)
	teleportWSAssertEconomy(t, proofRole, afterFirst, "跨图21")
	// 真实等待当前已选冷却，不改MP/CD/站位。初次成功状态仍使第二次费用为13。
	time.Sleep(time.Duration(skill.CooldownBaseMS)*time.Millisecond + 200*time.Millisecond)
	standingScene, standingLife := c.entry.SceneEpoch, c.player().Life
	second, secondPaid := teleportWSCastAcceptedAndPaid(c, 13)
	secondResult := teleportWSResult(c, second.CastId)
	if secondResult.Outcome != 1 || secondResult.FromSceneEpoch != standingScene || secondResult.SceneEpoch != standingScene || secondResult.TransitionId != "" || secondResult.PlayerLife != standingLife {
		t.Fatal("同Town瞬移错误换scene/生命", secondResult)
	}
	var displacement *pb.EntityDisplacement
	travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool {
		if f.ID == p.MsgS2CEnterLine {
			t.Fatal("同图21不应再次资源入场")
		}
		if f.ID != p.MsgS2CEntityDisplacement {
			return false
		}
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
	travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && proto.Equal(c.player().Position, secondResult.Position) && c.player().Mp == secondPaid
	})
	teleportWSFlux(t, c.scene, 30)
	// 用正常恢复达到满HP后再核健康重入，避免把合法自然恢复误判成初始化。
	if c.player().Hp < c.player().HpMax {
		travelUntilWithin(c, 8*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().Hp == c.player().HpMax })
	}
	standing := proto.Clone(c.player()).(*pb.EntityState)
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
	if c.entry.RoomId != 2 || c.entry.SceneEpoch != standingScene || c.player().Life != standing.Life || c.player().Hp != standing.Hp || c.player().Mp != standing.Mp || !proto.Equal(c.player().Position, standing.Position) {
		t.Fatal("新票健康重入初始化了21后的Town现场")
	}
	teleportWSFlux(t, c.scene, 30)
	latestRole, err := inspection.Role(ctx, roleID)
	must(t, err)
	teleportWSAssertEconomy(t, proofRole, latestRole, "两次21与健康重入")
	if c.roleBinding == nil || !c.roleBinding.Trusted || c.roleBinding.SourceSeq != uint64(latestRole.BindingSourceSeq) {
		t.Fatal("健康Enter未静默恢复当前可信绑定")
	}
	// 只有实际落入新安全区时允许新增binding来源；不能用SourceSeq变化冒充21收费。
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
	for _, e := range c.scene.Entities {
		for _, status := range e.Statuses {
			if status.StatusId == skill.BuffID {
				t.Fatal("TemporalFlux临时现场被永久恢复")
			}
		}
	}
	learned := false
	for _, s := range c.inventory.LearnedSkills {
		if s.SkillId == 21 && s.Level == 0 {
			learned = true
		}
	}
	if !learned {
		t.Fatal("正常购书学习的21冷启动丢失")
	}
	t.Logf("通过: 随机库%s，正常注册Mage/买原书13/学习21、真实升级Preview的MP10→13/时长30000→30000/惩罚30→30且未升级、nil proof拒付、真实Town Walk证明、MP10与Delay200、30014/Entry/ACK、同图30013/Flux费用13、健康保持与冷proof；旧World付费事件及SourceSeq争用另由game/Store专项，不冒称本WS实际发生", cfg.Database)
}

func teleportWSUpgradePreviewOnly(t *testing.T, c *client, f devconfig.Fixture, store *persistence.Store, ctx context.Context, roleID string) {
	t.Helper()
	row, found := f.Upgrade(21, 1)
	if !found || len(row.BookCosts) != 1 || row.BookCosts[0].ItemID != 13 || row.BookCosts[0].Quantity != 2 || row.PageCost != 5 {
		t.Fatal("实际0→1升级成本不再是本组已选原书13×2及书页×5")
	}
	need := map[uint32]int64{13: 2, row.PageItemID: row.PageCost}
	for _, itemID := range []uint32{13, row.PageItemID} {
		offerID := uint32(0)
		currency := "gold"
		if itemID == row.PageItemID {
			currency = "ingot"
		}
		for _, offer := range f.SkillShop {
			if offer.ItemID == itemID && offer.Currency == currency && offer.Quantity == 1 {
				offerID = offer.ID
				break
			}
		}
		if offerID == 0 {
			t.Fatal("升级预览须正常购买所选原书/书页，不能预置材料", itemID)
		}
		if result := c.skillConfirm(c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offerID, Quantity: need[itemID]})); result.ErrorCode != 0 {
			t.Fatal("真实购买预览材料失败", result)
		}
	}
	request := &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: 21}
	remaining := map[uint32]int64{13: 2, row.PageItemID: row.PageCost}
	for _, item := range c.inventory.Items {
		n := min(remaining[item.ItemId], item.Quantity)
		if n > 0 {
			request.Materials = append(request.Materials, &pb.ItemConsumption{InstanceId: item.InstanceId, Version: item.Version, Quantity: n})
			remaining[item.ItemId] -= n
		}
	}
	for _, n := range remaining {
		if n != 0 {
			t.Fatal("正常购买后仍没有完整升级材料")
		}
	}
	before, err := store.Role(ctx, roleID)
	must(t, err)
	preview := c.skillPreview(request)
	if preview.Kind != 2 || preview.SkillId != 21 || preview.FromLevel != 0 || preview.ToLevel != 1 || len(preview.Stats) != 3 {
		t.Fatal("真实21升级PB预览身份或比较项数量错误", preview)
	}
	keys := []string{"mp_cost", "duration_ms", "teleport_mana_penalty_percent"}
	beforeValues, afterValues := []int64{10, 30000, 30}, []int64{13, 30000, 30}
	for i, comparison := range preview.Stats {
		if comparison.Key != keys[i] || comparison.BeforeMin != beforeValues[i] || comparison.BeforeMax != beforeValues[i] || comparison.AfterMin != afterValues[i] || comparison.AfterMax != afterValues[i] {
			t.Fatal("21升级比较混入伤害/准确/道士逐阶秒数/材料，或未读取已选固定毫秒和百分比", comparison)
		}
	}
	// 只取预览，不提交升级，后续真实21仍按rank0的10/13费用验收。
	after, err := store.Role(ctx, roleID)
	must(t, err)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("仅升级预览改变了永久角色、材料、钱包或技能阶")
	}
	if rank, learned := after.Skill(21); !learned || rank != 0 {
		t.Fatal("预览意外升级，破坏后续rank0真实施放前态")
	}
}

func teleportWSCastAcceptedAndPaid(c *client, cost int64) (*pb.CastAccepted, int64) {
	c.t.Helper()
	before := c.player().Mp
	c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: 21})
	accepted := &pb.CastAccepted{}
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

func teleportWSResult(c *client, cast uint64) *pb.TeleportResult {
	c.t.Helper()
	result := &pb.TeleportResult{}
	travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool {
		if f.ID != p.MsgS2CTeleportResult {
			return false
		}
		must(c.t, proto.Unmarshal(f.Payload, result))
		return result.CastId == cast
	})
	return result
}

func teleportWSEnterAndAck(c *client, result *pb.TeleportResult) {
	c.t.Helper()
	old := proto.Clone(c.entry).(*pb.EnterLineResult)
	travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CEnterLine })
	if c.entry.ServerTransitionId != result.TransitionId || c.entry.SceneEpoch != result.SceneEpoch || c.entry.RoomId != result.TargetMapId || !proto.Equal(c.entry.Position, result.Position) {
		c.t.Fatal("30014与Entry未严格同id/scene/map/point配对", result, c.entry)
	}
	travelUntilWithin(c, time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.scene.SceneEpoch == c.entry.SceneEpoch && c.scene.BaselineId == c.entry.BaselineId
	})
	if c.scene.ChunkCount != 1 || c.player() == nil {
		c.t.Fatal("缺新地图完整首视口基线")
	}
	c.send(p.MsgC2SEnterWorldAck, &pb.EnterWorldAck{SceneEpoch: old.SceneEpoch, BaselineId: old.BaselineId})
	request := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 2})
	travelExpectError(c, p.MsgC2SMoveIntent, request, p.CodeLineRecovering)
	resultSeq := c.watermark
	op := c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
	if op.ErrorCode != p.CodeLineRecovering || op.CommandSeq != resultSeq {
		c.t.Fatal("21 ACK前经济越过门禁/推进命令floor", op)
	}
	travelAcknowledge(c, true)
	request = c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: old.SceneEpoch, Direction: 2})
	travelExpectError(c, p.MsgC2SMoveIntent, request, p.CodeInvalidArgument)
}

func teleportWSFlux(t *testing.T, scene *pb.SceneBaselineChunk, percent uint32) {
	t.Helper()
	for _, e := range scene.Entities {
		if e.EntityKind == 1 {
			for _, s := range e.Statuses {
				if s.StatusId == 21 && s.TeleportManaPenaltyPercent == percent && s.RemainingMs > 0 && s.RemainingMs <= 30000 {
					return
				}
			}
		}
	}
	t.Fatal("实际权威基线缺成功30秒TemporalFlux观察")
}

func teleportWSAssertEconomy(t *testing.T, before, after domain.Role, phase string) {
	t.Helper()
	if before.EconomicSeq != after.EconomicSeq || before.Wallet != after.Wallet || before.HostingSeconds != after.HostingSeconds || before.Level != after.Level || before.Experience != after.Experience || !reflect.DeepEqual(before.Items, after.Items) || !reflect.DeepEqual(before.Skills, after.Skills) {
		t.Fatalf("%s产生非MP费用/新收益或更改所学技能", phase)
	}
}
