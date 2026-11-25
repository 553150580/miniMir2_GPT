// 本文件经真实 WebSocket 和隔离 MongoDB 验证挂机结算与技能槽独立比较。
package game

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestAttackSkillSlotsSettlementCASMongoWS 控制结算任务的执行顺序，所有帧和数据库事务走实际入口。
func TestAttackSkillSlotsSettlementCASMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("技能槽竞争验收需要提供独立 MongoDB 副本集")
	}
	w, r, _ := stageBWorker(t, 2, devconfig.SummonSkeletonSkillID, devconfig.SummonShinsuSkillID)
	fixture := newHostingMongoFixture(t, w.e.Store.Cfg)
	ctx, cancel := context.WithCancel(fixture.ctx)
	defer cancel()
	owner := fixture.owner
	roleSummary, err := fixture.account.CreateRole(ctx, fixture.accountAuth, &pb.CreateRoleRequest{
		OriginServerId: fixture.owner.Cfg.OriginID, Slot: 2, Job: r.Role.Job, Name: "槽位竞争" + domain.ID()[:5], OpId: domain.Bytes(domain.ID()),
	})
	if err != nil {
		t.Fatal(err)
	}
	roleID := domain.Hex(roleSummary.Key.RoleId)
	// 仅准备已学召唤的道士，后续结算、库存修订和槽位都由原事务写入。
	if _, err := owner.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": roleID}, bson.M{"$set": bson.M{
		"level": r.Role.Level, "learned_skills": r.Role.Skills,
	}}); err != nil {
		t.Fatal(err)
	}
	ticket, err := fixture.account.IssueTicket(ctx, fixture.accountAuth, roleSummary.Key)
	if err != nil {
		t.Fatal(err)
	}
	before, auth, err := owner.Enter(ctx, ticket.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	w.e.Store, w.e.ctx, w.e.jobs = owner, ctx, make(chan job, 1)
	w.rooms = map[string]*room{roleID: r}
	r.Role, r.Auth, r.Pet, r.Auto = before, auth, nil, true

	// 使用测试传输适配器附着已签票并确认的原角色，控制房间任务与网络帧交错顺序。
	accepted := make(chan *websocket.Conn, 1)
	web := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		socket, err := websocket.Accept(resp, req, nil)
		if err != nil {
			return
		}
		accepted <- socket
		<-ctx.Done()
		socket.CloseNow()
	}))
	t.Cleanup(web.Close)
	socket, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(web.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.CloseNow()
	conn := &movementWSConnection{socket: <-accepted, ctx: ctx}
	binding := uint64(auth.SessionEpoch)
	conn.SetBinding(binding)
	r.Conn = conn
	send := func(id uint16, request uint32, value proto.Message) {
		t.Helper()
		if err := socket.Write(ctx, websocket.MessageBinary, wire.Encode(wire.Message(id, request, binding, value))); err != nil {
			t.Fatal(err)
		}
		_, raw, err := conn.socket.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		frame, err := wire.Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		w.command(command{role: roleID, conn: conn, frame: frame})
	}
	receive := func(id uint16, request uint32, value proto.Message) {
		t.Helper()
		for {
			_, raw, err := socket.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			frame, err := wire.Decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if frame.Request != request || frame.ID != id {
				continue
			}
			if frame.Binding != binding || frame.Error != 0 || proto.Unmarshal(frame.Payload, value) != nil {
				t.Fatalf("回包身份或类型无效：消息%d 请求%d", id, request)
			}
			return
		}
	}
	complete := func() {
		t.Helper()
		select {
		case pending := <-w.e.jobs:
			pending.run()(w)
		default:
			t.Fatal("当前房间缺少应执行的数据库任务")
		}
	}
	queued := []uint32{devconfig.SummonSkeletonSkillID, devconfig.SummonShinsuSkillID, 0, 0}
	setSlots := func(request uint32, sequence, revision uint64, initial []uint32) *pb.SetSkillSlotsRequest {
		value := &pb.SetSkillSlotsRequest{SkillSlotIds: queued, ExpectedSkillSlotIds: initial, Operation: &pb.OperationContext{
			OpId: domain.Bytes(domain.ID()), CommandSeq: sequence,
			ExpectedRevision: revision, ExpectedRuleVersion: owner.Cfg.Fixture.RuleVersion,
		}}
		send(p.MsgC2SSetSkillSlots, request, value)
		return value
	}

	// 让一笔真实挂机收益事务先占住角色，确定性地在结算窗口送达技能槽点击。
	w.settle(r, persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Experience: 1, Gold: 1, EventAt: time.Now(),
		Kills: 1, MonsterID: r.Monsters[0].Def, MapID: owner.Cfg.Fixture.Map.ID,
	})
	initial := attackSlotsForRole(before, owner.Cfg.Fixture)
	first := setSlots(1, uint64(before.EconomicSeq+1), uint64(before.Revision), initial)
	if len(r.Waiting) != 1 || r.PendingOperationID == domain.Hex(first.Operation.OpId) {
		t.Fatal("原击杀结算忙时收到的槽位命令未保留在有界队列")
	}
	send(p.MsgC2SAutoBattle, 2, &pb.AutoBattleIntent{SceneEpoch: r.Scene,
		AutoSkillSettings: &pb.AutoSkillSettings{Enabled: true, DisabledSkillIds: queued[:1]},
	})
	settings := &pb.HostingState{}
	receive(p.MsgS2CHostingState, 2, settings)
	if !settings.AutoBattleEnabled || !settings.GetAutoSkillSettings().GetEnabled() || !slices.Equal(settings.GetAutoSkillSettings().GetDisabledSkillIds(), queued[:1]) {
		t.Fatalf("结算期间未确认原生自动施法设置：%v", settings)
	}
	complete()
	complete()
	saved := &pb.OperationResult{}
	receive(p.MsgS2COperationResult, 1, saved)
	if saved.Pending || saved.ErrorCode != 0 || !bytes.Equal(saved.OpId, first.Operation.OpId) ||
		saved.NewRevision != uint64(before.Revision+2) || saved.CommandSeq != uint64(before.EconomicSeq+1) || r.Pending || len(r.Waiting) != 0 {
		t.Fatalf("只有挂机收益推进修订时，原技能槽点击必须一次完成：%v", saved)
	}
	// 四槽其实已变化和缺省比较字段的旧请求都要拒绝。现行经济序号不得掩盖真实槽位冲突。
	for _, check := range []struct {
		request  uint32
		baseline []uint32
	}{{3, initial}, {4, nil}} {
		next := setSlots(check.request, saved.CommandSeq+1, uint64(before.Revision), check.baseline)
		complete()
		conflict := &pb.OperationResult{}
		receive(p.MsgS2COperationResult, check.request, conflict)
		if conflict.Pending || conflict.ErrorCode != p.CodeOperationConflict || !bytes.Equal(conflict.OpId, next.Operation.OpId) ||
			conflict.NewRevision != saved.NewRevision || conflict.CommandSeq != saved.CommandSeq {
			t.Fatalf("过期槽位或旧格式请求未按原经济身份拒绝：%v", conflict)
		}
	}
	send(p.MsgC2SSetSkillSlots, 5, first)
	complete()
	replayed := &pb.OperationResult{}
	receive(p.MsgS2COperationResult, 5, replayed)
	stored, err := owner.Role(ctx, roleID)
	if err != nil {
		t.Fatal(err)
	}
	count, err := owner.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": roleID, "kind": persistence.AssetSetSkillSlots})
	if err != nil || !proto.Equal(saved, replayed) || count != 1 || !slices.Equal(stored.SkillSlotIDs, queued) ||
		stored.EconomicSeq != before.EconomicSeq+1 || stored.SourceSeq != before.SourceSeq+1 || stored.Revision != before.Revision+2 {
		t.Fatalf("技能槽重放、收益或经济流水异常：操作%d，已存槽%v，回执%v，错误%v", count, stored.SkillSlotIDs, replayed, err)
	}
}
