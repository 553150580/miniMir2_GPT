// 本文件在当前可观察场景的完整基线后发送一次宠物出生事实，并核对附着和生命身份。
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
	// Conn 记录连接。
	Conn Connection
	// Auth 记录授权。
	Auth persistence.Auth
	// RoleID 标识当前业务使用的角色。
	RoleID string
	// Origin 记录大区或起点，使用数值表示。
	Origin uint64
	// World 记录场景世界，使用场景世界引用表示。
	World worldRef
	// Execution 记录“execution”，使用数值表示。
	// Scene 记录场景，使用数值表示。
	// Baseline 记录场景基线，使用数值表示。
	Execution, Scene, Baseline uint64
}

// capturePetBornAttachment 返回“capture”宠物出生“attachment”供调用方使用。
// r 指向当前执行器拥有并更新的目标房间。
func capturePetBornAttachment(r *room) *petBornAttachment {
	if r.personalBoss != nil && r.personalBoss.battle != nil || r.worldBossObservation != nil && r.worldBossObservation.shown {
		return nil
	}
	if !r.Ready || r.Isolated || r.Auth.SessionEpoch <= 0 || r.Auth.RoleID != r.Role.ID || r.Conn == nil || !r.Conn.Alive() || r.Conn.Binding() != uint64(r.Auth.SessionEpoch) || r.Scene == 0 || r.Baseline == 0 {
		return nil
	}
	return &petBornAttachment{Conn: r.Conn, Auth: r.Auth, RoleID: r.Role.ID, Origin: r.Role.Origin, World: r.currentWorld().Ref, Execution: r.currentWorld().Ref.Epoch, Scene: r.Scene, Baseline: r.Baseline}
}

// current 判断当前状态是否满足要求。
// a 是调用该方法的宠物出生“attachment”实例，r 指向当前执行器拥有并更新的目标房间。
func (a *petBornAttachment) current(r *room) bool {
	return a != nil && r.Ready && !r.Isolated && r.Conn == a.Conn && r.Conn != nil && r.Conn.Alive() && r.Conn.Binding() == uint64(a.Auth.SessionEpoch) &&
		r.Auth == a.Auth && r.Role.ID == a.RoleID && r.Role.Origin == a.Origin && r.ExecutionEpoch == a.Execution && r.currentWorld().Ref == a.World && r.Scene == a.Scene && r.Baseline == a.Baseline
}

// 先实际成功入发送队列，再记录当前附件；失败不能给后续新票继承显示资格。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，request 关联客户端请求与服务端的对应响应，value 记录值，使用场景同步就绪表示。
func (w *worker) sendSyncReady(r *room, request uint32, value *pb.SyncReady) {
	r.PetBornReady = nil
	if r.Isolated || r.Conn == nil {
		return
	}
	// conn 保存当前处理所用的连接，binding 保存由无符号整数“64”取得的会话绑定。
	conn, binding := r.Conn, uint64(r.Auth.SessionEpoch)
	// attachment 记录“attachment”，使用宠物出生“attachment”表示。
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
	// Attachment 记录“attachment”，使用宠物出生“attachment”表示。
	Attachment *petBornAttachment
	// Actor 记录行动实体，使用行动实体引用表示。
	Actor actorRef
	// Body 记录消息正文，使用实体表示。
	Body *entity
	// Definition 记录定义，使用数值表示。
	Definition uint32
	// Position 记录位置，使用点位表示。
	Position devconfig.Point
	// Direction 记录朝向，使用数值表示。
	Direction uint32
	// Movement 记录移动，使用数值表示。
	Movement uint64
	// Cast 记录施法，使用数值表示。
	Cast uint64
	// Skill 记录技能，使用数值表示。
	Skill uint32
	// BornAt 记录出生发生时间，使用时间点表示。
	BornAt time.Time
	// Consumed 标记是否处于“consumed”状态。
	Consumed bool
}

// newPetBirthFact 构造宠物出生已发生的事实。
// r 指向当前执行器拥有并更新的目标房间，world 记录场景世界，使用场景世界状态表示，pet 记录宠物，使用实体表示，cast 记录施法，使用待确认施法表示。
// now 提供当前时点，作为状态有效期和计时判断的基准。
func newPetBirthFact(r *room, world *worldState, pet *entity, cast pendingCast, now time.Time) *petBirthFact {
	if world == nil || pet == nil || cast.Summon == nil || cast.Summon.Recall || cast.Skill.ID != 17 || pet.PetRuntime == nil || !pet.PetRuntime.Train || pet.PetRuntime.LegacyUntrained {
		return nil
	}
	return &petBirthFact{Attachment: r.PetBornReady, Actor: referenceIn(world, pet), Body: pet, Definition: pet.Def, Position: pet.Pos, Direction: pet.Direction,
		Movement: pet.MovementRevision, Cast: cast.ID, Skill: cast.Skill.ID, BornAt: now}
}

