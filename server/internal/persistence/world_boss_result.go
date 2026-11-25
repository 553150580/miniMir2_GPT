// 本文件记录运行服世界首领的唯一预约及永久终态。写入必须经过当前运行服和游戏节点租约，本模块不分配奖励。
package persistence

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
)

const worldBossResultCollection = "world_boss_results"
const worldBossRunGuardGroup uint64 = 0 // 同一运行服的场次写入固定使用现有的一组围栏，与租约换代原子竞争。

// WorldBossOutcome 标记原场次是否形成了永久结果。
type WorldBossOutcome uint32

const (
	WorldBossActive      WorldBossOutcome = iota // 原场已预约，尚未确认终态。
	WorldBossKilled                              // 实际命中把首领生命减为零。
	WorldBossExpired                             // 原运营窗口走完，首领仍存活。
	WorldBossInterrupted                         // 原执行中断，不能继续或重复战斗。
)

// WorldBossSpec 是首次预约时确认且不再修改的场次身份和运营窗口。
type WorldBossSpec struct {
	RunID        uint64    `bson:"run_server_id"` // 逻辑运行服，分区及原入口不参与场次归属。
	OccurrenceID string    `bson:"occurrence_id"` // 由运行服、原模板及开场毫秒生成的稳定编号。
	TemplateID   uint32    `bson:"template_id"`   // 原首领模板。
	Execution    uint64    `bson:"execution"`     // 首次预约的游戏执行代次。
	RuleVersion  uint64    `bson:"rule_version"`  // 本场冻结的规则版本。
	ConfigHash   string    `bson:"config_hash"`   // 本场冻结的完整配置摘要。
	OpensAt      time.Time `bson:"opens_at"`      // 上海运营日换算成绝对时间的开场时刻。
	ClosesAt     time.Time `bson:"closes_at"`     // 原运营结束时间，提前击杀不会改写它。
	HPMax        int64     `bson:"hp_max"`        // 本场原首领最大生命。
}

// WorldBossContribution 记录真实入场及有效扣血，零伤害仍可证明入场但没有伤害名次。
type WorldBossContribution struct {
	RoleID         string    `bson:"role_id"`         // 角色永久身份，包括已离场角色。
	DisplayName    string    `bson:"display_name"`    // 原场冻结的角色名称。
	ReservationID  string    `bson:"reservation_id"`  // 原参战席位的入场身份。
	CombatSnapshot string    `bson:"combat_snapshot"` // 原入场战斗快照摘要。
	JoinedAt       time.Time `bson:"joined_at"`       // 该席位首次确认参战的绝对时间。
	Damage         int64     `bson:"damage"`          // 对共享生命实际生效的伤害，已合并原宠物归属。
}

// WorldBossFinish 保存世界 actor 冻结的终态与有效贡献。
type WorldBossFinish struct {
	Outcome       WorldBossOutcome        `bson:"outcome"`                 // 首次永久终态。
	FinishedAt    time.Time               `bson:"finished_at,omitempty"`   // 权威击杀、逃走或确认中断的时刻。
	HPRemaining   int64                   `bson:"hp_remaining,omitempty"`  // 已确认时的剩余生命。
	Contributions []WorldBossContribution `bson:"contributions,omitempty"` // 有效伤害降序，同伤害按永久角色键排序。
}

// WorldBossResult 对外提供原始预约与已确认结果。发生执行器故障后，不补造未持久的生命或贡献。
type WorldBossResult struct {
	WorldBossSpec   `bson:",inline"`
	WorldBossFinish `bson:",inline"`
	OwnerInstance   string `bson:"owner_instance"` // 首次预约使用的游戏进程实例。
	RunEpoch        int64  `bson:"run_epoch"`      // 首次预约使用的运行服代次。
	ProgressKnown   bool   `bson:"progress_known"` // 为假时没有可证明的终态生命或排行。
}

type worldBossRecord struct {
	ID              string `bson:"_id"` // 由运行服和原场次编号组成的唯一业务键。
	WorldBossResult `bson:",inline"`
	FinishHash      string `bson:"finish_hash,omitempty"` // 确认同一终态证据重放使用的规范摘要。
}

func worldBossResultIndexes() []mongo.IndexModel {
	return []mongo.IndexModel{
		// 同一运行服至多一个未清的预约；永久终态既不覆写也不设 TTL。
		{Keys: bson.D{{Key: "run_server_id", Value: 1}}, Options: options.Index().SetName("one_active_per_run").
			SetUnique(true).SetPartialFilterExpression(bson.M{"outcome": WorldBossActive})},
		{Keys: bson.D{{Key: "run_server_id", Value: 1}, {Key: "opens_at", Value: -1}, {Key: "_id", Value: -1}}},
	}
}

