// 本文件核对武器用油的付费指令、待落库磨损和持久确认后的属性投影。
package game

import (
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// weaponOilRoom 组装一件当前穿戴、已经产生磨损的原武器。
func weaponOilRoom(t *testing.T) (*worker, *room, *capturedConnection, *pb.WeaponOilCommitRequest) {
	t.Helper()
	w, r, conn := equipmentWorker(t, 0)
	fixture := &w.e.Store.Cfg.Fixture
	fixture.LuckOil = &devconfig.LuckOilConfig{}
	fixture.CombatLuck = luckFixture()
	r.Role.ID, r.Role.Revision, r.Role.EconomicSeq = domain.ID(), 11, 4
	r.Auth.RoleID = r.Role.ID
	w.rooms[r.Role.ID] = r
	weapon := luckTestWeapon()
	weapon.Item.InitialLuck, weapon.Item.Version = 0, 3
	weapon.Item.OriginalDurabilityU, weapon.Item.MaxDurabilityU, weapon.Item.DurabilityU = 100, 100, 90
	r.Role.Equipped = []domain.Equipped{weapon}
	r.Wear.open = []persistence.EquipmentWear{{Slot: weapon.Slot, InstanceID: weapon.Item.ID, Version: weapon.Item.Version, AttackU: 2}}
	request := &pb.WeaponOilCommitRequest{QuoteId: domain.Bytes(domain.ID()), UiSessionId: domain.Bytes(domain.ID()), AllowBound: true,
		Weapon:    &pb.WeaponOilItemRef{InstanceId: domain.Bytes(weapon.Item.ID), Version: uint64(weapon.Item.Version), Equipped: true},
		Operation: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: 5, ExpectedRevision: 11, ExpectedRuleVersion: fixture.RuleVersion}}
	return w, r, conn, request
}

// TestWeaponOilWornCommitDefersToOriginalWear 只有当前在穿武器的首次操作等待同件耐久确认。
func TestWeaponOilWornCommitDefersToOriginalWear(t *testing.T) {
	w, r, conn, request := weaponOilRoom(t)
	if !weaponOilWearPending(r, request) {
		t.Fatal("同件同版本的原磨损需要先确认")
	}
	request.Weapon.Equipped = false
	if weaponOilWearPending(r, request) {
		t.Fatal("背包武器报价不应等待穿戴武器的磨损")
	}
	request.Weapon.Equipped = true
	request.Weapon.Version--
	if weaponOilWearPending(r, request) {
		t.Fatal("较早版本的原武器应交给持久报价核验")
	}
	request.Weapon.Version++
	request.Operation.CommandSeq--
	if weaponOilWearPending(r, request) {
		t.Fatal("已提交原操作必须先查询持久回执")
	}
	request.Operation.CommandSeq++
	frame := wire.Message(p.MsgC2SWeaponOilCommit, 1, conn.Binding(), request)
	now := time.Unix(1800000000, 0)
	w.roomCommand(r, command{role: r.Role.ID, conn: conn, frame: frame, at: now})
	if !r.Pending || r.PendingOperationID != "settle:1" || r.PendingRetry == nil || len(r.Wear.open) != 0 || len(r.Waiting) != 1 || len(conn.frames) != 0 {
		t.Fatal("穿戴用油没有等待原来源结算，也不应预先确认扣油")
	}
	queued, valid := readWeaponOilCommit(r.Waiting[0].frame)
	if !valid || !proto.Equal(queued, request) || !r.Waiting[0].at.Equal(now) || r.Waiting[0].frame.Request != frame.Request {
		t.Fatal("等待耐久的过程中改写了原提交、授权或受理时间")
	}
}

