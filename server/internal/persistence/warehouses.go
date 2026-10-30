package persistence

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	WarehousePersonal uint32 = 1
	WarehouseShared   uint32 = 2
)

// 元数据和物品分别存储，个人容量增长不扩大角色聚合或单次网络报文。
type Warehouse struct {
	ID        string `bson:"_id"`
	Scope     uint32 `bson:"scope"`
	RoleID    string `bson:"role_id,omitempty"`
	AccountID string `bson:"account_id"`
	Origin    uint64 `bson:"origin_server_id"`
	Revision  int64  `bson:"revision"`
	Capacity  int64  `bson:"capacity"`
	Gold      int64  `bson:"gold"`
	Yuanbao   int64  `bson:"yuanbao_units"`
}
type WarehouseStoredItem struct {
	ID          string      `bson:"_id"`
	WarehouseID string      `bson:"warehouse_id"`
	Slot        int64       `bson:"slot"`
	Item        domain.Item `bson:"item"`
}
type WarehouseChange struct {
	Slot   uint64       `bson:"slot"`
	Before *domain.Item `bson:"before,omitempty"`
	After  *domain.Item `bson:"after,omitempty"`
}
type WarehouseReceipt struct {
	Scope            uint32            `bson:"scope"`
	Action           uint32            `bson:"action"`
	WarehouseID      string            `bson:"warehouse_id"`
	RevisionBefore   int64             `bson:"revision_before"`
	RevisionAfter    int64             `bson:"revision_after"`
	Capacity         int64             `bson:"capacity"`
	Changes          []WarehouseChange `bson:"changes"`
	Original         *domain.Item      `bson:"original,omitempty"`
	Transferred      *domain.Item      `bson:"transferred,omitempty"`
	Remainder        *domain.Item      `bson:"remainder,omitempty"`
	RoleWalletBefore domain.Wallet     `bson:"role_wallet_before"`
	RoleWalletAfter  domain.Wallet     `bson:"role_wallet_after"`
	GoldBefore       int64             `bson:"gold_before"`
	GoldAfter        int64             `bson:"gold_after"`
	YuanbaoBefore    int64             `bson:"yuanbao_before"`
	YuanbaoAfter     int64             `bson:"yuanbao_after"`
	RuleVersion      uint64            `bson:"rule_version"`
	ConfigHash       string            `bson:"config_hash"`
}

func copyWarehouseItem(i *domain.Item) *domain.Item {
	if i == nil {
		return nil
	}
	v := *i
	return &v
}
func cloneWarehouseReceipt(in *WarehouseReceipt) *WarehouseReceipt {
	if in == nil {
		return nil
	}
	out := *in
	out.Original = copyWarehouseItem(in.Original)
	out.Transferred = copyWarehouseItem(in.Transferred)
	out.Remainder = copyWarehouseItem(in.Remainder)
	out.Changes = slices.Clone(in.Changes)
	for i := range out.Changes {
		out.Changes[i].Before = copyWarehouseItem(out.Changes[i].Before)
		out.Changes[i].After = copyWarehouseItem(out.Changes[i].After)
	}
	return &out
}
func (r *WarehouseReceipt) Proto(role domain.Role, op Operation) *pb.WarehouseActionResult {
	if r == nil {
		return nil
	}
	id := strings.TrimPrefix(op.ID, "asset:"+role.ID+":")
	if len(domain.Bytes(id)) != 16 {
		return nil
	}
	out := &pb.WarehouseActionResult{Role: role.Summary().Key, RoleRevision: uint64(op.Revision), OpId: domain.Bytes(id), Scope: r.Scope, Action: r.Action, WarehouseRevision: uint64(r.RevisionAfter), Capacity: uint64(r.Capacity), Gold: r.GoldAfter, YuanbaoUnits: r.YuanbaoAfter}
	for _, c := range r.Changes {
		v := &pb.WarehouseItemChange{Slot: c.Slot}
		if c.Before != nil {
			v.Before = c.Before.Proto()
		}
		if c.After != nil {
			v.After = c.After.Proto()
		}
		out.Changes = append(out.Changes, v)
	}
	return out
}

