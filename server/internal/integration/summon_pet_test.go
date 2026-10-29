package integration

import (
	"context"
	"os"
	"reflect"
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

// 资格与物品前态显式隔离：19级、正常初始原书25、两Count护符；没有推断主服包规格或伪造临时宠物。
// 资格之后全部走真实注册/创角/Enter/学习/装卸/施法/攻击/旅行/健康票/冷建协议。
func TestSummonPetMaterialRecallTrainingNoPetsAndColdMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启专属Mongo副本集，验证17真实协议隔离链路")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	catalog, err := devconfig.LoadSummonCandidates("../../../config/development/summon-candidates.json")
	must(t, err)
	if cfg.Fixture.RuleVersion != 18 || cfg.Fixture.CastingSupply.Enabled {
		t.Fatal("必须从主rule17关闭材料的真实配置创建隔离样本")
	}
	if _, enabled := cfg.Fixture.Skill(17); enabled {
		t.Fatal("主配置不能因候选测试被开启17")
	}
	f := &cfg.Fixture
	f.Skills, f.Items = append(f.Skills, catalog.Skills...), append(f.Items, catalog.Books...)
	f.SkillUpgrades = append(f.SkillUpgrades, catalog.Upgrades...)
	f.CastingSupply.Enabled, f.PetRuntime.BootstrapMode = true, "none"
	for i := range f.Items {
		if f.Items[i].ID == 187 {
			f.Items[i].MaxStack = 2 // 明确的隔离Count容量，不是DuraMax、售价或购包单位。
		}
	}
	for i := range f.InitialSkillBooks {
		if f.InitialSkillBooks[i].JobID == 2 {
			f.InitialSkillBooks[i].ItemID = 25
		}
	}
	for i := range f.Maps {
		if f.Maps[i].ID == 2 {
			blocked := true
			f.Maps[i].NoPets = &blocked
		}
	}
	f.Map.MonsterSpawns[0] = devconfig.Point{X: 9, Y: 8}
	for i := range f.Monsters {
		m := &f.Monsters[i]
		m.HP, m.AttackMin, m.AttackMax, m.Agility = 1, 0, 0, 0
		m.AttackMS, m.MoveMS = 600000, 600000
	}
	must(t, f.Validate())
	skill := catalog.Skills[0]
	cfg.Database = "minimir_dev_game_test_summon_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	ctx := context.Background()
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	launch := func() func() {
		run, cancel := context.WithCancel(context.Background())
		var group sync.WaitGroup
		for _, mode := range []string{"account", "game", "gm"} {
			group.Go(func() {
				if err := bootstrap.Run(run, cfg, mode); err != nil && run.Err() == nil {
					t.Errorf("召唤%s节点: %v", mode, err)
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
		must(t, store.DB.Drop(cleanup))
		store.Close(cleanup)
	})
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "summon_" + domain.ID()[:9], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 2, Name: "召唤" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	createdRole, err := store.Role(ctx, roleID)
	must(t, err)
	initialBookID := ""
	for _, item := range createdRole.Items {
		if item.ItemID == 25 {
			initialBookID = item.ID
			break
		}
	}
	if initialBookID == "" {
		t.Fatal("正常创角没有给本地初始书25")
	}
	supply := domain.Item{ID: domain.ID(), ItemID: 187, Quantity: 2, Bound: true, Version: 1, SourceKind: "development_fixture", SourceID: "two_explicit_count_units"}
	inputs := []domain.Item{supply}
	upgrade := catalog.Upgrades[0]
	for n := int64(0); n < upgrade.BookCosts[0].Quantity; n++ {
		inputs = append(inputs, domain.Item{ID: domain.ID(), ItemID: 25, Quantity: 1, Bound: true, Version: 1, Quality: 1, SourceKind: "development_fixture", SourceID: "explicit_upgrade_inputs"})
	}
	inputs = append(inputs, domain.Item{ID: domain.ID(), ItemID: upgrade.PageItemID, Quantity: upgrade.PageCost, Bound: true, Version: 1, SourceKind: "development_fixture", SourceID: "explicit_upgrade_inputs"})
	_, err = store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"level": 19}, "$push": bson.M{"items": bson.M{"$each": inputs}}})
	must(t, err)
	storedRole := func() domain.Role { r, err := store.Role(ctx, roleID); must(t, err); return r }
	birthAudit := &summonBirthAudit{t: t, role: role.Key}
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), birthAudit.observe)
	defer func() { c.close() }()
	c.auto(false, 1)
	if c.scene.OwnedPet != nil || travelCount(c.scene, 2) != 0 {
		t.Fatal("none模式仍免费创建宠物，不能作为正常召唤前态")
	}
	var book *pb.ItemView
	for _, item := range c.inventory.Items {
		if item.ItemId == 25 && domain.Hex(item.InstanceId) == initialBookID {
			book = item
		}
	}
	if book == nil {
		t.Fatal("隔离创角未按本地配置给原书25")
	}
	learn := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: book.InstanceId, SkillId: 17, Operation: c.operation()})
	if learn.ErrorCode != 0 {
		t.Fatal("正常学习原书失败", learn)
	}
	c.inventoryAt(learn.NewRevision)
	equip := c.asset(p.MsgC2SSetCastingSupply, &pb.SetCastingSupplyRequest{ItemInstanceId: domain.Bytes(supply.ID), ItemVersion: 1, Operation: c.operation()})
	if equip.ErrorCode != 0 {
		t.Fatal("正常装符失败", equip)
	}
	c.inventoryAt(equip.NewRevision)
	beforeFresh := storedRole()
	mp := c.player().Mp
	request := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: 17, Direction: 2})
	travelExpectError(c, p.MsgC2SCastIntent, request, p.CodeSkillMaterialRequired)
	if c.scene.OwnedPet != nil || c.player().Mp != mp || storedRole().EconomicSeq != beforeFresh.EconomicSeq {
		t.Fatal("缺材料RPC的新生请求提前占宠、扣MP或消费经济序号")
	}
	material := summonWSEquipped(t, c)
	firstRequest := &pb.CastWithMaterialRequest{SceneEpoch: c.entry.SceneEpoch, CasterLife: c.player().Life, SkillId: 17, Direction: 2,
		MaterialInstanceId: material.InstanceId, MaterialVersion: material.Version, Operation: c.operation()}
	birthAudit.allow = true
	result, accepted := summonWSMaterial(c, firstRequest)
	if result.ErrorCode != 0 || accepted == nil || accepted.DelayMs != 500 {
		t.Fatal("真实扣耗未产生唯一500ms受理", result, accepted)
	}
	c.inventoryAt(result.NewRevision)
	if summonWSEquipped(t, c).Quantity != 1 {
		t.Fatal("新召没有恰好扣一个Count")
	}
	if len(birthAudit.events) == 0 {
		travelUntilWithin(c, 4*time.Second, func(wire.Frame) bool { return len(birthAudit.events) == 1 })
	}
	birthAudit.allow = false
	if len(birthAudit.events) != 1 || birthAudit.events[0].SourceCastId != accepted.CastId {
		t.Fatal("实际新生未产生唯一配对30015", birthAudit.events)
	}
	born := proto.Clone(c.scene.OwnedPet).(*pb.OwnedPetState)
	if c.player().Mp != mp-skill.MPBaseCost {
		t.Fatal("新召未按已学习0阶恰好扣一次MP", mp, c.player().Mp)
	}
	body := petRuntimeWSEntity(c.scene, born.EntityHandle)
	if born.MapId != 1 || born.Life == 0 || born.EntityHandle <= 2 || born.Hp <= 0 || born.PetTraining == nil || born.PetTraining.TrainingLevel != 0 || born.PetTraining.MaxTrainingLevel != 4 ||
		body == nil || body.Position.X != 9 || body.Position.Y != 8 || body.Direction != 5 || !reflect.DeepEqual(born.RecallSkillIds, []uint32{17}) {
		t.Fatal("真正出生未沿请求Front/源初向/独立训练，或缺Owned身份", born, body)
	}
	// 在不启用自动战斗的情况下，用普通方向攻击击杀与新宠叠格的真实怪物；Pet非最后一击也训练。
	time.Sleep(time.Duration(skill.GlobalSpellMS) * time.Millisecond)
	beforeKill := storedRole()
	c.send(p.MsgC2SAttackIntent, &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 2})
	credit := uint64(f.Monsters[0].Exp) * f.PetRuntime.Definitions[0].Training.ExperienceMultiplier
	travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.scene.GetOwnedPet().GetPetTraining().GetExperience() == credit
	})
	afterKill := storedRole()
	if afterKill.SourceSeq != beforeKill.SourceSeq+1 || afterKill.Experience != beforeKill.Experience+f.Monsters[0].Exp || afterKill.EconomicSeq != beforeKill.EconomicSeq ||
		c.scene.OwnedPet.EntityHandle != born.EntityHandle || c.scene.OwnedPet.Life != born.Life || c.scene.OwnedPet.PetTraining.TrainingLevel != 0 {
		t.Fatal("确认永久角色EXP后的原birth训练不一致", beforeKill.SourceSeq, afterKill.SourceSeq, c.scene.OwnedPet)
	}
	// 已有宠升级技能仍保留原出生上限；预览只说明后续新生，材料来自上方明确的隔离输入。
	previewRequest := &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: 17}
	for _, item := range c.inventory.Items {
		if item.ItemId == 25 || item.ItemId == upgrade.PageItemID {
			previewRequest.Materials = append(previewRequest.Materials, &pb.ItemConsumption{InstanceId: item.InstanceId, Version: item.Version, Quantity: item.Quantity})
		}
	}
	preview := c.skillPreview(previewRequest)
	wantKeys := []string{"mp_cost", "new_pet_training_level", "new_pet_max_training_level", "new_pet_material_quantity", "new_pet_hp_max"}
	keys := []string{}
	for _, stat := range preview.Stats {
		keys = append(keys, stat.Key)
	}
	if !reflect.DeepEqual(keys, wantKeys) || preview.Stats[0].BeforeMin != 12 || preview.Stats[0].AfterMin != 16 || preview.Stats[2].BeforeMin != 4 || preview.Stats[2].AfterMin != 5 {
		t.Fatal("真实17预览仍显示通用伤害或错误新生上限", preview)
	}
	rankResult := c.asset(p.MsgC2SSkillAssetCommit, &pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()})
	if rankResult.ErrorCode != 0 {
		t.Fatal("正常17升级提交失败", rankResult)
	}
	c.inventoryAt(rankResult.NewRevision)
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.scene.OwnedPet.EntityHandle != born.EntityHandle || c.scene.OwnedPet.PetTraining.MaxTrainingLevel != 4 || c.scene.OwnedPet.PetTraining.Experience != credit {
		t.Fatal("学习技能新阶错误改写已有宠出生训练上限", c.scene.OwnedPet)
	}
	material = summonWSEquipped(t, c)
	unequip := c.asset(p.MsgC2SSetCastingSupply, &pb.SetCastingSupplyRequest{ItemInstanceId: material.InstanceId, ItemVersion: material.Version, Unequip: true, Operation: c.operation()})
	if unequip.ErrorCode != 0 {
		t.Fatal("正常卸符失败", unequip)
	}
	c.inventoryAt(unequip.NewRevision)
	beforeRecall := storedRole()
	standing := proto.Clone(petRuntimeWSEntity(c.scene, born.EntityHandle)).(*pb.EntityState)
	time.Sleep(time.Duration(f.CombatTiming.PlayerAttackActionMS) * time.Millisecond)
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	recallMP, recallSequence := c.player().Mp, c.player().CastSequence
	recallIntent := &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: 17, Direction: 4}
	c.send(p.MsgC2SCastIntent, recallIntent)
	recalled := &pb.CastAccepted{}
	travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool {
		if f.ID != p.MsgS2CCastAccepted {
			return false
		}
		must(t, proto.Unmarshal(f.Payload, recalled))
		return recalled.SkillId == 17 && recalled.InputSequence == recallIntent.InputSequence
	})
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.player().CastSequence > recallSequence
	})
	wantRecallCost := skill.MPBaseCost + int64(recalled.SkillLevel)*skill.MPLevelCost
	if c.player().Mp != recallMP-wantRecallCost || recalled.SkillLevel != 1 {
		t.Fatal("无符Recall须使用升级后的MP/动作，而不是无成本施法", recallMP, c.player().Mp, recalled)
	}
	time.Sleep(time.Duration(recalled.DelayMs)*time.Millisecond + 100*time.Millisecond)
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	afterRecall := storedRole()
	if afterRecall.EconomicSeq != beforeRecall.EconomicSeq || !reflect.DeepEqual(afterRecall.Items, beforeRecall.Items) || !reflect.DeepEqual(afterRecall.Equipped, beforeRecall.Equipped) ||
		c.scene.OwnedPet.EntityHandle != born.EntityHandle || c.scene.OwnedPet.Life != born.Life || c.scene.OwnedPet.PetTraining.Experience != credit ||
		!proto.Equal(petRuntimeWSEntity(c.scene, born.EntityHandle).Position, standing.Position) {
		t.Fatal("无装符同图Recall扣耗、重生或擅自搬动原宠")
	}
	travelToBaseline(c, 1, 2)
	travelAcknowledge(c, true)
	if c.scene.OwnedPet == nil || c.scene.OwnedPet.MapId != 1 || !c.scene.OwnedPet.FollowSuppressed || travelCount(c.scene, 2) != 0 {
		t.Fatal("NoPets旅行搬入宠物或漏掉异图Owned观察", c.scene.OwnedPet)
	}
	// 通用CD在地图政策之前检查；先等合法冷却，才可将拒绝归因于NoPets。
	time.Sleep(time.Duration(skill.CooldownBaseMS) * time.Millisecond)
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	mp = c.player().Mp
	request = c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: 17})
	travelExpectError(c, p.MsgC2SCastIntent, request, p.CodeSkillStateBlocked)
	beforeReplay := storedRole()
	replayed, secondAccept := summonWSMaterial(c, proto.Clone(firstRequest).(*pb.CastWithMaterialRequest))
	if replayed.ErrorCode != 0 || secondAccept != nil {
		t.Fatal("历史成功回执应优先当前禁宠/材料门禁，且不能再次排Recall/新生", replayed, secondAccept)
	}
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.player().Mp < mp || c.scene.OwnedPet.EntityHandle != born.EntityHandle || c.scene.OwnedPet.Life != born.Life || !c.scene.OwnedPet.FollowSuppressed {
		t.Fatal("禁宠拒绝或历史回执在新图产生临时施放")
	}
	afterReplay := storedRole()
	if afterReplay.EconomicSeq != beforeReplay.EconomicSeq || afterReplay.SourceSeq != beforeReplay.SourceSeq || !reflect.DeepEqual(afterReplay.Items, beforeReplay.Items) {
		t.Fatal("重放旧请求改变资产/来源序号")
	}
	oldScene := c.entry.SceneEpoch
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), birthAudit.observe)
	if c.entry.SceneEpoch != oldScene || c.scene.OwnedPet == nil || c.scene.OwnedPet.MapId != 1 || c.scene.OwnedPet.Life != born.Life || c.scene.OwnedPet.PetTraining.Experience != credit {
		t.Fatal("健康新票丢失异图原宠/训练或重建世界")
	}
	travelToBaseline(c, 2, 1)
	travelAcknowledge(c, false)
	if c.scene.OwnedPet == nil || c.scene.OwnedPet.MapId != 1 || c.scene.OwnedPet.FollowSuppressed || petRuntimeWSEntity(c.scene, born.EntityHandle) == nil {
		t.Fatal("允许图恢复未解除抑制或重造宠物")
	}
	beforeCold := storedRole()
	c.close()
	stop()
	stop = launch()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), birthAudit.observe)
	if c.scene.OwnedPet != nil || travelCount(c.scene, 2) != 0 || c.entry.SceneEpoch == oldScene {
		t.Fatal("冷启动恢复了临时宠物/训练或沿用旧执行身份")
	}
	afterCold := storedRole()
	travelAssertEconomy(t, beforeCold, afterCold, "17冷建永久资产")
	if !reflect.DeepEqual(beforeCold.Skills, afterCold.Skills) || !reflect.DeepEqual(beforeCold.Equipped, afterCold.Equipped) || !reflect.DeepEqual(beforeCold.Items, afterCold.Items) || beforeCold.Experience != afterCold.Experience || beforeCold.SourceSeq != afterCold.SourceSeq {
		t.Fatal("冷建遗失或重发了已提交原书学习、Count及角色EXP")
	}
	if rank, learned := afterCold.Skill(17); !learned || rank != 1 {
		t.Fatal("冷建没有保留原书25正常学习及材料升级的17")
	}
	var document bson.M
	must(t, store.DB.Collection("roles").FindOne(ctx, bson.M{"_id": roleID}).Decode(&document))
	for _, key := range []string{"owned_pet", "pet", "pet_runtime", "pet_training", "pending_pet_birth", "pet_world", "position", "hp", "mp"} {
		if _, exists := document[key]; exists {
			t.Fatal("角色文档误存临时现场", key)
		}
	}
	var receipts []persistence.Operation
	cursor, err := store.DB.Collection("operations").Find(ctx, bson.M{"role_id": roleID, "kind": "cast_material"})
	must(t, err)
	must(t, cursor.All(ctx, &receipts))
	if len(receipts) != 1 || receipts[0].Casting == nil || receipts[0].Casting.Quantity != 1 || receipts[0].Casting.SkillID != 17 {
		t.Fatal("召唤扣耗未形成唯一typed材料回执", receipts)
	}
	if len(birthAudit.events) != 1 {
		t.Fatal("Recall/健康票/冷建补播新生事实", len(birthAudit.events))
	}
	t.Log("真实Mongo/WS通过：明确隔离19级/初始书25/两Count/HP1零伤怪/禁宠Town；正常学习装符→普通fresh拒绝→永久扣1→原Front叠格500ms出生→玩家击杀确认EXP训练→卸符后同图Recall零经济扣耗→NoPets异图Owned与旧回执零二次作用→健康保留/允许图恢复→冷建无宠且永久技能和耗材保留。不是自然购书购符、主包规格或正式17开放。")
}

