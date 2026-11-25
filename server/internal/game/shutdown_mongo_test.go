// 本文件验证受控停服使用真实隔离 MongoDB 结清已累计的托管整秒。
package game

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// TestHostingDrainDuringControlledShutdownMongo 使用真实 MongoDB 检查受控停服结清已经计量的整秒，重复执行收束不产生第二笔扣时。
// t 提供断言失败报告、子用例和清理注册。
func TestHostingDrainDuringControlledShutdownMongo(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// fixture 保存由创建离线托管MongoDB开发测试配置取得的开发测试配置。
	fixture := newHostingMongoFixture(t, cfg)
	// e 接收创建供后续步骤使用。
	e := New(fixture.ctx, fixture.owner)
	t.Cleanup(e.Stop)
	// w 接收房间执行器供后续步骤使用。
	w := e.worker(fixture.role.ID)
	// session 保存由标识取得的会话。
	session := domain.ID()
	// prepared 创建用于维护“prepared”的容器或通道。
	prepared := make(chan bool, 1)
	// w 记录房间执行器。
	w.commands <- command{done: func(w *worker) {
		// r 保存当前处理所用的房间。
		r := &room{Auth: fixture.roleAuth, worldState: &worldState{}}
		w.initialize(r, fixture.role)
		// 常规计时帧暂不推进测试现场，确认只由受控停服发起。
		r.LastTick = time.Now().Add(time.Minute)
		r.HostSession = session
		for range 11 {
			w.meterHosting(r, 200*time.Millisecond, r.LastTick)
		}
		w.rooms[r.Role.ID] = r
		prepared <- !r.Pending && r.hostSecondsOpen == 2 && r.hostRemainder == 200*time.Millisecond
	}}
	select {
	// ok 表明查询、校验或类型转换是否成功。
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
	// role 接收角色供后续步骤使用。
	role, err := fixture.owner.Role(fixture.ctx, fixture.role.ID)
	if err != nil {
		t.Fatal(err)
	}
	if role.HostingSeconds != 3 || role.SourceSeq != fixture.role.SourceSeq+1 || role.Report.SessionID != session || role.Report.UsedSeconds != 2 || role.Report.Kills != 0 {
		t.Fatalf("受控停服未确认恰好两整秒: 余额=%d 来源=%d 战报=%+v", role.HostingSeconds, role.SourceSeq, role.Report)
	}
	// filter 初始化用作筛选条件的MongoDB BSON 文档。
	filter := bson.M{"role_id": role.ID, "source_seq": role.SourceSeq, "host_seconds": int64(2), "hosting_session": session}
	// count 保存由数量文档取得的数量。
	count, err := fixture.owner.DB.Collection("audit_outbox").CountDocuments(fixture.ctx, filter)
	if err != nil || count != 1 {
		t.Fatalf("重复收束应仍只有一笔托管审计: count=%d err=%v", count, err)
	}
}
