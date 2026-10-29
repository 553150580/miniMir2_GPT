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

func skillPageMergeFixture(t *testing.T, quantities ...int64) (domain.Role, devconfig.Fixture) {
	t.Helper()
	r, f := skillEconomyFixture(t)
	r.Items = nil
	source := domain.ID()
	for i, quantity := range quantities {
		r.Items = append(r.Items, domain.Item{ID: domain.ID(), ItemID: f.SkillEconomy.PageItemID, Quantity: quantity, Version: int64(i + 1), SourceKind: "skill_recycle", SourceID: source, Bound: true})
	}
	r.Capacity = len(r.Items)
	return r, f
}

func skillPageMergeRequest(r domain.Role) *pb.SkillAssetPreviewRequest {
	q := &pb.SkillAssetPreviewRequest{Kind: 4}
	for _, item := range r.Items {
		q.Materials = append(q.Materials, &pb.ItemConsumption{InstanceId: domain.Bytes(item.ID), Quantity: item.Quantity, Version: uint64(item.Version)})
	}
	return q
}

func TestSkillPageMergeReleasesFullBagSlotAndKeepsExactSource(t *testing.T) {
	for _, source := range []string{"skill_recycle", "npc_shop", "monster_drop"} {
		t.Run(source, func(t *testing.T) {
			r, f := skillPageMergeFixture(t, 5, 5)
			for i := range r.Items {
				r.Items[i].SourceKind = source
			}
			before := append([]domain.Item(nil), r.Items...)
			plan, code := buildSkillPlan(r, skillPageMergeRequest(r), f)
			if code != 0 || len(plan.Materials) != 2 || len(plan.Outputs) != 1 || plan.Outputs[0].Quantity != 10 || plan.Outputs[0].Version != 1 || plan.Outputs[0].ID != "" || !reflect.DeepEqual(before, r.Items) {
				t.Fatal("满包预览未释放格子或改变原件", code, plan, r.Items)
			}
			if !reflect.DeepEqual(normalizedSkillPage(plan.Outputs[0]), normalizedSkillPage(r.Items[0])) {
				t.Fatal("预览改写了来源、绑定或品质")
			}
			preview := (skillQuote{Plan: plan}).Proto(f)
			if len(preview.Materials) != 2 || len(preview.Outputs) != 1 || preview.Outputs[0].SourceId != r.Items[0].SourceID || preview.Outputs[0].SourceKind != source || preview.Outputs[0].Version != 1 {
				t.Fatal("预览缺少实际原件或原来源", preview)
			}
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

func TestSkillPageMergeSplitsByKnownMaximumAndPinsOriginalMaterials(t *testing.T) {
	r, f := skillPageMergeFixture(t, 30000, 30000, 10000)
	definition, known := f.Item(f.SkillEconomy.PageItemID)
	if !known || definition.MaxStack != 65535 {
		t.Fatal("本用例依赖当前唯一Excel书页最大堆叠")
	}
	plan, code := buildSkillPlan(r, skillPageMergeRequest(r), f)
	if code != 0 || len(plan.Outputs) != 2 || plan.Outputs[0].Quantity != definition.MaxStack || plan.Outputs[1].Quantity != 70000-definition.MaxStack {
		t.Fatal("未按已知MaxStack拆分或未减少格子", code, plan)
	}
	for i, material := range plan.Materials {
		if material.Quantity != material.Item.Quantity || i > 0 && plan.Materials[i-1].Item.ID >= material.Item.ID {
			t.Fatal("没有冻结完整原件并按身份规范化材料")
		}
	}
	out, applied := applySkillPlan(r, plan, domain.ID())
	if len(out.Items) != 2 || applied.Outputs[0].ID == applied.Outputs[1].ID || applied.Outputs[0].SourceID != r.Items[0].SourceID || applied.Outputs[1].SourceID != r.Items[0].SourceID {
		t.Fatal("拆分输出身份或来源错误", out.Items)
	}
	snapshot := cloneSkillPlan(&applied)
	applied.Materials[0].Item.SourceID = "changed"
	applied.Outputs[0].SourceID = "changed"
	if snapshot.Materials[0].Item.SourceID != r.Items[0].SourceID || snapshot.Outputs[0].SourceID != r.Items[0].SourceID {
		t.Fatal("typed回执未复制输入输出")
	}
}

func TestSkillPageMergeRejectsMixedUnknownProtectedAndPartialInputs(t *testing.T) {
	cases := []struct {
		name   string
		change func(*domain.Role, *devconfig.Fixture, *pb.SkillAssetPreviewRequest)
	}{
		{"single", func(r *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			q.Materials = q.Materials[:1]
		}},
		{"duplicate", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			q.Materials[1] = q.Materials[0]
		}},
		{"not_owned", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			q.Materials[1].InstanceId = domain.Bytes(domain.ID())
		}},
		{"partial", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) { q.Materials[0].Quantity-- }},
		{"stale_version", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) { q.Materials[0].Version++ }},
		{"zero_version", func(r *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			r.Items[0].Version, q.Materials[0].Version = 0, 0
		}},
		{"exhausted_version", func(r *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			r.Items[0].Version, q.Materials[0].Version = math.MaxInt64, math.MaxInt64
		}},
		{"negative_quantity", func(r *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			r.Items[0].Quantity, q.Materials[0].Quantity = -1, -1
		}},
		{"over_maximum", func(r *domain.Role, f *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) {
			d, _ := f.Item(r.Items[0].ItemID)
			r.Items[0].Quantity, q.Materials[0].Quantity = d.MaxStack+1, d.MaxStack+1
		}},
		{"source_id", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			r.Items[1].SourceID = domain.ID()
		}},
		{"source_kind", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			r.Items[1].SourceKind = "npc_shop"
		}},
		{"bound", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) { r.Items[1].Bound = false }},
		{"quality", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) { r.Items[1].Quality = 1 }},
		{"missing_source_id", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			for i := range r.Items {
				r.Items[i].SourceID = ""
			}
		}},
		{"unknown_source_kind", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			for i := range r.Items {
				r.Items[i].SourceKind = ""
			}
		}},
		{"unregistered_source_kind", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			for i := range r.Items {
				r.Items[i].SourceKind = "initial_grant"
			}
		}},
		{"locked", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			for i := range r.Items {
				r.Items[i].Locked = true
			}
		}},
		{"pending", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			for i := range r.Items {
				r.Items[i].PendingResultID = "pending"
			}
		}},
		{"reserved", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			for i := range r.Items {
				r.Items[i].ReservationID = "reserved"
			}
		}},
		{"worn_duplicate", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			r.Equipped = []domain.Equipped{{Slot: 0, Item: r.Items[0]}}
		}},
		{"not_page", func(r *domain.Role, f *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			for i := range f.Items {
				if f.Items[i].ID == r.Items[0].ItemID {
					f.Items[i].IsSkillPage = false
				}
			}
		}},
		{"not_stackable", func(r *domain.Role, f *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			for i := range f.Items {
				if f.Items[i].ID == r.Items[0].ItemID {
					f.Items[i].Stackable = false
				}
			}
		}},
		{"unknown_maximum", func(r *domain.Role, f *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			for i := range f.Items {
				if f.Items[i].ID == r.Items[0].ItemID {
					f.Items[i].MaxStack = 0
				}
			}
		}},
		{"unit_maximum", func(r *domain.Role, f *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			for i := range f.Items {
				if f.Items[i].ID == r.Items[0].ItemID {
					f.Items[i].MaxStack = 1
				}
			}
		}},
		{"offer", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) { q.OfferId = 1 }},
		{"quantity", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) { q.Quantity = 1 }},
		{"skill", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.SkillAssetPreviewRequest) { q.SkillId = 1 }},
		{"preview_budget", func(_ *domain.Role, f *devconfig.Fixture, _ *pb.SkillAssetPreviewRequest) {
			f.SkillEconomy.PreviewMaterialLimit = 1
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			r, f := skillPageMergeFixture(t, 5, 5)
			q := skillPageMergeRequest(r)
			test.change(&r, &f, q)
			before := append([]domain.Item(nil), r.Items...)
			if _, code := buildSkillPlan(r, q, f); code != p.CodeSkillMaterials || !reflect.DeepEqual(before, r.Items) {
				t.Fatal("非法合并被接受或改变原件", code, r.Items)
			}
		})
	}
}

