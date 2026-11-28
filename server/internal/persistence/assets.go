// 资产命令和收益请求的持久化入口。以操作编号、内容摘要、角色版本和经济命令水位处理业务拒绝与幂等重放，并在同一事务应用资产变更。

package persistence

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"strconv"
	"time"
)

// AssetRequest 是游戏逻辑提交给持久层的已路由资产命令。
type AssetRequest struct {
	Kind             string                  // 业务分派键，决定应用哪类资产命令。
	Fingerprint      string                  // 稳定请求摘要，用于拒绝相同操作编号的不同内容。
	Context          *pb.OperationContext    // 客户端操作编号、命令序号与预期版本。
	ItemID           string                  // 单物品操作选中的库存实例编号。
	ItemIDs          []string                // 批量操作选择的库存实例编号列表。
	Slot             uint32                  // 换装或卸装指定的装备槽。
	Benefit          uint32                  // 当前领取托管时长所请求的权益类型。
	HostingClaimAt   time.Time               `json:"-"` // 服务端受理原领取的时刻，由同一任务重试复用。
	HostingPurchase  *HostingPurchaseTerms   // 玩家在确认页看到并提交的原补时交易事实。
	CardPurchase     *RoleCardQuote          // 当前场景签发的原双卡报价，客户端只发送报价编号。
	CardDay          string                  // 特权每日领取原本看到的上海运营日。
	CardAcceptedAt   time.Time               `json:"-"` // 服务端实际受理领取的时刻，重试时会保留。
	ValidRevive      bool                    // 房间判断本次复活选择是否有对应的有效现场。
	ReviveDeathID    string                  // 请求复活的原死亡身份。
	ItemDefinitionID uint32                  // 需要和物品实例一致的配置物品编号。
	SceneEpoch       uint64                  // 药效或可信补时报价所属的场景代次。
	PotionGateCode   uint32                  // 房间对用药条件的预先判定。
	SkillID          uint32                  // 学习、配置耗材或实际施法对应的技能。
	PreviewID        string                  // 提交时必须复核的短期技能资产计划标识。
	ItemVersion      uint64                  // 客户端或房间选定物品时的实例版本。
	CasterLife       uint64                  // 需要应用耗材效果的施法者生命代次。
	CastGateCode     uint32                  // 房间对技能与耗材使用条件的预先判定。
	Unequip          bool                    // 装备命令是否表示卸下槽位物品。
	SourceSeq        int64                   // 服务端自动资产命令的独立来源水位。
	AutoRecycle      *pb.AutoRecyclePolicy   // 待提交的自动回收策略。
	ApplyExisting    bool                    // 保存策略时是否同时尝试回收现有物品。
	Locked           bool                    // 库存实例要切换到的锁定状态。
	Warehouse        *pb.WarehouseAction     // 仓库动作的完整协议参数。
	Vendor           *pb.VendorCommitRequest // NPC 买卖或修理的报价提交参数。
	VendorShop       string                  // 房间根据 NPC 与当前地图确定的商店身份。
	ForgeCommit      *pb.ForgeCommitRequest  // 图纸打造的原确认报价编号。
	ForgeGate        uint32                  // 当前场景对首次打造写入的门禁，原回执优先。
	ForgeAcceptedAt  time.Time               `json:"-"` // 实际收到打造确认请求的时刻，在同一任务的重试中保持。

	WeaponOilCommit     *pb.WeaponOilCommitRequest // 武器或翅膀用油的原确认报价、实例和界面会话。
	WeaponOilGate       uint32                     // 房间只对首次用油生效的门禁码。
	WeaponOilAcceptedAt time.Time                  `json:"-"` // 房间收到原用油确认的时刻，在排队和重试中保持。
	WingAction          *pb.WingActionRequest      // 角色确认的原翅膀升阶或外观请求。
	WingGate            uint32                     // 首次变更时原场景的门禁，不影响历史回执。
	WingAcceptedAt      time.Time                  `json:"-"` // 游戏房间首次受理本请求的时间。

	EquipmentEnhancement           *pb.EquipmentEnhancementCommitRequest // 角色确认的原部位强化报价。
	EquipmentEnhancementGate       uint32                                // 首次提交在当前场景的受理结果。
	EquipmentEnhancementAcceptedAt time.Time                             `json:"-"` // 原确认到达游戏房间的时刻，排队重试保持。

	EquipmentRefine           *pb.EquipmentRefineActionRequest // 原付费、采纳或放弃指令。
	EquipmentRefineGate       uint32                           // 当前场景对首次洗炼写入的门禁。
	EquipmentRefineAcceptedAt time.Time                        `json:"-"` // 首次确认到达游戏房间的时间，排队重试保持。

	MedalCommit     *pb.MedalCommitRequest // 角色在原面板确认的勋章逐级报价。
	MedalGate       uint32                 // 当前场景只对首次成长生效的门禁。
	MedalAcceptedAt time.Time              `json:"-"` // 原确认到达游戏房间的时刻，排队和重试保持。

	RebirthCommit     *pb.RebirthCommitRequest // 原面板确认的转生报价。
	RebirthGate       uint32                   // 当前场景对首次转生的门禁。
	RebirthAcceptedAt time.Time                `json:"-"` // 原确认到达时刻，排队重试沿用。

	BodyTrainingCommit     *pb.BodyTrainingCommitRequest // 原炼体确认。
	BodyTrainingGate       uint32                        // 当前位置的首次尝试门禁。
	BodyTrainingAcceptedAt time.Time                     `json:"-"` // 已受理的时刻用于报价时限。
	SoulAction             *pb.SoulActionRequest         // 原收藏修订与魂兽确认条款。
	SoulGate               bool                          // 首次写入受当前地图限制。
	GrowthShopCommit       *pb.GrowthShopCommitRequest   // 原商城确认的报价、商品和份数。
	GrowthShopQuote        *GrowthShopQuote              // 房间在原附着签发的可信购买条款。
	GrowthShopGate         bool                          // 仅首次确认受原场景门禁。
	BattlePass             *pb.BattlePassClaimRequest    // 原周期、相对日和期待的领取差额。
	BattlePassGate         bool                          // 只拒绝当前场景发起的新领取。

	TaskCodex            *pb.TaskCodexClaimRequest      // 携带待领取定义及当日资格的完整请求。
	ChallengeRewardID    string                         // 领取的原挑战待领编号。
	SkillSlotIDs         []uint32                       // 四个有序技能槽，零为合法空位。
	ExpectedSkillSlotIDs []uint32                       // 客户端编辑前看到的四槽，缺省时比较角色修订。
	MirrorExchange       *pb.MirrorArenaExchangeRequest // 兑换份数与确认页上的权威商品条款。
	MirrorExchangeGate   uint32                         // 原普通场景是否允许一次新的兑换，重放先核对回执。
	ItemMarket           *pb.ItemMarketAction           // 原确认页的道具市场命令。
	ItemMarketGate       uint32                         // 只拒绝当前场景的新市场写入，不覆盖原有回执。
	GoldMarket           *pb.GoldMarketAction           // 原金币委托、撤销或领取命令。
	GoldMarketGate       uint32                         // 当前场景对首次金币委托写入的门禁。
	MiningAutoRecycle    bool                           // 玩家为后续新矿石选定的独立自动回收开关。
}

