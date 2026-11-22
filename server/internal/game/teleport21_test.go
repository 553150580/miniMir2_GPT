// 本文件回归瞬移的绑定凭据、耗魔、落点采样、生命、水位和跨地图事件接续。
package game

import (
	"context"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// 只构造已通过Enter的可信内存缓存，不预置Mongo receipt或冒称真实WS证明。
// t 提供断言失败报告、子用例和清理注册。
func teleport21Worker(t *testing.T) (*worker, *room, *capturedConnection, devconfig.Skill, time.Time) {
	t.Helper()
	// w 接收绑定瞬移“21”房间执行器供后续步骤使用，r 接收绑定瞬移“21”房间执行器供后续步骤使用，c 接收绑定瞬移“21”房间执行器供后续步骤使用。
	// s 接收绑定瞬移“21”房间执行器供后续步骤使用，now 保存由绑定瞬移“21”房间执行器取得的当前时间。
	w, r, c, s, now := boundTeleport21Worker(t)
	useLegacySourceTown(&w.e.Store.Cfg.Fixture)
	r.TownWorld = r.newWorld(2)
	return w, r, c, s, now
}

// boundTeleport21Worker 处理绑定瞬移“21”房间执行器。
// t 提供断言失败报告、子用例和清理注册。
func boundTeleport21Worker(t *testing.T) (*worker, *room, *capturedConnection, devconfig.Skill, time.Time) {
	t.Helper()
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 1, 21, 1, 9)
	if w.e.Store.Cfg.Fixture.RuleVersion < 15 {
		t.Fatal("本组需真实唯一Excel rule15及后续的21合同")
	}
	// s 接收技能供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	s, ok := w.e.Store.Cfg.Fixture.Skill(21)
	if !ok || s.Behavior != "teleport" || s.Teleport == nil || s.DelayBaseMS != 200 || s.Teleport.MaxAttempts != 200 {
		t.Fatal("真实来源21合同缺失")
	}
	r.Pet = nil
	r.MapID = w.e.Store.Cfg.Fixture.Map.ID
	r.PersonalWorld = r.currentWorld()
	// id 区分当前步骤指向的目标对象。
	id := domain.ID()
	r.Role.ID, r.Auth.RoleID, r.Role.Origin = id, id, 1
	r.Role.Revision, r.Role.SourceSeq, r.Role.BindingSourceSeq = 15, 7, 7
	r.Role.Binding = &domain.RoleBinding{MapID: 2, X: 650, Y: 631}
	r.Role.BindingSafeZoneID = 106
	r.Player.Life = 9
	w.rooms = map[string]*room{id: r}
	w.e.ctx = context.Background()
	w.e.jobs = make(chan job, 8)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	return w, r, c, s, now
}

// teleport21Rank 处理瞬移“21”排序位次。
// r 指向当前执行器拥有并更新的目标房间，rank 记录排序位次，使用数值表示。
func teleport21Rank(r *room, rank uint32) {
	// i 标记遍历技能集合时访问的下标。
	for i := range r.Role.Skills {
		if r.Role.Skills[i].SkillID == 21 {
			r.Role.Skills[i].Level = rank
			return
		}
	}
	panic("本组缺已学习21")
}

// teleport21Results 返回瞬移“21”结果集合供调用方使用。
// t 提供断言失败报告、子用例和清理注册，c 记录已捕获连接。
func teleport21Results(t *testing.T, c *capturedConnection) []*pb.TeleportResult {
	t.Helper()
	// results 记录结果集合，使用瞬移结果列表表示。
	var results []*pb.TeleportResult
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CTeleportResult {
			// q 保存当前处理所用的瞬移结果。
			q := &pb.TeleportResult{}
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := proto.Unmarshal(frame.Payload, q); err != nil {
				t.Fatal(err)
			}
			results = append(results, q)
		}
	}
	return results
}

// teleport21Last 返回瞬移“21”上一次状态供调用方使用。
// t 提供断言失败报告、子用例和清理注册，c 记录已捕获连接。
func teleport21Last(t *testing.T, c *capturedConnection) *pb.TeleportResult {
	t.Helper()
	// all 保存由瞬移“21”结果集合取得的全部。
	all := teleport21Results(t, c)
	if len(all) == 0 {
		t.Fatal("缺明确已执行/失败结果")
	}
	return all[len(all)-1]
}

// 非方地图仅作为半开随机公式独立模型；它不是新的来源城镇/永久proof。
// t 提供断言失败报告、子用例和清理注册，w 记录房间执行器，r 指向当前执行器拥有并更新的目标房间。
func teleport21Rectangle(t *testing.T, w *worker, r *room) *devconfig.MapTemplate {
	t.Helper()
	// m 接收地图“by”标识供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	m, ok := w.e.Store.Cfg.Fixture.MapByID(2)
	if !ok {
		t.Fatal("缺Town")
	}
	m.Width, m.Height = 13, 9
	// identity 保存当前处理所用的身份。
	identity := *m.Source
	identity.Width, identity.Height = m.Width, m.Height
	m.Source = &identity
	m.CollisionBits = make([]byte, (m.Width*m.Height+7)/8)
	m.SourceStaticBits = make([]byte, len(m.CollisionBits))
	m.SafeZones = []devconfig.SafeZone{{ID: 106, Center: devconfig.Point{}, Radius: 0}, {ID: 107, Center: devconfig.Point{X: 4, Y: 3}, Radius: 0}}
	r.Role.Binding = &domain.RoleBinding{MapID: 2, X: 0, Y: 0}
	if !w.activateWorld(r, r.TownWorld, devconfig.Point{X: 2, Y: 2}, nil) {
		t.Fatal("模型World入场失败")
	}
	return m
}

