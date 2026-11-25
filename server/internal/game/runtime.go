// 本文件装配游戏引擎与房间分片，路由连接和指令，管理入场、场景基线和操作分发。
package game

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/townnav"
	"minimir/internal/wire"
	"sync"
	"sync/atomic"
	"time"
)

// Connection 定义房间可使用的 WebSocket 附着、收发和会话状态接口。
type Connection interface {
	Alive() bool             // 报告连接是否仍能处理当前请求或接收推送。
	Deliver(wire.Frame) bool // 尝试将完整应用帧移交给连接的发送队列。
	Close(string)            // 按服务端原因关闭连接，并触发对应的断连清理。
	SetRole(string)          // 在查到有效票据角色后写入连接路由键。
	RoleID() string          // 返回连接当前路由的角色编号。
	SetBinding(uint64)       // 在入场事务确认后写入新会话代次。
	Binding() uint64         // 读取会话代次，用来拦截旧连接或旧帧。
}

// command 表示应该交给单个房间分片串行处理的输入或控制事件。
type command struct {
	role        string              // 房间分片内的角色索引键。
	conn        Connection          // 命令来源连接，后台时钟及内部回调可不带连接。
	frame       wire.Frame          // 原始请求帧，保留消息号、请求号和绑定代次。
	ticket      *persistence.Ticket // 入场命令携带已校验的短票，待确认时按原身份查证。
	runStore    *persistence.Store  // 首次查票后选定的运行服持久层，等待和重试时保留。
	done        func(*worker)       // 在房间分片 goroutine 上执行的内部工作。
	tick        time.Time           // 发出房间逻辑帧时采样的服务器时刻。
	at          time.Time           // 记录断开发生时刻，标定在线和托管事件的归属边界。
	disconnect  bool                // 把这条控制命令解释为对应连接的断开。
	fromWaiting bool                // 该命令从房间等待队列重放时为真。
}

// job 表示数据库协程同房间分片之间的异步提交与回收约定。
type job struct {
	run        func() func(*worker) // 在数据库执行协程中运行；返回的函数只在目标分片应用结果。
	target     *worker              // 拥有该房间并接收结果的分片。
	settlement bool                 // 将收益等持久结算任务路由到独立等待队列。
	admission  bool                 // 将会话接管及其重试路由到专用数据库队列。
}

// Engine 持有游戏分片和数据库队列，负责路由协议及管理运行周期。
type Engine struct {
	Store            *persistence.Store            // 共享规则配置及安装范围的票据预查入口。
	runStores        map[uint64]*persistence.Store // 每个获准运行服的独立围栏与代次，启动后只读。
	workers          []*worker                     // 所有房间分片，按角色散列映射保持操作顺序。
	jobs             chan job                      // 普通业务数据库任务的有界队列。
	settlementJobs   chan job                      // 收益与同类持久结算任务的独立等待队列。
	admissionTickets chan job                      // 入场第一阶段的短票角色查询队列。
	admissionEntries chan job                      // 入场第二阶段的持久会话接管队列。
	ctx              context.Context               // 引擎生命周期，取消后停止推进和投递任务。
	cancel           context.CancelFunc            // 通知分片、时钟和数据库执行协程退出。
	wg               sync.WaitGroup                // 等待引擎持有的后台协程完成。
	baseline         atomic.Uint64                 // 跨分片分配客户端场景基线编号。
	stopped          atomic.Bool                   // 表示已冻结新增游戏操作，停服时仍可处理已受理回执。
	TownGrids        map[uint64]*townnav.Grid      // 按城镇地图编号保存可共用的导航静态网格。
	sharedTowns      sharedTownDirectory           // 当前引擎每个运行服的共享城镇席位与动态占格。
}

// workerCount 固定角色散列的分片数量。
const workerCount = 8

// maxRoomsPerWorker 约束正式开发装配中每个分片保留的房间数量。
const maxRoomsPerWorker = 256

// worker 独占所属角色房间的状态修改权，避免网络及数据库协程并发改写房间。
type worker struct {
	e                  *Engine                     // 当前分片所属的引擎。
	commands           chan command                // 接收游戏指令、入场请求和逻辑帧。
	control            chan command                // 优先独立接收断连等控制命令。
	done               chan func(*worker)          // 接收数据库结果，在分片内串行应用。
	rooms              map[string]*room            // 该分片持有的角色房间，只有其事件循环改写。
	roll               func(int64) int64           // 供测试注入可复现掷点的函数。
	townNavigation     *townNavigationWorker       // 该分片的城镇寻路队列、预算与工作区。
	personalNavigation personalNavigationWorkspace // 可在分片内跨个人房间复用的寻路数组。
	worldBossArenas    map[uint64]*worldBossArena  // 本分片唯一拥有的运行服共享首领场次。
}

// entity 保存当前进程中的战斗身体、动作时钟和空间身份。
type entity struct {
	Handle               uint32                   // 在场景实体集内引用该身体的句柄。
	Kind                 uint32                   // 区分角色、宠物、怪物与场景对象的类别。
	Def                  uint32                   // 对应配置中的实体定义编号。
	Pos                  devconfig.Point          // 地图逻辑网格位置。
	Direction            uint32                   // 当前朝向的八方向编号。
	HP                   int64                    // 当前生命值，归零表示死亡。
	MP                   int64                    // 当前魔法值，供施法和恢复逻辑使用。
	MaxMP                int64                    // 魔法上限，恢复时用来裁剪。
	MaxHP                int64                    // 生命上限，恢复或复活时用来裁剪。
	Poisons              *monsterPoisons          // 只在原生命受到施毒时分配，记录当前红绿毒。
	Action               uint32                   // 向客户端观察的当前动作类型。
	AttackSequence       uint32                   // 观察端用来区别相邻攻击动作的单调序号。
	AttackSkillID        uint32                   // 这次攻击动作使用的技能定义编号。
	AttackSkillLevel     uint32                   // 这次攻击动作的技能等级。
	ActionUntil          time.Time                // 当前动作表现到期的时刻。
	NextMove             time.Time                // 下一次允许行走的独立时刻。
	NextAttack           time.Time                // 下一次允许发起攻击的独立时刻。
	ActionReadyAt        time.Time                // 当前行动锁结束的时刻。
	RespawnAt            time.Time                // 死亡怪物允许尝试复活的时刻。
	Monster              *devconfig.Monster       // 怪物或宠物采用的配置数值。
	CastSequence         uint32                   // 观察端用来区别施法动作的单调序号。
	CastSkillID          uint32                   // 这次施法动作使用的技能编号。
	Life                 uint64                   // 当前出生代次，阻止旧事件误命中新生身体。
	HealAmount           int64                    // 尚未按治疗时钟消化的技能恢复量。
	NextHeal             time.Time                // 下一次技能治疗量生效的时刻。
	Statuses             map[uint32]*combatStatus // 按效果编号保存尚生效的战斗状态。
	MovementRevision     uint64                   // 位置变化的单调修订号，区分强制位移和正常行走。
	MovementKind         uint32                   // 上次位置变化的类型。
	CellEntry            uint64                   // 此次进入逻辑格的唯一序号。
	PetRuntime           *petRuntimeState         // 宠物出生后才有的运行配置与成长状态。
	IsPetCorpse          bool                     // 表示实体仅作为已归档的宠物尸体显示。
	CapturedUntilRespawn bool                     // 原怪物已转为宠物，此占位只等待普通刷新，不展示尸体。
	ShockUntilMS         int64                    // 怪物停止主动移动、攻击的 Unix 毫秒截止时间。
}

