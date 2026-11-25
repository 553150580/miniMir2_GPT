// 本文件持久化防守版本并分配角色每日可选的原镜像挑战凭证。
package persistence

import (
	"context"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

type mirrorPoolEntry struct {
	ID          string `bson:"_id"`          // 原赛季、入口和防守角色的组合。
	Scope       string `bson:"scope"`        // 只在同运行服、同永久入口和同赛季参与匹配。
	RoleID      string `bson:"role_id"`      // 防守者原永久角色。
	AccountID   string `bson:"account_id"`   // 匹配时排除自己的全部角色。
	Name        string `bson:"name"`         // 原防守版本的名称。
	Job         uint32 `bson:"job"`          // 原防守版本的职业。
	Level       uint32 `bson:"level"`        // 原防守版本对应等级。
	DefenseID   string `bson:"defense_id"`   // 最新版本的指针，已发候选不会跟随它变更。
	Version     uint64 `bson:"version"`      // 当前角色的防守版本水位。
	RuleVersion uint64 `bson:"rule_version"` // 可读取的原规则版本。
	ConfigHash  string `bson:"config_hash"`  // 同一规则版本内也校验发布摘要。
	Digest      string `bson:"digest"`       // 相同匹配快照反复发布时保留此前版本。
	Selector    string `bson:"selector"`     // 由角色真值计算的稳定顺序，客户端不可指定。
}

type mirrorOfferRecord struct {
	MirrorOffer    `bson:",inline"`
	DefenseID      string `bson:"defense_id"`       // 不可变防守版本。
	DefenderRoleID string `bson:"defender_role_id"` // 原永久角色。
}

type mirrorDay struct {
	ID          string              `bson:"_id"`          // 原赛季和原上海运营日的角色键。
	Entries     uint32              `bson:"entries"`      // 预占与已有效结算的次数。
	Offers      []mirrorOfferRecord `bson:"offers"`       // 首次选择分配后的固定候选。
	RatedJob    uint32              `bson:"rated_job"`    // 首次分配时用于匹配的原职业。
	RatedLevel  uint32              `bson:"rated_level"`  // 首次分配时用于匹配的原等级。
	RuleVersion uint64              `bson:"rule_version"` // 候选所用规则版本。
	ConfigHash  string              `bson:"config_hash"`  // 候选所用规则摘要。
}

// PublishMirrorDefense 原子更新防守目录，只创建不可变版本，一致内容重复发布读回原版本。
func (s *Store) PublishMirrorDefense(ctx context.Context, a Auth, req MirrorPublish) (MirrorDefense, error) {
	var result MirrorDefense
	if req.ExpectedRevision <= 0 {
		return result, Fail(p.CodeInvalidArgument, "镜像防守缺少原角色修订")
	}
	now := bossTime(time.Now())
	_, begin, _, err := s.currentMirrorSeason(now)
	if err != nil {
		return result, err
	}
	digest, err := bossDigest(req.Snapshot)
	if err != nil {
		return result, Fail(p.CodeInvalidArgument, "无法编码镜像防守快照")
	}
	err = s.guardedTx(ctx, a, func(tc context.Context) error {
		result = MirrorDefense{}
		r, err := s.mirrorRole(tc, a)
		if err != nil {
			return err
		}
		if r.Revision != req.ExpectedRevision || !validMirrorSnapshot(&s.Cfg.Fixture, r, req.Snapshot) {
			return Fail(p.CodeOperationConflict, "镜像防守已偏离原角色快照")
		}
		scope := mirrorScope(s.Cfg.RunID, r.Origin, begin)
		id := scope + ":" + r.ID
		pool := s.DB.Collection(mirrorPoolCollection)
		latest, err := mirrorOptional[mirrorPoolEntry](tc, pool, id)
		if err != nil {
			return err
		}
		f := &s.Cfg.Fixture
		defenses := s.DB.Collection(mirrorDefenseCollection)
		if latest.Digest == digest && latest.RuleVersion == f.RuleVersion && latest.ConfigHash == f.Hash && latest.DefenseID != "" {
			return defenses.FindOne(tc, bson.M{"_id": latest.DefenseID, "role_id": r.ID, "origin": r.Origin}).Decode(&result)
		}
		if latest.Version >= math.MaxInt64 {
			return Fail(p.CodeOperationConflict, "防守快照版本已达上限")
		}
		result = MirrorDefense{ID: domain.ID(), RoleID: r.ID, AccountID: r.AccountID, Origin: r.Origin, RunID: s.Cfg.RunID,
			SeasonStart: begin, Version: latest.Version + 1, RoleRevision: r.Revision, RuleVersion: f.RuleVersion, ConfigHash: f.Hash,
			Digest: digest, Snapshot: req.Snapshot}
		if _, err := defenses.InsertOne(tc, result); err != nil {
			return err
		}
		next := mirrorPoolEntry{ID: id, Scope: scope, RoleID: r.ID, AccountID: r.AccountID, Name: r.Name, Job: r.Job, Level: r.Level,
			DefenseID: result.ID, Version: result.Version, RuleVersion: f.RuleVersion, ConfigHash: f.Hash, Digest: digest, Selector: Hash([]byte(r.ID))}
		_, err = pool.ReplaceOne(tc, bson.M{"_id": id}, next, options.Replace().SetUpsert(true))
		return err
	})
	return result, err
}

// QueryMirrorArena 对当前合法角色读取已确认资格、发展本日候选与有限对战历史。
func (s *Store) QueryMirrorArena(ctx context.Context, a Auth) (MirrorInfo, error) {
	var out MirrorInfo
	now := bossTime(time.Now())
	cfg, begin, end, err := s.currentMirrorSeason(now)
	if err != nil {
		return out, err
	}
	err = s.guardedTx(ctx, a, func(tc context.Context) error {
		out = MirrorInfo{SeasonStart: begin, SeasonEnd: end, DailyLimit: cfg.DailyLimit}
		r, err := s.mirrorRole(tc, a)
		if err != nil {
			return err
		}
		scope := mirrorScope(s.Cfg.RunID, r.Origin, begin)
		day, err := s.ensureMirrorDay(tc, r, scope, now, cfg)
		if err != nil {
			return err
		}
		out.EntriesUsed = day.Entries
		for _, offer := range day.Offers {
			out.Offers = append(out.Offers, offer.MirrorOffer)
		}
		score, err := mirrorOptional[mirrorSeasonScore](tc, s.DB.Collection(mirrorSeasonCollection), scope+":"+r.ID)
		if err != nil {
			return err
		}
		out.SeasonPoints, out.Streak = score.Points, score.Streak
		active, err := mirrorOptional[mirrorActive](tc, s.DB.Collection(mirrorActiveCollection), mirrorRoleKey(r.Origin, r.ID))
		if err != nil {
			return err
		}
		if active.ChallengeID != "" {
			record, err := s.readMirrorRecord(tc, active.ChallengeID)
			if err != nil {
				return err
			}
			if record.Origin != r.Origin || record.AttackerRoleID != r.ID || record.Outcome != MirrorActive {
				return Fail(p.CodeOperationConflict, "镜像竞技原活动占位已失去身份")
			}
			out.ActiveChallenge = &record.MirrorChallenge
		}
		out.AttackHistory, err = s.mirrorHistory(tc, "attacker_role_id", r, cfg.AttackHistoryLimit)
		if err != nil {
			return err
		}
		out.DefenseHistory, err = s.mirrorHistory(tc, "defender_role_id", r, cfg.DefenseHistoryLimit)
		return err
	})
	return out, err
}

// ensureMirrorDay 不替用户更换已发对手；配置换版时只更换候选的解释规则，当日次数不归零。
func (s *Store) ensureMirrorDay(ctx context.Context, r domain.Role, scope string, now time.Time, cfg *devconfig.MirrorArenaConfig) (mirrorDay, error) {
	collection := s.DB.Collection(mirrorDayCollection)
	id := mirrorDayKey(scope, r.ID, now)
	day, err := mirrorOptional[mirrorDay](ctx, collection, id)
	if err != nil {
		return day, err
	}
	f := &s.Cfg.Fixture
	if len(day.Offers) != 0 && day.RuleVersion == f.RuleVersion && day.ConfigHash == f.Hash {
		return day, nil
	}
	day.ID, day.Offers = id, nil
	entries, err := s.selectMirrorDefenses(ctx, r, scope, Hash([]byte(id)), cfg)
	if err != nil || len(entries) == 0 {
		return day, err
	}
	day.RatedJob, day.RatedLevel, day.RuleVersion, day.ConfigHash = r.Job, r.Level, f.RuleVersion, f.Hash
	for _, entry := range entries {
		day.Offers = append(day.Offers, mirrorOfferRecord{MirrorOffer: MirrorOffer{ID: domain.ID(), DefenderName: entry.Name,
			DefenderJob: entry.Job, DefenderLevel: entry.Level, DefenseVersion: entry.Version}, DefenseID: entry.DefenseID, DefenderRoleID: entry.RoleID})
	}
	_, err = collection.ReplaceOne(ctx, bson.M{"_id": id}, day, options.Replace().SetUpsert(true))
	return day, err
}

// selectMirrorDefenses 从真实索引中按等级、角色摘要和日旋转有界选取；候选不接受任何客户端角色号。
func (s *Store) selectMirrorDefenses(ctx context.Context, r domain.Role, scope, pivot string, cfg *devconfig.MirrorArenaConfig) ([]mirrorPoolEntry, error) {
	f := &s.Cfg.Fixture
	low := uint32(1)
	if r.Level > cfg.MaxLevelGap {
		low = r.Level - cfg.MaxLevelGap
	}
	high := min(uint64(math.MaxUint32), uint64(r.Level)+uint64(cfg.MaxLevelGap))
	filter := bson.M{"scope": scope, "account_id": bson.M{"$ne": r.AccountID}, "rule_version": f.RuleVersion, "config_hash": f.Hash}
	if cfg.MatchPolicy == devconfig.MirrorArenaMatchSameJobLevelWindow {
		filter["job"] = r.Job
	}
	var entries []mirrorPoolEntry
	segments := []struct{ level, selector any }{
		{r.Level, bson.M{"$gte": pivot}},
		{bson.M{"$gt": r.Level, "$lte": high}, nil},
		{bson.M{"$gte": low, "$lt": r.Level}, nil},
		{r.Level, bson.M{"$lt": pivot}},
	}
	for _, segment := range segments {
		left := int(cfg.OpponentLimit) - len(entries)
		if left <= 0 {
			break
		}
		filter["level"] = segment.level
		delete(filter, "selector")
		if segment.selector != nil {
			filter["selector"] = segment.selector
		}
		cursor, err := s.DB.Collection(mirrorPoolCollection).Find(ctx, filter,
			options.Find().SetSort(bson.D{{Key: "level", Value: 1}, {Key: "selector", Value: 1}}).SetLimit(int64(left)))
		if err != nil {
			return nil, err
		}
		var rows []mirrorPoolEntry
		if err := cursor.All(ctx, &rows); err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.RoleID != r.ID && row.AccountID != r.AccountID && cfg.AllowsMatch(r.Job, r.Level, row.Job, row.Level) {
				entries = append(entries, row)
			}
		}
	}
	return entries, nil
}