// TestTeleport21RejectsUnprovedCacheBeforeCostOrDamageRandomness 核对「无可信绑定受理21、先扣费或抽不相关伤害随机」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21RejectsUnprovedCacheBeforeCostOrDamageRandomness(t *testing.T) {
	// kind 接收遍历文本列表时当前的元素。
	for _, kind := range []string{"nil", "legacy_center", "missing_zone", "unknown_map"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(kind, func(t *testing.T) {
			// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，now 保存由瞬移“21”房间执行器取得的当前时间。
			w, r, c, _, now := teleport21Worker(t)
			switch kind {
			case "nil":
				r.Role.Binding = nil
			case "legacy_center":
				r.Role.BindingSourceSeq = 0
			case "missing_zone":
				r.Role.BindingSafeZoneID = 0
			case "unknown_map":
				r.Role.Binding.MapID = 999
			}
			// calls 规定调用集合的初始取值。
			calls := 0
			w.roll = func(int64) int64 { calls++; return 0 }
			// mp 保存当前处理所用的魔法值，source 保存持久收益来源序号，用于当前步骤中的来源，econ 保存经济操作序号，用于当前步骤中的“econ”。
			mp, source, econ := r.Player.MP, r.Role.SourceSeq, r.Role.EconomicSeq
			// code 保存由施法取得的结果码。
			if code := w.cast(r, 21, 0, 1, now); code == 0 || r.Player.MP != mp || calls != 0 || r.eventCount() != 0 || len(c.frames) != 0 || r.Role.SourceSeq != source || r.Role.EconomicSeq != econ {
				t.Fatal("无可信绑定受理21、先扣费或抽不相关伤害随机")
			}
		})
	}
}

// TestTeleport21AcceptedOncePaysMPAndCrossesAt200Milliseconds 核验已核绑定应受理无目标21，用诊断「21不是一次付费/源绑定随机/原Life移交」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21AcceptedOncePaysMPAndCrossesAt200Milliseconds(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，s 接收瞬移“21”房间执行器供后续步骤使用。
	// now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, s, now := teleport21Worker(t)
	// origin 保存位置，用于当前步骤中的大区或起点，scene 保存当前处理所用的场景，mp 保存当前处理所用的魔法值，life 保存当前处理所用的生命代次。
	// source 保存持久收益来源序号，用于当前步骤中的来源，econ 保存经济操作序号，用于当前步骤中的“econ”。
	origin, scene, mp, life, source, econ := r.Player.Pos, r.Scene, r.Player.MP, r.Player.Life, r.Role.SourceSeq, r.Role.EconomicSeq
	// bounds 建立带初始值的数值列表，供本段继续处理。
	bounds := []int64{}
	// n 记录数值。
	w.roll = func(n int64) int64 { bounds = append(bounds, n); return n / 2 }
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 21, 0, 1, now); code != 0 {
		t.Fatal("已核绑定没有受理无目标21", code)
	}
	if r.Player.MP != mp-s.MPBaseCost || r.eventCount() != 1 || len(bounds) != 0 || !r.Casts[0].Due.Equal(now.Add(200*time.Millisecond)) {
		t.Fatal("受理扣MP/200ms队列错误，或抽了伤害RNG")
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond-time.Nanosecond))
	if r.Player.Pos != origin || r.Scene != scene || len(bounds) != 0 {
		t.Fatal("200ms前已传送/抽落点")
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	// result 保存由瞬移“21”上一次状态取得的结果。
	result := teleport21Last(t, c)
	if result.Outcome != 1 || r.MapID != 2 || r.Player.Pos != (devconfig.Point{X: 650, Y: 631}) || r.Player.Life != life || r.Player.MP != mp-s.MPBaseCost || r.Scene != scene+1 || r.Role.SourceSeq != source || r.Role.EconomicSeq != econ || !reflect.DeepEqual(bounds, []int64{1400, 1400}) {
		t.Fatal("21不是一次付费/源绑定随机/原Life移交", result, bounds)
	}
	// status 保存从状态集合取出的状态。
	if status := r.Player.Statuses[s.BuffID]; status == nil || status.RemainingMS != 30000 || status.TeleportManaPenaltyPercent != 30 {
		t.Fatal("成功后才应应用来源30秒TemporalFlux")
	}
	// entry 记录入场，使用入场线路结果表示。
	var entry *pb.EnterLineResult
	// resultIndex 保存当前处理所用的结果下标，entryIndex 保存当前处理所用的入场下标。
	resultIndex, entryIndex := -1, -1
	// i 定位协议帧集合中正在处理的成员，frame 接收遍历协议帧集合时当前的元素。
	for i, frame := range c.frames {
		if frame.ID == p.MsgS2CTeleportResult {
			resultIndex = i
		}
		if frame.ID == p.MsgS2CEnterLine {
			entry = &pb.EnterLineResult{}
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := proto.Unmarshal(frame.Payload, entry); err != nil {
				t.Fatal(err)
			}
			entryIndex = i
		}
	}
	if entry == nil || result.TransitionId == "" || entry.ServerTransitionId != result.TransitionId || entry.SceneEpoch != result.SceneEpoch || result.FromSceneEpoch != scene || entry.RoomId != 2 || resultIndex >= entryIndex || r.Ready {
		t.Fatal("跨图已执行结果未与新Entry资源ACK配对", result, entry)
	}
	// count 记录瞬移“21”结果集合中的条目数。
	count := len(teleport21Results(t, c))
	w.advanceEvents(r, now.Add(time.Second))
	if len(teleport21Results(t, c)) != count || r.Player.MP != mp-s.MPBaseCost {
		t.Fatal("已消费21被后续tick重复执行/扣费")
	}
}