// room 汇总角色资产的已确认视图与活动场景，分片循环统一推进所有临时状态。
type room struct {
	*worldState                                     // 当前活动世界；匿名嵌入让常用战斗代码直接访问场景状态。
	runStore              *persistence.Store        // 房间创建时选定的运行服视图，在整个房间生命周期中固定。
	PersonalWorld         *worldState               // 此角色独有的挂机世界，切到城镇时仍可保留原现场。
	TownWorld             *worldState               // 当前持有的城镇世界引用。
	TownWorlds            []*worldState             // 已创建的其他城镇现场，用于按地图查找和保留事件归属。
	townMembership        townMembership            // 角色当前的共享城镇席位，由所属分片管理。
	personalBoss          *personalBossRun          // 永久角色房间正在批准、观察或结算的独享挑战。
	bossBattle            *personalBossBattle       // 独享挑战子房间只持有临时战斗能力，不能直接写永久数据。
	worldBossObservation  *worldBossObservation     // 原角色控制分片上的共享场次观察身份。
	worldBossResume       worldBossOccurrence       // 健康掉线或会话接管时原场次的接回提示。
	worldBossCombat       *worldBossArena           // 仅场次分片的临时战斗角色保存此共享实体空间。
	ExecutionEpoch        uint64                    // 此房间在当前进程中的唯一执行代次。
	WorldSequence         uint64                    // 从同一执行代次分配世界编号的水位。
	InflightEvents        int                       // 已移入到期处理批次的事件数量。
	EventBatch            []dueEvent                // 本轮从各世界取出的待执行事件列表。
	AdvancingEvents       bool                      // 标记正在推进统一事件批次，避免中途重入。
	FactOrder             uint64                    // 来源事实的房间顺序水位，协调绑定和击杀。
	BindingCandidate      *safeZoneBindingFact      // 尚待占用来源流水的安全区绑定事实。
	SettlementGeneration  uint64                    // 收益结算回调的房间内代次。
	AssetGeneration       uint64                    // 经济指令回调的房间内代次。
	Role                  domain.Role               // 最近通过持久层确认的角色资产和业务水位。
	Auth                  persistence.Auth          // 当前角色拥有者、账号和会话代次的授权信息。
	Conn                  Connection                // 当前附着连接；为空时按离线现场处理。
	Ready                 bool                      // 当前连接是否已确认正确场景基线。
	PetBornReady          *petBornAttachment        // 宠物新生事实所需的已确认观察连接身份。
	Travel                *mapTravelPlan            // 当前或最近一次的换图计划及终态。
	TravelHistory         *mapTravelHistory         // 在当前会话和场景内拒绝旧换图编号的已接受历史。
	SavedPersonal         *personalWorld            // 切往城镇时停驻的个人现场标记。
	TownNavigation        [2]*townNavigationRequest // 玩家和宠物各自的城镇寻路引用。
	Scene                 uint64                    // 客户端观察的场景代次，与内部世界执行身份分开。
	Baseline              uint64                    // 当前连接等待确认的场景基线编号。
	Sequence              uint64                    // 场景推送状态的单调编号。
	Player                entity                    // 该房间中唯一的角色身体。
	Pet                   *entity                   // 角色目前拥有的宠物身体。
	PetWorld              worldRef                  // 宠物实际驻留世界的执行身份。
	PetLifeSequence       uint64                    // 分配宠物出生代次的单调水位。
	PetHandleSequence     uint32                    // 分配宠物实体句柄的水位。
	PendingPetBirth       *petBirthReservation      // 已接受但尚未完成的宠物出生预约。
	Manual                bool                      // 摇杆手动移动是否仍在持续。
	ManualDirection       uint32                    // 当前手动移动方向。
	AutoSkills            *autoSkillSettings        // 角色对自动释放技能的独立偏好。
	AutoTarget            actorRef                  // 当前自动战斗锁定的怪物生命引用。
	Pending               bool                      // 房间存在需要冻结推进的在途持久操作。
	PendingRetry          *job                      // 结果未知或排队失败后需原样重投的数据库任务。
	entryAttempt          string                    // 待确认的原入场尝试，期间冻结旧连接的游戏输入。
	PendingBinding        *safeZoneBindingFact      // 已作为当前在途操作提交的安全区绑定事实。
	RetryAt               time.Time                 // 下一次允许重投在途任务的时刻。
	Waiting               []command                 // 在途操作结束后准备重放的有限命令队列。
	LastTick              time.Time                 // 上次处理的服务器逻辑帧时刻，用于计算定步计量。
	idleSince             time.Time                 // 首次满足释放条件的时刻，健康回收窗口从这里计起。
	HostMS                int64                     // 健康owner累计的未满秒托管余量，跨连接、地图和生命保留。
	hostSecondsOpen       int64                     // 当前会话已经运行、尚未绑定结算流水的整秒。
	HostSession           string                    // 当前连续离线托管的会话编号。
	DeathID               string                    // 当前死亡待选分支的唯一编号。
	AwaitUntil            time.Time                 // 当前附着等待首次场景 ACK 的截止时刻。
	LastBroadcast         time.Time                 // 最近一次场景快照发送时刻。
	Potions               potionState               // 药水恢复池、冷却和回执水位。
	Wear                  equipmentWearState        // 待确认及新积累的装备耐久磨损。
	PendingOperationID    string                    // 当前在途操作的原始编号。
	PendingFingerprint    string                    // 当前操作原业务内容的指纹，检测错配回执和重试。
	InputSequence         uint32                    // 当前附着已接收的移动与施法共用输入水位。
	NextSpell             time.Time                 // 玩家下一次允许施法的时刻。
	SkillCooldowns        map[uint32]time.Time      // 按技能编号保存的各自冷却时刻。
	CastID                uint64                    // 分配施法事件编号的水位。
	EventID               uint64                    // 分配战斗观察事件编号的水位。
	ScheduleID            uint64                    // 分配统一战斗事件排序号的水位。
	KillFacts             []killFact                // 尚未以来源事务确认的击杀事实，保持原身份和随机结果。
	PreviewInFlight       bool                      // 本房间是否已有只读预览任务在途。
	PreviewGeneration     uint64                    // 只读预览回调的代次，用于使旧结果失效。
	Isolated              bool                      // 出现无法安全继续的授权或身份问题时冻结房间。
	MaterialEconomicSeq   int64                     // 已应用材料技能回执的经济指令水位。
	MaterialSourceSeq     int64                     // 已应用材料技能回执的来源水位。
	MaterialGeneration    uint64                    // 材料技能回调的房间内代次。
	Melee                 meleeState                // 普攻模式、蓄力和修订状态。
	CellEntrySequence     uint64                    // 分配角色及实体格进入身份的单调水位。
	DeferMovementSnapshot bool                      // 暂缓中途位移快照，留待同一组合动作完整提交后再观察。
}

// New 为单运行服启动持有共享分片、数据库队列与逻辑时钟的引擎。
// ctx 控制引擎生命周期，s 提供该运行服的持久化视图。
func New(ctx context.Context, s *persistence.Store) *Engine {
	return NewRunEngine(ctx, []*persistence.Store{s})
}

