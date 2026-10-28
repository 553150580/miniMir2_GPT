const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const ts=require('../../../tools/client/creator_paths.cjs').typescript;
const root=path.resolve(__dirname,'../../..');
class Label{}class Button{static EventType={CLICK:'click'};}class List{constructor(){this.offset=0;this.bindings=[];}setItems(rows,renderer,preserve){this.rows=rows.slice();this.bindings.push(preserve);if(!preserve)this.offset=0;}}
class Node{constructor(name){this.name=name;this.children=[];this.parts=new Map();this.events=new Map();}set parent(p){this._parent=p;if(p)p.children.push(this);}get parent(){return this._parent;}getChildByName(name){return this.children.find(n=>n.name===name);}getChildByPath(p){return p.split('/').reduce((n,k)=>n?.getChildByName(k),this);}getComponent(type){return this.parts.get(type);}on(type,handler){this.events.set(type,handler);}off(type){this.events.delete(type);}destroy(){this.destroyed=true;if(this._parent)this._parent.children=this._parent.children.filter(n=>n!==this);}}
function saved(file,start){const s=JSON.parse(fs.readFileSync(file));const build=id=>{const row=s[id],n=new Node(row._name);for(const c of row._components??[]){const data=s[c.__id__];if(data.__type__==='cc.Label')n.parts.set(Label,{string:data._string});if(data.__type__==='cc.Button')n.parts.set(Button,{});if(data.__type__==='39a82802rlT3Z9OWj3G8DOj')n.parts.set(List,new List());}for(const child of row._children??[])build(child.__id__).parent=n;return n;};return build(start??s.findIndex(r=>r.__type__==='cc.Node'&&r._name==='Safe'));}
const game={totalTime:0},file=path.join(root,'client/assets/scripts/app/GameCombatStatus.ts');
const output=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}}),moduleLike={exports:{}};
new Function('require','module','exports',output.outputText)(id=>id==='cc'?{Button,game,instantiate:()=>saved(path.join(root,'client/assets/resources/prefabs/StatusBadge.prefab'),1),isValid:n=>!!n&&!n.destroyed,Label}:id==='../ui/List'?{List}:id==='./CombatStatusModel'?require('../../../.work/ui-tests/app/CombatStatusModel.js'):{},moduleLike,moduleLike.exports);
const {GameCombatStatus}=moduleLike.exports;
test('真实状态控制器绑定保存UI：0秒徽标仍在、换life点击取新实体，详情滚动和下层列表位置保留',()=>{
    const safe=saved(path.join(root,'client/assets/scenes/Game.scene')),overview=safe.getChildByPath('UIMgr/Panels/CombatStatus/List').getComponent(List),details=safe.getChildByPath('PopupView/Panels/StatusDetails/List').getComponent(List);
    const ui={top:{name:'Skills'},open(name){this.top={name};}},popup={top:null,show(name,value){this.top={name};this.value=value;}},controller=new GameCombatStatus(safe,ui,popup,{},1);
    const buff=(remainingMs=2000)=>({statusId:31,sourceSkillId:31,skillLevel:1,remainingMs,maxPhysicalDefenceBonus:'0',maxMagicDefenceBonus:'0',damageReductionPercent:30,casterHandle:1,casterLife:'0'});
    const baseline=(seq,life,statuses)=>({sceneEpoch:'9007199254740993',stateSequence:seq,entities:[{entityHandle:2,entityKind:1,life:'0',hp:'0',statuses:[{...buff(),damageReductionPercent:99}]},{entityHandle:1,entityKind:1,life,hp:'100',statuses}]});
    game.totalTime=0;controller.refresh(baseline('1','0',[buff()]),true);const bar=safe.getChildByPath('MapViewport/BuffBar'),badge=bar.children.find(n=>n.name==='StatusBadge');assert(badge);assert.equal(badge.getChildByName('Time').getComponent(Label).string,'2秒');
    overview.offset=130;badge.events.get('click')();assert.equal(popup.top.name,'StatusDetails');assert.equal(ui.top.name,'Skills');details.offset=210;
    game.totalTime=2100;controller.update();assert.equal(badge.getChildByName('Time').getComponent(Label).string,'0秒');assert.equal(badge.destroyed,undefined);assert.equal(details.offset,210);assert.equal(overview.offset,130);
    controller.refresh(baseline('2','1',[buff(0)]),true);badge.events.get('click')();assert(details.rows.some(s=>s.includes('伤害减免 30%')),'不能仍用旧life闭包打开已结束状态');
    popup.top=null;controller.refresh(baseline('3','1',[]),true);assert.equal(badge.destroyed,true);assert.equal(overview.offset,130);assert.equal(ui.top.name,'Skills');
    controller.openOverview();assert.equal(ui.top.name,'CombatStatus');controller.dispose();
});
