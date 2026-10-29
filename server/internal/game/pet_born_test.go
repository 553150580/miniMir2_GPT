package game

import (
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"testing"
	"time"
)

type petBornConnection struct {
	frames        []wire.Frame
	alive         bool
	binding       uint64
	failID        uint16
	failChunk     int
	chunkAttempts int
	closed        int
	onDeliver     func(wire.Frame)
}

func (c *petBornConnection) Alive() bool { return c.alive }
func (c *petBornConnection) Deliver(f wire.Frame) bool {
	if !c.alive {
		return false
	}
	if f.ID == p.MsgS2CSceneBaseline {
		c.chunkAttempts++
	}
	if f.ID == c.failID || f.ID == p.MsgS2CSceneBaseline && c.failChunk > 0 && c.chunkAttempts == c.failChunk {
		return false
	}
	c.frames = append(c.frames, f)
	if c.onDeliver != nil {
		c.onDeliver(f)
	}
	return true
}
func (c *petBornConnection) Close(string)        { c.closed++; c.alive = false }
func (c *petBornConnection) SetRole(string)      {}
func (c *petBornConnection) RoleID() string      { return "role" }
func (c *petBornConnection) SetBinding(b uint64) { c.binding = b }
func (c *petBornConnection) Binding() uint64     { return c.binding }

func petBornWorker(t *testing.T) (*worker, *room, *petBornConnection, *worldState, *worldState, time.Time) {
	t.Helper()
	w, r, _, personal, town, _ := summonWorker(t, false)
	w.e.Store.Cfg.Fixture.RuleVersion = 18 // 当前新观察合同的隔离前态，不降低主配置校验。
	c := &petBornConnection{alive: true, binding: uint64(r.Auth.SessionEpoch)}
	r.Conn = c
	w.sendSyncReady(r, 71, &pb.SyncReady{SceneEpoch: r.Scene, BaselineId: r.Baseline, NextEconomicCommandSeq: 1})
	if r.PetBornReady == nil || len(c.frames) != 1 || c.frames[0].ID != p.MsgS2CSyncReady {
		t.Fatal("未由真实成功发送SyncReady建立附件前态")
	}
	c.frames = nil
	return w, r, c, personal, town, time.Now().Add(-2 * time.Second)
}

func petBornEvents(t *testing.T, c *petBornConnection) []*pb.PetBorn {
	t.Helper()
	events := []*pb.PetBorn{}
	for _, f := range c.frames {
		if f.ID == p.MsgS2CPetBorn {
			q := &pb.PetBorn{}
			if err := proto.Unmarshal(f.Payload, q); err != nil {
				t.Fatal(err)
			}
			events = append(events, q)
		}
	}
	return events
}

func TestPetBornActualSpawnIdentityAndCompleteBaselinePrecedeOneFact(t *testing.T) {
	w, r, c, _, _, start := petBornWorker(t)
	r.Scene = uint64(1)<<63 | 11
	r.Baseline = uint64(1)<<63 | 13
	r.Sequence = uint64(1)<<63 | 17
	r.CastID = uint64(1)<<63 | 19
	r.PetLifeSequence = uint64(1)<<63 | 21
	w.sendSyncReady(r, 72, &pb.SyncReady{SceneEpoch: r.Scene, BaselineId: r.Baseline})
	r.Player.Direction = 2
	plan := paidSummonPlan(t, w, r, start)
	c.frames = nil
	bornAt := time.Now()
	w.advanceEvents(r, bornAt)
	events := petBornEvents(t, c)
	if len(events) != 1 || r.Pet == nil {
		t.Fatal("真实新生没有且仅有一条事实", len(events))
	}
	e := events[0]
	if e.SceneEpoch != r.Scene || e.BaselineId != r.Baseline || e.EntityHandle != r.Pet.Handle || e.EntityLife != r.Pet.Life || e.DefinitionId != r.Pet.Def || e.MapId != 1 ||
		e.SourceCastId != r.CastID || e.SourceSkillId != 17 || e.BornAtUnixMs != uint64(bornAt.UnixMilli()) || e.MovementRevision != 0 || e.Direction != 5 ||
		e.Role.OriginServerId != r.Role.Origin || domain.Hex(e.Role.RoleId) != r.Role.ID {
		t.Fatal("出生身份、实际now、0运动水位或角色围栏错误", e)
	}
	if e.BornAtUnixMs == uint64(start.Add(time.Duration(plan.DelayMS)*time.Millisecond).UnixMilli()) {
		t.Fatal("将旧due伪装为实际出生时间")
	}
	chunks := map[uint32]bool{}
	count := uint32(0)
	foundBody := false
	foundFact := false
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CSceneBaseline {
			b := &pb.SceneBaselineChunk{}
			_ = proto.Unmarshal(frame.Payload, b)
			if b.StateSequence == e.StateSequence {
				chunks[b.ChunkIndex] = true
				count = b.ChunkCount
				for _, body := range b.Entities {
					if body.EntityHandle == e.EntityHandle && body.Life == e.EntityLife && body.Direction == e.Direction && body.MovementRevision == e.MovementRevision {
						foundBody = true
					}
				}
			}
		}
		if frame.ID == p.MsgS2CPetBorn {
			if !foundBody || count == 0 || len(chunks) != int(count) || frame.Binding != uint64(r.Auth.SessionEpoch) {
				t.Fatal("完整匹配身体基线未在事实之前")
			}
			foundFact = true
		}
	}
	if !foundFact {
		t.Fatal("未观察到事实")
	}
	w.advanceEvents(r, time.Now())
	w.baseline(r)
	if len(petBornEvents(t, c)) != 1 {
		t.Fatal("后续baseline重复生成出生事实")
	}
}