// 放在统一readFrame之后观察，资产/旅行/Join内部until也不能吞掉出生事实。
// 仅保留当前一组、最多64块和2048实体；不把所有周期baseline累计到历史数组。
type summonBirthAudit struct {
	t                         *testing.T
	role                      *pb.RoleKey
	allow, ready              bool
	scene, baseline, sequence uint64
	count                     uint32
	parts                     map[uint32]*pb.SceneBaselineChunk
	cast                      uint64
	events                    []*pb.PetBorn
}

func (a *summonBirthAudit) observe(c *client, frame wire.Frame) {
	a.t.Helper()
	if frame.Error != 0 {
		return
	}
	switch frame.ID {
	case p.MsgS2CEnterLine:
		a.ready = false
		a.parts = nil
		a.sequence = 0
		a.scene = c.entry.SceneEpoch
		a.baseline = c.entry.BaselineId
	case p.MsgS2CSyncReady:
		q := &pb.SyncReady{}
		must(a.t, proto.Unmarshal(frame.Payload, q))
		if q.SceneEpoch == a.scene && q.BaselineId == a.baseline {
			a.ready = true
		}
	case p.MsgS2CCastAccepted:
		q := &pb.CastAccepted{}
		must(a.t, proto.Unmarshal(frame.Payload, q))
		if q.SkillId == 17 {
			a.cast = q.CastId
		}
	case p.MsgS2CSceneBaseline:
		q := &pb.SceneBaselineChunk{}
		must(a.t, proto.Unmarshal(frame.Payload, q))
		if q.SceneEpoch != a.scene || q.BaselineId != a.baseline {
			a.t.Fatal("出生观测收到旧附件基线")
		}
		if q.StateSequence != a.sequence {
			a.sequence = q.StateSequence
			a.count = q.ChunkCount
			a.parts = map[uint32]*pb.SceneBaselineChunk{}
		}
		if q.ChunkCount == 0 || q.ChunkCount > 64 || q.ChunkCount != a.count || q.ChunkIndex >= a.count || a.parts[q.ChunkIndex] != nil {
			a.t.Fatal("出生观测基线分组非法")
		}
		a.parts[q.ChunkIndex] = q
	case p.MsgS2CPetBorn:
		q := &pb.PetBorn{}
		must(a.t, proto.Unmarshal(frame.Payload, q))
		if !a.allow || !a.ready || len(a.events) != 0 {
			a.t.Fatal("未Ready/Recall/健康重入或重复30015", q)
		}
		if q.SceneEpoch != a.scene || q.BaselineId != a.baseline || q.StateSequence != a.sequence || len(a.parts) != int(a.count) || a.count == 0 ||
			frame.Binding != c.binding || q.MapId != c.entry.RoomId || q.SourceSkillId != 17 || q.SourceCastId != a.cast || q.BornAtUnixMs == 0 || !proto.Equal(q.Role, a.role) {
			a.t.Fatal("30015未配对先前完整基线/原附件/角色", q)
		}
		var body *pb.EntityState
		total := 0
		for _, part := range a.parts {
			total += len(part.Entities)
			for _, entity := range part.Entities {
				if entity.EntityHandle == q.EntityHandle {
					body = entity
				}
			}
		}
		if total > 2048 || body == nil || body.EntityKind != 2 || body.IsPetCorpse || body.Hp <= 0 || body.Life != q.EntityLife || body.DefinitionId != q.DefinitionId || body.MovementRevision != q.MovementRevision || body.Direction != q.Direction {
			a.t.Fatal("30015之前未完整应用精确出生身体", q, body)
		}
		a.events = append(a.events, proto.Clone(q).(*pb.PetBorn))
	}
}

func summonWSEquipped(t *testing.T, c *client) *pb.ItemView {
	t.Helper()
	for _, entry := range c.inventory.Equipped {
		if entry.Slot == devconfig.CastingSupplySlot {
			return entry.Item
		}
	}
	t.Fatal("没有当前装备材料观察")
	return nil
}

func summonWSMaterial(c *client, request *pb.CastWithMaterialRequest) (*pb.OperationResult, *pb.CastAccepted) {
	c.t.Helper()
	id := c.send(p.MsgC2SCastWithMaterial, request)
	var accepted *pb.CastAccepted
	result := &pb.OperationResult{}
	travelUntilWithin(c, 4*time.Second, func(f wire.Frame) bool {
		if f.ID == p.MsgS2CCastAccepted {
			q := &pb.CastAccepted{}
			must(c.t, proto.Unmarshal(f.Payload, q))
			if q.SkillId == 17 {
				accepted = q
			}
		}
		if f.ID != p.MsgS2COperationResult || f.Request != id {
			return false
		}
		must(c.t, proto.Unmarshal(f.Payload, result))
		if result.CommandSeq > c.watermark {
			c.watermark = result.CommandSeq
		}
		return true
	})
	return result, accepted
}
