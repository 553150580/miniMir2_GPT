// 只经真实PB/HTTP/WS观察健康与故障现场；凭据和原操作正文不写日志。
'use strict';
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const readline = require('node:readline');
const {Session, ids, encode, decode, until, sleep, unsigned, id, signature} = require('./recovery_probe.cjs');
const root = path.resolve(__dirname, '../..');
const fixture = JSON.parse(fs.readFileSync(path.join(root, 'config/development/first-loop.json'), 'utf8'));
const skill = fixture.skills.find(value => value.id === 1);
const state = {sessions: []};
const stamp = () => Number(process.hrtime.bigint() / 1000000n);
const samePoint = (a, b) => a.x === b.x && a.y === b.y;

function checkpoint(stage, session = state.session, extra = {}) {
  if (!state.diagnostic) return;
  const snapshot = {stage, elapsed_ms: stamp() - state.diagnostic.started_ms, ...extra};
  if (session?.entry && session.entities.length) snapshot.body = safeBody(session);
  state.diagnostic.samples.push(snapshot);
  if (state.diagnostic.samples.length > 12) state.diagnostic.samples.shift();
}
function safeBody(session) {
  try { return session?.entry && session.entities.length ? session.body() : null; }
  catch { return null; }
}
function failureDiagnostic() {
  const session = state.session;
  return {elapsed_ms: stamp() - state.diagnostic.started_ms, samples: state.diagnostic.samples,
    current_body: safeBody(session),
    entities: (session?.entities || []).map(entity => ({entity_kind: entity.entity_kind,
      entity_handle: entity.entity_handle, hp: String(entity.hp), position: entity.position}))};
}

class RuntimeSession extends Session {
  constructor(url) { super(url); this.input = 2; }
  receive(frame) { frame.received_ms = stamp(); super.receive(frame); }
  operation() {
    return {op_id: id(), command_seq: this.ready.next_economic_command_seq,
      expected_revision: this.inventory.revision, expected_rule_version: unsigned(state.ruleVersion)};
  }
  async enterCold(value) {
    await this.connect();
    const request = this.send(ids.MsgC2SEnterLine, {account_ticket: value});
    const response = await this.result(ids.MsgS2CEnterLine, request);
    assert.equal(response.error, 0, '冷入场错误码');
    await until(() => this.baseline && String(this.baseline.scene_epoch) === String(this.entry.scene_epoch) &&
      String(this.baseline.baseline_id) === String(this.entry.baseline_id), '冷入场ACK前完整首基线');
    assert(!this.inventory && !this.wallet && !this.hosting && !this.ready, '冷首ACK前提前同步资产');
    this.initialBody = {...this.body(), hp_max: String(this.player().hp_max), mp_max: String(this.player().mp_max)};
  }
  async activateCold() {
    const request = this.send(ids.MsgC2SEnterWorldAck,
      {scene_epoch: this.entry.scene_epoch, baseline_id: this.entry.baseline_id});
    assert.equal((await this.result(ids.MsgS2CSyncReady, request)).error, 0);
    await until(() => this.inventory && this.wallet && this.hosting, '冷首ACK后资产同步');
    await this.auto(false);
  }
  async auto(enabled) {
    const request = this.send(ids.MsgC2SAutoBattle, {scene_epoch: this.entry.scene_epoch, enabled, monster_count: 1});
    const response = await this.result(ids.MsgS2CHostingState, request);
    assert.equal(response.error, 0, '自动战斗设置失败');
    assert.equal(response.value.auto_battle_enabled, enabled);
    await until(() => this.baselineOrdinal > response.ordinal, '自动设置后身体观察');
  }
  async cast(target) {
    const input = ++this.input;
    const sent = stamp();
    const request = this.send(ids.MsgC2SCastIntent,
      {scene_epoch: this.entry.scene_epoch, input_sequence: input, skill_id: 1, target_handle: target});
    await until(() => this.frames.some(frame =>
      frame.message === ids.MsgS2CCastAccepted && frame.value.input_sequence === input ||
      frame.request === request && frame.error), '火球受理或拒绝');
    const frame = this.frames.find(value => value.message === ids.MsgS2CCastAccepted && value.value.input_sequence === input ||
      value.request === request && value.error);
    return {frame, sent};
  }
  enemy() { return this.entities.find(value => value.entity_kind === 3 && BigInt(String(value.hp)) > 0n); }
  async injured() {
    // 关闭Auto后怪物仍按真实网格追击。既不改HP，也不授予临时受伤快照。
    await until(() => BigInt(String(this.player().hp)) <= 0n ||
      BigInt(String(this.player().hp)) < BigInt(String(this.player().hp_max)), '自然受伤前态', 29000);
    const body = this.body();
    assert(BigInt(body.hp) > 0n, '自然受伤前态已经死亡');
    return body;
  }
}

