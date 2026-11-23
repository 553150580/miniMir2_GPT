// 技能书与材料的预览及持久提交。生成购买、升级、分解和合页计划，提交时核对原报价、角色版本和物品来源。

package persistence

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"reflect"
	"slices"
	"sort"
	"time"
)

// skillMaterial 报价中选定的材料原实例及本次使用的数量。
type skillMaterial struct {
	// Item 记录道具。
	Item domain.Item `bson:"item"`
	// Quantity 记录数量，使用数值表示。
	Quantity int64 `bson:"quantity"`
}

// skillPlan 固定技能资产动作、价格、材料、技能等级变化和待发产物。
type skillPlan struct {
	// Kind 记录类型，使用数值表示。
	Kind uint32 `bson:"kind"`
	// OfferID 标识当前业务使用的“offer”。
	OfferID uint32 `bson:"offer_id"`
	// Quantity 记录数量，使用数值表示。
	Quantity int64 `bson:"quantity"`
	// SkillID 标识当前业务使用的技能。
	SkillID uint32 `bson:"skill_id"`
	// From 记录来源，使用数值表示。
	From uint32 `bson:"from"`
	// To 记录目标，使用数值表示。
	To uint32 `bson:"to"`
	// Spend 记录“spend”，使用钱包表示。
	Spend domain.Wallet `bson:"spend"`
	// Materials 记录材料集合，使用技能材料列表表示。
	Materials []skillMaterial `bson:"materials"`
	// Outputs 记录“outputs”，使用道具列表表示。
	Outputs []domain.Item `bson:"outputs"`
}

// cloneSkillPlan 复制计划中的材料与产物切片，保护原永久回执。
// plan 记录计划，使用技能计划表示。
func cloneSkillPlan(plan *skillPlan) *skillPlan {
	if plan == nil {
		return nil
	}
	// out 保存当前处理所用的输出。
	out := *plan
	out.Materials = slices.Clone(plan.Materials)
	out.Outputs = slices.Clone(plan.Outputs)
	return &out
}

// 每角色至多一个可替换预览；仅预览允许过期，永久操作回执不受其清理影响。
type skillQuote struct {
	// RoleID 标识当前业务使用的角色。
	RoleID string `bson:"_id"`
	// ID 区分当前步骤指向的目标对象。
	ID string `bson:"preview_id"`
	// Revision 记录修订号，使用数值表示。
	Revision int64 `bson:"revision"`
	// RuleVersion 记录规则版本，使用数值表示。
	RuleVersion uint64 `bson:"rule_version"`
	// Hash 记录摘要，使用文本表示。
	Hash string `bson:"config_hash"`
	// Expires 记录过期集合，使用时间点表示。
	Expires time.Time `bson:"expires_at"`
	// Request 记录请求，使用字节序列表示。
	Request []byte `bson:"request"`
	// Plan 记录计划，使用技能计划表示。
	Plan skillPlan `bson:"plan"`
}

// multiply 安全相乘非负数量或价格，避免 int64 计算溢出。
// a 记录a，使用数值表示，b 记录b，使用数值表示。
func multiply(a, b int64) (int64, bool) {
	if a < 0 || b < 0 || (a > 0 && b > math.MaxInt64/a) {
		return 0, false
	}
	return a * b, true
}

