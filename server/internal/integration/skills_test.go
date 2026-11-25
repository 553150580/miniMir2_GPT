// 通过真实入场和技能消息验证学习、技能列表、施法与战斗状态。

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

// expectError 处理“expect”错误。
// c 是调用该方法的客户端实例，request 关联客户端请求与服务端的对应响应，code 记录结果码，使用数值表示。
func (c *client) expectError(request, code uint32) {
	c.t.Helper()
	for {
		// f 接收读取协议帧供后续步骤使用。
		f := c.readFrame()
		if f.Error != 0 {
			if f.Request != request || f.Error != code {
				c.t.Fatalf("请求%d错误=%d，预期%d: %+v", request, f.Error, code, f)
			}
			return
		}
	}
}

// entity 返回实体供调用方使用。
// c 是调用该方法的客户端实例，handle 记录实体句柄，使用数值表示。
func (c *client) entity(handle uint32) *pb.EntityState {
	// e 接收遍历“get”实体时当前的元素。
	for _, e := range c.scene.GetEntities() {
		if e.EntityHandle == handle {
			return e
		}
	}
	return nil
}

// TestSkillLearningAndCastingMongoWS 让角色通过正常协议消耗技能书、确认已学状态并施放技能。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillLearningAndCastingMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以验证三职业技能真实Mongo/WS")
	}
	// job 接收遍历数值列表时当前的元素。
	for _, job := range []uint32{0, 1, 2} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run([]string{"fencing", "fireball", "healing"}[job], func(t *testing.T) {
			// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
			cfg, err := loadIntegrationConfig()
			must(t, err)
			if job == 2 {
				// 治疗回归保留旧式受伤宠物前态。规则29的新角色召唤由单独用例验证。
				cfg.Fixture.PetRuntime.BootstrapMode = "legacy_untrained_pet"
			}
			cfg.Database = "minimir_dev_game_test_" + domain.ID()
			cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
			// 仅隔离测试实例调整交战位置/耐久，以确定性观察延迟和宠物恢复；配置文件未修改。
			cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: 4, Y: 8}
			if job == 1 {
				// 本例核学习/投射命中，不用法师在资产RPC期间被怪物击杀来决定施法前态。
				cfg.Fixture.Monsters[0].AttackMin, cfg.Fixture.Monsters[0].AttackMax = 0, 0
			}
			if job == 2 {
				// 首入自动的一步也不能使玩家与宠物同距；否则怪物会转打玩家，受伤宠物前态永远不成立。
				cfg.Fixture.Map.PlayerSpawn = devconfig.Point{X: 8, Y: 10}
				cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: 8, Y: 6}
				cfg.Fixture.Monsters[0].HP = cfg.Fixture.Pet.AttackMax * 10
				cfg.Fixture.Monsters[0].AttackMin = cfg.Fixture.Pet.DefenceMax + cfg.Fixture.Healing.PetBasePerTick*2
				cfg.Fixture.Monsters[0].AttackMax = cfg.Fixture.Monsters[0].AttackMin
				cfg.Fixture.Monsters[0].Accuracy = cfg.Fixture.Pet.Agility
				cfg.Fixture.Monsters[0].Agility = 0
			}
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
						t.Errorf("技能测试%s: %v", mode, err)
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
			rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "skill_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
			// create 保存当前处理所用的创建。
			create := &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: job, Name: "技能" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}
			// role 保存当前处理所用的角色。
			role := &pb.RoleSummary{}
			rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, create, role, 0)
			// replayRole 保存当前处理所用的重放角色。
			replayRole := &pb.RoleSummary{}
			rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, create, replayRole, 0)
			// roleID 标识当前业务使用的角色。
			roleID := domain.Hex(role.Key.RoleId)
			// persisted 保存由角色取得的“persisted”。
			persisted, err := inspection.Role(ctx, roleID)
			must(t, err)
			// skill 接收技能供后续步骤使用。
			skill, _ := cfg.Fixture.Skill(map[uint32]uint32{0: 3, 1: 1, 2: 2}[job])
			// book 记录技能书，使用道具表示。
			var book domain.Item
			// bookCount 规定技能书数量的初始取值。
			bookCount := 0
			// item 接收遍历道具集合时当前的元素。
			for _, item := range persisted.Items {
				if item.ItemID == skill.BookItemID {
					book = item
					bookCount++
				}
			}
			if bookCount != 1 || book.Quantity != 1 || len(persisted.Skills) != 0 {
				t.Fatal("新角色/建角重传未精确发一本职业书，或直接授予技能")
			}
			// 唯一随机测试库把待测角色准备至开发学习门槛；不接触现有开发库。
			_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{"level": skill.LearnLevel}})
			must(t, err)
			// monsterHit 标记是否处于怪物命中状态，petHit 标记是否处于宠物命中状态。
			var monsterHit, petHit bool
			// observeImmediate 封装观测“immediate”对应的后续处理，c 记录客户端，frame 记录协议帧。
			observeImmediate := func(c *client, frame wire.Frame) {
				if job != 2 || frame.ID != p.MsgS2CCombatImpact {
					return
				}
				// hit 记录命中，使用战斗命中影响表示。
				var hit pb.CombatImpact
				must(t, proto.Unmarshal(frame.Payload, &hit))
				// monsterAttacks 保存根据施法者实体句柄与当前数据的计算结果与目标实体句柄与当前数据的计算结果计算的怪物攻击集合。
				monsterAttacks := hit.CasterHandle == 10 && hit.TargetHandle == 2
				// petAttacks 保存根据施法者实体句柄与当前数据的计算结果与目标实体句柄与当前数据的计算结果计算的宠物攻击集合。
				petAttacks := hit.CasterHandle == 2 && hit.TargetHandle == 10
				if !monsterAttacks && !petAttacks {
					return
				}
				// target 保存由实体取得的目标。
				target := c.entity(hit.TargetHandle)
				if hit.SceneEpoch != c.entry.SceneEpoch || hit.EventId == 0 || hit.AttackSequence == 0 || hit.TargetGrid == nil ||
					hit.CastId != 0 || hit.SkillId != 0 || hit.Amount <= 0 || hit.Healing || hit.Outcome != 1 ||
					target == nil || hit.TargetLife != target.Life {
					t.Fatal("怪物或宠物即时扣血事件身份错误", &hit)
				}
				monsterHit = monsterHit || monsterAttacks
				petHit = petHit || petAttacks
			}
			// c 接收加入供后续步骤使用。
			c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), observeImmediate)
			defer func() { c.close() }()
			c.auto(false, 1)
			// 关闭自动不取消已受理移动；按协议实际间隔等待首入动作结束，保留服务端门禁。
			time.Sleep(time.Duration(c.player().MoveIntervalMs) * time.Millisecond)
			// use 封装使用对应的后续处理，q 记录学习技能请求。
			use := func(q *pb.LearnSkillRequest) *pb.OperationResult {
				// result 保存由资产取得的结果。
				result := c.asset(p.MsgC2SLearnSkill, q)
				c.inventoryAt(result.NewRevision)
				return result
			}
			// 错职业与不存在的另一角色书身份均形成可重放失败，不消耗真实职业书。
			// wrongSkill 保存由无符号整数“32”取得的“wrong”技能。
			wrongSkill := uint32(1)
			if skill.ID == 1 {
				wrongSkill = 2
			}
			// wrong 保存由使用取得的“wrong”。
			wrong := use(&pb.LearnSkillRequest{ItemInstanceId: domain.Bytes(book.ID), SkillId: wrongSkill, Operation: c.operation()})
			if wrong.ErrorCode != p.CodeInvalidArgument {
				t.Fatal("错职业书学习未拒绝")
			}
			// other 保存当前处理所用的另一项。
			other := &pb.RoleSummary{}
			rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 2, Job: job, Name: "他人书" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, other, 0)
			// otherRole 保存由角色取得的另一项角色。
			otherRole, err := inspection.Role(ctx, domain.Hex(other.Key.RoleId))
			must(t, err)
			// otherBook 记录另一项技能书，使用道具表示。
			var otherBook domain.Item
			// item 接收遍历道具集合时当前的元素。
			for _, item := range otherRole.Items {
				if item.ItemID == skill.BookItemID {
					otherBook = item
				}
			}
			// r 接收使用供后续步骤使用。
			if r := use(&pb.LearnSkillRequest{ItemInstanceId: domain.Bytes(otherBook.ID), SkillId: skill.ID, Operation: c.operation()}); r.ErrorCode != p.CodeInvalidArgument {
				t.Fatal("跨角色书实例被消费")
			}
			// original 保存当前处理所用的原始。
			original := &pb.LearnSkillRequest{ItemInstanceId: domain.Bytes(book.ID), SkillId: skill.ID, Operation: c.operation()}
			// first 保存由使用取得的第一项。
			first := use(original)
			if first.ErrorCode != 0 || len(c.inventory.LearnedSkills) != 1 || c.inventory.LearnedSkills[0].SkillId != skill.ID || c.inventory.LearnedSkills[0].Level != skill.LearnedLevel {
				t.Fatalf("真实学习未同步零级已学:%+v", first)
			}
			// item 接收遍历道具集合时当前的元素。
			for _, item := range c.inventory.Items {
				if domain.Hex(item.InstanceId) == book.ID {
					t.Fatal("学习未消费唯一书")
				}
			}
			// r 接收使用供后续步骤使用。
			if r := use(original); r.ErrorCode != 0 || r.CommandSeq != first.CommandSeq {
				t.Fatal("原学习重传不能核对相同回执")
			}
			// r 接收使用供后续步骤使用。
			if r := use(&pb.LearnSkillRequest{ItemInstanceId: domain.Bytes(book.ID), SkillId: skill.ID, Operation: c.operation()}); r.ErrorCode != p.CodeOperationConflict {
				t.Fatal("重复学习被当作升级")
			}
			// r 接收使用供后续步骤使用。
			if r := use(original); r.ErrorCode != 0 || r.CommandSeq <= first.CommandSeq {
				t.Fatal("历史成功学习重查没有返回当前水位")
			}
			persisted, err = inspection.Role(ctx, roleID)
			must(t, err)
			// level 保存由技能取得的等级，learned 保存由技能取得的已学习。
			if level, learned := persisted.Skill(skill.ID); !learned || level != 0 || len(persisted.Skills) != 1 {
				t.Fatal("学习状态未真实持久化")
			}
			// count 保存由数量文档取得的数量。
			count, err := inspection.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(original.Operation.OpId)})
			must(t, err)
			if count != 1 {
				t.Fatal("学习重传重复审计")
			}
			if job == 0 {
				// request 保存由发送取得的请求。
				request := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: skill.ID, TargetHandle: 10})
				c.expectError(request, p.CodeSkillStateBlocked)
			} else {
				applyAutoSkillPreferences(c, &pb.AutoSkillSettings{})
				// target 保存由无符号整数“32”取得的目标。
				target := uint32(10)
				if job == 2 {
					target = 2
					// f 记录协议帧。
					c.until(func(f wire.Frame) bool {
						pet := c.entity(target)
						return f.ID == p.MsgS2CSceneBaseline && pet != nil && pet.Hp < pet.HpMax
					})
				}
				// hpBefore 保存生命值，用于当前步骤中的生命值操作前的状态，mpBefore 保存魔法值，用于当前步骤中的魔法值操作前的状态。
				hpBefore, mpBefore := c.entity(target).Hp, c.player().Mp
				// targetLife 保存生命代次，用于当前步骤中的目标生命代次。
				targetLife := c.entity(target).Life
				// request 保存由发送取得的请求。
				request := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: skill.ID, TargetHandle: target})
				// eventFrame 保存由有效截止时间取得的事件协议帧，f 记录协议帧。
				eventFrame := c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CCastAccepted })
				// event 保存当前处理所用的事件。
				event := &pb.CastAccepted{}
				must(t, proto.Unmarshal(eventFrame.Payload, event))
				// acceptedAt 记录本次处理开始时的时间。
				acceptedAt := time.Now()
				// f 记录协议帧。
				c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().CastSequence == 1 })
				// acceptanceHPValid 保存根据生命值与生命值操作前的状态计算的“acceptance”生命值有效。
				acceptanceHPValid := c.entity(target).Hp == hpBefore
				if job == 2 {
					// 活世界仍可受击；只禁止受理瞬间凭空回血，不把持续伤害误判成施法时序错误。
					acceptanceHPValid = c.entity(target).Hp > 0 && c.entity(target).Hp <= hpBefore && c.entity(target).Life == targetLife
				}
				if c.player().Mp != mpBefore-skill.MPBaseCost || event.CastId != 1 || event.InputSequence != c.input || !acceptanceHPValid {
					t.Fatalf("受理时MP/输入/延迟/动作基线不符：job=%d mp=%d expected_mp=%d cast=%d input=%d expected_input=%d target_hp=%d before_hp=%d", job, c.player().Mp, mpBefore-skill.MPBaseCost, event.CastId, event.InputSequence, c.input, c.entity(target).Hp, hpBefore)
				}
				// 治疗恢复从受理后的新鲜受伤基线观察；不能要求恢复掉之后继续受到的全部伤害。
				// healingObservationHP 保存生命值，用于当前步骤中的治疗“observation”生命值。
				healingObservationHP := c.entity(target).Hp
				// 相同输入不重放效果；新输入在动作锁内也不能重扣MP。
				// stale 保存由发送取得的已失效。
				stale := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: skill.ID, TargetHandle: target, InputSequence: event.InputSequence})
				c.expectError(stale, p.CodeStaleInput)
				// blocked 保存由发送取得的阻挡。
				blocked := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: skill.ID, TargetHandle: target})
				c.expectError(blocked, p.CodeSkillStateBlocked)
				_ = request
				// f 记录协议帧。
				c.until(func(f wire.Frame) bool {
					if f.ID != p.MsgS2CSceneBaseline {
						return false
					}
					if job == 1 {
						return c.entity(target).Hp < hpBefore
					}
					if c.entity(target).Life != targetLife || c.entity(target).Hp <= 0 {
						t.Fatal("治疗观察前态失效：目标已死亡或换代")
					}
					return c.entity(target).Hp > healingObservationHP
				})
				if time.Since(acceptedAt) < time.Duration(event.DelayMs-50)*time.Millisecond {
					t.Fatal("真实WS效果早于投射/治疗延迟")
				}
				// castSequence 保存当前处理所用的施法序号。
				castSequence := c.player().CastSequence
				// scene 保存场景代次，用于当前步骤中的场景。
				scene := c.entry.SceneEpoch
				// manaAfter 保存魔法值，用于当前步骤中的魔法值操作后的状态。
				manaAfter := c.player().Mp
				// oldBinding 保存会话绑定，用于当前步骤中的原状态会话绑定。
				oldBinding := c.binding
				c.close()
				// disabled 保存当前处理所用的“disabled”。
				disabled := &pb.AutoSkillSettings{Enabled: true, DisabledSkillIds: []uint32{skill.ID}}
				c = joinWithAutoSkills(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), disabled, observeImmediate)
				c.auto(false, 1)
				if c.entry.SceneEpoch != scene || c.binding == oldBinding || c.player().CastSequence != castSequence || c.player().Mp != manaAfter || len(c.inventory.LearnedSkills) != 1 {
					t.Fatal("健康重连重置场景/魔力/施法观察或丢失持久技能")
				}
				if !proto.Equal(c.hosting.GetAutoSkillSettings(), disabled) {
					t.Fatal("健康重连ACK未在就绪前同步自动施法偏好", c.hosting)
				}
				// 新连接可从1重新输入；旧的高水位不会永久阻止合法操作。
				c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: scene, ReleaseJoystick: true, InputSequence: 1})
				// barrier 保存由发送取得的屏障。
				barrier := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: scene, Direction: 8})
				c.expectError(barrier, p.CodeInvalidArgument)
				// station 按目标类型读取当前接口值，得到“station”。
				station := proto.Clone(c.player().Position).(*pb.GridPoint)
				// movement 保存移动修订号，用于当前步骤中的移动，life 保存当前处理所用的生命代次。
				movement, life := c.player().MovementRevision, c.player().Life
				// atStation 封装发生时间“station”对应的后续处理。
				atStation := func() {
					t.Helper()
					if !proto.Equal(c.player().Position, station) || c.player().MovementRevision != movement {
						t.Fatalf("自动挂机改变所选脚点或移动修订：want=%v got=%v", station, c.player())
					}
				}
				c.auto(true, 1)
				// 用持续的权威基线越过手动施法冷却，隔离单技能禁用与冷却造成的静默。
				// observeUntil 保存由新增取得的观测有效截止时间。
				observeUntil := acceptedAt.Add(time.Duration(skill.GlobalSpellMS+300) * time.Millisecond)
				// f 记录协议帧。
				c.until(func(f wire.Frame) bool {
					if f.ID == p.MsgS2CCastAccepted || c.player().CastSequence > castSequence {
						t.Fatal("单技能自动禁用后仍受理该技能")
					}
					if f.ID == p.MsgS2CSceneBaseline {
						atStation()
					}
					return f.ID == p.MsgS2CSceneBaseline && time.Now().After(observeUntil)
				})
				applyAutoSkillPreferences(c, &pb.AutoSkillSettings{Enabled: true})
				// automatic 保存当前处理所用的自动。
				automatic := &pb.CastAccepted{}
				// f 记录协议帧。
				travelUntilWithin(c, 8*time.Second, func(f wire.Frame) bool {
					if f.ID != p.MsgS2CCastAccepted {
						return false
					}
					must(t, proto.Unmarshal(f.Payload, automatic))
					return true
				})
				if automatic.SkillId != skill.ID || automatic.InputSequence != 0 {
					t.Fatal("重新启用后未产生真实服务端自动施法", automatic)
				}
				// f 记录协议帧。
				travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool {
					if f.ID != p.MsgS2CSceneBaseline {
						return false
					}
					atStation()
					return c.player().CastSequence > castSequence
				})
				if job == 2 && (!monsterHit || !petHit) {
					// f 记录协议帧。
					travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool { return monsterHit && petHit })
				}
				if job == 1 {
					// claimed 保存由资产取得的已领取。
					claimed := c.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
					if claimed.ErrorCode != 0 {
						t.Fatal("定点托管验收未能领取正常免费时长", claimed)
					}
					c.inventoryAt(claimed.NewRevision)
					// beforeOffline 保存由角色取得的操作前的状态离线，err 接收角色过程中可能产生的错误，供后续失败分支处理。
					beforeOffline, err := inspection.Role(ctx, roleID)
					must(t, err)
					c.close()
					// nowBilled 保存角色剩余托管秒数，用于当前步骤中的当前时间“billed”。
					nowBilled := beforeOffline.HostingSeconds
					// deadline 保存由新增取得的截止时间。
					for deadline := time.Now().Add(6 * time.Second); nowBilled == beforeOffline.HostingSeconds && time.Now().Before(deadline); {
						time.Sleep(100 * time.Millisecond)
						// current 保存由角色取得的当前状态，err 接收角色过程中可能产生的错误，供后续失败分支处理。
						current, err := inspection.Role(ctx, roleID)
						must(t, err)
						nowBilled = current.HostingSeconds
					}
					if nowBilled >= beforeOffline.HostingSeconds {
						t.Fatal("健康断线后没有实际托管扣时")
					}
					c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
					if c.entry.SceneEpoch != scene || c.player().Life != life || c.player().Hp <= 0 {
						t.Fatal("健康重连未恢复原定点现场")
					}
					atStation()
				}
			}
			// document 记录文档，使用MongoDB BSON 文档表示。
			var document bson.M
			must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": roleID}).Decode(&document))
			// key 接收遍历文本列表时当前的元素。
			for _, key := range []string{"hp", "mp", "casts", "next_spell", "skill_cooldowns", "heal_amount", "next_heal", "input_sequence", "auto_skill_settings"} {
				// exists 保存从文档取出的存在。
				if _, exists := document[key]; exists {
					t.Fatalf("技能临时状态%s被持久化", key)
				}
			}
			t.Logf("通过：job%d初始书/仅本角学习/错职业/原回执/重复禁升级/零级持久化/真实PB施法与重连", job)
		})
	}
}

// applyAutoSkillPreferences 应用自动技能“preferences”。
// c 记录客户端，settings 记录“settings”，使用自动技能“settings”表示。
func applyAutoSkillPreferences(c *client, settings *pb.AutoSkillSettings) {
	c.t.Helper()
	// globalAuto 保存由“get”自动战斗已开启取得的全局自动。
	globalAuto := c.hosting.GetAutoBattleEnabled()
	// request 保存由发送取得的请求。
	request := c.send(p.MsgC2SAutoBattle, &pb.AutoBattleIntent{SceneEpoch: c.entry.SceneEpoch, AutoSkillSettings: settings})
	// f 记录协议帧。
	travelUntilWithin(c, 2*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CHostingState && f.Request == request
	})
	if !proto.Equal(c.hosting.GetAutoSkillSettings(), settings) || c.hosting.GetAutoBattleEnabled() != globalAuto {
		c.t.Fatal("独立偏好未正确回显或改了全局自动战斗", c.hosting)
	}
}
