// 本文件按出矿原件和角色设置决定每轮矿石的回收及对应金币钱包。
package persistence

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// applyMiningAutoRecycle 仅回收本次新产出的合格矿石。结算超限或收入为零时保留原矿。
func applyMiningAutoRecycle(role *domain.Role, fixture *devconfig.Fixture, ore *domain.Item, hosted bool) *domain.Wallet {
	if ore == nil || !role.MiningAutoRecycle {
		return nil
	}
	price, allowed := fixture.MiningAutoRecyclePrice(ore.ItemID)
	if !allowed {
		return nil
	}
	amount := mineralBuybackUnitPrice(fixture, price, *ore)
	if amount <= 0 {
		return nil
	}
	reward, wallet := domain.Wallet{}, role.Wallet
	balance := &wallet.Gold
	if ore.Bound {
		reward.BoundGold, balance = amount, &wallet.BoundGold
	} else {
		reward.Gold = amount
	}
	var err error
	*balance, err = add(*balance, amount)
	if err != nil {
		return nil
	}
	if hosted {
		// 使用副本核对所有战报字段，避免任一字段溢出后留下部分回收纪录。
		report := role.Report
		for _, field := range []struct {
			total *int64
			delta int64
		}{
			{&report.MiningRecycleCount, 1},
			{&report.MiningRecycleGold, reward.Gold}, {&report.MiningRecycleBoundGold, reward.BoundGold},
			{&report.Earned.Gold, reward.Gold}, {&report.Earned.BoundGold, reward.BoundGold},
		} {
			*field.total, err = add(*field.total, field.delta)
			if err != nil {
				return nil
			}
		}
		role.Report = report
	}
	role.Wallet = wallet
	return &reward
}
