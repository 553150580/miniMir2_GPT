// 本文件将已冻结运行服的市场门禁接入角色和后台事务。
package persistence

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	p "minimir/internal/protocol"
)

// errMarketMergeFrozen 通知后台市场本轮无需执行，保留定时检查供恢复后使用。
var errMarketMergeFrozen = errors.New("运行服市场已为合服清退冻结")

// marketMergeFrozen 读取运行服的市场门禁。业务写入仍须与冻结共用同一个组写屏障。
func (s *Store) marketMergeFrozen(ctx context.Context) (bool, error) {
	err := s.DB.Collection(marketMergeGateCollection).FindOne(ctx, bson.M{"_id": s.Cfg.RunID},
		options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	return err == nil, err
}

// requireOpenMarkets 只拒绝新市场命令，同一原操作已提交的永久回执仍可读回。
func (s *Store) requireOpenMarkets(ctx context.Context) error {
	frozen, err := s.marketMergeFrozen(ctx)
	if err != nil {
		return err
	}
	if frozen {
		return Fail(p.CodeLineRecovering, "运行服市场维护中")
	}
	return nil
}

// marketAdvanceResult 让调度器安静等待维护结束，也保留其他存储错误用于告警。
func marketAdvanceResult(err error) (bool, error) {
	if errors.Is(err, errMarketMergeFrozen) {
		return false, nil
	}
	return err == nil, err
}
