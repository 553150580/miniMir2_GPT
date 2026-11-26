// 本文件签发和校验短期用油报价，将原确认条款、结果和审计纳入资产事务。
package persistence

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
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

const (
	AssetWeaponOilCommit     = "weapon_oil_commit" // 进入原角色经济流水的主业务键。
	weaponOilQuoteCollection = "weapon_oil_quotes" // 每角色仅保留最新的一份短期报价。
	weaponOilQuoteDuration   = 30 * time.Second    // 超过此期限，首次提交需要另取报价。
	weaponOilQuoteTTLGrace   = 5 * time.Minute     // 保留已受理报价，覆盖排队和数据库内重试。
)

// weaponOilStoredQuote 固定签发时的原会话、规则、原条款和从未公开的抽样值。
type weaponOilStoredQuote struct {
	RoleID       string        `bson:"_id"`           // 报价归属角色及每角色唯一键。
	ID           string        `bson:"quote_id"`      // 玩家本次看到并确认的报价身份。
	Scene        string        `bson:"scene_epoch"`   // 报价所在原场景，无损保存 uint64。
	UISession    string        `bson:"ui_session_id"` // 本次打开用油界面的身份。
	SessionEpoch int64         `bson:"session_epoch"` // 签发时有效的已附着角色会话代次。
	RunID        uint64        `bson:"run_server_id"` // 签发节点承载的运行服。
	RunEpoch     int64         `bson:"run_epoch"`     // 报价签发时的运行服租约代次。
	Revision     int64         `bson:"revision"`      // 确认页将回传的原角色修订。
	Rule         uint64        `bson:"rule_version"`  // 选择当前档位的规则版本。
	Config       string        `bson:"config_hash"`   // 同版本参数变化也会失效的配置摘要。
	Issued       time.Time     `bson:"issued_at"`     // 原报价建立时间。
	Expires      time.Time     `bson:"expires_at"`    // 客户端最后确认边界，不含后台清理宽限。
	PurgeAt      time.Time     `bson:"purge_at"`      // 清理无用报价的时间。
	Request      []byte        `bson:"request"`       // 原客户端引用、场景、授权及界面会话。
	Plan         weaponOilPlan `bson:"plan"`          // 实际选择的两个原实例和消费规则。
	Roll         uint32        `bson:"roll"`          // 固定该报价的私有概率抽样。
}

// WeaponOilResultReceipt 保存一次已经实际耗油的结果，失败和回退同样为已受理事实。
type WeaponOilResultReceipt struct {
	QuoteID      string            `bson:"quote_id"`                // 原报价编号。
	Scene        string            `bson:"scene_epoch"`             // 首次操作的场景代次。
	UISession    string            `bson:"ui_session_id"`           // 玩家授权原绑定投入的界面会话。
	Rule         uint64            `bson:"rule_version"`            // 原操作的规则版本。
	Config       string            `bson:"config_hash"`             // 原操作采用的不可变配置信息。
	Plan         weaponOilPlan     `bson:"plan"`                    // 原培养对象、油堆、绑定及完整确认条款。
	Weapon       domain.Item       `bson:"weapon"`                  // 武器用油后的原件，翅膀培养时为空值。
	Wing         *domain.WingState `bson:"wing,omitempty"`          // 实际培养后的同一专属翅膀，保留两个阶段进度。
	OilRemainder *domain.Item      `bson:"oil_remainder,omitempty"` // 仅扣一瓶后的同一油堆，原堆耗完时为空。
	OilConsumed  int64             `bson:"oil_consumed"`            // 原物品实际扣除的数量，固定为一瓶。
	Success      bool              `bson:"success"`                 // 本瓶是否将当前幸运提升一级。
	PityUsed     uint32            `bson:"pity_used"`               // 已计入当前瓶后，原所属阶段的累计瓶数。
}

