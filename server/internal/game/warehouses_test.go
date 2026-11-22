// 本文件回归仓库分页与回调隔离、失效内容保护和已确认操作重放。
package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"testing"
	"time"
)

// TestWarehouseReadDoesNotFreezeCombatAndCannotClearPermanentPending 核对「查询回调释放新永久任务」与「查询占永久槽」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestWarehouseReadDoesNotFreezeCombatAndCannotClearPermanentPending(t *testing.T) {
	// w 接收预览房间执行器供后续步骤使用，r 接收预览房间执行器供后续步骤使用，c 接收预览房间执行器供后续步骤使用。
	w, r, c, _ := previewWorker()
	r.Role.ID = domain.ID()
	r.Role.Origin = 1
	// f 接收消息供后续步骤使用。
	f := wire.Message(p.MsgC2SWarehouseQuery, 31, 3, &pb.WarehouseQuery{Scope: 1, Limit: 30})
	w.warehouseQuery(r, f)
	// b 保存由当前状态预览会话绑定取得的b。
	b := currentPreviewBinding(r)
	if r.Pending || !r.PreviewInFlight || len(w.e.jobs) != 1 {
		t.Fatal("查询占永久槽")
	}
	w.attack(r, &r.Player, r.Monsters[0], time.Unix(1800000000, 0))
	if r.Player.AttackSequence != 1 {
		t.Fatal("查询暂停战斗")
	}
	r.Pending = true
	r.PendingOperationID = "new-op"
	r.PendingFingerprint = "new-digest"
	// page 保存当前处理所用的页面或分页。
	page := &pb.WarehousePage{Role: r.Role.Summary().Key, Scope: 1, Revision: 0, Capacity: 30, Limit: 30}
	w.finishWarehouseQuery(b, f, page, nil)
	if !r.Pending || r.PendingOperationID != "new-op" || r.PreviewInFlight || c.frames[len(c.frames)-1].ID != p.MsgS2CWarehousePage {
		t.Fatal("查询回调释放新永久任务")
	}
}

// TestWarehouseReadStaleAndExpiredNeverLeaksContent 核对「旧附着/过期回调泄漏仓储」与「旧回调清新查询槽」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestWarehouseReadStaleAndExpiredNeverLeaksContent(t *testing.T) {
	// kind 接收遍历文本列表时当前的元素。
	for _, kind := range []string{"old-conn", "old-scene", "expired", "new-query"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(kind, func(t *testing.T) {
			// w 接收预览房间执行器供后续步骤使用，r 接收预览房间执行器供后续步骤使用，c 接收预览房间执行器供后续步骤使用。
			w, r, c, _ := previewWorker()
			r.Role.ID = domain.ID()
			r.Role.Origin = 1
			r.Role.PrivilegeUntil = time.Now().Add(time.Hour)
			// f 接收消息供后续步骤使用。
			f := wire.Message(p.MsgC2SWarehouseQuery, 31, 3, &pb.WarehouseQuery{Scope: 2, Limit: 30})
			w.warehouseQuery(r, f)
			// b 保存由当前状态预览会话绑定取得的b。
			b := currentPreviewBinding(r)
			// page 保存当前处理所用的页面或分页。
			page := &pb.WarehousePage{Role: r.Role.Summary().Key, Scope: 2, Capacity: 50, Gold: 11}
			switch kind {
			case "old-conn":
				r.Conn = &previewConnection{alive: true, binding: 3}
			case "old-scene":
				r.Scene++
			case "expired":
				r.Role.PrivilegeUntil = time.Now().Add(-time.Second)
			case "new-query":
				r.PreviewGeneration++
			}
			w.finishWarehouseQuery(b, f, page, nil)
			// f 接收遍历协议帧集合时当前的元素。
			for _, f := range c.frames {
				if f.ID == p.MsgS2CWarehousePage {
					t.Fatal("旧附着/过期回调泄漏仓储")
				}
			}
			if kind == "new-query" && !r.PreviewInFlight {
				t.Fatal("旧回调清新查询槽")
			}
		})
	}
}

// TestWarehouseCommittedReplayExpiresWithoutContentOrNewEffects 核对「旧回调清理新Pending或重播」与「回执权限/世界状态不符」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestWarehouseCommittedReplayExpiresWithoutContentOrNewEffects(t *testing.T) {
	// active 接收遍历布尔状态列表时当前的元素。
	for _, active := range []bool{false, true} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(map[bool]string{false: "expired", true: "active"}[active], func(t *testing.T) {
			// w 接收预览房间执行器供后续步骤使用，r 接收预览房间执行器供后续步骤使用，c 接收预览房间执行器供后续步骤使用。
			w, r, c, _ := previewWorker()
			r.Role.ID = domain.ID()
			r.Role.Origin = 1
			if active {
				r.Role.PrivilegeUntil = time.Now().Add(time.Hour)
			}
			// id 区分当前步骤指向的目标对象。
			id := domain.ID()
			r.Pending = true
			r.PendingOperationID = id
			r.PendingFingerprint = "warehouse"
			r.AssetGeneration = 7
			// b 建立带初始值的资产会话绑定，供本段继续处理。
			b := assetBinding{room: r, auth: r.Auth, generation: 7, id: id, fingerprint: "warehouse"}
			// op 组装操作所需的数据。
			op := persistence.Operation{ID: "asset:" + r.Role.ID + ":" + id, Revision: r.Role.Revision, Warehouse: &persistence.WarehouseReceipt{Scope: 2, Action: 3, RevisionAfter: 1, Capacity: 50, GoldAfter: 17}}
			// hp 保存当前处理所用的生命值，life 保存当前处理所用的生命代次，mp 保存当前处理所用的魔法值。
			hp, life, mp := r.Player.HP, r.Player.Life, r.Player.MP
			w.finishAsset(b, persistence.AssetRequest{Kind: "warehouse"}, wire.Frame{ID: p.MsgC2SWarehouseAction}, "", r.Role, &pb.OperationResult{}, op, nil, nil)
			// result 保存当前处理所用的结果，contents 保存当前处理所用的“contents”。
			result, contents := false, false
			// f 接收遍历协议帧集合时当前的元素。
			for _, f := range c.frames {
				result = result || f.ID == p.MsgS2COperationResult
				contents = contents || f.ID == p.MsgS2CWarehouseActionResult
			}
			if !result || contents != active || r.Player.HP != hp || r.Player.MP != mp || r.Player.Life != life || r.Pending {
				t.Fatal("回执权限/世界状态不符", result, contents)
			}
			// count 记录协议帧集合中的条目数。
			count := len(c.frames)
			r.Pending = true
			r.PendingOperationID = "new-op"
			r.AssetGeneration++
			w.finishAsset(b, persistence.AssetRequest{}, wire.Frame{}, "", r.Role, &pb.OperationResult{}, op, nil, nil)
			if !r.Pending || len(c.frames) != count {
				t.Fatal("旧回调清理新Pending或重播")
			}
		})
	}
}
