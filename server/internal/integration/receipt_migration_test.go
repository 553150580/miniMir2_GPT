// 验证 MongoDB 永久回执索引迁移，以及错误配置或迁移失败时拒绝节点启动。

package integration

import (
	"bytes"
	"context"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	"os"
	"testing"
	"time"
)

// 只创建随机专属测试库，不连接来源库，不向统一集成报告写入局部结果。
// t 提供断言失败报告、子用例和清理注册。
func receiptMigrationDB(t *testing.T) (context.Context, devconfig.Config, *mongo.Database) {
	t.Helper()
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("真实索引迁移需显式MINIMIR_MONGO_E2E=1")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.MongoURI != devconfig.DevelopmentMongoURI() {
		t.Fatal("拒绝连接本任务专属开发Mongo之外的地址")
	}
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	// client 保存由“connect”取得的客户端。
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI).SetServerSelectionTimeout(3 * time.Second))
	must(t, err)
	t.Cleanup(func() {
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, client.Database(cfg.Database).Drop(cleanup))
		must(t, client.Disconnect(cleanup))
		cancel()
	})
	must(t, client.Ping(ctx, readpref.Primary()))
	// db 接收数据库供后续步骤使用。
	db := client.Database(cfg.Database)
	_, err = db.Collection("application_meta").InsertOne(ctx, bson.M{"_id": "minimir_development", "application": "minimir_first_loop", "schema_version": 1})
	must(t, err)
	return ctx, cfg, db
}

// TestMongoPermanentReceiptIndexMigration 搭建旧 TTL 索引并验证迁移后永久回执得到保留。
// t 提供断言失败报告、子用例和清理注册。
func TestMongoPermanentReceiptIndexMigration(t *testing.T) {
	// ctx 保存由回执迁移数据库取得的上下文，cfg 保存由回执迁移数据库取得的服务配置，db 保存由回执迁移数据库取得的数据库。
	ctx, cfg, db := receiptMigrationDB(t)
	// operations 保存由集合取得的操作集合。
	operations := db.Collection("operations")
	// err 接收创建“one”过程中可能产生的错误，供后续失败分支处理。
	_, err := operations.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "created_at", Value: 1}}, Options: options.Index().SetName("legacy_receipt_expiry").SetExpireAfterSeconds(30 * 24 * 3600)})
	must(t, err)
	// 非目标TTL与复合索引必须保留；缺失expires_at的回执不受此测试TTL删除。
	_, err = operations.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		{Keys: bson.D{{Key: "created_at", Value: 1}, {Key: "kind", Value: 1}}},
	})
	must(t, err)
	// name 接收遍历文本列表时当前的元素。
	for _, name := range []string{"skill_quotes", "account_tokens", "entry_tickets"} {
		_, err = db.Collection(name).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)})
		must(t, err)
		_, err = db.Collection(name).InsertOne(ctx, bson.M{"_id": "preview_or_ticket", "expires_at": time.Now().Add(time.Hour)})
		must(t, err)
	}
	// 不放入已到TTL截止线的文档，避免TTL后台线程在迁移前删掉它并造成随机测试结果。
	// 本测试证明现存回执原字节保留，不声称已被旧TTL删除的历史可以恢复。
	_, err = operations.InsertOne(ctx, persistence.Operation{ID: domain.ID(), Digest: "permanent-before-migration", RoleID: "role", CommandSeq: 9, Revision: 11, Kind: "learn_skill", Created: time.Now().Add(-29 * 24 * time.Hour).Truncate(time.Millisecond)})
	must(t, err)
	// before 记录操作前的状态，使用原始值表示。
	var before bson.Raw
	must(t, operations.FindOne(ctx, bson.M{"digest": "permanent-before-migration"}).Decode(&before))

	// account/game/gm可同时启动；旧TTL名不能被后来的迁移误当新普通索引删除。
	type opened struct {
		// store 记录持久层存储。
		store *persistence.Store
		// err 保存当前操作的失败原因，供调用方检查或返回。
		err error
	}
	// results 创建用于维护结果集合的容器或通道。
	results := make(chan opened, 3)
	// start 创建用于维护起点或开始时间的容器或通道。
	start := make(chan struct{})
	// i 标记循环或列表中的当前位置。
	for i := 0; i < cap(results); i++ {
		go func() {
			<-start
			// store 保存由打开取得的持久层存储，err 接收打开过程中可能产生的错误，供后续失败分支处理。
			store, err := persistence.Open(ctx, cfg)
			results <- opened{store, err}
		}()
	}
	close(start)
	// store 记录持久层存储。
	var store *persistence.Store
	// failures 记录失败集合，使用错误列表表示。
	var failures []error
	// i 标记循环或列表中的当前位置。
	for i := 0; i < cap(results); i++ {
		// result 接收结果集合通道送来的结果。
		result := <-results
		if result.err != nil {
			failures = append(failures, result.err)
			continue
		}
		store = result.store
		t.Cleanup(func() { result.store.Close(context.Background()) })
	}
	if len(failures) > 0 {
		t.Fatalf("并发启动迁移失败: %v", failures)
	}
	// attempt 规定“attempt”的初始取值。
	for attempt := 0; attempt < 2; attempt++ {
		must(t, store.Migrate(ctx))
		// after 记录操作后的状态，使用原始值表示。
		var after bson.Raw
		must(t, operations.FindOne(ctx, bson.M{"digest": "permanent-before-migration"}).Decode(&after))
		if !bytes.Equal(before, after) {
			t.Fatal("迁移修改了现有永久回执业务文档")
		}
		// indexes 保存由列表“specifications”取得的“indexes”，err 接收列表“specifications”过程中可能产生的错误，供后续失败分支处理。
		indexes, err := operations.Indexes().ListSpecifications(ctx)
		must(t, err)
		// regular 保存当前处理所用的“regular”，unrelatedTTL 保存当前处理所用的“unrelated”有效期限，compound 保存当前处理所用的“compound”。
		regular, unrelatedTTL, compound := false, false, false
		// index 接收遍历“indexes”时当前的元素。
		for _, index := range indexes {
			// keys 保存由“elements”取得的键集合，err 接收“elements”过程中可能产生的错误，供后续失败分支处理。
			keys, err := index.KeysDocument.Elements()
			must(t, err)
			if len(keys) == 1 && keys[0].Key() == "created_at" {
				if index.ExpireAfterSeconds != nil {
					t.Fatal("永久回执created_at仍存在TTL")
				}
				regular = true
			}
			unrelatedTTL = unrelatedTTL || index.Name == "expires_at_1" && index.ExpireAfterSeconds != nil && *index.ExpireAfterSeconds == 0
			compound = compound || index.Name == "created_at_1_kind_1" && index.ExpireAfterSeconds == nil
		}
		if !regular || !unrelatedTTL || !compound {
			t.Fatalf("普通索引缺失或越界修改其他索引: regular=%v unrelatedTTL=%v compound=%v", regular, unrelatedTTL, compound)
		}
		// name 接收遍历文本列表时当前的元素。
		for _, name := range []string{"skill_quotes", "account_tokens", "entry_tickets"} {
			// indexes 保存由列表“specifications”取得的“indexes”，err 接收列表“specifications”过程中可能产生的错误，供后续失败分支处理。
			indexes, err := db.Collection(name).Indexes().ListSpecifications(ctx)
			must(t, err)
			// ttl 保存当前处理所用的有效期限。
			ttl := false
			// index 接收遍历“indexes”时当前的元素。
			for _, index := range indexes {
				ttl = ttl || index.Name == "expires_at_1" && index.ExpireAfterSeconds != nil && *index.ExpireAfterSeconds == 0
			}
			if !ttl {
				t.Fatalf("预览或票据的expires_at TTL被修改: %s", name)
			}
			// count 保存由数量文档取得的数量。
			count, err := db.Collection(name).CountDocuments(ctx, bson.M{"_id": "preview_or_ticket"})
			must(t, err)
			if count != 1 {
				t.Fatalf("迁移修改了预览或票据文档: %s", name)
			}
		}
	}
}

