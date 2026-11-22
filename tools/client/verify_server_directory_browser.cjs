// 在独立试玩入口用正常触屏走完两个区服的选服、建角和入场，只记录连接目标与确认帧。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const manifestPath = path.resolve(process.argv[2] ?? '');
const work = path.resolve('.work');
assert(manifestPath.startsWith(work + path.sep), '选服验收清单必须在独立工作目录');
const { baseUrl, entries } = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
const base = new URL(baseUrl);
assert(base.protocol === 'http:' && base.hostname === '127.0.0.1' && base.port !== '18080', '仅允许独立回环试玩入口');
assert.equal(entries.length, 2);
assert.notEqual(entries[0].origin, entries[1].origin);
assert.notEqual(entries[0].endpoint, entries[1].endpoint);
for (const entry of entries) {
    const target = new URL(entry.endpoint);
    assert(target.protocol === 'ws:' && target.hostname === '127.0.0.1' && target.pathname === '/ws');
}

async function main() {
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const report = { baseUrl, reached: [], errors: [] };
    let page;
    try {
        const context = await browser.newContext({ viewport: { width: 720, height: 1280 }, hasTouch: true, isMobile: true, deviceScaleFactor: 1 });
        page = await context.newPage();
        page.on('pageerror', error => report.errors.push(error.message));
        const connections = [];
        const createdOrigins = [];
        page.on('request', request => {
            if (new URL(request.url()).pathname !== '/account/rpc') return;
            const raw = request.postDataBuffer();
            if (!raw || raw.length < 6 || raw[1] !== 0 || raw.readUInt16BE(2) !== 10005) return;
            let offset = 4;
            for (let field = 0; field < 2; field++) while (offset < raw.length && raw[offset++] & 0x80) {}
            createdOrigins.push(pb.CreateRoleRequest.decode(raw.subarray(offset)).origin_server_id.toString());
        });
        page.on('websocket', socket => {
            const state = { endpoint: socket.url(), ready: false };
            connections.push(state);
            socket.on('framereceived', ({ payload }) => {
                // 消息 6 是首基线确认后的 SyncReady。观察不保存账号、口令、网络票据或资产正文。
                if (typeof payload !== 'string' && payload.length >= 6 && payload[0] === 1 && payload[1] === 0
                    && payload[2] * 256 + payload[3] === 6) state.ready = true;
            });
        });
        const scene = async name => {
            report.step = '等待场景 ' + name;
            await page.waitForFunction(name => {
                const current = window.cc?.director?.getScene();
                return current?.name === name && current.getComponentsInChildren('AppRoot').length
                    && cc.find('Canvas/Safe')?.activeInHierarchy && !cc.find('Canvas/Loading')?.activeInHierarchy;
            }, name, { timeout: 60000 });
            await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
        };
        const tap = async (suffix, label) => {
            report.step = `触屏 ${suffix}${label === undefined ? '' : ' · ' + label}`;
            const handle = await page.waitForFunction(({ suffix, label }) => {
                const node = cc.find('Canvas/Safe/' + suffix);
                const target = label == null ? node : node?.children.find(child => {
                    const value = child.getChildByName('Text')?.getComponent(cc.Label)?.string;
                    return value?.replace(/^● /u, '') === label;
                });
                if (!target?.activeInHierarchy || label != null && !target.getComponent(cc.Button)?.interactable) return null;
                const transform = target._uiProps?.uiTransformComp;
                if (!transform) return null;
                const box = transform.getBoundingBoxToWorld();
                const visible = cc.view.getVisibleSize(), rect = document.querySelector('canvas').getBoundingClientRect();
                return { x: rect.x + (box.x + box.width / 2) / visible.width * rect.width,
                    y: rect.y + (visible.height - box.y - box.height / 2) / visible.height * rect.height };
            }, { suffix, label }, { timeout: 30000 });
            const point = await handle.jsonValue();
            await handle.dispose();
            await page.touchscreen.tap(point.x, point.y);
        };
        const edit = async (name, value) => {
            await tap(name);
            const widget = page.locator('input:visible');
            await widget.waitFor({ state: 'visible', timeout: 5000 });
            await widget.fill(value);
        };
        await page.goto(base.href);
        await scene('Login');
        await edit('Account', 'scope_' + crypto.randomBytes(5).toString('hex'));
        await edit('Password', crypto.randomBytes(8).toString('hex'));
        await tap('Register');
        for (const [index, entry] of entries.entries()) {
            await scene('ServerSelect');
            await tap('RegionList/View/Content', entry.region);
            await tap('ServerList/View/Content', entry.server);
            await scene('RoleSelect');
            await page.waitForFunction(name => cc.find('Canvas/Safe/Title')?.getComponent(cc.Label)?.string.startsWith(name + ' · '), entry.server);
            assert.equal(await page.evaluate(() => cc.find('Canvas/Safe/Role0/Label')?.getComponent(cc.Label)?.string), '空角色槽');
            const role = '路由' + crypto.randomBytes(2).toString('hex');
            await edit('Name', role);
            await tap('Create');
            await page.waitForFunction(name => cc.find('Canvas/Safe/Role0/Label')?.getComponent(cc.Label)?.string.includes(name), role);
            assert.equal(createdOrigins[index], entry.origin);
            const before = connections.length;
            await tap('Enter');
            await scene('Game');
            await page.waitForFunction(() => cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.reconciliationStarted,
                null, { timeout: 30000 });
            const opened = connections.slice(before);
            assert.deepEqual(opened, [{ endpoint: entry.endpoint, ready: true }]);
            report.reached.push({ region: entry.region, server: entry.server, origin: createdOrigins[index], ...opened[0] });
            if (entry !== entries.at(-1)) {
                await tap('BottomHUD/Footer/ChangeRole');
                await scene('RoleSelect');
                await tap('BackServers');
            }
        }
        assert.deepEqual(createdOrigins, entries.map(entry => entry.origin));
        assert.deepEqual(report.errors, []);
        delete report.step;
        report.passed = true;
    } catch (error) {
        if (page) report.observed = await page.evaluate(() => {
            const scene = window.cc?.director?.getScene()?.name;
            const rows = name => cc.find('Canvas/Safe/' + name + '/View/Content')?.children.map(
                node => node.getChildByName('Text')?.getComponent(cc.Label)?.string).filter(Boolean);
            return { scene, status: cc.find('Canvas/Safe/Status')?.getComponent(cc.Label)?.string,
                regions: rows('RegionList'), servers: rows('ServerList') };
        }).catch(() => null);
        throw new Error(`${report.step}: ${error.message}${report.observed ? ' ' + JSON.stringify(report.observed) : ''}`);
    } finally {
        await browser.close();
        fs.writeFileSync(path.join(path.dirname(manifestPath), 'server-directory-browser-report.json'), JSON.stringify(report, null, 2) + '\n', 'utf8');
    }
}

main().catch(error => { console.error(error.message); process.exitCode = 1; });
