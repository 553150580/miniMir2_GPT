// 本文件记录讨伐链原场次、真实参战者、入场扣次和同队唯一扣次回执。
package persistence

import (
	"cmp"
	"context"
	"errors"
	"math"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const dungeonCollection = "dungeon_challenges" // 原场次与终态永久保留。

// DungeonOutcome 区分原场次状态、正常战败或超时，以及有资格退款的系统中断。
type DungeonOutcome uint32

const (
	DungeonActive            DungeonOutcome = iota // 原队伍已扣次且尚未确认终态。
	DungeonSucceeded                               // 权威战斗确认通关。
	DungeonFailed                                  // 普通战败、主动离场或到期，保留原扣次。
	DungeonSystemInterrupted                       // 当前合法执行明确取消或新执行回收无结果的旧场次。
)

// DungeonAdmissionMember 只能由各角色所有者提供；AI 补位没有账号、角色或扣次。
type DungeonAdmissionMember struct {
	Auth             Auth   // 当前游戏连接判定的原角色授权。
	ExpectedRevision int64  // 锁定原场次战斗快照的持久角色修订。
	CombatSnapshot   string // 原战斗属性、职业、已学技能和装备的稳定摘要。
}

// DungeonBegin 固定队伍名单和挑战身份，未知结果重试使用同一份请求。
type DungeonBegin struct {
	ChallengeID string                   // 游戏角色所有者协调分配的唯一场次编号。
	Execution   uint64                   // 原挑战执行代次。
	TemplateID  uint32                   // 原讨伐链配置模板。
	RuleVersion uint64                   // 原配置规则版本。
	ConfigHash  string                   // 原版本的不可变配置摘要。
	StartedAt   time.Time                // 由原执行者冻结的正式扣次时刻。
	Duration    time.Duration            // 原配置允许的挑战时长。
	Members     []DungeonAdmissionMember // 各真实参战者的原授权及入场快照。
}

// DungeonMember 记录原角色、永久入口、入场运营日，以及可由同场健康重连推进的会话围栏。
type DungeonMember struct {
	RoleID               string    `bson:"role_id"`                          // 原真实玩家的永久角色编号。
	AccountID            string    `bson:"account_id"`                       // 原角色所属账号。
	Origin               uint64    `bson:"origin"`                           // 角色永久入口服，不能替换其他同服玩家。
	CombatSnapshot       string    `bson:"combat_snapshot"`                  // 原入场冻结的战斗摘要。
	EntryDay             string    `bson:"entry_day"`                        // 扣次时刻所属的上海运营日。
	EntriesAfter         uint32    `bson:"entries_after"`                    // 本场扣次后观察到的原日已用次数。
	DailyLimit           uint32    `bson:"daily_limit"`                      // 原时刻由有效托管卡确定的每日上限。
	HostingCardUntil     time.Time `bson:"hosting_card_until,omitempty"`     // 原扣次采纳的卡截止时间。
	SessionEpoch         int64     `bson:"session_epoch"`                    // 当前可提交本场终态的已确认原席位会话。
	PreviousSessionEpoch int64     `bson:"previous_session_epoch,omitempty"` // 核对最近一次健康接回的原请求。
}

// DungeonChallenge 只保存永久业务事实。临时战斗场、血量和客户端连接不存入该记录。
type DungeonChallenge struct {
	ChallengeID   string          `bson:"_id"`                   // 一支原队伍的唯一业务键。
	ReservationID string          `bson:"reservation_id"`        // 首次确认入场时生成的场次回执。
	RunID         uint64          `bson:"run_server_id"`         // 当前获准承载场次的运行服。
	RunEpoch      int64           `bson:"run_epoch"`             // 原运行服的授权代次。
	OwnerInstance string          `bson:"owner_instance"`        // 原游戏进程实例。
	Execution     uint64          `bson:"execution"`             // 场次执行代次。
	TemplateID    uint32          `bson:"template_id"`           // 原讨伐链模板编号。
	RuleVersion   uint64          `bson:"rule_version"`          // 原规则版本。
	ConfigHash    string          `bson:"config_hash"`           // 规则配置摘要。
	StartedAt     time.Time       `bson:"started_at"`            // 原权威入场时刻。
	ExpiresAt     time.Time       `bson:"expires_at"`            // 原战斗截止时刻。
	Members       []DungeonMember `bson:"members"`               // 仅包含实际扣次的真实队员。
	Outcome       DungeonOutcome  `bson:"outcome"`               // 首次确认的终态。
	FinishedAt    time.Time       `bson:"finished_at,omitempty"` // 原执行确认结果或合法回收的时刻。
	Refunded      bool            `bson:"refunded,omitempty"`    // 原系统中断退款已和终态同事务确认。
	Recovered     bool            `bson:"recovered,omitempty"`   // 中断由新运行服代次确认。
}

type dungeonRecord struct {
	DungeonChallenge `bson:",inline"`
	BeginDigest      string `bson:"begin_digest"`            // 原开始参数摘要，拒绝同编号换队或修改配置。
	FinishDigest     string `bson:"finish_digest,omitempty"` // 原终态提案摘要，拒绝同场再次改判。
}

func dungeonIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{{Keys: bson.D{{Key: "run_server_id", Value: 1}, {Key: "outcome", Value: 1}, {Key: "run_epoch", Value: 1}}}}
}