// selectSkillMaterials 复核客户端逐实例选择、数量和版本，并按实例编号稳定排列。
// r 记录角色，q 记录技能资产预览请求，need 记录所需量，使用按数值索引的数值映射表示，limit 记录上限，使用数值表示。
func selectSkillMaterials(r domain.Role, q *pb.SkillAssetPreviewRequest, need map[uint32]int64, limit int) ([]skillMaterial, uint32) {
	// selected 保存材料集合，用于当前步骤中的已选择。
	selected := q.Materials
	if len(selected) == 0 || len(selected) > limit {
		return nil, p.CodeSkillMaterials
	}
	// out 初始化用作输出的技能材料列表。
	out := []skillMaterial{}
	// seen 建立带初始值的按文本索引的布尔状态映射，供本段继续处理。
	seen := map[string]bool{}
	// actual 建立带初始值的按数值索引的数值映射，供本段继续处理。
	actual := map[uint32]int64{}
	// c 接收遍历已选择时当前的元素。
	for _, c := range selected {
		if c == nil || len(c.InstanceId) != 16 || c.Quantity <= 0 || c.Version == 0 || c.Version > math.MaxInt64 {
			return nil, p.CodeSkillMaterials
		}
		// id 区分当前步骤指向的目标对象。
		id := domain.Hex(c.InstanceId)
		if seen[id] {
			return nil, p.CodeSkillMaterials
		}
		seen[id] = true
		// found 保存当前处理所用的已找到标记。
		found := false
		// item 接收遍历道具集合时当前的元素。
		for _, item := range r.Items {
			if item.ID != id {
				continue
			}
			if item.Version != int64(c.Version) || item.Version == math.MaxInt64 || item.Quantity < c.Quantity {
				return nil, p.CodeSkillMaterials
			}
			// total 保存由新增取得的合计，err 接收新增过程中可能产生的错误，供后续失败分支处理。
			total, err := add(actual[item.ItemID], c.Quantity)
			if err != nil {
				return nil, p.CodeSkillMaterials
			}
			actual[item.ItemID] = total
			out = append(out, skillMaterial{item, c.Quantity})
			found = true
			break
		}
		if !found {
			return nil, p.CodeSkillMaterials
		}
	}
	if q.Kind == 2 && !reflect.DeepEqual(actual, need) {
		return nil, p.CodeSkillMaterials
	}
	// i 记录当前位置的下标，使用数值表示，j 记录当前位置的下标，使用数值表示。
	sort.Slice(out, func(i, j int) bool { return out[i].Item.ID < out[j].Item.ID })
	return out, 0
}

// 仅合并身份以外完全同源的完整堆叠；新增实例字段也必须参与比较。
// item 记录道具。
func normalizedSkillPage(item domain.Item) domain.Item {
	item.ID, item.Quantity, item.Version = "", 0, 0
	return item
}

// mergeSkillPages 要求完整同源页堆，计算按最大堆叠量重建后能否节省背包格。
// materials 记录材料集合，使用技能材料列表表示，f 记录开发测试配置。
func mergeSkillPages(materials []skillMaterial, f devconfig.Fixture) ([]domain.Item, uint32) {
	if len(materials) < 2 {
		return nil, p.CodeSkillMaterials
	}
	// first 保存道具，用于当前步骤中的第一项。
	first := materials[0].Item
	// definition 保存由道具取得的定义，known 保存由道具取得的已知状态。
	definition, known := f.Item(first.ItemID)
	if !known || !definition.IsSkillPage || !definition.Stackable || definition.MaxStack <= 1 || first.SourceKind == "" || first.SourceID == "" {
		return nil, p.CodeSkillMaterials
	}
	switch first.SourceKind {
	case "skill_recycle", "npc_shop", "monster_drop":
	default:
		return nil, p.CodeSkillMaterials
	}
	// total 保存由整数“64”取得的合计。
	total := int64(0)
	// material 接收遍历材料集合时当前的元素。
	for _, material := range materials {
		// item 保存当前处理所用的道具。
		item := material.Item
		if item.Quantity <= 0 || item.Quantity > definition.MaxStack || material.Quantity != item.Quantity || item.Locked || item.PendingResultID != "" || item.ReservationID != "" || !reflect.DeepEqual(normalizedSkillPage(item), normalizedSkillPage(first)) {
			return nil, p.CodeSkillMaterials
		}
		// err 保存当前操作的失败原因，供调用方检查或返回。
		var err error
		total, err = add(total, item.Quantity)
		if err != nil {
			return nil, p.CodeSkillMaterials
		}
	}
	// 无法减少至少一个格子时拒绝；不为重发或排序更换原实例身份。
	// outputCount 保存根据合计与最大堆叠计算的输出数量。
	outputCount := total / definition.MaxStack
	if total%definition.MaxStack != 0 {
		outputCount++
	}
	if outputCount >= int64(len(materials)) {
		return nil, p.CodeSkillMaterials
	}
	first.ID, first.Quantity, first.Version = "", total, 1
	return []domain.Item{first}, 0
}

