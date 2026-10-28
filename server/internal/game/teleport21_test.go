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
func teleport21Worker(t *testing.T) (*worker, *room, *capturedConnection, devconfig.Skill, time.Time) {
	t.Helper()
	w, r, c := stageBWorker(t, 1, 21, 1, 9)
	if w.e.Store.Cfg.Fixture.RuleVersion != 15 {
		t.Fatal("本组需真实唯一Excel rule15输入")
	}
	s, ok := w.e.Store.Cfg.Fixture.Skill(21)
	if !ok || s.Behavior != "teleport" || s.Teleport == nil || s.DelayBaseMS != 200 || s.Teleport.MaxAttempts != 200 {
		t.Fatal("真实来源21合同缺失")
	}
	r.Pet = nil
	r.MapID = w.e.Store.Cfg.Fixture.Map.ID
	r.PersonalWorld = r.currentWorld()
	r.TownWorld = r.newWorld(2)
	id := domain.ID()
	r.Role.ID, r.Auth.RoleID, r.Role.Origin = id, id, 1
	r.Role.Revision, r.Role.SourceSeq, r.Role.BindingSourceSeq = 15, 7, 7
	r.Role.Binding = &domain.RoleBinding{MapID: 2, X: 650, Y: 631}
	r.Role.BindingSafeZoneID = 106
	r.Player.Life = 9
	w.rooms = map[string]*room{id: r}
	w.e.ctx = context.Background()
	w.e.jobs = make(chan job, 8)
	now := time.Unix(1800000000, 0)
	return w, r, c, s, now
}

func teleport21Rank(r *room, rank uint32) {
	for i := range r.Role.Skills {
		if r.Role.Skills[i].SkillID == 21 {
			r.Role.Skills[i].Level = rank
			return
		}
	}
	panic("本组缺已学习21")
}

func teleport21Results(t *testing.T, c *capturedConnection) []*pb.TeleportResult {
	t.Helper()
	var results []*pb.TeleportResult
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CTeleportResult {
			q := &pb.TeleportResult{}
			if err := proto.Unmarshal(frame.Payload, q); err != nil {
				t.Fatal(err)
			}
			results = append(results, q)
		}
	}
	return results
}

func teleport21Last(t *testing.T, c *capturedConnection) *pb.TeleportResult {
	t.Helper()
	all := teleport21Results(t, c)
	if len(all) == 0 {
		t.Fatal("缺明确已执行/失败结果")
	}
	return all[len(all)-1]
}