// dungeonProgress 兼容尚未运行讨伐链的旧角色，避免空指针或直接修改原聚合。
func dungeonProgress(role domain.Role) domain.DungeonProgress {
	if role.Dungeon == nil {
		return domain.DungeonProgress{}
	}
	return *role.Dungeon
}

// dungeonEntryLimit 仅使用事件时真实有效的托管卡，特权卡与此次数无关。
func dungeonEntryLimit(role domain.Role, at time.Time) uint32 {
	limit := uint32(devconfig.DungeonDailyEntries)
	if role.HostingCardUntil.After(at) {
		limit += devconfig.DungeonHostingBonusEntries
	}
	return limit
}

// normalizeDungeonBegin 固定原开战的摘要格式，正常开战和未知结果查证采用同一规范。
func normalizeDungeonBegin(q DungeonBegin) (DungeonBegin, error) {
	if !lowerHex(q.ChallengeID, 16) || !lowerHex(q.ConfigHash, 32) || q.Execution == 0 || q.Execution > math.MaxInt64 ||
		q.TemplateID == 0 || q.RuleVersion == 0 || q.RuleVersion > math.MaxInt64 || q.StartedAt.IsZero() || q.Duration <= 0 ||
		len(q.Members) == 0 || len(q.Members) > devconfig.DungeonMaxPlayers {
		return DungeonBegin{}, Fail(p.CodeInvalidArgument, "讨伐链原开始参数无效")
	}
	q.StartedAt = bossTime(q.StartedAt)
	q.Members = slices.Clone(q.Members)
	slices.SortFunc(q.Members, func(left, right DungeonAdmissionMember) int { return cmp.Compare(left.Auth.RoleID, right.Auth.RoleID) })
	for i, member := range q.Members {
		if !lowerHex(member.CombatSnapshot, 32) || member.ExpectedRevision <= 0 || i > 0 && member.Auth.RoleID == q.Members[i-1].Auth.RoleID {
			return DungeonBegin{}, Fail(p.CodeInvalidArgument, "讨伐链重复角色或入场快照无效")
		}
	}
	return q, nil
}

// BeginDungeon 短事务确认所有已准备角色的会话、修订和资格，然后一次扣完原队伍的所有次数。
func (s *Store) BeginDungeon(ctx context.Context, q DungeonBegin) (DungeonChallenge, []domain.Role, error) {
	q, err := normalizeDungeonBegin(q)
	if err != nil {
		return DungeonChallenge{}, nil, err
	}
	auths := make([]Auth, len(q.Members))
	for i, member := range q.Members {
		auths[i] = member.Auth
	}
	digest, err := bossDigest(q)
	if err != nil {
		return DungeonChallenge{}, nil, err
	}
	var result DungeonChallenge
	var roles []domain.Role
	err = s.dungeonPartyTx(ctx, auths, func(tc context.Context) error {
		var err error
		roles, err = s.dungeonRoles(tc, auths)
		if err != nil {
			return err
		}
		record, err := s.readDungeon(tc, q.ChallengeID)
		if err == nil {
			if !s.ownsDungeon(record) || record.BeginDigest != digest {
				return Fail(p.CodeOperationConflict, "讨伐链原开始请求不匹配")
			}
			result = record.DungeonChallenge
			return nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		cfg := &s.Cfg.Fixture
		template, enabled := cfg.DungeonTemplateByID(q.TemplateID)
		if !enabled {
			return Fail(p.CodeLineForbidden, "讨伐链模板尚未开放")
		}
		expires, now := bossTime(q.StartedAt.Add(q.Duration)), bossTime(time.Now())
		if q.ConfigHash != cfg.Hash || q.RuleVersion != cfg.RuleVersion || q.Duration != template.Duration || q.StartedAt.After(now) || !now.Before(expires) {
			return Fail(p.CodeOperationConflict, "讨伐链原版本、时长或启动时效已变更")
		}
		result = DungeonChallenge{ChallengeID: q.ChallengeID, ReservationID: domain.ID(), RunID: s.Cfg.RunID, RunEpoch: s.Epoch, OwnerInstance: s.Instance,
			Execution: q.Execution, TemplateID: q.TemplateID, RuleVersion: q.RuleVersion, ConfigHash: q.ConfigHash, StartedAt: q.StartedAt, ExpiresAt: expires}
		day := shanghaiOperatingDay(q.StartedAt)
		for i := range roles {
			member, err := chargeDungeonMember(&roles[i], q, q.Members[i], template.MinimumLevel, day)
			if err != nil {
				return err
			}
			result.Members = append(result.Members, member)
		}
		record = dungeonRecord{DungeonChallenge: result, BeginDigest: digest}
		if _, err = s.DB.Collection(dungeonCollection).InsertOne(tc, record); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return Fail(p.CodeOperationConflict, "讨伐链场次编号已存在")
			}
			return err
		}
		return s.writeDungeonRoles(tc, result, roles, "begin")
	})
	if err != nil {
		return DungeonChallenge{}, nil, err
	}
	return result, roles, nil
}

