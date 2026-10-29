// 独立rule18工程的正常触屏链；内部对象/协议/渲染事件只读观察，凭据仅在内存。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const ROOT = path.resolve(__dirname, '../..');
const QA_DATABASE_PREFIX = 'minimir_dev_game_test_pet_viz_';
const EDITOR_CACHES = Object.freeze(['settings/v2/packages/information.json']);
const PROJECT_INPUTS = Object.freeze(['assets', 'settings', 'profiles', '.creator', 'node_modules', 'package.json', 'package-lock.json', 'tsconfig.json']);
const MEMORY_SECRETS = [];
const redact = value => MEMORY_SECRETS.reduce((s, secret) => s.split(secret).join('[内存凭据已省略]'), String(value));
const hash = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const textHash = value => crypto.createHash('sha256').update(value).digest('hex');
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

function packet(payload) {
    const b = Buffer.from(payload);
    if (b.length < 6 || b.length > 65536) return null;
    let offset = 4;
    const variable = () => {
        let value = 0n;
        for (let i = 0; i < 10; i++) {
            assert(offset < b.length, '协议头截断');
            const byte = b[offset++];
            assert(i !== 9 || byte <= 1, '协议头uint64溢出');
            value |= BigInt(byte & 127) << BigInt(i * 7);
            if (!(byte & 128)) return value;
        }
        throw Error('协议头超长');
    };
    const request = variable(), binding = variable().toString(), error = b[1] & 1 ? Number(variable()) : 0;
    assert(request <= BigInt(Number.MAX_SAFE_INTEGER), 'request越过安全整数');
    return { id: b.readUInt16BE(2), request: Number(request), binding, error, body: b.subarray(offset) };
}

function validateQAIdentity(evidence, root, evidencePath) {
    const directory = workDirectory(root, path.dirname(evidencePath));
    assert.equal(path.resolve(evidencePath), path.join(directory, 'runtime.json'), '必须读取新run的精确runtime.json');
    assert.equal(evidence.project_root, root);
    assert.equal(evidence.purpose, 'isolated_pet_visual_gui');
    assert.match(evidence.run_id, /^[0-9a-f]{32}$/);
    assert.equal(typeof evidence.database, 'string');
    assert(Buffer.byteLength(evidence.database, 'utf8') <= 63, '临时库名超过Mongo 63字节限制');
    assert.equal(evidence.database, QA_DATABASE_PREFIX + evidence.run_id);
    assert.equal(String(evidence.rule_version), '18');
    const artifact = workDirectory(root, evidence.artifact_directory);
    assert.equal(typeof evidence.artifact_reused, 'boolean', '必须明确是否复用构建产物');
    if (evidence.artifact_reused) {
        assert.notEqual(artifact, directory, '复用产物必须引用另一个受控run');
        assert.equal(path.resolve(evidence.artifact_proof), path.join(directory, 'reused-artifact.json'));
    } else {
        assert.equal(artifact, directory, '未复用时只允许本轮工程');
        assert(evidence.artifact_proof == null, '本轮构建不得携带其它复用凭据');
    }
    assert.equal(path.resolve(evidence.project), path.join(artifact, 'client-project'));
    assert.equal(path.resolve(evidence.creator_report), path.join(directory, 'creator-report.json'));
    assert.equal(evidence.static_port, 18082, '仅使用本轮明确隔离静态端口');
    assert(Number.isSafeInteger(evidence.static_pid) && evidence.static_pid > 0, '必须登记本轮静态进程PID');
    assert.equal(path.resolve(evidence.static_runtime), path.join(directory, 'static-runtime.json'));
    assert.deepEqual(evidence.ports, { account: 18110, game: 18111, gm: 18112 });
    for (const mode of Object.keys(evidence.ports)) assert(evidence.health_instances?.[mode], '缺少节点instance');
}

function workDirectory(root, value) {
    assert.equal(typeof value, 'string', '缺少受控work路径');
    assert(value && path.isAbsolute(value), 'work路径必须明确为绝对路径');
    const directory = path.resolve(value), work = path.join(root, '.work');
    assert(directory.startsWith(work + path.sep), 'QA证据必须位于项目.work子目录');
    // 有真实文件时同时核物理路径，不能用junction/symlink把受控前缀转向仓库外。
    if (fs.existsSync(directory)) assert(fs.realpathSync(directory).startsWith(fs.realpathSync(work) + path.sep), 'work物理路径越界');
    return directory;
}

function containedFile(directory, relative) {
    assert.equal(typeof relative, 'string');
    assert(relative && !path.isAbsolute(relative), '摘要键必须是工程相对路径');
    const file = path.resolve(directory, relative);
    assert(file.startsWith(directory + path.sep), '摘要键越过工程边界');
    assert(fs.realpathSync(file).startsWith(fs.realpathSync(directory) + path.sep), '摘要文件物理路径越界');
    return file;
}

function directorySnapshot(directory, exclusions = []) {
    const result = {};
    const visit = relative => {
        if (exclusions.includes(relative)) return;
        const absolute = path.join(directory, relative), stat = fs.lstatSync(absolute);
        assert(!stat.isSymbolicLink(), '输入/产物不能通过链接逃逸：' + relative);
        if (stat.isDirectory()) {
            assert(fs.realpathSync(absolute) === absolute, '输入/产物目录不能通过junction逃逸：' + relative);
            for (const name of fs.readdirSync(absolute).sort()) visit(relative ? relative + '/' + name : name);
        } else if (stat.isFile()) result[relative] = hash(containedFile(directory, relative));
    };
    for (const name of fs.readdirSync(directory).sort()) visit(name);
    return result;
}