func TestPetBornNeverSynthesizedForRecallLegacyOrRule17(t *testing.T) {
	for _, mode := range []string{"recall", "legacy", "rule17"} {
		t.Run(mode, func(t *testing.T) {
			w, r, c, _, _, start := petBornWorker(t)
			switch mode {
			case "rule17":
				w.e.Store.Cfg.Fixture.RuleVersion = 17
				paidSummonPlan(t, w, r, start)
				w.advanceEvents(r, time.Now())
			case "legacy":
				pet := &entity{Handle: 0, Kind: 2, Def: w.e.Store.Cfg.Fixture.Pet.DefinitionID, HP: 15, MaxHP: 15, Pos: r.Player.Pos}
				if !w.attachLegacyPetState(pet) || !w.installPet(r, r.currentWorld(), pet) {
					t.Fatal("legacy隔离安装失败")
				}
				w.baseline(r)
			case "recall":
				paidSummonPlan(t, w, r, start)
				w.advanceEvents(r, time.Now())
				c.frames = nil
				r.Player.ActionReadyAt = time.Time{}
				r.NextSpell = time.Time{}
				r.SkillCooldowns = nil
				if code := w.cast(r, 17, 0, 2, time.Now()); code != 0 {
					t.Fatal(code)
				}
				w.advanceEvents(r, time.Now().Add(time.Second))
			}
			if len(petBornEvents(t, c)) != 0 {
				t.Fatal("Recall/legacy/旧rule从状态推导出生")
			}
		})
	}
}

func TestPetBornUnobservedWorldAndBeforeReadyNeverReplayAfterRecallOrNewTicket(t *testing.T) {
	for _, mode := range []string{"other_world", "not_ready", "new_connection_without_sync", "sync_send_failed"} {
		t.Run(mode, func(t *testing.T) {
			w, r, c, _, town, start := petBornWorker(t)
			paidSummonPlan(t, w, r, start)
			switch mode {
			case "other_world":
				r.worldState = town
				r.Player.Pos = devconfig.Point{X: 633, Y: 613}
			case "not_ready":
				r.Ready = false
			case "new_connection_without_sync":
				r.Conn = &petBornConnection{alive: true, binding: uint64(r.Auth.SessionEpoch)}
			case "sync_send_failed":
				c.failID = p.MsgS2CSyncReady
				w.sendSyncReady(r, 1, &pb.SyncReady{SceneEpoch: r.Scene, BaselineId: r.Baseline})
			}
			w.advanceEvents(r, time.Now())
			if r.Pet == nil || len(petBornEvents(t, c)) != 0 {
				t.Fatal("未观察/未Sync出生没有保留业务身体或错误发事实")
			}
			if mode == "other_world" {
				w.tickResidentPetFollow(r, time.Now())
			}
			next := &petBornConnection{alive: true, binding: uint64(r.Auth.SessionEpoch)}
			r.Conn = next
			r.Ready = true
			w.sendSyncReady(r, 2, &pb.SyncReady{SceneEpoch: r.Scene, BaselineId: r.Baseline})
			w.baseline(r)
			if len(petBornEvents(t, next)) != 0 {
				t.Fatal("AutoRecall或健康新票补播历史出生")
			}
		})
	}
}

