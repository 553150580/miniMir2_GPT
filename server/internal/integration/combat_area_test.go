// 验证多个范围技能的学习、攻击覆盖、命中和击杀收益，使用真实入场与战斗消息。

package integration

import (
	"context"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"os"
	"sync"
	"testing"
	"time"
)

// buyAndLearnSkill 处理“buy”“and”学习技能。
// c 是调用该方法的客户端实例，f 记录开发测试配置，id 区分当前步骤指向的目标对象。
func (c *client) buyAndLearnSkill(f devconfig.Fixture, id uint32) {
	c.t.Helper()
	// s 接收技能供后续步骤使用，exists 保存由技能取得的存在。
	s, exists := f.Skill(id)
	if !exists {
		c.t.Fatal("缺少学习候选")
	}
	// offer 保存由无符号整数“32”取得的尝试提交。
	offer := uint32(0)
	// o 接收遍历技能商店时当前的元素。
	for _, o := range f.SkillShop {
		if o.ItemID == s.BookItemID && o.Currency == "gold" {
			offer = o.ID
		}
	}
	// preview 保存由技能预览取得的预览。
	preview := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer, Quantity: 1})
	// r 接收技能确认供后续步骤使用。
	if r := c.skillConfirm(preview); r.ErrorCode != 0 {
		c.t.Fatal("新技能购书失败")
	}
	// book 记录技能书，使用道具观察视图表示。
	var book *pb.ItemView
	// item 接收遍历道具集合时当前的元素。
	for _, item := range c.inventory.Items {
		if item.ItemId == s.BookItemID && item.SourceKind == "npc_shop" {
			book = item
			break
		}
	}
	if book == nil {
		c.t.Fatal("NPC没有真实交付技能书")
	}
	// result 保存由资产取得的结果。
	result := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: book.InstanceId, SkillId: id, Operation: c.operation()})
	c.inventoryAt(result.NewRevision)
	if result.ErrorCode != 0 {
		c.t.Fatalf("技能%d学习失败:%v", id, result)
	}
	// found 保存当前处理所用的已找到标记。
	found := false
	// learned 接收遍历已学习技能集合时当前的元素。
	for _, learned := range c.inventory.LearnedSkills {
		if learned.SkillId == id && learned.Level == s.LearnedLevel {
			found = true
		}
	}
	if !found {
		c.t.Fatal("已学零级没有同步")
	}
}

// castAccepted 返回施法已受理供调用方使用。
// c 是调用该方法的客户端实例，id 区分当前步骤指向的目标对象，target 记录目标，使用数值表示，grid 记录网格，使用网格点位表示。
func (c *client) castAccepted(id, target uint32, grid *pb.GridPoint) *pb.CastAccepted {
	c.t.Helper()
	c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: id, TargetHandle: target, TargetGrid: grid})
	// event 保存当前处理所用的事件。
	event := &pb.CastAccepted{}
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool {
		if f.ID != p.MsgS2CCastAccepted {
			return false
		}
		must(c.t, proto.Unmarshal(f.Payload, event))
		return event.SkillId == id
	})
	return event
}

// combatImpact 返回战斗命中影响供调用方使用。
// c 是调用该方法的客户端实例，id 区分当前步骤指向的目标对象，target 记录目标，使用数值表示。
func (c *client) combatImpact(id, target uint32) *pb.CombatImpact {
	c.t.Helper()
	// out 保存当前处理所用的输出。
	out := &pb.CombatImpact{}
	// f 记录协议帧。
	c.until(func(f wire.Frame) bool {
		if f.ID != p.MsgS2CCombatImpact {
			return false
		}
		must(c.t, proto.Unmarshal(f.Payload, out))
		return out.SkillId == id && out.TargetHandle == target
	})
	return out
}