func warehouseIdentity(r domain.Role, scope uint32, cfg *devconfig.WarehouseRules) (Warehouse, error) {
	if cfg == nil || r.Origin == 0 || len(domain.Bytes(r.ID)) != 16 || len(domain.Bytes(r.AccountID)) != 16 {
		return Warehouse{}, Fail(p.CodeWarehouseBlocked, "仓库归属或配置不可用")
	}
	w := Warehouse{Scope: scope, AccountID: r.AccountID, Origin: r.Origin}
	switch scope {
	case WarehousePersonal:
		w.ID = "personal:" + r.ID
		w.RoleID = r.ID
		w.Capacity = int64(cfg.PersonalInitialSlots)
	case WarehouseShared:
		w.ID = fmt.Sprintf("shared:%s:%d", r.AccountID, r.Origin)
		w.Capacity = int64(cfg.SharedSlots)
	default:
		return Warehouse{}, Fail(p.CodeInvalidArgument, "仓库类型未知")
	}
	return w, nil
}
func (s *Store) warehouse(ctx context.Context, r domain.Role, scope uint32) (Warehouse, bool, error) {
	w, e := warehouseIdentity(r, scope, s.Cfg.Fixture.Warehouses)
	if e != nil {
		return w, false, e
	}
	expected := w
	e = s.DB.Collection("warehouses").FindOne(ctx, bson.M{"_id": w.ID}).Decode(&w)
	if errors.Is(e, mongo.ErrNoDocuments) {
		return expected, false, nil
	}
	if e != nil {
		return w, false, e
	}
	if w.Scope != expected.Scope || w.RoleID != expected.RoleID || w.AccountID != r.AccountID || w.Origin != r.Origin || w.Revision < 1 || w.Capacity < expected.Capacity || w.Gold < 0 || w.Yuanbao < 0 || scope == WarehousePersonal && (w.Gold != 0 || w.Yuanbao != 0 || uint64(w.Capacity-expected.Capacity)%s.Cfg.Fixture.Warehouses.PersonalExpansionSlots != 0) || scope == WarehouseShared && w.Capacity != expected.Capacity {
		return w, false, Fail(p.CodeWarehouseBlocked, "仓库存档不符合归属或容量合同")
	}
	return w, true, nil
}

// 只读核验不递增run_guards/accountGuard，不冻结角色，也不建立空仓库。
func (s *Store) warehouseReadFence(ctx context.Context, a Auth) error {
	if err := s.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": s.Cfg.RunID, "owner_instance": s.Instance, "run_epoch": s.Epoch, "state": "active", "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}).Err(); err != nil {
		return Fail(p.CodeRouteChanged, "运行所有权已失效")
	}
	if err := s.DB.Collection("node_leases").FindOne(ctx, bson.M{"_id": s.Cfg.NodeID, "owner_instance": s.Instance, "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}).Err(); err != nil {
		return Fail(p.CodeRouteChanged, "节点许可已失效")
	}
	if err := s.DB.Collection("accounts").FindOne(ctx, bson.M{"_id": a.AccountID, "active": true, "auth_version": a.AuthVersion}).Err(); err != nil {
		return Fail(p.CodeUnauthenticated, "账号授权失效")
	}
	return nil
}
func (s *Store) QueryWarehouse(ctx context.Context, a Auth, q *pb.WarehouseQuery) (*pb.WarehousePage, error) {
	cfg := s.Cfg.Fixture.Warehouses
	if cfg == nil || q == nil || q.Limit == 0 || q.Limit > cfg.QueryPageLimit || q.StartSlot > math.MaxInt64 || len(q.ProtoReflect().GetUnknown()) != 0 {
		return nil, Fail(p.CodeInvalidArgument, "仓储分页参数非法")
	}
	var out *pb.WarehousePage
	err := s.tx(ctx, func(tc context.Context) error {
		out = nil
		if err := s.warehouseReadFence(tc, a); err != nil {
			return err
		}
		r, err := s.Role(tc, a.RoleID)
		if err != nil {
			return err
		}
		if err = s.checkRole(r, a); err != nil {
			return err
		}
		if q.Scope == WarehouseShared && !r.PrivilegeUntil.After(time.Now()) {
			return Fail(p.CodeWarehousePrivilegeRequired, "当前角色特权无效")
		}
		w, exists, err := s.warehouse(tc, r, q.Scope)
		if err != nil {
			return err
		}
		if q.StartSlot >= uint64(w.Capacity) {
			return Fail(p.CodeInvalidArgument, "分页起点超过仓库容量")
		}
		end := q.StartSlot + uint64(q.Limit)
		if end > uint64(w.Capacity) {
			end = uint64(w.Capacity)
		}
		page := &pb.WarehousePage{Role: r.Summary().Key, Scope: q.Scope, Revision: uint64(w.Revision), Capacity: uint64(w.Capacity), StartSlot: q.StartSlot, Limit: q.Limit, Gold: w.Gold, YuanbaoUnits: w.Yuanbao}
		cursor, err := s.DB.Collection("warehouse_items").Find(tc, bson.M{"warehouse_id": w.ID, "slot": bson.M{"$gte": int64(q.StartSlot), "$lt": int64(end)}}, options.Find().SetSort(bson.D{{Key: "slot", Value: 1}}).SetLimit(int64(q.Limit)))
		if err != nil {
			return err
		}
		defer cursor.Close(tc)
		for cursor.Next(tc) {
			var x WarehouseStoredItem
			if err = cursor.Decode(&x); err != nil {
				return err
			}
			if !exists || x.ID != x.Item.ID || !warehouseItemValid(x.Item, s.Cfg.Fixture) || q.Scope == WarehouseShared && x.Item.Bound {
				return Fail(p.CodeWarehouseBlocked, "仓储物品状态无效")
			}
			page.Items = append(page.Items, &pb.WarehouseItem{Slot: uint64(x.Slot), Item: x.Item.Proto()})
		}
		if err = cursor.Err(); err != nil {
			return err
		}
		out = page
		return nil
	})
	return out, err
}

