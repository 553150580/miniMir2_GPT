// 本文件控制共享首领席位的健康重连，撤销旧会话但保留原战斗身体和贡献。
package game

import (
	"math"
	"minimir/internal/devconfig"
	"time"
)

// shouldSuspendWorldBoss 判断原观战连接是否已断开或被健康会话接管。
func shouldSuspendWorldBoss(seat *worldBossSeat) bool {
	obs := seat.observation
	return !seat.room.Isolated && seat.room.Player.HP > 0 && (obs.rejoin.Load() || !obs.conn.Alive() || obs.conn.Binding() != uint64(obs.auth.SessionEpoch))
}

// suspendWorldBossSeat 暂停原战斗身体，撤销旧输入且保留已确认的生命和参与事实。
func (w *worker) suspendWorldBossSeat(arena *worldBossArena, seat *worldBossSeat) {
	child, obs := seat.room, seat.observation
	if arena.challenge.participants[child.Role.ID] == nil {
		w.removeWorldBossSeat(arena, seat, worldBossLeft)
		return
	}
	delete(arena.seats, child.Role.ID)
	obs.revoked.Store(true)
	seat.priorEpoch, seat.observation = obs.auth.SessionEpoch, nil
	seat.shown, seat.done = false, false
	child.Conn, child.Ready, child.PetBornReady = nil, false, nil
	// 掉线后不追放未实际命中的动作。冷却、状态、生命、魔法和出生身份保持原值。
	child.currentWorld().Attacks, child.currentWorld().Casts = nil, nil
	child.Manual, child.Destination = false, nil
	child.Navigation = navigationState{}
	arena.resumable[child.Role.ID] = seat
}

// resumeSeat 只用新认证恢复原角色。旧占格已有人使用时安全地重新安置同一个身体。
func (a *worldBossArena) resumeSeat(enter worldBossEnter, seat *worldBossSeat, now time.Time) bool {
	obs := enter.observation
	child := seat.room
	if obs.auth.RoleID != child.Role.ID || obs.auth.SessionEpoch <= seat.priorEpoch ||
		child.bossBattle.grant.challenge != a.challenge.identity || child.Player.MovementRevision == math.MaxUint64 ||
		child.Pet != nil && child.Pet.MovementRevision == math.MaxUint64 {
		return false
	}
	occupied := make(map[devconfig.Point]uint64)
	a.eachLiving(func(body *entity) { occupied[body.Pos] = 1 })
	point, ok := sharedTownLanding(&a.config.Map, child.Player.Pos, nil, occupied, 0, true)
	if !ok {
		return false
	}
	var petPoint devconfig.Point
	if child.Pet != nil && child.Pet.HP > 0 {
		petPoint, ok = sharedTownLanding(&a.config.Map, child.Pet.Pos, &point, occupied, 0, true)
		if !ok {
			return false
		}
	}
	if child.Player.Pos != point {
		child.Player.Pos = point
		child.Player.MovementRevision++
		child.Player.MovementKind = movementTeleport
	}
	if child.Pet != nil && child.Pet.HP > 0 && child.Pet.Pos != petPoint {
		child.Pet.Pos = petPoint
		child.Pet.MovementRevision++
		child.Pet.MovementKind = movementTeleport
	}
	delete(a.resumable, child.Role.ID)
	a.seats[child.Role.ID] = seat
	seat.observation = obs
	child.Auth, child.Conn, child.Scene, child.Baseline = obs.auth, obs.conn, obs.scene, obs.baseline
	child.Role.EconomicSeq, child.Role.Revision = enter.role.EconomicSeq, enter.role.Revision
	child.Ready, child.InputSequence, child.Sequence = false, 0, 0
	child.AwaitUntil = minBossTime(now.Add(10*time.Second), a.occurrence.closesAt)
	child.LastTick, child.LastBroadcast = now, now
	return true
}