// TestTeleport21CurrentSourceNoTeleportAtDueFailsWithoutRefund 核验到期NoTeleport失败退款、产生成功状态或应确定消费，以及禁传失败不应先抽落点。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21CurrentSourceNoTeleportAtDueFailsWithoutRefund(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，s 接收瞬移“21”房间执行器供后续步骤使用。
	// now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, s, now := teleport21Worker(t)
	// mp 保存当前处理所用的魔法值，origin 保存位置，用于当前步骤中的大区或起点，scene 保存当前处理所用的场景。
	mp, origin, scene := r.Player.MP, r.Player.Pos, r.Scene
	w.roll = func(int64) int64 { t.Fatal("禁传失败不应先抽落点"); return 0 }
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理测试动作")
	}
	// deny 保存当前处理所用的“deny”。
	deny := true
	w.e.Store.Cfg.Fixture.Map.NoTeleport = &deny
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	// result 保存由瞬移“21”上一次状态取得的结果。
	if result := teleport21Last(t, c); result.Outcome != 2 || r.Player.MP != mp-s.MPBaseCost || r.Player.Pos != origin || r.Scene != scene || r.Player.Statuses[s.BuffID] != nil || r.eventCount() != 0 {
		t.Fatal("到期NoTeleport失败退款、产生成功状态或没有确定消费", result)
	}
}

// TestTeleport21DoesNotMistakeDestinationNoTeleportForCurrentMap 核对「目的图NoTeleport被提前当成本次源图禁止」与「到期错误检查目的图NoTeleport」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21DoesNotMistakeDestinationNoTeleportForCurrentMap(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, _, now := teleport21Worker(t)
	// town 保存由地图“by”标识取得的城镇。
	town, _ := w.e.Store.Cfg.Fixture.MapByID(2)
	// deny 保存当前处理所用的“deny”。
	deny := true
	town.NoTeleport = &deny
	// n 记录数值。
	w.roll = func(n int64) int64 { return n / 2 }
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("目的图NoTeleport被提前当成本次源图禁止")
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if teleport21Last(t, c).Outcome != 1 || r.MapID != 2 {
		t.Fatal("到期错误检查目的图NoTeleport")
	}
}

// TestTeleport21HalfOpenIndependentAxesUseNonSquareDimensionsAllRanks 核对「轴半径/上界不含/地图维度错误」与「尚未到期即抽落点/伤害」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21HalfOpenIndependentAxesUseNonSquareDimensionsAllRanks(t *testing.T) {
	// rank 保存由无符号整数“32”取得的排序位次。
	for rank := uint32(0); rank <= 3; rank++ {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(string(rune('0'+rank)), func(t *testing.T) {
			// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，now 保存由瞬移“21”房间执行器取得的当前时间。
			w, r, c, _, now := teleport21Worker(t)
			// m 接收瞬移“21”“rectangle”供后续步骤使用。
			m := teleport21Rectangle(t, w, r)
			teleport21Rank(r, rank)
			// bounds 建立带初始值的数值列表，供本段继续处理。
			bounds := []int64{}
			// n 记录数值。
			w.roll = func(n int64) int64 { bounds = append(bounds, n); return n - 1 }
			if w.cast(r, 21, 0, 1, now) != 0 {
				t.Fatal("未受理矩形来源模型")
			}
			if len(bounds) != 0 {
				t.Fatal("尚未到期即抽落点/伤害")
			}
			w.advanceEvents(r, now.Add(200*time.Millisecond))
			// sx 保存根据宽度与整数计算的“sx”，sy 保存根据高度与整数计算的“sy”。
			sx, sy := m.Width/int(rank+1), m.Height/int(rank+1)
			if !reflect.DeepEqual(bounds, []int64{int64(2 * sx), int64(2 * sy)}) || r.Player.Pos != (devconfig.Point{X: sx - 1, Y: sy - 1}) || teleport21Last(t, c).Outcome != 1 {
				t.Fatal("轴半径/上界不含/地图维度错误", rank, bounds, r.Player.Pos)
			}
		})
	}
}

// TestTeleport21TwoHundredInvalidRepeatedSamplesAreNotClippedOrDeduplicated 核对「越界重复样本未照计200次、预裁为可走格、退款或错加状态」与「未受理耗尽尝试」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21TwoHundredInvalidRepeatedSamplesAreNotClippedOrDeduplicated(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，s 接收瞬移“21”房间执行器供后续步骤使用。
	// now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, s, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	// calls 规定调用集合的初始取值。
	calls := 0
	// n 记录数值。
	w.roll = func(n int64) int64 {
		if calls%2 == 0 && n != 26 || calls%2 == 1 && n != 18 {
			t.Fatal("非方轴混用", n)
		}
		calls++
		return 0
	}
	// origin 保存位置，用于当前步骤中的大区或起点，mp 保存当前处理所用的魔法值，scene 保存当前处理所用的场景。
	origin, mp, scene := r.Player.Pos, r.Player.MP, r.Scene
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理耗尽尝试")
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if calls != 400 || r.Player.Pos != origin || r.Scene != scene || r.Player.MP != mp-s.MPBaseCost || r.Player.Statuses[s.BuffID] != nil || teleport21Last(t, c).Outcome != 3 {
		t.Fatal("越界重复样本未照计200次、预裁为可走格、退款或错加状态", calls)
	}
}

