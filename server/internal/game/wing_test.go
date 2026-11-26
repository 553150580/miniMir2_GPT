// 本文件核对角色专属翅膀的战斗投影、外观独立性和旧操作回放顺序。
package game

import (
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

// TestWingCombatAndInventoryUseOriginalOwner 保证隐藏与切换外观都不影响唯一原翅膀的幸运贡献。
func TestWingCombatAndInventoryUseOriginalOwner(t *testing.T) {
	w, r, _ := equipmentWorker(t, 1)
	f := &w.e.Store.Cfg.Fixture
	f.CombatLuck = luckFixture()
	f.Wings = &devconfig.WingConfig{}
	r.Role.ID = domain.ID()
	r.Role.Equipped = []domain.Equipped{luckTestWeapon()}
	wing := domain.WingState{ID: domain.ID(), OwnerRoleID: r.Role.ID, Rank: 3, AppearanceTier: 3, Luck: 9,
		OilToSix: 100, OilToNine: 3000, Version: 4}
	r.Role.Wing = &wing
	if got := w.combatAttributesPB(r, &r.Player).GetLuckAttackPpm(); got != 344000 {
		t.Fatal("武器 +3 与翅膀 +9 必须各只计一次", got)
	}
	snapshot := w.freezeBossCombatSnapshot(r, &f.Map)
	wing.Hidden, wing.AppearanceTier, wing.Version = true, 1, 5
	if got := w.combatAttributesPB(r, &r.Player).GetLuckAttackPpm(); got != 344000 || snapshot.Fighter.LuckAttackPPM != got {
		t.Fatal("隐藏或换外观改写原幸运贡献或既有挑战快照", got, snapshot.Fighter.LuckAttackPPM)
	}
	contract, err := persistence.GrowthContract(*f)
	if err != nil {
		t.Fatal(err)
	}
	r.Role.ProgressionFormatVersion, r.Role.ProgressionCurveVersion = contract.FormatVersion, contract.CurveVersion
	view, err := snapshotRoleAssets(r.Role, *f)
	if err != nil {
		t.Fatal(err)
	}
	if value := view.inventory.GetWing(); value == nil || domain.Hex(value.WingId) != wing.ID || value.Version != 5 ||
		value.AppearanceTier != 1 || value.OilToSix != 100 || value.OilToNine != 3000 || !value.Hidden {
		t.Fatal("角色库存快照没有投影同一把永久翅膀", value)
	}
	wing.OwnerRoleID = domain.ID()
	if got := w.combatAttributesPB(r, &r.Player).GetLuckAttackPpm(); got != 44000 {
		t.Fatal("其它角色的实例影响了当前职业属性", got)
	}
	wing.OwnerRoleID = r.Role.ID
	f.Wings = nil
	view, err = snapshotRoleAssets(r.Role, *f)
	if err != nil || view.inventory.GetWing() != nil || w.fighter(r, &r.Player).LuckAttackPPM != 44000 {
		t.Fatal("关闭本规则时原配置错误公开翅膀或让翅膀参与战斗", view.inventory)
	}
}

// TestWingHistoricalActionUsesOriginalEconomyWhenUnavailable 验证历史请求不会被首次动作门禁截断。
func TestWingHistoricalActionUsesOriginalEconomyWhenUnavailable(t *testing.T) {
	w, r, conn := equipmentWorker(t, 0)
	r.Role.ID, r.Role.Revision, r.Role.EconomicSeq = domain.ID(), 11, 5
	r.Auth.RoleID = r.Role.ID
	w.rooms[r.Role.ID] = r
	preview := &pb.WingQuoteRequest{SceneEpoch: r.Scene, UiSessionId: domain.Bytes(domain.ID())}
	frame := wire.Message(p.MsgC2SWingQuote, 1, conn.Binding(), preview)
	w.roomCommand(r, command{role: r.Role.ID, conn: conn, frame: frame})
	if len(conn.frames) != 1 || conn.frames[0].Error != p.CodeWingUnavailable || r.PreviewInFlight {
		t.Fatal("默认配置的翅膀报价没有拒绝或占用了预览槽")
	}
	request := &pb.WingActionRequest{Action: 2, WingId: domain.Bytes(domain.ID()), WingVersion: 2, AppearanceTier: 1, Hidden: true,
		Operation: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: 5, ExpectedRevision: 10}}
	frame = wire.Message(p.MsgC2SWingAction, 2, conn.Binding(), request)
	if !economic(frame.ID) || !waitsForAssetSlot(frame.ID) || !proto.Equal(operationContext(frame), request.Operation) {
		t.Fatal("翅膀操作没有进入原经济身份和有界等待槽")
	}
	r.dungeonStart = &dungeonStartAttempt{}
	w.command(command{role: r.Role.ID, conn: conn, frame: frame, at: time.Unix(1800000000, 0)})
	if !r.Pending || r.PendingRetry == nil || r.PendingOperationID != domain.Hex(request.Operation.OpId) || len(conn.frames) != 1 {
		t.Fatal("旧翅膀回执无法在关功能或挑战门禁下沿原请求继续查证")
	}
}
