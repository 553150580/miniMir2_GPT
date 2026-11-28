// 本文件记录不可交易的魂兽收藏、通用碎片和角色附体。
package domain

import (
	"cmp"
	"slices"

	pb "minimir/api/generated/minimirv1"
)

// SoulBeast 保存同一种魂兽唯一的星级和三池选项。
type SoulBeast struct {
	TypeID uint32    `bson:"type_id" json:"type_id"` // 原名录编号。
	Star   uint32    `bson:"star" json:"star"`       // 当前的一至七星。
	Affix  [3]uint32 `bson:"affixes" json:"affixes"` // 依次为伤害、防守和特效选项。
}

// SoulCollection 是角色聚合的一部分，独立修订允许普通挂机结算并行推进。
type SoulCollection struct {
	Revision  int64       `bson:"revision" json:"revision"`           // 仅魂兽成功操作或实际发奖时增长。
	Fragments int64       `bson:"fragments" json:"fragments"`         // 不进入背包的角色通用碎片。
	Attached  uint32      `bson:"attached,omitempty" json:"attached"` // 零表示当前没有附体。
	Beasts    []SoulBeast `bson:"beasts" json:"beasts"`               // 按类型唯一排序的收藏。
}

// SoulAward 固定原获奖类型和重复折片条款，领取不重新抽样或取价。
type SoulAward struct {
	TypeID    uint32    `bson:"type_id" json:"type_id"`     // 确认掉落的原类型。
	Duplicate int64     `bson:"duplicate" json:"duplicate"` // 对已持有角色折为通用碎片。
	Defaults  [3]uint32 `bson:"defaults" json:"defaults"`   // 首次获得使用原三池配置。
}

// SoulReceipt 保存实际消费或增发的永久结算。
type SoulReceipt struct {
	Action     uint32         `bson:"action"`     // 原动作，7 表示首领掉落。
	TypeID     uint32         `bson:"type_id"`    // 原动作或掉落对象。
	Delta      int64          `bson:"delta"`      // 实际通用碎片变化。
	Collection SoulCollection `bson:"collection"` // 原操作完成时的完整收藏。
}

// SoulState 读出当前状态，缺省支持旧角色。
func (r Role) SoulState() SoulCollection {
	if r.Soul == nil {
		return SoulCollection{}
	}
	return *r.Soul
}

// Clone 复制可写集合，拒绝时不污染原角色聚合。
func (s SoulCollection) Clone() SoulCollection {
	s.Beasts = slices.Clone(s.Beasts)
	return s
}

// Index 查找指定类型，缺省为 -1。
func (s SoulCollection) Index(id uint32) int {
	for index, beast := range s.Beasts {
		if beast.TypeID == id {
			return index
		}
	}
	return -1
}

// Add 将新的、已经过唯一性验证的类型写入稳定排序的收藏。
func (s *SoulCollection) Add(beast SoulBeast) {
	s.Beasts = append(s.Beasts, beast)
	slices.SortFunc(s.Beasts, func(a, b SoulBeast) int { return cmp.Compare(a.TypeID, b.TypeID) })
}

// Valid 核对持久状态，与当前配置关闭、选项池调整相互独立。
func (s SoulCollection) Valid() bool {
	if s.Revision < 0 || s.Fragments < 0 || len(s.Beasts) > 13 || s.Attached != 0 && s.Index(s.Attached) < 0 {
		return false
	}
	var previous uint32
	for _, beast := range s.Beasts {
		if beast.TypeID <= previous || beast.TypeID > 13 || beast.Star == 0 || beast.Star > 7 || !soulPoolsValid(beast.Affix) {
			return false
		}
		previous = beast.TypeID
	}
	return true
}

// Valid 核对已确认奖品的持久条款。
func (a SoulAward) Valid() bool {
	return a.TypeID > 0 && a.TypeID <= 13 && a.Duplicate > 0 && soulPoolsValid(a.Defaults)
}

// soulPoolsValid 保证三个选项各归原一池。
func soulPoolsValid(affix [3]uint32) bool {
	for index, id := range affix {
		low := uint32(7*index + 1)
		if id < low || id > low+6 {
			return false
		}
	}
	return true
}

// Proto 按同一库存水位下发角色收藏。
func (s SoulCollection) Proto() *pb.SoulCollection {
	value := &pb.SoulCollection{Revision: uint64(s.Revision), Fragments: s.Fragments, AttachedTypeId: s.Attached}
	for _, beast := range s.Beasts {
		value.Beasts = append(value.Beasts, &pb.SoulBeast{TypeId: beast.TypeID, Star: beast.Star, AffixIds: slices.Clone(beast.Affix[:])})
	}
	return value
}

// Proto 对已确认原操作提供同一结果。
func (s *SoulReceipt) Proto() *pb.SoulReceipt {
	if s == nil {
		return nil
	}
	return &pb.SoulReceipt{Action: s.Action, TypeId: s.TypeID, FragmentDelta: s.Delta, Collection: s.Collection.Proto()}
}
