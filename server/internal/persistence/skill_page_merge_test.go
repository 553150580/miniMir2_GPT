// 验证技能页堆叠合并的来源一致性、产物身份和背包容量约束。

package persistence

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
)

// skillPageMergeFixture 处理技能页面或分页合并开发测试配置。
// t 提供断言失败报告、子用例和清理注册，quantities 记录“quantities”，使用可变数量的数值表示。
func skillPageMergeFixture(t *testing.T, quantities ...int64) (domain.Role, devconfig.Fixture) {
	t.Helper()
	// r 接收技能经济开发测试配置供后续步骤使用，f 接收技能经济开发测试配置供后续步骤使用。
	r, f := skillEconomyFixture(t)
	r.Items = nil
	// source 保存由标识取得的来源。
	source := domain.ID()
	// i 定位“quantities”中正在处理的成员，quantity 接收遍历“quantities”时当前的元素。
	for i, quantity := range quantities {
		r.Items = append(r.Items, domain.Item{ID: domain.ID(), ItemID: f.SkillEconomy.PageItemID, Quantity: quantity, Version: int64(i + 1), SourceKind: "skill_recycle", SourceID: source, Bound: true})
	}
	r.Capacity = len(r.Items)
	return r, f
}

// skillPageMergeRequest 返回技能页面或分页合并请求供调用方使用。
// r 记录角色。
func skillPageMergeRequest(r domain.Role) *pb.SkillAssetPreviewRequest {
	// q 保存当前处理所用的技能资产预览请求。
	q := &pb.SkillAssetPreviewRequest{Kind: 4}
	// item 接收遍历道具集合时当前的元素。
	for _, item := range r.Items {
		q.Materials = append(q.Materials, &pb.ItemConsumption{InstanceId: domain.Bytes(item.ID), Quantity: item.Quantity, Version: uint64(item.Version)})
	}
	return q
}

// TestSkillPageMergeReleasesFullBagSlotAndKeepsExactSource 核对「提交未保留原来源或原完整堆叠未全部消耗」与「合并污染原预览、原角色或无关资产」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPageMergeReleasesFullBagSlotAndKeepsExactSource(t *testing.T) {
	// source 接收遍历文本列表时当前的元素。
	for _, source := range []string{"skill_recycle", "npc_shop", "monster_drop"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(source, func(t *testing.T) {
			// r 接收技能页面或分页合并开发测试配置供后续步骤使用，f 接收技能页面或分页合并开发测试配置供后续步骤使用。
			r, f := skillPageMergeFixture(t, 5, 5)
			// i 标记遍历道具集合时访问的下标。
			for i := range r.Items {
				r.Items[i].SourceKind = source
			}
			// before 保留加入新条目后的操作前的状态。
			before := append([]domain.Item(nil), r.Items...)
			// plan 保存由构建技能计划取得的计划，code 保存由构建技能计划取得的结果码。
			plan, code := buildSkillPlan(r, skillPageMergeRequest(r), f)
			if code != 0 || len(plan.Materials) != 2 || len(plan.Outputs) != 1 || plan.Outputs[0].Quantity != 10 || plan.Outputs[0].Version != 1 || plan.Outputs[0].ID != "" || !reflect.DeepEqual(before, r.Items) {
				t.Fatal("满包预览未释放格子或改变原件", code, plan, r.Items)
			}
			if !reflect.DeepEqual(normalizedSkillPage(plan.Outputs[0]), normalizedSkillPage(r.Items[0])) {
				t.Fatal("预览改写了来源、绑定或品质")
			}
			// preview 保存由协议取得的预览。
			preview := (skillQuote{Plan: plan}).Proto(f)
			if len(preview.Materials) != 2 || len(preview.Outputs) != 1 || preview.Outputs[0].SourceId != r.Items[0].SourceID || preview.Outputs[0].SourceKind != source || preview.Outputs[0].Version != 1 {
				t.Fatal("预览缺少实际原件或原来源", preview)
			}
			// out 保存由应用技能计划取得的输出，committed 保存由应用技能计划取得的已提交。
			out, committed := applySkillPlan(r, plan, domain.ID())
			if len(out.Items) != 1 || out.Items[0].ID == "" || out.Items[0].ID == before[0].ID || out.Items[0].ID == before[1].ID || out.Items[0].Version != 1 || out.Items[0].Quantity != 10 || out.Items[0].SourceID != before[0].SourceID || !reflect.DeepEqual(out.Items, committed.Outputs) {
				t.Fatal("提交未保留原来源或原完整堆叠未全部消耗", out.Items, committed)
			}
			if !reflect.DeepEqual(before, r.Items) || plan.Outputs[0].ID != "" || out.Wallet != r.Wallet || !reflect.DeepEqual(out.Skills, r.Skills) {
				t.Fatal("合并污染原预览、原角色或无关资产")
			}
		})
	}
}

