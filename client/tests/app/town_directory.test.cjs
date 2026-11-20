const assert = require('node:assert/strict');
const test = require('node:test');
const { load, Long } = require('./helpers/session_harness.cjs');
const { MapTravelModel, travelInteger } = require('../../../.work/ui-tests/app/MapTravelModel.js');
const { townDirectoryRegions, compactTownMap } = load('client/assets/scripts/app/TownMapModel.ts', () => ({ travelInteger }));
const floor = require('../../assets/resources/development/maps/first-map.json');

const directory = {
    town_travel: { destination_map_ids: ['3'] },
    map_catalog: [
        { id: '1', name: '林间战场', scene_kind: 1 }, { id: '3', name: '银杏山谷', scene_kind: 2, compact_town: true },
        { id: '6', name: '矿道', scene_kind: 1 },
    ],
    map_regions: [{ id: '0', name: '比奇省', map_ids: ['1', '3'] }, { id: '1', name: '矿区', map_ids: ['6'] }],
};

test('地图区域允许纯野外，右栏顺序、当前图与64位目标不被替换', () => {
    const mapId = '9007199254740993', value = structuredClone(directory);
    value.map_catalog[2].id = mapId; value.map_regions[1].map_ids = [mapId];
    const regions = townDirectoryRegions(value, mapId);
    assert.deepEqual(regions.map(region => region.maps.map(row => [row.mapId, row.destination, row.current])), [[['1', 2, false], ['3', 1, false]], [[mapId, 2, true]]]);
    value.map_regions[0].map_ids.push(mapId);
    assert.throws(() => townDirectoryRegions(value, '3'), /目录目标无效/);
});

test('保存地图面板两列分别消费区域和地图按钮，默认当前区域且当前图禁跳', () => {
    class Button {}
    class Label {}
    function list() {
        const events = new Map(), node = {
            on(id, fn, self) { events.set(id, [fn, self]); }, off(id) { events.delete(id); },
        };
        return {
            node, setItems(rows, render) { this.rows = rows; this.render = render; }, setRenderer(render) { this.render = render; },
            tap(index) { const [fn, self] = events.get('list-item-click') ?? []; fn?.call(self, index, this.rows[index]); },
        };
    }
    const regions = list(), maps = list(), panel = {
        activeInHierarchy: true, getChildByName(name) { return { getComponent: () => name === 'RegionList' ? regions : maps }; },
    };
    const clicks = [], List = { EVENT_ITEM_CLICK: 'list-item-click' };
    const { TownDirectory } = load('client/assets/scripts/app/TownDirectory.ts', id => id === 'cc' ? { Button, Label, isValid: Boolean } : { List });
    const view = new TownDirectory(panel, row => clicks.push(row));
    view.show(townDirectoryRegions(directory, '6'));
    assert.deepEqual(maps.rows.map(row => row.mapId), ['6']); maps.tap(0); assert.deepEqual(clicks, []);
    regions.tap(0); assert.deepEqual(maps.rows.map(row => row.mapId), ['1', '3']);
    maps.tap(1); assert.equal(clicks[0].mapId, '3');
    panel.activeInHierarchy = false; maps.tap(0); regions.tap(1); assert.equal(clicks.length, 1);
    view.dispose();
});

test('紧凑城镇覆盖野外地图身份和走格，保留地板几何与独立NPC定义', () => {
    const npc = { id: '3001', name: '书店老板', role: '书店', position: { x: 4, y: 4 }, profile: 'player_male' };
    const entry = { id: '3', name: '银杏山谷', scene_kind: 2, width: 17, height: 17, blocked: [npc.position],
        safe_zones: [], player_spawn: { x: 8, y: 13 }, npcs: [npc] };
    const map = compactTownMap(floor, entry);
    assert.deepEqual([map.id, map.name, map.scene_kind, map.width, map.height, map.projection], ['3', '银杏山谷', 2, 17, 17, 'isometric_diamond']);
    assert.equal(map.surface_width, floor.surface_width); assert.deepEqual(map.props, []); assert.equal(map.npcs[0], npc);
    assert.throws(() => compactTownMap(floor, { ...entry, player_spawn: npc.position }), /出生格/);
    assert.throws(() => compactTownMap(floor, { ...entry, npcs: [npc, { ...npc, id: '3002' }] }), /NPC/);
});

test('旅行计划只接受原列表选择的地图，包括返回野外', () => {
    const context = { connection: 1, binding: '1', role: 'role', scene: '4', baseline: '9', life: '0', rule: '24' }, id = '08'.repeat(16);
    const travel = new MapTravelModel();
    travel.begin(context, id, 2, Long.UONE);
    const plan = { travelId: id, targetMapId: '17', fromSceneEpoch: context.scene, targetSceneKind: 1, position: { x: 8, y: 8 }, expiresAtUnixMs: '500' };
    assert.throws(() => travel.acceptPlan(plan, context, 1), /原请求身份/);
    travel.acceptPlan({ ...plan, targetMapId: Long.UONE }, context, 1);
    assert.equal(travel.plan.targetMapId, '1');
});