// 非方地图仅作为半开随机公式独立模型；它不是新的来源城镇/永久proof。
func teleport21Rectangle(t *testing.T, w *worker, r *room) *devconfig.MapTemplate {
	t.Helper()
	m, ok := w.e.Store.Cfg.Fixture.MapByID(2)
	if !ok {
		t.Fatal("缺Town")
	}
	m.Width, m.Height = 13, 9
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

func TestTeleport21RejectsUnprovedCacheBeforeCostOrDamageRandomness(t *testing.T) {
	for _, kind := range []string{"nil", "legacy_center", "missing_zone", "unknown_map"} {
		t.Run(kind, func(t *testing.T) {
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
			calls := 0
			w.roll = func(int64) int64 { calls++; return 0 }
			mp, source, econ := r.Player.MP, r.Role.SourceSeq, r.Role.EconomicSeq
			if code := w.cast(r, 21, 0, 1, now); code == 0 || r.Player.MP != mp || calls != 0 || r.eventCount() != 0 || len(c.frames) != 0 || r.Role.SourceSeq != source || r.Role.EconomicSeq != econ {
				t.Fatal("无可信绑定受理21、先扣费或抽不相关伤害随机")
			}
		})
	}
}

func TestTeleport21AcceptedOncePaysMPAndCrossesAt200Milliseconds(t *testing.T) {
	w, r, c, s, now := teleport21Worker(t)
	origin, scene, mp, life, source, econ := r.Player.Pos, r.Scene, r.Player.MP, r.Player.Life, r.Role.SourceSeq, r.Role.EconomicSeq
	bounds := []int64{}
	w.roll = func(n int64) int64 { bounds = append(bounds, n); return n / 2 }
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
	result := teleport21Last(t, c)
	if result.Outcome != 1 || r.MapID != 2 || r.Player.Pos != (devconfig.Point{X: 650, Y: 631}) || r.Player.Life != life || r.Player.MP != mp-s.MPBaseCost || r.Scene != scene+1 || r.Role.SourceSeq != source || r.Role.EconomicSeq != econ || !reflect.DeepEqual(bounds, []int64{1400, 1400}) {
		t.Fatal("21不是一次付费/源绑定随机/原Life移交", result, bounds)
	}
	if status := r.Player.Statuses[s.BuffID]; status == nil || status.RemainingMS != 30000 || status.TeleportManaPenaltyPercent != 30 {
		t.Fatal("成功后才应应用来源30秒TemporalFlux")
	}
	var entry *pb.EnterLineResult
	resultIndex, entryIndex := -1, -1
	for i, frame := range c.frames {
		if frame.ID == p.MsgS2CTeleportResult {
			resultIndex = i
		}
		if frame.ID == p.MsgS2CEnterLine {
			entry = &pb.EnterLineResult{}
			if err := proto.Unmarshal(frame.Payload, entry); err != nil {
				t.Fatal(err)
			}
			entryIndex = i
		}
	}
	if entry == nil || result.TransitionId == "" || entry.ServerTransitionId != result.TransitionId || entry.SceneEpoch != result.SceneEpoch || result.FromSceneEpoch != scene || entry.RoomId != 2 || resultIndex >= entryIndex || r.Ready {
		t.Fatal("跨图已执行结果未与新Entry资源ACK配对", result, entry)
	}
	count := len(teleport21Results(t, c))
	w.advanceEvents(r, now.Add(time.Second))
	if len(teleport21Results(t, c)) != count || r.Player.MP != mp-s.MPBaseCost {
		t.Fatal("已消费21被后续tick重复执行/扣费")
	}
}

func TestTeleport21CurrentSourceNoTeleportAtDueFailsWithoutRefund(t *testing.T) {
	w, r, c, s, now := teleport21Worker(t)
	mp, origin, scene := r.Player.MP, r.Player.Pos, r.Scene
	w.roll = func(int64) int64 { t.Fatal("禁传失败不应先抽落点"); return 0 }
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理测试动作")
	}
	deny := true
	w.e.Store.Cfg.Fixture.Map.NoTeleport = &deny
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if result := teleport21Last(t, c); result.Outcome != 2 || r.Player.MP != mp-s.MPBaseCost || r.Player.Pos != origin || r.Scene != scene || r.Player.Statuses[s.BuffID] != nil || r.eventCount() != 0 {
		t.Fatal("到期NoTeleport失败退款、产生成功状态或没有确定消费", result)
	}
}

func TestTeleport21DoesNotMistakeDestinationNoTeleportForCurrentMap(t *testing.T) {
	w, r, c, _, now := teleport21Worker(t)
	town, _ := w.e.Store.Cfg.Fixture.MapByID(2)
	deny := true
	town.NoTeleport = &deny
	w.roll = func(n int64) int64 { return n / 2 }
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("目的图NoTeleport被提前当成本次源图禁止")
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if teleport21Last(t, c).Outcome != 1 || r.MapID != 2 {
		t.Fatal("到期错误检查目的图NoTeleport")
	}
}

func TestTeleport21HalfOpenIndependentAxesUseNonSquareDimensionsAllRanks(t *testing.T) {
	for rank := uint32(0); rank <= 3; rank++ {
		t.Run(string(rune('0'+rank)), func(t *testing.T) {
			w, r, c, _, now := teleport21Worker(t)
			m := teleport21Rectangle(t, w, r)
			teleport21Rank(r, rank)
			bounds := []int64{}
			w.roll = func(n int64) int64 { bounds = append(bounds, n); return n - 1 }
			if w.cast(r, 21, 0, 1, now) != 0 {
				t.Fatal("未受理矩形来源模型")
			}
			if len(bounds) != 0 {
				t.Fatal("尚未到期即抽落点/伤害")
			}
			w.advanceEvents(r, now.Add(200*time.Millisecond))
			sx, sy := m.Width/int(rank+1), m.Height/int(rank+1)
			if !reflect.DeepEqual(bounds, []int64{int64(2 * sx), int64(2 * sy)}) || r.Player.Pos != (devconfig.Point{X: sx - 1, Y: sy - 1}) || teleport21Last(t, c).Outcome != 1 {
				t.Fatal("轴半径/上界不含/地图维度错误", rank, bounds, r.Player.Pos)
			}
		})
	}
}

