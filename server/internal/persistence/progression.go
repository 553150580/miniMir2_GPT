// 成长配置与数据库角色经验的适配。按纯领域成长合同计算实际得失，生成客户端展示状态、经验回执及迁移审计。

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

// ExperienceGrantReceipt 沿用领域成长结果的统一回执类型，供多个持久化流程共享。
type ExperienceGrantReceipt = domain.ExperienceGrantReceipt

// RoleProgressionFormatVersion 规定角色成长格式版本的初始取值。
const RoleProgressionFormatVersion uint32 = 1

// GrowthContract 从当前玩法配置生成领域层经验成长所需的版本、等级和曲线约束。
// f 记录开发测试配置。
func GrowthContract(f devconfig.Fixture) (domain.GrowthContract, error) {
	// rules 保存成长，用于当前步骤中的规则集合。
	rules := f.Progression
	// c 组装成长规则合同所需的数据。
	c := domain.GrowthContract{FormatVersion: RoleProgressionFormatVersion, CurveVersion: rules.CurveVersion, InitialLevelCap: rules.InitialLevelCap, RebirthLevelIncrement: rules.RebirthLevelCapIncrement, OperatingLevelCap: rules.OperatingLevelCap, ExpPerLevel: rules.ExpPerLevel}
	if f.RuleVersion < 20 || rules.CatalogMaxLevel != uint32(len(rules.ExpPerLevel)) || rules.RebirthEnabled || rules.RebirthResetProgression || rules.PendingPoolPolicy != "one_next_level" || rules.NewOverflowPolicy != "reject_new_with_receipt" || rules.LegacyOverflowPolicy != "preserve_frozen" || rules.LevelUpResources != "source_full_if_alive_original_birth" {
		return c, Fail(p.CodeOperationConflict, "成长合同不完整或不属于当前开放范围")
	}
	return c, c.Validate()
}

// ProgressionState 把角色的当前成长状态投影成客户端使用的协议视图。
// r 记录角色，f 记录开发测试配置。
func ProgressionState(r domain.Role, f devconfig.Fixture) (*pb.ProgressionState, error) {
	if f.RuleVersion < 20 {
		return nil, nil
	}
	// c 接收成长规则合同供后续步骤使用，err 接收成长规则合同过程中可能产生的错误，供后续失败分支处理。
	c, err := GrowthContract(f)
	if err != nil {
		return nil, err
	}
	// cap 保存由有效上限取得的上限。
	cap, err := c.EffectiveCap(r.RebirthCount)
	if err != nil {
		return nil, err
	}
	// next 保存由整数“64”取得的下一状态，poolCap 保存从经验“per”等级取出的连接或数值池上限。
	next, poolCap := int64(0), c.ExpPerLevel[cap-1]
	// unknown 保存根据成长迁移“required”与成长“curve”版本与“curve”版本的计算结果计算的未知。
	unknown := r.ProgressionMigrationRequired || r.ProgressionCurveVersion != c.CurveVersion
	if !unknown && r.Level > 0 && uint64(r.Level) <= uint64(len(c.ExpPerLevel)) {
		next = c.ExpPerLevel[r.Level-1]
	}
	if unknown {
		poolCap = 0
	}
	return &pb.ProgressionState{FormatVersion: r.ProgressionFormatVersion, CurveVersion: c.CurveVersion, LevelCap: cap, RebirthCount: r.RebirthCount, NextLevelExperience: next, PendingExperience: r.PendingExperience, PendingExperienceLimit: poolCap, LegacyExperienceOverflow: r.LegacyExperienceOverflow, AtCap: r.Level >= cap, MigrationRequired: unknown, RebirthEnabled: f.Progression.RebirthEnabled, CatalogMaxLevel: f.Progression.CatalogMaxLevel}, nil
}

// experienceWithContract 让领域合同计算经验结果，在需要时更新角色的成长格式与等级。
// r 记录角色，raw 记录原始值，使用数值表示，c 记录成长规则合同。
func experienceWithContract(r domain.Role, raw int64, c domain.GrowthContract) (domain.Role, *ExperienceGrantReceipt, error) {
	// out 保存由应用经验取得的输出，receipt 保存由应用经验取得的回执，err 接收应用经验过程中可能产生的错误，供后续失败分支处理。
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
// recordProgressionMigration 只有角色实际发生格式迁移时，才保存可审计的成长迁移记录。
// s 是调用该方法的持久层存储实例，tc 记录MongoDB 事务上下文，使用请求或事务上下文表示，r 记录角色，grant 记录授予，使用经验赋予道具或权限回执表示。
func (s *Store) recordProgressionMigration(tc context.Context, r domain.Role, grant *ExperienceGrantReceipt) error {
	if grant == nil || grant.MigrationBefore == nil {
		return nil
	}
	// id 区分当前步骤指向的目标对象。
	id := fmt.Sprintf("progression_migration:%s:%d", r.ID, r.ProgressionFormatVersion)
	// raw 保存由序列化取得的原始值，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	raw, err := json.Marshal(grant)
	if err != nil {
		return err
	}
	// op 组装操作所需的数据。
	op := Operation{ID: id, Digest: Hash(raw), RoleID: r.ID, Revision: r.Revision, CommandSeq: r.EconomicSeq, Kind: "progression_migration", Created: time.Now(), Experience: grant}
	if _, err = s.DB.Collection("operations").InsertOne(tc, op); err != nil {
		return err
	}
	_, err = s.DB.Collection("audit_outbox").InsertOne(tc, bson.M{"_id": id, "kind": "progression_migration", "role_id": r.ID, "revision": r.Revision, "source_seq_before": r.SourceSeq, "experience_grant": grant, "rule_version": s.Cfg.Fixture.RuleVersion, "config_hash": s.Cfg.Fixture.Hash, "at": time.Now()})
	return err
}