func TestSkillPageMergeRequiresActualSlotReductionAndChecksInt64Sum(t *testing.T) {
	r, f := skillPageMergeFixture(t, 65535, 1)
	if _, code := buildSkillPlan(r, skillPageMergeRequest(r), f); code != p.CodeSkillMaterials {
		t.Fatal("没有减少槽位仍可换新身份")
	}
	for i := range f.Items {
		if f.Items[i].ID == f.SkillEconomy.PageItemID {
			f.Items[i].MaxStack = math.MaxInt64
		}
	}
	r.Items[0].Quantity, r.Items[1].Quantity = math.MaxInt64-1, 2
	if _, code := buildSkillPlan(r, skillPageMergeRequest(r), f); code != p.CodeSkillMaterials {
		t.Fatal("原堆叠总量int64溢出未拒绝")
	}
	r.Items[1].Quantity = 1
	plan, code := buildSkillPlan(r, skillPageMergeRequest(r), f)
	if code != 0 || len(plan.Outputs) != 1 || plan.Outputs[0].Quantity != math.MaxInt64 {
		t.Fatal("合法边界总量被拒绝", code, plan)
	}
}

func TestSkillPageMergeDoesNotChangeBookRecyclingLineage(t *testing.T) {
	r, f := skillEconomyFixture(t)
	rule := f.SkillBookRecycle[0]
	r.Items = []domain.Item{{ID: domain.ID(), ItemID: rule.ItemID, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: "first", Quality: rule.BookQuality, Bound: true}, {ID: domain.ID(), ItemID: rule.ItemID, Quantity: 1, Version: 1, SourceKind: "monster_drop", SourceID: "second", Quality: rule.BookQuality, Bound: true}}
	q := skillPageMergeRequest(r)
	q.Kind = 3
	plan, code := buildSkillPlan(r, q, f)
	if code != 0 || len(plan.Outputs) != 2 {
		t.Fatal("原拆书被合并", code, plan)
	}
	op := domain.ID()
	out, _ := applySkillPlan(r, plan, op)
	if len(out.Items) != 2 || out.Items[0].SourceID != op || out.Items[1].SourceID != op || out.Items[0].SourceKind != "skill_recycle" || out.Items[1].SourceKind != "skill_recycle" {
		t.Fatal("原拆书逐书来源合同被更改", out.Items)
	}
	merge, code := buildSkillPlan(out, skillPageMergeRequest(out), f)
	if code != 0 || len(merge.Outputs) != 1 || merge.Outputs[0].Quantity != 2*rule.PagesPerBook || merge.Outputs[0].SourceID != op {
		t.Fatal("同一次拆书产生的同源书页无法显式合并", code, merge)
	}
}
