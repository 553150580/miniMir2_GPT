package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
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

type Fault struct {
	Code uint32
	Text string
}

func (e *Fault) Error() string            { return e.Text }
func Fail(code uint32, text string) error { return &Fault{code, text} }
func Code(err error) uint32 {
	var f *Fault
	if errors.As(err, &f) {
		return f.Code
	}
	if err != nil {
		return p.CodeServerBusy
	}
	return 0
}

type Store struct {
	Client   *mongo.Client
	Control  *mongo.Client
	DB       *mongo.Database
	Cfg      devconfig.Config
	Instance string
	Epoch    int64
}
type Auth struct {
	AccountID    string
	AuthVersion  int64
	RoleID       string
	SessionEpoch int64
}
type Run struct {
	ID             uint64    `bson:"_id"`
	Owner          string    `bson:"owner_instance"`
	Epoch          int64     `bson:"run_epoch"`
	ControlVersion int64     `bson:"control_version"`
	LeaseUntil     time.Time `bson:"lease_until"`
	State          string    `bson:"state"`
}
type Operation struct {
	ID         string          `bson:"_id"`
	Digest     string          `bson:"digest"`
	RoleID     string          `bson:"role_id"`
	CommandSeq int64           `bson:"command_seq"`
	SourceSeq  int64           `bson:"source_seq,omitempty"`
	Revision   int64           `bson:"revision"`
	Code       uint32          `bson:"code"`
	Kind       string          `bson:"kind"`
	Created    time.Time       `bson:"created_at"`
	Potion     *PotionReceipt  `bson:"potion,omitempty"`
	Casting    *CastingReceipt `bson:"casting_use,omitempty"`
	Binding    *BindingReceipt `bson:"binding_use,omitempty"`
}

// 保存首次受理的消耗身份与效果参数；不保存角色HP/MP、恢复队列或冷却现场。
type PotionReceipt struct {
	Scene  string `bson:"scene_epoch"`
	ItemID uint32 `bson:"item_id"`
	HP     int64  `bson:"restore_hp"`
	MP     int64  `bson:"restore_mp"`
}

func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Open(ctx context.Context, cfg devconfig.Config) (*Store, error) {
	if cfg.Database != "minimir_dev_game" && !strings.HasPrefix(cfg.Database, "minimir_dev_game_test_") {
		return nil, errors.New("拒绝连接非本任务游戏库")
	}
	c, e := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI).SetMaxPoolSize(16).SetServerSelectionTimeout(5 * time.Second))
	if e != nil {
		return nil, e
	}
	if e = c.Ping(ctx, readpref.Primary()); e != nil {
		return nil, e
	}
	ctl, e := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI).SetMaxPoolSize(3))
	if e != nil {
		return nil, e
	}
	s := &Store{Client: c, Control: ctl, DB: c.Database(cfg.Database), Cfg: cfg, Instance: domain.ID()}
	if e = s.Migrate(ctx); e != nil {
		c.Disconnect(ctx)
		ctl.Disconnect(ctx)
		return nil, e
	}
	return s, nil
}
func (s *Store) Close(ctx context.Context) { s.Client.Disconnect(ctx); s.Control.Disconnect(ctx) }
func (s *Store) Migrate(ctx context.Context) error {
	names, e := s.DB.ListCollectionNames(ctx, bson.M{})
	if e != nil {
		return e
	}
	if len(names) > 0 {
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
	indexes := map[string][]mongo.IndexModel{
		"accounts":       {{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true)}},
		"roles":          {{Keys: bson.D{{Key: "account_id", Value: 1}, {Key: "origin_server_id", Value: 1}, {Key: "slot", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "name_key", Value: 1}}, Options: options.Index().SetUnique(true)}},
		"account_tokens": {{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}},
		"entry_tickets":  {{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}},
		"skill_quotes":   {{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}},
	}
	for name, models := range indexes {
		if _, e = s.DB.Collection(name).Indexes().CreateMany(ctx, models); e != nil {
			return e
		}
	}
	_, e = s.DB.Collection("origin_routes").UpdateOne(ctx, bson.M{"_id": s.Cfg.OriginID}, bson.M{"$setOnInsert": bson.M{"run_server_id": s.Cfg.RunID, "node_id": s.Cfg.NodeID, "route_version": int64(1)}}, options.UpdateOne().SetUpsert(true))
	if e != nil {
		return e
	}
	_, e = s.DB.Collection("run_control").UpdateOne(ctx, bson.M{"_id": s.Cfg.RunID}, bson.M{"$setOnInsert": Run{ID: s.Cfg.RunID, State: "stopped", LeaseUntil: time.Unix(0, 0)}}, options.UpdateOne().SetUpsert(true))
	return e
}

