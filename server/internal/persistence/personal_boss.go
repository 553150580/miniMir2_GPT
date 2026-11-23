// 本文件由角色所有者事务确认个人首领预约、结算与失去原场景后的取消。
package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"math"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	personalBossCollection = "personal_boss_challenges" // 永久预约和终态记录的集合。
)

// PersonalBossOutcome 区分正在进行、真实击杀、逃走、中断与玩家阵亡。
type PersonalBossOutcome uint8

const (
	PersonalBossActive         PersonalBossOutcome = iota // 仍在进行，尚无永久终态。
	PersonalBossDefeated                                  // 战斗确认首领死亡。
	PersonalBossEscaped                                   // 时限到达，首领尚未死亡。
	PersonalBossInterrupted                               // 玩家退出、场景中断或原执行失权。
	PersonalBossPlayerDefeated                            // 战斗先确认玩家阵亡。
)

// PersonalBossBegin 携带原角色执行器冻结的入场属性与配置身份。
type PersonalBossBegin struct {
	ChallengeID      string        // 角色所有者分配的稳定唯一编号，重复提交时沿用。
	Execution        uint64        // 请求启动的个人场景执行代次。
	TemplateID       uint32        // 挑战读取的首领模板编号。
	CombatSnapshot   string        // 固定入场属性、装备和技能快照的 SHA-256。
	ExpectedRevision int64         // 冻结战斗快照时的角色版本。
	RuleVersion      uint64        // 请求采用的规则版本。
	ConfigHash       string        // 规则和首领模板采用的配置摘要。
	StartedAt        time.Time     // 服务端冻结的原入场时间。
	Duration         time.Duration // 首领仍存活时允许战斗的时间。
	SuccessCooldown  time.Duration // 成功后该模板的冷却时长。
	RetryCooldown    time.Duration // 未成功后该模板的重试冷却。
	BossHP           int64         // 原挑战具有的首领生命值。
}

// PersonalBossFinish 携带原挑战执行器冻结的终态和有效伤害证明。
type PersonalBossFinish struct {
	ChallengeID   string              // 被结算的原挑战编号。
	ReservationID string              // 原事务确认的角色入场回执编号。
	Execution     uint64              // 确认终态的挑战执行代次。
	RuleVersion   uint64              // 原预约固定的规则版本。
	ConfigHash    string              // 原预约固定的配置摘要。
	Outcome       PersonalBossOutcome // 本场权威首领击杀或未成功原因。
	FinishedAt    time.Time           // 原执行器记录的胜负时间，决定成功日和冷却。
	Damage        int64               // 该角色与其召唤物累计确认扣掉的首领生命。
	HPRemaining   int64               // 冻结首领终态时剩余的权威生命。
}

// PersonalBossChallenge 是永久预约与最新唯一终态，不持久化临时地图或战斗对象。
type PersonalBossChallenge struct {
	ChallengeID     string              `bson:"_id"`                    // 永久场次编号。
	ReservationID   string              `bson:"reservation_id"`         // 初次预约事务生成的回执身份。
	RoleID          string              `bson:"role_id"`                // 拥有该独享挑战的角色。
	RunID           uint64              `bson:"run_server_id"`          // 获准执行的运行服。
	Execution       uint64              `bson:"execution"`              // 场景执行代次。
	RuleVersion     uint64              `bson:"rule_version"`           // 冻结的首领与战斗规则版本。
	ConfigHash      string              `bson:"config_hash"`            // 原配置的不可变哈希。
	TemplateID      uint32              `bson:"template_id"`            // 同模板复战冷却对应的编号。
	CombatSnapshot  string              `bson:"combat_snapshot"`        // 原入场属性、装备及技能快照引用。
	StartedAt       time.Time           `bson:"started_at"`             // 首次预约时采用的场景开始时间。
	ExpiresAt       time.Time           `bson:"expires_at"`             // 场景时钟上的首领逃走时间。
	BossHP          int64               `bson:"boss_hp"`                // 原入场模板的完整首领生命。
	DailyLimit      uint32              `bson:"daily_limit"`            // 此次预约冻结的特权成功次数上限。
	SuccessCooldown time.Duration       `bson:"success_cooldown"`       // 原规则成功时的模板冷却。
	RetryCooldown   time.Duration       `bson:"retry_cooldown"`         // 原规则未成功时的模板冷却。
	Outcome         PersonalBossOutcome `bson:"outcome"`                // 是否确认唯一终态。
	FinishedAt      time.Time           `bson:"finished_at,omitempty"`  // 原事件实际结束时间。
	AvailableAt     time.Time           `bson:"available_at,omitempty"` // 该模板原冷却的明确终点。
	Damage          int64               `bson:"damage,omitempty"`       // 原执行确认的累计有效伤害。
	HPRemaining     int64               `bson:"hp_remaining,omitempty"` // 原执行确认的首领剩余生命。
	SuccessDay      string              `bson:"success_day,omitempty"`  // 成功归属的上海运营日。
	Successes       uint32              `bson:"successes,omitempty"`    // 该运营日确认后的成功次数。
	Recovered       bool                `bson:"recovered,omitempty"`    // 新所有者取消了失去原执行身份的挑战。
	RewardGold      int64               `bson:"reward_gold,omitempty"`  // 此次击杀实际入账的直接金币。
	RewardBound     bool                `bson:"reward_bound,omitempty"` // 该金币依据原击杀时刻进入绑定账户。
}