func (s *Store) mirrorHistory(ctx context.Context, field string, r domain.Role, limit uint32) ([]MirrorSummary, error) {
	filter := bson.M{field: r.ID, "origin": r.Origin, "finished_at": bson.M{"$exists": true}}
	project := bson.M{"_id": 1, "offer_id": 1, "attacker_role_id": 1, "defender_role_id": 1, "attacker_name": 1,
		"defender_name": 1, "defense_version": 1, "outcome": 1, "points": 1, "started_at": 1, "finished_at": 1}
	cursor, err := s.DB.Collection(mirrorChallengeCollection).Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "finished_at", Value: -1}, {Key: "_id", Value: -1}}).SetProjection(project).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	var out []MirrorSummary
	err = cursor.All(ctx, &out)
	return out, err
}

// validMirrorSnapshot 不读取客户端状态，并核验冻结输入没有额外技能或超出发布边界的内容。
func validMirrorSnapshot(f *devconfig.Fixture, r domain.Role, snap domain.MirrorCombatSnapshot) bool {
	if snap.Job != r.Job || snap.Gender != r.Gender || snap.Name != r.Name || snap.Level != r.Level || snap.Level == 0 ||
		snap.Fighter.JobID != snap.Job || !validMirrorFighter(snap.Fighter) || len(snap.Skills) > len(f.Skills) ||
		len(snap.SkillSlotIDs) != 0 && len(snap.SkillSlotIDs) != devconfig.AttackSkillSlotCount || len(snap.Potions) > len(f.Items) {
		return false
	}
	if snap.Pet != nil && (snap.Pet.DefinitionID == 0 || !validMirrorFighter(snap.Pet.Fighter)) {
		return false
	}
	seen := make(map[uint32]bool, len(snap.Skills))
	for _, skill := range snap.Skills {
		rank, known := r.Skill(skill.SkillID)
		if !known || rank != skill.Level || seen[skill.SkillID] || skill.SkillID == 0 {
			return false
		}
		seen[skill.SkillID] = true
	}
	slots := ResolvedSkillSlots(r, f)
	for i, slot := range snap.SkillSlotIDs {
		if slot != slots[i] {
			return false
		}
	}
	clear(seen)
	for _, supply := range snap.Potions {
		if supply.ItemID == 0 || supply.Quantity <= 0 || seen[supply.ItemID] {
			return false
		}
		seen[supply.ItemID] = true
	}
	return snap.Material == nil || snap.Material.ItemID != 0 && snap.Material.Quantity > 0
}

