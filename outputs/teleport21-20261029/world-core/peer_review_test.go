package game

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
)

// 独立审查探针只通过Go overlay加入测试包，不修改生产源码。
func TestPeerWorldOrderedFactsUnknownAndLateCallbacks(t *testing.T) {
	w, r, _, personal, town, now := worldWorker(t)
	r.Role.BindingSourceSeq, r.Role.BindingSafeZoneID = 3, 106
	center := domain.RoleBinding{MapID: 2, X: 650, Y: 631}
	r.Role.Binding = &center
	point := r.Player.Pos
	w.enqueueKillFactInWorld(r, personal, personal.Monsters[0], now, &r.Player, "", r.Role.PrivilegeUntil)
	first := r.KillFacts[0]
	if !w.commitWorldTransfer(r, testWorldTransfer(r, town), now) {
		t.Fatal("移交被未排号旧kill阻止")
	}
	fact := &safeZoneBindingFact{request: persistence.SafeZoneBindingRequest{MapID: 2, Position: devconfig.Point{X: 650, Y: 631}, Action: "teleport", EventID: domain.ID(), ExpectedRuleVersion: 15, ExpectedConfigHash: w.e.Store.Cfg.Fixture.Hash, OriginBindingSourceSeq: 3}, center: center, zoneID: 106}
	if !w.queueBindingCandidate(r, fact) {
		t.Fatal("不能排入实际动作候选")
	}
	second := *personal.Monsters[0]
	second.Handle++
	w.enqueueKillFactInWorld(r, personal, &second, now, &r.Player, "", r.Role.PrivilegeUntil)
	w.flushKillFacts(r)
	q := w.killSettlement(r, first)
	old := settlementBinding{room: r, auth: r.Auth, world: personal.Ref, point: point, generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
	retry := &job{}
	bad := r.Role
	bad.Revision, bad.SourceSeq = 999, 998
	w.finishSettlement(old, q, bad, persistence.Fail(p.CodeOperationPending, "审查模拟未知结果"), retry)
	if !r.Pending || r.PendingRetry != retry || r.Role.Revision == 999 || fact.request.SourceSeq != 0 || fact.request.OriginBindingSourceSeq != 3 || len(r.KillFacts) != 2 {
		t.Fatal("unknown采用out Role、改原job或给候选抢号")
	}
	role := r.Role
	role.SourceSeq, role.Revision = q.SourceSeq, role.Revision+1
	w.finishSettlement(old, q, role, nil, retry, func() { r.KillFacts = r.KillFacts[1:] })
	if r.PendingBinding != fact || fact.request.SourceSeq != q.SourceSeq+1 || fact.request.ExpectedRevision != role.Revision || fact.request.OriginBindingSourceSeq != 3 || r.KillFacts[0].Prepared {
		t.Fatal("候选未严格在两kill之间按最新永久游标冻结")
	}
	before := fact.request
	w.finishSettlement(old, q, bad, nil, retry)
	if r.PendingBinding != fact || !r.Pending || r.Role.Revision == 999 || fact.request != before {
		t.Fatal("迟到旧kill回调破坏当前binding Pending")
	}
	w.finishSafeZoneBinding(fact, bad, persistence.Operation{}, persistence.Fail(p.CodeServerBusy, "审查模拟入队忙"), retry)
	if r.PendingRetry != retry || fact.request != before || r.Role.Revision == 999 {
		t.Fatal("binding未知结果换原始请求或采用out Role")
	}
	bound, receipt := safeBindingCommitted(r, fact)
	receipt.Binding.Action = "teleport"
	w.finishSafeZoneBinding(fact, bound, receipt, nil, retry)
	if r.PendingBinding != nil || !r.Pending || len(r.KillFacts) != 1 || !r.KillFacts[0].Prepared {
		t.Fatal("binding完成后未轮到后发生的kill")
	}
	id, fp, rev := r.PendingOperationID, r.PendingFingerprint, r.Role.Revision
	w.finishSafeZoneBinding(fact, bad, receipt, nil, retry)
	if !r.Pending || r.PendingOperationID != id || r.PendingFingerprint != fp || r.Role.Revision != rev {
		t.Fatal("迟到binding回调破坏下一kill的Pending")
	}
}

func TestPeerWorldOriginLootDeadlineStableAcrossUnknownAndReplay(t *testing.T) {
	w, r, _, personal, town, now := worldWorker(t)
	point := devconfig.Point{X: 3, Y: 4}
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Item: &domain.Item{ItemID: 218}}
	if !w.commitWorldTransfer(r, testWorldTransfer(r, town), now) {
		t.Fatal("不能先移交Town")
	}
	w.settleInWorld(r, personal, point, q)
	b := settlementBinding{room: r, auth: r.Auth, world: personal.Ref, point: point, generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
	retry := &job{}
	role := r.Role
	role.SourceSeq, role.Revision = q.SourceSeq, role.Revision+1
	w.finishSettlement(b, q, role, persistence.Fail(p.CodeOperationPending, "审查模拟未知结果"), retry)
	if len(personal.Loot) != 0 || len(town.Loot) != 0 {
		t.Fatal("永久未知时已经创建临时掉落")
	}
	w.finishSettlement(b, q, role, nil, retry)
	deadline, loot := personal.LootUntil, personal.Loot
	if len(loot) != 1 || loot[0].Pos != point || len(town.Loot) != 0 {
		t.Fatal("掉落使用当前Town坐标或World")
	}
	w.finishSettlement(b, q, role, nil, retry)
	if personal.LootUntil != deadline || !reflect.DeepEqual(personal.Loot, loot) {
		t.Fatal("重复回调刷新原World期限或重建掉落")
	}
}

func TestPeerWorldOldAuthUnknownCannotClaimNewPending(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "unknown"}[unknown], func(t *testing.T) {
			w, r, _, personal, _, _ := worldWorker(t)
			q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1}
			w.settleInWorld(r, personal, r.Player.Pos, q)
			b := settlementBinding{room: r, auth: r.Auth, world: personal.Ref, generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
			r.Auth.SessionEpoch++
			r.PendingOperationID, r.PendingFingerprint = "current-op", "current-fingerprint"
			current := &job{}
			r.PendingRetry = current
			role := r.Role
			role.Revision++
			var err error
			if unknown {
				err = persistence.Fail(p.CodeOperationPending, "审查模拟未知结果")
			}
			w.finishSettlement(b, q, role, err, &job{})
			if !r.Pending || r.PendingRetry != current || r.PendingOperationID != "current-op" || r.PendingFingerprint != "current-fingerprint" || r.Role.Revision == role.Revision {
				t.Fatal("旧授权成功/未知回调污染当前永久槽")
			}
		})
	}
}
