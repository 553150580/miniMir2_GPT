// 验证召唤宠物尸体、过期、健康重连与冷入场的实际运行态边界。

package integration

import (
	"context"
	"fmt"
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

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

// 仅随机库局部cfg缩短尸体期限、降低宠物HP并控制怪物首击；不注入临时body或开启17/材料。
// 此用例覆盖WS生命周期，原180秒参数、训练收益、来源NoMaster友方筛选另有配置/game专项。
// TestPetRuntimeCorpseHealthyExpiryAndColdEntryMongoWS 核对宠物死亡、尸体到期和健康或冷重连下各自的场景行为。
// t 提供断言失败报告、子用例和清理注册。
func TestPetRuntimeCorpseHealthyExpiryAndColdEntryMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1连接本任务专属Mongo副本集，验证真实宠物尸体WS生命周期")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	if cfg.Fixture.RuleVersion < 16 || cfg.Fixture.PetRuntime == nil || !cfg.Fixture.PetRuntime.Enabled || cfg.Fixture.PetRuntime.BootstrapMode != "legacy_untrained_pet" {
		t.Fatal("真实用例必须从当前生命周期及显式legacy_untrained_pet合同开始")
	}
	// enabled 保存由技能取得的已开启。
	if _, enabled := cfg.Fixture.Skill(17); enabled || cfg.Fixture.CastingSupply.Enabled {
		t.Fatal("本阶段不得开启17或护符")
	}
	// corpseTTL 保存根据当前数据与秒计算的尸体有效期限。
	const corpseTTL = 10 * time.Second
	// 来源定义和legacy Fighter必须同步，不能靠绕过Validate制造两份基础属性。
	cfg.Fixture.Pet.HP, cfg.Fixture.Pet.AttackMin, cfg.Fixture.Pet.AttackMax = 1, 0, 0
	// matched 保存当前处理所用的已匹配。
	matched := false
	// i 标记遍历定义集合时访问的下标。
	for i := range cfg.Fixture.PetRuntime.Definitions {
		// definition 保存当前处理所用的定义。
		definition := &cfg.Fixture.PetRuntime.Definitions[i]
		if definition.Base.DefinitionID == cfg.Fixture.Pet.DefinitionID {
			definition.Base = cfg.Fixture.Pet
			definition.Lifecycle.CorpseTTLMS = corpseTTL.Milliseconds()
			matched = true
		}
	}
	if !matched {
		t.Fatal("当前legacy宠物没有匹配的typed定义")
	}
	// 宠物距主人2格，首怪距宠物1格且距主人3格；怪物首先真实选择并攻击宠物。
	cfg.Fixture.Map.PetSpawn = devconfig.Point{X: 8, Y: 6}
	cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: 8, Y: 5}
	// i 标记遍历怪物集合时访问的下标。
	for i := range cfg.Fixture.Monsters {
		// monster 保存当前处理所用的怪物。
		monster := &cfg.Fixture.Monsters[i]
		monster.AttackMin, monster.AttackMax, monster.Accuracy = 20, 20, 255
		// 允许首击；后续长节流避免尸体观察期间怪物转攻主人，把生命周期前态变成死亡重生。
		monster.AttackMS, monster.MoveMS = 600000, 600000
	}
	must(t, cfg.Fixture.Validate())
	cfg.Database = "minimir_dev_game_test_pet_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// ctx 保存由“background”取得的上下文。
	ctx := context.Background()
	// inspection 保存由打开取得的检查。
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	// launch 封装启动对应的后续处理。
	launch := func() func() {
		// run 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
		run, cancel := context.WithCancel(context.Background())
		// wait 记录等待，使用协程汇合计数器表示。
		var wait sync.WaitGroup
		// mode 接收遍历文本列表时当前的元素。
		for _, mode := range []string{"account", "game", "gm"} {
			wait.Go(func() {
				// runErr 保存由运行批次取得的运行批次错误。
				if runErr := bootstrap.Run(run, cfg, mode); runErr != nil && run.Err() == nil {
					t.Errorf("宠物%s节点: %v", mode, runErr)
				}
			})
		}
		// endpoint 接收遍历文本列表时当前的元素。
		for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
			healthy(t, endpoint)
		}
		return func() { cancel(); wait.Wait() }
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
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "pet_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	// create 封装创建对应的后续处理，slot 记录槽位，使用数值表示。
	create := func(slot uint32) *pb.RoleSummary {
		// role 保存当前处理所用的角色。
		role := &pb.RoleSummary{}
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: 2, Name: fmt.Sprintf("宠物%d%s", slot, domain.ID()[:5]), OpId: domain.Bytes(domain.ID())}, role, 0)
		return role
	}
	// readRole 封装读取角色对应的后续处理，role 记录角色，使用角色“summary”表示。
	readRole := func(role *pb.RoleSummary) domain.Role {
		// stored 保存由角色取得的已持久保存，readErr 保存由角色取得的读取错误。
		stored, readErr := inspection.Role(ctx, domain.Hex(role.Key.RoleId))
		must(t, readErr)
		return stored
	}
	// assertPermanent 封装“assert”“permanent”对应的后续处理，role 记录角色，使用角色“summary”表示，before 记录操作前的状态，使用角色表示。
	// acceptedEntries 记录已受理“entries”，使用数值表示，phase 记录阶段，使用文本表示。
	assertPermanent := func(role *pb.RoleSummary, before domain.Role, acceptedEntries int64, phase string) {
		// after 保存由读取角色取得的操作后的状态。
		after := readRole(role)
		travelAssertEconomy(t, before, after, phase)
		// Store.Enter正常接管会同时推进SessionEpoch/revision，尸体自身不推进永久版本。
		if after.Revision != before.Revision+acceptedEntries || after.SessionEpoch != before.SessionEpoch+acceptedEntries {
			t.Fatalf("%s永久版本超出%d次合法Enter: revision %d -> %d, session %d -> %d", phase, acceptedEntries, before.Revision, after.Revision, before.SessionEpoch, after.SessionEpoch)
		}
		// document 记录文档，使用MongoDB BSON 文档表示。
		var document bson.M
		must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}).Decode(&document))
		// field 接收遍历文本列表时当前的元素。
		for _, field := range []string{"hp", "mp", "position", "pet", "pet_runtime", "pet_corpses", "pet_training", "pet_world", "pet_life_sequence", "pending_pet_birth", "worlds", "saved_personal", "monsters", "ground_items"} {
			// present 保存从文档取出的“present”。
			if _, present := document[field]; present {
				t.Fatalf("%s角色文档误存临时宠物/世界字段%s", phase, field)
			}
		}
	}

	// first 保存由创建取得的第一项。
	first := create(1)
	// before 保存由读取角色取得的操作前的状态。
	before := readRole(first)
	// c 接收加入供后续步骤使用。
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, first.Key))
	defer func() { c.close() }()
	// initial 保存由宠物运行时WebSocket初始值宠物取得的初始值。
	initial := petRuntimeWSInitialPet(t, c)
	c.auto(false, 1)
	petRuntimeWSWaitCorpse(t, c, initial)
	// deathObservedAt 记录本次处理开始时的时间。
	deathObservedAt := time.Now()
	// corpseScene 保存场景代次，用于当前步骤中的尸体场景，oldBinding 保存会话绑定，用于当前步骤中的原状态会话绑定。
	corpseScene, oldBinding := c.entry.SceneEpoch, c.binding
	// corpse 按目标类型读取当前接口值，得到尸体。
	corpse := proto.Clone(petRuntimeWSEntity(c.scene, initial.EntityHandle)).(*pb.EntityState)
	assertPermanent(first, before, 1, "首次真实怪物击杀宠物")
	c.close()
	// 将重入安排在原TTL中段，若重入重置期限，将明显晚于下面的原截止窗口。
	time.Sleep(1300 * time.Millisecond)
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, first.Key))
	if c.entry.SceneEpoch != corpseScene || c.binding <= oldBinding {
		t.Fatal("健康新票未沿原scene换代连接，或重建了世界", c.entry)
	}
	petRuntimeWSAssertCorpse(t, c.scene, initial)
	if !proto.Equal(petRuntimeWSEntity(c.scene, initial.EntityHandle).Position, corpse.Position) {
		t.Fatal("健康新票改变了原尸体位置")
	}
	if time.Since(deathObservedAt) >= corpseTTL-300*time.Millisecond {
		t.Fatal("健康重入已过原TTL观察前置窗口，不能据此认证原期限保留")
	}
	c.auto(false, 1)
	// deadline 保存由新增取得的截止时间。
	deadline := deathObservedAt.Add(corpseTTL + 700*time.Millisecond)
	// frame 记录协议帧。
	travelUntilWithin(c, time.Until(deadline), func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && petRuntimeWSEntity(c.scene, initial.EntityHandle) == nil
	})
	if time.Now().After(deadline) {
		t.Fatal("健康重入刷新了尸体期限或到期没有移除")
	}
	// entity 接收遍历实体时当前的元素。
	for _, entity := range c.scene.Entities {
		if entity.EntityKind == 2 {
			t.Fatal("尸体到期后自动恢复/新生了宠物", entity)
		}
	}
	if c.player() == nil || c.player().Hp <= 0 {
		t.Fatal("隔离观察前态意外杀死主人，不能认证健康重入")
	}
	assertPermanent(first, before, 2, "健康重入及原尸体期限")
	c.close()

	// 第二个正常角色在原尸体仍未到期时停全部节点，防止仅测试自然到期后的空世界。
	// second 保存由创建取得的秒。
	second := create(2)
	// secondBefore 保存由读取角色取得的秒操作前的状态。
	secondBefore := readRole(second)
	// 此刻早于初次Enter，故也早于任何可能的真实死亡；它给出严格的原到期下界。
	// secondFirstEnterAt 记录本次处理开始时的时间。
	secondFirstEnterAt := time.Now()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, second.Key))
	// secondInitial 保存由宠物运行时WebSocket初始值宠物取得的秒初始值。
	secondInitial := petRuntimeWSInitialPet(t, c)
	c.auto(false, 1)
	petRuntimeWSWaitCorpse(t, c, secondInitial)
	// secondDeathObservedAt 记录本次处理开始时的时间，oldScene 保存场景代次，用于当前步骤中的原状态场景。
	secondDeathObservedAt, oldScene := time.Now(), c.entry.SceneEpoch
	assertPermanent(second, secondBefore, 1, "冷建前未到期尸体")
	c.close()
	stop()
	if time.Since(secondDeathObservedAt) >= corpseTTL {
		t.Fatal("停止节点时尸体原期限已过，不能认证冷建不恢复未到期临时对象")
	}
	stop = launch()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, second.Key))
	if !time.Now().Before(secondFirstEnterAt.Add(corpseTTL)) {
		t.Fatal("冷建初基线/ACK已超过原尸体最早可能到期时间，不能认证未到期临时对象不恢复")
	}
	if c.entry.SceneEpoch == oldScene || c.entry.RoomId != cfg.Fixture.Map.ID {
		t.Fatal("完整停服冷建未重新创建个人世界", c.entry)
	}
	// fresh 保存由宠物运行时WebSocket初始值宠物取得的首次装载。
	fresh := petRuntimeWSInitialPet(t, c)
	if fresh.IsPetCorpse || fresh.Hp != cfg.Fixture.Pet.HP || fresh.PetTraining != nil {
		t.Fatal("冷建恢复旧尸体/训练，或未按明确免费legacy模式新建宠物", fresh)
	}
	assertPermanent(second, secondBefore, 2, "完整停止后冷建")
	t.Logf("通过真实Mongo/WS宠物生命周期：随机库%s；道士正常初始宠物由怪物首击死亡，同handle/Life原尸体、无第二活宠、健康新票原scene/尸体/TTL、到期移除、未到期停服后仅legacy免费新建；角色文档无临时宠物字段且宠物死亡零killEXP/经济奖励。隔离HP1/宠攻击0/怪伤20与命中255/怪动作600000ms/前格/TTL10000ms仅局部cfg，不是主180秒、训练或17正常耗符验收；NoMaster友方过滤由game专项覆盖。", cfg.Database)
}