// personalBossRecord 额外保存仅持久层使用的所有者围栏和幂等指纹。
type personalBossRecord struct {
	PersonalBossChallenge `bson:",inline"`
	OwnerInstance         string `bson:"owner_instance"`          // 初次预约的游戏节点进程实例。
	RunEpoch              int64  `bson:"run_epoch"`               // 初次预约的运行服所有权代次。
	SessionEpoch          int64  `bson:"session_epoch"`           // 初次预约的角色会话代次。
	BeginDigest           string `bson:"begin_digest"`            // 初次预约内容摘要。
	FinishDigest          string `bson:"finish_digest,omitempty"` // 首次确认的结束提案内容摘要。

	KillRewardGold int64     `bson:"kill_reward_gold,omitempty"`         // 预约时固化的独立金币奖励，旧场次默认为零。
	PrivilegeUntil time.Time `bson:"privilege_until_at_entry,omitempty"` // 原角色授权的卡到期时刻，当前挑战不受理卡资产操作。
}

// BeginPersonalBoss 在角色 owner 的事务内确认独占场次，返回的活动预约才能启动新的场景。
func (s *Store) BeginPersonalBoss(ctx context.Context, a Auth, q PersonalBossBegin) (domain.Role, PersonalBossChallenge, error) {
	var out domain.Role
	var grant PersonalBossChallenge
	if !lowerHex(q.ChallengeID, 16) || q.Execution == 0 || q.Execution > math.MaxInt64 || q.TemplateID == 0 ||
		!lowerHex(q.CombatSnapshot, 32) || q.ExpectedRevision <= 0 || q.StartedAt.IsZero() {
		return out, grant, Fail(p.CodeInvalidArgument, "个人首领预约身份或快照无效")
	}
	q.StartedAt = bossTime(q.StartedAt)
	now := bossTime(time.Now())
	digest, err := bossDigest(q)
	if err != nil {
		return out, grant, Fail(p.CodeInvalidArgument, "个人首领预约无法编码")
	}
	err = s.guardedTx(ctx, a, func(tc context.Context) error {
		grant = PersonalBossChallenge{}
		r, err := s.personalBossRole(tc, a)
		if err != nil {
			return err
		}
		out = r
		var old personalBossRecord
		err = s.DB.Collection(personalBossCollection).FindOne(tc, bson.M{"_id": q.ChallengeID}).Decode(&old)
		if err == nil {
			if !s.personalBossOwned(old, r) || old.BeginDigest != digest {
				return Fail(p.CodeOperationConflict, "原个人首领预约身份或内容不符")
			}
			grant = old.PersonalBossChallenge
			return nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		cfg := &s.Cfg.Fixture
		if !cfg.ValidPersonalBoss() || cfg.RuleVersion == 0 || !lowerHex(cfg.Hash, 32) {
			return Fail(p.CodeLineForbidden, "个人首领模板尚未开放")
		}
		template := cfg.PersonalBoss
		if q.RuleVersion != cfg.RuleVersion || q.ConfigHash != cfg.Hash || q.TemplateID != template.TemplateID || q.BossHP != template.Monster.HP ||
			q.Duration != template.Duration || q.SuccessCooldown != template.SuccessCooldown || q.RetryCooldown != template.RetryCooldown ||
			q.ExpectedRevision != r.Revision || r.Revision == math.MaxInt64 {
			return Fail(p.CodeOperationConflict, "个人首领配置、角色或战斗快照版本变化")
		}
		expires := bossTime(q.StartedAt.Add(q.Duration))
		if q.StartedAt.After(now) || !now.Before(expires) {
			return Fail(p.CodeOperationConflict, "原个人首领预约已错过启动窗口")
		}
		state := personalBossProgress(r)
		availability := personalBossAvailability(r, state, template, q.StartedAt)
		if err := personalBossAdmissionError(availability.EligibilityReason); err != nil {
			return err
		}
		// 回执编号和所有者代次同源冻结，不能由其他角色或新执行接管旧预约。
		grant = PersonalBossChallenge{ChallengeID: q.ChallengeID, ReservationID: domain.ID(), RoleID: r.ID,
			RunID: s.Cfg.RunID, Execution: q.Execution, TemplateID: q.TemplateID, RuleVersion: q.RuleVersion,
			ConfigHash: q.ConfigHash, CombatSnapshot: q.CombatSnapshot, StartedAt: q.StartedAt, ExpiresAt: expires,
			BossHP: q.BossHP, DailyLimit: availability.DailyLimit, SuccessCooldown: q.SuccessCooldown, RetryCooldown: q.RetryCooldown}
		record := personalBossRecord{PersonalBossChallenge: grant, OwnerInstance: s.Instance, BeginDigest: digest, RunEpoch: s.Epoch, SessionEpoch: r.SessionEpoch,
			KillRewardGold: template.KillRewardGold, PrivilegeUntil: r.PrivilegeUntil}
		state.ActiveChallengeID = grant.ChallengeID
		r.PersonalBoss = &state
		r.Revision++
		if err := s.replace(tc, r, out.Revision); err != nil {
			return err
		}
		if _, err := s.DB.Collection(personalBossCollection).InsertOne(tc, record); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return Fail(p.CodeOperationConflict, "个人首领挑战编号已占用")
			}
			return err
		}
		if err := s.writePersonalBossAudit(tc, record, "begin", r.Revision); err != nil {
			return err
		}
		out = r
		return nil
	})
	return out, grant, err
}

