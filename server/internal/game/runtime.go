package game

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"sync"
	"sync/atomic"
	"time"
)

type Connection interface {
	Alive() bool
	Deliver(wire.Frame) bool
	Close(string)
	SetRole(string)
	RoleID() string
	SetBinding(uint64)
	Binding() uint64
}
type command struct {
	role       string
	conn       Connection
	frame      wire.Frame
	ticket     string
	done       func(*worker)
	tick       time.Time
	disconnect bool
}
type job struct {
	run    func() func(*worker)
	target *worker
}
type Engine struct {
	Store    *persistence.Store
	workers  []*worker
	jobs     chan job
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	baseline atomic.Uint64
	stopped  atomic.Bool
}
type worker struct {
	e        *Engine
	commands chan command
	control  chan command
	done     chan func(*worker)
	rooms    map[string]*room
}
type entity struct {
	Handle         uint32
	Kind           uint32
	Def            uint32
	Pos            devconfig.Point
	Direction      uint32
	HP             int64
	MP             int64
	MaxMP          int64
	MaxHP          int64
	Action         uint32
	AttackSequence uint32
	ActionUntil    time.Time
	NextMove       time.Time
	NextAttack     time.Time
	RespawnAt      time.Time
	Monster        *devconfig.Monster
}
type room struct {
	Role               domain.Role
	Auth               persistence.Auth
	Conn               Connection
	Ready              bool
	Scene              uint64
	Baseline           uint64
	Sequence           uint64
	Player             entity
	Pet                *entity
	Monsters           []*entity
	MonsterCount       int
	Auto               bool
	Manual             bool
	ManualDirection    uint32
	Destination        *devconfig.Point
	Pending            bool
	PendingRetry       *job
	RetryAt            time.Time
	Waiting            []command
	LastTick           time.Time
	HostMS             int64
	HostSession        string
	DeathID            string
	AwaitUntil         time.Time
	LastBroadcast      time.Time
	Loot               []*entity
	LootUntil          time.Time
	Potions            potionState
	PendingOperationID string
	PendingFingerprint string
}