func validMirrorFighter(f devconfig.Fighter) bool {
	return f.HP > 0 && f.HP <= math.MaxInt32 && f.MP >= 0 && f.MP <= math.MaxInt32 && f.AttackMS > 0 && f.MoveMS > 0
}

// mirrorArenaIndexes 支持按适配等级检索和有限战报观察，不为永久回执设置 TTL。
func mirrorArenaIndexes() map[string][]mongo.IndexModel {
	return map[string][]mongo.IndexModel{
		mirrorPoolCollection: {
			{Keys: bson.D{{Key: "scope", Value: 1}, {Key: "config_hash", Value: 1}, {Key: "rule_version", Value: 1}, {Key: "level", Value: 1}, {Key: "selector", Value: 1}}},
			{Keys: bson.D{{Key: "scope", Value: 1}, {Key: "config_hash", Value: 1}, {Key: "rule_version", Value: 1}, {Key: "job", Value: 1}, {Key: "level", Value: 1}, {Key: "selector", Value: 1}}},
		},
		mirrorChallengeCollection: {
			{Keys: bson.D{{Key: "origin", Value: 1}, {Key: "attacker_role_id", Value: 1}, {Key: "finished_at", Value: -1}, {Key: "_id", Value: -1}}},
			{Keys: bson.D{{Key: "origin", Value: 1}, {Key: "defender_role_id", Value: 1}, {Key: "finished_at", Value: -1}, {Key: "_id", Value: -1}}},
		},
		mirrorSeasonCollection: {{Keys: bson.D{{Key: "scope", Value: 1}, {Key: "points", Value: -1}, {Key: "last_match_at", Value: 1}, {Key: "role_id", Value: 1}}}},
	}
}