// AssetAutoUsePotion 区分服务端自动扣药和玩家主动用药。
const AssetAutoUsePotion = "auto_use_item"

// add 安全增加非负数量，超出 int64 范围时拒绝此次业务变更。
// a 是已有余额或计数，b 是本次待累加的非负增量。
func add(a, b int64) (int64, error) {
	if b < 0 || a > math.MaxInt64-b {
		return a, Fail(p.CodeInvalidArgument, "整数余额溢出")
	}
	return a + b, nil
}

// Asset 执行资产命令，面向无需读取分类回执的调用者。
// s 提供业务存储，ctx 限定执行时长，a 提供账号与角色身份，q 携带本次资产请求。
func (s *Store) Asset(ctx context.Context, a Auth, q AssetRequest) (domain.Role, *pb.OperationResult, error) {
	// role 是事务确认的角色状态，result 是协议操作结果，err 区分事务失败与确定的业务拒绝。
	role, result, _, err := s.AssetWithReceipt(ctx, a, q)
	return role, result, err
}

// 回执保留首次提交的原命令序号；OperationResult仍返回客户端需要的当前经济水位。
// s 提供业务存储，ctx 约束排队与提交，a 用于运行、账号和会话围栏，q 保留原操作身份与参数。
func (s *Store) AssetWithReceipt(ctx context.Context, a Auth, q AssetRequest) (domain.Role, *pb.OperationResult, Operation, error) {
	// out 保存当前角色状态，提交确认后替换成该操作结果。
	var out domain.Role
	// receipt 保存首次确定的永久操作回执，兼供相同请求重放。
	var receipt Operation
	// result 返回客户端核对所需的操作编号、当前版本和经济序号。
	result := &pb.OperationResult{}
	if q.Context == nil || len(q.Context.OpId) != 16 {
		return out, result, receipt, Fail(p.CodeInvalidArgument, "资产请求缺少16字节op_id")
	}
	result.OpId = q.Context.OpId
	var taskEventAt time.Time // 事务重试在同一次任务处理内采用同一个运营时点。
	if s.Cfg.Fixture.TasksCodex != nil {
		taskEventAt = time.Now()
	}
	// hostingClaimAt 固定领取的资格和运营日，数据库重试期间不再读取时钟。
	hostingClaimAt := q.HostingClaimAt
	if q.Kind == AssetClaimHostingTime {
		if hostingClaimAt.IsZero() {
			hostingClaimAt = time.Now()
		}
		hostingClaimAt = hostingClaimAt.UTC().Truncate(time.Millisecond)
	}
	if q.Kind == AssetForgeCommit && q.ForgeAcceptedAt.IsZero() {
		q.ForgeAcceptedAt = time.Now()
	}
	if q.Kind == AssetWeaponOilCommit && q.WeaponOilAcceptedAt.IsZero() {
		q.WeaponOilAcceptedAt = time.Now()
	}
	if q.Kind == AssetWingAction && q.WingAcceptedAt.IsZero() {
		q.WingAcceptedAt = time.Now()
	}
	if q.Kind == AssetEquipmentEnhancement && q.EquipmentEnhancementAcceptedAt.IsZero() {
		q.EquipmentEnhancementAcceptedAt = time.Now()
	}
	if q.Kind == AssetEquipmentRefine && q.EquipmentRefineAcceptedAt.IsZero() {
		q.EquipmentRefineAcceptedAt = time.Now()
	}
	if q.Kind == AssetMedalCommit && q.MedalAcceptedAt.IsZero() {
		q.MedalAcceptedAt = time.Now()
	}
	if q.Kind == AssetRebirthCommit && q.RebirthAcceptedAt.IsZero() {
		q.RebirthAcceptedAt = time.Now()
	}
	if q.Kind == AssetBodyTrainingCommit && q.BodyTrainingAcceptedAt.IsZero() {
		q.BodyTrainingAcceptedAt = time.Now()
	}
	// 经济命令和服务端自动来源使用不同的序号与操作命名空间。
	// opID 标识当前业务使用的操作。
	opID := "asset:" + a.RoleID + ":" + domain.Hex(q.Context.OpId)
	if q.SourceSeq > 0 {
		// source 是自动施法材料或自动补蓝的操作键前缀。
		var source string
		switch q.Kind {
		case "cast_material":
			source = "source_cast:"
		case AssetAutoUsePotion:
			source = "source_potion:"
		}
		if source == "" || q.Context.CommandSeq != 0 {
			return out, result, receipt, Fail(p.CodeInvalidArgument, "自动资产操作必须使用独立来源流水")
		}
		opID = source + a.RoleID + ":" + domain.Hex(q.Context.OpId)
	} else if q.Kind == AssetAutoUsePotion {
		return out, result, receipt, Fail(p.CodeInvalidArgument, "自动用药缺少来源流水")
	}
	recycleDraw := equipmentRecycleAttempt{operationID: opID} // 懒取种子，已有回执不抽样，事务重试沿用。
	// e 保存整个事务的执行错误，tc 将回调里的所有数据库操作纳入同一次事务。
	e := s.guardedTx(ctx, a, func(tc context.Context) error {
		receipt = Operation{}
		// e 保存运行组或账号围栏未通过时的错误，提前终止本次事务。
		if e := s.fence(tc, a); e != nil {
			return e
		}
		// r 是当前事务快照中的角色聚合，e 保存读取失败原因。
		r, e := s.Role(tc, a.RoleID)
		if e != nil {
			return e
		}
		if e = s.checkRole(r, a); e != nil {
			return e
		}
		out = r
		result.NewRevision = uint64(r.Revision)
		result.CommandSeq = uint64(r.EconomicSeq)
		// old 保存相同操作键已有的永久回执，供幂等核对。
		var old Operation
		e = s.DB.Collection("operations").FindOne(tc, bson.M{"_id": opID}).Decode(&old)
		if e == nil {
			if old.Digest != q.Fingerprint || q.SourceSeq > 0 && old.SourceSeq != q.SourceSeq {
				return Fail(p.CodeOperationConflict, "同op_id请求内容改变")
			}
			result.CommandSeq = uint64(r.EconomicSeq)
			result.NewRevision = uint64(r.Revision)
			result.ErrorCode = old.Code
			receipt = old
			return nil
		}
		if !errors.Is(e, mongo.ErrNoDocuments) {
			return e
		}
		// 读取已有回执后才检查下一序号和期望版本，允许重放早已提交的原命令。
		// sequenceValid 检查请求是否恰好推进其所属流水，玩家命令用 EconomicSeq，服务端自动事实用 SourceSeq。
		sequenceValid := q.SourceSeq == 0 && q.Context.CommandSeq == uint64(r.EconomicSeq+1) || q.SourceSeq > 0 && q.SourceSeq == r.SourceSeq+1
		if r.Revision == math.MaxInt64 || q.SourceSeq == 0 && r.EconomicSeq == math.MaxInt64 || q.SourceSeq > 0 && r.SourceSeq == math.MaxInt64 {
			return Fail(p.CodeOperationConflict, "资产序号已耗尽")
		}
		revisionValid := matchesAssetRevision(r, &s.Cfg.Fixture, q)
		switch q.Kind {
		case AssetMirrorArenaExchange, AssetBuyRoleCard, AssetClaimRoleCardDaily, AssetItemMarket, AssetGoldMarket, AssetSetMiningAutoRecycle, AssetForgeCommit, AssetWeaponOilCommit, AssetWingAction, AssetEquipmentEnhancement, AssetEquipmentRefine, AssetMedalCommit, AssetRebirthCommit, AssetBodyTrainingCommit, AssetSoulAction, AssetGrowthShopCommit, AssetBattlePassClaim:
			// 自动收益会推进角色修订。这些操作仍核对经济序号和规则，并在业务分支复核当前资源或偏好资格。
			revisionValid = q.Context.ExpectedRevision > 0 && q.Context.ExpectedRevision <= uint64(r.Revision)
		}
		if !sequenceValid || !revisionValid || q.Context.ExpectedRuleVersion != s.Cfg.Fixture.RuleVersion {
			return Fail(p.CodeOperationConflict, "操作序列/版本变化，请刷新后保留原操作身份")
		}
		// oldRevision 保存修订号，用于当前步骤中的原状态修订号。
		oldRevision := r.Revision
		// businessCode 保存本次确定的业务结果，零表示业务操作成功。
		businessCode := uint32(0)
		// potion 记录药水，使用药水回执表示。
		var potion *PotionReceipt
		// skillDetails 保存已经应用的技能材料、费用和产物计划。
		var skillDetails *skillPlan
		// casting 记录施法，使用施法回执表示。
		var casting *CastingReceipt
		// recycling 记录自动回收，使用装备自动回收回执表示。
		var recycling *EquipmentRecyclingReceipt
		// equipment 记录装备，使用装备回执表示。
		var equipment *EquipmentReceipt
		// warehouse 记录仓库，使用仓库回执表示。
		var warehouse *WarehouseReceipt
		// vendor 保存实际成交的商人买卖或修理结果。
		var vendor *VendorReceipt
		var forge *ForgeResultReceipt                      // 真成功或真失败时的原打造事实。
		var weaponOil *WeaponOilResultReceipt              // 一瓶原油的实际扣除和原培养对象结果。
		var wing *WingActionReceipt                        // 原翅膀的新状态和确切羽毛消费。
		var enhancement *EquipmentEnhancementReceipt       // 原强化结果和每个已扣石堆。
		var refine *EquipmentRefineReceipt                 // 原洗炼动作与已付结果的永久确认。
		var medal *MedalCommitReceipt                      // 原勋章档案与确切碎片消费。
		var rebirth *RebirthCommitReceipt                  // 原转生、经验释放与确切资产扣除。
		var bodyTraining *BodyTrainingReceipt              // 炼体每次实际的扣款、成功与原等级。
		var soul *domain.SoulReceipt                       // 角色魂兽、碎片变更或原首领附件结果。
		var growthShop *GrowthShopReceipt                  // 商城原扣款、额度和绑定到账实例。
		var battlePass *BattlePassReceipt                  // 真实已领取的原战令档位和到账差额。
		var taskCodex *TaskCodexClaimReceipt               // 首次领取确认的任务或图鉴奖励。
		var challengeReward *domain.ChallengePendingReward // 首次领取的原挑战资产。
		var hostingClaim *HostingClaimReceipt              // 首次确认的日领时长及原权益快照。
		var hostingPurchase *HostingPurchaseReceipt        // 首次补时确认的原价格、扣款与到账。
		var mirrorExchange *MirrorExchangeReceipt          // 首次兑换确认的原扣分、余量与绑定羽毛。
		var cardPurchase *RoleCardPurchaseReceipt          // 首次购卡的扣款和原有效期变更。
		var cardDaily *RoleCardDailyReceipt                // 首次特权日领操作所确认的绑定元宝。
		var itemMarket *ItemMarketReceipt                  // 原上架、冻结、成交或领回的永久回执。
		var goldMarket *GoldMarketReceipt                  // 原金币委托、撤销或领取的永久回执。
		// 计算本次领域变更，并为需要专属回执的操作保留分类结果。
		switch q.Kind {
		case AssetMirrorArenaExchange:
			r, mirrorExchange, businessCode, e = s.exchangeMirrorFeather(tc, r, q, opID)
			if e != nil {
				return e
			}
		case AssetVendor:
			r, businessCode, vendor, e = s.commitVendor(tc, a, r, q)
			if e != nil {
				return e
			}
		case AssetForgeCommit:
			r, businessCode, forge, e = s.commitForge(tc, a, r, q, opID)
			if e != nil {
				return e
			}
		case AssetWeaponOilCommit:
			r, businessCode, weaponOil, e = s.commitWeaponOil(tc, a, r, q)
			if e != nil {
				return e
			}
		case AssetWingAction:
			r, businessCode, wing, e = s.commitWingAction(tc, a, r, q, opID)
			if e != nil {
				return e
			}
		case AssetEquipmentEnhancement:
			r, businessCode, enhancement, e = s.commitEquipmentEnhancement(tc, a, r, q)
			if e != nil {
				return e
			}
		case AssetEquipmentRefine:
			r, businessCode, refine, e = s.commitEquipmentRefine(tc, a, r, q, opID)
			if e != nil {
				return e
			}
		case AssetMedalCommit:
			r, businessCode, medal, e = s.commitMedal(tc, a, r, q)
			if e != nil {
				return e
			}
		case AssetRebirthCommit:
			r, businessCode, rebirth, e = s.commitRebirth(tc, a, r, q)
			if e != nil {
				return e
			}
		case AssetBodyTrainingCommit:
			r, businessCode, bodyTraining, e = s.commitBodyTraining(tc, a, r, q)
			if e != nil {
				return e
			}
		case AssetSoulAction:
			r, soul, businessCode = commitSoulAction(r, s.Cfg.Fixture.Souls, q)
		case AssetGrowthShopCommit:
			r, growthShop, businessCode = commitGrowthShop(r, &s.Cfg.Fixture, q, time.Now())
		case AssetBattlePassClaim:
			r, battlePass, businessCode, e = s.claimBattlePass(tc, r, q, taskEventAt, opID)
			if e != nil {
				return e
			}
		case "warehouse":
			if q.Warehouse == nil || !proto.Equal(q.Warehouse.Operation, q.Context) {
				businessCode = p.CodeInvalidArgument
				break
			}
			r, warehouse, businessCode, e = s.applyWarehouse(tc, r, q.Warehouse)
			if e != nil {
				return e
			}
		case "equip":
			r, equipment, businessCode = equipEquipment(r, q, s.Cfg.Fixture)
		case "recycle":
			r, recycling, businessCode, e = recycleManual(r, q.ItemIDs, s.Cfg.Fixture, &recycleDraw)
			if e != nil {
				return e
			}
		case "set_auto_recycle":
			r, recycling, businessCode, e = setAutoRecycle(r, q, s.Cfg.Fixture)
			if e != nil {
				return e
			}
		case AssetSetMiningAutoRecycle:
			r, businessCode = setMiningAutoRecycle(r, q.MiningAutoRecycle, &s.Cfg.Fixture)
		case "set_item_lock":
			r, businessCode = setItemLock(r, q, s.Cfg.Fixture)
		case AssetClaimHostingTime:
			r, hostingClaim, businessCode, e = applyHostingClaim(r, q.Benefit, hostingClaimAt, s.Cfg.Fixture.HostingBenefits)
			if e != nil {
				return e
			}
		case AssetBuyHostingTime:
			// 仅对首次写入检查事务当前的价档和时钟。更早的确定回执已在前面按原操作身份返回。
			r, hostingPurchase, businessCode = applyHostingPurchase(r, &s.Cfg.Fixture, q.HostingPurchase, q.SceneEpoch, time.Now())
		case AssetBuyRoleCard:
			r, cardPurchase, businessCode = applyRoleCardPurchase(r, &s.Cfg.Fixture, q.CardPurchase, q.SceneEpoch, time.Now())
		case AssetClaimRoleCardDaily:
			r, cardDaily, businessCode, e = s.applyRoleCardDaily(tc, r, q.CardDay, bossTime(q.CardAcceptedAt))
			if e != nil {
				return e
			}
		case AssetItemMarket:
			r, itemMarket, businessCode, e = s.applyItemMarket(tc, r, q.ItemMarket, q.ItemMarketGate)
			if e != nil {
				return e
			}
		case AssetGoldMarket:
			r, goldMarket, businessCode, e = s.applyGoldMarket(tc, r, q.GoldMarket, q.GoldMarketGate)
			if e != nil {
				return e
			}
		case "revive":
			if !q.ValidRevive || r.RetiredDeathID != "" && r.RetiredDeathID != q.ReviveDeathID {
				businessCode = p.CodeInvalidArgument
				break
			}
			// 只有成功确认的原复活选择能清除死亡标记。临时位置和生命仍由场景处理。
			r.RetiredDeathID = ""
		case "use_item", AssetAutoUsePotion:
			// effect 记录效果，使用药水效果表示。
			var effect devconfig.PotionEffect
			r, effect, businessCode = consumePotion(r, q, s.Cfg.Fixture)
			if businessCode != 0 {
				if q.Kind == AssetAutoUsePotion {
					return Fail(businessCode, "未满足自动用药资格")
				}
				break
			}
			potion = &PotionReceipt{Scene: strconv.FormatUint(q.SceneEpoch, 10), ItemID: effect.ItemID, HP: effect.HP, MP: effect.MP}
		case "learn_skill":
			r, businessCode = learnSkill(r, q, s.Cfg.Fixture)
		case AssetTaskCodexClaim:
			if q.TaskCodex == nil || !proto.Equal(q.TaskCodex.Operation, q.Context) {
				businessCode = p.CodeInvalidArgument
				break
			}
			var worldProgress domain.TaskCodexProgress
			if s.Cfg.Fixture.HasWorldBossTaskProgress() {
				worldProgress, e = s.ReadWorldBossTaskProgress(tc, r)
				if e != nil {
					return e
				}
			}
			r, taskCodex, businessCode = claimTaskCodex(r, &s.Cfg.Fixture, q.TaskCodex, taskEventAt, opID, worldProgress)
			if businessCode == 0 && q.TaskCodex.Kind == devconfig.TaskCodexDaily {
				if e = s.recordBattlePassActivity(tc, r, taskEventAt); e != nil {
					return e
				}
			}
		case AssetChallengeRewardClaim:
			r, challengeReward, businessCode = claimChallengeReward(r, q.ChallengeRewardID)
			if businessCode == p.CodeOperationConflict {
				// 个人挑战无此编号时，再按原永久角色查询世界和讨伐附件。
				r, challengeReward, soul, businessCode, e = s.claimExternalChallengeReward(tc, r, q.ChallengeRewardID, opID)
				if e != nil {
					return e
				}
			}
		case AssetSetSkillSlots:
			r, businessCode = setSkillSlots(r, &s.Cfg.Fixture, q.SkillSlotIDs)
		case "set_casting_supply":
			r, businessCode = setCastingSupply(r, q, s.Cfg.Fixture)
		case "cast_material":
			r, casting, businessCode = consumeCastingMaterial(r, q, s.Cfg.Fixture)
		case "skill_assets_commit":
			r, businessCode, skillDetails, e = s.commitSkillAsset(tc, r, q)
			if e != nil {
				return e
			}
		default:
			businessCode = p.CodeInvalidArgument
		}
		// 确定的业务拒绝保留原资产内容，随后写入原操作终态避免重试改判。
		if businessCode != 0 {
			r = out
			recycling = nil
			skillDetails = nil
			equipment = nil
			warehouse = nil
			vendor = nil
			forge = nil
			taskCodex = nil
		} else {
			r = advanceAssetTasks(r, &s.Cfg.Fixture, q.Kind, recycling, taskEventAt)
		}
		if businessCode == 0 && len(r.Items) < r.Capacity {
			r.PickupStopped = false
		}
		if q.SourceSeq > 0 {
			r.SourceSeq++
		} else {
			r.EconomicSeq++
		}
		r.Revision++
		if e = s.replace(tc, r, oldRevision); e != nil {
			return e
		}
		if cardPurchase != nil {
			if e = s.insertRoleCardPeriod(tc, r, opID, cardPurchase.Quote.Kind, cardPurchase.EffectiveAt,
				cardPurchase.PreviousSince, cardPurchase.PreviousUntil); e != nil {
				return e
			}
		}
		// 角色变更、永久回执和审计同属本事务，其中任何一项失败都会回滚。
		// op 组装操作所需的数据。
		op := Operation{ID: opID, Digest: q.Fingerprint, RoleID: r.ID, CommandSeq: r.EconomicSeq, SourceSeq: q.SourceSeq, Revision: r.Revision, Code: businessCode, Kind: q.Kind, Created: time.Now(), Potion: potion, Casting: casting, Recycling: recycling}
		op.Skill = cloneSkillPlan(skillDetails)
		op.Equipment = cloneEquipmentReceipt(equipment)
		op.Warehouse = cloneWarehouseReceipt(warehouse)
		op.Vendor = vendor
		op.Forge = forge
		op.WeaponOil = weaponOil
		op.Wing = wing
		op.EquipmentEnhancement = enhancement
		op.EquipmentRefine = refine
		op.Medal = medal
		op.Rebirth = rebirth
		op.BodyTraining = bodyTraining
		op.Soul = soul
		op.GrowthShop = growthShop
		op.BattlePass = battlePass
		op.TaskCodex = taskCodex
		op.ChallengeReward = challengeReward
		if q.Kind == AssetSetSkillSlots && businessCode == 0 {
			op.SkillSlotIDs = r.SkillSlotIDs
		}
		op.HostingClaim = hostingClaim
		op.HostingPurchase = hostingPurchase
		op.MirrorExchange = mirrorExchange
		op.CardPurchase, op.CardDaily = cardPurchase, cardDaily
		op.ItemMarket = itemMarket
		op.GoldMarket = goldMarket
		if q.Kind == "set_auto_recycle" && businessCode == 0 {
			op.AutoRecyclePolicy = policySnapshot(r.AutoRecycle)
		}
		if q.Kind == AssetSetMiningAutoRecycle && businessCode == 0 {
			enabled := r.MiningAutoRecycle
			op.MiningAutoRecycle = &enabled
		}
		if _, e = s.DB.Collection("operations").InsertOne(tc, op); e != nil {
			return e
		}
		// audit 初始化用作审计记录的MongoDB BSON 文档。
		audit := bson.M{"_id": opID, "role_id": r.ID, "account_id": a.AccountID, "kind": q.Kind, "op_id": domain.Hex(q.Context.OpId), "revision": r.Revision, "result_code": businessCode, "at": time.Now()}
		if op.AutoRecyclePolicy != nil {
			audit["auto_recycle_policy"] = op.AutoRecyclePolicy
		}
		if op.MiningAutoRecycle != nil {
			audit["mining_auto_recycle"] = *op.MiningAutoRecycle
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
		}
		if skillDetails != nil {
			audit["preview_id"] = q.PreviewID
			audit["skill_plan"] = skillDetails
		}
		if equipment != nil {
			audit["equipment"] = cloneEquipmentReceipt(equipment)
		}
		if warehouse != nil {
			audit["warehouse"] = cloneWarehouseReceipt(warehouse)
		}
		if vendor != nil {
			audit["npc_trade"] = vendor
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
		}
		if forge != nil {
			audit["forge"] = forge
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
			audit["origin_server_id"] = r.Origin
		}
		if weaponOil != nil {
			audit["weapon_oil"] = weaponOil
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
			audit["origin_server_id"] = r.Origin
		}
		if wing != nil {
			audit["wing"] = wing
			audit["rule_version"] = wing.Rule
			audit["config_hash"] = wing.Config
			audit["origin_server_id"] = r.Origin
		}
		if enhancement != nil {
			audit["equipment_enhancement"] = enhancement
			audit["rule_version"] = enhancement.Rule
			audit["config_hash"] = enhancement.Config
			audit["origin_server_id"] = r.Origin
		}
		if refine != nil {
			audit["equipment_refine"] = refine
			audit["rule_version"] = refine.Rule
			audit["config_hash"] = refine.Config
			audit["origin_server_id"] = r.Origin
		}
		if medal != nil {
			audit["medal"] = medal
			audit["rule_version"] = medal.Rule
			audit["config_hash"] = medal.Config
			audit["origin_server_id"] = r.Origin
		}
		if rebirth != nil {
			audit["rebirth"] = rebirth
			audit["rule_version"] = rebirth.Rule
			audit["config_hash"] = rebirth.Config
			audit["origin_server_id"] = r.Origin
		}
		if bodyTraining != nil {
			audit["body_training"] = bodyTraining
			audit["rule_version"] = bodyTraining.Rule
			audit["config_hash"] = bodyTraining.Config
			audit["origin_server_id"] = r.Origin
		}
		if soul != nil {
			audit["soul"] = soul
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
			audit["origin_server_id"] = r.Origin
		}
		if growthShop != nil {
			audit["growth_shop"] = growthShop
			audit["rule_version"] = growthShop.Quote.Rule
			audit["config_hash"] = growthShop.Quote.Config
			audit["origin_server_id"] = r.Origin
		}
		if battlePass != nil {
			audit["battle_pass"] = battlePass
			audit["origin_server_id"] = r.Origin
		}
		if casting != nil {
			audit["casting_use"] = casting
			audit["source_seq"] = q.SourceSeq
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
		}
		if q.Kind == AssetAutoUsePotion {
			audit["potion"] = potion
			audit["item_instance_id"] = q.ItemID
			audit["source_seq"] = q.SourceSeq
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
		}
		if recycling != nil {
			audit["recycling"] = recycling
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
		}
		if taskCodex != nil {
			audit["tasks_codex_claim"] = taskCodex
		}
		if challengeReward != nil {
			audit["challenge_reward_claim"] = challengeReward
		}
		if op.SkillSlotIDs != nil {
			audit["skill_slot_ids"] = op.SkillSlotIDs
		}
		if hostingClaim != nil {
			audit["hosting_claim"] = hostingClaim
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
		}
		if hostingPurchase != nil {
			audit["hosting_purchase"] = hostingPurchase
			audit["rule_version"] = hostingPurchase.Quote.RuleVersion
			audit["config_hash"] = hostingPurchase.Quote.ConfigHash
		}
		if mirrorExchange != nil {
			audit["mirror_arena_exchange"] = mirrorExchange
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
		}
		if cardPurchase != nil {
			audit["role_card_purchase"] = cardPurchase
			audit["rule_version"] = cardPurchase.Quote.RuleVersion
			audit["config_hash"] = cardPurchase.Quote.ConfigHash
			audit["origin_server_id"] = r.Origin
		}
		if cardDaily != nil {
			audit["role_card_daily"] = cardDaily
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
			audit["origin_server_id"] = r.Origin
		}
		if itemMarket != nil {
			audit["item_market"] = itemMarket
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
			audit["origin_server_id"] = r.Origin
		}
		if goldMarket != nil {
			audit["gold_market"] = goldMarket
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
			audit["origin_server_id"] = r.Origin
		}
		if _, e = s.DB.Collection("audit_outbox").InsertOne(tc, audit); e != nil {
			return e
		}
		out = r
		result.NewRevision = uint64(r.Revision)
		result.CommandSeq = uint64(r.EconomicSeq)
		result.ErrorCode = businessCode
		receipt = op
		return nil
	})
	if e != nil {
		result.ErrorCode = Code(e)
		result.Pending = result.ErrorCode == p.CodeOperationPending
	} else {
		result.HostingPurchase = receipt.HostingPurchase.Proto()
		result.MirrorArenaExchange = receipt.MirrorExchange.Proto()
		result.RoleCardPurchase = receipt.CardPurchase.Proto()
		result.RoleCardDaily = receipt.CardDaily.Proto()
		result.ItemMarket = receipt.ItemMarket.Proto()
		result.GoldMarket = receipt.GoldMarket.Proto()
		result.Forge = receipt.Forge.Proto()
		result.WeaponOil = receipt.WeaponOil.Proto()
		result.Wing = receipt.Wing.Proto()
		if receipt.EquipmentEnhancement != nil {
			result.EquipmentEnhancement = receipt.EquipmentEnhancement.Proto()
		}
		result.EquipmentRefine = receipt.EquipmentRefine.Proto()
		result.Medal = receipt.Medal.Proto()
		result.Rebirth = receipt.Rebirth.Proto()
		result.BodyTraining = receipt.BodyTraining.Proto()
		result.Soul = receipt.Soul.Proto()
		result.GrowthShop = receipt.GrowthShop.Proto()
		result.BattlePass = receipt.BattlePass.Proto()
		result.RecycleOil = receipt.Recycling.oilProto()
		result.RecycleRefine = receipt.Recycling.refineProto()
	}
	return out, result, receipt, e
}

