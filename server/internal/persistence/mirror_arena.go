// 本文件描述镜像竞技的永久试合资格、胜负回执和攻守快照边界。
package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	mirrorPoolCollection      = "mirror_arena_defense_pool"       // 当前赛季参与匹配的最新角色快照指针。
	mirrorDefenseCollection   = "mirror_arena_defenses"           // 历史防守版本连同原属性永久保留。
	mirrorDayCollection       = "mirror_arena_days"               // 原运营日候选及预占次数。
	mirrorChallengeCollection = "mirror_arena_challenges"         // 权威开始和结算回执永久保留。
	mirrorActiveCollection    = "mirror_arena_active"             // 任何赛季共用的角色独占占位。
	mirrorSeasonCollection    = "mirror_arena_scores"             // 原赛季独立进攻积分和连胜。
	MirrorPrepareWindow       = devconfig.MirrorArenaReadyTimeout // 原客户端首帧准备时间上限。
)

// MirrorOutcome 区分实际攻守结果与原执行明确中断。
type MirrorOutcome uint32

const (
	MirrorActive      MirrorOutcome = iota // 已预占且尚未获得唯一终态。
	MirrorWon                              // 原攻击者真实获胜。
	MirrorLost                             // 原攻击者真实失败。
	MirrorInterrupted                      // 系统放弃原执行并退回原预占。
)

// MirrorPublish 只接收原角色所有者组装的防守身份和属性版本。
type MirrorPublish struct {
	ExpectedRevision int64                       // 冻结属性时原角色的已确认修订。
	Snapshot         domain.MirrorCombatSnapshot // 已折算装备的战斗投影。
}

// MirrorDefense 是某一防守形象在原赛季的固定版本。
type MirrorDefense struct {
	ID           string                      `bson:"_id"`           // 原快照编号，候选始终引用此编号。
	RoleID       string                      `bson:"role_id"`       // 原防守角色永久身份。
	AccountID    string                      `bson:"account_id"`    // 用于排除同账号的角色。
	Origin       uint64                      `bson:"origin"`        // 原永久入口。
	RunID        uint64                      `bson:"run_server_id"` // 生成此防守镜像的运行服。
	SeasonStart  time.Time                   `bson:"season_start"`  // 配置明确的原赛季起点。
	Version      uint64                      `bson:"version"`       // 同角色同赛季的单调版本。
	RoleRevision int64                       `bson:"role_revision"` // 生成战斗属性时的角色修订。
	RuleVersion  uint64                      `bson:"rule_version"`  // 当前可解释快照的规则版本。
	ConfigHash   string                      `bson:"config_hash"`   // 同一版本的不可变配置摘要。
	Digest       string                      `bson:"digest"`        // 冻结战斗输入的摘要。
	Snapshot     domain.MirrorCombatSnapshot `bson:"snapshot"`      // 原固定战斗投影，不下发到候选列表。
}

// MirrorOffer 只给原候选持有者展示该对手和本日的不透明挑战凭证。
type MirrorOffer struct {
	ID             string `bson:"offer_id"`        // 原角色拥有的候选凭证，客户端不能自定对手。
	DefenderName   string `bson:"defender_name"`   // 界面展示的原防守角色名称。
	DefenderJob    uint32 `bson:"defender_job"`    // 原职业投影。
	DefenderLevel  uint32 `bson:"defender_level"`  // 原角色等级。
	DefenseVersion uint64 `bson:"defense_version"` // 本日这个候选固定的防守版本。
}

// MirrorSummary 为攻守双方展示原场次结果，积分始终是攻击者所得。
type MirrorSummary struct {
	ChallengeID    string        `bson:"_id"`                   // 该场的永久挑战标识。
	OfferID        string        `bson:"offer_id"`              // 原攻击者持有的候选凭证。
	AttackerRoleID string        `bson:"attacker_role_id"`      // 实际请求挑战的永久角色。
	DefenderRoleID string        `bson:"defender_role_id"`      // 原防守快照的永久角色。
	AttackerName   string        `bson:"attacker_name"`         // 入场时固定的攻方名称。
	DefenderName   string        `bson:"defender_name"`         // 原快照版本固定的守方名称。
	DefenseVersion uint64        `bson:"defense_version"`       // 该场冻结的防守快照版本。
	Outcome        MirrorOutcome `bson:"outcome"`               // 原战斗唯一终态。
	Points         int64         `bson:"points"`                // 原攻击方实际增加的积分。
	StartedAt      time.Time     `bson:"started_at"`            // 预占原运营日的服务端时刻。
	FinishedAt     time.Time     `bson:"finished_at,omitempty"` // 原执行器确认的结束时刻。
}