function projectInputSnapshot(project) {
    const result = {};
    for (const name of PROJECT_INPUTS) {
        const absolute = path.join(project, name), stat = fs.lstatSync(absolute);
        assert(!stat.isSymbolicLink() && fs.realpathSync(absolute) === absolute, '受控输入不允许链接：' + name);
        if (stat.isDirectory()) {
            const skipped = EDITOR_CACHES.filter(value => value.startsWith(name + '/')).map(value => value.slice(name.length + 1));
            for (const [relative, digest] of Object.entries(directorySnapshot(absolute, skipped))) result[name + '/' + relative] = digest;
        } else {
            assert(stat.isFile(), '受控输入缺少文件：' + name);
            result[name] = hash(absolute);
        }
    }
    return result;
}

function validateQAArtifacts(evidence, root, evidencePath) {
    validateQAIdentity(evidence, root, evidencePath);
    const directory = path.dirname(path.resolve(evidencePath)), artifact = path.resolve(evidence.artifact_directory);
    const project = path.resolve(evidence.project), copyPath = path.join(directory, 'project-copy.json');
    assert.equal(fs.realpathSync(project), path.join(fs.realpathSync(artifact), 'client-project'));
    const creatorPath = containedFile(directory, 'creator-report.json'), actualCopyPath = containedFile(directory, 'project-copy.json');
    const creator = JSON.parse(fs.readFileSync(creatorPath, 'utf8')), copied = JSON.parse(fs.readFileSync(actualCopyPath, 'utf8'));
    assert.deepEqual(copied.excluded_generated_editor_caches, EDITOR_CACHES, '复制只能排除精确Editor问卷缓存');
    assert.deepEqual(creator.excluded_generated_editor_caches, EDITOR_CACHES, '构建必须使用相同精确缓存边界');
    assert(creator.generated_editor_cache_sha256 && typeof creator.generated_editor_cache_sha256 === 'object' && !Array.isArray(creator.generated_editor_cache_sha256), '缺少独立Editor缓存原字节摘要');
    for (const [relative, digest] of Object.entries(creator.generated_editor_cache_sha256)) {
        assert(EDITOR_CACHES.includes(relative), '不能新增其它生成缓存例外');
        assert.match(digest, /^[0-9a-f]{64}$/, 'Editor缓存摘要必须是原字节SHA256');
    }
    assert(creator.passed === true && creator.source_stable === true && creator.rule_version === 18 && path.resolve(creator.project) === project, '必须是配对隔离工程的真实Creator实构');
    assert.equal(creator.creator, '3.8.8'); assert.equal(creator.platform, 'web-mobile'); assert.equal(creator.exit_code, 36);
    assert.equal(path.resolve(creator.log), path.join(artifact, 'creator.log'), 'Creator日志仍须配对原始构建run');
    assert(copied.only_public_changed === true && copied.scene_prefab_preserved === true && copied.normal_creator_project === true && copied.runtime_resource_override === false);
    assert.equal(path.resolve(copied.project), project);
    assert.equal(copied.public_sha256, creator.public_sha256);
    assert(copied.source_sha256 && Object.keys(copied.source_sha256).length > 0, '缺少当前主工程复制来源摘要');
    assert.deepEqual(projectInputSnapshot(path.join(root, 'client')), copied.source_sha256, '当前主工程完整输入与原构建来源不一致');
    assert.deepEqual(copied.project_sha256, creator.input_sha256, '复制投影与实构输入清单必须一致');
    assert(creator.input_sha256 && Object.keys(creator.input_sha256).length > 0, '缺少实构输入摘要');
    assert.deepEqual(projectInputSnapshot(project), creator.input_sha256, '隔离完整输入变化');
    assert(creator.build_sha256 && Object.keys(creator.build_sha256).length > 0, '缺少实构输出摘要');
    assert.deepEqual(directorySnapshot(path.join(project, 'build/web-mobile')), creator.build_sha256, '隔离实构全部产物变化');
    const publicPath = containedFile(project, 'assets/resources/development/gameplay.json');
    assert.equal(hash(publicPath), creator.public_sha256);
    let proof = null;
    if (evidence.artifact_reused) {
        const proofPath = containedFile(directory, 'reused-artifact.json');
        proof = JSON.parse(fs.readFileSync(proofPath, 'utf8'));
        for (const key of ['passed', 'current_source_matches', 'isolated_inputs_match', 'build_outputs_match']) assert.equal(proof[key], true, '复用证明未通过：' + key);
        assert.equal(proof.runtime_resource_override, false); assert.equal(proof.creator_rerun, false);
        assert.equal(proof.run_id, evidence.run_id);
        assert.equal(path.resolve(proof.artifact_directory), artifact);
        assert.equal(path.resolve(proof.project), project);
        assert.equal(proof.creator_report_sha256, hash(creatorPath));
        assert.equal(proof.project_copy_sha256, hash(actualCopyPath));
        assert.equal(proof.public_sha256, creator.public_sha256);
        assert.equal(proof.fixture_sha256, evidence.fixture_sha256);
        assert.equal(proof.source_files, Object.keys(copied.source_sha256).length);
        assert.equal(proof.build_files, Object.keys(creator.build_sha256).length);
    }
    return { directory, project, copyPath, creator, copied, proof };
}