// FinishPersonalBoss 只接受原执行的一个结果，重复提交同一结果重放原回执。
func (s *Store) FinishPersonalBoss(ctx context.Context, a Auth, q PersonalBossFinish) (domain.Role, PersonalBossChallenge, error) {
	var out domain.Role
	var grant PersonalBossChallenge
	if !lowerHex(q.ChallengeID, 16) || !lowerHex(q.ReservationID, 16) || q.Execution == 0 || q.Outcome < PersonalBossDefeated ||
		q.Outcome > PersonalBossPlayerDefeated || q.FinishedAt.IsZero() || q.Damage < 0 || q.HPRemaining < 0 {
		return out, grant, Fail(p.CodeInvalidArgument, "个人首领结束提案无效")
	}
	q.FinishedAt = bossTime(q.FinishedAt)
	now := bossTime(time.Now())
	digest, err := bossDigest(q)
	if err != nil {
		return out, grant, Fail(p.CodeInvalidArgument, "个人首领结束提案无法编码")
	}
	err = s.guardedTx(ctx, a, func(tc context.Context) error {
		grant = PersonalBossChallenge{}
		r, err := s.personalBossRole(tc, a)
		if err != nil {
			return err
		}
		out = r
		old, err := s.readPersonalBoss(tc, q.ChallengeID)
		if err != nil {
			return err
		}
		if !s.personalBossOwned(old, r) || old.ReservationID != q.ReservationID || old.Execution != q.Execution ||
			old.RuleVersion != q.RuleVersion || old.ConfigHash != q.ConfigHash {
			return Fail(p.CodeOperationConflict, "个人首领原执行、预约或配置身份不符")
		}
		grant = old.PersonalBossChallenge
		if old.Outcome != PersonalBossActive {
			if old.FinishDigest != digest {
				return Fail(p.CodeOperationConflict, "原个人首领场次已有不同终态")
			}
			return nil
		}
		if !validPersonalBossFinish(old, q, now) {
			return Fail(p.CodeInvalidArgument, "个人首领结果与原预约战斗或时限不符")
		}
		old.Outcome, old.FinishedAt, old.Damage, old.HPRemaining, old.FinishDigest = q.Outcome, q.FinishedAt, q.Damage, q.HPRemaining, digest
		next, err := s.closePersonalBoss(tc, r, &old)
		if err != nil {
			return err
		}
		out, grant = next, old.PersonalBossChallenge
		return nil
	})
	return out, grant, err
}

