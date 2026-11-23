// 本文件查询角色所有者已确认的个人首领资格、活动预约和最近历史。
package persistence

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	PersonalBossEligible       uint32 = iota // 资格、次数和冷却允许进入。
	PersonalBossUnavailable                  // 未启用个人首领模板。
	PersonalBossLevelRequired                // 角色尚未达到配置最低等级。
	PersonalBossAlreadyActive                // 角色仍有活动场次需要完成或恢复。
	PersonalBossCoolingDown                  // 当前模板还在冷却。
	PersonalBossDailyCap                     // 当前运营日的成功次数已经用完。
	PersonalBossSceneForbidden               // 角色当前场景或状态不接受新的预约。
)

// PersonalBossInfo 是一次只读快照中的模板、角色资格及历史记录。
type PersonalBossInfo struct {
	Open              bool                    // 是否存在已开放的个人首领模板。
	TemplateID        uint32                  // 当前允许进入的模板编号。
	DisplayName       string                  // 发布配置中供界面显示的名称。
	MapID             uint64                  // 专属于此模板的挑战地图编号。
	MonsterID         uint32                  // 模板实际使用的怪物来源编号。
	MinimumLevel      uint32                  // 资格所需的最低等级。
	Day               string                  // 当前资格对应的上海运营日。
	Successes         uint32                  // 当日已经确认的成功次数。
	DailyLimit        uint32                  // 当前角色按特权结算的当日上限。
	EligibilityReason uint32                  // 当前不能进入的状态；零允许入场。
	AvailableAt       time.Time               // 当前模板原冷却截止时间。
	ActiveChallengeID string                  // 角色尚未终结的原预约身份。
	ActiveExpiresAt   time.Time               // 该预约使用的原场景时限。
	History           []PersonalBossChallenge // 角色最近已终结场次，按结束时间倒序。
}

// personalBossAvailability 让目录查询和预约事务采用同一等级、次数与冷却规则。
func personalBossAvailability(r domain.Role, state domain.PersonalBossProgress, cfg *devconfig.PersonalBossConfig, at time.Time) PersonalBossInfo {
	day := shanghaiOperatingDay(at)
	out := PersonalBossInfo{Day: day, ActiveChallengeID: state.ActiveChallengeID, EligibilityReason: PersonalBossUnavailable}
	if cfg == nil {
		return out
	}
	out.Open, out.TemplateID, out.DisplayName = true, cfg.TemplateID, cfg.DisplayName
	out.MapID, out.MonsterID, out.MinimumLevel = cfg.Map.ID, cfg.Monster.ID, cfg.MinimumLevel
	out.DailyLimit = cfg.DailySuccessLimit
	if r.PrivilegeUntil.After(at) {
		out.DailyLimit += cfg.PrivilegeSuccessBonus
	}
	if state.SuccessDay == day {
		out.Successes = state.Successes
	}
	out.AvailableAt = state.Cooldowns[bossTemplateKey(cfg.TemplateID)]
	switch {
	case state.ActiveChallengeID != "":
		out.EligibilityReason = PersonalBossAlreadyActive
	case r.Level < cfg.MinimumLevel:
		out.EligibilityReason = PersonalBossLevelRequired
	case out.AvailableAt.After(at):
		out.EligibilityReason = PersonalBossCoolingDown
	case state.SuccessDay > day || out.Successes >= out.DailyLimit:
		out.EligibilityReason = PersonalBossDailyCap
	default:
		out.EligibilityReason = PersonalBossEligible
	}
	return out
}

// personalBossAdmissionError 将目录资格映射到事务的确定拒绝。
func personalBossAdmissionError(reason uint32) error {
	switch reason {
	case PersonalBossEligible:
		return nil
	case PersonalBossLevelRequired:
		return Fail(p.CodeLineForbidden, "角色未达到个人首领所需等级")
	case PersonalBossAlreadyActive:
		return Fail(p.CodeLineForbidden, "已有个人首领场次进行中")
	case PersonalBossCoolingDown:
		return Fail(p.CodeLineForbidden, "个人首领模板仍在冷却")
	case PersonalBossDailyCap:
		return Fail(p.CodeLineForbidden, "个人首领已达到本日成功次数上限")
	default:
		return Fail(p.CodeLineForbidden, "个人首领模板尚未开放")
	}
}

// QueryPersonalBoss 以当前账号、运行服和角色会话的同一个读快照获取数据。
func (s *Store) QueryPersonalBoss(ctx context.Context, a Auth) (PersonalBossInfo, error) {
	var out PersonalBossInfo
	now := bossTime(time.Now())
	err := s.tx(ctx, func(tc context.Context) error {
		// 复用活动租约检查。这里只构造条件，不执行写围栏或改变角色版本。
		if _, err := s.fenceWrites(tc, a); err != nil {
			return err
		}
		if err := s.DB.Collection("accounts").FindOne(tc, accountGuardFilter(a), options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); err != nil {
			return authorityReadError(err, p.CodeUnauthenticated, "账号授权失效")
		}
		r, err := s.Role(tc, a.RoleID)
		if err != nil {
			return err
		}
		if err = s.checkRole(r, a); err != nil {
			return err
		}
		cfg := &s.Cfg.Fixture
		template := cfg.PersonalBoss
		if !cfg.ValidPersonalBoss() {
			template = nil
		}
		state := personalBossProgress(r)
		out = personalBossAvailability(r, state, template, now)
		if state.ActiveChallengeID != "" {
			record, err := s.readPersonalBoss(tc, state.ActiveChallengeID)
			if err != nil {
				return err
			}
			if record.RoleID != r.ID || record.Outcome != PersonalBossActive {
				return Fail(p.CodeOperationConflict, "活动个人首领预约失效")
			}
			out.ActiveExpiresAt = record.ExpiresAt
		}
		if template == nil {
			return nil
		}
		cursor, err := s.DB.Collection(personalBossCollection).Find(tc, bson.M{"role_id": r.ID, "finished_at": bson.M{"$type": "date"}},
			options.Find().SetSort(bson.D{{Key: "finished_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(template.HistoryLimit)))
		if err != nil {
			return err
		}
		return cursor.All(tc, &out.History)
	})
	return out, err
}