// MirrorChallenge 原场次唯一回执，双方投影仅用于同一原执行的内存启动。
type MirrorChallenge struct {
	MirrorSummary    `bson:",inline"`
	ReservationID    string                      `bson:"reservation_id"` // 首次预占的唯一执行证明。
	Execution        uint64                      `bson:"execution"`      // 原比赛执行代次。
	RunID            uint64                      `bson:"run_server_id"`  // 原运行服。
	TemplateID       uint32                      `bson:"template_id"`    // 原格战场景模板编号。
	RuleVersion      uint64                      `bson:"rule_version"`   // 通过后保留原规则语义。
	ConfigHash       string                      `bson:"config_hash"`    // 原版本摘要。
	AttackerSnapshot domain.MirrorCombatSnapshot `bson:"-"`              // 原攻方内存投影只保留持久摘要。
	DefenseSnapshot  domain.MirrorCombatSnapshot `bson:"-"`              // 从原不可变防守目录读取，不在各场次重复保存。
	ExpiresAt        time.Time                   `bson:"expires_at"`     // 准备和战斗可接受的最长结束时刻。
	SeasonStart      time.Time                   `bson:"season_start"`   // 该场只计入原赛季。
	SeasonEnd        time.Time                   `bson:"season_end"`     // 原赛季切换边界。
	Duration         time.Duration               `bson:"duration"`       // 首帧确认后原战斗允许的最大时长。
	SeasonPoints     int64                       `bson:"season_points"`  // 本回执中原攻方的已确认赛季总分。
	Streak           uint32                      `bson:"streak"`         // 本回执中的原攻方进攻连胜。
}

// MirrorInfo 汇总当前角色的原运营日、必要候选和有限战报窗口。
type MirrorInfo struct {
	SeasonStart     time.Time        // 当前赛季起点。
	SeasonEnd       time.Time        // 当前赛季结束边界。
	DailyLimit      uint32           // 原配置中每日允许的次数。
	EntriesUsed     uint32           // 原运营日的已完成或预占次数。
	SeasonPoints    int64            // 原角色当前赛季已确认分数。
	Streak          uint32           // 原角色当前进攻连胜。
	DefenseVersion  uint64           // 当前赛季已发布且规则相符的防守版本。
	Offers          []MirrorOffer    // 仅当前原角色可选的已发布候选。
	AttackHistory   []MirrorSummary  // 最新的原进攻结果。
	DefenseHistory  []MirrorSummary  // 最新的原防守结果。
	ActiveChallenge *MirrorChallenge // 可用于同一原执行器对账，客户端不接收快照数据。
}

// MirrorBegin 绑定不透明候选和原快照，重复发起使用相同挑战编号和内容。
type MirrorBegin struct {
	ChallengeID      string                      // 原操作分配的全局唯一场次号。
	OfferID          string                      // 此角色本日已由服务端分配的候选。
	Execution        uint64                      // 原游戏场景执行代次。
	ExpectedRevision int64                       // 冻结原攻方属性时的角色修订。
	AttackerSnapshot domain.MirrorCombatSnapshot // 由攻击者的同一原执行器冻结。
}

// MirrorFinish 确认实际比赛结果或系统中断，仅由原执行器调用。
type MirrorFinish struct {
	ChallengeID   string        // 原请求使用的永久场次标识。
	ReservationID string        // 原预占回执的私有标识。
	Execution     uint64        // 原场次执行代次。
	Outcome       MirrorOutcome // 胜、负或系统中断。
	FinishedAt    time.Time     // 执行器确认终态的服务端时刻。
}

// MirrorResume 只将仍健康的原现场移交给已验证的新会话。
type MirrorResume struct {
	ChallengeID          string // 原场次编号。
	ReservationID        string // 原预占回执。
	Execution            uint64 // 原执行代次保持不变。
	PreviousSessionEpoch int64  // 原获准提交结果的会话代次。
}