// TestSkillPageMergeSplitsByKnownMaximumAndPinsOriginalMaterials 核对「本用例依赖当前唯一Excel书页最大堆叠」与「未按已知MaxStack拆分或未减少格子」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPageMergeSplitsByKnownMaximumAndPinsOriginalMaterials(t *testing.T) {
	// r 接收技能页面或分页合并开发测试配置供后续步骤使用，f 接收技能页面或分页合并开发测试配置供后续步骤使用。
	r, f := skillPageMergeFixture(t, 30000, 30000, 10000)
	// definition 保存由道具取得的定义，known 保存由道具取得的已知状态。
	definition, known := f.Item(f.SkillEconomy.PageItemID)
	if !known || definition.MaxStack != 65535 {
		t.Fatal("本用例依赖当前唯一Excel书页最大堆叠")
	}
	// plan 保存由构建技能计划取得的计划，code 保存由构建技能计划取得的结果码。
	plan, code := buildSkillPlan(r, skillPageMergeRequest(r), f)
	if code != 0 || len(plan.Outputs) != 2 || plan.Outputs[0].Quantity != definition.MaxStack || plan.Outputs[1].Quantity != 70000-definition.MaxStack {
		t.Fatal("未按已知MaxStack拆分或未减少格子", code, plan)
	}
	// i 定位材料集合中正在处理的成员，material 接收遍历材料集合时当前的元素。
	for i, material := range plan.Materials {
		if material.Quantity != material.Item.Quantity || i > 0 && plan.Materials[i-1].Item.ID >= material.Item.ID {
			t.Fatal("没有冻结完整原件并按身份规范化材料")
		}
	}
	// out 保存由应用技能计划取得的输出，applied 保存由应用技能计划取得的“applied”。
	out, applied := applySkillPlan(r, plan, domain.ID())
	if len(out.Items) != 2 || applied.Outputs[0].ID == applied.Outputs[1].ID || applied.Outputs[0].SourceID != r.Items[0].SourceID || applied.Outputs[1].SourceID != r.Items[0].SourceID {
		t.Fatal("拆分输出身份或来源错误", out.Items)
	}
	// snapshot 保存由副本技能计划取得的快照。
	snapshot := cloneSkillPlan(&applied)
	applied.Materials[0].Item.SourceID = "changed"
	applied.Outputs[0].SourceID = "changed"
	if snapshot.Materials[0].Item.SourceID != r.Items[0].SourceID || snapshot.Outputs[0].SourceID != r.Items[0].SourceID {
		t.Fatal("typed回执未复制输入输出")
	}
}