// Proto 只公开实际的原对象、原油堆及确认的幸运变化，隐藏抽样和内部围栏。
func (r *WeaponOilResultReceipt) Proto() *pb.WeaponOilReceipt {
	if r == nil {
		return nil
	}
	result := &pb.WeaponOilReceipt{QuoteId: domain.Bytes(r.QuoteID), Wing: r.Wing.Proto(), Oil: r.Plan.Oil.Proto(),
		Equipped: r.Plan.Equipped, BeforeLuck: r.Plan.BeforeLuck, Success: r.Success, Guaranteed: r.Plan.Guaranteed,
		PityUsed: r.PityUsed, PityAfter: r.Plan.PityAfter}
	if r.Wing == nil {
		result.Weapon = r.Weapon.Proto()
	}
	return result
}

// PreviewWeaponOil 经过当前角色的运行服和账号围栏后，保存一份可以重核的短期用油报价。
func (s *Store) PreviewWeaponOil(ctx context.Context, a Auth, q *pb.WeaponOilQuoteRequest) (*pb.WeaponOilQuote, error) {
	fixture := &s.Cfg.Fixture
	if fixture.LuckOil == nil {
		return nil, Fail(p.CodeWeaponOilUnavailable, "本规则未开放武器祝福油")
	}
	if !validWeaponOilQuoteRequest(q) {
		return nil, Fail(p.CodeInvalidArgument, "用油报价缺少唯一原培养对象、原场景或界面会话")
	}
	if q.Wing != nil && !wingOilEnabled(fixture) {
		return nil, Fail(p.CodeWeaponOilUnavailable, "本规则未开放翅膀用油")
	}
	payload, err := proto.Marshal(q)
	if err != nil {
		return nil, err
	}
	roll, err := rand.Int(rand.Reader, big.NewInt(weaponOilPPM))
	if err != nil {
		return nil, err
	}
	issued := time.Now().UTC().Truncate(time.Millisecond)
	quote := weaponOilStoredQuote{RoleID: a.RoleID, ID: domain.ID(), Scene: strconv.FormatUint(q.SceneEpoch, 10),
		UISession: domain.Hex(q.UiSessionId), SessionEpoch: a.SessionEpoch, RunID: s.Cfg.RunID, RunEpoch: s.Epoch,
		Rule: fixture.RuleVersion, Config: fixture.Hash, Issued: issued, Expires: issued.Add(weaponOilQuoteDuration),
		PurgeAt: issued.Add(weaponOilQuoteDuration + weaponOilQuoteTTLGrace), Request: payload, Roll: uint32(roll.Uint64())}
	err = s.guardedTx(ctx, a, func(tc context.Context) error {
		if err := s.fence(tc, a); err != nil {
			return err
		}
		role, err := s.Role(tc, a.RoleID)
		if err != nil {
			return err
		}
		if err = s.checkRole(role, a); err != nil {
			return err
		}
		plan, code := buildWeaponOilPlan(role, fixture, q)
		if code != 0 {
			return Fail(code, "不满足原培养对象或实际祝福油的投入条件")
		}
		quote.Revision, quote.Plan = role.Revision, plan
		_, err = s.DB.Collection(weaponOilQuoteCollection).ReplaceOne(tc, bson.M{"_id": role.ID}, quote, options.Replace().SetUpsert(true))
		return err
	})
	if err != nil {
		return nil, err
	}
	return quote.public(q.SceneEpoch), nil
}

// public 使用冻结条款明确显示原油堆、当前和可能的幸运、绑定结果以及保底。
func (q weaponOilStoredQuote) public(scene uint64) *pb.WeaponOilQuote {
	plan := q.Plan
	result := &pb.WeaponOilQuote{QuoteId: domain.Bytes(q.ID), SceneEpoch: scene, UiSessionId: domain.Bytes(q.UISession), Wing: plan.Wing.Proto(),
		Oil: plan.Oil.Proto(), Equipped: plan.Equipped, AllowBound: plan.AllowBound, CurrentLuck: plan.BeforeLuck,
		SuccessLuck: plan.SuccessLuck, FailureLuck: plan.FailureLuck, SuccessPpm: plan.SuccessPPM, PityUsed: plan.PityUsed,
		PityAfter: plan.PityAfter, Guaranteed: plan.Guaranteed, OutputBound: plan.OutputBound,
		ExpiresAtUnixMs: uint64(q.Expires.UnixMilli()), RoleRevision: uint64(q.Revision)}
	if plan.Wing == nil {
		result.Weapon = plan.Weapon.Proto()
	}
	return result
}