// newEngine 启动一套供同一游戏进程内所有运行服共享的执行资源。
// ctx 控制生命周期，s 用于共享规则与票据预查，runs 保留所有获准使用的运行服视图。
func newEngine(ctx context.Context, s *persistence.Store, runs map[uint64]*persistence.Store) *Engine {
	// cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancel(ctx)
	// e 保存当前处理所用的游戏引擎。
	e := &Engine{
		Store: s, runStores: runs, ctx: ctx, cancel: cancel,
		jobs:             make(chan job, 256),
		settlementJobs:   make(chan job, workerCount*maxRoomsPerWorker),
		admissionTickets: make(chan job, 128),
		admissionEntries: make(chan job, 128),
	}
	// i 标记循环或列表中的当前位置。
	for i := 0; i < workerCount; i++ {
		// w 保存当前处理所用的房间执行器。
		w := &worker{e: e, commands: make(chan command, 256), control: make(chan command, 64), done: make(chan func(*worker), 256), rooms: map[string]*room{}}
		e.workers = append(e.workers, w)
	}
	// err 接收城镇导航静态网格装配失败的原因。
	if err := e.initializeTownNavigation(); err != nil {
		cancel()
		panic("城镇导航准入初始化失败：" + err.Error())
	}
	// w 接收遍历房间执行器集合时当前的元素。
	for _, w := range e.workers {
		e.wg.Go(w.loop)
	}
	e.startDatabaseWorkers()
	e.wg.Go(func() {
		// 逻辑帧以同一采样时钟发给所有分片；满队列时跳过投递，避免时钟阻塞其它分片。
		// ticker 按 100 毫秒节拍采样同一时刻并分发逻辑帧。
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			// now 接收c通道送来的当前时间。
			case now := <-ticker.C:
				// w 接收遍历房间执行器集合时当前的元素。
				for _, w := range e.workers {
					select {
					case w.commands <- command{tick: now}:
					default:
					}
				}
			}
		}
	})
	return e
}

// worker 由角色编号计算稳定的所有分片，同一角色的操作始终进入原分片。
// e 是调用该方法的游戏引擎实例，id 区分当前步骤指向的目标对象。
func (e *Engine) worker(id string) *worker {
	// raw 保存由字节取得的原始值。
	raw := domain.Bytes(id)
	// h 是角色编号的 SHA-256 摘要；前八字节用于选择固定分片。
	h := sha256.Sum256(raw)
	return e.workers[binary.BigEndian.Uint64(h[:8])%workerCount]
}

// Enter 受理客户端短票，异步查出角色后将入场流程交给角色所属分片。
// e 是调用该方法的游戏引擎实例，token 记录令牌，使用文本表示，c 记录连接，request 关联客户端请求与服务端的对应响应。
func (e *Engine) Enter(token string, c Connection, request uint32) {
	if e.stopped.Load() {
		c.Close("maintenance")
		return
	}
	// j 初始化用作当前位置的下标的任务或职业编号。
	j := job{run: func() func(*worker) {
		if !c.Alive() {
			return nil
		}
		// t 保存由入场票据角色取得的t，err 接收入场票据角色过程中可能产生的错误，供后续失败分支处理。
		t, err := e.Store.TicketRole(e.ctx, token)
		if !c.Alive() {
			return nil
		}
		if err != nil {
			rejectEntry(c, request, persistence.Code(err), "invalid ticket")
			return nil
		}
		// runStore 取自批准的运行服表；非本节点或过期代次的票据不能进入房间分片。
		runStore := e.storeForTicket(t)
		if runStore == nil {
			rejectEntry(c, request, p.CodeRouteChanged, "ticket run not hosted")
			return nil
		}
		c.SetRole(t.RoleID)
		e.postEntry(e.worker(t.RoleID), command{role: t.RoleID, conn: c, ticket: &t, runStore: runStore, frame: wire.Frame{Request: request}})
		return nil
	}}
	if !offerDatabaseJob(e.admissionTickets, j) {
		rejectEntry(c, request, p.CodeServerBusy, "admission ticket queue full")
	}
}

// rejectEntry 先尽力同步原请求错误，再关闭入场失败的连接。
// c 记录连接，request 关联客户端请求与服务端的对应响应，code 记录结果码，使用数值表示，reason 记录原因，使用文本表示。
func rejectEntry(c Connection, request, code uint32, reason string) {
	// f 组装协议帧所需的数据。
	f := wire.Frame{ID: p.MsgS2CEnterLine, Request: request, Error: code}
	// terminal 按目标类型读取当前接口值，得到终止，ok 表明运行时值能否按要求的类型读取。
	// DeliverAndClose 将原请求错误放入终止发送槽，并在发送后关闭连接。
	if terminal, ok := c.(interface{ DeliverAndClose(wire.Frame, string) }); ok {
		terminal.DeliverAndClose(f, reason)
		return
	}
	c.Deliver(f)
	c.Close(reason)
}

// post 无阻塞投递连接命令；队列拥堵时关闭相应连接，保护分片吞吐。
// e 是调用该方法的游戏引擎实例，w 记录房间执行器，c 记录命令。
func (e *Engine) post(w *worker, c command) bool {
	select {
	case w.commands <- c:
		return true
	default:
		if c.conn != nil {
			c.conn.Close("server busy")
		}
		return false
	}
}

// postEntry 按入场协议报告分片队列已满，使客户端得到明确拒绝原因。
// e 是调用该方法的游戏引擎实例，w 记录房间执行器，c 记录命令。
func (e *Engine) postEntry(w *worker, c command) {
	select {
	case w.commands <- c:
	default:
		rejectEntry(c.conn, c.frame.Request, p.CodeServerBusy, "admission command queue full")
	}
}

// Handle 把已知角色的应用帧交给分片，未入场或停服时立即拒绝。
// e 是调用该方法的游戏引擎实例，c 记录连接，f 记录协议帧。
func (e *Engine) Handle(c Connection, f wire.Frame) {
	if e.stopped.Load() {
		c.Close("maintenance")
		return
	}
	// id 区分当前步骤指向的目标对象。
	id := c.RoleID()
	if id == "" {
		c.Deliver(wire.Frame{ID: f.ID, Request: f.Request, Error: p.CodeUnauthenticated})
		return
	}
	e.post(e.worker(id), command{role: id, conn: c, frame: f})
}

// Disconnect 将连接退出排入独立控制队列，并保存断开时刻。
// e 是调用该方法的游戏引擎实例，c 记录连接。
func (e *Engine) Disconnect(c Connection) {
	// id 区分当前步骤指向的目标对象。
	if id := c.RoleID(); id != "" {
		select {
		case e.worker(id).control <- command{role: id, conn: c, disconnect: true, at: time.Now()}:
		case <-e.ctx.Done():
		}
	}
}

// Stop 冻结新操作，关停连接和后台协程，并兜底关闭尚残留的长连接。
// e 是调用该方法的游戏引擎实例。
func (e *Engine) Stop() {
	e.stopped.Store(true)
	// w 接收遍历房间执行器集合时当前的元素。
	for _, w := range e.workers {
		// w 记录房间执行器。
		e.post(w, command{done: func(w *worker) {
			// r 接收遍历房间集合时当前的元素。
			for _, r := range w.rooms {
				w.leaveSharedTown(r)
				w.cancelTownNavigation(r)
				w.cancelMapTravelOnDetach(r)
				if r.Conn != nil {
					r.Conn.Close("maintenance")
					r.Conn = nil
					r.PetBornReady = nil
				}
				r.Auto = false
			}
		}})
	}
	// deadline 是连接清理命令与待处理回调的短暂退出缓冲。
	deadline := time.NewTimer(500 * time.Millisecond)
	<-deadline.C
	e.cancel()
	e.wg.Wait()
	// 父context先取消时worker已退出，仍必须关闭已劫持的WS；此时遍历由Stop独占。
	// w 接收遍历房间执行器集合时当前的元素。
	for _, w := range e.workers {
		// r 接收遍历房间集合时当前的元素。
		for _, r := range w.rooms {
			w.leaveSharedTown(r)
			if r.Conn != nil {
				r.Conn.Close("maintenance")
				r.Conn = nil
				r.PetBornReady = nil
			}
		}
	}
}