// buildSkillPlan 为技能书购买、升级、分解或合页计算费用、材料和确定产物。
// r 记录角色，q 记录技能资产预览请求，f 记录开发测试配置。
func buildSkillPlan(r domain.Role, q *pb.SkillAssetPreviewRequest, f devconfig.Fixture) (skillPlan, uint32) {
	// plan 初始化用作计划的技能计划。
	plan := skillPlan{Kind: q.Kind, OfferID: q.OfferId, Quantity: q.Quantity, SkillID: q.SkillId}
	if r.Wallet.Gold < 0 || r.Wallet.BoundGold < 0 || r.Wallet.Yuanbao < 0 || r.Wallet.BoundYuanbao < 0 {
		return plan, p.CodeInvalidArgument
	}
	switch q.Kind {
	case 1:
		// offer 保存由商店尝试提交取得的尝试提交，ok 表明查询、校验或类型转换是否成功。
		offer, ok := f.ShopOffer(q.OfferId)
		if !ok || q.Quantity <= 0 || q.Quantity > offer.MaxPurchaseQuantity || r.Level < offer.RequiredRoleLevel || len(q.Materials) > 0 || q.SkillId != 0 {
			return plan, p.CodeSkillPurchaseBlocked
		}
		// quantity 保存由相乘取得的数量，valid 保存由相乘取得的有效。
		quantity, valid := multiply(offer.Quantity, q.Quantity)
		if !valid {
			return plan, p.CodeSkillPurchaseBlocked
		}
		// cost 保存由相乘取得的消耗。
		cost, valid := multiply(offer.PriceUnits, q.Quantity)
		if !valid {
			return plan, p.CodeSkillPurchaseBlocked
		}
		switch offer.Currency {
		case "gold":
			plan.Spend.BoundGold = cost
			if plan.Spend.BoundGold > r.Wallet.BoundGold {
				plan.Spend.BoundGold = r.Wallet.BoundGold
			}
			plan.Spend.Gold = cost - plan.Spend.BoundGold
		case "ingot":
			plan.Spend.Yuanbao = cost
		case "bound_ingot":
			plan.Spend.BoundYuanbao = cost
		default:
			return plan, p.CodeSkillPurchaseBlocked
		}
		plan.Outputs = []domain.Item{{ItemID: offer.ItemID, Quantity: quantity, Bound: offer.OutputBound, Version: 1, SourceKind: "npc_shop"}}
	case 2:
		// s 接收技能供后续步骤使用，exists 保存由技能取得的存在。
		s, exists := f.Skill(q.SkillId)
		// level 保存由技能取得的等级，learned 保存由技能取得的已学习。
		level, learned := r.Skill(q.SkillId)
		if !exists || !learned || r.Job != s.JobID || level < s.LearnedLevel || level >= s.MaxLevel || q.OfferId != 0 || q.Quantity != 0 {
			return plan, p.CodeSkillUpgradeBlocked
		}
		// row 保存由升级取得的行记录。
		row, exists := f.Upgrade(s.ID, level+1)
		if !exists || r.Level < row.RequiredRoleLevel {
			return plan, p.CodeSkillUpgradeBlocked
		}
		plan.From, plan.To = level, level+1
		// need 初始化用作所需量的按数值索引的数值映射。
		need := map[uint32]int64{}
		// c 接收遍历技能书“costs”时当前的元素。
		for _, c := range row.BookCosts {
			// n 接收新增供后续步骤使用，err 接收新增过程中可能产生的错误，供后续失败分支处理。
			n, err := add(need[c.ItemID], c.Quantity)
			if err != nil {
				return plan, p.CodeSkillMaterials
			}
			need[c.ItemID] = n
		}
		if row.PageCost > 0 {
			// n 接收新增供后续步骤使用，err 接收新增过程中可能产生的错误，供后续失败分支处理。
			n, err := add(need[row.PageItemID], row.PageCost)
			if err != nil {
				return plan, p.CodeSkillMaterials
			}
			need[row.PageItemID] = n
		}
		// materials 保存由选择技能材料集合取得的材料集合，code 保存由选择技能材料集合取得的结果码。
		materials, code := selectSkillMaterials(r, q, need, f.SkillEconomy.PreviewMaterialLimit)
		if code != 0 {
			return plan, code
		}
		plan.Materials = materials
	case 3:
		if q.OfferId != 0 || q.Quantity != 0 || q.SkillId != 0 {
			return plan, p.CodeSkillBookSource
		}
		// materials 保存由选择技能材料集合取得的材料集合，code 保存由选择技能材料集合取得的结果码。
		materials, code := selectSkillMaterials(r, q, nil, f.SkillEconomy.PreviewMaterialLimit)
		if code != 0 {
			return plan, code
		}
		plan.Materials = materials
		// m 接收遍历材料集合时当前的元素。
		for _, m := range materials {
			// item 保存当前处理所用的道具。
			item := m.Item
			// rule 保存由技能书回收取得的规则，ok 表明查询、校验或类型转换是否成功。
			rule, ok := f.BookRecycle(item.ItemID, item.Quality)
			// allowed 保存当前处理所用的允许。
			allowed := false
			// kind 接收遍历允许来源“kinds”时当前的元素。
			for _, kind := range rule.AllowedSourceKinds {
				if item.SourceKind == kind {
					allowed = true
				}
			}
			if !ok || !allowed || item.SourceKind != "monster_drop" || item.SourceID == "" {
				return plan, p.CodeSkillBookSource
			}
			// quantity 保存由相乘取得的数量，valid 保存由相乘取得的有效。
			quantity, valid := multiply(m.Quantity, rule.PagesPerBook)
			if !valid {
				return plan, p.CodeSkillMaterials
			}
			// 不同原书独立输出，不合并来源或绑定，防止用堆叠抹掉输入来源。
			plan.Outputs = append(plan.Outputs, domain.Item{ItemID: rule.PageItemID, Quantity: quantity, Bound: item.Bound, Version: 1, SourceKind: "skill_recycle"})
		}
	case 4:
		if q.OfferId != 0 || q.Quantity != 0 || q.SkillId != 0 {
			return plan, p.CodeSkillMaterials
		}
		// materials 保存由选择技能材料集合取得的材料集合，code 保存由选择技能材料集合取得的结果码。
		materials, code := selectSkillMaterials(r, q, nil, f.SkillEconomy.PreviewMaterialLimit)
		if code != 0 {
			return plan, code
		}
		// material 接收遍历材料集合时当前的元素。
		for _, material := range materials {
			if r.RecycleBlockFlags(material.Item) != 0 {
				return plan, p.CodeSkillMaterials
			}
		}
		// outputs 保存由合并技能“pages”取得的“outputs”。
		outputs, code := mergeSkillPages(materials, f)
		if code != 0 {
			return plan, code
		}
		plan.Materials, plan.Outputs = materials, outputs
	default:
		return plan, p.CodeInvalidArgument
	}
	if plan.Spend.Gold > r.Wallet.Gold || plan.Spend.BoundGold > r.Wallet.BoundGold || plan.Spend.Yuanbao > r.Wallet.Yuanbao || plan.Spend.BoundYuanbao > r.Wallet.BoundYuanbao {
		return plan, p.CodeSkillPurchaseBlocked
	}
	// remaining 记录道具集合中的条目数。
	remaining := len(r.Items)
	// m 接收遍历材料集合时当前的元素。
	for _, m := range plan.Materials {
		if m.Quantity == m.Item.Quantity {
			remaining--
		}
	}
	// split 初始化用作拆分的道具列表。
	split := []domain.Item{}
	// item 接收遍历“outputs”时当前的元素。
	for _, item := range plan.Outputs {
		// definition 保存由道具取得的定义，exists 保存由道具取得的存在。
		definition, exists := f.Item(item.ItemID)
		if !exists {
			return plan, p.CodeInvalidArgument
		}
		// maxStack 保存当前处理所用的最大堆叠。
		maxStack := definition.MaxStack
		if maxStack <= 0 {
			maxStack = 1
		}
		for item.Quantity > 0 {
			// part 保存当前处理所用的分段。
			part := item
			if part.Quantity > maxStack {
				part.Quantity = maxStack
			}
			split = append(split, part)
			item.Quantity -= part.Quantity
			if remaining+len(split) > r.Capacity {
				return plan, p.CodeSkillMaterials
			}
		}
	}
	plan.Outputs = split
	if remaining+len(plan.Outputs) > r.Capacity {
		return plan, p.CodeSkillMaterials
	}
	return plan, 0
}

