// 本文件核对魂兽七星上限、合成、分解以及拒绝请求对原收藏的影响。
package persistence

import (
	"testing"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestSoulSevenStarConservation 验证从一星逐级成长后分解的碎片守恒，同一时刻仅一只附体。
func TestSoulSevenStarConservation(t *testing.T) {
	stars := [7]devconfig.SoulStar{{Salvage: 4}, {Salvage: 7, Upgrade: 6}, {Salvage: 11, Upgrade: 8},
		{Salvage: 16, Upgrade: 10}, {Salvage: 22, Upgrade: 12}, {Salvage: 29, Upgrade: 14}, {Salvage: 37, Upgrade: 16}}
	cfg := &devconfig.SoulRules{Types: []devconfig.SoulType{
		{ID: 1, Synthesis: 12, Stars: stars}, {ID: 2, Synthesis: 12, Stars: stars},
	}, Defaults: [3]uint32{7, 8, 18}}
	role := domain.Role{Soul: &domain.SoulCollection{Fragments: 100}}
	apply := func(action, id uint32, delta int64, code uint32) {
		t.Helper()
		op := &pb.OperationContext{OpId: domain.Bytes(domain.ID())}
		q := &pb.SoulActionRequest{Action: action, TypeId: id, FragmentDelta: delta, Operation: op, SoulRevision: uint64(role.Soul.Revision)}
		next, receipt, actual := commitSoulAction(role, cfg, AssetRequest{Context: op, SoulAction: q})
		if code != actual || (receipt != nil) != (actual == 0) || actual != 0 && next.Soul != role.Soul {
			t.Fatal("魂兽结算违反拒绝原子性或返回错误回执", q, receipt, actual)
		}
		role = next
	}
	apply(soulSynthesize, 1, -12, 0)
	apply(soulSynthesize, 1, -12, p.CodeSoulChanged)
	apply(soulSynthesize, 2, -12, 0)
	apply(soulAttach, 1, 0, 0)
	apply(soulAttach, 2, 0, 0)
	apply(soulDetach, 1, 0, p.CodeSoulChanged)
	apply(soulSalvage, 2, 4, p.CodeSoulChanged)
	for _, star := range stars[1:] {
		apply(soulUpgrade, 1, -star.Upgrade, 0)
	}
	if role.Soul.Beasts[0].Star != 7 || role.Soul.Attached != 2 || role.Soul.Fragments != 10 {
		t.Fatal("没有到达唯一七星和正确的碎片余额", role.Soul)
	}
	apply(soulUpgrade, 1, 0, p.CodeSoulChanged)
	apply(soulSalvage, 1, 37, 0)
	if !role.Soul.Valid() || role.Soul.Fragments != 47 || len(role.Soul.Beasts) != 1 || role.Soul.Beasts[0].TypeID != 2 {
		t.Fatal("分解后的碎片、原收藏和附体不一致", role.Soul)
	}
}
