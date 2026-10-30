package persistence

import (
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"reflect"
	"testing"
)

func TestWarehouseSplitConservesCompleteOriginalIdentity(t *testing.T) {
	i := domain.Item{ID: domain.ID(), ItemID: 900001, Quantity: 17, Version: 8, Bound: true, Quality: 2, Locked: true, SourceKind: "skill_recycle", SourceID: "original-source"}
	before := i
	for _, quantity := range []int64{1, 8, 16, 17} {
		out, remaining, ok := warehouseSplit(i, quantity, domain.ID())
		if !ok {
			t.Fatal("合法转移失败")
		}
		if i != before {
			t.Fatal("改写原物品")
		}
		total := out.Quantity
		if remaining != nil {
			total += remaining.Quantity
			if remaining.ID != i.ID || remaining.Version != i.Version+1 {
				t.Fatal("原余量身份丢失")
			}
		}
		if total != i.Quantity {
			t.Fatal("数量不守恒")
		}
		normalized := out
		normalized.ID = i.ID
		normalized.Quantity = i.Quantity
		normalized.Version = i.Version
		if !reflect.DeepEqual(normalized, i) {
			t.Fatal("移动时改变原件字段")
		}
		if remaining == nil {
			if out.ID != i.ID || out.Version != i.Version+1 {
				t.Fatal("完整移动没有保留UID/增加版本")
			}
		} else if out.ID == i.ID || out.Version != 1 {
			t.Fatal("拆分没有新生身份")
		}
	}
	for _, quantity := range []int64{-1, 0, 18, math.MaxInt64} {
		if _, _, ok := warehouseSplit(i, quantity, domain.ID()); ok {
			t.Fatal("非法数量", quantity)
		}
	}
	i.Version = math.MaxInt64
	if _, _, ok := warehouseSplit(i, 1, domain.ID()); ok {
		t.Fatal("版本回绕")
	}
}
func TestWarehouseIdentityUsesPermanentRoleOrigin(t *testing.T) {
	cfg := &devconfig.WarehouseRules{PersonalInitialSlots: 30, SharedSlots: 50}
	r := domain.Role{ID: domain.ID(), AccountID: domain.ID(), Origin: 7}
	a, err := warehouseIdentity(r, WarehouseShared, cfg)
	if err != nil {
		t.Fatal(err)
	}
	other := r
	other.ID = domain.ID()
	b, err := warehouseIdentity(other, WarehouseShared, cfg)
	if err != nil || a.ID != b.ID {
		t.Fatal("同origin不同角色不共享")
	}
	other.Origin = 8
	c, _ := warehouseIdentity(other, WarehouseShared, cfg)
	if c.ID == a.ID {
		t.Fatal("同run其它永久入口合仓")
	}
	d, _ := warehouseIdentity(other, WarehousePersonal, cfg)
	e, _ := warehouseIdentity(r, WarehousePersonal, cfg)
	if d.ID == e.ID {
		t.Fatal("个人仓合并")
	}
}
func TestWarehouseExplicitActionsRejectUnrelatedFields(t *testing.T) {
	base := &pb.WarehouseAction{Scope: 1, Action: 1, ItemInstanceId: domain.Bytes(domain.ID()), ItemVersion: 1, Quantity: 1}
	if !validWarehouseAction(base) {
		t.Fatal("合法存入")
	}
	v := proto.Clone(base).(*pb.WarehouseAction)
	v.Amount = 1
	if validWarehouseAction(v) {
		t.Fatal("存物带币")
	}
	v = proto.Clone(base).(*pb.WarehouseAction)
	v.Quantity = 0
	if validWarehouseAction(v) {
		t.Fatal("默认数量")
	}
	if validWarehouseAction(&pb.WarehouseAction{Scope: 1, Action: 3, Currency: 1, Amount: 1}) {
		t.Fatal("个人存币")
	}
	if validWarehouseAction(&pb.WarehouseAction{Scope: 2, Action: 5}) {
		t.Fatal("共享扩页")
	}
	if !validWarehouseAction(&pb.WarehouseAction{Scope: 2, Action: 4, Currency: 2, Amount: 1}) {
		t.Fatal("合法取元宝")
	}
}
func TestWarehouseReceiptSnapshotsDoNotAliasLaterState(t *testing.T) {
	i := domain.Item{ID: domain.ID(), Quantity: 3, Version: 1}
	in := &WarehouseReceipt{Original: &i, Changes: []WarehouseChange{{Slot: 0, Before: &i}}}
	copy := cloneWarehouseReceipt(in)
	i.Quantity = 1
	in.Changes[0].Slot = 9
	if copy.Original.Quantity != 3 || copy.Changes[0].Before.Quantity != 3 || copy.Changes[0].Slot != 0 {
		t.Fatal("旧回执随新状态变化")
	}
}
