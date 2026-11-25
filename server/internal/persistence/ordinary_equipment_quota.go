// 本文件用原结算事务确认普通怪高品质装备的分窗额度和实际掉落。
package persistence

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	ordinaryEquipmentQuotaCollection = "equipment_drop_quota_windows"      // 永久额度窗，常规配置发布不会改写。
	ordinaryEquipmentQuotaSource     = devconfig.OrdinaryMonsterDropSource // 普通怪在线和托管使用同一来源分账。
	ordinaryEquipmentItemSource      = "monster_drop"                      // 原掉落实例沿用的物品来源名称。

	OrdinaryEquipmentGranted   = "granted"   // 实物完整入包。
	OrdinaryEquipmentRecycled  = "recycled"  // 原实例已按保存的自动回收策略兑现。
	OrdinaryEquipmentDiscarded = "discarded" // 背包无空位，原掉落已放弃且没有兑现。
)

// OrdinaryEquipmentPlan 保存从击杀时已冻结的高品质候选向低品质回退的顺序。
// 主候选在 Settlement.Item 中。每档降级保持概率流程已经选出的实际模板，无需在事务重试时掷点。
type OrdinaryEquipmentPlan struct {
	Fallbacks []domain.Item `json:"fallbacks"` // 严格逐档降低，末项是同阶段的普通或精良装备。
}

// OrdinaryEquipmentReceipt 固定真正落地的实例、原候选及兑现或丢弃的处置。
type OrdinaryEquipmentReceipt struct {
	Item             domain.Item `bson:"item"`                         // 降档后的实际模板、品质、绑定和实例。
	CandidateItemID  uint32      `bson:"candidate_item_id"`            // 本事实最先抽到的高品质装备模板。
	Stage            uint32      `bson:"stage"`                        // 装备自身阶段，由真实配置确认。
	Disposition      string      `bson:"disposition"`                  // 入包、兑现回收或丢弃。
	QuotaConsumed    bool        `bson:"quota_consumed"`               // 实际确认稀缺额度扣减时为真。
	QuotaWindowStart time.Time   `bson:"quota_window_start,omitempty"` // 被扣额度对应原事件所属的窗口起点。
	QuotaWindowEnd   time.Time   `bson:"quota_window_end,omitempty"`   // 被扣额度对应窗口的排他终点。
}

// ordinaryEquipmentCandidate 是同阶段、来源和物品实例均已验证的一个候选。
type ordinaryEquipmentCandidate struct {
	item  domain.Item                       // 本档能够实际发放、回收或丢弃的模板及物品字段。
	quota *devconfig.OrdinaryEquipmentQuota // 普通、精良无额度。稀有和史诗对应配置各自独立。
}

// ordinaryQuotaWindow 保存当前窗口已确认消耗的各来源额度。
type ordinaryQuotaWindow struct {
	ID      string           `bson:"_id"`           // 运行服、实际阶段、品质及窗口起点构成的稳定主键。
	RunID   uint64           `bson:"run_server_id"` // 权威运行服，不随角色原区、场景或执行进程变化。
	Stage   uint32           `bson:"stage"`         // 被控制供给的物品阶段。
	Quality uint32           `bson:"quality"`       // 被控制供给的物品品质。
	Starts  time.Time        `bson:"starts_at"`     // 事件窗的包含起点。
	Ends    time.Time        `bson:"ends_at"`       // 事件窗的排他终点。
	Sources map[string]int64 `bson:"sources"`       // 各权威来源实际确认消耗的单位。
}

// ordinaryEquipmentChoice 携带事务快照内选中的实际产物及可用于原子扣减的账本视图。
type ordinaryEquipmentChoice struct {
	receipt OrdinaryEquipmentReceipt // 最终结果，由结算资产处理后补齐实际处置。
	window  ordinaryQuotaWindow      // 允许扣减的窗口快照。无配额的品质保持零值。
	exists  bool                     // 该窗口在事务快照中是否已经存在。
}

