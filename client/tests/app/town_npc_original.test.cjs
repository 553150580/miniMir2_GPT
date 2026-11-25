const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const ts = require('../../../tools/client/creator_paths.cjs').typescript;

const file = path.resolve(__dirname, '../../assets/scripts/app/TownMapModel.ts');
const code = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
    compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS },
}).outputText;
const bindings = { exports: {} };
new Function('require', 'module', 'exports', code)(name => {
    assert.equal(name, './MapTravelModel');
    return {};
}, bindings, bindings.exports);
const { mergeTownNpcAppearances, townNpcAppearance, townNpcPreloadFrames } = bindings.exports;

const npc = { id: '4002', name: '边界守将', role: '守卫', position: { x: 3, y: 4 }, profile: 'player_male' };

function inputs() {
    const original = { schema_version: 1, npc_profiles: { '4002': 'npc_original_27' }, profiles: {}, frames: {} };
    const directions = [0, 1, 2].map(direction => ({ direction, frames: [0, 1, 2, 3].map(offset => {
        const index = 1680 + direction * 10 + offset;
        const key = `Npc:${index}`;
        original.frames[key] = {
            png: `town-npcs/frames/frame_${index}.png`, width: 40, height: 75, offset_x: 7, offset_y: -25, declared_empty: false,
        };
        return key;
    }) }));
    original.profiles.npc_original_27 = { id: 'npc_original_27', layers: [{ package: 'Npc', actions: {
        standing: { count: 4, duration_ms: 200, directions },
    } }] };
    const manifest = {
        profiles: { player_male: { layers: ['Hum', 'Hair', 'Weapon'].map(packageName => ({ package: packageName,
            actions: { standing: { directions: Array.from({ length: 8 }, () => ({ frames: [packageName, '未使用'] })) } },
        })) } },
        frames: { Hum: { declared_empty: false }, Hair: { declared_empty: false }, Weapon: { declared_empty: false } },
    };
    return { original, manifest };
}

test('原版守将采用单图层原始帧及站立时钟，NPC交互身份保持原值', () => {
    const { original, manifest } = inputs();
    const projected = mergeTownNpcAppearances(manifest, original, [npc]);
    const source = townNpcAppearance(projected, npc);
    assert.equal(source.profile.id, 'npc_original_27');
    assert.equal(source.parts[0].frameMs, 200);
    assert.deepEqual(source.parts[0].frames, ['Npc:1680', 'Npc:1681', 'Npc:1682', 'Npc:1683']);
    assert.deepEqual(townNpcPreloadFrames(projected, npc), source.parts[0].frames);
    assert.equal(projected.frames['Npc:1680'].offset_y, -25);
    assert.equal(npc.profile, 'player_male');
    assert.equal(npc.role, '守卫');
    assert.equal(manifest.profiles.npc_original_27, undefined);
});

test('旧版包只持有当前NPC需要的玩家静态身体和头发', () => {
    const { manifest } = inputs();
    assert.deepEqual(townNpcPreloadFrames(manifest, npc), ['Hum', 'Hair']);
    assert.deepEqual(townNpcAppearance(manifest, npc).parts.map(part => part.frameMs), [0, 0]);
});

test('拒绝缺失城镇身份、缺少原站姿方向帧和非法纹理路径', async t => {
    const failures = [
        ['身份', extra => delete extra.npc_profiles['4002'], /缺少原画/],
        ['帧', extra => delete extra.frames['Npc:1703'], /外观无效/],
        ['路径', extra => { extra.frames['Npc:1680'].png = '../evil'; }, /原画帧无效/],
    ];
    for (const [name, breakManifest, error] of failures) await t.test(name, () => {
        const { original, manifest } = inputs();
        breakManifest(original);
        assert.throws(() => mergeTownNpcAppearances(manifest, original, [npc]), error);
    });
});
