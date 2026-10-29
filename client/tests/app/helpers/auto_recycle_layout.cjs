const assert=require('node:assert/strict');
// rule19仅移动Settings原Help，为追加保存策略区留位置；历史HUD哈希只还原这两项显式授权变化。
module.exports=function restoreHistoricalSettingsHelp(scene){
 if(scene.length<=436)return scene;
 assert.equal(scene[435]._name,'Help');assert.equal(scene[435]._parent.__id__,425);
 assert.deepEqual(scene[435]._lpos,{__type__:'cc.Vec3',x:0,y:-385,z:0});assert.deepEqual(scene[436]._contentSize,{__type__:'cc.Size',width:220,height:36});
 scene[435]._lpos.y=0;scene[436]._contentSize.width=330;scene[436]._contentSize.height=60;return scene;
};
