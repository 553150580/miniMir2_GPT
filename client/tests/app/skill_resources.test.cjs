const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const root=path.resolve(__dirname,'../../..'),base=path.join(root,'client/assets/resources/development');
const manifest=JSON.parse(fs.readFileSync(path.join(base,'manifest.json'),'utf8'));
test('五项新增候选资源对应原包帧号与独立时长，精神力不冒造特效',()=>{
    const expected={5:[400,570,600,600],11:[20,10,300,400],23:[1650,1660,600,1000],29:[1790,1800,600,1000]};
    for(const [id,[cast,hit,castMs,hitMs]]of Object.entries(expected)){
        const value=manifest.effects[id],package=id==='11'?'Magic2':'Magic';
        assert.equal(value.cast.frames[0],`${package}:${cast}`);assert.equal(value.hit.frames[0],`${package}:${hit}`);
        assert.equal(value.cast.duration_ms,castMs);assert.equal(value.hit.duration_ms,hitMs);
        for(const key of value.cast.frames.concat(value.hit.frames)){const frame=manifest.frames[key];assert(frame&&!frame.declared_empty);const png=fs.readFileSync(path.join(base,frame.png));assert.equal(png.readUInt32BE(16),frame.width);assert.equal(png.readUInt32BE(20),frame.height);}
    }
    assert.equal(manifest.effects['4'],undefined);
    const flight=manifest.effects['5'].projectile;assert.equal(flight.directions.length,16);flight.directions.forEach((frames,d)=>assert.deepEqual(frames,Array.from({length:6},(_,i)=>`Magic:${410+d*10+i}`)));
});
test('雷电巨大原始纵向帧保留997像素与WIL偏移，动作与旧资源保持原像素',()=>{
    const thunder=manifest.frames['Magic2:12'];assert.equal(thunder.height,997);assert.equal(thunder.offset_y,-923);
    for(const id of ['player_male','player_female']){const cast=manifest.profiles[id].layers[0].actions.casting;assert.equal(cast.count,6);assert.equal(cast.duration_ms,100);}
    for(const key of ['Magic:0','Magic:179','Magic:370'])assert(manifest.frames[key]);
    assert.equal(manifest.official_180_restoration_verified,false);
});
test('实际public投影的主动技能均有同kind完整资源，被动精神力读取独立准确阶表',()=>{
    const gameplay=JSON.parse(fs.readFileSync(path.join(base,'gameplay.json'),'utf8'));
    assert.equal(gameplay.skills.length,16);
    for(const skill of gameplay.skills){
        if(skill.active){assert.equal(manifest.effects[String(skill.id)]?.kind,skill.kind);if(skill.target_kind.endsWith('_grid'))assert.equal(manifest.effects[String(skill.id)].hit_anchor,'grid');}
    }
    const spirit=gameplay.skills.find(skill=>skill.id===4);assert.equal(spirit.kind,'spirit_sword');assert.equal(spirit.active,false);assert.equal(spirit.target_kind,'passive');assert.deepEqual(spirit.accuracy_by_level,[0,3,5,8]);
    for(const [id,activation,target]of [[7,'passive_proc','passive'],[9,'cast','enemy_direction'],[12,'toggle','attack_mode'],[25,'toggle','attack_mode'],[26,'charge','attack_charge'],[10,'cast','enemy_direction'],[24,'cast','enemy_self'],[33,'cast','enemy_grid']]){
        const row=gameplay.skills.find(skill=>skill.id===id);assert.equal(row.activation_kind,activation);assert.equal(row.target_kind,target);assert.equal(manifest.effects[String(id)].kind,row.kind);
    }
    assert.equal(gameplay.skills.find(skill=>skill.id===9).line_cells,4);
});
test('四种战士刀光按原方向及阶数组取原WIL帧，身体仍六帧Attack1，未把紧凑索引当原帧',()=>{
    for(const [id,start]of [[7,1820],[12,2190],[25,2560]])for(let rank=0;rank<4;rank++)for(let direction=0;direction<8;direction++){
        const value=manifest.effects[String(id)].attack;assert.equal(value.duration_ms,600);
        assert.deepEqual(value.levels[rank].directions[direction],Array.from({length:6},(_,frame)=>`Magic:${start+rank*90+direction*10+frame}`));
    }
    for(let direction=0;direction<8;direction++)assert.deepEqual(manifest.effects['26'].attack.directions[direction],Array.from({length:6},(_,frame)=>`Magic:${3480+direction*10+frame}`));
    const attack=manifest.profiles.player_male.layers[0].actions.attack;assert.equal(attack.raw_start,1400);assert.equal(attack.count,6);assert.equal(attack.duration_ms,100);
});
test('疾光八方向梁、雷光自身单效果、冰咆哮原起手及地表各自保留真实时长与宽高',()=>{
    const beam=manifest.effects['10'].beam;assert.equal(beam.duration_ms,600);
    assert.equal(manifest.effects['10'].beam_anchor,'origin');
    for(let direction=0;direction<8;direction++)assert.deepEqual(beam.directions[direction],Array.from({length:6},(_,frame)=>`Magic:${970+direction*20+frame}`));
    assert.equal(manifest.effects['10'].projectile,undefined);assert.equal(manifest.effects['10'].hit,undefined);
    assert.equal(manifest.effects['24'].cast.frames[0],'Magic:1680');assert.equal(manifest.effects['24'].cast_anchor,'origin');assert.equal(manifest.effects['24'].hit,undefined);
    assert.equal(manifest.effects['33'].cast.frames[0],'Magic:3840');assert.equal(manifest.effects['33'].hit.frames[0],'Magic:3850');assert.equal(manifest.effects['33'].hit.frames.length,20);assert.equal(manifest.effects['33'].hit.duration_ms,1300);
    const hell=manifest.effects['9'];assert.equal(hell.cast.frames[0],'Magic:920');assert.equal(hell.cast.frames.length,10);assert.equal(hell.cast_anchor,'origin');
    assert.equal(hell.step.frames[0],'Magic:930');assert.equal(hell.step.frames.length,6);assert.equal(hell.step.duration_ms,500);assert.equal(hell.step_trigger,'authoritative_cast_step');
    const ts=require('/Applications/Cocos/Creator/3.8.8/CocosCreator.app/Contents/Resources/app.asar.unpacked/node_modules/typescript'),file=path.join(root,'client/assets/scripts/app/WorldAssets.ts');
    const output=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}}),module={exports:{}};
    new Function('require','module','exports',output.outputText)(()=>({}),module,module.exports);
    const keys=module.exports.effectFrameKeys(manifest.effects['7']);assert.equal(keys.length,192);assert(keys.includes('Magic:2165'));
    for(const id of [7,9,10,12,24,25,26,33])for(const key of module.exports.effectFrameKeys(manifest.effects[id])){
        const frame=manifest.frames[key],png=fs.readFileSync(path.join(base,frame.png));assert.equal(png.readUInt32BE(16),frame.width);assert.equal(png.readUInt32BE(20),frame.height);assert.equal(frame.png_roundtrip_verified,true);
    }
});
