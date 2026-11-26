// 本文件回归装备回收的操作身份、重试、通知次序、锁定和背包满丢弃行为。
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
	"time"
)

// recycleOwner 处理回收所有者。
// t 提供断言失败报告、子用例和清理注册。
func recycleOwner(t *testing.T) (*worker, *room, *capturedConnection, assetBinding, persistence.AssetRequest) {
	t.Helper()
	// r 接收观察到的房间供后续步骤使用，c 接收观察到的房间供后续步骤使用。
	r, c := observedRoom()
	r.Pet = nil
	r.Auto = false
	r.Role = domain.Role{ID: domain.ID(), Origin: 1, Level: 1, Revision: 10, EconomicSeq: 5, SourceSeq: 4, Capacity: 32}
	r.Auth.RoleID = r.Role.ID
	r.AssetGeneration = 7
	r.Pending = true
	r.PendingOperationID = domain.ID()
	r.PendingFingerprint = "original-payload"
	// w 接收观察到的房间执行器供后续步骤使用。
	w := observedWorker(t, r)
	w.rooms = map[string]*room{r.Role.ID: r}
	w.e.Store.Cfg.Fixture.RuleVersion = 19
	// b 建立带初始值的资产会话绑定，供本段继续处理。
	b := assetBinding{room: r, auth: r.Auth, generation: 7, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
	// q 组装资产请求所需的数据。
	q := persistence.AssetRequest{Kind: "set_auto_recycle", Fingerprint: b.fingerprint, Context: &pb.OperationContext{OpId: domain.Bytes(b.id)}}
	return w, r, c, b, q
}

// TestAutoRecycleAssetUnknownKeepsExactOwnerSlotAndNoSuccessNotification 核对「未知提交更换原事实或发假成功」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAutoRecycleAssetUnknownKeepsExactOwnerSlotAndNoSuccessNotification(t *testing.T) {
	// w 接收回收所有者供后续步骤使用，r 接收回收所有者供后续步骤使用，c 接收回收所有者供后续步骤使用，b 保存由回收所有者取得的b，q 接收回收所有者供后续步骤使用。
	w, r, c, b, q := recycleOwner(t)
	// retry 保存当前处理所用的重试。
	retry := &job{}
	w.finishAsset(b, q, wire.Frame{Request: 9}, "", domain.Role{Revision: 11}, &pb.OperationResult{ErrorCode: p.CodeOperationPending}, persistence.Operation{}, persistence.Fail(p.CodeOperationPending, "unknown"), retry)
	if !r.Pending || r.PendingRetry != retry || r.PendingOperationID != b.id || r.PendingFingerprint != b.fingerprint || r.AssetGeneration != 7 || r.Role.Revision != 10 || len(c.frames) != 1 || c.frames[0].ID != p.MsgS2COperationResult {
		t.Fatal("未知提交更换原事实或发假成功")
	}
}

// TestAutoRecycleAssetOldAuthorityOrSlotCannotClearCurrentPending 核对「旧回调清掉新槽或更新了当前观察」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAutoRecycleAssetOldAuthorityOrSlotCannotClearCurrentPending(t *testing.T) {
	// name 接收遍历文本列表时当前的元素。
	for _, name := range []string{"room", "auth", "generation", "operation", "fingerprint"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// w 接收回收所有者供后续步骤使用，r 接收回收所有者供后续步骤使用，c 接收回收所有者供后续步骤使用，b 保存由回收所有者取得的b，q 接收回收所有者供后续步骤使用。
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
			// id 区分当前步骤指向的目标对象，fp 保存待确认“fingerprint”，用于当前步骤中的“fp”，generation 保存资产代次，用于当前步骤中的代次。
			id, fp, generation := r.PendingOperationID, r.PendingFingerprint, r.AssetGeneration
			// err 接收遍历错误列表时当前的元素。
			for _, err := range []error{nil, persistence.Fail(p.CodeOperationPending, "unknown"), errors.New("network")} {
				w.finishAsset(b, q, wire.Frame{}, "", domain.Role{Revision: 99}, &pb.OperationResult{}, persistence.Operation{}, err, &job{})
			}
			if !r.Pending || r.Role.Revision != 10 || r.PendingOperationID != id || r.PendingFingerprint != fp || r.AssetGeneration != generation || len(c.frames) != 0 {
				t.Fatal("旧回调清掉新槽或更新了当前观察")
			}
		})
	}
}

