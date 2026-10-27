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
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"regexp"
	"strings"
	"time"
)

var loginBudget = make(chan struct{}, 4)

var localPattern = regexp.MustCompile(`^[A-Za-z0-9_]{6,20}$`)

type Token struct {
	ID          string    `bson:"_id"`
	AccountID   string    `bson:"account_id"`
	AuthVersion int64     `bson:"auth_version"`
	Expires     time.Time `bson:"expires_at"`
}
type Ticket struct {
	ID           string    `bson:"_id"`
	AccountID    string    `bson:"account_id"`
	AuthVersion  int64     `bson:"auth_version"`
	RoleID       string    `bson:"role_id"`
	Origin       uint64    `bson:"origin"`
	Run          uint64    `bson:"run"`
	Epoch        int64     `bson:"epoch"`
	Owner        string    `bson:"owner"`
	RouteVersion int64     `bson:"route_version"`
	Expires      time.Time `bson:"expires_at"`
	Used         bool      `bson:"used"`
}

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
	name = strings.ToLower(name)
	var a domain.Account
	e := s.DB.Collection("accounts").FindOne(ctx, bson.M{"name": name}).Decode(&a)
	if register {
		if !errors.Is(e, mongo.ErrNoDocuments) {
			return nil, Fail(p.CodeOperationConflict, "账号已存在")
		}
		salt := make([]byte, 16)
		rand.Read(salt)
		a = domain.Account{ID: domain.ID(), Name: name, Salt: salt, PasswordHash: argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32), Active: true, AuthVersion: 1}
		if _, e = s.DB.Collection("accounts").InsertOne(ctx, a); e != nil {
			return nil, Fail(p.CodeOperationConflict, "账号已存在或登记失败")
		}
	} else {
		if e != nil || !a.Active {
			return nil, Fail(p.CodeUnauthenticated, "账号或口令不符")
		}
		actual := argon2.IDKey([]byte(password), a.Salt, 3, 64*1024, 1, 32)
		if subtle.ConstantTimeCompare(actual, a.PasswordHash) != 1 {
			return nil, Fail(p.CodeUnauthenticated, "账号或口令不符")
		}
	}
	token := domain.ID() + domain.ID()
	until := time.Now().Add(2 * time.Hour)
	_, e = s.DB.Collection("account_tokens").InsertOne(ctx, Token{ID: Hash([]byte(token)), AccountID: a.ID, AuthVersion: a.AuthVersion, Expires: until})
	return &pb.LoginResult{AccountId: domain.Bytes(a.ID), AccountToken: token, ExpiresAtUnix: uint64(until.Unix())}, e
}
func (s *Store) Authenticate(ctx context.Context, token string) (Auth, error) {
	var t Token
	if len(token) != 64 {
		return Auth{}, Fail(p.CodeUnauthenticated, "需要账号认证")
	}
	if e := s.DB.Collection("account_tokens").FindOne(ctx, bson.M{"_id": Hash([]byte(token)), "expires_at": bson.M{"$gt": time.Now()}}).Decode(&t); e != nil {
		return Auth{}, Fail(p.CodeUnauthenticated, "账号认证已过期")
	}
	var a domain.Account
	if e := s.DB.Collection("accounts").FindOne(ctx, bson.M{"_id": t.AccountID, "active": true, "auth_version": t.AuthVersion}).Decode(&a); e != nil {
		return Auth{}, Fail(p.CodeUnauthenticated, "账号授权失效")
	}
	return Auth{AccountID: a.ID, AuthVersion: a.AuthVersion}, nil
}
func (s *Store) Roles(ctx context.Context, a Auth, origin uint64) (*pb.RoleDirectoryResult, error) {
	if origin != s.Cfg.OriginID {
		return nil, Fail(p.CodeRouteChanged, "入口不存在")
	}
	cursor, e := s.DB.Collection("roles").Find(ctx, bson.M{"account_id": a.AccountID, "origin_server_id": origin})
	if e != nil {
		return nil, e
	}
	defer cursor.Close(ctx)
	out := &pb.RoleDirectoryResult{RouteVersion: 1}
	for cursor.Next(ctx) {
		var r domain.Role
		if e = cursor.Decode(&r); e != nil {
			return nil, e
		}
		out.Roles = append(out.Roles, r.Summary())
	}
	return out, cursor.Err()
}
func (s *Store) CreateRole(ctx context.Context, a Auth, q *pb.CreateRoleRequest) (*pb.RoleSummary, error) {
	if q.OriginServerId != s.Cfg.OriginID || q.Slot < 1 || q.Slot > 5 || q.Job > 2 || len(q.OpId) != 16 {
		return nil, Fail(p.CodeInvalidArgument, "角色槽位或操作ID不合法")
	}
	if _, ok := s.Cfg.Fixture.Player(q.Job); !ok {
		return nil, Fail(p.CodeInvalidArgument, "该职业尚无开发fixture")
	}
	name := norm.NFKC.String(strings.TrimSpace(q.Name))
	if len([]rune(name)) < 1 || len([]rune(name)) > 16 {
		return nil, Fail(p.CodeInvalidArgument, "角色名须为1至16字")
	}
	raw, _ := json.Marshal(q)
	fingerprint := Hash(raw)
	opID := "create:" + a.AccountID + ":" + domain.Hex(q.OpId)
	r := domain.Role{ID: domain.ID(), AccountID: a.AccountID, Origin: q.OriginServerId, Slot: q.Slot, Job: q.Job, Gender: 0, Name: name, NameKey: strings.ToLower(name), Level: 1, Revision: 1, Capacity: s.Cfg.Fixture.InventoryCapacity, Wallet: domain.Wallet{BoundGold: s.Cfg.Fixture.InitialBoundGold}, Items: []domain.Item{}, Equipped: []domain.Equipped{}}
	if q.Job == 1 {
		r.Gender = 1
	}
	for _, id := range s.Cfg.Fixture.InitialInventory {
		r.Items = append(r.Items, domain.Item{ID: domain.ID(), ItemID: id, Quantity: 1, Bound: true, Version: 1})
	}
	for _, book := range s.Cfg.Fixture.InitialSkillBooks {
		if book.JobID == q.Job {
			r.Items = append(r.Items, domain.Item{ID: domain.ID(), ItemID: book.ItemID, Quantity: 1, Bound: true, Version: 1})
		}
	}
	var result domain.Role
	e := s.tx(ctx, func(tc context.Context) error {
		if e := s.accountGuard(tc, a); e != nil {
			return e
		}
		var previous Operation
		err := s.DB.Collection("operations").FindOne(tc, bson.M{"_id": opID}).Decode(&previous)
		if err == nil {
			if previous.Digest != fingerprint {
				return Fail(p.CodeOperationConflict, "同操作ID参数变化")
			}
			result, err = s.Role(tc, previous.RoleID)
			return err
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		if _, err = s.DB.Collection("roles").InsertOne(tc, r); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return Fail(p.CodeOperationConflict, "槽位或名字已占用")
			}
			return err
		}
		_, err = s.DB.Collection("operations").InsertOne(tc, Operation{ID: opID, Digest: fingerprint, RoleID: r.ID, Kind: "create_role", Created: time.Now()})
		result = r
		return err
	})
	if e != nil {
		return nil, e
	}
	return result.Summary(), nil
}
func (s *Store) IssueTicket(ctx context.Context, a Auth, key *pb.RoleKey) (*pb.EntryTicketResult, error) {
	if key == nil || len(key.RoleId) != 16 || key.OriginServerId != s.Cfg.OriginID {
		return nil, Fail(p.CodeInvalidArgument, "角色不存在")
	}
	r, e := s.Role(ctx, domain.Hex(key.RoleId))
	if e != nil || r.AccountID != a.AccountID || r.Origin != key.OriginServerId {
		return nil, Fail(p.CodeUnauthenticated, "角色归属不符")
	}
	var run Run
	e = s.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": s.Cfg.RunID, "state": "active", "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}).Decode(&run)
	if e != nil {
		return nil, Fail(p.CodeLineRecovering, "游戏节点尚未就绪")
	}
	token := domain.ID() + domain.ID()
	until := time.Now().Add(45 * time.Second)
	_, e = s.DB.Collection("entry_tickets").InsertOne(ctx, Ticket{ID: Hash([]byte(token)), AccountID: a.AccountID, AuthVersion: a.AuthVersion, RoleID: r.ID, Origin: r.Origin, Run: s.Cfg.RunID, Owner: run.Owner, Epoch: run.Epoch, RouteVersion: 1, Expires: until})
	if e != nil {
		return nil, e
	}
	return &pb.EntryTicketResult{Ticket: token, GameWssUrl: "ws://" + s.Cfg.GameAddress + "/ws", RunServerId: s.Cfg.RunID, RouteVersion: 1, ExpiresAtUnix: uint64(until.Unix())}, nil
}
func (s *Store) TicketRole(ctx context.Context, token string) (Ticket, error) {
	var t Ticket
	if len(token) != 64 {
		return t, Fail(p.CodeUnauthenticated, "入场票据不合法")
	}
	e := s.DB.Collection("entry_tickets").FindOne(ctx, bson.M{"_id": Hash([]byte(token)), "used": false, "expires_at": bson.M{"$gt": time.Now()}}).Decode(&t)
	if e != nil {
		return t, Fail(p.CodeUnauthenticated, "票据过期或已使用")
	}
	return t, nil
}
func (s *Store) Enter(ctx context.Context, token string) (domain.Role, Auth, error) {
	t, e := s.TicketRole(ctx, token)
	if e != nil {
		return domain.Role{}, Auth{}, e
	}
	a := Auth{AccountID: t.AccountID, AuthVersion: t.AuthVersion, RoleID: t.RoleID}
	var r domain.Role
	e = s.tx(ctx, func(tc context.Context) error {
		if t.Owner != s.Instance || t.Epoch != s.Epoch || t.Run != s.Cfg.RunID {
			return Fail(p.CodeRouteChanged, "票据目标运行代次过期")
		}
		if err := s.fence(tc, a); err != nil {
			return err
		}
		used, err := s.DB.Collection("entry_tickets").UpdateOne(tc, bson.M{"_id": t.ID, "used": false, "expires_at": bson.M{"$gt": time.Now()}}, bson.M{"$set": bson.M{"used": true, "used_at": time.Now()}})
		if err != nil {
			return err
		}
		if used.MatchedCount != 1 {
			return Fail(p.CodeUnauthenticated, "票据不可重用")
		}
		r, err = s.Role(tc, t.RoleID)
		if err != nil {
			return err
		}
		old := r.Revision
		r.Owner = s.Instance
		r.RunEpoch = s.Epoch
		r.SessionEpoch++
		r.Revision++
		res, err := s.DB.Collection("roles").ReplaceOne(tc, bson.M{"_id": r.ID, "revision": old, "account_id": t.AccountID}, r)
		if err != nil {
			return err
		}
		if res.MatchedCount != 1 {
			return Fail(p.CodeOperationConflict, "角色接管冲突")
		}
		return nil
	})
	a.SessionEpoch = r.SessionEpoch
	return r, a, e
}