// TestTeleport21ReadsCurrentBindingAndCurrentRankAtDue 核对「21把受理时绑定/rank冻结冒充到期当前值」与「未受理测试动作」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21ReadsCurrentBindingAndCurrentRankAtDue(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，s 接收瞬移“21”房间执行器供后续步骤使用。
	// now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, s, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理测试动作")
	}
	teleport21Rank(r, 3)
	r.Role.Binding = &domain.RoleBinding{MapID: 2, X: 4, Y: 3}
	r.Role.BindingSafeZoneID, r.Role.BindingSourceSeq, r.Role.SourceSeq, r.Role.Revision = 107, 8, 8, 17
	// bounds 建立带初始值的数值列表，供本段继续处理。
	bounds := []int64{}
	// n 记录数值。
	w.roll = func(n int64) int64 { bounds = append(bounds, n); return n / 2 }
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if r.Player.Pos != (devconfig.Point{X: 4, Y: 3}) || !reflect.DeepEqual(bounds, []int64{6, 4}) || teleport21Last(t, c).Outcome != 1 || r.Player.Statuses[s.BuffID] == nil {
		t.Fatal("21把受理时绑定/rank冻结冒充到期当前值", r.Player.Pos, bounds)
	}
}

// TestTeleport21AcceptedSameLifeDeadExecutesButNewDeadCastIsRejected 核验新的死人施法应拒绝，用诊断「到期尸体动作被通用死亡门禁取消或错误复活」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21AcceptedSameLifeDeadExecutesButNewDeadCastIsRejected(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, _, now := teleport21Worker(t)
	// n 记录数值。
	w.roll = func(n int64) int64 { return n / 2 }
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理活角色21")
	}
	// life 保存当前处理所用的生命代次。
	life := r.Player.Life
	r.Player.HP = 0
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if r.Player.HP != 0 || r.Player.Life != life || r.MapID != 2 || teleport21Last(t, c).Outcome != 1 {
		t.Fatal("到期尸体动作被通用死亡门禁取消或错误复活")
	}
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 21, 0, 2, now.Add(10*time.Second)); code == 0 {
		t.Fatal("新的死人施法没有拒绝")
	}
}

// TestTeleport21SameWorldKeepsSceneAndUsesAuthoritativeDisplacement 核对「同World改变了执行/观察身份或没发唯一权威位移」与「同图位移缺权威生命/scene」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21SameWorldKeepsSceneAndUsesAuthoritativeDisplacement(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, _, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	// scene 保存当前处理所用的场景，baseline 保存当前处理所用的场景基线，world 保存由当前状态场景世界取得的场景世界。
	scene, baseline, world := r.Scene, r.Baseline, r.currentWorld()
	// n 记录数值。
	w.roll = func(n int64) int64 { return n / 2 }
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理同图21")
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	// displacements 规定位移集合的初始取值。
	displacements := 0
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CEnterLine {
			t.Fatal("同图瞬移伪造新资源入场")
		}
		if frame.ID == p.MsgS2CEntityDisplacement {
			// q 保存当前处理所用的实体位移。
			q := &pb.EntityDisplacement{}
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := proto.Unmarshal(frame.Payload, q); err != nil {
				t.Fatal(err)
			}
			if q.Kind != movementTeleport || q.SceneEpoch != scene || q.EntityLife != r.Player.Life {
				t.Fatal("同图位移缺权威生命/scene")
			}
			displacements++
		}
	}
	if r.Scene != scene || r.Baseline != baseline || r.currentWorld() != world || !r.Ready || displacements != 1 || r.Player.Pos != (devconfig.Point{}) {
		t.Fatal("同World改变了执行/观察身份或没发唯一权威位移")
	}
}

