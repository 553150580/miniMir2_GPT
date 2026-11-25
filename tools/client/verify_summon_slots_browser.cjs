// 用本机独立 Creator 触屏客户端，观察真实技能槽请求、权威回执和重连接回。
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const manifest = path.resolve(process.argv[2] ?? '');
assert(manifest.startsWith(path.resolve('.work') + path.sep), '触屏验收只能使用独立工作目录');
const expected = JSON.parse(fs.readFileSync(manifest, 'utf8'));
const output = path.dirname(manifest);
const base = new URL(expected.baseUrl), endpoint = new URL(expected.endpoint);
assert(base.protocol === 'http:' && base.hostname === '127.0.0.1' && base.port !== '18080');
assert(endpoint.protocol === 'ws:' && endpoint.hostname === '127.0.0.1' && !['18101', '18103'].includes(endpoint.port));
const safe = 'Canvas/Safe/', panel = `${safe}UIMgr/Panels/Skills`, reconnect = 'Canvas/ReconnectDialog';
const confirm = `${safe}PopupView/Panels/Confirm/ConfirmButton`;
const testRevisionGap = process.env.MINIMIR_SLOTS_BROWSER_REVISION_GAP === '1';
const testSlotFirst = process.env.MINIMIR_SLOTS_BROWSER_SLOT_FIRST === '1';

async function scene(page, name) {
    await page.waitForFunction(name => {
        const current = window.cc?.director?.getScene();
        return current?.name === name && current.getComponentsInChildren('AppRoot').length &&
            cc.find('Canvas/Safe')?.activeInHierarchy && !cc.find('Canvas/Loading')?.activeInHierarchy;
    }, name, { timeout: 60000 });
}

// 只从已保存的按钮或实际列表行读取可触区域，通过 Chromium 触屏派发，而不是执行业务函数。
async function touch(page, fullPath, option) {
    const handle = await page.waitForFunction(({ fullPath, option }) => {
        const root = cc.find(fullPath);
        const node = typeof option === 'number' ? root?.getComponent('List')?.rows.get(option) : option === undefined ? root
            : root?.children.find(child => child.activeInHierarchy && child.getChildByName('Text')?.getComponent(cc.Label)?.string.replace(/^● /u, '') === option);
        if (!node?.activeInHierarchy || node.getComponent(cc.Button)?.interactable === false) return null;
        const bounds = node._uiProps?.uiTransformComp?.getBoundingBoxToWorld();
        if (!bounds) return null;
        const size = cc.view.getVisibleSize(), rect = document.querySelector('canvas').getBoundingClientRect();
        return { x: rect.x + (bounds.x + bounds.width / 2) / size.width * rect.width,
            y: rect.y + (size.height - bounds.y - bounds.height / 2) / size.height * rect.height };
    }, { fullPath, option }, { timeout: 20000 });
    try {
        const point = await handle.jsonValue();
        await page.touchscreen.tap(point.x, point.y);
    } finally { await handle.dispose(); }
}

async function edit(page, name, value) {
    assert(value, '隔离登录缺少认证测试入参');
    await touch(page, safe + name);
    const editor = page.locator('input:visible');
    await editor.waitFor({ state: 'visible', timeout: 5000 });
    await editor.fill(value);
}

