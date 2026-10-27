const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const crypto=require('node:crypto');
const {GridProjection}=require('../../../.work/ui-tests/app/GridProjection.js');
const {beamGeometry}=require('../../../.work/ui-tests/app/BeamGeometry.js');
const base=path.resolve(__dirname,'../../assets/resources/development');
const map=JSON.parse(fs.readFileSync(path.join(base,'maps/first-map.json'),'utf8')),manifest=JSON.parse(fs.readFileSync(path.join(base,'manifest.json'),'utf8'));
test('实际17×17地图只显示四周4件原尺寸小物，原19片大景物仅保留来源且不残留空气阻挡',()=>{
    assert.equal(map.width,17);assert.equal(map.height,17);assert.equal(map.cells.length,17);assert(map.cells.every(row=>row.length===17));
    assert.equal(map.blocked.length,4);assert.equal(map.source_blocked.length,17);assert.equal(map.connected_walkable_count,285);
    assert.equal(map.collision_basis,'authored_edge_decorations');assert.equal(manifest.map.collision_basis,map.collision_basis);
    assert.equal(map.collision_verified_against_source_bits,false);assert.equal(map.source_collision_evidence_verified,true);
    assert.equal(map.authored_collision_verified_against_excel,true);assert.equal(map.source_fronts_rendered,false);
    const projection=new GridProjection(map);assert.equal(projection.surfaceWidth,1088);assert.equal(projection.surfaceHeight,544);assert.equal(projection.worldHeight,592);assert.equal(projection.floorCenterY,24);
    for(const key of ['width','height','tile_width','tile_height','projection','soil_thickness_px','surface_width','surface_height','world_height','background_offset_y'])assert.equal(manifest.map[key],map[key],key);
    const png=fs.readFileSync(path.join(base,map.background));assert.equal(png.readUInt32BE(16),1088);assert.equal(png.readUInt32BE(20),544);
    assert.equal(map.props.length,4);assert.equal(new Set(map.props.map(prop=>prop.id)).size,4);assert.equal(new Set(map.props.map(prop=>prop.group_id)).size,4);
    assert.equal(map.source_props.length,19);assert.equal(new Set(map.source_props.map(prop=>prop.group_id)).size,5);
    const expected=new Map([['Objects:9760',[1,7,60,57]],['Objects:1127',[7,1,48,67]],['Objects:8898',[15,9,48,41]],['Objects:7092',[7,15,48,47]]]);
    for(const prop of map.props){
        const frame=manifest.frames[prop.frame];assert(frame&&!frame.declared_empty);assert.equal(prop.anchor_x,.5);assert.equal(prop.anchor_y,0);
        assert.deepEqual([prop.x,prop.y,frame.width,frame.height],expected.get(prop.frame));assert.equal(prop.display_x,prop.x);assert.equal(prop.display_y,prop.y);
        assert.equal(prop.pixel_offset_x,0);assert.equal(prop.pixel_offset_y,0);assert.equal(prop.part_index,0);
        assert.equal(prop.anchor_basis,'authored_collision_cell_center_native_sprite_bottom_center');assert(prop.source_ref.includes('原图不裁切不缩放'));
        assert.deepEqual(map.blocked[prop.collision_cell_index],{x:prop.x,y:prop.y});
        const image=fs.readFileSync(path.join(base,frame.png));assert.equal(crypto.createHash('sha256').update(image).digest('hex'),frame.png_sha256);
        assert.equal(image.readUInt32BE(16),frame.width);assert.equal(image.readUInt32BE(20),frame.height);
        assert.deepEqual(projection.cellAt(projection.project(prop.x,prop.y)),{x:prop.x,y:prop.y});
    }
    const runtimeBlocked=new Set(map.blocked.map(p=>`${p.x},${p.y}`)),sourceBlocked=new Set(map.source_blocked.map(p=>`${p.x},${p.y}`));
    for(let y=0;y<17;y++)for(let x=0;x<17;x++){
        assert.equal(map.cells[y][x].blocked,runtimeBlocked.has(`${x},${y}`));assert.equal(map.cells[y][x].source_blocked,sourceBlocked.has(`${x},${y}`));
    }
    assert.equal(map.cells[0][0].source_blocked,true);assert.equal(map.cells[0][0].blocked,false);
    assert.equal(map.cells[4][7].source_blocked,true);assert.equal(map.cells[4][7].blocked,false);
    const reached=new Set(['8,8']),queue=[{x:8,y:8}];
    while(queue.length){const p=queue.shift();for(const [dx,dy]of [[0,-1],[1,0],[0,1],[-1,0]]){const x=p.x+dx,y=p.y+dy,key=`${x},${y}`;if(x<0||y<0||x>=17||y>=17||runtimeBlocked.has(key)||reached.has(key))continue;reached.add(key);queue.push({x,y});}}
    assert.equal(reached.size,285);
    for(const sourceProp of map.source_props)assert(map.layers.some(layer=>layer.x===sourceProp.x&&layer.y===sourceProp.y&&layer.layer==='front'&&layer.frame===sourceProp.frame));
    assert(map.background_layers.every(layer=>(typeof layer==='string'?layer:layer.layer)!=='front'));
    for(let x=0;x<17;x++)for(let y=0;y<17;y++)assert.deepEqual(projection.cellAt(projection.project(x,y)),{x,y});
});
test('实际疾光48帧保持原PNG哈希和尺寸，来源8向向量与六格投影端点匹配刚体Mask方案',()=>{
    const resource=manifest.effects['10'],projection=new GridProjection(map),gameplay=JSON.parse(fs.readFileSync(path.join(base,'gameplay.json'),'utf8'));
    const definition=gameplay.skills.find(row=>row.id===10);assert.equal(definition.line_cells,6);assert.equal(resource.beam_anchor,'origin');assert.equal(resource.beam_alignment,'rigid_rotation_then_forward_range_mask');
    assert.deepEqual(resource.beam_native_direction_vectors,[[0,32],[48,32],[48,0],[48,-32],[0,-32],[-48,-32],[-48,0],[-48,32]]);
    for(let direction=0;direction<8;direction++){
        const keys=resource.beam.directions[projection.visualDirection(direction)],frames=keys.map(key=>manifest.frames[key]);assert.equal(keys.length,6);
        const result=beamGeometry(projection,direction,definition.line_cells,resource.beam_native_direction_vectors,frames);assert(result.length>0&&result.height>0);
        for(const frame of frames){const png=fs.readFileSync(path.join(base,frame.png));assert.equal(crypto.createHash('sha256').update(png).digest('hex'),frame.png_sha256);assert.equal(png.readUInt32BE(16),frame.width);assert.equal(png.readUInt32BE(20),frame.height);}
    }
});