// chargeDungeonMember 检查原角色快照、独占挑战和原业务日后才扣掉一个入场名额。
func chargeDungeonMember(role *domain.Role, q DungeonBegin, member DungeonAdmissionMember, minimumLevel uint32, day string) (DungeonMember, error) {
	state := dungeonProgress(*role)
	limit := dungeonEntryLimit(*role, q.StartedAt)
	if role.Revision != member.ExpectedRevision || role.Revision == math.MaxInt64 || state.EntryDay > day {
		return DungeonMember{}, Fail(p.CodeOperationConflict, "讨伐链角色版本或原扣次日期已变化")
	}
	if role.Level < minimumLevel || state.ActiveChallengeID != "" || personalBossProgress(*role).ActiveChallengeID != "" {
		return DungeonMember{}, Fail(p.CodeLineForbidden, "讨伐链队员等级或独占挑战资格不足")
	}
	if state.EntryDay != day {
		state = domain.DungeonProgress{EntryDay: day}
	}
	if state.Entries >= limit {
		return DungeonMember{}, Fail(p.CodeLineForbidden, "讨伐链队员当日次数已用完")
	}
	state.Entries++
	state.ActiveChallengeID = q.ChallengeID
	role.Dungeon = &state
	role.Revision++
	return DungeonMember{RoleID: role.ID, AccountID: role.AccountID, Origin: role.Origin, CombatSnapshot: member.CombatSnapshot,
		EntryDay: day, EntriesAfter: state.Entries, DailyLimit: limit, HostingCardUntil: role.HostingCardUntil, SessionEpoch: role.SessionEpoch}, nil
}

// dungeonRoles 逐角色核实同一运行服当前会话后读取权威角色版本。
func (s *Store) dungeonRoles(ctx context.Context, auths []Auth) ([]domain.Role, error) {
	roles := make([]domain.Role, 0, len(auths))
	for _, auth := range auths {
		role, err := s.Role(ctx, auth.RoleID)
		if err != nil {
			return nil, err
		}
		if err = s.checkRole(role, auth); err != nil {
			return nil, err
		}
		if role.RunServerID != s.Cfg.RunID {
			return nil, Fail(p.CodeRouteChanged, "原讨伐链真实队员所属运行服不符")
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func (s *Store) readDungeon(ctx context.Context, id string) (dungeonRecord, error) {
	var record dungeonRecord
	err := s.DB.Collection(dungeonCollection).FindOne(ctx, bson.M{"_id": id}).Decode(&record)
	return record, err
}

func (s *Store) ownsDungeon(record dungeonRecord) bool {
	return record.RunID == s.Cfg.RunID && record.RunEpoch == s.Epoch && record.OwnerInstance == s.Instance
}

// writeDungeonRoles 同时写回角色版本与明确的来源证据，每个真实席位保留独立审计。
func (s *Store) writeDungeonRoles(ctx context.Context, challenge DungeonChallenge, roles []domain.Role, phase string) error {
	for i, role := range roles {
		if err := s.replace(ctx, role, role.Revision-1); err != nil {
			return err
		}
		_, err := s.DB.Collection("audit_outbox").InsertOne(ctx, bson.M{
			"_id": "dungeon:" + challenge.ChallengeID + ":" + phase + ":" + role.ID, "kind": "dungeon_" + phase,
			"role_id": role.ID, "revision": role.Revision, "challenge_id": challenge.ChallengeID, "reservation_id": challenge.ReservationID,
			"run_server_id": challenge.RunID, "outcome": challenge.Outcome, "refunded": challenge.Refunded, "entry": challenge.Members[i], "at": time.Now(),
		})
		if err != nil {
			return err
		}
	}
	return nil
}