// TestCombatAreaFiveSkillsAndEightKillMongoWS 覆盖五种范围能力的真实命中，核对击杀与持久收益。
// t 提供断言失败报告、子用例和清理注册。
func TestCombatAreaFiveSkillsAndEightKillMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以验证8技能与多杀真实流水")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// testRoleLevel 规定测试角色等级的初始取值。
	const testRoleLevel uint32 = 36
	// 仅隔离用例使用近距离弱目标和可受伤宝宝，确定性观察区域命中/治疗；不改唯一Excel。
	cfg.Fixture.Monsters[0].HP = 5
	cfg.Fixture.Monsters[0].AttackMin = cfg.Fixture.Healing.PetBasePerTick * 2
	cfg.Fixture.Monsters[0].AttackMax = cfg.Fixture.Monsters[0].AttackMin
	cfg.Fixture.Monsters[0].Accuracy = cfg.Fixture.Pet.Agility
	cfg.Fixture.Pet.DefenceMin, cfg.Fixture.Pet.DefenceMax = 0, 0
	cfg.Fixture.Pet.AttackMin, cfg.Fixture.Pet.AttackMax = 0, 0
	// base 保存由玩家发生时间等级取得的基础值。
	base, _ := cfg.Fixture.PlayerAtLevel(1, testRoleLevel)
	cfg.Fixture.Monsters[0].DefenceMin, cfg.Fixture.Monsters[0].DefenceMax = base.AttackMax, base.AttackMax
	// 首怪与道士、宠物等距，优先攻击道士；开启七怪后右边的怪离宠物更近。
	// 第八槽留在宠物初始格，群疗不启用该槽。无宠法师则可绕过首怪走到八怪中心。
	cfg.Fixture.Map.PlayerSpawn = devconfig.Point{X: 5, Y: 6}
	cfg.Fixture.Map.PetSpawn = devconfig.Point{X: 6, Y: 6}
	cfg.Fixture.Map.MonsterSpawns = []devconfig.Point{{X: 6, Y: 7}, {X: 6, Y: 8}, {X: 7, Y: 6}, {X: 7, Y: 8}, {X: 8, Y: 6}, {X: 8, Y: 7}, {X: 8, Y: 8}, {X: 6, Y: 6}}
	// injuryMonsterCount 保存由无符号整数“32”取得的“injury”怪物数量。
	injuryMonsterCount := uint32(len(cfg.Fixture.Map.MonsterSpawns) - 1)
	// moveWaitMS 保存根据移动毫秒与当前数据计算的移动等待毫秒。
	moveWaitMS := cfg.Fixture.Players[0].MoveMS + 100 // 沿用本例已有的动作结束观察余量。
	// massHealing 保存由技能取得的“mass”治疗，ok 表明查询、校验或类型转换是否成功。
	massHealing, ok := cfg.Fixture.Skill(29)
	if !ok {
		t.Fatal("缺少群疗唯一配置")
	}
	// taoStats 保存由玩家发生时间等级取得的“tao”统计或属性。
	taoStats, ok := cfg.Fixture.PlayerAtLevel(2, testRoleLevel)
	if !ok {
		t.Fatal("缺少道士成长配置")
	}
	// maximumRoll 封装最大值随机值对应的后续处理，base 记录基础值，使用数值表示，bonus 记录加成，使用数值表示。
	maximumRoll := func(base, bonus int64) int64 {
		if bonus > 0 {
			return base + bonus - 1
		}
		return base
	}
	// maximumQueue 保存由来源伤害取得的最大值队列。
	maximumQueue := massHealing.SourceDamage(massHealing.LearnedLevel, taoStats.TaoMax*massHealing.HealingStatMultiplier, massHealing.SourcePower(massHealing.LearnedLevel, maximumRoll(massHealing.MPowerBase, massHealing.MPowerBonus), maximumRoll(massHealing.PowerBase, massHealing.PowerBonus)))
	if massHealing.AddCasterLevel {
		maximumQueue += int64(testRoleLevel)
	}
	if maximumQueue > cfg.Fixture.Healing.QueueCap {
		maximumQueue = cfg.Fixture.Healing.QueueCap
	}
	// ceil 封装“ceil”对应的后续处理，amount 记录数量，使用数值表示，unit 记录单位，使用数值表示。
	ceil := func(amount, unit int64) int64 {
		if amount <= 0 || unit <= 0 {
			t.Fatal("群疗隔离前态预算缺少正数配置")
		}
		return (amount + unit - 1) / unit
	}
	// recoveryTicks 保存由“ceil”取得的恢复模拟帧集合。
	recoveryTicks := ceil(maximumQueue, cfg.Fixture.Healing.PetBasePerTick)
	// attackPeriodMS 保存攻击毫秒，用于当前步骤中的攻击周期毫秒。
	attackPeriodMS := cfg.Fixture.Monsters[0].AttackMS
	// 只增加本例宠物耐久：动作等待、投射生效、完整恢复跳数，再留一个原怪攻击周期。
	// observationMS 保存根据移动等待毫秒与“delay”基础值毫秒的计算结果与恢复模拟帧集合与模拟帧毫秒的计算结果的计算结果与攻击周期毫秒计算的“observation”毫秒。
	observationMS := moveWaitMS + massHealing.DelayBaseMS + recoveryTicks*cfg.Fixture.Healing.TickMS + attackPeriodMS
	// maximumBursts 保存根据当前数据与“ceil”计算的最大值“bursts”。
	maximumBursts := 1 + ceil(observationMS, attackPeriodMS)
	// basePetHP 保存生命值，用于当前步骤中的基础值宠物生命值。
	basePetHP := cfg.Fixture.Pet.HP
	// petHeadroom 保存根据整数“64”与攻击最大的计算结果与最大值“bursts”计算的宠物“headroom”。
	petHeadroom := int64(injuryMonsterCount) * cfg.Fixture.Monsters[0].AttackMax * maximumBursts
	cfg.Fixture.Pet.HP += petHeadroom
	t.Logf("群疗隔离生存预算：wait=%d delay=%d maxQueue=%d petTick=%d×%d attackPeriod=%d window=%d bursts=%d damage=%d×%d×%d=%d，PetHP=%d+%d=%d（仅测试实例）", moveWaitMS, massHealing.DelayBaseMS, maximumQueue, recoveryTicks, cfg.Fixture.Healing.TickMS, attackPeriodMS, observationMS, maximumBursts, injuryMonsterCount, cfg.Fixture.Monsters[0].AttackMax, maximumBursts, petHeadroom, basePetHP, petHeadroom, cfg.Fixture.Pet.HP)
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
			// err 接收运行批次过程中可能产生的错误，供后续失败分支处理。
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("区域测试%s:%v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	// a 接收遍历文本列表时当前的元素。
	for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, a)
	}
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "area_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	// create 封装创建对应的后续处理，slot 记录槽位，使用数值表示，job 记录任务或职业编号，使用数值表示。
	create := func(slot, job uint32) *pb.RoleSummary {
		// role 保存当前处理所用的角色。
		role := &pb.RoleSummary{}
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: job, Name: "范围" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())}, role, 0)
		// err 接收更新“one”过程中可能产生的错误，供后续失败分支处理。
		_, err := inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{"$set": bson.M{"level": testRoleLevel, "wallet": domain.Wallet{Gold: 1000000, BoundGold: 1000000, BoundYuanbao: 1000000}, "no_equipment_kills": cfg.Fixture.Drops.EquipmentGuarantee - 1, "privilege_until": time.Now().Add(time.Minute)}})
		must(t, err)
		return role
	}
	// mageRole 保存由创建取得的“mage”角色。
	mageRole := create(1, 1)
	// mage 保存由加入取得的“mage”。
	mage := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, mageRole.Key))
	defer mage.close()
	mage.auto(false, 2)
	// id 接收遍历数值列表时当前的元素。
	for _, id := range []uint32{5, 11, 23} {
		mage.buyAndLearnSkill(cfg.Fixture, id)
	}
	t.Log("法师NPC购书学习三项完成")
	time.Sleep(time.Duration(cfg.Fixture.Players[0].MoveMS+100) * time.Millisecond)
	// i 定位数值列表中正在处理的成员，id 接收遍历数值列表时当前的元素。
	for i, id := range []uint32{5, 11} {
		if i > 0 {
			time.Sleep(time.Duration(cfg.Fixture.Skills[0].GlobalSpellMS+100) * time.Millisecond)
		}
		// target 保存由无符号整数“32”取得的目标。
		target := uint32(10 + i)
		// mp 保存当前处理所用的魔法值。
		mp := mage.player().Mp
		// accepted 保存由施法已受理取得的已受理。
		accepted := mage.castAccepted(id, target, nil)
		// f 记录协议帧。
		mage.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && mage.player().CastSkillId == id })
		// s 接收技能供后续步骤使用。
		s, _ := cfg.Fixture.Skill(id)
		if mage.player().Mp != mp-s.MPBaseCost || accepted.CasterLife != mage.player().Life || accepted.TargetHandle != target {
			t.Fatal("新单体技能受理/生命代次/耗蓝不符")
		}
		// impact 保存由战斗命中影响取得的命中影响。
		impact := mage.combatImpact(id, target)
		if impact.Outcome != 1 || impact.Amount <= 0 {
			t.Fatal("新单体技能没有实际伤害结果")
		}
		t.Logf("法师技能%d真实命中完成", id)
	}

	// taoRole 保存由创建取得的“tao”角色。
	taoRole := create(2, 2)
	// tao 保存由加入取得的“tao”。
	tao := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, taoRole.Key))
	defer tao.close()
	tao.auto(false, 1) // 购书/学习RPC耗时不再落入宠物承受七怪伤害的观察窗。
	// id 接收遍历数值列表时当前的元素。
	for _, id := range []uint32{4, 29} {
		tao.buyAndLearnSkill(cfg.Fixture, id)
	}
	t.Log("道士NPC购书学习精神力/群疗完成")
	tao.auto(false, injuryMonsterCount)
	// ownerHandle 保存玩家实体句柄，用于当前步骤中的所有者实体句柄。
	ownerHandle := tao.entry.PlayerHandle
	if ownerHandle == 0 {
		t.Fatal("群疗入场缺少权威本人句柄")
	}
	// petHandle 保存由无符号整数“32”取得的宠物实体句柄。
	petHandle := uint32(0)
	// entity 接收遍历实体时当前的元素。
	for _, entity := range tao.scene.Entities {
		if entity.EntityKind == 2 {
			if petHandle != 0 {
				t.Fatal("群疗隔离前态出现多只宠物")
			}
			petHandle = entity.EntityHandle
		}
	}
	if petHandle == 0 {
		t.Fatal("群疗隔离前态缺少宠物")
	}
	// f 记录协议帧。
	tao.until(func(f wire.Frame) bool {
		if f.ID != p.MsgS2CSceneBaseline {
			return false
		}
		// player 保存由实体取得的玩家，pet 保存由实体取得的宠物。
		player, pet := tao.entity(ownerHandle), tao.entity(petHandle)
		if player == nil || pet == nil || player.Hp <= 0 || pet.Hp <= 0 {
			t.Fatal("群疗制造受伤阶段前态失效：友方缺失或死亡")
		}
		return player.Hp < player.HpMax && pet.Hp < pet.HpMax
	})
	time.Sleep(time.Duration(moveWaitMS) * time.Millisecond)
	// 同owner命令的回复后紧接其基线，排空旧观察；不能把睡眠前的缓存HP当施法时前态。
	tao.auto(false, injuryMonsterCount)
	// f 记录协议帧。
	tao.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	// player 保存由实体取得的玩家，pet 保存由实体取得的宠物。
	player, pet := tao.entity(ownerHandle), tao.entity(petHandle)
	if player == nil || pet == nil {
		t.Fatal("群疗发起阶段前态失效：友方缺失")
	}
	// center 按目标类型读取当前接口值，得到“center”。
	center := proto.Clone(player.Position).(*pb.GridPoint)
	// healingScene 保存场景代次，用于当前步骤中的治疗场景，casterLife 保存生命代次，用于当前步骤中的施法者生命代次。
	// petLife 保存生命代次，用于当前步骤中的宠物生命代次。
	healingScene, casterLife, petLife := tao.entry.SceneEpoch, player.Life, pet.Life
	// inside 封装“inside”对应的后续处理，point 记录点位，使用网格点位表示。
	inside := func(point *pb.GridPoint) bool {
		if point == nil {
			return false
		}
		// dx 保存根据整数“64”与整数“64”计算的横向差值，dy 保存根据整数“64”与整数“64”计算的纵向差值。
		dx, dy := int64(point.X)-int64(center.X), int64(point.Y)-int64(center.Y)
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		return dx <= int64(massHealing.AreaRadius) && dy <= int64(massHealing.AreaRadius)
	}
	// healed 初始化用作已治疗的按数值索引的布尔状态映射。
	healed := map[uint32]bool{}
	// checkPendingFriends 封装“check”待确认“friends”对应的后续处理，stage 记录阶段，使用文本表示。
	checkPendingFriends := func(stage string) {
		if tao.scene.SceneEpoch != healingScene {
			t.Fatalf("群疗%s前态失效：scene已变化", stage)
		}
		// friend 接收遍历临时结构列表时当前的元素。
		for _, friend := range []struct {
			// handle 记录实体句柄，使用数值表示。
			handle uint32
			// life 记录生命代次，使用数值表示。
			life uint64
		}{{ownerHandle, casterLife}, {petHandle, petLife}} {
			if healed[friend.handle] {
				continue
			}
			// state 保存由实体取得的状态。
			state := tao.entity(friend.handle)
			if state == nil || state.Hp <= 0 || state.Life != friend.life || !inside(state.Position) {
				t.Fatalf("群疗%s前态失效：handle=%d期望life=%d，当前=%v，中心=%v", stage, friend.handle, friend.life, state, center)
			}
		}
	}
	checkPendingFriends("发起")
	if player.Hp >= player.HpMax || pet.Hp >= pet.HpMax {
		t.Fatal("群疗发起阶段前态失效：两个友方必须均受伤")
	}
	// beforePlayer 保存生命值，用于当前步骤中的操作前的状态玩家，beforePet 保存生命值，用于当前步骤中的操作前的状态宠物。
	beforePlayer, beforePet := player.Hp, pet.Hp
	t.Logf("准备群疗：state=%d owner=%d life=%d hp=%d pet=%d life=%d hp=%d center=%v", tao.scene.StateSequence, ownerHandle, casterLife, player.Hp, petHandle, petLife, pet.Hp, center)
	// accepted 保存由施法已受理取得的已受理。
	accepted := tao.castAccepted(29, 0, center)
	if accepted.SceneEpoch != healingScene || accepted.CasterHandle != ownerHandle || accepted.CasterLife != casterLife || accepted.InputSequence != tao.input || !proto.Equal(accepted.Origin, center) || !proto.Equal(accepted.TargetGrid, center) || accepted.DelayMs != uint32(massHealing.DelayBaseMS) {
		t.Fatalf("群疗本次CA与发起life/中心/输入/延迟不符：%v", accepted)
	}
	checkPendingFriends("受理后")
	for len(healed) < 2 {
		// f 接收读取供后续步骤使用。
		f := tao.read()
		checkPendingFriends("生效观察")
		if f.ID != p.MsgS2CCombatImpact {
			continue
		}
		// impact 保存当前处理所用的命中影响。
		impact := &pb.CombatImpact{}
		must(t, proto.Unmarshal(f.Payload, impact))
		if impact.SkillId != 29 || impact.CastId != accepted.CastId {
			continue
		}
		// expectedLife 保存当前处理所用的预期结果生命代次。
		expectedLife := casterLife
		if impact.TargetHandle == petHandle {
			expectedLife = petLife
		} else if impact.TargetHandle != ownerHandle {
			t.Fatalf("群疗本次cast出现非预期目标：%v", impact)
		}
		if impact.SceneEpoch != healingScene || impact.TargetLife != expectedLife || impact.Outcome != 4 || !impact.Healing || impact.Amount <= 0 {
			t.Fatalf("群疗合法前态未逐友方入队：%v", impact)
		}
		healed[impact.TargetHandle] = true
	}
	if !healed[ownerHandle] || !healed[petHandle] {
		t.Fatal("群疗没有逐友方入队")
	}
	// playerRecovered 保存当前处理所用的玩家“recovered”，petRecovered 保存当前处理所用的宠物“recovered”。
	playerRecovered, petRecovered := false, false
	// f 记录协议帧。
	tao.until(func(f wire.Frame) bool {
		if f.ID != p.MsgS2CSceneBaseline {
			return false
		}
		// player 保存由实体取得的玩家，pet 保存由实体取得的宠物。
		player, pet := tao.entity(ownerHandle), tao.entity(petHandle)
		if player == nil || pet == nil || player.Hp <= 0 || pet.Hp <= 0 || player.Life != casterLife || pet.Life != petLife {
			t.Fatal("群疗分跳恢复观察前态失效：友方缺失/死亡/换life")
		}
		// playerHP 保存生命值，用于当前步骤中的玩家生命值，petHP 保存生命值，用于当前步骤中的宠物生命值。
		playerHP, petHP := player.Hp, pet.Hp
		playerRecovered = playerRecovered || playerHP > beforePlayer
		petRecovered = petRecovered || petHP > beforePet
		beforePlayer, beforePet = playerHP, petHP
		return playerRecovered && petRecovered
	})
	t.Log("群疗双方真实分跳完成")
	tao.close()

	// waveRole 保存由创建取得的“wave”角色。
	waveRole := create(3, 1)
	// waveID 标识当前业务使用的“wave”。
	waveID := domain.Hex(waveRole.Key.RoleId)
	// wave 保存由加入取得的“wave”。
	wave := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, waveRole.Key))
	defer func() { wave.close() }()
	wave.auto(false, 1)
	wave.buyAndLearnSkill(cfg.Fixture, 23)
	waveCenter := &pb.GridPoint{X: 7, Y: 7}
	wave.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: wave.entry.SceneEpoch, Destination: waveCenter})
	// f 记录协议帧。
	wave.until(func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && proto.Equal(wave.player().Position, waveCenter)
	})
	time.Sleep(time.Duration(moveWaitMS) * time.Millisecond)
	wave.auto(false, 8)
	wave.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	waveSkill, ok := cfg.Fixture.Skill(23)
	if !ok {
		t.Fatal("缺少爆裂火焰唯一配置")
	}
	waveRadius := int32(waveSkill.AreaRadius)
	occupied := map[[2]int32]bool{{waveCenter.X, waveCenter.Y}: true}
	for handle := uint32(10); handle < 18; handle++ {
		monster := wave.entity(handle)
		if monster == nil || monster.Hp <= 0 || monster.Position == nil {
			t.Fatalf("八杀前态缺少原活怪：handle=%d state=%v", handle, monster)
		}
		x, y := monster.Position.X, monster.Position.Y
		cell := [2]int32{x, y}
		if occupied[cell] || x < waveCenter.X-waveRadius || x > waveCenter.X+waveRadius || y < waveCenter.Y-waveRadius || y > waveCenter.Y+waveRadius {
			t.Fatalf("八杀前态的活怪重叠或超出爆裂范围：handle=%d position=%v center=%v", handle, monster.Position, waveCenter)
		}
		occupied[cell] = true
	}
	t.Log("准备同帧八杀")
	// before 保存由角色取得的操作前的状态。
	before, err := inspection.Role(ctx, waveID)
	must(t, err)
	if before.NoEquipmentKills != cfg.Fixture.Drops.EquipmentGuarantee-1 || before.SourceSeq != 0 {
		t.Fatal("八杀前态必须保留建角时设定的隔离保底，并且未产生过击杀收益")
	}
	wave.castAccepted(23, 0, waveCenter)
	// f 记录协议帧。
	wave.until(func(f wire.Frame) bool {
		if f.ID != p.MsgS2CSceneBaseline {
			return false
		}
		// dead 规定死亡状态的初始取值。
		dead := 0
		// entity 接收遍历实体时当前的元素。
		for _, entity := range wave.scene.Entities {
			if entity.EntityKind == 3 && entity.Hp == 0 {
				dead++
			}
		}
		return dead == 8
	})
	// audit 保存审计记录所需的状态。
	type audit struct {
		// SourceSeq 记录持久收益来源序号，使用数值表示。
		SourceSeq int64 `bson:"source_seq"`
		// At 记录发生时间，使用时间点表示。
		At time.Time `bson:"event_at"`
		// Bound 标记是否处于绑定状态。
		Bound bool `bson:"bound"`
		// Target 记录目标，使用数值表示。
		Target uint32 `bson:"target_handle"`
		// Item 记录道具。
		Item *domain.Item `bson:"item"`
		// Experience 记录经验，使用数值表示。
		Experience int64 `bson:"experience"`
	}
	// journal 记录“journal”，使用审计记录列表表示。
	var journal []audit
	// deadline 保存由新增取得的截止时间。
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		// cursor 保存由查找取得的游标，err 接收查找过程中可能产生的错误，供后续失败分支处理。
		cursor, err := inspection.DB.Collection("audit_outbox").Find(ctx, bson.M{"role_id": waveID, "kind": "settlement", "kills": 1, "source_seq": bson.M{"$gt": before.SourceSeq}}, options.Find().SetSort(bson.D{{Key: "source_seq", Value: 1}}))
		must(t, err)
		journal = nil
		must(t, cursor.All(ctx, &journal))
		if len(journal) == 8 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(journal) != 8 {
		t.Fatalf("同帧八杀漏结算:%d", len(journal))
	}
	// counter 保存“no”装备击杀集合，用于当前步骤中的计数。
	counter := before.NoEquipmentKills
	// i 定位“journal”中正在处理的成员，row 接收遍历“journal”时当前的元素。
	for i, row := range journal {
		if row.SourceSeq != before.SourceSeq+int64(i)+1 || row.Target != uint32(10+i) || row.Bound || row.At.IsZero() || !row.At.Equal(journal[0].At) {
			t.Fatal("逐杀序号/稳定目标次序/killAt/绑定不符")
		}
		counter++
		if row.Item != nil {
			// definition 保存由道具取得的定义。
			definition, _ := cfg.Fixture.Item(row.Item.ItemID)
			if definition.IsEquipment {
				counter = 0
			}
			if row.Item.Bound || row.Item.SourceKind != "monster_drop" {
				t.Fatal("击杀事件绑定或来源丢失")
			}
		}
		if i == 0 {
			if row.Item == nil {
				t.Fatal("八杀第一条第20杀保底丢失")
			}
			// definition 保存由道具取得的定义。
			definition, _ := cfg.Fixture.Item(row.Item.ItemID)
			if !definition.IsEquipment {
				t.Fatal("保底没有真实发装备")
			}
		}
	}
	// after 保存由角色取得的操作后的状态。
	after, err := inspection.Role(ctx, waveID)
	must(t, err)
	if after.SourceSeq != before.SourceSeq+8 || after.NoEquipmentKills != counter || after.Experience != before.Experience+8*cfg.Fixture.Monsters[0].Exp {
		t.Fatal("多杀永久经验/序号/保底未逐事件守恒")
	}
	// scene 保存场景代次，用于当前步骤中的场景。
	scene := wave.entry.SceneEpoch
	wave.close()
	wave = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, waveRole.Key))
	wave.auto(false, 8)
	// again 保存由角色取得的“again”。
	again, err := inspection.Role(ctx, waveID)
	must(t, err)
	if wave.entry.SceneEpoch != scene || again.SourceSeq != after.SourceSeq || again.Wallet != after.Wallet || len(again.Items) != len(after.Items) {
		t.Fatal("健康重连重播多杀收益")
	}
	t.Log("通过：NPC学习新增五项、两单体实际耗蓝命中、Spirit零级与群疗受伤友方分跳、八杀逐事件source/保底/killAt/绑定/审计、重连不重奖")
}
