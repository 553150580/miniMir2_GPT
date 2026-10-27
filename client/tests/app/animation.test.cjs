const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const ts=require('/Applications/Cocos/Creator/3.8.8/CocosCreator.app/Contents/Resources/app.asar.unpacked/node_modules/typescript');
const file=path.resolve(__dirname,'../../assets/scripts/app/EntityAnimation.ts');
const output=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}});
const loaded={exports:{}};new Function('module','exports',output.outputText)(loaded,loaded.exports);
const {EntityAnimation}=loaded.exports;
const definitions={standing:{count:4,duration_ms:500},walking:{count:6,duration_ms:100},attack:{count:6,duration_ms:100},struck:{count:3,duration_ms:100},die:{count:4,duration_ms:100}};
const idle={moving:false,direction:4,stepProgress:1};
const state=(changes={})=>({direction:4,action:0,attackSequence:0,dead:false,injured:false,...changes});
const controller=()=>{const value=new EntityAnimation(definitions);value.observe(state(),0);return value;};

test('八方向施法按独立序号完整播放6×100毫秒，重复快照与受击不重启',()=>{
    for(let direction=0;direction<8;direction++){
        const value=new EntityAnimation({...definitions,casting:{count:6,duration_ms:100}});
        value.observe(state({castSequence:0}),0);value.observe(state({action:5,castSequence:1,direction}),100);
        for(let frame=0;frame<6;frame++){
            if(frame)value.observe(state({action:3,castSequence:1,direction:(direction+2)%8,injured:true}),100+100*frame);
            assert.deepEqual(value.sample(100+100*frame,idle),{action:'casting',direction,frame});
        }
        assert.equal(value.playbackUntil,700);assert.equal(value.sample(700,idle).action,'standing');
        value.observe(state({castSequence:2,direction}),800);assert.equal(value.sample(800,idle).action,'casting');
    }
});
test('健康重连的历史施法序号不补播，仍在施法的首次快照可显示当前动作',()=>{
    const value=new EntityAnimation({...definitions,casting:{count:6,duration_ms:100}});
    value.observe(state({castSequence:100}),1000);assert.equal(value.sample(1000,idle).action,'standing');
    value.observe(state({castSequence:100}),1200);assert.equal(value.sample(1200,idle).action,'standing');
    const active=new EntityAnimation({...definitions,casting:{count:6,duration_ms:100}});
    active.observe(state({castSequence:100,action:5,direction:7}),1000);
    assert.deepEqual(active.sample(1000,idle),{action:'casting',direction:7,frame:0});
});
test('权威AttackAccepted可立即排队身体动作，后续同序号基线不再重播',()=>{
    const value=controller();value.acceptAttack(1,6);assert.deepEqual(value.sample(100,idle),{action:'attack',direction:6,frame:0});
    value.observe(state({attackSequence:1,direction:2}),300);assert.deepEqual(value.sample(300,idle),{action:'attack',direction:6,frame:2});
    value.acceptAttack(1,2);assert.equal(value.sample(700,idle).action,'standing');
    value.observe(state({attackSequence:1,dead:true}),800);value.acceptAttack(2,2);assert.equal(value.sample(800,idle).action,'die');
});