// 使用裁剪区内的真实触屏拖动，不通过列表脚本直接跳到目标。
async function selectSkill(page, cdp, id, learned) {
    const trace = [];
    for (let swipe = 0; swipe <= 8; swipe++) {
        const target = await page.evaluate(({ panel, id, learned }) => {
            const list = cc.find(panel + '/List').getComponent('List');
            const index = list.items.findIndex(row => row.definition.id === id && row.learned === learned);
            const pointIn = (node, fraction) => {
                const transform = node._uiProps.uiTransformComp;
                return transform.convertToWorldSpaceAR(new cc.Vec3(
                    (.5 - transform.anchorX) * transform.width, (fraction - transform.anchorY) * transform.height));
            };
            const viewBottom = pointIn(list.viewport, 0).y, viewTop = pointIn(list.viewport, 1).y;
            const row = list.rows.get(index), cellY = row ? pointIn(row, .5).y : undefined;
            const onScreen = cellY !== undefined && cellY > viewBottom + 8 && cellY < viewTop - 8;
            const size = cc.view.getVisibleSize(), rect = document.querySelector('canvas').getBoundingClientRect();
            const point = fraction => {
                const position = pointIn(list.viewport, fraction);
                return { x: rect.x + position.x / size.width * rect.width, y: rect.y + (size.height - position.y) / size.height * rect.height };
            };
            // 接近裁剪边时也往视窗中心滑，避免目标刚进下边缘时测试反向退出。
            const up = cellY !== undefined ? cellY < (viewBottom + viewTop) / 2 : index > Math.max(...Array.from(list.rows.keys()));
            return { index, onScreen: !!onScreen, start: point(up ? .2 : .8), end: point(up ? .8 : .2),
                rows: Array.from(list.rows.keys()), offset: list.scrollView.getScrollOffset().y, maximum: list.scrollView.getMaxScrollOffset().y,
                cellY, viewBottom };
        }, { panel, id, learned });
        assert(target.index >= 0, '技能列表没有加载目标召唤状态');
        if (target.onScreen) {
            await touch(page, panel + '/List', target.index);
            await page.waitForFunction(id => cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]?.skills?.selected?.definition.id === id, id, { timeout: 10000 });
            return swipe;
        }
        trace.push(target);
        await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [target.start] });
        for (let i = 1; i <= 8; i++) {
            await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: target.start.x,
                y: target.start.y + (target.end.y - target.start.y) * i / 8 }] });
            await page.waitForTimeout(55);
        }
        await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
        await page.waitForFunction(path => {
            const scrolling = cc.find(path).getComponent('List').scrollView;
            return !scrolling.isScrolling() && !scrolling.isAutoScrolling();
        }, panel + '/List', { timeout: 10000 });
    }
    assert.fail('触屏滚动后仍无法选中技能 ' + id + ' ' + JSON.stringify(trace));
}

// 两个无符号变长字段依次为请求号与连接绑定，用于配对本次业务回执。
function header(payload) {
    let offset = 4;
    const next = () => {
        let value = 0n, shift = 0n;
        do {
            const byte = payload[offset++];
            value |= BigInt(byte & 0x7f) << shift;
            if (!(byte & 0x80)) return value;
            shift += 7n;
        } while (offset < payload.length);
        throw new Error('WebSocket应用帧头意外结束');
    };
    const request = next().toString(), binding = next().toString();
    return { request, binding, offset };
}

async function observe(page, session, slots, fromBinding = '') {
    const seen = await page.waitForFunction(({ session, slots, fromBinding }) => {
        const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
        if (!session.ready || session.entry?.sceneKind !== 1 || !app?.world?.isMapReady || !session.inventory) return null;
        const binding = session.connection.binding.toString(), observed = Array.from(session.inventory.skillSlotIds ?? []);
        if (fromBinding && BigInt(binding) <= BigInt(fromBinding) || observed.length !== slots.length || observed.some((id, index) => id !== slots[index])) return null;
        return { binding, slots: observed, ruleVersion: session.entry.resourceVersion.toString(), role: session.entryRole };
    }, { session, slots, fromBinding }, { timeout: 18000 });
    try { return await seen.jsonValue(); } finally { await seen.dispose(); }
}

// 配对原请求、编辑时看到的四槽及真实回执，跳过学习和重连消息。
function recordAssignment(report, requests, replies, ids, previous, gap) {
    const item = Array.from(requests).find(([, request]) => request.type === 40031 && JSON.stringify(request.value.skill_slot_ids) === JSON.stringify(ids));
    assert(item, '技能槽按钮没有发送40031');
    const [key, { value: request }] = item, reply = replies.get(key);
    assert(reply && reply.error_code === 0 && reply.new_revision.toString() !== '0', '40031缺少真实成功回执');
    assert.equal(request.operation.op_id.length, 16);
    assert.deepEqual(Buffer.from(reply.op_id), Buffer.from(request.operation.op_id));
    assert.equal(reply.command_seq.toString(), request.operation.command_seq.toString());
    assert.deepEqual(request.expected_skill_slot_ids, previous, '技能槽比较字段必须保留触摸时看到的四槽');
    if (gap) assert.equal(request.operation.expected_revision.toString(), gap.inventory, '修订错位时必须提交读到的实际库存修订');
    report.assignments.push({ ids, previous, commandSeq: reply.command_seq.toString(), revision: reply.new_revision.toString(), sameOperation: true });
}

