// 验证角色创建后的默认复活绑定、历史角色的补齐以及健康重连保持出生绑定。

package integration

import (
	"context"
	"go.mongodb.org/mongo-driver/v2/bson"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"os"
	"slices"
	"sync"
	"testing"
	"time"
)

// TestRoleBindingInitializationBackfillAndHealthyReconnectMongoWS 通过真实取票入场检查新角色默认绑定、旧角色补齐与健康重连。
// t 提供断言失败报告、子用例和清理注册。
func TestRoleBindingInitializationBackfillAndHealthyReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo/WS验证绑定初始化和旧角色接管迁移")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// 仅随机测试库启用未审定运行起点的业务合同；(0,0)故意与角色出生点不同。
	cfg.Fixture = bindingInitializationRule12Fixture(cfg.Fixture)
	cfg.Fixture.Binding = &devconfig.BindingRules{Enabled: true, StartPoints: []devconfig.BindingPoint{{MapID: cfg.Fixture.Map.ID, X: 0, Y: 0}}}
	must(t, cfg.Fixture.Validate())
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
				t.Errorf("绑定%s：%v", mode, err)
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
	// address 接收遍历文本列表时当前的元素。
	for _, address := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, address)
	}
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// login 保存当前处理所用的登录。
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "bind_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	// slot 定位文本列表中正在处理的成员，scenario 接收遍历文本列表时当前的元素。
	for slot, scenario := range []string{"created", "legacy_missing", "existing", "invalid_map"} {
		// create 保存当前处理所用的创建。
		create := &pb.CreateRoleRequest{OriginServerId: 1, Slot: uint32(slot + 1), Job: 0, Name: "绑定" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())}
		// role 保存当前处理所用的角色，replay 保存当前处理所用的重放。
		role, replay := &pb.RoleSummary{}, &pb.RoleSummary{}
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, create, role, 0)
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, create, replay, 0)
		// id 区分当前步骤指向的目标对象。
		id := domain.Hex(role.Key.RoleId)
		if domain.Hex(replay.Key.RoleId) != id {
			t.Fatal("建角重传产生新角色")
		}
		// stored 保存由角色取得的已持久保存，err 接收角色过程中可能产生的错误，供后续失败分支处理。
		stored, err := inspection.Role(ctx, id)
		must(t, err)
		if stored.Binding == nil || *stored.Binding != (domain.RoleBinding{MapID: cfg.Fixture.Map.ID}) || stored.Revision != 1 || stored.EconomicSeq != 0 {
			t.Fatal("绑定未与初始角色原子创建，或重传推进了永久流水", stored.Binding)
		}
		// expected 保存当前处理所用的预期结果。
		expected := *stored.Binding
		switch scenario {
		case "legacy_missing":
			_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": id, "revision": 1}, bson.M{"$unset": bson.M{"binding": ""}})
		case "existing":
			expected.X, expected.Y = 2, 2
			_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": id, "revision": 1}, bson.M{"$set": bson.M{"binding": expected}})
		case "invalid_map":
			expected.MapID = math.MaxInt64
			_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": id, "revision": 1}, bson.M{"$set": bson.M{"binding": expected}})
		}
		must(t, err)
		// ticket 保存由签发取得的入场票据。
		ticket := issue(t, endpoint, login.AccountToken, role.Key)
		if scenario == "invalid_map" {
			assertEntryRejectionFrame(t, cfg.GameAddress, ticket, p.CodeLineForbidden)
			healthy(t, cfg.GameAddress)
			stored, err = inspection.Role(ctx, id)
			must(t, err)
			if stored.Revision != 1 || stored.SessionEpoch != 0 || stored.Binding == nil || *stored.Binding != expected {
				t.Fatal("无效绑定拒绝仍接管或覆盖角色")
			}
			// used 保存由数量文档取得的“used”，err 接收数量文档过程中可能产生的错误，供后续失败分支处理。
			used, err := inspection.DB.Collection("entry_tickets").CountDocuments(ctx, bson.M{"role_id": id, "used": true})
			must(t, err)
			if used != 0 {
				t.Fatal("拒绝绑定仍消耗了入场票据")
			}
			continue
		}
		// c 接收加入供后续步骤使用。
		c := join(t, cfg.GameAddress, ticket)
		c.auto(false, 1)
		// beforeScene 保存场景代次，用于当前步骤中的操作前的状态场景，beforeLife 保存生命代次，用于当前步骤中的操作前的状态生命代次。
		beforeScene, beforeLife := c.entry.SceneEpoch, c.player().Life
		// 实际场景位置不等于绑定；入场与普通走动不把当前格持久化为绑定。
		if c.player().Position.X == expected.X && c.player().Position.Y == expected.Y {
			t.Fatal("本测试未区分当前坐标和绑定坐标")
		}
		stored, err = inspection.Role(ctx, id)
		must(t, err)
		if stored.Binding == nil || *stored.Binding != expected || stored.EconomicSeq != 0 {
			t.Fatal("初始化/旧角色补齐/既有绑定保留不符")
		}
		c.close()
		c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
		c.auto(false, 1)
		if c.entry.SceneEpoch != beforeScene || c.player().Life != beforeLife {
			t.Fatal("健康重连为初始化绑定重建了战斗现场")
		}
		c.close()
		// document 记录文档，使用MongoDB BSON 文档表示。
		var document bson.M
		must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": id}).Decode(&document))
		// key 接收遍历文本列表时当前的元素。
		for _, key := range []string{"position", "current_map_id", "hp", "mp", "scene", "casts", "skill_cooldowns"} {
			// exists 保存从文档取出的存在。
			if _, exists := document[key]; exists {
				t.Fatal("绑定业务额外持久化临时现场：", key)
			}
		}
		stored, err = inspection.Role(ctx, id)
		must(t, err)
		if stored.Binding == nil || *stored.Binding != expected {
			t.Fatal("健康重连覆盖永久绑定")
		}
	}
}

