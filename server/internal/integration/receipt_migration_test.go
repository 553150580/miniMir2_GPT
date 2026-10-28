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
func receiptMigrationDB(t *testing.T) (context.Context, devconfig.Config, *mongo.Database) {
	t.Helper()
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("真实索引迁移需显式MINIMIR_MONGO_E2E=1")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.MongoURI != devconfig.DevelopmentMongoURI() {
		t.Fatal("拒绝连接本任务专属开发Mongo之外的地址")
	}
	cfg.Database = "minimir_dev_game_test_" + domain.ID()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI).SetServerSelectionTimeout(3 * time.Second))
	must(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, client.Database(cfg.Database).Drop(cleanup))
		must(t, client.Disconnect(cleanup))
		cancel()
	})
	must(t, client.Ping(ctx, readpref.Primary()))
	db := client.Database(cfg.Database)
	_, err = db.Collection("application_meta").InsertOne(ctx, bson.M{"_id": "minimir_development", "application": "minimir_first_loop", "schema_version": 1})
	must(t, err)
	return ctx, cfg, db
}

func TestMongoPermanentReceiptIndexMigration(t *testing.T) {
	ctx, cfg, db := receiptMigrationDB(t)
	operations := db.Collection("operations")
	_, err := operations.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "created_at", Value: 1}}, Options: options.Index().SetName("legacy_receipt_expiry").SetExpireAfterSeconds(30 * 24 * 3600)})
	must(t, err)
	// 非目标TTL与复合索引必须保留；缺失expires_at的回执不受此测试TTL删除。
	_, err = operations.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		{Keys: bson.D{{Key: "created_at", Value: 1}, {Key: "kind", Value: 1}}},
	})
	must(t, err)
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
	var before bson.Raw
	must(t, operations.FindOne(ctx, bson.M{"digest": "permanent-before-migration"}).Decode(&before))

	// account/game/gm可同时启动；旧TTL名不能被后来的迁移误当新普通索引删除。
	type opened struct {
		store *persistence.Store
		err   error
	}
	results := make(chan opened, 3)
	start := make(chan struct{})
	for i := 0; i < cap(results); i++ {
		go func() {
			<-start
			store, err := persistence.Open(ctx, cfg)
			results <- opened{store, err}
		}()
	}
	close(start)
	var store *persistence.Store
	var failures []error
	for i := 0; i < cap(results); i++ {
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
	for attempt := 0; attempt < 2; attempt++ {
		must(t, store.Migrate(ctx))
		var after bson.Raw
		must(t, operations.FindOne(ctx, bson.M{"digest": "permanent-before-migration"}).Decode(&after))
		if !bytes.Equal(before, after) {
			t.Fatal("迁移修改了现有永久回执业务文档")
		}
		indexes, err := operations.Indexes().ListSpecifications(ctx)
		must(t, err)
		regular, unrelatedTTL, compound := false, false, false
		for _, index := range indexes {
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
		for _, name := range []string{"skill_quotes", "account_tokens", "entry_tickets"} {
			indexes, err := db.Collection(name).Indexes().ListSpecifications(ctx)
			must(t, err)
			ttl := false
			for _, index := range indexes {
				ttl = ttl || index.Name == "expires_at_1" && index.ExpireAfterSeconds != nil && *index.ExpireAfterSeconds == 0
			}
			if !ttl {
				t.Fatalf("预览或票据的expires_at TTL被修改: %s", name)
			}
			count, err := db.Collection(name).CountDocuments(ctx, bson.M{"_id": "preview_or_ticket"})
			must(t, err)
			if count != 1 {
				t.Fatalf("迁移修改了预览或票据文档: %s", name)
			}
		}
	}
}

func TestMongoReceiptMigrationFailureRejectsStartup(t *testing.T) {
	ctx, cfg, db := receiptMigrationDB(t)
	_, err := db.Collection("operations").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "created_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(30 * 24 * 3600)},
		{Keys: bson.D{{Key: "different_key", Value: 1}}, Options: options.Index().SetName("operations_created_at_retained")},
	})
	must(t, err)
	_, err = db.Collection("operations").InsertOne(ctx, bson.M{"_id": "receipt-must-survive", "created_at": time.Now()})
	must(t, err)
	store, err := persistence.Open(ctx, cfg)
	if err == nil {
		store.Close(context.Background())
		t.Fatal("永久回执索引迁移失败后仍允许启动")
	}
	count, err := db.Collection("operations").CountDocuments(ctx, bson.M{"_id": "receipt-must-survive"})
	must(t, err)
	if count != 1 {
		t.Fatal("失败迁移删除了永久回执")
	}
}
