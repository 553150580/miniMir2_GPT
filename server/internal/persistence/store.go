// MongoDB 存储总入口。装配业务与控制连接池、创建索引，统一协议错误、角色版本替换、运行围栏和事务重试。

package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"strings"
	"time"
)

// Fault 携带可上送客户端的稳定协议码和供日志排查的文字。
type Fault struct {
	Code uint32 // 与公共协议清单一致的错误分类。
	Text string // 当前失败原因的可读说明。
}

// Error 实现 Go error 接口，让持久化错误沿标准错误链传播。
// e 是已经构造的带协议码业务错误。
func (e *Fault) Error() string { return e.Text }

// Fail 把可确定的业务拒绝转换成带协议码的错误。
// code 是对客户端稳定的协议错误码，text 说明本次拒绝原因。
func Fail(code uint32, text string) error { return &Fault{code, text} }

// Code 提取错误链里的业务码，其他非空错误按服务器繁忙返回。
// err 保存当前操作的失败原因，供调用方检查或返回。
func Code(err error) uint32 {
	// f 接收错误链中可识别的业务拒绝。
	var f *Fault
	if errors.As(err, &f) {
		return f.Code
	}
	if err != nil {
		return p.CodeServerBusy
	}
	return 0
}

// Store 持有本进程的数据库客户端、业务配置和已经获准的运行身份。
type Store struct {
	Client          *mongo.Client       // 账号与角色读写、业务事务使用的连接池。
	Control         *mongo.Client       // 专供节点续租，避免续租被业务请求积压。
	DB              *mongo.Database     // 业务客户端指向的本项目游戏库。
	Cfg             devconfig.Config    // 当前数据库、节点、原区服、运行服和玩法配置快照。
	Instance        string              // 启动时分配的进程实例标识。
	Epoch           int64               // 运行服授权代次，随所有权接管增大。
	runGuardSlots   *runGuardSlots      // 本进程内等待同组围栏的并发槽。
	gameMemberships []gameRunMembership // 根存储拥有的完整启动授权，监听前一次性安装。
	borrowedRunView bool                // 此视图只借用连接池及运行服业务权限。
}

// 业务连接池为任务执行器保留并发空间。续租走独立控制连接池。
const businessPoolSize = 48

// Auth 汇集账号鉴权和角色入场后由服务端确定的身份。
type Auth struct {
	AccountID    string // 账号的持久键，用于角色归属、分组和账号权限核验。
	AuthVersion  int64  // 账号授权代次，封禁或换代后旧快照失效。
	RoleID       string // 已选角色的持久键，纯账号鉴权时为空。
	SessionEpoch int64  // WebSocket 所绑定的角色会话代次，接管后旧连接失效。
}

// Run 是 MongoDB 中运行服所有权的权威快照。
type Run struct {
	ID                uint64    `bson:"_id"`                          // 运行服编号，可与角色的永久入口编号独立。
	GameNodeID        string    `bson:"game_node_id,omitempty"`       // 注册获准承载本运行服的稳定节点编号。
	AssignmentVersion int64     `bson:"assignment_version,omitempty"` // 注册时核对的持久部署版本。
	Owner             string    `bson:"owner_instance"`               // 当前有权操作该运行服的游戏进程实例。
	Epoch             int64     `bson:"run_epoch"`                    // 所有权代次，阻止旧进程和旧票据写入。
	ControlVersion    int64     `bson:"control_version"`              // 和每个分组围栏对齐的控制版本，释放或接管时递增。
	LeaseUntil        time.Time `bson:"lease_until"`                  // 按数据库时钟计算的租约结束时刻。
	State             string    `bson:"state"`                        // active 表示已授权运行，stopped 表示已停止。
}

