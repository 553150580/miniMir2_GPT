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
	enabled  bool
	disabled []uint32
}

func (s *autoSkillSettings) permits(id uint32) bool {
	if s == nil {
		return true
	}
	_, disabled := slices.BinarySearch(s.disabled, id)
	return s.enabled && !disabled
}

func (s *autoSkillSettings) proto() *pb.AutoSkillSettings {
	if s == nil {
		return &pb.AutoSkillSettings{Enabled: true}
	}
	return &pb.AutoSkillSettings{Enabled: s.enabled, DisabledSkillIds: slices.Clone(s.disabled)}
}

func automaticSkillForJob(skill devconfig.Skill, job uint32) bool {
	handler, known := skillHandlers[skill.Behavior]
	// 定点挂机的自动技能不能进入位移或跨图执行路径。
	return known && !handler.Passive && !handler.Displacement && !handler.Teleport &&
		skill.AutoEnabled && skill.Activation() == "cast" && skill.JobID == job
}

// 空字段由调用方保留room的既有偏好；整份新偏好核验成功后才可以替换。
func (w *worker) validateAutoSkillSettings(r *room, incoming *pb.AutoSkillSettings) (*autoSkillSettings, bool) {
	f := &w.e.Store.Cfg.Fixture
	if len(incoming.DisabledSkillIds) > len(f.Skills) {
		return nil, false
	}
	ids := slices.Clone(incoming.DisabledSkillIds)
	slices.Sort(ids)
	for i, id := range ids {
		skill, exists := f.Skill(id)
		if !exists || !automaticSkillForJob(skill, r.Role.Job) || i > 0 && id == ids[i-1] {
			return nil, false
		}
	}
	return &autoSkillSettings{enabled: incoming.Enabled, disabled: ids}, true
}

func (w *worker) autoBattleIntent(r *room, frame wire.Frame) {
	var q pb.AutoBattleIntent
	if proto.Unmarshal(frame.Payload, &q) != nil || q.SceneEpoch != r.Scene || q.MonsterCount > 8 {
		w.failure(r, frame, p.CodeInvalidArgument)
		return
	}
	var settings *autoSkillSettings
	if q.AutoSkillSettings != nil {
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
	if m, ok := w.currentMap(r); !ok || m.SceneKindValue() == devconfig.SceneKindTown {
		w.failure(r, frame, p.CodeLineForbidden)
		w.hosting(r, frame.Request)
		return
	}
	if q.MonsterCount > 0 {
		r.MonsterCount = int(q.MonsterCount)
	}
	if settings != nil {
		r.AutoSkills = settings
	}
	r.Auto = q.Enabled
	w.hosting(r, frame.Request)
	w.snapshot(r)
}
