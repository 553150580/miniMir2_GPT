// 验证召唤耗材回执、召回与训练、禁止召唤场景以及冷世界恢复约束。

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
// TestSummonPetMaterialRecallTrainingNoPetsAndColdMongoWS 跟踪召唤耗材及宠物生命周期，在禁宠场景和冷入场复核边界。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonPetMaterialRecallTrainingNoPetsAndColdMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启专属Mongo副本集，验证17真实协议隔离链路")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	// catalog 保存由加载召唤候选集合取得的目录。
	catalog, err := devconfig.LoadSummonCandidates("../../../config/development/summon-candidates.json")
	must(t, err)
	if cfg.Fixture.RuleVersion < 23 || cfg.Fixture.CastingSupply.Enabled {
		t.Fatal("必须从主rule21关闭材料的真实配置创建隔离样本；独立召唤候选仍required17")
	}
	// enabled 保存由技能取得的已开启。
	if _, enabled := cfg.Fixture.Skill(17); enabled {
		t.Fatal("主配置不能因候选测试被开启17")
	}
	// f 保存当前处理所用的开发测试配置。
	f := &cfg.Fixture
	f.Skills, f.Items = append(f.Skills, catalog.Skills...), append(f.Items, catalog.Books...)
	f.SkillUpgrades = append(f.SkillUpgrades, catalog.Upgrades...)
	f.CastingSupply.Enabled, f.PetRuntime.BootstrapMode = true, "none"
	// i 标记遍历道具集合时访问的下标。
	for i := range f.Items {
		if f.Items[i].ID == 187 {
			f.Items[i].MaxStack = 2 // 明确的隔离Count容量，不是DuraMax、售价或购包单位。
		}
	}
	// i 标记遍历初始值技能技能书集合时访问的下标。
	for i := range f.InitialSkillBooks {
		if f.InitialSkillBooks[i].JobID == 2 {
			f.InitialSkillBooks[i].ItemID = 25
		}
	}
	// i 标记遍历地图集合时访问的下标。
	for i := range f.Maps {
		if f.Maps[i].ID == 3 {
			// blocked 保存当前处理所用的阻挡。
			blocked := true
			f.Maps[i].NoPets = &blocked
		}
	}
	f.Map.MonsterSpawns[0] = devconfig.Point{X: 9, Y: 8}
	// i 标记遍历怪物集合时访问的下标。
	for i := range f.Monsters {
		// m 保存当前处理所用的怪物集合中的指定项。
		m := &f.Monsters[i]
		m.HP, m.AttackMin, m.AttackMax, m.Agility = 1, 0, 0, 0
		m.AttackMS, m.MoveMS = 600000, 600000
	}
	must(t, f.Validate())
	// skill 保存从技能集合取出的技能。
	skill := catalog.Skills[0]
	cfg.Database = "minimir_dev_game_test_summon_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// ctx 保存由“background”取得的上下文。
	ctx := context.Background()
	// store 保存由打开取得的持久层存储。
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	// launch 封装启动对应的后续处理。
	launch := func() func() {
		// run 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		run, cancel := context.WithCancel(context.Background())
		// group 记录分组，使用协程汇合计数器表示。
		var group sync.WaitGroup
		// mode 接收遍历文本列表时当前的元素。
		for _, mode := range []string{"account", "game", "gm"} {
			group.Go(func() {
				// err 接收运行批次过程中可能产生的错误，供后续失败分支处理。
				if err := bootstrap.Run(run, cfg, mode); err != nil && run.Err() == nil {
					t.Errorf("召唤%s节点: %v", mode, err)
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
		must(t, store.DB.Drop(cleanup))
		store.Close(cleanup)
	})
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录，role 保存当前处理所用的角色。
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "summon_" + domain.ID()[:9], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 2, Name: "召唤" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	// roleID 标识当前业务使用的角色。
	roleID := domain.Hex(role.Key.RoleId)
	// createdRole 保存由角色取得的“created”角色。
	createdRole, err := store.Role(ctx, roleID)
	must(t, err)
	// initialBookID 标识当前业务使用的初始值技能书。
	initialBookID := ""
	// item 接收遍历道具集合时当前的元素。
	for _, item := range createdRole.Items {
		if item.ItemID == 25 {
			initialBookID = item.ID
			break
		}
	}
	if initialBookID == "" {
		t.Fatal("正常创角没有给本地初始书25")
	}
	// supply 建立带初始值的道具，供本段继续处理。
	supply := domain.Item{ID: domain.ID(), ItemID: 187, Quantity: 2, Bound: true, Version: 1, SourceKind: "development_fixture", SourceID: "two_explicit_count_units"}
	// inputs 建立带初始值的道具列表，供本段继续处理。
	inputs := []domain.Item{supply}
	// upgrade 保存从“upgrades”取出的升级。
	upgrade := catalog.Upgrades[0]
	// n 接收整数“64”供后续步骤使用。
	for n := int64(0); n < upgrade.BookCosts[0].Quantity; n++ {
		inputs = append(inputs, domain.Item{ID: domain.ID(), ItemID: 25, Quantity: 1, Bound: true, Version: 1, Quality: 1, SourceKind: "development_fixture", SourceID: "explicit_upgrade_inputs"})
	}
	inputs = append(inputs, domain.Item{ID: domain.ID(), ItemID: upgrade.PageItemID, Quantity: upgrade.PageCost, Bound: true, Version: 1, SourceKind: "development_fixture", SourceID: "explicit_upgrade_inputs"})
	_, err = store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"level": 19}, "$push": bson.M{"items": bson.M{"$each": inputs}}})
	must(t, err)
	// storedRole 封装已持久保存角色对应的后续处理，r 接收角色供后续步骤使用，err 接收角色过程中可能产生的错误，供后续失败分支处理。
	storedRole := func() domain.Role { r, err := store.Role(ctx, roleID); must(t, err); return r }
	// birthAudit 保存当前处理所用的出生审计记录。
	birthAudit := &summonBirthAudit{t: t, role: role.Key}
	// c 接收加入供后续步骤使用。
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), birthAudit.observe)
	defer func() { c.close() }()
	c.auto(false, 1)
	if c.scene.OwnedPet != nil || travelCount(c.scene, 2) != 0 {
		t.Fatal("none模式仍免费创建宠物，不能作为正常召唤前态")
	}
	// book 记录技能书，使用道具观察视图表示。
	var book *pb.ItemView
	// item 接收遍历道具集合时当前的元素。
	for _, item := range c.inventory.Items {
		if item.ItemId == 25 && domain.Hex(item.InstanceId) == initialBookID {
			book = item
		}
	}
	if book == nil {
		t.Fatal("隔离创角未按本地配置给原书25")
	}
	// learn 保存由资产取得的学习。
	learn := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: book.InstanceId, SkillId: 17, Operation: c.operation()})
	if learn.ErrorCode != 0 {
		t.Fatal("正常学习原书失败", learn)
	}
	c.inventoryAt(learn.NewRevision)
	// equip 保存由资产取得的穿戴装备。
	equip := c.asset(p.MsgC2SSetCastingSupply, &pb.SetCastingSupplyRequest{ItemInstanceId: domain.Bytes(supply.ID), ItemVersion: 1, Operation: c.operation()})
	if equip.ErrorCode != 0 {
		t.Fatal("正常装符失败", equip)
	}
	c.inventoryAt(equip.NewRevision)
	// beforeFresh 保存由已持久保存角色取得的操作前的状态首次装载。
	beforeFresh := storedRole()
	// mp 保存当前处理所用的魔法值。
	mp := c.player().Mp
	// request 保存由发送取得的请求。
	request := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: 17, Direction: 2})
	travelExpectError(c, p.MsgC2SCastIntent, request, p.CodeSkillMaterialRequired)
	if c.scene.OwnedPet != nil || c.player().Mp != mp || storedRole().EconomicSeq != beforeFresh.EconomicSeq {
		t.Fatal("缺材料RPC的新生请求提前占宠、扣MP或消费经济序号")
	}
	// material 保存由召唤WebSocket已穿戴装备取得的材料。
	material := summonWSEquipped(t, c)
	// firstRequest 保存当前处理所用的第一项请求。
	firstRequest := &pb.CastWithMaterialRequest{SceneEpoch: c.entry.SceneEpoch, CasterLife: c.player().Life, SkillId: 17, Direction: 2,
		MaterialInstanceId: material.InstanceId, MaterialVersion: material.Version, Operation: c.operation()}
	birthAudit.allow = true
	// result 保存由召唤WebSocket材料取得的结果，accepted 保存由召唤WebSocket材料取得的已受理。
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
	// born 按目标类型读取当前接口值，得到“born”。
	born := proto.Clone(c.scene.OwnedPet).(*pb.OwnedPetState)
	if c.player().Mp != mp-skill.MPBaseCost {
		t.Fatal("新召未按已学习0阶恰好扣一次MP", mp, c.player().Mp)
	}
	// body 保存由宠物运行时WebSocket实体取得的消息正文。
	body := petRuntimeWSEntity(c.scene, born.EntityHandle)
	if born.MapId != 1 || born.Life == 0 || born.EntityHandle <= 2 || born.Hp <= 0 || born.PetTraining == nil || born.PetTraining.TrainingLevel != 0 || born.PetTraining.MaxTrainingLevel != 4 ||
		body == nil || body.Position.X != 9 || body.Position.Y != 8 || body.Direction != 5 || !reflect.DeepEqual(born.RecallSkillIds, []uint32{17}) {
		t.Fatal("真正出生未沿请求Front/源初向/独立训练，或缺Owned身份", born, body)
	}
	// 在不启用自动战斗的情况下，用普通方向攻击击杀与新宠叠格的真实怪物；Pet非最后一击也训练。
	time.Sleep(time.Duration(skill.GlobalSpellMS) * time.Millisecond)
	// beforeKill 保存由已持久保存角色取得的操作前的状态击杀。
	beforeKill := storedRole()
	c.send(p.MsgC2SAttackIntent, &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, Direction: 2})
	// credit 保存根据无符号整数“64”与经验倍率计算的入账。
	credit := uint64(f.Monsters[0].Exp) * f.PetRuntime.Definitions[0].Training.ExperienceMultiplier
	// f 记录协议帧。
	travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.scene.GetOwnedPet().GetPetTraining().GetExperience() == credit
	})
	// afterKill 保存由已持久保存角色取得的操作后的状态击杀。
	afterKill := storedRole()
	if afterKill.SourceSeq != beforeKill.SourceSeq+1 || afterKill.Experience != beforeKill.Experience+f.Monsters[0].Exp || afterKill.EconomicSeq != beforeKill.EconomicSeq ||
		c.scene.OwnedPet.EntityHandle != born.EntityHandle || c.scene.OwnedPet.Life != born.Life || c.scene.OwnedPet.PetTraining.TrainingLevel != 0 {
		t.Fatal("确认永久角色EXP后的原birth训练不一致", beforeKill.SourceSeq, afterKill.SourceSeq, c.scene.OwnedPet)
	}
	// 已有宠升级技能仍保留原出生上限；预览只说明后续新生，材料来自上方明确的隔离输入。
	// previewRequest 保存当前处理所用的预览请求。
	previewRequest := &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: 17}
	// item 接收遍历道具集合时当前的元素。
	for _, item := range c.inventory.Items {
		if item.ItemId == 25 || item.ItemId == upgrade.PageItemID {
			previewRequest.Materials = append(previewRequest.Materials, &pb.ItemConsumption{InstanceId: item.InstanceId, Version: item.Version, Quantity: item.Quantity})
		}
	}
	// preview 保存由技能预览取得的预览。
	preview := c.skillPreview(previewRequest)
	// wantKeys 初始化用作期望值键集合的文本列表。
	wantKeys := []string{"mp_cost", "new_pet_training_level", "new_pet_max_training_level", "new_pet_material_quantity", "new_pet_hp_max"}
	// keys 初始化用作键集合的文本列表。
	keys := []string{}
	// stat 接收遍历统计或属性时当前的元素。
	for _, stat := range preview.Stats {
		keys = append(keys, stat.Key)
	}
	if !reflect.DeepEqual(keys, wantKeys) || preview.Stats[0].BeforeMin != 12 || preview.Stats[0].AfterMin != 16 || preview.Stats[2].BeforeMin != 4 || preview.Stats[2].AfterMin != 5 {
		t.Fatal("真实17预览仍显示通用伤害或错误新生上限", preview)
	}
	// rankResult 保存由资产取得的排序位次结果。
	rankResult := c.asset(p.MsgC2SSkillAssetCommit, &pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()})
	if rankResult.ErrorCode != 0 {
		t.Fatal("正常17升级提交失败", rankResult)
	}
	c.inventoryAt(rankResult.NewRevision)
	// f 记录协议帧。
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.scene.OwnedPet.EntityHandle != born.EntityHandle || c.scene.OwnedPet.PetTraining.MaxTrainingLevel != 4 || c.scene.OwnedPet.PetTraining.Experience != credit {
		t.Fatal("学习技能新阶错误改写已有宠出生训练上限", c.scene.OwnedPet)
	}
	material = summonWSEquipped(t, c)
	// unequip 保存由资产取得的“unequip”。
	unequip := c.asset(p.MsgC2SSetCastingSupply, &pb.SetCastingSupplyRequest{ItemInstanceId: material.InstanceId, ItemVersion: material.Version, Unequip: true, Operation: c.operation()})
	if unequip.ErrorCode != 0 {
		t.Fatal("正常卸符失败", unequip)
	}
	c.inventoryAt(unequip.NewRevision)
	// beforeRecall 保存由已持久保存角色取得的操作前的状态召回。
	beforeRecall := storedRole()
	// standing 按目标类型读取当前接口值，得到“standing”。
	standing := proto.Clone(petRuntimeWSEntity(c.scene, born.EntityHandle)).(*pb.EntityState)
	time.Sleep(time.Duration(f.CombatTiming.PlayerAttackActionMS) * time.Millisecond)
	// f 记录协议帧。
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	// recallMP 保存魔法值，用于当前步骤中的召回魔法值，recallSequence 保存施法序号，用于当前步骤中的召回序号。
	recallMP, recallSequence := c.player().Mp, c.player().CastSequence
	// recallIntent 保存当前处理所用的召回意图。
	recallIntent := &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: 17, Direction: 4}
	c.send(p.MsgC2SCastIntent, recallIntent)
	// recalled 保存当前处理所用的“recalled”。
	recalled := &pb.CastAccepted{}
	// f 记录协议帧。
	travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool {
		if f.ID != p.MsgS2CCastAccepted {
			return false
		}
		must(t, proto.Unmarshal(f.Payload, recalled))
		return recalled.SkillId == 17 && recalled.InputSequence == recallIntent.InputSequence
	})
	// f 记录协议帧。
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.player().CastSequence > recallSequence
	})
	// wantRecallCost 保存根据魔法值基础值消耗与整数“64”与魔法值等级消耗的计算结果计算的期望值召回消耗。
	wantRecallCost := skill.MPBaseCost + int64(recalled.SkillLevel)*skill.MPLevelCost
	if c.player().Mp != recallMP-wantRecallCost || recalled.SkillLevel != 1 {
		t.Fatal("无符Recall须使用升级后的MP/动作，而不是无成本施法", recallMP, c.player().Mp, recalled)
	}
	time.Sleep(time.Duration(recalled.DelayMs)*time.Millisecond + 100*time.Millisecond)
	// f 记录协议帧。
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	// afterRecall 保存由已持久保存角色取得的操作后的状态召回。
	afterRecall := storedRole()
	if afterRecall.EconomicSeq != beforeRecall.EconomicSeq || !reflect.DeepEqual(afterRecall.Items, beforeRecall.Items) || !reflect.DeepEqual(afterRecall.Equipped, beforeRecall.Equipped) ||
		c.scene.OwnedPet.EntityHandle != born.EntityHandle || c.scene.OwnedPet.Life != born.Life || c.scene.OwnedPet.PetTraining.Experience != credit ||
		!proto.Equal(petRuntimeWSEntity(c.scene, born.EntityHandle).Position, standing.Position) {
		t.Fatal("无装符同图Recall扣耗、重生或擅自搬动原宠")
	}
	travelToBaseline(c, 1, 3)
	travelAcknowledge(c, true)
	if c.scene.OwnedPet == nil || c.scene.OwnedPet.MapId != 1 || !c.scene.OwnedPet.FollowSuppressed || travelCount(c.scene, 2) != 0 {
		t.Fatal("NoPets旅行搬入宠物或漏掉异图Owned观察", c.scene.OwnedPet)
	}
	// 通用CD在地图政策之前检查；先等合法冷却，才可将拒绝归因于NoPets。
	time.Sleep(time.Duration(skill.CooldownBaseMS) * time.Millisecond)
	// f 记录协议帧。
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	mp = c.player().Mp
	request = c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: 17})
	travelExpectError(c, p.MsgC2SCastIntent, request, p.CodeSkillStateBlocked)
	// beforeReplay 保存由已持久保存角色取得的操作前的状态重放。
	beforeReplay := storedRole()
	// replayed 保存由召唤WebSocket材料取得的“replayed”，secondAccept 保存由召唤WebSocket材料取得的秒“accept”。
	replayed, secondAccept := summonWSMaterial(c, proto.Clone(firstRequest).(*pb.CastWithMaterialRequest))
	if replayed.ErrorCode != 0 || secondAccept != nil {
		t.Fatal("历史成功回执应优先当前禁宠/材料门禁，且不能再次排Recall/新生", replayed, secondAccept)
	}
	// f 记录协议帧。
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if c.player().Mp < mp || c.scene.OwnedPet.EntityHandle != born.EntityHandle || c.scene.OwnedPet.Life != born.Life || !c.scene.OwnedPet.FollowSuppressed {
		t.Fatal("禁宠拒绝或历史回执在新图产生临时施放")
	}
	// afterReplay 保存由已持久保存角色取得的操作后的状态重放。
	afterReplay := storedRole()
	if afterReplay.EconomicSeq != beforeReplay.EconomicSeq || afterReplay.SourceSeq != beforeReplay.SourceSeq || !reflect.DeepEqual(afterReplay.Items, beforeReplay.Items) {
		t.Fatal("重放旧请求改变资产/来源序号")
	}
	// oldScene 保存场景代次，用于当前步骤中的原状态场景。
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
	// beforeCold 保存由已持久保存角色取得的操作前的状态“cold”。
	beforeCold := storedRole()
	c.close()
	stop()
	stop = launch()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), birthAudit.observe)
	if c.scene.OwnedPet != nil || travelCount(c.scene, 2) != 0 || c.entry.SceneEpoch == oldScene {
		t.Fatal("冷启动恢复了临时宠物/训练或沿用旧执行身份")
	}
	// afterCold 保存由已持久保存角色取得的操作后的状态“cold”。
	afterCold := storedRole()
	travelAssertEconomy(t, beforeCold, afterCold, "17冷建永久资产")
	if !reflect.DeepEqual(beforeCold.Skills, afterCold.Skills) || !reflect.DeepEqual(beforeCold.Equipped, afterCold.Equipped) || !reflect.DeepEqual(beforeCold.Items, afterCold.Items) || beforeCold.Experience != afterCold.Experience || beforeCold.SourceSeq != afterCold.SourceSeq {
		t.Fatal("冷建遗失或重发了已提交原书学习、Count及角色EXP")
	}
	// rank 保存由技能取得的排序位次，learned 保存由技能取得的已学习。
	if rank, learned := afterCold.Skill(17); !learned || rank != 1 {
		t.Fatal("冷建没有保留原书25正常学习及材料升级的17")
	}
	// document 记录文档，使用MongoDB BSON 文档表示。
	var document bson.M
	must(t, store.DB.Collection("roles").FindOne(ctx, bson.M{"_id": roleID}).Decode(&document))
	// key 接收遍历文本列表时当前的元素。
	for _, key := range []string{"owned_pet", "pet", "pet_runtime", "pet_training", "pending_pet_birth", "pet_world", "position", "hp", "mp"} {
		// exists 保存从文档取出的存在。
		if _, exists := document[key]; exists {
			t.Fatal("角色文档误存临时现场", key)
		}
	}
	// receipts 记录回执集合，使用操作列表表示。
	var receipts []persistence.Operation
	// cursor 保存由查找取得的游标。
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
	// t 提供断言失败报告、子用例和清理注册。
	t *testing.T
	// role 记录角色，使用角色键表示。
	role *pb.RoleKey
	// allow 标记是否处于“allow”状态。
	// ready 标记是否处于就绪状态。
	allow, ready bool
	// scene 记录场景，使用数值表示。
	// baseline 记录场景基线，使用数值表示。
	// sequence 记录序号，使用数值表示。
	scene, baseline, sequence uint64
	// count 记录数量，使用数值表示。
	count uint32
	// parts 记录分段集合，使用按数值索引的场景场景基线分片映射表示。
	parts map[uint32]*pb.SceneBaselineChunk
	// cast 记录施法，使用数值表示。
	cast uint64
	// events 记录事件集合，使用宠物出生列表表示。
	events []*pb.PetBorn
}

