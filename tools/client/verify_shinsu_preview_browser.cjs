// 使用本机 Creator 构建和保存的 Game 场景验证神兽原帧接入。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));

const work = fs.realpathSync(path.resolve('.work'));
const reportPath = fs.realpathSync(process.argv[2]);
assert(reportPath.startsWith(work + path.sep), '验收只使用隔离构建报告');
const creator = JSON.parse(fs.readFileSync(reportPath, 'utf8'));
const build = fs.realpathSync(creator.build);
assert(creator.passed && build.startsWith(work + path.sep), '隔离 Creator 构建必须已通过');
const results = path.dirname(reportPath);
const webMainBundleSha256 = crypto.createHash('sha256').update(fs.readFileSync(path.join(build, 'assets/main/index.js'))).digest('hex');
const report = { passed: false, build, webMainBundleSha256, sourceProfileSha256: creator.source_profile_sha256, errors: [], rendered: [] };
const mimeTypes = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.json': 'application/json', '.css': 'text/css', '.png': 'image/png', '.wasm': 'application/wasm', '.wav': 'audio/wav' };

// 专项静态端口只读，静态预览输入不访问主试玩或任何数据库。
const server = http.createServer((request, response) => {
    try {
        const relative = '.' + decodeURIComponent(new URL(request.url, 'http://localhost').pathname);
        let file = path.resolve(build, relative);
        if (file === build) file = path.join(build, 'index.html');
        if (!file.startsWith(build + path.sep) || !fs.statSync(file).isFile()) throw Error('构建文件不存在');
        response.writeHead(200, { 'Content-Type': mimeTypes[path.extname(file)] ?? 'application/octet-stream' });
        fs.createReadStream(file).pipe(response);
    } catch { response.writeHead(request.url === '/favicon.ico' ? 204 : 404).end(); }
});

