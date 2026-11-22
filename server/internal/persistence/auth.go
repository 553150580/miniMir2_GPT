// 账号身份与角色入场。校验本地密码、签发账号令牌和一次性入场票据，处理角色创建幂等、角色目录查询和持久角色会话接管。

package persistence

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"regexp"
	"strings"
	"time"
)

// loginBudget 限制 Argon2 同时占用的高内存计算槽，过量请求直接返回繁忙。
var loginBudget = make(chan struct{}, 4)

// localPattern 约束开发登录使用的账号和口令字符及长度。
var localPattern = regexp.MustCompile(`^[A-Za-z0-9_]{6,20}$`)

// errRoleSlotOrNameOccupied 由唯一索引冲突转换，供上层在新事务封存确定拒绝。
var errRoleSlotOrNameOccupied = errors.New("角色槽位或名字已占用")

// Token 是账号登录返回的随机凭据在数据库中的摘要与授权快照。
type Token struct {
	ID          string    `bson:"_id"`          // 客户端随机令牌的哈希，库里仅保存查询键。
	AccountID   string    `bson:"account_id"`   // 令牌所属账号。
	AuthVersion int64     `bson:"auth_version"` // 签发时的账号授权代次。
	Expires     time.Time `bson:"expires_at"`   // 不再接受该令牌的时间，供查询校验与 TTL 清理。
}

// Ticket 是账号服签发给某个运行实例和角色的一次性入场许可。
type Ticket struct {
	ID           string    `bson:"_id"`           // 随机票据的哈希。
	AccountID    string    `bson:"account_id"`    // 入场要求的账号归属。
	AuthVersion  int64     `bson:"auth_version"`  // 签发时的账号授权代次。
	RoleID       string    `bson:"role_id"`       // 票据允许接管的角色键。
	Origin       uint64    `bson:"origin"`        // 角色的永久入口服。
	Run          uint64    `bson:"run"`           // 票据指定的运行服。
	Epoch        int64     `bson:"epoch"`         // 运行服授权代次。
	Owner        string    `bson:"owner"`         // 票据绑定的游戏进程实例。
	RouteVersion int64     `bson:"route_version"` // 对应本次路由的控制版本。
	Expires      time.Time `bson:"expires_at"`    // 票据的有效截止时刻。
	Used         bool      `bson:"used"`          // 是否已经在一次成功事务中被消耗。
}

// Login 注册或校验本地账号，成功后签发短期账号令牌。
// s 提供账号持久化，ctx 限定数据库访问时长，name 是输入的账号名，password 仅在本次密码派生中使用。
// register 为真时创建新账号，为假时核验既有账号。
func (s *Store) Login(ctx context.Context, name, password string, register bool) (*pb.LoginResult, error) {
	if !localPattern.MatchString(name) || !localPattern.MatchString(password) {
		return nil, Fail(p.CodeInvalidArgument, "本地账号与口令须为6至20位字母数字下划线")
	}
	select {
	case loginBudget <- struct{}{}:
		defer func() { <-loginBudget }()
	default:
		return nil, Fail(p.CodeServerBusy, "账号认证并发预算已满")
	}
	// 统一账号大小写，使登录和唯一索引使用相同身份。
	name = strings.ToLower(name)
	// a 容纳已找到的账号文档，注册路径会换成新文档。
	var a domain.Account
	// e 区分账号已存在、未找到及数据库读取失败。
	e := s.DB.Collection("accounts").FindOne(ctx, bson.M{"name": name}).Decode(&a)
	if register {
		if e == nil {
			return nil, Fail(p.CodeOperationConflict, "账号已存在")
		}
		if !errors.Is(e, mongo.ErrNoDocuments) {
			return nil, e
		}
		// salt 保存该账号独立生成的随机密码盐字节。
		salt := make([]byte, 16)
		rand.Read(salt)
		a = domain.Account{ID: domain.ID(), Name: name, Salt: salt, PasswordHash: argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32), Active: true, AuthVersion: 1}
		if _, e = s.DB.Collection("accounts").InsertOne(ctx, a); e != nil {
			if mongo.IsDuplicateKeyError(e) {
				return nil, Fail(p.CodeOperationConflict, "账号已存在")
			}
			return nil, e
		}
	} else {
		if e != nil {
			return nil, authorityReadError(e, p.CodeUnauthenticated, "账号或口令不符")
		}
		if !a.Active {
			return nil, Fail(p.CodeUnauthenticated, "账号或口令不符")
		}
		// actual 是本次输入与持久盐派生出的密码哈希，供常量时间比较。
		actual := argon2.IDKey([]byte(password), a.Salt, 3, 64*1024, 1, 32)
		if subtle.ConstantTimeCompare(actual, a.PasswordHash) != 1 {
			return nil, Fail(p.CodeUnauthenticated, "账号或口令不符")
		}
	}
	// 只在注册或密码校验成功后生成凭据，向客户端交付令牌原文。
	// token 是交给客户端的随机账号令牌原文。
	token := domain.ID() + domain.ID()
	// until 是账号令牌的两小时有效期终点。
	until := time.Now().Add(2 * time.Hour)
	_, e = s.DB.Collection("account_tokens").InsertOne(ctx, Token{ID: Hash([]byte(token)), AccountID: a.ID, AuthVersion: a.AuthVersion, Expires: until})
	return &pb.LoginResult{AccountId: domain.Bytes(a.ID), AccountToken: token, ExpiresAtUnix: uint64(until.Unix())}, e
}