func New(ctx context.Context, s *persistence.Store) *Engine {
	ctx, cancel := context.WithCancel(ctx)
	e := &Engine{Store: s, ctx: ctx, cancel: cancel, jobs: make(chan job, 256)}
	for i := 0; i < 8; i++ {
		w := &worker{e: e, commands: make(chan command, 256), control: make(chan command, 64), done: make(chan func(*worker), 256), rooms: map[string]*room{}}
		e.workers = append(e.workers, w)
		e.wg.Go(w.loop)
	}
	for i := 0; i < 8; i++ {
		e.wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case j := <-e.jobs:
					callback := j.run()
					select {
					case j.target.done <- callback:
					case <-ctx.Done():
						return
					}
				}
			}
		})
	}
	e.wg.Go(func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				for _, w := range e.workers {
					select {
					case w.commands <- command{tick: now}:
					default:
					}
				}
			}
		}
	})
	return e
}
func (e *Engine) worker(id string) *worker {
	raw := domain.Bytes(id)
	h := sha256.Sum256(raw)
	return e.workers[binary.BigEndian.Uint64(h[:8])%8]
}
func (e *Engine) Enter(token string, c Connection, request uint32) {
	if e.stopped.Load() {
		c.Close("maintenance")
		return
	}
	w := e.workers[0]
	j := job{target: w, run: func() func(*worker) {
		t, err := e.Store.TicketRole(e.ctx, token)
		return func(_ *worker) {
			if !c.Alive() {
				return
			}
			if err != nil {
				c.Deliver(wire.Frame{ID: p.MsgS2CEnterLine, Request: request, Error: persistence.Code(err)})
				c.Close("invalid ticket")
				return
			}
			c.SetRole(t.RoleID)
			e.post(e.worker(t.RoleID), command{role: t.RoleID, conn: c, ticket: token, frame: wire.Frame{Request: request}})
		}
	}}
	select {
	case e.jobs <- j:
	default:
		c.Close("server busy")
	}
}
func (e *Engine) post(w *worker, c command) bool {
	select {
	case w.commands <- c:
		return true
	default:
		if c.conn != nil {
			c.conn.Close("server busy")
		}
		return false
	}
}
func (e *Engine) Handle(c Connection, f wire.Frame) {
	id := c.RoleID()
	if id == "" {
		c.Deliver(wire.Frame{ID: f.ID, Request: f.Request, Error: p.CodeUnauthenticated})
		return
	}
	e.post(e.worker(id), command{role: id, conn: c, frame: f})
}
func (e *Engine) Disconnect(c Connection) {
	if id := c.RoleID(); id != "" {
		select {
		case e.worker(id).control <- command{role: id, conn: c, disconnect: true}:
		case <-e.ctx.Done():
		}
	}
}
func (e *Engine) Stop() {
	e.stopped.Store(true)
	for _, w := range e.workers {
		e.post(w, command{done: func(w *worker) {
			for _, r := range w.rooms {
				if r.Conn != nil {
					r.Conn.Close("maintenance")
					r.Conn = nil
				}
				r.Auto = false
			}
		}})
	}
	deadline := time.NewTimer(500 * time.Millisecond)
	<-deadline.C
	e.cancel()
	e.wg.Wait()
	// 父context先取消时worker已退出，仍必须关闭已劫持的WS；此时遍历由Stop独占。
	for _, w := range e.workers {
		for _, r := range w.rooms {
			if r.Conn != nil {
				r.Conn.Close("maintenance")
				r.Conn = nil
			}
		}
	}
}
func (w *worker) loop() {
	for {
		select {
		case <-w.e.ctx.Done():
			return
		case done := <-w.done:
			done(w)
		case c := <-w.control:
			w.command(c)
		case c := <-w.commands:
			if c.done != nil {
				c.done(w)
				continue
			}
			if !c.tick.IsZero() {
				for _, r := range w.rooms {
					w.step(r, c.tick)
				}
				continue
			}
			w.command(c)
		}
	}
}
func (w *worker) submit(j job) bool {
	select {
	case w.e.jobs <- j:
		return true
	default:
		return false
	}
}
func (w *worker) command(c command) {
	r := w.rooms[c.role]
	if c.ticket != "" {
		if !c.conn.Alive() {
			return
		}
		if r != nil && r.Pending {
			if len(r.Waiting) < 2 {
				r.Waiting = append(r.Waiting, c)
			} else {
				c.conn.Close("role busy")
			}
			return
		}
		if r == nil {
			if len(w.rooms) >= 256 {
				c.conn.Close("development room budget")
				return
			}
			r = &room{Role: domain.Role{ID: c.role}}
			w.rooms[c.role] = r
		}
		w.join(r, c)
		return
	}
	if r == nil {
		return
	}
	if c.disconnect {
		if r.Conn == c.conn {
			r.Conn = nil
			r.Ready = false
			r.Manual = false
			r.LastTick = time.Now()
			r.HostSession = domain.ID()
			r.HostMS = 0
		}
		return
	}
	if r.Conn != c.conn || c.frame.Binding != uint64(r.Auth.SessionEpoch) {
		if economic(c.frame.ID) {
			c.conn.Deliver(wire.Message(p.MsgS2COperationResult, c.frame.Request, c.conn.Binding(), &pb.OperationResult{OpId: operationID(c.frame), ErrorCode: p.CodeStaleSession, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)}))
		} else {
			c.conn.Deliver(wire.Frame{ID: c.frame.ID, Request: c.frame.Request, Binding: c.conn.Binding(), Error: p.CodeStaleSession})
		}
		return
	}
	if c.frame.ID == p.MsgC2SEnterWorldAck {
		if r.Ready {
			return
		}
		var q pb.EnterWorldAck
		if proto.Unmarshal(c.frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.BaselineId != r.Baseline {
			return
		}
		r.Ready = true
		w.snapshot(r)
		w.assets(r)
		w.send(r, p.MsgS2CHostingReport, 0, r.Role.HostingReport())
		if r.Player.HP == 0 {
			w.deathNotice(r)
		}
		w.send(r, p.MsgS2CSyncReady, c.frame.Request, &pb.SyncReady{SceneEpoch: r.Scene, BaselineId: r.Baseline, NextEconomicCommandSeq: uint64(r.Role.EconomicSeq + 1)})
		return
	}
	if !r.Ready {
		w.failure(r, c.frame, p.CodeLineRecovering)
		return
	}
	if c.frame.ID == p.MsgC2SUseItem && r.Pending {
		w.queuePotion(r, c)
		return
	}
	switch c.frame.ID {
	case p.MsgC2SMoveIntent:
		var q pb.MoveIntent
		if proto.Unmarshal(c.frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.Direction > 7 {
			w.failure(r, c.frame, p.CodeInvalidArgument)
			return
		}
		if q.ReleaseJoystick {
			r.Manual = false
			return
		}
		if q.Destination != nil {
			point := devconfig.Point{X: int(q.Destination.X), Y: int(q.Destination.Y)}
			if !w.walkable(point) {
				w.failure(r, c.frame, p.CodeInvalidArgument)
				return
			}
			r.Destination = &point
			r.Manual = false
		} else {
			r.Manual = true
			r.ManualDirection = q.Direction
			r.Destination = nil
		}
	case p.MsgC2SAutoBattle:
		var q pb.AutoBattleIntent
		if proto.Unmarshal(c.frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.MonsterCount > 8 {
			w.failure(r, c.frame, p.CodeInvalidArgument)
			return
		}
		if q.Enabled && r.Role.PickupStopped {
			w.failure(r, c.frame, p.CodeLineForbidden)
			w.hosting(r, 0)
			return
		}
		if q.MonsterCount > 0 {
			r.MonsterCount = int(q.MonsterCount)
		}
		r.Auto = q.Enabled
		w.hosting(r, c.frame.Request)
		w.snapshot(r)
	case p.MsgC2SCastIntent:
		var q pb.CastIntent
		if proto.Unmarshal(c.frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.SkillId != 0 {
			w.failure(r, c.frame, p.CodeInvalidArgument)
			return
		}
		for _, m := range r.Monsters[:r.MonsterCount] {
			if m.Handle == q.TargetHandle && m.HP > 0 {
				w.attack(r, &r.Player, m, time.Now())
				break
			}
		}
	case p.MsgC2SEquip, p.MsgC2SRecycle, p.MsgC2SClaimHostingTime, p.MsgC2SBuyHostingTime, p.MsgC2SReviveChoice, p.MsgC2SUseItem:
		w.asset(r, c.frame)
	default:
		w.failure(r, c.frame, p.CodeInvalidArgument)
	}
}
func (w *worker) join(r *room, c command) {
	r.Pending = true
	fresh := r.Scene == 0
	j := job{target: w, run: func() func(*worker) {
		role, auth, err := w.e.Store.Enter(w.e.ctx, c.ticket)
		return func(w *worker) {
			r.Pending = false
			if err != nil {
				c.conn.Deliver(wire.Frame{ID: p.MsgS2CEnterLine, Request: c.frame.Request, Error: persistence.Code(err)})
				c.conn.Close("entry rejected")
				if fresh {
					delete(w.rooms, c.role)
					for _, next := range r.Waiting {
						w.command(next)
					}
				} else {
					w.after(r)
				}
				return
			}
			if fresh {
				w.initialize(r, role)
			}
			if r.Conn != nil && r.Conn != c.conn {
				r.Conn.Close("role taken over")
			}
			r.Role = role
			r.Auth = auth
			r.Conn = c.conn
			r.Ready = false
			r.Baseline = w.e.baseline.Add(1)
			r.HostMS = 0
			r.HostSession = ""
			r.LastTick = time.Now()
			r.AwaitUntil = time.Now().Add(10 * time.Second)
			if !c.conn.Alive() {
				r.Conn = nil
				r.HostSession = domain.ID()
				w.after(r)
				return
			}
			c.conn.SetBinding(uint64(auth.SessionEpoch))
			w.send(r, p.MsgS2CEnterLine, c.frame.Request, &pb.EnterLineResult{SceneEpoch: r.Scene, BaselineId: r.Baseline, RoomId: w.e.Store.Cfg.Fixture.Map.ID, LineId: 1, ResourceVersion: w.e.Store.Cfg.Fixture.RuleVersion, Position: pointPB(r.Player.Pos), SceneKind: 1, NextEconomicCommandSeq: uint64(role.EconomicSeq + 1)})
			w.baseline(r)
			w.after(r)
		}
	}}
	if !w.submit(j) {
		r.Pending = false
		c.conn.Close("server busy")
		if fresh {
			delete(w.rooms, c.role)
		}
	}
}
func (w *worker) initialize(r *room, role domain.Role) {
	f := &w.e.Store.Cfg.Fixture
	player, _ := f.Player(role.Job)
	r.Scene = binary.BigEndian.Uint64(domain.Bytes(domain.ID())[:8])
	r.Role = role
	r.Potions.LastAppliedSeq = role.EconomicSeq
	r.Auto = true
	r.MonsterCount = 1
	r.Player = entity{Handle: 1, Kind: 1, Def: role.Job, Pos: f.Map.PlayerSpawn, HP: player.HP, MaxHP: player.HP, MP: player.MP, MaxMP: player.MP}
	if role.Job == f.Pet.JobID {
		r.Pet = &entity{Handle: 2, Kind: 2, Def: f.Pet.DefinitionID, Pos: f.Map.PetSpawn, HP: f.Pet.HP, MaxHP: f.Pet.HP, MP: f.Pet.MP, MaxMP: f.Pet.MP}
	}
	for i := 0; i < 8; i++ {
		def := f.Monsters[i%len(f.Monsters)]
		hp := def.HP
		r.Monsters = append(r.Monsters, &entity{Handle: uint32(10 + i), Kind: 3, Def: def.ID, Pos: f.Map.MonsterSpawns[i], HP: hp, MaxHP: def.HP, Monster: &def})
	}
}
func (w *worker) after(r *room) {
	for len(r.Waiting) > 0 && !r.Pending {
		c := r.Waiting[0]
		r.Waiting = r.Waiting[1:]
		w.command(c)
	}
}
func (w *worker) send(r *room, id uint16, request uint32, msg proto.Message) {
	if r.Conn != nil {
		r.Conn.Deliver(wire.Message(id, request, uint64(r.Auth.SessionEpoch), msg))
	}
}
func (w *worker) failure(r *room, f wire.Frame, code uint32) {
	if economic(f.ID) {
		w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: code, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
		return
	}
	r.Conn.Deliver(wire.Frame{ID: f.ID, Request: f.Request, Binding: uint64(r.Auth.SessionEpoch), Error: code})
}
func (w *worker) assets(r *room) {
	if r.Conn == nil || !r.Ready {
		return
	}
	w.send(r, p.MsgS2CInventory, 0, r.Role.Inventory())
	w.send(r, p.MsgS2CWallet, 0, r.Role.Wallet.Proto(r.Role.Revision))
	w.hosting(r, 0)
}
func (w *worker) hosting(r *room, request uint32) {
	if r.Conn == nil || !r.Ready {
		return
	}
	w.send(r, p.MsgS2CHostingState, request, &pb.HostingState{RemainHostingSec: uint32(r.Role.HostingSeconds), AutoBattleEnabled: r.Auto, StateRevision: uint64(r.Role.Revision)})
}
func (w *worker) snapshot(r *room) {
	if !r.Ready {
		return
	}
	w.baseline(r)
}

// 入场基线在ACK之前发送；周期同步仍经snapshot的Ready门槛。
func (w *worker) baseline(r *room) {
	if r.Conn == nil {
		return
	}
	r.Sequence++
	out := &pb.SceneBaselineChunk{SceneEpoch: r.Scene, BaselineId: r.Baseline, ChunkCount: 1, StateSequence: r.Sequence}
	entities := []*entity{&r.Player}
	if r.Pet != nil {
		entities = append(entities, r.Pet)
	}
	entities = append(entities, r.Monsters[:r.MonsterCount]...)
	entities = append(entities, r.Loot...)
	for _, e := range entities {
		moveMS := uint32(0)
		if e.Kind >= 1 && e.Kind <= 3 {
			moveMS = uint32(w.fighter(r, e).MoveMS)
		}
		out.Entities = append(out.Entities, &pb.EntityState{EntityHandle: e.Handle, EntityKind: e.Kind, DefinitionId: e.Def, Position: pointPB(e.Pos), Direction: e.Direction, Hp: e.HP, HpMax: e.MaxHP, Mp: e.MP, MpMax: e.MaxMP, Action: e.Action, MoveIntervalMs: moveMS, AttackSequence: e.AttackSequence})
	}
	w.send(r, p.MsgS2CSceneBaseline, 0, out)
}
func pointPB(p devconfig.Point) *pb.GridPoint { return &pb.GridPoint{X: int32(p.X), Y: int32(p.Y)} }
func (w *worker) asset(r *room, f wire.Frame) {
	reviveID := ""
	q := persistence.AssetRequest{Fingerprint: persistence.Hash(append([]byte{byte(f.ID >> 8), byte(f.ID)}, f.Payload...))}
	switch f.ID {
	case p.MsgC2SEquip:
		var x pb.EquipRequest
		if proto.Unmarshal(f.Payload, &x) != nil {
			w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
			return
		}
		q.Kind = "equip"
		q.Context = x.Operation
		q.ItemID = domain.Hex(x.ItemInstanceId)
		q.Slot = x.Slot
	case p.MsgC2SRecycle:
		var x pb.RecycleRequest
		if proto.Unmarshal(f.Payload, &x) != nil {
			w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
			return
		}
		q.Kind = "recycle"
		q.Context = x.Operation
		for _, id := range x.ItemInstanceIds {
			q.ItemIDs = append(q.ItemIDs, domain.Hex(id))
		}
	case p.MsgC2SClaimHostingTime:
		var x pb.ClaimHostingTimeRequest
		if proto.Unmarshal(f.Payload, &x) != nil {
			w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
			return
		}
		q.Kind = "claim_hosting"
		q.Context = x.Operation
		q.Benefit = x.BenefitKind
	case p.MsgC2SBuyHostingTime:
		w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
		return
	case p.MsgC2SReviveChoice:
		var x pb.ReviveChoice
		if proto.Unmarshal(f.Payload, &x) != nil {
			w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
			return
		}
		reviveID = domain.Hex(x.DeathId)
		q.Kind = "revive"
		q.Context = x.Operation
		q.ValidRevive = w.e.Store.Cfg.Fixture.AllowFreeDevRevive && x.Action == 1 && r.Player.HP == 0 && reviveID == r.DeathID
	case p.MsgC2SUseItem:
		var x pb.UseItemRequest
		if proto.Unmarshal(f.Payload, &x) != nil || len(x.ItemInstanceId) != 16 {
			w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
			return
		}
		q.Kind = "use_item"
		q.Context = x.Operation
		q.ItemID = domain.Hex(x.ItemInstanceId)
		q.ItemDefinitionID = x.ItemId
		q.SceneEpoch = x.SceneEpoch
		q.PotionGateCode = potionGate(r, x.SceneEpoch, time.Now())
	}
	if q.Context == nil || len(q.Context.OpId) != 16 {
		w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
		return
	}
	if r.Pending {
		op := &pb.OperationResult{ErrorCode: p.CodeOperationPending, Pending: true, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)}
		if q.Context != nil {
			op.OpId = q.Context.OpId
		}
		w.send(r, p.MsgS2COperationResult, f.Request, op)
		return
	}
	r.Pending = true
	r.PendingOperationID = domain.Hex(q.Context.OpId)
	r.PendingFingerprint = q.Fingerprint
	auth := r.Auth
	var j job
	j = job{target: w, run: func() func(*worker) {
		role, result, receipt, err := w.e.Store.AssetWithReceipt(w.e.ctx, auth, q)
		return func(w *worker) {
			if err != nil && persistence.Code(err) == p.CodeOperationPending {
				r.PendingRetry = &j
				r.RetryAt = time.Now().Add(500 * time.Millisecond)
				result.Pending = true
				w.send(r, p.MsgS2COperationResult, f.Request, result)
				return
			}
			r.Pending = false
			r.PendingOperationID = ""
			r.PendingFingerprint = ""
			if err == nil && role.Revision >= r.Role.Revision {
				r.Role = role
			}
			if err != nil {
				result.ErrorCode = persistence.Code(err)
			}
			if err == nil && result.ErrorCode == 0 && q.Kind == "revive" && reviveID == r.DeathID && r.Player.HP <= 0 {
				r.Player.HP = r.Player.MaxHP
				r.Player.MP = r.Player.MaxMP
				r.Player.Pos = w.e.Store.Cfg.Fixture.Map.PlayerSpawn
				r.Player.Action = 0
				r.DeathID = ""
				r.Destination = nil
				r.HostMS = 0
			}
			if err == nil && result.ErrorCode == 0 && q.Kind == "use_item" {
				w.applyPotionReceipt(r, receipt, time.Now())
			}
			w.send(r, p.MsgS2COperationResult, f.Request, result)
			w.assets(r)
			w.snapshot(r)
			w.after(r)
		}
	}}
	if !w.submit(j) {
		r.PendingRetry = &j
		r.RetryAt = time.Now().Add(100 * time.Millisecond)
	}
}

func economic(id uint16) bool {
	switch id {
	case p.MsgC2SEquip, p.MsgC2SRecycle, p.MsgC2SClaimHostingTime, p.MsgC2SBuyHostingTime, p.MsgC2SReviveChoice, p.MsgC2SUseItem:
		return true
	}
	return false
}
func operationID(f wire.Frame) []byte {
	var op *pb.OperationContext
	switch f.ID {
	case p.MsgC2SEquip:
		q := &pb.EquipRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SRecycle:
		q := &pb.RecycleRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SClaimHostingTime:
		q := &pb.ClaimHostingTimeRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SBuyHostingTime:
		q := &pb.BuyHostingTimeRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SReviveChoice:
		q := &pb.ReviveChoice{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SUseItem:
		q := &pb.UseItemRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	}
	if op != nil {
		return op.OpId
	}
	return nil
}
