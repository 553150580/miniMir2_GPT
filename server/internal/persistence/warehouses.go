// 个人与共享仓库的分页查询和资产变更。分表存储元数据与物品，在事务内处理容量、拆分、版本与特权校验。

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
	WarehousePersonal uint32 = 1 // 归角色所有的个人仓库。
	WarehouseShared   uint32 = 2 // 同账号、同永久入口角色共享的仓库。
)

// 元数据和物品分别存储，个人容量增长不扩大角色聚合或单次网络报文。
type Warehouse struct {
	ID        string `bson:"_id"`               // 根据个人或共享归属推导的仓库键。
	Scope     uint32 `bson:"scope"`             // 个人与共享的协议分类。
	RoleID    string `bson:"role_id,omitempty"` // 个人仓库所属角色，共享仓库为空。
	AccountID string `bson:"account_id"`        // 两类仓库都需要匹配的账号归属。
	Origin    uint64 `bson:"origin_server_id"`  // 永久入口服标识，用于隔开不同区服的共享资产。
	Revision  int64  `bson:"revision"`          // 并发资产动作比较的仓库版本。
	Capacity  int64  `bson:"capacity"`          // 当前可使用的格数。
	Gold      int64  `bson:"gold"`              // 共享仓库中的可转移金币。
	Yuanbao   int64  `bson:"yuanbao_units"`     // 共享仓库中的可转移元宝整数单位。
}

// WarehouseStoredItem 在独立文档存放仓库格及其物品实例，以便局部更新和分页。
type WarehouseStoredItem struct {
	// ID 区分当前步骤指向的目标对象。
	ID string `bson:"_id"`
	// WarehouseID 标识当前业务使用的仓库。
	WarehouseID string `bson:"warehouse_id"`
	// Slot 记录槽位，使用数值表示。
	Slot int64 `bson:"slot"`
	// Item 记录道具。
	Item domain.Item `bson:"item"`
}

// WarehouseChange 记录某个仓库格在一次资产动作前后的内容。
type WarehouseChange struct {
	// Slot 记录槽位，使用数值表示。
	Slot uint64 `bson:"slot"`
	// Before 记录操作前的状态，使用道具表示。
	Before *domain.Item `bson:"before,omitempty"`
	// After 记录操作后的状态，使用道具表示。
	After *domain.Item `bson:"after,omitempty"`
}

// WarehouseReceipt 冻结仓储格、货币和实例的前后快照，供按原操作身份复查。
type WarehouseReceipt struct {
	// Scope 记录作用域，使用数值表示。
	Scope uint32 `bson:"scope"`
	// Action 记录动作，使用数值表示。
	Action uint32 `bson:"action"`
	// WarehouseID 标识当前业务使用的仓库。
	WarehouseID string `bson:"warehouse_id"`
	// RevisionBefore 记录修订号操作前的状态，使用数值表示。
	RevisionBefore int64 `bson:"revision_before"`
	// RevisionAfter 记录修订号操作后的状态，使用数值表示。
	RevisionAfter int64 `bson:"revision_after"`
	// Capacity 记录容量，使用数值表示。
	Capacity int64 `bson:"capacity"`
	// Changes 记录“changes”，使用仓库变更列表表示。
	Changes []WarehouseChange `bson:"changes"`
	// Original 记录原始，使用道具表示。
	Original *domain.Item `bson:"original,omitempty"`
	// Transferred 记录“transferred”，使用道具表示。
	Transferred *domain.Item `bson:"transferred,omitempty"`
	// Remainder 记录“remainder”，使用道具表示。
	Remainder *domain.Item `bson:"remainder,omitempty"`
	// RoleWalletBefore 记录角色钱包操作前的状态。
	RoleWalletBefore domain.Wallet `bson:"role_wallet_before"`
	// RoleWalletAfter 记录角色钱包操作后的状态。
	RoleWalletAfter domain.Wallet `bson:"role_wallet_after"`
	// GoldBefore 记录金币操作前的状态，使用数值表示。
	GoldBefore int64 `bson:"gold_before"`
	// GoldAfter 记录金币操作后的状态，使用数值表示。
	GoldAfter int64 `bson:"gold_after"`
	// YuanbaoBefore 记录元宝操作前的状态，使用数值表示。
	YuanbaoBefore int64 `bson:"yuanbao_before"`
	// YuanbaoAfter 记录元宝操作后的状态，使用数值表示。
	YuanbaoAfter int64 `bson:"yuanbao_after"`
	// RuleVersion 记录规则版本，使用数值表示。
	RuleVersion uint64 `bson:"rule_version"`
	// ConfigHash 记录配置摘要，使用文本表示。
	ConfigHash string `bson:"config_hash"`
}

