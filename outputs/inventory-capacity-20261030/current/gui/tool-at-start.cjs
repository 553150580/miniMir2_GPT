// 普通满包持续战斗与保存背包触屏；正常新角色，不预置等级、物品、余额或临时状态。
'use strict';
const assert = require('node:assert/strict'), crypto = require('node:crypto'), fs = require('node:fs'), path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const root = path.resolve(__dirname, '../..'), flag = process.argv.indexOf('--report-dir');
const out = path.resolve(root, flag < 0 ? '.work/inventory-capacity-20261030/browser-main' : process.argv[flag + 1]);
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
assert.equal(fixture.rule_version, 21);
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
    const page = await context.newPage(); let observed, expectedRoleKey = null;
    const report = { passed: false, expectedRuleVersion: 21, startedAt: new Date().toISOString(),
        scope: '正常新道士原补给/装备/Auto8自然满包，真实弃物后继续原经验与Auto，保存背包Mask/末行/即时上下文及Auto保持On跨健康票；没有GUI离线扣时、真机或容量认证。',
        checks: [], errors: [], warnings: [], operations: [], receipts: [], recycleResults: [], hostingStates: [], entries: [], inventoryBaselines: [], skillQuotes: [], observations: {}, samples: [],
        sourceBefore: frozen, creatorReportSha256: hash('docs/客户端构建检查.json'), fixtureSha256: hash('config/development/first-loop.json'), publicSha256: hash('client/assets/resources/development/gameplay.json'),
        toolSha256: hash('tools/client/verify_inventory_capacity_browser.cjs'), touchInput: true, routeOverride: false, bodySeeded: false, seeded: false, privateBusinessInvoked: false, physicalMobileVerified: false, offlineChargingVerifiedByThisGui: false };
    const long = x => x.toString(), roleKey = x => long(x.origin_server_id) + ':' + Buffer.from(x.role_id).toString('hex');
    const rawItem = x => ({ id: Buffer.from(x.instance_id).toString('hex'), item: x.item_id, quantity: long(x.quantity), version: long(x.version), bound: x.bound, quality: x.quality, sourceKind: x.source_kind, sourceId: x.source_id, locked: x.locked, flags: x.recycle_block_flags });
    page.on('pageerror', e => report.errors.push(e.stack || e.message));
    page.on('console', m => { if (['warning', 'error'].includes(m.type())) report.warnings.push({ type: m.type(), text: m.text().slice(0, 600) }); });
    page.on('websocket', socket => {
        socket.on('framesent', event => { try {
            const p = packet(event.payload); if (!p || ![30004, 40001, 40009, 40010, 40011, 40013].includes(p.id)) return;
            const operation = { time: Date.now(), id: p.id, binding: p.binding, request: p.request };
            if (p.id === 30004) { const q = pb.AutoBattleIntent.decode(p.body); operation.enabled = q.enabled; operation.monsterCount = q.monster_count; }
            if (p.id === 40013) operation.preview = Buffer.from(pb.SkillAssetCommitRequest.decode(p.body).preview_id).toString('hex');
            report.operations.push(operation);
        } catch (error) { report.errors.push('发送协议观察：' + error.message); } });
        socket.on('framereceived', event => { try {
            const p = packet(event.payload); if (!p) return; if (p.error) { report.errors.push('游戏协议拒绝：' + p.id + '/' + p.error); return; }
            const base = { time: Date.now(), binding: p.binding };
            if (p.id === 20002) { const q = pb.EnterLineResult.decode(p.body); assert.equal(long(q.resource_version), '21'); report.entries.push({ ...base, scene: long(q.scene_epoch), baseline: long(q.baseline_id), map: long(q.room_id), resourceVersion: long(q.resource_version) }); }
            if (p.id === 40003 && !report.inventoryBaselines.some(x => x.binding === p.binding)) { const q = pb.InventoryResult.decode(p.body); report.inventoryBaselines.push({ ...base, revision: long(q.revision), itemCount: q.items.length }); }
            if (p.id === 30005) { const q = pb.HostingState.decode(p.body); report.hostingStates.push({ ...base, auto: q.auto_battle_enabled, remain: long(q.remain_hosting_sec) }); }
            if (p.id === 40012) { const q = pb.SkillAssetPreview.decode(p.body); report.skillQuotes.push({ ...base, kind: q.kind, rule: long(q.rule_version), revision: long(q.role_revision), preview: Buffer.from(q.preview_id).toString('hex'), materials: q.materials.map(x => ({ ...rawItem(x.item), consume: long(x.consume_quantity) })), outputs: q.outputs.map(rawItem) }); }
            if (p.id === 40018) {
                const q = pb.EquipmentRecycleResult.decode(p.body), role = roleKey(q.role); if (expectedRoleKey) assert.equal(role, expectedRoleKey);
                const value = { ...base, role, revision: long(q.role_revision), source: long(q.source_seq), op: Buffer.from(q.op_id).toString('hex'), policyRevision: long(q.policy_revision), count: long(q.recycled_count), gold: long(q.gold), boundGold: long(q.bound_gold), discarded: long(q.discarded_count), reason: q.discard_reason, trigger: q.trigger };
                for (const name of ['revision', 'source', 'policyRevision', 'count', 'gold', 'boundGold', 'discarded']) assert(BigInt(value[name]) >= 0n);
                if (value.trigger === 1) { assert(BigInt(value.source) > 0n); assert.equal(value.op, ''); }
                report.recycleResults.push(value);
            }
            if (p.id === 40019) {
                const q = pb.ExperienceGrantResult.decode(p.body), role = roleKey(q.role); if (expectedRoleKey) assert.equal(role, expectedRoleKey);
                assert.equal(long(q.rule_version), '21'); assert.equal(q.curve_version, fixture.progression.curve_version);
                const value = { ...base, role, revision: long(q.role_revision), source: long(q.source_seq), rule: long(q.rule_version), curve: q.curve_version, raw: long(q.raw_experience), accepted: long(q.accepted_experience), pooled: long(q.pooled_experience), rejected: long(q.rejected_experience), before: q.level_before, after: q.level_after, migrated: q.migrated };
                assert.equal(BigInt(value.raw), BigInt(value.accepted) + BigInt(value.rejected)); assert(BigInt(value.pooled) <= BigInt(value.accepted)); report.receipts.push(value);
            }
        } catch (error) { report.errors.push('接收协议观察：' + error.message); } });
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
        const suffix = 'UIMgr/Panels/Inventory/List'; let targetId = null, attempts = 0;
        // 保存List保留用户滚动；先以真实反向滑动回顶，再找原实例，不调用scrollTo接口。
        for (let i = 0; i < 32; i++) { const y = await page.evaluate(() => cc.find('Canvas/Safe/UIMgr/Panels/Inventory/List').getComponent('List').scrollView.getScrollOffset().y); if (y <= 2) break; await swipeList(suffix, true); }
        for (let swipe = 0; swipe <= 64; swipe++) {
            const point = await page.evaluate(ids => {
                const list = cc.find('Canvas/Safe/UIMgr/Panels/Inventory/List').getComponent('List'), view = list.viewport._uiProps.uiTransformComp;
                for (const [index, row] of list.rows) {
                    const item = list.items[index], id = Array.from(item.instanceId, x => x.toString(16).padStart(2, '0')).join(''); if (!ids.includes(id) || !row.activeInHierarchy) continue;
                    const t = row._uiProps.uiTransformComp;
                    // 保存的按钮可覆盖列表下缘；只选择原行内部、Mask内且未被其它活跃Button覆盖的点。
                    for (const fy of [.5, .75, .25]) for (const fx of [.25, .5, .75]) {
                        const world = t.convertToWorldSpaceAR(new cc.Vec3((fx - t.anchorPoint.x) * t.width, (fy - t.anchorPoint.y) * t.height, 0)), local = view.convertToNodeSpaceAR(world), rowLocal = t.convertToNodeSpaceAR(world);
                        const rowHit = rowLocal.x >= -t.anchorPoint.x * t.width + 8 && rowLocal.x <= (1 - t.anchorPoint.x) * t.width - 8 && rowLocal.y >= -t.anchorPoint.y * t.height + 8 && rowLocal.y <= (1 - t.anchorPoint.y) * t.height - 8;
                        if (!rowHit) continue;
                        if (local.x < -view.anchorPoint.x * view.width + 8 || local.x > (1 - view.anchorPoint.x) * view.width - 8 || local.y < -view.anchorPoint.y * view.height + 8 || local.y > (1 - view.anchorPoint.y) * view.height - 8) continue;
                        const controls = cc.find('Canvas/Safe/UIMgr/Panels/Inventory').getComponentsInChildren(cc.Button).flatMap(button => { const node = button.node, ui = node?._uiProps.uiTransformComp; if (!node?.activeInHierarchy || !ui) return []; for (let ancestor = node; ancestor; ancestor = ancestor.parent) if (ancestor === row) return []; const point = ui.convertToNodeSpaceAR(world); return [{ name: node.name, active: true, local: { x: point.x, y: point.y }, width: ui.width, height: ui.height, anchor: { x: ui.anchorPoint.x, y: ui.anchorPoint.y }, contains: point.x >= -ui.anchorPoint.x * ui.width - 2 && point.x <= (1 - ui.anchorPoint.x) * ui.width + 2 && point.y >= -ui.anchorPoint.y * ui.height - 2 && point.y <= (1 - ui.anchorPoint.y) * ui.height + 2 }]; });
                        if (controls.some(x => x.contains)) continue;
                        const v = cc.view.getVisibleSize(), r = document.querySelector('canvas').getBoundingClientRect(); return { id, row: index, fraction: { x: fx, y: fy }, rowHit, rowLocal: { x: rowLocal.x, y: rowLocal.y }, local: { x: local.x, y: local.y }, controls, x: r.x + world.x / v.width * r.width, y: r.y + (v.height - world.y) / v.height * r.height };
                    }
                }
                return null;
            }, targetId ? [targetId] : ids);
            if (point) {
                targetId ??= point.id; attempts++; const sample = { ...point, time: Date.now(), attempt: attempts }; audit.push(sample);
                await page.touchscreen.tap(point.x, point.y); await frames();
                try {
                    await page.waitForFunction(id => { const a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]; return a?.popup?.top?.name === 'Details' && Array.from(a.selectedItem?.instanceId ?? [], b => b.toString(16).padStart(2, '0')).join('') === id; }, targetId, { timeout: 2000 });
                    sample.actual = { popup: 'Details', selectedId: targetId }; await closePanel(); return targetId;
                } catch (error) {
                    if (error.name !== 'TimeoutError') throw error;
                    sample.actual = await page.evaluate(() => { const a = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]; return { popup: a?.popup?.top?.name ?? '', selectedId: Array.from(a?.selectedItem?.instanceId ?? [], b => b.toString(16).padStart(2, '0')).join('') }; });
                    const current = await state(); assert(!current.error && report.errors.length === 0, current.error || report.errors.join('\n'));
                    assert(!sample.actual.popup || sample.actual.popup === 'Details', '普通选取误开了非详情面板，停止且不确认');
                    if (sample.actual.popup === 'Details') await closePanel();
                    assert(attempts < 2, '同一原书页两次普通触屏仍未被选中，保留实际选择并停止');
                    await sleep(350); continue;
                }
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

    async function state() {
        return page.evaluate(s => {
            const a = cc.director.getScene().getComponentsInChildren('AppRoot')[0], player = s.baseline?.entities.find(e => e.entityHandle === s.entry.playerHandle), str = x => x?.toString();
            const item = x => ({ id: Array.from(x.instanceId, b => b.toString(16).padStart(2, '0')).join(''), item: x.itemId, quantity: str(x.quantity), version: str(x.version), bound: x.bound, quality: x.quality, sourceKind: x.sourceKind, sourceId: x.sourceId, locked: x.locked, flags: x.recycleBlockFlags });
            return { time: Date.now(), role: s.role.name, job: s.role.job, roleKey: str(s.role.key.originServerId) + ':' + Array.from(s.role.key.roleId, b => b.toString(16).padStart(2, '0')).join(''), binding: str(s.connection.binding), scene: str(s.entry.sceneEpoch), baseline: str(s.entry.baselineId), map: str(s.entry.roomId), rule: str(s.entry.resourceVersion),
                revision: str(s.inventory.revision), level: s.inventory.level, experience: str(s.inventory.experience), items: s.inventory.items.map(item), equipped: s.inventory.equipped.map(x => ({ slot: x.slot, ...item(x.item) })), skills: s.inventory.learnedSkills.map(x => ({ id: x.skillId, level: x.level })),
                auto: a.auto, count: a.count, hosting: { auto: s.hosting.autoBattleEnabled, remain: str(s.hosting.remainHostingSec) }, autoRecycleEnabled: s.inventory.autoRecycle?.enabled,
                hp: str(player?.hp), maxHp: str(player?.hpMax), mp: str(player?.mp), maxMp: str(player?.mpMax), life: str(player?.life), wallet: s.wallet && { revision: str(s.wallet.revision), gold: str(s.wallet.gold), boundGold: str(s.wallet.boundGold), yuanbaoUnits: str(s.wallet.yuanbaoUnits), boundYuanbaoUnits: str(s.wallet.boundYuanbaoUnits) }, recycleNotice: s.recycleNotice, experienceNotice: s.experienceNotice,
                ui: a.ui?.top?.name ?? '', popup: a.popup?.top?.name ?? '', error: s.lastError, messages: a.messages.log.messages.map(x => ({ id: x.id, title: x.title, message: x.message })) };
        }, observed);
    }
    const bagFullEvents = () => Array.from(new Map(report.recycleResults.filter(x => x.trigger === 1 && x.reason === 'bag_full' && BigInt(x.discarded) > 0n).map(x => [x.role + ':' + x.source, x])).values());
    async function progress(phase) { const s = await state(), value = { phase, last: s, experience_receipts: report.receipts.length, recycle_results: report.recycleResults.length, bag_full_events: bagFullEvents().length, firstBagFull: bagFullEvents()[0] ?? null }; report.samples.push(value); fs.writeFileSync(path.join(out, 'progress.json'), redact(JSON.stringify(value, null, 2)) + '\n'); console.log(JSON.stringify({ phase, level: s.level, experience: s.experience, items: s.items.length, auto: s.auto, experience_receipts: value.experience_receipts, bag_full_events: value.bag_full_events })); return s; }
    async function waitUntil(phase, budget, predicate) {
        const end = Date.now() + budget; let sampled = 0;
        while (Date.now() < end) { const s = await state(); assert.equal(s.rule, '21'); assert(!s.error, s.error); assert.equal(report.errors.length, 0, report.errors.join('\n')); assert(BigInt(s.hp) > 0n, '本验收要求正常存活，角色倒下时保留实际现场停止'); if (Date.now() - sampled >= 20000) { sampled = Date.now(); await progress(phase); } if (await predicate(s)) return s; await sleep(500); }
        throw Error(phase + '未在正常观察预算内完成；不预置角色补验');
    }
    async function learnOriginalBook() { await auto(false); await tap('BottomHUD/Skills/SkillsPanel'); await tap('UIMgr/Panels/Skills/Learn'); await confirm(); await page.waitForFunction(s => s.inventory.learnedSkills.some(x => x.skillId === 2), observed, { timeout: 15000 }); await idle(); report.observations.learned = await state(); await screenshot('02-normal-book-learn'); await closeAll(); await auto(true); }
    async function geometry() {
        return page.evaluate(() => {
            const panel = cc.find('Canvas/Safe/UIMgr/Panels/Inventory'), parent = panel._uiProps.uiTransformComp, list = panel.getChildByName('List').getComponent('List');
            const rect = node => { const t = node._uiProps.uiTransformComp, points = [0, 1].flatMap(x => [0, 1].map(y => parent.convertToNodeSpaceAR(t.convertToWorldSpaceAR(new cc.Vec3((x - t.anchorPoint.x) * t.width, (y - t.anchorPoint.y) * t.height, 0))))); return { left: Math.min(...points.map(p => p.x)), right: Math.max(...points.map(p => p.x)), bottom: Math.min(...points.map(p => p.y)), top: Math.max(...points.map(p => p.y)), width: t.width, height: t.height }; };
            return { window: { width: innerWidth, height: innerHeight }, panel: { width: parent.width, height: parent.height }, view: rect(list.viewport), itemLock: rect(panel.getChildByName('ItemLock')), equip: rect(panel.getChildByName('Equip')), recycle: rect(panel.getChildByName('Recycle')), rowHeight: list.rowHeight, offset: list.scrollView.getScrollOffset().y };
        });
    }
    function checkGeometry(g) { assert.equal(g.panel.width, 660); assert.equal(g.panel.height, 820); assert.equal(g.view.width, 600); assert.equal(g.view.height, 414); assert.equal(g.rowHeight, 72); assert(Math.abs(g.view.bottom - g.itemLock.top - 12) < .1); for (const b of [g.itemLock, g.equip, g.recycle]) assert(Math.min(g.view.right, b.right) <= Math.max(g.view.left, b.left) || Math.min(g.view.top, b.top) <= Math.max(g.view.bottom, b.bottom)); }
    async function installSelectionObserver() {
        await page.evaluate(() => {
            const list = cc.find('Canvas/Safe/UIMgr/Panels/Inventory/List').getComponent('List'), a = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
            window.__inventoryCapacitySelections = [];
            list.node.on(cc.Node.EventType.TOUCH_END, () => { window.__inventoryCapacityTouchBefore = { time: Date.now(), offset: list.scrollView.getScrollOffset().y }; }, undefined, true);
            list.node.on('list-item-click', (_index, item) => { const button = cc.find('Canvas/Safe/UIMgr/Panels/Inventory/ItemLock'); window.__inventoryCapacitySelections.push({ time: Date.now(), id: Array.from(item.instanceId, b => b.toString(16).padStart(2, '0')).join(''), item: item.itemId, label: button.getChildByName('Label').getComponent(cc.Label).string, interactable: button.getComponent(cc.Button).interactable, touchBefore: window.__inventoryCapacityTouchBefore, offset: list.scrollView.getScrollOffset().y, selectedId: Array.from(a.selectedItem?.instanceId ?? [], b => b.toString(16).padStart(2, '0')).join('') }); });
        });
    }
    async function selectAndObserve(id) {
        const touches = []; const index = await page.evaluate(() => window.__inventoryCapacitySelections.length); await selectInventoryInstance([id], touches);
        const events = await page.evaluate(start => window.__inventoryCapacitySelections.slice(start), index), event = events.find(x => x.id === id && x.selectedId === id); assert(event, '没有真实原行选择事件');
        assert(event.touchBefore && event.time - event.touchBefore.time < 1000, '选择缺少同一次普通触屏捕获证据'); assert(Math.abs(event.offset - event.touchBefore.offset) < .1, '同一选择回调改变了列表偏移');
        const offsetAfter = await page.evaluate(() => cc.find('Canvas/Safe/UIMgr/Panels/Inventory/List').getComponent('List').scrollView.getScrollOffset().y); assert(Math.abs(offsetAfter - event.offset) < 1, '详情返回改变原ScrollView偏移');
        return { touches, sameEvent: event, offsetAfter };
    }
    async function probeSavedInventory() {
        await auto(false); await idle(); await sleep(1200); await tap('BottomHUD/Footer/Inventory'); await installSelectionObserver();
        const initial = report.observations.geometry = { initial: await geometry() }; checkGeometry(initial.initial);
        const stock = await state(), last = stock.items.at(-1), choice = report.observations.lastRowSelection = await selectAndObserve(last.id); choice.item = last;
        const definition = gameplay.items.find(x => x.item_id === last.item), expected = definition?.kind === 'skill_page' ? '合并书页' : last.locked ? '解锁装备' : '锁定装备'; assert.equal(choice.sameEvent.label, expected); await screenshot('05-last-row-details-return');
        await page.setViewportSize({ width: 720, height: 1600 }); await frames(); initial.tall = await geometry(); checkGeometry(initial.tall); await screenshot('06-tall-inventory');
        await page.setViewportSize({ width: 960, height: 1280 }); await frames(); initial.wide = await geometry(); checkGeometry(initial.wide); await screenshot('06-wide-inventory');
        await page.setViewportSize({ width: 720, height: 1280 }); await frames(); initial.restored = await geometry(); checkGeometry(initial.restored); await closeAll();
        const before = await state(), candidates = recyclableDropBooks(before); assert(candidates.length >= 2, '正常满包未获得两本可拆的非本职或重复原书，不制造材料');
        const probe = report.observations.bookProbe = { before, selectionTouches: [] }; await tap('BottomHUD/Skills/SkillsPanel'); await tap('UIMgr/Panels/Skills/Salvage'); const remaining = new Set(candidates.map(x => x.id)), selected = [];
        for (let swipe = 0; swipe <= 16 && selected.length < 2; swipe++) { let id; while (selected.length < 2 && (id = await selectVisibleBook(Array.from(remaining), probe.selectionTouches))) { selected.push(id); remaining.delete(id); } if (selected.length < 2) await swipeList('UIMgr/Panels/SkillAssets/MaterialsList'); }
        assert.equal(selected.length, 2); const count = report.skillQuotes.length; await tap('UIMgr/Panels/SkillAssets/Preview'); await page.waitForFunction(() => cc.find('Canvas/Safe/PopupView/Panels/SkillQuote')?.activeInHierarchy); assert.equal(report.skillQuotes.length, count + 1); const quote = probe.quote = report.skillQuotes.at(-1); assert.equal(quote.kind, 3); assert.deepEqual(quote.materials.map(x => x.id).sort(), selected.slice().sort()); await readFullQuote(probe); await screenshot('07-book-quote'); await tap('PopupView/Panels/SkillQuote/ConfirmButton'); await idle();
        await page.waitForFunction(({ s, ids }) => ids.every(id => !s.inventory.items.some(x => Array.from(x.instanceId, b => b.toString(16).padStart(2, '0')).join('') === id)), { s: observed, ids: selected }, { timeout: 15000 }); await closeAll(); probe.after = await state();
        const oldIds = new Set(before.items.map(x => x.id)), pages = probe.after.items.filter(x => isPage(x) && !oldIds.has(x.id)); assert(pages.length >= 2); assert.equal(probe.after.items.length, 32);
        await tap('BottomHUD/Footer/Inventory'); const pageSelection = report.observations.pageSelection = await selectAndObserve(pages[0].id); pageSelection.item = pages[0]; assert.equal(pageSelection.sameEvent.label, '合并书页'); assert.equal(pageSelection.sameEvent.interactable, true);
        const quotes = report.skillQuotes.length, commits = report.operations.filter(x => x.id === 40013).length; await tap('UIMgr/Panels/Inventory/ItemLock'); await page.waitForFunction(() => cc.find('Canvas/Safe/PopupView/Panels/SkillQuote')?.activeInHierarchy); assert.equal(report.skillQuotes.length, quotes + 1); const merge = report.observations.mergeQuoteCancelled = { quote: report.skillQuotes.at(-1) }; assert.equal(merge.quote.kind, 4); await readFullQuote(merge); assert.equal(merge.confirmationText.title, '合并确认'); await screenshot('08-immediate-merge-quote'); await tap('PopupView/Panels/SkillQuote/CancelButton'); await idle(); await closeAll(); merge.after = await state(); assert.equal(merge.after.items.length, 32); assert.equal(report.operations.filter(x => x.id === 40013).length, commits, '取消不得提交合并经济操作');
    }
    try {
        report.health = []; for (const port of [18100, 18101, 18102]) { const response = await fetch(`http://127.0.0.1:${port}/healthz`, { signal: AbortSignal.timeout(5000) }), q = await response.json(); assert(response.ok && q.status === 'ok' && q.development_only && q.rule_version === 21 && q.fixture_hash === report.fixtureSha256); report.health.push({ port, ...q }); }
        await page.goto('http://127.0.0.1:18080/'); await scene('Login'); const account = 'icqa' + crypto.randomBytes(5).toString('hex'), password = crypto.randomBytes(9).toString('hex'); secrets.push(account, password); await edit('Account', account); await edit('Password', password); await tap('Register'); await scene('RoleSelect');
        const name = '容量' + crypto.randomBytes(3).toString('hex'), end = Date.now() + 10000; while (!await page.evaluate(() => cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.selectedJob === 2)) { assert(Date.now() < end); await tap('JobTabs/Job2'); await sleep(100); } await edit('Name', name); await tap('Create'); await page.waitForFunction(name => cc.find('Canvas/Safe/Role0')?.getComponentsInChildren(cc.Label).some(x => x.string.includes(name)), name); await tap('Role0'); await tap('Enter'); await scene('Game'); await ready(); await auto(false);
        const initial = report.observations.initial = await state(); expectedRoleKey = initial.roleKey; assert.equal(initial.job, 2); assert.equal(initial.level, 1); assert.equal(initial.experience, '0'); assert.equal(initial.hosting.remain, '0'); assert.equal(initial.autoRecycleEnabled, false);
        await tap('BottomHUD/Footer/Inventory'); for (const item of [218, 193]) { await selectItem(item); await tap('UIMgr/Panels/Inventory/Equip'); await page.waitForFunction(({ s, item }) => s.inventory.equipped.some(x => x.item.itemId === item), { s: observed, item }); } await closeAll(); for (let i = 0; (await state()).count !== 8; i++) { assert(i < 4); await tap('MapViewport/Actions/Count'); } await auto(true); await screenshot('01-equipped-normal-auto'); report.checks.push('正常注册1级道士，原木剑布衣保存按钮穿戴；默认回收关闭/初始托管0，普通Auto8开始自然战斗');
        await waitUntil('natural_to_level7', 12 * 60 * 1000, s => s.level >= 7); await learnOriginalBook(); report.checks.push('自然达到7级后正常原书学习治愈术，再普通开启Auto；没有预置等级或材料');
        const full = report.observations.firstBagFull = await waitUntil('natural_to_first_bag_full', 25 * 60 * 1000, s => bagFullEvents().length > 0 && s.items.length === 32); assert(full.auto && full.hosting.auto); report.observations.firstDiscard = bagFullEvents()[0]; await screenshot('03-first-bag-full');
        const firstSource = BigInt(report.observations.firstDiscard.source), receiptStart = report.receipts.length, acceptedStart = report.receipts.reduce((n, x) => n + BigInt(x.accepted), 0n);
        const continued = report.observations.afterFullContinued = await waitUntil('full_bag_auto_continues', 180000, s => bagFullEvents().filter(x => BigInt(x.source) > firstSource).length >= 3 && report.receipts.slice(receiptStart).some(x => BigInt(x.source) > firstSource && BigInt(x.accepted) > 0n) && s.auto && s.hosting.auto && s.items.length === 32);
        report.observations.afterFullProof = { firstSource: firstSource.toString(), laterDiscards: bagFullEvents().filter(x => BigInt(x.source) > firstSource), acceptedBefore: acceptedStart.toString(), acceptedAfter: report.receipts.reduce((n, x) => n + BigInt(x.accepted), 0n).toString(), receiptsBefore: receiptStart, receiptsAfter: report.receipts.length }; assert(BigInt(report.observations.afterFullProof.acceptedAfter) > acceptedStart); await screenshot('04-full-bag-continued'); report.checks.push('原40018 bag_full之后仍32格且权威Auto开启，后续多次实际弃物与40019经验接收继续；不从背包或钱包差猜收益');
        await probeSavedInventory(); report.checks.push('实际View与固定按钮AR矩形不交并保留12逻辑像素；末行普通触屏选择同事件更新上下文、详情返回不丢offset，720×1600和960×1280仍固定尺寸'); report.checks.push('正常白名单拆两本掉落书后真实选原页，当次事件立即显示可用合并书页；完整报价正常取消且不发40013');
        report.observations.autoOff = await state(); assert(!report.observations.autoOff.auto && !report.observations.autoOff.hosting.auto); await auto(true); const onSource = bagFullEvents().at(-1).source, onReceipts = report.receipts.length;
        report.observations.autoOn = await waitUntil('full_bag_reenable_auto', 180000, s => s.auto && s.hosting.auto && s.items.length === 32 && bagFullEvents().some(x => BigInt(x.source) > BigInt(onSource)) && report.receipts.length > onReceipts); report.checks.push('仍满包时真实关闭/开启Auto均按用户动作确认，重开后继续新奖励和弃物，没有容量禁战');
        const before = report.observations.beforeReentry = await state(); await tap('BottomHUD/Footer/ChangeRole'); await scene('RoleSelect'); await tap('Role0'); await tap('Enter'); await scene('Game'); await ready(); const reentry = report.observations.reentry = await state();
        assert.notEqual(reentry.binding, before.binding); for (const field of ['roleKey', 'life', 'scene', 'map']) assert.equal(reentry[field], before[field]); assert(reentry.auto && reentry.hosting.auto); assert.equal(reentry.items.length, 32);
        const inventory = report.inventoryBaselines.find(x => x.binding === reentry.binding); assert(inventory); const watermark = BigInt(inventory.revision); report.observations.reentryInitialInventory = inventory;
        for (const event of [...report.recycleResults, ...report.receipts].filter(x => x.binding === reentry.binding)) assert(BigInt(event.revision) > watermark, '新票重播了首Inventory水位内的旧提交');
        for (const notice of [reentry.recycleNotice, reentry.experienceNotice]) if (notice) assert(BigInt(notice.revision) > watermark, '新票UI误报旧提交');
        const after = report.observations.reentryContinued = await waitUntil('healthy_ticket_auto_stays_on', 180000, s => s.auto && s.hosting.auto && s.items.length === 32 && report.recycleResults.some(x => x.binding === reentry.binding && x.reason === 'bag_full' && BigInt(x.revision) > watermark && BigInt(x.discarded) > 0n) && report.receipts.some(x => x.binding === reentry.binding && BigInt(x.revision) > watermark && BigInt(x.accepted) > 0n));
        for (const event of [...report.recycleResults, ...report.receipts].filter(x => x.binding === reentry.binding)) assert(BigInt(event.revision) > watermark); assert.equal(after.life, before.life);
        const total = s => fixture.progression.exp_per_level.slice(0, s.level - 1).reduce((n, x) => n + BigInt(x), 0n) + BigInt(s.experience); assert(total(after) >= total(before)); report.observations.reentryProgress = { before: total(before).toString(), after: total(after).toString() };
        await screenshot('09-healthy-auto-on'); report.checks.push('保持Auto开启跨正常健康票，原Life/scene与满包保持；仅首Inventory水位之后的新奖励弃物可提示，无历史重播或自动停战');
        await auto(false); await idle(); report.observations.cleanup = await state(); assert(!report.observations.cleanup.auto && !report.observations.cleanup.hosting.auto); report.checks.push('验收结束通过正常保存Toggle关闭Auto；没有操作后台服务或数据库');
        report.sourceAfter = Object.fromEntries(Object.keys(frozen).map(file => [file, hash(file)])); assert.deepEqual(report.sourceAfter, frozen); report.toolSha256After = hash('tools/client/verify_inventory_capacity_browser.cjs'); assert.equal(report.toolSha256After, report.toolSha256); assert.equal(hash('docs/客户端构建检查.json'), report.creatorReportSha256);
        report.healthAfter = []; for (const first of report.health) { const response = await fetch(`http://127.0.0.1:${first.port}/healthz`, { signal: AbortSignal.timeout(5000) }), q = await response.json(); assert(response.ok && q.status === 'ok' && q.instance === first.instance && q.rule_version === 21 && q.fixture_hash === report.fixtureSha256); report.healthAfter.push({ port: first.port, ...q }); } assert.equal(report.errors.length, 0); report.passed = true;
    } catch (error) { report.error = error.stack || String(error); try { if (await page.evaluate(() => ['Game', 'RoleSelect'].includes(window.cc?.director?.getScene()?.name))) await screenshot('failure'); } catch {} throw error; }
    finally { report.finishedAt = new Date().toISOString(); await observed?.dispose(); fs.writeFileSync(path.join(out, 'report.json'), redact(JSON.stringify(report, null, 2)) + '\n'); await browser.close(); }
    console.log(JSON.stringify({ passed: report.passed, checks: report.checks.length, errors: report.errors.length, warnings: report.warnings.length, experience_receipts: report.receipts.length, bag_full_events: bagFullEvents().length }));
}
main().catch(error => { console.error(redact(error.stack || error.message)); process.exitCode = 1; });
