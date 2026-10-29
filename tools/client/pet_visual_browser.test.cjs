// 工具边界测试不启动浏览器、不写库，防止把main或旧资源当成隔离新生验收。
const { test } = require('node:test');
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const path = require('node:path');
const fs = require('node:fs');
const { packet, validateQAIdentity, validateQAArtifacts, validateStaticIdentity, projectInputSnapshot, parseQualification, initialSummonCooldown, hostingAutoDisabled, disableAutoForManual, observerDiagnostics, failureDiagnostics, EDITOR_CACHES, QA_DATABASE_PREFIX } = require('./verify_pet_visual_browser.cjs');
const root = path.resolve(__dirname, '../..');
const evidencePath = path.join(root, '.work', 'pet-visual-unit', 'runtime.json');
const run = 'a'.repeat(32);
const evidence = () => ({ project_root: root, purpose: 'isolated_pet_visual_gui', run_id: run, database: QA_DATABASE_PREFIX + run, rule_version: 18, project: path.join(path.dirname(evidencePath), 'client-project'), creator_report: path.join(path.dirname(evidencePath), 'creator-report.json'), artifact_directory: path.dirname(evidencePath), artifact_reused: false, artifact_proof: null, static_port: 18082, static_pid: 1234, static_runtime: path.join(path.dirname(evidencePath), 'static-runtime.json'), ports: { account: 18110, game: 18111, gm: 18112 }, health_instances: { account: 'a', game: 'g', gm: 'm' } });

test('临时库名符合Mongo 63字节限制且与Root资格helper前缀一致', () => {
    const python = fs.readFileSync(path.join(root, 'tools/client/prepare_pet_visual_qa.py'), 'utf8');
    const match = python.match(/^DB_PREFIX\s*=\s*['"]([^'"]+)['"]/m);
    assert(match, 'Python helper须有显式前缀');
    assert.equal(QA_DATABASE_PREFIX, match[1]);
    const good = evidence(); assert.equal(Buffer.byteLength(good.database, 'utf8'), 62); validateQAIdentity(good, root, evidencePath);
    const tooLong = { ...good, database: 'minimir_dev_game_test_pet_visual_' + run };
    assert.equal(Buffer.byteLength(tooLong.database, 'utf8'), 65);
    assert.throws(() => validateQAIdentity(tooLong, root, evidencePath), /63字节/);
    assert.throws(() => validateQAIdentity({ ...good, database: good.database + 'x' }, root, evidencePath));
});

test('隔离身份接受本轮三个新端口和配对工程，拒main端口/旧rule/其它工程', () => {
    validateQAIdentity(evidence(), root, evidencePath);
    for (const change of [e => { e.ports.account = 18100; }, e => { e.rule_version = 17; }, e => { e.project = path.join(root, 'client'); }, e => { e.database += '_other'; }, e => { delete e.health_instances.game; }, e => { e.static_port = 18080; }, e => { e.static_port = 18081; }, e => { e.static_pid = 0; }, e => { e.static_runtime = path.join(root, '.work', 'other', 'static-runtime.json'); }, e => { delete e.artifact_reused; }, e => { e.artifact_reused = 'false'; }, e => { e.artifact_directory = path.join(root, '.work', 'old-run'); }]) {
        const e = evidence(); change(e); assert.throws(() => validateQAIdentity(e, root, evidencePath));
    }
    assert.throws(() => validateQAIdentity(evidence(), root, path.join(root, '.work-unowned', 'runtime.json')));
});

