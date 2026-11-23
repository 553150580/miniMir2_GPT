// 本文件在当前场景校验 NPC 身份和相邻位置，查询该 NPC 的交易报价。
package game

import (
	"math"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// vendorShop 返回商人商店供调用方使用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，scene 记录场景，使用数值表示，npcID 标识当前业务使用的NPC。
func (w *worker) vendorShop(r *room, scene, npcID uint64) *devconfig.NPCShop {
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	if !r.Ready || r.Player.HP <= 0 || scene != r.Scene || f.EquipmentDurability == nil {
		return nil
	}
	// m 接收当前状态地图供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := w.currentMap(r)
	if !ok || m.SceneKindValue() != devconfig.SceneKindTown {
		return nil
	}
	// npc 接收遍历“np”“cs”时当前的元素。
	for _, npc := range m.NPCs {
		if npc.ID == npcID && distance(npc.Position, r.Player.Pos) <= f.EquipmentDurability.RepairRangeCells {
			return f.NPCShopByRole(npc.Role)
		}
	}
	return nil
}

// vendorPreview 处理商人预览。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，f 记录协议帧。
func (w *worker) vendorPreview(r *room, f wire.Frame) {
	if r.Pending || r.PreviewInFlight || r.PreviewGeneration == math.MaxUint64 {
		w.failure(r, f, p.CodeOperationPending)
		return
	}
	// q 保存当前处理所用的商人报价请求。
	q := &pb.VendorQuoteRequest{}
	if proto.Unmarshal(f.Payload, q) != nil {
		w.failure(r, f, p.CodeInvalidArgument)
		return
	}
	// shop 保存由商人商店取得的商店。
	shop := w.vendorShop(r, q.SceneEpoch, q.NpcId)
	if shop == nil {
		w.failure(r, f, p.CodeVendorUnavailable)
		return
	}
	r.PreviewGeneration++
	r.PreviewInFlight = true
	// binding 初始化用作会话绑定的技能预览会话绑定。
	binding := skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
	// store 以报价发起方房间的运行服核对角色授权。
	store := w.roomStore(r)
	// j 初始化用作当前位置的下标的任务或职业编号。
	j := job{target: w, run: func() func(*worker) {
		// result 保存由预览商人取得的结果，err 接收预览商人过程中可能产生的错误，供后续失败分支处理。
		result, err := store.PreviewVendor(w.e.ctx, binding.auth, shop.Key, q)
		// w 记录房间执行器。
		return func(w *worker) {
			if !w.claimPreview(binding) {
				return
			}
			if w.vendorShop(r, q.SceneEpoch, q.NpcId) != shop {
				w.failure(r, f, p.CodeVendorUnavailable)
			} else if err != nil {
				w.failure(r, f, persistence.Code(err))
			} else {
				w.send(r, p.MsgS2CVendorQuote, f.Request, result)
			}
		}
	}}
	if !w.submit(j) {
		r.PreviewInFlight = false
		w.failure(r, f, p.CodeServerBusy)
	}
}
