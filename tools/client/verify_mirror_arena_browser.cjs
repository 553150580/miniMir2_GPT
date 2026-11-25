// 真实触屏验收攻守双客户端、原场首帧、格战表现、健康重连及原普通收益。
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const manifest = path.resolve(process.argv[2] ?? '');
assert(manifest.startsWith(path.resolve('.work') + path.sep), '只允许独立验收目录');
const expected = JSON.parse(fs.readFileSync(manifest, 'utf8'));
const base = new URL(expected.baseUrl), endpoint = new URL(expected.endpoint), output = path.dirname(manifest);
assert(base.protocol === 'http:' && base.hostname === '127.0.0.1' && base.port !== '18080');
assert(endpoint.protocol === 'ws:' && endpoint.hostname === '127.0.0.1' && !['18101', '18103'].includes(endpoint.port));
const safe = 'Canvas/Safe/', lobby = safe + 'UIMgr/Panels/MirrorArena';
const battle = safe + 'MirrorArenaBattle', confirm = safe + 'PopupView/Panels/Confirm/ConfirmButton';
const sessionModule = 'chunks:///_virtual/GameSession.ts';

async function wait(page, predicate, argument, timeout = 18000) {
    const value = await page.waitForFunction(predicate, argument, { timeout });
    try { return await value.jsonValue(); } finally { await value.dispose(); }
}

async function scene(page, name) {
    await wait(page, name => {
        const active = window.cc?.director?.getScene();
        return active?.name === name && active.getComponentsInChildren('AppRoot').length > 0
            && cc.find('Canvas/Safe')?.activeInHierarchy && !cc.find('Canvas/Loading')?.activeInHierarchy;
    }, name, 45000).catch(error => { throw new Error('等待场景 ' + name + '：' + error.message); });
}

async function touch(page, nodePath, text) {
    const point = await wait(page, ({ nodePath, text }) => {
        const root = cc.find(nodePath);
        const target = text === undefined ? root : root?.children.find(node => node.activeInHierarchy &&
            node.getChildByName('Text')?.getComponent(cc.Label)?.string.includes(text));
        if (!target?.activeInHierarchy || target.getComponent(cc.Button)?.interactable === false) return null;
        const bounds = target._uiProps.uiTransformComp?.getBoundingBoxToWorld();
        if (!bounds) return null;
        const size = cc.view.getVisibleSize(), box = document.querySelector('canvas').getBoundingClientRect();
        return { x: box.left + (bounds.x + bounds.width / 2) * box.width / size.width,
            y: box.top + (size.height - bounds.y - bounds.height / 2) * box.height / size.height };
    }, { nodePath, text }).catch(error => { throw new Error('等待触屏节点 ' + nodePath + '：' + error.message); });
    await page.touchscreen.tap(point.x, point.y);
}

async function label(page, nodePath, text, timeout) {
    return wait(page, ({ nodePath, text }) => {
        const node = cc.find(nodePath), value = node?.getComponent(cc.Label)?.string;
        return node?.activeInHierarchy && value?.includes(text) ? value : null;
    }, { nodePath, text }, timeout).catch(error => { throw new Error('等待文字 ' + nodePath + '：' + error.message); });
}

async function fill(page, name, value) {
    assert(value, '独立验收账号凭据未提供');
    await touch(page, safe + name);
    const editor = page.locator('input:visible');
    await editor.waitFor({ state: 'visible', timeout: 5000 });
    await editor.fill(value);
}

async function login(page, side) {
    await page.bringToFront();
    await page.goto(base.href);
    await scene(page, 'Login');
    await fill(page, 'Account', process.env['MINIMIR_MIRROR_' + side.toUpperCase() + '_ACCOUNT']);
    await fill(page, 'Password', process.env['MINIMIR_MIRROR_' + side.toUpperCase() + '_PASSWORD']);
    await touch(page, safe + 'Login');
    await scene(page, 'ServerSelect');
    await touch(page, safe + 'RegionList/View/Content', expected.region);
    await touch(page, safe + 'ServerList/View/Content', expected.server);
    await scene(page, 'RoleSelect');
    await label(page, safe + 'Role0/Label', expected[side]);
    await touch(page, safe + 'Enter');
    await scene(page, 'Game');
    const session = await page.evaluateHandle(async module => (await System.import(module)).session, sessionModule);
    await wait(page, session => {
        const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
        return session.ready && !!session.inventory && !!session.hosting && !!app.world?.isMapReady;
    }, session, 35000);
    return session;
}

