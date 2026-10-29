package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"time"
)

type ExperienceGrantReceipt = domain.ExperienceGrantReceipt

const RoleProgressionFormatVersion uint32 = 1

func GrowthContract(f devconfig.Fixture) (domain.GrowthContract, error) {
	rules := f.Progression
	c := domain.GrowthContract{FormatVersion: RoleProgressionFormatVersion, CurveVersion: rules.CurveVersion, InitialLevelCap: rules.InitialLevelCap, RebirthLevelIncrement: rules.RebirthLevelCapIncrement, OperatingLevelCap: rules.OperatingLevelCap, ExpPerLevel: rules.ExpPerLevel}
	if f.RuleVersion < 20 || rules.CatalogMaxLevel != uint32(len(rules.ExpPerLevel)) || rules.RebirthEnabled || rules.RebirthResetProgression || rules.PendingPoolPolicy != "one_next_level" || rules.NewOverflowPolicy != "reject_new_with_receipt" || rules.LegacyOverflowPolicy != "preserve_frozen" || rules.LevelUpResources != "source_full_if_alive_original_birth" {
		return c, Fail(p.CodeOperationConflict, "成长合同不完整或不属于当前开放范围")
	}
	return c, c.Validate()
}

func ProgressionState(r domain.Role, f devconfig.Fixture) (*pb.ProgressionState, error) {
	if f.RuleVersion < 20 {
		return nil, nil
	}
	c, err := GrowthContract(f)
	if err != nil {
		return nil, err
	}
	cap, err := c.EffectiveCap(r.RebirthCount)
	if err != nil {
		return nil, err
	}
	next, poolCap := int64(0), c.ExpPerLevel[cap-1]
	unknown := r.ProgressionMigrationRequired || r.ProgressionCurveVersion != c.CurveVersion
	if !unknown && r.Level > 0 && uint64(r.Level) <= uint64(len(c.ExpPerLevel)) {
		next = c.ExpPerLevel[r.Level-1]
	}
	if unknown {
		poolCap = 0
	}
	return &pb.ProgressionState{FormatVersion: r.ProgressionFormatVersion, CurveVersion: c.CurveVersion, LevelCap: cap, RebirthCount: r.RebirthCount, NextLevelExperience: next, PendingExperience: r.PendingExperience, PendingExperienceLimit: poolCap, LegacyExperienceOverflow: r.LegacyExperienceOverflow, AtCap: r.Level >= cap, MigrationRequired: unknown, RebirthEnabled: f.Progression.RebirthEnabled, CatalogMaxLevel: f.Progression.CatalogMaxLevel}, nil
}

func experienceWithContract(r domain.Role, raw int64, c domain.GrowthContract) (domain.Role, *ExperienceGrantReceipt, error) {
	out, receipt, err := domain.ApplyExperience(r, raw, c)
	if err != nil {
		return r, nil, Fail(p.CodeOperationConflict, "角色成长规则或余额无效："+err.Error())
	}
	if receipt.MigrationBefore != nil && out.Report.RawExperience == 0 && out.Report.Experience > 0 && out.Report.RejectedExperience == 0 && out.Report.PooledExperience == 0 {
		out.Report.RawExperience = out.Report.Experience
	}
	return out, &receipt, nil
}

// 入场迁移附在原接管事务；不占击杀SourceSeq，也不伪装新奖励或恢复现场。
func (s *Store) recordProgressionMigration(tc context.Context, r domain.Role, grant *ExperienceGrantReceipt) error {
	if grant == nil || grant.MigrationBefore == nil {
		return nil
	}
	id := fmt.Sprintf("progression_migration:%s:%d", r.ID, r.ProgressionFormatVersion)
	raw, err := json.Marshal(grant)
	if err != nil {
		return err
	}
	op := Operation{ID: id, Digest: Hash(raw), RoleID: r.ID, Revision: r.Revision, CommandSeq: r.EconomicSeq, Kind: "progression_migration", Created: time.Now(), Experience: grant}
	if _, err = s.DB.Collection("operations").InsertOne(tc, op); err != nil {
		return err
	}
	_, err = s.DB.Collection("audit_outbox").InsertOne(tc, bson.M{"_id": id, "kind": "progression_migration", "role_id": r.ID, "revision": r.Revision, "source_seq_before": r.SourceSeq, "experience_grant": grant, "rule_version": s.Cfg.Fixture.RuleVersion, "config_hash": s.Cfg.Fixture.Hash, "at": time.Now()})
	return err
}
