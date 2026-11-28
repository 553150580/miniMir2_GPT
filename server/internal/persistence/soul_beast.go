// 本文件结算魂兽的角色独有奖励、星级、附体和通用碎片。
package persistence

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"slices"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	AssetSoulAction = "soul_action" // 通过原角色资产事务写入的一次确认。
	soulAttach      = uint32(iota)  // 与协议登记的动作编号一致，从一开始。
	soulDetach
	soulSelectAffix
	soulUpgrade
	soulSalvage
	soulSynthesize
	soulGrant
)

// ValidSoulAction 核对原请求形状。收费和收藏修订由事务中的角色状态再次验证。
func ValidSoulAction(q *pb.SoulActionRequest) bool {
	return q != nil && q.Operation != nil && len(q.Operation.OpId) == 16 &&
		len(q.ProtoReflect().GetUnknown()) == 0 && len(q.Operation.ProtoReflect().GetUnknown()) == 0 &&
		q.Action >= soulAttach && q.Action <= soulSynthesize && q.TypeId > 0 && q.TypeId <= 13 &&
		(q.Action == soulSelectAffix) == (q.AffixId != 0) && q.AffixId <= 21 && q.SoulRevision <= math.MaxInt64
}

// commitSoulAction 按收藏修订对拍已确认条款，时序上与普通击杀收益分开判断。
func commitSoulAction(role domain.Role, cfg *devconfig.SoulRules, request AssetRequest) (domain.Role, *domain.SoulReceipt, uint32) {
	if cfg == nil || request.SoulGate {
		return role, nil, p.CodeSoulUnavailable
	}
	q := request.SoulAction
	if !ValidSoulAction(q) || !proto.Equal(q.Operation, request.Context) {
		return role, nil, p.CodeInvalidArgument
	}
	before := role.SoulState()
	if before.Revision == math.MaxInt64 || !before.Valid() || q.SoulRevision != uint64(before.Revision) {
		return role, nil, p.CodeSoulChanged
	}
	kind, configured := cfg.Type(q.TypeId)
	if !configured {
		return role, nil, p.CodeSoulChanged
	}
	index := before.Index(q.TypeId)
	if (q.Action == soulSynthesize) != (index < 0) {
		return role, nil, p.CodeSoulChanged
	}
	after := before.Clone()
	var delta int64
	switch q.Action {
	case soulAttach:
		if after.Attached == q.TypeId {
			return role, nil, p.CodeSoulChanged
		}
		after.Attached = q.TypeId
	case soulDetach:
		if after.Attached != q.TypeId {
			return role, nil, p.CodeSoulChanged
		}
		after.Attached = 0
	case soulSelectAffix:
		affix, found := cfg.Affix(q.AffixId)
		pool := devconfig.SoulAffixPool(q.AffixId)
		if !found || pool < 0 || after.Beasts[index].Affix[pool] == affix.ID {
			return role, nil, p.CodeSoulChanged
		}
		after.Beasts[index].Affix[pool] = affix.ID
	case soulUpgrade:
		star := after.Beasts[index].Star
		if star >= uint32(len(kind.Stars)) {
			return role, nil, p.CodeSoulChanged
		}
		delta = -kind.Stars[star].Upgrade
		after.Beasts[index].Star++
	case soulSalvage:
		if before.Attached == q.TypeId {
			return role, nil, p.CodeSoulChanged
		}
		delta = kind.Stars[after.Beasts[index].Star-1].Salvage
		after.Beasts = slices.Delete(after.Beasts, index, index+1)
	case soulSynthesize:
		delta = -kind.Synthesis
		after.Add(domain.SoulBeast{TypeID: q.TypeId, Star: 1, Affix: cfg.Defaults})
	}
	if q.FragmentDelta != delta || delta < -before.Fragments || delta > math.MaxInt64-before.Fragments {
		return role, nil, p.CodeSoulChanged
	}
	after.Revision++
	after.Fragments += delta
	role.Soul = &after
	return role, &domain.SoulReceipt{Action: q.Action, TypeID: q.TypeId, Delta: delta, Collection: after}, 0
}

// freezeSoulAward 使用原服务端产生的预约身份，事务重试仍得到同一结果。
func freezeSoulAward(cfg *devconfig.SoulRules, drop devconfig.SoulDrop, reservation string) *domain.SoulAward {
	kind, found := cfg.Type(drop.TypeID)
	if !found || drop.ChancePPM == 0 || !lowerHex(reservation, 16) {
		return nil
	}
	digest := sha256.Sum256([]byte("soul_beast:" + reservation))
	if binary.BigEndian.Uint64(digest[:8])%uint64(devconfig.BodyChancePPM) >= uint64(drop.ChancePPM) {
		return nil
	}
	return &domain.SoulAward{TypeID: kind.ID, Duplicate: kind.Duplicate, Defaults: cfg.Defaults}
}

// grantSoulAward 在原胜利或原附件领取事务内直接更新角色，无需背包格。
func grantSoulAward(role domain.Role, award *domain.SoulAward) (domain.Role, *domain.SoulReceipt, error) {
	before := role.SoulState()
	if award == nil || !award.Valid() || !before.Valid() || before.Revision == math.MaxInt64 {
		return role, nil, Fail(p.CodeOperationConflict, "原魂兽奖项或角色收藏状态无效")
	}
	after := before.Clone()
	var delta int64
	if after.Index(award.TypeID) >= 0 {
		delta = award.Duplicate
		if delta > math.MaxInt64-after.Fragments {
			return role, nil, Fail(p.CodeOperationConflict, "魂兽碎片余额超出范围")
		}
		after.Fragments += delta
	} else {
		after.Add(domain.SoulBeast{TypeID: award.TypeID, Star: 1, Affix: award.Defaults})
	}
	after.Revision++
	role.Soul = &after
	return role, &domain.SoulReceipt{Action: soulGrant, TypeID: award.TypeID, Delta: delta, Collection: after}, nil
}
