// 验证位移技能学习、升级、实际施法与重连恢复，检查位移前后的场景与回执。

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

// TestDisplacementSkillsLearningUpgradeCastingAndReconnectMongoWS 检查位移技能的学习、升级和施法，以及重连继续使用当前状态。
// t 提供断言失败报告、子用例和清理注册。
func TestDisplacementSkillsLearningUpgradeCastingAndReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启真实Mongo/WS后验证位移技能学习、升级、施放及重入")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// 隔离随机测试库只固定边界站位和无伤怪物，不改唯一运行Excel或新增调试入口。
	cfg.Fixture.Map.PlayerSpawn = devconfig.Point{X: 0, Y: 0}
	cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: 1, Y: 0}
	// i 标记遍历怪物集合时访问的下标。
	for i := range cfg.Fixture.Monsters {
		// m 保存当前处理所用的怪物集合中的指定项。
		m := &cfg.Fixture.Monsters[i]
		m.HP, m.AttackMin, m.AttackMax, m.MoveMS, m.AttackMS = 1000, 0, 0, 600000, 600000
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
				t.Errorf("位移%s：%v", mode, err)
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
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "displace_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	// slot 定位数值列表中正在处理的成员，id 接收遍历数值列表时当前的元素。
	for slot, id := range []uint32{8, 27} {
		// s 接收技能供后续步骤使用。
		s, _ := cfg.Fixture.Skill(id)
		// role 保存当前处理所用的角色。
		role := &pb.RoleSummary{}
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: uint32(slot + 1), Job: s.JobID, Name: "位移" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())}, role, 0)
		_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{"$set": bson.M{"level": 40, "wallet": domain.Wallet{Gold: 1000000, BoundGold: 1000000, Yuanbao: 1000000, BoundYuanbao: 1000000}}})
		must(t, err)
		// SyncReady到关闭Auto之间仍可能释放正常挥刀。关闭Auto不会撤销已受理动作，
		// 必须观察这些动作的实际结算，再建立本次位移技能的权威HP前态。
		// pendingAttacks 创建用于维护待确认攻击集合的容器或通道。
		pendingAttacks := make(map[uint32]*pb.AttackAccepted)
		// autoStopped 保存当前处理所用的自动“stopped”，priorAttacks 规定“prior”攻击集合的初始取值。
		// priorTerminals 规定“prior”终止集合的初始取值。
		autoStopped, priorAttacks, priorTerminals := false, 0, 0
		// priorDamage 保存由整数“64”取得的“prior”伤害。
		priorDamage := int64(0)
		// observe 封装观测对应的后续处理，c 记录客户端，frame 记录协议帧。
		observe := func(c *client, frame wire.Frame) {
			if frame.Error != 0 {
				return
			}
			switch frame.ID {
			case p.MsgS2CAttackAccepted:
				// attack 记录攻击，使用攻击已受理表示。
				var attack pb.AttackAccepted
				must(t, proto.Unmarshal(frame.Payload, &attack))
				if attack.CasterHandle != c.entry.GetPlayerHandle() {
					return
				}
				if autoStopped {
					t.Fatalf("关闭Auto后仍新增普通挥刀: skill=%d attack=%v", id, &attack)
				}
				// 空挥不创建pendingAttack，因而没有需要等待的CombatImpact。
				if attack.TargetHandle == 0 {
					return
				}
				if len(pendingAttacks) >= cfg.Fixture.CombatTiming.EventCapacity {
					t.Fatal("关闭Auto前待结算攻击超过来源事件容量")
				}
				pendingAttacks[attack.AttackSequence] = &attack
				priorAttacks++
				t.Logf("施放前自动挥刀: skill=%d scene=%d life=%d sequence=%d target=%d delay_ms=%d", id, attack.SceneEpoch, attack.CasterLife, attack.AttackSequence, attack.TargetHandle, attack.DelayMs)
			case p.MsgS2CCombatImpact:
				// impact 记录命中影响，使用战斗命中影响表示。
				var impact pb.CombatImpact
				must(t, proto.Unmarshal(frame.Payload, &impact))
				if impact.SkillId == 8 && !impact.Healing && impact.Amount > 0 {
					t.Fatalf("抗拒产生了正伤害命中事件: %v", &impact)
				}
				if impact.CasterHandle == c.entry.GetPlayerHandle() && impact.AttackSequence != 0 {
					// pending 保存从待确认攻击集合取出的待确认。
					if _, pending := pendingAttacks[impact.AttackSequence]; pending {
						delete(pendingAttacks, impact.AttackSequence)
						priorTerminals++
						if impact.SkillId == 0 && impact.TargetHandle == 10 && impact.Amount > 0 {
							priorDamage += impact.Amount
						}
						t.Logf("施放前自动挥刀终结: skill=%d sequence=%d target=%d life=%d outcome=%d amount=%d", id, impact.AttackSequence, impact.TargetHandle, impact.TargetLife, impact.Outcome, impact.Amount)
					}
				}
			}
		}
		// c 接收加入供后续步骤使用。
		c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key), observe)
		if id == 8 && priorDamage == 0 {
			// 用真实Auto挥刀及命中建立可复核的已受伤前态；不靠sleep或私有写坐标/HP，
			// 同时证明初始化1000不能被当作稍后抗拒施放前的HP。
			// frame 记录协议帧。
			c.until(func(frame wire.Frame) bool { return priorDamage > 0 })
		}
		c.auto(false, 1)
		autoStopped = true
		// 同一World内的已受理有效目标攻击，无论命中、闪避或失效都会给CombatImpact；
		// 保持join原80秒deadline，不用等待时长猜测旧事件是否已完成。
		for len(pendingAttacks) > 0 {
			c.read()
		}
		// freshBaseline 封装首次装载场景基线对应的后续处理。
		freshBaseline := func() {
			// CancelNavigation只取消导航，不改Auto/攻击队列，也不结算战斗；同Request
			// 的基线越过网络中积压的旧快照，作为施法前当前owner的权威观察。
			// request 保存由发送取得的请求。
			request := c.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, CancelNavigation: true})
			// frame 记录协议帧。
			c.until(func(frame wire.Frame) bool { return frame.ID == p.MsgS2CSceneBaseline && frame.Request == request })
			if c.scene.SceneEpoch != c.entry.SceneEpoch || c.scene.BaselineId != c.entry.BaselineId || c.scene.ChunkCount != 1 {
				t.Fatal("位移施法前未收到同请求的完整权威基线")
			}
		}
		freshBaseline()
		if id == 8 && (priorDamage == 0 || c.entity(10).Hp <= 0 || c.entity(10).Hp >= 1000) {
			t.Fatal("抗拒前未通过真实普通挥刀建立同生命受伤怪物前态")
		}
		t.Logf("关闭Auto后权威前态: skill=%d attacks=%d terminals=%d prior_damage=%d monster_life=%d monster_hp=%d state_sequence=%d", id, priorAttacks, priorTerminals, priorDamage, c.entity(10).Life, c.entity(10).Hp, c.scene.StateSequence)
		c.buyAndLearnSkill(cfg.Fixture, id)
		// bookOffer 记录技能书尝试提交，使用数值表示，pageOffer 记录页面或分页尝试提交，使用数值表示。
		var bookOffer, pageOffer uint32
		// offer 接收遍历技能商店时当前的元素。
		for _, offer := range cfg.Fixture.SkillShop {
			if offer.ItemID == s.BookItemID {
				bookOffer = offer.ID
			}
			if offer.ItemID == cfg.Fixture.SkillEconomy.PageItemID && offer.Currency == "ingot" {
				pageOffer = offer.ID
			}
		}
		// buy 封装“buy”对应的后续处理，offer 记录尝试提交，使用数值表示，quantity 记录数量，使用数值表示。
		buy := func(offer uint32, quantity int64) {
			// result 保存由技能确认取得的结果。
			if result := c.skillConfirm(c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer, Quantity: quantity})); result.ErrorCode != 0 {
				t.Fatal("位移材料购买失败", result)
			}
		}
		buy(bookOffer, 9)
		buy(pageOffer, 32)
		buy(pageOffer, 3)
		// quantity 封装数量对应的后续处理，id 区分当前步骤指向的目标对象。
		quantity := func(id uint32) int64 {
			// n 记录数值。
			var n int64
			// item 接收遍历道具集合时当前的元素。
			for _, item := range c.inventory.Items {
				if item.ItemId == id {
					n += item.Quantity
				}
			}
			return n
		}
		time.Sleep(time.Second)
		// rank 保存由无符号整数“32”取得的排序位次。
		for rank := uint32(0); rank <= 3; rank++ {
			if rank > 0 {
				// row 保存由升级取得的行记录。
				row, _ := cfg.Fixture.Upgrade(id, rank)
				// need 初始化用作所需量的按数值索引的数值映射。
				need := map[uint32]int64{s.BookItemID: int64(rank + 1), row.PageItemID: row.PageCost}
				// q 保存当前处理所用的技能资产预览请求。
				q := &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: id}
				// item 接收遍历道具集合时当前的元素。
				for _, item := range c.inventory.Items {
					// n 保存从所需量取出的所需量中的指定项。
					n := need[item.ItemId]
					if n > item.Quantity {
						n = item.Quantity
					}
					if n > 0 {
						q.Materials = append(q.Materials, &pb.ItemConsumption{InstanceId: item.InstanceId, Version: item.Version, Quantity: n})
						need[item.ItemId] -= n
					}
				}
				// preview 保存由技能预览取得的预览。
				preview := c.skillPreview(q)
				// beforeBooks 保存由数量取得的操作前的状态技能书集合，beforePages 保存由数量取得的操作前的状态“pages”。
				beforeBooks, beforePages := quantity(s.BookItemID), quantity(row.PageItemID)
				// request 保存当前处理所用的请求。
				request := &pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()}
				// result 保存由技能提交取得的结果。
				if result := c.skillCommit(request); result.ErrorCode != 0 {
					t.Fatal("位移升级提交失败", result)
				}
				if quantity(s.BookItemID) != beforeBooks-int64(rank+1) || quantity(row.PageItemID) != beforePages-row.PageCost {
					t.Fatal("位移升级未精确扣除所选原书/书页")
				}
				// result 保存由技能提交取得的结果。
				if result := c.skillCommit(request); result.ErrorCode != 0 || quantity(s.BookItemID) != beforeBooks-int64(rank+1) {
					t.Fatal("位移升级重传重复扣料")
				}
				time.Sleep(time.Duration(s.CooldownBaseMS) * time.Millisecond)
			}
			freshBaseline()
			// beforeMP 保存魔法值，用于当前步骤中的操作前的状态魔法值。
			beforeMP := c.player().Mp
			// monsterBefore 按目标类型读取当前接口值，得到怪物操作前的状态。
			monsterBefore := proto.Clone(c.entity(10)).(*pb.EntityState)
			c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: id, Direction: 2})
			// accepted 保存当前处理所用的已受理。
			accepted := &pb.CastAccepted{}
			// frame 记录协议帧。
			c.until(func(frame wire.Frame) bool {
				if frame.ID != p.MsgS2CCastAccepted {
					return false
				}
				must(t, proto.Unmarshal(frame.Payload, accepted))
				return accepted.SkillId == id
			})
			if accepted.SkillLevel != rank || accepted.DelayMs != 0 {
				t.Fatal("未按实际阶受理即时位移")
			}
			// sawPath 保存当前处理所用的“saw”路径。
			sawPath := false
			// frame 记录协议帧。
			c.until(func(frame wire.Frame) bool {
				if frame.ID == p.MsgS2CEntityDisplacement {
					// move 记录移动，使用实体位移表示。
					var move pb.EntityDisplacement
					must(t, proto.Unmarshal(frame.Payload, &move))
					if move.SourceSkillId == id {
						sawPath = sawPath || len(move.Path) > 0
					}
				}
				return frame.ID == p.MsgS2CSceneBaseline && c.player().Mp == beforeMP-s.MPBaseCost-int64(rank)*s.MPLevelCost
			})
			if rank == 0 && !sawPath {
				t.Fatal("首阶真实WS没有收到成功位移路径")
			}
			if id == 8 && (c.entity(10).Life != monsterBefore.Life || c.entity(10).Hp != monsterBefore.Hp) {
				t.Fatalf("抗拒改变了同生命怪物HP: rank=%d before_life=%d before_hp=%d after_life=%d after_hp=%d", rank, monsterBefore.Life, monsterBefore.Hp, c.entity(10).Life, c.entity(10).Hp)
			}
			t.Logf("位移技能真实施放: skill=%d rank=%d cast=%d before_hp=%d after_hp=%d before_life=%d after_life=%d state_sequence=%d", id, rank, accepted.CastId, monsterBefore.Hp, c.entity(10).Hp, monsterBefore.Life, c.entity(10).Life, c.scene.StateSequence)
		}
		if id == 27 {
			time.Sleep(time.Duration(s.CooldownBaseMS) * time.Millisecond)
			// frame 记录协议帧。
			c.until(func(frame wire.Frame) bool { return frame.ID == p.MsgS2CSceneBaseline })
			// beforeMP 保存魔法值，用于当前步骤中的操作前的状态魔法值，position 按目标类型读取当前接口值，得到位置。
			beforeMP, position := c.player().Mp, proto.Clone(c.player().Position).(*pb.GridPoint)
			c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: id, Direction: 0})
			// frame 记录协议帧。
			c.until(func(frame wire.Frame) bool {
				if frame.ID != p.MsgS2CEntityDisplacement {
					return false
				}
				// move 记录移动，使用实体位移表示。
				var move pb.EntityDisplacement
				must(t, proto.Unmarshal(frame.Payload, &move))
				return move.SourceSkillId == id && move.Kind == 5 && len(move.Path) == 0
			})
			// frame 记录协议帧。
			c.until(func(frame wire.Frame) bool {
				return frame.ID == p.MsgS2CSceneBaseline && c.player().Mp == beforeMP-s.MPBaseCost-3*s.MPLevelCost
			})
			if !proto.Equal(position, c.player().Position) {
				t.Fatal("零步冲撞失败伪造移动")
			}
		}
		// mp 保存当前处理所用的魔法值，revision 保存移动修订号，用于当前步骤中的修订号，life 保存当前处理所用的生命代次。
		mp, revision, life := c.player().Mp, c.player().MovementRevision, c.player().Life
		c.close()
		c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
		if c.player().Mp != mp || c.player().MovementRevision != revision || c.player().Life != life {
			t.Fatal("健康重入重置了位移现场或消耗")
		}
		// stored 保存由角色取得的已持久保存，err 接收角色过程中可能产生的错误，供后续失败分支处理。
		stored, err := inspection.Role(ctx, domain.Hex(role.Key.RoleId))
		must(t, err)
		// rank 保存由技能取得的排序位次，ok 表明查询、校验或类型转换是否成功。
		if rank, ok := stored.Skill(id); !ok || rank != 3 {
			t.Fatal("位移技能真实学习/升级未持久保存")
		}
		// raw 记录原始值，使用MongoDB BSON 文档表示。
		var raw bson.M
		must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": stored.ID}).Decode(&raw))
		// field 接收遍历文本列表时当前的元素。
		for _, field := range []string{"position", "pos", "hp", "mp", "movement_revision"} {
			// exists 保存从原始值取出的存在。
			if _, exists := raw[field]; exists {
				t.Fatal("临时位移现场错误写入永久角色", field)
			}
		}
		c.close()
	}
}
