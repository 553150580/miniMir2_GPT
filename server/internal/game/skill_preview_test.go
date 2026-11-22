// 本文件回归技能预览不冻结战斗，不覆盖资产在途状态，以及失效回调和队列忙处理。
package game

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// previewConnection 保存预览连接所需的状态。
type previewConnection struct {
	// capturedConnection 提供嵌入的已捕获连接能力。
	capturedConnection
	// alive 标记是否处于存活状态状态。
	alive bool
	// binding 携带连接上的会话代次，用于防止旧连接命令被接受。
	binding uint64
}

// Alive 判断存活状态是否满足要求。
// c 是调用该方法的预览连接实例。
func (c *previewConnection) Alive() bool { return c.alive }

// Binding 返回会话绑定供调用方使用。
// c 是调用该方法的预览连接实例。
func (c *previewConnection) Binding() uint64 { return c.binding }

// SetBinding 设置会话绑定。
// c 是调用该方法的预览连接实例，v 记录数值。
func (c *previewConnection) SetBinding(v uint64) { c.binding = v }

// previewWorker 处理预览房间执行器。
func previewWorker() (*worker, *room, *previewConnection, wire.Frame) {
	// w 接收药水房间执行器供后续步骤使用，r 接收药水房间执行器供后续步骤使用。
	w, r, _ := potionWorker()
	// c 保存当前处理所用的预览连接。
	c := &previewConnection{alive: true, binding: 3}
	r.Conn = c
	r.Auth.RoleID = "role"
	r.Monsters[0].Monster = &devconfig.Monster{}
	w.e.ctx = context.Background()
	w.e.jobs = make(chan job, 2)
	// f 接收消息供后续步骤使用。
	f := wire.Message(p.MsgC2SSkillAssetPreview, 44, 3, &pb.SkillAssetPreviewRequest{Kind: 1, OfferId: 1, Quantity: 1})
	return w, r, c, f
}

// currentPreviewBinding 返回当前状态预览会话绑定供调用方使用。
// r 指向当前执行器拥有并更新的目标房间。
func currentPreviewBinding(r *room) skillPreviewBinding {
	return skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
}

// TestSkillPreviewInFlightDoesNotFreezeCombatOrRecovery 核验同代第二个报价应被有界拒绝，用诊断「异步报价占用永久Pending，或未持有唯一预览槽」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPreviewInFlightDoesNotFreezeCombatOrRecovery(t *testing.T) {
	// w 接收预览房间执行器供后续步骤使用，r 接收预览房间执行器供后续步骤使用，c 接收预览房间执行器供后续步骤使用，f 接收预览房间执行器供后续步骤使用。
	w, r, c, f := previewWorker()
	w.skillAssetPreview(r, f)
	if r.Pending || !r.PreviewInFlight || len(w.e.jobs) != 1 {
		t.Fatal("异步报价占用永久Pending，或未持有唯一预览槽")
	}
	w.skillAssetPreview(r, f)
	if len(w.e.jobs) != 1 || len(c.frames) != 1 || c.frames[0].Error != p.CodeOperationPending {
		t.Fatal("同代第二个报价没有被有界拒绝")
	}
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	w.attack(r, &r.Player, r.Monsters[0], now)
	if r.Player.AttackSequence != 1 || r.Monsters[0].HP != 99 {
		t.Fatal("等待报价期间普攻被停住")
	}
	r.Auto, r.MonsterCount, r.Pet = false, 0, nil
	r.Potions.HP, r.Potions.NextTick = 8, now.Add(-time.Millisecond)
	r.LastTick, r.LastBroadcast = now, now
	w.step(r, now)
	if r.Player.HP != 14 || r.Potions.HP != 4 || !r.PreviewInFlight || r.Pending {
		t.Fatal("等待报价期间恢复队列或场景时钟被停住")
	}
}