// Operation 保存首个确定结果。重试读取这份回执核对请求内容和返回原结果。
type Operation struct {
	ID                string                         `bson:"_id"`                              // 包含业务前缀和作用域的唯一操作键。
	Digest            string                         `bson:"digest"`                           // 首次提交请求的内容摘要，检测同键参数变化。
	RoleID            string                         `bson:"role_id"`                          // 操作或收益所属的角色持久键。
	CommandSeq        int64                          `bson:"command_seq"`                      // 首次处理时的客户端经济命令水位。
	SourceSeq         int64                          `bson:"source_seq,omitempty"`             // 由服务端发起的收益或自动操作来源水位。
	Revision          int64                          `bson:"revision"`                         // 首次操作完成时的角色版本。
	Code              uint32                         `bson:"code"`                             // 首次确定结果，零表示成功。
	Kind              string                         `bson:"kind"`                             // 区分建角、战斗结算与各类资产操作。
	Created           time.Time                      `bson:"created_at"`                       // 首次结果的记录时间，供查询和审计。
	Potion            *PotionReceipt                 `bson:"potion,omitempty"`                 // 已扣库存及归属场景的药水效果。
	Casting           *CastingReceipt                `bson:"casting_use,omitempty"`            // 施法扣除的材料和原生命代次。
	Binding           *BindingReceipt                `bson:"binding_use,omitempty"`            // 安全区绑定变更的地图和材料来源。
	Recycling         *EquipmentRecyclingReceipt     `bson:"equipment_recycling,omitempty"`    // 自动或批量回收的资产增减。
	Settlement        *SettlementReceipt             `bson:"settlement,omitempty"`             // 战斗收益、托管扣时与磨损明细。
	AutoRecyclePolicy *domain.AutoRecyclePolicy      `bson:"auto_recycle_policy,omitempty"`    // 已提交的回收策略快照。
	Experience        *ExperienceGrantReceipt        `bson:"experience_grant,omitempty"`       // 经验派发及成长迁移明细。
	Skill             *skillPlan                     `bson:"skill_plan,omitempty"`             // 首次应用后用于回放的技能资产计划。
	Equipment         *EquipmentReceipt              `bson:"equipment,omitempty"`              // 装备相关操作产生的确定资产回执。
	Warehouse         *WarehouseReceipt              `bson:"warehouse,omitempty"`              // 仓库资产、页号与版本变化。
	Vendor            *VendorReceipt                 `bson:"npc_trade,omitempty"`              // NPC 买卖或修理的原成交参数和结果。
	TaskCodex         *TaskCodexClaimReceipt         `bson:"tasks_codex_claim,omitempty"`      // 原任务或图鉴领取确认的定义及绑定奖励。
	ChallengeReward   *domain.ChallengePendingReward `bson:"challenge_reward_claim,omitempty"` // 由挑战待领转入背包的原实例。
	SkillSlotIDs      []uint32                       `bson:"skill_slot_ids,omitempty"`         // 已确认的四个技能槽位。
	HostingClaim      *HostingClaimReceipt           `bson:"hosting_claim,omitempty"`          // 首次确认的日领时长及原权益快照。
	HostingPurchase   *HostingPurchaseReceipt        `bson:"hosting_purchase,omitempty"`       // 首次购买确认的账本来源、价格和扣款。
	MirrorExchange    *MirrorExchangeReceipt         `bson:"mirror_arena_exchange,omitempty"`  // 首次换取绑定羽毛确认的扣分与实例。
	CardPurchase      *RoleCardPurchaseReceipt       `bson:"role_card_purchase,omitempty"`     // 首次游戏内购卡的扣款和有效期。
	CardDaily         *RoleCardDailyReceipt          `bson:"role_card_daily,omitempty"`        // 首次特权按上海运营日领取的绑元。
	ItemMarket        *ItemMarketReceipt             `bson:"item_market,omitempty"`            // 市场唯一资产操作原回执。
	GoldMarket        *GoldMarketReceipt             `bson:"gold_market,omitempty"`            // 金币委托原操作回执。
}

// 保存首次受理的消耗身份与效果参数；不保存角色HP/MP、恢复队列或冷却现场。
type PotionReceipt struct {
	Scene  string `bson:"scene_epoch"` // 可接受这次回复效果的原场景代次。
	ItemID uint32 `bson:"item_id"`     // 扣除药水的配置编号。
	HP     int64  `bson:"restore_hp"`  // 该药水按配置提供的生命恢复量。
	MP     int64  `bson:"restore_mp"`  // 该药水按配置提供的魔法恢复量。
}

