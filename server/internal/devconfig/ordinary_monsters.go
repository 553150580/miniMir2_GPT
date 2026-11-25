// 本文件校验普通怪物目录，并按地图出生槽解析静态模板。
package devconfig

import (
	"errors"
	"fmt"
)

// OrdinaryMonsterRosterRule 开启每个普通地图的独立怪物模板和地图选择。
const OrdinaryMonsterRosterRule = 30

// OrdinaryMonsterSlots 是角色能调节的普通怪物名额上限。
const OrdinaryMonsterSlots = 8

// OrdinaryMonsterRosterEnabled 判断当前夹具是否采用分地图普通怪物目录。
func (f *Fixture) OrdinaryMonsterRosterEnabled() bool {
	return f.RuleVersion >= OrdinaryMonsterRosterRule
}

// MonsterByID 按稳定编号读取共享的怪物静态模板。
func (f *Fixture) MonsterByID(id uint32) (*Monster, bool) {
	for i := range f.Monsters {
		if f.Monsters[i].ID == id {
			return &f.Monsters[i], true
		}
	}
	return nil, false
}

// validateOrdinaryMonsterRoster 校验每个出生槽的明确模板及初始可见数量。
func (f *Fixture) validateOrdinaryMonsterRoster(m *MapTemplate) error {
	if !f.OrdinaryMonsterRosterEnabled() {
		if len(m.MonsterSpawnIDs) != 0 || m.InitialMonsterCount != 0 {
			return errors.New("分地图普通怪物目录须使用规则30及后续版本")
		}
		return nil
	}
	if len(m.MonsterSpawns) != OrdinaryMonsterSlots || len(m.MonsterSpawnIDs) != len(m.MonsterSpawns) ||
		m.InitialMonsterCount < 1 || m.InitialMonsterCount > len(m.MonsterSpawns) {
		return errors.New("普通地图须为八个可选槽定义出生格、对应怪物和有效初始数量")
	}
	for i, id := range m.MonsterSpawnIDs {
		if _, ok := f.MonsterByID(id); !ok || id == 0 {
			return fmt.Errorf("槽位%d的普通怪物模板%d不存在", i, id)
		}
	}
	return nil
}
