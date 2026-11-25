// 本文件通过正常区服、角色和地图入口验收规则30的普通地图与怪物槽配置。
package integration

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// m1FieldConfig 只接受受控临时副本集，按已发布的规则30候选启动专项。
func m1FieldConfig(t *testing.T) devconfig.Config {
	t.Helper()
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("普通地图验收要求独立 MongoDB 副本集与真实 WebSocket")
	}
	cfg, err := loadIntegrationConfig()
	must(t, err)
	if !cfg.Fixture.OrdinaryMonsterRosterEnabled() {
		t.Skip("尚未发布规则30普通地图候选")
	}
	return cfg
}

// m1FieldServices 启动本用例专有数据库及临时监听，关闭后回收同一测试库。
func m1FieldServices(t *testing.T, cfg devconfig.Config) (devconfig.Config, context.Context, *persistence.Store) {
	t.Helper()
	cfg.Database = "minimir_dev_game_test_m1_field_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	ctx, stop := context.WithCancel(context.Background())
	store, err := persistence.Open(ctx, cfg)
	if err != nil {
		stop()
		t.Fatal(err)
	}
	var services sync.WaitGroup
	t.Cleanup(func() {
		stop()
		services.Wait()
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		defer store.Close(cleanup)
		must(t, store.DB.Drop(cleanup))
	})
	for _, mode := range []string{"account", "game"} {
		services.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("普通图专项 %s 节点：%v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	return cfg, ctx, store
}

// m1FieldRole 创建普通账号和职业角色。路由、短票及场景附着使用现有真实联机入口。
func m1FieldRole(t *testing.T, cfg devconfig.Config, job uint32) (string, string, *pb.RoleKey) {
	t.Helper()
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login, role := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "field_" + domain.ID()[:9], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: job, Name: "野外" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	return endpoint, login.AccountToken, role.Key
}

// TestMonsterRespawnM1DefaultMapCatalogMongoWS 使用未经改写的默认地图和怪物表检查跨图区服、八个真实怪槽及原图重连。
func TestMonsterRespawnM1DefaultMapCatalogMongoWS(t *testing.T) {
	cfg, _, _ := m1FieldServices(t, m1FieldConfig(t))
	endpoint, token, key := m1FieldRole(t, cfg, 0)
	c := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
	t.Cleanup(c.close)
	fixture := cfg.Fixture
	maps := []devconfig.MapTemplate{fixture.Map}
	for _, m := range fixture.Maps {
		if m.SceneKindValue() == devconfig.SceneKindPersonal {
			maps = append(maps, m)
		}
	}
	if len(maps) < 3 {
		t.Fatal("默认候选须含主图和至少两张可到达的普通野外图")
	}
	seenTemplates := map[uint32]bool{}
	for i, m := range maps {
		if m.Width != 13 || m.Height != 13 || len(m.MonsterSpawns) != 8 || len(m.MonsterSpawnIDs) != len(m.MonsterSpawns) {
			t.Fatalf("地图%d未按13×13八槽配置独立怪物表", m.ID)
		}
		if i != 0 {
			travelToBaseline(c, 2, m.ID)
			travelAcknowledge(c, false)
		}
		if c.entry.RoomId != m.ID || c.entry.SceneKind != devconfig.SceneKindPersonal {
			t.Fatalf("真实场景未到达普通图%d：%v", m.ID, c.entry)
		}
		if travelCount(c.scene, 3) != m.InitialMonsterCount || m.InitialMonsterCount != 1 {
			t.Fatalf("地图%d首入场初始怪数不等于配置的一个槽", m.ID)
		}
		c.auto(false, uint32(len(m.MonsterSpawns)))
		travelUntilWithin(c, 3*time.Second, func(frame wire.Frame) bool {
			return frame.ID == p.MsgS2CSceneBaseline && travelCount(c.scene, 3) == len(m.MonsterSpawns)
		})
		for slot, monsterID := range m.MonsterSpawnIDs {
			entity := c.entity(uint32(slot + 10))
			definition, known := fixture.MonsterByID(monsterID)
			if !known || entity == nil || entity.DefinitionId != definition.DefinitionID || entity.EntityKind != 3 || entity.HpMax != definition.HP {
				t.Fatalf("地图%d的刷怪槽%d未按模板%d出现在真实基线：%v", m.ID, slot, monsterID, entity)
			}
			seenTemplates[monsterID] = true
		}
		t.Logf("地图%d八槽模板：%v", m.ID, m.MonsterSpawnIDs)
	}
	if len(seenTemplates) < 3 {
		t.Fatal("多张普通图仍共同套用单一怪物谱", seenTemplates)
	}
	currentMap, currentScene := c.entry.RoomId, c.entry.SceneEpoch
	c.close()
	rejoined := join(t, cfg.GameAddress, issue(t, endpoint, token, key))
	t.Cleanup(rejoined.close)
	if rejoined.entry.RoomId != currentMap || rejoined.entry.SceneEpoch != currentScene || travelCount(rejoined.scene, 3) != 8 {
		t.Fatal("健康重连未恢复角色所在普通图及当前八个怪槽", rejoined.entry)
	}
}