async function rpc(message, value, token = '') {
  const response = await fetch(state.account + '/rpc', {method: 'POST', signal: AbortSignal.timeout(5000),
    headers: {'Content-Type': 'application/octet-stream', ...(token ? {Authorization: 'Bearer ' + token} : {})},
    body: encode(message, 1, 0, value)});
  assert(response.ok, 'account HTTP状态异常');
  const frame = decode(await response.arrayBuffer());
  assert.equal(frame.error, 0, 'account业务拒绝 ' + frame.error);
  return frame.value;
}
async function ticket() { return (await rpc(ids.MsgC2SEntryTicket, {role: state.role.key}, state.token)).ticket; }
async function join(value) {
  const session = new RuntimeSession(state.game);
  state.sessions.push(session);
  await session.enter(value);
  return session;
}
async function joinCold() {
  const session = new RuntimeSession(state.game);
  state.sessions.push(session);
  await session.enterCold(await ticket());
  state.session = session;
  return session;
}
async function liveEnemy(session) {
  assert(session.entities.every(value => Number.isInteger(value.entity_kind) && value.entity_kind > 0),
    '实体观察缺少协议entity_kind');
  await until(() => session.enemy(), '原怪物复活', 13000);
  return session.enemy();
}
async function castSuccessfully(session) {
  const enemy = await liveEnemy(session);
  const result = await session.cast(enemy.entity_handle);
  assert.equal(result.frame.error, 0, '火球没有正常受理：' + result.frame.error);
  assert.equal(result.frame.value.skill_id, 1);
  return result;
}
async function settleObservation(session) {
  // 投射物及可能的击杀结算结束后再冻结永久观察，不把在途奖励混入重连对照。
  await sleep(1300);
  await session.heartbeat();
  return session.permanent();
}