// RecoverPersonalBoss 用新所有者的当前围栏取消已失去原执行身份的活动挑战。
func (s *Store) RecoverPersonalBoss(ctx context.Context, a Auth) (domain.Role, PersonalBossChallenge, error) {
	var out domain.Role
	var grant PersonalBossChallenge
	now := bossTime(time.Now())
	err := s.guardedTx(ctx, a, func(tc context.Context) error {
		grant = PersonalBossChallenge{}
		r, err := s.personalBossRole(tc, a)
		if err != nil {
			return err
		}
		out = r
		id := personalBossProgress(r).ActiveChallengeID
		if id == "" {
			return nil
		}
		old, err := s.readPersonalBoss(tc, id)
		if err != nil {
			return err
		}
		if old.RoleID != r.ID || old.Outcome != PersonalBossActive || now.Before(old.StartedAt) {
			return Fail(p.CodeOperationConflict, "失效的个人首领预约与角色记录不符")
		}
		if s.personalBossOwned(old, r) {
			return Fail(p.CodeOperationConflict, "仍为当前执行的个人首领预约")
		}
		// 原运行已失权，无法认定其临时击杀进度；原次数仍保留供重试。
		old.Outcome, old.FinishedAt, old.Recovered = PersonalBossInterrupted, now, true
		next, err := s.closePersonalBoss(tc, r, &old)
		if err != nil {
			return err
		}
		out, grant = next, old.PersonalBossChallenge
		return nil
	})
	return out, grant, err
}

// closePersonalBoss 将冷却、次数与首次永久终态同批提交，不借用普通挂机收益的来源编号。
func (s *Store) closePersonalBoss(ctx context.Context, r domain.Role, record *personalBossRecord) (domain.Role, error) {
	state := personalBossProgress(r)
	if state.ActiveChallengeID != record.ChallengeID || r.Revision == math.MaxInt64 {
		return r, Fail(p.CodeOperationConflict, "个人首领活动预约或角色版本变化")
	}
	cooldown := record.RetryCooldown
	if record.Outcome == PersonalBossDefeated {
		day := shanghaiOperatingDay(record.FinishedAt)
		if state.SuccessDay > day || state.SuccessDay == day && state.Successes >= record.DailyLimit {
			return r, Fail(p.CodeOperationConflict, "个人首领成功次数与原预约不符")
		}
		if state.SuccessDay != day {
			state.SuccessDay, state.Successes = day, 0
		}
		state.Successes++
		record.SuccessDay, record.Successes = day, state.Successes
		cooldown = record.SuccessCooldown
		if err := applyPersonalBossGold(&r, record); err != nil {
			return r, err
		}
	}
	record.AvailableAt = bossTime(record.FinishedAt.Add(cooldown))
	state.Cooldowns = maps.Clone(state.Cooldowns)
	if state.Cooldowns == nil {
		state.Cooldowns = make(map[string]time.Time)
	}
	state.Cooldowns[bossTemplateKey(record.TemplateID)] = record.AvailableAt
	state.ActiveChallengeID = ""
	r.PersonalBoss = &state
	r.Revision++
	if err := s.replace(ctx, r, r.Revision-1); err != nil {
		return r, err
	}
	result, err := s.DB.Collection(personalBossCollection).ReplaceOne(ctx, bson.M{"_id": record.ChallengeID, "outcome": PersonalBossActive}, record)
	if err != nil {
		return r, err
	}
	if result.MatchedCount != 1 {
		return r, Fail(p.CodeOperationConflict, "个人首领场次已有终态")
	}
	return r, s.writePersonalBossAudit(ctx, *record, "finish", r.Revision)
}