// loop 串行处理数据库结果、连接控制、客户端命令和房间逻辑帧。
// w 是调用该方法的房间执行器实例。
func (w *worker) loop() {
	for {
		select {
		case <-w.e.ctx.Done():
			return
		// done 接收完成信号通道送来的完成信号。
		case done := <-w.done:
			done(w)
		// c 接收控制通道送来的控制通道。
		case c := <-w.control:
			w.command(c)
		// c 接收命令集合通道送来的命令集合通道。
		case c := <-w.commands:
			if c.done != nil {
				c.done(w)
				continue
			}
			if !c.tick.IsZero() {
				w.tickWorldBosses(c.tick)
				w.beginTownNavigationTick(c.tick)
				// roleID 定位房间集合中正在处理的成员，r 接收遍历房间集合时当前的元素。
				for roleID, r := range w.rooms {
					if r.Scene == 0 && !w.e.stopped.Load() {
						// 冷入场查证时还没有场景，但也需要继续重投原任务。
						w.retryPendingRoom(r, c.tick)
					}
					w.stepPersonalBoss(r, c.tick)
					w.step(r, c.tick)
					w.expireIdleRoom(roleID, r, c.tick)
				}
				w.endTownNavigationTick()
				continue
			}
			w.command(c)
		}
	}
}

// command 核对附着身份及待确认状态，并按消息类型触发房间内的相应处理。
// w 是调用该方法的房间执行器实例，c 记录命令。
func (w *worker) command(c command) {
	if w.e.stopped.Load() {
		if c.conn != nil && !c.disconnect {
			c.conn.Close("maintenance")
		}
		return
	}
	// r 保存从房间集合取出的房间集合中的指定项。
	r := w.rooms[c.role]
	if c.ticket != nil {
		// 同一角色保持原分片，其他运行服不能借用现有世界或待确认结算。
		if r != nil && r.runStore != c.runStore {
			rejectEntry(c.conn, c.frame.Request, p.CodeRouteChanged, "room belongs to another run")
			return
		}
		if r != nil && r.Isolated {
			rejectEntry(c.conn, c.frame.Request, p.CodeLineRecovering, "room authority isolated")
			return
		}
		if !c.conn.Alive() {
			return
		}
		if r != nil && r.worldBossObservation != nil {
			w.suspendWorldBossObservation(r)
		}
		if r != nil && r.personalBoss != nil {
			w.interruptPersonalBoss(r, time.Now())
			// 即使中断尚未拿到永久操作槽，也先等待同一场次收束，不能抢走原会话。
			if r.personalBoss != nil && !r.Pending {
				if len(r.Waiting) >= 2 {
					rejectEntry(c.conn, c.frame.Request, p.CodeLineRecovering, "challenge is closing")
				} else {
					r.Waiting = append(r.Waiting, c)
				}
				return
			}
		}
		if r != nil && w.queueEntryAfterSettlement(r, c) {
			return
		}
		if r == nil {
			if len(w.rooms) >= maxRoomsPerWorker {
				if !w.releaseIdleSlot(time.Now()) {
					rejectEntry(c.conn, c.frame.Request, p.CodeServerBusy, "development room budget")
					return
				}
			}
			r = &room{Role: domain.Role{ID: c.role}, worldState: &worldState{}, runStore: c.runStore}
			w.rooms[c.role] = r
		}
		w.join(r, c)
		return
	}
	if r == nil {
		return
	}
	if r.Isolated {
		if c.disconnect {
			if r.Conn == c.conn {
				r.Conn = nil
				r.PetBornReady = nil
			}
			return
		}
		if c.conn != nil {
			c.conn.Close("room authority isolated")
		}
		return
	}
	if c.disconnect {
		if r.Conn == c.conn {
			w.suspendWorldBossObservation(r)
			w.interruptPersonalBoss(r, time.Now())
			w.leaveSharedTown(r)
			w.cancelMapTravelOnDetach(r)
			w.cancelTownNavigation(r)
			r.PreviewGeneration++
			r.PreviewInFlight = false
			r.Conn = nil
			r.Ready = false
			r.PetBornReady = nil
			r.Manual = false
			r.LastTick = time.Now()
			r.HostSession = domain.ID()
			// at 保存当前处理所用的发生时间。
			at := c.at
			if at.IsZero() {
				at = time.Now()
			}
			// m 接收当前状态地图供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
			if m, ok := w.currentMap(r); ok && m.SceneKindValue() == devconfig.SceneKindTown {
				r.HostSession = ""
			}
			transitionEventHosting(r, at, r.HostSession)
		}
		return
	}
	if r.Conn != c.conn || c.frame.Binding != uint64(r.Auth.SessionEpoch) {
		if economic(c.frame.ID) {
			c.conn.Deliver(wire.Message(p.MsgS2COperationResult, c.frame.Request, c.conn.Binding(), &pb.OperationResult{OpId: operationID(c.frame), ErrorCode: p.CodeStaleSession, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)}))
		} else {
			c.conn.Deliver(wire.Frame{ID: c.frame.ID, Request: c.frame.Request, Binding: c.conn.Binding(), Error: p.CodeStaleSession})
		}
		return
	}
	if r.entryAttempt != "" {
		w.failure(r, c.frame, p.CodeLineRecovering)
		return
	}
	if w.worldBossCommand(r, c) {
		return
	}
	if w.personalBossCommand(r, c) {
		return
	}
	w.roomCommand(r, c)
}

