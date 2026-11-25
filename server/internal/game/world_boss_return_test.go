// 本文件用受控扣料回执验证世界首领离场与原挂机施法的顺序。
package game

import (
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func TestWorldBossReturnWaitsForOriginalMaterialReceipt(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		name := "退场后回原普通图"
		if disconnect {
			name = "等待期间断线释放观察且不接回已离开的席位"
		}
		t.Run(name, func(t *testing.T) {
			w, owner, conn := statusWorker(t, 2)
			w.e.ctx, w.e.jobs = t.Context(), make(chan job, 1)
			owner.Role.ID, owner.Auth.RoleID = "role", "role"
			owner.Role.Revision, owner.Role.SourceSeq, owner.Role.EconomicSeq = 6, 3, 2
			owner.Role.Equipped = []domain.Equipped{{Slot: devconfig.CastingSupplySlot,
				Item: domain.Item{ID: domain.ID(), ItemID: 187, Version: 1, Quantity: 2}}}
			oldScene, oldBaseline, oldMana, oldPosition := owner.Scene, owner.Baseline, owner.Player.MP, owner.Player.Pos
			now := time.Now()
			obs := worldBossReturnObservation(t, w, owner, now)
			item := owner.Role.Equipped[0].Item
			q := persistence.AssetRequest{Kind: "cast_material", SkillID: 14, ItemID: item.ID, ItemVersion: uint64(item.Version),
				SceneEpoch: oldScene, CasterLife: owner.Player.Life, SourceSeq: owner.Role.SourceSeq + 1, Fingerprint: "原普通图自动耗符",
				Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), ExpectedRevision: uint64(owner.Role.Revision), ExpectedRuleVersion: w.e.Store.Cfg.Fixture.RuleVersion}}
			plan, code := w.prepareMaterialCast(owner, q, &pb.CastWithMaterialRequest{SkillId: q.SkillID, TargetGrid: pointPB(oldPosition)}, now)
			if code != 0 || plan == nil || plan.Cost <= 0 {
				t.Fatal("原普通图技能应当取得有效的耗符计划", code, plan)
			}
			w.submitMaterialCast(owner, q, plan, 0, nil)
			binding := materialCommitBinding{roleID: owner.Role.ID, room: owner, auth: owner.Auth, scene: oldScene,
				life: owner.Player.Life, generation: owner.MaterialGeneration, conn: conn}
			originalJob := <-w.e.jobs

			// 走实际共享席位主动离场路径，原扣料数据库作业由本用例控制回执到达顺序。
			leave := wire.Message(p.MsgC2SWorldBossLeave, 1, conn.Binding(), &pb.WorldBossLeave{SceneEpoch: obs.scene, OccurrenceId: obs.occurrence.id})
			w.command(command{role: owner.Role.ID, conn: conn, frame: leave})
			if !obs.revoked.Load() || !obs.finalReturn.Load() || len(w.worldBossArenas[obs.occurrence.runID].seats) != 0 ||
				!owner.Pending || owner.Scene != oldScene || owner.Baseline != oldBaseline || len(owner.Casts) != 0 {
				t.Fatal("退场应先撤销原席位，并保留等待扣料的普通场景代次", owner.Scene, owner.Baseline, owner.Pending, obs.revoked.Load())
			}
			if len(conn.frames) != 1 || conn.frames[0].ID != p.MsgS2CWorldBossState {
				t.Fatal("扣料确认前向旧附着推送了普通地图", conn.frames)
			}
			attack := wire.Message(p.MsgC2SAttackIntent, 2, conn.Binding(), &pb.AttackIntent{SceneEpoch: obs.scene, InputSequence: 99})
			w.command(command{role: owner.Role.ID, conn: conn, frame: attack})
			if len(conn.frames) != 2 || conn.frames[1].Error != p.CodeLineRecovering || owner.InputSequence != 0 {
				t.Fatal("退场等待接受了旧首领输入或污染普通动作流水", conn.frames, owner.InputSequence)
			}
			w.finishMaterialCast(binding, q, plan, 0, nil, domain.Role{}, &pb.OperationResult{}, persistence.Operation{},
				persistence.Fail(p.CodeOperationPending, "受控未决提交"), &originalJob)
			if !owner.Pending || owner.PendingRetry != &originalJob || owner.Scene != oldScene || len(owner.Casts) != 0 {
				t.Fatal("未知回执提前退场或改变原回执与施法计划")
			}
			if disconnect {
				w.command(command{role: owner.Role.ID, conn: conn, disconnect: true})
				if owner.Conn != nil || owner.worldBossObservation != nil || owner.worldBossResume.id != "" || owner.Scene != oldScene {
					t.Fatal("退场等待中的断线没有清理观察，或仍尝试重连已离开席位")
				}
			}
			role := owner.Role
			role.Revision++
			role.SourceSeq = q.SourceSeq
			role.Equipped = slices.Clone(role.Equipped)
			role.Equipped[0].Item.Version++
			role.Equipped[0].Item.Quantity--
			receipt := persistence.Operation{Kind: q.Kind, Digest: q.Fingerprint, RoleID: role.ID, SourceSeq: q.SourceSeq, Revision: role.Revision,
				Casting: &persistence.CastingReceipt{Scope: "auto", SkillID: q.SkillID, InstanceID: item.ID, Quantity: 1}}
			result := &pb.OperationResult{NewRevision: uint64(role.Revision), CommandSeq: uint64(role.EconomicSeq)}
			w.finishMaterialCast(binding, q, plan, 0, nil, role, result, receipt, nil, &originalJob)
			w.finishMaterialCast(binding, q, plan, 0, nil, role, result, receipt, nil, &originalJob)
			if owner.Pending || owner.worldBossObservation != nil || len(owner.Casts) != 1 || owner.MaterialSourceSeq != q.SourceSeq || owner.MaterialEconomicSeq != 0 ||
				owner.Role.EconomicSeq != 2 || owner.Role.SourceSeq != q.SourceSeq || owner.Role.Equipped[0].Item.Quantity != 1 || owner.Player.MP != oldMana-plan.Cost || owner.Player.Pos != oldPosition {
				t.Fatal("迟到或重放回执没有保留原普通图的一次扣料、一次施法与原位置")
			}
			entries := 0
			for _, frame := range conn.frames {
				if frame.ID == p.MsgS2CCastAccepted {
					t.Fatal("把原挂机施法混入了已退场的首领观察")
				}
				if frame.ID != p.MsgS2CEnterLine {
					continue
				}
				entries++
				var entry pb.EnterLineResult
				if err := proto.Unmarshal(frame.Payload, &entry); err != nil || entry.SceneEpoch != obs.scene+1 ||
					entry.BaselineId <= obs.baseline || entry.ServerTransitionId != obs.occurrence.id || entry.SceneKind != devconfig.SceneKindPersonal {
					t.Fatal("返回入场没有匹配原退场身份与新普通图基线", &entry, err)
				}
			}
			if disconnect && (entries != 0 || owner.Scene != oldScene) || !disconnect && (entries != 1 || owner.Scene != obs.scene+1 || owner.Ready) {
				t.Fatal("返回地图未按原附着状态只推送一次", entries, owner.Scene, owner.Ready)
			}
		})
	}
}

