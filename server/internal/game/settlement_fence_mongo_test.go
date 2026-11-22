// 本文件使用真实隔离 MongoDB 验证结算围栏失效时资产、回执与审计原子回滚。
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

// TestSettlementBulkFenceRollsBackRejectedAuthorityMongo 使用真实 MongoDB 验证运行围栏缺失、版本不符和过期账号授权都会阻止结算，并回滚角色、围栏、回执和审计。
// t 提供断言失败报告、子用例和清理注册。
func TestSettlementBulkFenceRollsBackRejectedAuthorityMongo(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// fixture 保存由创建离线托管MongoDB开发测试配置取得的开发测试配置。
	fixture := newHostingMongoFixture(t, cfg)
	// ctx 保存当前处理所用的上下文，owner 保存当前处理所用的所有者，before 保存角色，用于当前步骤中的操作前的状态。
	ctx, owner, before := fixture.ctx, fixture.owner, fixture.role
	// guards 保存由集合取得的围栏集合，accounts 保存由集合取得的账号集合。
	guards, accounts := owner.DB.Collection("run_guards"), owner.DB.Collection("accounts")
	// accountKey 初始化用作账号键的MongoDB BSON 文档。
	accountKey := bson.M{"_id": fixture.roleAuth.AccountID}
	// 随机库只有一次成功入场，只有该账号的分组在初始化后推进过流水。
	// advanced 建立带初始值的MongoDB BSON 文档，供本段继续处理。
	advanced := bson.M{"seq": bson.M{"$gt": int64(1)}}
	// n 接收数量文档供后续步骤使用，err 接收数量文档过程中可能产生的错误，供后续失败分支处理。
	if n, err := guards.CountDocuments(ctx, advanced); err != nil || n != 1 {
		t.Fatalf("入场后应只有一个运行围栏已推进: n=%d err=%v", n, err)
	}
	// originalGuard 保存由原始值取得的原始围栏。
	originalGuard, err := guards.FindOne(ctx, advanced).Raw()
	if err != nil {
		t.Fatal(err)
	}
	// guardID 标识当前业务使用的围栏，ok 表明查询、校验或类型转换是否成功。
	guardID, ok := originalGuard.Lookup("_id").StringValueOK()
	if !ok {
		t.Fatal("运行围栏缺少有效身份")
	}
	// guardKey 初始化用作围栏键的MongoDB BSON 文档。
	guardKey := bson.M{"_id": guardID}
	// readGuard 封装读取围栏对应的后续处理，t 提供断言失败报告、子用例和清理注册。
	readGuard := func(t *testing.T) bson.Raw {
		t.Helper()
		// raw 接收原始值供后续步骤使用，err 接收原始值过程中可能产生的错误，供后续失败分支处理。
		raw, err := guards.FindOne(ctx, guardKey).Raw()
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	// beforeAccount 保存由原始值取得的操作前的状态账号。
	beforeAccount, err := accounts.FindOne(ctx, accountKey).Raw()
	if err != nil {
		t.Fatal(err)
	}
	// q 组装结算所需的数据。
	q := persistence.Settlement{SourceSeq: before.SourceSeq + 1, HostingSession: domain.ID(), HostSeconds: 1, Kills: 1, Experience: 3, Gold: 1}
	// operationKey 初始化用作操作键的MongoDB BSON 文档。
	operationKey := bson.M{"_id": fmt.Sprintf("settle:%s:%d", before.ID, q.SourceSeq)}
	// assertLedgerCount 封装“assert”“ledger”数量对应的后续处理，t 提供断言失败报告、子用例和清理注册，expected 记录预期结果，使用数值表示。
	assertLedgerCount := func(t *testing.T, expected int64) {
		t.Helper()
		// collection 接收遍历文本列表时当前的元素。
		for _, collection := range []string{"operations", "audit_outbox"} {
			// n 接收数量文档供后续步骤使用，err 接收数量文档过程中可能产生的错误，供后续失败分支处理。
			if n, err := owner.DB.Collection(collection).CountDocuments(ctx, operationKey); err != nil || n != expected {
				t.Fatalf("结算记录数量错误: collection=%s count=%d err=%v", collection, n, err)
			}
		}
	}
	// test 接收遍历临时结构列表时当前的元素。
	for _, test := range []struct {
		// name 记录名称，使用文本表示。
		name string
		// code 记录结果码，使用数值表示。
		code uint32
	}{
		{"运行围栏缺失", p.CodeRouteChanged},
		{"运行围栏版本不符", p.CodeRouteChanged},
		{"账号授权版本过期", p.CodeUnauthenticated},
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(test.name, func(t *testing.T) {
			// auth 保存角色授权，用于当前步骤中的授权。
			auth := fixture.roleAuth
			switch test.name {
			case "运行围栏缺失":
				// err 接收“delete”“one”过程中可能产生的错误，供后续失败分支处理。
				if _, err := guards.DeleteOne(ctx, guardKey); err != nil {
					t.Fatal(err)
				}
			case "运行围栏版本不符":
				// err 接收更新“one”过程中可能产生的错误，供后续失败分支处理。
				if _, err := guards.UpdateOne(ctx, guardKey, bson.M{"$inc": bson.M{"control_version": 1}}); err != nil {
					t.Fatal(err)
				}
			case "账号授权版本过期":
				auth.AuthVersion--
			}
			if test.code == p.CodeRouteChanged {
				t.Cleanup(func() {
					// err 接收替换“one”过程中可能产生的错误，供后续失败分支处理。
					if _, err := guards.ReplaceOne(ctx, guardKey, originalGuard, options.Replace().SetUpsert(true)); err != nil {
						t.Error(err)
					}
				})
			}
			// wantGuard 保存由读取围栏取得的期望值围栏。
			wantGuard := readGuard(t)
			// receipt 保存由结算带有回执取得的回执，err 接收结算带有回执过程中可能产生的错误，供后续失败分支处理。
			_, receipt, err := owner.SettleWithReceipt(ctx, auth, q)
			if persistence.Code(err) != test.code || receipt.ID != "" {
				t.Fatalf("拒绝原因错误或意外产生永久回执: err=%v receipt=%+v", err, receipt)
			}
			if !bytes.Equal(wantGuard, readGuard(t)) {
				t.Fatal("拒绝结算时改写了运行围栏")
			}
			// afterAccount 保存由原始值取得的操作后的状态账号。
			afterAccount, err := accounts.FindOne(ctx, accountKey).Raw()
			if err != nil || !bytes.Equal(beforeAccount, afterAccount) {
				t.Fatalf("拒绝结算时改写了账号围栏: err=%v", err)
			}
			// after 保存由角色取得的操作后的状态。
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
	// settled 保存由结算带有回执取得的“settled”，receipt 保存由结算带有回执取得的回执。
	settled, receipt, err := owner.SettleWithReceipt(ctx, fixture.roleAuth, q)
	if err != nil || receipt.ID == "" || settled.SourceSeq != q.SourceSeq || settled.HostingSeconds != before.HostingSeconds-1 || settled.Report.Kills != 1 {
		t.Fatalf("恢复合法围栏后原来源应成功: err=%v revision=%d receipt=%+v", err, settled.Revision, receipt)
	}
	// replayed 保存由结算带有回执取得的“replayed”，repeated 保存由结算带有回执取得的“repeated”。
	replayed, repeated, err := owner.SettleWithReceipt(ctx, fixture.roleAuth, q)
	if err != nil || repeated.ID != receipt.ID || repeated.Digest != receipt.Digest || replayed.Revision != settled.Revision || replayed.HostingSeconds != settled.HostingSeconds || replayed.Wallet != settled.Wallet {
		t.Fatalf("重试原来源应返回原回执，扣时保持一致: err=%v revision=%d receipt=%+v", err, replayed.Revision, repeated)
	}
	assertLedgerCount(t, 1)
}