// TestTeleport21StaticDoorAndOccupiedCellRemainLegalSourceLandings 核验真实来源位图缺闭门阻挡而原静态Walk合法的格子，不能编造门反例，用诊断「来源700维度被换成局部空格/安全区半径」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21StaticDoorAndOccupiedCellRemainLegalSourceLandings(t *testing.T) {
	// kind 接收遍历文本列表时当前的元素。
	for _, kind := range []string{"closed_source_door", "occupied"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(kind, func(t *testing.T) {
			// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，now 保存由瞬移“21”房间执行器取得的当前时间。
			w, r, c, _, now := teleport21Worker(t)
			// m 接收地图“by”标识供后续步骤使用。
			m, _ := w.e.Store.Cfg.Fixture.MapByID(2)
			// goal 建立带初始值的点位，供本段继续处理。
			goal := devconfig.Point{X: 650, Y: 631}
			if kind == "closed_source_door" {
				// found 保存当前处理所用的已找到标记。
				found := false
				// y 规定纵坐标的初始取值。
				for y := 0; y < m.Height && !found; y++ {
					// x 规定横坐标的初始取值。
					for x := 0; x < m.Width; x++ {
						// q 组装点位所需的数据。
						q := devconfig.Point{X: x, Y: y}
						if !m.Walkable(q) && m.TeleportWalkable(q) {
							goal, found = q, true
							break
						}
					}
				}
				if !found {
					t.Fatal("真实来源位图缺闭门阻挡而原静态Walk合法的格子，不能编造门反例")
				}
			}
			if !w.activateWorld(r, r.TownWorld, devconfig.Point{X: 633, Y: 613}, nil) {
				t.Fatal("原Town入场失败")
			}
			if kind == "occupied" {
				// 独立动态占格模型，不声明当前法师可取得道士宠物或Town已开放公共AOI。
				r.Pet = &entity{Handle: 2, Kind: 2, HP: 1, MaxHP: 1, Life: 3, Pos: goal}
			}
			// calls 规定调用集合的初始取值。
			calls := 0
			// n 记录数值。
			w.roll = func(n int64) int64 {
				calls++
				if n != 1400 {
					t.Fatal("来源700维度被换成局部空格/安全区半径", n)
				}
				if calls%2 == 1 {
					return int64(goal.X-650) + n/2
				}
				return int64(goal.Y-631) + n/2
			}
			if w.cast(r, 21, 0, 1, now) != 0 {
				t.Fatal("未受理21")
			}
			w.advanceEvents(r, now.Add(200*time.Millisecond))
			if r.Player.Pos != goal || teleport21Last(t, c).Outcome != 1 || calls != 2 {
				t.Fatal("普通走门/动态占格限制被错误套入源Teleport", goal, r.Player.Pos, calls)
			}
		})
	}
}

// TestTeleport21CurrentMapAtDueIsIndependentOfAcceptanceWorld 核验到期当前World禁止时不应抽样，用诊断「21用接受队列OriginWorld冒充到期当前地图」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21CurrentMapAtDueIsIndependentOfAcceptanceWorld(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, _, now := teleport21Worker(t)
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理21")
	}
	// town 保存由地图“by”标识取得的城镇。
	town, _ := w.e.Store.Cfg.Fixture.MapByID(2)
	// deny 保存当前处理所用的“deny”。
	deny := true
	town.NoTeleport = &deny
	if !w.activateWorld(r, r.TownWorld, devconfig.Point{X: 633, Y: 613}, nil) {
		t.Fatal("已受理动作后的真实执行World变化失败")
	}
	w.roll = func(int64) int64 { t.Fatal("到期当前World禁止时不应抽样"); return 0 }
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if teleport21Last(t, c).Outcome != 2 || r.Player.Pos != (devconfig.Point{X: 633, Y: 613}) {
		t.Fatal("21用接受队列OriginWorld冒充到期当前地图")
	}
}

// TestTeleport21ActualDueNodeAtFirstMiddleLastDoesNotLoseOldWorldSuffix 核对「旧World命中伪装成新Town scene」与「真实21批首/中/末丢后缀，或原地图动作串图/角色动作未按同图条件失败」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21ActualDueNodeAtFirstMiddleLastDoesNotLoseOldWorldSuffix(t *testing.T) {
	// position 接收遍历文本列表时当前的元素。
	for _, position := range []string{"first", "middle", "last"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(position, func(t *testing.T) {
			// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，now 保存由瞬移“21”房间执行器取得的当前时间。
			w, r, c, _, now := teleport21Worker(t)
			// personal 保存个人场景世界，用于当前步骤中的个人。
			personal := r.PersonalWorld
			// monster 保存从怪物集合取出的怪物。
			monster := personal.Monsters[0]
			monster.Pos = stepPoint(r.Player.Pos, 0, 1)
			// n 记录数值。
			w.roll = func(n int64) int64 { return n / 2 }
			if w.cast(r, 21, 0, 1, now) != 0 {
				t.Fatal("真实21受理失败")
			}
			// teleport 保存从施法集合取出的瞬移。
			teleport := personal.Casts[0]
			// fireball 保存由技能取得的“fireball”。
			fireball, _ := w.e.Store.Cfg.Fixture.Skill(1)
			// hellfire 保存由技能取得的“hellfire”。
			hellfire, _ := w.e.Store.Cfg.Fixture.Skill(9)
			// 构造已释放事件的批次时序，仅验证真实21节点；不冒称正常按钮同时施放的输入认证。
			// before 保存由新增取得的操作前的状态，after 保存由新增取得的操作后的状态。
			before, after := teleport.Due.Add(-time.Millisecond), teleport.Due.Add(time.Millisecond)
			// fireDue 保存当前处理所用的“fire”已到期，mapDue 保存由新增取得的地图已到期。
			fireDue, mapDue := after, after.Add(time.Millisecond)
			if position == "middle" {
				fireDue = before
			}
			if position == "last" {
				fireDue, mapDue = before.Add(-time.Millisecond), before
			}
			personal.Casts = []pendingCast{teleport, {Scene: r.Scene, Skill: fireball, Caster: teleport.Caster, Target: monster.Handle, TargetLife: monster.Life, TargetPoint: monster.Pos, Amount: 10, Due: fireDue, ID: 10, Order: 10}, {Scene: r.Scene, Skill: hellfire, Caster: teleport.Caster, TargetPoint: r.Player.Pos, Direction: 0, LineRemaining: 1, Amount: 2, Due: mapDue, ID: 11, Order: 11}}
			w.advanceEvents(r, now.Add(250*time.Millisecond))
			// want 保存由整数“64”取得的期望值。
			want := int64(988)
			if position == "first" {
				want = 998
			}
			if teleport21Last(t, c).Outcome != 1 || r.MapID != 2 || monster.HP != want || r.eventCount() != 0 {
				t.Fatal("真实21批首/中/末丢后缀，或原地图动作串图/角色动作未按同图条件失败", position, monster.HP, want)
			}
			// frame 接收遍历协议帧集合时当前的元素。
			for _, frame := range c.frames {
				if frame.ID == p.MsgS2CCombatImpact {
					// q 保存当前处理所用的战斗命中影响。
					q := &pb.CombatImpact{}
					// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
					if err := proto.Unmarshal(frame.Payload, q); err != nil {
						t.Fatal(err)
					}
					if q.SceneEpoch == r.Scene {
						t.Fatal("旧World命中伪装成新Town scene")
					}
				}
			}
		})
	}
}