// Authenticate 同时核验令牌有效期与最新账号授权代次。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，token 记录令牌，使用文本表示。
func (s *Store) Authenticate(ctx context.Context, token string) (Auth, error) {
	// t 保存从令牌哈希查到的账号、签发版本与有效期。
	var t Token
	if len(token) != 64 {
		return Auth{}, Fail(p.CodeUnauthenticated, "需要账号认证")
	}
	// e 区分令牌不存在、过期及数据库读取错误。
	if e := s.DB.Collection("account_tokens").FindOne(ctx, bson.M{"_id": Hash([]byte(token)), "expires_at": bson.M{"$gt": time.Now()}}).Decode(&t); e != nil {
		return Auth{}, authorityReadError(e, p.CodeUnauthenticated, "账号认证已过期")
	}
	// a 保存仍启用、且授权代次与令牌一致的账号。
	var a domain.Account
	// e 表示当前账号授权已失效或账号查询失败。
	if e := s.DB.Collection("accounts").FindOne(ctx, bson.M{"_id": t.AccountID, "active": true, "auth_version": t.AuthVersion}).Decode(&a); e != nil {
		return Auth{}, authorityReadError(e, p.CodeUnauthenticated, "账号授权失效")
	}
	return Auth{AccountID: a.ID, AuthVersion: a.AuthVersion}, nil
}

// Roles 返回账号在请求入口服下的角色摘要目录。
// s 提供角色目录，ctx 限定读取时长，a 是已认证账号，origin 是要查询的永久入口服。
func (s *Store) Roles(ctx context.Context, a Auth, origin uint64) (*pb.RoleDirectoryResult, error) {
	if origin != s.Cfg.OriginID {
		return nil, Fail(p.CodeRouteChanged, "入口不存在")
	}
	// cursor 迭代该账号及入口下的角色，e 保存建游标或解码期间的错误。
	cursor, e := s.DB.Collection("roles").Find(ctx, bson.M{"account_id": a.AccountID, "origin_server_id": origin})
	if e != nil {
		return nil, e
	}
	defer cursor.Close(ctx)
	// out 保存当前处理所用的输出。
	out := &pb.RoleDirectoryResult{RouteVersion: 1}
	for cursor.Next(ctx) {
		// r 记录角色。
		var r domain.Role
		if e = cursor.Decode(&r); e != nil {
			return nil, e
		}
		out.Roles = append(out.Roles, r.Summary())
	}
	return out, cursor.Err()
}

