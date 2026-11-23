// 本文件管理紧凑城镇内按运行服隔离的玩家席位、占格和跨分片观察状态。
package game

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/townnav"
	"slices"
	"sort"
	"sync"
	"time"
)

// 共享观察句柄独立于每个房间的本地玩家、怪物和宠物句柄空间。
const sharedTownHandleFirst uint32 = 1 << 31

// sharedTownKey 限定可互相观察的运行服和城镇。一个城镇目前只有一条线。
type sharedTownKey struct {
	runID uint64 // 运行服编号，合服后的多个入口服可以进入同一场景。
	mapID uint64 // 紧凑城镇的配置地图编号。
}

// townMembership 是房间所有者持有的入镇凭据。旧连接不得使用此处的当前代次。
type townMembership struct {
	key   sharedTownKey // 已占用的共享场景。
	owner uint64        // 角色临时执行代次，跨冷重建时不会复用。
}

// sharedTownResident 存放跨分片所需的值，不引用可变角色房间或战斗实体。
type sharedTownResident struct {
	handle   uint32          // 其他玩家观察此角色时使用的句柄。
	position devconfig.Point // 当前玩家占格；已死亡的玩家保留表现但不占格。
	blocking bool            // 活玩家占格或正在确认入场的出生格。
	view     *pb.EntityState // 当前确认附着的不可变表现，nil 表示等待入场确认。
}

// sharedTown 保存一条城镇线的玩家席位和权威占格。
type sharedTown struct {
	capacity  int                            // 以可走格数限制席位，死亡角色恢复时仍保有空格。
	residents map[uint64]*sharedTownResident // 按角色执行代次索引入镇席位。
	occupied  map[devconfig.Point]uint64     // 当前位置到该占格执行代次的唯一映射。
}

// sharedTownDirectory 以短临界区交换占格和观察值，角色战斗仍由各自分片执行。
type sharedTownDirectory struct {
	mu         sync.Mutex                    // 保护跨城镇的转移、占格和读取。
	nextHandle uint32                        // 最近分配的共享观察句柄，进程内不复用。
	towns      map[sharedTownKey]*sharedTown // 只保留当前存在席位的城镇。
}

// sharedTownKeyFor 为可共享城镇取出当前角色已获授权的运行服身份。
func (w *worker) sharedTownKeyFor(r *room, m *devconfig.MapTemplate) (sharedTownKey, bool) {
	if m == nil || !m.CompactTown || m.SceneKindValue() != devconfig.SceneKindTown || r.Conn == nil || !r.Conn.Alive() || r.Isolated {
		return sharedTownKey{}, false
	}
	store := w.roomStore(r)
	if store == nil || store.Cfg.RunID == 0 {
		return sharedTownKey{}, false
	}
	return sharedTownKey{runID: store.Cfg.RunID, mapID: m.ID}, true
}

// removeLocked 回收格子和席位，并释放已清空的城镇现场。
func (d *sharedTownDirectory) removeLocked(membership townMembership) {
	town := d.towns[membership.key]
	if town == nil {
		return
	}
	resident := town.residents[membership.owner]
	if resident == nil {
		return
	}
	if resident.blocking {
		delete(town.occupied, resident.position)
	}
	delete(town.residents, membership.owner)
	if len(town.residents) == 0 {
		delete(d.towns, membership.key)
	}
}

// leaveSharedTown 在换图、断线、隔离和关闭的原分片上释放席位。
func (w *worker) leaveSharedTown(r *room) {
	if r.townMembership.owner == 0 {
		return
	}
	d := &w.e.sharedTowns
	d.mu.Lock()
	d.removeLocked(r.townMembership)
	d.mu.Unlock()
	r.townMembership = townMembership{}
}

// sharedTownLanding 在首选点被占时选择最近的空闲可走格，同行宠物的落点也保留。
// 地图按有界紧凑图的稳定行序扫描；瞬息移动调用方可要求落点保持精确。
func sharedTownLanding(m *devconfig.MapTemplate, preferred devconfig.Point, pet *devconfig.Point, occupied map[devconfig.Point]uint64, owner uint64, alternate bool) (devconfig.Point, bool) {
	available := func(point devconfig.Point) bool {
		other := occupied[point]
		return (other == 0 || other == owner) && (pet == nil || point != *pet) && m.Walkable(point)
	}
	if available(preferred) {
		return preferred, true
	}
	if !alternate {
		return devconfig.Point{}, false
	}
	best, bestDistance := devconfig.Point{}, math.MaxInt
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			point := devconfig.Point{X: x, Y: y}
			if score := distance(preferred, point); score < bestDistance && available(point) {
				best, bestDistance = point, score
			}
		}
	}
	return best, bestDistance != math.MaxInt
}