// TestMongoReceiptMigrationFailureRejectsStartup 让永久回执索引迁移失败，核对服务拒绝就绪。
// t 提供断言失败报告、子用例和清理注册。
func TestMongoReceiptMigrationFailureRejectsStartup(t *testing.T) {
	// ctx 保存由回执迁移数据库取得的上下文，cfg 保存由回执迁移数据库取得的服务配置，db 保存由回执迁移数据库取得的数据库。
	ctx, cfg, db := receiptMigrationDB(t)
	// err 接收创建“many”过程中可能产生的错误，供后续失败分支处理。
	_, err := db.Collection("operations").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "created_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(30 * 24 * 3600)},
		{Keys: bson.D{{Key: "different_key", Value: 1}}, Options: options.Index().SetName("operations_created_at_retained")},
	})
	must(t, err)
	_, err = db.Collection("operations").InsertOne(ctx, bson.M{"_id": "receipt-must-survive", "created_at": time.Now()})
	must(t, err)
	// store 保存由打开取得的持久层存储。
	store, err := persistence.Open(ctx, cfg)
	if err == nil {
		store.Close(context.Background())
		t.Fatal("永久回执索引迁移失败后仍允许启动")
	}
	// count 保存由数量文档取得的数量。
	count, err := db.Collection("operations").CountDocuments(ctx, bson.M{"_id": "receipt-must-survive"})
	must(t, err)
	if count != 1 {
		t.Fatal("失败迁移删除了永久回执")
	}
}