// WorldBossOccurrenceID 将原运营窗口映射为与世界 actor 一致的场次编号。
func WorldBossOccurrenceID(runID uint64, templateID uint32, opensAt time.Time) string {
	return Hash(fmt.Appendf(nil, "%d:%d:%d", runID, templateID, opensAt.UnixMilli()))[:32]
}

func worldBossKey(runID uint64, occurrence string) string {
	return fmt.Sprintf("%d:%s", runID, occurrence)
}

// BeginWorldBoss 预约新场次。相同执行的重放返回已确认预约；同场已有终态则不再开启。
func (s *Store) BeginWorldBoss(ctx context.Context, spec WorldBossSpec) (*WorldBossResult, error) {
	spec = normalizeWorldBossSpec(spec)
	if err := s.validateWorldBossIdentity(spec); err != nil {
		return nil, err
	}
	var result *WorldBossResult
	err := s.worldBossTx(ctx, func(tc context.Context) error {
		record, err := s.readWorldBoss(tc, bson.M{"_id": worldBossKey(spec.RunID, spec.OccurrenceID)})
		if err != nil {
			return err
		}
		if record != nil {
			if record.Outcome == WorldBossActive && (!s.ownsWorldBoss(record) || record.WorldBossSpec != spec) {
				return Fail(p.CodeOperationConflict, "原世界首领场次已有其它执行")
			}
			result = &record.WorldBossResult
			return nil
		}
		now := time.Now()
		if _, err = s.worldBossTemplate(spec); err != nil {
			return err
		}
		if now.Before(spec.OpensAt) || !now.Before(spec.ClosesAt) {
			return Fail(p.CodeLineForbidden, "世界首领原开场窗口当前不可预约")
		}
		active, err := s.readWorldBoss(tc, bson.M{"run_server_id": spec.RunID, "outcome": WorldBossActive})
		if err != nil {
			return err
		}
		if active != nil {
			return Fail(p.CodeLineRecovering, "本运行服有未结算的世界首领场次")
		}
		created := &worldBossRecord{ID: worldBossKey(spec.RunID, spec.OccurrenceID), WorldBossResult: WorldBossResult{
			WorldBossSpec: spec, OwnerInstance: s.Instance, RunEpoch: s.Epoch,
		}}
		if _, err = s.DB.Collection(worldBossResultCollection).InsertOne(tc, created); err != nil {
			return err
		}
		result = &created.WorldBossResult
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// FinishWorldBoss 只接受原预约的执行者，以同一事务确认有效伤害和唯一终态。
func (s *Store) FinishWorldBoss(ctx context.Context, spec WorldBossSpec, finish WorldBossFinish) (*WorldBossResult, error) {
	spec = normalizeWorldBossSpec(spec)
	if err := s.validateWorldBossIdentity(spec); err != nil {
		return nil, err
	}
	finish, digest, err := normalizeWorldBossFinish(spec, finish)
	if err != nil {
		return nil, err
	}
	var result *WorldBossResult
	err = s.worldBossTx(ctx, func(tc context.Context) error {
		collection := s.DB.Collection(worldBossResultCollection)
		record, err := s.readWorldBoss(tc, bson.M{"_id": worldBossKey(spec.RunID, spec.OccurrenceID)})
		if err != nil {
			return err
		}
		if record == nil || !s.ownsWorldBoss(record) || record.WorldBossSpec != spec {
			return Fail(p.CodeOperationConflict, "世界首领终态与原场次执行不一致")
		}
		if record.Outcome != WorldBossActive {
			if record.FinishHash != digest {
				return Fail(p.CodeOperationConflict, "世界首领已确认不同终态证据")
			}
			result = &record.WorldBossResult
			return nil
		}
		if finish.FinishedAt.After(worldBossTime(time.Now())) {
			return Fail(p.CodeInvalidArgument, "世界首领不能确认尚未发生的终态")
		}
		template, err := s.worldBossTemplate(spec)
		if err != nil {
			return err
		}
		if len(finish.Contributions) > template.ParticipantLimit {
			return Fail(p.CodeInvalidArgument, "世界首领贡献超过原场次人数")
		}
		record.WorldBossFinish, record.ProgressKnown, record.FinishHash = finish, true, digest
		write, err := collection.ReplaceOne(tc, bson.M{"_id": record.ID, "outcome": WorldBossActive}, record)
		if err != nil {
			return err
		}
		if write.MatchedCount != 1 {
			return Fail(p.CodeOperationPending, "原世界首领结果待查证")
		}
		result = &record.WorldBossResult
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RecoverWorldBoss 使用新取得的运行服租约结束仍悬置的旧执行，并返回原最近一场终态。
func (s *Store) RecoverWorldBoss(ctx context.Context) (*WorldBossResult, error) {
	var result *WorldBossResult
	err := s.worldBossTx(ctx, func(tc context.Context) error {
		active, err := s.readWorldBoss(tc, bson.M{"run_server_id": s.Cfg.RunID, "outcome": WorldBossActive})
		if err != nil {
			return err
		}
		if active != nil && !s.ownsWorldBoss(active) {
			if active.RunEpoch >= s.Epoch {
				return Fail(p.CodeRouteChanged, "世界首领属于仍有效或更新的运行执行")
			}
			// 故障后仅能证明原预约已被取消。无现场证明时不得把满血、空榜或击杀写成结论。
			update := bson.M{"$set": bson.M{"outcome": WorldBossInterrupted, "progress_known": false},
				"$currentDate": bson.M{"finished_at": true}}
			write, err := s.DB.Collection(worldBossResultCollection).UpdateOne(tc, bson.M{"_id": active.ID, "outcome": WorldBossActive}, update)
			if err != nil {
				return err
			}
			if write.MatchedCount != 1 {
				return Fail(p.CodeOperationPending, "原世界首领恢复结果待查证")
			}
		}
		recent, err := s.readWorldBoss(tc, bson.M{"run_server_id": s.Cfg.RunID, "outcome": bson.M{"$gt": WorldBossActive}},
			options.FindOne().SetSort(bson.D{{Key: "opens_at", Value: -1}, {Key: "_id", Value: -1}}))
		if err != nil {
			return err
		}
		result = nil
		if recent != nil {
			result = &recent.WorldBossResult
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) readWorldBoss(ctx context.Context, filter bson.M, opts ...options.Lister[options.FindOneOptions]) (*worldBossRecord, error) {
	var row worldBossRecord
	err := s.DB.Collection(worldBossResultCollection).FindOne(ctx, filter, opts...).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.WorldBossSpec = normalizeWorldBossSpec(row.WorldBossSpec)
	return &row, nil
}

func (s *Store) ownsWorldBoss(record *worldBossRecord) bool {
	return record.OwnerInstance == s.Instance && record.RunEpoch == s.Epoch
}

// worldBossTx 复用已部署的运行组写屏障，确保节点接管与场次结果不能同时成功。
func (s *Store) worldBossTx(ctx context.Context, fn func(context.Context) error) error {
	if s.Cfg.RunID == 0 || s.Epoch <= 0 || s.runGuardSlots == nil {
		return Fail(p.CodeRouteChanged, "未取得世界首领所属运行服授权")
	}
	ctx, cancel := context.WithTimeout(ctx, transactionTimeout)
	defer cancel()
	slot, err := s.runGuardSlots.acquire(ctx, worldBossRunGuardGroup)
	if err != nil {
		return Fail(p.CodeOperationPending, "原世界首领操作待查证")
	}
	defer func() { <-slot }()
	return s.tx(ctx, func(tc context.Context) error {
		guard, err := s.runFenceWrite(tc, worldBossRunGuardGroup)
		if err != nil {
			return err
		}
		write, err := s.Client.BulkWrite(tc, []mongo.ClientBulkWrite{guard}, options.ClientBulkWrite().SetVerboseResults(true))
		if err != nil {
			return transactionBulkWriteError(err)
		}
		if !write.Acknowledged || write.UpdateResults[0].MatchedCount != 1 {
			return Fail(p.CodeRouteChanged, "世界首领运行服写屏障失效")
		}
		return fn(tc)
	})
}

func normalizeWorldBossSpec(spec WorldBossSpec) WorldBossSpec {
	spec.OpensAt, spec.ClosesAt = worldBossTime(spec.OpensAt), worldBossTime(spec.ClosesAt)
	return spec
}

func worldBossTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Millisecond)
}

func (s *Store) validateWorldBossIdentity(spec WorldBossSpec) error {
	if spec.RunID == 0 || spec.RunID != s.Cfg.RunID || spec.RunID > math.MaxInt64 ||
		spec.TemplateID == 0 || spec.Execution == 0 || spec.Execution > math.MaxInt64 || spec.RuleVersion == 0 || spec.RuleVersion > math.MaxInt64 ||
		spec.OpensAt.IsZero() || !spec.ClosesAt.After(spec.OpensAt) || spec.HPMax <= 0 || spec.ConfigHash == "" ||
		spec.OccurrenceID != WorldBossOccurrenceID(spec.RunID, spec.TemplateID, spec.OpensAt) {
		return Fail(p.CodeInvalidArgument, "世界首领场次身份、窗口或原生命错误")
	}
	return nil
}

func (s *Store) worldBossTemplate(spec WorldBossSpec) (*devconfig.WorldBossConfig, error) {
	fixture := &s.Cfg.Fixture
	if !fixture.ValidWorldBoss() || spec.RuleVersion != fixture.RuleVersion || spec.ConfigHash != fixture.Hash {
		return nil, Fail(p.CodeLineForbidden, "世界首领规则或配置版本已失效")
	}
	row := fixture.WorldBoss
	if row.TemplateID != spec.TemplateID {
		index := slices.IndexFunc(fixture.WorldBossVariants, func(row devconfig.WorldBossConfig) bool { return row.TemplateID == spec.TemplateID })
		if index < 0 {
			return nil, Fail(p.CodeInvalidArgument, "未找到原世界首领模板")
		}
		row = &fixture.WorldBossVariants[index]
	}
	local := spec.OpensAt.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	if !slices.Contains(row.DailyOpenOffsets, local.Sub(midnight)) || !spec.OpensAt.Add(row.Window).Equal(spec.ClosesAt) ||
		spec.HPMax != row.Monster.HP {
		return nil, Fail(p.CodeInvalidArgument, "世界首领原日程或模板生命不符")
	}
	return row, nil
}

// normalizeWorldBossFinish 冻结毫秒精度的事件和稳定名次，允许相同贡献换序重试。
func normalizeWorldBossFinish(spec WorldBossSpec, finish WorldBossFinish) (WorldBossFinish, string, error) {
	invalid := func(message string) (WorldBossFinish, string, error) {
		return WorldBossFinish{}, "", Fail(p.CodeInvalidArgument, message)
	}
	finish.FinishedAt = worldBossTime(finish.FinishedAt)
	if finish.FinishedAt.Before(spec.OpensAt) || finish.HPRemaining < 0 || finish.HPRemaining > spec.HPMax {
		return invalid("世界首领终态时间或剩余生命错误")
	}
	switch finish.Outcome {
	case WorldBossKilled:
		if finish.HPRemaining != 0 || !finish.FinishedAt.Before(spec.ClosesAt) {
			return invalid("只有原窗口内归零才能确认世界首领击杀")
		}
	case WorldBossExpired:
		if finish.HPRemaining == 0 || !finish.FinishedAt.Equal(spec.ClosesAt) {
			return invalid("世界首领超时必须使用原场次截止和正生命")
		}
	case WorldBossInterrupted:
		if finish.HPRemaining == 0 || !finish.FinishedAt.Before(spec.ClosesAt) {
			return invalid("原执行者须在活动窗口内报告中断")
		}
	default:
		return invalid("未确认世界首领终态类型")
	}
	finish.Contributions = slices.Clone(finish.Contributions)
	if len(finish.Contributions) == 0 {
		finish.Contributions = nil
	}
	remaining := spec.HPMax - finish.HPRemaining
	roles := make(map[string]struct{}, len(finish.Contributions))
	reservations := make(map[string]struct{}, len(finish.Contributions))
	for i := range finish.Contributions {
		row := &finish.Contributions[i]
		row.JoinedAt = worldBossTime(row.JoinedAt)
		if !worldBossHexID(row.RoleID, 32) || !worldBossHexID(row.ReservationID, 32) || !worldBossHexID(row.CombatSnapshot, 64) ||
			strings.TrimSpace(row.DisplayName) == "" || !utf8.ValidString(row.DisplayName) || len(row.DisplayName) > 128 ||
			row.JoinedAt.Before(spec.OpensAt) || !row.JoinedAt.Before(spec.ClosesAt) || row.JoinedAt.After(finish.FinishedAt) ||
			row.Damage < 0 || row.Damage > remaining {
			return invalid("世界首领存在错误入场事实或超量贡献")
		}
		if _, exists := roles[row.RoleID]; exists {
			return invalid("同一角色具有重复世界首领贡献")
		}
		if _, exists := reservations[row.ReservationID]; exists {
			return invalid("同一世界首领入场回执重复使用")
		}
		roles[row.RoleID], reservations[row.ReservationID] = struct{}{}, struct{}{}
		remaining -= row.Damage
	}
	if remaining != 0 {
		return invalid("世界首领有效贡献与实际扣血不相等")
	}
	slices.SortFunc(finish.Contributions, func(left, right WorldBossContribution) int {
		if order := cmp.Compare(right.Damage, left.Damage); order != 0 {
			return order
		}
		return cmp.Compare(left.RoleID, right.RoleID)
	})
	encoded, err := json.Marshal(finish)
	if err != nil {
		return WorldBossFinish{}, "", err
	}
	return finish, Hash(encoded), nil
}

func worldBossHexID(value string, length int) bool {
	return len(value) == length && strings.Trim(value, "0123456789abcdef") == ""
}