// TestSkillPageMergeRejectsMixedUnknownProtectedAndPartialInputs 核对「非法合并被接受或改变原件」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPageMergeRejectsMixedUnknownProtectedAndPartialInputs(t *testing.T) {
	// cases 建立带初始值的临时结构列表，供本段继续处理。
	cases := []struct {
		// name 记录名称，使用文本表示。
		name string
		// change 记录变更，使用供当前流程调用的函数表示。
		change func(*domain.Role, *devconfig.Fixture, *pb.SkillAssetPreviewRequest)
	}{
		// r 记录角色，q 记录技能资产预览请求。
		{"single", func(r *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			q.Materials = q.Materials[:1]
		}},
		// q 记录技能资产预览请求。
		{"duplicate", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			q.Materials[1] = q.Materials[0]
		}},
		// q 记录技能资产预览请求。
		{"not_owned", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			q.Materials[1].InstanceId = domain.Bytes(domain.ID())
		}},
		// q 记录技能资产预览请求。
		{"partial", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) { q.Materials[0].Quantity-- }},
		// q 记录技能资产预览请求。
		{"stale_version", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) { q.Materials[0].Version++ }},
		// r 记录角色，q 记录技能资产预览请求。
		{"zero_version", func(r *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			r.Items[0].Version, q.Materials[0].Version = 0, 0
		}},
		// r 记录角色，q 记录技能资产预览请求。
		{"exhausted_version", func(r *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			r.Items[0].Version, q.Materials[0].Version = math.MaxInt64, math.MaxInt64
		}},
		// r 记录角色，q 记录技能资产预览请求。
		{"negative_quantity", func(r *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			r.Items[0].Quantity, q.Materials[0].Quantity = -1, -1
		}},
		// r 记录角色，f 记录开发测试配置，q 记录技能资产预览请求。
		{"over_maximum", func(r *domain.Role, f *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			// d 接收道具供后续步骤使用。
			d, _ := f.Item(r.Items[0].ItemID)
			r.Items[0].Quantity, q.Materials[0].Quantity = d.MaxStack+1, d.MaxStack+1
		}},
		// r 记录角色。
		{"source_id", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			r.Items[1].SourceID = domain.ID()
		}},
		// r 记录角色。
		{"source_kind", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			r.Items[1].SourceKind = "npc_shop"
		}},
		// r 记录角色。
		{"bound", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) { r.Items[1].Bound = false }},
		// r 记录角色。
		{"quality", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) { r.Items[1].Quality = 1 }},
		// r 记录角色。
		{"missing_source_id", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			// i 标记遍历道具集合时访问的下标。
			for i := range r.Items {
				r.Items[i].SourceID = ""
			}
		}},
		// r 记录角色。
		{"unknown_source_kind", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			// i 标记遍历道具集合时访问的下标。
			for i := range r.Items {
				r.Items[i].SourceKind = ""
			}
		}},
		// r 记录角色。
		{"unregistered_source_kind", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			// i 标记遍历道具集合时访问的下标。
			for i := range r.Items {
				r.Items[i].SourceKind = "initial_grant"
			}
		}},
		// r 记录角色。
		{"locked", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			// i 标记遍历道具集合时访问的下标。
			for i := range r.Items {
				r.Items[i].Locked = true
			}
		}},
		// r 记录角色。
		{"pending", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			// i 标记遍历道具集合时访问的下标。
			for i := range r.Items {
				r.Items[i].PendingResultID = "pending"
			}
		}},
		// r 记录角色。
		{"reserved", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			// i 标记遍历道具集合时访问的下标。
			for i := range r.Items {
				r.Items[i].ReservationID = "reserved"
			}
		}},
		// r 记录角色。
		{"worn_duplicate", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			r.Equipped = []domain.Equipped{{Slot: 0, Item: r.Items[0]}}
		}},
		// r 记录角色，f 记录开发测试配置。
		{"not_page", func(r *domain.Role, f *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			// i 标记遍历道具集合时访问的下标。
			for i := range f.Items {
				if f.Items[i].ID == r.Items[0].ItemID {
					f.Items[i].IsSkillPage = false
				}
			}
		}},
		// r 记录角色，f 记录开发测试配置。
		{"not_stackable", func(r *domain.Role, f *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			// i 标记遍历道具集合时访问的下标。
			for i := range f.Items {
				if f.Items[i].ID == r.Items[0].ItemID {
					f.Items[i].Stackable = false
				}
			}
		}},
		// r 记录角色，f 记录开发测试配置。
		{"unknown_maximum", func(r *domain.Role, f *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			// i 标记遍历道具集合时访问的下标。
			for i := range f.Items {
				if f.Items[i].ID == r.Items[0].ItemID {
					f.Items[i].MaxStack = 0
				}
			}
		}},
		// r 记录角色，f 记录开发测试配置。
		{"unit_maximum", func(r *domain.Role, f *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			// i 标记遍历道具集合时访问的下标。
			for i := range f.Items {
				if f.Items[i].ID == r.Items[0].ItemID {
					f.Items[i].MaxStack = 1
				}
			}
		}},
		// q 记录技能资产预览请求。
		{"offer", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) { q.OfferId = 1 }},
		// q 记录技能资产预览请求。
		{"quantity", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) { q.Quantity = 1 }},
		// q 记录技能资产预览请求。
		{"skill", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) { q.SkillId = 1 }},
		// f 记录开发测试配置。
		{"preview_budget", func(_ *domain.Role, f *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			f.SkillEconomy.PreviewMaterialLimit = 1
		}},
	}
	// test 接收遍历“cases”时当前的元素。
	for _, test := range cases {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(test.name, func(t *testing.T) {
			// r 接收技能页面或分页合并开发测试配置供后续步骤使用，f 接收技能页面或分页合并开发测试配置供后续步骤使用。
			r, f := skillPageMergeFixture(t, 5, 5)
			// q 接收技能页面或分页合并请求供后续步骤使用。
			q := skillPageMergeRequest(r)
			test.change(&r, &f, q)
			// before 保留加入新条目后的操作前的状态。
			before := append([]domain.Item(nil), r.Items...)
			// code 保存由构建技能计划取得的结果码。
			if _, code := buildSkillPlan(r, q, f); code != p.CodeSkillMaterials || !reflect.DeepEqual(before, r.Items) {
				t.Fatal("非法合并被接受或改变原件", code, r.Items)
			}
		})
	}
}

