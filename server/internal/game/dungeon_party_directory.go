// 本文件维护单个运行服的公开招募、成员资格、版本及队长离线回收。
package game

import (
	"cmp"
	"slices"
	"sync/atomic"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	dungeonPartyList   uint32 = iota + 1 // 查询本服可招募房间和自己的队伍。
	dungeonPartyCreate                   // 以原申请身份建立一个公开招募房间。
	dungeonPartyJoin                     // 加入满足约束且仍有空位的队伍。
	dungeonPartyLeave                    // 主动退队；队长退出同时解散。
	dungeonPartyKick                     // 队长从招募房间移除一名成员。
	dungeonPartyUpdate                   // 队长调整招募条件，不主动移除原成员。
)

const dungeonPartyJobMask uint32 = 0b111 // 职业位按战士、法师和道士的协议编号映射。
const dungeonPartyListLimit = 20         // 单包公开房间上限，自己的队伍始终单独展示。

// dungeonPartyRevision 防止同一申请编号解散后再创建时旧操作撞上新队伍的版本。
var dungeonPartyRevision atomic.Uint64

// dungeonPartyIdentity 是原角色分片已确认的身份，客户端不能自行赋值。
type dungeonPartyIdentity struct {
	id       string                // 角色永久编号。
	name     string                // 角色当前显示名。
	job      uint32                // 真实职业编号。
	level    uint32                // 角色最新已确认等级。
	binding  uint64                // 采样时的附着代次。
	presence *dungeonPartyPresence // 原角色分片更新的全会话观察。
}

type dungeonPartyRequirements struct {
	minimumLevel uint32 // 入队者最低等级，不低于副本模板。
	jobsMask     uint32 // 允许申请入队的职业集合。
	sameJob      bool   // 是否接受队中已有相同职业。
}

type dungeonPartyMember struct {
	dungeonPartyIdentity
	online bool // 目录最近一次已确认的在线状态。
}

type dungeonParty struct {
	id           string                   // 招募的唯一申请身份。
	revision     uint64                   // 队伍条件、人数或在线状态变化后递增。
	template     uint32                   // 该队招募的已装载副本模板。
	createdAt    time.Time                // 稳定排序公开队伍的创建时刻。
	requirements dungeonPartyRequirements // 新成员必须满足的条件。
	members      []dungeonPartyMember     // 按入队次序排列，索引零为队长。
	deadline     time.Time                // 队长离线后的解散时刻，在线时为空。
}

// dungeonPartyDirectory 由所属运行服的唯一分片写入。
type dungeonPartyDirectory struct {
	parties map[string]*dungeonParty // 招募编号到当前队伍。
	byRole  map[string]*dungeonParty // 角色同时只占一个本服招募席位。
}

func newDungeonPartyDirectory() *dungeonPartyDirectory {
	return &dungeonPartyDirectory{parties: make(map[string]*dungeonParty), byRole: make(map[string]*dungeonParty)}
}

// validDungeonPartyRequest 拒绝不属于该动作的字段，避免旧界面把参数误用到另一类操作。
func validDungeonPartyRequest(q *pb.DungeonPartyRequest) bool {
	partyID, targetID := len(q.PartyId), len(q.TargetRoleId)
	filters := q.MinimumLevel != 0 || q.AllowedJobsMask != 0 || q.AllowSameJob
	if q.SceneEpoch == 0 || q.AllowedJobsMask&^dungeonPartyJobMask != 0 {
		return false
	}
	switch q.Action {
	case dungeonPartyList:
		return partyID == 0 && targetID == 0 && q.ExpectedRevision == 0 && q.TemplateId == 0 && !filters
	case dungeonPartyCreate:
		return partyID == 16 && targetID == 0 && q.ExpectedRevision == 0 && q.TemplateId != 0
	case dungeonPartyJoin, dungeonPartyLeave:
		return partyID == 16 && targetID == 0 && q.ExpectedRevision != 0 && q.TemplateId == 0 && !filters
	case dungeonPartyKick:
		return partyID == 16 && targetID == 16 && q.ExpectedRevision != 0 && q.TemplateId == 0 && !filters
	case dungeonPartyUpdate:
		return partyID == 16 && targetID == 0 && q.ExpectedRevision != 0 && q.TemplateId == 0
	}
	return false
}

