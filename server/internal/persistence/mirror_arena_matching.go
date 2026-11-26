// 本文件以原候选级差窗从防守目录有界查找近等级快照，并用运营日摘要打散同差候选。
package persistence

import (
	"cmp"
	"context"
	"maps"
	"math"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// selectMirrorDefenses 优先取尽同级候选，再对高低两侧按绝对等级差合并。同差共享同一个日轮换点。
func (s *Store) selectMirrorDefenses(ctx context.Context, r domain.Role, scope, pivot string, cfg *devconfig.MirrorArenaConfig) ([]mirrorPoolEntry, error) {
	if !cfg.AllowsMatch(r.Job, r.Level, r.Job, r.Level) || cfg.OpponentLimit == 0 {
		return nil, nil
	}
	f := &s.Cfg.Fixture
	filter := bson.M{"scope": scope, "account_id": bson.M{"$ne": r.AccountID}, "role_id": bson.M{"$ne": r.ID},
		"rule_version": f.RuleVersion, "config_hash": f.Hash, "job": bson.M{"$gte": 0, "$lte": 2}}
	if cfg.MatchPolicy == devconfig.MirrorArenaMatchSameJobLevelWindow {
		filter["job"] = r.Job
	}
	limit := int(cfg.OpponentLimit)
	entries, err := s.readMirrorLevel(ctx, filter, r.Level, pivot, limit)
	left := limit - len(entries)
	if err != nil || left == 0 || cfg.MaxLevelGap == 0 {
		return entries, err
	}
	// 从两侧靠近当前等级开始读。即使配置跨度很大，也不需要逐个检查空等级。
	low := uint32(1)
	if r.Level > cfg.MaxLevelGap {
		low = r.Level - cfg.MaxLevelGap
	}
	high := uint32(min(uint64(math.MaxUint32), uint64(r.Level)+uint64(cfg.MaxLevelGap)))
	var nearby []mirrorPoolEntry
	for _, side := range [...]struct {
		levels bson.M
		order  int
	}{
		{levels: bson.M{"$gte": low, "$lt": r.Level}, order: -1},
		{levels: bson.M{"$gt": r.Level, "$lte": high}, order: 1},
	} {
		rows, err := s.readMirrorNearestSide(ctx, filter, side.levels, side.order, pivot, left)
		if err != nil {
			return nil, err
		}
		nearby = append(nearby, rows...)
	}
	slices.SortFunc(nearby, func(a, b mirrorPoolEntry) int {
		aGap := max(a.Level, r.Level) - min(a.Level, r.Level)
		bGap := max(b.Level, r.Level) - min(b.Level, r.Level)
		if distance := cmp.Compare(aGap, bGap); distance != 0 {
			return distance
		}
		if aWrap, bWrap := a.Selector < pivot, b.Selector < pivot; aWrap != bWrap {
			if aWrap {
				return 1
			}
			return -1
		}
		if order := cmp.Compare(a.Selector, b.Selector); order != 0 {
			return order
		}
		return cmp.Compare(a.RoleID, b.RoleID)
	})
	return append(entries, nearby[:min(left, len(nearby))]...), nil
}

// readMirrorLevel 在同一级的既有 selector 索引上先读日轮换点之后，再回到环首。
func (s *Store) readMirrorLevel(ctx context.Context, base bson.M, level uint32, pivot string, limit int) ([]mirrorPoolEntry, error) {
	filter := maps.Clone(base)
	filter["level"], filter["selector"] = level, bson.M{"$gte": pivot}
	rows, err := s.readMirrorPool(ctx, filter, 1, limit)
	if err != nil || len(rows) == limit {
		return rows, err
	}
	filter["selector"] = bson.M{"$lt": pivot}
	wrapped, err := s.readMirrorPool(ctx, filter, 1, limit-len(rows))
	if err != nil {
		return nil, err
	}
	return append(rows, wrapped...), nil
}

// readMirrorNearestSide 限定单侧读取量。达到上限时，只有最后一级可能被截断，需要按原日轮换点选满。
func (s *Store) readMirrorNearestSide(ctx context.Context, base, levels bson.M, order int, pivot string, limit int) ([]mirrorPoolEntry, error) {
	filter := maps.Clone(base)
	filter["level"] = levels
	rows, err := s.readMirrorPool(ctx, filter, order, limit)
	if err != nil || len(rows) < limit {
		return rows, err
	}
	boundary := rows[len(rows)-1].Level
	start := slices.IndexFunc(rows, func(row mirrorPoolEntry) bool { return row.Level == boundary })
	rotated, err := s.readMirrorLevel(ctx, base, boundary, pivot, limit-start)
	if err != nil {
		return nil, err
	}
	return append(rows[:start], rotated...), nil
}

// readMirrorPool 沿现有等级及 selector 复合索引的同向或反向读取，限制返回文档数量。
func (s *Store) readMirrorPool(ctx context.Context, filter bson.M, order, limit int) ([]mirrorPoolEntry, error) {
	orderBy := bson.D{{Key: "level", Value: order}, {Key: "selector", Value: order}}
	cursor, err := s.DB.Collection(mirrorPoolCollection).Find(ctx, filter,
		options.Find().SetSort(orderBy).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	var rows []mirrorPoolEntry
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
