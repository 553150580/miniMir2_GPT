package game

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

func TestSettlementBulkFenceRollsBackRejectedAuthorityMongo(t *testing.T) {
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture := newHostingMongoFixture(t, cfg)
	ctx, owner, before := fixture.ctx, fixture.owner, fixture.role
	guards, accounts := owner.DB.Collection("run_guards"), owner.DB.Collection("accounts")
	accountKey := bson.M{"_id": fixture.roleAuth.AccountID}
	// 随机库只有一次成功入场，只有该账号的分组在初始化后推进过流水。
	advanced := bson.M{"seq": bson.M{"$gt": int64(1)}}
	if n, err := guards.CountDocuments(ctx, advanced); err != nil || n != 1 {
		t.Fatalf("入场后应只有一个运行围栏已推进: n=%d err=%v", n, err)
	}
	originalGuard, err := guards.FindOne(ctx, advanced).Raw()
	if err != nil {
		t.Fatal(err)
	}
	guardID, ok := originalGuard.Lookup("_id").StringValueOK()
	if !ok {
		t.Fatal("运行围栏缺少有效身份")
	}
	guardKey := bson.M{"_id": guardID}
	readGuard := func(t *testing.T) bson.Raw {
		t.Helper()
		raw, err := guards.FindOne(ctx, guardKey).Raw()
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	beforeAccount, err := accounts.FindOne(ctx, accountKey).Raw()
	if err != nil {
		t.Fatal(err)
	}
	q := persistence.Settlement{SourceSeq: before.SourceSeq + 1, HostingSession: domain.ID(), HostSeconds: 1, Kills: 1, Experience: 3, Gold: 1}
	operationKey := bson.M{"_id": fmt.Sprintf("settle:%s:%d", before.ID, q.SourceSeq)}
	assertLedgerCount := func(t *testing.T, expected int64) {
		t.Helper()
		for _, collection := range []string{"operations", "audit_outbox"} {
			if n, err := owner.DB.Collection(collection).CountDocuments(ctx, operationKey); err != nil || n != expected {
				t.Fatalf("结算记录数量错误: collection=%s count=%d err=%v", collection, n, err)
			}
		}
	}
	for _, test := range []struct {
		name string
		code uint32
	}{
		{"运行围栏缺失", p.CodeRouteChanged},
		{"运行围栏版本不符", p.CodeRouteChanged},
		{"账号授权版本过期", p.CodeUnauthenticated},
	} {
		t.Run(test.name, func(t *testing.T) {
			auth := fixture.roleAuth
			switch test.name {
			case "运行围栏缺失":
				if _, err := guards.DeleteOne(ctx, guardKey); err != nil {
					t.Fatal(err)
				}
			case "运行围栏版本不符":
				if _, err := guards.UpdateOne(ctx, guardKey, bson.M{"$inc": bson.M{"control_version": 1}}); err != nil {
					t.Fatal(err)
				}
			case "账号授权版本过期":
				auth.AuthVersion--
			}
			if test.code == p.CodeRouteChanged {
				t.Cleanup(func() {
					if _, err := guards.ReplaceOne(ctx, guardKey, originalGuard, options.Replace().SetUpsert(true)); err != nil {
						t.Error(err)
					}
				})
			}
			wantGuard := readGuard(t)
			_, receipt, err := owner.SettleWithReceipt(ctx, auth, q)
			if persistence.Code(err) != test.code || receipt.ID != "" {
				t.Fatalf("拒绝原因错误或意外产生永久回执: err=%v receipt=%+v", err, receipt)
			}
			if !bytes.Equal(wantGuard, readGuard(t)) {
				t.Fatal("拒绝结算时改写了运行围栏")
			}
			afterAccount, err := accounts.FindOne(ctx, accountKey).Raw()
			if err != nil || !bytes.Equal(beforeAccount, afterAccount) {
				t.Fatalf("拒绝结算时改写了账号围栏: err=%v", err)
			}
			after, err := owner.Role(ctx, before.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.SourceSeq != before.SourceSeq || after.Revision != before.Revision || after.HostingSeconds != before.HostingSeconds ||
				after.Wallet != before.Wallet || after.Experience != before.Experience || after.Report.Sequence != before.Report.Sequence {
				t.Fatal("拒绝结算时改写了角色流水、托管时长或收益")
			}
			assertLedgerCount(t, 0)
		})
	}
	settled, receipt, err := owner.SettleWithReceipt(ctx, fixture.roleAuth, q)
	if err != nil || receipt.ID == "" || settled.SourceSeq != q.SourceSeq || settled.HostingSeconds != before.HostingSeconds-1 || settled.Report.Kills != 1 {
		t.Fatalf("恢复合法围栏后原来源应成功: err=%v revision=%d receipt=%+v", err, settled.Revision, receipt)
	}
	replayed, repeated, err := owner.SettleWithReceipt(ctx, fixture.roleAuth, q)
	if err != nil || repeated.ID != receipt.ID || repeated.Digest != receipt.Digest || replayed.Revision != settled.Revision || replayed.HostingSeconds != settled.HostingSeconds || replayed.Wallet != settled.Wallet {
		t.Fatalf("重试原来源应返回原回执，扣时保持一致: err=%v revision=%d receipt=%+v", err, replayed.Revision, repeated)
	}
	assertLedgerCount(t, 1)
}