// reserveSharedTown 原子更换城镇席位，目标满员时保留来源场景和席位。
// 真实换图或重连的席位等待入场确认后才可见；同图瞬移保留原观察身份。
func (w *worker) reserveSharedTown(r *room, m *devconfig.MapTemplate, preferred devconfig.Point, pet *devconfig.Point, alternate, hide bool) (devconfig.Point, bool) {
	key, shared := w.sharedTownKeyFor(r, m)
	if !shared {
		w.leaveSharedTown(r)
		return preferred, true
	}
	owner := r.currentWorld().Ref.Epoch
	d := &w.e.sharedTowns
	d.mu.Lock()
	defer d.mu.Unlock()
	town := d.towns[key]
	if town == nil {
		town = &sharedTown{residents: make(map[uint64]*sharedTownResident), occupied: make(map[devconfig.Point]uint64)}
		for y := 0; y < m.Height; y++ {
			for x := 0; x < m.Width; x++ {
				if m.Walkable(devconfig.Point{X: x, Y: y}) {
					town.capacity++
				}
			}
		}
	}
	resident := town.residents[owner]
	if resident == nil && (len(town.residents) >= town.capacity || d.nextHandle == math.MaxUint32) {
		return devconfig.Point{}, false
	}
	point, ok := sharedTownLanding(m, preferred, pet, town.occupied, owner, alternate)
	if !ok {
		return devconfig.Point{}, false
	}
	if resident == nil {
		if d.nextHandle < sharedTownHandleFirst {
			d.nextHandle = sharedTownHandleFirst
		}
		d.nextHandle++
		resident = &sharedTownResident{handle: d.nextHandle}
		d.removeLocked(r.townMembership)
		if d.towns == nil {
			d.towns = make(map[sharedTownKey]*sharedTown)
		}
		d.towns[key], town.residents[owner] = town, resident
		r.townMembership = townMembership{key: key, owner: owner}
	} else if resident.blocking {
		delete(town.occupied, resident.position)
	}
	if hide {
		resident.view = nil
	}
	resident.position, resident.blocking = point, r.Player.HP > 0
	if resident.blocking {
		town.occupied[point] = owner
	}
	return point, true
}

// attachSharedTown 在重连握手前保留角色落点，旧位置被后来者占用时重新分配空格。
func (w *worker) attachSharedTown(r *room, m *devconfig.MapTemplate) bool {
	var petPosition *devconfig.Point
	if pet := r.worldPet(r.currentWorld()); pet != nil && pet.HP > 0 {
		petPosition = &pet.Pos
	}
	point, ok := w.reserveSharedTown(r, m, r.Player.Pos, petPosition, true, true)
	if !ok || point == r.Player.Pos {
		return ok
	}
	if r.CellEntrySequence == math.MaxUint64 || !w.beginMovement(r, &r.Player, movementTeleport) || !w.commitRelocation(r, &r.Player, point) {
		w.leaveSharedTown(r)
		return false
	}
	return true
}

// townCellFree 检查共享占格并允许寻路者忽略自己的当前位置。
func (w *worker) townCellFree(r *room, e *entity, point devconfig.Point) bool {
	membership := r.townMembership
	if membership.owner == 0 {
		return true
	}
	d := &w.e.sharedTowns
	d.mu.Lock()
	defer d.mu.Unlock()
	town := d.towns[membership.key]
	if town == nil {
		return false
	}
	owner := town.occupied[point]
	return owner == 0 || owner == membership.owner && e == &r.Player
}

// moveSharedTownPlayer 在实际落格处再次核对占位，防止两个分片同时走进同一空格。
func (w *worker) moveSharedTownPlayer(r *room, point devconfig.Point) bool {
	membership := r.townMembership
	if membership.owner == 0 {
		return true
	}
	d := &w.e.sharedTowns
	d.mu.Lock()
	defer d.mu.Unlock()
	town := d.towns[membership.key]
	if town == nil {
		return false
	}
	resident, occupant := town.residents[membership.owner], town.occupied[point]
	if resident == nil || r.Player.HP > 0 && occupant != 0 && occupant != membership.owner {
		return false
	}
	if resident.blocking {
		delete(town.occupied, resident.position)
	}
	resident.position, resident.blocking = point, r.Player.HP > 0
	if resident.blocking {
		town.occupied[point] = membership.owner
	}
	return true
}

