// 本文件定义账号、角色、物品、钱包和托管战报的公共数据，并组装对外协议视图。

package domain

import (
	"crypto/rand"
	"encoding/hex"
	pb "minimir/api/generated/minimirv1"
	"time"
)

// ID 返回标识供调用方使用。
func ID() string {
	// b 创建用于维护b的容器或通道。
	b := make([]byte, 16)
	// e 接收读取供后续步骤使用。
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}

// Bytes 返回字节供调用方使用。
// id 区分当前步骤指向的目标对象，b 保存由解码文本取得的b。
func Bytes(id string) []byte { b, _ := hex.DecodeString(id); return b }

// Hex 返回十六进制供调用方使用。
// b 携带供当前流程读取或解析的原始字节。
func Hex(b []byte) string { return hex.EncodeToString(b) }

// Account 保存账号所需的状态。
type Account struct {
	ID           string `bson:"_id" json:"account_id"`   // 持久化账号标识。
	Name         string `bson:"name" json:"account"`     // 登录时使用的账号名。
	Salt         []byte `bson:"salt" json:"-"`           // 存储口令摘要所用的随机盐。
	PasswordHash []byte `bson:"password_hash" json:"-"`  // Argon2id 口令摘要。
	Active       bool   `bson:"active" json:"-"`         // 账号能否继续认证及发起业务。
	AuthVersion  int64  `bson:"auth_version" json:"-"`   // 撤销旧凭据时递增的账号授权版本。
	AuthGuard    int64  `bson:"auth_guard_seq" json:"-"` // 资产事务实际更新的授权屏障序号。
}

// Wallet 保存钱包所需的状态。
type Wallet struct {
	Gold         int64 `bson:"gold" json:"gold"`                               // 非绑定金币余额。
	BoundGold    int64 `bson:"bound_gold" json:"bound_gold"`                   // 绑定金币余额。
	Yuanbao      int64 `bson:"yuanbao_units" json:"yuanbao_units"`             // 以整数最小单位存储的非绑定元宝。
	BoundYuanbao int64 `bson:"bound_yuanbao_units" json:"bound_yuanbao_units"` // 以整数最小单位存储的绑定元宝。
}

// Proto 返回协议供调用方使用。
// w 是调用该方法的钱包实例，rev 记录修订号，使用数值表示。
func (w Wallet) Proto(rev int64) *pb.WalletState {
	return &pb.WalletState{Gold: w.Gold, BoundGold: w.BoundGold, YuanbaoUnits: w.Yuanbao, BoundYuanbaoUnits: w.BoundYuanbao, Revision: uint64(rev)}
}

// Item 保存道具所需的状态。
type Item struct {
	ID                  string `bson:"instance_id" json:"instance_id"`                                         // 同一配置可对应多件物品，本字段唯一标识其中一个实例。
	ItemID              uint32 `bson:"item_id" json:"item_id"`                                                 // 配置目录中的物品定义号。
	Quantity            int64  `bson:"quantity" json:"quantity"`                                               // 当前堆叠数量。
	Bound               bool   `bson:"bound" json:"bound"`                                                     // 限制转移并决定回收收入的绑定状态。
	Version             int64  `bson:"version" json:"version"`                                                 // 该实例的乐观并发版本。
	DurabilityU         int64  `bson:"durability_u,omitempty" json:"durability_u,omitempty"`                   // 当前耐久，使用约定的定点存储单位。
	MaxDurabilityU      int64  `bson:"max_durability_u,omitempty" json:"max_durability_u,omitempty"`           // 可经普通修理降低的当前耐久上限。
	OriginalDurabilityU int64  `bson:"original_durability_u,omitempty" json:"original_durability_u,omitempty"` // 出厂时的耐久上限。
	SourceKind          string `bson:"source_kind,omitempty" json:"source_kind"`                               // 生成此物品的业务类别。
	SourceID            string `bson:"source_id,omitempty" json:"source_id"`                                   // 具体的来源流水或操作标识。
	Quality             uint32 `bson:"quality,omitempty" json:"quality"`                                       // 配置允许的品质档。
	Locked              bool   `bson:"locked,omitempty" json:"locked"`                                         // 锁定时禁止相应资产流转。
	PendingResultID     string `bson:"pending_result_id,omitempty" json:"-"`                                   // 待确认结果占用该物品时记录的操作标识。
	ReservationID       string `bson:"reservation_id,omitempty" json:"-"`                                      // 当前预留此物品的业务标识。
}

