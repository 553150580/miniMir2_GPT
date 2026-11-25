// 本文件核验显式隔离装配的镜像赛季奖表，不提供默认正式奖量。
package devconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
)

// MirrorArenaRankingEarlierLastChallenge 使用工作簿已定的同分判定规则。
const MirrorArenaRankingEarlierLastChallenge = "earlier_last_challenge"

// maxMirrorSeasonRankTiers 限定隔离装配的档位条数，不限制后台全量积分结转人数。
const maxMirrorSeasonRankTiers = 32

// MirrorArenaSeasonSettlement 保存已定同分规则、参与门槛和显式隔离奖励。
type MirrorArenaSeasonSettlement struct {
	RankingTiebreak            string                  `json:"ranking_tiebreak"`             // 同分由最后挑战时间较早者优先。
	ParticipationMinChallenges uint32                  `json:"participation_min_challenges"` // 达到参与奖门槛的有效结算场数。
	Ranks                      []MirrorArenaSeasonRank `json:"ranks"`                        // 由起始名次递增的封账派奖区间。
	Participation              TaskCodexReward         `json:"participation"`                // 与名次无关的一份绑定参与奖。
}

// MirrorArenaSeasonRank 表示一段包含两端名次的固定绑定奖励。
type MirrorArenaSeasonRank struct {
	FirstRank uint32          `json:"first_rank"` // 包含本档起始名次，从第一名计数。
	LastRank  uint32          `json:"last_rank"`  // 包含本档末名，不与下一档重叠。
	Reward    TaskCodexReward `json:"reward"`     // 复用任务奖励的绑定元宝定点单位。
}

// MirrorArenaExchangeRule 保存显式候选兑换商品，发放物品始终绑定。
type MirrorArenaExchangeRule struct {
	ItemID   uint32 `json:"item_id"`  // 已知非装备、可堆叠的物品定义。
	Price    int64  `json:"price"`    // 每份商品消耗的兑换积分。
	Quantity int64  `json:"quantity"` // 每份商品得到的整数件数。
}

// UnmarshalJSON 防止未知称号或头像框字段在读取奖表时被悄悄忽略。
func (s *MirrorArenaSeasonSettlement) UnmarshalJSON(raw []byte) error {
	type definition MirrorArenaSeasonSettlement
	return decodeMirrorSeasonJSON(raw, (*definition)(s))
}

// UnmarshalJSON 拒绝任何试图为绑定兑换附加新行为的字段。
func (e *MirrorArenaExchangeRule) UnmarshalJSON(raw []byte) error {
	type definition MirrorArenaExchangeRule
	return decodeMirrorSeasonJSON(raw, (*definition)(e))
}

func decodeMirrorSeasonJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// clone 复制已验证的固定奖励档，装配后的结算计划不引用原配置切片。
func (s *MirrorArenaSeasonSettlement) clone() *MirrorArenaSeasonSettlement {
	if s == nil {
		return nil
	}
	snapshot := *s
	snapshot.Ranks = slices.Clone(s.Ranks)
	return &snapshot
}

// validateMirrorArenaSettlement 保持旧夹具可读，只允许开发候选引用可发放的整数绑定资产。
func (f *Fixture) validateMirrorArenaSettlement(definition *MirrorArenaDefinition) error {
	rules := definition.Settlement
	if rules == nil {
		if definition.Exchange != nil {
			return fmt.Errorf("镜像竞技开启兑换前须装配赛季结算奖表")
		}
		return nil
	}
	if !f.DevelopmentOnly || f.RuntimeReady {
		return fmt.Errorf("镜像赛季结算奖表只允许在独立开发候选中装配")
	}
	if rules.RankingTiebreak != MirrorArenaRankingEarlierLastChallenge ||
		rules.ParticipationMinChallenges == 0 ||
		uint64(rules.ParticipationMinChallenges) > uint64(definition.SeasonDays)*uint64(definition.DailyLimit) ||
		len(rules.Ranks) == 0 || len(rules.Ranks) > maxMirrorSeasonRankTiers ||
		!validMirrorSeasonReward(rules.Participation) {
		return fmt.Errorf("镜像赛季同分规则、参与门槛或奖励档不合法")
	}
	var previousRank uint32 // 前一档包含的最后名次。
	for _, rank := range rules.Ranks {
		if rank.FirstRank <= previousRank || rank.LastRank < rank.FirstRank || !validMirrorSeasonReward(rank.Reward) ||
			rank.Reward.BoundYuanbaoUnits > math.MaxInt64-rules.Participation.BoundYuanbaoUnits {
			return fmt.Errorf("镜像赛季奖励档 %d–%d 重叠或奖励数量不合法", rank.FirstRank, rank.LastRank)
		}
		previousRank = rank.LastRank
	}
	if exchange := definition.Exchange; exchange != nil {
		item, known := f.Item(exchange.ItemID)
		if !known || item.IsEquipment || !item.Stackable || exchange.Price <= 0 ||
			!item.validChallengeRewardQuantity(exchange.Quantity) {
			return fmt.Errorf("镜像竞技绑定兑换商品、数量或积分价格不合法")
		}
	}
	return nil
}

// validMirrorSeasonReward 限定当前账本实际可确认的奖励。称号和头像框需先有正式目录。
func validMirrorSeasonReward(reward TaskCodexReward) bool {
	return reward.BoundYuanbaoUnits > 0 && reward.BoundGold == 0 && len(reward.Items) == 0
}
