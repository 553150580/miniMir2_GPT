package integration

import (
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
	"sync"
	"testing"
	"time"
)

func entityStatus(entity *pb.EntityState, id uint32) *pb.EntityStatusState {
	for _, status := range entity.GetStatuses() {
		if status.StatusId == id {
			return status
		}
	}
	return nil
}

func TestCombatStatusThreeSkillsMaterialReceiptsAndReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("真实状态/材料/重连需显式MINIMIR_MONGO_E2E=1")
	}
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "runtime_disabled", true: "isolated_material_units"}[enabled], func(t *testing.T) {
			cfg, err := devconfig.Load("../../../conf/dev/server.json")
			must(t, err)
			cfg.Database = "minimir_dev_game_test_" + domain.ID()
			cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
			// 仅随机测试实例声明小正数数量单位，未启用或修改唯一运行Excel的护符价格/包数。
			cfg.Fixture.CastingSupply.Enabled = enabled
			if enabled {
				for i := range cfg.Fixture.Items {
					if cfg.Fixture.Items[i].ID == 187 {
						cfg.Fixture.Items[i].MaxStack = 4
					}
				}
			}
			for i := range cfg.Fixture.Monsters {
				m := &cfg.Fixture.Monsters[i]
				m.AttackMin, m.AttackMax, m.MoveMS, m.AttackMS = 0, 0, 600000, 600000
			}
			ctx := context.Background()
			inspection, err := persistence.Open(ctx, cfg)
			must(t, err)
			launch := func() func() {
				run, stop := context.WithCancel(ctx)
				var wait sync.WaitGroup
				for _, mode := range []string{"account", "game", "gm"} {
					wait.Go(func() {
						if err := bootstrap.Run(run, cfg, mode); err != nil && run.Err() == nil {
							t.Errorf("状态%s:%v", mode, err)
						}
					})
				}
				for _, a := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
					healthy(t, a)
				}
				return func() { stop(); wait.Wait() }
			}
			stop := launch()
			t.Cleanup(func() {
				stop()
				cleanup, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				must(t, inspection.DB.Drop(cleanup))
				inspection.Close(cleanup)
			})
			endpoint := "http://" + cfg.AccountAddress + "/rpc"
			login := &pb.LoginResult{}
			rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "status_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
			create := func(slot, job uint32) *pb.RoleSummary {
				role := &pb.RoleSummary{}
				rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: job, Name: "状态" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
				_, err := inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{"$set": bson.M{"level": 40, "wallet": domain.Wallet{Gold: 1000000}}})
				must(t, err)
				return role
			}
			mageRole, taoRole := create(1, 1), create(2, 2)
			supply := domain.Item{ID: domain.ID(), ItemID: 187, Quantity: 4, Version: 1, Bound: true, SourceKind: "npc_shop", SourceID: "isolated_test_units"}
			_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(taoRole.Key.RoleId)}, bson.M{"$push": bson.M{"items": supply}})
			must(t, err)
			mage := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, mageRole.Key))
			defer func() { mage.close() }()
			mage.auto(false, 1)
			mage.buyAndLearnSkill(cfg.Fixture, 31)
			time.Sleep(time.Duration(mage.player().MoveIntervalMs) * time.Millisecond)
			mp := mage.player().Mp
			accepted := mage.castAccepted(31, 0, &pb.GridPoint{X: 0, Y: 0})
			if accepted.DelayMs != 500 {
				t.Fatal("盾未使用500ms受理/生效分离")
			}
			mage.until(func(f wire.Frame) bool {
				return f.ID == p.MsgS2CSceneBaseline && entityStatus(mage.player(), 31) != nil
			})
			status := entityStatus(mage.player(), 31)
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

			tao := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, taoRole.Key))
			defer func() { tao.close() }()
			tao.auto(false, 1)
			tao.buyAndLearnSkill(cfg.Fixture, 14)
			tao.buyAndLearnSkill(cfg.Fixture, 15)
			time.Sleep(time.Duration(tao.player().MoveIntervalMs) * time.Millisecond)
			equip := tao.asset(p.MsgC2SSetCastingSupply, &pb.SetCastingSupplyRequest{ItemInstanceId: domain.Bytes(supply.ID), ItemVersion: 1, Operation: tao.operation()})
			tao.inventoryAt(equip.NewRevision)
			if !enabled {
				if equip.ErrorCode != p.CodeSkillMaterialRequired {
					t.Fatal("未定单位护符被启用")
				}
				request := &pb.CastWithMaterialRequest{SceneEpoch: tao.entry.SceneEpoch, CasterLife: tao.player().Life, SkillId: 14, TargetGrid: tao.player().Position, MaterialInstanceId: domain.Bytes(supply.ID), MaterialVersion: 1, Operation: tao.operation()}
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
			slot := func() *pb.ItemView {
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
			illegal := tao.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: domain.Bytes(supply.ID), Slot: 100, Operation: tao.operation()})
			tao.inventoryAt(illegal.NewRevision)
			if illegal.ErrorCode == 0 {
				t.Fatal("普通装备接受slot100")
			}
			requestID := tao.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: tao.entry.SceneEpoch, SkillId: 14, TargetGrid: tao.player().Position})
			tao.expectError(requestID, p.CodeSkillMaterialRequired)
			badDirection := &pb.CastWithMaterialRequest{SceneEpoch: tao.entry.SceneEpoch, CasterLife: tao.player().Life, SkillId: 14, TargetGrid: tao.player().Position, Direction: 8, MaterialInstanceId: domain.Bytes(supply.ID), MaterialVersion: 2, Operation: tao.operation()}
			badResult := tao.asset(p.MsgC2SCastWithMaterial, badDirection)
			tao.inventoryAt(badResult.NewRevision)
			if badResult.ErrorCode != p.CodeSkillTarget || slot().Quantity != 4 || slot().Version != 2 {
				t.Fatal("非法八向材料请求仍扣除了护符")
			}
			manual := &pb.CastWithMaterialRequest{SceneEpoch: tao.entry.SceneEpoch, CasterLife: tao.player().Life, SkillId: 14, TargetGrid: tao.player().Position, MaterialInstanceId: domain.Bytes(supply.ID), MaterialVersion: 2, InputSequence: 100, Operation: tao.operation()}
			tao.input = 100
			beforeSource, err := inspection.Role(ctx, domain.Hex(taoRole.Key.RoleId))
			must(t, err)
			first := tao.asset(p.MsgC2SCastWithMaterial, manual)
			tao.inventoryAt(first.NewRevision)
			if first.ErrorCode != 0 || slot().Quantity != 3 || slot().Version != 3 {
				t.Fatalf("手动材料未原子减1:%v", first)
			}
			tao.until(func(f wire.Frame) bool {
				return f.ID == p.MsgS2CSceneBaseline && entityStatus(tao.player(), 14) != nil && entityStatus(tao.entity(2), 14) != nil
			})
			if entityStatus(tao.entity(2), 14).MaxMagicDefenceBonus != 6 {
				t.Fatal("宠物实际Level15未产生MAC+6")
			}
			castSequence := tao.player().CastSequence
			oldScene := tao.entry.SceneEpoch
			tao.close()
			tao = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, taoRole.Key))
			tao.auto(false, 1)
			replay := tao.asset(p.MsgC2SCastWithMaterial, manual)
			tao.inventoryAt(replay.NewRevision)
			if replay.ErrorCode != 0 || slot().Quantity != 3 || tao.player().CastSequence != castSequence || tao.entry.SceneEpoch != oldScene {
				t.Fatal("重连原回执重复施法/消耗")
			}
			requestID = tao.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: tao.entry.SceneEpoch, SkillId: 14, TargetGrid: tao.player().Position})
			for {
				response := tao.readFrame()
				if response.Request == requestID && response.Error != 0 {
					if response.Error != p.CodeSkillCooldown && response.Error != p.CodeSkillMaterialRequired {
						t.Fatalf("旧高输入污染新窗口：%v", response)
					}
					break
				}
			}
			// 原高输入100没有污染新binding序号；先等公共CD，再由Auto使用独立来源流水补缺失AC状态。
			time.Sleep(2 * time.Second)
			economic := tao.watermark
			tao.auto(true, 1)
			tao.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && entityStatus(tao.player(), 15) != nil })
			tao.auto(false, 1)
			afterSource, err := inspection.Role(ctx, domain.Hex(taoRole.Key.RoleId))
			must(t, err)
			if afterSource.SourceSeq != beforeSource.SourceSeq+1 || afterSource.EconomicSeq != int64(economic) {
				t.Fatal("自动耗符未独立SourceSeq或偷推客户端经济流水")
			}
			if tao.inventory.Revision < uint64(afterSource.Revision) {
				tao.until(func(f wire.Frame) bool {
					return f.ID == p.MsgS2CInventory && tao.inventory.Revision >= uint64(afterSource.Revision)
				})
			}
			if slot().Quantity != 2 || tao.player().CombatAttributes.PhysicalDefenceMax <= tao.player().CombatAttributes.PhysicalDefenceMin {
				t.Fatal("自动实际状态或剩余耗符不同步")
			}
			// 无论重登/配置停用，永久旧回执都在当前门禁前查找，且回执不携带随机时长/战斗现场。
			var receipt bson.M
			must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + domain.Hex(taoRole.Key.RoleId) + ":" + domain.Hex(manual.Operation.OpId)}).Decode(&receipt))
			encoded, err := bson.Marshal(receipt["casting_use"])
			must(t, err)
			var use bson.M
			must(t, bson.Unmarshal(encoded, &use))
			if len(use) != 4 {
				t.Fatalf("消耗回执混入临时状态:%v", use)
			}
			var document bson.M
			must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": domain.Hex(taoRole.Key.RoleId)}).Decode(&document))
			for _, key := range []string{"statuses", "buffs", "hp", "mp", "casts", "next_spell", "skill_cooldowns", "input_sequence"} {
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
			changed := proto.Clone(manual).(*pb.CastWithMaterialRequest)
			changed.MaterialVersion++
			if result := tao.asset(p.MsgC2SCastWithMaterial, changed); result.ErrorCode != p.CodeOperationConflict {
				t.Fatal("原op改材料版本未拒绝")
			}
		})
	}
}
