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

func (c *client) buyAndLearnSkill(f devconfig.Fixture, id uint32) {
	c.t.Helper()
	s, exists := f.Skill(id)
	if !exists {
		c.t.Fatal("缺少学习候选")
	}
	offer := uint32(0)
	for _, o := range f.SkillShop {
		if o.ItemID == s.BookItemID && o.Currency == "gold" {
			offer = o.ID
		}
	}
	preview := c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer, Quantity: 1})
	if r := c.skillConfirm(preview); r.ErrorCode != 0 {
		c.t.Fatal("新技能购书失败")
	}
	var book *pb.ItemView
	for _, item := range c.inventory.Items {
		if item.ItemId == s.BookItemID && item.SourceKind == "npc_shop" {
			book = item
			break
		}
	}
	if book == nil {
		c.t.Fatal("NPC没有真实交付技能书")
	}
	result := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: book.InstanceId, SkillId: id, Operation: c.operation()})
	c.inventoryAt(result.NewRevision)
	if result.ErrorCode != 0 {
		c.t.Fatalf("技能%d学习失败:%v", id, result)
	}
	found := false
	for _, learned := range c.inventory.LearnedSkills {
		if learned.SkillId == id && learned.Level == s.LearnedLevel {
			found = true
		}
	}
	if !found {
		c.t.Fatal("已学零级没有同步")
	}
}
func (c *client) castAccepted(id, target uint32, grid *pb.GridPoint) *pb.CastAccepted {
	c.t.Helper()
	c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: id, TargetHandle: target, TargetGrid: grid})
	event := &pb.CastAccepted{}
	c.until(func(f wire.Frame) bool {
		if f.ID != p.MsgS2CCastAccepted {
			return false
		}
		must(c.t, proto.Unmarshal(f.Payload, event))
		return event.SkillId == id
	})
	return event
}
func (c *client) combatImpact(id, target uint32) *pb.CombatImpact {
	c.t.Helper()
	out := &pb.CombatImpact{}
	c.until(func(f wire.Frame) bool {
		if f.ID != p.MsgS2CCombatImpact {
			return false
		}
		must(c.t, proto.Unmarshal(f.Payload, out))
		return out.SkillId == id && out.TargetHandle == target
	})
	return out
}