// Hash 生成十六进制 SHA-256 摘要，用于固定操作内容或保存令牌查询键。
// b 是待散列的原始字节，h 暂存 SHA-256 的 32 字节结果。
func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// Open 校验开发库名称和可用性，装配业务、控制连接池并完成索引迁移。
// ctx 向下传递当前请求的截止时间和取消信号，cfg 记录服务配置。
func Open(ctx context.Context, cfg devconfig.Config) (*Store, error) {
	if cfg.Database != "minimir_dev_game" && !strings.HasPrefix(cfg.Database, "minimir_dev_game_test_") {
		return nil, errors.New("拒绝连接非本任务游戏库")
	}
	// c 是用于账号、角色和事务写入的 MongoDB 客户端，e 保存启动期间的连接与迁移错误。
	c, e := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI).SetMaxPoolSize(businessPoolSize).SetServerSelectionTimeout(5 * time.Second))
	if e != nil {
		return nil, e
	}
	if e = c.Ping(ctx, readpref.Primary()); e != nil {
		return nil, e
	}
	// ctl 是只供控制面使用的独立连接客户端，以免业务拥塞阻塞节点续租。
	ctl, e := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI).SetMaxPoolSize(3))
	if e != nil {
		return nil, e
	}
	// s 保存当前处理所用的持久层存储。
	s := &Store{Client: c, Control: ctl, DB: c.Database(cfg.Database), Cfg: cfg, Instance: domain.ID(), runGuardSlots: newRunGuardSlots()}
	if e = s.Migrate(ctx); e != nil {
		c.Disconnect(ctx)
		ctl.Disconnect(ctx)
		return nil, e
	}
	return s, nil
}

// Close 关闭该持久层实例持有的两类 MongoDB 客户端。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号。
func (s *Store) Close(ctx context.Context) {
	if !s.borrowedRunView {
		s.Client.Disconnect(ctx)
		s.Control.Disconnect(ctx)
	}
}

// Migrate 核验数据库归属，确保应用元数据、索引、路由和待授权运行服记录存在。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号。
func (s *Store) Migrate(ctx context.Context) error {
	// names 用于判断目标库是否已有数据，e 保存获取集合或后续初始化失败的错误。
	names, e := s.DB.ListCollectionNames(ctx, bson.M{})
	if e != nil {
		return e
	}
	if len(names) > 0 {
		// marker 接收可证明既有数据库属于本开发项目的元数据。
		var marker bson.M
		if e = s.DB.Collection("application_meta").FindOne(ctx, bson.M{"_id": "minimir_development", "application": "minimir_first_loop"}).Decode(&marker); e != nil {
			return errors.New("既有数据库没有本项目开发标记，拒绝认领或修改")
		}
	}
	if _, e = s.DB.Collection("application_meta").UpdateOne(ctx, bson.M{"_id": "minimir_development"}, bson.M{"$setOnInsert": bson.M{"application": "minimir_first_loop", "schema_version": 1}}, options.UpdateOne().SetUpsert(true)); e != nil {
		return e
	}
	if e = s.retainOperationReceipts(ctx); e != nil {
		return fmt.Errorf("迁移永久操作回执索引失败: %w", e)
	}
	// 账号与角色索引强制唯一性，令牌和短期报价可过期，永久回执由单独迁移保留。
	// indexes 按集合名列出需要建立或确认的索引定义。
	indexes := map[string][]mongo.IndexModel{
		"accounts":        {{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true)}},
		"roles":           {{Keys: bson.D{{Key: "account_id", Value: 1}, {Key: "origin_server_id", Value: 1}, {Key: "slot", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "name_key", Value: 1}}, Options: options.Index().SetUnique(true)}},
		"account_tokens":  {{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}},
		"entry_tickets":   {{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}},
		"origin_routes":   {{Keys: bson.D{{Key: "run_server_id", Value: 1}}}},
		"skill_quotes":    {{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}},
		"vendor_quotes":   {{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}},
		"warehouse_items": {{Keys: bson.D{{Key: "warehouse_id", Value: 1}, {Key: "slot", Value: 1}}, Options: options.Index().SetUnique(true)}},

		// 按角色定位挑战记录和结束时间，永久回执不设过期索引。
		personalBossCollection:    {{Keys: bson.D{{Key: "role_id", Value: 1}, {Key: "finished_at", Value: -1}, {Key: "_id", Value: -1}}}},
		worldBossResultCollection: worldBossResultIndexes(),
		worldBossRewardCollection: externalChallengeRewardIndexes(),
		dungeonCollection:         dungeonIndexes(),
		dungeonRewardCollection:   externalChallengeRewardIndexes(),
		roleCardPeriodCollection:  roleCardPeriodIndexes(),
	}
	if s.Cfg.Fixture.MirrorArena != nil {
		for collection, models := range mirrorArenaIndexes() {
			indexes[collection] = models
		}
		for collection, models := range mirrorSeasonIndexes() {
			indexes[collection] = models
		}
	}
	for collection, models := range itemMarketIndexes() {
		indexes[collection] = models
	}
	for collection, models := range goldMarketIndexes() {
		indexes[collection] = models
	}
	// name 定位目标集合，models 是为该集合建立的索引列表。
	for name, models := range indexes {
		if _, e = s.DB.Collection(name).Indexes().CreateMany(ctx, models); e != nil {
			return e
		}
	}
	return s.seedRoutingCatalog(ctx)
}