// TestSkillPreviewCompletionPreservesConcurrentPermanentPending 核验当前连接应沿原请求收到实际预览结果，用诊断「预览回调修改了永久Pending或排队命令」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPreviewCompletionPreservesConcurrentPermanentPending(t *testing.T) {
	// w 接收预览房间执行器供后续步骤使用，r 接收预览房间执行器供后续步骤使用，c 接收预览房间执行器供后续步骤使用，f 接收预览房间执行器供后续步骤使用。
	w, r, c, f := previewWorker()
	w.skillAssetPreview(r, f)
	// binding 保存由当前状态预览会话绑定取得的会话绑定。
	binding := currentPreviewBinding(r)
	// 已受理的永久命令可以在报价I/O期间进入Pending；报价回调无权解冻它。
	r.Pending = true
	r.Waiting = []command{{}}
	// result 保存当前处理所用的结果。
	result := &pb.SkillAssetPreview{Kind: 1, Quantity: 2}
	w.finishSkillAssetPreview(binding, f, result, nil)
	if r.PreviewInFlight || !r.Pending || len(r.Waiting) != 1 || len(c.frames) != 1 {
		t.Fatal("预览回调修改了永久Pending或排队命令")
	}
	// got 保存当前处理所用的实际结果。
	got := &pb.SkillAssetPreview{}
	if c.frames[0].ID != p.MsgS2CSkillAssetPreview || c.frames[0].Request != f.Request || c.frames[0].Binding != 3 || proto.Unmarshal(c.frames[0].Payload, got) != nil || !proto.Equal(result, got) {
		t.Fatal("当前连接没有沿原请求收到实际预览结果")
	}
	w.finishSkillAssetPreview(binding, f, result, nil)
	if len(c.frames) != 1 {
		t.Fatal("已终结的预览回调重复投递")
	}
}

// TestSkillPreviewStaleBindingNeverDeliversToReplacement 核验失效报价仍投递，或应释放自身代次的槽，用诊断「旧连接报价被发送到替代连接」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPreviewStaleBindingNeverDeliversToReplacement(t *testing.T) {
	// name 定位按文本索引的供当前流程调用的函数映射中正在处理的成员，invalidate 接收遍历按文本索引的供当前流程调用的函数映射时当前的元素。
	for name, invalidate := range map[string]func(*worker, *room, *previewConnection){
		// r 指向当前执行器拥有并更新的目标房间。
		"connection": func(_ *worker, r *room, _ *previewConnection) { r.Conn = &previewConnection{alive: true, binding: 3} },
		// r 指向当前执行器拥有并更新的目标房间。
		"session": func(_ *worker, r *room, _ *previewConnection) { r.Auth.SessionEpoch++ },
		// r 指向当前执行器拥有并更新的目标房间。
		"auth": func(_ *worker, r *room, _ *previewConnection) { r.Auth.AuthVersion++ },
		// r 指向当前执行器拥有并更新的目标房间。
		"scene": func(_ *worker, r *room, _ *previewConnection) { r.Scene++ },
		// w 记录房间执行器。
		"room": func(w *worker, _ *room, _ *previewConnection) { delete(w.rooms, "role") },
		// r 指向当前执行器拥有并更新的目标房间。
		"baseline": func(_ *worker, r *room, _ *previewConnection) { r.Ready = false },
		// c 记录预览连接。
		"closed": func(_ *worker, _ *room, c *previewConnection) { c.alive = false },
		// c 记录预览连接。
		"binding": func(_ *worker, _ *room, c *previewConnection) { c.binding++ },
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// w 接收预览房间执行器供后续步骤使用，r 接收预览房间执行器供后续步骤使用，c 接收预览房间执行器供后续步骤使用，f 接收预览房间执行器供后续步骤使用。
			w, r, c, f := previewWorker()
			w.skillAssetPreview(r, f)
			// binding 保存由当前状态预览会话绑定取得的会话绑定。
			binding := currentPreviewBinding(r)
			invalidate(w, r, c)
			w.finishSkillAssetPreview(binding, f, &pb.SkillAssetPreview{Kind: 1}, nil)
			if len(c.frames) != 0 || r.PreviewInFlight {
				t.Fatal("失效报价仍投递，或没有释放自身代次的槽")
			}
			// next 按目标类型读取当前接口值，得到下一状态，ok 表明运行时值能否按要求的类型读取。
			if next, ok := r.Conn.(*previewConnection); ok && len(next.frames) != 0 {
				t.Fatal("旧连接报价被发送到替代连接")
			}
		})
	}
}

