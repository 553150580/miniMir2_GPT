const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const crypto=require('node:crypto');
const {SourceMapModel,parseSourceMapManifest,sourceMapId,sourceAnimationKey,decodeSourceBase64,compareSourceDraws,SourceMapValidationError}=require('../../../.work/ui-tests/app/SourceMapModel.js');
const {SourceChunkCache,SourceChunkCancelledError,SourceChunkCapacityError}=require('../../../.work/ui-tests/app/SourceChunkCache.js');
const {SourceProjection}=require('../../../.work/ui-tests/app/SourceProjection.js');
const {beamGeometry}=require('../../../.work/ui-tests/app/BeamGeometry.js');
const sha='a'.repeat(64),clone=value=>JSON.parse(JSON.stringify(value));
function frame(width=96,height=64,extra={}){return{png:'frames/Tiles/frame_000000.png',width,height,offset_x:0,offset_y:0,declared_empty:false,png_sha256:sha,...extra};}
function fixture(width=48,height=16){
    const chunks=[],data={};
    for(let cy=0;cy<Math.ceil(height/16);cy++)for(let cx=0;cx<Math.ceil(width/16);cx++){
        const id=`${cx}_${cy}`,bounds=[cx*16,cy*16,Math.min(16,width-cx*16),Math.min(16,height-cy*16)];
        chunks.push({id,x:cx,y:cy,path:`chunks/chunk_${id}.json`,cell_bounds:bounds,draw_bounds:null,sha256:sha});
        data[id]={schema_version:1,id,x:cx,y:cy,cell_bounds:bounds,cells_base64:Buffer.alloc(bounds[2]*bounds[3]*12).toString('base64'),floor:[],objects:[]};
    }
    return{manifest:{schema_version:1,id:'2',width,height,tile_width:48,tile_height:32,projection:'source_rectangular',soil_thickness_px:0,source_origin:{x:0,y:0},chunk_size_cells:16,pixel_width:width*48,pixel_height:height*32,frames:{'Tiles:0':frame()},chunks,spatial_index:{bucket_width_px:768,bucket_height_px:512,buckets:{}},collision:{runtime_bits:Buffer.alloc(Math.ceil(width*height/8)).toString('base64')}},data};
}
// 测试显式提供实例的已知像素包围盒；不调用待测模块生成测试期望。
function bounds(f,id,rect,buckets){f.manifest.chunks.find(c=>c.id===id).draw_bounds=rect;for(const key of buckets){const list=f.manifest.spatial_index.buckets[key]??=[];if(!list.includes(id))list.push(id);}}
function cacheFixture(){const f=fixture();for(let cx=0;cx<3;cx++){const id=`${cx}_0`;f.data[id].floor.push([cx*16,0,0,'Tiles:0']);bounds(f,id,[cx*768,0,cx*768+96,64],[`${cx},0`]);}return f;}
function controlled(f,options={}){
    const requests=[],releases=[];
    const loader=(info,token)=>new Promise((resolve,reject)=>requests.push({id:info.id,info,token,reject,resolve:(raw=f.data[info.id],value=info.id)=>resolve({chunk:raw,value,release:()=>releases.push(value)})}));
    const cache=new SourceChunkCache({model:new SourceMapModel(f.manifest),concurrency:1,capacity:3,loader,...options});
    return{cache,requests,releases};
}
const flush=()=>new Promise(resolve=>setImmediate(resolve));