async function main() {
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
    const browser = await chromium.launch({ channel: 'chrome', args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    try {
        const page = await browser.newPage({ viewport: { width: 720, height: 1280 }, hasTouch: true, isMobile: true, deviceScaleFactor: 1 });
        page.on('pageerror', error => report.errors.push(error.message));
        await page.goto(`http://127.0.0.1:${server.address().port}/`);
        await page.waitForFunction(() => window.cc?.director?.getScene()?.name === 'Login', null, { timeout: 60000 });
        report.imports = await page.evaluate(async () => {
            const engine = await System.import('cc');
            const { WorldAssets } = await System.import('chunks:///_virtual/WorldAssets.ts');
            const { GameWorldView } = await System.import('chunks:///_virtual/GameWorldView.ts');
            const bundle = engine.assetManager.getBundle('main');
            const game = await new Promise((resolve, reject) => bundle.loadScene('Game', (error, asset) => error ? reject(error) : resolve(asset.scene)));
            for (const component of ['AppRoot', 'MapTouchNavigator', 'TouchJoystick']) {
                for (const script of game.getComponentsInChildren(component)) script.enabled = false;
            }
            engine.director.runSceneImmediate(game);
            const readyFrame = () => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
            await readyFrame();
            const viewport = engine.find('Canvas/Safe/MapViewport');
            const world = viewport.getChildByName('World'), entities = world.getChildByName('Entities');
            const assets = await WorldAssets.prepare(2, () => {}, '1');
            const view = new GameWorldView(viewport, world, entities, '开发外观验收', 2, '1', 1);
            view.prepare(assets, () => {});
            view.bindPresentation({ connection: 1, binding: '1', role: '1:0123456789abcdef0123456789abcdef', scene: '1', baseline: '1', map: '1', travelId: '' }, () => true);
            const number = value => ({
                toString: () => String(value), isZero: () => value === 0,
                lessThan: other => BigInt(value) < BigInt(other.toString()), equals: other => BigInt(value) === BigInt(other.toString()),
            });
            const point = assets.map.player_spawn ?? { x: 6, y: 6 };
            const body = (handle, kind, definition, position) => ({
                entityHandle: handle, entityKind: kind, definitionId: definition, position, direction: 2,
                moveIntervalMs: 600, attackIntervalMs: 600, attackSkillId: 0, attackSkillLevel: 0, action: 0,
                life: '1', movementRevision: '0', movementKind: 0, attackSequence: 0, castSequence: 0,
                hp: number(100), hpMax: number(100), mp: number(100), mpMax: number(100), statuses: [],
            });
            const player = body(1, 1, 2, point);
            let pet = body(20, 2, 80030, { x: point.x - 1, y: point.y });
            const profile = assets.profile(pet, 2);
            if (profile?.id !== 'pet_shinsu') throw Error('真实 Creator 构建没有可加载的神兽外观');
            const actions = Object.entries(profile.layers[0].actions).map(([name, action]) => ({
                name, count: action.count, interval: action.duration_ms, directions: action.directions.length,
                loaded: action.directions.flatMap(direction => direction.frames).filter(key => assets.frames.get(key)?.texture).length,
            }));
            const refresh = () => { view.apply({ entities: [player, pet] }); view.update(0, 2); };
            window.shinsuPreviewQA = {
                async show(action) {
                    if (action === 'appear') {
                        const event = { sceneEpoch: '1', mapId: '1', entityHandle: 20, entityLife: '1', definitionId: 80030,
                            movementRevision: '0', direction: 2, sourceCastId: '1' };
                        if (!view.presentPetBorn({ event, elapsedMs: 0 })) throw Error('真实新生显示未被接受');
                    } else if (action === 'walking') {
                        pet = { ...pet, position: { ...pet.position, y: pet.position.y + 1 }, movementRevision: '1', movementKind: 1 };
                    } else if (action === 'attack') {
                        pet = { ...pet, action: 2, attackSequence: 1 };
                    } else if (action === 'struck') {
                        // 已开始的攻击应先完整播放，再注入一次独立受击。
                        const finishAttack = performance.now() + 1800;
                        while (view.petAppearanceAudit[0]?.action === 'attack' && performance.now() < finishAttack) {
                            await readyFrame();
                            view.update(0, 2);
                        }
                        pet = { ...pet, action: 3, hp: number(60) };
                    } else if (action === 'die') {
                        pet = { ...pet, action: 4, hp: number(0), isPetCorpse: true };
                    }
                    refresh();
                    const deadline = performance.now() + 1800;
                    do {
                        await readyFrame();
                        view.update(0, 2);
                        const audit = view.petAppearanceAudit[0];
                        const visual = view.visuals.get(20);
                        if (audit?.action === action && visual.body[0].spriteFrame?.texture) {
                            const frame = assets.manifest.frames[audit.frames[0]];
                            return { ...audit, source: frame.source_archive, rawFrame: frame.raw_frame,
                                visiblePixels: frame.visible_alpha_pixels, placeholder: !!visual.placeholder, corpseOnGround: visual.node.parent === world };
                        }
                    } while (performance.now() < deadline);
                    throw Error(`未观察到 ${action} 的神兽原帧`);
                },
                dispose: () => view.dispose(),
            };
            refresh();
            return { mapReady: view.isMapReady, profile: profile.id, actions };
        });
        assert.equal(report.imports.profile, 'pet_shinsu');
        assert(report.imports.mapReady);
        assert.equal(report.imports.actions.reduce((sum, action) => sum + action.loaded, 0), 304);
        assert(report.imports.actions.every(action => action.directions === 8 && action.loaded === action.count * 8));
        for (const [action, source] of Object.entries({ standing: '79.Lib', appear: '79.Lib', walking: '79.Lib', attack: '80.Lib', struck: '80.Lib', die: '80.Lib' })) {
            const sample = await page.evaluate(action => window.shinsuPreviewQA.show(action), action);
            assert.equal(sample.source, source);
            assert.equal(sample.corpseOnGround, action === 'die');
            assert(!sample.placeholder && sample.visiblePixels > 0);
            report.rendered.push(sample);
        }
        await page.evaluate(() => window.shinsuPreviewQA.dispose());
        assert.deepEqual(report.errors, []);
        report.passed = true;
    } finally { await browser.close(); }
}

main().catch(error => {
    report.error = error.message;
    process.exitCode = 1;
}).finally(() => {
    server.close();
    fs.writeFileSync(path.join(results, 'shinsu-browser-report.json'), JSON.stringify(report, null, 2) + '\n', 'utf8');
    console.log(JSON.stringify(report));
});