// 永久回执不能按时间删除；仅移除本集合created_at单字段TTL，不修改任何业务文档。
func (s *Store) retainOperationReceipts(ctx context.Context) error {
	view := s.DB.Collection("operations").Indexes()
	list := func() ([]mongo.IndexSpecification, error) {
		indexes, err := view.ListSpecifications(ctx)
		var command mongo.CommandError
		if errors.As(err, &command) && command.Code == 26 {
			return nil, nil // 新库尚无operations集合，后面的建索引会创建它。
		}
		return indexes, err
	}
	direction := func(index mongo.IndexSpecification) (int32, error) {
		keys, err := index.KeysDocument.Elements()
		if err != nil {
			return 0, err
		}
		if len(keys) != 1 || keys[0].Key() != "created_at" {
			return 0, nil
		}
		value, ok := keys[0].Value().AsInt64OK()
		if !ok || (value != 1 && value != -1) {
			return 0, nil
		}
		return int32(value), nil
	}
	indexes, err := list()
	if err != nil {
		return err
	}
	legacyNames := map[string]bool{}
	required := map[int32]mongo.IndexSpecification{1: {}}
	for _, index := range indexes {
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
			var command mongo.CommandError
			if !errors.As(err, &command) || command.Code != 27 {
				return err
			}
			// 并发启动可能已删除同一旧TTL；只容忍该明确错误，随后仍核对普通索引。
		}
	}
	for order, previous := range required {
		hasRegular := func() (bool, error) {
			current, err := list()
			if err != nil {
				return false, err
			}
			found := false
			for _, index := range current {
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
		found, err := hasRegular()
		if err != nil {
			return err
		}
		if found {
			continue
		}
		name := "operations_created_at_retained"
		if order < 0 {
			name += "_desc"
		}
		// 不复用旧TTL名称，避免另一启动者拿旧列表误删刚建立的普通索引。
		for legacyNames[name] {
			name += "_retained"
		}
		opts := options.Index().SetName(name)
		if previous.Unique != nil {
			opts.SetUnique(*previous.Unique)
		}
		if previous.Sparse != nil {
			opts.SetSparse(*previous.Sparse)
		}
		if _, err = view.CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "created_at", Value: order}}, Options: opts}); err != nil {
			// 其他模式同时建立同key普通索引时只接受已核实的终态，其余错误使启动失败。
			if found, inspectErr := hasRegular(); inspectErr != nil || !found {
				return err
			}
		}
	}
	return nil
}
func (s *Store) tx(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	sess, e := s.Client.StartSession()
	if e != nil {
		return e
	}
	defer sess.EndSession(context.Background())
	_, e = sess.WithTransaction(ctx, func(tc context.Context) (any, error) { return nil, fn(tc) }, options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))
	if e != nil {
		var se mongo.ServerError
		if errors.As(e, &se) && (se.HasErrorLabel("UnknownTransactionCommitResult") || se.HasErrorLabel("TransientTransactionError")) {
			return Fail(p.CodeOperationPending, "原操作提交待查证")
		}
		if mongo.IsTimeout(e) || mongo.IsNetworkError(e) {
			return Fail(p.CodeOperationPending, "原操作提交待查证")
		}
	}
	return e
}
func (s *Store) Role(ctx context.Context, id string) (domain.Role, error) {
	var r domain.Role
	raw, err := s.DB.Collection("roles").FindOne(ctx, bson.M{"_id": id}).Raw()
	if err != nil {
		return r, err
	}
	if err = checkBindingDocument(raw); err != nil {
		return r, err
	}
	err = bson.Unmarshal(raw, &r)
	if err == nil {
		_, err = s.restoreBindingTrust(ctx, &r)
	}
	return r, err
}
func (s *Store) accountGuard(ctx context.Context, a Auth) error {
	r, e := s.DB.Collection("accounts").UpdateOne(ctx, bson.M{"_id": a.AccountID, "active": true, "auth_version": a.AuthVersion}, bson.M{"$inc": bson.M{"auth_guard_seq": 1}})
	if e != nil {
		return e
	}
	if r.MatchedCount != 1 {
		return Fail(p.CodeUnauthenticated, "账号授权失效")
	}
	return nil
}
func (s *Store) fence(ctx context.Context, a Auth) error {
	filter := bson.M{"_id": s.Cfg.RunID, "owner_instance": s.Instance, "run_epoch": s.Epoch, "state": "active", "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}
	var run Run
	if e := s.DB.Collection("run_control").FindOne(ctx, filter).Decode(&run); e != nil {
		return Fail(p.CodeRouteChanged, "运行所有权已失效")
	}
	var node bson.M
	if e := s.DB.Collection("node_leases").FindOne(ctx, bson.M{"_id": s.Cfg.NodeID, "owner_instance": s.Instance, "$expr": bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}}).Decode(&node); e != nil {
		return Fail(p.CodeRouteChanged, "节点许可已失效")
	}
	raw := domain.Bytes(a.AccountID)
	h := sha256.Sum256(raw)
	group := binary.BigEndian.Uint64(h[:8]) % 32
	r, e := s.DB.Collection("run_guards").UpdateOne(ctx, bson.M{"_id": fmt.Sprintf("%d:%d", s.Cfg.RunID, group), "owner_instance": s.Instance, "run_epoch": s.Epoch, "control_version": run.ControlVersion}, bson.M{"$inc": bson.M{"seq": 1}})
	if e != nil {
		return e
	}
	if r.MatchedCount != 1 {
		return Fail(p.CodeRouteChanged, "写屏障失效")
	}
	return s.accountGuard(ctx, a)
}
func (s *Store) checkRole(r domain.Role, a Auth) error {
	if r.AccountID != a.AccountID || r.Owner != s.Instance || r.RunEpoch != s.Epoch || r.SessionEpoch != a.SessionEpoch {
		return Fail(p.CodeStaleSession, "角色绑定已失效")
	}
	return nil
}
func (s *Store) replace(ctx context.Context, r domain.Role, oldRevision int64) error {
	res, e := s.DB.Collection("roles").ReplaceOne(ctx, bson.M{"_id": r.ID, "revision": oldRevision, "owner_instance": r.Owner, "run_epoch": r.RunEpoch, "session_epoch": r.SessionEpoch}, r)
	if e != nil {
		return e
	}
	if res.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "角色版本变化")
	}
	return nil
}