func TestTeleport21TwoHundredInvalidRepeatedSamplesAreNotClippedOrDeduplicated(t *testing.T) {
	w, r, c, s, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	calls := 0
	w.roll = func(n int64) int64 {
		if calls%2 == 0 && n != 26 || calls%2 == 1 && n != 18 {
			t.Fatal("非方轴混用", n)
		}
		calls++
		return 0
	}
	origin, mp, scene := r.Player.Pos, r.Player.MP, r.Scene
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理耗尽尝试")
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if calls != 400 || r.Player.Pos != origin || r.Scene != scene || r.Player.MP != mp-s.MPBaseCost || r.Player.Statuses[s.BuffID] != nil || teleport21Last(t, c).Outcome != 3 {
		t.Fatal("越界重复样本未照计200次、预裁为可走格、退款或错加状态", calls)
	}
}

func TestTeleport21ReadsCurrentBindingAndCurrentRankAtDue(t *testing.T) {
	w, r, c, s, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理测试动作")
	}
	teleport21Rank(r, 3)
	r.Role.Binding = &domain.RoleBinding{MapID: 2, X: 4, Y: 3}
	r.Role.BindingSafeZoneID, r.Role.BindingSourceSeq, r.Role.SourceSeq, r.Role.Revision = 107, 8, 8, 17
	bounds := []int64{}
	w.roll = func(n int64) int64 { bounds = append(bounds, n); return n / 2 }
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if r.Player.Pos != (devconfig.Point{X: 4, Y: 3}) || !reflect.DeepEqual(bounds, []int64{6, 4}) || teleport21Last(t, c).Outcome != 1 || r.Player.Statuses[s.BuffID] == nil {
		t.Fatal("21把受理时绑定/rank冻结冒充到期当前值", r.Player.Pos, bounds)
	}
}

func TestTeleport21AcceptedSameLifeDeadExecutesButNewDeadCastIsRejected(t *testing.T) {
	w, r, c, _, now := teleport21Worker(t)
	w.roll = func(n int64) int64 { return n / 2 }
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理活角色21")
	}
	life := r.Player.Life
	r.Player.HP = 0
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	if r.Player.HP != 0 || r.Player.Life != life || r.MapID != 2 || teleport21Last(t, c).Outcome != 1 {
		t.Fatal("到期尸体动作被通用死亡门禁取消或错误复活")
	}
	if code := w.cast(r, 21, 0, 2, now.Add(10*time.Second)); code == 0 {
		t.Fatal("新的死人施法没有拒绝")
	}
}

