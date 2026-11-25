const assert = require('node:assert/strict');
const fs = require('node:fs');
const { test } = require('node:test');
const { load } = require('./helpers/session_harness.cjs');

const scene = JSON.parse(fs.readFileSync('client/assets/scenes/Game.scene', 'utf8'));
const atlas = JSON.parse(fs.readFileSync('client/assets/resources/ui180/sprite-frames.json', 'utf8')).assets;
const child = (node, name) => node._children.map(ref => scene[ref.__id__]).find(value => value._name === name);
const component = (node, name) => node._components.map(ref => scene[ref.__id__]).find(value => value.__type__ === name);
const safe = ['Canvas', 'Safe', 'UIMgr', 'Panels'].reduce(child, scene[1]);
const role = child(safe, 'Role'), bag = child(safe, 'Inventory');
const board = child(role, 'EquipmentBoard');
const slotIds = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 100];
const boundsY = node => {
    const h = component(node, 'cc.UITransform')._contentSize.height;
    return [node._lpos.y - h / 2, node._lpos.y + h / 2];
};

test('角色保存实际装备槽、肖像和紧凑穿戴列表，背包仍有容量及不遮挡网格的可用导航', () => {
    for (const [panel, own] of [[role, 'Role'], [bag, 'Inventory']]) {
        const nav = child(panel, 'SectionTabs');
        for (const name of ['Role', 'Inventory', 'Skills']) {
            const tab = child(nav, name);
            assert(component(child(tab, 'Label'), 'cc.Label'));
            assert.equal(!!component(tab, 'cc.Button'), name !== own);
        }
        assert(boundsY(nav)[0] > boundsY(child(panel, 'List'))[1]);
    }
    const roleList = child(role, 'List'), mask = child(roleList, 'View');
    assert(component(mask, 'cc.Mask'));
    assert(boundsY(board)[0] > boundsY(roleList)[1]);
    assert(boundsY(roleList)[0] > boundsY(child(role, 'Equip'))[1]);
    assert(component(child(bag, 'Capacity'), 'cc.Label')._fontSize >= 30);
    assert.deepEqual(board._children.map(ref => scene[ref.__id__]).filter(node => /^Slot/.test(node._name))
        .map(node => [Number(node._name.slice(4)), !!component(node, 'cc.Button')]), slotIds.map(id => [id, true]));
    const portraits = child(board, 'Portrait');
    for (const [job, name] of ['warrior', 'wizard', 'taoist'].entries()) for (const [gender, sex] of ['male', 'female'].entries()) {
        assert.equal(component(child(portraits, `Job${job}Gender${gender}`), 'cc.Sprite')._spriteFrame.__uuid__,
            atlas[`portrait-${name}-${sex}`].sprite_frame_uuid);
    }
});

test('角色预览读真实当前肖像和装备部位，空槽及未开放施法槽不能操作', () => {
    class Button {}
    class Label {}
    const slots = new Map(slotIds.map(id => [id, { label: {}, button: {}, active: true }]));
    const characters = ['Job0Gender0', 'Job1Gender0', 'Job1Gender1', 'Job2Gender1'].map(name => ({ name, active: false }));
    const caption = {};
    const root = { activeInHierarchy: true, getChildByName(name) {
        if (name === 'Portrait') return { children: characters };
        if (name === 'CharacterName') return { getComponent: () => caption };
        const slot = slots.get(Number(name.slice(4)));
        return Object.assign(slot, { getComponent: () => slot.button, getChildByName: () => ({ getComponent: () => slot.label }) });
    } };
    const { GameRoleEquipment } = load('client/assets/scripts/app/GameRoleEquipment.ts', id => id === 'cc' ? { Button, Label }
        : require('../../../.work/ui-tests/app/EquipmentModel.js'));
    const view = new GameRoleEquipment({ getChildByName: () => root });
    const wearing = [{ slot: 0, item: { itemId: 210 } }, { slot: 100, item: { itemId: 187 } }];
    const definitions = [{ name: '木剑', item_id: 210 }, { name: '护身符', item_id: 187 }];
    view.refresh({ job: 1, gender: 0, name: '原来角色' }, wearing, definitions, undefined, 0, true);
    assert.equal(caption.string, '原来角色');
    assert.deepEqual(characters.filter(node => node.active).map(node => node.name), ['Job1Gender0']);
    assert.equal(slots.get(0).label.string, '● 木剑');
    assert.equal(slots.get(0).button.interactable, true);
    assert.equal(slots.get(2).label.string, '头盔');
    assert.equal(slots.get(2).button.interactable, false);
    assert.equal(slots.get(100).active, false);
    view.refresh({ job: 2, gender: 1, name: '道士' }, wearing, definitions, 100, 100, false);
    assert.deepEqual(characters.filter(node => node.active).map(node => node.name), ['Job2Gender1']);
    assert.equal(slots.get(100).active, true);
    assert.equal(slots.get(100).label.string, '● 护身符');
    assert.equal(slots.get(100).button.interactable, false);
});