// ordinaryEquipmentCandidates 检查整条冻结链，避免凭任意模板、玩法来源或质量绕过额度。
func ordinaryEquipmentCandidates(f *devconfig.Fixture, q Settlement) ([]ordinaryEquipmentCandidate, error) {
	if q.OrdinaryEquipment == nil {
		// 基础道具、高品质技能书和历史低品装备仍沿用原逻辑，不访问额度集合。
		if q.Item != nil && q.Item.SourceKind == ordinaryEquipmentItemSource && (q.Item.Quality >= 2 || len(f.OrdinaryEquipmentQuotas) != 0) {
			definition, ok := f.Item(q.Item.ItemID)
			if ok && definition.IsEquipment && (definition.DropQuality >= 2 || q.Item.Quality >= 2) {
				return nil, Fail(p.CodeInvalidArgument, "高品质普通装备缺少冻结的额度和降档方案")
			}
		}
		return nil, nil
	}
	if q.Item == nil || q.Item.Quality < 2 || q.Item.Quality > 3 || !lowerHex(q.Item.ID, 16) || q.EventAt.IsZero() ||
		q.Kills != 1 || q.NoEquipmentKills != 0 || q.MapID == 0 || q.Gold != 0 || q.StopReason != 0 ||
		len(q.OrdinaryEquipment.Fallbacks) == 0 || len(q.OrdinaryEquipment.Fallbacks) > 2 {
		return nil, Fail(p.CodeInvalidArgument, "高品质普通装备结算事实无效")
	}
	pool, ok := f.OrdinaryDropPool(q.DropPoolID)
	if !ok || !slices.ContainsFunc(f.Monsters, func(monster devconfig.Monster) bool {
		return monster.ID == q.MonsterID && monster.DropPoolID == q.DropPoolID
	}) {
		return nil, Fail(p.CodeInvalidArgument, "高品质普通装备没有匹配的普通怪来源")
	}
	primary, ok := f.Item(q.Item.ItemID)
	if !ok {
		return nil, Fail(p.CodeInvalidArgument, "高品质普通装备不存在原模板")
	}
	candidates := make([]ordinaryEquipmentCandidate, 1+len(q.OrdinaryEquipment.Fallbacks))
	for index := range candidates {
		item := *q.Item
		if index > 0 {
			item = q.OrdinaryEquipment.Fallbacks[index-1]
		}
		definition, found := f.Item(item.ItemID)
		if !found || !definition.IsEquipment || definition.DropQuality != item.Quality || definition.EquipmentStage != primary.EquipmentStage ||
			!slices.ContainsFunc(pool.Equipment, func(entry devconfig.OrdinaryDropEntry) bool { return entry.ItemID == item.ItemID }) ||
			(item != domain.Item{ID: q.Item.ID, ItemID: item.ItemID, Quantity: 1, Version: 1, Quality: item.Quality,
				Bound: q.Bound, SourceKind: ordinaryEquipmentItemSource, SourceID: strconv.FormatInt(q.SourceSeq, 10)}) {
			return nil, Fail(p.CodeInvalidArgument, "高品质普通装备的模板、阶段或物品实例与原击杀不符")
		}
		if index > 0 {
			previous := candidates[index-1]
			if previous.quota == nil || item.Quality >= previous.item.Quality || previous.quota.ExhaustedDowngradeWeights[item.Quality] <= 0 {
				return nil, Fail(p.CodeInvalidArgument, "高品质普通装备降档不是原配置确认的降低品质")
			}
		}
		candidates[index].item = item
		if item.Quality >= 2 {
			quota, found := f.OrdinaryEquipmentQuota(primary.EquipmentStage, item.Quality)
			if !found || quota == nil || quota.SourceKind != ordinaryEquipmentQuotaSource {
				return nil, Fail(p.CodeInvalidArgument, "高品质普通装备没有对应阶段和来源的额度策略")
			}
			candidates[index].quota = quota
		}
	}
	if candidates[len(candidates)-1].item.Quality > 1 {
		return nil, Fail(p.CodeInvalidArgument, "高品质普通装备缺少已冻结的最终低档模板")
	}
	return candidates, nil
}

// resolveOrdinaryEquipment 按原击杀时刻查看分时已释放额度，依次尝试冻结候选。
func (s *Store) resolveOrdinaryEquipment(ctx context.Context, q Settlement) (*ordinaryEquipmentChoice, error) {
	candidates, err := ordinaryEquipmentCandidates(&s.Cfg.Fixture, q)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	for _, candidate := range candidates {
		definition, _ := s.Cfg.Fixture.Item(candidate.item.ItemID) // 上一步已校验配置身份。
		choice := ordinaryEquipmentChoice{receipt: OrdinaryEquipmentReceipt{
			Item: InitializeItemDurability(candidate.item, &s.Cfg.Fixture), CandidateItemID: q.Item.ItemID, Stage: definition.EquipmentStage,
		}}
		if candidate.quota == nil {
			return &choice, nil
		}
		window, found, released, err := s.readOrdinaryQuotaWindow(ctx, candidate.quota, q.EventAt)
		if err != nil {
			return nil, err
		}
		if window.Sources[ordinaryEquipmentQuotaSource] < released {
			choice.window, choice.exists = window, found
			return &choice, nil
		}
	}
	return nil, Fail(p.CodeOperationConflict, "普通装备额度耗尽且没有可用的原降档结果")
}