// petRuntimeWSEntity 返回宠物运行时WebSocket实体供调用方使用。
// scene 记录场景，使用场景场景基线分片表示，handle 记录实体句柄，使用数值表示。
func petRuntimeWSEntity(scene *pb.SceneBaselineChunk, handle uint32) *pb.EntityState {
	// entity 接收遍历“get”实体时当前的元素。
	for _, entity := range scene.GetEntities() {
		if entity.EntityHandle == handle {
			return entity
		}
	}
	return nil
}

// petRuntimeWSInitialPet 返回宠物运行时WebSocket初始值宠物供调用方使用。
// t 提供断言失败报告、子用例和清理注册，c 记录客户端。
func petRuntimeWSInitialPet(t *testing.T, c *client) *pb.EntityState {
	t.Helper()
	if c.scene.ChunkCount != 1 || c.scene.SceneEpoch != c.entry.SceneEpoch || c.scene.BaselineId != c.entry.BaselineId {
		t.Fatal("小实体场景未收到单块完整初始基线")
	}
	// pet 记录宠物，使用实体状态表示。
	var pet *pb.EntityState
	// entity 接收遍历实体时当前的元素。
	for _, entity := range c.scene.Entities {
		if entity.EntityKind != 2 {
			continue
		}
		if pet != nil || entity.IsPetCorpse || entity.Hp <= 0 || entity.PetTraining != nil {
			t.Fatal("初始基线缺少唯一真实免费未训练活宠", entity)
		}
		pet = entity
	}
	if pet == nil {
		t.Fatal("正常道士未收到初始宠物")
	}
	return proto.Clone(pet).(*pb.EntityState)
}