type mirrorRecord struct {
	MirrorChallenge      `bson:",inline"`
	Origin               uint64 `bson:"origin"`                           // 原两角色所属永久入口。
	OwnerInstance        string `bson:"owner_instance"`                   // 唯一允许接回和直接提交终态的原进程。
	RunEpoch             int64  `bson:"run_epoch"`                        // 旧进程失权后只能由新执行作废回收。
	SessionEpoch         int64  `bson:"session_epoch"`                    // 当前健康现场绑定的会话。
	PreviousSessionEpoch int64  `bson:"previous_session_epoch,omitempty"` // 健康接回幂等依据。
	BeginDigest          string `bson:"begin_digest"`                     // 原预占请求的不可变摘要。
	FinishDigest         string `bson:"finish_digest,omitempty"`          // 相同终态重放时核验内容。
	DayKey               string `bson:"day_key"`                          // 跨日回收时准确定位原预占。
	ScoreKey             string `bson:"score_key"`                        // 隔离原赛季积分空间。
	DefenseID            string `bson:"defense_id"`                       // 原候选对应的不可变防守快照。
	WinPoints            int64  `bson:"win_points"`                       // 预占时按原连胜规则固定的获胜分。
	LossPoints           int64  `bson:"loss_points"`                      // 预占时固定的失败分。
}

type mirrorActive struct {
	ID          string `bson:"_id"`          // 赛季和运行服变更时仍对应同一永久角色。
	ChallengeID string `bson:"challenge_id"` // 当前有权保留预占的活动场次。
}

type mirrorSeasonScore struct {
	ID          string    `bson:"_id"`           // 运行服、入口、赛季和永久角色组成的键。
	Scope       string    `bson:"scope"`         // 原赛季排名的分区。
	RoleID      string    `bson:"role_id"`       // 参赛角色永久编号。
	Points      int64     `bson:"points"`        // 原赛季所有已确认攻方增加的分数。
	Streak      uint32    `bson:"streak"`        // 原攻方最近的实战连胜。
	Matches     uint32    `bson:"matches"`       // 已结算的有效进攻次数。
	LastMatchAt time.Time `bson:"last_match_at"` // 赛季同分时较早完成最近挑战者优先。
}

func mirrorRoleKey(origin uint64, roleID string) string {
	return fmt.Sprintf("%d:%s", origin, roleID)
}

func mirrorScope(run, origin uint64, season time.Time) string {
	return fmt.Sprintf("%d:%d:%d", run, origin, season.Unix())
}

func mirrorDayKey(scope, roleID string, now time.Time) string {
	return scope + ":" + roleID + ":" + shanghaiOperatingDay(now)
}

// mirrorOptional 保留数据库失败，仅把已确认无记录解释为初始状态。
func mirrorOptional[T any](ctx context.Context, collection *mongo.Collection, id string) (T, error) {
	var value T
	err := collection.FindOne(ctx, bson.M{"_id": id}).Decode(&value)
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = nil
	}
	return value, err
}

func (s *Store) mirrorRole(ctx context.Context, a Auth) (domain.Role, error) {
	if err := s.fence(ctx, a); err != nil {
		return domain.Role{}, err
	}
	r, err := s.Role(ctx, a.RoleID)
	if err != nil {
		return r, err
	}
	if r.Origin == 0 || r.RunServerID != s.Cfg.RunID {
		return r, Fail(p.CodeRouteChanged, "镜像挑战的原入口服或运行服不符")
	}
	return r, s.checkRole(r, a)
}

// checkMirrorRoleFree 与原角色的其它独占预约在同一账号写围栏内竞争，避免跨挑战重复入场。
func (s *Store) checkMirrorRoleFree(ctx context.Context, r domain.Role) error {
	active, err := mirrorOptional[mirrorActive](ctx, s.DB.Collection(mirrorActiveCollection), mirrorRoleKey(r.Origin, r.ID))
	if err == nil && active.ChallengeID != "" {
		return Fail(p.CodeLineForbidden, "该角色已有尚未结算的镜像挑战")
	}
	return err
}

func (s *Store) currentMirrorSeason(now time.Time) (*devconfig.MirrorArenaConfig, time.Time, time.Time, error) {
	cfg := s.Cfg.Fixture.MirrorArena
	if cfg != nil {
		begin, end, ok := cfg.SeasonAt(now)
		if ok {
			return cfg, begin, end, nil
		}
	}
	return nil, time.Time{}, time.Time{}, Fail(p.CodeLineForbidden, "镜像竞技赛季尚未开放")
}