// readOrdinaryQuotaWindow 只读取权威服的原事件窗，当前时间和规则发布编号不参与额度身份。
func (s *Store) readOrdinaryQuotaWindow(ctx context.Context, quota *devconfig.OrdinaryEquipmentQuota, at time.Time) (ordinaryQuotaWindow, bool, int64, error) {
	starts, ends, released := quota.Window(at)
	if starts.IsZero() || !ends.After(starts) || at.Before(starts) || !at.Before(ends) || released < 0 || released > quota.QuotaUnits {
		return ordinaryQuotaWindow{}, false, 0, Fail(p.CodeInvalidArgument, "普通装备额度无法识别原事件窗口")
	}
	want := ordinaryQuotaWindow{ID: fmt.Sprintf("%d:%d:%d:%d", s.Cfg.RunID, quota.Stage, quota.Quality, starts.Unix()),
		RunID: s.Cfg.RunID, Stage: quota.Stage, Quality: quota.Quality, Starts: starts.UTC(), Ends: ends.UTC()}
	if released == 0 {
		return want, false, released, nil
	}
	var found ordinaryQuotaWindow
	err := s.DB.Collection(ordinaryEquipmentQuotaCollection).FindOne(ctx, bson.M{"_id": want.ID}).Decode(&found)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return want, false, released, nil
	}
	if err != nil {
		return ordinaryQuotaWindow{}, false, 0, err
	}
	if found.RunID != want.RunID || found.Stage != want.Stage || found.Quality != want.Quality ||
		!found.Starts.Equal(want.Starts) || !found.Ends.Equal(want.Ends) || found.Sources[ordinaryEquipmentQuotaSource] < 0 {
		return ordinaryQuotaWindow{}, false, 0, Fail(p.CodeOperationConflict, "普通装备额度窗持久身份或用量无效")
	}
	return found, true, released, nil
}

// confirmOrdinaryEquipment 在实际入包或兑现回收时扣减本档额度。丢弃仍由原结算归零保底。
func (s *Store) confirmOrdinaryEquipment(ctx context.Context, choice *ordinaryEquipmentChoice, details *SettlementReceipt) error {
	if choice == nil {
		return nil
	}
	receipt := &choice.receipt
	switch {
	case details.GrantedItem:
		receipt.Disposition = OrdinaryEquipmentGranted
	case details.Recycling.DiscardedCount > 0:
		receipt.Disposition = OrdinaryEquipmentDiscarded
	case slices.ContainsFunc(details.Recycling.Recycled, func(item domain.Item) bool { return item.ID == receipt.Item.ID }):
		receipt.Disposition = OrdinaryEquipmentRecycled
	default:
		return Fail(p.CodeOperationConflict, "原普通装备结算没有实际处置结果")
	}
	if choice.window.ID != "" && receipt.Disposition != OrdinaryEquipmentDiscarded {
		if err := s.consumeOrdinaryQuota(ctx, choice.window, choice.exists); err != nil {
			return err
		}
		receipt.QuotaConsumed = true
		receipt.QuotaWindowStart, receipt.QuotaWindowEnd = choice.window.Starts, choice.window.Ends
	}
	details.OrdinaryEquipment = receipt
	return nil
}

// consumeOrdinaryQuota 在原 Mongo 事务预留并确认一单位，失败会同时回滚角色、审计和回执。
func (s *Store) consumeOrdinaryQuota(ctx context.Context, window ordinaryQuotaWindow, exists bool) error {
	collection := s.DB.Collection(ordinaryEquipmentQuotaCollection)
	if !exists {
		window.Sources = map[string]int64{ordinaryEquipmentQuotaSource: 1}
		_, err := collection.InsertOne(ctx, window)
		if mongo.IsDuplicateKeyError(err) {
			return Fail(p.CodeOperationPending, "普通装备额度窗同时创建，请重试原收益事实")
		}
		return err
	}
	source := "sources." + ordinaryEquipmentQuotaSource
	filter := bson.M{"_id": window.ID, source: bson.M{"$exists": false}}
	if spent, recorded := window.Sources[ordinaryEquipmentQuotaSource]; recorded {
		filter[source] = spent
	}
	result, err := collection.UpdateOne(ctx, filter, bson.M{"$inc": bson.M{source: int64(1)}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return Fail(p.CodeOperationPending, "普通装备额度已并发变更，请重试原收益事实")
	}
	return nil
}
