// 本文件验证矿层、工具和时间，按原判定身份抽样并计算资产变化。
package persistence

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"slices"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const miningPPM = 1_000_000 // 挖矿成功与纯度档采用百万分比。

// validSingleMiningOre 验证有原挖矿来源及有效纯度的单块矿石。归属、绑定和操作占用由各入口校验。
func validSingleMiningOre(item domain.Item, definition devconfig.Item) bool {
	return item.SourceKind == "mining" && lowerHex(item.SourceID, 16) && !definition.IsEquipment && !definition.Stackable &&
		item.Quantity == 1 && item.OrePurity >= 1 && item.OrePurity <= 100
}

// validateMiningAttempt 核对楼层、等级、满包、完整时间间隔与离线托管余额。
func validateMiningAttempt(role domain.Role, fixture *devconfig.Fixture, q MiningAttemptRequest) (*devconfig.MiningLayer, time.Duration, error) {
	layer, ok := fixture.MiningLayer(q.MapID)
	if !ok || role.Level < layer.MinimumLevel {
		return nil, 0, Fail(p.CodeLineForbidden, "矿层未开放或角色等级不足")
	}
	if fixture.Mining.IntervalMS <= 0 || fixture.Mining.IntervalMS > math.MaxInt64/int64(time.Millisecond) || fixture.Mining.IntervalMS%1_000 != 0 {
		return nil, 0, Fail(p.CodeOperationConflict, "矿层判定间隔无效")
	}
	interval := time.Duration(fixture.Mining.IntervalMS) * time.Millisecond
	if q.At.Sub(q.StartedAt) < interval || !role.LastMiningAt.IsZero() && q.At.Sub(role.LastMiningAt) < interval {
		return nil, 0, Fail(p.CodeOperationConflict, "本轮尚未经过一个完整矿层判定间隔")
	}
	if q.HostingSession != "" && (q.HostSeconds != int64(interval/time.Second) || role.HostingSeconds < q.HostSeconds) {
		return nil, 0, Fail(p.CodeOperationConflict, "离线托管剩余时间不足一个完整挖矿间隔")
	}
	// 满包在抽样和耗锄之前停止。最后一次将耗尽锄头也必须先具备空位。
	if len(role.Items) >= role.Capacity {
		return nil, 0, Fail(p.CodeEquipmentBagFull, "背包已满，挖矿暂停")
	}
	return layer, interval, nil
}

// MiningToolForAttempt 从背包选择当前矿层可用的锄头，不使用战斗装备槽。
// 指定实例失效时由调度方决定是否停止或重新选择，查询本身不会改用其他锄头。
func MiningToolForAttempt(role domain.Role, fixture *devconfig.Fixture, mapID uint64, requestedID string) (domain.Item, bool) {
	if fixture == nil {
		return domain.Item{}, false
	}
	layer, ok := fixture.MiningLayer(mapID)
	if !ok {
		return domain.Item{}, false
	}
	for _, item := range role.Items {
		if requestedID != "" && item.ID != requestedID {
			continue
		}
		tool, ok := fixture.MiningTool(item.ItemID)
		if !ok || tool.Layer != layer.Layer || !lowerHex(item.ID, 16) || item.Quantity != 1 || item.OrePurity != 0 ||
			item.Version <= 0 || item.Version == math.MaxInt64 || item.Locked || equipmentOccupied(item) {
			continue
		}
		item = InitializeItemDurability(item, fixture)
		if item.DurabilityU > 0 && item.HasValidDurability() {
			return item, true
		}
	}
	return domain.Item{}, false
}

// sampleMiningOre 依次判定成功、矿种、纯度档与档内纯度，重试保持同一结果。
func sampleMiningOre(roleID, eventID string, layer uint32, fixture *devconfig.Fixture) (*domain.Item, error) {
	rules := fixture.Mining
	if rules == nil || layer < 1 || layer > 2 || rules.SuccessPPM == 0 || rules.SuccessPPM > miningPPM {
		return nil, Fail(p.CodeOperationConflict, "矿层产出规则无效")
	}
	seed := "minimir:mining:v1:" + roleID + ":" + eventID
	if sourceDraw(seed+":success", miningPPM) >= rules.SuccessPPM {
		return nil, nil
	}
	mineral := sourceDraw(seed+":ore", 100)
	var itemID uint32 // 按当前矿层分布选中的矿石定义。
	for _, ore := range rules.Ores {
		weight := ore.LayerWeights[layer-1]
		if mineral < weight {
			itemID = ore.ItemID
			break
		}
		mineral -= weight
	}
	def, exists := fixture.Item(itemID)
	if !exists || itemID == 0 {
		return nil, Fail(p.CodeOperationConflict, "矿种权重未解析为有效矿石")
	}
	purityChoice := sourceDraw(seed+":purity_band", miningPPM)
	for _, band := range rules.Purity {
		if purityChoice < band.WeightPPM {
			if band.Min == 0 || band.Max > 100 || band.Min > band.Max {
				break
			}
			purity := band.Min + sourceDraw(seed+":purity_value", band.Max-band.Min+1)
			digest := sha256.Sum256([]byte(seed + ":instance"))
			return &domain.Item{ID: hex.EncodeToString(digest[:16]), ItemID: itemID, Quantity: 1, OrePurity: purity,
				Version: 1, SourceKind: "mining", SourceID: eventID, Quality: def.DropQuality}, nil
		}
		purityChoice -= band.WeightPPM
	}
	return nil, Fail(p.CodeOperationConflict, "矿石纯度配置未解析为有效档位")
}