async function openLobby(page) {
    await touch(page, safe + 'BottomHUD/Footer/Functions');
    await touch(page, safe + 'UIMgr/Panels/Functions/Pages/Page0/MirrorArena');
    await label(page, lobby + '/Title', '镜像竞技');
    await label(page, lobby + '/Attempts', '/5');
}

async function historyRow(page, name, result) {
    return wait(page, ({ lobby, name, result }) => {
        const list = cc.find(lobby + '/List/View/Content');
        return list?.children.some(row => row.activeInHierarchy &&
            row.getChildByName('Text')?.getComponent(cc.Label)?.string.includes(name + '　' + result));
    }, { lobby, name, result });
}

async function originalWorld(page, session, sameWorld = false) {
    return page.evaluate(({ session, sameWorld }) => {
        const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
        const player = session.baseline?.entities.find(row => row.entityHandle === session.entry?.playerHandle);
        return { map: session.entry.roomId.toString(), scene: session.entry.sceneEpoch.toString(),
            life: player?.life.toString(), binding: session.connection.binding.toString(), rule: session.entry.resourceVersion.toString(),
            ready: session.ready, sceneKind: session.entry.sceneKind, role: session.role?.name,
            auto: session.hosting?.autoBattleEnabled ?? false,
            ...(sameWorld ? { sameView: app.world === window.originalMirrorWorld } : {}) };
    }, { session, sameWorld });
}

function packet(payload) {
    if (!Buffer.isBuffer(payload) || payload.length < 6 || payload[0] !== 1 || payload[1] !== 0) return null;
    let offset = 4;
    function next() {
        let value = 0n, shift = 0n;
        while (offset < payload.length) {
            const byte = payload[offset++];
            value |= BigInt(byte & 127) << shift;
            if (!(byte & 128)) return value.toString();
            shift += 7n;
        }
        return '';
    }
    const request = next(), binding = next();
    return { id: payload.readUInt16BE(2), request, binding, body: payload.subarray(offset) };
}

function watchSocket(page, errors) {
    const seen = { sockets: 0, disconnected: 0, requests: [], states: [], frames: [], casts: [], rewards: [], ordinaryMoves: 0 };
    page.on('pageerror', error => errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) {
            errors.push('实际连接未走隔离游戏节点');
            return;
        }
        seen.sockets++;
        socket.on('close', () => seen.disconnected++);
        socket.on('framesent', ({ payload }) => {
            const message = packet(payload);
            if (message?.id === 20006) seen.ordinaryMoves++;
            if (message?.id === 20025) seen.requests.push({ binding: message.binding, value: pb.MirrorArenaRequest.decode(message.body) });
        });
        socket.on('framereceived', ({ payload }) => {
            const message = packet(payload);
            if (message?.id === 20026) seen.states.push(pb.MirrorArenaState.decode(message.body));
            if (message?.id === 40019) seen.rewards.push({ binding: message.binding, value: pb.ExperienceGrantResult.decode(message.body) });
            if (message?.id !== 20027) return;
            const frame = pb.MirrorArenaCombatFrame.decode(message.body);
            const summary = { binding: message.binding, scene: frame.scene_epoch.toString(), battle: frame.battle_scene_epoch.toString(),
                challenge: Buffer.from(frame.challenge_id).toString('hex'), sequence: frame.sequence.toString() };
            seen.frames.push(summary);
            for (const event of frame.observations) {
                if (event.message_id !== 30006) continue;
                const cast = pb.CastAccepted.decode(event.payload);
                if (cast.skill_id === expected.fireball && cast.caster_handle === 2 && cast.target_handle === 1 &&
                    cast.scene_epoch.toString() === summary.battle) seen.casts.push(summary);
            }
        });
    });
    return seen;
}