// PreviewSkillAsset 在账号和运行围栏内计算并替换本角色的短期技能资产报价。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，a 保存本次调用使用的账号或角色授权，q 记录技能资产预览请求。
func (s *Store) PreviewSkillAsset(ctx context.Context, a Auth, q *pb.SkillAssetPreviewRequest) (*pb.SkillAssetPreview, error) {
	if q == nil {
		return nil, Fail(p.CodeInvalidArgument, "缺少技能预览")
	}
	// quote 记录报价，使用技能报价表示。
	var quote skillQuote
	// err 接收已围栏事务过程中可能产生的错误，供后续失败分支处理，tc 记录MongoDB 事务上下文，使用请求或事务上下文表示。
	err := s.guardedTx(ctx, a, func(tc context.Context) error {
		// err 接收权限围栏过程中可能产生的错误，供后续失败分支处理。
		if err := s.fence(tc, a); err != nil {
			return err
		}
		// r 接收角色供后续步骤使用，err 接收角色过程中可能产生的错误，供后续失败分支处理。
		r, err := s.Role(tc, a.RoleID)
		if err != nil {
			return err
		}
		if err = s.checkRole(r, a); err != nil {
			return err
		}
		// plan 保存由构建技能计划取得的计划，code 保存由构建技能计划取得的结果码。
		plan, code := buildSkillPlan(r, q, s.Cfg.Fixture)
		if code != 0 {
			return Fail(code, "技能预览条件变化或材料不足")
		}
		// normalized 按目标类型读取当前接口值，得到“normalized”。
		normalized := proto.Clone(q).(*pb.SkillAssetPreviewRequest)
		normalized.Materials = nil
		// m 接收遍历材料集合时当前的元素。
		for _, m := range plan.Materials {
			normalized.Materials = append(normalized.Materials, &pb.ItemConsumption{InstanceId: domain.Bytes(m.Item.ID), Version: uint64(m.Item.Version), Quantity: m.Quantity})
		}
		// payload 保存由序列化取得的协议负载。
		payload, err := proto.Marshal(normalized)
		if err != nil {
			return err
		}
		quote = skillQuote{RoleID: r.ID, ID: domain.ID(), Revision: r.Revision, RuleVersion: s.Cfg.Fixture.RuleVersion, Hash: s.Cfg.Fixture.Hash, Expires: time.Now().Add(time.Duration(s.Cfg.Fixture.SkillEconomy.PreviewTTLMS) * time.Millisecond), Request: payload, Plan: plan}
		_, err = s.DB.Collection("skill_quotes").ReplaceOne(tc, bson.M{"_id": r.ID}, quote, options.Replace().SetUpsert(true))
		return err
	})
	if err != nil {
		return nil, err
	}
	return quote.Proto(s.Cfg.Fixture), nil
}