// 永久回执不能按时间删除；仅移除本集合created_at单字段TTL，不修改任何业务文档。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号。
func (s *Store) retainOperationReceipts(ctx context.Context) error {
	// view 管理永久回执集合上的索引。
	view := s.DB.Collection("operations").Indexes()
	// list 读取当前索引规格，并接受新库集合尚未创建的情况。
	list := func() ([]mongo.IndexSpecification, error) {
		// indexes 是数据库返回的索引规格，err 区分集合不存在与读取失败。
		indexes, err := view.ListSpecifications(ctx)
		// command 记录命令，使用命令错误表示。
		var command mongo.CommandError
		if errors.As(err, &command) && command.Code == 26 {
			return nil, nil // 新库尚无operations集合，后面的建索引会创建它。
		}
		return indexes, err
	}
	// direction 读取回执时间单字段索引的排序方向，index 是待核查的完整规格。
	direction := func(index mongo.IndexSpecification) (int32, error) {
		// keys 列出索引使用的 BSON 字段，err 表示键文档无法解析。
		keys, err := index.KeysDocument.Elements()
		if err != nil {
			return 0, err
		}
		if len(keys) != 1 || keys[0].Key() != "created_at" {
			return 0, nil
		}
		// value 是索引排序数值，ok 标记其能否按整数读取。
		value, ok := keys[0].Value().AsInt64OK()
		if !ok || (value != 1 && value != -1) {
			return 0, nil
		}
		return int32(value), nil
	}
	// indexes 保存由列表取得的“indexes”，err 接收列表过程中可能产生的错误，供后续失败分支处理。
	indexes, err := list()
	if err != nil {
		return err
	}
	// legacyNames 初始化用作兼容旧数据名称集合的按文本索引的布尔状态映射。
	legacyNames := map[string]bool{}
	// required 建立带初始值的按数值索引的下标“specification”映射，供本段继续处理。
	required := map[int32]mongo.IndexSpecification{1: {}}
	// index 接收遍历“indexes”时当前的元素。
	for _, index := range indexes {
		// order 保存由朝向取得的顺序，err 接收朝向过程中可能产生的错误，供后续失败分支处理。
		order, err := direction(index)
		if err != nil {
			return err
		}
		if order == 0 || index.ExpireAfterSeconds == nil {
			continue
		}
		legacyNames[index.Name] = true
		required[order] = index
		if err = view.DropOne(ctx, index.Name); err != nil {
			// command 记录命令，使用命令错误表示。
			var command mongo.CommandError
			if !errors.As(err, &command) || command.Code != 27 {
				return err
			}
			// 并发启动可能已删除同一旧TTL；只容忍该明确错误，随后仍核对普通索引。
		}
	}
	// order 定位“required”中正在处理的成员，previous 接收遍历“required”时当前的元素。
	for order, previous := range required {
		// hasRegular 封装“has”“regular”对应的后续处理。
		hasRegular := func() (bool, error) {
			// current 保存由列表取得的当前状态，err 接收列表过程中可能产生的错误，供后续失败分支处理。
			current, err := list()
			if err != nil {
				return false, err
			}
			// found 保存当前处理所用的已找到标记。
			found := false
			// index 接收遍历当前状态时当前的元素。
			for _, index := range current {
				// keyOrder 保存由朝向取得的键顺序，err 接收朝向过程中可能产生的错误，供后续失败分支处理。
				keyOrder, err := direction(index)
				if err != nil {
					return false, err
				}
				if keyOrder == order {
					if index.ExpireAfterSeconds != nil {
						return false, errors.New("永久回执created_at仍存在TTL索引")
					}
					found = true
				}
			}
			return found, nil
		}
		// found 保存由“has”“regular”取得的已找到标记，err 接收“has”“regular”过程中可能产生的错误，供后续失败分支处理。
		found, err := hasRegular()
		if err != nil {
			return err
		}
		if found {
			continue
		}
		// name 保存名称所使用的文本。
		name := "operations_created_at_retained"
		if order < 0 {
			name += "_desc"
		}
		// 不复用旧TTL名称，避免另一启动者拿旧列表误删刚建立的普通索引。
		for legacyNames[name] {
			name += "_retained"
		}
		// opts 保存由设置名称取得的选项。
		opts := options.Index().SetName(name)
		if previous.Unique != nil {
			opts.SetUnique(*previous.Unique)
		}
		if previous.Sparse != nil {
			opts.SetSparse(*previous.Sparse)
		}
		if _, err = view.CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "created_at", Value: order}}, Options: opts}); err != nil {
			// 其他模式同时建立同key普通索引时只接受已核实的终态，其余错误使启动失败。
			// found 保存由“has”“regular”取得的已找到标记，inspectErr 保存由“has”“regular”取得的“inspect”错误。
			if found, inspectErr := hasRegular(); inspectErr != nil || !found {
				return err
			}
		}
	}
	return nil
}

