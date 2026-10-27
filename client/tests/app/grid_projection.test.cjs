const {test}=require('node:test');
const assert=require('node:assert/strict');
const {GridProjection,LOGICAL_DIRECTIONS,compareProjectedDepth}=require('../../../.work/ui-tests/app/GridProjection.js');
const {beamGeometry}=require('../../../.work/ui-tests/app/BeamGeometry.js');
const {miniMapPoint}=require('../../../.work/ui-tests/app/HudModel.js');
const geometry={projection:'isometric_diamond',width:17,height:17,tile_width:64,tile_height:32,soil_thickness_px:48};
const projection=new GridProjection(geometry),close=(a,b)=>assert(Math.abs(a-b)<1e-8,`${a} != ${b}`);
test('17×17菱形所有289格正逆一致，地板1088×544、土层48且整体中心分配正确',()=>{
    assert.equal(projection.surfaceWidth,1088);assert.equal(projection.surfaceHeight,544);assert.equal(projection.worldHeight,592);assert.equal(projection.floorCenterY,24);
    assert.deepEqual(projection.project(8,8),{x:0,y:24});assert.deepEqual(projection.corners(),[{x:0,y:296},{x:544,y:24},{x:0,y:-248},{x:-544,y:24}]);
    for(let x=0;x<17;x++)for(let y=0;y<17;y++){
        assert.deepEqual(projection.cellAt(projection.project(x,y)),{x,y});
        for(const [dx,dy]of [[-.49,-.49],[.49,.49],[.23,-.31]]){const original={x:x+dx,y:y+dy},inverse=projection.unproject(projection.project(original.x,original.y));close(inverse.x,original.x);close(inverse.y,original.y);assert.deepEqual(projection.cellAt(projection.project(original.x,original.y)),{x,y});}
    }
});
test('拒绝包围盒内而菱形外的四角和土层，边缘有效格仍可点击，不把土层当导航地面',()=>{
    for(const point of [{x:500,y:250},{x:-500,y:250},{x:500,y:-230},{x:-500,y:-230},{x:0,y:-280},{x:0,y:320},{x:NaN,y:0}])assert.equal(projection.cellAt(point),null);
    for(const point of [{x:16,y:0},{x:0,y:16},{x:0,y:0},{x:16,y:16}])assert.deepEqual(projection.cellAt(projection.project(point.x,point.y)),point);
});
test('身体武器刀光统一逻辑8→视觉8，摇杆屏幕8向反转正确，投射16向来自实际投影向量',()=>{
    const screens=[[0,1],[1,1],[1,0],[1,-1],[0,-1],[-1,-1],[-1,0],[-1,1]];
    screens.forEach(([x,y],index)=>assert.equal(projection.logicalDirectionFromScreen(x,y),(index+7)%8));
    LOGICAL_DIRECTIONS.forEach((delta,direction)=>{
        assert.equal(projection.visualDirection(direction),(direction+1)%8);
        const vector=projection.delta(delta.x,delta.y);assert.equal(projection.logicalDirectionFromScreen(vector.x,vector.y),direction);
        assert.equal(projection.visualDirectionBetween({x:8,y:8},{x:8+delta.x,y:8+delta.y},16),[3,4,5,8,11,12,13,0][direction]);
    });
    assert.equal(projection.logicalDirectionFromScreen(0,0),null);assert.equal(projection.visualDirectionBetween({x:8,y:8},{x:8,y:8},16,6),13);
});
test('树石与移动实体按同一投影脚底排序，图像高度和原像素校正不改变前后关系',()=>{
    const row=(id,x,y,layer)=>({id,layer,...projection.project(x,y)}),tree=row('tree',8,8,0),back=row('entity_back',7,7,1),front=row('entity_front',9,9,1),same=row('entity_same',8,8,1);
    assert.deepEqual([front,tree,back,same].sort(compareProjectedDepth).map(value=>value.id),['entity_back','tree','entity_same','entity_front']);
    assert.deepEqual(miniMapPoint({x:8,y:8},geometry,136,68),{x:0,y:0});assert.deepEqual(miniMapPoint({x:16,y:0},geometry,136,68),{x:64,y:0});
    assert.deepEqual(miniMapPoint({x:0,y:16},geometry,136,68),{x:-64,y:0});assert.deepEqual(miniMapPoint({x:0,y:0},geometry,136,68),{x:0,y:32});
});
test('疾光原PNG及偏移刚体旋转到新六格轴，原图尺寸不变并以精确端点作Mask，不逐帧PCA或拉伸',()=>{
    const vectors=[[0,32],[48,32],[48,0],[48,-32],[0,-32],[-48,-32],[-48,0],[-48,32]];
    const frames=[{width:240,height:180,offset_x:-24,offset_y:-100}],snapshot=JSON.stringify(frames);
    LOGICAL_DIRECTIONS.forEach((delta,direction)=>{
        const layout=beamGeometry(projection,direction,6,vectors,frames),end=projection.delta(delta.x*6,delta.y*6);
        close(layout.length,Math.hypot(end.x,end.y));close(Math.cos(layout.angleDegrees*Math.PI/180)*layout.length,end.x);close(Math.sin(layout.angleDegrees*Math.PI/180)*layout.length,end.y);
        close(((layout.angleDegrees+layout.spriteAngleDegrees+180)%360+360)%360-180,layout.rigidRotationDegrees);
        if(direction%2===0)close(Math.abs(layout.rigidRotationDegrees),7.125016348901795);else close(layout.rigidRotationDegrees,0);
        assert(layout.height>0);assert.deepEqual(frames[0],JSON.parse(snapshot)[0]);
    });
});