// applyPersonalBossGold 将独立金币在原击杀时刻判绑，结果和永久胜利一起提交。
func applyPersonalBossGold(r *domain.Role, record *personalBossRecord) error {
	if record.KillRewardGold == 0 {
		return nil
	}
	bound := !record.PrivilegeUntil.After(record.FinishedAt)
	wallet := &r.Wallet.Gold
	if bound {
		wallet = &r.Wallet.BoundGold
	}
	total, err := add(*wallet, record.KillRewardGold)
	if err != nil {
		return err
	}
	*wallet = total
	record.RewardGold, record.RewardBound = record.KillRewardGold, bound
	return nil
}

// personalBossRole 同时以运行服和账号围栏保护永久角色所有者。
func (s *Store) personalBossRole(ctx context.Context, a Auth) (domain.Role, error) {
	if err := s.fence(ctx, a); err != nil {
		return domain.Role{}, err
	}
	r, err := s.Role(ctx, a.RoleID)
	if err != nil {
		return r, err
	}
	return r, s.checkRole(r, a)
}

// personalBossOwned 要求挑战仍属于预约时的运行服、节点实例、角色和会话。
func (s *Store) personalBossOwned(record personalBossRecord, r domain.Role) bool {
	return record.RoleID == r.ID && record.RunID == s.Cfg.RunID && record.RunEpoch == s.Epoch &&
		record.OwnerInstance == s.Instance && record.SessionEpoch == r.SessionEpoch
}

// readPersonalBoss 查询永久场次并按业务错误区分未知预约。
func (s *Store) readPersonalBoss(ctx context.Context, id string) (personalBossRecord, error) {
	var record personalBossRecord
	err := s.DB.Collection(personalBossCollection).FindOne(ctx, bson.M{"_id": id}).Decode(&record)
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = Fail(p.CodeOperationConflict, "缺少个人首领预约")
	}
	return record, err
}

// writePersonalBossAudit 将原回执和当前角色版本写入同一事务的审计。
func (s *Store) writePersonalBossAudit(ctx context.Context, r personalBossRecord, phase string, revision int64) error {
	_, err := s.DB.Collection("audit_outbox").InsertOne(ctx, bson.M{"_id": "personal_boss:" + r.ChallengeID + ":" + phase,
		"kind": "personal_boss_" + phase, "role_id": r.RoleID, "revision": revision, "challenge": r.PersonalBossChallenge, "at": time.Now()})
	return err
}

// personalBossProgress 读取角色原状态，兼容未有挑战数据的旧角色。
func personalBossProgress(r domain.Role) domain.PersonalBossProgress {
	if r.PersonalBoss == nil {
		return domain.PersonalBossProgress{}
	}
	return *r.PersonalBoss
}

// validPersonalBossFinish 核对原窗口、权威生命守恒，以及到期与击杀的时间边界。
func validPersonalBossFinish(r personalBossRecord, q PersonalBossFinish, now time.Time) bool {
	if q.FinishedAt.Before(r.StartedAt) || q.FinishedAt.After(now) || q.HPRemaining > r.BossHP || q.Damage != r.BossHP-q.HPRemaining {
		return false
	}
	if q.Outcome == PersonalBossEscaped {
		return q.FinishedAt.Equal(r.ExpiresAt) && q.HPRemaining > 0
	}
	return q.FinishedAt.Before(r.ExpiresAt) && (q.Outcome == PersonalBossDefeated) == (q.HPRemaining == 0)
}

// bossTime 使用 MongoDB 原生毫秒精度，确保跨驱动重试比较相同事件时间。
func bossTime(at time.Time) time.Time { return at.UTC().Truncate(time.Millisecond) }

// bossDigest 为仅含固定类型的预约或结果参数计算稳定指纹。
func bossDigest(v any) (string, error) {
	raw, err := json.Marshal(v)
	return Hash(raw), err
}

// bossTemplateKey 将模板编号编码为 MongoDB 可存储的冷却键。
func bossTemplateKey(id uint32) string { return strconv.FormatUint(uint64(id), 10) }
