// 本文件校验职业可用的自动技能配置，独立维护自动战斗开关与自动释放偏好。
package game

import (
	"slices"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"

	"google.golang.org/protobuf/proto"
)

// 偏好属于角色的当前执行现场，换地图时由同一room保存，冷入场可由客户端恢复。
type autoSkillSettings struct {
	// enabled 标记是否处于已开启状态。
	enabled bool
	// disabled 记录“disabled”，使用数值列表表示。
	disabled []uint32
}

// permits 判断“permits”是否满足要求。
// s 是调用该方法的自动技能“settings”实例，id 区分当前步骤指向的目标对象。
func (s *autoSkillSettings) permits(id uint32) bool {
	if s == nil {
		return true
	}
	// disabled 保存由“binary”“search”取得的“disabled”。
	_, disabled := slices.BinarySearch(s.disabled, id)
	return s.enabled && !disabled
}

// proto 返回协议供调用方使用。
// s 是调用该方法的自动技能“settings”实例。
func (s *autoSkillSettings) proto() *pb.AutoSkillSettings {
	if s == nil {
		return &pb.AutoSkillSettings{Enabled: true}
	}
	return &pb.AutoSkillSettings{Enabled: s.enabled, DisabledSkillIds: slices.Clone(s.disabled)}
}

// automaticSkillForJob 判断自动技能“for”任务或职业编号是否满足要求。
// skill 记录技能，job 记录任务或职业编号，使用数值表示。
func automaticSkillForJob(skill devconfig.Skill, job uint32) bool {
	// handler 保存从技能处理器集合取出的处理器，known 保存从技能处理器集合取出的已知状态。
	handler, known := skillHandlers[skill.Behavior]
	// 定点挂机的自动技能不能进入位移或跨图执行路径。
	return known && !handler.Passive && !handler.Displacement && !handler.Teleport &&
		skill.AutoEnabled && skill.Activation() == "cast" && skill.JobID == job
}

// 空字段由调用方保留room的既有偏好；整份新偏好核验成功后才可以替换。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，incoming 记录新到达项，使用自动技能“settings”表示。
func (w *worker) validateAutoSkillSettings(r *room, incoming *pb.AutoSkillSettings) (*autoSkillSettings, bool) {
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	if len(incoming.DisabledSkillIds) > len(f.Skills) {
		return nil, false
	}
	// ids 保存由副本取得的标识集合。
	ids := slices.Clone(incoming.DisabledSkillIds)
	slices.Sort(ids)
	// i 定位标识集合中正在处理的成员，id 接收遍历标识集合时当前的元素。
	for i, id := range ids {
		// skill 接收技能供后续步骤使用，exists 保存由技能取得的存在。
		skill, exists := f.Skill(id)
		if !exists || skill.JobID != r.Role.Job || !automaticSkillForJob(skill, r.Role.Job) && !skill.AttackSlotSummon() || i > 0 && id == ids[i-1] {
			return nil, false
		}
	}
	return &autoSkillSettings{enabled: incoming.Enabled, disabled: ids}, true
}

// autoBattleIntent 处理自动战斗意图。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，frame 记录协议帧。
func (w *worker) autoBattleIntent(r *room, frame wire.Frame) {
	// q 记录自动战斗意图。
	var q pb.AutoBattleIntent
	if proto.Unmarshal(frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.MonsterCount > devconfig.OrdinaryMonsterSlots {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	// settings 记录“settings”，使用自动技能“settings”表示。
	var settings *autoSkillSettings
	if q.AutoSkillSettings != nil {
		// valid 标记是否处于有效状态。
		var valid bool
		settings, valid = w.validateAutoSkillSettings(r, q.AutoSkillSettings)
		if !valid {
			w.failure(r, frame, p.CodeInvalidArgument)
			w.hosting(r, frame.Request)
			return
		}
		if q.MonsterCount == 0 {
			r.AutoSkills = settings
			w.hosting(r, frame.Request)
			return
		}
	}
	m, ok := w.currentMap(r)
	if !ok || m.SceneKindValue() == devconfig.SceneKindTown {
		w.failure(r, frame, p.CodeLineForbidden)
		w.hosting(r, frame.Request)
		return
	}
	if q.MonsterCount > 0 && !w.setVisibleMonsterCount(r, m, int(q.MonsterCount)) {
		w.failure(r, frame, p.CodeServerBusy)
		w.hosting(r, frame.Request)
		return
	}
	if settings != nil {
		r.AutoSkills = settings
	}
	r.Auto = q.Enabled
	if !r.Auto {
		r.AutoTarget = actorRef{}
	} else {
		r.liveAutoTarget()
	}
	w.hosting(r, frame.Request)
	w.snapshot(r)
}
