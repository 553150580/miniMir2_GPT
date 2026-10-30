// 通过正常触屏检查保存确认窗的完整排版、取消和地图往返；不播种数据库或调用私有业务。
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs'), path = require('node:path'), crypto = require('node:crypto');
const {packet} = require('./verify_warehouses_browser.cjs');
const root = path.resolve(__dirname, '../..');
const sha = p => crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const read = p => JSON.parse(fs.readFileSync(p, 'utf8'));
const arg = name => { const i = process.argv.indexOf(name); return i < 0 ? null : process.argv[i + 1]; };
const secrets = [], redact = s => secrets.reduce((text, secret) => text.replaceAll(secret, '[已隐去测试凭据]'), s);
async function main() {
    const creatorPath = path.resolve(arg('--creator-report') || 'docs/客户端构建检查.json');
    const out = path.resolve(root, arg('--report-dir') || '.work/map-confirm-wrap-20261030/browser/run-01');
    assert(out.startsWith(path.join(root, '.work/map-confirm-wrap-20261030') + path.sep));
    assert(!fs.existsSync(path.join(out, 'report.json'))); fs.mkdirSync(out, {recursive: true});
    const creator = read(creatorPath), fixturePath = path.join(root, 'config/development/first-loop.json');
    assert(creator.success && creator.creator === '3.8.8' && creator.exit_code === 36);
    const source = {...creator.asset_hashes, ...creator.script_hashes};
    const snapshot = () => Object.fromEntries(Object.keys(source).map(p => [p, sha(path.join(root, p))]));
    assert.deepEqual(snapshot(), source);
    const report = {passed: false, startedAt: new Date().toISOString(), sourceBefore: snapshot(), creatorReportPath: creatorPath,
        creatorReportSha256: sha(creatorPath), fixtureSha256: sha(fixturePath), toolSha256: sha(__filename),
        checks: [], errors: [], warnings: [], confirmations: [], screenshots: [], travelCommits: [], warehouseWrites: 0,
        touchInput: true, privateBusinessInvoked: false, qualificationSeeded: false, credentialsWritten: false, physicalMobileVerified: false};
    let browser, page, observed;
    try {
        const health = async () => {
            const rows = [];
            for (const port of [18080, 18100, 18101, 18102]) {
                const r = await fetch(`http://127.0.0.1:${port}/healthz`, {signal: AbortSignal.timeout(5000)});
                assert(r.ok); const value = await r.json();
                if (port !== 18080) assert(value.status === 'ok' && value.rule_version === read(fixturePath).rule_version && value.fixture_hash === report.fixtureSha256);
                rows.push({port, ...value});
            }
            return rows;
        };
        report.healthBefore = await health();
        const {chromium} = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
        browser = await chromium.launch({channel: 'chrome', headless: true, args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader']});
        const context = await browser.newContext({viewport: {width: 720, height: 1280}, hasTouch: true, isMobile: true, deviceScaleFactor: 1});
        page = await context.newPage();
        page.on('pageerror', e => report.errors.push(e.stack || e.message));
        page.on('console', m => { if (m.type() === 'error') report.errors.push(m.text()); else if (m.type() === 'warning') report.warnings.push(m.text()); });
        const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
        page.on('websocket', socket => socket.on('framesent', event => {
            try {
                const p = packet(event.payload); if (!p) return;
                if (p.id === 40022) report.warehouseWrites++;
                if (p.id === 20009) { const q = pb.MapTravelCommit.decode(p.body); report.travelCommits.push({request: p.request, binding: p.binding, id: q.travel_id, scene: q.scene_epoch.toString(), cancel: q.cancel}); }
            } catch (e) { report.errors.push(e.message); }
        }));
        const frames = () => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
        async function scene(name) { await page.waitForFunction(name => Boolean(window.cc?.director?.getScene()?.name === name && cc.find('Canvas/Safe')?.activeInHierarchy && !cc.find('Canvas/Loading')?.activeInHierarchy), name, {timeout: 90000}); await frames(); }
        async function tap(suffix) {
            await page.waitForFunction(suffix => { const n = cc.find('Canvas/Safe/' + suffix), a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0], b = n?.getComponent(cc.Button); return Boolean(n?.activeInHierarchy && (!b || b.interactable && b.enabledInHierarchy) && (suffix.startsWith('PopupView/') || !a?.uiActionBusy)); }, suffix, {timeout: 30000});
            const p = await page.evaluate(suffix => { const t = cc.find('Canvas/Safe/' + suffix)._uiProps.uiTransformComp, w = t.convertToWorldSpaceAR(new cc.Vec3((.5 - t.anchorPoint.x) * t.width, (.5 - t.anchorPoint.y) * t.height)), v = cc.view.getVisibleSize(), r = document.querySelector('canvas').getBoundingClientRect(); return {x: r.x + w.x / v.width * r.width, y: r.y + (v.height - w.y) / v.height * r.height}; }, suffix);
            await page.touchscreen.tap(p.x, p.y); await frames();
        }
        async function edit(suffix, value) { await tap(suffix); const input = page.locator('input:visible'); await input.waitFor({timeout: 10000}); await input.fill(value); await frames(); }
        async function ready(map) { await page.waitForFunction(({s, map}) => { const a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]; return Boolean(s.ready && s.entry?.roomId.toString() === map && a?.world?.isMapReady && a.reconciliationStarted && !a.attaching && !a.inputBarrier && !a.uiActionBusy); }, {s: observed, map}, {timeout: 120000}); await frames(); }
        const state = () => page.evaluate(s => { const p = s.baseline.entities.find(x => x.entityHandle === s.entry.playerHandle); return {map: s.entry.roomId.toString(), scene: s.entry.sceneEpoch.toString(), life: p.life.toString(), items: s.inventory.items.map(x => ({id: Array.from(x.instanceId), version: x.version.toString(), quantity: x.quantity.toString()}))}; }, observed);
        async function inspect(name, title, expected, screenshot = true) {
            await page.waitForFunction(() => cc.find('Canvas/Safe/PopupView/Panels/Confirm')?.activeInHierarchy, null, {timeout: 120000}); await frames();
            const data = await page.evaluate(() => {
                const base = 'Canvas/Safe/PopupView/Panels/Confirm/', node = cc.find(base + 'Message'), label = node.getComponent(cc.Label), t = node._uiProps.uiTransformComp;
                const lines = Array.from(label.textLayoutData.parsedString), ctx = label.assemblerData.context;
                const box = n => { const r = n._uiProps.uiTransformComp.getBoundingBoxToWorld(); return {x: r.x, y: r.y, width: r.width, height: r.height}; };
                return {title: cc.find(base + 'Title').getComponent(cc.Label).string, text: label.string, wrap: label.enableWrapText,
                    fontSize: label.fontSize, actualFontSize: label.actualFontSize, lineHeight: label.lineHeight, lines,
                    lineWidths: lines.map(line => ctx.measureText(line).width), width: t.width, height: t.height,
                    message: box(node), buttons: ['ConfirmButton', 'CancelButton'].map(name => box(cc.find(base + name))), viewport: {width: innerWidth, height: innerHeight}};
            });
            report.confirmations.push({name, ...data});
            assert.equal(data.title, title); if (expected) assert.equal(data.text, expected);
            assert(data.wrap); assert.equal(data.fontSize, 24); assert.equal(data.actualFontSize, 24);
            assert.equal(data.width, 530); assert.equal(data.height, 280); assert(data.lines.length > 1);
            assert.equal(data.lines.join('').replace(/\s/g, ''), data.text.replace(/\s/g, ''));
            assert(data.lineWidths.every(width => width <= data.width + 1)); assert(data.lines.length * data.lineHeight <= data.height);
            const overlap = (a, b) => a.x < b.x + b.width && a.x + a.width > b.x && a.y < b.y + b.height && a.y + a.height > b.y;
            assert(data.buttons.every(button => !overlap(data.message, button)));
            if (screenshot) { const file = name + '.png'; await page.screenshot({path: path.join(out, file)}); report.screenshots.push(file); }
        }
        await page.goto('http://127.0.0.1:18080/'); await scene('Login');
        const account = 'wrap' + crypto.randomBytes(5).toString('hex'), password = crypto.randomBytes(9).toString('hex'); secrets.push(account, password);
        await edit('Account', account); await edit('Password', password); await tap('Register'); await scene('RoleSelect'); await tap('JobTabs/Job2');
        await page.waitForFunction(() => cc.director.getScene().getComponentsInChildren('AppRoot')[0].selectedJob === 2);
        const name = '换行' + crypto.randomBytes(3).toString('hex'); await edit('Name', name); await tap('Create');
        await page.waitForFunction(name => cc.find('Canvas/Safe/Role0')?.getComponentsInChildren(cc.Label).some(l => l.string.includes(name)), name);
        await tap('Role0'); await tap('Enter'); await scene('Game');
        observed = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
        await ready('1'); if (await page.evaluate(() => cc.find('Canvas/Safe/MapViewport/Actions/Auto').getComponent(cc.Toggle).isChecked)) await tap('MapViewport/Actions/Auto');
        const sizes = [{width: 720, height: 1280}, {width: 1024, height: 1280}, {width: 720, height: 1600}];
        const messages = {town: '目标城镇资源已准备。确认前往城镇？原个人战斗现场将由服务端保留。', personal: '目标个人地图资源已准备。确认返回原战斗现场？'};
        report.initial = await state();
        for (const destination of ['town', 'personal']) {
            const source = await state();
            for (const size of sizes) {
                await page.setViewportSize(size); await frames(); await tap('BottomHUD/Footer/Map');
                await inspect(`${destination}-${size.width}x${size.height}`, '地图往返', messages[destination]);
                await tap('PopupView/Panels/Confirm/CancelButton'); await ready(source.map);
                const after = await state(); assert.equal(after.scene, source.scene); assert.equal(after.life, source.life);
            }
            await page.setViewportSize(sizes[0]); await frames(); await tap('BottomHUD/Footer/Map');
            await inspect(destination + '-accepted', '地图往返', messages[destination], false);
            await tap('PopupView/Panels/Confirm/ConfirmButton'); await ready(destination === 'town' ? '2' : '1');
            assert.equal((await state()).life, source.life);
        }
        await tap('BottomHUD/Footer/Functions'); await tap('UIMgr/Panels/Functions/Pages/Page0/Warehouse');
        await page.waitForFunction(() => Boolean(cc.director.getScene().getComponentsInChildren('AppRoot')[0].warehouse?.page));
        for (const list of ['BagList', 'WarehouseList']) {
            const row = await page.evaluate(list => { const listNode = cc.find('Canvas/Safe/UIMgr/Panels/Warehouse/' + list), c = listNode.getComponent('List'), n = c.rows.get(0), t = n._uiProps.uiTransformComp, w = t.convertToWorldSpaceAR(new cc.Vec3(0, -t.height / 2)), v = cc.view.getVisibleSize(), r = document.querySelector('canvas').getBoundingClientRect(); return {x: r.x + w.x / v.width * r.width, y: r.y + (v.height - w.y) / v.height * r.height}; }, list);
            await page.touchscreen.tap(row.x, row.y); await frames();
        }
        const beforeWarehouse = await state(); await tap('UIMgr/Panels/Warehouse/DepositItem');
        await inspect('warehouse-confirm', '仓库操作确认'); await tap('PopupView/Panels/Confirm/CancelButton');
        assert.deepEqual((await state()).items, beforeWarehouse.items); assert.equal(report.warehouseWrites, 0);
        assert.equal(report.travelCommits.filter(q => q.cancel).length, 6); assert.equal(report.travelCommits.filter(q => !q.cancel).length, 2);
        report.checks = ['三种视口完整自然换行，原24字号与530×280正文尺寸保持，文字不与按钮重叠', '往返六次取消保留原场景和Life，两次确认正常完成旅行', '仓储八行确认完整可见，取消不发资产请求、不改变背包'];
        report.healthAfter = await health(); assert.deepEqual(report.healthAfter, report.healthBefore);
        report.sourceAfter = snapshot(); assert.deepEqual(report.sourceAfter, report.sourceBefore); assert.equal(sha(__filename), report.toolSha256);
        assert.deepEqual(report.errors, []); report.passed = true;
    } catch (error) {
        report.error = redact(error.stack || error.message);
        if (page) try { await page.screenshot({path: path.join(out, 'failure.png')}); } catch {}
        throw error;
    } finally {
        report.finishedAt = new Date().toISOString(); await observed?.dispose(); await browser?.close();
        fs.writeFileSync(path.join(out, 'report.json'), redact(JSON.stringify(report, null, 2)) + '\n');
    }
    console.log(JSON.stringify({passed: report.passed, checks: report.checks.length, errors: report.errors.length, warnings: report.warnings.length}));
}
if (require.main === module) main().catch(error => { console.error(redact(error.stack || error.message)); process.exitCode = 1; });
