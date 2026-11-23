// 本文件定义首领场景 actor 的生命周期和伤害事实合同。
// 只有角色 owner 已经持久批准的入场可以转为参战席位；本模块不写资产。
package game

import (
	"errors"
	"sort"
	"time"
)

// bossChallengeIdentity 是已登记挑战的运行服、场次、执行代次和规则身份。
type bossChallengeIdentity struct {
	runID       uint64 // 获准承载场次的运行服，场次不跨运行服共享。
	challengeID string // 角色 owner 或公共场次调度器持久分配的稳定挑战编号。
	execution   uint64 // 获准执行挑战的场景代次，隔离旧场次的迟到事实。
	ruleVersion uint64 // 入场时采用的规则版本。
	configHash  string // 入场模板、属性和技能规则所引用的配置哈希。
}

// bossParticipantGrant 表示已确认的永久入场预约及本次不可变战斗快照引用。
type bossParticipantGrant struct {
	challenge      bossChallengeIdentity // 预约绑定的挑战和执行代次。
	roleID         string                // 在本场次里拥有参与结果的角色。
	reservationID  string                // 原角色 owner 已持久确认的业务预约回执。
	combatSnapshot string                // 冻结属性、装备计算和已学技能的快照引用。
}

// bossFinishKind 区分实际击杀、逃走和已授权的取消，以供 owner 选用合法资产来源。
type bossFinishKind uint8

const (
	// bossDefeated 表示权威生命已经被实战伤害扣完。
	bossDefeated bossFinishKind = iota + 1
	// bossEscaped 表示挑战到达本场次的时限且首领尚活着。
	bossEscaped
	// bossInterrupted 表示已确认的场次中断，需要 owner 执行原业务终止规则。
	bossInterrupted
	// bossPlayerDefeated 表示个人挑战的原战斗身体先被首领击败。
	bossPlayerDefeated
)

// bossContribution 以活跃席位为归属，宠物造成的有效伤害与主人伤害累加。
type bossContribution struct {
	grant    bossParticipantGrant // 角色 owner 可核验的原预约身份。
	damage   int64                // 实际从共享生命扣除的归属伤害，不累计过量伤害。
	lastHit  uint64               // 本参战席位最近接纳的内部命中序号。
	joinedAt time.Time            // 有效入场的模拟时间。
}

// bossFinishProposal 是冻结终态后的原始事实，仍需角色 owner 核验并做持久结算。
type bossFinishProposal struct {
	challenge     bossChallengeIdentity // 原授权代次和配置引用。
	kind          bossFinishKind        // 一场挑战只允许存在一种原始终态。
	finishedAt    time.Time             // 实际击杀、到期或中断的规则时间。
	hpRemaining   int64                 // 终态锁定的权威首领生命。
	contributions []bossContribution    // 冻结贡献和预约引用，供各角色 owner 核验。
}

// bossDamageFact 必须来自场景已完成格子、技能、宠物归属和命中校验的一次伤害。
type bossDamageFact struct {
	grant    bossParticipantGrant // 此伤害所属角色的既有预约和快照。
	sequence uint64               // 来自场景逻辑的单调去重序号。
	at       time.Time            // 这次合法命中的模拟时间。
	amount   int64                // 战斗核心确认的原始正伤害。
}

// bossChallenge 持有一个首领实体的权威生命及有界参与表，只能由该场景 actor 修改。
type bossChallenge struct {
	identity        bossChallengeIdentity        // 本场次固定的授权身份。
	personalRole    string                       // 个人首领只接受该角色；公共场次为空。
	startsAt        time.Time                    // 挑战或公共场次开放时间。
	expiresAt       time.Time                    // 首领仍未被击杀时的真实逃走时间。
	lastEventAt     time.Time                    // 权威事件时钟，不允许新入场、命中或取消倒拨时间。
	hp              int64                        // 当前场次唯一可被战斗核心扣除的首领生命。
	maxParticipants int                          // 已批准的场景人数预算，个人挑战固定一个席位。
	participants    map[string]*bossContribution // 当前场次已激活的参战席位。
	finish          *bossFinishProposal          // 首次冻结终态的结果，等待原 owner 处理。
}