// CreateRole 校验入口、槽位、职业和名字，用原操作键幂等写入角色及回执。
// s 提供角色和回执集合，ctx 限制提交时长，a 确定角色归属，q 携带槽位、职业、姓名和幂等键。
func (s *Store) CreateRole(ctx context.Context, a Auth, q *pb.CreateRoleRequest) (*pb.RoleSummary, error) {
	if q.OriginServerId != s.Cfg.OriginID || q.Slot < 1 || q.Slot > 5 || q.Job > 2 || len(q.OpId) != 16 {
		return nil, Fail(p.CodeInvalidArgument, "角色槽位或操作ID不合法")
	}
	// ok 表明查询、校验或类型转换是否成功。
	if _, ok := s.Cfg.Fixture.Player(q.Job); !ok {
		return nil, Fail(p.CodeInvalidArgument, "该职业尚无开发fixture")
	}
	// name 是去掉前后空白且完成 NFKC 规范化的显示名。
	name := norm.NFKC.String(strings.TrimSpace(q.Name))
	if len([]rune(name)) < 1 || len([]rune(name)) > 16 {
		return nil, Fail(p.CodeInvalidArgument, "角色名须为1至16字")
	}
	// 先固定原请求的内容与身份，之后的事务重试沿用相同回执键。
	// raw 保存由序列化取得的原始值。
	raw, _ := json.Marshal(q)
	// fingerprint 是原建角请求的内容摘要，用于重复提交核对。
	fingerprint := Hash(raw)
	// opID 标识当前业务使用的操作。
	opID := "create:" + a.AccountID + ":" + domain.Hex(q.OpId)
	// r 组装角色所需的数据。
	r := domain.Role{ID: domain.ID(), AccountID: a.AccountID, Origin: q.OriginServerId, Slot: q.Slot, Job: q.Job, Gender: 0, Name: name, NameKey: strings.ToLower(name), Level: 1, Revision: 1, Capacity: s.Cfg.Fixture.InventoryCapacity, Wallet: domain.Wallet{BoundGold: s.Cfg.Fixture.InitialBoundGold}, Items: []domain.Item{}, Equipped: []domain.Equipped{}}
	if s.Cfg.Fixture.RuleVersion >= 20 {
		// contract 提供新角色的成长格式和曲线版本，err 留存规则合同无法构造的错误。
		contract, err := GrowthContract(s.Cfg.Fixture)
		if err != nil {
			return nil, err
		}
		r.ProgressionFormatVersion = contract.FormatVersion
		r.ProgressionCurveVersion = contract.CurveVersion
	}
	if q.Job == 1 {
		r.Gender = 1
	}
	// id 接收遍历初始值背包资产时当前的元素。
	for _, id := range s.Cfg.Fixture.InitialInventory {
		r.Items = append(r.Items, InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: id, Quantity: 1, Bound: true, Version: 1}, &s.Cfg.Fixture))
	}
	// book 接收遍历初始值技能技能书集合时当前的元素。
	for _, book := range s.Cfg.Fixture.InitialSkillBooks {
		if book.JobID == q.Job {
			r.Items = append(r.Items, domain.Item{ID: domain.ID(), ItemID: book.ItemID, Quantity: 1, Bound: true, Version: 1, SourceKind: "initial_grant", SourceID: domain.Hex(q.OpId)})
		}
	}
	// binding 是新角色的默认复活安全区，err 保存该安全区无法由配置确定的原因。
	binding, err := s.initialBinding()
	if err != nil {
		return nil, err
	}
	r.Binding = binding
	// creation 建立带初始值的操作，供本段继续处理。
	creation := Operation{ID: opID, Digest: fingerprint, RoleID: r.ID, Kind: "create_role", Created: time.Now()}
	// result 是本次创建或重放得到的角色，code 表示永久业务结果。
	result, code, err := s.persistRoleCreation(ctx, a, r, creation)
	if errors.Is(err, errRoleSlotOrNameOccupied) {
		// 插入冲突已使原事务回滚。重新持有账号围栏，先重查原回执，再把确定拒绝封存为同一个操作的终态。
		creation.RoleID = ""
		creation.Code = p.CodeRoleSlotOrNameOccupied
		result, code, err = s.persistRoleCreation(ctx, a, r, creation)
	}
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, Fail(code, "原建角已终止，槽位或名字已占用")
	}
	return result.Summary(), nil
}

