const { test } = require('node:test');
const assert = require('node:assert/strict');
const { makeWorld, state, Long, cc } = require('../support/world_runtime.cjs');

function head(visual) {
    const name = visual.node.getChildByName('Name');
    const hp = visual.node.getChildByName('HP');
    return { name: name.active, text: name.getComponent(cc.Label).string, hp: hp.active, progress: hp.getComponent(cc.ProgressBar).progress };
}

test('活怪在死亡基线隐藏名字和血条，尸体动画继续播放，同句柄新生命恢复头顶显示', () => {
    const world = makeWorld(1), monster = state(2, 3, 9, 8, { hp: Long.fromInt(67) });
    try {
        world.apply([monster]);
        const visual = world.view.visuals.get(2), standing = head(visual);
        assert.equal(standing.name, true);
        assert.ok(standing.text);
        assert.equal(standing.hp, true);
        assert.equal(standing.progress, 0.67);

        world.apply([{ ...monster, hp: Long.UZERO }], 100);
        assert.equal(world.view.visuals.get(2), visual);
        assert.equal(visual.node.active, true);
        assert.deepEqual(head(visual), { ...standing, name: false, hp: false, progress: 0 });
        assert.equal(visual.pose.action, 'die');
        const death = visual.profile.layers[0].actions.die;
        const corpseTime = 100 + death.count * death.duration_ms + 1;
        world.update(corpseTime);
        assert.equal(visual.pose.frame, death.count - 1);
        assert.equal(visual.body[0].node.active, true);

        const newLife = Long.fromString('9007199254740993', true);
        world.apply([{ ...monster, life: newLife, hp: monster.hpMax }], corpseTime + 1);
        assert.equal(world.view.visuals.get(2), visual);
        assert.equal(visual.state.life, newLife);
        assert.equal(visual.pose.action, 'standing');
        assert.deepEqual(head(visual), { ...standing, progress: 1 });
    } finally { world.view.dispose(); }
});

test('首次收到怪物尸体隐藏头顶，倒下玩家、宠物尸体与掉落沿用各自头顶显示', () => {
    const world = makeWorld(1);
    try {
        // 用当前已知宠物显示名核对尸体标识，确保怪物规则没有覆盖宠物。
        const pet = { ...world.assets.gameplay.pet_profiles[0], job_id: 0 };
        world.assets.gameplay.pet_profiles = [pet];
        world.apply([
            state(1, 1, 8, 8, { hp: Long.UZERO }),
            state(2, 3, 9, 8, { hp: Long.UZERO }),
            state(3, 2, 8, 9, { hp: Long.UZERO, isPetCorpse: true, definitionId: pet.definition_id }),
            state(4, 4, 9, 9, { hp: Long.UZERO }),
        ]);
        const [player, monster, petCorpse, item] = [1, 2, 3, 4].map(handle => head(world.view.visuals.get(handle)));
        assert.equal(player.name, true);
        assert.equal(player.hp, true);
        assert.equal(monster.name, false);
        assert.equal(monster.hp, false);
        assert.equal(world.view.visuals.get(2).pose.action, 'die');
        assert.deepEqual(petCorpse, { name: true, text: `${pet.name}·尸体`, hp: true, progress: 0 });
        assert.deepEqual(item, { name: true, text: '掉落', hp: false, progress: 0 });
    } finally { world.view.dispose(); }
});
