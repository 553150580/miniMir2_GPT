package game

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/townnav"
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
	role        string
	conn        Connection
	frame       wire.Frame
	ticket      string
	done        func(*worker)
	tick        time.Time
	at          time.Time
	disconnect  bool
	fromWaiting bool
}
type job struct {
	run    func() func(*worker)
	target *worker
}
type Engine struct {
	Store     *persistence.Store
	workers   []*worker
	jobs      chan job
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	baseline  atomic.Uint64
	stopped   atomic.Bool
	TownGrids map[uint64]*townnav.Grid
}
type worker struct {
	e              *Engine
	commands       chan command
	control        chan command
	done           chan func(*worker)
	rooms          map[string]*room
	roll           func(int64) int64
	townNavigation *townNavigationWorker
}
type entity struct {
	Handle           uint32
	Kind             uint32
	Def              uint32
	Pos              devconfig.Point
	Direction        uint32
	HP               int64
	MP               int64
	MaxMP            int64
	MaxHP            int64
	Action           uint32
	AttackSequence   uint32
	AttackSkillID    uint32
	AttackSkillLevel uint32
	ActionUntil      time.Time
	NextMove         time.Time
	NextAttack       time.Time
	ActionReadyAt    time.Time
	RespawnAt        time.Time
	Monster          *devconfig.Monster
	CastSequence     uint32
	CastSkillID      uint32
	Life             uint64
	HealAmount       int64
	NextHeal         time.Time
	Statuses         map[uint32]*combatStatus
	MovementRevision uint64
	MovementKind     uint32
	CellEntry        uint64
	PetRuntime       *petRuntimeState
	IsPetCorpse      bool
}
type room struct {
	*worldState
	PersonalWorld         *worldState
	TownWorld             *worldState
	ExecutionEpoch        uint64
	WorldSequence         uint64
	InflightEvents        int
	EventBatch            []dueEvent
	AdvancingEvents       bool
	FactOrder             uint64
	BindingCandidate      *safeZoneBindingFact
	SettlementGeneration  uint64
	AssetGeneration       uint64
	Role                  domain.Role
	Auth                  persistence.Auth
	Conn                  Connection
	Ready                 bool
	PetBornReady          *petBornAttachment
	Travel                *mapTravelPlan
	TravelHistory         *mapTravelHistory
	SavedPersonal         *personalWorld
	TownNavigation        [2]*townNavigationRequest
	Scene                 uint64
	Baseline              uint64
	Sequence              uint64
	Player                entity
	Pet                   *entity
	PetWorld              worldRef
	PetLifeSequence       uint64
	PetHandleSequence     uint32
	PendingPetBirth       *petBirthReservation
	Manual                bool
	ManualDirection       uint32
	Pending               bool
	PendingRetry          *job
	PendingBinding        *safeZoneBindingFact
	RetryAt               time.Time
	Waiting               []command
	LastTick              time.Time
	HostMS                int64
	HostSession           string
	DeathID               string
	AwaitUntil            time.Time
	LastBroadcast         time.Time
	Potions               potionState
	PendingOperationID    string
	PendingFingerprint    string
	InputSequence         uint32
	NextSpell             time.Time
	SkillCooldowns        map[uint32]time.Time
	CastID                uint64
	EventID               uint64
	ScheduleID            uint64
	KillFacts             []killFact
	PreviewInFlight       bool
	PreviewGeneration     uint64
	Isolated              bool
	MaterialEconomicSeq   int64
	MaterialSourceSeq     int64
	MaterialGeneration    uint64
	Melee                 meleeState
	CellEntrySequence     uint64
	DeferMovementSnapshot bool
}

