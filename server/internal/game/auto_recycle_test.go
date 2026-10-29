package game

import (
	"errors"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"testing"
)

func recycleOwner(t *testing.T) (*worker, *room, *capturedConnection, assetBinding, persistence.AssetRequest) {
	t.Helper()
	r, c := observedRoom()
	r.Pet = nil
	r.Auto = false
	r.Role = domain.Role{ID: domain.ID(), Origin: 1, Level: 1, Revision: 10, EconomicSeq: 5, SourceSeq: 4, Capacity: 32}
	r.Auth.RoleID = r.Role.ID
	r.AssetGeneration = 7
	r.Pending = true
	r.PendingOperationID = domain.ID()
	r.PendingFingerprint = "original-payload"
	w := observedWorker(t, r)
	w.rooms = map[string]*room{r.Role.ID: r}
	w.e.Store.Cfg.Fixture.RuleVersion = 19
	b := assetBinding{room: r, auth: r.Auth, generation: 7, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
	q := persistence.AssetRequest{Kind: "set_auto_recycle", Fingerprint: b.fingerprint, Context: &pb.OperationContext{OpId: domain.Bytes(b.id)}}
	return w, r, c, b, q
}

func TestAutoRecycleAssetUnknownKeepsExactOwnerSlotAndNoSuccessNotification(t *testing.T) {
	w, r, c, b, q := recycleOwner(t)
	retry := &job{}
	w.finishAsset(b, q, wire.Frame{Request: 9}, "", domain.Role{Revision: 11}, &pb.OperationResult{ErrorCode: p.CodeOperationPending}, persistence.Operation{}, persistence.Fail(p.CodeOperationPending, "unknown"), retry)
	if !r.Pending || r.PendingRetry != retry || r.PendingOperationID != b.id || r.PendingFingerprint != b.fingerprint || r.AssetGeneration != 7 || r.Role.Revision != 10 || len(c.frames) != 1 || c.frames[0].ID != p.MsgS2COperationResult {
		t.Fatal("未知提交更换原事实或发假成功")
	}
}

func TestAutoRecycleAssetOldAuthorityOrSlotCannotClearCurrentPending(t *testing.T) {
	for _, name := range []string{"room", "auth", "generation", "operation", "fingerprint"} {
		t.Run(name, func(t *testing.T) {
			w, r, c, b, q := recycleOwner(t)
			switch name {
			case "room":
				w.rooms[r.Role.ID] = &room{}
			case "auth":
				r.Auth.SessionEpoch++
			case "generation":
				r.AssetGeneration++
			case "operation":
				r.PendingOperationID = domain.ID()
			case "fingerprint":
				r.PendingFingerprint = "new"
			}
			id, fp, generation := r.PendingOperationID, r.PendingFingerprint, r.AssetGeneration
			for _, err := range []error{nil, persistence.Fail(p.CodeOperationPending, "unknown"), errors.New("network")} {
				w.finishAsset(b, q, wire.Frame{}, "", domain.Role{Revision: 99}, &pb.OperationResult{}, persistence.Operation{}, err, &job{})
			}
			if !r.Pending || r.Role.Revision != 10 || r.PendingOperationID != id || r.PendingFingerprint != fp || r.AssetGeneration != generation || len(c.frames) != 0 {
				t.Fatal("旧回调清掉新槽或更新了当前观察")
			}
		})
	}
}

func TestAutoRecycleCommitSendsCompleteInventoryBeforeExactReceipt(t *testing.T) {
	w, r, c, b, q := recycleOwner(t)
	next := r.Role
	next.Revision++
	next.EconomicSeq++
	next.AutoRecycle = domain.AutoRecyclePolicy{Enabled: true, QualityIDs: []uint32{0}, Revision: 1}
	op := persistence.Operation{ID: "asset:" + r.Role.ID + ":" + b.id, RoleID: r.Role.ID, Revision: next.Revision, Recycling: &persistence.EquipmentRecyclingReceipt{PolicyRevision: 1, Trigger: 2, Recycled: []domain.Item{{ID: domain.ID(), Bound: true}}, BoundGold: 50}}
	w.finishAsset(b, q, wire.Frame{Request: 9}, "", next, &pb.OperationResult{}, op, nil, &job{})
	if r.Pending || r.Auto || r.Role.SourceSeq != 4 {
		t.Fatal("保存策略改变Auto或来源队列")
	}
	inventoryIndex, eventIndex := -1, -1
	for i, f := range c.frames {
		if f.ID == p.MsgS2CInventory {
			var v pb.InventoryResult
			_ = proto.Unmarshal(f.Payload, &v)
			if v.Revision != 11 || v.AutoRecycle.Revision != 1 {
				t.Fatal(v)
			}
			inventoryIndex = i
		}
		if f.ID == p.MsgS2CEquipmentRecycleResult {
			var v pb.EquipmentRecycleResult
			_ = proto.Unmarshal(f.Payload, &v)
			if v.RoleRevision != 11 || v.SourceSeq != 0 || domain.Hex(v.OpId) != b.id || v.RecycledCount != 1 || v.BoundGold != 50 || v.Trigger != 2 {
				t.Fatal(v)
			}
			eventIndex = i
		}
	}
	if inventoryIndex < 0 || eventIndex <= inventoryIndex {
		t.Fatal("回收事件未在对应完整Inventory之后发送", c.frames)
	}
	before := len(c.frames)
	w.finishAsset(b, q, wire.Frame{}, "", next, &pb.OperationResult{}, op, nil, &job{})
	if len(c.frames) != before {
		t.Fatal("同完成回调重放再次发送事件")
	}
}

func TestRecycleDiscardOnlyNotificationAndNoAttachmentHistoryReplay(t *testing.T) {
	w, r, c, _, _ := recycleOwner(t)
	op := persistence.Operation{RoleID: r.Role.ID, SourceSeq: 5, Revision: 11, Recycling: &persistence.EquipmentRecyclingReceipt{PolicyRevision: 2, Trigger: 1, DiscardedCount: 1, DiscardReason: "bag_full"}}
	w.equipmentRecycleResult(r, op)
	if len(c.frames) != 1 {
		t.Fatal("零回收但真丢弃必须可见")
	}
	var v pb.EquipmentRecycleResult
	_ = proto.Unmarshal(c.frames[0].Payload, &v)
	if v.RecycledCount != 0 || v.DiscardedCount != 1 || v.SourceSeq != 5 || v.DiscardReason != "bag_full" || len(v.OpId) != 0 {
		t.Fatal(v)
	}
	r.Ready = false
	w.equipmentRecycleResult(r, op)
	r.Ready = true
	r.Isolated = true
	w.equipmentRecycleResult(r, op)
	r.Isolated = false
	op.RoleID = domain.ID()
	w.equipmentRecycleResult(r, op)
	if len(c.frames) != 1 {
		t.Fatal("未Ready/旧角色收到补播结果")
	}
}

func TestAutoRecycleAndLockAreEconomicCommandsWithOriginalOperation(t *testing.T) {
	op := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: 6, ExpectedRevision: 10, ExpectedRuleVersion: 19}
	for _, tc := range []struct {
		id  uint16
		msg proto.Message
	}{
		{p.MsgC2SSetAutoRecycle, &pb.SetAutoRecycleRequest{Policy: &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0}}, Operation: op}},
		{p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: domain.Bytes(domain.ID()), ItemVersion: 3, Locked: true, Operation: op}},
	} {
		f := wire.Message(tc.id, 8, 3, tc.msg)
		if !economic(tc.id) || !proto.Equal(operationContext(f), op) {
			t.Fatal("新请求绕过经济门禁或操作身份", tc.id)
		}
		w, r, _, _, _ := recycleOwner(t)
		r.Pending = false
		w.asset(r, f)
		if !r.Pending || r.PendingOperationID != domain.Hex(op.OpId) || r.PendingRetry == nil || r.AssetGeneration != 8 {
			t.Fatal("新请求未占原owner单槽", tc.id)
		}
	}
}