test('攻击独立序号能看见同快照被受击覆盖的攻击，后续idle不会截断六帧',()=>{
    const value=controller();value.observe(state({action:3,attackSequence:1,injured:true}),100);
    assert.equal(value.sample(100,idle).action,'attack');
    for(let frame=0;frame<6;frame++){
        value.observe(state({direction:2,attackSequence:1}),100+frame*100);
        assert.deepEqual(value.sample(100+frame*100,idle),{action:'attack',direction:4,frame});
    }
    assert.equal(value.sample(700,idle).action,'standing');
});
test('重复攻击序号不重播，新攻击序号即使action相同也能再次起播',()=>{
    const value=controller();value.observe(state({action:2,attackSequence:1}),0);
    value.sample(0,idle);value.observe(state({action:2,attackSequence:1}),300);
    assert.equal(value.sample(300,idle).frame,3);
    assert.equal(value.sample(700,idle).action,'standing');
    value.observe(state({action:2,attackSequence:2}),1000);
    assert.deepEqual(value.sample(1000,idle),{action:'attack',direction:4,frame:0});
});
test('健康重连首次看到历史攻击序号但standing不会补播旧攻击',()=>{
    const value=new EntityAnimation(definitions);value.observe(state({attackSequence:100}),1000);
    assert.equal(value.sample(1000,idle).action,'standing');
    value.observe(state({attackSequence:100}),1200);assert.equal(value.sample(1200,idle).action,'standing');
});
test('走路六帧由一步进度采样，1000或1500毫秒的步长不在中途反复重启',()=>{
    const value=controller();
    for(const duration of [600,1000,1500])for(let frame=0;frame<6;frame++){
        const progress=(frame+.1)/6;
        const pose=value.sample(duration*progress,{moving:true,direction:2,stepProgress:progress});
        assert.deepEqual(pose,{action:'walking',direction:2,frame});
    }
});
test('走到落脚点后完整播放排队攻击，方向切换保留行走相位',()=>{
    const value=controller();value.observe(state({attackSequence:1}),100);
    assert.equal(value.sample(100,{moving:true,direction:2,stepProgress:.5}).frame,3);
    assert.equal(value.sample(200,{moving:true,direction:4,stepProgress:.7}).frame,4);
    assert.equal(value.sample(600,idle).action,'attack');assert.equal(value.sample(900,idle).frame,3);
});
test('受击完整播放三帧，不因后续standing重复快照重置或截短',()=>{
    const value=controller();value.observe(state({action:3,injured:true}),100);
    assert.equal(value.sample(100,idle).action,'struck');
    value.observe(state(),200);assert.equal(value.sample(200,idle).frame,1);
    assert.equal(value.sample(300,idle).frame,2);assert.equal(value.sample(400,idle).action,'standing');
});
test('死亡优先并停在末帧，复活才离开尸体；首次尸体不重复倒下',()=>{
    const value=controller();value.observe(state({dead:true}),100);
    assert.equal(value.sample(100,idle).action,'die');
    assert.equal(value.sample(300,idle).frame,2);assert.equal(value.sample(10000,idle).frame,3);
    value.observe(state({dead:false}),10001);assert.equal(value.sample(10001,idle).action,'standing');
    const corpse=new EntityAnimation(definitions);corpse.observe(state({dead:true}),0);
    assert.deepEqual(corpse.sample(0,idle),{action:'die',direction:4,frame:3});
});
test('已开始的攻击不被新移动截断，死亡在已知步落脚后倒下而不滑尸体',()=>{
    const value=controller();value.observe(state({attackSequence:1}),100);value.sample(100,idle);
    assert.equal(value.playbackUntil,700);
    assert.deepEqual(value.sample(300,{moving:true,direction:2,stepProgress:0}),{action:'attack',direction:4,frame:2});
    assert.equal(value.sample(700,{moving:true,direction:2,stepProgress:0}).action,'walking');
    value.observe(state({attackSequence:1,dead:true}),800);
    assert.equal(value.sample(800,{moving:true,direction:2,stepProgress:.5}).action,'walking');
    assert.equal(value.sample(1000,idle).action,'die');
    assert.equal(value.sample(1200,idle).frame,2);assert.equal(value.sample(3000,idle).frame,3);
});
test('后台恢复按绝对时钟结束旧动作，不把一帧误当一段动画时长',()=>{
    const value=controller();value.observe(state({attackSequence:1}),100);value.sample(100,idle);
    assert.equal(value.sample(10000,idle).action,'standing');
    assert.equal(value.sample(10500,idle).frame,1);
});