function validateStaticIdentity(evidence, meta, health) {
    assert.equal(meta.project_root, evidence.project_root);
    assert.equal(meta.run_id, evidence.run_id);
    // Windows创建时钟由Python所有权helper核验；这里不把其64位值经Number比较。
    assert(Number.isSafeInteger(meta.pid) && meta.pid > 0, '缺少已登记静态进程PID');
    assert.equal(meta.pid, evidence.static_pid);
    const option = name => { const index = meta.argv.indexOf(name); assert(index >= 0, '静态进程缺少参数：' + name); return meta.argv[index + 1]; };
    assert.equal(path.resolve(meta.argv[1]), path.join(evidence.project_root, 'tools/client/serve.mjs'));
    assert.equal(option('--instance-id'), evidence.run_id);
    assert.equal(path.resolve(option('--build-root')), path.join(evidence.project, 'build/web-mobile'));
    assert.equal(option('--port'), String(evidence.static_port));
    assert.equal(option('--account-port'), '18110'); assert.equal(option('--game-port'), '18111');
    assert.equal(health.pid, meta.pid); assert.equal(health.run_id, evidence.run_id);
    assert.equal(health.port, evidence.static_port);
    assert.equal(health.account_port, 18110); assert.equal(health.game_port, 18111);
    assert.equal(path.resolve(health.build_root), path.join(evidence.project, 'build/web-mobile'));
    assert.equal(health.webMobileBuild, true); assert.equal(health.developmentOnly, true);
}

function parseQualification(value, roleHex) {
    const q = JSON.parse(value.trim());
    assert(q.passed && q.only_before_owner && q.book_from_normal_create);
    assert.equal(q.role_id_sha256, textHash(roleHex));
    assert.equal(q.explicit_count, 2);
    assert.equal(q.level, 19);
    for (const key of ['skills_seeded', 'binding_seeded', 'position_seeded', 'body_seeded', 'pack_size_or_price_inferred']) assert.equal(q[key], false, key);
    return q;
}

function initialSummonCooldown(skill) {
    // 此流程只正常学习初学0级，等待实际base/global/action中最长值，不引用不存在的派生字段。
    assert(skill && skill.id === 17, '缺少真实召唤候选');
    const values = ['cooldown_base_ms', 'global_spell_ms', 'action_ms'].map(key => {
        assert(Number.isSafeInteger(skill[key]) && skill[key] > 0, '缺少候选时序：' + key);
        return skill[key];
    });
    return Math.max(...values);
}

function hostingAutoDisabled(session) {
    // Playwright轮询必须同步返回Boolean，不能把未await的Promise当作已关闭。
    return Boolean(session?.hosting && session.hosting.autoBattleEnabled === false);
}

async function disableAutoForManual(page, observed, tap) {
    const checked = await page.evaluate(() => {
        const toggle = cc.find('Canvas/Safe/MapViewport/Actions/Auto')?.getComponent(cc.Toggle);
        if (!toggle) throw Error('保存Auto Toggle不存在');
        return toggle.isChecked;
    });
    if (checked) await tap('MapViewport/Actions/Auto');
    await page.waitForFunction(hostingAutoDisabled, observed, { timeout: 15000 });
    return { touched: checked, authoritativeDisabled: true };
}

function observerDiagnostics(value) {
    if (!value) return null;
    // 白名单只读元数据；不保存captures PNG、handler或未知payload。
    return {
        phase: value.phase,
        samples: (value.samples ?? []).slice(0, 128).map(({ at, phase, handle, life, definitionId, action, frame, direction, frames, blend }) =>
            ({ at, phase, handle, life, definitionId, action, frame, direction, frames, blend })),
        audio: (value.audio ?? []).slice(0, 1024).map(({ at, phase, loaded, gestureUnlocked, gestureAt, voices }) =>
            ({ at, phase, loaded, gestureUnlocked, gestureAt, voices: (voices ?? []).slice(0, 2).map(({ cue, identity, elapsedMs, playCalls, playCallAt, playing, clipBound }) =>
                ({ cue, identity, elapsedMs, playCalls, playCallAt, playing, clipBound })) })),
        error: String(value.error ?? '').slice(0, 4096),
        captureFrameIndices: Object.keys(value.captures ?? {}).filter(key => /^(0|[1-9]\d*)$/.test(key))
            .map(Number).filter(Number.isSafeInteger).sort((a, b) => a - b).slice(0, 64),
    };
}

async function failureDiagnostics(page, readState, readObserver) {
    const sceneName = await page.evaluate(() => window.cc?.director?.getScene()?.name ?? '');
    if (!['Game', 'RoleSelect'].includes(sceneName)) return { failureScene: sceneName, failureScreenshot: '登录前不读取或保存可能含凭据的画面' };
    const result = { failureScene: sceneName };
    try { result.failureState = await readState(); } catch (error) { result.failureStateReadError = String(error.message); }
    try { result.failureObservation = observerDiagnostics(await readObserver()); } catch (error) { result.failureObserverReadError = String(error.message); }
    return result;
}

