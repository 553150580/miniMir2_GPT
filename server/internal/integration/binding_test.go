package integration

import (
	"context"
	"errors"
	"github.com/coder/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
	"io"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"os"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestRoleBindingInitializationBackfillAndHealthyReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo/WS验证绑定初始化和旧角色接管迁移")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	// 仅随机测试库启用未审定运行起点的业务合同；(0,0)故意与角色出生点不同。
	cfg.Fixture = bindingInitializationRule12Fixture(cfg.Fixture)
	cfg.Fixture.Binding = &devconfig.BindingRules{Enabled: true, StartPoints: []devconfig.BindingPoint{{MapID: cfg.Fixture.Map.ID, X: 0, Y: 0}}}
	must(t, cfg.Fixture.Validate())
	ctx, cancel := context.WithCancel(context.Background())
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	var wg sync.WaitGroup
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("绑定%s：%v", mode, err)
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
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "bind_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	for slot, scenario := range []string{"created", "legacy_missing", "existing", "invalid_map"} {
		create := &pb.CreateRoleRequest{OriginServerId: 1, Slot: uint32(slot + 1), Job: 0, Name: "绑定" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())}
		role, replay := &pb.RoleSummary{}, &pb.RoleSummary{}
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, create, role, 0)
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, create, replay, 0)
		id := domain.Hex(role.Key.RoleId)
		if domain.Hex(replay.Key.RoleId) != id {
			t.Fatal("建角重传产生新角色")
		}
		stored, err := inspection.Role(ctx, id)
		must(t, err)
		if stored.Binding == nil || *stored.Binding != (domain.RoleBinding{MapID: cfg.Fixture.Map.ID}) || stored.Revision != 1 || stored.EconomicSeq != 0 {
			t.Fatal("绑定未与初始角色原子创建，或重传推进了永久流水", stored.Binding)
		}
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
		ticket := issue(t, endpoint, login.AccountToken, role.Key)
		if scenario == "invalid_map" {
			requestCtx, stop := context.WithTimeout(ctx, 3*time.Second)
			ws, _, err := websocket.Dial(requestCtx, "ws://"+cfg.GameAddress+"/ws", nil)
			must(t, err)
			must(t, ws.Write(requestCtx, websocket.MessageBinary, wire.Encode(wire.Message(p.MsgC2SEnterLine, 1, 0, &pb.EnterLineRequest{AccountTicket: ticket}))))
			_, raw, readErr := ws.Read(requestCtx)
			ws.CloseNow()
			stop()
			// 既有入场拒绝立即关闭连接，队列内错误帧不保证先送达；超时或其它网络失败不算拒绝通过。
			if readErr == nil {
				frame, err := wire.Decode(raw)
				must(t, err)
				if frame.Error != p.CodeLineForbidden || frame.Request != 1 {
					t.Fatal("未知绑定地图返回错误的拒绝帧", frame)
				}
			} else if !errors.Is(readErr, io.EOF) {
				t.Fatal("入场拒绝之外的网络失败", readErr)
			}
			healthy(t, cfg.GameAddress)
			stored, err = inspection.Role(ctx, id)
			must(t, err)
			if stored.Revision != 1 || stored.SessionEpoch != 0 || stored.Binding == nil || *stored.Binding != expected {
				t.Fatal("无效绑定拒绝仍接管或覆盖角色")
			}
			used, err := inspection.DB.Collection("entry_tickets").CountDocuments(ctx, bson.M{"role_id": id, "used": true})
			must(t, err)
			if used != 0 {
				t.Fatal("拒绝绑定仍消耗了入场票据")
			}
			continue
		}
		c := join(t, cfg.GameAddress, ticket)
		c.auto(false, 1)
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
		var document bson.M
		must(t, inspection.DB.Collection("roles").FindOne(ctx, bson.M{"_id": id}).Decode(&document))
		for _, key := range []string{"position", "current_map_id", "hp", "mp", "scene", "casts", "skill_cooldowns"} {
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
func bindingInitializationRule12Fixture(f devconfig.Fixture) devconfig.Fixture {
	teleport, _ := f.Skill(21)
	bookID, buffID := teleport.BookItemID, teleport.BuffID
	f.RuleVersion = 12
	f.EquipmentRecycling = nil // rule19开发回收合同不能带回历史rule12初始化样本。
	f.PetRuntime = nil         // 历史初始化合同不混入rule16生命周期能力。
	if f.Binding != nil {
		binding := *f.Binding
		binding.SafeZoneEnabled = nil
		f.Binding = &binding
	}
	f.TownTravel, f.TownNavigation = nil, nil
	f.Map.NoTeleport, f.Map.SourceStaticBits = nil, nil
	f.Map.NoPets = nil
	for i := range f.Maps {
		f.Maps[i].NoTeleport, f.Maps[i].SourceStaticBits = nil, nil
		f.Maps[i].NoPets = nil
	}
	f.Skills = slices.DeleteFunc(f.Skills, func(s devconfig.Skill) bool { return s.ID == 21 })
	f.Buffs = slices.DeleteFunc(f.Buffs, func(b devconfig.BuffDefinition) bool { return b.ID == buffID || b.Kind == "teleport_mana_penalty" })
	f.Items = slices.DeleteFunc(f.Items, func(item devconfig.Item) bool { return item.ID == bookID || item.BookSkillID == 21 })
	f.SkillUpgrades = slices.DeleteFunc(f.SkillUpgrades, func(row devconfig.SkillUpgrade) bool { return row.SkillID == 21 })
	f.SkillShop = slices.DeleteFunc(f.SkillShop, func(row devconfig.SkillShop) bool { return row.ItemID == bookID })
	f.SkillBookRecycle = slices.DeleteFunc(f.SkillBookRecycle, func(row devconfig.SkillBookRecycle) bool { return row.ItemID == bookID })
	f.InitialSkillBooks = slices.DeleteFunc(f.InitialSkillBooks, func(row devconfig.InitialSkillBook) bool { return row.ItemID == bookID })
	f.InitialInventory = slices.DeleteFunc(f.InitialInventory, func(id uint32) bool { return id == bookID })
	f.Drops.ItemIDs = slices.DeleteFunc(f.Drops.ItemIDs, func(id uint32) bool { return id == bookID })
	return f
}