// roomCommand 接收已经按角色附着和场景白名单分流的指令，普通和首领场景复用格子行动核心。
func (w *worker) roomCommand(r *room, c command) {
	if c.frame.ID == p.MsgC2SEnterWorldAck {
		if r.Ready {
			return
		}
		// q 记录入场场景世界确认。
		var q pb.EnterWorldAck
		if proto.Unmarshal(c.frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.BaselineId != r.Baseline {
			return
		}
		if !w.finishMapTravelAck(r, time.Now()) {
			return
		}
		if q.AutoSkillSettings != nil {
			// settings 为通过职业和技能限制的自动施法偏好，valid 说明校验是否通过。
			if settings, valid := w.validateAutoSkillSettings(r, q.AutoSkillSettings); valid {
				r.AutoSkills = settings
			}
		}
		r.Ready = true
		w.expireModes(r, time.Now())
		w.snapshot(r)
		w.modes(r, time.Now())
		w.assets(r)
		w.send(r, p.MsgS2CHostingReport, 0, r.Role.HostingReport())
		if r.Player.HP == 0 {
			w.deathNotice(r)
		}
		w.sendSyncReady(r, c.frame.Request, &pb.SyncReady{SceneEpoch: r.Scene, BaselineId: r.Baseline, NextEconomicCommandSeq: uint64(r.Role.EconomicSeq + 1)})
		w.resumeWorldBossObservation(r, time.Now())
		return
	}
	if c.frame.ID == p.MsgC2SMapTravelPrepare {
		w.prepareMapTravel(r, c.frame)
		return
	}
	if c.frame.ID == p.MsgC2SMapTravelCommit {
		w.commitMapTravel(r, c.frame)
		return
	}
	if !r.Ready {
		w.failure(r, c.frame, p.CodeLineRecovering)
		return
	}
	if mapTravelQuiescing(r) && !c.fromWaiting {
		w.failure(r, c.frame, p.CodeLineRecovering)
		return
	}
	if w.flushWearBeforeAsset(r, c.frame, time.Now()) {
		return
	}
	if (c.frame.ID == p.MsgC2SUseItem || c.frame.ID == p.MsgC2SLearnSkill || c.frame.ID == p.MsgC2SSkillAssetCommit || c.frame.ID == p.MsgC2SCastWithMaterial || c.frame.ID == p.MsgC2SSetCastingSupply) && r.Pending {
		w.queuePotion(r, c)
		return
	}
	switch c.frame.ID {
	case p.MsgC2SAutoTargetIntent:
		w.autoTargetIntent(r, c.frame)
	case p.MsgC2SSkillModeIntent:
		// q 记录技能模式意图。
		var q pb.SkillModeIntent
		if proto.Unmarshal(c.frame.Payload, &q) != nil || q.SceneEpoch != r.Scene {
			w.failure(r, c.frame, p.CodeInvalidArgument)
			return
		}
		if !acceptInput(r, q.InputSequence) {
			w.failure(r, c.frame, p.CodeStaleInput)
			w.modes(r, time.Now())
			return
		}
		// code 是调整当前已学技能模式的协议结果；非零时同步拒绝和原模式。
		if code := w.setSkillMode(r, q.SkillId, q.Enabled, time.Now(), c.frame.Request); code != 0 {
			w.failure(r, c.frame, code)
			w.modes(r, time.Now())
		}
	case p.MsgC2SAttackIntent:
		// q 记录攻击意图。
		var q pb.AttackIntent
		if proto.Unmarshal(c.frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.Direction > 7 {
			w.failure(r, c.frame, p.CodeInvalidArgument)
			return
		}
		if !acceptInput(r, q.InputSequence) {
			w.failure(r, c.frame, p.CodeStaleInput)
			return
		}
		// code 是当前朝向普攻的受理结果；非零时返回原请求错误。
		if code := w.attackFacing(r, q.Direction, q.SkillId, q.InputSequence, time.Now()); code != 0 {
			w.failure(r, c.frame, code)
		}
	case p.MsgC2SCastWithMaterial:
		w.materialCast(r, c.frame)
	case p.MsgC2SSkillAssetPreview:
		w.skillAssetPreview(r, c.frame)
	case p.MsgC2SVendorQuote:
		w.vendorPreview(r, c.frame)
	case p.MsgC2SWarehouseQuery:
		w.warehouseQuery(r, c.frame)
	case p.MsgC2STaskCodexState:
		w.taskCodexQuery(r, c.frame)
	case p.MsgC2SMoveIntent:
		// q 记录移动意图。
		var q pb.MoveIntent
		if proto.Unmarshal(c.frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.Direction > 7 || q.CancelNavigation && (q.Destination != nil || q.ReleaseJoystick || q.Direction != 0) {
			w.failure(r, c.frame, p.CodeInvalidArgument)
			return
		}
		if !acceptInput(r, q.InputSequence) {
			w.failure(r, c.frame, p.CodeStaleInput)
			return
		}
		if q.CancelNavigation {
			w.cancelTownNavigation(r)
			// 明确退出才取消地面目标；保留原目标身份供客户端核对，健康断线和摇杆释放不走此分支。
			r.Destination, r.Manual = nil, false
			if r.Navigation.Destination != nil && r.Navigation.Revision != 0 {
				r.Navigation.Status = navigationCancelled
			} else {
				r.Navigation.Status = navigationNone
			}
			w.baseline(r, c.frame.Request)
			return
		}
		if q.ReleaseJoystick {
			r.Manual = false
			return
		}
		if q.Destination != nil {
			// point 组装点位所需的数据。
			point := devconfig.Point{X: int(q.Destination.X), Y: int(q.Destination.Y)}
			if !w.walkable(r, point) {
				w.failure(r, c.frame, p.CodeInvalidArgument)
				return
			}
			if r.Navigation.Revision == math.MaxUint64 {
				w.isolateRoom(r)
				return
			}
			w.cancelTownNavigation(r)
			r.Destination = &point
			r.Navigation = navigationState{InputSequence: q.InputSequence, Destination: &point, Status: navigationActive, Revision: r.Navigation.Revision + 1}
			r.Manual = false
		} else {
			w.cancelTownNavigation(r)
			r.Manual = true
			r.ManualDirection = q.Direction
			r.Destination = nil
			if r.Navigation.InputSequence != 0 {
				r.Navigation.Status = navigationManual
			}
		}
	case p.MsgC2SAutoBattle:
		w.autoBattleIntent(r, c.frame)
	case p.MsgC2SCastIntent:
		// q 记录施法意图。
		var q pb.CastIntent
		if proto.Unmarshal(c.frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.Direction > 7 {
			w.failure(r, c.frame, p.CodeInvalidArgument)
			return
		}
		if !acceptInput(r, q.InputSequence) {
			w.failure(r, c.frame, p.CodeStaleInput)
			return
		}
		if q.SkillId != 0 {
			// code 是带自动补蓝判定的施法受理结果。
			if code := w.castRoomIntent(r, &q, time.Now()); code != 0 {
				w.failure(r, c.frame, code)
			}
			return
		}
		// m 接收遍历怪物集合中的范围时当前的元素。
		for _, m := range r.Monsters[:r.MonsterCount] {
			if m.Handle == q.TargetHandle && m.HP > 0 {
				w.attack(r, &r.Player, m, time.Now(), q.InputSequence)
				break
			}
		}
	case p.MsgC2SEquip, p.MsgC2SRecycle, p.MsgC2SClaimHostingTime, p.MsgC2SBuyHostingTime, p.MsgC2SReviveChoice, p.MsgC2SUseItem, p.MsgC2SLearnSkill, p.MsgC2SSkillAssetCommit, p.MsgC2SSetCastingSupply, p.MsgC2SSetAutoRecycle, p.MsgC2SSetItemLock, p.MsgC2SWarehouseAction, p.MsgC2SVendorCommit, p.MsgC2STaskCodexClaim:
		w.asset(r, c.frame)
	default:
		w.failure(r, c.frame, p.CodeInvalidArgument)
	}
}

// enterLineResult 用当前模板和角色现场构造客户端入场结果。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) enterLineResult(r *room) *pb.EnterLineResult {
	// m 接收当前状态地图供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := w.currentMap(r)
	if !ok {
		w.isolateRoom(r)
		return nil
	}
	return &pb.EnterLineResult{SceneEpoch: r.Scene, BaselineId: r.Baseline, RoomId: m.ID, LineId: 1, ResourceVersion: w.e.Store.Cfg.Fixture.RuleVersion, Position: pointPB(r.Player.Pos), SceneKind: m.SceneKindValue(), NextEconomicCommandSeq: uint64(r.Role.EconomicSeq + 1), PlayerHandle: r.Player.Handle}
}

// initialize 用数据库角色创建冷启动房间，初始化个人场景、身体和临时状态。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，role 记录角色。
func (w *worker) initialize(r *room, role domain.Role) {
	w.leaveSharedTown(r)
	w.cancelTownNavigation(r)
	r.PetBornReady = nil
	r.Travel, r.SavedPersonal, r.TravelHistory = nil, nil, nil
	r.worldState = &worldState{}
	r.PersonalWorld, r.TownWorld = nil, nil
	r.TownWorlds = nil
	r.ExecutionEpoch, r.WorldSequence, r.InflightEvents = 0, 0, 0
	r.Pet, r.PetWorld, r.PetLifeSequence = nil, worldRef{}, 0
	r.PetHandleSequence, r.PendingPetBirth = petHandle, nil
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	r.MapID = f.Map.ID
	// player 接收玩家供后续步骤使用。
	player, _ := f.Player(role.Job)
	r.Scene = binary.BigEndian.Uint64(domain.Bytes(domain.ID())[:8])
	r.PersonalWorld = r.currentWorld()
	r.Role = role
	// 故障后的新执行现场不恢复旧临时计量；健康重入不经过此初始化。
	r.HostMS = 0
	r.hostSecondsOpen = 0
	r.Potions.LastAppliedSeq = role.EconomicSeq
	r.Potions.LastSourceSeq = role.SourceSeq
	r.Navigation = navigationState{}
	r.Destination = nil
	r.Manual = false
	r.AutoSkills = nil
	r.AutoTarget = actorRef{}
	r.MaterialEconomicSeq, r.MaterialSourceSeq = role.EconomicSeq, role.SourceSeq
	r.Auto = true
	r.MonsterCount = 1
	r.Player = entity{Handle: 1, Kind: 1, Def: role.Job, Pos: f.Map.PlayerSpawn, HP: player.HP, MaxHP: player.HP, MP: player.MP, MaxMP: player.MP}
	w.refreshPlayerStats(r)
	r.Player.HP, r.Player.MP = r.Player.MaxHP, r.Player.MaxMP
	if role.RetiredDeathID != "" {
		r.DeathID = role.RetiredDeathID
		r.Player.HP, r.Player.Action = 0, 4
	} else if role.Job == f.Pet.JobID && (!w.petRuntimeEnabled() || f.PetRuntime.BootstrapMode == "legacy_untrained_pet") {
		r.Pet = &entity{Handle: 2, Kind: 2, Def: f.Pet.DefinitionID, Pos: f.Map.PetSpawn, HP: f.Pet.HP, MaxHP: f.Pet.HP, MP: f.Pet.MP, MaxMP: f.Pet.MP}
		r.PetWorld = r.currentWorld().Ref
		if w.petRuntimeEnabled() && !w.attachLegacyPetState(r.Pet) {
			w.isolateRoom(r)
			return
		}
	}
	w.applyPetMapEntryPolicy(r, &f.Map)
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 8; i++ {
		// def 保存从怪物集合取出的“def”。
		def := f.Monsters[i%len(f.Monsters)]
		// hp 保存当前处理所用的生命值。
		hp := def.HP
		r.Monsters = append(r.Monsters, &entity{Handle: uint32(10 + i), Kind: 3, Def: def.ID, Pos: f.Map.MonsterSpawns[i], HP: hp, MaxHP: def.HP, Monster: &def})
	}
}

// after 在当前操作结束后依次重放尚可处理的房间等待命令。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) after(r *room) {
	for len(r.Waiting) > 0 && !r.Pending && !r.Isolated {
		// c 保存从等待取出的等待中的指定项。
		c := r.Waiting[0]
		r.Waiting = r.Waiting[1:]
		c.fromWaiting = true
		w.command(c)
	}
}

// revivePlayer 为同一角色创建新生命代次，按当前地图规则恢复到复活位置。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) revivePlayer(r *room) {
	// m 接收当前状态地图供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := w.currentMap(r)
	if !ok || !m.Walkable(m.PlayerSpawn) {
		w.isolateRoom(r)
		return
	}
	spawn, reserved := w.reserveSharedTownRevival(r, m)
	if !reserved {
		w.isolateRoom(r)
		return
	}
	r.Player.Life++
	r.AutoTarget = actorRef{}
	r.Player.MovementRevision, r.Player.MovementKind = 0, movementInitial
	// id 标识当前战斗效果，status 是该编号下已有的效果实例。
	for id, status := range r.Player.Statuses {
		if status.RemoveOnDeath {
			delete(r.Player.Statuses, id)
		} else {
			status.TargetLife = r.Player.Life
		}
	}
	// 生命代次切换必须同时推进模式观察版本；同连接客户端不能被旧revision永久挡住。
	r.Melee.Revision++
	r.Player.HP = r.Player.MaxHP
	r.Player.MP = r.Player.MaxMP
	if !w.relocate(r, &r.Player, spawn) {
		return
	}
	r.Player.Action = 0
	r.DeathID = ""
	// 同图复活仅释放摇杆，原地面导航目标保留并从新脚点继续规划。
	r.Manual = false
}

// send 向当前附着投递协议帧，按场景确认状态过滤尚不可见的战斗表现。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，id 区分当前步骤指向的目标对象，request 关联客户端请求与服务端的对应响应。
// msg 记录“msg”，使用消息表示。
func (w *worker) send(r *room, id uint16, request uint32, msg proto.Message) {
	if r.Isolated || suppressOrdinaryObservation(r, id) {
		return
	}
	if !r.Ready {
		switch id {
		case p.MsgS2CCastAccepted, p.MsgS2CAttackAccepted, p.MsgS2CCombatImpact, p.MsgS2CCastStep, p.MsgS2CSkillModeState, p.MsgS2CDeathNotice, p.MsgS2CEntityDisplacement, p.MsgS2CPetBorn:
			return
		}
	}
	if r.Conn != nil {
		r.Conn.Deliver(wire.Message(id, request, uint64(r.Auth.SessionEpoch), msg))
	}
}

// failure 按经济命令和普通指令的协议格式返回当前请求失败。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，f 记录协议帧，code 记录结果码，使用数值表示。
func (w *worker) failure(r *room, f wire.Frame, code uint32) {
	if economic(f.ID) {
		w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: code, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
		return
	}
	r.Conn.Deliver(wire.Frame{ID: f.ID, Request: f.Request, Binding: uint64(r.Auth.SessionEpoch), Error: code})
}

// assets 在场景确认后同步最新持久背包、钱包和托管状态。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) assets(r *room) {
	if r.Conn == nil || !r.Ready {
		return
	}
	// state 保存由成长状态取得的状态，err 接收成长状态过程中可能产生的错误，供后续失败分支处理。
	state, err := persistence.ProgressionState(r.Role, w.e.Store.Cfg.Fixture)
	if err != nil {
		w.isolateRoom(r)
		return
	}
	w.send(r, p.MsgS2CInventory, 0, r.Role.Inventory(state))
	w.send(r, p.MsgS2CWallet, 0, r.Role.Wallet.Proto(r.Role.Revision))
	w.hosting(r, 0)
}