// commitSkillAsset 重新核对报价与角色、配置、选中材料的版本，再应用原报价结果。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，r 记录角色，q 记录资产请求。
func (s *Store) commitSkillAsset(ctx context.Context, r domain.Role, q AssetRequest) (domain.Role, uint32, *skillPlan, error) {
	// quote 记录报价，使用技能报价表示。
	var quote skillQuote
	// err 接收解码过程中可能产生的错误，供后续失败分支处理。
	err := s.DB.Collection("skill_quotes").FindOne(ctx, bson.M{"_id": r.ID, "preview_id": q.PreviewID}).Decode(&quote)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return r, p.CodeSkillPreviewStale, nil, nil
	}
	if err != nil {
		return r, 0, nil, err
	}
	if quote.Revision != r.Revision || quote.RuleVersion != s.Cfg.Fixture.RuleVersion || quote.Hash != s.Cfg.Fixture.Hash || !time.Now().Before(quote.Expires) {
		return r, p.CodeSkillPreviewStale, nil, nil
	}
	// request 保存当前处理所用的请求。
	request := &pb.SkillAssetPreviewRequest{}
	if proto.Unmarshal(quote.Request, request) != nil {
		return r, p.CodeSkillPreviewStale, nil, nil
	}
	// plan 保存由构建技能计划取得的计划，code 保存由构建技能计划取得的结果码。
	plan, code := buildSkillPlan(r, request, s.Cfg.Fixture)
	if code != 0 {
		return r, code, nil, nil
	}
	if !reflect.DeepEqual(plan, quote.Plan) {
		return r, p.CodeSkillPreviewStale, nil, nil
	}
	// out 保存由应用技能计划取得的输出。
	out, plan := applySkillPlan(r, plan, domain.Hex(q.Context.OpId))
	return out, 0, &plan, nil
}

