package game

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

func TestHostingDrainDuringControlledShutdownMongo(t *testing.T) {
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture := newHostingMongoFixture(t, cfg)
	e := New(fixture.ctx, fixture.owner)
	t.Cleanup(e.Stop)
	w := e.worker(fixture.role.ID)
	session := domain.ID()
	prepared := make(chan bool, 1)
	w.commands <- command{done: func(w *worker) {
		r := &room{Auth: fixture.roleAuth, worldState: &worldState{}}
		w.initialize(r, fixture.role)
		// 常规计时帧暂不推进测试现场，确认只由受控停服发起。
		r.LastTick = time.Now().Add(time.Minute)
		r.HostSession = session
		for range 11 {
			w.meterHosting(r, 200*time.Millisecond, r.LastTick)
		}
		w.rooms[r.Role.ID] = r
		prepared <- !r.Pending && r.hostSecondsOpen == 2 && r.HostMS == 200
	}}
	select {
	case ok := <-prepared:
		if !ok {
			t.Fatal("停服前应持有两整秒及200毫秒余量，且未提交检查点")
		}
	case <-fixture.ctx.Done():
		t.Fatal(fixture.ctx.Err())
	}
	if err = e.DrainSettlements(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.DrainSettlements(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	role, err := fixture.owner.Role(fixture.ctx, fixture.role.ID)
	if err != nil {
		t.Fatal(err)
	}
	if role.HostingSeconds != 3 || role.SourceSeq != fixture.role.SourceSeq+1 || role.Report.SessionID != session || role.Report.UsedSeconds != 2 || role.Report.Kills != 0 {
		t.Fatalf("受控停服未确认恰好两整秒: 余额=%d 来源=%d 战报=%+v", role.HostingSeconds, role.SourceSeq, role.Report)
	}
	filter := bson.M{"role_id": role.ID, "source_seq": role.SourceSeq, "host_seconds": int64(2), "hosting_session": session}
	count, err := fixture.owner.DB.Collection("audit_outbox").CountDocuments(fixture.ctx, filter)
	if err != nil || count != 1 {
		t.Fatalf("重复收束应仍只有一笔托管审计: count=%d err=%v", count, err)
	}
}