// hosting 同步余额、自动战斗开关、资产修订号和自动技能偏好。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，request 关联客户端请求与服务端的对应响应。
func (w *worker) hosting(r *room, request uint32) {
	if r.Conn == nil || !r.Ready {
		return
	}
	w.send(r, p.MsgS2CHostingState, request, &pb.HostingState{RemainHostingSec: uint32(r.Role.HostingSeconds), AutoBattleEnabled: r.Auto,
		StateRevision: uint64(r.Role.Revision), AutoSkillSettings: r.AutoSkills.proto()})
}

// snapshot 在场景可观察且当前没有组合位移延迟时发送基线。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) snapshot(r *room) {
	if !r.Ready || r.Isolated || r.DeferMovementSnapshot {
		return
	}
	w.baseline(r)
}

// 入场基线在ACK之前发送；周期同步仍经snapshot的Ready门槛。明确取消沿同一请求号确认后才关闭连接。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，request 记录请求，使用可变数量的数值表示。
func (w *worker) baseline(r *room, request ...uint32) {
	if r.Conn == nil || suppressOrdinaryObservation(r, p.MsgS2CSceneBaseline) {
		return
	}
	// requestID 标识当前业务使用的请求。
	requestID := uint32(0)
	if len(request) > 0 {
		requestID = request[0]
	}
	// chunks 保存按协议帧上限切分后的完整基线，err 表示基线没有构造成功。
	chunks, err := w.buildBaseline(r, requestID)
	if err != nil {
		w.isolateRoom(r)
		return
	}
	// chunk 接收遍历“chunks”时当前的元素。
	for _, chunk := range chunks {
		w.send(r, p.MsgS2CSceneBaseline, requestID, chunk)
	}
}

