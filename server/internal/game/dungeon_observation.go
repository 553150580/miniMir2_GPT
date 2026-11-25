// 本文件衔接讨伐链的独立战斗观察、健康接回以及已确认终态回场。
package game

import (
	"math"
	"sync/atomic"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// dungeonAttachment 的身份在两处分片间只读。shown 只由原角色分片修改，acked 只由场次分片修改。
type dungeonAttachment struct {
	actor          *worker                           // 管理同一支讨伐队伍战斗的分片。
	proof          persistence.DungeonResume         // 原入场编号、执行代次和回执。
	auth           persistence.Auth                  // 此观察使用的当前角色认证。
	conn           Connection                        // 当前绑定的新票或原票连接。
	sourceScene    uint64                            // 本次附着在原连接上替换的普通场景。
	scene          uint64                            // 该观察者看到的副本场景代次。
	baseline       uint64                            // 该观察者必须确认的副本基线。
	assets         roleAssetView                     // 角色分片冻结的只读库存和钱包。
	expiresAt      time.Time                         // 原场次截止后不开始新的健康续接。
	economicSeq    int64                             // 此会话最近确认的永久操作序号。
	hostingSeconds int64                             // 此会话最近确认的普通托管余额。
	present        atomic.Bool                       // 角色分片已经停止普通场景投影。
	sent           atomic.Bool                       // 场次分片已实际投递副本首帧。
	revoked        atomic.Bool                       // 原连接已断开或被接管。
	next           atomic.Pointer[dungeonAttachment] // 已经持久确认的新观察。
	stage          atomic.Uint32                     // 场次分片最后发布的原段号。
	shown          bool                              // 原角色分片当前是否观察该挑战。
	acked          bool                              // 当前观察绑定已正确确认首帧。
	deniedSession  int64                             // 当前原续接已明确拒绝的新票，不反复查证。
}

const (
	dungeonLoading     uint32 = iota + 1 // 等待原队员确认各自的入场基线。
	dungeonFighting                      // 当前战斗小区正在授权推进。
	dungeonSettling                      // 已停止动作，正在确认原终态。
	dungeonWon                           // 原终态已经确认通关。
	dungeonLost                          // 原终态已经确认战败或超时。
	dungeonInterrupted                   // 已确认系统中断及其退款。
)

// installDungeonBattle 从已扣次的原名单构造同一个场次；任一身体装配失败按系统中断收束。
func (w *worker) installDungeonBattle(store *persistence.Store, template *devconfig.DungeonConfig, grant persistence.DungeonChallenge, prepared []dungeonPreparedMember) []*dungeonAttachment {
	if w.dungeonArenas == nil {
		w.dungeonArenas = make(map[string]*dungeonArena)
	}
	if _, existing := w.dungeonArenas[grant.ChallengeID]; existing {
		return nil
	}
	now := time.Now()
	arena := newDungeonArena(grant, template, now)
	if arena == nil {
		arena = &dungeonArena{grant: grant, template: template, outcome: persistence.DungeonSystemInterrupted, finishedAt: now}
	}
	arena.store = store
	arena.observations = make(map[string]*dungeonAttachment, len(prepared))
	w.dungeonArenas[grant.ChallengeID] = arena
	result := make([]*dungeonAttachment, len(prepared))
	for i, member := range prepared {
		obs := &dungeonAttachment{actor: w, auth: member.auth, conn: member.conn, sourceScene: member.scene - 1, scene: member.scene, baseline: member.baseline, assets: member.assets,
			expiresAt: grant.ExpiresAt, economicSeq: member.role.EconomicSeq, hostingSeconds: member.role.HostingSeconds,
			proof: persistence.DungeonResume{ChallengeID: grant.ChallengeID, ReservationID: grant.ReservationID,
				Execution: grant.Execution, PreviousSessionEpoch: member.auth.SessionEpoch}}
		arena.observations[member.role.ID], result[i] = obs, obs
		if arena.outcome == persistence.DungeonActive && arena.addSeat(member.role, member.auth, store, member.snapshot, obs.scene, obs.baseline, obs.conn) == nil {
			arena.outcome, arena.finishedAt = persistence.DungeonSystemInterrupted, now
		}
	}
	if arena.outcome == persistence.DungeonActive && !w.addDungeonHires(arena) {
		arena.end(persistence.DungeonSystemInterrupted, now)
	}
	w.observeDungeonRewards(arena)
	return result
}

// activateDungeonMember 先保留同一原房间，再允许场次分片替换当前连接的画面。
func (w *worker) activateDungeonMember(owner *room, obs *dungeonAttachment) {
	owner.dungeonMatch = obs
	if owner.Conn != obs.conn || owner.Auth != obs.auth || !obs.conn.Alive() {
		obs.revoked.Store(true)
		return
	}
	owner.Manual, owner.PetBornReady = false, nil
	w.cancelTownNavigation(owner)
	w.leaveSharedTown(owner)
	obs.shown = true
	obs.present.Store(true)
}

// suspendDungeonObservation 停止旧输入，已确认的原身体留在场次内按自动战斗推进。
func (w *worker) suspendDungeonObservation(owner *room) {
	if obs := owner.dungeonMatch; obs != nil {
		obs.shown = false
		obs.revoked.Store(true)
	}
}

// resumeDungeonObservation 在新票的普通地图基线确认后，用原持久回执续接同一个身体。
func (w *worker) resumeDungeonObservation(owner *room, now time.Time) {
	prior := owner.dungeonMatch
	if prior == nil || prior.shown || owner.Isolated || owner.Pending || !owner.Ready || owner.Conn == nil || !owner.Conn.Alive() ||
		owner.Auth.SessionEpoch <= prior.auth.SessionEpoch || owner.Auth.SessionEpoch == prior.deniedSession || !now.Before(prior.expiresAt) ||
		owner.Role.Dungeon == nil || owner.Role.Dungeon.ActiveChallengeID != prior.proof.ChallengeID {
		return
	}
	scene := max(owner.Scene, prior.scene)
	baseline, ok := w.nextTravelBaseline(max(owner.Baseline, prior.baseline))
	if !ok || scene >= math.MaxUint64-1 {
		w.isolateRoom(owner)
		return
	}
	view, err := snapshotRoleAssets(owner.Role, w.roomStore(owner).Cfg.Fixture)
	if err != nil {
		w.isolateRoom(owner)
		return
	}
	obs := &dungeonAttachment{actor: prior.actor, proof: prior.proof, auth: owner.Auth, conn: owner.Conn, sourceScene: owner.Scene, scene: scene + 1, baseline: baseline, assets: view,
		expiresAt: prior.expiresAt, economicSeq: owner.Role.EconomicSeq, hostingSeconds: owner.Role.HostingSeconds}
	proof := prior.proof
	proof.PreviousSessionEpoch = prior.auth.SessionEpoch
	obs.proof.PreviousSessionEpoch = obs.auth.SessionEpoch
	store := w.roomStore(owner)
	owner.Pending = true
	owner.PendingOperationID = "dungeon:resume:" + proof.ChallengeID
	var retry job
	retry = job{target: w, settlement: true, run: func() func(*worker) {
		_, err := store.ResumeDungeon(w.e.ctx, obs.auth, proof)
		return func(w *worker) {
			if w.rooms[obs.auth.RoleID] != owner || owner.dungeonMatch != prior || owner.Auth != obs.auth {
				return
			}
			code := persistence.Code(err)
			if code == p.CodeOperationPending || code == p.CodeServerBusy {
				owner.PendingRetry, owner.RetryAt = &retry, time.Now().Add(250*time.Millisecond)
				return
			}
			if err == nil {
				w.activateDungeonMember(owner, obs)
				prior.next.Store(obs)
			} else {
				prior.deniedSession = obs.auth.SessionEpoch
			}
			owner.PendingRetry, owner.PendingOperationID = nil, ""
			owner.resumeAfterPending(time.Now())
			w.after(owner)
		}
	}}
	if !w.submit(retry) {
		owner.PendingRetry, owner.RetryAt = &retry, now.Add(250*time.Millisecond)
	}
}

// returnDungeonOwner 只有在同一原终态提交并且普通资产操作已结束后才替换观察。
func (w *worker) returnDungeonOwner(roleID string, challenge persistence.DungeonChallenge, confirmed domain.Role) bool {
	owner := w.rooms[roleID]
	if owner == nil || owner.Isolated {
		return true
	}
	if owner.Pending {
		return false
	}
	obs := owner.dungeonMatch
	if obs != nil && obs.proof.ChallengeID != challenge.ChallengeID {
		return true
	}
	if owner.Role.Revision <= confirmed.Revision && owner.Role.Dungeon != nil && owner.Role.Dungeon.ActiveChallengeID == challenge.ChallengeID {
		prior := owner.Role.PrivilegeUntil
		// 终态可能先于一次普通场的新票提交。角色内存保留当前凭据对应的会话版本。
		if confirmed.SessionEpoch == owner.Auth.SessionEpoch {
			owner.Role = confirmed
		} else {
			owner.Role.Dungeon = confirmed.Dungeon
		}
		w.acceptRolePrivilege(owner, time.Now(), prior)
	}
	owner.dungeonMatch = nil
	if obs == nil || !obs.shown || !obs.sent.Load() || owner.Conn != obs.conn || owner.Auth != obs.auth || !obs.conn.Alive() {
		return true
	}
	obs.revoked.Store(true)
	phase := dungeonLost
	if challenge.Outcome == persistence.DungeonSucceeded {
		phase = dungeonWon
	} else if challenge.Outcome == persistence.DungeonSystemInterrupted {
		phase = dungeonInterrupted
	}
	w.send(owner, p.MsgS2CDungeonBattleState, 0, &pb.DungeonBattleState{ChallengeId: domain.Bytes(challenge.ChallengeID), SceneEpoch: obs.scene,
		Stage: obs.stage.Load(), StageCount: 4, ExpiresAtUnixMs: challenge.ExpiresAt.UnixMilli(), State: phase})
	baseline, ok := w.nextTravelBaseline(obs.baseline)
	if !ok || obs.scene == math.MaxUint64 {
		w.isolateRoom(owner)
		return true
	}
	owner.Scene, owner.Baseline, owner.Ready = obs.scene+1, baseline, false
	owner.PetBornReady, owner.InputSequence, owner.TravelHistory, owner.Travel = nil, 0, nil, nil
	owner.AwaitUntil = time.Now().Add(10 * time.Second)
	if m, known := w.currentMap(owner); known && m.SceneKindValue() == devconfig.SceneKindTown && !w.attachSharedTown(owner, m) {
		owner.Conn.Close("shared town is full")
		return true
	}
	if entry := w.enterLineResult(owner); entry != nil {
		w.sendDungeonSceneTransition(owner, obs.scene, challenge.ChallengeID, entry)
		w.bindingState(owner, 1)
		w.baseline(owner)
	}
	return true
}
