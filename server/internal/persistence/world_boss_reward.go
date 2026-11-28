// 本文件按世界首领的原贡献、席位状态和击杀时特权冻结每个角色的永久奖励附件。
package persistence

import (
	"context"
	"fmt"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	worldBossRewardCollection = "world_boss_rewards" // 独立附件永久保存，领取后保留原发放和到账身份。
	worldBossKillReward       = "world_boss"         // 实际击杀触发的参与与排名奖励。
	worldBossTimeoutReward    = "world_boss_timeout" // 超时独立配置的绑定参与附件。
	worldBossSoulReward       = "world_boss_soul"    // 独立于背包的同名角色奖项。
)

// WorldBossPresence 保存首领终态时已获准的席位状态，由当前世界分片裁定。
type WorldBossPresence uint32

const (
	WorldBossPresenceOnline   WorldBossPresence = iota + 1 // 当前已确认并在线的席位。
	WorldBossPresenceOffline                               // 原席待新连接恢复，原贡献仍有效。
	WorldBossPresenceLeft                                  // 玩家主动退出，原有效伤害仍参与历史排行。
	WorldBossPresenceDefeated                              // 身体战败结束席位，原有效伤害仍参与历史排行。
)

// WorldBossRewardEvidence 绑定角色永久入口与席位权益，不靠领取时配置或当前卡状态推导。
type WorldBossRewardEvidence struct {
	Origin         uint64            `bson:"origin"`                    // 角色创建时的永久入口服。
	Presence       WorldBossPresence `bson:"presence"`                  // 原场次终态时的席位状态。
	PrivilegeUntil time.Time         `bson:"privilege_until,omitempty"` // 已确认的原特权卡截止时间。
}

// writeWorldBossRewards 将附件插入确认同一场终态的事务。事务重试按稳定场次键得到相同实例。
func (s *Store) writeWorldBossRewards(ctx context.Context, spec WorldBossSpec, finish WorldBossFinish, rules *devconfig.WorldBossRewards) error {
	rows, err := freezeWorldBossRewards(&s.Cfg.Fixture, spec, finish, rules)
	if err != nil || len(rows) == 0 {
		return err
	}
	_, err = s.DB.Collection(worldBossRewardCollection).InsertMany(ctx, rows)
	return err
}

// freezeWorldBossRewards 只消耗已经校验、排序、不可变的场次贡献。未配置或中断场不产生奖励。
func freezeWorldBossRewards(f *devconfig.Fixture, spec WorldBossSpec, finish WorldBossFinish, rules *devconfig.WorldBossRewards) ([]externalChallengeRewardRecord, error) {
	if rules == nil || finish.Outcome != WorldBossKilled && finish.Outcome != WorldBossExpired {
		return nil, nil
	}
	participation, sourceKind := rules.Participation, worldBossKillReward
	var soulDrop devconfig.SoulDrop
	if finish.Outcome == WorldBossKilled && f.Souls != nil {
		soulDrop = f.Souls.World[spec.TemplateID]
	}
	if finish.Outcome == WorldBossExpired {
		participation, sourceKind = rules.TimeoutParticipation, worldBossTimeoutReward
		if len(participation) == 0 {
			return nil, nil
		}
	}
	var rewards []externalChallengeRewardRecord // 每位有效角色最多一笔物品和一笔独立魂兽。
	for index, row := range finish.Contributions {
		if row.Damage < rules.MinimumDamage {
			continue
		}
		rank := uint32(index + 1) // 贡献已按实际有效伤害降序，资格筛选后仍保留原名次。
		var ranking []devconfig.ChallengeRewardItem
		if finish.Outcome == WorldBossKilled {
			for _, tier := range rules.Ranks {
				if rank >= tier.FirstRank && rank <= tier.LastRank {
					ranking = tier.Items
					break
				}
			}
		}
		if len(participation) == 0 && len(ranking) == 0 && soulDrop.TypeID == 0 {
			continue
		}
		evidence := row.RewardEvidence
		if evidence == nil {
			return nil, Fail(p.CodeInvalidArgument, "世界首领缺少有效贡献的原奖励证据")
		}
		if !worldBossRewardPresenceAllowed(evidence.Presence, rules) {
			continue
		}
		id := Hash(fmt.Appendf(nil, "world_boss_reward:%d:%s:%s", spec.RunID, spec.OccurrenceID, row.RoleID))[:32]
		bound := finish.Outcome != WorldBossKilled || !evidence.PrivilegeUntil.After(finish.FinishedAt)
		items := make([]domain.Item, 0, len(participation)+len(ranking))
		for _, group := range [][]devconfig.ChallengeRewardItem{participation, ranking} {
			for _, line := range group {
				definition, known := f.Item(line.ItemID)
				if !known {
					return nil, Fail(p.CodeInvalidArgument, "世界首领奖励原物品已失效")
				}
				itemID := Hash(fmt.Appendf(nil, "world_boss_reward_item:%s:%d", id, len(items)))[:32]
				item := domain.Item{ID: itemID, ItemID: line.ItemID, Quantity: line.Quantity, Bound: bound, Version: 1,
					SourceKind: sourceKind, SourceID: spec.OccurrenceID, Quality: definition.DropQuality}
				items = append(items, InitializeItemDurability(item, f))
			}
		}
		attachment := domain.ChallengePendingReward{
			RewardID: id, SourceKind: sourceKind, SourceID: spec.OccurrenceID, Origin: evidence.Origin, TemplateID: spec.TemplateID,
			RuleVersion: spec.RuleVersion, EarnedAt: finish.FinishedAt, Items: items, SourceRunID: spec.RunID, SourceRank: rank,
			ConfigHash: spec.ConfigHash, PrivilegeUntil: evidence.PrivilegeUntil,
		}
		if len(items) != 0 {
			rewards = append(rewards, externalChallengeRewardRecord{ID: id, RoleID: row.RoleID, ChallengePendingReward: attachment})
		}
		if award := freezeSoulAward(f.Souls, soulDrop, row.ReservationID); award != nil {
			soulID := Hash([]byte("world_boss_soul:" + id))[:32]
			attachment.RewardID, attachment.SourceKind, attachment.Items, attachment.Soul = soulID, worldBossSoulReward, nil, award
			rewards = append(rewards, externalChallengeRewardRecord{ID: soulID, RoleID: row.RoleID, ChallengePendingReward: attachment})
		}
	}
	return rewards, nil
}

// worldBossRewardPresenceAllowed 的席位门槛独立于最低有效伤害和特权。
func worldBossRewardPresenceAllowed(presence WorldBossPresence, rules *devconfig.WorldBossRewards) bool {
	switch presence {
	case WorldBossPresenceOnline:
		return true
	case WorldBossPresenceOffline:
		return rules.AllowDisconnected
	case WorldBossPresenceLeft:
		return rules.AllowWithdrawn
	case WorldBossPresenceDefeated:
		return rules.AllowDefeated
	default:
		return false
	}
}
