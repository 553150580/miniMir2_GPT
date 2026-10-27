const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {profileHeadTop,entityHeadLabels,entityDisplayName}=require('../../../.work/ui-tests/app/EntityHeadAnchor.js');
const base=path.resolve(__dirname,'../../assets/resources'),manifest=JSON.parse(fs.readFileSync(path.join(base,'development/manifest.json'),'utf8'));
const gameplay=JSON.parse(fs.readFileSync(path.join(base,'development/gameplay.json'),'utf8')),prefab=JSON.parse(fs.readFileSync(path.join(base,'prefabs/Entity.prefab'),'utf8'));
const node=name=>prefab.find(row=>row.__type__==='cc.Node'&&row._name===name),part=(n,type)=>n._components.map(ref=>prefab[ref.__id__]).find(row=>row.__type__===type);
const body=node('Body'),name=node('Name'),hp=node('HP'),bodyTransform=part(body,'cc.UITransform'),nameTransform=part(name,'cc.UITransform'),hpTransform=part(hp,'cc.UITransform');
const template={bodyY:body._lpos.y,bodyHeight:bodyTransform._contentSize.height,bodyAnchorY:bodyTransform._anchorPoint.y,bodyScaleY:body._lscale.y,nameY:name._lpos.y,hpY:hp._lpos.y};
test('实际三类形象静态头顶66/65/75/68，名字与HP依保存模板一次整体移到头上且保持顺序间距',()=>{
    for(const [id,top]of [['player_male',66],['player_female',65],['monster_scarecrow',75],['pet_skeleton',68]]){
        const layout=entityHeadLabels(manifest.profiles[id],manifest.frames,template);
        assert.equal(layout.headTop,top);assert.equal(layout.hpY,top+10);assert.equal(layout.nameY,top+30);
        const hpTop=layout.hpY+hpTransform._contentSize.height/2,nameBottom=layout.nameY-nameTransform._contentSize.height*name._lscale.y/2;
        assert(layout.hpY-hpTransform._contentSize.height/2>top);assert(nameBottom>hpTop);assert.equal(part(name,'cc.Label')._fontSize,24);assert.equal(name._lscale.y,.7);
    }
});
test('头顶只取非空站立/行走身体头发，武器高举、施法与倒地帧不能改变稳定锚点',()=>{
    const action=key=>({directions:[{frames:[key]}]}),profile={layers:[{package:'Hum',actions:{standing:action('body'),walking:action('walk'),casting:action('cast'),die:action('die')}},{package:'Hair',actions:{standing:action('hair')}},{package:'Weapon',actions:{standing:action('weapon')}}]};
    const frames={body:{offset_y:-48},walk:{offset_y:-50},hair:{offset_y:-51},cast:{offset_y:-500},die:{offset_y:-900},weapon:{offset_y:-1200}};
    assert.equal(profileHeadTop(profile,frames),67);frames.weapon.offset_y=-9999;frames.die.offset_y=-8888;assert.equal(profileHeadTop(profile,frames),67);
    assert.throws(()=>profileHeadTop({layers:[profile.layers[2]]},frames),/缺少可核验/);
});
test('角色、怪物、宝宝显示实际中文名，宝宝按当前职业匹配，未知定义不伪造名称',()=>{
    assert.equal(entityDisplayName(1,0,'模式验收战士',0,gameplay),'模式验收战士');assert.equal(entityDisplayName(3,1,'角色',0,gameplay),'稻草人');
    assert.equal(entityDisplayName(2,2,'角色',2,gameplay),'变异骷髅');assert.equal(entityDisplayName(2,2,'角色',0,gameplay),'');assert.equal(entityDisplayName(3,999,'角色',0,gameplay),'');
});