// 在原生 WebSocket 发送时只记录已保存战斗层和真实资源状态，协议动作由外部独立解码核对。
async function observeMountedDispatch(page) {
    await page.addInitScript(() => {
        window.mirrorDispatches = [];
        const originalSend = WebSocket.prototype.send;
        WebSocket.prototype.send = function (value) {
            const bytes = ArrayBuffer.isView(value) ? new Uint8Array(value.buffer, value.byteOffset, value.byteLength)
                : value instanceof ArrayBuffer ? new Uint8Array(value) : null;
            if (bytes?.length >= 6 && ((bytes[2] << 8) | bytes[3]) === 20025) {
                const app = window.cc?.director?.getScene()?.getComponentsInChildren('AppRoot')[0];
                const field = app?.mirrorArena?.presentation?.active?.view;
                window.mirrorDispatches.push({ ready: !!field?.isMapReady, scene: field?.sceneEpoch.toString(),
                    independent: !!field && field !== app.world,
                    savedWorld: field?.world === window.cc?.find('Canvas/Safe/MirrorArenaBattle/MapViewport/World'),
                    ordinaryMap: app?.world?.assets?.mapId });
            }
            return originalSend.call(this, value);
        };
    });
}

async function waitForRewardDuringBattle(page, session, prior) {
    return wait(page, ({ session, prior }) => {
        const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
        const notice = session.experienceNotice;
        return app.mirrorArena?.transport?.snapshot.battle?.phase === 2 && notice?.source &&
            BigInt(notice.source) > BigInt(prior) ? notice.source : null;
    }, { session, prior }, 25000);
}