// TestTeleport21ManaCostUsesIntegerPenaltyOnlyForTeleportAndRejectsOverflow 核对「TemporalFlux错误增加非传送技能MP」与「来源整数基础费后截断加30%错误」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21ManaCostUsesIntegerPenaltyOnlyForTeleportAndRejectsOverflow(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，skill 保存由瞬移“21”房间执行器取得的技能。
	w, r, _, skill, _ := teleport21Worker(t)
	r.Player.Statuses = map[uint32]*combatStatus{skill.BuffID: {Kind: "teleport_mana_penalty", TargetLife: r.Player.Life, TeleportManaPenaltyPercent: 30}}
	// rank 定位数值列表中正在处理的成员，want 接收遍历数值列表时当前的元素。
	for rank, want := range []int64{13, 16, 20, 24} {
		// got 保存由瞬移魔法值消耗取得的实际结果，ok 表明查询、校验或类型转换是否成功。
		if got, ok := teleportManaCost(&r.Player, skill, uint32(rank)); !ok || got != want {
			t.Fatal("来源整数基础费后截断加30%错误", rank, got, want)
		}
	}
	// fireball 保存由技能取得的“fireball”。
	fireball, _ := w.e.Store.Cfg.Fixture.Skill(1)
	// got 保存由瞬移魔法值消耗取得的实际结果，ok 表明查询、校验或类型转换是否成功。
	if got, ok := teleportManaCost(&r.Player, fireball, 1); !ok || got != fireball.MPBaseCost+fireball.MPLevelCost {
		t.Fatal("TemporalFlux错误增加非传送技能MP")
	}
	// q 接收遍历技能列表时当前的元素。
	for _, q := range []devconfig.Skill{{Behavior: "teleport", MPBaseCost: math.MaxInt64, MPLevelCost: 1}, {Behavior: "teleport", MPBaseCost: math.MaxInt64}, {Behavior: "teleport", MPBaseCost: -1}} {
		// ok 表明查询、校验或类型转换是否成功。
		if _, ok := teleportManaCost(&r.Player, q, 1); ok {
			t.Fatal("整数溢出/负费未拒绝", q)
		}
	}
}

// TestTeleport21FluxRefreshRestartsThirtySecondsWithoutStackingPenalty 核对「重施未重置30秒、叠加惩罚或本次费用使用了新状态」与「已准备好且有足够MP的第二21被拒绝」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21FluxRefreshRestartsThirtySecondsWithoutStackingPenalty(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，skill 保存由瞬移“21”房间执行器取得的技能。
	// now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, skill, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	// n 记录数值。
	w.roll = func(n int64) int64 { return n / 2 }
	// initialMP 保存魔法值，用于当前步骤中的初始值魔法值。
	initialMP := r.Player.MP
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("首21未受理")
	}
	// firstDue 保存由新增取得的第一项已到期。
	firstDue := now.Add(200 * time.Millisecond)
	w.advanceEvents(r, firstDue)
	// first 保存从状态集合取出的第一项。
	first := r.Player.Statuses[skill.BuffID]
	if first == nil {
		t.Fatal("首成功缺状态")
	}
	w.tickStatuses(r, firstDue.Add(25*time.Second))
	// secondAccepted 保存由新增取得的秒已受理。
	secondAccepted := now.Add(26 * time.Second)
	if w.cast(r, 21, 0, 2, secondAccepted) != 0 {
		t.Fatal("已准备好且有足够MP的第二21被拒绝")
	}
	// secondDue 保存由新增取得的秒已到期。
	secondDue := secondAccepted.Add(200 * time.Millisecond)
	w.advanceEvents(r, secondDue)
	// current 保存从状态集合取出的当前状态。
	current := r.Player.Statuses[skill.BuffID]
	if current == nil || current.RemainingMS != 30000 || current.TeleportManaPenaltyPercent != 30 || !current.LastProcess.Equal(secondDue) || r.Player.MP != initialMP-10-13 || teleport21Last(t, c).Outcome != 1 {
		t.Fatal("重施未重置30秒、叠加惩罚或本次费用使用了新状态")
	}
	w.tickStatuses(r, firstDue.Add(30*time.Second))
	if r.Player.Statuses[skill.BuffID] == nil {
		t.Fatal("刷新后仍按旧期限过期")
	}
	w.tickStatuses(r, secondDue.Add(30*time.Second))
	if r.Player.Statuses[skill.BuffID] != nil {
		t.Fatal("新30秒到期仍保留惩罚")
	}
}

