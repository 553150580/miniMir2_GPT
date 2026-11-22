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

func TestSettlementRejectsOrphanAheadOfRoleWatermarkMongo(t *testing.T) {
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture := newHostingMongoFixture(t, cfg)
	ctx, owner, before := fixture.ctx, fixture.owner, fixture.role
	q := persistence.Settlement{SourceSeq: before.SourceSeq + 1, HostingSession: domain.ID(), HostSeconds: 1, Kills: 1, Experience: 3, Gold: 1}
	raw, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("settle:%s:%d", before.ID, q.SourceSeq)
	key := bson.M{"_id": id}
	for _, test := range []struct{ orphan, counterpart string }{
		{"operations", "audit_outbox"}, {"audit_outbox", "operations"},
	} {
		t.Run(test.orphan, func(t *testing.T) {
			orphan, counterpart := owner.DB.Collection(test.orphan), owner.DB.Collection(test.counterpart)
			// 保留同一指纹与真实来源身份，防止把异常回执当成已正常结算。
			row := bson.M{"_id": id, "kind": "settlement", "role_id": before.ID, "source_seq": q.SourceSeq, "digest": persistence.Hash(raw), "revision": before.Revision + 1,
				"settlement": persistence.SettlementReceipt{RuleVersion: cfg.Fixture.RuleVersion, HostSeconds: q.HostSeconds, Kills: q.Kills}, "test_orphan": true}
			if _, err := orphan.InsertOne(ctx, row); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := orphan.DeleteOne(ctx, key); err != nil {
					t.Error(err)
				}
			})
			_, receipt, err := owner.SettleWithReceipt(ctx, fixture.roleAuth, q)
			if persistence.Code(err) != p.CodeOperationConflict || receipt.ID != "" {
				t.Fatalf("超前记录应冲突并且不确认收益: err=%v receipt=%+v", err, receipt)
			}
			after, err := owner.Role(ctx, before.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.SourceSeq != before.SourceSeq || after.Revision != before.Revision || after.HostingSeconds != before.HostingSeconds ||
				after.Wallet != before.Wallet || after.Experience != before.Experience || after.Report.Sequence != before.Report.Sequence {
				t.Fatal("拒绝超前记录时更改了角色水位、扣时或收益")
			}
			if count, err := counterpart.CountDocuments(ctx, key); err != nil || count != 0 {
				t.Fatalf("另一集合的原子写入未回滚: count=%d err=%v", count, err)
			}
			if count, err := orphan.CountDocuments(ctx, bson.M{"_id": id, "test_orphan": true}); err != nil || count != 1 {
				t.Fatalf("拒绝了超前记录但更改了原记录: count=%d err=%v", count, err)
			}
		})
	}
}
