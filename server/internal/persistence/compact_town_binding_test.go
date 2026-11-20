package persistence

import (
	"strings"
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

func TestCompactTownBindingVerifiesHistoricalReceiptAndRequiresCorrespondingDestination(t *testing.T) {
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Cfg: cfg}
	f := &store.Cfg.Fixture
	legacy, _ := f.MapByID(2)
	town, _ := f.MapByID(3)
	oldRequest := SafeZoneBindingRequest{
		SourceSeq: 9, ExpectedRevision: 10, ExpectedRuleVersion: 23, ExpectedConfigHash: strings.Repeat("e", 64),
		MapID: 2, Position: devconfig.Point{X: 650, Y: 631}, Action: "walk", EventID: strings.Repeat("d", 32),
	}
	currentVersion, currentHash := f.RuleVersion, f.Hash
	f.RuleVersion, f.Hash = oldRequest.ExpectedRuleVersion, oldRequest.ExpectedConfigHash
	oldReceipt, oldDigest, err := store.prepareSafeZoneBinding(oldRequest)
	if err != nil {
		t.Fatal(err)
	}
	f.RuleVersion, f.Hash = currentVersion, currentHash
	role := domain.Role{
		ID: strings.Repeat("c", 32), Job: 1, SourceSeq: 9, EconomicSeq: 2, Revision: 11,
		Binding: &oldReceipt.Point, BindingSourceSeq: 9, BindingSafeZoneID: oldReceipt.ZoneID,
		Skills: []domain.LearnedSkill{{SkillID: 21, Level: 0}},
	}
	oldOperation := Operation{
		ID: bindingOperationID(role.ID, 9), RoleID: role.ID, Kind: safeZoneBindingKind, Digest: oldDigest,
		SourceSeq: 9, Revision: 11, CommandSeq: 2, Binding: oldReceipt,
	}
	if err := store.validateBindingOperation(role, oldOperation); err != nil || oldReceipt.SourceMapSHA256 != legacy.Source.SHA256 || oldReceipt.ZoneID != 106 {
		t.Fatal("rule24未保持原rule23地图2经类型核验的原始凭证", err)
	}
	q := SafeZoneBindingRequest{
		SourceSeq: 10, ExpectedRevision: 11, ExpectedRuleVersion: f.RuleVersion, ExpectedConfigHash: f.Hash,
		MapID: 3, Position: town.SafeZones[0].Center, Action: "teleport", EventID: strings.Repeat("f", 32), OriginBindingSourceSeq: 9,
	}
	if err := store.validateBindingOrigin(role, q); err != nil {
		t.Fatal("已核历史安全区未允许本地紧凑目标", err)
	}
	current, digest, err := store.prepareSafeZoneBinding(q)
	if err != nil || current.Point.MapID != 3 || current.ZoneID != town.SafeZones[0].ID ||
		current.SourceMapSHA256 == oldReceipt.SourceMapSHA256 {
		t.Fatal("紧凑地图的新回执没有独立布局与安全区证明", current, err)
	}
	migrated := role
	migrated.Binding, migrated.BindingSourceSeq, migrated.SourceSeq, migrated.Revision = &current.Point, 10, 10, 12
	currentOperation := Operation{
		ID: bindingOperationID(role.ID, 10), RoleID: role.ID, Kind: safeZoneBindingKind, Digest: digest,
		SourceSeq: 10, Revision: 12, CommandSeq: 2, Binding: current,
	}
	if err := store.validateBindingOperation(migrated, currentOperation); err != nil {
		t.Fatal("新地图凭证无法重新核验", err)
	}
	q.MapID = 4
	if code := Code(store.validateBindingOrigin(role, q)); code != p.CodeLineForbidden {
		t.Fatal("历史银杏证明被冒用到其他城镇", code)
	}
	q.MapID, q.Position = 2, oldRequest.Position
	if _, _, err := store.prepareSafeZoneBinding(q); Code(err) != p.CodeLineForbidden {
		t.Fatal("rule24继续在历史地图创建新的凭证", err)
	}
}