function artifactFixture(t, reused) {
    // 所有真实文件只写唯一.work沙盒，不在主client或来源工程制造假输入。
    const sandbox = fs.mkdtempSync(path.join(root, '.work', 'pet-visual-artifact-unit-'));
    t.after(() => { assert(sandbox.startsWith(path.join(root, '.work') + path.sep)); fs.rmSync(sandbox, { recursive: true, force: true }); });
    const fakeRoot = sandbox, directory = path.join(fakeRoot, '.work', 'current'), artifact = reused ? path.join(fakeRoot, '.work', 'original') : directory;
    const project = path.join(artifact, 'client-project'), runtime = path.join(directory, 'runtime.json');
    const write = (file, value) => { fs.mkdirSync(path.dirname(file), { recursive: true }); fs.writeFileSync(file, typeof value === 'string' ? value : JSON.stringify(value)); };
    const digest = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
    const source = 'assets/scripts/actor.ts', publicFile = 'assets/resources/development/gameplay.json';
    for (const base of [path.join(fakeRoot, 'client'), project]) {
        for (const name of ['assets', 'settings', 'profiles', '.creator', 'node_modules']) fs.mkdirSync(path.join(base, name), { recursive: true });
        for (const name of ['package.json', 'package-lock.json', 'tsconfig.json']) write(path.join(base, name), {});
    }
    write(path.join(fakeRoot, 'client', source), '当前Actor实现');
    write(path.join(fakeRoot, 'client', publicFile), { rule_version: 18, skills: [] });
    write(path.join(project, source), '当前Actor实现');
    write(path.join(project, publicFile), { rule_version: 18, skills: [{ id: 17 }] });
    write(path.join(project, 'build/web-mobile/index.html'), '<html>隔离正常Creator实构</html>');
    write(path.join(fakeRoot, 'client', EDITOR_CACHES[0]), { information: '主编辑器可再生缓存' });
    write(path.join(project, EDITOR_CACHES[0]), { information: '本轮Creator重新生成缓存' });
    const sourceHashes = projectInputSnapshot(path.join(fakeRoot, 'client'));
    const inputHashes = projectInputSnapshot(project);
    const buildHashes = { 'index.html': digest(path.join(project, 'build/web-mobile/index.html')) };
    const publicHash = digest(path.join(project, publicFile));
    const creator = { creator: '3.8.8', platform: 'web-mobile', passed: true, source_stable: true, rule_version: 18, exit_code: 36,
        project, log: path.join(artifact, 'creator.log'), public_sha256: publicHash, input_sha256: inputHashes, build_sha256: buildHashes,
        excluded_generated_editor_caches: Array.from(EDITOR_CACHES), generated_editor_cache_sha256: { [EDITOR_CACHES[0]]: digest(path.join(project, EDITOR_CACHES[0])) } };
    const copied = { project, source_sha256: sourceHashes, project_sha256: inputHashes, public_sha256: publicHash, only_public_changed: true,
        scene_prefab_preserved: true, normal_creator_project: true, runtime_resource_override: false, excluded_generated_editor_caches: Array.from(EDITOR_CACHES) };
    const e = { ...evidence(), project_root: fakeRoot, project, creator_report: path.join(directory, 'creator-report.json'),
        artifact_directory: artifact, artifact_reused: reused, artifact_proof: reused ? path.join(directory, 'reused-artifact.json') : null,
        fixture_sha256: 'f'.repeat(64), static_runtime: path.join(directory, 'static-runtime.json') };
    write(e.creator_report, creator); write(path.join(directory, 'project-copy.json'), copied); write(runtime, e);
    const proof = { passed: true, current_source_matches: true, isolated_inputs_match: true, build_outputs_match: true,
        run_id: e.run_id, artifact_directory: artifact, project, creator_report_sha256: digest(e.creator_report),
        project_copy_sha256: digest(path.join(directory, 'project-copy.json')), public_sha256: publicHash,
        fixture_sha256: e.fixture_sha256, source_files: Object.keys(sourceHashes).length, build_files: 1,
        runtime_resource_override: false, creator_rerun: false };
    if (reused) write(e.artifact_proof, proof);
    return { fakeRoot, directory, project, runtime, evidence: e, creator, copied, proof, write, source, publicFile };
}

test('原始与复用工程都需真实输入/完整产物摘要，复用精确绑定新run凭据与旧project', t => {
    for (const reused of [false, true]) {
        const f = artifactFixture(t, reused), result = validateQAArtifacts(f.evidence, f.fakeRoot, f.runtime);
        assert.equal(result.project, f.project); assert.equal(!!result.proof, reused);
        const changed = { ...f.evidence, project: path.join(f.fakeRoot, '.work', 'other', 'client-project') };
        assert.throws(() => validateQAArtifacts(changed, f.fakeRoot, f.runtime));
        assert.throws(() => validateQAIdentity(f.evidence, f.fakeRoot, path.join(f.directory, 'some-runtime.json')));
        if (reused) {
            assert.throws(() => validateQAIdentity({ ...f.evidence, creator_report: path.join(f.evidence.artifact_directory, 'creator-report.json') }, f.fakeRoot, f.runtime));
            assert.throws(() => validateQAIdentity({ ...f.evidence, artifact_proof: path.join(f.evidence.artifact_directory, 'reused-artifact.json') }, f.fakeRoot, f.runtime));
        }
    }
});

