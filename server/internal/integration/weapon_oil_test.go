// 本文件用真实联机和原经济回执核对武器祝福油、穿戴磨损与掉线重放。
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestWeaponOilMongoWS 操作真实油堆，核对背包、穿戴和失去原响应后的永久结果。
func TestWeaponOilMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	raw, err := os.ReadFile(os.Getenv("MINIMIR_LUCK_OIL_TEST_FIXTURE"))
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	if fixture.LuckOil == nil || fixture.LuckOil.Steps[4] != (devconfig.LuckOilStep{SuccessPPM: 300_000, FailureLuck: 3, PityAfter: 100}) {
		t.Fatal("隔离输入没有工作簿的幸运 +4 档")
	}
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) { cfg.Fixture = fixture })
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "oil_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	role := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: route.origin.OriginID,
		Slot: 1, Job: 0, Name: "用油" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, role, 0)
	roleID := domain.Hex(role.Key.RoleId)
	mainWeapon := persistence.InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1}, &fixture)
	secondWeapon := mainWeapon
	secondWeapon.ID, secondWeapon.InitialLuck, secondWeapon.CurrentLuck, secondWeapon.LuckOilToSix = domain.ID(), 3, 4, 99
	freeOil := domain.Item{ID: domain.ID(), ItemID: fixture.LuckOil.ItemID, Quantity: 3, Version: 1}
	boundOil := domain.Item{ID: domain.ID(), ItemID: fixture.LuckOil.ItemID, Quantity: 8, Version: 1, Bound: true}
	_, err = store.DB.Collection("roles").UpdateByID(ctx, roleID, bson.M{"$set": bson.M{
		"items": []domain.Item{mainWeapon, secondWeapon, freeOil, boundOil}, "equipped": []domain.Equipped{},
	}})
	must(t, err)
	attach := func() *client {
		ticket := directoryTicket(t, endpoint, login.AccountToken, role.Key, route)
		return join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
	}
	readRole := func() domain.Role {
		current, err := store.Role(ctx, roleID)
		must(t, err)
		return current
	}
	c := attach()
	defer func() { c.close() }()
	c.auto(false, 1)
	sessionID := domain.Bytes(domain.ID())

	// 关闭绑材时，真实报价选择非绑堆，首瓶按已确认规则必成。
	bagRef := &pb.WeaponOilItemRef{InstanceId: domain.Bytes(mainWeapon.ID), Version: 1}
	first := weaponOilQuoteWS(c, bagRef, sessionID, false)
	if first.SuccessPpm != 1_000_000 || first.Guaranteed || first.OutputBound || domain.Hex(first.Oil.InstanceId) != freeOil.ID {
		t.Fatal("第一瓶报价没有选中原非绑定油堆或基础必成档", first)
	}
	firstResult := c.asset(p.MsgC2SWeaponOilCommit, weaponOilCommitWS(c, first, bagRef))
	worn := firstResult.GetWeaponOil().GetWeapon()
	if firstResult.ErrorCode != 0 || !firstResult.GetWeaponOil().GetSuccess() || worn.GetCurrentLuck() != 1 || worn.GetInitialLuck() != 0 {
		t.Fatal("背包武器未保留原初始值并升到 +1", firstResult)
	}
	if c.inventory.GetRevision() < firstResult.NewRevision {
		c.inventoryAt(firstResult.NewRevision)
	}
	if code := luckyEquip(c, mainWeapon.ID, devconfig.WeaponSlot, worn.Version, false); code != 0 {
		t.Fatal("不能穿戴刚培养的原武器", code)
	}
	luckyAttributes(c, 20_000, 0)

	// 穿戴武器报价后发生真实普攻。旧报价不能覆盖已经发生的耐久损耗。
	current := readRole()
	if len(current.Equipped) != 1 {
		t.Fatal("武器未进入权威穿戴槽", current.Equipped)
	}
	wornItem := current.Equipped[0].Item
	wearRef := &pb.WeaponOilItemRef{InstanceId: domain.Bytes(mainWeapon.ID), Version: uint64(wornItem.Version), Equipped: true}
	heldQuote := weaponOilQuoteWS(c, wearRef, sessionID, false)
	attack := c.send(p.MsgC2SAttackIntent, &pb.AttackIntent{SceneEpoch: c.entry.SceneEpoch, InputSequence: 1, Direction: 2})
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool {
		if frame.Request == attack && frame.Error != 0 {
			return true
		}
		var accepted pb.AttackAccepted
		return frame.ID == p.MsgS2CAttackAccepted && proto.Unmarshal(frame.Payload, &accepted) == nil &&
			accepted.InputSequence == 1 && accepted.CasterHandle == c.entry.PlayerHandle
	})
	if frame.Error != 0 {
		t.Fatal("用于验证耐久的真实普攻没有受理", frame.Error)
	}
	stale := c.asset(p.MsgC2SWeaponOilCommit, weaponOilCommitWS(c, heldQuote, wearRef))
	if stale.ErrorCode != p.CodeWeaponOilQuoteChanged || stale.WeaponOil != nil {
		t.Fatal("旧报价覆盖了已经发生的原磨损", stale)
	}
	current = readRole()
	if equipped := current.Equipped[0].Item; equipped.ID != mainWeapon.ID || equipped.Version <= wornItem.Version ||
		equipped.DurabilityU >= wornItem.DurabilityU || equipped.EffectiveLuck() != 1 {
		t.Fatal("旧报价失败后原武器磨损或幸运记录有误", equipped)
	}
	if index := slices.IndexFunc(current.Items, func(item domain.Item) bool { return item.ID == freeOil.ID }); index < 0 || current.Items[index].Quantity != 2 {
		t.Fatal("穿戴武器旧报价误扣了非绑定油")
	}

	// 绑定油在第 100 瓶仍按原概率。只在独立测试库固定隐藏抽样，让失败和回退必现。
	secondRef := &pb.WeaponOilItemRef{InstanceId: domain.Bytes(secondWeapon.ID), Version: 1}
	quote := weaponOilQuoteWS(c, secondRef, sessionID, true)
	if !quote.OutputBound || quote.Guaranteed || quote.SuccessPpm != 300_000 || quote.FailureLuck != 3 ||
		quote.PityUsed != 99 || quote.PityAfter != 100 || domain.Hex(quote.Oil.InstanceId) != boundOil.ID {
		t.Fatal("第 100 瓶或绑定优先的原报价有误", quote)
	}
	fixed, err := store.DB.Collection("weapon_oil_quotes").UpdateOne(ctx, bson.M{"_id": roleID, "quote_id": domain.Hex(quote.QuoteId)},
		bson.M{"$set": bson.M{"roll": uint32(999_999)}})
	must(t, err)
	if fixed.MatchedCount != 1 {
		t.Fatal("不能固定本次隔离报价的隐藏抽样")
	}
	wrong := weaponOilCommitWS(c, quote, secondRef)
	wrong.AllowBound = false
	if got := c.asset(p.MsgC2SWeaponOilCommit, wrong); got.ErrorCode != p.CodeWeaponOilQuoteChanged || got.WeaponOil != nil {
		t.Fatal("关闭绑定授权后的旧报价仍成交", got)
	}
	request := weaponOilCommitWS(c, quote, secondRef)
	c.send(p.MsgC2SWeaponOilCommit, request)
	confirmed := waitWeaponOilReceipt(t, ctx, store, roleID, domain.Hex(request.Operation.OpId))
	c.close()
	c = attach()
	replayed := c.asset(p.MsgC2SWeaponOilCommit, request)
	original := confirmed.WeaponOil.Proto()
	if replayed.Pending || replayed.ErrorCode != 0 || !proto.Equal(original, replayed.WeaponOil) || original.Success ||
		original.PityUsed != 100 || original.Weapon.CurrentLuck != 3 || !original.Weapon.Bound || original.Weapon.InitialLuck != 3 {
		t.Fatal("失败回退、即时绑定或新票重放没有从同一次原事务取结果", replayed)
	}
	current = readRole()
	if index := slices.IndexFunc(current.Items, func(item domain.Item) bool { return item.ID == boundOil.ID }); index < 0 || current.Items[index].Quantity != 7 {
		t.Fatal("失败响应重放重复扣了一瓶绑定油")
	}
	nextRef := &pb.WeaponOilItemRef{InstanceId: original.Weapon.InstanceId, Version: original.Weapon.Version}
	next := weaponOilQuoteWS(c, nextRef, domain.Bytes(domain.ID()), false)
	if !next.Guaranteed || next.CurrentLuck != 3 || next.SuccessLuck != 4 || next.SuccessPpm != 1_000_000 ||
		next.PityUsed != 100 || domain.Hex(next.Oil.InstanceId) != freeOil.ID || !next.OutputBound {
		t.Fatal("达到保底后的下一瓶应逐级必成，非绑油也不能解绑", next)
	}
	result := c.asset(p.MsgC2SWeaponOilCommit, weaponOilCommitWS(c, next, nextRef))
	if result.ErrorCode != 0 || !result.GetWeaponOil().GetSuccess() || result.GetWeaponOil().GetWeapon().GetCurrentLuck() != 4 ||
		result.GetWeaponOil().GetPityUsed() != 101 {
		t.Fatal("第 101 瓶未按原武器逐瓶必成", result)
	}
	t.Log("真实联机观察首瓶升至 +1、穿戴磨损拒绝旧报价、第 100 瓶失败回退并绑定、重连只返原结果、第 101 瓶必成")
}