func warehouseItemValid(i domain.Item, f devconfig.Fixture) bool {
	if i.ItemID == 0 || len(domain.Bytes(i.ID)) != 16 || domain.Hex(domain.Bytes(i.ID)) != i.ID || i.Version < 1 || i.Quantity < 1 || i.Quality > 4 {
		return false
	}
	// 完整迁移不重解释旧目录；部分拆分另核当前明确的堆叠能力。
	return true
}
func warehouseSplit(original domain.Item, quantity int64, splitID string) (domain.Item, *domain.Item, bool) {
	if quantity <= 0 || quantity > original.Quantity || original.Version <= 0 || original.Version == math.MaxInt64 {
		return domain.Item{}, nil, false
	}
	transferred := original
	transferred.Quantity = quantity
	transferred.Version++
	if quantity == original.Quantity {
		return transferred, nil, true
	}
	if len(domain.Bytes(splitID)) != 16 || domain.Hex(domain.Bytes(splitID)) != splitID || splitID == original.ID {
		return domain.Item{}, nil, false
	}
	remainder := original
	remainder.Quantity -= quantity
	remainder.Version++
	transferred.ID = splitID
	transferred.Version = 1
	return transferred, &remainder, true
}
func validWarehouseAction(q *pb.WarehouseAction) bool {
	if q == nil || q.Scope < 1 || q.Scope > 2 || q.WarehouseRevision >= math.MaxInt64 || q.WarehouseSlot > math.MaxInt64 || len(q.ProtoReflect().GetUnknown()) != 0 {
		return false
	}
	switch q.Action {
	case 1, 2:
		return len(q.ItemInstanceId) == 16 && q.ItemVersion > 0 && q.ItemVersion < math.MaxInt64 && q.Quantity > 0 && q.Currency == 0 && q.Amount == 0
	case 3, 4:
		return q.Scope == WarehouseShared && len(q.ItemInstanceId) == 0 && q.ItemVersion == 0 && q.Quantity == 0 && q.WarehouseSlot == 0 && (q.Currency == 1 || q.Currency == 2) && q.Amount > 0
	case 5:
		return q.Scope == WarehousePersonal && len(q.ItemInstanceId) == 0 && q.ItemVersion == 0 && q.Quantity == 0 && q.WarehouseSlot == 0 && q.Currency == 0 && q.Amount == 0
	}
	return false
}