// TestAutoRecycleCommitSendsCompleteInventoryBeforeExactReceipt 核对「回收事件未在对应完整Inventory之后发送」与「保存策略改变Auto或来源队列」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAutoRecycleCommitSendsCompleteInventoryBeforeExactReceipt(t *testing.T) {
	// w 接收回收所有者供后续步骤使用，r 接收回收所有者供后续步骤使用，c 接收回收所有者供后续步骤使用，b 保存由回收所有者取得的b，q 接收回收所有者供后续步骤使用。
	w, r, c, b, q := recycleOwner(t)
	// next 保存角色，用于当前步骤中的下一状态。
	next := r.Role
	next.Revision++
	next.EconomicSeq++
	next.AutoRecycle = domain.AutoRecyclePolicy{Enabled: true, QualityIDs: []uint32{0}, Revision: 1}
	// op 组装操作所需的数据。
	op := persistence.Operation{ID: "asset:" + r.Role.ID + ":" + b.id, RoleID: r.Role.ID, Revision: next.Revision, Recycling: &persistence.EquipmentRecyclingReceipt{PolicyRevision: 1, Trigger: 2, Recycled: []domain.Item{{ID: domain.ID(), Bound: true}}, BoundGold: 50}}
	w.finishAsset(b, q, wire.Frame{Request: 9}, "", next, &pb.OperationResult{}, op, nil, &job{})
	if r.Pending || r.Auto || r.Role.SourceSeq != 4 {
		t.Fatal("保存策略改变Auto或来源队列")
	}
	// inventoryIndex 保存当前处理所用的背包资产下标，eventIndex 保存当前处理所用的事件下标。
	inventoryIndex, eventIndex := -1, -1
	// i 定位协议帧集合中正在处理的成员，f 接收遍历协议帧集合时当前的元素。
	for i, f := range c.frames {
		if f.ID == p.MsgS2CInventory {
			// v 记录背包资产结果。
			var v pb.InventoryResult
			_ = proto.Unmarshal(f.Payload, &v)
			if v.Revision != 11 || v.AutoRecycle.Revision != 1 {
				t.Fatal(&v)
			}
			inventoryIndex = i
		}
		if f.ID == p.MsgS2CEquipmentRecycleResult {
			// v 记录装备回收结果。
			var v pb.EquipmentRecycleResult
			_ = proto.Unmarshal(f.Payload, &v)
			if v.RoleRevision != 11 || v.SourceSeq != 0 || domain.Hex(v.OpId) != b.id || v.RecycledCount != 1 || v.BoundGold != 50 || v.Trigger != 2 {
				t.Fatal(&v)
			}
			eventIndex = i
		}
	}
	if inventoryIndex < 0 || eventIndex <= inventoryIndex {
		t.Fatal("回收事件未在对应完整Inventory之后发送", c.frames)
	}
	// before 记录协议帧集合中的条目数。
	before := len(c.frames)
	w.finishAsset(b, q, wire.Frame{}, "", next, &pb.OperationResult{}, op, nil, &job{})
	if len(c.frames) != before {
		t.Fatal("同完成回调重放再次发送事件")
	}
}

// TestRecycleDiscardOnlyNotificationAndNoAttachmentHistoryReplay 核验零回收但真丢弃必须可见，用诊断「未Ready/旧角色收到补播结果」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestRecycleDiscardOnlyNotificationAndNoAttachmentHistoryReplay(t *testing.T) {
	// w 接收回收所有者供后续步骤使用，r 接收回收所有者供后续步骤使用，c 接收回收所有者供后续步骤使用。
	w, r, c, _, _ := recycleOwner(t)
	// op 组装操作所需的数据。
	op := persistence.Operation{RoleID: r.Role.ID, SourceSeq: 5, Revision: 11, Recycling: &persistence.EquipmentRecyclingReceipt{PolicyRevision: 2, Trigger: 1, DiscardedCount: 1, DiscardReason: "bag_full"}}
	w.equipmentRecycleResult(r, op)
	if len(c.frames) != 1 {
		t.Fatal("零回收但真丢弃必须可见")
	}
	// v 记录装备回收结果。
	var v pb.EquipmentRecycleResult
	_ = proto.Unmarshal(c.frames[0].Payload, &v)
	if v.RecycledCount != 0 || v.DiscardedCount != 1 || v.SourceSeq != 5 || v.DiscardReason != "bag_full" || len(v.OpId) != 0 {
		t.Fatal(&v)
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

// TestAutoRecycleAndLockAreEconomicCommandsWithOriginalOperation 核对「新请求绕过经济门禁或操作身份」与「新请求未占原owner单槽」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAutoRecycleAndLockAreEconomicCommandsWithOriginalOperation(t *testing.T) {
	// op 保存当前处理所用的操作。
	op := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: 6, ExpectedRevision: 10, ExpectedRuleVersion: 19}
	// tc 接收遍历临时结构列表时当前的元素。
	for _, tc := range []struct {
		// id 区分当前步骤指向的目标对象。
		id uint16
		// msg 记录“msg”，使用消息表示。
		msg proto.Message
	}{
		{p.MsgC2SSetAutoRecycle, &pb.SetAutoRecycleRequest{Policy: &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0}}, Operation: op}},
		{p.MsgC2SSetMiningAutoRecycle, &pb.SetMiningAutoRecycleRequest{Enabled: true, Operation: op}},
		{p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: domain.Bytes(domain.ID()), ItemVersion: 3, Locked: true, Operation: op}},
	} {
		// f 接收消息供后续步骤使用。
		f := wire.Message(tc.id, 8, 3, tc.msg)
		if !economic(tc.id) || !proto.Equal(operationContext(f), op) {
			t.Fatal("新请求绕过经济门禁或操作身份", tc.id)
		}
		if tc.id == p.MsgC2SSetMiningAutoRecycle && !waitsForAssetSlot(tc.id) {
			t.Fatal("矿石回收设置必须等待已经受理的采矿结算")
		}
		// w 接收回收所有者供后续步骤使用，r 接收回收所有者供后续步骤使用。
		w, r, _, _, _ := recycleOwner(t)
		r.Pending = false
		w.asset(r, f, time.Time{})
		if !r.Pending || r.PendingOperationID != domain.Hex(op.OpId) || r.PendingRetry == nil || r.AssetGeneration != 8 {
			t.Fatal("新请求未占原owner单槽", tc.id)
		}
	}
}