// TestTeleport21SameWorldDisplacementStartsAtActualDuePositionAfterPush 核对「21使用受理旧脚点冒充到期权威位移Origin」与「缺同图21位移」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21SameWorldDisplacementStartsAtActualDuePositionAfterPush(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, _, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	// n 记录数值。
	w.roll = func(n int64) int64 { return n / 2 }
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理21")
	}
	// out 保存由来源被推动位移取得的输出。
	if out := w.sourcePushed(r, reference(r, &r.Player), 2, 1, now.Add(100*time.Millisecond), displacementSource{}); out.Steps != 1 {
		t.Fatal("权威推移前态失败", out)
	}
	// origin 保存位置，用于当前步骤中的大区或起点。
	origin := r.Player.Pos
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	// i 保存根据“len”与当前数据计算的当前位置的下标。
	for i := len(c.frames) - 1; i >= 0; i-- {
		if c.frames[i].ID == p.MsgS2CEntityDisplacement {
			// q 保存当前处理所用的实体位移。
			q := &pb.EntityDisplacement{}
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
			if err := proto.Unmarshal(c.frames[i].Payload, q); err != nil {
				t.Fatal(err)
			}
			if q.Kind == movementTeleport {
				if q.Origin.X != int32(origin.X) || q.Origin.Y != int32(origin.Y) {
					t.Fatal("21使用受理旧脚点冒充到期权威位移Origin", q, origin)
				}
				return
			}
		}
	}
	t.Fatal("缺同图21位移")
}

// TestTeleport21TwoHundredthAttemptCanSucceedAndStopsImmediately 核对「第200次合法样本被提前拒绝或成功后继续抽样」与「未受理21」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21TwoHundredthAttemptCanSucceedAndStopsImmediately(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, _, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	// calls 规定调用集合的初始取值。
	calls := 0
	// n 记录数值。
	w.roll = func(n int64) int64 {
		calls++
		if calls <= 398 {
			return 0
		}
		return n / 2
	}
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理21")
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if calls != 400 || teleport21Last(t, c).Outcome != 1 || r.Player.Pos != (devconfig.Point{}) {
		t.Fatal("第200次合法样本被提前拒绝或成功后继续抽样", calls)
	}
}

// TestTeleport21ProofLostAtDueDoesNotRefundOrRetryAsNewCast 核对「到期资格失效未确定失败、退款或补成功状态」与「资格已失效仍抽落点」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21ProofLostAtDueDoesNotRefundOrRetryAsNewCast(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，s 接收瞬移“21”房间执行器供后续步骤使用。
	// now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, s, now := teleport21Worker(t)
	// mp 保存当前处理所用的魔法值，scene 保存当前处理所用的场景。
	mp, scene := r.Player.MP, r.Scene
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理21")
	}
	r.Role.BindingSourceSeq = 0
	w.roll = func(int64) int64 { t.Fatal("资格已失效仍抽落点"); return 0 }
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if teleport21Last(t, c).Outcome != 4 || r.Player.MP != mp-s.MPBaseCost || r.Scene != scene || r.eventCount() != 0 || r.Player.Statuses[s.BuffID] != nil {
		t.Fatal("到期资格失效未确定失败、退款或补成功状态")
	}
}

// TestTeleport21LandingBindingKeepsSelectedProofAndQueuesAfterEarlierOldMapKill 核验前序原World kill应先占唯一永久slot，用诊断「成功新区域landing事实抢号/丢原证明，或为了旧kill延迟传送」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21LandingBindingKeepsSelectedProofAndQueuesAfterEarlierOldMapKill(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，s 接收瞬移“21”房间执行器供后续步骤使用。
	// now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, s, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	// personal 保存个人场景世界，用于当前步骤中的个人。
	personal := r.PersonalWorld
	// monster 保存从怪物集合取出的怪物。
	monster := personal.Monsters[0]
	// hellfire 保存由技能取得的“hellfire”。
	hellfire, _ := w.e.Store.Cfg.Fixture.Skill(9)
	r.CastID, r.ScheduleID = 1, 1
	// n 记录数值。
	w.roll = func(n int64) int64 {
		if n == 26 {
			return 13 + 4
		}
		if n == 18 {
			return 9 + 3
		}
		return 0
	}
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理21")
	}
	// 前缀是原图已付费地图动作，实际击杀后才捕获永久fact；21不等其事务先完成。
	personal.Casts = append(personal.Casts, pendingCast{Scene: r.Scene, Skill: hellfire, Caster: referenceIn(personal, &r.Player), TargetPoint: devconfig.Point{X: monster.Pos.X, Y: monster.Pos.Y + 1}, Direction: 0, LineRemaining: 1, Amount: 1000, Due: now.Add(199 * time.Millisecond), Order: 1, ID: 1})
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	// candidate 保存会话绑定候选，用于当前步骤中的候选。
	candidate := r.BindingCandidate
	if teleport21Last(t, c).Outcome != 1 || r.Player.Pos != (devconfig.Point{X: 4, Y: 3}) || candidate == nil || len(r.KillFacts) != 1 || candidate.order <= r.KillFacts[0].Order || candidate.request.SourceSeq != 0 || candidate.request.ExpectedRevision != 0 || candidate.request.Action != "teleport" || candidate.request.OriginBindingSourceSeq != 7 || candidate.center != (domain.RoleBinding{MapID: 2, X: 4, Y: 3}) || candidate.zoneID != 107 || r.Player.Statuses[s.BuffID] == nil {
		t.Fatal("成功新区域landing事实抢号/丢原证明，或为了旧kill延迟传送", candidate)
	}
	w.flushKillFacts(r)
	if !r.Pending || r.PendingBinding != nil || r.BindingCandidate != candidate || candidate.request.SourceSeq != 0 {
		t.Fatal("前序原World kill没有先占唯一永久slot")
	}
}

