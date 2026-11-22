// NPC 商店、收购和修理报价的持久入口。冻结短期报价及角色版本，提交时复核报价和归属，并保存永久交易回执。

package persistence

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// AssetVendor 标识通过永久资产入口提交的 NPC 买卖或修理命令。
const AssetVendor = "npc_vendor"

// vendorQuote 短期保存一次 NPC 报价的场景、NPC、角色版本、配置与确定交易计划。
type vendorQuote struct {
	RoleID       string     `bson:"_id"`           // 当前报价所属角色，同时限制每角色一条报价。
	ID           string     `bson:"quote_id"`      // 客户端提交时应带回的本次报价编号。
	Shop         string     `bson:"shop"`          // 当前玩法配置中商店目录的身份。
	Scene        string     `bson:"scene_epoch"`   // 打开 NPC 对话时的场景代次。
	NPC          uint64     `bson:"npc_id"`        // 发出报价的 NPC 标识。
	SessionEpoch int64      `bson:"session_epoch"` // 定位报价发起时的角色会话。
	Revision     int64      `bson:"revision"`      // 计算报价时读取的角色资产版本。
	Rule         uint64     `bson:"rule_version"`  // 生成报价时适用的规则版本。
	Config       string     `bson:"config_hash"`   // 生成报价时的配置摘要。
	Expires      time.Time  `bson:"expires_at"`    // 短期报价自行失效和可清理的截止时刻。
	Request      []byte     `bson:"request"`       // 原始协议请求，提交时用来重算交易计划。
	Plan         vendorPlan `bson:"plan"`          // 报价时冻结的资产变化，提交前逐项比对。
}

// VendorReceipt 保留本次已成交报价和执行后的资产计划，供相同操作编号重放。
type VendorReceipt struct {
	// QuoteID 标识当前业务使用的报价。
	QuoteID string `bson:"quote_id"`
	// NPC 记录NPC，使用数值表示。
	NPC uint64 `bson:"npc_id"`
	// Shop 记录商店，使用文本表示。
	Shop string `bson:"shop"`
	// Scene 记录场景，使用文本表示。
	Scene string `bson:"scene_epoch"`
	// Plan 记录计划，使用商人计划表示。
	Plan vendorPlan `bson:"plan"`
}

// PreviewVendor 在仍持有角色与场景时生成买卖或修理报价，覆盖该角色旧报价。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，a 保存本次调用使用的账号或角色授权，shop 记录商店，使用文本表示，q 记录商人报价请求。
func (s *Store) PreviewVendor(ctx context.Context, a Auth, shop string, q *pb.VendorQuoteRequest) (*pb.VendorQuoteResult, error) {
	if q == nil || q.SceneEpoch == 0 || q.NpcId == 0 || shop == "" {
		return nil, Fail(p.CodeVendorUnavailable, "缺少当前商人身份")
	}
	// payload 保存由序列化取得的协议负载，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	payload, err := proto.Marshal(q)
	if err != nil {
		return nil, err
	}
	// quote 记录报价，使用商人报价表示。
	var quote vendorQuote
	// f 保存当前处理所用的开发测试配置。
	f := &s.Cfg.Fixture
	// tc 记录MongoDB 事务上下文，使用请求或事务上下文表示。
	err = s.guardedTx(ctx, a, func(tc context.Context) error {
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
		// plan 保存由构建商人计划取得的计划，code 保存由构建商人计划取得的结果码。
		plan, code := buildVendorPlan(r, f, f.NPCShopByKey(shop), q)
		if code != 0 {
			return Fail(code, "NPC资产预览条件变化")
		}
		quote = vendorQuote{RoleID: r.ID, ID: domain.ID(), Shop: shop, Scene: strconv.FormatUint(q.SceneEpoch, 10), NPC: q.NpcId,
			SessionEpoch: a.SessionEpoch, Revision: r.Revision, Rule: f.RuleVersion, Config: f.Hash,
			Expires: time.Now().Add(time.Duration(f.SkillEconomy.PreviewTTLMS) * time.Millisecond), Request: payload, Plan: plan}
		_, err = s.DB.Collection("vendor_quotes").ReplaceOne(tc, bson.M{"_id": r.ID}, quote, options.Replace().SetUpsert(true))
		return err
	})
	if err != nil {
		return nil, err
	}
	return quote.result(q), nil
}

