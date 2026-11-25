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

async function selectSkill(page, id) {
    const index = await page.evaluate(({ panel, id }) => {
        const list = cc.find(panel + '/List').getComponent('List');
        const index = list.items.findIndex(row => row.definition.id === id && row.learned);
        if (index >= 0) list.scrollToIndex(index);
        return index;
    }, { panel, id });
    assert(index >= 0, '技能列表没有加载已学召唤');
    await touch(page, panel + '/List', index);
    await page.waitForFunction(id => cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]?.skills?.selected?.definition.id === id, id);
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

async function main() {
    const report = { passed: false, errors: [], assignments: [], connectedSockets: 0 };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: { width: 720, height: 1280 }, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage(), requests = new Map(), replies = new Map();
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== expected.endpoint) { report.errors.push('连接未走隔离游戏节点'); return; }
        report.connectedSockets++;
        socket.on('framesent', ({ payload }) => {
            if (typeof payload === 'string' || payload.length < 6 || payload.readUInt16BE(2) !== 40031) return;
            const { request, binding, offset } = header(payload);
            requests.set(`${binding}:${request}`, pb.SetSkillSlotsRequest.decode(payload.subarray(offset)));
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
        const first = await observe(page, session, [0, 0, 0, 0]);
        assert.equal(first.ruleVersion, String(expected.ruleVersion));
        await page.waitForFunction(session => !!session.hosting, session, { timeout: 5000 });
        if (await page.evaluate(session => session.hosting.autoBattleEnabled, session)) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await page.waitForFunction(session => session.hosting?.autoBattleEnabled === false, session, { timeout: 10000 });
        }
        await touch(page, safe + 'BottomHUD/Skills/SkillsPanel');
        for (const [slot, skill, ids] of [[1, 17, [17, 0, 0, 0]], [2, 30, [17, 30, 0, 0]]]) {
            report.step = `从保存的技能页选择技能${skill}并设置第${slot}槽`;
            await selectSkill(page, skill);
            await touch(page, `${panel}/AttackSlot${slot}`);
            await observe(page, session, ids);
            const item = Array.from(requests).find(([, request]) => JSON.stringify(request.skill_slot_ids) === JSON.stringify(ids));
            assert(item, '技能槽按钮没有发送40031');
            const [key, request] = item, reply = replies.get(key);
            assert(reply && reply.error_code === 0 && reply.new_revision.toString() !== '0', '40031缺少真实成功回执');
            assert.equal(request.operation.op_id.length, 16);
            assert.deepEqual(Buffer.from(reply.op_id), Buffer.from(request.operation.op_id));
            assert.equal(reply.command_seq.toString(), request.operation.command_seq.toString());
            report.assignments.push({ ids, commandSeq: reply.command_seq.toString(), revision: reply.new_revision.toString(), sameOperation: true });
        }
        await page.screenshot({ path: path.join(output, 'summon-slots.png') });
        report.step = '用保存的重连按钮取新票并复核原技能槽';
        await page.evaluate(session => session.connection.socket.close(4001, '隔离召唤槽触屏验收'), session);
        await touch(page, `${reconnect}/Panel/Reconnect`);
        const after = await observe(page, session, [17, 30, 0, 0], first.binding);
        await page.waitForFunction(path => !cc.find(path)?.activeInHierarchy, reconnect);
        if (!await page.evaluate(path => cc.find(path)?.activeInHierarchy ?? false, panel)) await touch(page, safe + 'BottomHUD/Skills/SkillsPanel');
        await page.waitForFunction(path => {
            const label = name => cc.find(`${path}/${name}/Label`)?.getComponent(cc.Label)?.string;
            return label('AttackSlot1')?.includes('召唤骷髅') && label('AttackSlot2')?.includes('召唤神兽');
        }, panel);
        await page.screenshot({ path: path.join(output, 'summon-slots-reconnected.png') });
        assert.equal(after.role, first.role);
        assert.equal(report.connectedSockets, 2);
        assert.equal(requests.size, 2);
        assert.deepEqual(report.errors, []);
        report.reconnect = { newBinding: true, sameRole: true, slots: after.slots };
        report.passed = true;
        delete report.step;
    } finally {
        await session?.dispose();
        await browser.close();
        fs.writeFileSync(path.join(output, 'summon-slots-browser-report.json'), JSON.stringify(report, null, 2) + '\n', 'utf8');
    }
    console.log('实际触屏成功保存骷髅/神兽的两个40031，新票后保留四槽');
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