// 首次永久操作取回执后才会进入本方法；同一原op不会再次拆分、收费或判断到期。
func (s *Store) applyWarehouse(tc context.Context, r domain.Role, q *pb.WarehouseAction) (domain.Role, *WarehouseReceipt, uint32, error) {
	f := s.Cfg.Fixture
	if f.Warehouses == nil || !validWarehouseAction(q) || q.Operation == nil || len(q.Operation.OpId) != 16 || !equipmentRoleUnique(r) {
		return r, nil, p.CodeInvalidArgument, nil
	}
	if q.Scope == WarehouseShared && !r.PrivilegeUntil.After(time.Now()) {
		return r, nil, p.CodeWarehousePrivilegeRequired, nil
	}
	w, exists, err := s.warehouse(tc, r, q.Scope)
	if err != nil {
		return r, nil, 0, err
	}
	if uint64(w.Revision) != q.WarehouseRevision || w.Revision == math.MaxInt64 {
		return r, nil, p.CodeWarehouseChanged, nil
	}
	receipt := &WarehouseReceipt{Scope: q.Scope, Action: q.Action, WarehouseID: w.ID, RevisionBefore: w.Revision, RevisionAfter: w.Revision + 1, Capacity: w.Capacity, RoleWalletBefore: r.Wallet, GoldBefore: w.Gold, YuanbaoBefore: w.Yuanbao, RuleVersion: f.RuleVersion, ConfigHash: f.Hash}
	out := r
	out.Items = slices.Clone(r.Items)
	var put *WarehouseStoredItem
	var remove *WarehouseStoredItem
	switch q.Action {
	case 1, 2:
		if q.WarehouseSlot >= uint64(w.Capacity) {
			return r, nil, p.CodeInvalidArgument, nil
		}
		var stored WarehouseStoredItem
		err = s.DB.Collection("warehouse_items").FindOne(tc, bson.M{"warehouse_id": w.ID, "slot": int64(q.WarehouseSlot)}).Decode(&stored)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return r, nil, 0, err
		}
		bag := -1
		for n, i := range r.Items {
			if i.ID == domain.Hex(q.ItemInstanceId) {
				bag = n
				break
			}
		}
		var original domain.Item
		if q.Action == 1 {
			if err == nil {
				return r, nil, p.CodeWarehouseFull, nil
			}
			if bag < 0 {
				return r, nil, p.CodeWarehouseChanged, nil
			}
			original = r.Items[bag]
			if n, e := s.DB.Collection("warehouse_items").CountDocuments(tc, bson.M{"_id": original.ID}, options.Count().SetLimit(1)); e != nil {
				return r, nil, 0, e
			} else if n != 0 {
				return r, nil, p.CodeWarehouseChanged, nil
			}
		} else {
			if errors.Is(err, mongo.ErrNoDocuments) || stored.ID != stored.Item.ID || stored.ID != domain.Hex(q.ItemInstanceId) || bag >= 0 {
				return r, nil, p.CodeWarehouseChanged, nil
			}
			original = stored.Item
			for _, eq := range r.Equipped {
				if eq.Item.ID == original.ID {
					return r, nil, p.CodeWarehouseChanged, nil
				}
			}
			if len(r.Items) >= r.Capacity {
				return r, nil, p.CodeWarehouseFull, nil
			}
		}
		if !warehouseItemValid(original, f) || uint64(original.Version) != q.ItemVersion || q.Quantity > original.Quantity {
			return r, nil, p.CodeWarehouseChanged, nil
		}
		if equipmentOccupied(original) || q.Scope == WarehouseShared && original.Bound {
			return r, nil, p.CodeWarehouseBlocked, nil
		}
		if q.Quantity < original.Quantity {
			d, ok := f.Item(original.ItemID)
			if !ok || !d.Stackable || d.MaxStack <= 1 || original.Quantity > d.MaxStack {
				return r, nil, p.CodeWarehouseBlocked, nil
			}
		}
		// 拆分身份由原永久操作决定，事务重试及未知回查保持同一ID。
		digest := Hash([]byte("warehouse-split:" + r.ID + ":" + domain.Hex(q.Operation.OpId)))
		transferred, remainder, valid := warehouseSplit(original, q.Quantity, digest[:32])
		if !valid {
			return r, nil, p.CodeWarehouseChanged, nil
		}
		receipt.Original = copyWarehouseItem(&original)
		receipt.Transferred = copyWarehouseItem(&transferred)
		receipt.Remainder = copyWarehouseItem(remainder)
		if q.Action == 1 {
			if remainder == nil {
				out.Items = append(out.Items[:bag], out.Items[bag+1:]...)
			} else {
				out.Items[bag] = *remainder
			}
			put = &WarehouseStoredItem{ID: transferred.ID, WarehouseID: w.ID, Slot: int64(q.WarehouseSlot), Item: transferred}
			receipt.Changes = []WarehouseChange{{Slot: q.WarehouseSlot, After: copyWarehouseItem(&transferred)}}
		} else {
			out.Items = append(out.Items, transferred)
			remove = &stored
			receipt.Changes = []WarehouseChange{{Slot: q.WarehouseSlot, Before: copyWarehouseItem(&original), After: copyWarehouseItem(remainder)}}
			if remainder != nil {
				v := stored
				v.Item = *remainder
				put = &v
			}
		}
	case 3, 4:
		var roleBalance, warehouseBalance *int64
		if q.Currency == 1 {
			roleBalance, warehouseBalance = &out.Wallet.Gold, &w.Gold
		} else {
			roleBalance, warehouseBalance = &out.Wallet.Yuanbao, &w.Yuanbao
		}
		if *roleBalance < 0 || *warehouseBalance < 0 {
			return r, nil, p.CodeWarehouseBlocked, nil
		}
		if q.Action == 3 {
			if *roleBalance < q.Amount || *warehouseBalance > math.MaxInt64-q.Amount {
				return r, nil, p.CodeWarehouseBlocked, nil
			}
			*roleBalance -= q.Amount
			*warehouseBalance += q.Amount
		} else {
			if *warehouseBalance < q.Amount || *roleBalance > math.MaxInt64-q.Amount {
				return r, nil, p.CodeWarehouseBlocked, nil
			}
			*warehouseBalance -= q.Amount
			*roleBalance += q.Amount
		}
	case 5:
		cost := f.Warehouses.PersonalExpansionYuanbaoUnits
		slots := f.Warehouses.PersonalExpansionSlots
		if cost <= 0 || out.Wallet.Yuanbao < cost || slots > uint64(math.MaxInt64-w.Capacity) {
			return r, nil, p.CodeWarehouseBlocked, nil
		}
		out.Wallet.Yuanbao -= cost
		w.Capacity += int64(slots)
		receipt.Capacity = w.Capacity
	}
	if !equipmentRoleUnique(out) {
		return r, nil, p.CodeWarehouseChanged, nil
	}
	if remove != nil {
		filter := bson.M{"_id": remove.ID, "warehouse_id": w.ID, "slot": remove.Slot, "item.version": remove.Item.Version}
		if put != nil && put.ID == remove.ID {
			res, e := s.DB.Collection("warehouse_items").ReplaceOne(tc, filter, put)
			if e != nil {
				return r, nil, 0, e
			}
			if res.MatchedCount != 1 {
				return r, nil, 0, Fail(p.CodeWarehouseChanged, "仓储原堆叠已变化")
			}
			put = nil
		} else {
			res, e := s.DB.Collection("warehouse_items").DeleteOne(tc, filter)
			if e != nil {
				return r, nil, 0, e
			}
			if res.DeletedCount != 1 {
				return r, nil, 0, Fail(p.CodeWarehouseChanged, "仓储原件已变化")
			}
		}
	}
	if put != nil {
		if _, e := s.DB.Collection("warehouse_items").InsertOne(tc, put); e != nil {
			return r, nil, 0, e
		}
	}
	w.Revision++
	if !exists {
		if _, e := s.DB.Collection("warehouses").InsertOne(tc, w); e != nil {
			if mongo.IsDuplicateKeyError(e) {
				return r, nil, 0, Fail(p.CodeWarehouseChanged, "首次仓库版本已变化")
			}
			return r, nil, 0, e
		}
	} else {
		res, e := s.DB.Collection("warehouses").ReplaceOne(tc, bson.M{"_id": w.ID, "revision": receipt.RevisionBefore}, w)
		if e != nil {
			return r, nil, 0, e
		}
		if res.MatchedCount != 1 {
			return r, nil, 0, Fail(p.CodeWarehouseChanged, "仓库版本已变化")
		}
	}
	receipt.RoleWalletAfter = out.Wallet
	receipt.GoldAfter = w.Gold
	receipt.YuanbaoAfter = w.Yuanbao
	return out, receipt, 0, nil
}