// 已学骷髅且第 4 槽保留治愈术时，先点空槽应等待点选，并保护已有槽和未学习技能。
async function assignKnownSummonBySlot(page, cdp, session, report, requests, replies, first) {
    const target = `${panel}/AttackSlot1`, targetLabel = target + '/Label';
    const waitForLabel = value => page.waitForFunction(({ path, value }) => cc.find(path)?.getComponent(cc.Label)?.string === value,
        { path: targetLabel, value }, { timeout: 3500 });
    report.step = '旧道士先点空的第一槽';
    await touch(page, target);
    await waitForLabel('1 · 待选');
    const hint = await page.evaluate(path => cc.find(path)?.getComponent(cc.Label)?.string, `${panel}/Hint`);
    assert.match(hint, /第 1 槽待选.*已学技能/);
    await observe(page, session, first.slots);
    assert.equal(requests.size, 0, '点空槽不能立即将默认预览技能移入');
    await page.screenshot({ path: path.join(output, 'summon-slot-awaiting-selection.png') });
    await selectSkill(page, cdp, 30, false);
    await waitForLabel('1 · 待选');
    assert.equal(requests.size, 0, '未学技能不能保存到待选槽');
    await touch(page, target);
    await waitForLabel('1 · 空位');
    assert.equal(requests.size, 0, '取消待选不能发送资产操作');
    report.slotFirst = { hint, inactiveSkillIgnored: true, cancelledWithoutWrite: true };
    report.step = '先点槽位，再通过触屏滚动选已学召唤骷髅';
    await touch(page, target);
    await waitForLabel('1 · 待选');
    const swipes = await selectSkill(page, cdp, 17, true);
    assert(swipes > 0);
    const intermediate = [17, 0, 0, 2];
    await observe(page, session, intermediate);
    recordAssignment(report, requests, replies, intermediate, first.slots);
    await waitForLabel('1 · 召唤骷髅');
    report.step = '重新选技能后点第 3 槽，沿用原操作顺序';
    await selectSkill(page, cdp, 17, true);
    await touch(page, `${panel}/AttackSlot3`);
    const final = [0, 0, 17, 2];
    await observe(page, session, final);
    recordAssignment(report, requests, replies, final, intermediate);
    return final;
}

