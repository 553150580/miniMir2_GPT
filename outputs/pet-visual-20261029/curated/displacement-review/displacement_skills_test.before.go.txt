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

func TestDisplacementSkillsLearningUpgradeCastingAndReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启真实Mongo/WS后验证位移技能学习、升级、施放及重入")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// 隔离随机测试库只固定边界站位和无伤怪物，不改唯一运行Excel或新增调试入口。
	cfg.Fixture.Map.PlayerSpawn = devconfig.Point{X: 0, Y: 0}
	cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: 1, Y: 0}
	for i := range cfg.Fixture.Monsters {
		m := &cfg.Fixture.Monsters[i]
		m.HP, m.AttackMin, m.AttackMax, m.MoveMS, m.AttackMS = 1000, 0, 0, 600000, 600000
	}
	ctx, cancel := context.WithCancel(context.Background())
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	var wg sync.WaitGroup
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("位移%s：%v", mode, err)
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
	for _, address := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, address)
	}
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "displace_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	for slot, id := range []uint32{8, 27} {
		s, _ := cfg.Fixture.Skill(id)
		role := &pb.RoleSummary{}
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: uint32(slot + 1), Job: s.JobID, Name: "位移" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())}, role, 0)
		_, err = inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{"$set": bson.M{"level": 40, "wallet": domain.Wallet{Gold: 1000000, BoundGold: 1000000, Yuanbao: 1000000, BoundYuanbao: 1000000}}})
		must(t, err)
		c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
		c.auto(false, 1)
		c.buyAndLearnSkill(cfg.Fixture, id)
		var bookOffer, pageOffer uint32
		for _, offer := range cfg.Fixture.SkillShop {
			if offer.ItemID == s.BookItemID {
				bookOffer = offer.ID
			}
			if offer.ItemID == cfg.Fixture.SkillEconomy.PageItemID && offer.Currency == "ingot" {
				pageOffer = offer.ID
			}
		}
		buy := func(offer uint32, quantity int64) {
			if result := c.skillConfirm(c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer, Quantity: quantity})); result.ErrorCode != 0 {
				t.Fatal("位移材料购买失败", result)
			}
		}
		buy(bookOffer, 9)
		buy(pageOffer, 32)
		buy(pageOffer, 3)
		quantity := func(id uint32) int64 {
			var n int64
			for _, item := range c.inventory.Items {
				if item.ItemId == id {
					n += item.Quantity
				}
			}
			return n
		}
		time.Sleep(time.Second)
		for rank := uint32(0); rank <= 3; rank++ {
			if rank > 0 {
				row, _ := cfg.Fixture.Upgrade(id, rank)
				need := map[uint32]int64{s.BookItemID: int64(rank + 1), row.PageItemID: row.PageCost}
				q := &pb.SkillAssetPreviewRequest{Kind: 2, SkillId: id}
				for _, item := range c.inventory.Items {
					n := need[item.ItemId]
					if n > item.Quantity {
						n = item.Quantity
					}
					if n > 0 {
						q.Materials = append(q.Materials, &pb.ItemConsumption{InstanceId: item.InstanceId, Version: item.Version, Quantity: n})
						need[item.ItemId] -= n
					}
				}
				preview := c.skillPreview(q)
				beforeBooks, beforePages := quantity(s.BookItemID), quantity(row.PageItemID)
				request := &pb.SkillAssetCommitRequest{PreviewId: preview.PreviewId, Operation: c.operation()}
				if result := c.skillCommit(request); result.ErrorCode != 0 {
					t.Fatal("位移升级提交失败", result)
				}
				if quantity(s.BookItemID) != beforeBooks-int64(rank+1) || quantity(row.PageItemID) != beforePages-row.PageCost {
					t.Fatal("位移升级未精确扣除所选原书/书页")
				}
				if result := c.skillCommit(request); result.ErrorCode != 0 || quantity(s.BookItemID) != beforeBooks-int64(rank+1) {
					t.Fatal("位移升级重传重复扣料")
				}
				time.Sleep(time.Duration(s.CooldownBaseMS) * time.Millisecond)
			}
			c.until(func(frame wire.Frame) bool { return frame.ID == p.MsgS2CSceneBaseline })
			beforeMP := c.player().Mp
			c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: id, Direction: 2})
			accepted := &pb.CastAccepted{}
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
			sawPath := false
			c.until(func(frame wire.Frame) bool {
				if frame.ID == p.MsgS2CEntityDisplacement {
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
			if id == 8 && c.entity(10).Hp != 1000 {
				t.Fatal("抗拒对怪物产生了伤害")
			}
		}
		if id == 27 {
			time.Sleep(time.Duration(s.CooldownBaseMS) * time.Millisecond)
			c.until(func(frame wire.Frame) bool { return frame.ID == p.MsgS2CSceneBaseline })
			beforeMP, position := c.player().Mp, proto.Clone(c.player().Position).(*pb.GridPoint)
			c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, SkillId: id, Direction: 0})
			c.until(func(frame wire.Frame) bool {
				if frame.ID != p.MsgS2CEntityDisplacement {
					return false
				}
				var move pb.EntityDisplacement
				must(t, proto.Unmarshal(frame.Payload, &move))
				return move.SourceSkillId == id && move.Kind == 5 && len(move.Path) == 0
			})
			c.until(func(frame wire.Frame) bool {
				return frame.ID == p.MsgS2CSceneBaseline && c.player().Mp == beforeMP-s.MPBaseCost-3*s.MPLevelCost
			})
			if !proto.Equal(position, c.player().Position) {
				t.Fatal("零步冲撞失败伪造移动")
			}
		}
		mp, revision, life := c.player().Mp, c.player().MovementRevision, c.player().Life
		c.close()
		c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
		if c.player().Mp != mp || c.player().MovementRevision != revision || c.player().Life != life {
			t.Fatal("健康重入重置了位移现场或消耗")
		}
		stored, err := inspection.Role(ctx, domain.Hex(role.Key.RoleId))
		must(t, err)
		if rank, ok := stored.Skill(id); !ok || rank != 3 {
			t.Fatal("位移技能真实学习/升级未持久保存")
		}
		var raw bson.M
		must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": stored.ID}).Decode(&raw))
		for _, field := range []string{"position", "pos", "hp", "mp", "movement_revision"} {
			if _, exists := raw[field]; exists {
				t.Fatal("临时位移现场错误写入永久角色", field)
			}
		}
		c.close()
	}
}
