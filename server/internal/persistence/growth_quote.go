// 本文件统一角色养成报价的附着身份、规则和接受时限。
package persistence

import (
	"strconv"
	"time"

	"minimir/internal/domain"
)

const (
	growthQuoteDuration = 30 * time.Second // 展示后允许首次受理的时间。
	growthQuoteGrace    = 5 * time.Minute  // 排队或事务重试后清理过期记录。
)

// growthQuoteBinding 固定报价所属角色、原界面、原运行服及原角色修订。
type growthQuoteBinding struct {
	RoleID       string    `bson:"_id"`           // 角色永久编号。
	ID           string    `bson:"quote_id"`      // 本次报价编号。
	Scene        string    `bson:"scene_epoch"`   // 原场景，保留完整64位。
	UISession    string    `bson:"ui_session_id"` // 原界面会话。
	SessionEpoch int64     `bson:"session_epoch"` // 签发时的登录代次。
	RunID        uint64    `bson:"run_server_id"` // 签发报价的运行服。
	RunEpoch     int64     `bson:"run_epoch"`     // 原运行服所有权代次。
	Revision     int64     `bson:"revision"`      // 签发时的角色修订。
	Rule         uint64    `bson:"rule_version"`  // 签发时的规则版本。
	Config       string    `bson:"config_hash"`   // 规则精确字节摘要。
	Issued       time.Time `bson:"issued_at"`     // 原报价签发时刻。
	Expires      time.Time `bson:"expires_at"`    // 首次确认排他终点。
	PurgeAt      time.Time `bson:"purge_at"`      // 数据库可删除时刻。
}

// newGrowthQuote 用同一个时钟和运行服上下文生成报价身份。
func (s *Store) newGrowthQuote(auth Auth, scene uint64, ui []byte) growthQuoteBinding {
	f := &s.Cfg.Fixture
	issued := bossTime(time.Now())
	return growthQuoteBinding{RoleID: auth.RoleID, ID: domain.ID(), Scene: strconv.FormatUint(scene, 10),
		UISession: domain.Hex(ui), SessionEpoch: auth.SessionEpoch, RunID: s.Cfg.RunID, RunEpoch: s.Epoch,
		Rule: f.RuleVersion, Config: f.Hash, Issued: issued, Expires: issued.Add(growthQuoteDuration),
		PurgeAt: issued.Add(growthQuoteDuration + growthQuoteGrace)}
}

// matchesGrowthQuote 核对首次受理的原附着。已成交回执在事务入口优先恢复。
func (s *Store) matchesGrowthQuote(q growthQuoteBinding, role domain.Role, auth Auth, revision, scene uint64, ui []byte, at time.Time) bool {
	f := &s.Cfg.Fixture
	return q.RoleID == role.ID && q.Scene == strconv.FormatUint(scene, 10) && q.UISession == domain.Hex(ui) &&
		q.SessionEpoch == auth.SessionEpoch && q.RunID == s.Cfg.RunID && q.RunEpoch == s.Epoch &&
		!at.Before(q.Issued) && at.Before(q.Expires) && q.Rule == f.RuleVersion && q.Config == f.Hash &&
		q.Revision > 0 && q.Revision <= role.Revision && uint64(q.Revision) == revision
}
