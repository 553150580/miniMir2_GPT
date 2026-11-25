// 本文件维护一场讨伐链的原席位、共享地图、敌人和不可逆战斗终态。
package game

import (
	"slices"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// dungeonArena 由同一个战斗分片串行更新。所有成员引用同一组地图格和敌人。
type dungeonArena struct {
	grant          persistence.DungeonChallenge        // 原扣次事务固定的场次、成员和截止时刻。
	template       *devconfig.DungeonConfig            // 原版本的四段配置快照。
	store          *persistence.Store                  // 本场运行服的持久化入口。
	seats          map[string]*dungeonCombatSeat       // 真人使用原角色编号，佣兵使用仅在本场存在的席位键。
	hireSeats      []*dungeonCombatSeat                // 佣兵按开战事务确认的选项顺序排列。
	geometry       devconfig.MapTemplate               // 已开放小区构成的共享可通行地形。
	monsters       []*entity                           // 仅含当前段的共享敌人身体。
	stage          int                                 // 当前段的零基索引。
	openedAt       time.Time                           // 当前段开始接受本段动作的时刻。
	clearedAt      time.Time                           // 当前段最后一头敌人死亡的权威时刻。
	lastTick       time.Time                           // 场次已推进的逻辑时刻。
	finishedAt     time.Time                           // 第一次权威终态形成的时刻。
	rewardAt       time.Time                           // 最终首领实际死亡的逻辑时刻，可能早于最终段清空。
	rewardWatches  []*challengePrivilegeWitness        // 按原真人席位顺序观察普通角色已确认的特权变化。
	rewardEvidence []persistence.DungeonRewardEvidence // 击杀时冻结的原真人权益，失败时不交给资产事务。
	outcome        persistence.DungeonOutcome          // 互斥的成功、战败或系统中断。
	ledger         dungeonFinishState                  // 原终态提案和提交结果。
	castID         uint64                              // 全场唯一施法流水。
	eventID        uint64                              // 全场唯一可见战斗事件流水。
	nextHandle     uint32                              // 下一只敌人的场内句柄，不复用旧段编号。
	observations   map[string]*dungeonAttachment       // 临时战斗身体附着的当前输入和观测。
	reportedStage  int                                 // 已向席位报告的段索引。
}

// dungeonCombatSeat 保存原角色的临时战斗身体和首次确认状态。
type dungeonCombatSeat struct {
	room    *room // 本席位的输入、动作队列和只读角色战斗快照。
	started bool  // 真人首次地图确认后为真，佣兵的身体在开战首帧已装配。
	hired   bool  // 佣兵没有角色分片或当前附着，仅在共享场内处理战斗事实。
}

// newDungeonArena 根据已确认的持久场次装配第一段，不创建普通收益或刷新任务。
func newDungeonArena(grant persistence.DungeonChallenge, template *devconfig.DungeonConfig, now time.Time) *dungeonArena {
	if grant.Outcome != persistence.DungeonActive || grant.Execution == 0 || template == nil ||
		template.TemplateID != grant.TemplateID || len(template.Stages) == 0 {
		return nil
	}
	a := &dungeonArena{grant: grant, template: template, openedAt: grant.StartedAt, lastTick: now,
		seats:        make(map[string]*dungeonCombatSeat, len(grant.Members)+len(grant.HireOptionIDs)),
		observations: make(map[string]*dungeonAttachment, len(grant.Members)), nextHandle: 1000}
	a.openStage()
	return a
}

// addSeat 按持久名单顺序安排出生和句柄，不接受原场次之外的成员或替换已存在的身体。
func (a *dungeonArena) addSeat(role domain.Role, auth persistence.Auth, store *persistence.Store, snapshot personalBossSnapshot,
	scene, baseline uint64, conn Connection) *dungeonCombatSeat {
	index := slices.IndexFunc(a.grant.Members, func(member persistence.DungeonMember) bool {
		return member.RoleID == role.ID && member.AccountID == auth.AccountID && member.Origin == role.Origin &&
			member.SessionEpoch == auth.SessionEpoch && auth.RoleID == role.ID
	})
	spawns := a.template.Stages[0].PlayerSpawns
	if index < 0 || index >= len(spawns) || a.seats[role.ID] != nil || a.stage != 0 || a.outcome != persistence.DungeonActive {
		return nil
	}
	a.store = store
	ref := worldRef{Epoch: a.grant.Execution, ID: 1}
	world := &worldState{Ref: ref, MapID: a.geometry.ID, BossMap: &a.geometry, Monsters: a.monsters, MonsterCount: len(a.monsters), Auto: true}
	role.Skills = slices.Clone(snapshot.Skills)
	role.SkillSlotIDs = slices.Clone(snapshot.SkillSlotIDs)
	child := &room{worldState: world, runStore: store, ExecutionEpoch: a.grant.Execution, WorldSequence: ref.ID,
		Auth: auth, Conn: conn, Scene: scene, Baseline: baseline, Role: role, AutoSkills: snapshot.autoSkills,
		Player: entity{Handle: uint32(index + 1), Kind: 1, Def: role.Job, Pos: spawns[index],
			HP: snapshot.Fighter.HP, MaxHP: snapshot.Fighter.HP, MP: snapshot.Fighter.MP, MaxMP: snapshot.Fighter.MP, Life: 1},
		bossBattle: &personalBossBattle{fighter: snapshot.Fighter}, dungeonCombat: a,
		AwaitUntil: minBossTime(a.lastTick.Add(10*time.Second), a.grant.ExpiresAt), LastTick: a.lastTick, LastBroadcast: a.lastTick}
	seat := &dungeonCombatSeat{room: child}
	a.seats[role.ID] = seat
	if snapshot.Pet != nil && snapshot.Pet.HP > 0 && !mapForbidsPets(&a.geometry) {
		a.placePet(child, snapshot.Pet, uint32(101+index), spawns)
	}
	return seat
}

// acceptsActions 把输入和迟到结算限制在当前小区开启后的原截止窗口。
func (a *dungeonArena) acceptsActions(at time.Time) bool {
	return a.outcome == persistence.DungeonActive && a.clearedAt.IsZero() &&
		!at.Before(a.openedAt) && at.Before(a.grant.ExpiresAt)
}

// placePet 为已批准的临时宠物分配唯一身体，并避开其余成员的预约出生点。
func (a *dungeonArena) placePet(child *room, frozen *entity, handle uint32, spawns []devconfig.Point) {
	for _, direction := range directions {
		point := devconfig.Point{X: child.Player.Pos.X + direction.X, Y: child.Player.Pos.Y + direction.Y}
		if !a.geometry.Walkable(point) || slices.Contains(spawns, point) || a.occupied(point, nil) {
			continue
		}
		body := *frozen
		body.Handle, body.Pos = handle, point
		if body.PetRuntime != nil {
			runtime := *body.PetRuntime
			body.PetRuntime = &runtime
		}
		child.Pet, child.PetWorld, child.PetLifeSequence = &body, child.Ref, body.Life
		child.recordPetPosition()
		return
	}
}

// eachSeat 按原真人名单与已批准的佣兵顺序仲裁并列动作和怪物目标。
func (a *dungeonArena) eachSeat(visit func(*dungeonCombatSeat)) {
	for _, member := range a.grant.Members {
		if seat := a.seats[member.RoleID]; seat != nil {
			visit(seat)
		}
	}
	for _, seat := range a.hireSeats {
		visit(seat)
	}
}

// eachLiving 访问全场真实活体，包括未确认席位占用的出生格和已断线的原身体。
func (a *dungeonArena) eachLiving(visit func(*entity)) {
	for _, monster := range a.monsters {
		if monster.HP > 0 {
			visit(monster)
		}
	}
	a.eachSeat(func(seat *dungeonCombatSeat) {
		child := seat.room
		if child.Player.HP > 0 {
			visit(&child.Player)
		}
		if child.Pet != nil && child.Pet.HP > 0 {
			visit(child.Pet)
		}
	})
}

// occupied 判断目标格是否由另一个存活身体占用。
func (a *dungeonArena) occupied(point devconfig.Point, self *entity) bool {
	occupied := false
	a.eachLiving(func(body *entity) {
		if body != self && body.Pos == point {
			occupied = true
		}
	})
	return occupied
}

// openStage 追加开放一个小区，原走过的区域仍可通行，新敌人使用全新句柄。
func (a *dungeonArena) openStage() {
	stage := &a.template.Stages[a.stage]
	mapTemplate := &a.template.Map
	a.geometry = *mapTemplate
	a.geometry.Blocked = slices.Clone(mapTemplate.Blocked)
	for y := range mapTemplate.Height {
		for x := range mapTemplate.Width {
			point := devconfig.Point{X: x, Y: y}
			if !slices.Contains(mapTemplate.Blocked, point) &&
				!slices.ContainsFunc(a.template.Stages[:a.stage+1], func(open devconfig.DungeonStage) bool { return open.Area.Contains(point) }) {
				a.geometry.Blocked = append(a.geometry.Blocked, point)
			}
		}
	}
	a.monsters = make([]*entity, 0, len(stage.Enemies))
	for index := range stage.Enemies {
		enemy := &stage.Enemies[index]
		a.monsters = append(a.monsters, &entity{Handle: a.nextHandle, Kind: 3, Def: enemy.Monster.ID, Pos: enemy.Spawn,
			HP: enemy.Monster.HP, MaxHP: enemy.Monster.HP, MP: enemy.Monster.MP, MaxMP: enemy.Monster.MP,
			Monster: &enemy.Monster, Life: uint64(a.stage + 1)})
		a.nextHandle++
	}
}

// end 固定唯一场次终态并封住旧动作；持久化由场次结算入口执行。
func (a *dungeonArena) end(outcome persistence.DungeonOutcome, at time.Time) {
	if a.outcome != persistence.DungeonActive || outcome == persistence.DungeonActive {
		return
	}
	a.outcome, a.finishedAt = outcome, at
	a.eachSeat(func(seat *dungeonCombatSeat) { resetDungeonActions(seat.room) })
}

// resetDungeonActions 阻止原段在途攻击、方向和导航状态泄漏到新段或终态。
func resetDungeonActions(child *room) {
	world := child.currentWorld()
	world.Attacks, world.Casts, world.FireWalls, world.TrapHexagon = nil, nil, nil, nil
	child.AutoTarget, child.Manual, child.Destination = actorRef{}, false, nil
	if child.Navigation.Status == navigationActive {
		child.Navigation.Status = navigationCancelled
	}
}