// TestWorldBossReturnPreservesActorFirstHealthyReconnect 钉住场次先撤旧输入时，仍可依原席位签新观察票。
func TestWorldBossReturnPreservesActorFirstHealthyReconnect(t *testing.T) {
	w, owner, conn := statusWorker(t, 2)
	owner.Role.ID, owner.Auth.RoleID = "role", "role"
	obs := worldBossReturnObservation(t, w, owner, time.Now())
	arena := w.worldBossArenas[obs.occurrence.runID]
	seat := arena.seats[owner.Role.ID]
	w.suspendWorldBossSeat(arena, seat)
	if !obs.revoked.Load() || obs.finalReturn.Load() || arena.resumable[owner.Role.ID] != seat {
		t.Fatal("场次先撤回原输入时未保存同一健康身体")
	}
	w.command(command{role: owner.Role.ID, conn: conn, disconnect: true})
	if owner.worldBossObservation != nil || owner.worldBossResume != obs.occurrence || owner.Conn != nil {
		t.Fatal("角色收到掉线时遗漏了场次先已暂挂的原席位")
	}
}

// worldBossReturnObservation 安装同一个测试分片上的原普通房间、已确认共享席位和真实命令归属。
func worldBossReturnObservation(t *testing.T, w *worker, owner *room, now time.Time) *worldBossObservation {
	t.Helper()
	identity := bossChallengeIdentity{runID: 1, challengeID: domain.ID(), execution: 2, ruleVersion: w.e.Store.Cfg.Fixture.RuleVersion, configHash: "隔离退场"}
	open, close := now.Add(-time.Second), now.Add(time.Minute)
	challenge, err := newBossChallenge(identity, "", open, close, 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	grant := bossParticipantGrant{challenge: identity, roleID: owner.Role.ID, reservationID: domain.ID(), combatSnapshot: "隔离角色"}
	if !challenge.admit(grant, open) {
		t.Fatal("未能确认原共享席位")
	}
	occurrence := worldBossOccurrence{runID: identity.runID, id: identity.challengeID, opensAt: open, closesAt: close}
	obs := &worldBossObservation{occurrence: occurrence, owner: w, actor: w, conn: owner.Conn, auth: owner.Auth,
		scene: owner.Scene + 1, baseline: owner.Baseline + 1, shown: true}
	obs.started.Store(true)
	child := &room{worldState: &worldState{}, Role: owner.Role, Auth: owner.Auth, Conn: owner.Conn,
		Scene: obs.scene, Baseline: obs.baseline, Ready: true, Player: entity{Handle: 101, Life: 1, HP: 10}}
	seat := &worldBossSeat{observation: obs, room: child, shown: true}
	arena := &worldBossArena{occurrence: occurrence, config: w.e.Store.Cfg.Fixture.WorldBoss, challenge: challenge,
		boss: &entity{HP: 10, MaxHP: 10}, seats: map[string]*worldBossSeat{owner.Role.ID: seat}, resumable: make(map[string]*worldBossSeat)}
	w.worldBossArenas = map[uint64]*worldBossArena{occurrence.runID: arena}
	owner.worldBossObservation = obs
	return obs
}
