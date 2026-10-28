const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const ts=require('../../../tools/client/creator_paths.cjs').typescript;
function load(relative){const source=fs.readFileSync(path.resolve(__dirname,'../../assets/scripts',relative+'.ts'),'utf8'),result=ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}}),module={exports:{}};new Function('module','exports',result.outputText)(module,module.exports);return module.exports;}
const {EntityDisplacement}=load('app/EntityDisplacement'),{EntityMotion}=load('app/EntityMotion'),{EntityAnimation}=load('app/EntityAnimation'),{NavigationIntent}=load('input/NavigationIntent');
const point=(x,y=0)=>({x,y}),target=(x,y=0)=>({x,y,direction:2,durationMs:600});
const event=(changes={})=>({sceneEpoch:'1',entityHandle:1,entityLife:'0',movementRevision:'1',kind:2,origin:point(0),path:[point(1)],facingDirection:6,movementDirection:2,stepDurationMs:300,sourceSkillId:8,castId:'0',attackSequence:0,castSequence:0,...changes});
const definitions={standing:{count:4,duration_ms:500},walking:{count:6,duration_ms:100},running:{count:6,duration_ms:100},attack:{count:6,duration_ms:100},casting:{count:6,duration_ms:100},struck:{count:3,duration_ms:100},die:{count:4,duration_ms:100}};
const observed={direction:2,action:0,attackSequence:0,castSequence:0,dead:false,injured:false},idle={moving:false,direction:2,stepProgress:1};
const near=(actual,expected)=>assert(Math.abs(actual-expected)<1e-8,`${actual} != ${expected}`);
function motion(){const value=new EntityMotion();value.push(target(0),0);return value;}

