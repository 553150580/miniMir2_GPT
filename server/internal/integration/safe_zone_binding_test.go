// 验证安全区绑定原回执、冷入场恢复以及失权节点的拒写约束。

package integration

import (
	"context"
	"encoding/binary"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"os"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
)

// 真实隔离副本集只验证永久事务；正常行走与客户端观察由独立WS用例验证。
// TestSafeZoneBindingPermanentReceiptColdEnterAndOwnershipMongo 核对安全区绑定首次持久回执、冷入场恢复和旧节点拒写。
// t 提供断言失败报告、子用例和清理注册。
func TestSafeZoneBindingPermanentReceiptColdEnterAndOwnershipMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo验证来源绑定永久事务")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	// enabled 保存当前处理所用的已开启。
	enabled := true
	cfg.Fixture.RuleVersion = 14
	cfg.Fixture.Binding = &devconfig.BindingRules{Enabled: false, StartPoints: []devconfig.BindingPoint{}, SafeZoneEnabled: &enabled}
	cfg.Fixture.Hash = persistence.Hash([]byte(cfg.Fixture.Hash + ":isolated-safe-zone-binding"))
	must(t, cfg.Fixture.ValidateBinding())
	// m 接收地图“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := cfg.Fixture.MapByID(2)
	if !ok || len(m.SafeZones) < 2 {
		t.Fatal("测试必须使用真实来源地图及完整安全区")
	}
	// point 保存中心格，用于当前步骤中的点位。
	point := m.SafeZones[0].Center
	// zone 保存由安全区域发生时间取得的区域，inside 保存由安全区域发生时间取得的“inside”。
	zone, inside := m.SafeZoneAt(point)
	if !inside {
		t.Fatal("来源中心未命中安全区")
	}
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// authority 保存由打开取得的权限围栏。
	authority, err := persistence.Open(ctx, cfg)
	must(t, err)
	// game 保存由打开取得的游戏。
	game, err := persistence.Open(ctx, cfg)
	must(t, err)
	// next 保存由打开取得的下一状态。
	next, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, authority.DB.Drop(cleanup))
		next.Close(cleanup)
		game.Close(cleanup)
		authority.Close(cleanup)
	})
	must(t, authority.Singleton(ctx, "account"))
	// grant 保存由批准游戏取得的授予。
	grant, err := authority.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
	must(t, err)
	game.Epoch = grant.Epoch
	// registered 保存由登录取得的“registered”。
	registered, err := authority.Login(ctx, "safe_bind_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	// account 保存由“authenticate”取得的账号。
	account, err := authority.Authenticate(ctx, registered.AccountToken)
	must(t, err)
	// create 封装创建对应的后续处理，slot 记录槽位，使用数值表示，job 记录任务或职业编号，使用数值表示。
	create := func(slot, job uint32) *pb.RoleSummary {
		// role 保存由创建角色取得的角色，err 接收创建角色过程中可能产生的错误，供后续失败分支处理。
		role, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: job, Name: "源绑定" + domain.ID()[:7], OpId: domain.Bytes(domain.ID())})
		must(t, err)
		return role
	}
	// enter 封装入场对应的后续处理，store 记录持久层存储，key 记录键，使用角色键表示。
	enter := func(store *persistence.Store, key *pb.RoleKey) (domain.Role, persistence.Auth) {
		// ticket 保存由签发入场票据取得的入场票据，err 接收签发入场票据过程中可能产生的错误，供后续失败分支处理。
		ticket, err := authority.IssueTicket(ctx, account, key)
		must(t, err)
		// role 保存由入场取得的角色，auth 保存由入场取得的授权。
		role, auth, err := store.Enter(ctx, ticket.Ticket)
		must(t, err)
		return role, auth
	}
	// request 封装请求对应的后续处理，role 记录角色，position 记录位置，使用点位表示。
	request := func(role domain.Role, position devconfig.Point) persistence.SafeZoneBindingRequest {
		return persistence.SafeZoneBindingRequest{SourceSeq: role.SourceSeq + 1, ExpectedRevision: role.Revision, ExpectedRuleVersion: cfg.Fixture.RuleVersion, ExpectedConfigHash: cfg.Fixture.Hash, MapID: m.ID, Position: position, Action: "walk", EventID: domain.ID()}
	}
	// assertOnlyBindingChanged 封装“assert”“only”会话绑定已变化对应的后续处理，before 记录操作前的状态，使用角色表示。
	// after 记录操作后的状态，使用角色表示。
	assertOnlyBindingChanged := func(before, after domain.Role) {
		t.Helper()
		// want 保存当前处理所用的期望值。
		want := before
		want.Binding, want.BindingSourceSeq, want.BindingSafeZoneID = after.Binding, after.BindingSourceSeq, after.BindingSafeZoneID
		want.SourceSeq, want.Revision = after.SourceSeq, after.Revision
		if !reflect.DeepEqual(want, after) || after.SourceSeq != before.SourceSeq+1 || after.Revision != before.Revision+1 {
			t.Fatal("绑定事务改变经济序号、钱包、背包或其它永久资产", before, after)
		}
	}
	// summary 保存由创建取得的“summary”。
	summary := create(1, 0)
	// before 保存由入场取得的操作前的状态，auth 保存由入场取得的授权。
	before, auth := enter(game, summary.Key)
	if before.Binding != nil || before.BindingSourceSeq != 0 || before.BindingSafeZoneID != 0 {
		t.Fatal("随机新生池关闭时新角色被补造绑定")
	}
	// q 接收请求供后续步骤使用。
	q := request(before, point)
	// bound 保存由绑定安全区域带有回执取得的绑定，receipt 保存由绑定安全区域带有回执取得的回执。
	bound, receipt, err := game.BindSafeZoneWithReceipt(ctx, auth, q)
	must(t, err)
	assertOnlyBindingChanged(before, bound)
	if bound.Binding == nil || *bound.Binding != (domain.RoleBinding{MapID: m.ID, X: int32(zone.Center.X), Y: int32(zone.Center.Y)}) || bound.BindingSourceSeq != q.SourceSeq || bound.BindingSafeZoneID != zone.ID || receipt.Binding == nil || receipt.Binding.ZoneID != zone.ID {
		t.Fatal("未原子保存首匹配来源中心及可信元数据", bound, receipt)
	}
	// 模拟已提交响应未被采用后沿原请求对账；不冒称注入了UnknownTransactionCommitResult。
	// replayed 保存由绑定安全区域带有回执取得的“replayed”，original 保存由绑定安全区域带有回执取得的原始。
	replayed, original, err := game.BindSafeZoneWithReceipt(ctx, auth, q)
	must(t, err)
	if !reflect.DeepEqual(bound, replayed) || original.ID != receipt.ID || original.SourceSeq != receipt.SourceSeq {
		t.Fatal("原请求重放重复推进永久绑定")
	}
	// collection 接收遍历文本列表时当前的元素。
	for _, collection := range []string{"operations", "audit_outbox"} {
		// count 保存由数量文档取得的数量，err 接收数量文档过程中可能产生的错误，供后续失败分支处理。
		count, err := game.DB.Collection(collection).CountDocuments(ctx, bson.M{"_id": receipt.ID})
		must(t, err)
		if count != 1 {
			t.Fatal("绑定回执/审计不是单一永久事实", collection, count)
		}
	}
	// changed 保存当前处理所用的已变化。
	changed := q
	changed.EventID = domain.ID()
	if _, _, err = game.BindSafeZoneWithReceipt(ctx, auth, changed); persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("同来源ID改变事件仍被接受", err)
	}
	// advanced 保存由结算取得的“advanced”。
	advanced, err := game.Settle(ctx, auth, persistence.Settlement{SourceSeq: bound.SourceSeq + 1})
	must(t, err)
	if advanced.BindingSafeZoneID != zone.ID || advanced.BindingSourceSeq != bound.BindingSourceSeq {
		t.Fatal("其他永久操作读取Role丢失已核安全区身份")
	}
	// withoutReceipt 保存由请求取得的“without”回执。
	withoutReceipt := request(advanced, point)
	withoutReceipt.SourceSeq = advanced.SourceSeq
	if _, _, err = game.BindSafeZoneWithReceipt(ctx, auth, withoutReceipt); persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("旧SourceSeq无绑定回执仍被当成功", err)
	}
	// secondPoint 保存当前处理所用的秒点位。
	secondPoint := point
	// candidate 接收遍历安全区域集合时当前的元素。
	for _, candidate := range m.SafeZones {
		if candidate.Center != point {
			secondPoint = candidate.Center
			break
		}
	}
	if secondPoint == point {
		t.Fatal("测试必须区分来源重复行与另一个真实中心")
	}
	// second 保存由请求取得的秒。
	second := request(advanced, secondPoint)
	// latest 保存由绑定安全区域带有回执取得的“latest”，latestReceipt 保存由绑定安全区域带有回执取得的“latest”回执。
	latest, latestReceipt, err := game.BindSafeZoneWithReceipt(ctx, auth, second)
	must(t, err)
	assertOnlyBindingChanged(advanced, latest)
	// late 保存由绑定安全区域带有回执取得的“late”，oldReceipt 保存由绑定安全区域带有回执取得的原状态回执。
	late, oldReceipt, err := game.BindSafeZoneWithReceipt(ctx, auth, q)
	must(t, err)
	if !reflect.DeepEqual(late, latest) || oldReceipt.ID != receipt.ID || late.BindingSourceSeq == oldReceipt.SourceSeq {
		t.Fatal("历史回执将当前绑定回滚")
	}
	// cold 保存由角色取得的“cold”。
	cold, err := next.Role(ctx, latest.ID)
	must(t, err)
	// trusted 保存由会话绑定可信状态取得的可信状态。
	trusted, err := next.BindingTrusted(ctx, cold)
	must(t, err)
	if !trusted || cold.BindingSafeZoneID != latestReceipt.Binding.ZoneID {
		t.Fatal("冷读取无法恢复原已核安全区身份")
	}
	// legacySummary 保存由创建取得的兼容旧数据“summary”。
	legacySummary := create(2, 0)
	// legacyID 标识当前业务使用的兼容旧数据。
	legacyID := domain.Hex(legacySummary.Key.RoleId)
	_, err = authority.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": legacyID}, bson.M{"$set": bson.M{"binding": latestReceipt.Binding.Point}})
	must(t, err)
	// legacy 保存由入场取得的兼容旧数据，legacyAuth 保存由入场取得的兼容旧数据授权。
	legacy, legacyAuth := enter(game, legacySummary.Key)
	trusted, err = game.BindingTrusted(ctx, legacy)
	must(t, err)
	if trusted || legacy.Binding == nil || legacy.BindingSafeZoneID != 0 || legacy.BindingSourceSeq != 0 {
		t.Fatal("同中心历史绑定被免验证提升为可信")
	}
	// proved 保存由绑定安全区域带有回执取得的“proved”。
	proved, _, err := game.BindSafeZoneWithReceipt(ctx, legacyAuth, request(legacy, secondPoint))
	must(t, err)
	if proved.Binding == nil || *proved.Binding != *legacy.Binding || proved.BindingSourceSeq == 0 || proved.BindingSafeZoneID == 0 {
		t.Fatal("同中心无证明跳过了正式来源提交")
	}
	// corruption 接收遍历文本列表时当前的元素。
	for _, corruption := range []string{"missing_receipt", "wrong_kind", "wrong_role", "fraction_zone", "source_hash", "binding_changed", "missing_source_receipt", "zero_source"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(corruption, func(t *testing.T) {
			must(t, authority.Renew(ctx, "account"))
			must(t, game.Renew(ctx, "game"))
			// ticket 保存由签发入场票据取得的入场票据，err 接收签发入场票据过程中可能产生的错误，供后续失败分支处理。
			ticket, err := authority.IssueTicket(ctx, account, summary.Key)
			must(t, err)
			// roleRaw 保存由原始值取得的角色原始值。
			roleRaw, err := game.DB.Collection("roles").FindOne(ctx, bson.M{"_id": latest.ID}).Raw()
			must(t, err)
			// opRaw 保存由原始值取得的操作原始值。
			opRaw, err := game.DB.Collection("operations").FindOne(ctx, bson.M{"_id": latestReceipt.ID}).Raw()
			must(t, err)
			switch corruption {
			case "missing_receipt":
				_, err = game.DB.Collection("operations").DeleteOne(ctx, bson.M{"_id": latestReceipt.ID})
			case "wrong_kind":
				_, err = game.DB.Collection("operations").UpdateOne(ctx, bson.M{"_id": latestReceipt.ID}, bson.M{"$set": bson.M{"kind": "cast_material"}})
			case "wrong_role":
				_, err = game.DB.Collection("operations").UpdateOne(ctx, bson.M{"_id": latestReceipt.ID}, bson.M{"$set": bson.M{"role_id": legacy.ID}})
			case "fraction_zone":
				_, err = game.DB.Collection("operations").UpdateOne(ctx, bson.M{"_id": latestReceipt.ID}, bson.M{"$set": bson.M{"binding_use.zone_id": 1.0}})
			case "source_hash":
				_, err = game.DB.Collection("operations").UpdateOne(ctx, bson.M{"_id": latestReceipt.ID}, bson.M{"$set": bson.M{"binding_use.source_map_sha256": persistence.Hash([]byte("wrong source"))}})
			case "binding_changed":
				_, err = game.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": latest.ID}, bson.M{"$inc": bson.M{"binding.x": 1}})
			case "missing_source_receipt":
				_, err = game.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": latest.ID}, bson.M{"$set": bson.M{"binding_source_seq": advanced.SourceSeq}})
			case "zero_source":
				_, err = game.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": latest.ID}, bson.M{"$set": bson.M{"binding_source_seq": int64(0)}})
			}
			must(t, err)
			// rejected 保存由入场取得的拒绝。
			_, _, rejected := game.Enter(ctx, ticket.Ticket)
			if persistence.Code(rejected) != p.CodeLineForbidden {
				t.Fatal("损坏/伪造绑定证明仍可入场", rejected)
			}
			// storedTicket 记录已持久保存入场票据，使用MongoDB BSON 文档表示。
			var storedTicket bson.M
			must(t, game.DB.Collection("entry_tickets").FindOne(ctx, bson.M{"_id": persistence.Hash([]byte(ticket.Ticket))}).Decode(&storedTicket))
			if storedTicket["used"] != false {
				t.Fatal("拒绝事务仍消费入场票据")
			}
			// 只修复本隔离库注入的原样文档，复用原未消费票据验证回滚完整。
			_, err = game.DB.Collection("roles").ReplaceOne(ctx, bson.M{"_id": latest.ID}, roleRaw)
			must(t, err)
			if corruption == "missing_receipt" {
				_, err = game.DB.Collection("operations").InsertOne(ctx, opRaw)
			} else {
				_, err = game.DB.Collection("operations").ReplaceOne(ctx, bson.M{"_id": latestReceipt.ID}, opRaw)
			}
			must(t, err)
			latest, auth, err = game.Enter(ctx, ticket.Ticket)
			must(t, err)
			if latest.BindingSafeZoneID != latestReceipt.Binding.ZoneID {
				t.Fatal("恢复后未沿原回执恢复安全区身份")
			}
		})
	}
	// 同库按rule15读取旧walk14证明；只补本Store事务专项需要的21候选与真实静态位。
	// 此处QA预置已学21不代表购书/升级流程验收，正常业务由独立WS测试覆盖。
	cfg.Fixture.RuleVersion = 15
	cfg.Fixture.Hash = persistence.Hash([]byte(cfg.Fixture.Hash + ":teleport-proof-v15"))
	// selected 保存当前处理所用的已选择。
	selected := false
	// skill 接收遍历技能集合时当前的元素。
	for _, skill := range cfg.Fixture.Skills {
		selected = selected || skill.ID == 21
	}
	if !selected {
		cfg.Fixture.Skills = append(cfg.Fixture.Skills, devconfig.Skill{ID: 21, JobID: 1, Behavior: "teleport", LearnedLevel: 0, MaxLevel: 3})
	}
	// source 保存由读取文件取得的来源。
	source, err := os.ReadFile("../../../资料参考/myf/Mir200/Map/0.map")
	must(t, err)
	if m.Source == nil || persistence.Hash(source) != m.Source.SHA256 || len(source) != 52+m.Width*m.Height*12 {
		t.Fatal("静态落点测试不接受来源换版或裁图")
	}
	m.SourceStaticBits = make([]byte, (m.Width*m.Height+7)/8)
	// x 规定横坐标的初始取值。
	for x := 0; x < m.Width; x++ {
		// y 规定纵坐标的初始取值。
		for y := 0; y < m.Height; y++ {
			// offset 保存根据当前数据与横坐标与高度的计算结果与纵坐标的计算结果与当前数据的计算结果计算的偏移。
			offset := 52 + (x*m.Height+y)*12
			if (binary.LittleEndian.Uint16(source[offset:])|binary.LittleEndian.Uint16(source[offset+2:])|binary.LittleEndian.Uint16(source[offset+4:]))&0x8000 != 0 {
				// index 保存根据纵坐标与宽度的计算结果与横坐标计算的下标。
				index := y*m.Width + x
				m.SourceStaticBits[index/8] |= 1 << uint(index%8)
			}
		}
	}
	// store 接收遍历持久层存储列表时当前的元素。
	for _, store := range []*persistence.Store{authority, game, next} {
		store.Cfg.Fixture = cfg.Fixture
	}
	must(t, authority.Renew(ctx, "account"))
	must(t, game.Renew(ctx, "game"))
	// oldProof 保存由角色取得的原状态“proof”。
	oldProof, err := game.Role(ctx, latest.ID)
	must(t, err)
	if oldProof.BindingSourceSeq != latest.BindingSourceSeq || oldProof.BindingSafeZoneID != latest.BindingSafeZoneID {
		t.Fatal("rule15废弃了原walk14证明")
	}
	// oldReplay 保存由绑定安全区域带有回执取得的原状态重放，oldOperation 保存由绑定安全区域带有回执取得的原状态操作。
	oldReplay, oldOperation, err := game.BindSafeZoneWithReceipt(ctx, auth, q)
	must(t, err)
	if oldReplay.BindingSourceSeq != latest.BindingSourceSeq || oldOperation.ID != receipt.ID {
		t.Fatal("rule15无法沿旧walk14原摘要对账")
	}
	// wizardSummary 保存由创建取得的“wizard”“summary”。
	wizardSummary := create(3, 1)
	// wizardID 标识当前业务使用的“wizard”。
	wizardID := domain.Hex(wizardSummary.Key.RoleId)
	_, err = game.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": wizardID}, bson.M{"$set": bson.M{"learned_skills": []domain.LearnedSkill{{SkillID: 21, Level: 1}}, "level": uint32(19)}})
	must(t, err)
	// wizard 保存由入场取得的“wizard”，wizardAuth 保存由入场取得的“wizard”授权。
	wizard, wizardAuth := enter(game, wizardSummary.Key)
	// unprovedTeleport 保存由请求取得的“unproved”瞬移。
	unprovedTeleport := request(wizard, secondPoint)
	unprovedTeleport.Action, unprovedTeleport.OriginBindingSourceSeq = "teleport", 1
	if _, _, err = game.BindSafeZoneWithReceipt(ctx, wizardAuth, unprovedTeleport); persistence.Code(err) != p.CodeLineForbidden {
		t.Fatal("已学21但无原可信绑定仍能凭传送标签造证明", err)
	}
	// wizardBound 保存由绑定安全区域带有回执取得的“wizard”绑定。
	wizardBound, _, err := game.BindSafeZoneWithReceipt(ctx, wizardAuth, request(wizard, point))
	must(t, err)
	assertOnlyBindingChanged(wizard, wizardBound)
	// teleport 保存由请求取得的瞬移。
	teleport := request(wizardBound, secondPoint)
	teleport.Action, teleport.OriginBindingSourceSeq = "teleport", wizardBound.BindingSourceSeq
	// teleported 保存由绑定安全区域带有回执取得的已瞬移，teleportReceipt 保存由绑定安全区域带有回执取得的瞬移回执。
	teleported, teleportReceipt, err := game.BindSafeZoneWithReceipt(ctx, wizardAuth, teleport)
	must(t, err)
	assertOnlyBindingChanged(wizardBound, teleported)
	if teleportReceipt.Binding == nil || teleportReceipt.Binding.Action != "teleport" || teleported.Binding == nil || *teleported.Binding == *wizardBound.Binding || teleported.BindingSafeZoneID == 0 {
		t.Fatal("成功传送来源未提交另一个首匹配中心")
	}
	// 当前来源已变为新seq；原请求必须先查回执，不能被旧origin资格检查误拒。
	// teleportReplay 保存由绑定安全区域带有回执取得的瞬移重放，replayReceipt 保存由绑定安全区域带有回执取得的重放回执。
	teleportReplay, replayReceipt, err := game.BindSafeZoneWithReceipt(ctx, wizardAuth, teleport)
	must(t, err)
	if !reflect.DeepEqual(teleportReplay, teleported) || replayReceipt.ID != teleportReceipt.ID {
		t.Fatal("teleport原q重试重复提交或被已更新origin误拒")
	}
	// staleOrigin 保存由请求取得的已失效大区或起点。
	staleOrigin := request(teleported, point)
	staleOrigin.Action, staleOrigin.OriginBindingSourceSeq = "teleport", wizardBound.BindingSourceSeq
	if _, _, err = game.BindSafeZoneWithReceipt(ctx, wizardAuth, staleOrigin); persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("新teleport事件采用旧来源资格", err)
	}
	// coldTeleport 保存由角色取得的“cold”瞬移。
	coldTeleport, err := next.Role(ctx, wizardID)
	must(t, err)
	if coldTeleport.BindingSourceSeq != teleported.BindingSourceSeq || coldTeleport.BindingSafeZoneID != teleportReceipt.Binding.ZoneID {
		t.Fatal("冷读未恢复teleport15 typed证明")
	}
	// oldAuth 保存当前处理所用的原状态授权。
	oldAuth := auth
	// stale 保存由请求取得的已失效。
	stale := request(latest, point)
	must(t, game.Release(ctx, "game"))
	grant, err = authority.ApproveGame(ctx, cfg.NodeID, next.Instance, cfg.RunID)
	must(t, err)
	next.Epoch = grant.Epoch
	if _, _, err = game.BindSafeZoneWithReceipt(ctx, oldAuth, stale); persistence.Code(err) != p.CodeRouteChanged {
		t.Fatal("旧owner绕过写围栏提交绑定", err)
	}
	// current 保存由入场取得的当前状态，newAuth 保存由入场取得的创建授权。
	current, newAuth := enter(next, summary.Key)
	if current.BindingSafeZoneID != latestReceipt.Binding.ZoneID || current.BindingSourceSeq != latest.BindingSourceSeq {
		t.Fatal("新owner冷入场丢失永久来源证明")
	}
	if _, _, err = next.BindSafeZoneWithReceipt(ctx, oldAuth, request(current, point)); persistence.Code(err) != p.CodeStaleSession {
		t.Fatal("新owner接受旧session绑定动作", err)
	}
	// final 保存由绑定安全区域带有回执取得的最终状态。
	final, _, err := next.BindSafeZoneWithReceipt(ctx, newAuth, request(current, point))
	must(t, err)
	assertOnlyBindingChanged(current, final)
	must(t, next.Release(ctx, "game"))
	t.Log("通过：nil/legacy保留、正式来源证明、重复/冲突、无回执旧游标拒绝、历史回执不回滚、冷验/票据回滚、旧owner/session隔离")
}
