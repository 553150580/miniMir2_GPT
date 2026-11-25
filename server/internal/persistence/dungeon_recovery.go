// 本文件在新运行服接管后找出旧执行留下的讨伐链，只用原场次记录恢复次数。
package persistence

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	p "minimir/internal/protocol"
)

// dungeonRecoveryPageSize 限制一次读取的原场次编号，实际退款仍逐场走原事务。
const dungeonRecoveryPageSize = 32

// RecoverInterruptedDungeons 在获准运行服上线前对账所有失权活动场次，允许配置关闭后继续退款。
func (s *Store) RecoverInterruptedDungeons(ctx context.Context) error {
	if s.Cfg.RunID == 0 || s.Epoch <= 0 || s.runGuardSlots == nil {
		return Fail(p.CodeRouteChanged, "没有用于恢复讨伐链的运行服授权")
	}
	// 只读索引负责寻找待处理编号，退款仍须在事务中重新验证旧执行和每位原成员。
	filter := bson.M{"run_server_id": s.Cfg.RunID, "outcome": DungeonActive, "run_epoch": bson.M{"$lt": s.Epoch}}
	selection := options.Find().SetProjection(bson.M{"_id": 1}).SetLimit(dungeonRecoveryPageSize)
	for {
		cursor, err := s.DB.Collection(dungeonCollection).Find(ctx, filter, selection)
		if err != nil {
			return err
		}
		var identities []struct {
			ChallengeID string `bson:"_id"` // 保留旧执行已经写入的唯一原场次键。
		}
		if err = cursor.All(ctx, &identities); err != nil {
			return err
		}
		for _, identity := range identities {
			if _, _, err = s.RecoverDungeon(ctx, identity.ChallengeID); err != nil {
				return fmt.Errorf("恢复讨伐链场次 %s：%w", identity.ChallengeID, err)
			}
		}
		if len(identities) < dungeonRecoveryPageSize {
			return nil
		}
	}
}