// petRuntimeWSAssertCorpse 处理宠物运行时WebSocket“assert”尸体。
// t 提供断言失败报告、子用例和清理注册，scene 记录场景，使用场景场景基线分片表示，original 记录原始，使用实体状态表示。
func petRuntimeWSAssertCorpse(t *testing.T, scene *pb.SceneBaselineChunk, original *pb.EntityState) {
	t.Helper()
	// count 记录数量，使用数值表示。
	var count int
	// entity 接收遍历“get”实体时当前的元素。
	for _, entity := range scene.GetEntities() {
		if entity.EntityKind != 2 {
			continue
		}
		count++
		if entity.EntityHandle != original.EntityHandle || entity.Life != original.Life || entity.Hp != 0 || !entity.IsPetCorpse || entity.PetTraining != nil {
			t.Fatal("尸体基线改变了原handle/Life、漏no-master标志或额外生成活宠", entity, original)
		}
	}
	if count != 1 {
		t.Fatalf("真实死亡后应只有原尸体，实际宠物身体数%d", count)
	}
}

// petRuntimeWSWaitCorpse 处理宠物运行时WebSocket等待尸体。
// t 提供断言失败报告、子用例和清理注册，c 记录客户端，original 记录原始，使用实体状态表示。
func petRuntimeWSWaitCorpse(t *testing.T, c *client, original *pb.EntityState) {
	t.Helper()
	// frame 记录协议帧。
	travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
		// body 保存由宠物运行时WebSocket实体取得的消息正文。
		body := petRuntimeWSEntity(c.scene, original.EntityHandle)
		return frame.ID == p.MsgS2CSceneBaseline && body != nil && body.IsPetCorpse && body.Hp == 0
	})
	petRuntimeWSAssertCorpse(t, c.scene, original)
	// monster 保存由地图切换实体取得的怪物。
	monster := travelEntity(c.scene, 3)
	if monster == nil || monster.Hp <= 0 || monster.AttackSequence == 0 {
		t.Fatal("没有真实怪物攻击因果，或宠物死亡时误发怪物击杀奖励", monster)
	}
}
