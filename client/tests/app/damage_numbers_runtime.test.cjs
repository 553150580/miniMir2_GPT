const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { makeWorld, state, Long, UITransform, Vec3, cc } = require('../support/world_runtime.cjs');

// 真实脚本使用节点坐标变换，夹具明确让实体容器和特效容器的原点不同。
Vec3.ZERO = new Vec3();
UITransform.prototype.convertToWorldSpaceAR = function (point) {
    const result = new Vec3(point.x, point.y, point.z);
    for (let node = this.node; node; node = node.parent) {
        result.x = result.x * node.scale.x + node.position.x;
        result.y = result.y * node.scale.y + node.position.y;
    }
    return result;
};
UITransform.prototype.convertToNodeSpaceAR = function (point) {
    const chain = [];
    for (let node = this.node; node; node = node.parent) chain.unshift(node);
    return chain.reduce((result, node) => new Vec3((result.x - node.position.x) / node.scale.x,
        (result.y - node.position.y) / node.scale.y, result.z), point);
};

function fixture() {
    const world = makeWorld(1, { damageNumbers: true });
    let current = true;
    world.view.bindPresentation({ connection: 1, binding: Long.UONE, role: '测试角色', scene: Long.UONE, baseline: Long.UONE, map: Long.UONE, travelId: '' }, () => current);
    const player = state(1), monster = state(2, 3, 9, 8), effects = world.view.world.getChildByName('Effects');
    world.entities.setPosition(5, -6);
    effects.setPosition(11, 7);
    world.apply([player, monster]);
    const eventId = Long.fromString('9007199254740993', true);
    const impact = (seq, changes = {}) => ({ sceneEpoch: Long.UONE, eventId: eventId.add(seq), skillId: 0,
        casterHandle: 1, targetHandle: 2, targetLife: Long.UZERO, targetGrid: { x: 9, y: 8 }, amount: Long.fromInt(7),
        outcome: 1, healing: false, castId: Long.UZERO, attackSequence: 0, ...changes });
    return { ...world, effects, player, monster, impact, damage: world.view.damageNumbers, retire: () => { current = false; } };
}

test('保存的伤害数字预制体根提供非交互文字、透明度与二维位置', () => {
    const source = JSON.parse(fs.readFileSync(path.join(__dirname, '../../assets/resources/prefabs/DamageNumber.prefab'), 'utf8'));
    const root = source[source[0].data.__id__];
    const components = root._components.map(ref => source[ref.__id__]);
    assert.equal(root._name, 'DamageNumber');
    assert.deepEqual(components.map(part => part.__type__).sort(), ['cc.Label', 'cc.UIOpacity', 'cc.UITransform'].sort());
    assert.ok(components.find(part => part.__type__ === 'cc.Label')._fontSize >= 24);
});

test('真实世界仅根据一次正命中显示精确Long，数字从目标名字上方升起并淡出，同生命尸体保留末击', () => {
    const f = fixture();
    try {
        const visual = f.view.visuals.get(2), name = visual.node.getChildByName('Name');
        const amount = Long.fromString('9007199254740995');
        const event = f.impact(0, { amount });
        assert.equal(f.damage.active.length, 0);
        f.view.acceptImpact(event); f.view.acceptImpact(event);
        assert.equal(f.damage.active.length, 1);
        const number = f.damage.active[0];
        assert.equal(number.node.parent, f.effects);
        assert.equal(number.label.string, amount.toString());
        assert.equal(number.opacity.opacity, 255);
        assert.equal(number.node.position.x, visual.node.position.x + name.position.x - 6);
        assert.equal(number.node.position.y, visual.node.position.y + name.position.y - 13 + 29);

        f.apply([f.player, { ...f.monster, hp: Long.UZERO }], 240);
        assert.equal(name.active, false);
        assert.equal(f.damage.active[0], number);
        assert.ok(number.node.position.y > number.origin.y);
        assert.ok(number.opacity.opacity < 255 && number.opacity.opacity > 0);
        f.update(720);
        assert.equal(f.damage.active.length, 0);
        assert.equal(number.node.active, false);
        assert.equal(number.node.parent, null);
        f.view.acceptImpact(event);
        assert.equal(f.damage.active.length, 0);
    } finally { f.view.dispose(); }
});

test('治疗、闪避、失效和零伤害均不生成数字，旧场景、错误Life与重生后的旧伤害也被丢弃', () => {
    const f = fixture();
    try {
        const rejected = [
            { sceneEpoch: Long.fromInt(2, true) }, { targetLife: Long.UONE }, { targetHandle: 88 },
            { healing: true }, { outcome: 2 }, { outcome: 3 }, { outcome: 4, healing: true },
            { amount: Long.ZERO }, { amount: Long.NEG_ONE },
        ];
        rejected.forEach((changes, index) => f.view.acceptImpact(f.impact(index, changes)));
        assert.equal(f.effects.children.filter(node => node.name === 'DamageNumber').length, 0);

        f.view.acceptImpact(f.impact(20));
        const first = f.damage.active[0];
        assert.ok(first);
        f.apply([f.player, { ...f.monster, life: Long.UONE }], 100);
        assert.equal(f.damage.active.length, 0);
        f.view.acceptImpact(f.impact(21));
        assert.equal(f.damage.active.length, 0);
        f.view.acceptImpact(f.impact(22, { targetLife: Long.UONE }));
        assert.equal(f.damage.active[0].node, first.node);
        f.retire();
        f.view.acceptImpact(f.impact(23, { targetLife: Long.UONE }));
        assert.equal(f.damage.active.length, 1);
    } finally { f.view.dispose(); }
});

test('多重命中保持独立事件，同屏最多十条并复用节点，世界资源退休销毁活动与空闲引用', () => {
    const f = fixture();
    try {
        for (let index = 0; index < 7; index++) f.view.acceptImpact(f.impact(index));
        assert.equal(f.damage.active.length, 5);
        assert.equal(new Set(f.damage.active.map(number => number.node.position.x)).size, 5);

        const monsters = Array.from({ length: 13 }, (_, index) => state(index + 2, 3, index + 2, 8));
        f.apply([f.player, ...monsters], 10);
        const nodes = new Set(f.damage.active.map(number => number.node));
        for (let index = 0; index < 13; index++) {
            f.view.acceptImpact(f.impact(10 + index, { targetHandle: index + 2, castId: Long.UONE }));
            f.damage.active.forEach(number => nodes.add(number.node));
        }
        assert.equal(f.damage.active.length, 10);
        assert.equal(f.effects.children.filter(node => node.name === 'DamageNumber').length, 10);
        assert.equal(nodes.size, 10);
        f.update(740);
        assert.equal(f.damage.active.length, 0);
        f.view.acceptImpact(f.impact(40, { targetHandle: 1, casterHandle: 2 }));
        assert.equal(f.damage.active.length, 1);
        assert.ok(nodes.has(f.damage.active[0].node));

        f.assets.dispose();
        assert.equal(f.damage.active.length, 0);
        assert.equal(f.damage.spare.length, 0);
        assert.ok([...nodes].every(node => node.destroyed && node.parent === null));
        assert.equal(f.effects.children.filter(node => node.name === 'DamageNumber').length, 0);
    } finally { f.view.dispose(); }
});
