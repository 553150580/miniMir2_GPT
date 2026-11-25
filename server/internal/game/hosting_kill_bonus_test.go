// 本文件用普通怪死亡和延迟命中验证托管卡收益的原时刻归属。
package game

import (
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestHostingKillBonusUsesOriginalImpact 为同一原到期帧核对在线、托管和卡到期三种情况。
func TestHostingKillBonusUsesOriginalImpact(t *testing.T) {
	for _, row := range []struct {
		name    string        // 分支名称。
		offset  time.Duration // 相对于卡到期时刻的原命中时间。
		session string        // 原命中发生时的托管会话，空值表示在线。
		active  bool          // 本次击杀是否享有卡加成。
		legacy  bool          // 原配置是否没有托管权益表。
	}{
		{name: "在线到期前", offset: -time.Millisecond, active: true},
		{name: "离线到期前", offset: -time.Millisecond, session: "原托管会话", active: true},
		{name: "到期等号", session: "原托管会话"},
		{name: "到期之后", offset: time.Millisecond},
		{name: "旧版未配置", offset: -time.Millisecond, legacy: true},
	} {
		t.Run(row.name, func(t *testing.T) {
			w, r, _ := stageAWorker(t, 0)
			f := &w.e.Store.Cfg.Fixture
			r.MapID = f.Map.ID
			f.HostingBenefits = nil
			if !row.legacy {
				f.HostingBenefits = &devconfig.HostingBenefits{KillExperienceBonusBP: 2000, DirectGoldBonusBP: 2000}
			}
			// expiry 是卡到期时刻，hitAt 是已经发生但尚未处理的原命中。
			expiry := time.Unix(1800000000, 0)
			hitAt := expiry.Add(row.offset)
			r.Role.HostingCardUntil = expiry
			m := r.Monsters[0]
			m.HP, m.Monster.Exp, m.Monster.GoldMin, m.Monster.GoldMax = 1, 17, 19, 19
			w.roll = func(limit int64) int64 {
				if limit == 1000000 {
					return int64(f.Drops.EmptyPPM)
				}
				return 0
			}
			w.applyDamage(r, &r.Player, m, 1, expiry.Add(time.Second), hitContext{At: hitAt, HostSession: row.session})
			if len(r.KillFacts) != 1 {
				t.Fatal("原命中未登记击杀事实")
			}
			// 队列中的旧事实不因再次读取卡期限而改判。
			r.Role.HostingCardUntil = expiry.Add(time.Hour)
			q := w.killSettlement(r, r.KillFacts[0])
			if !q.EventAt.Equal(hitAt) || q.HostingSession != row.session || q.Item != nil {
				t.Fatal("普通金掉落、原时刻或托管归属发生变化")
			}
			if row.active {
				bonus := q.HostingKillBonus
				if bonus == nil || !bonus.CardUntil.Equal(expiry) || q.Experience != 20 || q.Gold != 22 ||
					bonus.BaseExperience != 17 || bonus.BonusExperience != 3 || bonus.BaseDirectGold != 19 || bonus.BonusDirectGold != 3 {
					t.Fatalf("原直接收益未按击杀时的卡权益逐次取整：%+v", q)
				}
			} else if q.HostingKillBonus != nil || q.Experience != 17 || q.Gold != 19 {
				t.Fatalf("无有效卡或旧配置取得了额外收益：%+v", q)
			}
		})
	}
}

// TestHostingKillBonusRespectsEquipmentGuarantee 验证真实保底替换了原金币分支后只增加直接经验。
func TestHostingKillBonusRespectsEquipmentGuarantee(t *testing.T) {
	w, r, _ := stageAWorker(t, 0)
	f := &w.e.Store.Cfg.Fixture
	r.MapID = f.Map.ID
	f.HostingBenefits = &devconfig.HostingBenefits{KillExperienceBonusBP: 2000, DirectGoldBonusBP: 2000}
	at := time.Unix(1800000000, 0)
	r.Role.HostingCardUntil = at.Add(time.Second)
	r.Role.NoEquipmentKills = f.Drops.EquipmentGuarantee - 1
	m := r.Monsters[0]
	m.Monster.Exp, m.Monster.GoldMin, m.Monster.GoldMax = 17, 19, 19
	w.roll = func(limit int64) int64 {
		if limit == 1000000 {
			return int64(f.Drops.EmptyPPM)
		}
		return 0
	}
	w.enqueueKillFact(r, m, at, &r.Player, "", time.Time{})
	q := w.killSettlement(r, r.KillFacts[0])
	if q.Item == nil || q.NoEquipmentKills != 0 || q.Experience != 20 ||
		q.HostingKillBonus == nil || q.HostingKillBonus.BaseDirectGold != 0 || q.HostingKillBonus.BonusDirectGold != 0 || q.Gold != 0 {
		t.Fatalf("保底装备、原掉落分类或卡收益计量不符：%+v", q)
	}
}

// TestHostingKillBonusReceiptMongo 核对真实来源事务、到期后查证和原审计分账。
func TestHostingKillBonusReceiptMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要独立的 MongoDB 托管卡收益验收副本集")
	}
	w, r, _ := stageAWorker(t, 0)
	cfg := w.e.Store.Cfg
	cfg.Fixture.HostingBenefits = &devconfig.HostingBenefits{FreeDailySeconds: 21600, CardDailySeconds: 43200,
		KillExperienceBonusBP: 2000, DirectGoldBonusBP: 2000}
	monster := r.Monsters[0]
	definition, found := cfg.Fixture.MonsterByID(monster.Monster.ID)
	if !found {
		t.Fatal("隔离普通图缺少原怪物配置")
	}
	definition.Exp, definition.GoldMin, definition.GoldMax = 17, 19, 19
	monster.Monster = definition
	fixture := newHostingMongoFixture(t, cfg)
	w.e.Store, w.e.ctx = fixture.owner, fixture.ctx
	r.Role, r.Auth, r.MapID = fixture.role, fixture.roleAuth, cfg.Fixture.Map.ID
	w.roll = func(limit int64) int64 {
		if limit == 1000000 {
			return int64(cfg.Fixture.Drops.EmptyPPM)
		}
		return 0
	}
	// 分别模拟已持卡的原击杀和提交时卡已到期的角色记录。
	at := time.Unix(1800000000, 0)
	until := at.Add(time.Second)
	r.Role.HostingCardUntil = until
	w.enqueueKillFact(r, monster, at, &r.Player, domain.ID(), at)
	if len(r.KillFacts) != 1 {
		t.Fatal("原普通怪未登记击杀")
	}
	q := w.killSettlement(r, r.KillFacts[0])
	q.HostSeconds = 1
	filter := bson.M{"_id": r.Role.ID}
	if _, err := fixture.owner.DB.Collection("roles").UpdateOne(fixture.ctx, filter,
		bson.M{"$set": bson.M{"hosting_card_until": at}}); err != nil {
		t.Fatal(err)
	}
	out, op, err := fixture.owner.SettleWithReceipt(fixture.ctx, r.Auth, q)
	if err != nil || q.HostingKillBonus == nil || op.Settlement == nil || op.Settlement.HostingKillBonus == nil ||
		out.Wallet.BoundGold != r.Role.Wallet.BoundGold+22 || out.Report.UsedSeconds != 1 ||
		out.Report.RawExperience != 20 || out.Report.Earned.BoundGold != 22 || out.HostingSeconds != 4 {
		t.Fatalf("原卡奖励没有同托管消费真实入账：结算=%+v、错误=%v", op.Settlement, err)
	}
	var audit struct {
		Settlement persistence.SettlementReceipt `bson:"settlement"` // 同主键原收益审计。
	}
	if err := fixture.owner.DB.Collection("audit_outbox").FindOne(fixture.ctx, bson.M{"_id": op.ID}).Decode(&audit); err != nil {
		t.Fatal(err)
	}
	bonus := audit.Settlement.HostingKillBonus
	if bonus == nil || !bonus.CardUntil.Equal(until) || bonus.BaseExperience != 17 || bonus.BonusExperience != 3 ||
		bonus.BaseDirectGold != 19 || bonus.BonusDirectGold != 3 {
		t.Fatalf("永久审计缺少原期限或原奖励和加成的分账：%+v", bonus)
	}
	replayed, original, err := fixture.owner.SettleWithReceipt(fixture.ctx, r.Auth, q)
	if err != nil || original.ID != op.ID || original.Settlement == nil || original.Settlement.HostingKillBonus == nil ||
		replayed.Wallet != out.Wallet || replayed.SourceSeq != out.SourceSeq || replayed.HostingSeconds != out.HostingSeconds {
		t.Fatalf("原收益确认重查改变了余额或回执：%v", err)
	}
	changed := q
	changed.Gold++
	if _, _, err := fixture.owner.SettleWithReceipt(fixture.ctx, r.Auth, changed); persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("同来源重放允许修改已确认收益", err)
	}
}
