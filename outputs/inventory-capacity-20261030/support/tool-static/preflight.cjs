const assert = require('node:assert/strict'), fs = require('node:fs'), path = require('node:path'), crypto = require('node:crypto');
const root = path.resolve(__dirname, '../../..'), tool = path.join(root, 'tools/client/verify_inventory_capacity_browser.cjs'), source = fs.readFileSync(tool, 'utf8');
const pb = require(path.join(root, 'client/assets/scripts/generated/protobuf/client_pb.js')).minimir.v1;
const ts = require(path.join(root, 'tools/client/creator_paths.cjs')).typescript;
const Long = require('node:module').createRequire(path.join(root, 'client/assets/scripts/app/GameSession.ts'))('long');
const file = ts.createSourceFile(tool, source, ts.ScriptTarget.ES2020, true, ts.ScriptKind.JS), checks = [];
const record = (name, work) => { work(); checks.push(name); };
record('全部8个原PB decoder存在，实际字段匹配', () => {
 const used = Array.from(new Set(Array.from(source.matchAll(/\bpb\.([A-Z]\w*)\.decode\b/g), m => m[1]))).sort(); assert.equal(used.length, 8);
 const fields = { AutoBattleIntent: ['scene_epoch', 'enabled', 'monster_count'], EnterLineResult: ['scene_epoch', 'baseline_id', 'room_id', 'resource_version'], InventoryResult: ['revision', 'items'], HostingState: ['remain_hosting_sec', 'auto_battle_enabled'], SkillAssetPreview: ['materials', 'outputs', 'rule_version', 'role_revision'], SkillAssetCommitRequest: ['preview_id'], EquipmentRecycleResult: ['source_seq', 'op_id', 'discarded_count', 'discard_reason', 'role_revision'], ExperienceGrantResult: ['raw_experience', 'accepted_experience', 'pooled_experience', 'rejected_experience', 'role_revision', 'rule_version'] };
 for (const name of used) { assert.equal(typeof pb[name]?.decode, 'function', name); for (const field of fields[name]) assert(Object.hasOwn(pb[name].prototype, field), name + '.' + field); }
});
record('原PB观察保持超过Number安全范围的Long及Source/op分支', () => {
 const large = '9007199254740993', value = Long.fromString(large, true), q = pb.EquipmentRecycleResult.decode(pb.EquipmentRecycleResult.encode({ role_revision: value, source_seq: value, discarded_count: value, discard_reason: 'bag_full', trigger: 1 }).finish());
 assert.equal(q.role_revision.toString(), large); assert.equal(q.source_seq.toString(), large); assert.equal(q.discarded_count.toString(), large); assert.equal(q.op_id.length, 0);
 const intent = pb.AutoBattleIntent.decode(pb.AutoBattleIntent.encode({ scene_epoch: value, enabled: true, monster_count: 8 }).finish()); assert.equal(intent.scene_epoch.toString(), large); assert(intent.enabled);
});
function functionNode(name) { let found; const visit = node => { if (ts.isFunctionDeclaration(node) && node.name?.text === name) found = node; ts.forEachChild(node, visit); }; visit(file); assert(found, name); return found; }
function evaluateCallback(name) { const fn = functionNode(name); let call; const visit = node => { if (ts.isCallExpression(node) && node.expression.getText(file) === 'page.evaluate') call = node; ts.forEachChild(node, visit); }; visit(fn.body); assert(call); return call.arguments[0].getText(file); }
record('工具实际geometry回调使用保存自身AR矩形，不把子树bounds当Mask', () => {
 const scene = JSON.parse(fs.readFileSync(path.join(root, 'client/assets/scenes/Game.scene'), 'utf8')), nodes = new Map(), paths = new Map();
 class Vec3 { constructor(x = 0, y = 0, z = 0) { Object.assign(this, { x, y, z }); } }
 for (let i = 0; i < scene.length; i++) if (scene[i].__type__ === 'cc.Node') nodes.set(i, { data: scene[i] });
 for (const node of nodes.values()) {
  node.parent = nodes.get(node.data._parent?.__id__); node.getChildByName = name => node.data._children.map(x => nodes.get(x.__id__)).find(x => x?.data._name === name);
  const ui = node.data._components.map(x => scene[x.__id__]).find(x => x.__type__ === 'cc.UITransform'); if (!ui) continue;
  const position = () => { let x = 0, y = 0; for (let n = node; n; n = n.parent) { assert.equal(n.data._lscale.x, 1); assert.equal(n.data._lscale.y, 1); x += n.data._lpos.x; y += n.data._lpos.y; } return { x, y }; };
  node._uiProps = { uiTransformComp: { width: ui._contentSize.width, height: ui._contentSize.height, anchorPoint: ui._anchorPoint,
   convertToWorldSpaceAR(p) { const o = position(); return { x: p.x + o.x, y: p.y + o.y }; }, convertToNodeSpaceAR(p) { const o = position(); return { x: p.x - o.x, y: p.y - o.y }; } } };
 }
 const canvas = Array.from(nodes.values()).find(x => x.data._name === 'Canvas'); const index = (node, prefix) => { const here = prefix ? prefix + '/' + node.data._name : node.data._name; paths.set(here, node); for (const child of node.data._children) if (nodes.has(child.__id__)) index(nodes.get(child.__id__), here); }; index(canvas, '');
 const host = paths.get('Canvas/Safe/UIMgr/Panels/Inventory/List'); host.getComponent = name => { assert.equal(name, 'List'); return { viewport: paths.get('Canvas/Safe/UIMgr/Panels/Inventory/List/View'), rowHeight: 72, scrollView: { getScrollOffset: () => ({ y: 517 }) } }; };
 const result = new Function('cc', 'innerWidth', 'innerHeight', 'return (' + evaluateCallback('geometry') + ')();')({ find: p => paths.get(p), Vec3 }, 720, 1280);
 assert.equal(result.view.width, 600); assert.equal(result.view.height, 414); assert.equal(result.view.top, 200); assert.equal(result.view.bottom, -214); assert.equal(result.view.bottom - result.itemLock.top, 12); assert.equal(result.offset, 517);
 for (const button of [result.itemLock, result.equip, result.recycle]) assert(result.view.bottom > button.top);
});
record('只读同事件观察器记录触屏前后offset与即时文本，不触发业务函数', () => {
 const listeners = new Map(), item = Object.freeze({ instanceId: new Uint8Array(16).fill(7), itemId: 900001 }), app = new Proxy({ selectedItem: item }, { set() { throw Error('禁止改AppRoot'); } });
 const list = Object.freeze({ node: { on(event, callback, _context, capture) { listeners.set(event, { callback, capture }); } }, scrollView: { getScrollOffset: () => ({ y: 517 }) } });
 const button = Object.freeze({ getChildByName(name) { assert.equal(name, 'Label'); return { getComponent: () => ({ string: '合并书页' }) }; }, getComponent: () => ({ interactable: true }) });
 const win = {}, cc = { Node: { EventType: { TOUCH_END: 'touch-end' } }, Label: class {}, Button: class {}, director: { getScene: () => ({ getComponentsInChildren: () => [app] }) }, find(p) { return p.endsWith('/List') ? { getComponent: () => list } : button; } };
 new Function('cc', 'window', 'return (' + evaluateCallback('installSelectionObserver') + ')();')(cc, win);
 assert.equal(listeners.get('touch-end').capture, true); listeners.get('touch-end').callback(); listeners.get('list-item-click').callback(0, item);
 const event = win.__inventoryCapacitySelections[0]; assert.equal(event.label, '合并书页'); assert(event.interactable); assert.equal(event.touchBefore.offset, 517); assert.equal(event.offset, 517); assert.equal(event.id, event.selectedId);
});
record('工具保持正常触屏、同水位Long判断及凭据脱敏边界', () => {
 assert(!source.includes('getComponent(cc.UITransform)')); assert(!/\bnew\s+(?:cc\.)?Node\s*\(/.test(source)); assert(!/\.scrollTo(?:Index|Offset)\s*\(/.test(source)); assert(!/\.refreshGame\s*\(|\.emit\s*\(/.test(source));
 assert(source.includes('BigInt(event.revision) > watermark')); assert(source.includes('offlineChargingVerifiedByThisGui: false')); assert(source.includes('redact(JSON.stringify(report'));
 const old = fs.readFileSync(path.join(root, 'tools/client/verify_level_growth_browser.cjs')), archived = fs.readFileSync(path.join(root, '.work/level-growth-20261029/browser/run-08/tool-at-start.cjs')); assert(old.equals(archived), '历史成长工具不得被新工具覆盖');
});
console.log(JSON.stringify({ passed: checks.length, checks, tool_sha256: crypto.createHash('sha256').update(source).digest('hex'), scope: '纯工具PB/静态保存几何与只读回调替身预检，不是Creator或真实GUI' }, null, 2));