// applySkillPlan 扣除原材料和货币，应用技能等级变化并按操作身份标记产物。
// r 记录角色，plan 记录计划，使用技能计划表示，operationID 标识当前业务使用的操作。
func applySkillPlan(r domain.Role, plan skillPlan, operationID string) (domain.Role, skillPlan) {
	// out 保存当前处理所用的输出。
	out := r
	out.Items = append([]domain.Item(nil), r.Items...)
	out.Skills = append([]domain.LearnedSkill(nil), r.Skills...)
	plan.Outputs = slices.Clone(plan.Outputs)
	// consumption 建立带初始值的按文本索引的数值映射，供本段继续处理。
	consumption := map[string]int64{}
	// m 接收遍历材料集合时当前的元素。
	for _, m := range plan.Materials {
		consumption[m.Item.ID] = m.Quantity
	}
	// kept 建立带初始值的道具列表，供本段继续处理。
	kept := []domain.Item{}
	// item 接收遍历道具集合时当前的元素。
	for _, item := range out.Items {
		// n 保存从“consumption”取出的“consumption”中的指定项。
		n := consumption[item.ID]
		if n == 0 {
			kept = append(kept, item)
			continue
		}
		item.Quantity -= n
		item.Version++
		if item.Quantity > 0 {
			kept = append(kept, item)
		}
	}
	out.Items = kept
	// i 定位“outputs”中正在处理的成员，item 接收遍历“outputs”时当前的元素。
	for i, item := range plan.Outputs {
		item.ID = domain.ID()
		if plan.Kind != 4 {
			item.SourceID = operationID
		}
		out.Items = append(out.Items, item)
		plan.Outputs[i] = item
	}
	out.Wallet.Gold -= plan.Spend.Gold
	out.Wallet.BoundGold -= plan.Spend.BoundGold
	out.Wallet.Yuanbao -= plan.Spend.Yuanbao
	out.Wallet.BoundYuanbao -= plan.Spend.BoundYuanbao
	if plan.Kind == 2 {
		// i 定位技能集合中正在处理的成员，skill 接收遍历技能集合时当前的元素。
		for i, skill := range out.Skills {
			if skill.SkillID == plan.SkillID {
				out.Skills[i].Level = plan.To
			}
		}
	}
	return out, plan
}

// powerBounds 从当前技能配置及目标等级计算供报价展示的效果上下界。
// s 记录技能，rank 记录排序位次，使用数值表示。
func powerBounds(s devconfig.Skill, rank uint32) (int64, int64) {
	// round 封装“round”对应的后续处理，m 记录数值，power 记录能力值，使用数值表示。
	round := func(m, power int64) int64 {
		return s.SourcePower(rank, m, power)
	}
	// mpower 保存m能力值基础值，用于当前步骤中的“mpower”，power 保存能力值基础值，用于当前步骤中的能力值。
	mpower, power := s.MPowerBase, s.PowerBase
	if s.MPowerBonus > 0 {
		mpower += s.MPowerBonus - 1
	}
	if s.PowerBonus > 0 {
		power += s.PowerBonus - 1
	}
	return round(s.MPowerBase, s.PowerBase), round(mpower, power)
}