// tx 以快照读和多数派确认执行事务，允许驱动在同一截止内重试回调。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，fn 在驱动提供的事务上下文内执行一组业务读写，失败返回后统一回滚。
func (s *Store) tx(ctx context.Context, fn func(context.Context) error) error {
	// cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(ctx, transactionTimeout)
	defer cancel()
	// sess 是事务要复用的 MongoDB 会话，e 保存会话创建或事务提交的错误。
	sess, e := s.Client.StartSession()
	if e != nil {
		return e
	}
	defer sess.EndSession(context.Background())
	// tc 由驱动为当前事务回调注入，业务读写必须使用它以加入同一次事务。
	_, e = sess.WithTransaction(ctx, func(tc context.Context) (any, error) { return nil, fn(tc) }, options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))
	if e != nil {
		// se 识别驱动错误标签，以区分提交结果未知与可直接反馈的失败。
		var se mongo.ServerError
		// 网络、截止或提交结果未知时，调用方必须沿原操作身份查证。
		if errors.As(e, &se) && (se.HasErrorLabel("UnknownTransactionCommitResult") || se.HasErrorLabel("TransientTransactionError")) {
			return Fail(p.CodeOperationPending, "原操作提交待查证")
		}
		if mongo.IsTimeout(e) || mongo.IsNetworkError(e) {
			return Fail(p.CodeOperationPending, "原操作提交待查证")
		}
	}
	return e
}

// Role 按持久角色编号读取完整聚合，并应用读取期兼容核验。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，id 区分当前步骤指向的目标对象。
func (s *Store) Role(ctx context.Context, id string) (domain.Role, error) {
	return s.readRole(ctx, bson.M{"_id": id})
}

// readRole 使用调用方提供的授权查询条件，恢复绑定信任并补齐旧装备耐久。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，filter 记录筛选条件，使用MongoDB BSON 文档表示。
func (s *Store) readRole(ctx context.Context, filter bson.M) (domain.Role, error) {
	// r 保存从 BSON 解码且完成旧字段兼容处理的角色聚合。
	var r domain.Role
	// raw 是角色原始 BSON 文档，err 保存读取、结构检查或绑定恢复失败。
	raw, err := s.DB.Collection("roles").FindOne(ctx, filter).Raw()
	if err != nil {
		return r, err
	}
	if err = checkBindingDocument(raw); err != nil {
		return r, err
	}
	// 先检查 BSON 字段形状，再解码，避免非法旧字段被零值掩盖。
	err = bson.Unmarshal(raw, &r)
	if err == nil && (r.SettlementReceiptStartSeq < 0 || r.SettlementReceiptStartSeq > r.SourceSeq) {
		return r, Fail(p.CodeOperationConflict, "收益回执界限无效")
	}
	if err == nil {
		_, err = s.restoreBindingTrust(ctx, &r)
		r = hydrateRoleDurability(r, &s.Cfg.Fixture)
	}
	return r, err
}