// TestSkillPageMergeRequiresActualSlotReductionAndChecksInt64Sum 核对「原堆叠总量int64溢出未拒绝」与「没有减少槽位仍可换新身份」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPageMergeRequiresActualSlotReductionAndChecksInt64Sum(t *testing.T) {
	// r 接收技能页面或分页合并开发测试配置供后续步骤使用，f 接收技能页面或分页合并开发测试配置供后续步骤使用。
	r, f := skillPageMergeFixture(t, 65535, 1)
	// code 保存由构建技能计划取得的结果码。
	if _, code := buildSkillPlan(r, skillPageMergeRequest(r), f); code != p.CodeSkillMaterials {
		t.Fatal("没有减少槽位仍可换新身份")
	}
	// i 标记遍历道具集合时访问的下标。
	for i := range f.Items {
		if f.Items[i].ID == f.SkillEconomy.PageItemID {
			f.Items[i].MaxStack = math.MaxInt64
		}
	}
	r.Items[0].Quantity, r.Items[1].Quantity = math.MaxInt64-1, 2
	// code 保存由构建技能计划取得的结果码。
	if _, code := buildSkillPlan(r, skillPageMergeRequest(r), f); code != p.CodeSkillMaterials {
		t.Fatal("原堆叠总量int64溢出未拒绝")
	}
	r.Items[1].Quantity = 1
	// plan 保存由构建技能计划取得的计划，code 保存由构建技能计划取得的结果码。
	plan, code := buildSkillPlan(r, skillPageMergeRequest(r), f)
	if code != 0 || len(plan.Outputs) != 1 || plan.Outputs[0].Quantity != math.MaxInt64 {
		t.Fatal("合法边界总量被拒绝", code, plan)
	}
}

// TestSkillPageMergeDoesNotChangeBookRecyclingLineage 核对「同一次拆书产生的同源书页无法显式合并」与「原拆书逐书来源合同被更改」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillPageMergeDoesNotChangeBookRecyclingLineage(t *testing.T) {
	// r 接收技能经济开发测试配置供后续步骤使用，f 接收技能经济开发测试配置供后续步骤使用。
	r, f := skillEconomyFixture(t)
	// rule 保存从技能技能书回收取出的规则。
	rule := f.SkillBookRecycle[0]
	r.Items = []domain.Item{{ID: domain.ID(), ItemID: rule.ItemID, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: "first", Quality: rule.BookQuality, Bound: true}, {ID: domain.ID(), ItemID: rule.ItemID, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: "second", Quality: rule.BookQuality, Bound: true}}
	// q 接收技能页面或分页合并请求供后续步骤使用。
	q := skillPageMergeRequest(r)
	q.Kind = 3
	// plan 保存由构建技能计划取得的计划，code 保存由构建技能计划取得的结果码。
	plan, code := buildSkillPlan(r, q, f)
	if code != 0 || len(plan.Outputs) != 2 {
		t.Fatal("原拆书被合并", code, plan)
	}
	// op 保存由标识取得的操作。
	op := domain.ID()
	// out 保存由应用技能计划取得的输出。
	out, _ := applySkillPlan(r, plan, op)
	if len(out.Items) != 2 || out.Items[0].SourceID != op || out.Items[1].SourceID != op || out.Items[0].SourceKind != "skill_recycle" || out.Items[1].SourceKind != "skill_recycle" {
		t.Fatal("原拆书逐书来源合同被更改", out.Items)
	}
	// merge 保存由构建技能计划取得的合并。
	merge, code := buildSkillPlan(out, skillPageMergeRequest(out), f)
	if code != 0 || len(merge.Outputs) != 1 || merge.Outputs[0].Quantity != 2*rule.PagesPerBook || merge.Outputs[0].SourceID != op {
		t.Fatal("同一次拆书产生的同源书页无法显式合并", code, merge)
	}
}
