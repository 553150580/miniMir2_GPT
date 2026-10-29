// 真实Creator保存UI与正常新角色；不预置数据库，不调用业务方法或改写角色/库存。
'use strict';
const assert = require('node:assert/strict'), crypto = require('node:crypto'), fs = require('node:fs'), path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const root = path.resolve(__dirname, '../..'), flag = process.argv.indexOf('--report-dir');
const out = path.resolve(root, flag < 0 ? '.work/auto-recycle-20261029/browser-main' : process.argv[flag + 1]);
assert(out.startsWith(path.join(root, '.work') + path.sep), '报告只写本项目隔离工作目录');
assert(!fs.existsSync(path.join(out, 'report.json')), '已有终态证据必须保留，请选择新的report-dir');
fs.mkdirSync(out, { recursive: true });
const hash = name => crypto.createHash('sha256').update(fs.readFileSync(path.join(root, name))).digest('hex');
const build = JSON.parse(fs.readFileSync(path.join(root, 'docs/客户端构建检查.json'), 'utf8'));
assert(build.success && build.scene_prefab_layout_preserved && build.source_stable_during_build, '须先完成当前Creator实构');
const frozen = { ...build.asset_hashes, ...build.script_hashes };
for (const [file, expected] of Object.entries(frozen)) assert.equal(hash(file), expected, '当前文件已不同于构建输入：' + file);
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const fixture = JSON.parse(fs.readFileSync(path.join(root, 'config/development/first-loop.json'), 'utf8'));
assert.equal(fixture.rule_version, 19);
function packet(value) {
    const b = Buffer.from(value); if (b.length < 6) return null; let offset = 4;
    const variable = () => { let value = 0n; for (let i = 0; i < 10; i++) { assert(offset < b.length); const byte = b[offset++]; assert(i < 9 || byte <= 1); value |= BigInt(byte & 127) << BigInt(i * 7); if (!(byte & 128)) return value; } throw Error('协议头超长'); };
    const request = variable().toString(), binding = variable().toString(), error = b[1] & 1 ? Number(variable()) : 0;
    return { id: b.readUInt16BE(2), request, binding, error, body: b.subarray(offset) };
}
function interactive(name) { const engine = window.cc, scene = engine?.director.getScene(); return Boolean(scene?.name === name && scene.getComponentsInChildren('AppRoot').length && engine.find('Canvas/Safe')?.activeInHierarchy && !engine.find('Canvas/Loading')?.activeInHierarchy); }
async function main() {
    const browser = await chromium.launch({ channel: 'chrome', headless: true, args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: { width: 720, height: 1280 }, hasTouch: true, isMobile: true, deviceScaleFactor: 1 });
    const page = await context.newPage();
    const report = { passed: false, scope: '主rule19正常注册/新道士、保存UI设置与装备锁、明确应用背包、真实提交结果与健康新票；不预置数据库，不作正式分解经济/真机/容量认证', checks: [], errors: [], warnings: [], receipts: [], operations: [], observations: {}, sourceBefore: frozen, toolSha256: hash('tools/client/verify_auto_recycle_browser.cjs'), fixtureSha256: hash('config/development/first-loop.json'), publicSha256: hash('client/assets/resources/development/gameplay.json'), physicalMobileVerified: false, touchInput: true, privateBusinessInvoked: false, routeOverride: false, bodySeeded: false };
    page.on('pageerror', e => report.errors.push(e.stack || e.message));
    page.on('console', m => { if (['warning', 'error'].includes(m.type())) report.warnings.push({ type: m.type(), text: m.text().slice(0, 600) }); });
    page.on('websocket', socket => {
        socket.on('framesent', event => { const p = packet(event.payload); if (p && [40016, 40017].includes(p.id)) report.operations.push({ id: p.id, request: p.request, binding: p.binding }); });
        socket.on('framereceived', event => { try {
            const p = packet(event.payload); if (!p) return;
            if (p.id === 40018 && !p.error) {
                // 生成器固定--no-convert，没有toObject；保持原snake字段和Long精度。
                const q = pb.EquipmentRecycleResult.decode(p.body);
                report.receipts.push({ binding: p.binding, role_revision: q.role_revision.toString(), source_seq: q.source_seq.toString(),
                    op_id: Buffer.from(q.op_id).toString('hex'), policy_revision: q.policy_revision.toString(), recycled_count: q.recycled_count.toString(),
                    gold: q.gold.toString(), bound_gold: q.bound_gold.toString(), discarded_count: q.discarded_count.toString(), discard_reason: q.discard_reason, trigger: q.trigger });
            }
            if (p.error && [40016, 40017, 40005].includes(p.id)) report.errors.push('资产协议拒绝：' + p.id + '/' + p.error);
        } catch (e) { report.errors.push('协议观察：' + e.message); } });
    });
    async function scene(name) { await page.waitForFunction(interactive, name, { timeout: 90000 }); await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))); }
    async function tap(suffix) {
        await page.waitForFunction(suffix => {
            const n = cc.find('Canvas/Safe/' + suffix), a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
            const control = n?.getComponent(cc.Button) ?? n?.getComponent(cc.Toggle);
            return Boolean(n?.activeInHierarchy && (!control || control.interactable && control.enabledInHierarchy)
                && (suffix.startsWith('PopupView/') || !a?.uiActionBusy));
        }, suffix, { timeout: 15000 });
        const p = await page.evaluate(suffix => { const n = cc.find('Canvas/Safe/' + suffix); if (!n?.activeInHierarchy) throw Error('节点不可交互：' + suffix); const b = n._uiProps.uiTransformComp.getBoundingBoxToWorld(), v = cc.view.getVisibleSize(), r = document.querySelector('canvas').getBoundingClientRect(); return { x: r.x + (b.x + b.width / 2) / v.width * r.width, y: r.y + (v.height - b.y - b.height / 2) / v.height * r.height }; }, suffix);
        await page.touchscreen.tap(p.x, p.y);
        await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    }
    async function edit(suffix, value) {
        const input = page.locator('input:visible'), deadline = Date.now() + 10000;
        do { await tap(suffix); if (await input.isVisible()) break; await new Promise(resolve => setTimeout(resolve, 100)); } while (Date.now() < deadline);
        await input.waitFor({ timeout: 1000 }); await input.fill(value);
    }
    async function ready() { await page.waitForFunction(() => { const a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]; return Boolean(a?.reconciliationStarted && a.world?.isMapReady && a.canSendInput()); }, null, { timeout: 120000 }); }
    async function state() {
        return page.evaluate(async () => { const { session } = await System.import('chunks:///_virtual/GameSession.ts'), a = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
            const item = i => ({ id: Array.from(i.instanceId, x => x.toString(16).padStart(2, '0')).join(''), itemId: i.itemId, quantity: i.quantity.toString(), bound: i.bound, version: i.version.toString(), quality: i.quality, locked: i.locked, flags: i.recycleBlockFlags });
            const p = session.inventory.autoRecycle;
            return { binding: session.connection.binding.toString(), role: Array.from(session.role.key.roleId).join(','), rule: session.entry.resourceVersion.toString(), revision: session.inventory.revision.toString(), policy: p && { enabled: p.enabled, qualityIds: p.qualityIds, maxRequiredLevel: p.maxRequiredLevel, protectedItemIds: p.protectedItemIds, protectBound: p.protectBound, protectUnbound: p.protectUnbound, revision: p.revision.toString() },
                items: session.inventory.items.map(item), walletRevision: session.wallet.revision.toString(), wallet: { gold: session.wallet.gold.toString(), boundGold: session.wallet.boundGold.toString() },
                settingsText: cc.find('Canvas/Safe/UIMgr/Panels/Settings/Message').getComponent(cc.Label).string,
                reportText: cc.find('Canvas/Safe/UIMgr/Panels/Report/Message').getComponent(cc.Label).string,
                selected: a.selectedItem && Array.from(a.selectedItem.instanceId, x => x.toString(16).padStart(2, '0')).join('') };
        });
    }
    async function policyRevision(value) {
        const handle = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
        try { await page.waitForFunction(({ handle, value }) => handle.inventory?.autoRecycle?.revision.toString() === value && handle.wallet?.revision.equals(handle.inventory.revision), { handle, value }, { timeout: 15000 }); } finally { await handle.dispose(); }
    }
    async function toggle(name, value) { const checked = await page.evaluate(name => cc.find('Canvas/Safe/UIMgr/Panels/Settings/' + name).getComponent(cc.Toggle).isChecked, name); if (checked !== value) await tap('UIMgr/Panels/Settings/' + name); }
    async function confirm(expected) {
        await page.waitForFunction(() => cc.find('Canvas/Safe/PopupView/Panels/Confirm')?.activeInHierarchy, null, { timeout: 15000 });
        if (expected) {
            const message = await page.evaluate(() => cc.find('Canvas/Safe/PopupView/Panels/Confirm/Message').getComponent(cc.Label).string);
            for (const text of expected) assert(message.includes(text), '真实确认窗缺少预期：' + text + '\n' + message);
            report.observations.applyConfirmation = message; await snapshot('03-before-apply-confirm');
        }
        await tap('PopupView/Panels/Confirm/ConfirmButton');
    }
    async function closePanel() {
        const old = await page.evaluate(() => ({ popup: cc.find('Canvas/Safe/PopupView').getComponent('PopupView').top?.name ?? '', ui: cc.find('Canvas/Safe/UIMgr').getComponent('UIMgr').top?.name ?? '' }));
        await page.touchscreen.tap(10, 200);
        await page.waitForFunction(old => old.popup ? cc.find('Canvas/Safe/PopupView').getComponent('PopupView').top?.name !== old.popup
            : cc.find('Canvas/Safe/UIMgr').getComponent('UIMgr').top?.name !== old.ui, old, { timeout: 10000 });
        await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    }
    async function snapshot(name) { await page.screenshot({ path: path.join(out, name + '.png') }); }
    try {
        report.health = [];
        for (const port of [18100, 18101, 18102]) { const response = await fetch(`http://127.0.0.1:${port}/healthz`, { signal: AbortSignal.timeout(3000) }); const health = await response.json(); assert(response.ok && health.status === 'ok' && health.rule_version === 19 && health.development_only); assert.equal(health.fixture_hash, report.fixtureSha256); report.health.push({ port, ...health }); }
        await page.goto('http://127.0.0.1:18080/'); await scene('Login');
        const account = 'arqa' + crypto.randomBytes(5).toString('hex'), password = crypto.randomBytes(9).toString('hex'), name = '回收' + crypto.randomBytes(3).toString('hex');
        await edit('Account', account); await edit('Password', password); await tap('Register'); await scene('RoleSelect');
        const jobDeadline = Date.now() + 10000; let selectedJob = false, jobTaps = 0;
        do {
            selectedJob = await page.evaluate(() => cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.selectedJob === 2);
            if (selectedJob) break;
            await tap('JobTabs/Job2'); ++jobTaps; await new Promise(resolve => setTimeout(resolve, 100));
        } while (Date.now() < jobDeadline);
        assert(selectedJob, '正常职业按钮尚未选中道士，不创建替代职业'); report.observations.jobTaps = jobTaps;
        await edit('Name', name); await tap('Create');
        await page.waitForFunction(name => cc.find('Canvas/Safe/Role0')?.getComponentsInChildren(cc.Label).some(l => l.string.includes(name)), name, { timeout: 20000 });
        await tap('Role0'); await tap('Enter'); await scene('Game'); await ready();
        if (await page.evaluate(() => cc.find('Canvas/Safe/MapViewport/Actions/Auto').getComponent(cc.Toggle).isChecked)) await tap('MapViewport/Actions/Auto');
        const initial = report.observations.initial = await state(); assert.equal(initial.rule, '19'); assert.equal(initial.policy.enabled, false); assert.equal(initial.policy.revision, '0');
        report.checks.push('正常注册与创角，原初始物品，服务端自动回收默认关闭');
        await tap('BottomHUD/Footer/Inventory');
        const selected = initial.items.find(i => fixture.items.find(d => d.id === i.itemId)?.is_equipment); assert(selected);
        const index = initial.items.findIndex(i => i.id === selected.id);
        const point = await page.evaluate(index => { const list = cc.find('Canvas/Safe/UIMgr/Panels/Inventory/List').getComponent('List'), n = list.rows.get(index); if (!n?.activeInHierarchy) throw Error('所选初始装备未在可见列表'); const b = n._uiProps.uiTransformComp.getBoundingBoxToWorld(), v = cc.view.getVisibleSize(), r = document.querySelector('canvas').getBoundingClientRect(); return { x: r.x + (b.x + b.width / 2) / v.width * r.width, y: r.y + (v.height - b.y - b.height / 2) / v.height * r.height }; }, index);
        await page.touchscreen.tap(point.x, point.y); await page.waitForFunction(() => cc.find('Canvas/Safe/PopupView')?.getComponent('PopupView')?.top, null, { timeout: 5000 }); await closePanel();
        assert.equal((await state()).selected, selected.id); await tap('UIMgr/Panels/Inventory/ItemLock'); await confirm();
        const handle = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
        try { await page.waitForFunction(({ handle, id }) => handle.inventory.items.some(i => Array.from(i.instanceId, x => x.toString(16).padStart(2, '0')).join('') === id && i.locked), { handle, id: selected.id }, { timeout: 15000 }); } finally { await handle.dispose(); }
        report.observations.locked = await state(); await snapshot('01-locked-equipment'); report.checks.push('保存背包按钮锁定原实例，收到权威锁定与版本更新');
        await closePanel(); await tap('BottomHUD/Skills/Settings');
        await toggle('AutoRecycleEnabled', true); await toggle('RecycleQuality0', true); for (let i = 1; i <= 4; i++) await toggle('RecycleQuality' + i, false);
        await toggle('RecycleProtectBound', false); await toggle('RecycleProtectUnbound', false); await edit('UIMgr/Panels/Settings/RecycleMaxLevel', '0'); await edit('UIMgr/Panels/Settings/RecycleProtectedTypes', '');
        const beforeSave = await state(); await tap('UIMgr/Panels/Settings/RecycleSave'); await confirm(); await policyRevision('1');
        const future = report.observations.futureOnly = await state(); assert.deepEqual(future.items, beforeSave.items); assert.deepEqual(future.wallet, beforeSave.wallet); assert.equal(future.policy.enabled, true);
        await snapshot('02-future-only'); report.checks.push('仅保存未来策略不处理既有背包，不暗扣原物品');
        const recyclable = future.items.filter(i => i.id !== selected.id && fixture.items.find(d => d.id === i.itemId && d.is_equipment && d.drop_quality === i.quality && d.required_level != null));
        const expected = recyclable.reduce((sum, i) => { const def = fixture.items.find(d => d.id === i.itemId); sum[i.bound ? 'boundGold' : 'gold'] += BigInt(def.recycle_gold); return sum; }, { gold: 0n, boundGold: 0n }); assert(recyclable.length > 0);
        await tap('UIMgr/Panels/Settings/RecycleApply'); await confirm([`预计回收${recyclable.length}件`, `金币 ${expected.gold}`, `绑定金币 ${expected.boundGold}`]); await policyRevision('2');
        const applied = report.observations.applied = await state(); assert(applied.items.some(i => i.id === selected.id && i.locked)); for (const i of recyclable) assert(!applied.items.some(x => x.id === i.id));
        assert.equal(BigInt(applied.wallet.gold) - BigInt(future.wallet.gold), expected.gold); assert.equal(BigInt(applied.wallet.boundGold) - BigInt(future.wallet.boundGold), expected.boundGold);
        await page.waitForFunction(() => !cc.find('Canvas/Safe/PopupView/Panels/Confirm')?.activeInHierarchy, null, { timeout: 10000 });
        const deadline = Date.now() + 10000; let result;
        do { result = report.receipts.find(r => r.trigger === 2 && r.role_revision === applied.revision); if (result) break; await new Promise(resolve => setTimeout(resolve, 50)); } while (Date.now() < deadline);
        assert(result, '缺少原提交的明确回收结果'); assert.equal(result.recycled_count, String(recyclable.length)); assert.equal(result.gold, String(expected.gold)); assert.equal(result.bound_gold, String(expected.boundGold));
        await snapshot('03-applied'); report.checks.push('明确确认应用当前背包，锁定原件保留、其余原件按原Bound分币一次到账并有40018提交结果');
        await closePanel();
        await page.waitForFunction(() => ['Line0', 'Line1'].some(name => { const n = cc.find('Canvas/Safe/BottomHUD/Chat/' + name); return n?.activeInHierarchy && n.getComponent(cc.Label).string.includes('[背包回收]'); }), null, { timeout: 10000 });
        report.observations.visibleMessage = await page.evaluate(() => { const a = cc.director.getScene().getComponentsInChildren('AppRoot')[0]; return { entries: a.messages.log.messages.filter(m => m.title === '背包回收').map(m => m.message), labels: ['Line0', 'Line1'].map(name => cc.find('Canvas/Safe/BottomHUD/Chat/' + name).getComponent(cc.Label).string) }; });
        assert.equal(report.observations.visibleMessage.entries.length, 1); await snapshot('03-visible-result-message'); report.checks.push('真实消息Label显示一次已提交中文回收结果');
        const receiptCountBeforeReentry = report.receipts.length;
        await tap('BottomHUD/Footer/ChangeRole'); await scene('RoleSelect'); await tap('Role0'); await tap('Enter'); await scene('Game'); await ready();
        const reentry = report.observations.reentry = await state(); assert.notEqual(reentry.binding, applied.binding); assert.equal(reentry.role, applied.role); assert.deepEqual(reentry.policy, applied.policy); assert.deepEqual(reentry.items, applied.items); assert.deepEqual(reentry.wallet, applied.wallet); assert.equal(report.receipts.length, receiptCountBeforeReentry, '健康重入补播了历史回收通知');
        await tap('BottomHUD/Skills/Settings'); await snapshot('04-healthy-reentry'); report.checks.push('健康新票重入保持永久策略、锁定原件和已提交资产，没有重复回收');
        await page.setViewportSize({ width: 720, height: 1600 }); await page.waitForTimeout(150); await snapshot('05-taller-settings');
        for (const [file, expectedHash] of Object.entries(frozen)) assert.equal(hash(file), expectedHash, '浏览器运行期间项目输入变化：' + file);
        assert.equal(hash('tools/client/verify_auto_recycle_browser.cjs'), report.toolSha256);
        assert.equal(hash('config/development/first-loop.json'), report.fixtureSha256);
        assert.equal(hash('client/assets/resources/development/gameplay.json'), report.publicSha256);
        assert.equal(report.errors.length, 0, report.errors.join('\n')); report.passed = true;
    } catch (e) {
        report.failure = e.stack || e.message;
        const name = await page.evaluate(() => window.cc?.director.getScene()?.name).catch(() => '');
        if (['Game', 'RoleSelect'].includes(name)) await snapshot('failure');
        throw e;
    } finally {
        report.sourceAfter = Object.fromEntries(Object.keys(frozen).map(name => [name, fs.existsSync(path.join(root, name)) ? hash(name) : null]));
        report.toolSha256After = hash('tools/client/verify_auto_recycle_browser.cjs');
        fs.writeFileSync(path.join(out, 'report.json'), JSON.stringify(report, null, 2) + '\n'); await browser.close();
    }
    console.log(JSON.stringify({ passed: report.passed, checks: report.checks.length, errors: report.errors.length, warnings: report.warnings.length, report: path.relative(root, path.join(out, 'report.json')) }));
}
main().catch(e => { console.error(e.message); process.exitCode = 1; });
