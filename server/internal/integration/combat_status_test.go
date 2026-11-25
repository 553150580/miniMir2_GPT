// 回归战斗伤害、增减益、施法材料回执和重连后的运行态关系。

package integration

import (
	"context"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"os"
	"sync"
	"testing"
	"time"
)

// entityStatus 返回实体状态供调用方使用。
// entity 记录实体，使用实体状态表示，id 区分当前步骤指向的目标对象。
func entityStatus(entity *pb.EntityState, id uint32) *pb.EntityStatusState {
	// status 接收遍历“get”状态集合时当前的元素。
	for _, status := range entity.GetStatuses() {
		if status.StatusId == id {
			return status
		}
	}
	return nil
}

// TestCombatStatusThreeSkillsMaterialReceiptsAndReconnectMongoWS 比较三种状态技能的施法材料、战斗状态与重连后的持久和临时结果。
// t 提供断言失败报告、子用例和清理注册。
func TestCombatStatusThreeSkillsMaterialReceiptsAndReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("真实状态/材料/重连需显式MINIMIR_MONGO_E2E=1")
	}
	// enabled 接收遍历布尔状态列表时当前的元素。
	for _, enabled := range []bool{false, true} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(map[bool]string{false: "runtime_disabled", true: "isolated_material_units"}[enabled], func(t *testing.T) {
			// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
			cfg, err := loadIntegrationConfig()
			must(t, err)
			// 保留原骷髅测试伙伴，验证幽灵盾可同时改变角色与宠物的防御。
			cfg.Fixture.PetRuntime.BootstrapMode = "legacy_untrained_pet"
			cfg.Database = "minimir_dev_game_test_" + domain.ID()
			cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
			// 仅随机测试实例声明小正数数量单位，未启用或修改唯一运行Excel的护符价格/包数。
			cfg.Fixture.CastingSupply.Enabled = enabled
			if enabled {
				// i 标记遍历道具集合时访问的下标。
				for i := range cfg.Fixture.Items {
					if cfg.Fixture.Items[i].ID == 187 {
						cfg.Fixture.Items[i].MaxStack = 4
					}
				}
			}
			// i 标记遍历怪物集合时访问的下标。
			for i := range cfg.Fixture.Monsters {
				// m 保存当前处理所用的怪物集合中的指定项。
				m := &cfg.Fixture.Monsters[i]
				m.AttackMin, m.AttackMax, m.MoveMS, m.AttackMS = 0, 0, 600000, 600000
			}
			// ctx 保存由“background”取得的上下文。
			ctx := context.Background()
			// inspection 保存由打开取得的检查。
			inspection, err := persistence.Open(ctx, cfg)
			must(t, err)
			// launch 封装启动对应的后续处理。
			launch := func() func() {
				// run 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
				run, stop := context.WithCancel(ctx)
				// wait 记录等待，使用协程汇合计数器表示。
				var wait sync.WaitGroup
				// mode 接收遍历文本列表时当前的元素。
				for _, mode := range []string{"account", "game", "gm"} {
					wait.Go(func() {
						// err 接收运行批次过程中可能产生的错误，供后续失败分支处理。
						if err := bootstrap.Run(run, cfg, mode); err != nil && run.Err() == nil {
							t.Errorf("状态%s:%v", mode, err)
						}
					})
				}
				// a 接收遍历文本列表时当前的元素。
				for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
					healthy(t, a)
				}
				return func() { stop(); wait.Wait() }
			}
			// stop 保存由启动取得的停止。
			stop := launch()
			t.Cleanup(func() {
				stop()
				// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
				cleanup, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				must(t, inspection.DB.Drop(cleanup))
				inspection.Close(cleanup)
			})
			// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
			endpoint := "http://" + cfg.AccountAddress + "/rpc"
			// login 保存当前处理所用的登录。
			login := &pb.LoginResult{}
			rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "status_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
			// create 封装创建对应的后续处理，slot 记录槽位，使用数值表示，job 记录任务或职业编号，使用数值表示。
			create := func(slot, job uint32) *pb.RoleSummary {
				// role 保存当前处理所用的角色。
				role := &pb.RoleSummary{}
				rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: job, Name: "状态" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
				// err 接收更新“one”过程中可能产生的错误，供后续失败分支处理。
				_, err := inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{"$set": bson.M{"level": 40, "wallet": domain.Wallet{Gold: 1000000}}})
				must(t, err)
				return role
			}
			// mageRole 保存由创建取得的“mage”角色，taoRole 保存由创建取得的“tao”角色。
			mageRole, taoRole := create(1, 1), create(2, 2)
			// supply 建立带初始值的道具，供本段继续处理。
			supply := domain.Item{ID: domain.ID(), ItemID: 187, Quantity: 4, Version: 1, Bound: true, SourceKind: "npc_shop", SourceID: "isolated_test_units"}
			_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(taoRole.Key.RoleId)}, bson.M{"$push": bson.M{"items": supply}})
			must(t, err)
			// mage 保存由加入取得的“mage”。
			mage := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, mageRole.Key))
			defer func() { mage.close() }()
			mage.auto(false, 1)
			mage.buyAndLearnSkill(cfg.Fixture, 31)
			time.Sleep(time.Duration(mage.player().MoveIntervalMs) * time.Millisecond)
			// mp 保存当前处理所用的魔法值。
			mp := mage.player().Mp
			// accepted 保存由施法已受理取得的已受理。
			accepted := mage.castAccepted(31, 0, &pb.GridPoint{X: 0, Y: 0})
			if accepted.DelayMs != 500 {
				t.Fatal("盾未使用500ms受理/生效分离")
			}
			// f 记录协议帧。
			mage.until(func(f wire.Frame) bool {
				return f.ID == p.MsgS2CSceneBaseline && entityStatus(mage.player(), 31) != nil
			})
			// status 保存由实体状态取得的状态。
			status := entityStatus(mage.player(), 31)
			// sequence 保存施法序号，用于当前步骤中的序号，scene 保存场景代次，用于当前步骤中的场景，mana 保存魔法值，用于当前步骤中的魔法值。
			sequence, scene, mana := mage.player().CastSequence, mage.entry.SceneEpoch, mage.player().Mp
			if status.DamageReductionPercent != 20 || mage.player().CombatAttributes.DamageReductionPercent != 20 || mana >= mp {
				t.Fatal("真实盾状态/属性或MP不一致")
			}
			mage.close()
			mage = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, mageRole.Key))
			mage.auto(false, 1)
			if mage.entry.SceneEpoch != scene || mage.player().CastSequence != sequence || mage.player().Mp != mana || entityStatus(mage.player(), 31) == nil || entityStatus(mage.player(), 31).RemainingMs > status.RemainingMs {
				t.Fatal("健康重入重复盾/重置时长/MP")
			}

			// tao 保存由加入取得的“tao”。
			tao := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, taoRole.Key))
			defer func() { tao.close() }()
			tao.auto(false, 1)
			tao.buyAndLearnSkill(cfg.Fixture, 14)
			tao.buyAndLearnSkill(cfg.Fixture, 15)
			time.Sleep(time.Duration(tao.player().MoveIntervalMs) * time.Millisecond)
			// equip 保存由资产取得的穿戴装备。
			equip := tao.asset(p.MsgC2SSetCastingSupply, &pb.SetCastingSupplyRequest{ItemInstanceId: domain.Bytes(supply.ID), ItemVersion: 1, Operation: tao.operation()})
			tao.inventoryAt(equip.NewRevision)
			if !enabled {
				if equip.ErrorCode != p.CodeSkillMaterialRequired {
					t.Fatal("未定单位护符被启用")
				}
				// request 保存当前处理所用的请求。
				request := &pb.CastWithMaterialRequest{SceneEpoch: tao.entry.SceneEpoch, CasterLife: tao.player().Life, SkillId: 14, TargetGrid: tao.player().Position, MaterialInstanceId: domain.Bytes(supply.ID), MaterialVersion: 1, Operation: tao.operation()}
				// result 保存由资产取得的结果。
				result := tao.asset(p.MsgC2SCastWithMaterial, request)
				tao.inventoryAt(result.NewRevision)
				if result.ErrorCode != p.CodeSkillMaterialRequired {
					t.Fatalf("disabled材料施法未明确拒绝:%v", result)
				}
				return
			}
			if equip.ErrorCode != 0 {
				t.Fatalf("独立测试装符失败:%v", equip)
			}
			// slot 封装槽位对应的后续处理。
			slot := func() *pb.ItemView {
				// e 接收遍历已穿戴装备时当前的元素。
				for _, e := range tao.inventory.Equipped {
					if e.Slot == 100 {
						return e.Item
					}
				}
				return nil
			}
			if slot() == nil || slot().Quantity != 4 || slot().Version != 2 {
				t.Fatal("专用槽未按实际单位同步")
			}
			// 普通装备RPC不得接受耗材槽；无材料施法不得绕过永久扣除。
			// illegal 保存由资产取得的“illegal”。
			illegal := tao.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: domain.Bytes(supply.ID), Slot: 100, Operation: tao.operation()})
			tao.inventoryAt(illegal.NewRevision)
			if illegal.ErrorCode == 0 {
				t.Fatal("普通装备接受slot100")
			}
			// requestID 标识当前业务使用的请求。
			requestID := tao.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: tao.entry.SceneEpoch, SkillId: 14, TargetGrid: tao.player().Position})
			tao.expectError(requestID, p.CodeSkillMaterialRequired)
			// badDirection 保存当前处理所用的“bad”朝向。
			badDirection := &pb.CastWithMaterialRequest{SceneEpoch: tao.entry.SceneEpoch, CasterLife: tao.player().Life, SkillId: 14, TargetGrid: tao.player().Position, Direction: 8, MaterialInstanceId: domain.Bytes(supply.ID), MaterialVersion: 2, Operation: tao.operation()}
			// badResult 保存由资产取得的“bad”结果。
			badResult := tao.asset(p.MsgC2SCastWithMaterial, badDirection)
			tao.inventoryAt(badResult.NewRevision)
			if badResult.ErrorCode != p.CodeSkillTarget || slot().Quantity != 4 || slot().Version != 2 {
				t.Fatal("非法八向材料请求仍扣除了护符")
			}
			// manual 保存当前处理所用的手动。
			manual := &pb.CastWithMaterialRequest{SceneEpoch: tao.entry.SceneEpoch, CasterLife: tao.player().Life, SkillId: 14, TargetGrid: tao.player().Position, MaterialInstanceId: domain.Bytes(supply.ID), MaterialVersion: 2, InputSequence: 100, Operation: tao.operation()}
			tao.input = 100
			// beforeSource 保存由角色取得的操作前的状态来源。
			beforeSource, err := inspection.Role(ctx, domain.Hex(taoRole.Key.RoleId))
			must(t, err)
			// first 保存由资产取得的第一项。
			first := tao.asset(p.MsgC2SCastWithMaterial, manual)
			tao.inventoryAt(first.NewRevision)
			if first.ErrorCode != 0 || slot().Quantity != 3 || slot().Version != 3 {
				t.Fatalf("手动材料未原子减1:%v", first)
			}
			// f 记录协议帧。
			tao.until(func(f wire.Frame) bool {
				return f.ID == p.MsgS2CSceneBaseline && entityStatus(tao.player(), 14) != nil && entityStatus(tao.entity(2), 14) != nil
			})
			if entityStatus(tao.entity(2), 14).MaxMagicDefenceBonus != 6 {
				t.Fatal("宠物实际Level15未产生MAC+6")
			}
			// castSequence 保存当前处理所用的施法序号。
			castSequence := tao.player().CastSequence
			// oldScene 保存场景代次，用于当前步骤中的原状态场景。
			oldScene := tao.entry.SceneEpoch
			tao.close()
			tao = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, taoRole.Key))
			tao.auto(false, 1)
			// replay 保存由资产取得的重放。
			replay := tao.asset(p.MsgC2SCastWithMaterial, manual)
			tao.inventoryAt(replay.NewRevision)
			if replay.ErrorCode != 0 || slot().Quantity != 3 || tao.player().CastSequence != castSequence || tao.entry.SceneEpoch != oldScene {
				t.Fatal("重连原回执重复施法/消耗")
			}
			requestID = tao.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: tao.entry.SceneEpoch, SkillId: 14, TargetGrid: tao.player().Position})
			for {
				// response 保存由读取协议帧取得的响应。
				response := tao.readFrame()
				if response.Request == requestID && response.Error != 0 {
					if response.Error != p.CodeSkillCooldown && response.Error != p.CodeSkillMaterialRequired && response.Error != p.CodeSkillStateBlocked {
						t.Fatalf("旧高输入污染新窗口：%v", response)
					}
					break
				}
			}
			// 重入很快时旧施法动作仍可合法阻挡；同一新输入再次发送必须被水位拒绝，且不产生第二次施法。
			// repeated 保存由发送取得的“repeated”。
			repeated := tao.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: tao.entry.SceneEpoch, SkillId: 14, TargetGrid: tao.player().Position, InputSequence: tao.input})
			tao.expectError(repeated, p.CodeStaleInput)
			if slot().Quantity != 3 || tao.player().CastSequence != castSequence {
				t.Fatal("新窗口的拒绝或重放仍消耗护符/重复施法")
			}
			// 原高输入100没有污染新binding序号；先等公共CD，再由Auto使用独立来源流水补缺失AC状态。
			time.Sleep(2 * time.Second)
			// economic 保存最高已推进序号，用于当前步骤中的经济操作。
			economic := tao.watermark
			tao.auto(true, 1)
			// f 记录协议帧。
			tao.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && entityStatus(tao.player(), 15) != nil })
			tao.auto(false, 1)
			// afterSource 保存由角色取得的操作后的状态来源。
			afterSource, err := inspection.Role(ctx, domain.Hex(taoRole.Key.RoleId))
			must(t, err)
			if afterSource.SourceSeq != beforeSource.SourceSeq+1 || afterSource.EconomicSeq != int64(economic) {
				t.Fatal("自动耗符未独立SourceSeq或偷推客户端经济流水")
			}
			if tao.inventory.Revision < uint64(afterSource.Revision) {
				// f 记录协议帧。
				tao.until(func(f wire.Frame) bool {
					return f.ID == p.MsgS2CInventory && tao.inventory.Revision >= uint64(afterSource.Revision)
				})
			}
			if slot().Quantity != 2 || tao.player().CombatAttributes.PhysicalDefenceMax <= tao.player().CombatAttributes.PhysicalDefenceMin {
				t.Fatal("自动实际状态或剩余耗符不同步")
			}
			// 无论重登/配置停用，永久旧回执都在当前门禁前查找，且回执不携带随机时长/战斗现场。
			// receipt 记录回执，使用MongoDB BSON 文档表示。
			var receipt bson.M
			must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + domain.Hex(taoRole.Key.RoleId) + ":" + domain.Hex(manual.Operation.OpId)}).Decode(&receipt))
			// encoded 保存由序列化取得的“encoded”。
			encoded, err := bson.Marshal(receipt["casting_use"])
			must(t, err)
			// use 记录使用，使用MongoDB BSON 文档表示。
			var use bson.M
			must(t, bson.Unmarshal(encoded, &use))
			if len(use) != 4 {
				t.Fatalf("消耗回执混入临时状态:%v", use)
			}
			// document 记录文档，使用MongoDB BSON 文档表示。
			var document bson.M
			must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": domain.Hex(taoRole.Key.RoleId)}).Decode(&document))
			// key 接收遍历文本列表时当前的元素。
			for _, key := range []string{"statuses", "buffs", "hp", "mp", "casts", "next_spell", "skill_cooldowns", "input_sequence"} {
				// found 保存从文档取出的已找到标记。
				if _, found := document[key]; found {
					t.Fatalf("临时字段%s入库", key)
				}
			}
			// 真实节点重启丢弃旧状态；旧场景原op只核对永久事实，不能给新场景补施法。
			mage.close()
			tao.close()
			stop()
			cfg.Fixture.CastingSupply.Enabled = false
			stop = launch()
			tao = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, taoRole.Key))
			tao.auto(false, 1)
			if tao.entry.SceneEpoch == oldScene || entityStatus(tao.player(), 14) != nil || entityStatus(tao.player(), 15) != nil {
				t.Fatal("故障新scene复活旧Buff")
			}
			sequence = tao.player().CastSequence
			replay = tao.asset(p.MsgC2SCastWithMaterial, manual)
			tao.inventoryAt(replay.NewRevision)
			if replay.ErrorCode != 0 || slot().Quantity != 2 || tao.player().CastSequence != sequence {
				t.Fatal("新scene旧消费回执补随机效果")
			}
			// changed 按目标类型读取当前接口值，得到“changed”。
			changed := proto.Clone(manual).(*pb.CastWithMaterialRequest)
			changed.MaterialVersion++
			// result 保存由资产取得的结果。
			if result := tao.asset(p.MsgC2SCastWithMaterial, changed); result.ErrorCode != p.CodeOperationConflict {
				t.Fatal("原op改材料版本未拒绝")
			}
		})
	}
}