// persistRoleCreation 在账号围栏保护下优先找原回执，再一起写入角色与首次结果。
// s 提供角色与回执存储，ctx 限制事务时间，a 固定账号授权，role 是建角候选，creation 是与原操作键绑定的候选回执。
func (s *Store) persistRoleCreation(ctx context.Context, a Auth, role domain.Role, creation Operation) (domain.Role, uint32, error) {
	// result 保存首次创建或沿永久回执找到的原角色。
	var result domain.Role
	// code 保存建角的确定结果，零表示成功，非零可沿相同操作键重放。
	var code uint32
	// err 保存整个事务的错误，tc 让账号围栏、角色和回执变更一起提交。
	err := s.tx(ctx, func(tc context.Context) error {
		// e 接收账号授权版本围栏供后续步骤使用。
		if e := s.accountGuard(tc, a); e != nil {
			return e
		}
		// operations 保存由集合取得的操作集合。
		operations := s.DB.Collection("operations")
		// previous 保存同一操作键已经提交的建角回执。
		var previous Operation
		// err 接收解码过程中可能产生的错误，供后续失败分支处理。
		err := operations.FindOne(tc, bson.M{"_id": creation.ID}).Decode(&previous)
		if err == nil {
			if previous.Digest != creation.Digest {
				return Fail(p.CodeOperationConflict, "同操作ID参数变化")
			}
			code = previous.Code
			if code != 0 {
				return nil
			}
			result, err = s.Role(tc, previous.RoleID)
			return err
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		// 槽位或名字拒绝也可形成永久终态，重放原请求会返回相同拒绝。
		code = creation.Code
		if code == 0 {
			if _, err = s.DB.Collection("roles").InsertOne(tc, role); err != nil {
				if mongo.IsDuplicateKeyError(err) {
					return errRoleSlotOrNameOccupied
				}
				return err
			}
			result = role
		}
		_, err = operations.InsertOne(tc, creation)
		return err
	})
	return result, code, err
}

// IssueTicket 为账号拥有的角色签发与当前运行服进程及代次绑定的入场许可。
// s 提供票据与运行租约存储，ctx 约束读写时长，a 确定账号权限，key 标明所选角色和永久入口。
func (s *Store) IssueTicket(ctx context.Context, a Auth, key *pb.RoleKey) (*pb.EntryTicketResult, error) {
	if key == nil || len(key.RoleId) != 16 || key.OriginServerId != s.Cfg.OriginID {
		return nil, Fail(p.CodeInvalidArgument, "所选角色不可用")
	}
	// 先按账号和原区服过滤，再读取角色证明，避免向其他账号暴露角色状态。
	// r 是按账号和入口验证后的角色，e 保存权限查询或角色读入失败。
	r, e := s.readRole(ctx, bson.M{"_id": domain.Hex(key.RoleId), "account_id": a.AccountID, "origin_server_id": key.OriginServerId})
	if e != nil {
		return nil, authorityReadError(e, p.CodeInvalidArgument, "所选角色不可用")
	}
	// run 保存有效运行服的当前持有实例及其代次。
	var run Run
	e = s.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": s.Cfg.RunID, "state": "active", "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}).Decode(&run)
	if e != nil {
		return nil, authorityReadError(e, p.CodeLineRecovering, "游戏节点尚未就绪")
	}
	// token 是交给客户端、供 WebSocket 入场使用的一次性票据原文。
	token := domain.ID() + domain.ID()
	// until 是本张票据 45 秒的有效期终点。
	until := time.Now().Add(45 * time.Second)
	_, e = s.DB.Collection("entry_tickets").InsertOne(ctx, Ticket{ID: Hash([]byte(token)), AccountID: a.AccountID, AuthVersion: a.AuthVersion, RoleID: r.ID, Origin: r.Origin, Run: s.Cfg.RunID, Owner: run.Owner, Epoch: run.Epoch, RouteVersion: 1, Expires: until})
	if e != nil {
		return nil, e
	}
	return &pb.EntryTicketResult{Ticket: token, GameWssUrl: "ws://" + s.Cfg.GameAddress + "/ws", RunServerId: s.Cfg.RunID, RouteVersion: 1, ExpiresAtUnix: uint64(until.Unix())}, nil
}

// TicketRole 预读未消费且未过期的票据，供游戏服把入场分派到角色 worker。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，token 记录令牌，使用文本表示。
func (s *Store) TicketRole(ctx context.Context, token string) (Ticket, error) {
	// t 预读账号服签发的票据文档，供后续选择房间和正式接管。
	var t Ticket
	if len(token) != 64 {
		return t, Fail(p.CodeUnauthenticated, "入场票据不合法")
	}
	// e 区分票据失效与暂时无法读取。
	e := s.DB.Collection("entry_tickets").FindOne(ctx, bson.M{"_id": Hash([]byte(token)), "used": false, "expires_at": bson.M{"$gt": time.Now()}}).Decode(&t)
	if e != nil {
		return t, authorityReadError(e, p.CodeUnauthenticated, "票据过期或已使用")
	}
	return t, nil
}

// Enter 在同一事务消费票据并接管角色，返回新角色会话身份和持久状态。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，token 记录令牌，使用文本表示。
func (s *Store) Enter(ctx context.Context, token string) (domain.Role, Auth, error) {
	t, err := s.TicketRole(ctx, token)
	if err != nil {
		return domain.Role{}, Auth{}, err
	}
	role, auth, err := s.EnterTicket(ctx, t, domain.ID(), true)
	if err != nil {
		return domain.Role{}, Auth{}, err
	}
	return role, auth, nil
}