test('复用证明四个严格true/原报告SHA/当前fixture-public/run缺一不接受', t => {
    const f = artifactFixture(t, true);
    const changes = [
        ...['passed', 'current_source_matches', 'isolated_inputs_match', 'build_outputs_match'].flatMap(key => [q => { q[key] = false; }, q => { q[key] = 'true'; }, q => { delete q[key]; }]),
        ...['creator_report_sha256', 'project_copy_sha256', 'public_sha256', 'fixture_sha256'].map(key => q => { q[key] = '0'.repeat(64); }),
        q => { q.run_id = 'b'.repeat(32); }, q => { q.project = path.join(f.fakeRoot, 'client'); },
        q => { q.artifact_directory = f.directory; }, q => { q.runtime_resource_override = true; }, q => { q.creator_rerun = true; },
        q => { q.source_files++; }, q => { q.build_files++; },
    ];
    for (const change of changes) { const q = structuredClone(f.proof); change(q); f.write(f.evidence.artifact_proof, q); assert.throws(() => validateQAArtifacts(f.evidence, f.fakeRoot, f.runtime)); }
    f.write(f.evidence.artifact_proof, f.proof); validateQAArtifacts(f.evidence, f.fakeRoot, f.runtime);
});

test('复用不只信四个true：当前主源码、隔离输入、已编译产物任何字节漂移均拒绝', t => {
    for (const target of ['source', 'isolated', 'build', 'public']) {
        const f = artifactFixture(t, true);
        const file = target === 'source' ? path.join(f.fakeRoot, 'client', f.source) : target === 'isolated' ? path.join(f.project, f.source)
            : target === 'build' ? path.join(f.project, 'build/web-mobile/index.html') : path.join(f.project, f.publicFile);
        f.write(file, '漂移字节'); assert.throws(() => validateQAArtifacts(f.evidence, f.fakeRoot, f.runtime));
    }
});

test('Editor缓存排除必须是同一精确单路径，拒缺失、整个settings及扩大清单', t => {
    assert.deepEqual(EDITOR_CACHES, ['settings/v2/packages/information.json']);
    for (const value of [undefined, [], ['settings'], ['settings/**'], ['settings/v2/packages/information.json', 'assets'], ['settings/v2/packages/information.json', 'settings/v2/packages/information.json']]) {
        for (const reportName of ['creator', 'copied']) {
            const f = artifactFixture(t, false), changed = structuredClone(f[reportName]);
            if (value === undefined) delete changed.excluded_generated_editor_caches; else changed.excluded_generated_editor_caches = value;
            f.write(reportName === 'creator' ? f.evidence.creator_report : path.join(f.directory, 'project-copy.json'), changed);
            assert.throws(() => validateQAArtifacts(f.evidence, f.fakeRoot, f.runtime));
        }
    }
    for (const value of [undefined, { 'settings/other.json': 'a'.repeat(64) }, { [EDITOR_CACHES[0]]: 'wrong' }]) {
        const f = artifactFixture(t, false), creator = structuredClone(f.creator);
        if (value === undefined) delete creator.generated_editor_cache_sha256; else creator.generated_editor_cache_sha256 = value;
        f.write(f.evidence.creator_report, creator); assert.throws(() => validateQAArtifacts(f.evidence, f.fakeRoot, f.runtime));
    }
});

test('仅Editor缓存存在/再生值可变，其他当前source/isolated新增遗漏及任意build新文件仍拒绝', t => {
    const f = artifactFixture(t, false);
    f.write(path.join(f.fakeRoot, 'client', EDITOR_CACHES[0]), { information: '新查询结果' });
    f.write(path.join(f.project, EDITOR_CACHES[0]), { information: '另一次Editor缓存刷新' });
    validateQAArtifacts(f.evidence, f.fakeRoot, f.runtime);
    fs.unlinkSync(path.join(f.fakeRoot, 'client', EDITOR_CACHES[0])); fs.unlinkSync(path.join(f.project, EDITOR_CACHES[0]));
    validateQAArtifacts(f.evidence, f.fakeRoot, f.runtime);
    for (const file of [path.join(f.fakeRoot, 'client/settings/new-runtime-setting.json'), path.join(f.project, 'settings/v2/packages/other.json'),
        path.join(f.project, 'assets/new-resource.meta'), path.join(f.project, 'build/web-mobile', EDITOR_CACHES[0])]) {
        f.write(file, '未记录的新文件'); assert.throws(() => validateQAArtifacts(f.evidence, f.fakeRoot, f.runtime)); fs.unlinkSync(file);
    }
    validateQAArtifacts(f.evidence, f.fakeRoot, f.runtime);
});

