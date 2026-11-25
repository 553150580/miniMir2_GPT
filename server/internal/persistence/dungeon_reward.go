// 本文件按原讨伐队员独立抽选奖励，在原场次终态事务中冻结可整笔领取的物品。
package persistence

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	dungeonRewardCollection = "dungeon_rewards" // 仅保存有实际附件的独立讨伐奖励。
	dungeonRewardSource     = "dungeon"         // 原通关物品及待领附件的来源。
)

// DungeonRewardEvidence 记录实际最终首领击杀时观察到的某个真实队员的权益。
type DungeonRewardEvidence struct {
	RoleID         string    `bson:"role_id"`                   // 与已持久的扣次真人成员对应。
	PrivilegeUntil time.Time `bson:"privilege_until,omitempty"` // 击杀时能够确认的原特权截止。
}

// dungeonRewardDecision 记录原队员的抽选事实。抽到空包的成员也可沿原场审计。
type dungeonRewardDecision struct {
	DungeonRewardEvidence `bson:",inline"`
	Option                int    `bson:"option"`              // 原版本奖表中以零开始的选项编号。
	RewardID              string `bson:"reward_id,omitempty"` // 非空结果对应的领取附件编号。
}

// writeDungeonRewards 只为开始时有奖且实际通关的原场次写附件。未知提交重放在进入这里之前对账。
func (s *Store) writeDungeonRewards(ctx context.Context, record *dungeonRecord, q DungeonFinish) error {
	if q.Outcome != DungeonSucceeded || record.RewardSeed == "" {
		if !q.RewardAt.IsZero() || len(q.RewardEvidence) != 0 {
			return Fail(p.CodeInvalidArgument, "未获奖励的讨伐终态不能提交奖励证据")
		}
		return nil
	}
	fixture := &s.Cfg.Fixture
	template, ok := fixture.DungeonTemplateByID(record.TemplateID)
	if !ok || template.Rewards == nil || record.RuleVersion != fixture.RuleVersion || record.ConfigHash != fixture.Hash {
		return Fail(p.CodeOperationConflict, "讨伐链原奖励版本不可用")
	}
	rows, decisions, err := freezeDungeonRewards(fixture, *record, q, template.Rewards)
	if err != nil {
		return err
	}
	record.RewardAt, record.RewardDecisions = q.RewardAt, decisions
	if len(rows) != 0 {
		_, err = s.DB.Collection(dungeonRewardCollection).InsertMany(ctx, rows)
	}
	return err
}

// freezeDungeonRewards 校验原名单和击杀时刻，再从同一保存种子、不同角色键独立抽取一份结果。
func freezeDungeonRewards(fixture *devconfig.Fixture, record dungeonRecord, q DungeonFinish,
	rules *devconfig.DungeonRewards) ([]externalChallengeRewardRecord, []dungeonRewardDecision, error) {
	if q.RewardAt.IsZero() || q.RewardAt.Before(record.StartedAt) || q.RewardAt.After(q.FinishedAt) ||
		len(q.RewardEvidence) != len(record.Members) {
		return nil, nil, Fail(p.CodeInvalidArgument, "讨伐奖励缺少原击杀时间或完整真人名单")
	}
	for i, evidence := range q.RewardEvidence {
		if evidence.RoleID != record.Members[i].RoleID {
			return nil, nil, Fail(p.CodeInvalidArgument, "讨伐奖励原真人名单不一致")
		}
	}
	key := domain.Bytes(record.RewardSeed) // 仅存原预约，重试和并发结算产生相同结果。
	var total uint64                       // 有界奖表的所有选项权重之和。
	for _, option := range rules.Options {
		total += uint64(option.Weight)
	}
	if len(key) != 16 || total == 0 {
		return nil, nil, Fail(p.CodeOperationConflict, "讨伐奖励抽选种子或原奖表失效")
	}
	rows := make([]externalChallengeRewardRecord, 0, len(record.Members))
	decisions := make([]dungeonRewardDecision, 0, len(record.Members))
	for i, member := range record.Members {
		evidence := q.RewardEvidence[i]
		roll := rollDungeonReward(key, member.RoleID, total)
		var optionIndex int // 权重区间包含其左端，右端由下一个选项负责。
		for optionIndex = range rules.Options {
			weight := uint64(rules.Options[optionIndex].Weight)
			if roll < weight {
				break
			}
			roll -= weight
		}
		decision := dungeonRewardDecision{DungeonRewardEvidence: evidence, Option: optionIndex}
		option := rules.Options[optionIndex]
		if len(option.Items) > 0 {
			id := Hash(fmt.Appendf(nil, "dungeon_reward:%d:%s:%s", record.RunID, record.ChallengeID, member.RoleID))[:32]
			items, err := freezeDungeonRewardItems(fixture, record.ChallengeID, id, option.Items, evidence.PrivilegeUntil.After(q.RewardAt))
			if err != nil {
				return nil, nil, err
			}
			decision.RewardID = id
			rows = append(rows, externalChallengeRewardRecord{ID: id, RoleID: member.RoleID, ChallengePendingReward: domain.ChallengePendingReward{
				RewardID: id, SourceKind: dungeonRewardSource, SourceID: record.ChallengeID, Origin: member.Origin,
				TemplateID: record.TemplateID, RuleVersion: record.RuleVersion, EarnedAt: q.RewardAt, Items: items,
				SourceRunID: record.RunID, ConfigHash: record.ConfigHash, PrivilegeUntil: evidence.PrivilegeUntil,
			}})
		}
		decisions = append(decisions, decision)
	}
	return rows, decisions, nil
}

// rollDungeonReward 使用服务端密钥派生角色独立随机值，拒绝不均匀余数区间。
func rollDungeonReward(key []byte, roleID string, total uint64) uint64 {
	mac := hmac.New(sha256.New, key)
	threshold := -total % total // 无符号运算保留 2^64 对界限取模后的余数。
	var retry [8]byte           // 极少出现偏差区间时用计数器确定性重取。
	for index := uint64(0); ; index++ {
		mac.Reset()
		mac.Write([]byte("dungeon_reward_draw:"))
		mac.Write([]byte(roleID))
		binary.BigEndian.PutUint64(retry[:], index)
		mac.Write(retry[:])
		value := binary.BigEndian.Uint64(mac.Sum(nil))
		if value >= threshold {
			return value % total
		}
	}
}

// freezeDungeonRewardItems 保存最终物品实例，权益类型由原来源配置确定。
func freezeDungeonRewardItems(fixture *devconfig.Fixture, sourceID, rewardID string, options []devconfig.DungeonRewardItem,
	privilege bool) ([]domain.Item, error) {
	items := make([]domain.Item, 0, len(options))
	for _, line := range options {
		definition, ok := fixture.Item(line.ItemID)
		if !ok || line.BindPolicy != devconfig.DungeonRewardAlwaysBound && line.BindPolicy != devconfig.DungeonRewardBindOnKill {
			return nil, Fail(p.CodeOperationConflict, "讨伐奖励原物品或绑定规则失效")
		}
		id := Hash(fmt.Appendf(nil, "dungeon_reward_item:%s:%d", rewardID, len(items)))[:32]
		item := domain.Item{ID: id, ItemID: line.ItemID, Quantity: line.Quantity, Version: 1, Quality: definition.DropQuality,
			SourceKind: dungeonRewardSource, SourceID: sourceID, Bound: line.BindPolicy == devconfig.DungeonRewardAlwaysBound || !privilege}
		items = append(items, InitializeItemDurability(item, fixture))
	}
	return items, nil
}