func TestCombatAreaFiveSkillsAndEightKillMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以验证8技能与多杀真实流水")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// 仅隔离用例使用近距离弱目标和可受伤宝宝，确定性观察区域命中/治疗；不改唯一Excel。
	cfg.Fixture.Monsters[0].HP = 5
	cfg.Fixture.Monsters[0].AttackMin = cfg.Fixture.Healing.PetBasePerTick * 2
	cfg.Fixture.Monsters[0].AttackMax = cfg.Fixture.Monsters[0].AttackMin
	cfg.Fixture.Monsters[0].Accuracy = cfg.Fixture.Pet.Agility
	cfg.Fixture.Pet.DefenceMin, cfg.Fixture.Pet.DefenceMax = 0, 0
	base, _ := cfg.Fixture.PlayerAtLevel(1, 36)
	cfg.Fixture.Monsters[0].DefenceMin, cfg.Fixture.Monsters[0].DefenceMax = base.AttackMax, base.AttackMax
	cfg.Fixture.Map.MonsterSpawns = []devconfig.Point{{X: 6, Y: 6}, {X: 6, Y: 7}, {X: 6, Y: 8}, {X: 7, Y: 6}, {X: 7, Y: 8}, {X: 8, Y: 6}, {X: 8, Y: 7}, {X: 8, Y: 8}}
	ctx, cancel := context.WithCancel(context.Background())
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	var wg sync.WaitGroup
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("区域测试%s:%v", mode, err)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, a)
	}
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "area_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
	create := func(slot, job uint32) *pb.RoleSummary {
		role := &pb.RoleSummary{}
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: job, Name: "范围" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())}, role, 0)
		_, err := inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{"$set": bson.M{"level": 36, "wallet": domain.Wallet{Gold: 1000000, BoundGold: 1000000, BoundYuanbao: 1000000}, "no_equipment_kills": cfg.Fixture.Drops.EquipmentGuarantee - 1, "privilege_until": time.Now().Add(time.Minute)}})
		must(t, err)
		return role
	}
	mageRole := create(1, 1)
	mage := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, mageRole.Key))
	defer mage.close()
	mage.auto(false, 2)
	for _, id := range []uint32{5, 11, 23} {
		mage.buyAndLearnSkill(cfg.Fixture, id)
	}
	t.Log("法师NPC购书学习三项完成")
	time.Sleep(time.Duration(cfg.Fixture.Players[0].MoveMS+100) * time.Millisecond)
	for i, id := range []uint32{5, 11} {
		if i > 0 {
			time.Sleep(time.Duration(cfg.Fixture.Skills[0].GlobalSpellMS+100) * time.Millisecond)
		}
		target := uint32(10 + i)
		mp := mage.player().Mp
		accepted := mage.castAccepted(id, target, nil)
		mage.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && mage.player().CastSkillId == id })
		s, _ := cfg.Fixture.Skill(id)
		if mage.player().Mp != mp-s.MPBaseCost || accepted.CasterLife != mage.player().Life || accepted.TargetHandle != target {
			t.Fatal("新单体技能受理/生命代次/耗蓝不符")
		}
		impact := mage.combatImpact(id, target)
		if impact.Outcome != 1 || impact.Amount <= 0 {
			t.Fatal("新单体技能没有实际伤害结果")
		}
		t.Logf("法师技能%d真实命中完成", id)
	}

	taoRole := create(2, 2)
	tao := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, taoRole.Key))
	defer tao.close()
	tao.auto(false, 7)
	for _, id := range []uint32{4, 29} {
		tao.buyAndLearnSkill(cfg.Fixture, id)
	}
	t.Log("道士NPC购书学习精神力/群疗完成")
	tao.until(func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && tao.player().Hp < tao.player().HpMax && tao.entity(2).Hp > 0 && tao.entity(2).Hp < tao.entity(2).HpMax
	})
	beforePlayer, beforePet := tao.player().Hp, tao.entity(2).Hp
	time.Sleep(time.Duration(cfg.Fixture.Players[0].MoveMS+100) * time.Millisecond)
	t.Logf("准备群疗：player=%d pet=%d", tao.player().Hp, tao.entity(2).Hp)
	tao.castAccepted(29, 0, tao.player().Position)
	healed := map[uint32]bool{}
	for len(healed) < 2 {
		f := tao.read()
		if f.ID != p.MsgS2CCombatImpact {
			continue
		}
		impact := &pb.CombatImpact{}
		must(t, proto.Unmarshal(f.Payload, impact))
		if impact.SkillId == 29 && impact.Outcome == 4 && impact.Healing && impact.Amount > 0 {
			healed[impact.TargetHandle] = true
		}
	}
	if !healed[1] || !healed[2] {
		t.Fatal("群疗没有逐友方入队")
	}
	playerRecovered, petRecovered := false, false
	tao.until(func(f wire.Frame) bool {
		if f.ID != p.MsgS2CSceneBaseline {
			return false
		}
		playerHP, petHP := tao.player().Hp, tao.entity(2).Hp
		playerRecovered = playerRecovered || playerHP > beforePlayer
		petRecovered = petRecovered || petHP > beforePet
		beforePlayer, beforePet = playerHP, petHP
		return playerRecovered && petRecovered
	})
	t.Log("群疗双方真实分跳完成")
	tao.close()

	waveRole := create(3, 1)
	waveID := domain.Hex(waveRole.Key.RoleId)
	wave := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, waveRole.Key))
	defer func() { wave.close() }()
	wave.auto(false, 1)
	wave.buyAndLearnSkill(cfg.Fixture, 23)
	time.Sleep(time.Duration(cfg.Fixture.Players[0].MoveMS+100) * time.Millisecond)
	// 先清除占据站位的首怪，再正常等待其10秒重生；不把寻路穿活怪当测试前提。
	wave.castAccepted(23, 0, wave.entity(10).Position)
	wave.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && wave.entity(10).Hp == 0 })
	wave.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: wave.entry.SceneEpoch, Destination: &pb.GridPoint{X: 7, Y: 7}})
	wave.until(func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && wave.player().Position.X == 7 && wave.player().Position.Y == 7
	})
	time.Sleep(time.Duration(cfg.Fixture.Players[0].MoveMS+100) * time.Millisecond)
	wave.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && wave.entity(10).Hp > 0 })
	// 仅随机测试库设置本波19杀合法前态，再经真实购书事务同步owner，避免测试缓存写穿。
	_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": waveID}, bson.M{"$set": bson.M{"no_equipment_kills": cfg.Fixture.Drops.EquipmentGuarantee - 1}})
	must(t, err)
	bookOffer := uint32(0)
	waveSkill, _ := cfg.Fixture.Skill(23)
	for _, offer := range cfg.Fixture.SkillShop {
		if offer.ItemID == waveSkill.BookItemID && offer.Currency == "gold" {
			bookOffer = offer.ID
		}
	}
	prepared := wave.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: bookOffer, Quantity: 1})
	if result := wave.skillConfirm(prepared); result.ErrorCode != 0 {
		t.Fatal("本波owner前态同步失败")
	}
	wave.auto(false, 8)
	t.Log("准备同帧八杀")
	before, err := inspection.Role(ctx, waveID)
	must(t, err)
	wave.castAccepted(23, 0, &pb.GridPoint{X: 7, Y: 7})
	wave.until(func(f wire.Frame) bool {
		if f.ID != p.MsgS2CSceneBaseline {
			return false
		}
		dead := 0
		for _, entity := range wave.scene.Entities {
			if entity.EntityKind == 3 && entity.Hp == 0 {
				dead++
			}
		}
		return dead == 8
	})
	type audit struct {
		SourceSeq  int64        `bson:"source_seq"`
		At         time.Time    `bson:"event_at"`
		Bound      bool         `bson:"bound"`
		Target     uint32       `bson:"target_handle"`
		Item       *domain.Item `bson:"item"`
		Experience int64        `bson:"experience"`
	}
	var journal []audit
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
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
	counter := before.NoEquipmentKills
	for i, row := range journal {
		if row.SourceSeq != before.SourceSeq+int64(i)+1 || row.Target != uint32(10+i) || row.Bound || row.At.IsZero() || !row.At.Equal(journal[0].At) {
			t.Fatal("逐杀序号/稳定目标次序/killAt/绑定不符")
		}
		counter++
		if row.Item != nil {
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
			definition, _ := cfg.Fixture.Item(row.Item.ItemID)
			if !definition.IsEquipment {
				t.Fatal("保底没有真实发装备")
			}
		}
	}
	after, err := inspection.Role(ctx, waveID)
	must(t, err)
	if after.SourceSeq != before.SourceSeq+8 || after.NoEquipmentKills != counter || after.Experience != before.Experience+8*cfg.Fixture.Monsters[0].Exp {
		t.Fatal("多杀永久经验/序号/保底未逐事件守恒")
	}
	scene := wave.entry.SceneEpoch
	wave.close()
	wave = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, waveRole.Key))
	wave.auto(false, 8)
	again, err := inspection.Role(ctx, waveID)
	must(t, err)
	if wave.entry.SceneEpoch != scene || again.SourceSeq != after.SourceSeq || again.Wallet != after.Wallet || len(again.Items) != len(after.Items) {
		t.Fatal("健康重连重播多杀收益")
	}
	t.Log("通过：NPC学习新增五项、两单体实际耗蓝命中、Spirit零级与群疗受伤友方分跳、八杀逐事件source/保底/killAt/绑定/审计、重连不重奖")
}