const actions = {
  async initialize(command) {
    state.account = command.account;
    state.game = command.game;
    state.ruleVersion = command.rule_version;
    assert(/^http:\/\/127\.0\.0\.1:\d+$/.test(state.account) &&
      /^ws:\/\/127\.0\.0\.1:\d+\/ws$/.test(state.game), '探针只允许回环地址');
    assert.equal(skill.cooldown_base_ms, 1800, '本子矩阵只验当前1800ms火球CD');
    const login = await rpc(ids.MsgC2SLocalRegister, {account: 'field_' + crypto.randomBytes(6).toString('hex'),
      password: crypto.randomBytes(8).toString('hex')});
    state.token = login.account_token;
    state.role = await rpc(ids.MsgC2SCreateRole, {origin_server_id: unsigned(1), slot: 1, job: 1,
      name: '现场' + crypto.randomBytes(5).toString('hex'), op_id: id()}, state.token);
    return {registered_normally: true, created_normally: true, attached: false,
      role_id: Buffer.from(state.role.key.role_id).toString('hex'), qualification_required: {level: 7}};
  },
  async prepare_join() {
    state.session = await join(await ticket());
    const session = state.session;
    await session.auto(false);
    await sleep(650);
    const book = session.inventory.items.find(value => value.item_id === skill.book_item_id);
    assert(book, '原法师赠书不存在');
    await session.economic(ids.MsgC2SLearnSkill,
      {item_instance_id: book.instance_id, skill_id: 1, operation: session.operation()});
    assert(session.inventory.learned_skills.some(value => value.skill_id === 1), '未正常学会原书火球');
    state.claim = {benefit_kind: 0, operation: session.operation()};
    state.claimResult = await session.economic(ids.MsgC2SClaimHostingTime, state.claim);
    assert.equal(session.hosting.remain_hosting_sec, 21600);
    return {learned_original_book: true, free_six_hours_committed: true,
      initial_body: session.initialBody, body: session.body()};
  },
  async healthy_reconnect() {
    const original = state.session;
    checkpoint('等待自然受伤', original);
    const injured = await original.injured();
    checkpoint('已经自然受伤', original);
    const issued = await ticket();
    const first = await castSuccessfully(original);
    checkpoint('原火球已受理', original, {cast_id: String(first.frame.value.cast_id),
      sent_elapsed_ms: first.sent - state.diagnostic.started_ms});
    // CastAccepted先于下一个完整身体观察，先取到原场景确实已扣蓝的身体。
    await until(() => original.body().cast_sequence > injured.cast_sequence, '施法后完整身体观察');
    const actualBefore = original.body();
    checkpoint('原施法身体已观察', original);
    const oldBinding = String(original.binding);
    const session = await join(issued);
    state.session = session;
    const initial = session.initialBody;
    checkpoint('健康重连已激活', session);
    assert.notEqual(String(session.binding), oldBinding, '健康入场没有换连接附着');
    assert.equal(initial.life, actualBefore.life, '健康入场换了Life');
    assert.deepEqual(initial.position, actualBefore.position, '健康入场重置位置');
    assert.equal(initial.mp, actualBefore.mp, '健康入场补充或重复扣MP');
    assert(BigInt(initial.hp) > 0n && BigInt(initial.hp) <= BigInt(actualBefore.hp) &&
      BigInt(initial.hp) < BigInt(initial.hp_max), '健康入场重置受伤HP');
    assert.equal(initial.cast_sequence, actualBefore.cast_sequence, '健康入场重置或重复施法身份');
    assert(!session.frames.some(frame => frame.message === ids.MsgS2CCastAccepted &&
      String(frame.value.cast_id) === String(first.frame.value.cast_id)), '新连接重播原CastAccepted');
    original.close();
    await sleep(Math.max(0, first.sent + skill.action_ms + 100 - stamp()));
    const repeat = await session.cast(first.frame.value.target_handle);
    const repeatElapsed = repeat.frame.received_ms - first.sent;
    checkpoint('冷却内重试已返回', session, {repeat_error: repeat.frame.error, repeat_elapsed_ms: repeatElapsed});
    assert(repeatElapsed < skill.cooldown_base_ms, '重复施法未覆盖原CD窗口');
    assert.equal(repeat.frame.error, ids.CodeSkillCooldown, '健康重连后原CD未保留或仍在动作门禁');
    await sleep(Math.max(0, first.frame.received_ms + skill.cooldown_base_ms + 100 - stamp()));
    const expired = await castSuccessfully(session);
    checkpoint('原截止后施法已受理', session, {cast_elapsed_ms: expired.sent - first.sent});
    state.persisted = await settleObservation(session);
    return {healthy_injured_body_preserved: true, healthy_mp_preserved: true,
      original_cast_not_replayed: true, original_cooldown_preserved: true,
      cooldown_retry_error: repeat.frame.error, cooldown_retry_elapsed_ms: repeatElapsed,
      cooldown_ms: skill.cooldown_base_ms, action_ms: skill.action_ms,
      cooldown_scope: '火球单技与共享法术均为1800ms，只证明综合施法门禁保留，未分别辨识两种CD',
      post_deadline_cast_elapsed_ms: expired.sent - first.sent,
      injured_before_cast: injured, body_before_join: actualBefore, first_healthy_baseline: initial,
      permanent_sha256: signature(state.persisted)};
  },
  async prepare_crash() {
    const session = state.session;
    const injured = await session.injured();
    // 首次自动寻敌通常已经走离出生；否则通过正常点地移动一格建立位置丢失对照。
    if (samePoint(injured.position, fixture.map.player_spawn)) {
      const destination = {x: injured.position.x + 1, y: injured.position.y};
      const request = session.send(ids.MsgC2SMoveIntent, {scene_epoch: session.entry.scene_epoch,
        input_sequence: ++session.input, destination});
      await until(() => samePoint(session.player().position, destination) || session.frames.some(frame => frame.request === request && frame.error),
        '正常移动离开出生点');
      assert(samePoint(session.player().position, destination), '不能正常离开出生点');
      await sleep(700);
    }
    state.beforeCrash = session.body();
    assert(BigInt(state.beforeCrash.hp) > 0n && BigInt(state.beforeCrash.hp) < BigInt(String(session.player().hp_max)), '故障前未保持受伤活体');
    assert(BigInt(state.beforeCrash.mp) < BigInt(String(session.player().mp_max)), '故障前未保持耗蓝');
    assert(!samePoint(state.beforeCrash.position, fixture.map.player_spawn), '故障前位置未离出生');
    state.persisted = session.permanent();
    return {injured_alive: true, mana_spent: true, away_from_spawn: true,
      body: state.beforeCrash, permanent_sha256: signature(state.persisted)};
  },
  async cold_rejoin() {
    await until(() => state.session.closed, '强杀旧WS关闭');
    const session = await joinCold();
    const initial = session.initialBody;
    assert.equal(initial.hp, initial.hp_max, '冷首基线未重建满HP');
    assert.equal(initial.mp, initial.mp_max, '冷首基线未重建满MP');
    assert.deepEqual(initial.position, fixture.map.player_spawn, '冷首基线未重建出生位置');
    assert.equal(initial.cast_sequence, 0, '冷首基线恢复了旧施法身份');
    state.coldKind = 'online';
    return {cold_initial_full_hp_mp: true, temporary_position_and_cast_discarded: true,
      ack_sent: false, first_baseline: initial};
  },
  async offline_start() {
    const session = state.session;
    await session.auto(true);
    const before = session.hosting.remain_hosting_sec;
    for (const connection of state.sessions) connection.close();
    await until(() => state.sessions.every(connection => connection.closed), '全部WS断开进入真实托管');
    return {all_ws_closed: true, auto_enabled_before_disconnect: true, hosting_seconds: before};
  },
  async offline_rejoin() {
    const session = await joinCold();
    const initial = session.initialBody;
    assert.equal(initial.hp, initial.hp_max, '离线故障后未初始满HP');
    assert.equal(initial.mp, initial.mp_max, '离线故障后未初始满MP');
    assert.deepEqual(initial.position, fixture.map.player_spawn, '离线故障后未回初始出生');
    state.coldKind = 'offline';
    return {rejoined: true, ack_sent: false, initial_body: initial};
  },
  async activate_cold() {
    const session = state.session;
    assert(state.coldKind, '没有待确认冷首基线');
    await session.activateCold();
    if (state.coldKind === 'online') {
      assert.deepEqual(session.permanent(), state.persisted, '冷重建改变已确认资产');
      const revision = String(session.inventory.revision);
      const nextCommand = String(session.ready.next_economic_command_seq);
      const replay = await session.economic(ids.MsgC2SClaimHostingTime, state.claim);
      assert.equal(String(replay.new_revision), revision, '原领取重查返回的版本不是重查前版本');
      assert.equal(String(session.inventory.revision), revision, '原领取重查推进资产版本');
      assert.equal(String(session.ready.next_economic_command_seq), nextCommand, '原领取重查推进经济游标');
      assert(Buffer.from(replay.op_id).equals(Buffer.from(state.claimResult.op_id)), '原领取重查换了操作身份');
      assert.equal(replay.error_code, state.claimResult.error_code, '原领取重查换了终态');
      assert.equal(replay.pending, state.claimResult.pending, '原领取重查变成未决');
      assert.equal(String(replay.command_seq), String(state.claimResult.command_seq), '原领取重查改变原经济序号');
      assert.deepEqual(session.permanent(), state.persisted, '原领取重查重复发放');
    }
    const kind = state.coldKind;
    state.coldKind = null;
    return {kind, paused_after_join: true, confirmed_assets_preserved: true,
      original_claim_not_repeated: kind === 'online', hosting_seconds: session.hosting.remain_hosting_sec,
      permanent_sha256: signature(session.permanent())};
  },
  async finish() {
    for (const session of state.sessions) session.close();
    await sleep(100);
    return {closed: true};
  }
};

// 每次动作完成才受理下一条，失败保留原动作名供编排器终止和清理。
if (require.main === module) {
  const lines = readline.createInterface({input: process.stdin, crlfDelay: Infinity});
  let queue = Promise.resolve();
  lines.on('line', line => {
    queue = queue.then(async () => {
      let command;
      try {
        state.diagnostic = {started_ms: stamp(), samples: []};
        command = JSON.parse(line);
        assert(actions[command.action], '未知现场探针动作');
        const result = await actions[command.action](command);
        process.stdout.write(JSON.stringify({action: command.action, passed: true, result}) + '\n');
      } catch (error) {
        process.stdout.write(JSON.stringify({action: command?.action, passed: false, error: error.message,
          diagnostic: state.diagnostic ? failureDiagnostic() : null}) + '\n');
      }
    });
  });
  lines.on('close', () => queue.finally(() => {
    for (const session of state.sessions) session.close();
    setTimeout(() => process.exit(0), 100).unref();
  }));
}