// 先构造完整组；新生事实与普通同步使用同一实体投影及真实wire分块。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，requestID 标识当前业务使用的请求。
func (w *worker) buildBaseline(r *room, requestID uint32) ([]*pb.SceneBaselineChunk, error) {
	if r.Sequence == math.MaxUint64 {
		return nil, errors.New("场景基线次序耗尽")
	}
	r.Sequence++
	// out 保存当前处理所用的输出。
	out := &pb.SceneBaselineChunk{SceneEpoch: r.Scene, BaselineId: r.Baseline, ChunkCount: 1, StateSequence: r.Sequence, SkillModeState: w.modeState(r, time.Now()), NavigationState: r.Navigation.Proto(), OwnedPet: w.ownedPetPB(r)}
	out.FireWallCells = r.currentWorld().fireWallCells(time.Now())
	// m 接收当前状态地图供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	if m, ok := w.currentMap(r); ok {
		// zone 保存由安全区域发生时间取得的区域，inside 保存由安全区域发生时间取得的“inside”。
		if zone, inside := m.SafeZoneAt(r.Player.Pos); inside {
			out.SafeZoneId = zone.ID
		}
	}
	// entities 建立带初始值的实体列表，供本段继续处理。
	entities := []*entity{&r.Player}
	// pet 保存由场景世界宠物取得的宠物。
	if pet := r.worldPet(r.currentWorld()); pet != nil {
		entities = append(entities, pet)
	}
	// corpse 接收遍历宠物尸体集合时当前的元素。
	for _, corpse := range r.currentWorld().PetCorpses {
		entities = append(entities, corpse.Body)
	}
	entities = append(entities, r.Monsters[:r.MonsterCount]...)
	entities = append(entities, r.Loot...)
	// e 接收遍历实体时当前的元素。
	for _, e := range entities {
		if e.CapturedUntilRespawn {
			continue
		}
		// moveMS 保存由无符号整数“32”取得的移动毫秒。
		moveMS := uint32(0)
		// attackMS 保存由无符号整数“32”取得的攻击毫秒。
		attackMS := uint32(0)
		// attributes 记录“attributes”，使用战斗“attributes”表示。
		var attributes *pb.CombatAttributes
		if e.Kind >= 1 && e.Kind <= 3 {
			// stats 保存由战斗单位取得的统计或属性。
			stats := w.fighter(r, e)
			moveMS = uint32(stats.MoveMS)
			attackMS = uint32(stats.AttackMS)
			attributes = w.combatAttributesPB(r, e)
		}
		out.Entities = append(out.Entities, &pb.EntityState{EntityHandle: e.Handle, EntityKind: e.Kind, DefinitionId: e.Def, Position: pointPB(e.Pos), Direction: e.Direction, Hp: e.HP, HpMax: e.MaxHP, Mp: e.MP, MpMax: e.MaxMP, Action: e.Action, MoveIntervalMs: moveMS, AttackSequence: e.AttackSequence, CastSequence: e.CastSequence, CastSkillId: e.CastSkillID, Life: e.Life, AttackIntervalMs: attackMS, AttackSkillId: e.AttackSkillID, AttackSkillLevel: e.AttackSkillLevel, Statuses: statusesPB(e, time.Now()), CombatAttributes: attributes, MovementRevision: e.MovementRevision, MovementKind: e.MovementKind, PetTraining: petTrainingPB(e), IsPetCorpse: e.IsPetCorpse})
	}
	w.appendSharedTownViews(r, out)
	w.appendWorldBossViews(r, out)
	return splitBaseline(out, requestID, uint64(r.Auth.SessionEpoch))
}

// pointPB 把服务端逻辑格位置转换为协议格式。
// p 记录点位。
func pointPB(p devconfig.Point) *pb.GridPoint { return &pb.GridPoint{X: int32(p.X), Y: int32(p.Y)} }