// commitWeaponOil 只处理首次提交。已有回执由资产入口优先读取，不能再次扣原油。
func (s *Store) commitWeaponOil(ctx context.Context, a Auth, role domain.Role, q AssetRequest) (domain.Role, uint32, *WeaponOilResultReceipt, error) {
	input := q.WeaponOilCommit
	if input == nil || !proto.Equal(input.Operation, q.Context) || len(input.QuoteId) != 16 || len(input.UiSessionId) != 16 ||
		!validOilTargetRef(input.Weapon, input.Wing) || len(input.ProtoReflect().GetUnknown()) != 0 {
		return role, p.CodeInvalidArgument, nil, nil
	}
	if q.WeaponOilGate != 0 {
		return role, q.WeaponOilGate, nil, nil
	}
	fixture := &s.Cfg.Fixture
	if fixture.LuckOil == nil || input.Wing != nil && !wingOilEnabled(fixture) {
		return role, p.CodeWeaponOilUnavailable, nil, nil
	}
	filter := bson.M{"_id": role.ID, "quote_id": domain.Hex(input.QuoteId)}
	collection := s.DB.Collection(weaponOilQuoteCollection)
	var quote weaponOilStoredQuote
	err := collection.FindOne(ctx, filter).Decode(&quote)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return role, p.CodeWeaponOilQuoteChanged, nil, nil
	}
	if err != nil {
		return role, 0, nil, err
	}
	if q.SceneEpoch == 0 || quote.Scene != strconv.FormatUint(q.SceneEpoch, 10) || quote.UISession != domain.Hex(input.UiSessionId) ||
		quote.SessionEpoch != a.SessionEpoch || quote.RunID != s.Cfg.RunID || quote.RunEpoch != s.Epoch ||
		q.WeaponOilAcceptedAt.Before(quote.Issued) || !q.WeaponOilAcceptedAt.Before(quote.Expires) || quote.Roll >= weaponOilPPM ||
		quote.Revision < 1 || quote.Revision > role.Revision || q.Context.ExpectedRevision != uint64(quote.Revision) ||
		quote.Rule != fixture.RuleVersion || quote.Config != fixture.Hash {
		return role, p.CodeWeaponOilQuoteChanged, nil, nil
	}
	var request pb.WeaponOilQuoteRequest
	if proto.Unmarshal(quote.Request, &request) != nil || request.SceneEpoch != q.SceneEpoch ||
		input.AllowBound != request.AllowBound || domain.Hex(request.UiSessionId) != quote.UISession ||
		!proto.Equal(input.Weapon, request.Weapon) || !proto.Equal(input.Wing, request.Wing) {
		return role, p.CodeWeaponOilQuoteChanged, nil, nil
	}
	plan, code := buildWeaponOilPlan(role, fixture, &request)
	if code != 0 || !reflect.DeepEqual(plan, quote.Plan) {
		return role, p.CodeWeaponOilQuoteChanged, nil, nil
	}
	deleted, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		return role, 0, nil, err
	}
	if deleted.DeletedCount != 1 {
		return role, p.CodeWeaponOilQuoteChanged, nil, nil
	}
	out, result := applyWeaponOilPlan(role, quote)
	if result.Wing != nil {
		if !validRoleWing(result.Wing, role.ID, fixture.Wings) {
			return role, 0, nil, Fail(p.CodeOperationConflict, "翅膀用油结果的身份或状态无效")
		}
		if err = s.writeRoleWing(ctx, plan.Wing, *result.Wing); err != nil {
			return role, 0, nil, err
		}
	}
	return out, 0, &result, nil
}