async function main() {
    const report = { passed: false, errors: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const options = { viewport: { width: 720, height: 1280 }, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' };
    const defenderContext = await browser.newContext(options);
    const attackerContext = await browser.newContext(options);
    const defender = await defenderContext.newPage(), attacker = await attackerContext.newPage();
    const guardWire = watchSocket(defender, report.errors), attackWire = watchSocket(attacker, report.errors);
    await observeMountedDispatch(attacker);
    let guardSession, attackSession;
    try {
        report.step = '法师正常登录、选服并发布防守';
        guardSession = await login(defender, 'defender');
        assert.equal((await originalWorld(defender, guardSession)).role, expected.defender);
        await openLobby(defender);
        await touch(defender, lobby + '/Publish');
        await touch(defender, confirm);
        await label(defender, lobby + '/Defense', '防守快照 v1');
        const published = guardWire.states.find(state => state.open && state.published_defense_version.toString() === '1');
        assert(published && guardWire.requests.some(row => row.value.action === 2));
        await defender.evaluate(session => {
            session.connection.socket.close(4001, '防守发布后下线');
        }, guardSession);
        await wait(defender, session => !session.ready, guardSession, 6000);
        await guardSession.dispose();
        guardSession = undefined;
        await defenderContext.close();

        report.step = '战士正常登录，同运行服读取原候选并开普通自动';
        attackSession = await login(attacker, 'attacker');
        const original = await originalWorld(attacker, attackSession);
        assert.equal(original.role, expected.attacker);
        assert.equal(original.map, expected.normalMap);
        assert.equal(original.rule, expected.ruleVersion);
        if (!original.auto) {
            await touch(attacker, safe + 'MapViewport/Actions/Auto');
            await wait(attacker, session => session.hosting?.autoBattleEnabled === true, attackSession);
        }
        await attacker.evaluate(() => { window.originalMirrorWorld = cc.director.getScene().getComponentsInChildren('AppRoot')[0].world; });
        await openLobby(attacker);
        const offer = attackWire.states.find(state => state.offers.some(row => row.defender_name === expected.defender))
            ?.offers.find(row => row.defender_name === expected.defender);
        assert(offer && offer.defender_job === 1 && offer.defense_version.toString() === '1');
        await touch(attacker, lobby + '/List/View/Content', expected.defender);
        await label(attacker, lobby + '/Detail', '防守快照 v1');
        await attacker.screenshot({ path: path.join(output, 'mirror-lobby.png') });
        report.step = '触屏挑战并握手，检查同一场次的两侧外观与守方火球';
        await touch(attacker, lobby + '/Challenge');
        await touch(attacker, confirm);
        await label(attacker, battle + '/Header/Status', '双方自动战斗', 42000);
        const reserved = attackWire.states.find(state => state.battle_state === 1 && state.challenge);
        assert(reserved && reserved.battle_map_id.toString() === expected.map);
        const challenge = Buffer.from(reserved.challenge.challenge_id).toString('hex');
        const epoch = reserved.battle_scene_epoch.toString();
        const start = attackWire.requests.find(row => row.value.action === 3);
        assert(start && Buffer.from(start.value.offer_id).equals(Buffer.from(offer.offer_id)));
        const readyIndex = attackWire.requests.findIndex(row => row.value.action === 4);
        const readyRequest = attackWire.requests[readyIndex]?.value;
        const dispatch = (await attacker.evaluate(() => window.mirrorDispatches))[readyIndex];
        assert(readyRequest && Buffer.from(readyRequest.challenge_id).toString('hex') === challenge);
        assert.equal(readyRequest.battle_scene_epoch.toString(), epoch);
        assert.deepEqual(dispatch, { ready: true, scene: epoch, independent: true, savedWorld: true, ordinaryMap: expected.normalMap });
        const visible = await wait(attacker, ({ fireball, heading }) => {
            const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
            const view = app.mirrorArena?.presentation?.active?.view;
            const front = view?.visuals.get(1), guard = view?.visuals.get(2);
            if (!front?.body[0]?.spriteFrame || !guard?.body[0]?.spriteFrame || !guard.body[0].node.activeInHierarchy) return null;
            const fire = view.manifest.effects[String(fireball)];
            const keys = new Set([...(fire.cast?.frames ?? []), ...(fire.hit?.frames ?? []),
                ...(fire.projectile?.directions?.flat() ?? [])]);
            const effect = Array.from(view.effects?.nodes.values() ?? []).find(node => keys.has(node.frame) && node.sprite.spriteFrame
                && node.node.activeInHierarchy);
            if (!effect || !cc.find(heading)?.activeInHierarchy || guard.state.definitionId !== 1 || front.state.definitionId !== 0) return null;
            return { attacker: front.keys[0], defender: guard.keys[0], fireball: effect.frame, width: view.map.width, height: view.map.height };
        }, { fireball: expected.fireball, heading: battle + '/Header' }, 15000);
        assert(attackWire.casts.some(cast => cast.challenge === challenge && cast.binding === original.binding));
        assert.equal(visible.width, 13);
        assert.equal(visible.height, 13);
        assert.equal((await originalWorld(attacker, attackSession, true)).sameView, true);
        report.battle = { map: expected.map, firstFrameAfterMount: true, ...visible };
        await attacker.screenshot({ path: path.join(output, 'mirror-combat.png') });
        await attacker.touchscreen.tap(360, 640);

        report.step = '使用保存的重连按钮取得新票，接回原镜像场次';
        const priorFrame = attackWire.frames.filter(row => row.challenge === challenge).at(-1);
        assert(priorFrame && priorFrame.battle === epoch);
        await attacker.evaluate(session => session.connection.socket.close(4001, '独立镜像触屏重连验收'), attackSession);
        await touch(attacker, 'Canvas/ReconnectDialog/Panel/Reconnect');
        const reconnect = await wait(attacker, ({ session, old, epoch, previous, heading }) => {
            const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
            const active = app.mirrorArena?.presentation?.active;
            const binding = session.connection.binding.toString();
            if (!session.ready || BigInt(binding) <= BigInt(old) || !cc.find(heading)?.activeInHierarchy ||
                !active || active.view.sceneEpoch.toString() !== epoch || active.sequence <= BigInt(previous)) return null;
            return { binding, sequence: active.sequence.toString() };
        }, { session: attackSession, old: original.binding, epoch, previous: priorFrame.sequence, heading: battle }, 24000);
        assert(attackWire.frames.some(row => row.binding === reconnect.binding && row.challenge === challenge && row.battle === epoch
            && BigInt(row.sequence) > BigInt(priorFrame.sequence)));
        const retained = await originalWorld(attacker, attackSession);
        assert.equal(retained.map, original.map);
        assert.equal(retained.life, original.life);
        assert.equal(retained.role, original.role);
        report.reconnect = { newBinding: true, sameChallenge: true, originalMap: true, sequence: reconnect.sequence };

        report.step = '独立战斗仍在推进时观察普通自动收益提交';
        const beforeReward = attackWire.rewards.at(-1)?.value.source_seq.toString() ?? '0';
        const reward = await waitForRewardDuringBattle(attacker, attackSession, beforeReward);
        assert(attackWire.rewards.some(row => row.binding === reconnect.binding && row.value.source_seq.toString() === reward
            && BigInt(row.value.accepted_experience.toString()) > 0n));
        report.ordinaryReward = { source: reward, confirmedDuringMirror: true };

        report.step = '触屏认输并核对服务端确认的一次失败结果';
        await touch(attacker, battle + '/Footer/Surrender');
        await touch(attacker, confirm);
        await label(attacker, battle + '/Header/Status', '失败 · 积分 +1');
        const result = attackWire.states.find(state => state.battle_state === 4 && state.challenge &&
            Buffer.from(state.challenge.challenge_id).toString('hex') === challenge);
        assert(result && result.challenge.outcome === 2 && result.challenge.awarded_points.toString() === '1');
        const surrender = attackWire.requests.filter(row => row.value.action === 5);
        assert.equal(surrender.length, 1);
        assert.equal(Buffer.from(surrender[0].value.challenge_id).toString('hex'), challenge);
        await attacker.screenshot({ path: path.join(output, 'mirror-result.png') });
        await touch(attacker, battle + '/Footer/Return');
        await wait(attacker, node => !cc.find(node)?.activeInHierarchy, battle);
        const restored = await originalWorld(attacker, attackSession);
        assert(restored.ready && restored.auto && restored.sceneKind === 1);
        assert.equal(restored.map, expected.normalMap);
        await openLobby(attacker);
        await label(attacker, lobby + '/Attempts', '1/5');
        await label(attacker, lobby + '/Score', '积分 1');
        await touch(attacker, lobby + '/SectionTabs/AttackHistory');
        await historyRow(attacker, expected.defender, '失败');

        report.step = '守方新客户端查看防守胜利，个人积分和次数仍为零';
        const reportContext = await browser.newContext(options);
        const guardReport = await reportContext.newPage();
        const guardReportWire = watchSocket(guardReport, report.errors);
        guardSession = await login(guardReport, 'defender');
        await openLobby(guardReport);
        await label(guardReport, lobby + '/Attempts', '0/5');
        await label(guardReport, lobby + '/Score', '积分 0');
        await touch(guardReport, lobby + '/SectionTabs/DefenseHistory');
        await historyRow(guardReport, expected.attacker, '胜利');
        assert(guardReportWire.states.some(state => state.defense_history.some(record =>
            Buffer.from(record.challenge_id).toString('hex') === challenge && record.outcome === 2)));
        await guardSession.dispose();
        guardSession = undefined;
        await reportContext.close();
        assert.equal(guardWire.sockets, 1);
        assert.equal(guardWire.disconnected, 1);
        assert.equal(attackWire.sockets, 2);
        assert.equal(attackWire.ordinaryMoves, 0);
        assert.deepEqual(report.errors, []);
        report.finish = { outcome: result.challenge.outcome, points: 1, attempts: 1, returned: true, bothHistories: true };
        delete report.step;
        report.passed = true;
    } catch (error) {
        report.error = error.message;
        const observedPage = attackSession || defender.isClosed() ? attacker : defender;
        report.observed = await observedPage.evaluate(async module => {
            const { session } = await System.import(module);
            const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
            const state = app?.mirrorArena?.transport?.snapshot;
            return { scene: cc.director.getScene()?.name, ready: session.ready, rule: session.entry?.resourceVersion?.toString(),
                query: state?.lobby?.open, battle: state?.battle?.phase, visible: !!app?.mirrorArena?.visible,
                mounted: !!app?.mirrorArena?.presentation?.active?.view?.isMapReady,
                presented: app?.mirrorArena?.presentation?.active?.sequence?.toString(), localError: app?.mirrorArena?.localError,
                status: cc.find('Canvas/Safe/MirrorArenaBattle/Header/Status')?.getComponent(cc.Label)?.string,
                menu: app?.ui?.top?.name, error: session.lastError };
        }, sessionModule).catch(() => null);
        report.wire = { actionCodes: attackWire.requests.map(row => row.value.action), states: attackWire.states.map(row => row.battle_state),
            frames: attackWire.frames.length, casts: attackWire.casts.length, rewards: attackWire.rewards.length,
            guardSockets: guardWire.sockets, guardActions: guardWire.requests.map(row => row.value.action) };
        throw new Error(report.step + '：' + error.message + ' ' + JSON.stringify(report.observed));
    } finally {
        await guardSession?.dispose();
        await attackSession?.dispose();
        await browser.close();
        fs.writeFileSync(path.join(output, 'mirror-arena-browser-report.json'), JSON.stringify(report, null, 2) + '\n', 'utf8');
    }
    console.log('镜像触屏核对攻守实图、守方火球、原场重连、普通收益及唯一认输终态');
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