func New(ctx context.Context, s *persistence.Store) *Engine {
	ctx, cancel := context.WithCancel(ctx)
	e := &Engine{Store: s, ctx: ctx, cancel: cancel, jobs: make(chan job, 256)}
	for i := 0; i < 8; i++ {
		w := &worker{e: e, commands: make(chan command, 256), control: make(chan command, 64), done: make(chan func(*worker), 256), rooms: map[string]*room{}}
		e.workers = append(e.workers, w)
	}
	if err := e.initializeTownNavigation(); err != nil {
		cancel()
		panic("城镇导航准入初始化失败：" + err.Error())
	}
	for _, w := range e.workers {
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
		case e.worker(id).control <- command{role: id, conn: c, disconnect: true, at: time.Now()}:
		case <-e.ctx.Done():
		}
	}
}
func (e *Engine) Stop() {
	e.stopped.Store(true)
	for _, w := range e.workers {
		e.post(w, command{done: func(w *worker) {
			for _, r := range w.rooms {
				w.cancelTownNavigation(r)
				w.cancelMapTravelOnDetach(r)
				if r.Conn != nil {
					r.Conn.Close("maintenance")
					r.Conn = nil
					r.PetBornReady = nil
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
				r.PetBornReady = nil
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
				w.beginTownNavigationTick(c.tick)
				for _, r := range w.rooms {
					w.step(r, c.tick)
				}
				w.endTownNavigationTick()
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
		if r != nil && r.Isolated {
			c.conn.Deliver(wire.Frame{ID: p.MsgS2CEnterLine, Request: c.frame.Request, Error: p.CodeLineRecovering})
			c.conn.Close("room authority isolated")
			return
		}
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
			r = &room{Role: domain.Role{ID: c.role}, worldState: &worldState{}}
			w.rooms[c.role] = r
		}
		w.join(r, c)
		return
	}
	if r == nil {
		return
	}
	if r.Isolated {
		if c.disconnect {
			if r.Conn == c.conn {
				r.Conn = nil
				r.PetBornReady = nil
			}
			return
		}
		if c.conn != nil {
			c.conn.Close("room authority isolated")
		}
		return
	}
	if c.disconnect {
		if r.Conn == c.conn {
			w.cancelMapTravelOnDetach(r)
			w.cancelTownNavigation(r)
			r.PreviewGeneration++
			r.PreviewInFlight = false
			r.Conn = nil
			r.Ready = false
			r.PetBornReady = nil
			r.Manual = false
			r.LastTick = time.Now()
			r.HostSession = domain.ID()
			at := c.at
			if at.IsZero() {
				at = time.Now()
			}
			if m, ok := w.currentMap(r); ok && m.SceneKindValue() == devconfig.SceneKindTown {
				r.HostSession = ""
			}
			transitionEventHosting(r, at, r.HostSession)
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
		if !w.finishMapTravelAck(r, time.Now()) {
			return
		}
		r.Ready = true
		w.expireModes(r, time.Now())
		w.snapshot(r)
		w.modes(r, time.Now())
		w.assets(r)
		w.send(r, p.MsgS2CHostingReport, 0, r.Role.HostingReport())
		if r.Player.HP == 0 {
			w.deathNotice(r)
		}
		w.sendSyncReady(r, c.frame.Request, &pb.SyncReady{SceneEpoch: r.Scene, BaselineId: r.Baseline, NextEconomicCommandSeq: uint64(r.Role.EconomicSeq + 1)})
		return
	}
	if c.frame.ID == p.MsgC2SMapTravelPrepare {
		w.prepareMapTravel(r, c.frame)
		return
	}
	if c.frame.ID == p.MsgC2SMapTravelCommit {
		w.commitMapTravel(r, c.frame)
		return
	}
	if !r.Ready {
		w.failure(r, c.frame, p.CodeLineRecovering)
		return
	}
	if mapTravelQuiescing(r) && !c.fromWaiting {
		w.failure(r, c.frame, p.CodeLineRecovering)
		return
	}
	if (c.frame.ID == p.MsgC2SUseItem || c.frame.ID == p.MsgC2SLearnSkill || c.frame.ID == p.MsgC2SSkillAssetCommit || c.frame.ID == p.MsgC2SCastWithMaterial || c.frame.ID == p.MsgC2SSetCastingSupply) && r.Pending {
		w.queuePotion(r, c)
		return
	}
	switch c.frame.ID {
	case p.MsgC2SSkillModeIntent:
		var q pb.SkillModeIntent
		if proto.Unmarshal(c.frame.Payload, &q) != nil || q.SceneEpoch != r.Scene {
			w.failure(r, c.frame, p.CodeInvalidArgument)
			return
		}
		if !acceptInput(r, q.InputSequence) {
			w.failure(r, c.frame, p.CodeStaleInput)
			w.modes(r, time.Now())
			return
		}
		if code := w.setSkillMode(r, q.SkillId, q.Enabled, time.Now()); code != 0 {
			w.failure(r, c.frame, code)
			w.modes(r, time.Now())
		}
	case p.MsgC2SAttackIntent:
		var q pb.AttackIntent
		if proto.Unmarshal(c.frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.Direction > 7 {
			w.failure(r, c.frame, p.CodeInvalidArgument)
			return
		}
		if !acceptInput(r, q.InputSequence) {
			w.failure(r, c.frame, p.CodeStaleInput)
			return
		}
		if code := w.attackFacing(r, q.Direction, q.SkillId, q.InputSequence, time.Now()); code != 0 {
			w.failure(r, c.frame, code)
		}
	case p.MsgC2SCastWithMaterial:
		w.materialCast(r, c.frame)
	case p.MsgC2SSkillAssetPreview:
		w.skillAssetPreview(r, c.frame)
	case p.MsgC2SMoveIntent:
		var q pb.MoveIntent
		if proto.Unmarshal(c.frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.Direction > 7 || q.CancelNavigation && (q.Destination != nil || q.ReleaseJoystick || q.Direction != 0) {
			w.failure(r, c.frame, p.CodeInvalidArgument)
			return
		}
		if !acceptInput(r, q.InputSequence) {
			w.failure(r, c.frame, p.CodeStaleInput)
			return
		}
		if q.CancelNavigation {
			w.cancelTownNavigation(r)
			// 明确退出才取消地面目标；保留原目标身份供客户端核对，健康断线和摇杆释放不走此分支。
			r.Destination, r.Manual = nil, false
			if r.Navigation.Destination != nil && r.Navigation.Revision != 0 {
				r.Navigation.Status = navigationCancelled
			} else {
				r.Navigation.Status = navigationNone
			}
			w.baseline(r, c.frame.Request)
			return
		}
		if q.ReleaseJoystick {
			r.Manual = false
			return
		}
		if q.Destination != nil {
			point := devconfig.Point{X: int(q.Destination.X), Y: int(q.Destination.Y)}
			if !w.walkable(r, point) {
				w.failure(r, c.frame, p.CodeInvalidArgument)
				return
			}
			if r.Navigation.Revision == math.MaxUint64 {
				w.isolateRoom(r)
				return
			}
			w.cancelTownNavigation(r)
			r.Destination = &point
			r.Navigation = navigationState{InputSequence: q.InputSequence, Destination: &point, Status: navigationActive, Revision: r.Navigation.Revision + 1}
			r.Manual = false
		} else {
			w.cancelTownNavigation(r)
			r.Manual = true
			r.ManualDirection = q.Direction
			r.Destination = nil
			if r.Navigation.InputSequence != 0 {
				r.Navigation.Status = navigationManual
			}
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
		if m, ok := w.currentMap(r); !ok || m.SceneKindValue() == devconfig.SceneKindTown {
			w.failure(r, c.frame, p.CodeLineForbidden)
			w.hosting(r, c.frame.Request)
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
		if proto.Unmarshal(c.frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.Direction > 7 {
			w.failure(r, c.frame, p.CodeInvalidArgument)
			return
		}
		if !acceptInput(r, q.InputSequence) {
			w.failure(r, c.frame, p.CodeStaleInput)
			return
		}
		if q.SkillId != 0 {
			if code := w.castDirected(r, q.SkillId, q.TargetHandle, q.InputSequence, q.Direction, time.Now(), q.TargetGrid); code != 0 {
				w.failure(r, c.frame, code)
			}
			return
		}
		for _, m := range r.Monsters[:r.MonsterCount] {
			if m.Handle == q.TargetHandle && m.HP > 0 {
				w.attack(r, &r.Player, m, time.Now(), q.InputSequence)
				break
			}
		}
	case p.MsgC2SEquip, p.MsgC2SRecycle, p.MsgC2SClaimHostingTime, p.MsgC2SBuyHostingTime, p.MsgC2SReviveChoice, p.MsgC2SUseItem, p.MsgC2SLearnSkill, p.MsgC2SSkillAssetCommit, p.MsgC2SSetCastingSupply, p.MsgC2SSetAutoRecycle, p.MsgC2SSetItemLock:
		w.asset(r, c.frame)
	default:
		w.failure(r, c.frame, p.CodeInvalidArgument)
	}
}
func (w *worker) join(r *room, c command) {
	r.PreviewGeneration++
	r.PreviewInFlight = false
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
			w.cancelMapTravelOnDetach(r)
			w.cancelTownNavigation(r)
			r.Role = role
			r.Auth = auth
			transitionEventPrivilege(r, time.Now(), role.PrivilegeUntil)
			r.InputSequence = 0
			r.Manual = false
			r.Conn = c.conn
			r.Ready = false
			r.PetBornReady = nil
			r.Baseline = w.e.baseline.Add(1)
			r.HostMS = 0
			r.HostSession = ""
			r.LastTick = time.Now()
			r.AwaitUntil = time.Now().Add(10 * time.Second)
			if m, ok := w.currentMap(r); ok && m.SceneKindValue() == devconfig.SceneKindTown && w.e.Store.Cfg.Fixture.TownTravel != nil {
				r.AwaitUntil = time.Now().Add(time.Duration(w.e.Store.Cfg.Fixture.TownTravel.AckTTLMS) * time.Millisecond)
			}
			if !c.conn.Alive() {
				r.Conn = nil
				r.PetBornReady = nil
				r.HostSession = domain.ID()
				transitionEventHosting(r, time.Now(), r.HostSession)
				w.after(r)
				return
			}
			transitionEventHosting(r, time.Now(), "")
			c.conn.SetBinding(uint64(auth.SessionEpoch))
			if r.Player.Handle == 0 {
				w.isolateRoom(r)
				return
			}
			entry := w.enterLineResult(r)
			if entry == nil {
				return
			}
			w.send(r, p.MsgS2CEnterLine, c.frame.Request, entry)
			w.bindingState(r, 1)
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
func (w *worker) enterLineResult(r *room) *pb.EnterLineResult {
	m, ok := w.currentMap(r)
	if !ok {
		w.isolateRoom(r)
		return nil
	}
	return &pb.EnterLineResult{SceneEpoch: r.Scene, BaselineId: r.Baseline, RoomId: m.ID, LineId: 1, ResourceVersion: w.e.Store.Cfg.Fixture.RuleVersion, Position: pointPB(r.Player.Pos), SceneKind: m.SceneKindValue(), NextEconomicCommandSeq: uint64(r.Role.EconomicSeq + 1), PlayerHandle: r.Player.Handle}
}
func (w *worker) initialize(r *room, role domain.Role) {
	w.cancelTownNavigation(r)
	r.PetBornReady = nil
	r.Travel, r.SavedPersonal, r.TravelHistory = nil, nil, nil
	r.worldState = &worldState{}
	r.PersonalWorld, r.TownWorld = nil, nil
	r.ExecutionEpoch, r.WorldSequence, r.InflightEvents = 0, 0, 0
	r.Pet, r.PetWorld, r.PetLifeSequence = nil, worldRef{}, 0
	r.PetHandleSequence, r.PendingPetBirth = petHandle, nil
	f := &w.e.Store.Cfg.Fixture
	r.MapID = f.Map.ID
	player, _ := f.Player(role.Job)
	r.Scene = binary.BigEndian.Uint64(domain.Bytes(domain.ID())[:8])
	r.PersonalWorld = r.currentWorld()
	r.Role = role
	r.Potions.LastAppliedSeq = role.EconomicSeq
	r.Navigation = navigationState{}
	r.Destination = nil
	r.Manual = false
	r.MaterialEconomicSeq, r.MaterialSourceSeq = role.EconomicSeq, role.SourceSeq
	r.Auto = true
	r.MonsterCount = 1
	r.Player = entity{Handle: 1, Kind: 1, Def: role.Job, Pos: f.Map.PlayerSpawn, HP: player.HP, MaxHP: player.HP, MP: player.MP, MaxMP: player.MP}
	w.refreshPlayerStats(r)
	r.Player.HP, r.Player.MP = r.Player.MaxHP, r.Player.MaxMP
	if role.Job == f.Pet.JobID && (!w.petRuntimeEnabled() || f.PetRuntime.BootstrapMode == "legacy_untrained_pet") {
		r.Pet = &entity{Handle: 2, Kind: 2, Def: f.Pet.DefinitionID, Pos: f.Map.PetSpawn, HP: f.Pet.HP, MaxHP: f.Pet.HP, MP: f.Pet.MP, MaxMP: f.Pet.MP}
		r.PetWorld = r.currentWorld().Ref
		if w.petRuntimeEnabled() && !w.attachLegacyPetState(r.Pet) {
			w.isolateRoom(r)
			return
		}
	}
	w.applyPetMapEntryPolicy(r, &f.Map)
	for i := 0; i < 8; i++ {
		def := f.Monsters[i%len(f.Monsters)]
		hp := def.HP
		r.Monsters = append(r.Monsters, &entity{Handle: uint32(10 + i), Kind: 3, Def: def.ID, Pos: f.Map.MonsterSpawns[i], HP: hp, MaxHP: def.HP, Monster: &def})
	}
}
func (w *worker) after(r *room) {
	for len(r.Waiting) > 0 && !r.Pending && !r.Isolated {
		c := r.Waiting[0]
		r.Waiting = r.Waiting[1:]
		c.fromWaiting = true
		w.command(c)
	}
}

func (w *worker) revivePlayer(r *room) {
	m, ok := w.currentMap(r)
	if !ok || !m.Walkable(m.PlayerSpawn) {
		w.isolateRoom(r)
		return
	}
	r.Player.Life++
	r.Player.MovementRevision, r.Player.MovementKind = 0, movementInitial
	for id, status := range r.Player.Statuses {
		if status.RemoveOnDeath {
			delete(r.Player.Statuses, id)
		} else {
			status.TargetLife = r.Player.Life
		}
	}
	// 生命代次切换必须同时推进模式观察版本；同连接客户端不能被旧revision永久挡住。
	r.Melee.Revision++
	r.Player.HP = r.Player.MaxHP
	r.Player.MP = r.Player.MaxMP
	if !w.relocate(r, &r.Player, m.PlayerSpawn) {
		return
	}
	r.Player.Action = 0
	r.DeathID = ""
	// 同图复活仅释放摇杆，原地面导航目标保留并从新脚点继续规划。
	r.Manual = false
	r.HostMS = 0
}
func (w *worker) send(r *room, id uint16, request uint32, msg proto.Message) {
	if r.Isolated {
		return
	}
	if !r.Ready {
		switch id {
		case p.MsgS2CCastAccepted, p.MsgS2CAttackAccepted, p.MsgS2CCombatImpact, p.MsgS2CCastStep, p.MsgS2CSkillModeState, p.MsgS2CDeathNotice, p.MsgS2CEntityDisplacement, p.MsgS2CPetBorn:
			return
		}
	}
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
	if !r.Ready || r.Isolated || r.DeferMovementSnapshot {
		return
	}
	w.baseline(r)
}

// 入场基线在ACK之前发送；周期同步仍经snapshot的Ready门槛。明确取消沿同一请求号确认后才关闭连接。
func (w *worker) baseline(r *room, request ...uint32) {
	if r.Conn == nil {
		return
	}
	requestID := uint32(0)
	if len(request) > 0 {
		requestID = request[0]
	}
	chunks, err := w.buildBaseline(r, requestID)
	if err != nil {
		w.isolateRoom(r)
		return
	}
	for _, chunk := range chunks {
		w.send(r, p.MsgS2CSceneBaseline, requestID, chunk)
	}
}

// 先构造完整组；新生事实与普通同步使用同一实体投影及真实wire分块。
func (w *worker) buildBaseline(r *room, requestID uint32) ([]*pb.SceneBaselineChunk, error) {
	if r.Sequence == math.MaxUint64 {
		return nil, errors.New("场景基线次序耗尽")
	}
	r.Sequence++
	out := &pb.SceneBaselineChunk{SceneEpoch: r.Scene, BaselineId: r.Baseline, ChunkCount: 1, StateSequence: r.Sequence, SkillModeState: w.modeState(r, time.Now()), NavigationState: r.Navigation.Proto(), OwnedPet: w.ownedPetPB(r)}
	if m, ok := w.currentMap(r); ok {
		if zone, inside := m.SafeZoneAt(r.Player.Pos); inside {
			out.SafeZoneId = zone.ID
		}
	}
	entities := []*entity{&r.Player}
	if pet := r.worldPet(r.currentWorld()); pet != nil {
		entities = append(entities, pet)
	}
	for _, corpse := range r.currentWorld().PetCorpses {
		entities = append(entities, corpse.Body)
	}
	entities = append(entities, r.Monsters[:r.MonsterCount]...)
	entities = append(entities, r.Loot...)
	for _, e := range entities {
		moveMS := uint32(0)
		attackMS := uint32(0)
		var attributes *pb.CombatAttributes
		if e.Kind >= 1 && e.Kind <= 3 {
			stats := w.fighter(r, e)
			moveMS = uint32(stats.MoveMS)
			attackMS = uint32(stats.AttackMS)
			attributes = w.combatAttributesPB(r, e)
		}
		out.Entities = append(out.Entities, &pb.EntityState{EntityHandle: e.Handle, EntityKind: e.Kind, DefinitionId: e.Def, Position: pointPB(e.Pos), Direction: e.Direction, Hp: e.HP, HpMax: e.MaxHP, Mp: e.MP, MpMax: e.MaxMP, Action: e.Action, MoveIntervalMs: moveMS, AttackSequence: e.AttackSequence, CastSequence: e.CastSequence, CastSkillId: e.CastSkillID, Life: e.Life, AttackIntervalMs: attackMS, AttackSkillId: e.AttackSkillID, AttackSkillLevel: e.AttackSkillLevel, Statuses: statusesPB(e, time.Now()), CombatAttributes: attributes, MovementRevision: e.MovementRevision, MovementKind: e.MovementKind, PetTraining: petTrainingPB(e), IsPetCorpse: e.IsPetCorpse})
	}
	return splitBaseline(out, requestID, uint64(r.Auth.SessionEpoch))
}
func pointPB(p devconfig.Point) *pb.GridPoint { return &pb.GridPoint{X: int32(p.X), Y: int32(p.Y)} }
func (w *worker) asset(r *room, f wire.Frame) {
	reviveID := ""
	q := persistence.AssetRequest{Fingerprint: persistence.Hash(append([]byte{byte(f.ID >> 8), byte(f.ID)}, f.Payload...))}
	switch f.ID {
	case p.MsgC2SSetAutoRecycle:
		var x pb.SetAutoRecycleRequest
		if proto.Unmarshal(f.Payload, &x) != nil || x.Policy == nil {
			w.failure(r, f, p.CodeInvalidArgument)
			return
		}
		q.Kind, q.Context, q.AutoRecycle, q.ApplyExisting = "set_auto_recycle", x.Operation, x.Policy, x.ApplyExisting
	case p.MsgC2SSetItemLock:
		var x pb.SetItemLockRequest
		if proto.Unmarshal(f.Payload, &x) != nil || len(x.ItemInstanceId) != 16 {
			w.failure(r, f, p.CodeInvalidArgument)
			return
		}
		q.Kind, q.Context, q.ItemID, q.ItemVersion, q.Locked = "set_item_lock", x.Operation, domain.Hex(x.ItemInstanceId), x.ItemVersion, x.Locked
	case p.MsgC2SSetCastingSupply:
		var x pb.SetCastingSupplyRequest
		if proto.Unmarshal(f.Payload, &x) != nil || len(x.ItemInstanceId) != 16 {
			w.failure(r, f, p.CodeInvalidArgument)
			return
		}
		q.Kind, q.Context, q.ItemID, q.ItemVersion, q.Unequip = "set_casting_supply", x.Operation, domain.Hex(x.ItemInstanceId), x.ItemVersion, x.Unequip
	case p.MsgC2SSkillAssetCommit:
		var x pb.SkillAssetCommitRequest
		if proto.Unmarshal(f.Payload, &x) != nil || len(x.PreviewId) != 16 {
			w.failure(r, f, p.CodeInvalidArgument)
			return
		}
		q.Kind = "skill_assets_commit"
		q.Context = x.Operation
		q.PreviewID = domain.Hex(x.PreviewId)
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
	case p.MsgC2SLearnSkill:
		var x pb.LearnSkillRequest
		if proto.Unmarshal(f.Payload, &x) != nil || len(x.ItemInstanceId) != 16 {
			w.failure(r, f, p.CodeInvalidArgument)
			return
		}
		q.Kind = "learn_skill"
		q.Context = x.Operation
		q.ItemID = domain.Hex(x.ItemInstanceId)
		q.SkillID = x.SkillId
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
	if r.AssetGeneration == math.MaxUint64 {
		w.isolateRoom(r)
		return
	}
	r.AssetGeneration++
	binding := assetBinding{room: r, auth: r.Auth, generation: r.AssetGeneration, id: domain.Hex(q.Context.OpId), fingerprint: q.Fingerprint}
	r.Pending = true
	r.PendingOperationID = domain.Hex(q.Context.OpId)
	r.PendingFingerprint = q.Fingerprint
	auth := r.Auth
	var j job
	j = job{target: w, run: func() func(*worker) {
		role, result, receipt, err := w.e.Store.AssetWithReceipt(w.e.ctx, auth, q)
		return func(w *worker) {
			w.finishAsset(binding, q, f, reviveID, role, result, receipt, err, &j)
		}
	}}
	if !w.submit(j) {
		r.PendingRetry = &j
		r.RetryAt = time.Now().Add(100 * time.Millisecond)
	}
}

func economic(id uint16) bool {
	switch id {
	case p.MsgC2SEquip, p.MsgC2SRecycle, p.MsgC2SClaimHostingTime, p.MsgC2SBuyHostingTime, p.MsgC2SReviveChoice, p.MsgC2SUseItem, p.MsgC2SLearnSkill, p.MsgC2SSkillAssetCommit, p.MsgC2SSetCastingSupply, p.MsgC2SCastWithMaterial, p.MsgC2SSetAutoRecycle, p.MsgC2SSetItemLock:
		return true
	}
	return false
}
func operationContext(f wire.Frame) *pb.OperationContext {
	var op *pb.OperationContext
	switch f.ID {
	case p.MsgC2SSetAutoRecycle:
		q := &pb.SetAutoRecycleRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SSetItemLock:
		q := &pb.SetItemLockRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SCastWithMaterial:
		q := &pb.CastWithMaterialRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SSetCastingSupply:
		q := &pb.SetCastingSupplyRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SSkillAssetCommit:
		q := &pb.SkillAssetCommitRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
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
	case p.MsgC2SLearnSkill:
		q := &pb.LearnSkillRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	}
	return op
}
func operationID(f wire.Frame) []byte {
	op := operationContext(f)
	if op != nil {
		return op.OpId
	}
	return nil
}
