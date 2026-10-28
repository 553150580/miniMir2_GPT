// 仅验证锁定工具/依赖与独立开发库的能力，不实现游戏业务。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
	"golang.org/x/crypto/argon2"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
)

func main() {
	uri := flag.String("mongo-uri", "", "可选：仅新建的loopback开发副本集")
	flag.Parse()
	report := map[string]any{"go_dependencies": "loaded", "game_business": "not_implemented"}
	if err := checkProto(); err != nil {
		fail(err)
	}
	report["protobuf_uint64_roundtrip"] = "passed"
	key := argon2.IDKey([]byte("local-smoke-only"), []byte("minimir-smoke-salt"), 1, 8*1024, 1, 32)
	if len(key) != 32 {
		fail(errors.New("Argon2依赖检查失败"))
	}
	report["argon2_dependency"] = "loaded"
	_ = websocket.MessageBinary // 依赖链接与具体版本编译检查；网络协议在正式阶段验收。
	report["websocket_dependency"] = "loaded"
	if *uri != "" {
		if err := checkMongo(*uri, report); err != nil {
			fail(err)
		}
	} else {
		report["mongodb_transaction"] = "not_run"
	}
	b, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(b))
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(2) }

func checkProto() error {
	const expected uint64 = 9007199254740993
	msg := &pb.HeartbeatResult{ServerUnixMs: expected}
	encoded, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	var result pb.HeartbeatResult
	if err = proto.Unmarshal(encoded, &result); err != nil {
		return err
	}
	if result.ServerUnixMs != expected {
		return errors.New("Protobuf64位精度不一致")
	}
	return nil
}

func checkMongo(uri string, report map[string]any) error {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "mongodb" || u.Hostname() != "127.0.0.1" || u.User != nil || (u.Path != "" && u.Path != "/") {
		return errors.New("smoke仅连接无真实凭据的127.0.0.1独立开发副本集，不接受现有业务库")
	}
	if uri != devconfig.DevelopmentMongoURI() {
		return errors.New("smoke仅连接当前平台本任务固定端点及directConnection=true")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(10 * time.Second))
	if err != nil {
		return err
	}
	defer client.Disconnect(context.Background())
	if err = client.Ping(ctx, readpref.Primary()); err != nil {
		return err
	}
	var hello bson.M
	if err = client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return err
	}
	if hello["setName"] != u.Query().Get("replicaSet") {
		return errors.New("目标不是预期的专属开发副本集")
	}
	var build struct {
		Version string `bson:"version"`
	}
	if err = client.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build); err != nil {
		return err
	}
	report["mongodb_server_version"] = build.Version
	name := fmt.Sprintf("minimir_preflight_%d_%d", os.Getpid(), time.Now().UnixNano())
	if !strings.HasPrefix(name, "minimir_preflight_") {
		return errors.New("测试库名不合法")
	}
	db := client.Database(name)
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = db.Drop(cleanup)
	}()
	wallets, operations := db.Collection("wallets"), db.Collection("operations")
	if _, err = wallets.InsertMany(ctx, []any{bson.M{"_id": "a", "balance": int64(100)}, bson.M{"_id": "b", "balance": int64(0)}}); err != nil {
		return err
	}
	if _, err = operations.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "op_id", Value: 1}}, Options: options.Index().SetUnique(true)}); err != nil {
		return err
	}
	session, err := client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())
	txn := options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority())
	_, err = session.WithTransaction(ctx, func(txctx context.Context) (any, error) {
		res, e := wallets.UpdateOne(txctx, bson.M{"_id": "a", "balance": bson.M{"$gte": int64(30)}}, bson.M{"$inc": bson.M{"balance": int64(-30)}})
		if e != nil {
			return nil, e
		}
		if res.MatchedCount != 1 {
			return nil, errors.New("测试资金条件更新失败")
		}
		if _, e = wallets.UpdateOne(txctx, bson.M{"_id": "b"}, bson.M{"$inc": bson.M{"balance": int64(30)}}); e != nil {
			return nil, e
		}
		_, e = operations.InsertOne(txctx, bson.M{"op_id": "transfer-1"})
		return nil, e
	}, txn)
	if err != nil {
		return err
	}
	_, duplicate := session.WithTransaction(ctx, func(txctx context.Context) (any, error) {
		if _, e := wallets.UpdateOne(txctx, bson.M{"_id": "a"}, bson.M{"$inc": bson.M{"balance": int64(1000)}}); e != nil {
			return nil, e
		}
		_, e := operations.InsertOne(txctx, bson.M{"op_id": "transfer-1"})
		return nil, e
	}, txn)
	if duplicate == nil || !mongo.IsDuplicateKeyError(duplicate) {
		return errors.New("预期唯一操作重复导致事务中止")
	}
	var a, b struct {
		Balance int64 `bson:"balance"`
	}
	if err = wallets.FindOne(ctx, bson.M{"_id": "a"}).Decode(&a); err != nil {
		return err
	}
	if err = wallets.FindOne(ctx, bson.M{"_id": "b"}).Decode(&b); err != nil {
		return err
	}
	if a.Balance != 70 || b.Balance != 30 || a.Balance+b.Balance != 100 {
		return errors.New("事务提交/回滚后测试余额不守恒")
	}
	if err = db.Drop(ctx); err != nil {
		return fmt.Errorf("测试库清理失败: %w", err)
	}
	report["mongodb_transaction"] = "snapshot_majority_commit_and_duplicate_rollback_passed"
	report["mongodb_replica_set"] = hello["setName"]
	report["test_database_cleaned_after_check"] = true
	return nil
}