// Proto 返回协议供调用方使用。
// i 是调用该方法的道具实例。
func (i Item) Proto() *pb.ItemView {
	// wire 保存当前处理所用的线协议。
	wire := &pb.ItemView{InstanceId: Bytes(i.ID), ItemId: i.ItemID, Quantity: i.Quantity, Bound: i.Bound, Version: uint64(i.Version), SourceKind: i.SourceKind, SourceId: i.SourceID, Quality: i.Quality, Locked: i.Locked, RecycleBlockFlags: i.RecycleBlockFlags()}
	if i.HasValidDurability() {
		wire.DurabilityU = uint64(i.DurabilityU)
		wire.MaxDurabilityU = uint64(i.MaxDurabilityU)
		wire.OriginalDurabilityU = uint64(i.OriginalDurabilityU)
	}
	return wire
}

// HasValidDurability 判断是否具备有效耐久。
// i 是调用该方法的道具实例。
func (i Item) HasValidDurability() bool {
	return i.OriginalDurabilityU > 0 && i.MaxDurabilityU > 0 && i.MaxDurabilityU <= i.OriginalDurabilityU && i.DurabilityU >= 0 && i.DurabilityU <= i.MaxDurabilityU
}

// Equipped 保存已穿戴装备所需的状态。
type Equipped struct {
	Slot uint32 `bson:"slot" json:"slot"` // 装备所在的部位号。
	Item Item   `bson:"item" json:"item"` // 该部位当前持有的物品实例。
}

// Report 保存报告或战报所需的状态。
type Report struct {
	SessionID          string `bson:"session_id" json:"hosting_session_id"`           // 本战报归属的离线托管会话。
	UsedSeconds        int64  `bson:"used_seconds" json:"used_seconds"`               // 已经持久确认的计费秒数。
	Kills              int64  `bson:"kills" json:"kills"`                             // 本会话累计确认的击杀。
	Experience         int64  `bson:"experience" json:"experience"`                   // 按成长规则实际接受的经验。
	RawExperience      int64  `bson:"raw_experience" json:"raw_experience"`           // 套用等级和经验池上限前的原始经验。
	RejectedExperience int64  `bson:"rejected_experience" json:"rejected_experience"` // 因成长上限未能接受的经验。
	PooledExperience   int64  `bson:"pooled_experience" json:"pooled_experience"`     // 成功进入待用经验池的量。
	Earned             Wallet `bson:"earned" json:"earned"`                           // 本会话累计到账的货币。
	Drops              []Item `bson:"drops" json:"drops"`                             // 供界面展示的有限明细。
	StopReason         uint32 `bson:"stop_reason" json:"stop_reason"`                 // 本次停托管的业务原因码。
	Sequence           int64  `bson:"sequence" json:"settlement_sequence"`            // 本战报下的确认序号。
	RecycleCount       int64  `bson:"recycle_count" json:"recycle_count"`             // 已自动回收的物品条目数。
	RecycleGold        int64  `bson:"recycle_gold" json:"recycle_gold"`               // 自动回收到账的非绑定金币。
	RecycleBoundGold   int64  `bson:"recycle_bound_gold" json:"recycle_bound_gold"`   // 自动回收到账的绑定金币。
	DiscardCount       int64  `bson:"discard_count" json:"discard_count"`             // 未入包的丢弃条目数。
	LastDiscardReason  string `bson:"last_discard_reason" json:"last_discard_reason"` // 最近一次未入包的原因。
	TotalDropCount     int64  `bson:"total_drop_count" json:"total_drop_count"`       // 实际入包总数，可大于保留明细数量。
}

