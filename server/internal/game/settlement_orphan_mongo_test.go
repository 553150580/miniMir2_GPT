// 本文件使用真实隔离 MongoDB 验证孤立回执或审计超过角色来源水位时拒绝结算。
package game

import (
	"encoding/json"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestSettlementRejectsOrphanAheadOfRoleWatermarkMongo 使用真实 MongoDB 核验超前的孤立回执或审计会触发冲突，保留原角色、余额和已存在记录。
// t 提供断言失败报告、子用例和清理注册。
func TestSettlementRejectsOrphanAheadOfRoleWatermarkMongo(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// fixture 保存由创建离线托管MongoDB开发测试配置取得的开发测试配置。
	fixture := newHostingMongoFixture(t, cfg)
	// ctx 保存当前处理所用的上下文，owner 保存当前处理所用的所有者，before 保存角色，用于当前步骤中的操作前的状态。
	ctx, owner, before := fixture.ctx, fixture.owner, fixture.role
	// q 组装结算所需的数据。
	q := persistence.Settlement{SourceSeq: before.SourceSeq + 1, HostingSession: domain.ID(), HostSeconds: 1, Kills: 1, Experience: 3, Gold: 1}
	// raw 保存由序列化取得的原始值。
	raw, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	// id 区分当前步骤指向的目标对象。
	id := fmt.Sprintf("settle:%s:%d", before.ID, q.SourceSeq)
	// key 初始化用作键的MongoDB BSON 文档。
	key := bson.M{"_id": id}
	// test 接收遍历临时结构列表时当前的元素。
	// orphan 记录“orphan”，使用文本表示。
	// counterpart 记录“counterpart”，使用文本表示。
	for _, test := range []struct{ orphan, counterpart string }{
		{"operations", "audit_outbox"}, {"audit_outbox", "operations"},
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(test.orphan, func(t *testing.T) {
			// orphan 保存由集合取得的“orphan”，counterpart 保存由集合取得的“counterpart”。
			orphan, counterpart := owner.DB.Collection(test.orphan), owner.DB.Collection(test.counterpart)
			// 保留同一指纹与真实来源身份，防止把异常回执当成已正常结算。
			// row 初始化用作行记录的MongoDB BSON 文档。
			row := bson.M{"_id": id, "kind": "settlement", "role_id": before.ID, "source_seq": q.SourceSeq, "digest": persistence.Hash(raw), "revision": before.Revision + 1,
				"settlement": persistence.SettlementReceipt{RuleVersion: cfg.Fixture.RuleVersion, HostSeconds: q.HostSeconds, Kills: q.Kills}, "test_orphan": true}
			// err 接收插入“one”过程中可能产生的错误，供后续失败分支处理。
			if _, err := orphan.InsertOne(ctx, row); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				// err 接收“delete”“one”过程中可能产生的错误，供后续失败分支处理。
				if _, err := orphan.DeleteOne(ctx, key); err != nil {
					t.Error(err)
				}
			})
			// receipt 保存由结算带有回执取得的回执，err 接收结算带有回执过程中可能产生的错误，供后续失败分支处理。
			_, receipt, err := owner.SettleWithReceipt(ctx, fixture.roleAuth, q)
			if persistence.Code(err) != p.CodeOperationConflict || receipt.ID != "" {
				t.Fatalf("超前记录应冲突并且不确认收益: err=%v receipt=%+v", err, receipt)
			}
			// after 保存由角色取得的操作后的状态。
			after, err := owner.Role(ctx, before.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.SourceSeq != before.SourceSeq || after.Revision != before.Revision || after.HostingSeconds != before.HostingSeconds ||
				after.Wallet != before.Wallet || after.Experience != before.Experience || after.Report.Sequence != before.Report.Sequence {
				t.Fatal("拒绝超前记录时更改了角色水位、扣时或收益")
			}
			// count 保存由数量文档取得的数量，err 接收数量文档过程中可能产生的错误，供后续失败分支处理。
			if count, err := counterpart.CountDocuments(ctx, key); err != nil || count != 0 {
				t.Fatalf("另一集合的原子写入未回滚: count=%d err=%v", count, err)
			}
			// count 保存由数量文档取得的数量，err 接收数量文档过程中可能产生的错误，供后续失败分支处理。
			if count, err := orphan.CountDocuments(ctx, bson.M{"_id": id, "test_orphan": true}); err != nil || count != 1 {
				t.Fatalf("拒绝了超前记录但更改了原记录: count=%d err=%v", count, err)
			}
		})
	}
}