// 全部条件先验证；修改独立切片，业务拒绝不能污染原角色的库存。
// r 是扣药前角色，q 指定原库存实例和场景，fixture 提供当前药品规则。
func consumePotion(r domain.Role, q AssetRequest, fixture devconfig.Fixture) (domain.Role, devconfig.PotionEffect, uint32) {
	// effect 是配置中的药效，ok 表示该物品有药效记录。
	effect, ok := fixture.Potions.Effect(q.ItemDefinitionID)
	// definition 是物品定义，exists 用来拒绝不存在或旧配置中已经移除的物品。
	definition, exists := fixture.Item(q.ItemDefinitionID)
	if !ok || !exists || definition.IsEquipment || effect.HP < 0 || effect.MP < 0 || (effect.HP == 0 && effect.MP == 0) || q.SceneEpoch == 0 || len(q.ItemID) != 32 {
		return r, effect, p.CodeInvalidArgument
	}
	if q.PotionGateCode != 0 {
		return r, effect, q.PotionGateCode
	}
	// index 保存当前处理所用的下标。
	index := -1
	// i 定位道具集合中正在处理的成员，item 接收遍历道具集合时当前的元素。
	for i, item := range r.Items {
		if item.ID == q.ItemID {
			index = i
			break
		}
	}
	if index < 0 || r.Items[index].ItemID != q.ItemDefinitionID || r.Items[index].Quantity < 1 || r.Items[index].Version < 1 || r.Items[index].Version == math.MaxInt64 {
		return r, effect, p.CodeInvalidArgument
	}
	if q.Kind == AssetAutoUsePotion && (!AutoManaPotionEligible(r.Items[index], r.Job, fixture) || uint64(r.Items[index].Version) != q.ItemVersion) {
		return r, effect, p.CodeInvalidArgument
	}
	r.Items = append([]domain.Item(nil), r.Items...)
	if r.Items[index].Quantity == 1 {
		r.Items = append(r.Items[:index], r.Items[index+1:]...)
	} else {
		r.Items[index].Quantity--
		r.Items[index].Version++
	}
	return r, effect, 0
}