// asset 解析持久资产指令及操作身份，应用门禁并进入原子事务队列。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，f 记录协议帧。
func (w *worker) asset(r *room, f wire.Frame) {
	// reviveID 标识当前业务使用的复活。
	reviveID := ""
	// q 组装资产请求所需的数据。
	q := persistence.AssetRequest{Fingerprint: persistence.Hash(append([]byte{byte(f.ID >> 8), byte(f.ID)}, f.Payload...))}
	switch f.ID {
	case p.MsgC2STaskCodexClaim:
		// 领取只携带目录编号和查询时的运营日，进度及奖励由角色事务重新校验。
		var x pb.TaskCodexClaimRequest
		if proto.Unmarshal(f.Payload, &x) != nil || len(x.ProtoReflect().GetUnknown()) != 0 {
			w.failure(r, f, p.CodeInvalidArgument)
			return
		}
		q.Kind, q.Context, q.TaskCodex = persistence.AssetTaskCodexClaim, x.Operation, &x
	case p.MsgC2SVendorCommit:
		// x 记录商人提交请求。
		var x pb.VendorCommitRequest
		if proto.Unmarshal(f.Payload, &x) != nil || len(x.QuoteId) != 16 {
			w.failure(r, f, p.CodeInvalidArgument)
			return
		}
		q.Kind, q.Context, q.Vendor = persistence.AssetVendor, x.Operation, &x
		// 原回执优先核对；只有尚未提交的命令需要再次见到同一名商人。
		// shop 保存由商人商店取得的商店。
		if shop := w.vendorShop(r, x.SceneEpoch, x.NpcId); shop != nil {
			q.VendorShop = shop.Key
		}
	case p.MsgC2SWarehouseAction:
		// x 记录仓库动作。
		var x pb.WarehouseAction
		if proto.Unmarshal(f.Payload, &x) != nil {
			w.failure(r, f, p.CodeInvalidArgument)
			return
		}
		q.Kind, q.Context, q.Warehouse = "warehouse", x.Operation, &x
	case p.MsgC2SSetAutoRecycle:
		// x 记录设置自动回收请求。
		var x pb.SetAutoRecycleRequest
		if proto.Unmarshal(f.Payload, &x) != nil || x.Policy == nil {
			w.failure(r, f, p.CodeInvalidArgument)
			return
		}
		q.Kind, q.Context, q.AutoRecycle, q.ApplyExisting = "set_auto_recycle", x.Operation, x.Policy, x.ApplyExisting
	case p.MsgC2SSetItemLock:
		// x 记录设置道具锁定请求。
		var x pb.SetItemLockRequest
		if proto.Unmarshal(f.Payload, &x) != nil || len(x.ItemInstanceId) != 16 {
			w.failure(r, f, p.CodeInvalidArgument)
			return
		}
		q.Kind, q.Context, q.ItemID, q.ItemVersion, q.Locked = "set_item_lock", x.Operation, domain.Hex(x.ItemInstanceId), x.ItemVersion, x.Locked
	case p.MsgC2SSetCastingSupply:
		// x 记录设置施法耗材请求。
		var x pb.SetCastingSupplyRequest
		if proto.Unmarshal(f.Payload, &x) != nil || len(x.ItemInstanceId) != 16 {
			w.failure(r, f, p.CodeInvalidArgument)
			return
		}
		q.Kind, q.Context, q.ItemID, q.ItemVersion, q.Unequip = "set_casting_supply", x.Operation, domain.Hex(x.ItemInstanceId), x.ItemVersion, x.Unequip
	case p.MsgC2SSkillAssetCommit:
		// x 记录技能资产提交请求。
		var x pb.SkillAssetCommitRequest
		if proto.Unmarshal(f.Payload, &x) != nil || len(x.PreviewId) != 16 {
			w.failure(r, f, p.CodeInvalidArgument)
			return
		}
		q.Kind = "skill_assets_commit"
		q.Context = x.Operation
		q.PreviewID = domain.Hex(x.PreviewId)
	case p.MsgC2SEquip:
		// x 记录穿戴装备请求。
		var x pb.EquipRequest
		if proto.Unmarshal(f.Payload, &x) != nil {
			w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
			return
		}
		q.Kind = "equip"
		q.Context = x.Operation
		q.ItemID = domain.Hex(x.ItemInstanceId)
		q.Slot = x.Slot
		q.Unequip, q.ItemVersion = x.Unequip, x.ItemVersion
	case p.MsgC2SRecycle:
		// x 记录回收请求。
		var x pb.RecycleRequest
		if proto.Unmarshal(f.Payload, &x) != nil {
			w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
			return
		}
		q.Kind = "recycle"
		q.Context = x.Operation
		// id 接收遍历道具进程或道具实例标识集合时当前的元素。
		for _, id := range x.ItemInstanceIds {
			q.ItemIDs = append(q.ItemIDs, domain.Hex(id))
		}
	case p.MsgC2SClaimHostingTime:
		// x 记录领取离线托管时间请求。
		var x pb.ClaimHostingTimeRequest
		if proto.Unmarshal(f.Payload, &x) != nil {
			w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
			return
		}
		q.Kind = "claim_hosting"
		q.Context = x.Operation
		q.Benefit = x.BenefitKind
	case p.MsgC2SBuyHostingTime:
		w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
		return
	case p.MsgC2SReviveChoice:
		// x 记录复活选择。
		var x pb.ReviveChoice
		if proto.Unmarshal(f.Payload, &x) != nil {
			w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
			return
		}
		reviveID = domain.Hex(x.DeathId)
		q.Kind = "revive"
		q.Context = x.Operation
		q.ReviveDeathID = reviveID
		q.ValidRevive = w.e.Store.Cfg.Fixture.AllowFreeDevRevive && x.Action == 1 && r.Player.HP == 0 && reviveID == r.DeathID
	case p.MsgC2SUseItem:
		// x 记录使用道具请求。
		var x pb.UseItemRequest
		if proto.Unmarshal(f.Payload, &x) != nil || len(x.ItemInstanceId) != 16 {
			w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
			return
		}
		q.Kind = "use_item"
		q.Context = x.Operation
		q.ItemID = domain.Hex(x.ItemInstanceId)
		q.ItemDefinitionID = x.ItemId
		q.SceneEpoch = x.SceneEpoch
		q.PotionGateCode = potionGate(r, x.SceneEpoch, time.Now())
	case p.MsgC2SLearnSkill:
		// x 记录学习技能请求。
		var x pb.LearnSkillRequest
		if proto.Unmarshal(f.Payload, &x) != nil || len(x.ItemInstanceId) != 16 {
			w.failure(r, f, p.CodeInvalidArgument)
			return
		}
		q.Kind = "learn_skill"
		q.Context = x.Operation
		q.ItemID = domain.Hex(x.ItemInstanceId)
		q.SkillID = x.SkillId
	}
	if q.Context == nil || len(q.Context.OpId) != 16 {
		w.send(r, p.MsgS2COperationResult, f.Request, &pb.OperationResult{OpId: operationID(f), ErrorCode: p.CodeInvalidArgument, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)})
		return
	}
	if r.Pending {
		// op 保存当前处理所用的操作。
		op := &pb.OperationResult{ErrorCode: p.CodeOperationPending, Pending: true, CommandSeq: uint64(r.Role.EconomicSeq), NewRevision: uint64(r.Role.Revision)}
		if q.Context != nil {
			op.OpId = q.Context.OpId
		}
		w.send(r, p.MsgS2COperationResult, f.Request, op)
		return
	}
	w.submitAsset(r, q, f, reviveID)
}

// economic 判断该消息是否需要经济操作格式及原编号。
// id 区分当前步骤指向的目标对象。
func economic(id uint16) bool {
	switch id {
	case p.MsgC2SEquip, p.MsgC2SRecycle, p.MsgC2SClaimHostingTime, p.MsgC2SBuyHostingTime, p.MsgC2SReviveChoice, p.MsgC2SUseItem, p.MsgC2SLearnSkill, p.MsgC2SSkillAssetCommit, p.MsgC2SSetCastingSupply, p.MsgC2SCastWithMaterial, p.MsgC2SSetAutoRecycle, p.MsgC2SSetItemLock, p.MsgC2SWarehouseAction, p.MsgC2SVendorCommit, p.MsgC2STaskCodexClaim:
		return true
	}
	return false
}

// operationContext 从各类持久操作的协议正文提取经济身份。
// f 记录协议帧。
func operationContext(f wire.Frame) *pb.OperationContext {
	// op 记录操作，使用操作上下文表示。
	var op *pb.OperationContext
	switch f.ID {
	case p.MsgC2STaskCodexClaim:
		// 提取同一领取请求的幂等身份，供错误回包和原请求对账使用。
		q := &pb.TaskCodexClaimRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SVendorCommit:
		// q 保存当前处理所用的商人提交请求。
		q := &pb.VendorCommitRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SWarehouseAction:
		// q 保存当前处理所用的仓库动作。
		q := &pb.WarehouseAction{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SSetAutoRecycle:
		// q 保存当前处理所用的设置自动回收请求。
		q := &pb.SetAutoRecycleRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SSetItemLock:
		// q 保存当前处理所用的设置道具锁定请求。
		q := &pb.SetItemLockRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SCastWithMaterial:
		// q 保存当前处理所用的施法带有材料请求。
		q := &pb.CastWithMaterialRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SSetCastingSupply:
		// q 保存当前处理所用的设置施法耗材请求。
		q := &pb.SetCastingSupplyRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SSkillAssetCommit:
		// q 保存当前处理所用的技能资产提交请求。
		q := &pb.SkillAssetCommitRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SEquip:
		// q 保存当前处理所用的穿戴装备请求。
		q := &pb.EquipRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SRecycle:
		// q 保存当前处理所用的回收请求。
		q := &pb.RecycleRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SClaimHostingTime:
		// q 保存当前处理所用的领取离线托管时间请求。
		q := &pb.ClaimHostingTimeRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SBuyHostingTime:
		// q 保存当前处理所用的“buy”离线托管时间请求。
		q := &pb.BuyHostingTimeRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SReviveChoice:
		// q 保存当前处理所用的复活选择。
		q := &pb.ReviveChoice{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SUseItem:
		// q 保存当前处理所用的使用道具请求。
		q := &pb.UseItemRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	case p.MsgC2SLearnSkill:
		// q 保存当前处理所用的学习技能请求。
		q := &pb.LearnSkillRequest{}
		if proto.Unmarshal(f.Payload, q) == nil {
			op = q.Operation
		}
	}
	return op
}

// operationID 返回错误回执需要携带的原始操作编号。
// f 记录协议帧。
func operationID(f wire.Frame) []byte {
	// op 保存由操作上下文取得的操作。
	op := operationContext(f)
	if op != nil {
		return op.OpId
	}
	return nil
}