// copyWarehouseItem 复制可空的物品快照，避免回执和后续库存引用同一值。
// i 记录当前位置的下标，使用道具表示。
func copyWarehouseItem(i *domain.Item) *domain.Item {
	if i == nil {
		return nil
	}
	// v 保存当前处理所用的i。
	v := *i
	return &v
}

// cloneWarehouseReceipt 深复制回执中的可变成员，保留首次确认时的对应仓库状态。
// in 记录输入，使用仓库回执表示。
func cloneWarehouseReceipt(in *WarehouseReceipt) *WarehouseReceipt {
	if in == nil {
		return nil
	}
	// out 保存当前处理所用的输出。
	out := *in
	out.Original = copyWarehouseItem(in.Original)
	out.Transferred = copyWarehouseItem(in.Transferred)
	out.Remainder = copyWarehouseItem(in.Remainder)
	out.Changes = slices.Clone(in.Changes)
	// i 标记遍历“changes”时访问的下标。
	for i := range out.Changes {
		out.Changes[i].Before = copyWarehouseItem(out.Changes[i].Before)
		out.Changes[i].After = copyWarehouseItem(out.Changes[i].After)
	}
	return &out
}

// Proto 从首次操作回执生成客户端仓库变化，并带回角色与仓库版本。
// r 是调用该方法的仓库回执实例，role 记录角色，op 记录操作。
func (r *WarehouseReceipt) Proto(role domain.Role, op Operation) *pb.WarehouseActionResult {
	if r == nil {
		return nil
	}
	// id 区分当前步骤指向的目标对象。
	id := strings.TrimPrefix(op.ID, "asset:"+role.ID+":")
	if len(domain.Bytes(id)) != 16 {
		return nil
	}
	// out 保存当前处理所用的输出。
	out := &pb.WarehouseActionResult{Role: role.Summary().Key, RoleRevision: uint64(op.Revision), OpId: domain.Bytes(id), Scope: r.Scope, Action: r.Action, WarehouseRevision: uint64(r.RevisionAfter), Capacity: uint64(r.Capacity), Gold: r.GoldAfter, YuanbaoUnits: r.YuanbaoAfter}
	// c 接收遍历“changes”时当前的元素。
	for _, c := range r.Changes {
		// v 保存当前处理所用的仓库道具变更。
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

// warehouseIdentity 按角色或账号与永久入口服解析独立仓库键及初始容量。
// r 记录角色，scope 记录作用域，使用数值表示，cfg 记录服务配置，使用仓库规则集合表示。
func warehouseIdentity(r domain.Role, scope uint32, cfg *devconfig.WarehouseRules) (Warehouse, error) {
	if cfg == nil || r.Origin == 0 || len(domain.Bytes(r.ID)) != 16 || len(domain.Bytes(r.AccountID)) != 16 {
		return Warehouse{}, Fail(p.CodeWarehouseBlocked, "仓库归属或配置不可用")
	}
	// w 组装仓库所需的数据。
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

// warehouse 读取已建仓库并验证归属和容量，未建时返回计算得到的初始元数据。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，r 记录角色，scope 记录作用域，使用数值表示。
func (s *Store) warehouse(ctx context.Context, r domain.Role, scope uint32) (Warehouse, bool, error) {
	// w 接收仓库身份供后续步骤使用，e 接收仓库身份供后续步骤使用。
	w, e := warehouseIdentity(r, scope, s.Cfg.Fixture.Warehouses)
	if e != nil {
		return w, false, e
	}
	// expected 保存当前处理所用的预期结果。
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
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，a 保存本次调用使用的账号或角色授权。
func (s *Store) warehouseReadFence(ctx context.Context, a Auth) error {
	// err 保存当前调用返回的错误，供后续失败分支处理。
	if err := s.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": s.Cfg.RunID, "owner_instance": s.Instance, "run_epoch": s.Epoch, "state": "active", "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}).Err(); err != nil {
		return authorityReadError(err, p.CodeRouteChanged, "运行所有权已失效")
	}
	// err 保存当前调用返回的错误，供后续失败分支处理。
	if err := s.DB.Collection("node_leases").FindOne(ctx, bson.M{"_id": s.Cfg.NodeID, "owner_instance": s.Instance, "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}).Err(); err != nil {
		return authorityReadError(err, p.CodeRouteChanged, "节点许可已失效")
	}
	// err 保存当前调用返回的错误，供后续失败分支处理。
	if err := s.DB.Collection("accounts").FindOne(ctx, bson.M{"_id": a.AccountID, "active": true, "auth_version": a.AuthVersion}).Err(); err != nil {
		return authorityReadError(err, p.CodeUnauthenticated, "账号授权失效")
	}
	return nil
}

// QueryWarehouse 使用一致读快照和只读围栏返回指定连续格范围，并验证共享仓库特权。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，a 保存本次调用使用的账号或角色授权，q 记录仓库查询。
func (s *Store) QueryWarehouse(ctx context.Context, a Auth, q *pb.WarehouseQuery) (*pb.WarehousePage, error) {
	// cfg 保存仓库集合，用于当前步骤中的服务配置。
	cfg := s.Cfg.Fixture.Warehouses
	if cfg == nil || q == nil || q.Limit == 0 || q.Limit > cfg.QueryPageLimit || q.StartSlot > math.MaxInt64 || len(q.ProtoReflect().GetUnknown()) != 0 {
		return nil, Fail(p.CodeInvalidArgument, "仓储分页参数非法")
	}
	// out 记录输出，使用仓库页面或分页表示。
	var out *pb.WarehousePage
	// err 接收事务过程中可能产生的错误，供后续失败分支处理，tc 记录MongoDB 事务上下文，使用请求或事务上下文表示。
	err := s.tx(ctx, func(tc context.Context) error {
		out = nil
		// err 接收仓库读取权限围栏过程中可能产生的错误，供后续失败分支处理。
		if err := s.warehouseReadFence(tc, a); err != nil {
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
		if q.Scope == WarehouseShared && !r.PrivilegeUntil.After(time.Now()) {
			return Fail(p.CodeWarehousePrivilegeRequired, "当前角色特权无效")
		}
		// w 接收仓库供后续步骤使用，exists 保存由仓库取得的存在。
		w, exists, err := s.warehouse(tc, r, q.Scope)
		if err != nil {
			return err
		}
		if q.StartSlot >= uint64(w.Capacity) {
			return Fail(p.CodeInvalidArgument, "分页起点超过仓库容量")
		}
		// end 保存根据起点或开始时间槽位与无符号整数“64”计算的结束位置或时间。
		end := q.StartSlot + uint64(q.Limit)
		if end > uint64(w.Capacity) {
			end = uint64(w.Capacity)
		}
		// page 保存当前处理所用的页面或分页。
		page := &pb.WarehousePage{Role: r.Summary().Key, Scope: q.Scope, Revision: uint64(w.Revision), Capacity: uint64(w.Capacity), StartSlot: q.StartSlot, Limit: q.Limit, Gold: w.Gold, YuanbaoUnits: w.Yuanbao}
		// cursor 保存由查找取得的游标。
		cursor, err := s.DB.Collection("warehouse_items").Find(tc, bson.M{"warehouse_id": w.ID, "slot": bson.M{"$gte": int64(q.StartSlot), "$lt": int64(end)}}, options.Find().SetSort(bson.D{{Key: "slot", Value: 1}}).SetLimit(int64(q.Limit)))
		if err != nil {
			return err
		}
		defer cursor.Close(tc)
		for cursor.Next(tc) {
			// x 记录仓库已持久保存道具。
			var x WarehouseStoredItem
			if err = cursor.Decode(&x); err != nil {
				return err
			}
			if !exists || x.ID != x.Item.ID || !warehouseItemValid(x.Item, s.Cfg.Fixture) || q.Scope == WarehouseShared && x.Item.Bound {
				return Fail(p.CodeWarehouseBlocked, "仓储物品状态无效")
			}
			// item 保存由“initialize”道具耐久取得的道具。
			item := InitializeItemDurability(x.Item, &s.Cfg.Fixture)
			page.Items = append(page.Items, &pb.WarehouseItem{Slot: uint64(x.Slot), Item: item.Proto()})
		}
		if err = cursor.Err(); err != nil {
			return err
		}
		out = page
		return nil
	})
	return out, err
}

// warehouseItemValid 校验存取时必要的物品身份、计数和版本，兼容完整迁移旧物品。
// i 记录当前位置的下标，使用道具表示，f 记录开发测试配置。
func warehouseItemValid(i domain.Item, f devconfig.Fixture) bool {
	if i.ItemID == 0 || len(domain.Bytes(i.ID)) != 16 || domain.Hex(domain.Bytes(i.ID)) != i.ID || i.Version < 1 || i.Quantity < 1 || i.Quality > 4 {
		return false
	}
	// 完整迁移不重解释旧目录；部分拆分另核当前明确的堆叠能力。
	return true
}

// warehouseSplit 全量移动时保留原身份，部分移动时生成带独立身份的拆出堆并保留余量。
// original 记录原始，使用道具表示，quantity 记录数量，使用数值表示，splitID 标识当前业务使用的拆分。
func warehouseSplit(original domain.Item, quantity int64, splitID string) (domain.Item, *domain.Item, bool) {
	if quantity <= 0 || quantity > original.Quantity || original.Version <= 0 || original.Version == math.MaxInt64 {
		return domain.Item{}, nil, false
	}
	// transferred 保存当前处理所用的“transferred”。
	transferred := original
	transferred.Quantity = quantity
	transferred.Version++
	if quantity == original.Quantity {
		return transferred, nil, true
	}
	if len(domain.Bytes(splitID)) != 16 || domain.Hex(domain.Bytes(splitID)) != splitID || splitID == original.ID {
		return domain.Item{}, nil, false
	}
	// remainder 保存当前处理所用的“remainder”。
	remainder := original
	remainder.Quantity -= quantity
	remainder.Version++
	transferred.ID = splitID
	transferred.Version = 1
	return transferred, &remainder, true
}

// validWarehouseAction 按存取物品、货币或扩容分别约束请求可携带的字段。
// q 记录仓库动作。
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
// s 是调用该方法的持久层存储实例，tc 记录MongoDB 事务上下文，使用请求或事务上下文表示，r 记录角色，q 记录仓库动作。
func (s *Store) applyWarehouse(tc context.Context, r domain.Role, q *pb.WarehouseAction) (domain.Role, *WarehouseReceipt, uint32, error) {
	// f 保存当前处理所用的开发测试配置。
	f := s.Cfg.Fixture
	if f.Warehouses == nil || !validWarehouseAction(q) || q.Operation == nil || len(q.Operation.OpId) != 16 || !equipmentRoleUnique(r) {
		return r, nil, p.CodeInvalidArgument, nil
	}
	if q.Scope == WarehouseShared && !r.PrivilegeUntil.After(time.Now()) {
		return r, nil, p.CodeWarehousePrivilegeRequired, nil
	}
	// w 接收仓库供后续步骤使用，exists 保存由仓库取得的存在，err 接收仓库过程中可能产生的错误，供后续失败分支处理。
	w, exists, err := s.warehouse(tc, r, q.Scope)
	if err != nil {
		return r, nil, 0, err
	}
	if uint64(w.Revision) != q.WarehouseRevision || w.Revision == math.MaxInt64 {
		return r, nil, p.CodeWarehouseChanged, nil
	}
	// receipt 保存当前处理所用的回执。
	receipt := &WarehouseReceipt{Scope: q.Scope, Action: q.Action, WarehouseID: w.ID, RevisionBefore: w.Revision, RevisionAfter: w.Revision + 1, Capacity: w.Capacity, RoleWalletBefore: r.Wallet, GoldBefore: w.Gold, YuanbaoBefore: w.Yuanbao, RuleVersion: f.RuleVersion, ConfigHash: f.Hash}
	// out 保存当前处理所用的输出。
	out := r
	out.Items = slices.Clone(r.Items)
	// put 记录“put”，使用仓库已持久保存道具表示。
	var put *WarehouseStoredItem
	// remove 记录移除，使用仓库已持久保存道具表示。
	var remove *WarehouseStoredItem
	// 先在内存计算背包、货币与仓库格变化，所有条件满足后才写入数据库。
	switch q.Action {
	case 1, 2:
		if q.WarehouseSlot >= uint64(w.Capacity) {
			return r, nil, p.CodeInvalidArgument, nil
		}
		// stored 记录已持久保存，使用仓库已持久保存道具表示。
		var stored WarehouseStoredItem
		err = s.DB.Collection("warehouse_items").FindOne(tc, bson.M{"warehouse_id": w.ID, "slot": int64(q.WarehouseSlot)}).Decode(&stored)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return r, nil, 0, err
		}
		// bag 保存当前处理所用的背包。
		bag := -1
		// n 定位道具集合中正在处理的成员，i 接收遍历道具集合时当前的元素。
		for n, i := range r.Items {
			if i.ID == domain.Hex(q.ItemInstanceId) {
				bag = n
				break
			}
		}
		// original 记录原始，使用道具表示。
		var original domain.Item
		if q.Action == 1 {
			if err == nil {
				return r, nil, p.CodeWarehouseFull, nil
			}
			if bag < 0 {
				return r, nil, p.CodeWarehouseChanged, nil
			}
			original = r.Items[bag]
			// n 接收数量文档供后续步骤使用，e 接收数量文档供后续步骤使用。
			if n, e := s.DB.Collection("warehouse_items").CountDocuments(tc, bson.M{"_id": original.ID}, options.Count().SetLimit(1)); e != nil {
				return r, nil, 0, e
			} else if n != 0 {
				return r, nil, p.CodeWarehouseChanged, nil
			}
		} else {
			if errors.Is(err, mongo.ErrNoDocuments) || stored.ID != stored.Item.ID || stored.ID != domain.Hex(q.ItemInstanceId) || bag >= 0 {
				return r, nil, p.CodeWarehouseChanged, nil
			}
			original = InitializeItemDurability(stored.Item, &f)
			// eq 接收遍历已穿戴装备时当前的元素。
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
			// d 接收道具供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
			d, ok := f.Item(original.ItemID)
			if !ok || !d.Stackable || d.MaxStack <= 1 || original.Quantity > d.MaxStack {
				return r, nil, p.CodeWarehouseBlocked, nil
			}
		}
		// 拆分身份由原永久操作决定，事务重试及未知回查保持同一ID。
		// digest 接收摘要供后续步骤使用。
		digest := Hash([]byte("warehouse-split:" + r.ID + ":" + domain.Hex(q.Operation.OpId)))
		// transferred 保存由仓库拆分取得的“transferred”，remainder 保存由仓库拆分取得的“remainder”，valid 保存由仓库拆分取得的有效。
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
				// v 保存当前处理所用的已持久保存。
				v := stored
				v.Item = *remainder
				put = &v
			}
		}
	case 3, 4:
		// roleBalance 记录角色余额，使用数值表示，warehouseBalance 记录仓库余额，使用数值表示。
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
		// cost 保存个人“expansion”元宝单位集合，用于当前步骤中的消耗。
		cost := f.Warehouses.PersonalExpansionYuanbaoUnits
		// slots 保存个人“expansion”槽位集合，用于当前步骤中的槽位集合。
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
	// 修改或删除仓库格时还需匹配原实例版本，避免覆盖另一笔已经提交的移动。
	if remove != nil {
		// filter 初始化用作筛选条件的MongoDB BSON 文档。
		filter := bson.M{"_id": remove.ID, "warehouse_id": w.ID, "slot": remove.Slot, "item.version": remove.Item.Version}
		if put != nil && put.ID == remove.ID {
			// res 保存由替换“one”取得的结果，e 接收替换“one”供后续步骤使用。
			res, e := s.DB.Collection("warehouse_items").ReplaceOne(tc, filter, put)
			if e != nil {
				return r, nil, 0, e
			}
			if res.MatchedCount != 1 {
				return r, nil, 0, Fail(p.CodeWarehouseChanged, "仓储原堆叠已变化")
			}
			put = nil
		} else {
			// res 保存由“delete”“one”取得的结果，e 接收“delete”“one”供后续步骤使用。
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
		// e 接收插入“one”供后续步骤使用。
		if _, e := s.DB.Collection("warehouse_items").InsertOne(tc, put); e != nil {
			return r, nil, 0, e
		}
	}
	// 仓库元数据变更、格位变更与调用层的角色写入共用当前事务。
	w.Revision++
	if !exists {
		// e 接收插入“one”供后续步骤使用。
		if _, e := s.DB.Collection("warehouses").InsertOne(tc, w); e != nil {
			if mongo.IsDuplicateKeyError(e) {
				return r, nil, 0, Fail(p.CodeWarehouseChanged, "首次仓库版本已变化")
			}
			return r, nil, 0, e
		}
	} else {
		// res 保存由替换“one”取得的结果，e 接收替换“one”供后续步骤使用。
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