func TestPetBornFailedDueAndOldLifeHaveNoObservation(t *testing.T) {
	for _, mode := range []string{"dead", "new_life", "invalid_static"} {
		t.Run(mode, func(t *testing.T) {
			w, r, c, _, _, start := petBornWorker(t)
			plan := paidSummonPlan(t, w, r, start)
			switch mode {
			case "dead":
				r.Player.HP = 0
			case "new_life":
				r.Player.Life++
			case "invalid_static":
				w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{plan.Summon.Front, r.Player.Pos}
			}
			w.advanceEvents(r, time.Now())
			if r.Pet != nil || r.PendingPetBirth != nil || len(petBornEvents(t, c)) != 0 {
				t.Fatal("失败due/旧Life产生身体或事实")
			}
		})
	}
}

func TestPetBornBaselineDeliveryFailureAndAttachmentMutationDropFact(t *testing.T) {
	for _, mode := range []string{"baseline_failure", "fact_failure", "connection", "auth", "scene", "body_dead", "body_moved", "new_sequence"} {
		t.Run(mode, func(t *testing.T) {
			w, r, c, _, _, start := petBornWorker(t)
			paidSummonPlan(t, w, r, start)
			c.frames = nil
			if mode == "baseline_failure" {
				c.failID = p.MsgS2CSceneBaseline
			}
			if mode == "fact_failure" {
				c.failID = p.MsgS2CPetBorn
			}
			replaced := &petBornConnection{alive: true, binding: uint64(r.Auth.SessionEpoch)}
			c.onDeliver = func(frame wire.Frame) {
				if frame.ID != p.MsgS2CSceneBaseline {
					return
				}
				c.onDeliver = nil
				switch mode {
				case "connection":
					r.Conn = replaced
				case "auth":
					r.Auth.SessionEpoch++
				case "scene":
					r.Scene++
				case "body_dead":
					w.applyDamage(r, r.Monsters[0], r.Pet, r.Pet.HP, time.Now())
				case "body_moved":
					if !w.relocatePet(r, r.petReference(), r.currentWorld(), devconfig.Point{X: r.Pet.Pos.X + 1, Y: r.Pet.Pos.Y}) {
						t.Fatal("合法原宠移动前态失败")
					}
					w.snapshot(r)
				case "new_sequence":
					w.snapshot(r)
				}
			}
			w.advanceEvents(r, time.Now())
			if mode == "body_dead" {
				if r.Pet != nil || len(r.PetCorpses) != 1 {
					t.Fatal("完整原宠死亡未归档")
				}
			} else if r.Pet == nil {
				t.Fatal("显示失败回滚业务身体")
			}
			if len(petBornEvents(t, c)) != 0 || len(petBornEvents(t, replaced)) != 0 {
				t.Fatal("显示失败/旧附件变更发事实或回滚业务身体")
			}
			if (mode == "baseline_failure" || mode == "fact_failure") && c.closed == 0 {
				t.Fatal("发送失败未有界关闭原连接")
			}
		})
	}
}

func TestPetBornSyncReadyStampRequiresSuccessfulExactAttachment(t *testing.T) {
	for _, change := range []string{"not_ready", "bad_payload", "connection", "auth", "binding", "failed"} {
		t.Run(change, func(t *testing.T) {
			w, r, c, _, _, start := petBornWorker(t)
			value := &pb.SyncReady{SceneEpoch: r.Scene, BaselineId: r.Baseline}
			if change == "not_ready" {
				r.Ready = false
			}
			if change == "bad_payload" {
				value.BaselineId++
			}
			if change == "failed" {
				c.failID = p.MsgS2CSyncReady
			}
			c.onDeliver = func(f wire.Frame) {
				if f.ID != p.MsgS2CSyncReady {
					return
				}
				c.onDeliver = nil
				switch change {
				case "connection":
					r.Conn = &petBornConnection{alive: true, binding: uint64(r.Auth.SessionEpoch)}
				case "auth":
					r.Auth.SessionEpoch++
				case "binding":
					c.binding++
				}
			}
			w.sendSyncReady(r, 1, value)
			if r.PetBornReady != nil {
				t.Fatal("不匹配/未成功SyncReady建立新生资格")
			}
			r.Ready = true
			paidSummonPlan(t, w, r, start)
			w.advanceEvents(r, time.Now())
			if len(petBornEvents(t, c)) != 0 {
				t.Fatal("无原SyncReady证明仍发出生")
			}
		})
	}
}

func TestPetBornUnreadyFilterAndStateSequenceExhaustionFailClosed(t *testing.T) {
	w, r, c, _, _, start := petBornWorker(t)
	c.frames = nil
	r.Ready = false
	w.send(r, p.MsgS2CPetBorn, 0, &pb.PetBorn{})
	if len(c.frames) != 0 {
		t.Fatal("通用send未过滤ACK前出生")
	}
	r.Ready = true
	paidSummonPlan(t, w, r, start)
	r.Sequence = math.MaxUint64
	c.frames = nil
	w.advanceEvents(r, time.Now())
	if r.Pet == nil || !r.Isolated || r.Sequence != math.MaxUint64 || len(petBornEvents(t, c)) != 0 {
		t.Fatal("baseline序号耗尽回绕或回滚已生效宠物")
	}
}