// 从当前唯一输入派生受测的rule12合同，不把后来的瞬移/城镇运行字段带回旧版本。
// 只改变本例随机库夹具；生产Validate及唯一Excel保持原样。
// f 记录开发测试配置。
func bindingInitializationRule12Fixture(f devconfig.Fixture) devconfig.Fixture {
	// teleport 保存由技能取得的瞬移。
	teleport, _ := f.Skill(21)
	// bookID 标识当前业务使用的技能书，buffID 标识当前业务使用的“buff”。
	bookID, buffID := teleport.BookItemID, teleport.BuffID
	f.RuleVersion = 12
	if f.Map.Width == 13 {
		f.Map.Width, f.Map.Height = 17, 17
	}
	f.Warehouses = nil                                                                                    // 历史rule12不携带rule23仓储合同。
	f.Progression = devconfig.ProgressionRules{ExpPerLevel: slices.Clone(f.Progression.ExpPerLevel[:20])} // 保留历史20阈值，不混入rule20上限/池合同。
	f.EquipmentRecycling = nil                                                                            // rule19开发回收合同不能带回历史rule12初始化样本。
	f.PetRuntime = nil                                                                                    // 历史初始化合同不混入rule16生命周期能力。
	f.NPCShops, f.EquipmentDurability = nil, nil
	if f.Binding != nil {
		// binding 保存当前处理所用的会话绑定。
		binding := *f.Binding
		binding.SafeZoneEnabled = nil
		f.Binding = &binding
	}
	f.TownTravel, f.TownNavigation = nil, nil
	f.Map.NoTeleport, f.Map.SourceStaticBits = nil, nil
	f.Map.NoPets = nil
	// m 记录地图模板。
	f.Maps = slices.DeleteFunc(slices.Clone(f.Maps), func(m devconfig.MapTemplate) bool { return m.CompactTown })
	// i 标记遍历地图集合时访问的下标。
	for i := range f.Maps {
		f.Maps[i].NoTeleport, f.Maps[i].SourceStaticBits = nil, nil
		f.Maps[i].NoPets = nil
	}
	// s 记录技能。
	f.Skills = slices.DeleteFunc(f.Skills, func(s devconfig.Skill) bool { return s.ID == 21 })
	// b 记录b，使用状态效果定义表示。
	f.Buffs = slices.DeleteFunc(f.Buffs, func(b devconfig.BuffDefinition) bool { return b.ID == buffID || b.Kind == "teleport_mana_penalty" })
	// item 记录道具。
	f.Items = slices.DeleteFunc(f.Items, func(item devconfig.Item) bool { return item.ID == bookID || item.BookSkillID == 21 })
	// row 记录行记录，使用技能升级表示。
	f.SkillUpgrades = slices.DeleteFunc(f.SkillUpgrades, func(row devconfig.SkillUpgrade) bool { return row.SkillID == 21 })
	// row 记录行记录，使用技能商店表示。
	f.SkillShop = slices.DeleteFunc(f.SkillShop, func(row devconfig.SkillShop) bool { return row.ItemID == bookID })
	// row 记录行记录，使用技能技能书回收表示。
	f.SkillBookRecycle = slices.DeleteFunc(f.SkillBookRecycle, func(row devconfig.SkillBookRecycle) bool { return row.ItemID == bookID })
	// row 记录行记录，使用初始值技能技能书表示。
	f.InitialSkillBooks = slices.DeleteFunc(f.InitialSkillBooks, func(row devconfig.InitialSkillBook) bool { return row.ItemID == bookID })
	// id 区分当前步骤指向的目标对象。
	f.InitialInventory = slices.DeleteFunc(f.InitialInventory, func(id uint32) bool { return id == bookID })
	// id 区分当前步骤指向的目标对象。
	f.Drops.ItemIDs = slices.DeleteFunc(f.Drops.ItemIDs, func(id uint32) bool { return id == bookID })
	return f
}