// Role 保存角色所需的状态。
type Role struct {
	ID                           string            `bson:"_id" json:"role_id"`                                                             // 角色的持久身份。
	AccountID                    string            `bson:"account_id" json:"account_id"`                                                   // 拥有此角色的账号。
	Origin                       uint64            `bson:"origin_server_id" json:"origin_server_id"`                                       // 永久入口服编号。
	Slot                         uint32            `bson:"slot" json:"slot"`                                                               // 同一账号入口服内的角色槽。
	Job                          uint32            `bson:"job" json:"job"`                                                                 // 职业编号。
	Gender                       uint32            `bson:"gender" json:"gender"`                                                           // 角色外观性别编号。
	Name                         string            `bson:"name" json:"name"`                                                               // 展示给玩家的角色名。
	NameKey                      string            `bson:"name_key" json:"-"`                                                              // 用于去重和查找的规范化角色名。
	Level                        uint32            `bson:"level" json:"level"`                                                             // 当前永久等级。
	Experience                   int64             `bson:"experience" json:"experience"`                                                   // 当前成长版本下存留的等级经验。
	ProgressionFormatVersion     uint32            `bson:"progression_format_version,omitempty" json:"progression_format_version"`         // 角色数据采用的经验字段格式。
	ProgressionCurveVersion      string            `bson:"progression_curve_version,omitempty" json:"progression_curve_version"`           // 参与当前角色成长的曲线版本。
	RebirthCount                 uint32            `bson:"rebirth_count,omitempty" json:"rebirth_count"`                                   // 既有转生记录，按规则限定使用。
	PendingExperience            int64             `bson:"pending_experience,omitempty" json:"pending_experience"`                         // 无法立即用于升级而进入经验池的经验。
	LegacyExperienceOverflow     int64             `bson:"legacy_experience_overflow,omitempty" json:"legacy_experience_overflow"`         // 旧档迁移保留的历史溢出经验。
	ProgressionMigrationRequired bool              `bson:"progression_migration_required,omitempty" json:"progression_migration_required"` // 是否仍需执行版本化成长迁移。
	Revision                     int64             `bson:"revision" json:"revision"`                                                       // 角色聚合的持久版本，用于资产更新条件和客户端水位。
	RetiredDeathID               string            `bson:"retired_death_id,omitempty" json:"-"`                                            // 房间回收后仍待处理的原死亡身份。
	EconomicSeq                  int64             `bson:"economic_command_seq" json:"economic_command_seq"`                               // 玩家主动经济命令最后确认的序号。
	SourceSeq                    int64             `bson:"source_seq" json:"source_seq"`                                                   // 服务端来源事件最后确认的序号。
	SettlementReceiptStartSeq    int64             `bson:"settlement_receipt_start_seq,omitempty" json:"-"`                                // 开始要求永久收益回执的最早来源序号。
	Owner                        string            `bson:"owner_instance" json:"-"`                                                        // 获准写入此角色聚合的游戏节点实例。
	RunServerID                  uint64            `bson:"run_server_id,omitempty" json:"-"`                                               // 当前角色会话所属运行服，可在新连接接管时变更。
	RunEpoch                     int64             `bson:"run_epoch" json:"-"`                                                             // 游戏运行所有权代次。
	SessionEpoch                 int64             `bson:"session_epoch" json:"binding_epoch"`                                             // 同一角色换连接递增的绑定代次。
	LastEntryAttempt             string            `bson:"last_entry_attempt,omitempty" json:"-"`                                          // 当前绑定由哪一次票据接管产生，原票据清理后仍可查证。
	Wallet                       Wallet            `bson:"wallet" json:"wallet"`                                                           // 四类持久货币余额。
	Items                        []Item            `bson:"items" json:"items"`                                                             // 当前背包的物品实例列表。
	Equipped                     []Equipped        `bson:"equipped" json:"equipped"`                                                       // 当前装备栏的部位与物品。
	Capacity                     int               `bson:"capacity" json:"capacity"`                                                       // 背包允许的实例槽数量。
	PickupStopped                bool              `bson:"pickup_stopped" json:"pickup_stopped"`                                           // 上次入包失败后保留的拾取停止标记。
	AutoRecycle                  AutoRecyclePolicy `bson:"auto_recycle" json:"auto_recycle"`                                               // 玩家最后确认的自动回收设置。
	LastDiscardItem              uint32            `bson:"last_discard_item_id" json:"last_discard_item_id"`                               // 最近丢弃掉落的配置编号。
	LastDiscardBound             bool              `bson:"last_discard_bound" json:"last_discard_bound"`                                   // 最近丢弃掉落原本的绑定状态。
	HostingSeconds               int64             `bson:"hosting_seconds" json:"hosting_seconds"`                                         // 当前允许继续离线托管的剩余整秒。
	ClaimDay                     string            `bson:"claim_day" json:"-"`                                                             // 最近记录领取额度的业务日。
	ClaimedSeconds               int64             `bson:"claimed_seconds" json:"-"`                                                       // 该业务日已领取的托管秒数。
	NoEquipmentKills             int               `bson:"no_equipment_kills" json:"-"`                                                    // 为装备掉落保底跟踪的连续未掉装备次数。
	PrivilegeUntil               time.Time         `bson:"privilege_until" json:"-"`                                                       // 角色特权的到期时刻。
	HostingCardUntil             time.Time         `bson:"hosting_card_until" json:"-"`                                                    // 角色托管卡权益的到期时刻。
	Report                       Report            `bson:"hosting_report" json:"hosting_report"`                                           // 最近一段已确认的离线托管战报。
	Skills                       []LearnedSkill    `bson:"learned_skills" json:"learned_skills"`                                           // 已学习技能及其当前等级。
	Binding                      *RoleBinding      `bson:"binding,omitempty" json:"binding,omitempty"`                                     // 已持久确认的安全点，不随当前位置更新。
	BindingSourceSeq             int64             `bson:"binding_source_seq,omitempty" json:"-"`                                          // 最后一次确认绑定的服务端来源序号。
	BindingSafeZoneID            uint64            `bson:"-" json:"-"`                                                                     // 读取时核对出的可信安全区号，由配置推导。
}

