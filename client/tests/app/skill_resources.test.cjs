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
    for(const skill of gameplay.skills){
        if(skill.active){assert.equal(manifest.effects[String(skill.id)]?.kind,skill.kind);if(skill.target_kind.endsWith('_grid'))assert.equal(manifest.effects[String(skill.id)].hit_anchor,'grid');}
    }
    const spirit=gameplay.skills.find(skill=>skill.id===4);assert.equal(spirit.kind,'spirit_sword');assert.equal(spirit.active,false);assert.equal(spirit.target_kind,'passive');assert.deepEqual(spirit.accuracy_by_level,[0,3,5,8]);
});