// Proto 把报价、费用、材料和效果预览转换为客户端协议对象。
// q 是调用该方法的技能报价实例，f 记录开发测试配置。
func (q skillQuote) Proto(f devconfig.Fixture) *pb.SkillAssetPreview {
	// p 保存当前处理所用的计划。
	p := q.Plan
	// out 保存当前处理所用的输出。
	out := &pb.SkillAssetPreview{PreviewId: domain.Bytes(q.ID), Kind: p.Kind, RuleVersion: q.RuleVersion, ConfigHash: q.Hash, RoleRevision: uint64(q.Revision), ExpiresAtUnixMs: uint64(q.Expires.UnixMilli()), SkillId: p.SkillID, FromLevel: p.From, ToLevel: p.To, Spend: p.Spend.Proto(q.Revision), OfferId: p.OfferID, Quantity: p.Quantity}
	// m 接收遍历材料集合时当前的元素。
	for _, m := range p.Materials {
		out.Materials = append(out.Materials, &pb.SkillMaterialView{Item: m.Item.Proto(), ConsumeQuantity: m.Quantity})
	}
	// item 接收遍历“outputs”时当前的元素。
	for _, item := range p.Outputs {
		out.Outputs = append(out.Outputs, item.Proto())
	}
	if p.Kind == 2 {
		// s 接收技能供后续步骤使用。
		s, _ := f.Skill(p.SkillID)
		if s.Summon != nil {
			// d 接收定义“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
			d, ok := f.PetRuntime.DefinitionByID(s.Summon.DefinitionID)
			if !ok || uint64(p.From) >= uint64(len(d.Training.InitialLevelByRank)) || uint64(p.To) >= uint64(len(d.Training.InitialLevelByRank)) {
				return out
			}
			// beforeLevel 保存从初始值等级“by”排序位次取出的操作前的状态等级，beforeMax 保存从最大值等级“by”排序位次取出的操作前的状态最大。
			beforeLevel, beforeMax := d.Training.InitialLevelByRank[p.From], d.Training.MaximumLevelByRank[p.From]
			// afterLevel 保存从初始值等级“by”排序位次取出的操作后的状态等级，afterMax 保存从最大值等级“by”排序位次取出的操作后的状态最大。
			afterLevel, afterMax := d.Training.InitialLevelByRank[p.To], d.Training.MaximumLevelByRank[p.To]
			// before 保存由战斗单位发生时间取得的操作前的状态，beforeOK 保存由战斗单位发生时间取得的操作前的状态成功标记。
			before, beforeOK := d.FighterAt(beforeLevel, beforeMax)
			// after 保存由战斗单位发生时间取得的操作后的状态，afterOK 保存由战斗单位发生时间取得的操作后的状态成功标记。
			after, afterOK := d.FighterAt(afterLevel, afterMax)
			if !beforeOK || !afterOK {
				return out
			}
			// comparison 封装“comparison”对应的后续处理，key 记录键，使用文本表示，before 记录操作前的状态，使用数值表示，after 记录操作后的状态，使用数值表示。
			comparison := func(key string, before, after int64) *pb.SkillNumericComparison {
				return &pb.SkillNumericComparison{Key: key, BeforeMin: before, BeforeMax: before, AfterMin: after, AfterMax: after}
			}
			// 此比较只描述未来新生；升级技能不会重造或改写现有宠物的训练状态。
			out.Stats = []*pb.SkillNumericComparison{
				comparison("mp_cost", s.MPBaseCost+int64(p.From)*s.MPLevelCost, s.MPBaseCost+int64(p.To)*s.MPLevelCost),
				comparison("new_pet_training_level", int64(beforeLevel), int64(afterLevel)),
				comparison("new_pet_max_training_level", int64(beforeMax), int64(afterMax)),
				comparison("new_pet_material_quantity", s.MaterialQuantity, s.MaterialQuantity),
				comparison("new_pet_hp_max", before.HP, after.HP),
			}
			return out
		}
		if s.Behavior == "turn_undead" {
			// beforeCost、afterCost 是升级前后实际魔法消耗，before、after 是未叠加目标等级差的第二段阈值。
			beforeCost := s.MPBaseCost + int64(p.From)*s.MPLevelCost
			afterCost := s.MPBaseCost + int64(p.To)*s.MPLevelCost
			before, after := devconfig.TurnUndeadBaseThreshold(p.From), devconfig.TurnUndeadBaseThreshold(p.To)
			out.Stats = []*pb.SkillNumericComparison{
				{Key: "mp_cost", BeforeMin: beforeCost, BeforeMax: beforeCost, AfterMin: afterCost, AfterMax: afterCost},
				{Key: "turn_undead_base_threshold", BeforeMin: before, BeforeMax: before, AfterMin: after, AfterMax: after},
			}
			return out
		}
		// beforeMin 保存由能力值“bounds”取得的操作前的状态最小，beforeMax 保存由能力值“bounds”取得的操作前的状态最大。
		beforeMin, beforeMax := powerBounds(s, p.From)
		// afterMin 保存由能力值“bounds”取得的操作后的状态最小，afterMax 保存由能力值“bounds”取得的操作后的状态最大。
		afterMin, afterMax := powerBounds(s, p.To)
		out.Stats = []*pb.SkillNumericComparison{{Key: "base_power", BeforeMin: beforeMin, BeforeMax: beforeMax, AfterMin: afterMin, AfterMax: afterMax}, {Key: "accuracy_bonus", BeforeMin: s.AccuracyBonus(p.From), BeforeMax: s.AccuracyBonus(p.From), AfterMin: s.AccuracyBonus(p.To), AfterMax: s.AccuracyBonus(p.To)}, {Key: "mp_cost", BeforeMin: s.MPBaseCost + int64(p.From)*s.MPLevelCost, BeforeMax: s.MPBaseCost + int64(p.From)*s.MPLevelCost, AfterMin: s.MPBaseCost + int64(p.To)*s.MPLevelCost, AfterMax: s.MPBaseCost + int64(p.To)*s.MPLevelCost}}
		// d 保存当前处理所用的位移。
		if d := s.Displacement; d != nil {
			// cost 保存从统计或属性取出的消耗。
			cost := out.Stats[2]
			// key 保存键所使用的文本。
			key := "push_distance"
			if s.Behavior == "shoulder_dash" {
				key = "dash_distance"
			}
			// before 保存由整数“64”取得的操作前的状态，after 保存由整数“64”取得的操作后的状态。
			before, after := int64(d.DistanceBaseByLevel[p.From]), int64(d.DistanceBaseByLevel[p.To])
			// distance 保存当前处理所用的距离。
			distance := &pb.SkillNumericComparison{Key: key, BeforeMin: before, BeforeMax: before + d.DistanceRandomBound - 1, AfterMin: after, AfterMax: after + d.DistanceRandomBound - 1}
			if s.Behavior == "repulsion" {
				// beforeChance 保存根据成功基础值与整数“64”与成功“per”等级的计算结果的计算结果与当前数据的计算结果与成功“denominator”计算的操作前的状态“chance”。
				beforeChance := (d.SuccessBase + int64(p.From)*d.SuccessPerLevel) * 100 / d.SuccessDenominator
				// afterChance 保存根据成功基础值与整数“64”与成功“per”等级的计算结果的计算结果与当前数据的计算结果与成功“denominator”计算的操作后的状态“chance”。
				afterChance := (d.SuccessBase + int64(p.To)*d.SuccessPerLevel) * 100 / d.SuccessDenominator
				out.Stats = []*pb.SkillNumericComparison{distance, {Key: "push_base_success_percent", BeforeMin: beforeChance, BeforeMax: beforeChance, AfterMin: afterChance, AfterMax: afterChance}, {Key: "monster_damage"}, cost}
			} else {
				out.Stats = []*pb.SkillNumericComparison{distance, out.Stats[0], cost}
			}
		}
		// b 保存状态效果返回的结果，found 保存由状态效果取得的已找到标记。
		if b, found := f.Buff(s.BuffID); found {
			// comparison 封装“comparison”对应的后续处理，key 记录键，使用文本表示，before 记录操作前的状态，使用数值表示，after 记录操作后的状态，使用数值表示。
			comparison := func(key string, before, after int64) *pb.SkillNumericComparison {
				return &pb.SkillNumericComparison{Key: key, BeforeMin: before, BeforeMax: before, AfterMin: after, AfterMax: after}
			}
			out.Stats = []*pb.SkillNumericComparison{comparison("mp_cost", s.MPBaseCost+int64(p.From)*s.MPLevelCost, s.MPBaseCost+int64(p.To)*s.MPLevelCost)}
			if b.Kind == "teleport_mana_penalty" {
				// Flux固定时长不使用道士逐阶秒数，也不附加伤害或护符比较项。
				out.Stats = append(out.Stats, comparison("duration_ms", b.DurationMS, b.DurationMS), comparison("teleport_mana_penalty_percent", b.ValueByLevel[p.From], b.ValueByLevel[p.To]))
			} else if b.Kind == "magic_shield" {
				out.Stats = append(out.Stats, comparison("damage_reduction_percent", b.ValueByLevel[p.From], b.ValueByLevel[p.To]), comparison("duration_rank_factor", int64(p.From)+1, int64(p.To)+1))
			} else {
				out.Stats = append(out.Stats, comparison("base_duration_seconds", b.DurationSecondsByLevel[p.From], b.DurationSecondsByLevel[p.To]), comparison("material_quantity", s.MaterialQuantity, s.MaterialQuantity))
			}
		}
		if s.Behavior == "slaying" {
			out.Stats = append(out.Stats, &pb.SkillNumericComparison{Key: "max_attack_bonus", BeforeMin: s.MaximumAttackBonus(p.From), BeforeMax: s.MaximumAttackBonus(p.From), AfterMin: s.MaximumAttackBonus(p.To), AfterMax: s.MaximumAttackBonus(p.To)})
		}
	}
	return out
}