async function run() {
    const secrets = MEMORY_SECRETS;
    const arg = (name, fallback) => { const i = process.argv.indexOf(name); return i < 0 ? fallback : process.argv[i + 1]; };
    const out = path.resolve(ROOT, arg('--report-dir', '.work/pet-visual-20261029/gui-browser'));
    assert(out.startsWith(path.join(ROOT, '.work') + path.sep));
    fs.mkdirSync(out, { recursive: true });
    assert(!fs.existsSync(path.join(out, 'report.json')), '保留旧现场，必须使用新目录');
    const evidencePath = path.resolve(ROOT, arg('--qa-evidence', ''));
    const evidence = JSON.parse(fs.readFileSync(evidencePath, 'utf8'));
    const { directory, project, creator, copied, copyPath, proof } = validateQAArtifacts(evidence, ROOT, evidencePath);
    const protectedConfig = ['config/development/first-loop.json', 'config/development/pet-runtime-candidates.json', 'config/development/summon-candidates.json', 'config/schema/catalog.json', 'outputs/development-prep-20261026/玩法配置.xlsx', 'outputs/development-prep-20261026/架构配置.xlsx'];
    const configBefore = Object.fromEntries(protectedConfig.map(relative => [relative, hash(path.join(ROOT, relative))]));
    function verifyMainInputs() {
        assert.deepEqual(projectInputSnapshot(path.join(ROOT, 'client')), copied.source_sha256, '主工程与本轮复制来源不一致');
        for (const [relative, expected] of Object.entries(configBefore)) assert.equal(hash(path.join(ROOT, relative)), expected, 'QA改变主配置：' + relative);
    }
    verifyMainInputs();
    const configPath = evidence.argv[evidence.argv.indexOf('-config') + 1];
    assert.equal(path.resolve(configPath), path.join(directory, 'conf/dev/server.json'));
    assert.equal(hash(configPath), evidence.config_sha256);
    const config = JSON.parse(fs.readFileSync(configPath, 'utf8'));
    assert.equal(config.database, evidence.database);
    assert.equal(path.resolve(config.fixture_file), path.join(directory, 'config/development/first-loop.json'));
    assert.equal(hash(config.fixture_file), evidence.fixture_sha256);
    const fixture = JSON.parse(fs.readFileSync(config.fixture_file, 'utf8'));
    const cooldown = initialSummonCooldown(fixture.skills.find(s => s.id === 17));
    const publicPath = path.join(project, 'assets/resources/development/gameplay.json');
    assert.equal(hash(publicPath), creator.public_sha256);
    const gameplay = JSON.parse(fs.readFileSync(publicPath, 'utf8'));
    const main = JSON.parse(fs.readFileSync(path.join(ROOT, 'config/development/first-loop.json'), 'utf8'));
    assert(main.rule_version === 18 && !main.skills.some(s => s.id === 17) && !main.casting_supply.enabled && main.pet_runtime.bootstrap_mode === 'legacy_untrained_pet');
    assert(fixture.rule_version === 18 && fixture.skills.some(s => s.id === 17) && fixture.casting_supply.enabled && fixture.pet_runtime.bootstrap_mode === 'none');
    assert(gameplay.rule_version === 18 && gameplay.skills.some(s => s.id === 17) && gameplay.casting_supply.enabled);
    const manifestPath = path.join(project, 'assets/resources/development/manifest.json');
    const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
    const appear = manifest.profiles.pet_skeleton.layers[0].actions.appear;
    assert(appear.count > 2 && appear.duration_ms > 0 && appear.direction_stride === 0 && appear.blend === 'additive');
    assert.equal(manifest.effects['17'].cast_sound, 'summon_skeleton_cast');
    assert.equal(appear.sound, 'pet_skeleton_born');
    const indices = [0, Math.floor((appear.count - 1) / 2), appear.count - 1];
    const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
    const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
    async function verifyHealth() {
        const values = {};
        for (const [mode, port] of Object.entries(evidence.ports)) {
            const response = await fetch(`http://127.0.0.1:${port}/healthz`, { signal: AbortSignal.timeout(2000) });
            assert(response.ok); const value = await response.json();
            assert.equal(value.fixture_hash, evidence.fixture_sha256); assert.equal(value.mode, mode);
            assert.equal(String(value.rule_version), '18'); assert.equal(value.instance, evidence.health_instances[mode]); values[mode] = value.instance;
        }
        const staticResponse = await fetch(`http://127.0.0.1:${evidence.static_port}/healthz`, { signal: AbortSignal.timeout(2000) });
        assert(staticResponse.ok); const staticState = await staticResponse.json();
        const staticMeta = JSON.parse(fs.readFileSync(containedFile(directory, 'static-runtime.json'), 'utf8'));
        validateStaticIdentity(evidence, staticMeta, staticState);
        return values;
    }
    const report = { passed: false, scope: '独立rule18/正常注册道士创角/19级资格与Count2/原书25学习/触屏新召及Recall/健康重入；非购符包规格、自然养成、尸体重召、容量或真机', runId: evidence.run_id, fixtureHash: evidence.fixture_sha256, publicHash: creator.public_sha256, manifestHash: hash(manifestPath), mainInputIdentity: { projectCopy: copyPath, projectCopyHash: hash(copyPath), clientFileCount: Object.keys(copied.source_sha256).length, configBefore }, checks: [], errors: [], warnings: [], protocolErrors: [], sent: [], casts: [], born: [], entries: [], acks: [], states: [], gpu: [], credentialsWritten: false, bodySeeded: false, privateCastInvoked: false, routeOverride: false, health: [{ phase: 'startup', instances: await verifyHealth() }] };
    report.artifactIdentity = { directory: evidence.artifact_directory, reused: evidence.artifact_reused, proof: evidence.artifact_proof,
        proofHash: proof ? hash(evidence.artifact_proof) : null, creatorReport: evidence.creator_report, creatorReportHash: hash(evidence.creator_report),
        staticPort: evidence.static_port, staticPid: evidence.static_pid, staticRuntime: evidence.static_runtime,
        excludedGeneratedEditorCaches: EDITOR_CACHES, generatedEditorCacheSha256: creator.generated_editor_cache_sha256 };
    const browser = await chromium.launch({ channel: 'chrome', headless: true, args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader', '--disable-save-password-bubble'] });
    const context = await browser.newContext({ viewport: { width: 720, height: 1280 }, hasTouch: true, isMobile: true, deviceScaleFactor: 1 });
    const page = await context.newPage();
    let created = null, createRequest = null, observed;
    const watchdog = setTimeout(() => { report.errors.push('GUI总预算超时'); void page.close().catch(() => {}); }, 150000);
    page.on('pageerror', error => report.errors.push(error.stack || error.message));
    page.on('console', message => { if (['warning', 'error'].includes(message.type())) report.warnings.push({ type: message.type(), text: message.text().slice(0, 700) }); });
    page.on('request', request => { try { if (new URL(request.url()).pathname !== '/account/rpc') return; const body = request.postDataBuffer(); if (!body) return; const p = packet(body); if (p?.id === 10005) { const q = pb.CreateRoleRequest.decode(p.body); assert.equal(q.job, 2); assert.equal(createRequest, null); createRequest = { request: p.request, job: q.job, slot: q.slot }; } } catch (error) { report.errors.push('创角观察：' + error.message); } });
    page.on('response', async response => { try {
        if (new URL(response.url()).pathname !== '/account/rpc') return;
        const p = packet(await response.body()); if (!p || p.error) return;
        // 登录Token和短票仅加入内存脱敏表，任何console错误也不得把它们写入报告。
        if ([10002, 10010].includes(p.id)) { const q = pb.LoginResult.decode(p.body); if (q.account_token) secrets.push(q.account_token); }
        if (p.id === 10008) { const q = pb.EntryTicketResult.decode(p.body); if (q.ticket) secrets.push(q.ticket); }
        if (p.id === 10006) { assert(createRequest && createRequest.request === p.request); const q = pb.RoleSummary.decode(p.body); assert.equal(q.job, 2); assert.equal(q.key.role_id.length, 16); created = { hex: Buffer.from(q.key.role_id).toString('hex'), slot: q.slot, job: q.job }; }
    } catch (error) { report.errors.push('创角回包观察：' + error.message); } });
    page.on('websocket', socket => {
        socket.on('framesent', event => { try {
            const p = packet(event.payload); if (!p) return;
            assert(![30015, 30014, 20011].includes(p.id), 'S2C不能由客户端发出');
            if ([40014, 30003].includes(p.id)) { const q = (p.id === 40014 ? pb.CastWithMaterialRequest : pb.CastIntent).decode(p.body); report.sent.push({ time: Date.now(), id: p.id, skill: q.skill_id, scene: q.scene_epoch.toString() }); }
            if (p.id === 4) { const q = pb.EnterWorldAck.decode(p.body); report.acks.push({ scene: q.scene_epoch.toString(), baseline: q.baseline_id.toString() }); }
            assert(report.sent.length < 128 && report.acks.length < 8, '观察超过有限工具预算');
        } catch (error) { report.errors.push('发送观察：' + error.message); } });
        socket.on('framereceived', event => { try {
            const p = packet(event.payload); if (!p) return; if (p.error) { report.protocolErrors.push({ id: p.id, code: p.error }); return; }
            if (p.id === 20002) { const q = pb.EnterLineResult.decode(p.body); assert.equal(q.resource_version.toString(), '18'); report.entries.push({ binding: p.binding, scene: q.scene_epoch.toString(), baseline: q.baseline_id.toString(), map: q.room_id.toString() }); }
            if (p.id === 30006) { const q = pb.CastAccepted.decode(p.body); report.casts.push({ time: Date.now(), binding: p.binding, cast: q.cast_id.toString(), scene: q.scene_epoch.toString(), skill: q.skill_id, caster: q.caster_handle, life: q.caster_life.toString() }); }
            if (p.id === 30015) { const q = pb.PetBorn.decode(p.body); report.born.push({ time: Date.now(), binding: p.binding, scene: q.scene_epoch.toString(), baseline: q.baseline_id.toString(), sequence: q.state_sequence.toString(), handle: q.entity_handle, life: q.entity_life.toString(), cast: q.source_cast_id.toString(), skill: q.source_skill_id, map: q.map_id.toString(), direction: q.direction, elapsedMs: q.elapsed_ms }); }
            assert(report.casts.length < 32 && report.born.length < 8 && report.entries.length < 8, '接收观察超过有限工具预算');
        } catch (error) { report.errors.push('接收观察：' + error.message); } });
    });
    async function until(predicate, label, timeout = 30000) { const end = Date.now() + timeout; do { if (await predicate()) return; await sleep(50); } while (Date.now() < end); throw Error('等待超时：' + label); }
    async function scene(name) { await page.waitForFunction(name => { const s = window.cc?.director?.getScene(); return !!(s?.name === name && s.getComponentsInChildren('AppRoot').length && cc.find('Canvas/Safe')?.activeInHierarchy && !cc.find('Canvas/Loading')?.activeInHierarchy); }, name, { timeout: 60000 }); }
    async function idle() { await page.waitForFunction(() => !cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]?.uiActionBusy, null, { timeout: 15000 }); }
    async function click(expression, argument) { const p = await page.evaluate(({ expression, argument }) => { const n = Function('argument', 'return (' + expression + ')')(argument); if (!n?.activeInHierarchy) throw Error('节点不可见'); const b = n.getComponent(cc.Button); if (b && !b.interactable) throw Error('按钮不可用'); const r = n._uiProps.uiTransformComp.getBoundingBoxToWorld(), v = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect(); return { x: canvas.x + (r.x + r.width / 2) / v.width * canvas.width, y: canvas.y + (v.height - r.y - r.height / 2) / v.height * canvas.height }; }, { expression, argument }); await page.touchscreen.tap(p.x, p.y); }
    async function tap(suffix) { await click('cc.find("Canvas/Safe/"+argument)', suffix); }
    async function edit(suffix, value) { await tap(suffix); const input = page.locator('input:visible'); await until(async () => { if (await input.isVisible()) return true; await tap(suffix); return false; }, '保存EditBox触屏激活', 5000); await input.fill(value); }
    async function popup(name) { await page.waitForFunction(name => !!cc.find('Canvas/Safe/PopupView/Panels/' + name)?.activeInHierarchy, name); }
    async function dismiss() { await page.touchscreen.tap(12, 200); await sleep(80); }
    async function closePanels() { for (let i = 0; i < 8; i++) { if (!await page.evaluate(() => { const a = cc.director.getScene().getComponentsInChildren('AppRoot')[0]; return !!(a.ui?.top || a.popup?.top); })) return; await dismiss(); } throw Error('面板没有正常返回'); }
    async function list(suffix, field, value) { const index = await page.evaluate(({ suffix, field, value }) => { const l = cc.find('Canvas/Safe/' + suffix).getComponent('List'), i = l.items.findIndex(item => field.split('.').reduce((v, k) => v?.[k], item) === value); if (i < 0) throw Error('列表无目标'); l.scrollToIndex(i); return i; }, { suffix, field, value }); await sleep(100); await click('cc.find("Canvas/Safe/"+argument.suffix).getComponent("List").rows.get(argument.index)', { suffix, index }); }
    async function observe() { await observed?.dispose(); observed = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session); }
    async function ready() { await page.waitForFunction(s => { const a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]; return !!(s.ready && s.entry?.roomId.toString() === '1' && a?.world?.isMapReady && a.reconciliationStarted && a.canSendInput()); }, observed, { timeout: 60000 }); }
    async function state() { return page.evaluate(s => {
        const current = cc.director.getScene(), a = current.getComponentsInChildren('AppRoot')[0], inv = s?.inventory;
        const player = s?.baseline?.entities.find(i => i.entityHandle === s?.entry?.playerHandle);
        return { time: Date.now(), sceneName: current.name, scene: s?.entry?.sceneEpoch.toString(), baseline: s?.entry?.baselineId.toString(),
            binding: s?.connection.binding.toString(), ready: s?.ready, level: inv?.level,
            books: inv?.items.filter(i => i.itemId === 25).map(i => i.quantity.toString()), materialBag: inv?.items.filter(i => i.itemId === 187).map(i => i.quantity.toString()),
            materialEquipped: inv?.equipped.filter(i => i.item.itemId === 187).map(i => i.item.quantity.toString()),
            learned: inv?.learnedSkills.filter(i => i.skillId === 17).map(i => ({ skill: i.skillId, level: i.level })),
            pets: (s?.baseline?.entities ?? []).filter(i => i.entityKind === 2).map(i => ({ handle: i.entityHandle, life: i.life.toString(), hp: i.hp.toString(), corpse: i.isPetCorpse })),
            player: player && { handle: player.entityHandle, life: player.life.toString(), hp: player.hp.toString(), mp: player.mp.toString(), position: player.position,
                direction: player.direction, movementRevision: player.movementRevision.toString(), attackSequence: player.attackSequence, castSequence: player.castSequence },
            hosting: s?.hosting && { autoBattleEnabled: s.hosting.autoBattleEnabled, remainHostingSec: s.hosting.remainHostingSec },
            audio: a?.world?.audioAudit ?? null, appearance: a?.world?.petAppearanceAudit ?? [], lastError: s?.lastError };
    }, observed); }
    async function snapshot(name) { const value = await state(); await page.screenshot({ path: path.join(out, name + '.png') }); report.states.push({ name, ...value }); return value; }
    async function selectSkill() { await closePanels(); await tap('BottomHUD/Skills/SkillsPanel'); await list('UIMgr/Panels/Skills/List', 'definition.id', 17); await popup('StatusDetails'); await dismiss(); }
    async function installObserver() {
        await page.evaluate(({ indices, count }) => {
            const own = { phase: 'fresh', samples: [], audio: [], captures: {}, error: '', lastAudio: '', count, indices };
            const handler = () => {
                try {
                    const a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]; if (!a?.world) return;
                    const audio = a.world.audioAudit, signature = JSON.stringify(audio);
                    if (signature !== own.lastAudio) { own.lastAudio = signature; own.audio.push({ at: performance.now(), phase: own.phase, ...audio }); }
                    for (const pet of a.world.petAppearanceAudit ?? []) if (pet.action === 'appear') {
                        const value = { at: performance.now(), phase: own.phase, ...pet };
                        if (!own.samples.some(s => s.phase === own.phase && s.handle === pet.handle && s.life === pet.life && s.frame === pet.frame)) own.samples.push(value);
                        if (own.phase === 'fresh' && indices.includes(pet.frame) && !own.captures[pet.frame]) own.captures[pet.frame] = { ...value, blendConstants: { source: cc.gfx?.BlendFactor.SRC_ALPHA, destination: cc.gfx?.BlendFactor.ONE }, png: document.querySelector('canvas').toDataURL('image/png') };
                    }
                    if (own.audio.length > 1024 || own.samples.length > 128) throw Error('只读渲染观察超过预算');
                } catch (error) { own.error = String(error); }
            };
            own.handler = handler; window.__petVisualReadOnly = own;
            cc.director.on(cc.Director.EVENT_AFTER_DRAW, handler);
        }, { indices, count: appear.count });
    }
    async function readObserver() { return page.evaluate(() => { if (!window.__petVisualReadOnly) return null; const { handler, ...value } = window.__petVisualReadOnly; return value; }); }
    try {
        await page.goto(`http://127.0.0.1:${evidence.static_port}/?petVisualQA=1`); await scene('Login');
        const account = 'pvq' + crypto.randomBytes(6).toString('hex'), password = crypto.randomBytes(9).toString('hex'), name = 'PV' + crypto.randomBytes(4).toString('hex'); secrets.push(account, password);
        await edit('Account', account); await edit('Password', password); await tap('Register'); await scene('RoleSelect'); await idle(); await observe();
        await until(async () => { const selected = await page.evaluate(() => { const a = cc.director.getScene().getComponentsInChildren('AppRoot')[0], n = cc.find('Canvas/Safe/JobTabs/Job2'); return { selected: a.selectedJob, ready: n?.activeInHierarchy && n.getComponent(cc.Button)?.interactable && !cc.find('Canvas/Loading')?.activeInHierarchy }; }); if (!selected.ready) return false; if (selected.selected === 2) return true; await tap('JobTabs/Job2'); return false; }, '正常Job2选择', 10000);
        await edit('Name', name); await tap('Create'); await until(() => !!created, '正常CreateRole回包'); await idle();
        await page.waitForFunction(name => cc.find('Canvas/Safe/Role0')?.getComponentsInChildren(cc.Label).some(l => l.string.includes(name)), name);
        const qualification = spawnSync('powershell.exe', ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', path.join(ROOT, 'tools/dev.ps1'), 'python3', 'tools/client/prepare_pet_visual_qa.py', '--qualify', evidencePath], { cwd: ROOT, input: JSON.stringify({ role_id: created.hex }) + '\n', encoding: 'utf8', timeout: 20000, windowsHide: true });
        assert.equal(qualification.status, 0, '资格helper失败；不输出原回包或凭据'); report.qualification = parseQualification(qualification.stdout, created.hex); report.create = { ...createRequest, roleHash: textHash(created.hex) };
        await tap('Role0'); await idle(); await tap('Enter'); await scene('Game'); await observe(); await ready();
        report.manualAutoControl = await disableAutoForManual(page, observed, tap);
        const initial = await snapshot('01-normal-new-role'); assert.equal(initial.pets.length, 0); assert.equal(initial.level, 19); assert.deepEqual(initial.books, ['1']); assert.deepEqual(initial.materialBag, ['2']); assert.equal(initial.learned.length, 0); assert.equal(report.born.length, 0);
        report.checks.push('正常注册及Job2创角，未owner资格CAS只设19级与Count2；首入无宠、原书25且无预学技能');
        await selectSkill(); await tap('UIMgr/Panels/Skills/Learn'); await popup('Confirm'); await tap('PopupView/Panels/Confirm/ConfirmButton'); await idle(); await until(async () => (await state()).learned.some(s => s.level === 0), '正常原书学习17');
        await closePanels(); await tap('BottomHUD/QuickNav/Inventory'); await list('UIMgr/Panels/Inventory/List', 'itemId', 187); await popup('Details'); await dismiss(); await tap('UIMgr/Panels/Inventory/Equip'); await popup('Confirm'); await tap('PopupView/Panels/Confirm/ConfirmButton'); await idle(); await until(async () => (await state()).materialEquipped[0] === '2', '正常装入护符');
        await selectSkill(); await tap('UIMgr/Panels/Skills/BindQuick'); await closePanels(); await installObserver(); await tap('BottomHUD/Skills/BasicAttack');
        await until(async () => { const o = await readObserver(); assert.equal(o.error, ''); return indices.every(i => o.captures[i]); }, '实际GPU首中末出生帧', 10000);
        await until(async () => (await state()).pets.some(p => !p.corpse && BigInt(p.hp) > 0n) && report.born.length === 1, '真实新召成功');
        const freshObserver = await readObserver();
        for (const index of indices) { const frame = freshObserver.captures[index]; assert.match(frame.png, /^data:image\/png;base64,/); const bytes = Buffer.from(frame.png.split(',')[1], 'base64'); assert(bytes.length > 1024); const file = `02-appear-frame-${index}.png`; fs.writeFileSync(path.join(out, file), bytes); const { png, ...metadata } = frame; assert.equal(frame.frames[0], appear.directions[frame.direction].frames[index]); assert.equal(typeof frame.blendConstants.source, 'number'); assert.equal(typeof frame.blendConstants.destination, 'number'); assert.equal(frame.blend[0].source, frame.blendConstants.source); assert.equal(frame.blend[0].destination, frame.blendConstants.destination); report.gpu.push({ file, sha256: hash(path.join(out, file)), ...metadata }); }
        const fresh = await snapshot('03-fresh-pet'), live = fresh.pets.find(p => !p.corpse && BigInt(p.hp) > 0n), born = report.born[0];
        assert.equal(report.sent.filter(p => p.id === 40014 && p.skill === 17).length, 1); assert.equal(born.handle, live.handle); assert.equal(born.life, live.life); assert.equal(born.scene, fresh.scene); assert.equal(born.baseline, fresh.baseline); assert.equal(born.binding, fresh.binding); assert.equal(born.skill, 17); assert(report.casts.some(c => c.cast === born.cast && c.skill === 17)); assert.deepEqual(fresh.materialEquipped, ['1']);
        await until(async () => { const o = await readObserver(); return [0, 1].every(i => o.audio.some(a => a.voices[i].playing && a.voices[i].playCalls > 0 && a.voices[i].cue === (i ? 'pet_skeleton_born' : 'summon_skeleton_cast'))); }, '两个真实AudioSource均播放');
        report.freshObservation = { samples: freshObserver.samples, audio: (await readObserver()).audio }; report.checks.push('正常40014新召消耗Count1，30015与同birth/scene/基线配对，GPU首中末原帧及两声道真实播放');
        await closePanels(); await tap('BottomHUD/QuickNav/Role'); await tap('UIMgr/Panels/Role/Equip'); await popup('Confirm'); await tap('PopupView/Panels/Confirm/ConfirmButton'); await idle(); await closePanels();
        const recallBefore = await state(); assert.deepEqual(recallBefore.materialBag, ['1']); assert.equal(recallBefore.materialEquipped.length, 0);
        await sleep(Math.max(0, report.casts[0].time + cooldown + 250 - Date.now())); const bornCount = report.born.length, castsBefore = report.casts.length, materialCalls = report.sent.filter(p => p.id === 40014).length;
        await page.evaluate(() => { window.__petVisualReadOnly.phase = 'recall'; });
        await tap('BottomHUD/Skills/BasicAttack'); await until(() => report.casts.length > castsBefore, '无符普通Recall受理'); await sleep(1200);
        const recall = await snapshot('04-unequipped-recall'); assert.equal(report.sent.filter(p => p.id === 40014).length, materialCalls); assert(report.sent.some(p => p.id === 30003 && p.skill === 17)); assert.equal(report.born.length, bornCount); assert.deepEqual(recall.materialBag, ['1']); assert.equal(recall.pets[0].handle, live.handle); assert.equal(recall.pets[0].life, live.life); assert(!recall.appearance.some(p => p.action === 'appear')); assert.equal(recall.audio.voices[1].playCalls, recallBefore.audio.voices[1].playCalls); assert(recall.audio.voices[0].playCalls > recallBefore.audio.voices[0].playCalls);
        assert(!(await readObserver()).samples.some(s => s.phase === 'recall'));
        report.checks.push('卸符后正常30003 Recall，同handle/Life、Count仍1；只新增施法声，不新增Born或出场声');
        report.health.push({ phase: 'before_reentry', instances: await verifyHealth() });
        await page.evaluate(() => { window.__petVisualReadOnly.phase = 'healthy'; });
        await tap('BottomHUD/Footer/ChangeRole'); await scene('RoleSelect'); await page.waitForFunction(name => cc.find('Canvas/Safe/Role0')?.getComponentsInChildren(cc.Label).some(l => l.string.includes(name)), name); await tap('Role0'); await idle(); await tap('Enter'); await scene('Game'); await observe(); await ready(); await sleep(1200);
        report.healthyAutoControl = await disableAutoForManual(page, observed, tap);
        const healthy = await snapshot('05-healthy-reentry'); assert.notEqual(healthy.binding, recall.binding); assert.equal(healthy.pets[0].handle, live.handle); assert.equal(healthy.pets[0].life, live.life); assert.equal(report.born.length, bornCount); assert(!healthy.appearance.some(p => p.action === 'appear')); assert(healthy.audio.voices.every(v => v.playCalls === 0)); report.health.push({ phase: 'after_reentry', instances: await verifyHealth() });
        const finalObserver = await readObserver(); assert.equal(finalObserver.error, ''); assert(!finalObserver.samples.some(s => s.phase === 'healthy')); report.finalObservation = { samples: finalObserver.samples, audio: finalObserver.audio };
        report.checks.push('正常换角新票健康重入保持同birth，不重播出场动作或078-5声音');
        verifyMainInputs(); report.mainProtectedInputsStable = true;
        assert.equal(report.protocolErrors.length, 0); assert.equal(report.errors.length, 0, report.errors.join('\n')); report.zeroWarningsVerified = report.warnings.length === 0; report.passed = true;
    } catch (error) {
        report.error = error.stack || String(error);
        try {
            Object.assign(report, await failureDiagnostics(page, state, readObserver));
            if (['Game', 'RoleSelect'].includes(report.failureScene)) await page.screenshot({ path: path.join(out, 'failure.png') });
        } catch (failure) { report.failureDiagnosticsError = String(failure.message); }
        throw error;
    } finally {
        clearTimeout(watchdog);
        try { verifyMainInputs(); report.mainProtectedInputsStable = true; } catch (error) { report.mainProtectedInputsStable = false; report.passed = false; report.errors.push(error.message); }
        try { await page.evaluate(() => { const o = window.__petVisualReadOnly; if (o?.handler) cc.director.off(cc.Director.EVENT_AFTER_DRAW, o.handler); delete window.__petVisualReadOnly; }); await observed?.dispose(); } catch {}
        fs.writeFileSync(path.join(out, 'report.json'), redact(JSON.stringify(report, null, 2)) + '\n', 'utf8');
        await browser.close();
    }
    assert(report.passed && report.mainProtectedInputsStable, 'QA未通过或主输入发生变化');
    console.log(JSON.stringify({ passed: report.passed, checks: report.checks.length, errors: report.errors.length, warnings: report.warnings.length }));
}

module.exports = { packet, validateQAIdentity, validateQAArtifacts, validateStaticIdentity, projectInputSnapshot, directorySnapshot, parseQualification, initialSummonCooldown, hostingAutoDisabled, disableAutoForManual, observerDiagnostics, failureDiagnostics, EDITOR_CACHES, QA_DATABASE_PREFIX };
if (require.main === module) run().catch(error => { console.error(redact(error.message)); process.exitCode = 1; });