// Settlement 固定服务端某个来源序号需要同时提交的收益、托管扣时与磨损事实。
type Settlement struct {
	SourceSeq        int64           // 同角色必须连续递增的来源序号。
	Kills            int64           // 本来源对应的击杀次数。
	MonsterID        uint32          `json:"monster_id,omitempty"`   // 实际击杀的怪物配置号，旧来源或非击杀为零。
	MapID            uint64          `json:"map_id,omitempty"`       // 原击杀发生的地图配置号，不随之后换图而变。
	DropPoolID       uint32          `json:"drop_pool_id,omitempty"` // 怪物原掉落池号，历史全局掉落和非击杀为零。
	Experience       int64           // 应用等级及经验规则前的原始经验。
	Gold             int64           // 本来源直接产生的金币，不含装备回收。
	Bound            bool            // 直接金币应计入绑定钱包还是普通钱包。
	Item             *domain.Item    // 本来源产生的原掉落实例，可为空。
	NoEquipmentKills int             // 应由本来源确认的连续未出装备次数。
	HostSeconds      int64           // 本来源需要从托管余额扣除的已计量整秒。
	HostingSession   string          // 托管扣时和战报归属的会话标识。
	StopReason       uint32          // 本来源确认的托管停止原因，零表示没有新原因。
	EventAt          time.Time       // 战斗或计量事实的发生时刻，供审计追溯。
	TargetHandle     uint32          // 结算目标在原场景中的实体句柄。
	TargetLife       uint64          // 结算目标的原生命代次。
	CasterHandle     uint32          // 产生该事实的施法者或攻击者句柄。
	EquipmentWear    []EquipmentWear `json:"equipment_wear,omitempty"` // 并入本来源提交的磨损事实明细。
	WearDeathID      string          `json:"wear_death_id,omitempty"`  // 死亡磨损对应的原死亡标识。

	OrdinaryEquipment *OrdinaryEquipmentPlan `json:"ordinary_equipment,omitempty"` // 高品质掉落在原击杀冻结的降档方案。
	HostingKillBonus  *HostingKillBonus      `json:"hosting_kill_bonus,omitempty"` // 普通怪原击杀享受的托管卡直接收益。
}
