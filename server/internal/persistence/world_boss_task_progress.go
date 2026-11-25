// 本文件在世界首领的永久终态事务中确认任务进度，保持原角色资产仍由角色房间执行者写入。
package persistence

import (
	"context"
	"errors"
	"maps"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const worldBossTaskProgressCollection = "world_boss_task_progress"

// worldBossTaskProgressRecord 只保存世界击杀产生的计数增量。领奖、主线前置和其它来源仍在角色文档中。
type worldBossTaskProgressRecord struct {
	ID       string                   `bson:"_id"`      // 角色永久入口和角色编号组成的唯一键。
	Progress domain.TaskCodexProgress `bson:"progress"` // 世界首领产生的永久计数与最近运营日计数。
}

// worldBossTaskProgressKey 沿角色永久入口记录，跨运行服迁移仍保留已确认计数。
func worldBossTaskProgressKey(roleID string, origin uint64) string {
	return strconv.FormatUint(origin, 10) + ":" + roleID
}

// ReadWorldBossTaskProgress 读取已授权角色的世界计数。调用方用原读快照或领取事务检查角色身份。
func (s *Store) ReadWorldBossTaskProgress(ctx context.Context, r domain.Role) (domain.TaskCodexProgress, error) {
	ctx, cancel := context.WithTimeout(ctx, transactionTimeout)
	defer cancel()
	var row worldBossTaskProgressRecord
	err := s.DB.Collection(worldBossTaskProgressCollection).FindOne(ctx, bson.M{"_id": worldBossTaskProgressKey(r.ID, r.Origin)}).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.TaskCodexProgress{}, nil
	}
	return row.Progress, err
}

// worldBossTaskParticipants 沿用当前场次的原贡献和席位门槛，独立于角色是否获得任一实体附件。
func worldBossTaskParticipants(f *devconfig.Fixture, template *devconfig.WorldBossConfig, finish WorldBossFinish) (map[string]uint64, error) {
	rules := template.Rewards
	if finish.Outcome != WorldBossKilled || rules == nil || !f.HasWorldBossTaskTargets(template.TemplateID, template.Map.ID, template.Monster.ID) {
		return nil, nil
	}
	eligible := make(map[string]uint64)
	for _, contribution := range finish.Contributions {
		if contribution.Damage <= 0 || contribution.Damage < rules.MinimumDamage {
			continue
		}
		evidence := contribution.RewardEvidence
		if evidence == nil {
			return nil, Fail(p.CodeInvalidArgument, "世界首领缺少合格击杀贡献的原席位和永久入口")
		}
		if worldBossRewardPresenceAllowed(evidence.Presence, rules) {
			eligible[contribution.RoleID] = evidence.Origin
		}
	}
	return eligible, nil
}

// writeWorldBossTaskProgress 与原场次终态同事务批量写入；同一终态重放会在调用此处之前返回。
func (s *Store) writeWorldBossTaskProgress(ctx context.Context, template *devconfig.WorldBossConfig, finish WorldBossFinish) error {
	f := &s.Cfg.Fixture
	eligible, err := worldBossTaskParticipants(f, template, finish)
	if err != nil || len(eligible) == 0 {
		return err
	}
	roleIDs := make([]string, 0, len(eligible))
	for roleID := range eligible {
		roleIDs = append(roleIDs, roleID)
	}
	cursor, err := s.DB.Collection("roles").Find(ctx, bson.M{"_id": bson.M{"$in": roleIDs}},
		options.Find().SetProjection(bson.M{"_id": 1, "origin_server_id": 1, "level": 1, "tasks_codex": 1}))
	if err != nil {
		return err
	}
	var roles []domain.Role // 这里只读取原资格和前置，避免取得角色背包或改动资产水位。
	if err = cursor.All(ctx, &roles); err != nil {
		return err
	}
	keys := make([]string, 0, len(roles))
	for _, role := range roles {
		if eligible[role.ID] != role.Origin {
			return Fail(p.CodeOperationConflict, "世界首领原角色的永久入口不一致")
		}
		keys = append(keys, worldBossTaskProgressKey(role.ID, role.Origin))
	}
	if len(keys) == 0 {
		return nil
	}
	collection := s.DB.Collection(worldBossTaskProgressCollection)
	cursor, err = collection.Find(ctx, bson.M{"_id": bson.M{"$in": keys}})
	if err != nil {
		return err
	}
	var rows []worldBossTaskProgressRecord
	if err = cursor.All(ctx, &rows); err != nil {
		return err
	}
	previous := make(map[string]domain.TaskCodexProgress, len(rows))
	for _, row := range rows {
		previous[row.ID] = row.Progress
	}
	writes := make([]mongo.WriteModel, 0, len(roles))
	for _, role := range roles {
		key := worldBossTaskProgressKey(role.ID, role.Origin)
		before := previous[key]
		after := advanceWorldBossTaskProgress(role, before, f, template.TemplateID, template.Map.ID, template.Monster.ID, finish.FinishedAt)
		if before.Daily.Day == after.Daily.Day && maps.Equal(before.Daily.Counters, after.Daily.Counters) && maps.Equal(before.Permanent, after.Permanent) {
			continue
		}
		writes = append(writes, mongo.NewReplaceOneModel().SetFilter(bson.M{"_id": key}).SetUpsert(true).
			SetReplacement(worldBossTaskProgressRecord{ID: key, Progress: after}))
	}
	if len(writes) == 0 {
		return nil
	}
	_, err = collection.BulkWrite(ctx, writes)
	return err
}