test('700×700来源坐标完整投影、右下半开边界及uint64地图ID保真',()=>{
    const f=fixture(700,700);f.manifest.id='18446744073709551615';const model=new SourceMapModel(f.manifest);
    assert.equal(model.manifest.id,'18446744073709551615');assert.equal(model.manifest.chunks.length,1936);
    for(let y=0;y<700;y++)for(let x=0;x<700;x++){
        const pixel=model.project(x,y);assert.deepEqual(pixel,{x:x*48,y:y*32});assert.deepEqual(model.unproject(pixel),{x,y});
        assert.deepEqual(model.cellAt({x:pixel.x+47.99,y:pixel.y+31.99}),{x,y});
    }
    for(const p of [{x:-.001,y:0},{x:0,y:-1},{x:33600,y:0},{x:0,y:22400},{x:NaN,y:0}])assert.equal(model.cellAt(p),null);
    assert.deepEqual(model.unproject(model.project(650.25,631.75)),{x:650.25,y:631.75});
    assert.equal(sourceMapId(9007199254740993n),'9007199254740993');
    for(const id of [2,9007199254740992,'02','-1','18446744073709551616',null])assert.throws(()=>sourceMapId(id),SourceMapValidationError);
});
test('原生SourceProjection为现有实体/特效提供格中心、矩形点击与8/16方向；疾光无多余旋转',()=>{
    const model=new SourceMapModel(fixture(700,700).manifest),projection=new SourceProjection(model),directions=[[0,-1],[1,-1],[1,0],[1,1],[0,1],[-1,1],[-1,0],[-1,-1]];
    assert.equal(projection.geometry.projection,'source_rectangular');assert.equal(projection.surfaceWidth,33600);assert.equal(projection.worldHeight,22400);
    for(const [x,y]of [[0,0],[699,699],[650,631],[289,618],[331,267],[650.25,631.75]]){const pixel=projection.project(x,y);assert.deepEqual(pixel,{x:x*48+24,y:-y*32-16});assert.deepEqual(projection.unproject(pixel),{x,y});assert.deepEqual(projection.cellAt(pixel),{x:Math.floor(x+.5),y:Math.floor(y+.5)});}
    for(const point of [{x:-1,y:0},{x:33600,y:0},{x:0,y:1},{x:0,y:-22400}])assert.equal(projection.cellAt(point),null);
    const vectors=[[0,32],[48,32],[48,0],[48,-32],[0,-32],[-48,-32],[-48,0],[-48,32]];
    directions.forEach(([x,y],direction)=>{const delta=projection.delta(x,y);assert.equal(projection.visualDirection(direction),direction);assert.equal(projection.logicalDirectionFromScreen(delta.x,delta.y),direction);const beam=beamGeometry(projection,direction,6,vectors,[{width:400,height:220,offset_x:-20,offset_y:-160}]);assert(Math.abs(beam.rigidRotationDegrees)<1e-8);});
    assert.equal(projection.visualDirectionBetween({x:0,y:0},{x:1,y:-1},16),3);assert.equal(projection.logicalDirectionFromScreen(0,0),null);
});
test('小地图可选元数据只接受存在的非空同package/raw_frame帧，不默认另一张图',()=>{
    const f=fixture(16,16);assert.equal(parseSourceMapManifest(f.manifest).minimap,undefined);
    f.manifest.frames['mmap:100']=frame(196,166,{package:'mmap',raw_frame:100});f.manifest.minimap={package:'mmap',raw_frame:100,frame:'mmap:100'};
    assert.deepEqual(parseSourceMapManifest(f.manifest).minimap,f.manifest.minimap);
    for(const change of [m=>m.minimap=null,m=>m.minimap.frame='mmap:101',m=>m.minimap.package='other',m=>m.frames['mmap:100'].declared_empty=true,m=>m.frames['mmap:100'].raw_frame=101]){const m=clone(f.manifest);change(m);assert.throws(()=>parseSourceMapManifest(m),SourceMapValidationError);}
});
test('碰撞按全图y*width+x低位先查询，静态与关闭门按位或，不用当前裁片替代',()=>{
    const f=fixture(17,17),bits=Buffer.alloc(37),doors=Buffer.alloc(37);
    for(const index of [0,7,8,16,17,288])bits[Math.floor(index/8)]|=1<<(index%8);
    doors[2]|=4;const runtime=Buffer.from(bits.map((b,i)=>b|doors[i]));
    Object.assign(f.manifest.collision,{static_bits:bits.toString('base64'),closed_door_bits:doors.toString('base64'),runtime_bits:runtime.toString('base64'),bit_count:289,encoding:'row_major_lsb0_blocked_bitset_base64'});
    const model=new SourceMapModel(f.manifest);
    for(let y=0;y<17;y++)for(let x=0;x<17;x++)assert.equal(model.isWalkable(x,y),![0,7,8,16,17,18,288].includes(y*17+x));
    for(const p of [[-1,0],[17,0],[0,17],[.5,1],[NaN,0]])assert.equal(model.isWalkable(...p),false);
    f.manifest.collision.runtime_bits=bits.toString('base64');assert.throws(()=>new SourceMapModel(f.manifest),/static OR/);
});
test('987像素高前景来自下方owner块，视口通过空间桶找到且不误按cell块裁剪',()=>{
    const f=fixture(32,48);f.manifest.frames['Objects:1']=frame(100,987);
    f.data['1_2'].objects.push([16,32,4,'Objects:1',0,0,0,0,0,0]);
    bounds(f,'1_2',[768,69,868,1056],['1,0','1,1','1,2']);
    const model=new SourceMapModel(f.manifest),chunk=model.parseChunk(f.data['1_2'],'1_2');
    assert.deepEqual(model.queryChunks([768,69,780,70]).map(c=>c.id),['1_2']);
    assert.deepEqual(model.queryChunks([868,69,900,100]),[]);assert.deepEqual(model.queryChunks([768,68,780,69]),[]);
    assert.deepEqual(model.draws(chunk,0,[768,69,780,70]).map(d=>d.bounds),[[768,69,868,1056]]);
    assert.deepEqual(model.queryChunks([0,0,0,0]),[]);assert.deepEqual(model.queryChunks([-10000,-10000,100000,100000]).map(c=>c.id),['1_2']);
});
test('负桶火把偏移、十帧100ms动画和加色独立于关闭门offset',()=>{
    const f=fixture(16,16);for(let i=2723;i<2733;i++)f.manifest.frames[`Objects:${i}`]=frame(100,100,{offset_x:-51,offset_y:-113});
    f.data['0_0'].objects.push([0,0,4,'Objects:2723',10,0,1,1,7,77]);bounds(f,'0_0',[-51,-181,49,-81],['-1,-1','0,-1']);
    const model=new SourceMapModel(f.manifest),chunk=model.parseChunk(f.data['0_0']);
    assert.deepEqual(model.queryChunks([-51,-181,-50,-180]).map(c=>c.id),['0_0']);
    for(const [now,key]of [[0,2723],[99,2723],[100,2724],[999,2732],[1000,2723]]){
        const draw=model.draws(chunk,now)[0];assert.equal(draw.key,`Objects:${key}`);assert.deepEqual(draw.bounds,[-51,-181,49,-81]);assert.equal(draw.blend,1);assert.equal(draw.door_id,7);assert.equal(draw.door_offset,77);
    }
    assert.equal(sourceAnimationKey('Objects:20',3,2,299),'Objects:20');assert.equal(sourceAnimationKey('Objects:20',3,2,300),'Objects:21');assert.equal(sourceAnimationKey('Objects:20',3,2,900),'Objects:20');
    assert.equal(sourceAnimationKey('9007199254740993',2,0,100),'9007199254740994');
    for(const values of [['x',2,0,100],['0',-1,0,0],['0',2,256,0],['0',2,0,-1],['0',2,0,NaN]])assert.throws(()=>sourceAnimationKey(...values),SourceMapValidationError);
});
test('动画所有帧尺寸均参与空间桶，当前帧精确过滤视口',()=>{
    const f=fixture(16,48);f.manifest.frames['Objects:0']=frame(100,64);f.manifest.frames['Objects:1']=frame(100,987);
    f.data['0_2'].objects.push([0,32,4,'Objects:0',2,0,0,0,0,0]);bounds(f,'0_2',[0,69,100,1056],['0,0','0,1','0,2']);
    const model=new SourceMapModel(f.manifest),chunk=model.parseChunk(f.data['0_2']);
    assert.deepEqual(model.queryChunks([0,69,10,70]).map(c=>c.id),['0_2']);assert.equal(model.draws(chunk,0,[0,69,10,70]).length,0);
    assert.deepEqual(model.draws(chunk,100,[0,69,10,70]).map(d=>d.key),['Objects:1']);assert.deepEqual(model.draws(chunk,100,[0,69,0,70]),[]);
    assert.throws(()=>model.draws(chunk,NaN),SourceMapValidationError);
});
test('地板先按来源y/x/层，立体逐行x/层；普通帧忽略WIL偏移且空帧不造绘制',()=>{
    const f=fixture(16,16);f.manifest.frames['Objects:0']=frame(48,64,{offset_x:100,offset_y:-100});f.manifest.frames['Empty:0']=frame(0,0,{declared_empty:true,png:null,png_sha256:null});
    f.data['0_0'].floor=[[1,1,2,'Tiles:0'],[0,1,1,'Tiles:0'],[0,1,0,'Tiles:0'],[0,0,0,'Empty:0']];
    f.data['0_0'].objects=[[0,2,4,'Objects:0',0,0,0,0,0,0],[1,1,4,'Objects:0',0,0,0,0,0,0],[1,1,3,'Objects:0',0,0,0,0,0,0]];
    bounds(f,'0_0',[0,0,144,96],['0,0']);const model=new SourceMapModel(f.manifest),chunk=model.parseChunk(f.data['0_0']);
    const draws=model.draws(chunk,0);assert.deepEqual(draws.map(d=>[d.x,d.y,d.layer]),[[0,1,0],[0,1,1],[1,1,2],[1,1,3],[1,1,4],[0,2,4]]);
    assert.deepEqual(draws[3].bounds,[48,0,96,64]);assert.deepEqual(draws[5].bounds,[0,32,48,96]);
    assert.deepEqual(draws.slice().reverse().sort(compareSourceDraws),draws);assert.throws(()=>model.draws(f.data['0_0'],0),/未由当前地图验证/);
    assert(Object.isFrozen(chunk));assert(Object.isFrozen(chunk.objects[0]));
});
test('原格base64拒绝非规范填充、长度变化与碰撞尾位非零',()=>{
    for(let count=0;count<20;count++){const bytes=Buffer.from(Array.from({length:count},(_,i)=>i*17));assert.deepEqual(Buffer.from(decodeSourceBase64(bytes.toString('base64'),count)),bytes);}
    for(const [text,n]of [['AB==',1],['AAB=',2],['AAAA',1],['AA==\n',1],['AA=',1],['AA==',2],['',-1]])assert.throws(()=>decodeSourceBase64(text,n),SourceMapValidationError);
    const f=fixture(1,1);f.manifest.collision.runtime_bits=Buffer.from([2]).toString('base64');assert.throws(()=>new SourceMapModel(f.manifest),/尾位/);
});
test('manifest严格拒绝不完整地图、错误几何/碰撞/索引、窄化ID及资源路径越界',()=>{
    const original=cacheFixture().manifest;
    const changes=[m=>m.schema_version=2,m=>m.id=2,m=>m.tile_width=64,m=>m.soil_thickness_px=48,m=>m.source_origin.x=1,m=>m.pixel_height++,m=>m.width=48.5,m=>m.chunks.pop(),m=>m.chunks.push(m.chunks[0]),m=>m.chunks[0].cell_bounds[2]=15,m=>m.chunks[0].sha256='bad',m=>m.chunks[0].path='../escape.json',m=>m.frames['Tiles:0'].png='/root.png',m=>m.frames['Tiles:0'].offset_x=null,m=>delete m.frames['Tiles:0'].declared_empty,m=>m.collision.runtime_bits='',m=>m.spatial_index.bucket_width_px=1,m=>m.spatial_index.buckets['-0,0']=['0_0'],m=>m.spatial_index.buckets['0,0'].push('0_0'),m=>m.spatial_index.buckets['0,0'].push('unknown'),m=>m.spatial_index.buckets['0,1']=['0_0']];
    for(const change of changes){const m=clone(original);change(m);assert.throws(()=>parseSourceMapManifest(m),SourceMapValidationError);}
    const copy=clone(original),parsed=parseSourceMapManifest(copy);copy.frames['Tiles:0'].width=1;copy.chunks[0].cell_bounds[0]=100;assert.equal(parsed.frames['Tiles:0'].width,96);assert.equal(parsed.chunks[0].cell_bounds[0],0);
});
test('chunk严格验证owner、元组、原格长度、动画帧与实例索引完整性',()=>{
    const f=cacheFixture(),model=new SourceMapModel(f.manifest),original=f.data['0_0'];
    const changes=[c=>c.schema_version=2,c=>c.x=1,c=>c.cells_base64='AA==',c=>c.floor[0].push('extra'),c=>c.floor[0][0]=16,c=>c.floor[0][2]=3,c=>c.floor[0][3]='unknown',c=>c.floor.push(c.floor[0]),c=>c.objects.push([1,0,4,'Tiles:0',2,0,0,0,0,0]),c=>c.objects.push([1,0,4,'Tiles:0',0,0,2,0,0,0])];
    for(const change of changes){const c=clone(original);change(c);assert.throws(()=>model.parseChunk(c,'0_0'),SourceMapValidationError);}
    assert.throws(()=>model.parseChunk(original,'1_0'),/请求身份/);
    const bad=clone(f.manifest);delete bad.spatial_index.buckets['0,0'];assert.throws(()=>new SourceMapModel(bad).parseChunk(original),/空间桶/);
    const wrongBounds=clone(f.manifest);wrongBounds.chunks[0].draw_bounds[2]++;assert.throws(()=>new SourceMapModel(wrongBounds).parseChunk(original),/绘制范围/);
});
test('重复请求共用加载，释放一个等待者不取消其它引用，就绪缓存持有至销毁',async()=>{
    const {cache,requests,releases}=controlled(cacheFixture());const a=cache.acquire('0_0'),b=cache.acquire('0_0');assert.equal(requests.length,1);
    a.release();a.release();await assert.rejects(a.ready,SourceChunkCancelledError);assert.equal(requests[0].token.cancelled,false);
    requests[0].resolve();const value=await b.ready,c=cache.acquire('0_0');assert.equal(await c.ready,value);assert.equal(requests.length,1);
    b.release();c.release();assert.deepEqual(releases,[]);assert.equal(cache.stats.pinned,0);assert.equal(cache.stats.ready,1);
    cache.dispose();cache.dispose();assert.deepEqual(releases,['0_0']);assert.throws(()=>cache.acquire('0_0'),SourceChunkCancelledError);
});
test('显式并发上限、取消未开始请求，不会执行其loader',async()=>{
    const {cache,requests,releases}=controlled(cacheFixture());const a=cache.acquire('0_0'),b=cache.acquire('1_0'),c=cache.acquire('2_0');
    assert.equal(requests.length,1);assert.equal(cache.stats.queued,2);b.release();await assert.rejects(b.ready,SourceChunkCancelledError);
    requests[0].resolve();await a.ready;assert.equal(requests.length,2);assert.equal(requests[1].id,'2_0');assert.equal(cache.stats.active,1);
    requests[1].resolve();await c.ready;assert.equal(cache.stats.active,0);a.release();c.release();cache.dispose();assert.deepEqual(releases,['0_0','2_0']);
});
test('并发二可同时加载两块，完成一个只补一个且实际在途始终有界',async()=>{
    const {cache,requests}=controlled(cacheFixture(),{concurrency:2});const leases=['0_0','1_0','2_0'].map(id=>cache.acquire(id));
    assert.equal(requests.length,2);assert.equal(cache.stats.active,2);requests[1].resolve();await leases[1].ready;
    assert.equal(requests.length,3);assert.equal(cache.stats.active,2);assert.equal(requests[2].id,'2_0');
    requests[0].resolve();requests[2].resolve();await Promise.all(leases.map(l=>l.ready));assert.equal(cache.stats.active,0);
    leases.forEach(l=>l.release());cache.dispose();
});
test('可取消加载即使某个取消监听抛错仍释放并发，后续排队正常开始',async()=>{
    const {cache,requests}=controlled(cacheFixture());const a=cache.acquire('0_0'),b=cache.acquire('1_0');
    requests[0].token.onCancel(()=>{throw new Error('监听错误');});requests[0].token.onCancel(()=>requests[0].reject(new Error('底层已取消')));
    a.release();await assert.rejects(a.ready,SourceChunkCancelledError);await flush();assert.equal(requests.length,2);assert.equal(cache.stats.active,1);
    requests[1].resolve();await b.ready;b.release();cache.dispose();
});
test('使用中的容量不能淘汰，取消加载释放槽位但旧异步仍占真实并发',async()=>{
    const {cache,requests,releases}=controlled(cacheFixture(),{capacity:1});const a=cache.acquire('0_0');assert.throws(()=>cache.acquire('1_0'),SourceChunkCapacityError);
    let notified=0;requests[0].token.onCancel(()=>notified++);const off=requests[0].token.onCancel(()=>notified+=100);off();a.release();assert.equal(notified,1);await assert.rejects(a.ready,SourceChunkCancelledError);
    const b=cache.acquire('1_0');assert.equal(requests.length,1);assert.equal(cache.stats.active,1);assert.equal(cache.stats.queued,1);
    requests[0].resolve();await flush();assert.deepEqual(releases,['0_0']);assert.equal(requests.length,2);requests[1].resolve();await b.ready;b.release();cache.dispose();assert.deepEqual(releases,['0_0','1_0']);
});
test('换代隔离同ID旧回调，迟到资源释放且不写入新代缓存',async()=>{
    const {cache,requests,releases}=controlled(cacheFixture());const old=cache.acquire('0_0');assert.equal(cache.replaceGeneration(),1);await assert.rejects(old.ready,SourceChunkCancelledError);
    const current=cache.acquire('0_0');assert.equal(requests.length,1);requests[0].resolve(undefined,'old');await flush();assert.deepEqual(releases,['old']);assert.equal(requests.length,2);
    requests[1].resolve(undefined,'current');assert.equal((await current.ready).value,'current');old.release();assert.equal(cache.stats.pinned,1);current.release();cache.dispose();assert.deepEqual(releases,['old','current']);
});
test('网络失败和结构校验失败均可重试；无效加载结果仍释放一次',async()=>{
    const {cache,requests,releases}=controlled(cacheFixture());const a=cache.acquire('0_0');requests[0].reject(new Error('网络断开'));await assert.rejects(a.ready,/网络断开/);assert.equal(cache.stats.entries,0);
    const b=cache.acquire('0_0');requests[1].resolve({schema_version:9},'invalid');await assert.rejects(b.ready,SourceMapValidationError);assert.deepEqual(releases,['invalid']);assert.equal(cache.stats.active,0);
    const c=cache.acquire('0_0');requests[2].resolve();await c.ready;a.release();b.release();assert.equal(cache.stats.pinned,1);c.release();cache.dispose();assert.deepEqual(releases,['invalid','0_0']);
});
test('LRU只淘汰空闲就绪块、重复释放不重扣引用，销毁也释放正在返回的资源',async()=>{
    const {cache,requests,releases}=controlled(cacheFixture(),{capacity:2});const a=cache.acquire('0_0');requests[0].resolve();await a.ready;a.release();
    const b=cache.acquire('1_0');requests[1].resolve();await b.ready;b.release();const touch=cache.acquire('0_0');await touch.ready;touch.release();
    const c=cache.acquire('2_0');assert.deepEqual(releases,['1_0']);cache.dispose();await assert.rejects(c.ready,SourceChunkCancelledError);assert.deepEqual(releases,['1_0','0_0']);
    requests[2].resolve();await flush();c.release();assert.deepEqual(releases,['1_0','0_0','2_0']);assert.equal(cache.stats.entries,0);assert.equal(cache.stats.active,0);
});
test('加载器同步抛错、取消回调及资源release抛错不能卡住缓存调度',async()=>{
    const f=cacheFixture();let calls=0;const cache=new SourceChunkCache({model:new SourceMapModel(f.manifest),capacity:1,concurrency:1,loader:async info=>{if(++calls===1)throw new Error('首次失败');return{chunk:f.data[info.id],value:info.id,release(){throw new Error('释放失败');}};}});
    await assert.rejects(cache.acquire('0_0').ready,/首次失败/);const a=cache.acquire('0_0');await a.ready;a.release();cache.dispose();assert.equal(cache.stats.releaseErrorCount,1);assert.match(String(cache.stats.lastReleaseError),/释放失败/);
    for(const options of [{},{capacity:0,concurrency:1},{capacity:1,concurrency:0},{capacity:1.5,concurrency:1}])assert.throws(()=>new SourceChunkCache({model:new SourceMapModel(f.manifest),loader:async()=>{},...options}),/显式提供/);
});
test('实际700×700来源资源：1936块SHA、49万原格与碰撞、完整owner空间桶及三镇/四角/45火把对拍',()=>{
    const root=path.resolve(__dirname,'../../..'),base=path.join(root,'client/assets/resources/source-maps/map-2');
    const manifest=JSON.parse(fs.readFileSync(path.join(base,'manifest.json'),'utf8')),model=new SourceMapModel(manifest);
    const source=fs.readFileSync(path.join(root,'资料参考/myf/Mir200/Map/0.map'));
    assert.equal(crypto.createHash('sha256').update(source).digest('hex'),manifest.source.sha256);
    assert.equal(manifest.source.sha256,'2780766485d1e047dc2e8f03578ee83c6a14fd91597fe4562329cb8f24e1d4ec');
    assert.equal(model.manifest.id,'2');assert.equal(manifest.width,700);assert.equal(manifest.height,700);assert.equal(manifest.chunks.length,1936);
    assert.equal(source.readUInt16LE(0),700);assert.equal(source.readUInt16LE(2),700);assert.equal(source.length,52+490000*12);
    let cells=0,blocked=0,staticBlocked=0,doorBlocked=0,floors=0,objects=0,highest=null;const torches=[];
    for(const info of model.manifest.chunks){
        const bytes=fs.readFileSync(path.join(base,info.path));assert.equal(crypto.createHash('sha256').update(bytes).digest('hex'),info.sha256);
        const chunk=model.parseChunk(JSON.parse(bytes.toString('utf8')),info.id),raw=Buffer.from(chunk.cells_base64,'base64'),[x0,y0,width,height]=chunk.cell_bounds;
        for(let y=0;y<height;y++)for(let x=0;x<width;x++){
            const offset=(y*width+x)*12,sx=x+x0,sy=y+y0,sourceOffset=52+(sx*700+sy)*12;
            assert.equal(raw.compare(source,sourceOffset,sourceOffset+12,offset,offset+12),0,`${info.id}(${sx},${sy})原格`);
            const solid=!!((raw.readUInt16LE(offset)|raw.readUInt16LE(offset+2)|raw.readUInt16LE(offset+4))&0x8000),door=!!(raw[offset+6]&127);
            assert.equal(model.isWalkable(sx,sy),!solid&&!door,`(${sx},${sy})碰撞`);cells++;staticBlocked+=solid;doorBlocked+=door;blocked+=solid||door;
        }
        floors+=chunk.floor.length;objects+=chunk.objects.length;
        for(const record of chunk.objects){
            const frame=model.manifest.frames[record[3]];
            if(!highest||frame.height>highest.frame.height)highest={chunk,record,frame};
            if(record[4])torches.push({chunk,record});
        }
    }
    assert.equal(cells,490000);assert.equal(staticBlocked,162653);assert.equal(doorBlocked,182);assert.equal(blocked,162741);
    assert.equal(blocked,manifest.collision.runtime_count);assert.equal(floors,135390);assert.equal(objects,24887);
    // 每个实际索引桶内的所有owner均能查询到；parseChunk已逐实例、逐动画帧核对索引精确并集。
    for(const [key,ids]of Object.entries(manifest.spatial_index.buckets)){
        const [bx,by]=key.split(',').map(Number),found=new Set(model.queryChunks([bx*768,by*512,(bx+1)*768,(by+1)*512]).map(c=>c.id));
        for(const id of ids)assert(found.has(id),`${key}遗漏${id}`);
    }
    assert.equal(highest.frame.height,987);const highDraw=model.draws(highest.chunk,0).find(d=>d.x===highest.record[0]&&d.y===highest.record[1]&&d.layer===highest.record[2]);
    assert(highDraw);assert(model.queryChunks([highDraw.bounds[0],highDraw.bounds[1],highDraw.bounds[0]+1,highDraw.bounds[1]+1]).some(c=>c.id===highest.chunk.id));
    for(const [x,y]of [[650,631],[289,618],[331,267],[0,0],[699,0],[0,699],[699,699]]){
        const offset=52+(x*700+y)*12,solid=!!((source.readUInt16LE(offset)|source.readUInt16LE(offset+2)|source.readUInt16LE(offset+4))&0x8000),door=!!(source[offset+6]&127);
        assert.deepEqual(model.cellAt(model.project(x,y)),{x,y});assert.equal(model.isWalkable(x,y),!solid&&!door);
    }
    assert.equal(torches.length,45);
    for(const {chunk,record}of torches){
        assert.deepEqual(record.slice(3,8),['Objects:2723',10,0,1,1]);
        for(const [time,key]of [[0,2723],[99,2723],[100,2724],[999,2732],[1000,2723]]){
            const draw=model.draws(chunk,time).find(d=>d.x===record[0]&&d.y===record[1]&&d.layer===4);
            assert.equal(draw.key,`Objects:${key}`);assert.equal(draw.blend,1);
            assert.deepEqual(draw.bounds,[record[0]*48-51,(record[1]+1)*32-213,record[0]*48+49,(record[1]+1)*32-113]);
        }
    }
});