func TestTeleport21SameWorldKeepsSceneAndUsesAuthoritativeDisplacement(t *testing.T) {
	w, r, c, _, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	scene, baseline, world := r.Scene, r.Baseline, r.currentWorld()
	w.roll = func(n int64) int64 { return n / 2 }
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理同图21")
	}
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	displacements := 0
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CEnterLine {
			t.Fatal("同图瞬移伪造新资源入场")
		}
		if frame.ID == p.MsgS2CEntityDisplacement {
			q := &pb.EntityDisplacement{}
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

func TestTeleport21StaticDoorAndOccupiedCellRemainLegalSourceLandings(t *testing.T) {
	for _, kind := range []string{"closed_source_door", "occupied"} {
		t.Run(kind, func(t *testing.T) {
			w, r, c, _, now := teleport21Worker(t)
			m, _ := w.e.Store.Cfg.Fixture.MapByID(2)
			goal := devconfig.Point{X: 650, Y: 631}
			if kind == "closed_source_door" {
				found := false
				for y := 0; y < m.Height && !found; y++ {
					for x := 0; x < m.Width; x++ {
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
			calls := 0
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

func TestTeleport21CurrentMapAtDueIsIndependentOfAcceptanceWorld(t *testing.T) {
	w, r, c, _, now := teleport21Worker(t)
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理21")
	}
	town, _ := w.e.Store.Cfg.Fixture.MapByID(2)
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

func TestTeleport21ActualDueNodeAtFirstMiddleLastDoesNotLoseOldWorldSuffix(t *testing.T) {
	for _, position := range []string{"first", "middle", "last"} {
		t.Run(position, func(t *testing.T) {
			w, r, c, _, now := teleport21Worker(t)
			personal := r.PersonalWorld
			monster := personal.Monsters[0]
			monster.Pos = stepPoint(r.Player.Pos, 0, 1)
			w.roll = func(n int64) int64 { return n / 2 }
			if w.cast(r, 21, 0, 1, now) != 0 {
				t.Fatal("真实21受理失败")
			}
			teleport := personal.Casts[0]
			fireball, _ := w.e.Store.Cfg.Fixture.Skill(1)
			hellfire, _ := w.e.Store.Cfg.Fixture.Skill(9)
			// 构造已释放事件的批次时序，仅验证真实21节点；不冒称正常按钮同时施放的输入认证。
			before, after := teleport.Due.Add(-time.Millisecond), teleport.Due.Add(time.Millisecond)
			fireDue, mapDue := after, after.Add(time.Millisecond)
			if position == "middle" {
				fireDue = before
			}
			if position == "last" {
				fireDue, mapDue = before.Add(-time.Millisecond), before
			}
			personal.Casts = []pendingCast{teleport, {Scene: r.Scene, Skill: fireball, Caster: teleport.Caster, Target: monster.Handle, TargetLife: monster.Life, TargetPoint: monster.Pos, Amount: 10, Due: fireDue, ID: 10, Order: 10}, {Scene: r.Scene, Skill: hellfire, Caster: teleport.Caster, TargetPoint: r.Player.Pos, Direction: 0, LineRemaining: 1, Amount: 2, Due: mapDue, ID: 11, Order: 11}}
			w.advanceEvents(r, now.Add(250*time.Millisecond))
			want := int64(988)
			if position == "first" {
				want = 998
			}
			if teleport21Last(t, c).Outcome != 1 || r.MapID != 2 || monster.HP != want || r.eventCount() != 0 {
				t.Fatal("真实21批首/中/末丢后缀，或原地图动作串图/角色动作未按同图条件失败", position, monster.HP, want)
			}
			for _, frame := range c.frames {
				if frame.ID == p.MsgS2CCombatImpact {
					q := &pb.CombatImpact{}
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

func TestTeleport21ManaCostUsesIntegerPenaltyOnlyForTeleportAndRejectsOverflow(t *testing.T) {
	w, r, _, skill, _ := teleport21Worker(t)
	r.Player.Statuses = map[uint32]*combatStatus{skill.BuffID: {Kind: "teleport_mana_penalty", TargetLife: r.Player.Life, TeleportManaPenaltyPercent: 30}}
	for rank, want := range []int64{13, 16, 20, 24} {
		if got, ok := teleportManaCost(&r.Player, skill, uint32(rank)); !ok || got != want {
			t.Fatal("来源整数基础费后截断加30%错误", rank, got, want)
		}
	}
	fireball, _ := w.e.Store.Cfg.Fixture.Skill(1)
	if got, ok := teleportManaCost(&r.Player, fireball, 1); !ok || got != fireball.MPBaseCost+fireball.MPLevelCost {
		t.Fatal("TemporalFlux错误增加非传送技能MP")
	}
	for _, q := range []devconfig.Skill{{Behavior: "teleport", MPBaseCost: math.MaxInt64, MPLevelCost: 1}, {Behavior: "teleport", MPBaseCost: math.MaxInt64}, {Behavior: "teleport", MPBaseCost: -1}} {
		if _, ok := teleportManaCost(&r.Player, q, 1); ok {
			t.Fatal("整数溢出/负费未拒绝", q)
		}
	}
}

func TestTeleport21FluxRefreshRestartsThirtySecondsWithoutStackingPenalty(t *testing.T) {
	w, r, c, skill, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	w.roll = func(n int64) int64 { return n / 2 }
	initialMP := r.Player.MP
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("首21未受理")
	}
	firstDue := now.Add(200 * time.Millisecond)
	w.advanceEvents(r, firstDue)
	first := r.Player.Statuses[skill.BuffID]
	if first == nil {
		t.Fatal("首成功缺状态")
	}
	w.tickStatuses(r, firstDue.Add(25*time.Second))
	secondAccepted := now.Add(26 * time.Second)
	if w.cast(r, 21, 0, 2, secondAccepted) != 0 {
		t.Fatal("已准备好且有足够MP的第二21被拒绝")
	}
	secondDue := secondAccepted.Add(200 * time.Millisecond)
	w.advanceEvents(r, secondDue)
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

func TestTeleport21SameWorldDisplacementStartsAtActualDuePositionAfterPush(t *testing.T) {
	w, r, c, _, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	w.roll = func(n int64) int64 { return n / 2 }
	if w.cast(r, 21, 0, 1, now) != 0 {
		t.Fatal("未受理21")
	}
	if out := w.sourcePushed(r, reference(r, &r.Player), 2, 1, now.Add(100*time.Millisecond), displacementSource{}); out.Steps != 1 {
		t.Fatal("权威推移前态失败", out)
	}
	origin := r.Player.Pos
	w.advanceEvents(r, now.Add(200*time.Millisecond))
	for i := len(c.frames) - 1; i >= 0; i-- {
		if c.frames[i].ID == p.MsgS2CEntityDisplacement {
			q := &pb.EntityDisplacement{}
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

func TestTeleport21TwoHundredthAttemptCanSucceedAndStopsImmediately(t *testing.T) {
	w, r, c, _, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	calls := 0
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

func TestTeleport21ProofLostAtDueDoesNotRefundOrRetryAsNewCast(t *testing.T) {
	w, r, c, s, now := teleport21Worker(t)
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

func TestTeleport21LandingBindingKeepsSelectedProofAndQueuesAfterEarlierOldMapKill(t *testing.T) {
	w, r, c, s, now := teleport21Worker(t)
	teleport21Rectangle(t, w, r)
	personal := r.PersonalWorld
	monster := personal.Monsters[0]
	hellfire, _ := w.e.Store.Cfg.Fixture.Skill(9)
	r.CastID, r.ScheduleID = 1, 1
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
	candidate := r.BindingCandidate
	if teleport21Last(t, c).Outcome != 1 || r.Player.Pos != (devconfig.Point{X: 4, Y: 3}) || candidate == nil || len(r.KillFacts) != 1 || candidate.order <= r.KillFacts[0].Order || candidate.request.SourceSeq != 0 || candidate.request.ExpectedRevision != 0 || candidate.request.Action != "teleport" || candidate.request.OriginBindingSourceSeq != 7 || candidate.center != (domain.RoleBinding{MapID: 2, X: 4, Y: 3}) || candidate.zoneID != 107 || r.Player.Statuses[s.BuffID] == nil {
		t.Fatal("成功新区域landing事实抢号/丢原证明，或为了旧kill延迟传送", candidate)
	}
	w.flushKillFacts(r)
	if !r.Pending || r.PendingBinding != nil || r.BindingCandidate != candidate || candidate.request.SourceSeq != 0 {
		t.Fatal("前序原World kill没有先占唯一永久slot")
	}
}

func TestTeleport21FixedPenaltyExpiresAtDeadlineBeforeItsNextStatusTick(t *testing.T) {
	w, r, _, s, now := teleport21Worker(t)
	r.Player.Statuses = map[uint32]*combatStatus{s.BuffID: {ID: s.BuffID, Kind: "teleport_mana_penalty", TargetLife: r.Player.Life, TeleportManaPenaltyPercent: 30, RemainingMS: 100, LastProcess: now, NextProcess: now.Add(time.Second), TickMS: 1000}}
	w.tickStatuses(r, now.Add(100*time.Millisecond))
	if r.Player.Statuses[s.BuffID] != nil {
		t.Fatal("固定30秒状态被NextProcess拖到下一秒才过期")
	}
	if cost, ok := teleportManaCost(&r.Player, s, 0); !ok || cost != s.MPBaseCost {
		t.Fatal("已过期TemporalFlux仍增加下次费用", cost)
	}
}

func TestTeleport21SourceDoorLandingCanLeaveByNormalGroundNavigation(t *testing.T) {
	w, r, c, _, now := teleport21Worker(t)
	m, _ := w.e.Store.Cfg.Fixture.MapByID(2)
	var door, exit devconfig.Point
	found := false
	for y := 0; y < m.Height && !found; y++ {
		for x := 0; x < m.Width && !found; x++ {
			point := devconfig.Point{X: x, Y: y}
			if m.Walkable(point) || !m.TeleportWalkable(point) {
				continue
			}
			for _, d := range []devconfig.Point{{X: 1}, {Y: 1}, {X: -1}, {Y: -1}} {
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
	if err := w.e.initializeTownNavigation(); err != nil {
		t.Fatal(err)
	}
	if !w.activateWorld(r, r.TownWorld, devconfig.Point{X: 633, Y: 613}, nil) {
		t.Fatal("原Town入场失败")
	}
	calls := 0
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
	for i := 1; i <= 8 && r.Player.Pos != exit; i++ {
		at := now.Add(time.Duration(i) * time.Second)
		w.beginTownNavigationTick(at)
		w.step(r, at)
		w.endTownNavigationTick()
	}
	if r.Player.Pos != exit {
		t.Fatalf("合法源21落闭门%v后正常点地无法走向相邻合法出口%v，实际%v、导航%+v", door, exit, r.Player.Pos, r.Navigation)
	}
}
