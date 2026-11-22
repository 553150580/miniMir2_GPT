// 仅验证锁定工具/依赖与独立开发库的能力，不实现游戏业务。
// 本文件独立验证 Protobuf 64 位往返、Argon2、WebSocket 依赖和可选的开发副本集事务能力。

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

// main 执行本命令的入口流程。
func main() {
	// uri 保存由文本取得的“uri”。
	uri := flag.String("mongo-uri", "", "可选：仅新建的loopback开发副本集")
	flag.Parse()
	// report 初始化用作报告或战报的按文本索引的任意类型的数据映射。
	report := map[string]any{"go_dependencies": "loaded", "game_business": "not_implemented"}
	// err 接收“check”协议过程中可能产生的错误，供后续失败分支处理。
	if err := checkProto(); err != nil {
		fail(err)
	}
	report["protobuf_uint64_roundtrip"] = "passed"
	// key 保存由标识键取得的键。
	key := argon2.IDKey([]byte("local-smoke-only"), []byte("minimir-smoke-salt"), 1, 8*1024, 1, 32)
	if len(key) != 32 {
		fail(errors.New("Argon2依赖检查失败"))
	}
	report["argon2_dependency"] = "loaded"
	_ = websocket.MessageBinary // 依赖链接与具体版本编译检查；网络协议在正式阶段验收。
	report["websocket_dependency"] = "loaded"
	if *uri != "" {
		// err 接收“check”MongoDB过程中可能产生的错误，供后续失败分支处理。
		if err := checkMongo(*uri, report); err != nil {
			fail(err)
		}
	} else {
		report["mongodb_transaction"] = "not_run"
	}
	// b 保存序列化“indent”返回的结果。
	b, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(b))
}

// fail 处理失败。
// err 保存当前操作的失败原因，供调用方检查或返回。
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(2) }

// checkProto 检查协议。
func checkProto() error {
	// expected 规定预期结果的初始取值。
	const expected uint64 = 9007199254740993
	// msg 保存当前处理所用的“msg”。
	msg := &pb.HeartbeatResult{ServerUnixMs: expected}
	// encoded 保存由序列化取得的“encoded”，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	encoded, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	// result 记录结果，使用“heartbeat”结果表示。
	var result pb.HeartbeatResult
	if err = proto.Unmarshal(encoded, &result); err != nil {
		return err
	}
	if result.ServerUnixMs != expected {
		return errors.New("Protobuf64位精度不一致")
	}
	return nil
}

// checkMongo 检查MongoDB。
// uri 记录“uri”，使用文本表示，report 记录报告或战报，使用按文本索引的任意类型的数据映射表示。
func checkMongo(uri string, report map[string]any) error {
	// u 保存“parse”返回的结果，err 接收“parse”过程中可能产生的错误，供后续失败分支处理。
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "mongodb" || u.Hostname() != "127.0.0.1" || u.User != nil || (u.Path != "" && u.Path != "/") {
		return errors.New("smoke仅连接无真实凭据的127.0.0.1独立开发副本集，不接受现有业务库")
	}
	if uri != devconfig.DevelopmentMongoURI() {
		return errors.New("smoke仅连接当前平台本任务固定端点及directConnection=true")
	}
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// client 保存由“connect”取得的客户端。
	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(10 * time.Second))
	if err != nil {
		return err
	}
	defer client.Disconnect(context.Background())
	if err = client.Ping(ctx, readpref.Primary()); err != nil {
		return err
	}
	// hello 记录“hello”，使用MongoDB BSON 文档表示。
	var hello bson.M
	if err = client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return err
	}
	if hello["setName"] != u.Query().Get("replicaSet") {
		return errors.New("目标不是预期的专属开发副本集")
	}
	// build 记录构建，使用临时结构表示。
	var build struct {
		// Version 记录版本，使用文本表示。
		Version string `bson:"version"`
	}
	if err = client.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build); err != nil {
		return err
	}
	report["mongodb_server_version"] = build.Version
	// name 保存由“sprintf”取得的名称。
	name := fmt.Sprintf("minimir_preflight_%d_%d", os.Getpid(), time.Now().UnixNano())
	if !strings.HasPrefix(name, "minimir_preflight_") {
		return errors.New("测试库名不合法")
	}
	// db 接收数据库供后续步骤使用。
	db := client.Database(name)
	defer func() {
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = db.Drop(cleanup)
	}()
	// wallets 保存由集合取得的钱包集合，operations 保存由集合取得的操作集合。
	wallets, operations := db.Collection("wallets"), db.Collection("operations")
	if _, err = wallets.InsertMany(ctx, []any{bson.M{"_id": "a", "balance": int64(100)}, bson.M{"_id": "b", "balance": int64(0)}}); err != nil {
		return err
	}
	if _, err = operations.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "op_id", Value: 1}}, Options: options.Index().SetUnique(true)}); err != nil {
		return err
	}
	// session 保存由起点或开始时间会话取得的会话。
	session, err := client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())
	// txn 保存由设置写入“concern”取得的“txn”。
	txn := options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority())
	// txctx 记录“txctx”，使用请求或事务上下文表示。
	_, err = session.WithTransaction(ctx, func(txctx context.Context) (any, error) {
		// res 保存由更新“one”取得的结果，e 接收更新“one”供后续步骤使用。
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
	// duplicate 保存由带有“transaction”取得的“duplicate”，txctx 记录“txctx”，使用请求或事务上下文表示。
	_, duplicate := session.WithTransaction(ctx, func(txctx context.Context) (any, error) {
		// e 接收更新“one”供后续步骤使用。
		if _, e := wallets.UpdateOne(txctx, bson.M{"_id": "a"}, bson.M{"$inc": bson.M{"balance": int64(1000)}}); e != nil {
			return nil, e
		}
		// e 接收插入“one”供后续步骤使用。
		_, e := operations.InsertOne(txctx, bson.M{"op_id": "transfer-1"})
		return nil, e
	}, txn)
	if duplicate == nil || !mongo.IsDuplicateKeyError(duplicate) {
		return errors.New("预期唯一操作重复导致事务中止")
	}
	// a 记录a，使用临时结构表示，b 记录b，使用临时结构表示。
	var a, b struct {
		// Balance 记录余额，使用数值表示。
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
