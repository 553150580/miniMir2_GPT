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

type previewConnection struct {
	capturedConnection
	alive   bool
	binding uint64
}

func (c *previewConnection) Alive() bool         { return c.alive }
func (c *previewConnection) Binding() uint64     { return c.binding }
func (c *previewConnection) SetBinding(v uint64) { c.binding = v }

func previewWorker() (*worker, *room, *previewConnection, wire.Frame) {
	w, r, _ := potionWorker()
	c := &previewConnection{alive: true, binding: 3}
	r.Conn = c
	r.Auth.RoleID = "role"
	r.Monsters[0].Monster = &devconfig.Monster{}
	w.e.ctx = context.Background()
	w.e.jobs = make(chan job, 2)
	f := wire.Message(p.MsgC2SSkillAssetPreview, 44, 3, &pb.SkillAssetPreviewRequest{Kind: 1, OfferId: 1, Quantity: 1})
	return w, r, c, f
}

func currentPreviewBinding(r *room) skillPreviewBinding {
	return skillPreviewBinding{room: r, conn: r.Conn, auth: r.Auth, scene: r.Scene, generation: r.PreviewGeneration}
}

func TestSkillPreviewInFlightDoesNotFreezeCombatOrRecovery(t *testing.T) {
	w, r, c, f := previewWorker()
	w.skillAssetPreview(r, f)
	if r.Pending || !r.PreviewInFlight || len(w.e.jobs) != 1 {
		t.Fatal("异步报价占用永久Pending，或未持有唯一预览槽")
	}
	w.skillAssetPreview(r, f)
	if len(w.e.jobs) != 1 || len(c.frames) != 1 || c.frames[0].Error != p.CodeOperationPending {
		t.Fatal("同代第二个报价没有被有界拒绝")
	}
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

func TestSkillPreviewCompletionPreservesConcurrentPermanentPending(t *testing.T) {
	w, r, c, f := previewWorker()
	w.skillAssetPreview(r, f)
	binding := currentPreviewBinding(r)
	// 已受理的永久命令可以在报价I/O期间进入Pending；报价回调无权解冻它。
	r.Pending = true
	r.Waiting = []command{{}}
	result := &pb.SkillAssetPreview{Kind: 1, Quantity: 2}
	w.finishSkillAssetPreview(binding, f, result, nil)
	if r.PreviewInFlight || !r.Pending || len(r.Waiting) != 1 || len(c.frames) != 1 {
		t.Fatal("预览回调修改了永久Pending或排队命令")
	}
	got := &pb.SkillAssetPreview{}
	if c.frames[0].ID != p.MsgS2CSkillAssetPreview || c.frames[0].Request != f.Request || c.frames[0].Binding != 3 || proto.Unmarshal(c.frames[0].Payload, got) != nil || !proto.Equal(result, got) {
		t.Fatal("当前连接没有沿原请求收到实际预览结果")
	}
	w.finishSkillAssetPreview(binding, f, result, nil)
	if len(c.frames) != 1 {
		t.Fatal("已终结的预览回调重复投递")
	}
}

func TestSkillPreviewStaleBindingNeverDeliversToReplacement(t *testing.T) {
	for name, invalidate := range map[string]func(*worker, *room, *previewConnection){
		"connection": func(_ *worker, r *room, _ *previewConnection) { r.Conn = &previewConnection{alive: true, binding: 3} },
		"session":    func(_ *worker, r *room, _ *previewConnection) { r.Auth.SessionEpoch++ },
		"auth":       func(_ *worker, r *room, _ *previewConnection) { r.Auth.AuthVersion++ },
		"scene":      func(_ *worker, r *room, _ *previewConnection) { r.Scene++ },
		"room":       func(w *worker, _ *room, _ *previewConnection) { delete(w.rooms, "role") },
		"baseline":   func(_ *worker, r *room, _ *previewConnection) { r.Ready = false },
		"closed":     func(_ *worker, _ *room, c *previewConnection) { c.alive = false },
		"binding":    func(_ *worker, _ *room, c *previewConnection) { c.binding++ },
	} {
		t.Run(name, func(t *testing.T) {
			w, r, c, f := previewWorker()
			w.skillAssetPreview(r, f)
			binding := currentPreviewBinding(r)
			invalidate(w, r, c)
			w.finishSkillAssetPreview(binding, f, &pb.SkillAssetPreview{Kind: 1}, nil)
			if len(c.frames) != 0 || r.PreviewInFlight {
				t.Fatal("失效报价仍投递，或没有释放自身代次的槽")
			}
			if next, ok := r.Conn.(*previewConnection); ok && len(next.frames) != 0 {
				t.Fatal("旧连接报价被发送到替代连接")
			}
		})
	}
}

func TestSkillPreviewOldTerminalCannotClearNewGeneration(t *testing.T) {
	for name, terminal := range map[string]error{"success": nil, "error": persistence.Fail(p.CodeSkillPreviewStale, "旧报价失效")} {
		t.Run(name, func(t *testing.T) {
			w, r, c, f := previewWorker()
			w.skillAssetPreview(r, f)
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

func TestSkillPreviewErrorReleasesOnlyPreviewSlot(t *testing.T) {
	w, r, c, f := previewWorker()
	w.skillAssetPreview(r, f)
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

func TestSkillPreviewQueueBusyAndMalformedLeaveNoPending(t *testing.T) {
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

func TestSkillPreviewDisconnectInvalidatesQueuedCompletion(t *testing.T) {
	w, r, c, f := previewWorker()
	w.skillAssetPreview(r, f)
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
