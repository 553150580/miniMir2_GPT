// 使用真实持久数据验证比奇城商品与安全区规则升级后继续认可规则 24 的历史绑定。

package integration

import (
	"context"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestRule24BichengBindingRemainsTrustedAfterShopUpgradeMongo 先写入旧版本比奇城绑定，再用升级后的配置核对持久绑定信任。
// t 提供断言失败报告、子用例和清理注册。
func TestRule24BichengBindingRemainsTrustedAfterShopUpgradeMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启独立Mongo验证历史比奇城绑定留存")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_bicheng_" + domain.ID()
	// current 保存开发测试配置，用于当前步骤中的当前状态。
	current := cfg.Fixture
	// historical 保存当前处理所用的“historical”。
	historical := current
	historicalOrdinaryFixture(t, &historical)
	historical.RuleVersion = 24
	historical.Hash = "074726ce1df63b58ea21a4ea2892c74bd58815cf591dbbd2adc43c19c9adc418"
	historical.Map.Width, historical.Map.Height = 17, 17
	historical.NPCShops, historical.EquipmentDurability = nil, nil
	historical.Maps = slices.Clone(historical.Maps)
	// oldTown 保存由地图“by”标识取得的原状态城镇，ok 表明查询、校验或类型转换是否成功。
	oldTown, ok := historical.MapByID(5)
	if !ok {
		t.Fatal("当前目录缺少比奇城")
	}
	// 原7098ef62布局没有5008、5009两位商人及相应占位，安全区与区映射完全相同。
	// npc 记录NPC，使用城镇NPC表示。
	oldTown.NPCs = slices.DeleteFunc(slices.Clone(oldTown.NPCs), func(npc devconfig.TownNPC) bool { return npc.ID == 5008 || npc.ID == 5009 })
	// cell 记录地图格，使用点位表示。
	oldTown.Blocked = slices.DeleteFunc(slices.Clone(oldTown.Blocked), func(cell devconfig.Point) bool {
		return cell.Y == 5 && (cell.X == 6 || cell.X == 10)
	})
	cfg.Fixture = historical
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	// account 保存由打开取得的账号。
	account, err := persistence.Open(ctx, cfg)
	must(t, err)
	// game 保存由打开取得的游戏。
	game, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, account.DB.Drop(cleanup))
		game.Close(cleanup)
		account.Close(cleanup)
	})
	must(t, account.Singleton(ctx, "account"))
	// grant 保存由批准游戏取得的授予。
	grant, err := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
	must(t, err)
	game.Epoch = grant.Epoch
	// login 接收登录供后续步骤使用。
	login, err := account.Login(ctx, "bicheng_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	// auth 保存由“authenticate”取得的授权。
	auth, err := account.Authenticate(ctx, login.AccountToken)
	must(t, err)
	// character 保存由创建角色取得的“character”。
	character, err := account.CreateRole(ctx, auth, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Name: "比奇" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())})
	must(t, err)
	// ticket 保存由签发入场票据取得的入场票据。
	ticket, err := account.IssueTicket(ctx, auth, character.Key)
	must(t, err)
	// before 保存由入场取得的操作前的状态，gameAuth 保存由入场取得的游戏授权。
	before, gameAuth, err := game.Enter(ctx, ticket.Ticket)
	must(t, err)
	// q 组装安全区域会话绑定请求所需的数据。
	q := persistence.SafeZoneBindingRequest{SourceSeq: before.SourceSeq + 1, ExpectedRevision: before.Revision, ExpectedRuleVersion: historical.RuleVersion,
		ExpectedConfigHash: historical.Hash, MapID: oldTown.ID, Position: oldTown.SafeZones[0].Center, Action: "walk", EventID: domain.ID()}
	// bound 保存由绑定安全区域带有回执取得的绑定，proof 保存由绑定安全区域带有回执取得的“proof”。
	bound, proof, err := game.BindSafeZoneWithReceipt(ctx, gameAuth, q)
	must(t, err)
	if proof.Binding == nil || proof.Binding.SourceMapSHA256 != "b40701cea0618ff1bad495f65ffcabab8b9e9b398200fe39424e0e101f6cf832" ||
		proof.Binding.ZoneSetSHA256 != "eabc6551248f156e1881a9ba785d2583c505f150ee3fea4a6fe31118bb60e136" || bound.BindingSafeZoneID != 501 {
		t.Fatal("测试未生成已归档的原规则24绑定证明", proof.Binding)
	}
	must(t, game.Release(ctx, "game"))
	cfg.Fixture, account.Cfg.Fixture = current, current
	// next 保存由打开取得的下一状态。
	next, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		next.Close(cleanup)
	})
	grant, err = account.ApproveGame(ctx, cfg.NodeID, next.Instance, cfg.RunID)
	must(t, err)
	next.Epoch = grant.Epoch
	if next.Epoch != game.Epoch+1 {
		t.Fatal("新版未真实取得新运行所有权代次")
	}
	// seen 保存由角色取得的“seen”。
	seen, err := next.Role(ctx, bound.ID)
	must(t, err)
	if !reflect.DeepEqual(seen.Binding, bound.Binding) || seen.BindingSafeZoneID != bound.BindingSafeZoneID || seen.BindingSourceSeq != bound.BindingSourceSeq || seen.SourceSeq != bound.SourceSeq {
		t.Fatal("升级读取改变了原绑定与来源流水")
	}
	ticket, err = account.IssueTicket(ctx, auth, character.Key)
	must(t, err)
	// attached 保存由入场取得的“attached”。
	attached, _, err := next.Enter(ctx, ticket.Ticket)
	must(t, err)
	if !reflect.DeepEqual(attached.Binding, bound.Binding) || attached.BindingSafeZoneID != 501 || attached.BindingSourceSeq != bound.BindingSourceSeq || attached.SourceSeq != bound.SourceSeq {
		t.Fatal("升级入场重置了历史绑定或重复消耗来源流水")
	}
	// operations 保存由集合取得的操作集合。
	operations := game.DB.Collection("operations")
	// unchanged 记录“unchanged”，使用操作表示。
	var unchanged persistence.Operation
	must(t, operations.FindOne(ctx, bson.M{"_id": proof.ID}).Decode(&unchanged))
	if !reflect.DeepEqual(unchanged.Binding, proof.Binding) {
		t.Fatal("升级入场重签了旧永久回执")
	}
	// field 接收遍历临时结构列表时当前的元素。
	// name 记录名称，使用文本表示。
	// original 记录原始，使用文本表示。
	for _, field := range []struct{ name, original string }{
		{"binding_use.config_hash", proof.Binding.ConfigHash},
		{"binding_use.source_map_sha256", proof.Binding.SourceMapSHA256},
		{"binding_use.zone_set_sha256", proof.Binding.ZoneSetSHA256},
	} {
		_, err = operations.UpdateOne(ctx, bson.M{"_id": proof.ID}, bson.M{"$set": bson.M{field.name: persistence.Hash([]byte(field.name))}})
		must(t, err)
		if _, err = account.IssueTicket(ctx, auth, character.Key); persistence.Code(err) != p.CodeLineForbidden {
			t.Fatalf("历史兼容接受未发布的证明字段 %s: %v", field.name, err)
		}
		_, err = operations.UpdateOne(ctx, bson.M{"_id": proof.ID}, bson.M{"$set": bson.M{field.name: field.original}})
		must(t, err)
	}
	// roleFilter 初始化用作角色筛选条件的MongoDB BSON 文档。
	roleFilter := bson.M{"_id": bound.ID}
	// roles 保存由集合取得的角色集合。
	roles := game.DB.Collection("roles")
	_, err = operations.UpdateOne(ctx, bson.M{"_id": proof.ID}, bson.M{"$set": bson.M{"binding_use.point.x": 7}})
	must(t, err)
	_, err = roles.UpdateOne(ctx, roleFilter, bson.M{"$set": bson.M{"binding.x": 7}})
	must(t, err)
	if _, err = account.IssueTicket(ctx, auth, character.Key); persistence.Code(err) != p.CodeLineForbidden {
		t.Fatal("历史摘要被移到其他可走坐标后仍被接受", err)
	}
	_, err = operations.UpdateOne(ctx, bson.M{"_id": proof.ID}, bson.M{"$set": bson.M{"binding_use.point.x": bound.Binding.X}})
	must(t, err)
	_, err = roles.UpdateOne(ctx, roleFilter, bson.M{"$set": bson.M{"binding.x": bound.Binding.X}})
	must(t, err)
	// future 保存当前处理所用的“future”。
	future := current
	future.Maps = slices.Clone(current.Maps)
	// changed 保存由地图“by”标识取得的已变化。
	changed, _ := future.MapByID(5)
	changed.Blocked = append(slices.Clone(changed.Blocked), devconfig.Point{X: 1, Y: 1})
	account.Cfg.Fixture = future
	if _, err = account.IssueTicket(ctx, auth, character.Key); persistence.Code(err) != p.CodeLineForbidden {
		t.Fatal("历史证明被自动放宽至未经核验的未来地图", err)
	}
	t.Log("真实Mongo确认规则24比奇城原回执仍可领票进线，原证明未改，未知摘要与未来地图仍拒绝")
}