test('摘要路径不能通过绝对路径或父级逃逸指向其它工程', t => {
    const f = artifactFixture(t, false);
    for (const key of ['../outside.txt', path.join(f.fakeRoot, 'client', f.source)]) {
        const creator = structuredClone(f.creator), copied = structuredClone(f.copied);
        creator.input_sha256[key] = 'a'.repeat(64); copied.project_sha256 = creator.input_sha256;
        f.write(f.evidence.creator_report, creator); f.write(path.join(f.directory, 'project-copy.json'), copied);
        assert.throws(() => validateQAArtifacts(f.evidence, f.fakeRoot, f.runtime));
    }
});

test('静态健康须精确配对新run PID/18082/QA代理与原artifact build-root', () => {
    const e = evidence(), meta = { project_root: root, run_id: run, pid: e.static_pid,
        argv: ['node.exe', path.join(root, 'tools/client/serve.mjs'), '--instance-id', run, '--build-root', path.join(e.project, 'build/web-mobile'), '--port', '18082', '--account-port', '18110', '--game-port', '18111'] };
    const health = { pid: e.static_pid, run_id: run, port: 18082, account_port: 18110, game_port: 18111, build_root: path.join(e.project, 'build/web-mobile'), webMobileBuild: true, developmentOnly: true };
    validateStaticIdentity(e, meta, health);
    for (const changes of [{ pid: e.static_pid + 1 }, { run_id: 'b'.repeat(32) }, { port: 18080 }, { port: 18081 }, { account_port: 18100 }, { game_port: 18101 }, { build_root: path.join(root, 'client/build/web-mobile') }]) assert.throws(() => validateStaticIdentity(e, meta, { ...health, ...changes }));
    const wrongMeta = structuredClone(meta); wrongMeta.argv[wrongMeta.argv.indexOf('--account-port') + 1] = '18100';
    assert.throws(() => validateStaticIdentity(e, wrongMeta, health));
    assert.throws(() => validateStaticIdentity(e, { ...meta, pid: e.static_pid + 1 }, health));
});

test('只读协议解析保留uint64 binding且拒截断/溢出', () => {
    const bytes = Buffer.from([1, 0, 0x75, 0x3f, 7, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x10, 0xaa]);
    const q = packet(bytes); assert.equal(q.id, 30015); assert.equal(q.request, 7); assert.equal(q.binding, '9007199254740992'); assert.deepEqual(q.body, Buffer.from([0xaa]));
    assert.equal(packet(Buffer.alloc(5)), null);
    assert.throws(() => packet(Buffer.from([1, 0, 0, 5, 0x80, 0x80])));
    assert.throws(() => packet(Buffer.from([1, 0, 0, 5, ...Array(9).fill(0xff), 2, 0])));
});

test('资格响应必须匹配正常创角回包ID且声明未预置技能/身体/绑定', () => {
    const role = 'c'.repeat(32), q = { passed: true, only_before_owner: true, book_from_normal_create: true, role_id_sha256: crypto.createHash('sha256').update(role).digest('hex'), explicit_count: 2, level: 19, skills_seeded: false, binding_seeded: false, position_seeded: false, body_seeded: false, pack_size_or_price_inferred: false };
    assert.deepEqual(parseQualification(JSON.stringify(q), role), q);
    for (const changed of [{ ...q, body_seeded: true }, { ...q, explicit_count: 1000 }, { ...q, role_id_sha256: '0'.repeat(64) }, { ...q, only_before_owner: false }]) assert.throws(() => parseQualification(JSON.stringify(changed), role));
    const missing = { ...q }; delete missing.skills_seeded; assert.throws(() => parseQualification(JSON.stringify(missing), role));
});

test('正常初学Recall等待读取实际catalog时序，不要求不存在的cooldown_ms', () => {
    const catalog = JSON.parse(fs.readFileSync(path.join(root, 'config/development/summon-candidates.json'), 'utf8'));
    const skill = catalog.skills.find(s => s.id === 17);
    assert.equal(skill.cooldown_ms, undefined);
    assert.equal(initialSummonCooldown(skill), Math.max(skill.cooldown_base_ms, skill.global_spell_ms, skill.action_ms));
    for (const value of [{ id: 17, cooldown_ms: 1800 }, { ...skill, cooldown_base_ms: null }, { ...skill, global_spell_ms: false }]) assert.throws(() => initialSummonCooldown(value));
});