// TestSkillPreviewOldTerminalCannotClearNewGeneration 核对「旧成功/错误回调清除了后来代次，或越代回复」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPreviewOldTerminalCannotClearNewGeneration(t *testing.T) {
	// name 定位按文本索引的错误映射中正在处理的成员，terminal 接收遍历按文本索引的错误映射时当前的元素。
	for name, terminal := range map[string]error{"success": nil, "error": persistence.Fail(p.CodeSkillPreviewStale, "旧报价失效")} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// w 接收预览房间执行器供后续步骤使用，r 接收预览房间执行器供后续步骤使用，c 接收预览房间执行器供后续步骤使用，f 接收预览房间执行器供后续步骤使用。
			w, r, c, f := previewWorker()
			w.skillAssetPreview(r, f)
			// binding 保存由当前状态预览会话绑定取得的会话绑定。
			binding := currentPreviewBinding(r)
			r.PreviewGeneration++
			r.PreviewInFlight = true
			r.Pending = true
			w.finishSkillAssetPreview(binding, f, &pb.SkillAssetPreview{}, terminal)
			if !r.PreviewInFlight || !r.Pending || len(c.frames) != 0 {
				t.Fatal("旧成功/错误回调清除了后来代次，或越代回复")
			}
		})
	}
}

// TestSkillPreviewErrorReleasesOnlyPreviewSlot 核验当前代错误应沿原请求回复，或错误释放永久Pending，以及错误终态后不能重新受理新报价。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPreviewErrorReleasesOnlyPreviewSlot(t *testing.T) {
	// w 接收预览房间执行器供后续步骤使用，r 接收预览房间执行器供后续步骤使用，c 接收预览房间执行器供后续步骤使用，f 接收预览房间执行器供后续步骤使用。
	w, r, c, f := previewWorker()
	w.skillAssetPreview(r, f)
	// binding 保存由当前状态预览会话绑定取得的会话绑定。
	binding := currentPreviewBinding(r)
	r.Pending = true
	w.finishSkillAssetPreview(binding, f, nil, persistence.Fail(p.CodeSkillPreviewStale, "报价条件变化"))
	if r.PreviewInFlight || !r.Pending || len(c.frames) != 1 || c.frames[0].ID != f.ID || c.frames[0].Request != f.Request || c.frames[0].Error != p.CodeSkillPreviewStale {
		t.Fatal("当前代错误没有沿原请求回复，或错误释放永久Pending")
	}
	r.Pending = false
	w.skillAssetPreview(r, f)
	if !r.PreviewInFlight || r.PreviewGeneration == binding.generation || len(w.e.jobs) != 2 {
		t.Fatal("错误终态后不能重新受理新报价")
	}
}

// TestSkillPreviewQueueBusyAndMalformedLeaveNoPending 核对「任务队列满未明确拒绝，或留下预览/永久Pending」与「畸形报价占用了异步槽」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPreviewQueueBusyAndMalformedLeaveNoPending(t *testing.T) {
	// w 接收预览房间执行器供后续步骤使用，r 接收预览房间执行器供后续步骤使用，c 接收预览房间执行器供后续步骤使用，f 接收预览房间执行器供后续步骤使用。
	w, r, c, f := previewWorker()
	w.e.jobs = make(chan job)
	w.skillAssetPreview(r, f)
	if r.Pending || r.PreviewInFlight || len(c.frames) != 1 || c.frames[0].Error != p.CodeServerBusy {
		t.Fatal("任务队列满未明确拒绝，或留下预览/永久Pending")
	}
	f.Payload = []byte{0xff}
	w.skillAssetPreview(r, f)
	if r.Pending || r.PreviewInFlight || len(c.frames) != 2 || c.frames[1].Error != p.CodeInvalidArgument {
		t.Fatal("畸形报价占用了异步槽")
	}
}

// TestSkillPreviewDisconnectInvalidatesQueuedCompletion 核验当前连接断开应独立失效预览，用诊断「已断开的原连接收到旧报价」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPreviewDisconnectInvalidatesQueuedCompletion(t *testing.T) {
	// w 接收预览房间执行器供后续步骤使用，r 接收预览房间执行器供后续步骤使用，c 接收预览房间执行器供后续步骤使用，f 接收预览房间执行器供后续步骤使用。
	w, r, c, f := previewWorker()
	w.skillAssetPreview(r, f)
	// binding 保存由当前状态预览会话绑定取得的会话绑定。
	binding := currentPreviewBinding(r)
	w.command(command{role: "role", conn: c, disconnect: true})
	if r.PreviewInFlight || r.PreviewGeneration == binding.generation || r.Conn != nil || r.Pending {
		t.Fatal("当前连接断开没有独立失效预览")
	}
	w.finishSkillAssetPreview(binding, f, &pb.SkillAssetPreview{}, nil)
	if len(c.frames) != 0 {
		t.Fatal("已断开的原连接收到旧报价")
	}
}
