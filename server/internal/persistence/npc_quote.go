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

const AssetVendor = "npc_vendor"

type vendorQuote struct {
	RoleID       string     `bson:"_id"`
	ID           string     `bson:"quote_id"`
	Shop         string     `bson:"shop"`
	Scene        string     `bson:"scene_epoch"`
	NPC          uint64     `bson:"npc_id"`
	SessionEpoch int64      `bson:"session_epoch"`
	Revision     int64      `bson:"revision"`
	Rule         uint64     `bson:"rule_version"`
	Config       string     `bson:"config_hash"`
	Expires      time.Time  `bson:"expires_at"`
	Request      []byte     `bson:"request"`
	Plan         vendorPlan `bson:"plan"`
}

type VendorReceipt struct {
	QuoteID string     `bson:"quote_id"`
	NPC     uint64     `bson:"npc_id"`
	Shop    string     `bson:"shop"`
	Scene   string     `bson:"scene_epoch"`
	Plan    vendorPlan `bson:"plan"`
}

func (s *Store) PreviewVendor(ctx context.Context, a Auth, shop string, q *pb.VendorQuoteRequest) (*pb.VendorQuoteResult, error) {
	if q == nil || q.SceneEpoch == 0 || q.NpcId == 0 || shop == "" {
		return nil, Fail(p.CodeVendorUnavailable, "缺少当前商人身份")
	}
	payload, err := proto.Marshal(q)
	if err != nil {
		return nil, err
	}
	var quote vendorQuote
	f := &s.Cfg.Fixture
	err = s.tx(ctx, func(tc context.Context) error {
		if err := s.fence(tc, a); err != nil {
			return err
		}
		r, err := s.Role(tc, a.RoleID)
		if err != nil {
			return err
		}
		if err = s.checkRole(r, a); err != nil {
			return err
		}
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

func (quote vendorQuote) result(request *pb.VendorQuoteRequest) *pb.VendorQuoteResult {
	plan := quote.Plan
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

func (s *Store) commitVendor(ctx context.Context, a Auth, r domain.Role, q AssetRequest) (domain.Role, uint32, *VendorReceipt, error) {
	if q.Vendor == nil || !proto.Equal(q.Vendor.Operation, q.Context) || len(q.Vendor.QuoteId) != 16 {
		return r, p.CodeInvalidArgument, nil, nil
	}
	if q.VendorShop == "" {
		return r, p.CodeVendorUnavailable, nil, nil
	}
	var quote vendorQuote
	err := s.DB.Collection("vendor_quotes").FindOne(ctx, bson.M{"_id": r.ID, "quote_id": domain.Hex(q.Vendor.QuoteId)}).Decode(&quote)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return r, p.CodeVendorQuoteStale, nil, nil
	}
	if err != nil {
		return r, 0, nil, err
	}
	f := &s.Cfg.Fixture
	if quote.Scene != strconv.FormatUint(q.Vendor.SceneEpoch, 10) || quote.NPC != q.Vendor.NpcId || quote.Shop != q.VendorShop ||
		quote.SessionEpoch != a.SessionEpoch || quote.Revision != r.Revision || quote.Rule != f.RuleVersion ||
		quote.Config != f.Hash || !time.Now().Before(quote.Expires) {
		return r, p.CodeVendorQuoteStale, nil, nil
	}
	request := &pb.VendorQuoteRequest{}
	if proto.Unmarshal(quote.Request, request) != nil || request.SceneEpoch != q.Vendor.SceneEpoch || request.NpcId != q.Vendor.NpcId {
		return r, p.CodeVendorQuoteStale, nil, nil
	}
	plan, code := buildVendorPlan(r, f, f.NPCShopByKey(q.VendorShop), request)
	if code != 0 {
		return r, code, nil, nil
	}
	if !reflect.DeepEqual(plan, quote.Plan) {
		return r, p.CodeVendorQuoteStale, nil, nil
	}
	out, plan, code := applyVendorPlan(r, plan, domain.Hex(q.Context.OpId))
	if code != 0 {
		return r, code, nil, nil
	}
	return out, 0, &VendorReceipt{QuoteID: quote.ID, NPC: quote.NPC, Shop: quote.Shop, Scene: quote.Scene, Plan: plan}, nil
}