// observe 处理观测。
// a 是调用该方法的召唤出生审计记录实例，c 记录客户端，frame 记录协议帧。
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
		// q 保存当前处理所用的场景同步就绪。
		q := &pb.SyncReady{}
		must(a.t, proto.Unmarshal(frame.Payload, q))
		if q.SceneEpoch == a.scene && q.BaselineId == a.baseline {
			a.ready = true
		}
	case p.MsgS2CCastAccepted:
		// q 保存当前处理所用的施法已受理。
		q := &pb.CastAccepted{}
		must(a.t, proto.Unmarshal(frame.Payload, q))
		if q.SkillId == 17 {
			a.cast = q.CastId
		}
	case p.MsgS2CSceneBaseline:
		// q 保存当前处理所用的场景场景基线分片。
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
		// q 保存当前处理所用的宠物出生。
		q := &pb.PetBorn{}
		must(a.t, proto.Unmarshal(frame.Payload, q))
		if !a.allow || !a.ready || len(a.events) != 0 {
			a.t.Fatal("未Ready/Recall/健康重入或重复30015", q)
		}
		if q.SceneEpoch != a.scene || q.BaselineId != a.baseline || q.StateSequence != a.sequence || len(a.parts) != int(a.count) || a.count == 0 ||
			frame.Binding != c.binding || q.MapId != c.entry.RoomId || q.SourceSkillId != 17 || q.SourceCastId != a.cast || q.BornAtUnixMs == 0 || !proto.Equal(q.Role, a.role) {
			a.t.Fatal("30015未配对先前完整基线/原附件/角色", q)
		}
		// body 记录消息正文，使用实体状态表示。
		var body *pb.EntityState
		// total 规定合计的初始取值。
		total := 0
		// part 接收遍历分段集合时当前的元素。
		for _, part := range a.parts {
			total += len(part.Entities)
			// entity 接收遍历实体时当前的元素。
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

// summonWSEquipped 返回召唤WebSocket已穿戴装备供调用方使用。
// t 提供断言失败报告、子用例和清理注册，c 记录客户端。
func summonWSEquipped(t *testing.T, c *client) *pb.ItemView {
	t.Helper()
	// entry 接收遍历已穿戴装备时当前的元素。
	for _, entry := range c.inventory.Equipped {
		if entry.Slot == devconfig.CastingSupplySlot {
			return entry.Item
		}
	}
	t.Fatal("没有当前装备材料观察")
	return nil
}

// summonWSMaterial 处理召唤WebSocket材料。
// c 记录客户端，request 记录请求，使用施法带有材料请求表示。
func summonWSMaterial(c *client, request *pb.CastWithMaterialRequest) (*pb.OperationResult, *pb.CastAccepted) {
	c.t.Helper()
	// id 区分当前步骤指向的目标对象。
	id := c.send(p.MsgC2SCastWithMaterial, request)
	// accepted 记录已受理，使用施法已受理表示。
	var accepted *pb.CastAccepted
	// result 保存当前处理所用的结果。
	result := &pb.OperationResult{}
	// f 记录协议帧。
	travelUntilWithin(c, 4*time.Second, func(f wire.Frame) bool {
		if f.ID == p.MsgS2CCastAccepted {
			// q 保存当前处理所用的施法已受理。
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