test('位移按scene/handle/life/revision精确去重，大于2^53的水位不经Number',()=>{
 const tracker=new EntityDisplacement('18446744073709551615',1);assert.equal(tracker.observe('0','9007199254740992',0,point(0)),'initial');
 const value=event({sceneEpoch:'18446744073709551615',movementRevision:'9007199254740993'});assert.equal(tracker.accept(value).recover,false);
 assert.equal(tracker.accept(value),null);assert.equal(tracker.accept({...value,movementRevision:'9007199254740992'}),null);
 assert.equal(tracker.accept({...value,movementRevision:'9007199254740994',entityLife:'1'}),null);
 assert.equal(tracker.accept({...value,movementRevision:'9007199254740994',sceneEpoch:'1'}),null);
 assert.equal(tracker.accept({...value,movementRevision:9007199254740994}),null);
 assert.equal(tracker.observe('0','9007199254740993',2,point(1)),'same');
 assert.equal(tracker.observe('0','9007199254740992',0,point(0)),'stale');assert.deepEqual(tracker.position,point(1));
});
test('事件先到保留路径，同水位基线不重启；基线先到与健康重入只恢复落点',()=>{
 const first=new EntityDisplacement('1',1);first.observe('0','0',0,point(0));assert(first.accept(event()));assert.equal(first.observe('0','1',2,point(1)),'same');
 const baselineFirst=new EntityDisplacement('1',1);baselineFirst.observe('0','0',0,point(0));assert.equal(baselineFirst.observe('0','1',2,point(1)),'recover');assert.equal(baselineFirst.accept(event()),null);
 const rejoined=new EntityDisplacement('1',1);assert.equal(rejoined.observe('0','1',2,point(1)),'initial');assert.equal(rejoined.accept(event()),null);
 assert.equal(first.observe('0','2',1,point(2)),'walk');assert.equal(first.accept(event({movementRevision:'2'})),null);
});
test('只消费真实相邻path，断层恢复末格；相邻/原地传送显式成立，失败不造路径',()=>{
 const tracker=new EntityDisplacement('1',1);tracker.observe('0','0',0,point(0));
 for(const invalid of [event({path:[point(2)]}),event({path:[]}),event({stepDurationMs:0}),event({facingDirection:8}),event({kind:4,path:[]}),event({kind:5,path:[point(1)]})])assert.equal(tracker.accept(invalid),null);
 assert.equal(tracker.accept(event({origin:point(5),path:[point(6)]})).recover,true);assert.deepEqual(tracker.position,point(6));
 assert(tracker.accept(event({movementRevision:'2',kind:4,origin:point(6),path:[point(6)],stepDurationMs:0})));
 assert(tracker.accept(event({movementRevision:'3',kind:5,origin:point(6),path:[],stepDurationMs:0})));
});
test('Pushed面向与运动向量分开，单格按Walking5/3/1三个100ms播放',()=>{
 const value=motion(),animation=new EntityAnimation(definitions);animation.observe(observed,0);animation.displace(0,0);value.displace(event(),0,true);
 for(const [now,index]of [[0,5],[100,3],[200,1]]){const frame=value.sample(now);near(frame.x,now/300);assert.equal(frame.direction,6);assert.deepEqual(animation.sample(now,frame),{action:'walking',direction:6,frame:index});}
 const end=value.sample(300);assert.equal(end.x,1);assert.equal(end.moving,false);assert.equal(animation.sample(300,end).action,'standing');
});
test('更高强制路径追加已确认前缀和拐点，后续普通步不重排强制轨迹',()=>{
 const value=motion();value.displace(event(),0,true);const before=value.sample(100);
 value.displace(event({origin:point(1),path:[point(1,1)],facingDirection:0}),100,true);near(value.sample(100).x,before.x);
 value.push(target(2,1),150);const corner=value.sample(300);near(corner.x,1);near(corner.y,0);assert.equal(corner.direction,0);
 near(value.sample(450).x,1);near(value.sample(450).y,.5);
 const next=value.sample(600);near(next.x,1);near(next.y,1);assert.equal(next.movementKind,1);
 near(value.sample(900).x,1.5);assert.equal(value.sample(1200).x,2);assert.equal(value.sample(1800).x,2);
});
test('Dash每格三帧L/R交替；本人沿当前与待播续相，旁观者只按待播尾相',()=>{
 for(const owner of [true,false]){const value=motion(),animation=new EntityAnimation(definitions);animation.observe(observed,0);value.displace(event({kind:3,path:[point(1),point(2)]}),0,owner);
  for(const [now,index]of [[0,0],[100,1],[200,2],[300,3],[400,4],[500,5]]){const frame=value.sample(now);assert.equal(animation.sample(now,frame).frame,index);assert.equal(animation.sample(now,frame).action,'running');}
 }
 for(const [owner,expected]of [[true,1],[false,0]]){const value=motion();value.displace(event({kind:3}),0,owner);value.sample(100);value.displace(event({kind:3,origin:point(1),path:[point(2)]}),100,owner);assert.equal(value.sample(300).dashPhase,expected);}
 const queued=motion();queued.displace(event({kind:3}),0,false);queued.displace(event({kind:3,origin:point(1),path:[point(2)]}),0,false);assert.equal(queued.sample(300).dashPhase,1);
});
test('新force保留前一次force之后已确认的普通转角，不跳过夹在中间的normal点',()=>{
 const value=motion();value.displace(event(),0,true);value.push(target(1,1),100);
 value.displace(event({origin:point(1,1),path:[point(2,1)],facingDirection:2}),200,true);
 const firstCorner=value.sample(300);near(firstCorner.x,1);near(firstCorner.y,0);
 near(value.sample(600).x,1);near(value.sample(600).y,.5);
 const secondCorner=value.sample(900);near(secondCorner.x,1);near(secondCorner.y,1);
 near(value.sample(1050).x,1.5);near(value.sample(1050).y,1);assert.equal(value.sample(1200).x,2);
});
test('强制前缀不加速，后续密集普通路点只在末尾有界追赶并经过每个转角',()=>{
 const value=motion();value.displace(event({path:[point(1),point(2)]}),0,true);
 value.push(target(2,1),100);value.push(target(3,1),300);
 near(value.sample(450).x,1.5);near(value.sample(450).y,0);
 const endForce=value.sample(600);near(endForce.x,2);near(endForce.y,0);
 const corner=value.sample(700);near(corner.x,2);near(corner.y,1);
 const end=value.sample(800);near(end.x,3);near(end.y,1);assert.equal(end.moving,false);
});
test('旧攻击/施法与hold按水位失效，较新pending身体动作保留到强制落脚后',()=>{
 const value=motion(),animation=new EntityAnimation(definitions);animation.observe(observed,0);animation.acceptAttack(4,2);assert.equal(animation.sample(0,idle).action,'attack');value.holdUntil(600);
 animation.displace(4,5);value.displace(event({attackSequence:4,castSequence:5}),100,true);assert.equal(animation.playbackUntil,0);near(value.sample(250).x,.5);
 animation.acceptAttack(4,2);animation.observe({...observed,attackSequence:4,castSequence:5,action:5},200);assert.equal(animation.sample(400,value.sample(400)).action,'standing');
 const newer=new EntityAnimation(definitions);newer.observe(observed,0);newer.acceptAttack(8,4);newer.sample(0,idle);newer.displace(4,5);assert.equal(newer.playbackUntil,0);
 assert.equal(newer.sample(100,{moving:true,direction:6,stepProgress:.3,movementKind:2}).action,'walking');assert.deepEqual(newer.sample(400,idle),{action:'attack',direction:4,frame:0});assert.equal(newer.currentAttackSequence,8);
 const cast=new EntityAnimation(definitions);cast.observe(observed,0);cast.observe({...observed,castSequence:8,action:5,direction:7},10);cast.sample(10,idle);cast.displace(4,5);assert.deepEqual(cast.sample(400,idle),{action:'casting',direction:7,frame:0});
});
test('显式原地/相邻传送清旧步，失败无2秒锁，积压超限只做权威恢复',()=>{
 for(const end of [point(1),point(2)]){const value=motion();value.push(target(1),0);value.holdUntil(600);value.displace(event({kind:4,origin:point(1),path:[end]}),100,true);const frame=value.sample(100);assert.deepEqual([frame.x,frame.y,frame.moving],[end.x,end.y,false]);assert.equal(value.push(target(end.x+1),200).moving,true);}
 const failed=motion();failed.holdUntil(2000);failed.displace(event({kind:5,path:[]}),100,true);assert.equal(failed.push(target(1),200).moving,true);
 const limited=new EntityMotion({maxQueuedWaypoints:2});limited.push(target(0),0);limited.displace(event({path:[point(1),point(2),point(3)]}),0,true);assert.equal(limited.sample(0).x,3);assert.equal(limited.sample(0).moving,false);
});
test('复活清旧life位移，但地面目标继续；面板/后台释放不代替权威到达',()=>{
 const nav=new NavigationIntent(),tracker=new EntityDisplacement('1',1);nav.setGroundTarget(point(5));nav.bindGroundInput(2);nav.observeAuthority({inputSequence:2,destination:point(5),status:1,revision:'1'});
 tracker.observe('0','0',0,point(0));tracker.accept(event());assert.equal(tracker.observe('1','0',0,point(5)),'initial');assert.equal(tracker.accept(event({movementRevision:'2'})),null);
 nav.releaseJoystick();assert.deepEqual(nav.state.target,point(5));assert.equal(nav.observeAuthority({inputSequence:2,destination:point(5),status:2,revision:'1'}),true);assert.equal(nav.state.target,null);
});
test('同坐标新目标与健康重入复用seq由64位导航revision下界隔离',()=>{
 const nav=new NavigationIntent(),reasons=[];nav.subscribe((_state,reason)=>reasons.push(reason));
 const base={inputSequence:1,destination:point(5),status:1,revision:'9007199254740993'};assert.equal(nav.observeAuthority(base),true);assert.deepEqual(reasons,['restore']);
 nav.setGroundTarget(point(5));nav.bindGroundInput(1);assert.equal(nav.observeAuthority({...base,status:2}),false);assert.deepEqual(nav.state.target,point(5));
 const next={...base,revision:'9007199254740994'};assert.equal(nav.observeAuthority(next),true);assert.equal(nav.observeAuthority({...next,status:2}),true);assert.equal(nav.observeAuthority(next),false);
 nav.setGroundTarget(point(6));nav.bindGroundInput(2);assert.equal(nav.observeAuthority({...next,status:2}),false);assert.deepEqual(nav.state.target,point(6));
 nav.setJoystick({x:1,y:0});nav.releaseJoystick();assert.equal(nav.observeAuthority({...base,revision:'9007199254740995'}),false);assert.equal(nav.state.target,null);
});