// EnterTicket 提交已查验票据的原接管尝试。attempt 由服务端生成，同一次提交待确认时沿用。
// allowTakeover 为假时只查证已提交会话，已断开的请求不再发起新的接管。
// 拒绝时若已在持久围栏内读到角色，返回该视图供房间核对旧会话是否仍有效。
func (s *Store) EnterTicket(ctx context.Context, t Ticket, attempt string, allowTakeover bool) (domain.Role, Auth, error) {
	if t.Owner != s.Instance || t.Epoch != s.Epoch || t.Run != s.Cfg.RunID {
		return domain.Role{}, Auth{}, Fail(p.CodeRouteChanged, "票据目标运行代次过期")
	}
	if len(t.ID) != 64 || len(domain.Bytes(attempt)) != 16 {
		return domain.Role{}, Auth{}, Fail(p.CodeInvalidArgument, "缺少已校验的入场尝试身份")
	}
	key := Hash([]byte(t.ID + ":" + attempt))
	// 在重试事务之外固定本次候选；已存在的永久绑定始终优先。
	// initial 是角色缺少持久绑定时才会使用的默认复活位置。
	initial, e := s.initialBinding()
	if e != nil {
		return domain.Role{}, Auth{}, e
	}
	// a 从原票据提取账号和角色身份，角色所有权确认后再补会话代次。
	a := Auth{AccountID: t.AccountID, AuthVersion: t.AuthVersion, RoleID: t.RoleID}
	// r 记录角色。
	var r domain.Role
	// tc 将票据消费、角色接管和需要的迁移写入绑定到同一事务。
	e = s.guardedTx(ctx, a, func(tc context.Context) error {
		r = domain.Role{}
		// 原提交也写入这两道围栏。等待期间不能以一次未见回执的查询释放旧会话。
		if err := s.fence(tc, a); err != nil {
			return err
		}
		var err error
		r, err = s.Role(tc, t.RoleID)
		if err != nil {
			return err
		}
		if r.AccountID != t.AccountID || r.Origin != t.Origin {
			return Fail(p.CodeInvalidArgument, "票据角色归属已变化")
		}
		if r.LastEntryAttempt == key {
			return s.checkRole(r, Auth{AccountID: a.AccountID, RoleID: a.RoleID, SessionEpoch: r.SessionEpoch})
		}
		if !allowTakeover {
			return Fail(p.CodeUnauthenticated, "入场连接已关闭")
		}
		if r.SessionEpoch == math.MaxInt64 || r.Revision == math.MaxInt64 {
			return Fail(p.CodeOperationConflict, "角色会话或版本无法继续递增")
		}
		// 消费结果与新的绑定代次同时提交；仅原内部尝试可查证已消费的票据。
		now := time.Now()
		used, err := s.DB.Collection("entry_tickets").UpdateOne(tc, bson.M{
			"_id": t.ID, "account_id": t.AccountID, "auth_version": t.AuthVersion, "role_id": t.RoleID,
			"origin": t.Origin, "run": t.Run, "epoch": t.Epoch, "owner": t.Owner, "used": false, "expires_at": bson.M{"$gt": now},
		}, bson.M{"$set": bson.M{"used": true, "used_at": now, "entry_attempt": key, "session_epoch": r.SessionEpoch + 1}})
		if err != nil {
			return err
		}
		if used.MatchedCount != 1 {
			return Fail(p.CodeUnauthenticated, "票据不可重用")
		}
		if r.Binding == nil {
			r.Binding = initial
		}
		if err = s.checkBinding(r.Binding); err != nil {
			return err
		}
		// old 保存修订号，用于当前步骤中的原状态。
		old := r.Revision
		// migration 保存本次可能发生的成长格式迁移结果，供同一事务记录审计。
		var migration *ExperienceGrantReceipt
		if s.Cfg.Fixture.RuleVersion >= 20 {
			// contract 固定当前玩法的经验版本约束，e 保存合同构造失败。
			contract, e := GrowthContract(s.Cfg.Fixture)
			if e != nil {
				return e
			}
			r, migration, err = experienceWithContract(r, 0, contract)
			if err != nil {
				return err
			}
		}
		// 每次接管都推进会话代次和角色版本，使原连接的后续命令失效。
		r.Owner = s.Instance
		r.RunEpoch = s.Epoch
		r.SessionEpoch++
		r.Revision++
		r.LastEntryAttempt = key
		// res 的匹配数用于确认原角色版本与账号归属仍一致。
		res, err := s.DB.Collection("roles").ReplaceOne(tc, bson.M{"_id": r.ID, "revision": old, "account_id": t.AccountID}, r)
		if err != nil {
			return err
		}
		if res.MatchedCount != 1 {
			return Fail(p.CodeOperationConflict, "角色接管冲突")
		}
		if err = s.recordProgressionMigration(tc, r, migration); err != nil {
			return err
		}
		return nil
	})
	a.SessionEpoch = r.SessionEpoch
	return r, a, e
}
