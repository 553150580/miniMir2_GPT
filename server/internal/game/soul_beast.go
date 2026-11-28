// 本文件校验魂兽请求，并读取同一只附体魂兽的战斗投影。
package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	"minimir/internal/wire"
)

// readSoulAction 与角色队列、重连回执和首次写入共用原协议形状。
func readSoulAction(frame wire.Frame) (*pb.SoulActionRequest, bool) {
	var request pb.SoulActionRequest
	if proto.Unmarshal(frame.Payload, &request) != nil || !persistence.ValidSoulAction(&request) {
		return nil, false
	}
	return &request, true
}

// soulBonuses 仅根据当前角色聚合的附体星级和三池配置计算贡献。
func (w *worker) soulBonuses(r *room) devconfig.SoulBonuses {
	rules, state := w.e.Store.Cfg.Fixture.Souls, r.Role.Soul
	if rules == nil || state == nil || state.Attached == 0 {
		return devconfig.SoulBonuses{}
	}
	index := state.Index(state.Attached)
	if index < 0 {
		return devconfig.SoulBonuses{}
	}
	beast := state.Beasts[index]
	return rules.Bonuses(beast.Star, beast.Affix)
}
