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
func TestPetRuntimeCorpseHealthyExpiryAndColdEntryMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1连接本任务专属Mongo副本集，验证真实宠物尸体WS生命周期")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion != 16 || cfg.Fixture.PetRuntime == nil || !cfg.Fixture.PetRuntime.Enabled || cfg.Fixture.PetRuntime.BootstrapMode != "legacy_untrained_pet" {
		t.Fatal("真实用例必须从当前rule16及显式legacy_untrained_pet合同开始")
	}
	if _, enabled := cfg.Fixture.Skill(17); enabled || cfg.Fixture.CastingSupply.Enabled {
		t.Fatal("本阶段不得开启17或护符")
	}
	const corpseTTL = 3 * time.Second
	// 来源定义和legacy Fighter必须同步，不能靠绕过Validate制造两份基础属性。
	cfg.Fixture.Pet.HP, cfg.Fixture.Pet.AttackMin, cfg.Fixture.Pet.AttackMax = 1, 0, 0
	matched := false
	for i := range cfg.Fixture.PetRuntime.Definitions {
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
	for i := range cfg.Fixture.Monsters {
		monster := &cfg.Fixture.Monsters[i]
		monster.AttackMin, monster.AttackMax, monster.Accuracy = 20, 20, 255
		// 允许首击；后续长节流避免尸体观察期间怪物转攻主人，把生命周期前态变成死亡重生。
		monster.AttackMS, monster.MoveMS = 600000, 600000
	}
	must(t, cfg.Fixture.Validate())
	cfg.Database = "minimir_dev_game_test_pet_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	ctx := context.Background()
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	launch := func() func() {
		run, cancel := context.WithCancel(context.Background())
		var wait sync.WaitGroup
		for _, mode := range []string{"account", "game", "gm"} {
			wait.Go(func() {
				if runErr := bootstrap.Run(run, cfg, mode); runErr != nil && run.Err() == nil {
					t.Errorf("宠物%s节点: %v", mode, runErr)
				}
			})
		}
		for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
			healthy(t, endpoint)
		}
		return func() { cancel(); wait.Wait() }
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
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "pet_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	create := func(slot uint32) *pb.RoleSummary {
		role := &pb.RoleSummary{}
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: 2, Name: fmt.Sprintf("宠物%d%s", slot, domain.ID()[:5]), OpId: domain.Bytes(domain.ID())}, role, 0)
		return role
	}
	readRole := func(role *pb.RoleSummary) domain.Role {
		stored, readErr := inspection.Role(ctx, domain.Hex(role.Key.RoleId))
		must(t, readErr)
		return stored
	}
	assertPermanent := func(role *pb.RoleSummary, before domain.Role, phase string) {
		after := readRole(role)
		travelAssertEconomy(t, before, after, phase)
		if before.Revision != after.Revision {
			t.Fatalf("%s宠物死亡/期限推进了永久revision: %d -> %d", phase, before.Revision, after.Revision)
		}
		var document bson.M
		must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}).Decode(&document))
		for _, field := range []string{"hp", "mp", "position", "pet", "pet_runtime", "pet_corpses", "pet_training", "pet_world", "pet_life_sequence", "pending_pet_birth", "worlds", "saved_personal", "monsters", "ground_items"} {
			if _, present := document[field]; present {
				t.Fatalf("%s角色文档误存临时宠物/世界字段%s", phase, field)
			}
		}
	}

	first := create(1)
	before := readRole(first)
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, first.Key))
	defer func() { c.close() }()
	initial := petRuntimeWSInitialPet(t, c)
	c.auto(false, 1)
	petRuntimeWSWaitCorpse(t, c, initial)
	deathObservedAt := time.Now()
	corpseScene, oldBinding := c.entry.SceneEpoch, c.binding
	corpse := proto.Clone(petRuntimeWSEntity(c.scene, initial.EntityHandle)).(*pb.EntityState)
	assertPermanent(first, before, "首次真实怪物击杀宠物")
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
	deadline := deathObservedAt.Add(corpseTTL + 700*time.Millisecond)
	travelUntilWithin(c, time.Until(deadline), func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CSceneBaseline && petRuntimeWSEntity(c.scene, initial.EntityHandle) == nil
	})
	if time.Now().After(deadline) {
		t.Fatal("健康重入刷新了尸体期限或到期没有移除")
	}
	for _, entity := range c.scene.Entities {
		if entity.EntityKind == 2 {
			t.Fatal("尸体到期后自动恢复/新生了宠物", entity)
		}
	}
	if c.player() == nil || c.player().Hp <= 0 {
		t.Fatal("隔离观察前态意外杀死主人，不能认证健康重入")
	}
	assertPermanent(first, before, "健康重入及原尸体期限")
	c.close()

	// 第二个正常角色在原尸体仍未到期时停全部节点，防止仅测试自然到期后的空世界。
	second := create(2)
	secondBefore := readRole(second)
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, second.Key))
	secondInitial := petRuntimeWSInitialPet(t, c)
	c.auto(false, 1)
	petRuntimeWSWaitCorpse(t, c, secondInitial)
	secondDeathObservedAt, oldScene := time.Now(), c.entry.SceneEpoch
	assertPermanent(second, secondBefore, "冷建前未到期尸体")
	c.close()
	stop()
	if time.Since(secondDeathObservedAt) >= corpseTTL {
		t.Fatal("停止节点时尸体原期限已过，不能认证冷建不恢复未到期临时对象")
	}
	stop = launch()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, second.Key))
	if c.entry.SceneEpoch == oldScene || c.entry.RoomId != cfg.Fixture.Map.ID {
		t.Fatal("完整停服冷建未重新创建个人世界", c.entry)
	}
	fresh := petRuntimeWSInitialPet(t, c)
	if fresh.IsPetCorpse || fresh.Hp != cfg.Fixture.Pet.HP || fresh.PetTraining != nil {
		t.Fatal("冷建恢复旧尸体/训练，或未按明确免费legacy模式新建宠物", fresh)
	}
	assertPermanent(second, secondBefore, "完整停止后冷建")
	t.Logf("通过真实Mongo/WS宠物生命周期：随机库%s；道士正常初始宠物由怪物首击死亡，同handle/Life原尸体、无第二活宠、健康新票原scene/尸体/TTL、到期移除、未到期停服后仅legacy免费新建；角色文档无临时宠物字段且宠物死亡零killEXP/经济奖励。隔离HP1/宠攻击0/怪伤20与命中255/怪动作600000ms/前格/TTL3000ms仅局部cfg，不是主180秒、训练或17正常耗符验收；NoMaster友方过滤由game专项覆盖。", cfg.Database)
}

