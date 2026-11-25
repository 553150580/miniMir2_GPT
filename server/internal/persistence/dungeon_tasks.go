// 本文件按原成功场次的真实击杀推进讨伐任务、成就和图鉴。
package persistence

import (
	"cmp"
	"slices"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// DungeonKillEvidence 由战斗分片在首次死亡时建立。怪物编号从原模板查取。
type DungeonKillEvidence struct {
	Stage int       `bson:"stage"` // 阵容所在段的零基索引。
	Enemy int       `bson:"enemy"` // 该段阵容中的零基索引。
	At    time.Time `bson:"at"`    // 权威命中实际结算的时刻。
}

// normalizeDungeonKills 按业务时刻和模板顺序固定终态摘要，统一数据库的毫秒精度。
func normalizeDungeonKills(outcome DungeonOutcome, evidence []DungeonKillEvidence) ([]DungeonKillEvidence, error) {
	if len(evidence) > devconfig.MaxPersonalGridCells || outcome != DungeonSucceeded && len(evidence) != 0 {
		return nil, Fail(p.CodeInvalidArgument, "讨伐链击杀证据超出阵容上限，或对应场次未通关")
	}
	kills := slices.Clone(evidence)
	for i := range kills {
		kills[i].At = bossTime(kills[i].At)
	}
	slices.SortFunc(kills, func(left, right DungeonKillEvidence) int {
		if order := left.At.Compare(right.At); order != 0 {
			return order
		}
		if order := cmp.Compare(left.Stage, right.Stage); order != 0 {
			return order
		}
		return cmp.Compare(left.Enemy, right.Enemy)
	})
	return kills, nil
}

// validDungeonKills 确认每段原怪物各有一次死亡，且最后一次死亡正是原清场时刻。
func validDungeonKills(record dungeonRecord, q DungeonFinish, template *devconfig.DungeonConfig) bool {
	var offsets [4]int // 四段在原场次阵容中的累计起点。
	if len(template.Stages) != len(offsets) {
		return false
	}
	total := 0
	for i, stage := range template.Stages {
		if len(stage.Enemies) == 0 {
			return false
		}
		offsets[i], total = total, total+len(stage.Enemies)
	}
	if len(q.KillEvidence) != total || total > devconfig.MaxPersonalGridCells || !q.KillEvidence[total-1].At.Equal(q.FinishedAt) {
		return false
	}
	var seen [devconfig.MaxPersonalGridCells]bool // 根据段落和阵容槽编号识别同一只怪物。
	previousStage := 0
	for _, kill := range q.KillEvidence {
		if kill.Stage < previousStage || kill.Stage >= len(offsets) || kill.Enemy < 0 ||
			kill.At.IsZero() || kill.At.Before(record.StartedAt) || kill.At.After(q.FinishedAt) ||
			kill.Enemy >= len(template.Stages[kill.Stage].Enemies) {
			return false
		}
		index := offsets[kill.Stage] + kill.Enemy
		if seen[index] || kill.Stage == len(offsets)-1 && kill.Enemy == 0 && !q.RewardAt.IsZero() && !kill.At.Equal(q.RewardAt) {
			return false
		}
		seen[index], previousStage = true, kill.Stage
	}
	return true
}

// advanceDungeonTasks 只更新原真人账本。失败、退款和已确认终态的重放均不写进度。
func (s *Store) advanceDungeonTasks(record *dungeonRecord, q DungeonFinish, roles []domain.Role) error {
	if q.Outcome != DungeonSucceeded {
		return nil
	}
	f := &s.Cfg.Fixture
	template, ok := f.DungeonTemplateByID(record.TemplateID)
	if !ok || record.RuleVersion != f.RuleVersion || record.ConfigHash != f.Hash {
		return Fail(p.CodeOperationConflict, "讨伐链原阵容及挑战规则不可用")
	}
	if !f.HasDungeonTaskTargets(template) && len(q.KillEvidence) == 0 {
		return nil
	}
	if !validDungeonKills(*record, q, template) {
		return Fail(p.CodeInvalidArgument, "讨伐链未提供完整且唯一的原怪物死亡证据")
	}
	record.KillEvidence = q.KillEvidence
	for i, role := range roles {
		for _, kill := range q.KillEvidence {
			monster := template.Stages[kill.Stage].Enemies[kill.Enemy].Monster.ID
			role = advanceChallengeMonsterTasks(role, f, record.TemplateID, template.Map.ID, monster, kill.At)
		}
		roles[i] = advanceChallengeTasks(role, f, devconfig.TaskCodexDungeonClear, record.TemplateID, template.Map.ID, 0, q.FinishedAt)
	}
	return nil
}