// weaponOilQuoteWS 只接受同一观察请求收到的真实报价。
func weaponOilQuoteWS(c *client, ref *pb.WeaponOilItemRef, session []byte, allowBound bool) *pb.WeaponOilQuote {
	c.t.Helper()
	id := c.send(p.MsgC2SWeaponOilQuote, &pb.WeaponOilQuoteRequest{SceneEpoch: c.entry.SceneEpoch, Weapon: ref, UiSessionId: session, AllowBound: allowBound})
	frame := travelUntilWithin(c, 5*time.Second, func(frame wire.Frame) bool { return frame.Request == id })
	if frame.Error != 0 || frame.ID != p.MsgS2CWeaponOilQuote {
		c.t.Fatal("服务器未签发武器用油报价", frame.ID, frame.Error)
	}
	var quote pb.WeaponOilQuote
	must(c.t, proto.Unmarshal(frame.Payload, &quote))
	return &quote
}

// weaponOilCommitWS 确认已展示的原报价和原武器，以及本次原角色操作身份。
func weaponOilCommitWS(c *client, quote *pb.WeaponOilQuote, ref *pb.WeaponOilItemRef) *pb.WeaponOilCommitRequest {
	operation := c.operation()
	operation.ExpectedRevision = quote.RoleRevision
	return &pb.WeaponOilCommitRequest{QuoteId: quote.QuoteId, UiSessionId: quote.UiSessionId, AllowBound: quote.AllowBound, Weapon: ref, Operation: operation}
}

// waitWeaponOilReceipt 按原操作身份查询提交结果，不靠前端回包断定真实入库。
func waitWeaponOilReceipt(t *testing.T, ctx context.Context, store *persistence.Store, roleID, operationID string) persistence.Operation {
	t.Helper()
	var receipt persistence.Operation
	deadline := time.Now().Add(7 * time.Second)
	for {
		err := store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + roleID + ":" + operationID}).Decode(&receipt)
		if !errors.Is(err, mongo.ErrNoDocuments) || time.Now().After(deadline) {
			must(t, err)
			break
		}
		time.Sleep(35 * time.Millisecond)
	}
	if receipt.Kind != persistence.AssetWeaponOilCommit || receipt.Code != 0 || receipt.WeaponOil == nil {
		t.Fatal("用油未生成已确认的原永久回执", receipt)
	}
	return receipt
}