// reserveSharedTownRevival 为已经持久确认的复活占下空格，再由房间恢复生命。
func (w *worker) reserveSharedTownRevival(r *room, m *devconfig.MapTemplate) (devconfig.Point, bool) {
	membership := r.townMembership
	if membership.owner == 0 {
		return m.PlayerSpawn, true
	}
	d := &w.e.sharedTowns
	d.mu.Lock()
	defer d.mu.Unlock()
	town := d.towns[membership.key]
	if town == nil || town.residents[membership.owner] == nil {
		return devconfig.Point{}, false
	}
	point, ok := sharedTownLanding(m, m.PlayerSpawn, nil, town.occupied, membership.owner, true)
	if !ok {
		return devconfig.Point{}, false
	}
	resident := town.residents[membership.owner]
	if resident.blocking {
		delete(town.occupied, resident.position)
	}
	resident.position, resident.blocking = point, true
	town.occupied[point] = membership.owner
	return point, true
}

// appendSharedTownBlocks 把其他城镇玩家占格补进寻路工作区，供本次搜索读取。
func (w *worker) appendSharedTownBlocks(r *room, dst []townnav.Point) ([]townnav.Point, bool) {
	membership := r.townMembership
	if membership.owner == 0 {
		return dst, true
	}
	d := &w.e.sharedTowns
	d.mu.Lock()
	town := d.towns[membership.key]
	if town == nil {
		d.mu.Unlock()
		return dst, false
	}
	start := len(dst)
	for point, owner := range town.occupied {
		if owner == membership.owner {
			continue
		}
		cell := townPoint(point)
		if slices.Contains(dst[:start], cell) {
			continue
		}
		if len(dst) == cap(dst) {
			d.mu.Unlock()
			return dst, false
		}
		dst = append(dst, cell)
	}
	d.mu.Unlock()
	// 固定占位遍历次序，避免仅因 Go map 遍历换序触发无意义的重规划。
	sort.Slice(dst[start:], func(i, j int) bool {
		a, b := dst[start+i], dst[start+j]
		return a.Y < b.Y || a.Y == b.Y && a.X < b.X
	})
	return dst, true
}

// appendSharedTownViews 发布本人的成功入场视图，并为基线附上其他角色不可变快照。
func (w *worker) appendSharedTownViews(r *room, out *pb.SceneBaselineChunk) {
	membership := r.townMembership
	if membership.owner == 0 || len(out.Entities) == 0 {
		return
	}
	d := &w.e.sharedTowns
	d.mu.Lock()
	town := d.towns[membership.key]
	if town == nil {
		d.mu.Unlock()
		return
	}
	if resident := town.residents[membership.owner]; r.Ready && r.Conn != nil && r.Conn.Alive() && resident != nil {
		player := out.Entities[0]
		if player.Hp <= 0 && resident.blocking {
			delete(town.occupied, resident.position)
			resident.blocking = false
		}
		resident.view = &pb.EntityState{EntityHandle: resident.handle, EntityKind: player.EntityKind, DefinitionId: player.DefinitionId,
			DisplayName: r.Role.Name, Position: pointPB(r.Player.Pos), Direction: player.Direction, Hp: player.Hp, HpMax: player.HpMax, Action: player.Action,
			MoveIntervalMs: player.MoveIntervalMs, Life: player.Life, MovementRevision: player.MovementRevision, MovementKind: player.MovementKind}
	}
	start := len(out.Entities)
	for owner, resident := range town.residents {
		if owner != membership.owner && resident.view != nil {
			out.Entities = append(out.Entities, resident.view)
		}
	}
	d.mu.Unlock()
	sort.Slice(out.Entities[start:], func(i, j int) bool {
		return out.Entities[start+i].EntityHandle < out.Entities[start+j].EntityHandle
	})
}

// snapshotSharedTownWhenDue 让暂时不能行走或战斗的角色仍可观察同镇人员变化。
func (w *worker) snapshotSharedTownWhenDue(r *room, now time.Time) {
	if r.townMembership.owner == 0 || !r.Ready || now.Sub(r.LastBroadcast) < 200*time.Millisecond {
		return
	}
	r.LastBroadcast = now
	w.snapshot(r)
}