func (d *dungeonPartyDirectory) handle(identity dungeonPartyIdentity, f *devconfig.Fixture, q *pb.DungeonPartyRequest, now time.Time) *pb.DungeonPartyState {
	d.observe(identity)
	d.advance(now)
	code := d.mutate(identity, f, q, now)
	state := &pb.DungeonPartyState{SceneEpoch: q.SceneEpoch, ErrorCode: code, Templates: dungeonPartyTemplateViews(f)}
	if party := d.byRole[identity.id]; party != nil {
		state.OwnParty = party.view()
	}
	// 所有动作返回最新自己的队伍。条件冲突后可直接用最新版本重新发起。
	public := make([]*dungeonParty, 0, len(d.parties))
	for _, party := range d.parties {
		if party.members[0].online && len(party.members) < devconfig.DungeonMaxPlayers {
			public = append(public, party)
		}
	}
	slices.SortFunc(public, func(a, b *dungeonParty) int {
		if order := a.createdAt.Compare(b.createdAt); order != 0 {
			return order
		}
		return cmp.Compare(a.id, b.id)
	})
	state.More = len(public) > dungeonPartyListLimit
	for _, party := range public[:min(len(public), dungeonPartyListLimit)] {
		state.Parties = append(state.Parties, party.view())
	}
	return state
}

// mutate 仅处理普通场景 owner 已通过校验的原申请。
func (d *dungeonPartyDirectory) mutate(id dungeonPartyIdentity, f *devconfig.Fixture, q *pb.DungeonPartyRequest, now time.Time) uint32 {
	if q.Action == dungeonPartyList {
		return 0
	}
	own := d.byRole[id.id]
	partyID := domain.Hex(q.PartyId)
	party := d.parties[partyID]
	if q.Action == dungeonPartyCreate {
		if own != nil && own == party && party.members[0].id == id.id && party.template == q.TemplateId {
			return 0 // 原创建响应丢失时返回现有房间，条件不会被旧包覆盖。
		}
		return d.create(id, f, q, party, own, now)
	}
	if q.Action == dungeonPartyJoin && own != nil && own == party {
		return 0 // 原加入重试不再占用另一席位。
	}
	if q.Action == dungeonPartyLeave && own == nil {
		return 0 // 原退队已完成时不再改动任意队伍。
	}
	if party == nil || q.ExpectedRevision != party.revision {
		return p.CodeOperationConflict
	}
	if q.Action == dungeonPartyJoin {
		if own != nil {
			return p.CodeOperationConflict
		}
		return d.join(id, party)
	}
	if own != party {
		return p.CodeLineForbidden
	}
	if q.Action == dungeonPartyLeave {
		d.removeMember(party, id.id)
		return 0
	}
	if party.members[0].id != id.id {
		return p.CodeLineForbidden
	}
	if q.Action == dungeonPartyKick {
		target := domain.Hex(q.TargetRoleId)
		if target == id.id || d.byRole[target] != party {
			return p.CodeInvalidArgument
		}
		d.removeMember(party, target)
		return 0
	}
	return party.update(f, q)
}

func (d *dungeonPartyDirectory) create(id dungeonPartyIdentity, f *devconfig.Fixture, q *pb.DungeonPartyRequest, party, own *dungeonParty, now time.Time) uint32 {
	if party != nil || own != nil {
		return p.CodeOperationConflict
	}
	template, exists := f.DungeonTemplateByID(q.TemplateId)
	if !exists || id.level < template.MinimumLevel || id.job >= 3 {
		return p.CodeLineForbidden
	}
	created := &dungeonParty{id: domain.Hex(q.PartyId), revision: dungeonPartyRevision.Add(1), template: q.TemplateId, createdAt: now,
		requirements: dungeonPartyRules(template, q), members: []dungeonPartyMember{{dungeonPartyIdentity: id, online: true}}}
	d.parties[created.id], d.byRole[id.id] = created, created
	return 0
}

func dungeonPartyRules(template *devconfig.DungeonConfig, q *pb.DungeonPartyRequest) dungeonPartyRequirements {
	mask := q.AllowedJobsMask
	if mask == 0 {
		mask = dungeonPartyJobMask
	}
	return dungeonPartyRequirements{minimumLevel: max(template.MinimumLevel, q.MinimumLevel), jobsMask: mask, sameJob: q.AllowSameJob}
}

func (d *dungeonPartyDirectory) join(id dungeonPartyIdentity, party *dungeonParty) uint32 {
	if !party.members[0].online || !party.requirements.accepts(id, party.members) {
		return p.CodeLineForbidden
	}
	if len(party.members) >= devconfig.DungeonMaxPlayers {
		return p.CodeLineFull
	}
	party.members = append(party.members, dungeonPartyMember{dungeonPartyIdentity: id, online: true})
	party.revision = dungeonPartyRevision.Add(1)
	d.byRole[id.id] = party
	return 0
}