// applyMiningAttempt 应用原判定的工具、离线时间及单块出矿结果。
func applyMiningAttempt(role domain.Role, q MiningAttemptRequest, layer uint32, interval time.Duration, tool domain.Item, fixture *devconfig.Fixture, ore *domain.Item) (domain.Role, MiningAttemptReceipt, error) {
	wear := fixture.Mining.ToolWearU
	if wear <= 0 {
		return role, MiningAttemptReceipt{}, Fail(p.CodeOperationConflict, "锄头耐久消耗规则无效")
	}
	position := slices.IndexFunc(role.Items, func(item domain.Item) bool { return item.ID == tool.ID })
	if position < 0 {
		return role, MiningAttemptReceipt{}, Fail(p.CodeVendorItemBlocked, "原锄头已离开背包")
	}
	remaining := max(tool.DurabilityU-wear, 0)
	next := role
	next.Items = slices.Clone(role.Items)
	if remaining == 0 {
		next.Items = slices.Delete(next.Items, position, position+1)
	} else {
		updated := tool
		updated.DurabilityU, updated.Version = remaining, updated.Version+1
		next.Items[position] = updated
	}
	next.LastMiningAt, next.SourceSeq, next.Revision = q.At, q.SourceSeq, role.Revision+1
	next.HostingSeconds -= q.HostSeconds
	if err := updateMiningReport(&next, q, interval); err != nil {
		return role, MiningAttemptReceipt{}, err
	}
	recycled := applyMiningAutoRecycle(&next, fixture, ore, q.HostingSession != "")
	if ore != nil && recycled == nil {
		next.Items = append(next.Items, *ore)
		if q.HostingSession != "" {
			if err := recordMiningDrop(&next.Report, *ore); err != nil {
				return role, MiningAttemptReceipt{}, err
			}
		}
	}
	if len(next.Items) < next.Capacity {
		next.PickupStopped = false
	}
	return next, MiningAttemptReceipt{EventID: q.EventID, MapID: q.MapID, RuleVersion: q.RuleVersion, ConfigHash: q.ConfigHash,
		Layer: layer, StartedAt: q.StartedAt, At: q.At, ToolID: tool.ID, ToolItemID: tool.ItemID, ToolVersionBefore: tool.Version,
		ToolVersionAfter: tool.Version + 1, DurabilityBeforeU: tool.DurabilityU, DurabilityAfterU: remaining, ToolExhausted: remaining == 0,
		Ore: ore, AutoRecycle: recycled, HostingSession: q.HostingSession, HostSeconds: q.HostSeconds, RemainingHostSeconds: next.HostingSeconds}, nil
}

// updateMiningReport 应用原会话使用时长和水位，出矿后再记录入包或回收。
func updateMiningReport(role *domain.Role, q MiningAttemptRequest, interval time.Duration) error {
	if q.HostingSession == "" {
		return nil
	}
	report := role.Report
	if report.SessionID != q.HostingSession {
		report = domain.Report{SessionID: q.HostingSession, Drops: []domain.Item{}}
	}
	var err error // 任一报表计数溢出会取消整个原判定。
	report.UsedSeconds, err = add(report.UsedSeconds, q.HostSeconds)
	if err != nil {
		return err
	}
	report.Sequence, err = add(report.Sequence, 1)
	if err != nil {
		return err
	}
	if role.HostingSeconds < int64(interval/time.Second) && (report.StopReason == 0 || report.StopReason == 3) {
		// 可用秒数不足整轮时停止，未扣的尾数仍留在角色余额。
		report.StopReason = 1
	}
	role.Report = report
	return nil
}

// recordMiningDrop 仅把实际进入背包的原矿计入同会话的掉落汇总与限量明细。
func recordMiningDrop(report *domain.Report, ore domain.Item) error {
	var err error
	report.TotalDropCount, err = add(report.TotalDropCount, 1)
	if err != nil {
		return err
	}
	if len(report.Drops) < 100 {
		report.Drops = append(slices.Clone(report.Drops), ore)
	}
	return nil
}
