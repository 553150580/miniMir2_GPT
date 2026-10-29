// 真实保存UI、正常新角色和原打怪奖励；不预置等级、经验、物品或临时战斗状态。
'use strict';
const assert = require('node:assert/strict'), crypto = require('node:crypto'), fs = require('node:fs'), path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const root = path.resolve(__dirname, '../..'), flag = process.argv.indexOf('--report-dir');
const out = path.resolve(root, flag < 0 ? '.work/level-growth-20261029/browser-main' : process.argv[flag + 1]);
assert(out.startsWith(path.join(root, '.work') + path.sep), '证据只写本项目隔离目录');
assert(!fs.existsSync(path.join(out, 'report.json')), '终态证据不可覆盖，请使用新目录');
fs.mkdirSync(out, { recursive: true });
const hash = file => crypto.createHash('sha256').update(fs.readFileSync(path.join(root, file))).digest('hex');
const build = JSON.parse(fs.readFileSync(path.join(root, 'docs/客户端构建检查.json'), 'utf8'));
assert(build.success && build.scene_prefab_layout_preserved && build.source_stable_during_build, '需要当前Creator实构');
const frozen = { ...build.asset_hashes, ...build.script_hashes };
for (const [file, expected] of Object.entries(frozen)) assert.equal(hash(file), expected, '构建输入已变化：' + file);
const fixture = JSON.parse(fs.readFileSync(path.join(root, 'config/development/first-loop.json'), 'utf8'));
const gameplay = JSON.parse(fs.readFileSync(path.join(root, 'client/assets/resources/development/gameplay.json'), 'utf8'));
assert.equal(fixture.rule_version, 20);
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const secrets = [];
const redact = value => secrets.reduce((text, secret) => text.replaceAll(secret, '[已隐去开发测试凭据]'), value);
function packet(value) {
    const b = Buffer.from(value); if (b.length < 6) return null; let offset = 4;
    const variable = () => { let n = 0n; for (let i = 0; i < 10; i++) { assert(offset < b.length); const x = b[offset++]; assert(i < 9 || x <= 1); n |= BigInt(x & 127) << BigInt(i * 7); if (!(x & 128)) return n; } throw Error('协议头超长'); };
    const request = variable().toString(), binding = variable().toString(), error = b[1] & 1 ? Number(variable()) : 0;
    return { id: b.readUInt16BE(2), request, binding, error, body: b.subarray(offset) };
}
function interactive(name) { const scene = window.cc?.director?.getScene(); return Boolean(scene?.name === name && scene.getComponentsInChildren('AppRoot').length && cc.find('Canvas/Safe')?.activeInHierarchy && !cc.find('Canvas/Loading')?.activeInHierarchy); }
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
async function main() {
    const browser = await chromium.launch({ channel: 'chrome', headless: true, args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: { width: 720, height: 1280 }, hasTouch: true, isMobile: true, deviceScaleFactor: 1 });
    const page = await context.newPage(); let observed;
    const report = { passed: false, startedAt: new Date().toISOString(), scope: '主rule20正常注册新道士，自然打怪到7级学原初始书并跨21到22；保存成长详情/Long分账/健康重入。本脚本以普通触屏用药、白名单拆掉落书、保存背包同源书页合并管理格数，并非产品新增自动整理。50级池及历史迁移由独立服务端边界夹具验证，本工具不证明自然1至50周期或真机/容量', checks: [], errors: [], warnings: [], receipts: [], observations: {}, samples: [], operations: [], cleanups: [], skillQuotes: [], pageMerges: [], sourceBefore: frozen, fixtureSha256: hash('config/development/first-loop.json'), publicSha256: hash('client/assets/resources/development/gameplay.json'), toolSha256: hash('tools/client/verify_level_growth_browser.cjs'), seeded: false, privateBusinessInvoked: false, physicalMobileVerified: false };
    page.on('pageerror', e => report.errors.push(e.stack || e.message));
    page.on('console', m => { if (['warning', 'error'].includes(m.type())) report.warnings.push({ type: m.type(), text: m.text().slice(0, 600) }); });
    page.on('websocket', socket => {
        socket.on('framesent', e => { const p = packet(e.payload); if (p && [30002, 30004, 40001, 40009, 40010, 40011, 40013, 40016].includes(p.id)) report.operations.push({ time: Date.now(), id: p.id, binding: p.binding, request: p.request }); });
        socket.on('framereceived', e => { try {
            const p = packet(e.payload); if (!p) return;
            if (p.error) { report.errors.push('游戏协议拒绝：' + p.id + '/' + p.error); return; }
            if (p.id === 40012) {
                const q = pb.SkillAssetPreview.decode(p.body), item = x => ({ id: Buffer.from(x.instance_id).toString('hex'), item: x.item_id, quantity: x.quantity.toString(), version: x.version.toString(), bound: x.bound, quality: x.quality, sourceKind: x.source_kind, sourceId: x.source_id, locked: x.locked, flags: x.recycle_block_flags });
                report.skillQuotes.push({ time: Date.now(), kind: q.kind, rule: q.rule_version.toString(), revision: q.role_revision.toString(), preview: Buffer.from(q.preview_id).toString('hex'), spend: { gold: q.spend.gold.toString(), boundGold: q.spend.bound_gold.toString(), yuanbaoUnits: q.spend.yuanbao_units.toString(), boundYuanbaoUnits: q.spend.bound_yuanbao_units.toString() }, materials: q.materials.map(x => ({ ...item(x.item), consume: x.consume_quantity.toString() })), outputs: q.outputs.map(item) });
            }
            if (p.id !== 40019) return;
            const q = pb.ExperienceGrantResult.decode(p.body), state = q.state;
            assert.equal(q.rule_version.toString(), '20'); assert.equal(q.curve_version, fixture.progression.curve_version);
            assert.equal(q.raw_experience.toString(), (BigInt(q.accepted_experience.toString()) + BigInt(q.rejected_experience.toString())).toString());
            assert(BigInt(q.pooled_experience.toString()) <= BigInt(q.accepted_experience.toString()));
            report.receipts.push({ time: Date.now(), binding: p.binding, revision: q.role_revision.toString(), source: q.source_seq.toString(), raw: q.raw_experience.toString(), accepted: q.accepted_experience.toString(), pooled: q.pooled_experience.toString(), rejected: q.rejected_experience.toString(), before: q.level_before, after: q.level_after, migrated: q.migrated, cap: state.level_cap, curve: q.curve_version });
        } catch (e) { report.errors.push('协议观察：' + e.message); } });
    });
    async function scene(name) { await page.waitForFunction(interactive, name, { timeout: 90000 }); await frames(); }
    async function frames() { await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))); }
    async function tap(suffix) {
        await page.waitForFunction(suffix => { const n = cc.find('Canvas/Safe/' + suffix), a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0], c = n?.getComponent(cc.Button) ?? n?.getComponent(cc.Toggle); return Boolean(n?.activeInHierarchy && (!c || c.interactable && c.enabledInHierarchy) && (suffix.startsWith('PopupView/') || !a?.uiActionBusy)); }, suffix, { timeout: 15000 });
        const p = await page.evaluate(suffix => { const n = cc.find('Canvas/Safe/' + suffix), b = n._uiProps.uiTransformComp.getBoundingBoxToWorld(), v = cc.view.getVisibleSize(), r = document.querySelector('canvas').getBoundingClientRect(); return { x: r.x + (b.x + b.width / 2) / v.width * r.width, y: r.y + (v.height - b.y - b.height / 2) / v.height * r.height }; }, suffix);
        await page.touchscreen.tap(p.x, p.y); await frames();
    }
    async function edit(suffix, value) { const input = page.locator('input:visible'), deadline = Date.now() + 10000; do { await tap(suffix); if (await input.isVisible()) break; await sleep(100); } while (Date.now() < deadline); await input.waitFor({ timeout: 1000 }); await input.fill(value); }
    async function ready() { await observed?.dispose(); observed = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session); await page.waitForFunction(s => { const a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]; return Boolean(s.ready && s.inventory?.progression && a?.world?.isMapReady && a.reconciliationStarted && a.canSendInput()); }, observed, { timeout: 120000 }); }
    async function state() {
        return page.evaluate(s => { const a = cc.director.getScene().getComponentsInChildren('AppRoot')[0], p = s.baseline?.entities.find(e => e.entityHandle === s.entry?.playerHandle), growth = s.inventory?.progression, long = x => x?.toString();
            const item = i => ({ id: Array.from(i.instanceId, x => x.toString(16).padStart(2, '0')).join(''), item: i.itemId, quantity: long(i.quantity), version: long(i.version), bound: i.bound, quality: i.quality, sourceKind: i.sourceKind, sourceId: i.sourceId, locked: i.locked, flags: i.recycleBlockFlags });
            return { time: Date.now(), binding: long(s.connection.binding), role: s.role.name, roleKey: long(s.role.key.originServerId) + ':' + Array.from(s.role.key.roleId, x => x.toString(16).padStart(2, '0')).join(''), scene: long(s.entry.sceneEpoch), baseline: long(s.entry.baselineId), map: long(s.entry.roomId), rule: long(s.entry.resourceVersion), revision: long(s.inventory.revision), wallet: s.wallet && { revision: long(s.wallet.revision), gold: long(s.wallet.gold), boundGold: long(s.wallet.boundGold), yuanbaoUnits: long(s.wallet.yuanbaoUnits), boundYuanbaoUnits: long(s.wallet.boundYuanbaoUnits) }, level: s.inventory.level, experience: long(s.inventory.experience), growth: growth && { format: growth.formatVersion, catalog: growth.catalogMaxLevel, curve: growth.curveVersion, cap: growth.levelCap, next: long(growth.nextLevelExperience), pool: long(growth.pendingExperience), poolCap: long(growth.pendingExperienceLimit), legacy: long(growth.legacyExperienceOverflow), migration: growth.migrationRequired, rebirth: growth.rebirthCount }, items: s.inventory.items.map(item), equipped: s.inventory.equipped.map(x => ({ slot: x.slot, ...item(x.item) })), skills: s.inventory.learnedSkills.map(x => ({ id: x.skillId, level: x.level })), hp: long(p?.hp), maxHp: long(p?.hpMax), mp: long(p?.mp), maxMp: long(p?.mpMax), life: long(p?.life), auto: a.auto, count: a.count, ui: a.ui?.top?.name ?? '', popup: a.popup?.top?.name ?? '', hud: cc.find('Canvas/Safe/BottomHUD/ExperienceText').getComponent(cc.Label).string, notice: s.experienceNotice, detail: a.popup?.top?.name === 'StatusDetails' ? cc.find('Canvas/Safe/PopupView/Panels/StatusDetails/List').getComponent('List').items : [], error: s.lastError, messages: a.messages.log.messages.filter(x => /经验|升级/.test(x.title)).map(x => ({ title: x.title, message: x.message })) };
        }, observed);
    }
    async function closePanel() { const old = await state(); if (!old.popup && !old.ui) return; await page.touchscreen.tap(10, 200); await page.waitForFunction(old => { const a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]; return old.popup ? a?.popup?.top?.name !== old.popup : a?.ui?.top?.name !== old.ui; }, old, { timeout: 10000 }); await frames(); }
    async function closeAll() { for (let n = 0; n < 5; n++) { const s = await state(); if (!s.popup && !s.ui) return; await closePanel(); } throw Error('保存面板未返回'); }
    async function confirm() { await page.waitForFunction(() => cc.find('Canvas/Safe/PopupView/Panels/Confirm')?.activeInHierarchy, null, { timeout: 10000 }); await tap('PopupView/Panels/Confirm/ConfirmButton'); await page.waitForFunction(() => !cc.find('Canvas/Safe/PopupView/Panels/Confirm')?.activeInHierarchy, null, { timeout: 15000 }); }
    async function screenshot(name) { await page.screenshot({ path: path.join(out, name + '.png') }); }
    async function selectItem(itemId) {
        const p = await page.evaluate(itemId => { const list = cc.find('Canvas/Safe/UIMgr/Panels/Inventory/List').getComponent('List'), index = list.items.findIndex(i => i.itemId === itemId), n = list.rows.get(index); if (!n?.activeInHierarchy) throw Error('初始装备未在保存背包可见行'); const b = n._uiProps.uiTransformComp.getBoundingBoxToWorld(), v = cc.view.getVisibleSize(), r = document.querySelector('canvas').getBoundingClientRect(); return { x: r.x + (b.x + b.width / 2) / v.width * r.width, y: r.y + (v.height - b.y - b.height / 2) / v.height * r.height }; }, itemId);
        await page.touchscreen.tap(p.x, p.y); await page.waitForFunction(() => cc.find('Canvas/Safe/PopupView/Panels/Details')?.activeInHierarchy); await closePanel();
    }
    async function auto(enabled) { const value = await state(); if (value.auto !== enabled) { await tap('MapViewport/Actions/Auto'); await page.waitForFunction(({ s, enabled }) => { const a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]; return a?.auto === enabled && !a.uiActionBusy; }, { s: observed, enabled }); } }
    async function toggle(name, value) { const actual = await page.evaluate(name => cc.find('Canvas/Safe/UIMgr/Panels/Settings/' + name).getComponent(cc.Toggle).isChecked, name); if (actual !== value) await tap('UIMgr/Panels/Settings/' + name); }
    async function idle() { await page.waitForFunction(() => { const a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]; return Boolean(a && !a.uiActionBusy && !a.economicBusy && !a.unresolved); }, null, { timeout: 15000 }); await frames(); }
    async function swipeList(suffix, reverse = false) {
        // getBoundingBoxToWorld包含激活子树，不能用它代表Mask自身裁剪矩形。
        const p = await page.evaluate(suffix => { const t = cc.find('Canvas/Safe/' + suffix).getComponent('List').viewport._uiProps.uiTransformComp, v = cc.view.getVisibleSize(), r = document.querySelector('canvas').getBoundingClientRect(), point = fraction => { const w = t.convertToWorldSpaceAR(new cc.Vec3((.5 - t.anchorPoint.x) * t.width, (fraction - t.anchorPoint.y) * t.height, 0)); return { x: r.x + w.x / v.width * r.width, y: r.y + (v.height - w.y) / v.height * r.height }; }; return { start: point(.2), end: point(.8) }; }, suffix);
        if (reverse) [p.start, p.end] = [p.end, p.start];
        const cdp = await context.newCDPSession(page);
        await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [p.start] });
        for (let i = 1; i <= 8; i++) { await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: p.start.x + (p.end.x - p.start.x) * i / 8, y: p.start.y + (p.end.y - p.start.y) * i / 8 }] }); await sleep(25); }
        await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] }); await cdp.detach(); await sleep(300);
    }
    async function selectVisibleBook(ids, audit) {
        const point = await page.evaluate(ids => {
            const list = cc.find('Canvas/Safe/UIMgr/Panels/SkillAssets/MaterialsList').getComponent('List'), view = list.viewport._uiProps.uiTransformComp, anchor = view.anchorPoint, minX = -anchor.x * view.width + 8, maxX = (1 - anchor.x) * view.width - 8, minY = -anchor.y * view.height + 8, maxY = (1 - anchor.y) * view.height - 8;
            for (const [index, row] of list.rows) {
                const item = list.items[index], id = Array.from(item.instanceId, x => x.toString(16).padStart(2, '0')).join('');
                if (!ids.includes(id) || !row.activeInHierarchy) continue;
                const plus = row.getChildByName('Plus'), button = plus.getComponent(cc.Button), t = plus._uiProps.uiTransformComp, corners = [0, 1].flatMap(x => [0, 1].map(y => view.convertToNodeSpaceAR(t.convertToWorldSpaceAR(new cc.Vec3((x - t.anchorPoint.x) * t.width, (y - t.anchorPoint.y) * t.height, 0)))));
                const w = t.convertToWorldSpaceAR(new cc.Vec3((.5 - t.anchorPoint.x) * t.width, (.5 - t.anchorPoint.y) * t.height, 0)), center = view.convertToNodeSpaceAR(w);
                if (!button.interactable || !corners.every(p => p.x >= minX - 8 && p.x <= maxX + 8 && p.y >= minY - 8 && p.y <= maxY + 8) || center.x < minX || center.x > maxX || center.y < minY || center.y > maxY) continue;
                const v = cc.view.getVisibleSize(), r = document.querySelector('canvas').getBoundingClientRect(); return { id, item: item.itemId, row: index, mask: { minX, maxX, minY, maxY }, center: { x: center.x, y: center.y }, corners: corners.map(p => ({ x: p.x, y: p.y })), x: r.x + w.x / v.width * r.width, y: r.y + (v.height - w.y) / v.height * r.height };
            }
            return null;
        }, ids);
        if (!point) return null;
        const sample = { ...point, time: Date.now(), selected: false }; audit.push(sample);
        await page.touchscreen.tap(point.x, point.y); await frames();
        await page.waitForFunction(id => { const list = cc.find('Canvas/Safe/UIMgr/Panels/SkillAssets/MaterialsList').getComponent('List'); return Array.from(list.rows).some(([index, row]) => Array.from(list.items[index].instanceId, x => x.toString(16).padStart(2, '0')).join('') === id && row.getChildByName('Quantity').getComponent(cc.Label).string === '1'); }, point.id, { timeout: 5000 });
        sample.selected = true; return point.id;
    }
    async function readFullQuote(record) {
        const text = await page.evaluate(() => { const n = cc.find('Canvas/Safe/PopupView/Panels/SkillQuote'); return { title: n.getChildByName('Title').getComponent(cc.Label).string, message: n.getChildByName('Message').getComponent(cc.Label).string, confirm: n.getChildByName('ConfirmButton').getComponentsInChildren(cc.Label).map(x => x.string), cancel: n.getChildByName('CancelButton').getComponentsInChildren(cc.Label).map(x => x.string), lines: n.getChildByName('List').getComponent('List').items.map(String) }; });
        record.confirmationText = { title: text.title, message: text.message, confirm: text.confirm, cancel: text.cancel }; record.quoteLines = text.lines; record.quoteScroll = [];
        for (let i = 0; i < 32; i++) {
            const scroll = await page.evaluate(() => { const view = cc.find('Canvas/Safe/PopupView/Panels/SkillQuote/List').getComponent('List').scrollView; return { offset: view.getScrollOffset().y, max: view.getMaxScrollOffset().y }; });
            record.quoteScroll.push(scroll); if (scroll.offset >= scroll.max - 2) break;
            await swipeList('PopupView/Panels/SkillQuote/List');
        }
        const last = record.quoteScroll.at(-1); assert(last.offset >= last.max - 2, '普通触屏尚未滚读完整资产报价');
    }
    async function selectInventoryInstance(ids, audit) {
        const suffix = 'UIMgr/Panels/Inventory/List';
        // 保存List保留用户滚动；先以真实反向滑动回顶，再找原实例，不调用scrollTo接口。
        for (let i = 0; i < 32; i++) { const y = await page.evaluate(() => cc.find('Canvas/Safe/UIMgr/Panels/Inventory/List').getComponent('List').scrollView.getScrollOffset().y); if (y <= 2) break; await swipeList(suffix, true); }
        for (let swipe = 0; swipe <= 32; swipe++) {
            const point = await page.evaluate(ids => {
                const list = cc.find('Canvas/Safe/UIMgr/Panels/Inventory/List').getComponent('List'), view = list.viewport._uiProps.uiTransformComp;
                for (const [index, row] of list.rows) {
                    const item = list.items[index], id = Array.from(item.instanceId, x => x.toString(16).padStart(2, '0')).join(''); if (!ids.includes(id) || !row.activeInHierarchy) continue;
                    const t = row._uiProps.uiTransformComp, world = t.convertToWorldSpaceAR(new cc.Vec3((.5 - t.anchorPoint.x) * t.width, (.5 - t.anchorPoint.y) * t.height, 0)), local = view.convertToNodeSpaceAR(world);
                    if (local.x < -view.anchorPoint.x * view.width + 8 || local.x > (1 - view.anchorPoint.x) * view.width - 8 || local.y < -view.anchorPoint.y * view.height + 8 || local.y > (1 - view.anchorPoint.y) * view.height - 8) continue;
                    const v = cc.view.getVisibleSize(), r = document.querySelector('canvas').getBoundingClientRect(); return { id, row: index, local: { x: local.x, y: local.y }, x: r.x + world.x / v.width * r.width, y: r.y + (v.height - world.y) / v.height * r.height };
                }
                return null;
            }, ids);
            if (point) {
                audit.push({ ...point, time: Date.now() }); await page.touchscreen.tap(point.x, point.y); await frames();
                await page.waitForFunction(id => { const a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]; return a?.popup?.top?.name === 'Details' && Array.from(a.selectedItem?.instanceId ?? [], b => b.toString(16).padStart(2, '0')).join('') === id; }, point.id, { timeout: 10000 });
                await closePanel(); return point.id;
            }
            await swipeList(suffix);
        }
        throw Error('普通触屏未找到原同源书页的可见背包行');
    }
    const pageSourceKey = item => JSON.stringify([item.item, item.bound, item.quality, item.sourceKind, item.sourceId]);
    const isPage = item => gameplay.items.some(d => d.item_id === item.item && d.kind === 'skill_page');
    function pageSourceTotals(items) {
        const sums = new Map(); for (const item of items.filter(isPage)) { const key = pageSourceKey(item); sums.set(key, (sums.get(key) ?? 0n) + BigInt(item.quantity)); }
        return Object.fromEntries(Array.from(sums, ([key, quantity]) => [key, quantity.toString()]).sort(([a], [b]) => a.localeCompare(b)));
    }
    async function mergePagesNormally(preferredIds) {
        await closeAll(); await idle(); const before = await state(), groups = new Map();
        for (const item of before.items) {
            if (!isPage(item) || item.locked !== false || item.flags !== 0 || !['skill_recycle', 'npc_shop', 'monster_drop'].includes(item.sourceKind) || !item.sourceId || BigInt(item.quantity) <= 0n) continue;
            const key = pageSourceKey(item); if (!groups.has(key)) groups.set(key, []); groups.get(key).push(item);
        }
        const eligible = Array.from(groups.values()).filter(items => items.length >= 2), group = eligible.find(items => items.some(x => preferredIds.includes(x.id))) ?? eligible[0];
        assert(group, '正常拆书后未观察到至少两叠同源无保护书页');
        const record = { time: Date.now(), before, expectedMaterials: group, selectionTouches: [], quote: null, after: null, completed: false }; report.pageMerges.push(record);
        await tap('BottomHUD/Footer/Inventory'); record.selectedId = await selectInventoryInstance(group.map(x => x.id), record.selectionTouches);
        record.contextButtonText = await page.evaluate(() => cc.find('Canvas/Safe/UIMgr/Panels/Inventory/ItemLock/Label').getComponent(cc.Label).string); assert.equal(record.contextButtonText, '合并书页');
        const quoteCount = report.skillQuotes.length; await tap('UIMgr/Panels/Inventory/ItemLock'); await page.waitForFunction(() => cc.find('Canvas/Safe/PopupView/Panels/SkillQuote')?.activeInHierarchy, null, { timeout: 15000 });
        assert.equal(report.skillQuotes.length, quoteCount + 1); const quote = record.quote = report.skillQuotes.at(-1); assert.equal(quote.kind, 4); assert.equal(quote.rule, '20'); assert.equal(quote.revision, before.revision);
        assert.deepEqual(quote.materials.map(x => x.id).sort(), group.map(x => x.id).sort());
        for (const item of quote.materials) { const original = group.find(x => x.id === item.id); assert.equal(item.version, original.version); assert.equal(item.consume, original.quantity); assert.equal(pageSourceKey(item), pageSourceKey(original)); }
        assert(quote.outputs.length > 0 && quote.outputs.length < quote.materials.length); assert(quote.outputs.every(x => pageSourceKey(x) === pageSourceKey(group[0]) && x.locked === false && x.flags === 0 && BigInt(x.quantity) > 0n));
        assert.equal(quote.outputs.reduce((n, x) => n + BigInt(x.quantity), 0n), group.reduce((n, x) => n + BigInt(x.quantity), 0n)); assert(Object.values(quote.spend).every(x => x === '0'));
        await readFullQuote(record); assert.equal(record.confirmationText.title, '合并确认'); await screenshot('page-merge-' + report.pageMerges.length + '-quote');
        record.confirmedAt = Date.now(); await tap('PopupView/Panels/SkillQuote/ConfirmButton'); await idle();
        await page.waitForFunction(({ s, ids }) => ids.every(id => !s.inventory.items.some(x => Array.from(x.instanceId, b => b.toString(16).padStart(2, '0')).join('') === id)), { s: observed, ids: group.map(x => x.id) }, { timeout: 15000 });
        record.after = await state(); const after = record.after; for (const field of ['binding', 'roleKey', 'scene', 'baseline', 'map', 'rule']) assert.equal(after[field], before[field]); assert(BigInt(after.revision) > BigInt(before.revision));
        for (const field of ['gold', 'boundGold', 'yuanbaoUnits', 'boundYuanbaoUnits']) assert.equal(after.wallet[field], before.wallet[field]);
        const beforeIds = new Set(before.items.map(x => x.id)), afterIds = new Set(after.items.map(x => x.id));
        record.removedIds = before.items.filter(x => !afterIds.has(x.id)).map(x => x.id); record.newIds = after.items.filter(x => !beforeIds.has(x.id)).map(x => x.id);
        assert.deepEqual(record.removedIds.slice().sort(), group.map(x => x.id).sort());
        record.actualOutputs = after.items.filter(x => record.newIds.includes(x.id)); assert.deepEqual(record.actualOutputs.map(x => [pageSourceKey(x), x.quantity]).sort(), quote.outputs.map(x => [pageSourceKey(x), x.quantity]).sort());
        record.sourceTotalsBefore = pageSourceTotals(before.items); record.sourceTotalsAfter = pageSourceTotals(after.items); assert.deepEqual(record.sourceTotalsAfter, record.sourceTotalsBefore);
        record.slotDifference = before.items.length - after.items.length; assert.equal(record.slotDifference, quote.materials.length - quote.outputs.length); assert(record.slotDifference > 0);
        record.cumulativeSlotDifference = report.pageMerges.reduce((n, x) => n + (x.slotDifference ?? 0), 0); record.completed = true;
        await closeAll(); await screenshot('page-merge-' + report.pageMerges.length + '-completed'); return report.pageMerges.length - 1;
    }
    function recyclableDropBooks(stock) {
        const counts = new Map(); for (const item of stock.items) counts.set(item.item, (counts.get(item.item) ?? 0) + 1);
        const chosen = [];
        for (const item of stock.items) {
            const definition = gameplay.items.find(d => d.item_id === item.item), notOwnJob = definition?.kind === 'book' && Array.isArray(definition.allowed_jobs) && !definition.allowed_jobs.includes(2);
            const allowed = item.quantity === '1' && !item.locked && !item.flags && item.sourceKind === 'monster_drop' && item.sourceId && gameplay.skill_book_recycle.some(r => r.item_id === item.item && r.book_quality === item.quality && r.allowed_source_kinds.includes(item.sourceKind));
            if (allowed && (notOwnJob || (counts.get(item.item) ?? 0) > 1)) { chosen.push(item); counts.set(item.item, counts.get(item.item) - 1); }
            if (chosen.length === 12) break;
        }
        return chosen;
    }
    async function clearBagNormally(earlyBookProbe = false) {
        await closeAll(); await auto(false); await idle();
        let quiet = Date.now(), lastReceipt = report.receipts.length; const quietEnd = Date.now() + 10000;
        while (Date.now() - quiet < 1000) { assert(Date.now() < quietEnd, '普通背包处理前经验结算未稳定'); await sleep(100); if (report.receipts.length !== lastReceipt) { quiet = Date.now(); lastReceipt = report.receipts.length; } }
        const before = await state(), cleanup = { time: Date.now(), earlyBookProbe, before, potions: [], books: [], quote: null, after: null };
        report.cleanups.push(cleanup);
        // 满值用药仍走产品现有浪费确认，不伪造补给或调整角色属性。
        for (let i = 0; i < 6; i++) {
            const old = await state(); if (old.items.length <= 24) break;
            const itemId = old.items.some(x => x.item === 35) ? 35 : old.items.some(x => x.item === 36) ? 36 : 0; if (!itemId) break;
            const quantity = old.items.filter(x => x.item === itemId).reduce((n, x) => n + BigInt(x.quantity), 0n), full = itemId === 35 ? old.hp === old.maxHp : old.mp === old.maxMp;
            await tap('BottomHUD/Skills/' + (itemId === 35 ? 'RedPotion' : 'BluePotion')); await idleUnlessConfirm();
            const confirmation = (await state()).popup === 'Confirm'; if (confirmation) await confirm(); await idle();
            await page.waitForFunction(({ s, itemId, quantity }) => s.inventory.items.filter(x => x.itemId === itemId).reduce((n, x) => n + BigInt(x.quantity.toString()), 0n) < BigInt(quantity), { s: observed, itemId, quantity: quantity.toString() }, { timeout: 15000 });
            cleanup.potions.push({ item: itemId, fullAtSelection: full, confirmation, quantityBefore: quantity.toString(), revisionAfter: (await state()).revision }); await sleep(350);
        }
        const stock = await state();
        if (stock.items.length > 24 || earlyBookProbe) {
            const chosen = recyclableDropBooks(stock);
            if (chosen.length >= 2) {
                await tap('BottomHUD/Skills/SkillsPanel'); await tap('UIMgr/Panels/Skills/Salvage');
                const remaining = new Set(chosen.map(x => x.id)), selected = []; cleanup.selectionTouches = [];
                for (let swipe = 0; swipe <= 16 && selected.length < 2; swipe++) {
                    let id; while (selected.length < 2 && (id = await selectVisibleBook(Array.from(remaining), cleanup.selectionTouches))) { selected.push(id); remaining.delete(id); }
                    if (selected.length < 2) await swipeList('UIMgr/Panels/SkillAssets/MaterialsList');
                }
                assert.equal(selected.length, 2, '普通触屏未能选择两本重复掉落书');
                const quoteCount = report.skillQuotes.length; await tap('UIMgr/Panels/SkillAssets/Preview');
                await page.waitForFunction(() => cc.find('Canvas/Safe/PopupView/Panels/SkillQuote')?.activeInHierarchy, null, { timeout: 15000 });
                assert.equal(report.skillQuotes.length, quoteCount + 1); const quote = report.skillQuotes.at(-1); assert.equal(quote.kind, 3); assert.equal(quote.rule, '20');
                assert.deepEqual(quote.materials.map(x => x.id).sort(), selected.slice().sort()); assert(quote.materials.every(x => x.consume === '1' && x.sourceKind === 'monster_drop'));
                assert(quote.outputs.length > 0 && quote.outputs.every(x => gameplay.items.find(d => d.item_id === x.item)?.kind === 'skill_page' && BigInt(x.quantity) > 0n));
                cleanup.quote = quote; cleanup.books = chosen.filter(x => selected.includes(x.id));
                await readFullQuote(cleanup); assert.equal(cleanup.confirmationText.title, '拆书确认');
                await screenshot('cleanup-' + report.cleanups.length + '-book-quote'); await tap('PopupView/Panels/SkillQuote/ConfirmButton'); await idle();
                await page.waitForFunction(({ s, ids }) => ids.every(id => !s.inventory.items.some(x => Array.from(x.instanceId, b => b.toString(16).padStart(2, '0')).join('') === id)), { s: observed, ids: selected }, { timeout: 15000 });
                await closeAll();
                cleanup.afterBook = await state();
                const oldIds = new Set(stock.items.map(x => x.id)), pageIds = cleanup.afterBook.items.filter(x => isPage(x) && !oldIds.has(x.id)).map(x => x.id);
                cleanup.pageMergeIndex = await mergePagesNormally(pageIds);
            }
        }
        cleanup.after = await state(); assert(cleanup.after.items.length < before.items.length && cleanup.after.items.length < 32, '普通用药或拆书后合并未释放背包空间');
        if (cleanup.quote) for (const output of cleanup.quote.outputs) assert(cleanup.after.items.some(x => x.item === output.item && x.bound === output.bound && BigInt(x.quantity) >= BigInt(output.quantity)), '权威背包未观察到报价书页产物');
        await screenshot('cleanup-' + report.cleanups.length + '-completed'); await auto(true);
    }
    async function idleUnlessConfirm() { await page.waitForFunction(() => { const a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]; return Boolean(a && (a.popup?.top?.name === 'Confirm' || !a.uiActionBusy)); }, null, { timeout: 10000 }); }
    async function grow(target, budget) {
        const end = Date.now() + budget; let last = 0;
        while (Date.now() < end) {
            const s = await state(); assert.equal(s.rule, '20'); assert(!s.error, s.error); assert.equal(report.errors.length, 0, report.errors.join('\n'));
            if (Date.now() - last > 20000) { last = Date.now(); report.samples.push(s); fs.writeFileSync(path.join(out, 'progress.json'), JSON.stringify({ target, last: s, receipts: report.receipts.length }, null, 2)); console.log(JSON.stringify({ phase: 'natural_combat', target, level: s.level, experience: s.experience, receipts: report.receipts.length })); }
            if (s.level >= target) return s;
            const earlyBookProbe = !report.cleanups.some(x => x.quote) && recyclableDropBooks(s).length >= 2;
            if (s.items.length >= 28 || earlyBookProbe) { await clearBagNormally(earlyBookProbe); continue; }
            if (s.hp === '0') { await closeAll(); await tap('MapViewport/Actions/Attack'); await confirm(); report.operations.push({ kind: 'normal_revive', time: Date.now() }); await sleep(300); }
            else if (BigInt(s.hp) * 4n < BigInt(s.maxHp) && s.items.some(i => i.item === 35)) { await closeAll(); await tap('BottomHUD/Skills/RedPotion'); if ((await state()).popup === 'Confirm') await confirm(); }
            else if (s.skills.some(x => x.id === 2) && BigInt(s.mp) * 4n < BigInt(s.maxMp) && s.items.some(i => i.item === 36)) { await closeAll(); await tap('BottomHUD/Skills/BluePotion'); if ((await state()).popup === 'Confirm') await confirm(); }
            await sleep(1000);
        }
        throw Error('正常打怪未在观察预算内达到' + target + '级；保留全部真实进度，不预置角色补验');
    }
    try {
        report.health = [];
        for (const port of [18100, 18101, 18102]) { const response = await fetch(`http://127.0.0.1:${port}/healthz`, { signal: AbortSignal.timeout(5000) }), q = await response.json(); assert(response.ok && q.status === 'ok' && q.development_only && q.rule_version === 20); assert.equal(q.fixture_hash, report.fixtureSha256); report.health.push({ port, ...q }); }
        await page.goto('http://127.0.0.1:18080/'); await scene('Login');
        const account = 'lgqa' + crypto.randomBytes(5).toString('hex'), password = crypto.randomBytes(9).toString('hex'); secrets.push(account, password);
        await edit('Account', account); await edit('Password', password); await tap('Register'); await scene('RoleSelect');
        const name = '成长' + crypto.randomBytes(3).toString('hex'), deadline = Date.now() + 10000;
        while (!await page.evaluate(() => cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.selectedJob === 2)) { assert(Date.now() < deadline, '道士按钮未选中'); await tap('JobTabs/Job2'); await sleep(100); }
        await edit('Name', name); await tap('Create'); await page.waitForFunction(name => cc.find('Canvas/Safe/Role0')?.getComponentsInChildren(cc.Label).some(l => l.string.includes(name)), name, { timeout: 20000 });
        await tap('Role0'); await tap('Enter'); await scene('Game'); await ready(); await auto(false);
        const initial = report.observations.initial = await state(); assert.equal(initial.level, 1); assert.equal(initial.experience, '0'); assert.equal(initial.growth.format, 1); assert.equal(initial.growth.catalog, 50); assert.equal(initial.growth.cap, 50); assert.equal(initial.growth.pool, '0'); assert.equal(initial.growth.legacy, '0'); assert.equal(initial.growth.rebirth, 0); report.checks.push('正常注册新道士为1级0经验，格式1/50级目录，不预置转生或余额');
        await tap('BottomHUD/Footer/Inventory');
        for (const item of [218, 193]) { await selectItem(item); await tap('UIMgr/Panels/Inventory/Equip'); await page.waitForFunction(({ s, item }) => s.inventory.equipped.some(x => x.item.itemId === item), { s: observed, item }, { timeout: 15000 }); }
        await closeAll(); report.observations.equipped = await state(); report.checks.push('1级正常角色按保存背包按钮合法穿戴原需求1级木剑和布衣；不足等级拒绝由独立服务端边界验证');
        await tap('BottomHUD/Skills/Settings'); await toggle('AutoRecycleEnabled', true); await toggle('RecycleQuality0', true); for (let i = 1; i <= 4; i++) await toggle('RecycleQuality' + i, false); await toggle('RecycleProtectBound', false); await toggle('RecycleProtectUnbound', false); await tap('UIMgr/Panels/Settings/RecycleSave'); await confirm(); await closeAll();
        for (let i = 0; (await state()).count !== 8; i++) { assert(i < 4, '正常怪物数量按钮未推进到8'); await tap('MapViewport/Actions/Count'); } await auto(true);
        report.observations.level7 = await grow(7, 12 * 60 * 1000); await auto(false); await screenshot('01-natural-level7');
        assert(report.receipts.some(r => r.before < 7 && r.after >= 7 && !r.migrated)); report.checks.push('正常8怪自动战斗逐次真实40019奖励升到7级，非旧余额迁移');
        await tap('BottomHUD/Skills/SkillsPanel'); await page.waitForFunction(() => cc.find('Canvas/Safe/UIMgr/Panels/Skills/Learn').getComponent(cc.Button).interactable); await tap('UIMgr/Panels/Skills/Learn'); await confirm(); await page.waitForFunction(s => s.inventory.learnedSkills.some(x => x.skillId === 2), observed, { timeout: 15000 });
        report.observations.learned = await state(); assert(!report.observations.learned.items.some(i => i.item === 21 && initial.items.some(x => x.id === i.id))); await screenshot('02-normal-book-learn'); report.checks.push('自然7级后保存学习入口消费原初始治愈术书，权威已学列表确认'); await closeAll();
        await tap('TopHUD/Avatar'); await tap('UIMgr/Panels/Role/Message');
        const detail = report.observations.detail = await state(); assert(detail.detail.join('\n').includes('当前上限 50')); assert(detail.detail.join('\n').includes('待用经验')); assert(detail.detail.join('\n').includes('转生尚未开放')); await screenshot('03-growth-details');
        // 普通触摸拖动保存ScrollView，读取偏移仅作观察。
        const point = await page.evaluate(() => { const n = cc.find('Canvas/Safe/PopupView/Panels/StatusDetails/List'), b = n._uiProps.uiTransformComp.getBoundingBoxToWorld(), v = cc.view.getVisibleSize(), r = document.querySelector('canvas').getBoundingClientRect(); return { x: r.x + (b.x + b.width / 2) / v.width * r.width, y: r.y + (v.height - b.y - b.height / 2) / v.height * r.height }; });
        const offset = () => page.evaluate(() => cc.find('Canvas/Safe/PopupView/Panels/StatusDetails/List').getComponent('List').scrollView.getScrollOffset().y);
        const scrollBefore = await offset(), cdp = await context.newCDPSession(page); await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: point.x, y: point.y + 100 }] }); for (let i = 1; i <= 8; i++) { await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: point.x, y: point.y + 100 - i * 35 }] }); await sleep(25); } await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] }); await sleep(400); await cdp.detach(); const scrollAfter = await offset(); assert(scrollAfter > scrollBefore + 10, '正常触屏拖动没有推进保存ScrollView'); report.observations.detailScroll = { before: scrollBefore, after: scrollAfter }; await screenshot('04-growth-details-scrolled'); report.checks.push('正常触屏打开已有成长详情，拖动确实推进内置ScrollView，明确上限/待用池/封存/转生关闭'); await closeAll();
        await auto(true); report.observations.level22 = await grow(22, 40 * 60 * 1000); await auto(false);
        let quietSince = Date.now(), count = report.receipts.length; const quietDeadline = Date.now() + 15000;
        while (Date.now() - quietSince < 1500) { assert(Date.now() < quietDeadline, '停止自动后奖励仍未稳定'); await sleep(100); if (report.receipts.length !== count) { quietSince = Date.now(); count = report.receipts.length; } }
        const final = report.observations.final = await state(); assert(final.level >= 22); assert.equal(final.growth.next, String(fixture.progression.exp_per_level[final.level - 1])); assert.equal(final.growth.pool, '0'); assert.equal(final.growth.legacy, '0'); assert(report.receipts.some(r => r.before === 21 && r.after >= 22)); assert(report.receipts.every(r => r.rejected === '0' && r.pooled === '0' && !r.migrated));
        const earned = report.receipts.filter(r => BigInt(r.revision) <= BigInt(final.revision)).reduce((sum, r) => sum + BigInt(r.accepted), 0n), spent = fixture.progression.exp_per_level.slice(0, final.level - 1).reduce((sum, n) => sum + BigInt(n), 0n); assert.equal(earned, spent + BigInt(final.experience)); report.observations.earned = earned.toString(); await screenshot('05-natural-cross21'); report.checks.push('正常新角色持续真实战斗跨越旧21级断点到22，原奖励=实际入账+拒收，所有消费门槛与当前余数守恒');
        assert(report.pageMerges.length > 0 && report.pageMerges.every(x => x.completed && x.slotDifference > 0), '自然成长验收必须观察真实同源书页合并减格');
        report.totalMergedSlotsFreed = report.pageMerges.reduce((n, x) => n + x.slotDifference, 0);
        report.checks.push('普通触屏拆书后选原页、查看完整合并报价并提交，原来源分账守恒且实际释放' + report.totalMergedSlotsFreed + '格；不是新增自动清包产品功能');
        await tap('TopHUD/Avatar'); await tap('UIMgr/Panels/Role/Message'); await screenshot('06-level22-details'); await closeAll();
        let before = await state(), stableSince = Date.now(), receiptCount = report.receipts.length; const stableDeadline = Date.now() + 15000;
        while (Date.now() - stableSince < 1500) { assert(Date.now() < stableDeadline, '换角前成长结算仍未稳定'); await sleep(100); const s = await state(); if (s.revision !== before.revision || s.experience !== before.experience || report.receipts.length !== receiptCount) { stableSince = Date.now(); before = s; receiptCount = report.receipts.length; } }
        before = await state(); assert(BigInt(before.hp) > 0n, '健康重入验收必须从存活角色开始'); report.observations.beforeReentry = before;
        await tap('BottomHUD/Footer/ChangeRole'); await scene('RoleSelect'); await tap('Role0'); await tap('Enter'); await scene('Game'); await ready();
        const reentry = report.observations.reentry = await state(); assert.notEqual(reentry.binding, before.binding); assert.equal(reentry.life, before.life); assert.equal(reentry.level, before.level); assert.equal(reentry.experience, before.experience); assert.deepEqual(reentry.growth, before.growth); assert.equal(report.receipts.length, receiptCount); assert.equal(reentry.notice, null); await screenshot('07-healthy-reentry'); report.checks.push('正常换角新票保持成长与原Life，Inventory静默恢复不重播历史40019通知');
        report.sourceAfter = Object.fromEntries(Object.keys(frozen).map(file => [file, hash(file)])); assert.deepEqual(report.sourceAfter, frozen, '验收期间构建输入变化'); report.toolSha256After = hash('tools/client/verify_level_growth_browser.cjs'); assert.equal(report.toolSha256After, report.toolSha256);
        report.healthAfter = [];
        for (const before of report.health) { const response = await fetch(`http://127.0.0.1:${before.port}/healthz`, { signal: AbortSignal.timeout(5000) }), q = await response.json(); assert(response.ok && q.status === 'ok' && q.instance === before.instance && q.rule_version === 20 && q.fixture_hash === report.fixtureSha256); report.healthAfter.push({ port: before.port, ...q }); }
        assert.equal(report.errors.length, 0, report.errors.join('\n')); report.passed = true;
    } catch (e) { report.error = e.stack || String(e); try { if (await page.evaluate(() => ['Game', 'RoleSelect'].includes(window.cc?.director?.getScene()?.name))) await screenshot('failure'); } catch {} throw e; }
    finally { report.finishedAt = new Date().toISOString(); await observed?.dispose(); fs.writeFileSync(path.join(out, 'report.json'), redact(JSON.stringify(report, null, 2)) + '\n'); await browser.close(); }
    console.log(JSON.stringify({ passed: report.passed, checks: report.checks.length, errors: report.errors.length, warnings: report.warnings.length, receipts: report.receipts.length }));
}
main().catch(e => { console.error(redact(e.stack || e.message)); process.exitCode = 1; });