func (rule dungeonPartyRequirements) accepts(id dungeonPartyIdentity, earlier []dungeonPartyMember) bool {
	if id.job >= 3 || id.level < rule.minimumLevel || rule.jobsMask&(1<<id.job) == 0 {
		return false
	}
	return rule.sameJob || !slices.ContainsFunc(earlier, func(member dungeonPartyMember) bool { return member.job == id.job })
}

// update 不隐式踢掉已加入的成员，全部满足新条件后才更新同一队伍。
func (party *dungeonParty) update(f *devconfig.Fixture, q *pb.DungeonPartyRequest) uint32 {
	template, ok := f.DungeonTemplateByID(party.template)
	if !ok {
		return p.CodeLineForbidden
	}
	rule := dungeonPartyRules(template, q)
	for i := 1; i < len(party.members); i++ {
		if !rule.accepts(party.members[i].dungeonPartyIdentity, party.members[:i]) {
			return p.CodeLineForbidden
		}
	}
	if rule != party.requirements {
		party.requirements = rule
		party.revision = dungeonPartyRevision.Add(1)
	}
	return 0
}

func (d *dungeonPartyDirectory) removeMember(party *dungeonParty, roleID string) {
	if party.members[0].id == roleID {
		d.dissolve(party)
		return
	}
	for i, member := range party.members {
		if member.id == roleID {
			delete(d.byRole, roleID)
			party.members = slices.Delete(party.members, i, i+1)
			party.revision = dungeonPartyRevision.Add(1)
			return
		}
	}
}

func (d *dungeonPartyDirectory) dissolve(party *dungeonParty) {
	delete(d.parties, party.id)
	for _, member := range party.members {
		delete(d.byRole, member.id)
	}
}

// observe 兼容普通房间回收后同角色新票，队员身份按附着代次单调更新。
func (d *dungeonPartyDirectory) observe(id dungeonPartyIdentity) {
	party := d.byRole[id.id]
	if party == nil {
		return
	}
	for i, member := range party.members {
		if member.id == id.id && id.binding >= member.binding {
			party.members[i].dungeonPartyIdentity = id
			return
		}
	}
}

// advance 不依赖断线通知进入普通队列；超期队长的新票不能恢复已过期招募。
func (d *dungeonPartyDirectory) advance(now time.Time) {
	for _, party := range d.parties {
		changed := false
		previousDeadline := party.deadline
		for i := range party.members {
			member := &party.members[i]
			attachment := member.presence.current.Load()
			online := attachment.online()
			if i == 0 && party.expired(attachment, online, now) {
				d.dissolve(party)
				break
			}
			if online != member.online {
				member.online = online
				changed = true
			}
		}
		if changed || !previousDeadline.Equal(party.deadline) {
			party.revision = dungeonPartyRevision.Add(1)
		}
	}
}

func (party *dungeonParty) expired(attachment *dungeonPartyAttachment, online bool, now time.Time) bool {
	if attachment != nil && !attachment.disconnectedAt.IsZero() {
		deadline := attachment.disconnectedAt.Add(devconfig.DungeonLeaderDisconnectGrace)
		if attachment.conn != nil {
			// 已恢复过的断线只能证明原间隔。新连接刚关闭时等原分片发布本次精确时间。
			if !attachment.resumedAt.Before(deadline) {
				return true
			}
		} else {
			party.deadline = deadline
		}
	}
	if online {
		party.deadline = time.Time{}
		return false
	}
	if party.deadline.IsZero() {
		party.deadline = now.Add(devconfig.DungeonLeaderDisconnectGrace)
	}
	return !now.Before(party.deadline)
}

func (party *dungeonParty) view() *pb.DungeonPartyView {
	view := &pb.DungeonPartyView{PartyId: domain.Bytes(party.id), Revision: party.revision, TemplateId: party.template,
		MinimumLevel: party.requirements.minimumLevel, AllowedJobsMask: party.requirements.jobsMask, AllowSameJob: party.requirements.sameJob,
		LeaderRoleId: domain.Bytes(party.members[0].id), LeaderOfflineUntilUnixMs: optionalBossUnixMillis(party.deadline)}
	for _, member := range party.members {
		view.Members = append(view.Members, &pb.DungeonPartyMember{RoleId: domain.Bytes(member.id), DisplayName: member.name,
			Job: member.job, Level: member.level, Online: member.online})
	}
	return view
}
