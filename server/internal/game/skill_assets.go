// 本文件执行技能资产的异步只读预览，按连接、场景和代次控制回调有效性。
package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// skillPreviewBinding 保存技能预览会话绑定所需的状态。
type skillPreviewBinding struct {
	// room 记录房间。
	room *room
	// conn 记录连接。
	conn Connection
	// auth 记录授权。
	auth persistence.Auth
	// scene 记录场景，使用数值表示。
	scene uint64
	// generation 记录代次，使用数值表示。
	generation uint64
}

// skillAssetPreview 处理技能资产预览。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，f 记录协议帧。
func (w *worker) skillAssetPreview(r *room, f wire.Frame) {
	if r.Pending || r.PreviewInFlight {
		w.failure(r, f, p.CodeOperationPending)
		return
	}
	// q 保存当前处理所用的技能资产预览请求。
	q := &pb.SkillAssetPreviewRequest{}
	if proto.Unmarshal(f.Payload, q) != nil {
		w.failure(r, f, p.CodeInvalidArgument)
		return
	}
	// 报价只保留有界的异步槽，不占用冻结临时战斗的永久变更标记。
	r.PreviewGeneration++
	r.PreviewInFlight = true
	// binding 初始化用作会话绑定的技能预览会话绑定。
	binding := skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
	// j 初始化用作当前位置的下标的任务或职业编号。
	j := job{target: w, run: func() func(*worker) {
		// result 保存由预览技能资产取得的结果，err 接收预览技能资产过程中可能产生的错误，供后续失败分支处理。
		result, err := w.e.Store.PreviewSkillAsset(w.e.ctx, binding.auth, q)
		// w 记录房间执行器。
		return func(w *worker) {
			w.finishSkillAssetPreview(binding, f, result, err)
		}
	}}
	if !w.submit(j) {
		r.PreviewInFlight = false
		w.failure(r, f, p.CodeServerBusy)
	}
}

// claimPreview 判断领取预览是否满足要求。
// w 是调用该方法的房间执行器实例，binding 记录会话绑定，使用技能预览会话绑定表示。
func (w *worker) claimPreview(binding skillPreviewBinding) bool {
	// r 保存当前处理所用的房间。
	r := binding.room
	if r.PreviewGeneration != binding.generation || !r.PreviewInFlight {
		return false
	}
	// 只释放本代槽位；新连接或新请求建立的槽位不受旧回调影响。
	r.PreviewInFlight = false
	return w.rooms[binding.auth.RoleID] == r && r.Conn == binding.conn && r.Auth == binding.auth && r.Scene == binding.scene &&
		r.Ready && binding.conn != nil && binding.conn.Alive() && binding.conn.Binding() == uint64(binding.auth.SessionEpoch)
}

// finishSkillAssetPreview 完成技能资产预览。
// w 是调用该方法的房间执行器实例，binding 记录会话绑定，使用技能预览会话绑定表示，f 记录协议帧，result 记录结果，使用技能资产预览表示。
// err 保存当前操作的失败原因，供调用方检查或返回。
func (w *worker) finishSkillAssetPreview(binding skillPreviewBinding, f wire.Frame, result *pb.SkillAssetPreview, err error) {
	if !w.claimPreview(binding) {
		return
	}
	// r 保存当前处理所用的房间。
	r := binding.room
	if err != nil {
		w.failure(r, f, persistence.Code(err))
	} else {
		w.send(r, p.MsgS2CSkillAssetPreview, f.Request, result)
	}
}