// current 判断当前状态是否满足要求。
// f 是调用该方法的宠物出生已发生的事实实例，r 指向当前执行器拥有并更新的目标房间。
func (f *petBirthFact) current(r *room) bool {
	return f != nil && f.Attachment != nil && r.PetBornReady == f.Attachment && f.Attachment.current(r) && f.Actor.World.Epoch == r.ExecutionEpoch &&
		r.currentWorld().Ref == f.Actor.World && r.Pet == f.Body && r.worldPet(r.currentWorld()) == f.Body && r.Pet != nil && r.Pet.HP > 0 && !r.Pet.IsPetCorpse &&
		r.Pet.Handle == f.Actor.Handle && r.Pet.Life == f.Actor.Life && r.Pet.Def == f.Definition && r.Pet.Pos == f.Position && r.Pet.Direction == f.Direction && r.Pet.MovementRevision == f.Movement
}

// petBornElapsed 处理宠物出生已耗时。
// born 记录出生，使用时间点表示，sent 记录已发送，使用时间点表示。
func petBornElapsed(born, sent time.Time) (uint32, bool) {
	// 生产born/sent均保留Go单调时钟；Unix毫秒仅作为实际出生事实展示，客户端不据此算时差。
	// elapsed 保存由子项取得的已经经过的时长。
	elapsed := sent.Sub(born)
	if elapsed < 0 || elapsed/time.Millisecond > math.MaxUint32 {
		return 0, false
	}
	return uint32(elapsed / time.Millisecond), true
}

// publishPetBorn 处理发布宠物出生。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，fact 记录已发生的事实，使用宠物出生已发生的事实表示。
func (w *worker) publishPetBorn(r *room, fact *petBirthFact) {
	if fact == nil || fact.Consumed {
		return
	}
	fact.Consumed = true
	if w.e.Store.Cfg.Fixture.RuleVersion < 18 || fact.Skill != 17 || fact.Cast == 0 || r.DeferMovementSnapshot || !fact.current(r) {
		return
	}
	// roleID 标识当前业务使用的角色。
	roleID := domain.Bytes(fact.Attachment.RoleID)
	// bornUnix 保存由Unix 时间“milli”取得的出生Unix 时间。
	bornUnix := fact.BornAt.UnixMilli()
	if len(roleID) != 16 || fact.Attachment.Origin == 0 || bornUnix < 0 {
		return
	}
	// ok 表明查询、校验或类型转换是否成功。
	if _, ok := petBornElapsed(fact.BornAt, time.Now()); !ok {
		return
	}
	// m 接收场景世界地图供后续步骤使用，known 保存由场景世界地图取得的已知状态。
	m, known := w.worldMap(r.currentWorld())
	if !known {
		return
	}
	// chunks 保存由构建场景基线取得的“chunks”，err 接收构建场景基线过程中可能产生的错误，供后续失败分支处理。
	chunks, err := w.buildBaseline(r, 0)
	if err != nil {
		w.isolateRoom(r)
		return
	}
	// sequence 保存场景状态序号，用于当前步骤中的序号。
	sequence := chunks[0].StateSequence
	// conn 保存当前处理所用的连接，binding 保存由无符号整数“64”取得的会话绑定。
	conn, binding := fact.Attachment.Conn, uint64(fact.Attachment.Auth.SessionEpoch)
	// chunk 接收遍历“chunks”时当前的元素。
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
	// elapsed 保存由宠物出生已耗时取得的已经经过的时长，ok 表明查询、校验或类型转换是否成功。
	elapsed, ok := petBornElapsed(fact.BornAt, time.Now())
	if !ok {
		return
	}
	// event 保存当前处理所用的事件。
	event := &pb.PetBorn{SceneEpoch: fact.Attachment.Scene, BaselineId: fact.Attachment.Baseline, StateSequence: sequence, EntityHandle: fact.Actor.Handle, EntityLife: fact.Actor.Life,
		DefinitionId: fact.Definition, MapId: m.ID, SourceCastId: fact.Cast, SourceSkillId: fact.Skill, BornAtUnixMs: uint64(bornUnix), MovementRevision: fact.Movement, Direction: fact.Direction,
		Role: &pb.RoleKey{RoleId: roleID, OriginServerId: fact.Attachment.Origin}, ElapsedMs: elapsed}
	if !conn.Deliver(wire.Message(p.MsgS2CPetBorn, 0, binding, event)) {
		conn.Close("pet birth fact not queued")
	}
}