// 绑定是永久重入业务事实，不是当前地图位置；移动、重连和故障重建不得覆盖它。
type RoleBinding struct {
	MapID uint64 `bson:"map_id" json:"map_id"` // 绑定点所在地图的配置编号。
	X     int32  `bson:"x" json:"x"`           // 绑定点在该地图的横向格坐标。
	Y     int32  `bson:"y" json:"y"`           // 绑定点在该地图的纵向格坐标。
}

// LearnedSkill 保存已学习技能所需的状态。
type LearnedSkill struct {
	SkillID uint32 `bson:"skill_id" json:"skill_id"` // 已经习得的技能定义号。
	Level   uint32 `bson:"level" json:"level"`       // 该技能当前确认的等级。
}

// Skill 处理技能。
// r 是调用该方法的角色实例，id 区分当前步骤指向的目标对象。
func (r Role) Skill(id uint32) (uint32, bool) {
	// s 接收遍历技能集合时当前的元素。
	for _, s := range r.Skills {
		if s.SkillID == id {
			return s.Level, true
		}
	}
	return 0, false
}

// Summary 返回“summary”供调用方使用。
// r 是调用该方法的角色实例。
func (r Role) Summary() *pb.RoleSummary {
	return &pb.RoleSummary{Key: &pb.RoleKey{RoleId: Bytes(r.ID), OriginServerId: r.Origin}, Name: r.Name, Job: r.Job, Gender: r.Gender, Level: r.Level, Slot: r.Slot, AppearanceVersion: 1}
}

// Inventory 返回背包资产供调用方使用。
// r 是调用该方法的角色实例，progression 记录成长，使用可变数量的成长状态表示。
func (r Role) Inventory(progression ...*pb.ProgressionState) *pb.InventoryResult {
	// p 保存当前处理所用的背包资产结果。
	p := &pb.InventoryResult{Capacity: uint32(r.Capacity), Revision: uint64(r.Revision), Level: r.Level, Experience: r.Experience, AutoRecycle: r.AutoRecycle.Proto()}
	if len(progression) > 0 {
		p.Progression = progression[0]
	}
	// i 接收遍历道具集合时当前的元素。
	for _, i := range r.Items {
		// v 接收协议供后续步骤使用。
		v := i.Proto()
		v.RecycleBlockFlags = r.RecycleBlockFlags(i)
		p.Items = append(p.Items, v)
	}
	// i 接收遍历已穿戴装备时当前的元素。
	for _, i := range r.Equipped {
		// v 接收协议供后续步骤使用。
		v := i.Item.Proto()
		v.RecycleBlockFlags |= RecycleBlockWorn
		p.Equipped = append(p.Equipped, &pb.EquippedItem{Slot: i.Slot, Item: v})
	}
	// s 接收遍历技能集合时当前的元素。
	for _, s := range r.Skills {
		p.LearnedSkills = append(p.LearnedSkills, &pb.LearnedSkill{SkillId: s.SkillID, Level: s.Level})
	}
	return p
}

// HostingReport 返回离线托管报告或战报供调用方使用。
// r 是调用该方法的角色实例。
func (r Role) HostingReport() *pb.HostingReport {
	// p 保存当前处理所用的离线托管报告或战报。
	p := &pb.HostingReport{HostingSessionId: Bytes(r.Report.SessionID), UsedSeconds: uint64(r.Report.UsedSeconds), Kills: uint64(r.Report.Kills), Experience: r.Report.Experience, Earned: r.Report.Earned.Proto(r.Report.Sequence), StopReason: r.Report.StopReason, SettlementSequence: uint64(r.Report.Sequence), RecycleCount: uint64(r.Report.RecycleCount), RecycleGold: r.Report.RecycleGold, RecycleBoundGold: r.Report.RecycleBoundGold, DiscardCount: uint64(r.Report.DiscardCount), LastDiscardReason: r.Report.LastDiscardReason, TotalDropCount: uint64(r.Report.TotalDropCount)}
	p.RawExperience = r.Report.RawExperience
	p.RejectedExperience = r.Report.RejectedExperience
	p.PooledExperience = r.Report.PooledExperience
	// i 接收遍历掉落集合时当前的元素。
	for _, i := range r.Report.Drops {
		p.Drops = append(p.Drops, i.Proto())
	}
	return p
}
