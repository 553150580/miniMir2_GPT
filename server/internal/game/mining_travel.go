// 本文件核对独享矿层的入口和返程所需的普通现场。
package game

import "minimir/internal/devconfig"

// selectMiningDestination 允许从个人图、已附着城镇和其他矿层前往符合等级的目标矿区。
func (w *worker) selectMiningDestination(r *room, current *devconfig.MapTemplate, mapID uint64) (*devconfig.MapTemplate, bool) {
	f := &w.roomStore(r).Cfg.Fixture
	layer, ok := f.MiningLayer(mapID)
	if !ok || current.ID == mapID || r.Role.Level < layer.MinimumLevel || r.PersonalWorld == nil {
		return nil, false
	}
	fromPersonal := current.SceneKindValue() == devconfig.SceneKindPersonal && r.currentWorld() == r.PersonalWorld && r.SavedPersonal == nil
	fromTown := current.SceneKindValue() == devconfig.SceneKindTown && f.TownDestination(current.ID)
	fromSideMap := r.SavedPersonal == r.PersonalWorld && (fromTown || miningMap(f, current.ID))
	if !fromPersonal && !fromSideMap {
		return nil, false
	}
	return &layer.Map, true
}
