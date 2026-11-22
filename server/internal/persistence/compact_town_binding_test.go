// 验证紧凑城镇默认、历史和异常出生绑定在持久层的安全区一致性。

package persistence

import (
	"strings"
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestCompactTownBindingVerifiesHistoricalReceiptAndRequiresCorrespondingDestination 核验紧凑地图的新回执应独立布局与安全区证明，用诊断「rule24继续在历史地图创建新的凭证」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestCompactTownBindingVerifiesHistoricalReceiptAndRequiresCorrespondingDestination(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// store 组装持久层存储所需的数据。
	store := Store{Cfg: cfg}
	// f 保存当前处理所用的开发测试配置。
	f := &store.Cfg.Fixture
	// legacy 保存由地图“by”标识取得的兼容旧数据。
	legacy, _ := f.MapByID(2)
	// town 保存由地图“by”标识取得的城镇。
	town, _ := f.MapByID(3)
	// oldRequest 初始化用作原状态请求的安全区域会话绑定请求。
	oldRequest := SafeZoneBindingRequest{
		SourceSeq: 9, ExpectedRevision: 10, ExpectedRuleVersion: 23, ExpectedConfigHash: strings.Repeat("e", 64),
		MapID: 2, Position: devconfig.Point{X: 650, Y: 631}, Action: "walk", EventID: strings.Repeat("d", 32),
	}
	// currentVersion 保存规则版本，用于当前步骤中的当前状态版本，currentHash 保存摘要，用于当前步骤中的当前状态摘要。
	currentVersion, currentHash := f.RuleVersion, f.Hash
	f.RuleVersion, f.Hash = oldRequest.ExpectedRuleVersion, oldRequest.ExpectedConfigHash
	// oldReceipt 保存由准备安全区域会话绑定取得的原状态回执，oldDigest 保存由准备安全区域会话绑定取得的原状态摘要。
	oldReceipt, oldDigest, err := store.prepareSafeZoneBinding(oldRequest)
	if err != nil {
		t.Fatal(err)
	}
	f.RuleVersion, f.Hash = currentVersion, currentHash
	// role 组装角色所需的数据。
	role := domain.Role{
		ID: strings.Repeat("c", 32), Job: 1, SourceSeq: 9, EconomicSeq: 2, Revision: 11,
		Binding: &oldReceipt.Point, BindingSourceSeq: 9, BindingSafeZoneID: oldReceipt.ZoneID,
		Skills: []domain.LearnedSkill{{SkillID: 21, Level: 0}},
	}
	// oldOperation 组装原状态操作所需的数据。
	oldOperation := Operation{
		ID: bindingOperationID(role.ID, 9), RoleID: role.ID, Kind: safeZoneBindingKind, Digest: oldDigest,
		SourceSeq: 9, Revision: 11, CommandSeq: 2, Binding: oldReceipt,
	}
	// err 接收校验会话绑定操作过程中可能产生的错误，供后续失败分支处理。
	if err := store.validateBindingOperation(role, oldOperation); err != nil || oldReceipt.SourceMapSHA256 != legacy.Source.SHA256 || oldReceipt.ZoneID != 106 {
		t.Fatal("rule24未保持原rule23地图2经类型核验的原始凭证", err)
	}
	// q 组装安全区域会话绑定请求所需的数据。
	q := SafeZoneBindingRequest{
		SourceSeq: 10, ExpectedRevision: 11, ExpectedRuleVersion: f.RuleVersion, ExpectedConfigHash: f.Hash,
		MapID: 3, Position: town.SafeZones[0].Center, Action: "teleport", EventID: strings.Repeat("f", 32), OriginBindingSourceSeq: 9,
	}
	// err 接收校验会话绑定大区或起点过程中可能产生的错误，供后续失败分支处理。
	if err := store.validateBindingOrigin(role, q); err != nil {
		t.Fatal("已核历史安全区未允许本地紧凑目标", err)
	}
	// current 保存由准备安全区域会话绑定取得的当前状态，digest 保存由准备安全区域会话绑定取得的摘要。
	current, digest, err := store.prepareSafeZoneBinding(q)
	if err != nil || current.Point.MapID != 3 || current.ZoneID != town.SafeZones[0].ID ||
		current.SourceMapSHA256 == oldReceipt.SourceMapSHA256 {
		t.Fatal("紧凑地图的新回执没有独立布局与安全区证明", current, err)
	}
	// migrated 保存当前处理所用的“migrated”。
	migrated := role
	migrated.Binding, migrated.BindingSourceSeq, migrated.SourceSeq, migrated.Revision = &current.Point, 10, 10, 12
	// currentOperation 组装当前状态操作所需的数据。
	currentOperation := Operation{
		ID: bindingOperationID(role.ID, 10), RoleID: role.ID, Kind: safeZoneBindingKind, Digest: digest,
		SourceSeq: 10, Revision: 12, CommandSeq: 2, Binding: current,
	}
	// err 接收校验会话绑定操作过程中可能产生的错误，供后续失败分支处理。
	if err := store.validateBindingOperation(migrated, currentOperation); err != nil {
		t.Fatal("新地图凭证无法重新核验", err)
	}
	q.MapID = 4
	// code 接收结果码供后续步骤使用。
	if code := Code(store.validateBindingOrigin(role, q)); code != p.CodeLineForbidden {
		t.Fatal("历史银杏证明被冒用到其他城镇", code)
	}
	q.MapID, q.Position = 2, oldRequest.Position
	// err 接收准备安全区域会话绑定过程中可能产生的错误，供后续失败分支处理。
	if _, _, err := store.prepareSafeZoneBinding(q); Code(err) != p.CodeLineForbidden {
		t.Fatal("rule24继续在历史地图创建新的凭证", err)
	}
}