// newBossChallenge 使用已批准的配置和模板快照创建一次独立场次。
func newBossChallenge(identity bossChallengeIdentity, personalRole string, startsAt, expiresAt time.Time, hp int64, maxParticipants int) (*bossChallenge, error) {
	if identity.runID == 0 || identity.challengeID == "" || identity.execution == 0 || identity.ruleVersion == 0 || identity.configHash == "" ||
		startsAt.IsZero() || !expiresAt.After(startsAt) || hp <= 0 || maxParticipants < 1 || maxParticipants > maxRoomsPerWorker*workerCount ||
		personalRole != "" && maxParticipants != 1 {
		return nil, errors.New("首领场次权限、时段、实体模板或人数预算无效")
	}
	return &bossChallenge{identity: identity, personalRole: personalRole, startsAt: startsAt, expiresAt: expiresAt,
		hp: hp, maxParticipants: maxParticipants, participants: make(map[string]*bossContribution)}, nil
}

// admit 优先校验已确认的场次、预约和快照身份，同一回执重放仍只占一个席位。
func (b *bossChallenge) admit(grant bossParticipantGrant, now time.Time) bool {
	if b.finish != nil || now.Before(b.startsAt) || !now.Before(b.expiresAt) || grant.challenge != b.identity ||
		grant.roleID == "" || grant.reservationID == "" || grant.combatSnapshot == "" || b.personalRole != "" && grant.roleID != b.personalRole {
		return false
	}
	if existing := b.participants[grant.roleID]; existing != nil {
		return existing.grant == grant
	}
	if len(b.participants) == b.maxParticipants || now.Before(b.lastEventAt) {
		return false
	}
	b.participants[grant.roleID] = &bossContribution{grant: grant, joinedAt: now}
	b.lastEventAt = now
	return true
}

// hit 只对已入场的原身份应用实际命中，并由首次归零冻结一个终态。
func (b *bossChallenge) hit(fact bossDamageFact) int64 {
	if b.finish != nil || fact.amount <= 0 || !fact.at.Before(b.expiresAt) || fact.at.Before(b.lastEventAt) {
		return 0
	}
	participant := b.participants[fact.grant.roleID]
	if participant == nil || participant.grant != fact.grant || fact.sequence <= participant.lastHit || fact.at.Before(participant.joinedAt) {
		return 0
	}
	effective := min(b.hp, fact.amount)
	b.lastEventAt, participant.lastHit = fact.at, fact.sequence
	b.hp -= effective
	participant.damage += effective
	if b.hp == 0 {
		b.freeze(bossDefeated, fact.at)
	}
	return effective
}

// expire 在调度时间达到期限时生成到期事实，重复 tick 不会增加任何结果。
func (b *bossChallenge) expire(now time.Time) {
	if b.finish == nil && !now.Before(b.expiresAt) {
		b.freeze(bossEscaped, b.expiresAt)
	}
}

// interrupt 响应当前场次的权威中断命令，使旧执行无法抢在取消之后确认击杀。
func (b *bossChallenge) interrupt(expected bossChallengeIdentity, now time.Time) bool {
	if expected != b.identity || b.finish != nil || now.Before(b.lastEventAt) || now.Before(b.startsAt) {
		return false
	}
	if !now.Before(b.expiresAt) {
		b.expire(now)
		return false
	}
	b.freeze(bossInterrupted, now)
	return true
}

// playerDefeated 记录原个人挑战角色在到期前收到的致死命中。
func (b *bossChallenge) playerDefeated(grant bossParticipantGrant, at time.Time) bool {
	if b.finish != nil || b.personalRole == "" || grant.roleID != b.personalRole || !at.Before(b.expiresAt) || at.Before(b.lastEventAt) {
		return false
	}
	participant := b.participants[grant.roleID]
	if participant == nil || participant.grant != grant || at.Before(participant.joinedAt) {
		return false
	}
	b.freeze(bossPlayerDefeated, at)
	return true
}

// freeze 锁定首次终态并按角色编号稳定输出贡献；业务奖励排名仍由审定规则决定。
func (b *bossChallenge) freeze(kind bossFinishKind, at time.Time) {
	contributions := make([]bossContribution, 0, len(b.participants))
	for _, participant := range b.participants {
		contributions = append(contributions, *participant)
	}
	sort.Slice(contributions, func(i, j int) bool { return contributions[i].grant.roleID < contributions[j].grant.roleID })
	b.finish = &bossFinishProposal{challenge: b.identity, kind: kind, finishedAt: at, hpRemaining: b.hp, contributions: contributions}
}

// proposal 返回不可变终态的副本供重试投递；角色 owner 以挑战 ID、执行代次和持久围栏去重。
func (b *bossChallenge) proposal() (bossFinishProposal, bool) {
	if b.finish == nil {
		return bossFinishProposal{}, false
	}
	result := *b.finish
	result.contributions = append([]bossContribution(nil), result.contributions...)
	return result, true
}