// accountGuard 用匹配账号版本的真实写入与禁用、换代等并发操作形成写冲突。
// s 执行账号条件更新，ctx 关联调用者已有的事务及其截止，a 提供预期账号与授权代次。
func (s *Store) accountGuard(ctx context.Context, a Auth) error {
	// r 的匹配数用于确认账号授权仍有效，e 表示条件更新的数据库错误。
	r, e := s.DB.Collection("accounts").UpdateOne(ctx, accountGuardFilter(a), bson.M{"$inc": bson.M{"auth_guard_seq": 1}})
	if e != nil {
		return e
	}
	if r.MatchedCount != 1 {
		return Fail(p.CodeUnauthenticated, "账号授权失效")
	}
	return nil
}

// accountGuardFilter 给单独和合批的账号围栏提供同一组授权条件。
// a 提供账号主键和请求发起时的授权代次。
func accountGuardFilter(a Auth) bson.M {
	return bson.M{"_id": a.AccountID, "active": true, "auth_version": a.AuthVersion}
}

// fence 核验运行服和节点所有权，并为本次事务落下运行组与账号两道写围栏。
// s 提供当前运行与节点信息，ctx 关联业务事务，a 提供账号和其授权代次。
func (s *Store) fence(ctx context.Context, a Auth) error {
	// writes 是运行组与账号的两项条件更新，err 表示租约检查或写入计划构造失败。
	writes, err := s.fenceWrites(ctx, a)
	if err != nil {
		return err
	}
	return s.writeFence(ctx, writes)
}

// fenceWrites 读取当前运行授权和节点租约，构造待与业务变更一起提交的两道条件写。
// s 提供节点、运行服与数据库，ctx 关联当前事务，a 决定账号围栏和对应哈希组。
func (s *Store) fenceWrites(ctx context.Context, a Auth) ([]mongo.ClientBulkWrite, error) {
	runWrite, err := s.runFenceWrite(ctx, runGuardGroup(a.AccountID))
	if err != nil {
		return nil, err
	}
	// 原角色资产操作仍保留独立账号围栏，以及合批操作预留的容量。
	writes := make([]mongo.ClientBulkWrite, 2, 5)
	writes[0] = runWrite
	writes[1] = mongo.ClientBulkWrite{Database: s.DB.Name(), Collection: "accounts", Model: mongo.NewClientUpdateOneModel().
		SetFilter(accountGuardFilter(a)).SetUpdate(bson.M{"$inc": bson.M{"auth_guard_seq": 1}})}
	return writes, nil
}

// runFenceWrite 提取角色和运行服公共操作共用的节点租约校验及唯一分组写屏障。
func (s *Store) runFenceWrite(ctx context.Context, group uint64) (mongo.ClientBulkWrite, error) {
	// filter 选择仍在有效租约内、实例与运行代次都匹配的运行服状态。
	runMember := gameRunMembership{RunID: s.Cfg.RunID, Epoch: s.Epoch}
	filter := activeRunOwnership(s.Cfg.NodeID, s.Instance, runMember)
	filter["$expr"] = leaseFilter()
	// run 接收当前运行服的控制版本和租约时点。
	var run struct {
		// ControlVersion 记录控制版本，使用数值表示。
		ControlVersion int64 `bson:"control_version"`
		// LeaseUntil 记录租约有效截止时间，使用时间点表示。
		LeaseUntil time.Time `bson:"lease_until"`
	}
	// runFields 只请求围栏建立与租约类型核验所需的两个字段。
	runFields := options.FindOne().SetProjection(bson.M{"_id": 0, "control_version": 1, "lease_until": 1})
	// e 区分运行服已失权与数据库暂时无法完成核验。
	if e := s.DB.Collection("run_control").FindOne(ctx, filter, runFields).Decode(&run); e != nil {
		return mongo.ClientBulkWrite{}, authorityReadError(e, p.CodeRouteChanged, "运行所有权已失效")
	}
	// node 选择仍持有有效租约的当前游戏进程节点记录。
	node := nodeLeaseFilter(s.Cfg.NodeID, s.Instance, runMember)
	// e 区分节点租约已失效与查询失败。
	if e := s.DB.Collection("node_leases").FindOne(ctx, node, options.FindOne().SetProjection(bson.M{"_id": 1})).Err(); e != nil {
		return mongo.ClientBulkWrite{}, authorityReadError(e, p.CodeRouteChanged, "节点许可已失效")
	}
	// 围栏携带运行服代次和控制版本，防止旧游戏进程在所有权换代后通过。
	// guard 限定正在写入的组同时匹配运行实例、代次和控制版本。
	guard := bson.M{"_id": fmt.Sprintf("%d:%d", s.Cfg.RunID, group),
		"owner_instance": s.Instance, "run_epoch": s.Epoch, "control_version": run.ControlVersion}
	return mongo.ClientBulkWrite{Database: s.DB.Name(), Collection: "run_guards", Model: mongo.NewClientUpdateOneModel().
		SetFilter(guard).SetUpdate(bson.M{"$inc": bson.M{"seq": 1}})}, nil
}