// TestTeleport21FixedPenaltyExpiresAtDeadlineBeforeItsNextStatusTick 核对「已过期TemporalFlux仍增加下次费用」与「固定30秒状态被NextProcess拖到下一秒才过期」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21FixedPenaltyExpiresAtDeadlineBeforeItsNextStatusTick(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，s 接收瞬移“21”房间执行器供后续步骤使用，now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, _, s, now := teleport21Worker(t)
	r.Player.Statuses = map[uint32]*combatStatus{s.BuffID: {ID: s.BuffID, Kind: "teleport_mana_penalty", TargetLife: r.Player.Life, TeleportManaPenaltyPercent: 30, RemainingMS: 100, LastProcess: now, NextProcess: now.Add(time.Second), TickMS: 1000}}
	w.tickStatuses(r, now.Add(100*time.Millisecond))
	if r.Player.Statuses[s.BuffID] != nil {
		t.Fatal("固定30秒状态被NextProcess拖到下一秒才过期")
	}
	// cost 保存由瞬移魔法值消耗取得的消耗，ok 表明查询、校验或类型转换是否成功。
	if cost, ok := teleportManaCost(&r.Player, s, 0); !ok || cost != s.MPBaseCost {
		t.Fatal("已过期TemporalFlux仍增加下次费用", cost)
	}
}

// TestTeleport21SourceDoorLandingCanLeaveByNormalGroundNavigation 核对「合法源21落闭门后正常点地无法走向相邻合法出口」与「真实来源无闭门合法落点+相邻可走出口」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTeleport21SourceDoorLandingCanLeaveByNormalGroundNavigation(t *testing.T) {
	// w 接收瞬移“21”房间执行器供后续步骤使用，r 接收瞬移“21”房间执行器供后续步骤使用，c 接收瞬移“21”房间执行器供后续步骤使用，now 保存由瞬移“21”房间执行器取得的当前时间。
	w, r, c, _, now := teleport21Worker(t)
	// m 接收地图“by”标识供后续步骤使用。
	m, _ := w.e.Store.Cfg.Fixture.MapByID(2)
	// door 记录“door”，使用点位表示，exit 记录退出，使用点位表示。
	var door, exit devconfig.Point
	// found 保存当前处理所用的已找到标记。
	found := false
	// y 规定纵坐标的初始取值。
	for y := 0; y < m.Height && !found; y++ {
		// x 规定横坐标的初始取值。
		for x := 0; x < m.Width && !found; x++ {
			// point 组装点位所需的数据。
			point := devconfig.Point{X: x, Y: y}
			if m.Walkable(point) || !m.TeleportWalkable(point) {
				continue
			}
			// d 接收遍历点位列表时当前的元素。
			for _, d := range []devconfig.Point{{X: 1}, {Y: 1}, {X: -1}, {Y: -1}} {
				// next 初始化用作下一状态的点位。
				next := devconfig.Point{X: x + d.X, Y: y + d.Y}
				if m.Walkable(next) {
					door, exit, found = point, next, true
					break
				}
			}
		}
	}
	if !found {
		t.Fatal("真实来源无闭门合法落点+相邻可走出口")
	}
	w.e.workers = []*worker{w}
	// err 接收“initialize”城镇导航过程中可能产生的错误，供后续失败分支处理。
	if err := w.e.initializeTownNavigation(); err != nil {
		t.Fatal(err)
	}
	if !w.activateWorld(r, r.TownWorld, devconfig.Point{X: 633, Y: 613}, nil) {
		t.Fatal("原Town入场失败")
	}
	// calls 规定调用集合的初始取值。
	calls := 0
	// n 记录数值。
	w.roll = func(n int64) int64 {
		calls++
		if calls%2 == 1 {
			return int64(door.X-650) + n/2
		}
		return int64(door.Y-631) + n/2
	}
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理21")
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if r.Player.Pos != door || teleport21Last(t, c).Outcome != 1 {
		t.Fatal("没有走真实来源闭门传送分支")
	}
	w.command(command{role: r.Role.ID, conn: r.Conn, frame: wire.Message(p.MsgC2SMoveIntent, 55, uint64(r.Auth.SessionEpoch), &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 2, Destination: pointPB(exit)})})
	// i 标记循环或列表中的当前位置。
	for i := 1; i <= 8 && r.Player.Pos != exit; i++ {
		// at 保存由新增取得的发生时间。
		at := now.Add(time.Duration(i) * time.Second)
		w.beginTownNavigationTick(at)
		w.step(r, at)
		w.endTownNavigationTick()
	}
	if r.Player.Pos != exit {
		t.Fatalf("合法源21落闭门%v后正常点地无法走向相邻合法出口%v，实际%v、导航%+v", door, exit, r.Player.Pos, r.Navigation)
	}
}
