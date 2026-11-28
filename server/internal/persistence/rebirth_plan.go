// 本文件选定背包的凭证与绑定材料原堆，并验证真实的个人首领胜利。
package persistence

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// rebirthSpend 保存确认时将使用的原堆及扣量。
type rebirthSpend struct {
	ID       string `bson:"instance_id"` // 原背包实例。
	ItemID   uint32 `bson:"item_id"`     // 原目录物品。
	Version  int64  `bson:"version"`     // 扣除前的原堆版本。
	Quantity int64  `bson:"quantity"`    // 本次只扣这个数量。
}

// rebirthPlan 在报价和首次确认时独立计算，任何有影响的状态差异均须重新报价。
type rebirthPlan struct {
	Transition  domain.RebirthTransition `bson:"transition"`   // 角色原等级、解锁上限及经验前后值。
	ChallengeID string                   `bson:"challenge_id"` // 凭证对应的真实个人首领场次。
	Credential  rebirthSpend             `bson:"credential"`   // 本档的一份确切原凭证。
	Materials   []rebirthSpend           `bson:"materials"`    // 按背包顺序扣除的各个绑定材料堆。
	Spend       domain.Wallet            `bson:"spend"`        // 普通和绑定金币的实际拆分。
}

// buildRebirthPlan 计算转生后状态及投入明细，成功前只读取角色和原挑战。
func (s *Store) buildRebirthPlan(ctx context.Context, role domain.Role) (domain.Role, rebirthPlan, uint32, error) {
	f := &s.Cfg.Fixture
	if !f.Progression.RebirthEnabled || f.Rebirth == nil || uint64(role.RebirthCount) >= uint64(len(f.Rebirth.Stages)) {
		return role, rebirthPlan{}, p.CodeRebirthUnavailable, nil
	}
	stage := f.Rebirth.Stages[role.RebirthCount]
	if role.Level != stage.RequiredLevel || !equipmentRoleUnique(role) ||
		role.PersonalBoss != nil && role.PersonalBoss.ActiveChallengeID != "" {
		return role, rebirthPlan{}, p.CodeRebirthInputs, nil
	}
	contract, err := GrowthContract(*f)
	if err != nil {
		return role, rebirthPlan{}, 0, err
	}
	out, transition, err := domain.ApplyRebirth(role, contract)
	if err != nil {
		return role, rebirthPlan{}, p.CodeRebirthInputs, nil
	}
	boundGold := min(max(role.Wallet.BoundGold, 0), stage.GoldCost)
	plan := rebirthPlan{Transition: transition, Spend: domain.Wallet{BoundGold: boundGold, Gold: stage.GoldCost - boundGold}}
	if !vendorCanPay(role.Wallet, plan.Spend) {
		return role, rebirthPlan{}, p.CodeRebirthInputs, nil
	}
	proof, _ := f.Item(stage.CredentialItemID) // 装载时已验证的绑定凭证。
	material, _ := f.Item(stage.MaterialItemID)
	left := stage.MaterialCost // 每组只扣到规定费用，保留最后一堆余量。
	for _, item := range role.Items {
		if !item.Bound || !lowerHex(item.ID, 16) || !item.Refine.IsZero() {
			continue
		}
		if plan.Credential.ID == "" && validGrowthMaterial(item, proof) &&
			item.SourceKind == "personal_boss" && lowerHex(item.SourceID, 16) {
			plan.Credential = rebirthSpend{ID: item.ID, ItemID: item.ItemID, Version: item.Version, Quantity: 1}
			plan.ChallengeID = item.SourceID
		}
		if left > 0 && validGrowthMaterial(item, material) {
			take := min(left, item.Quantity)
			plan.Materials = append(plan.Materials, rebirthSpend{ID: item.ID, ItemID: item.ItemID, Version: item.Version, Quantity: take})
			left -= take
		}
	}
	if plan.Credential.ID == "" || left != 0 {
		return role, rebirthPlan{}, p.CodeRebirthInputs, nil
	}
	valid, err := s.verifyRebirthChallenge(ctx, role.ID, stage.ChallengeTemplateID, plan)
	if err != nil {
		return role, rebirthPlan{}, 0, err
	}
	if !valid {
		return role, rebirthPlan{}, p.CodeRebirthInputs, nil
	}
	uses := map[string]int64{plan.Credential.ID: 1} // 同一凭证和材料不重复扣除。
	for _, line := range plan.Materials {
		uses[line.ID] = line.Quantity
	}
	out.Items = spendGrowthMaterials(role.Items, uses)
	out.Wallet.BoundGold -= plan.Spend.BoundGold
	out.Wallet.Gold -= plan.Spend.Gold
	if !equipmentRoleArithmeticSafe(out, *f) || validateDungeonHireWallet(out) != nil {
		return role, rebirthPlan{}, p.CodeRebirthInputs, nil
	}
	return out, plan, 0, nil
}

// verifyRebirthChallenge 核对永久胜利记录及原凭证实例，别人的场次和伪来源均无效。
func (s *Store) verifyRebirthChallenge(ctx context.Context, roleID string, template uint32, plan rebirthPlan) (bool, error) {
	var record PersonalBossChallenge
	err := s.DB.Collection(personalBossCollection).FindOne(ctx, bson.M{"_id": plan.ChallengeID,
		"role_id": roleID, "template_id": template, "outcome": PersonalBossDefeated}).Decode(&record)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, item := range record.RewardItems {
		if item.ID == plan.Credential.ID && item.ItemID == plan.Credential.ItemID && item.Bound &&
			item.Quantity == 1 && item.SourceID == record.ChallengeID && item.SourceKind == "personal_boss" {
			return true, nil
		}
	}
	return false, nil
}

// proto 把确切的阶段结果、来源及投入转换成客户端可显示的原回执。
func (plan rebirthPlan) proto(quoteID string) *pb.RebirthReceipt {
	t := plan.Transition
	receipt := &pb.RebirthReceipt{QuoteId: domain.Bytes(quoteID), ChallengeId: domain.Bytes(plan.ChallengeID),
		Gold: plan.Spend.Gold, BoundGold: plan.Spend.BoundGold, Credential: plan.Credential.proto(),
		Transition: &pb.RebirthTransition{PreviousCap: t.PreviousCap, NewCap: t.NewCap, PreviousLevel: t.Before.Level,
			NewLevel: t.After.Level, NewCount: t.After.RebirthCount, ReleasedExperience: t.ReleasedXP, NewExperience: t.After.Experience}}
	for _, line := range plan.Materials {
		receipt.Materials = append(receipt.Materials, line.proto())
	}
	return receipt
}

// proto 发送实例、版本与扣量，客户端据此核对确认和重放结果。
func (line rebirthSpend) proto() *pb.RebirthSpend {
	return &pb.RebirthSpend{InstanceId: domain.Bytes(line.ID), ItemId: line.ItemID, Version: uint64(line.Version), Quantity: line.Quantity}
}
