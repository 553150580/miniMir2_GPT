package game

import (
	"bytes"
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func potionWorker() (*worker, *room, *capturedConnection) {
	r, conn := observedRoom()
	r.Player.HP, r.Player.MP, r.Player.MaxMP = 10, 5, 20
	r.Role.Level = 7
	r.Role.EconomicSeq, r.Role.Revision = 11, 17
	r.Potions.LastAppliedSeq = 11
	f := devconfig.Fixture{Potions: devconfig.PotionRules{TickMS: 7, BasePerTick: 3, LevelsPerStep: 4, UseDelayMS: 11, QueueCap: 23}}
	f.Map.ID, f.Map.Width, f.Map.Height = 1, 17, 17
	w := &worker{e: &Engine{Store: &persistence.Store{Cfg: devconfig.Config{Fixture: f}}}, rooms: map[string]*room{"role": r}}
	return w, r, conn
}

func potionReceipt(r *room, seq, hp, mp int64) persistence.Operation {
	return persistence.Operation{Kind: "use_item", CommandSeq: seq, Potion: &persistence.PotionReceipt{Scene: strconv.FormatUint(r.Scene, 10), ItemID: 35, HP: hp, MP: mp}}
}

func TestPotionReceiptAppliesOriginalSequenceOnceInOriginalScene(t *testing.T) {
	w, r, _ := potionWorker()
	r.Scene = math.MaxUint64
	now := time.Unix(1800000000, 0)
	receipt := potionReceipt(r, 12, 8, 10)
	if !w.applyPotionReceipt(r, receipt, now) || r.Player.HP != 10 || r.Player.MP != 5 {
		t.Fatal("提交药品没有进入原场景队列，或错误立即加满")
	}
	before := r.Potions
	r.Role.EconomicSeq = 50 // 当前水位不能冒充首次回执的原命令序号。
	if w.applyPotionReceipt(r, receipt, now.Add(time.Hour)) || !reflect.DeepEqual(before, r.Potions) {
		t.Fatal("回执重传再次追加药效或延长节流")
	}
	other := receipt
	copy := *receipt.Potion
	other.Potion = &copy
	other.Potion.Scene = "21"
	other.CommandSeq++
	if w.applyPotionReceipt(r, other, now) || !reflect.DeepEqual(before, r.Potions) {
		t.Fatal("旧场景回执向新场景追加药效")
	}
	r.Scene++ // 回绕为0也不能匹配原来的最大uint64场景字符串。
	if w.applyPotionReceipt(r, receipt, now) {
		t.Fatal("跨场景回执被重新应用")
	}
}

func TestPotionTicksIndependentPoolsCapAndNoCatchup(t *testing.T) {
	w, r, _ := potionWorker()
	now := time.Unix(1800000000, 0)
	w.tickPotions(r, now) // 空队列同样建立并维持周期节拍。
	w.applyPotionReceipt(r, potionReceipt(r, 12, 20, 10), now)
	if w.tickPotions(r, now.Add(7*time.Millisecond)) {
		t.Fatal("恢复周期未到却提前回血")
	}
	if !w.tickPotions(r, now.Add(8*time.Millisecond)) || r.Player.HP != 14 || r.Player.MP != 9 || r.Potions.HP != 16 || r.Potions.MP != 6 {
		t.Fatal("独立HP/MP恢复队列未使用配置周期与等级步长")
	}
	r.Player.HP = r.Player.MaxHP
	w.tickPotions(r, now.Add(9*time.Millisecond))
	if r.Potions.HP != 0 || r.Potions.MP != 6 || r.Player.MP != 9 {
		t.Fatal("满值清队列错误等待了下一跳，或影响另一条未满队列")
	}
	w.tickPotions(r, now.Add(time.Hour))
	if r.Player.HP != r.Player.MaxHP || r.Potions.HP != 0 || r.Player.MP != 13 || r.Potions.MP != 2 {
		t.Fatal("满HP未清独立队列，或时间跳变补算了多次恢复")
	}
	w.tickPotions(r, now.Add(time.Hour+8*time.Millisecond))
	if r.Player.MP != 15 || r.Potions.MP != 0 || r.Potions.NextTick.IsZero() {
		t.Fatal("末跳没有消耗剩余量或空队列停止了周期节拍")
	}
	next := r.Potions.NextTick
	w.applyPotionReceipt(r, potionReceipt(r, 13, math.MaxInt64, math.MaxInt64), now)
	if r.Potions.HP != 23 || r.Potions.MP != 23 || queueAdd(math.MaxInt64-2, 9, math.MaxInt64) != math.MaxInt64 || !r.Potions.NextTick.Equal(next) {
		t.Fatal("恢复队列未有界饱和或发生整数溢出")
	}
}

func TestPotionDeathAndPendingPauseWithoutReset(t *testing.T) {
	w, r, _ := potionWorker()
	now := time.Unix(1800000000, 0)
	w.applyPotionReceipt(r, potionReceipt(r, 12, 8, 10), now)
	r.Player.HP = 0
	before := r.Potions
	if w.tickPotions(r, now.Add(time.Hour)) || !reflect.DeepEqual(before, r.Potions) {
		t.Fatal("死亡期间恢复或丢弃了药效队列")
	}
	r.Player.HP = 10
	r.Pending = true
	w.step(r, now.Add(time.Hour))
	if !reflect.DeepEqual(before, r.Potions) || r.Player.HP != 10 {
		t.Fatal("提交未知期间推进了药效")
	}
	r.Pending = false
	if !w.tickPotions(r, now.Add(time.Hour)) || r.Player.HP != 14 || r.Player.MP != 9 {
		t.Fatal("恢复活动后没有只处理单跳原药效")
	}
}

func TestPotionGateSharedCooldownAndDeadSceneChecks(t *testing.T) {
	_, r, _ := potionWorker()
	now := time.Unix(1800000000, 0)
	r.Potions.NextUse = now.Add(11 * time.Millisecond)
	if potionGate(r, r.Scene, now.Add(10*time.Millisecond)) != p.CodeItemUseCooldown || potionGate(r, r.Scene, now.Add(11*time.Millisecond)) != 0 {
		t.Fatal("共享节流边界没有严格按配置计时")
	}
	r.Player.HP = 0
	if potionGate(r, r.Scene, now.Add(time.Hour)) != p.CodeItemUseBlocked || potionGate(r, r.Scene+1, now) != p.CodeStaleSession {
		t.Fatal("死亡或失效场景通过了用药验证")
	}
}

func TestPaidPotionOfflineClockDoesNotRunCombatOrChargeHosting(t *testing.T) {
	w, r, conn := potionWorker()
	now := time.Unix(1800000000, 0)
	r.Conn = nil
	r.Auto = false
	r.Role.HostingSeconds = 0
	beforeRole := r.Role
	beforeMonster := *r.Monsters[0]
	w.applyPotionReceipt(r, potionReceipt(r, 12, 8, 10), now)
	w.step(r, now)
	if r.Player.HP != 14 || r.Player.MP != 9 || !reflect.DeepEqual(beforeRole, r.Role) || !reflect.DeepEqual(beforeMonster, *r.Monsters[0]) || r.HostMS != 0 {
		t.Fatal("健康离线药效未推进，或越过无托管门槛运行了战斗/收益/扣时")
	}
	clock, delay := r.Potions.NextTick, r.Potions.NextUse
	r.Conn = conn
	r.Ready = false
	r.Monsters[0].HP = 0 // ACK仅门禁观察，恢复测试不混入仍在推进的普通战斗。
	w.step(r, now.Add(4*time.Millisecond))
	if r.Player.HP != 14 || !r.Potions.NextTick.Equal(clock) || !r.Potions.NextUse.Equal(delay) {
		t.Fatal("重新绑定等待ACK时重置或额外推进了药效时钟")
	}
	r.Ready = true
	r.Monsters[0].HP = 0 // 后续仅观察恢复；不让普通战斗伤害掩盖重复回血。
	r.Monsters[0].Monster = &devconfig.Monster{}
	w.step(r, now.Add(4*time.Millisecond))
	if r.Player.HP != 14 || potionGate(r, r.Scene, now.Add(4*time.Millisecond)) != p.CodeItemUseCooldown {
		t.Fatal("健康恢复观察导致额外回血或提前清除共享节流")
	}
	w.step(r, now.Add(8*time.Millisecond))
	if r.Player.HP != 18 || r.Player.MP != 13 || r.Potions.LastAppliedSeq != 12 {
		t.Fatal("重连后未按原队列与节拍只恢复一跳")
	}
}

func TestFreshRoomStartsPotionWatermarkAtPermanentEconomicSequence(t *testing.T) {
	w, _, _ := potionWorker()
	f := &w.e.Store.Cfg.Fixture
	f.Players = []devconfig.Fighter{{JobID: 0, HP: 20, MP: 10}}
	f.Pet.JobID = 2
	f.Monsters = []devconfig.Monster{{ID: 1, Fighter: devconfig.Fighter{HP: 10}}}
	f.Map.MonsterSpawns = make([]devconfig.Point, 8)
	r := &room{worldState: &worldState{}}
	w.initialize(r, domain.Role{Job: 0, EconomicSeq: 50})
	if r.Potions.LastAppliedSeq != 50 || r.Potions.HP != 0 || r.Potions.MP != 0 {
		t.Fatal("新世界没有从永久经济水位初始化药效屏障")
	}
	if w.applyPotionReceipt(r, potionReceipt(r, 12, 7, 0), time.Now()) {
		t.Fatal("新世界向旧经济序号补发了药效")
	}
}

func queuedPotion(r *room, conn *capturedConnection, request uint32, id []byte) command {
	q := &pb.UseItemRequest{ItemInstanceId: domain.Bytes(domain.ID()), ItemId: 35, SceneEpoch: r.Scene, Operation: &pb.OperationContext{OpId: id, CommandSeq: 12}}
	return command{role: "role", conn: conn, frame: wire.Message(p.MsgC2SUseItem, request, 3, q)}
}

func TestPotionBusyQueueBoundedAndImmutableWithoutFalsePending(t *testing.T) {
	w, r, conn := potionWorker()
	r.Pending = true
	one := queuedPotion(r, conn, 1, domain.Bytes(domain.ID()))
	two := queuedPotion(r, conn, 2, domain.Bytes(domain.ID()))
	w.command(one)
	w.command(two)
	if len(r.Waiting) != 2 || len(conn.frames) != 0 {
		t.Fatal("未保存两笔原命令或提前发送假Pending")
	}
	expected := append([]byte(nil), one.frame.Payload...)
	one.frame.Payload[0] ^= 1
	if !bytes.Equal(r.Waiting[0].frame.Payload, expected) {
		t.Fatal("排队命令与外部可变Payload共享内存")
	}
	w.command(queuedPotion(r, conn, 3, domain.Bytes(domain.ID())))
	reply := &pb.OperationResult{}
	if err := proto.Unmarshal(conn.frames[0].Payload, reply); err != nil {
		t.Fatal(err)
	}
	if len(r.Waiting) != 2 || reply.ErrorCode != p.CodeServerBusy || reply.Pending || reply.CommandSeq != 11 || reply.NewRevision != 17 {
		t.Fatal("满队列新请求没有明确返回未受理与当前水位")
	}
	replay := r.Waiting[0]
	replay.frame.Request = 4
	w.command(replay)
	if err := proto.Unmarshal(conn.frames[1].Payload, reply); err != nil {
		t.Fatal(err)
	}
	if !reply.Pending || reply.ErrorCode != p.CodeOperationPending {
		t.Fatal("已受理重传被误报为未受理")
	}
	changed := &pb.UseItemRequest{}
	if err := proto.Unmarshal(replay.frame.Payload, changed); err != nil {
		t.Fatal(err)
	}
	changed.ItemId++
	w.command(command{role: "role", conn: conn, frame: wire.Message(p.MsgC2SUseItem, 5, 3, changed)})
	if err := proto.Unmarshal(conn.frames[2].Payload, reply); err != nil || reply.ErrorCode != p.CodeOperationConflict || reply.Pending {
		t.Fatal("在途原op改变指纹没有拒绝")
	}
}

func TestPotionWaitingDrainRejectsStaleBindingsWithoutBlockingOthers(t *testing.T) {
	w, r, conn := potionWorker()
	r.Pending = true
	one := queuedPotion(r, conn, 1, domain.Bytes(domain.ID()))
	two := queuedPotion(r, conn, 2, domain.Bytes(domain.ID()))
	w.command(one)
	w.command(two)
	r.Pending = false
	r.Auth.SessionEpoch++
	w.after(r)
	if len(r.Waiting) != 0 || len(conn.frames) != 2 {
		t.Fatal("失效排队命令阻塞了后续命令或未分别返回")
	}
	for _, frame := range conn.frames {
		result := &pb.OperationResult{}
		if err := proto.Unmarshal(frame.Payload, result); err != nil || result.ErrorCode != p.CodeStaleSession {
			t.Fatal("旧绑定排队药品没有拒绝")
		}
	}
}

func TestPotionFullQueueKeepsHistoricalReceiptReconciliationPending(t *testing.T) {
	for _, sequence := range []uint64{1, 10, 11, 0, 12} {
		t.Run(strconv.FormatUint(sequence, 10), func(t *testing.T) {
			w, r, conn := potionWorker()
			r.Pending = true
			// 当前在途及两笔等待均是其它操作，不含客户端丢失回执的旧op。
			r.PendingOperationID = domain.ID()
			w.command(queuedPotion(r, conn, 1, domain.Bytes(domain.ID())))
			w.command(queuedPotion(r, conn, 2, domain.Bytes(domain.ID())))
			old := queuedPotion(r, conn, 3, domain.Bytes(domain.ID()))
			request := &pb.UseItemRequest{}
			if err := proto.Unmarshal(old.frame.Payload, request); err != nil {
				t.Fatal(err)
			}
			request.Operation.CommandSeq = sequence
			old.frame = wire.Message(p.MsgC2SUseItem, 3, 3, request)
			w.command(old)
			if len(r.Waiting) != 2 || len(conn.frames) != 1 {
				t.Fatal("满队列核对请求改变了等待预算，或没有明确回复")
			}
			reply := &pb.OperationResult{}
			if err := proto.Unmarshal(conn.frames[0].Payload, reply); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(reply.OpId, request.Operation.OpId) || reply.CommandSeq != 11 || reply.NewRevision != 17 {
				t.Fatal("满队列没有保留原操作身份和当前经济水位")
			}
			historical := sequence > 0 && sequence <= 11
			if historical && (!reply.Pending || reply.ErrorCode != p.CodeOperationPending) {
				t.Fatal("未查历史回执就把可能已提交的旧op误报为未受理")
			}
			if !historical && (reply.Pending || reply.ErrorCode != p.CodeServerBusy) {
				t.Fatal("不在历史或在途集合的新请求没有明确保持未受理语义")
			}
		})
	}
}

func TestManualAttackCannotTargetInactiveMonsterSlots(t *testing.T) {
	w, r, conn := potionWorker()
	w.e.Store.Cfg.Fixture.Players = []devconfig.Fighter{{JobID: 0, AttackMin: 3, AttackMax: 3, AttackMS: 100, MoveMS: 100}}
	definition := &devconfig.Monster{}
	r.Monsters[0].Monster = definition
	hidden := &entity{Handle: 11, Kind: 3, HP: 40, MaxHP: 40, Pos: devconfig.Point{X: 8, Y: 9}, Monster: definition}
	r.Monsters = append(r.Monsters, hidden)
	input := uint32(0)
	send := func(target uint32) {
		input++
		w.command(command{role: "role", conn: conn, frame: wire.Message(p.MsgC2SCastIntent, 1, 3, &pb.CastIntent{SceneEpoch: r.Scene, TargetHandle: target, InputSequence: input})})
	}
	send(hidden.Handle)
	if hidden.HP != 40 || r.Player.AttackSequence != 0 {
		t.Fatal("手动攻击命中了未激活怪物槽")
	}
	send(r.Monsters[0].Handle)
	if r.Monsters[0].HP != 97 || r.Player.AttackSequence != 1 {
		t.Fatal("激活怪物的合法普攻被一并禁用")
	}
}
