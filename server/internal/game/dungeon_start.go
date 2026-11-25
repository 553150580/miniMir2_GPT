// 本文件在运行服分片协调讨伐链原队伍开战。角色资料只由原角色分片冻结，扣次结果只接受原事务确认。
package game

import (
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

const dungeonStartPrepareWindow = 4 * time.Second        // 只限制写入前的组队准备，提交未知时必须继续核对。
const dungeonStartRetryInterval = 100 * time.Millisecond // 分片或数据库暂忙时保留原申请，等待可用容量。

// dungeonPreparedMember 是原角色分片冻结的入场视图，不包含可修改的普通场景指针。
type dungeonPreparedMember struct {
	owner    *worker                            // 继续拥有该角色普通场景及资产的分片。
	auth     persistence.Auth                   // 入场扣次所绑定的角色、账号和会话代次。
	conn     Connection                         // 开战发起时已经确认的附着连接。
	scene    uint64                             // 为本场保留的新场景代次。
	baseline uint64                             // 为本场保留的全量场景基线编号。
	snapshot personalBossSnapshot               // 原职业、属性、已学技能与宠物拍下的只读战斗快照。
	role     domain.Role                        // 新场景投影所需的精简角色身份。
	assets   roleAssetView                      // 与确认后的资产修订一致的首次场景视图。
	durable  persistence.DungeonAdmissionMember // 多角色事务需要核对的原版本和战斗快照摘要。
}

type dungeonStartPreparation struct {
	member *dungeonPreparedMember // 成功时原 owner 已占用角色的待确认槽，直到同一开战回填。
	code   uint32                 // 原 owner 的准备判定，暂忙时仍可准备原成员。
}

type dungeonStartSlot struct {
	identity  dungeonPartyIdentity                    // 发起开战时冻结的原成员及其入队代次。
	owner     *worker                                 // 向其投递准备和最终回填的原分片。
	answer    atomic.Pointer[dungeonStartPreparation] // 原分片直接发布准备结果，回执不会因目标队列拥塞丢失。
	released  atomic.Bool                             // 原分片已处理最终释放和可选场景安装。
	posted    bool                                    // 当前准备请求已投递，等待原分片结果。
	completed bool                                    // 已向原分片发送最终回填，不能重复投递。
	retryAt   time.Time                               // 临时忙等待原结算释放后的下一次准备机会。
}

// dungeonStartAttempt 固定同一原队伍、模板、角色快照和开始请求，数据库未知时只重投这份请求。
type dungeonStartAttempt struct {
	store        *persistence.Store           // 当前运行服的数据库围栏。
	template     *devconfig.DungeonConfig     // 开战选中的已装配模板。
	begin        persistence.DungeonBegin     // 事务参数只在首次实际提交前补齐。
	leader       dungeonPartyIdentity         // 只向发起开战的原队长回复请求。
	conn         Connection                   // 接收开战结果的原附着。
	scene        uint64                       // 原请求使用的普通地图场景。
	request      uint32                       // 原客户端序号。
	prepareUntil time.Time                    // 超过即释放尚未提交的准备。
	slots        []*dungeonStartSlot          // 固定成员次序，与原队伍一致。
	prepared     []dungeonPreparedMember      // 只在全部成员取得有效锁后组装。
	grant        persistence.DungeonChallenge // 已获 MongoDB 确认的真实场次。
	confirmed    map[string]domain.Role       // 原扣次后回填原 owner 的已确认角色。
	attachments  []*dungeonAttachment         // 只在扣次已确认后分配，由索引匹配准备名单。
	code         uint32                       // 写入前已拒绝、或同事务已确认的失败结果。
	abortCode    uint32                       // 原授权已失效，或成功回执不一致，需要核对并中断未开场的扣次。
	inFlight     bool                         // 原扣次事务已投递且未获得明确结果。
	submitted    bool                         // 至少提交过一次，不能按准备超时擅自解锁。
	uncertain    bool                         // 至少一次原开始事务结果未知，后续硬拒不能作为无扣次证明。
	replied      bool                         // 原队长只收到一次明确开战结果。
	retryAt      time.Time                    // 原事务结果未知时暂停后重试。
}

// startDungeonParty 按原名单修订占有一场开战。后续状态均由当前运行服分片管理。
func (w *worker) startDungeonParty(directory *dungeonPartyDirectory, store *persistence.Store, identity dungeonPartyIdentity, conn Connection,
	request *pb.DungeonPartyRequest, requestID uint32, now time.Time) {
	directory.observe(identity)
	directory.advance(now)
	party := directory.byRole[identity.id]
	reject := func(code uint32) {
		state := directory.state(identity.id, &store.Cfg.Fixture, request.SceneEpoch, code)
		state.Starting = party != nil && party.start != nil
		conn.Deliver(wire.Message(p.MsgS2CDungeonPartyState, requestID, identity.binding, state))
	}
	if party == nil || party.id != domain.Hex(request.PartyId) || party.members[0].id != identity.id {
		reject(p.CodeLineForbidden)
		return
	}
	if party.start != nil {
		reject(p.CodeOperationPending)
		return
	}
	if party.revision != request.ExpectedRevision || w.e.stopped.Load() {
		reject(p.CodeOperationConflict)
		return
	}
	template, enabled := store.Cfg.Fixture.DungeonTemplateByID(party.template)
	if !enabled {
		reject(p.CodeLineForbidden)
		return
	}
	for _, member := range party.members {
		if !member.online {
			reject(p.CodeLineRecovering)
			return
		}
	}
	execution := ownerExecutionEpoch.Add(1)
	if execution == 0 || execution > math.MaxInt64 {
		reject(p.CodeServerBusy)
		return
	}
	start := &dungeonStartAttempt{store: store, template: template, leader: identity, conn: conn, scene: request.SceneEpoch,
		request: requestID, prepareUntil: now.Add(dungeonStartPrepareWindow),
		begin: persistence.DungeonBegin{ChallengeID: domain.ID(), Execution: execution, TemplateID: template.TemplateID,
			RuleVersion: store.Cfg.Fixture.RuleVersion, ConfigHash: store.Cfg.Fixture.Hash, Duration: template.Duration}}
	for _, member := range party.members {
		start.slots = append(start.slots, &dungeonStartSlot{identity: member.dungeonPartyIdentity, owner: w.e.worker(member.id)})
	}
	party.start = start
	w.advanceDungeonStart(directory, party, now)
}

// advanceDungeonStart 先收齐原 owner，再实际扣次，最后将同一 MongoDB 结果送回所有原角色。
func (w *worker) advanceDungeonStart(directory *dungeonPartyDirectory, party *dungeonParty, now time.Time) {
	start := party.start
	if start.code != 0 || start.grant.ChallengeID != "" {
		w.publishDungeonStart(directory, party, now)
		return
	}
	if start.submitted {
		w.submitDungeonStart(start, now)
		return
	}
	if !now.Before(start.prepareUntil) || w.e.stopped.Load() {
		start.code = p.CodeLineRecovering
		w.publishDungeonStart(directory, party, now)
		return
	}
	if !w.collectDungeonStart(start, now) {
		return
	}
	for _, slot := range start.slots {
		member := slot.answer.Load().member
		if !member.conn.Alive() || member.conn.Binding() != uint64(member.auth.SessionEpoch) {
			start.code = p.CodeLineRecovering
			return
		}
	}
	if start.begin.StartedAt.IsZero() {
		start.begin.StartedAt = now.UTC().Truncate(time.Millisecond)
		for _, slot := range start.slots {
			member := slot.answer.Load().member
			start.prepared = append(start.prepared, *member)
			start.begin.Members = append(start.begin.Members, member.durable)
		}
	}
	w.submitDungeonStart(start, now)
}

// collectDungeonStart 的准备回执通过只写一次的原槽传递。暂忙重试请求的仍是原附着。
func (w *worker) collectDungeonStart(start *dungeonStartAttempt, now time.Time) bool {
	ready := true
	for _, slot := range start.slots {
		if reply := slot.answer.Load(); reply != nil {
			switch {
			case reply.code == 0:
				continue
			case reply.code == p.CodeOperationPending:
				slot.answer.Store(nil)
				slot.posted, slot.retryAt = false, now.Add(dungeonStartRetryInterval)
			default:
				start.code = reply.code
				return false
			}
		}
		ready = false
		if !slot.posted && !now.Before(slot.retryAt) {
			slot.posted = w.postWorldBoss(slot.owner, func(owner *worker) {
				slot.answer.Store(owner.prepareDungeonStartOwner(start, slot.identity, time.Now()))
			})
		}
	}
	return ready
}

// submitDungeonStart 永远沿原开始时刻、角色版本、分片和内容摘要查证已投递的扣次。
func (w *worker) submitDungeonStart(start *dungeonStartAttempt, now time.Time) {
	if start.inFlight || now.Before(start.retryAt) {
		return
	}
	request := start.begin
	aborting := start.abortCode != 0
	start.inFlight = w.submit(job{target: w, run: func() func(*worker) {
		var grant persistence.DungeonChallenge
		var roles []domain.Role
		var err error
		if aborting {
			grant, roles, err = start.store.AbortUnattachedDungeon(w.e.ctx, request)
		} else {
			grant, roles, err = start.store.BeginDungeon(w.e.ctx, request)
		}
		return func(w *worker) {
			start.inFlight = false
			code := persistence.Code(err)
			switch {
			case code == p.CodeOperationPending || code == p.CodeServerBusy:
				start.uncertain = true
				start.retryAt = time.Now().Add(dungeonStartRetryInterval)
			case aborting:
				start.code = start.abortCode
				if code == 0 {
					start.confirmRefund(roles)
				} else {
					slog.Error("讨伐链未开场扣次无法在原围栏中核对", "challenge", request.ChallengeID, "error_code", code)
				}
			case code == 0:
				w.confirmDungeonStart(start, grant, roles)
			case start.uncertain && (code == p.CodeUnauthenticated || code == p.CodeStaleSession):
				// 原会话无法读原提交时，仅允许按完整原摘要查证和系统中断。
				start.abortCode = code
			default:
				start.code = code
			}
		}
	}})
	start.submitted = start.submitted || start.inFlight
	if !start.inFlight {
		start.retryAt = now.Add(dungeonStartRetryInterval)
	}
}

// confirmRefund 仅回填已确认退款且仍属于原会话的角色，保留换代席位的版本围栏。
func (start *dungeonStartAttempt) confirmRefund(roles []domain.Role) {
	start.confirmed = make(map[string]domain.Role, len(roles))
	for _, member := range start.prepared {
		for _, role := range roles {
			if role.ID == member.auth.RoleID && role.AccountID == member.auth.AccountID && role.SessionEpoch == member.auth.SessionEpoch &&
				role.Origin == member.role.Origin && role.Revision > member.durable.ExpectedRevision &&
				role.RunServerID == start.store.Cfg.RunID && role.RunEpoch == start.store.Epoch && role.Owner == start.store.Instance &&
				role.Dungeon != nil && role.Dungeon.ActiveChallengeID == "" {
				start.confirmed[role.ID] = role
			}
		}
	}
}

// confirmDungeonStart 先核对持久角色水位和原名单，再分配场次及可观察的初始视图。
func (w *worker) confirmDungeonStart(start *dungeonStartAttempt, grant persistence.DungeonChallenge, roles []domain.Role) {
	confirmed := make(map[string]domain.Role, len(roles))
	for _, role := range roles {
		confirmed[role.ID] = role
	}
	admitted := make(map[string]persistence.DungeonMember, len(grant.Members))
	for _, member := range grant.Members {
		admitted[member.RoleID] = member
	}
	begin := start.begin
	valid := grant.ChallengeID == begin.ChallengeID && grant.Execution == begin.Execution && grant.TemplateID == begin.TemplateID &&
		grant.RuleVersion == begin.RuleVersion && grant.ConfigHash == begin.ConfigHash && grant.RunID == start.store.Cfg.RunID &&
		grant.RunEpoch == start.store.Epoch && grant.OwnerInstance == start.store.Instance && grant.Outcome == persistence.DungeonActive &&
		grant.StartedAt.Equal(begin.StartedAt) && grant.ExpiresAt.Equal(begin.StartedAt.Add(begin.Duration).Truncate(time.Millisecond)) &&
		validTravelID(grant.ReservationID) && len(admitted) == len(start.prepared) && len(grant.Members) == len(start.prepared) &&
		len(confirmed) == len(start.prepared) && len(roles) == len(start.prepared)
	for i := range start.prepared {
		member := &start.prepared[i]
		role := confirmed[member.auth.RoleID]
		seat := admitted[member.auth.RoleID]
		if seat.AccountID != member.auth.AccountID || seat.Origin != member.role.Origin || seat.SessionEpoch != member.auth.SessionEpoch ||
			seat.CombatSnapshot != member.durable.CombatSnapshot || role.Dungeon == nil || role.Dungeon.ActiveChallengeID != grant.ChallengeID ||
			role.Revision != member.durable.ExpectedRevision+1 || role.RunServerID != grant.RunID || role.RunEpoch != grant.RunEpoch || role.Owner != grant.OwnerInstance ||
			role.SessionEpoch != member.auth.SessionEpoch || role.AccountID != member.auth.AccountID || role.Origin != member.role.Origin {
			valid = false
			break
		}
		assets, err := snapshotRoleAssets(role, start.store.Cfg.Fixture)
		if err != nil {
			valid = false
			break
		}
		member.assets = assets
		member.role.Revision = role.Revision
		// 战斗属性仍用冻结快照，只有击杀判绑以扣次事务确认的原权益为观察起点。
		member.role.PrivilegeUntil = role.PrivilegeUntil
	}
	if !valid {
		slog.Error("讨伐链开战原持久结果不一致", "challenge", begin.ChallengeID)
		start.abortCode = p.CodeLineRecovering
		return
	}
	start.grant, start.confirmed = grant, confirmed
	attachments := w.installDungeonBattle(start.store, start.template, grant, start.prepared)
	if len(attachments) != len(start.prepared) {
		start.code = p.CodeLineRecovering
		return
	}
	for _, attachment := range attachments {
		if attachment == nil {
			start.code = p.CodeLineRecovering
			return
		}
	}
	start.attachments = attachments
}

// publishDungeonStart 逐席投递原确认结果。队列饱和只重投尚未接收的席位，释放后才解除队伍锁。
func (w *worker) publishDungeonStart(directory *dungeonPartyDirectory, party *dungeonParty, now time.Time) {
	start := party.start
	if start.code == 0 && !start.replied {
		w.replyDungeonStart(directory, start)
	}
	complete := true
	for index, slot := range start.slots {
		reply := slot.answer.Load()
		if reply == nil {
			complete = complete && !slot.posted
			continue
		}
		if reply.member == nil || slot.released.Load() {
			continue
		}
		complete = false
		if slot.completed {
			continue
		}
		member := *reply.member
		role := start.confirmed[member.auth.RoleID]
		var obs *dungeonAttachment
		if index < len(start.attachments) {
			obs = start.attachments[index]
		}
		slot.completed = w.postWorldBoss(slot.owner, func(owner *worker) {
			owner.completeDungeonStartOwner(start, member, role, func(r *room) {
				if obs != nil {
					owner.activateDungeonMember(r, obs)
				} else if role.Dungeon != nil && role.Dungeon.ActiveChallengeID == start.begin.ChallengeID {
					// 原扣次仍占用角色，禁止缺少副本观察的房间继续运行。
					owner.isolateRoom(r)
				}
			})
			slot.released.Store(true)
		})
	}
	if !complete {
		return
	}
	party.start = nil
	if start.code == 0 {
		directory.dissolve(party)
	} else {
		directory.advance(now)
		w.replyDungeonStart(directory, start)
	}
}

// replyDungeonStart 保留原请求场景，只有确认已扣次的场次才返回永久编号。
func (w *worker) replyDungeonStart(directory *dungeonPartyDirectory, start *dungeonStartAttempt) {
	start.replied = true
	if !start.conn.Alive() || start.conn.Binding() != start.leader.binding {
		return
	}
	state := directory.state(start.leader.id, &start.store.Cfg.Fixture, start.scene, start.code)
	if start.code == 0 {
		state.ChallengeId = domain.Bytes(start.grant.ChallengeID)
		state.Starting = false
	}
	start.conn.Deliver(wire.Message(p.MsgS2CDungeonPartyState, start.request, start.leader.binding, state))
}