test('正常触屏关闭Auto后等待真实Hosting false；缺失/true/Promise均不能当Boolean完成', async () => {
    for (const value of [undefined, {}, { hosting: {} }, { hosting: { autoBattleEnabled: true } }, { hosting: { autoBattleEnabled: 'false' } }]) {
        assert.equal(hostingAutoDisabled(value), false); assert.equal(typeof hostingAutoDisabled(value), 'boolean');
    }
    assert.equal(hostingAutoDisabled({ hosting: { autoBattleEnabled: false } }), true);
    const observed = { hosting: { autoBattleEnabled: true } }, taps = [];
    let poll, finished = false;
    const page = { async evaluate() { return true; }, waitForFunction(predicate, state, options) {
        assert.equal(state, observed); assert.equal(options.timeout, 15000);
        return new Promise(resolve => { poll = () => { const value = predicate(state); assert.equal(typeof value, 'boolean'); if (value === true) resolve(); }; poll(); });
    } };
    const waiting = disableAutoForManual(page, observed, async suffix => { taps.push(suffix); }).then(value => { finished = true; return value; });
    await new Promise(resolve => setImmediate(resolve));
    assert.deepEqual(taps, ['MapViewport/Actions/Auto']); assert.equal(finished, false);
    observed.hosting.autoBattleEnabled = false; poll();
    assert.deepEqual(await waiting, { touched: true, authoritativeDisabled: true });
    assert.equal(finished, true);
    const untouched = await disableAutoForManual({ async evaluate() { return false; }, async waitForFunction(predicate, value) { assert.equal(predicate(value), true); } }, observed, async () => assert.fail('原已关闭不得反向打开Auto'));
    assert.equal(untouched.touched, false);
});

test('失败诊断保留部分帧/声道/错误且白名单排除PNG与未知原payload', async () => {
    const observer = { phase: 'fresh', samples: [{ at: 1, handle: 3, life: '9007199254740993', frame: 4, frames: ['Mon4:2864'], operation: { token: 'secret' }, png: 'data:image/png;base64,secret' }],
        audio: [{ at: 2, phase: 'fresh', gestureUnlocked: true, voices: [{ cue: 'pet_skeleton_born', identity: 'birth:3:1', playing: true, clipBound: true, rawPayload: 'secret' }], accountToken: 'secret' }],
        captures: { 0: { png: 'data:image/png;base64,secret' }, 4: { png: 'data:image/png;base64,secret' }, 9: { png: 'data:image/png;base64,secret' } }, error: 'frame failed', handler: 'secret', ticket: 'secret' };
    const diagnostics = observerDiagnostics(observer);
    assert.deepEqual(diagnostics.captureFrameIndices, [0, 4, 9]); assert.equal(diagnostics.samples[0].frame, 4); assert.equal(diagnostics.audio[0].voices[0].playing, true);
    assert.equal(diagnostics.error, 'frame failed'); assert(!JSON.stringify(diagnostics).includes('secret')); assert(!JSON.stringify(diagnostics).includes('data:image'));
    for (const scene of ['Game', 'RoleSelect']) {
        const result = await failureDiagnostics({ async evaluate() { return scene; } }, async () => ({ player: { hp: '20' }, hosting: { autoBattleEnabled: false } }), async () => observer);
        assert.equal(result.failureScene, scene); assert.equal(result.failureState.player.hp, '20'); assert.deepEqual(result.failureObservation.captureFrameIndices, [0, 4, 9]);
    }
    const fallback = await failureDiagnostics({ async evaluate() { return 'Game'; } }, async () => { throw Error('state unavailable'); }, async () => observer);
    assert.equal(fallback.failureStateReadError, 'state unavailable'); assert.equal(fallback.failureObservation.samples.length, 1);
    const login = await failureDiagnostics({ async evaluate() { return 'Login'; } }, async () => assert.fail('不得读取登录state'), async () => assert.fail('不得读取登录observer'));
    assert.equal(login.failureScene, 'Login'); assert(!('failureState' in login)); assert(!('failureObservation' in login));
});