// result 转换持久报价为协议响应，修理时补充各项耐久前后值。
// quote 是调用该方法的商人报价实例，request 记录请求，使用商人报价请求表示。
func (quote vendorQuote) result(request *pb.VendorQuoteRequest) *pb.VendorQuoteResult {
	// plan 保存当前处理所用的计划。
	plan := quote.Plan
	// result 保存当前处理所用的结果。
	result := &pb.VendorQuoteResult{QuoteId: domain.Bytes(quote.ID), ExpiresAtUnixMs: uint64(quote.Expires.UnixMilli()),
		SceneEpoch: request.SceneEpoch, NpcId: request.NpcId, Action: plan.Action, OfferId: plan.OfferID,
		ItemId: plan.ItemID, InstanceId: request.InstanceId, ItemVersion: request.ItemVersion, Quantity: plan.Quantity,
		ItemQuantity: plan.ItemQuantity, Bound: plan.Bound, Gold: plan.Wallet.Gold, BoundGold: plan.Wallet.BoundGold,
		Ingot: plan.Wallet.Yuanbao, BoundIngot: plan.Wallet.BoundYuanbao}
	if plan.Repair != nil {
		result.DurabilityU = uint64(plan.Repair.Before.DurabilityU)
		result.MaxDurabilityU = uint64(plan.Repair.Before.MaxDurabilityU)
		result.OriginalDurabilityU = uint64(plan.Repair.Before.OriginalDurabilityU)
		result.RepairedDurabilityU = uint64(plan.Repair.After.DurabilityU)
		result.RepairedMaxDurabilityU = uint64(plan.Repair.After.MaxDurabilityU)
	}
	return result
}

// commitVendor 复核场景、商人、会话、角色版本及过期时间，重新计算后应用原报价。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，a 保存本次调用使用的账号或角色授权，r 记录角色，q 记录资产请求。
func (s *Store) commitVendor(ctx context.Context, a Auth, r domain.Role, q AssetRequest) (domain.Role, uint32, *VendorReceipt, error) {
	if q.Vendor == nil || !proto.Equal(q.Vendor.Operation, q.Context) || len(q.Vendor.QuoteId) != 16 {
		return r, p.CodeInvalidArgument, nil, nil
	}
	if q.VendorShop == "" {
		return r, p.CodeVendorUnavailable, nil, nil
	}
	// quote 记录报价，使用商人报价表示。
	var quote vendorQuote
	// err 接收解码过程中可能产生的错误，供后续失败分支处理。
	err := s.DB.Collection("vendor_quotes").FindOne(ctx, bson.M{"_id": r.ID, "quote_id": domain.Hex(q.Vendor.QuoteId)}).Decode(&quote)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return r, p.CodeVendorQuoteStale, nil, nil
	}
	if err != nil {
		return r, 0, nil, err
	}
	// f 保存当前处理所用的开发测试配置。
	f := &s.Cfg.Fixture
	if quote.Scene != strconv.FormatUint(q.Vendor.SceneEpoch, 10) || quote.NPC != q.Vendor.NpcId || quote.Shop != q.VendorShop ||
		quote.SessionEpoch != a.SessionEpoch || quote.Revision != r.Revision || quote.Rule != f.RuleVersion ||
		quote.Config != f.Hash || !time.Now().Before(quote.Expires) {
		return r, p.CodeVendorQuoteStale, nil, nil
	}
	// request 保存当前处理所用的请求。
	request := &pb.VendorQuoteRequest{}
	if proto.Unmarshal(quote.Request, request) != nil || request.SceneEpoch != q.Vendor.SceneEpoch || request.NpcId != q.Vendor.NpcId {
		return r, p.CodeVendorQuoteStale, nil, nil
	}
	// 用当前角色与原请求重算价格、余额和产物，计划有变化时要求重新报价。
	// plan 保存由构建商人计划取得的计划，code 保存由构建商人计划取得的结果码。
	plan, code := buildVendorPlan(r, f, f.NPCShopByKey(q.VendorShop), request)
	if code != 0 {
		return r, code, nil, nil
	}
	if !reflect.DeepEqual(plan, quote.Plan) {
		return r, p.CodeVendorQuoteStale, nil, nil
	}
	// out 保存由应用商人计划取得的输出。
	out, plan, code := applyVendorPlan(r, plan, domain.Hex(q.Context.OpId))
	if code != 0 {
		return r, code, nil, nil
	}
	return out, 0, &VendorReceipt{QuoteID: quote.ID, NPC: quote.NPC, Shop: quote.Shop, Scene: quote.Scene, Plan: plan}, nil
}
