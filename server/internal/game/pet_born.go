package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"time"
)

// 仅新生显示使用的附件凭据；Ready本身仍保留原战斗/经济语义。
type petBornAttachment struct {
	Conn                       Connection
	Auth                       persistence.Auth
	RoleID                     string
	Origin                     uint64
	World                      worldRef
	Execution, Scene, Baseline uint64
}

func capturePetBornAttachment(r *room) *petBornAttachment {
	if !r.Ready || r.Isolated || r.Auth.SessionEpoch <= 0 || r.Auth.RoleID != r.Role.ID || r.Conn == nil || !r.Conn.Alive() || r.Conn.Binding() != uint64(r.Auth.SessionEpoch) || r.Scene == 0 || r.Baseline == 0 {
		return nil
	}
	return &petBornAttachment{Conn: r.Conn, Auth: r.Auth, RoleID: r.Role.ID, Origin: r.Role.Origin, World: r.currentWorld().Ref, Execution: r.currentWorld().Ref.Epoch, Scene: r.Scene, Baseline: r.Baseline}
}

func (a *petBornAttachment) current(r *room) bool {
	return a != nil && r.Ready && !r.Isolated && r.Conn == a.Conn && r.Conn != nil && r.Conn.Alive() && r.Conn.Binding() == uint64(a.Auth.SessionEpoch) &&
		r.Auth == a.Auth && r.Role.ID == a.RoleID && r.Role.Origin == a.Origin && r.ExecutionEpoch == a.Execution && r.currentWorld().Ref == a.World && r.Scene == a.Scene && r.Baseline == a.Baseline
}

// 先实际成功入发送队列，再记录当前附件；失败不能给后续新票继承显示资格。
func (w *worker) sendSyncReady(r *room, request uint32, value *pb.SyncReady) {
	r.PetBornReady = nil
	if r.Isolated || r.Conn == nil {
		return
	}
	conn, binding := r.Conn, uint64(r.Auth.SessionEpoch)
	var attachment *petBornAttachment
	if w.e.Store.Cfg.Fixture.RuleVersion >= 18 && value != nil && value.SceneEpoch == r.Scene && value.BaselineId == r.Baseline {
		attachment = capturePetBornAttachment(r)
	}
	if !conn.Deliver(wire.Message(p.MsgS2CSyncReady, request, binding, value)) {
		conn.Close("sync ready not queued")
		return
	}
	if attachment.current(r) {
		r.PetBornReady = attachment
	}
}

// 只在本次正规install之后产生局部事实，不挂到Role、Owned、身体快照或重试队列。
type petBirthFact struct {
	Attachment *petBornAttachment
	Actor      actorRef
	Body       *entity
	Definition uint32
	Position   devconfig.Point
	Direction  uint32
	Movement   uint64
	Cast       uint64
	Skill      uint32
	BornAt     time.Time
	Consumed   bool
}

func newPetBirthFact(r *room, world *worldState, pet *entity, cast pendingCast, now time.Time) *petBirthFact {
	if world == nil || pet == nil || cast.Summon == nil || cast.Summon.Recall || cast.Skill.ID != 17 || pet.PetRuntime == nil || !pet.PetRuntime.Train || pet.PetRuntime.LegacyUntrained {
		return nil
	}
	return &petBirthFact{Attachment: r.PetBornReady, Actor: referenceIn(world, pet), Body: pet, Definition: pet.Def, Position: pet.Pos, Direction: pet.Direction,
		Movement: pet.MovementRevision, Cast: cast.ID, Skill: cast.Skill.ID, BornAt: now}
}

func (f *petBirthFact) current(r *room) bool {
	return f != nil && f.Attachment != nil && r.PetBornReady == f.Attachment && f.Attachment.current(r) && f.Actor.World.Epoch == r.ExecutionEpoch &&
		r.currentWorld().Ref == f.Actor.World && r.Pet == f.Body && r.worldPet(r.currentWorld()) == f.Body && r.Pet != nil && r.Pet.HP > 0 && !r.Pet.IsPetCorpse &&
		r.Pet.Handle == f.Actor.Handle && r.Pet.Life == f.Actor.Life && r.Pet.Def == f.Definition && r.Pet.Pos == f.Position && r.Pet.Direction == f.Direction && r.Pet.MovementRevision == f.Movement
}

func petBornElapsed(born, sent time.Time) (uint32, bool) {
	// 生产born/sent均保留Go单调时钟；Unix毫秒仅作为实际出生事实展示，客户端不据此算时差。
	elapsed := sent.Sub(born)
	if elapsed < 0 || elapsed/time.Millisecond > math.MaxUint32 {
		return 0, false
	}
	return uint32(elapsed / time.Millisecond), true
}

func (w *worker) publishPetBorn(r *room, fact *petBirthFact) {
	if fact == nil || fact.Consumed {
		return
	}
	fact.Consumed = true
	if w.e.Store.Cfg.Fixture.RuleVersion < 18 || fact.Skill != 17 || fact.Cast == 0 || r.DeferMovementSnapshot || !fact.current(r) {
		return
	}
	roleID := domain.Bytes(fact.Attachment.RoleID)
	bornUnix := fact.BornAt.UnixMilli()
	if len(roleID) != 16 || fact.Attachment.Origin == 0 || bornUnix < 0 {
		return
	}
	if _, ok := petBornElapsed(fact.BornAt, time.Now()); !ok {
		return
	}
	m, known := w.worldMap(r.currentWorld())
	if !known {
		return
	}
	chunks, err := w.buildBaseline(r, 0)
	if err != nil {
		w.isolateRoom(r)
		return
	}
	sequence := chunks[0].StateSequence
	conn, binding := fact.Attachment.Conn, uint64(fact.Attachment.Auth.SessionEpoch)
	for _, chunk := range chunks {
		if !fact.current(r) || r.Sequence != sequence {
			return
		}
		if !conn.Deliver(wire.Message(p.MsgS2CSceneBaseline, 0, binding, chunk)) {
			conn.Close("pet birth baseline not queued")
			return
		}
	}
	if !fact.current(r) || r.Sequence != sequence {
		return
	}
	elapsed, ok := petBornElapsed(fact.BornAt, time.Now())
	if !ok {
		return
	}
	event := &pb.PetBorn{SceneEpoch: fact.Attachment.Scene, BaselineId: fact.Attachment.Baseline, StateSequence: sequence, EntityHandle: fact.Actor.Handle, EntityLife: fact.Actor.Life,
		DefinitionId: fact.Definition, MapId: m.ID, SourceCastId: fact.Cast, SourceSkillId: fact.Skill, BornAtUnixMs: uint64(bornUnix), MovementRevision: fact.Movement, Direction: fact.Direction,
		Role: &pb.RoleKey{RoleId: roleID, OriginServerId: fact.Attachment.Origin}, ElapsedMs: elapsed}
	if !conn.Deliver(wire.Message(p.MsgS2CPetBorn, 0, binding, event)) {
		conn.Close("pet birth fact not queued")
	}
}