func petRuntimeWSEntity(scene *pb.SceneBaselineChunk, handle uint32) *pb.EntityState {
	for _, entity := range scene.GetEntities() {
		if entity.EntityHandle == handle {
			return entity
		}
	}
	return nil
}

func petRuntimeWSInitialPet(t *testing.T, c *client) *pb.EntityState {
	t.Helper()
	if c.scene.ChunkCount != 1 || c.scene.SceneEpoch != c.entry.SceneEpoch || c.scene.BaselineId != c.entry.BaselineId {
		t.Fatal("小实体场景未收到单块完整初始基线")
	}
	var pet *pb.EntityState
	for _, entity := range c.scene.Entities {
		if entity.EntityKind != 2 {
			continue
		}
		if pet != nil || entity.IsPetCorpse || entity.Hp <= 0 || entity.Life == 0 || entity.PetTraining != nil {
			t.Fatal("初始基线缺少唯一真实免费未训练活宠", entity)
		}
		pet = entity
	}
	if pet == nil {
		t.Fatal("正常道士未收到初始宠物")
	}
	return proto.Clone(pet).(*pb.EntityState)
}

func petRuntimeWSAssertCorpse(t *testing.T, scene *pb.SceneBaselineChunk, original *pb.EntityState) {
	t.Helper()
	var count int
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

func petRuntimeWSWaitCorpse(t *testing.T, c *client, original *pb.EntityState) {
	t.Helper()
	travelUntilWithin(c, 4*time.Second, func(frame wire.Frame) bool {
		body := petRuntimeWSEntity(c.scene, original.EntityHandle)
		return frame.ID == p.MsgS2CSceneBaseline && body != nil && body.IsPetCorpse && body.Hp == 0
	})
	petRuntimeWSAssertCorpse(t, c.scene, original)
	monster := travelEntity(c.scene, 3)
	if monster == nil || monster.Hp <= 0 || monster.AttackSequence == 0 {
		t.Fatal("没有真实怪物攻击因果，或宠物死亡时误发怪物击杀奖励", monster)
	}
}
