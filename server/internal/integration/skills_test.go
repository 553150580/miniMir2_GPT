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

func (c *client) expectError(request, code uint32) {
	c.t.Helper()
	for {
		f := c.readFrame()
		if f.Error != 0 {
			if f.Request != request || f.Error != code {
				c.t.Fatalf("请求%d错误=%d，预期%d: %+v", request, f.Error, code, f)
			}
			return
		}
	}
}
func (c *client) entity(handle uint32) *pb.EntityState {
	for _, e := range c.scene.GetEntities() {
		if e.EntityHandle == handle {
			return e
		}
	}
	return nil
}

func TestSkillLearningAndCastingMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("设置MINIMIR_MONGO_E2E=1以验证三职业技能真实Mongo/WS")
	}
	for _, job := range []uint32{0, 1, 2} {
		t.Run([]string{"fencing", "fireball", "healing"}[job], func(t *testing.T) {
			cfg, err := devconfig.Load("../../../conf/dev/server.json")
			must(t, err)
			cfg.Database = "minimir_dev_game_test_" + domain.ID()
			cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
			// 仅隔离测试实例调整交战位置/耐久，以确定性观察延迟和宠物恢复；配置文件未修改。
			cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: 4, Y: 8}
			if job == 2 {
				cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: 8, Y: 6}
				cfg.Fixture.Monsters[0].HP = cfg.Fixture.Pet.AttackMax * 10
				cfg.Fixture.Monsters[0].AttackMin = cfg.Fixture.Pet.DefenceMax + cfg.Fixture.Healing.PetBasePerTick*2
				cfg.Fixture.Monsters[0].AttackMax = cfg.Fixture.Monsters[0].AttackMin
				cfg.Fixture.Monsters[0].Accuracy = cfg.Fixture.Pet.Agility
			}
			ctx, cancel := context.WithCancel(context.Background())
			inspection, err := persistence.Open(ctx, cfg)
			must(t, err)
			var wg sync.WaitGroup
			for _, mode := range []string{"account", "game", "gm"} {
				wg.Go(func() {
					if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
						t.Errorf("技能测试%s: %v", mode, err)
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
			rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "skill_" + domain.ID()[:10], Password: "test_123456"}, login, 0)
			create := &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: job, Name: "技能" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}
			role := &pb.RoleSummary{}
			rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, create, role, 0)
			replayRole := &pb.RoleSummary{}
			rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, create, replayRole, 0)
			roleID := domain.Hex(role.Key.RoleId)
			persisted, err := inspection.Role(ctx, roleID)
			must(t, err)
			var skill devconfig.Skill
			for _, s := range cfg.Fixture.Skills {
				if s.JobID == job {
					skill = s
				}
			}
			var book domain.Item
			bookCount := 0
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
			c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
			defer func() { c.close() }()
			c.auto(false, 1)
			use := func(q *pb.LearnSkillRequest) *pb.OperationResult {
				result := c.asset(p.MsgC2SLearnSkill, q)
				c.inventoryAt(result.NewRevision)
				return result
			}
			// 错职业与不存在的另一角色书身份均形成可重放失败，不消耗真实职业书。
			wrongSkill := uint32(1)
			if skill.ID == 1 {
				wrongSkill = 2
			}
			wrong := use(&pb.LearnSkillRequest{ItemInstanceId: domain.Bytes(book.ID), SkillId: wrongSkill, Operation: c.operation()})
			if wrong.ErrorCode != p.CodeInvalidArgument {
				t.Fatal("错职业书学习未拒绝")
			}
			other := &pb.RoleSummary{}
			rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 2, Job: job, Name: "他人书" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, other, 0)
			otherRole, err := inspection.Role(ctx, domain.Hex(other.Key.RoleId))
			must(t, err)
			var otherBook domain.Item
			for _, item := range otherRole.Items {
				if item.ItemID == skill.BookItemID {
					otherBook = item
				}
			}
			if r := use(&pb.LearnSkillRequest{ItemInstanceId: domain.Bytes(otherBook.ID), SkillId: skill.ID, Operation: c.operation()}); r.ErrorCode != p.CodeInvalidArgument {
				t.Fatal("跨角色书实例被消费")
			}
			original := &pb.LearnSkillRequest{ItemInstanceId: domain.Bytes(book.ID), SkillId: skill.ID, Operation: c.operation()}
			first := use(original)
			if first.ErrorCode != 0 || len(c.inventory.LearnedSkills) != 1 || c.inventory.LearnedSkills[0].SkillId != skill.ID || c.inventory.LearnedSkills[0].Level != skill.LearnedLevel {
				t.Fatalf("真实学习未同步零级已学:%+v", first)
			}
			for _, item := range c.inventory.Items {
				if domain.Hex(item.InstanceId) == book.ID {
					t.Fatal("学习未消费唯一书")
				}
			}
			if r := use(original); r.ErrorCode != 0 || r.CommandSeq != first.CommandSeq {
				t.Fatal("原学习重传不能核对相同回执")
			}
			if r := use(&pb.LearnSkillRequest{ItemInstanceId: domain.Bytes(book.ID), SkillId: skill.ID, Operation: c.operation()}); r.ErrorCode != p.CodeOperationConflict {
				t.Fatal("重复学习被当作升级")
			}
			if r := use(original); r.ErrorCode != 0 || r.CommandSeq <= first.CommandSeq {
				t.Fatal("历史成功学习重查没有返回当前水位")
			}
			persisted, err = inspection.Role(ctx, roleID)
			must(t, err)
			if level, learned := persisted.Skill(skill.ID); !learned || level != 0 || len(persisted.Skills) != 1 {
				t.Fatal("学习状态未真实持久化")
			}
			count, err := inspection.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(original.Operation.OpId)})
			must(t, err)
			if count != 1 {
				t.Fatal("学习重传重复审计")
			}
			if job == 0 {
				request := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: skill.ID, TargetHandle: 10})
				c.expectError(request, p.CodeSkillStateBlocked)
			} else {
				target := uint32(10)
				if job == 2 {
					target = 2
					c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.entity(2).Hp < c.entity(2).HpMax })
				}
				hpBefore, mpBefore := c.entity(target).Hp, c.player().Mp
				request := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: skill.ID, TargetHandle: target})
				eventFrame := c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CCastAccepted })
				event := &pb.CastAccepted{}
				must(t, proto.Unmarshal(eventFrame.Payload, event))
				acceptedAt := time.Now()
				c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline && c.player().CastSequence == 1 })
				if c.player().Mp != mpBefore-skill.MPBaseCost || event.CastId != 1 || event.InputSequence != c.input || c.entity(target).Hp != hpBefore {
					t.Fatal("受理时MP/输入/延迟/动作基线不符")
				}
				// 相同输入不重放效果；新输入在动作锁内也不能重扣MP。
				stale := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: skill.ID, TargetHandle: target, InputSequence: event.InputSequence})
				c.expectError(stale, p.CodeStaleInput)
				blocked := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: skill.ID, TargetHandle: target})
				c.expectError(blocked, p.CodeSkillStateBlocked)
				_ = request
				c.until(func(f wire.Frame) bool {
					if f.ID != p.MsgS2CSceneBaseline {
						return false
					}
					if job == 1 {
						return c.entity(target).Hp < hpBefore
					}
					return c.entity(target).Hp > hpBefore
				})
				if time.Since(acceptedAt) < time.Duration(event.DelayMs-50)*time.Millisecond {
					t.Fatal("真实WS效果早于投射/治疗延迟")
				}
				castSequence := c.player().CastSequence
				scene := c.entry.SceneEpoch
				manaAfter := c.player().Mp
				oldBinding := c.binding
				c.close()
				c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
				c.auto(false, 1)
				if c.entry.SceneEpoch != scene || c.binding == oldBinding || c.player().CastSequence != castSequence || c.player().Mp != manaAfter || len(c.inventory.LearnedSkills) != 1 {
					t.Fatal("健康重连重置场景/魔力/施法观察或丢失持久技能")
				}
				// 新连接可从1重新输入；旧的高水位不会永久阻止合法操作。
				c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: scene, ReleaseJoystick: true, InputSequence: 1})
				barrier := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: scene, Direction: 8})
				c.expectError(barrier, p.CodeInvalidArgument)
			}
			var document bson.M
			must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": roleID}).Decode(&document))
			for _, key := range []string{"hp", "mp", "casts", "next_spell", "skill_cooldowns", "heal_amount", "next_heal", "input_sequence"} {
				if _, exists := document[key]; exists {
					t.Fatalf("技能临时状态%s被持久化", key)
				}
			}
			t.Logf("通过：job%d初始书/仅本角学习/错职业/原回执/重复禁升级/零级持久化/真实PB施法与重连", job)
		})
	}
}