// TestWeaponOilStaleQuoteAndDisabledReplayUseDistinctGates 核对报价错误与永久操作进入的不同路径。
func TestWeaponOilStaleQuoteAndDisabledReplayUseDistinctGates(t *testing.T) {
	w, r, conn, request := weaponOilRoom(t)
	w.e.Store.Cfg.Fixture.LuckOil = nil
	quote := &pb.WeaponOilQuoteRequest{SceneEpoch: r.Scene, Weapon: request.Weapon, UiSessionId: request.UiSessionId}
	frame := wire.Message(p.MsgC2SWeaponOilQuote, 10, conn.Binding(), quote)
	w.roomCommand(r, command{role: r.Role.ID, conn: conn, frame: frame})
	if len(conn.frames) != 1 || conn.frames[0].ID != frame.ID || conn.frames[0].Error != p.CodeWeaponOilUnavailable || r.PreviewInFlight {
		t.Fatal("功能关闭时出现了报价或占用了预览槽")
	}
	quote.SceneEpoch++
	frame = wire.Message(p.MsgC2SWeaponOilQuote, 11, conn.Binding(), quote)
	w.roomCommand(r, command{role: r.Role.ID, conn: conn, frame: frame})
	if len(conn.frames) != 2 || conn.frames[1].Error != p.CodeInvalidArgument {
		t.Fatal("旧场景报价没有被协议入口拒绝")
	}
	request.Operation.CommandSeq = uint64(r.Role.EconomicSeq)
	frame = wire.Message(p.MsgC2SWeaponOilCommit, 12, conn.Binding(), request)
	if !economic(frame.ID) || !waitsForAssetSlot(frame.ID) || !proto.Equal(operationContext(frame), request.Operation) {
		t.Fatal("武器用油没有进入经济身份、队列和重试入口")
	}
	r.dungeonStart = &dungeonStartAttempt{}
	w.command(command{role: r.Role.ID, conn: conn, frame: frame, at: time.Unix(1800000000, 0)})
	if !r.Pending || r.PendingRetry == nil || r.PendingOperationID != domain.Hex(request.Operation.OpId) || len(r.Wear.open) != 1 || len(conn.frames) != 2 {
		t.Fatal("旧操作被功能关闭或待入库磨损提前拦截，无法查询原回执")
	}
}

// TestWeaponOilConfirmedWornLuckIsImmediatelyVisible 确认后原穿戴武器的幸运同步到背包基线与战斗属性。
func TestWeaponOilConfirmedWornLuckIsImmediatelyVisible(t *testing.T) {
	w, r, conn, request := weaponOilRoom(t)
	contract, err := persistence.GrowthContract(w.e.Store.Cfg.Fixture)
	if err != nil {
		t.Fatal(err)
	}
	r.Role.ProgressionFormatVersion, r.Role.ProgressionCurveVersion = contract.FormatVersion, contract.CurveVersion
	r.Role.Capacity = 32
	r.Wear.open = nil
	if got := w.combatAttributesPB(r, &r.Player).LuckAttackPpm; got != 0 {
		t.Fatal("未持久确认就出现培养幸运", got)
	}
	frame := wire.Message(p.MsgC2SWeaponOilCommit, 20, conn.Binding(), request)
	q := persistence.AssetRequest{Kind: persistence.AssetWeaponOilCommit, Context: request.Operation,
		Fingerprint: assetCommandFingerprint(frame), WeaponOilCommit: request}
	w.submitAsset(r, q, frame, "")
	binding := assetBinding{room: r, auth: r.Auth, generation: r.AssetGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
	confirmed := r.Role
	confirmed.Revision++
	confirmed.EconomicSeq++
	confirmed.Equipped = slices.Clone(r.Role.Equipped)
	weapon := &confirmed.Equipped[0].Item
	weapon.Version++
	weapon.CurrentLuck = 1
	result := &pb.OperationResult{OpId: request.Operation.OpId, CommandSeq: uint64(confirmed.EconomicSeq), NewRevision: uint64(confirmed.Revision),
		WeaponOil: &pb.WeaponOilReceipt{QuoteId: request.QuoteId, Weapon: weapon.Proto(), Equipped: true, Success: true}}
	w.finishAsset(binding, q, frame, "", confirmed, result, persistence.Operation{}, nil, nil)
	if r.Pending || r.Isolated || r.Role.Equipped[0].Item.InitialLuck != 0 || r.Role.Equipped[0].Item.EffectiveLuck() != 1 {
		t.Fatal("已确认的培养没有更新原武器，或覆盖了原始幸运")
	}
	var sawInventory, sawScene bool
	for _, sent := range conn.frames {
		switch sent.ID {
		case p.MsgS2CInventory:
			var inventory pb.InventoryResult
			sawInventory = proto.Unmarshal(sent.Payload, &inventory) == nil && len(inventory.Equipped) == 1 &&
				inventory.Equipped[0].Item.GetCurrentLuck() == 1 && inventory.Equipped[0].Item.GetInitialLuck() == 0
		case p.MsgS2CSceneBaseline:
			var scene pb.SceneBaselineChunk
			if err := proto.Unmarshal(sent.Payload, &scene); err != nil {
				t.Fatal(err)
			}
			for _, actor := range scene.Entities {
				if actor.EntityHandle == r.Player.Handle {
					sawScene = actor.CombatAttributes.GetLuckAttackPpm() == 20000
				}
			}
		}
	}
	if !sawInventory || !sawScene {
		t.Fatal("原装备培养确认后没有立即投影新库存或战斗贡献", sawInventory, sawScene)
	}
}