async function main() {
    const report = { passed: false, errors: [], learning: [], assignments: [], connectedSockets: 0 };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage(), requests = new Map(), replies = new Map();
    const cdp = await context.newCDPSession(page);
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== expected.endpoint) { report.errors.push('连接未走隔离游戏节点'); return; }
        report.connectedSockets++;
        socket.on('framesent', ({ payload }) => {
            if (typeof payload === 'string' || payload.length < 6) return;
            const type = payload.readUInt16BE(2), codec = { 40010: pb.LearnSkillRequest, 40031: pb.SetSkillSlotsRequest }[type];
            if (!codec) return;
            const { request, binding, offset } = header(payload);
            requests.set(`${binding}:${request}`, { type, value: codec.decode(payload.subarray(offset)) });
        });
        socket.on('framereceived', ({ payload }) => {
            if (typeof payload === 'string' || payload.length < 6 || payload.readUInt16BE(2) !== 40005 || payload[1] & 1) return;
            const { request, binding, offset } = header(payload), key = `${binding}:${request}`;
            if (requests.has(key)) replies.set(key, pb.OperationResult.decode(payload.subarray(offset)));
        });
    });
    let session;
    try {
        report.step = '从保存的登录和选服界面进入隔离道士';
        await page.goto(base.href);
        await scene(page, 'Login');
        await edit(page, 'Account', process.env.MINIMIR_SLOTS_BROWSER_ACCOUNT);
        await edit(page, 'Password', process.env.MINIMIR_SLOTS_BROWSER_PASSWORD);
        await touch(page, safe + 'Login');
        await scene(page, 'ServerSelect');
        await touch(page, safe + 'RegionList/View/Content', expected.region);
        await touch(page, safe + 'ServerList/View/Content', expected.server);
        await scene(page, 'RoleSelect');
        await page.waitForFunction(name => cc.find('Canvas/Safe/Role0/Label')?.getComponent(cc.Label)?.string.includes(name), expected.roleName);
        await touch(page, safe + 'Enter');
        await scene(page, 'Game');
        session = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
        const first = await observe(page, session, [0, 0, 0, 2]);
        assert.equal(first.ruleVersion, String(expected.ruleVersion));
        await page.waitForFunction(session => !!session.hosting, session, { timeout: 5000 });
        if (!await page.evaluate(session => session.hosting.autoBattleEnabled, session)) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await page.waitForFunction(session => session.hosting?.autoBattleEnabled === true, session, { timeout: 10000 });
        }
        report.automaticBattle = true;
        await touch(page, safe + 'BottomHUD/Skills/SkillsPanel');
        const finalSlots = testSlotFirst ? await assignKnownSummonBySlot(page, cdp, session, report, requests, replies, first) : [17, 30, 0, 2];
        let previous = first.slots;
        const lessons = testSlotFirst ? [] : [[1, 17, [17, 0, 0, 2]], [2, 30, finalSlots]];
        for (const [slot, skill, ids] of lessons) {
            report.step = `触屏滚动选择并学习技能${skill}`;
            const swipes = await selectSkill(page, cdp, skill, false);
            await touch(page, panel + '/Learn');
            await touch(page, confirm);
            await page.waitForFunction(({ session, skill }) => session.inventory?.learnedSkills?.some(row => row.skillId === skill), { session, skill });
            const learnItem = Array.from(requests).find(([, request]) => request.type === 40010 && request.value.skill_id === skill);
            assert(learnItem && replies.get(learnItem[0])?.error_code === 0, '未收到学习原书的成功回执');
            report.learning.push({ skill, swipes, receipt: true });
            report.step = `学习完成后设置第${slot}槽`;
            const exerciseGap = testRevisionGap && slot === 1;
            let beforeBinding;
            if (exerciseGap) {
                // 只在隔离浏览器模拟绑定观察先到，库存、槽位、服务端接收及回执仍走真实连接。
                beforeBinding = await page.evaluateHandle(session => session.bindingState, session);
                report.revisionGap = await page.evaluate(session => {
                    const revision = session.inventory.revision, ahead = revision.add(1).toString();
                    session.bindingState = { ...session.bindingState, roleRevision: ahead };
                    return { inventory: revision.toString(), binding: ahead };
                }, session);
            }
            try {
                await touch(page, `${panel}/AttackSlot${slot}`);
                await observe(page, session, ids);
            } finally {
                if (beforeBinding) {
                    await page.evaluate(({ session, beforeBinding, revision }) => {
                        if (session.bindingState?.roleRevision === revision) session.bindingState = beforeBinding;
                    }, { session, beforeBinding, revision: report.revisionGap.binding });
                    await beforeBinding.dispose();
                }
            }
            recordAssignment(report, requests, replies, ids, previous, exerciseGap ? report.revisionGap : undefined);
            previous = ids;
        }
        await page.screenshot({ path: path.join(output, 'summon-slots.png') });
        report.step = '用保存的重连按钮取新票并复核原技能槽';
        await page.evaluate(session => session.connection.socket.close(4001, '隔离召唤槽触屏验收'), session);
        await touch(page, `${reconnect}/Panel/Reconnect`);
        const after = await observe(page, session, finalSlots, first.binding);
        await page.waitForFunction(path => !cc.find(path)?.activeInHierarchy, reconnect);
        if (!await page.evaluate(path => cc.find(path)?.activeInHierarchy ?? false, panel)) await touch(page, safe + 'BottomHUD/Skills/SkillsPanel');
        await page.waitForFunction(({ path, testSlotFirst }) => {
            const label = name => cc.find(`${path}/${name}/Label`)?.getComponent(cc.Label)?.string;
            return label('AttackSlot4')?.includes('治愈术') && (testSlotFirst
                ? label('AttackSlot1')?.includes('空位') && label('AttackSlot3')?.includes('召唤骷髅')
                : label('AttackSlot1')?.includes('召唤骷髅') && label('AttackSlot2')?.includes('召唤神兽'));
        }, { path: panel, testSlotFirst });
        await page.screenshot({ path: path.join(output, 'summon-slots-reconnected.png') });
        assert.equal(after.role, first.role);
        assert.equal(report.connectedSockets, 2);
        assert.equal(requests.size, testSlotFirst ? 2 : 4);
        assert(report.learning.every(row => row.swipes > 0), '两种召唤都应覆盖实际触屏滚动');
        assert.deepEqual(report.errors, []);
        report.reconnect = { newBinding: true, sameRole: true, slots: after.slots };
        report.passed = true;
        delete report.step;
    } catch (error) {
        report.failure = { error: error.message, sent: Array.from(requests.values(), request => request.type === 40031
            ? { id: request.type, slots: request.value.skill_slot_ids } : request.type),
            client: session ? await page.evaluate(session => session.lastError, session).catch(() => '') : '' };
        throw error;
    } finally {
        if (!report.passed) await page.screenshot({ path: path.join(output, 'summon-slots-failed.png') }).catch(() => {});
        await session?.dispose();
        await browser.close();
        fs.writeFileSync(path.join(output, 'summon-slots-browser-report.json'), JSON.stringify(report, null, 2) + '\n', 'utf8');
    }
    console.log(testSlotFirst ? '既有道士先点空槽再选骷髅可保存，反向流程与重连也正常'
        : '实际触屏滚动、学习并保存骷髅/神兽，重连后保留已配置四槽');
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