func TestPetBornSyncReadyWorldIdentityDoesNotFollowUnannouncedWorldChange(t *testing.T) {
	w, r, c, personal, town, start := petBornWorker(t)
	plan := paidSummonPlan(t, w, r, start)
	w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{plan.Summon.Front}
	// 模型停在内部World已经变化、但原scene/baseline尚未重新附着的边界。
	r.worldState = town
	r.Player.Pos = devconfig.Point{X: 633, Y: 613}
	if r.PetBornReady.World != personal.Ref {
		t.Fatal("SyncReady前态没有锁定原World")
	}
	w.advanceEvents(r, time.Now())
	if r.Pet == nil || r.petResidentWorld() != town || len(petBornEvents(t, c)) != 0 {
		t.Fatal("新World沿用旧SyncReady资格发布出生，或丢失真实fallback身体")
	}
	next := &petBornConnection{alive: true, binding: uint64(r.Auth.SessionEpoch + 1)}
	r.Conn = next
	r.Auth.SessionEpoch++
	r.Ready = true
	w.sendSyncReady(r, 1, &pb.SyncReady{SceneEpoch: r.Scene, BaselineId: r.Baseline})
	w.baseline(r)
	if r.PetBornReady == nil || r.PetBornReady.World != town.Ref || len(petBornEvents(t, next)) != 0 {
		t.Fatal("新附件重新同步后补播旧出生")
	}
}

func TestPetBornMultiChunkGroupAllDeliveredBeforeFactAndFailureDoesNotRetry(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "second_chunk_failure"}[fail], func(t *testing.T) {
			w, r, c, _, _, start := petBornWorker(t)
			paidSummonPlan(t, w, r, start)
			for i := uint32(1001); i < 1129; i++ {
				body := &entity{Handle: i, Kind: 2, Def: w.e.Store.Cfg.Fixture.Pet.DefinitionID, HP: 0, MaxHP: 15, Life: uint64(i), IsPetCorpse: true, Statuses: map[uint32]*combatStatus{}}
				for id := uint32(1); id < 65; id++ {
					body.Statuses[id] = &combatStatus{ID: id, TargetLife: body.Life, RemainingMS: math.MaxInt32, SourceSkillID: math.MaxUint32, SkillLevel: math.MaxUint32}
				}
				r.PetCorpses = append(r.PetCorpses, &petCorpse{Body: body, ExpiresAt: time.Now().Add(time.Hour)})
			}
			// wire压力前态显式放宽身体预算，仅验分块发送，不宣称128运行容量。
			w.e.Store.Cfg.Fixture.PetRuntime.Budgets.RetainedBodyCapacityPerOwner = 130
			c.frames = nil
			c.chunkAttempts = 0
			if fail {
				c.failChunk = 2
			}
			w.advanceEvents(r, time.Now())
			events := petBornEvents(t, c)
			if fail {
				if len(events) != 0 || c.chunkAttempts != 2 || r.Pet == nil {
					t.Fatal("半组发送失败仍发事实/重试或回滚")
				}
				return
			}
			if len(events) != 1 {
				t.Fatal("多块完整后缺事实", len(events))
			}
			count := 0
			for _, frame := range c.frames {
				if frame.ID == p.MsgS2CPetBorn {
					break
				}
				if frame.ID == p.MsgS2CSceneBaseline {
					count++
				}
			}
			if count < 2 {
				t.Fatal("压力前态未真实产生多块")
			}
		})
	}
}

func TestPetBornElapsedOverflowNegativeAndDuplicateCannotReplay(t *testing.T) {
	for _, duration := range []time.Duration{-time.Millisecond, (time.Duration(math.MaxUint32) + 1) * time.Millisecond} {
		anchor := time.Now()
		if _, ok := petBornElapsed(anchor.Add(-duration), anchor); ok {
			t.Fatal("elapsed负数/溢出未拒绝")
		}
	}
	w, r, c, _, _, start := petBornWorker(t)
	plan := paidSummonPlan(t, w, r, start)
	w.advanceEvents(r, time.Now())
	c.frames = nil
	fact := newPetBirthFact(r, r.currentWorld(), r.Pet, pendingCast{ID: r.CastID, Skill: plan.Skill, Summon: plan.Summon}, time.Now())
	w.publishPetBorn(r, fact)
	w.publishPetBorn(r, fact)
	if len(petBornEvents(t, c)) != 1 {
		t.Fatal("同局部事实可重复发送")
	}
}