// writeFence 在当前事务执行独立围栏写入，供其他资产和历史回执重放使用。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，writes 记录写入集合，使用客户端批量写入列表表示。
func (s *Store) writeFence(ctx context.Context, writes []mongo.ClientBulkWrite) error {
	// 在同一事务中仍执行两次条件写，用一个命令取得两项各自的匹配结果。
	// result 保存两项更新的独立匹配数，err 表示批量命令本身失败。
	result, err := s.Client.BulkWrite(ctx, writes, options.ClientBulkWrite().SetVerboseResults(true))
	if err != nil {
		return transactionBulkWriteError(err)
	}
	return verifyFenceWrites(result)
}

// verifyFenceWrites 逐项判定运行组和账号围栏，有一项未匹配就让事务回滚。
// result 的前两项结果按运行组、账号的命令顺序读取。
func verifyFenceWrites(result *mongo.ClientBulkWriteResult) error {
	if !result.Acknowledged || result.UpdateResults[0].MatchedCount != 1 {
		return Fail(p.CodeRouteChanged, "写屏障失效")
	}
	if result.UpdateResults[1].MatchedCount != 1 {
		return Fail(p.CodeUnauthenticated, "账号授权失效")
	}
	return nil
}

// 无匹配记录才证明权限失效；保留其他错误，供驱动与事务层判断是否重试。
// err 保存当前操作的失败原因，供调用方检查或返回，code 是确实查不到授权记录时返回给调用方的业务错误码，message 说明具体失效的运行或节点授权。
func authorityReadError(err error, code uint32, message string) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Fail(code, message)
	}
	return err
}

// checkRole 确认角色属于当前账号、游戏进程、运行服代次及角色会话。
// s 提供当前运行所有权，r 是已持久保存的角色，a 是该请求携带的账号和会话身份。
func (s *Store) checkRole(r domain.Role, a Auth) error {
	wrongRun := r.RunServerID != s.Cfg.RunID && (r.RunServerID != 0 || !s.acceptsLegacyRoleScope())
	if wrongRun || r.AccountID != a.AccountID || r.Owner != s.Instance || r.RunEpoch != s.Epoch || r.SessionEpoch != a.SessionEpoch {
		return Fail(p.CodeStaleSession, "角色绑定已失效")
	}
	return nil
}

// acceptsLegacyRoleScope 只允许旧单服存储接口继续核对尚无运行服字段的会话。
func (s *Store) acceptsLegacyRoleScope() bool {
	return !s.borrowedRunView && len(s.gameMemberships) == 0 && len(s.Cfg.RunServerIDs) <= 1
}

// replace 使用原版本和所有权条件替换角色聚合，防止覆盖更新过的角色。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，r 记录角色，oldRevision 记录原状态修订号，使用数值表示。
func (s *Store) replace(ctx context.Context, r domain.Role, oldRevision int64) error {
	if err := validateDungeonHireWallet(r); err != nil {
		return err
	}
	// res 的匹配数用于确认角色及原版本仍符合替换条件，e 保存数据库写入错误。
	res, e := s.DB.Collection("roles").ReplaceOne(ctx, roleReplaceFilter(r, oldRevision), r)
	if e != nil {
		return e
	}
	if res.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "角色版本变化")
	}
	return nil
}

// roleReplaceFilter 为普通替换与批量结算统一比较角色键、版本和两个代次。
// r 提供角色键和要保留的所有者代次，oldRevision 是业务计算前读到的角色版本。
func roleReplaceFilter(r domain.Role, oldRevision int64) bson.M {
	filter := bson.M{"_id": r.ID, "revision": oldRevision, "owner_instance": r.Owner, "run_epoch": r.RunEpoch, "session_epoch": r.SessionEpoch}
	if r.RunServerID != 0 {
		filter["run_server_id"] = r.RunServerID
	}
	return filter
}
