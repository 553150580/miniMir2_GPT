#!/usr/bin/env python3
"""Strict, bounded first-loop WIL/map development import; source packages are read-only."""
from __future__ import annotations
import argparse
from collections import deque
import hashlib
import json
from pathlib import Path
import struct
import sys

from PIL import Image, ImageDraw, __version__ as PILLOW_VERSION
import inspect_wil as wil

ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / 'client/assets/resources/development'
MAP_SOURCE = ROOT.parent / '传奇1.80客户端/Map/0.map'
# 原始Mir地图贴图格式；本作表现格宽高仅从唯一Excel读取。
TILE_WIDTH, TILE_HEIGHT = 48, 32
DIRECTIONS = ['up','up_right','right','down_right','down','down_left','left','up_left']
PLAYER_ACTIONS = {'standing':(0,4,8,500),'walking':(64,6,8,100),'attack':(200,6,8,100),
                  'casting':(392,6,8,100),'struck':(472,3,8,100),'die':(536,4,8,100)}
MONSTER_ACTIONS = {'standing':(0,4,10,500),'walking':(80,6,10,100),'attack':(160,6,10,100),
                   'struck':(240,2,2,200),'die':(260,10,10,100)}
PROFILES = [('player_male',[('Hum',1200),('Weapon',1200),('Hair',1200)],PLAYER_ACTIONS),
            ('player_female',[('Hum',1800),('Weapon',1800),('Hair',1800)],PLAYER_ACTIONS),
            ('monster_scarecrow',[('Mon3',2520)],MONSTER_ACTIONS),
            ('pet_skeleton',[('Mon4',2520)],MONSTER_ACTIONS)]

def sha(path): return wil.sha256_file(path)
def canonical(obj): return json.dumps(obj,ensure_ascii=False,indent=2)+'\n'
def evidence(relative, start, end, role):
    p=ROOT/relative
    return {'path':relative,'sha256':sha(p),'lines':[start,end],'role':role}
def point(x,y):return {'x':x,'y':y}

class Importer:
    def __init__(self,check=False):
        self.check=check;self.packages={};self.frames={};self.sources={}
    def package(self,name):
        if name not in self.packages:
            path,index=wil.locate_pair('client_180_named',name)
            data=wil.probe(path,index)
            self.packages[name]=(path,index,data)
            self.sources[name]={'source_id':'client_180_named','package':name,'wil_path':str(path),'wix_path':str(index),
                                'wil_sha256':sha(path),'wix_sha256':sha(index),'format':data['format_id'],'frame_count':data['frame_count']}
        return self.packages[name]
    def rgba(self,name,index):
        path,_,data=self.package(name)
        if index<0 or index>=data['frame_count']:raise ValueError(f'{name} frame out of range {index}')
        f=data['frames'][index]
        return f,wil.decode_frame(path,f,data['palette'])
    def image(self,name,index):
        f,rgba=self.rgba(name,index)
        return None if f['declared_empty'] else Image.frombytes('RGBA',(f['width'],f['height']),rgba)
    def write_image(self,relative,image):
        path=OUTPUT/relative
        if path.exists():
            with Image.open(path) as old:
                same=old.mode==image.mode and old.size==image.size and old.tobytes()==image.tobytes()
            # 未变化的原帧不重编码，保留既有PNG字节与导入引用。
            if same:return sha(path)
        if self.check:raise ValueError('image mismatch '+relative)
        path.parent.mkdir(parents=True,exist_ok=True);image.save(path,format='PNG')
        return sha(path)
    def frame(self,name,index):
        key=f'{name}:{index}'
        if key not in self.frames:
            f,rgba=self.rgba(name,index)
            relative=f'frames/{name}/frame_{index:06d}.png'
            image=self.image(name,index)
            png_sha=self.write_image(relative,image) if image is not None else None
            self.frames[key]={'package':name,'raw_frame':index,'png':relative if image is not None else None,
                              'png_sha256':png_sha,'rgba_sha256':hashlib.sha256(rgba).hexdigest(),
                              'width':f['width'],'height':f['height'],'offset_x':f['offset_x'],'offset_y':f['offset_y'],
                              'source_offset':f['source_offset'],'declared_empty':f['declared_empty'],
                              'visible_alpha_pixels':sum(a!=0 for a in rgba[3::4]),'png_roundtrip_verified':image is not None}
        return key
    def write_json(self,relative,data):
        path=OUTPUT/relative;expected=canonical(data)
        if self.check:
            if path.read_text(encoding='utf-8')!=expected:raise ValueError('metadata outdated '+relative)
        else:path.parent.mkdir(parents=True,exist_ok=True);path.write_text(expected,encoding='utf-8')

def parse_map():
    data=MAP_SOURCE.read_bytes();width,height=struct.unpack_from('<HH',data)
    if data[4:18]!=b'\rLegend of mir' or len(data)!=52+width*height*12:
        raise ValueError('Only the evidenced 52-byte header + 12-byte column-major map0 format is supported')
    def read(x,y):
        offset=52+(x*height+y)*12
        back,middle,front,door,door_offset,animation,tick,front_lib,light=struct.unpack_from('<HHHBBBBBB',data,offset)
        # Crystal server v0: all three 0x8000 bits prevent walking; middle is LowWall.
        blocked=bool((back|middle|front)&0x8000)
        return {'back_raw':back,'middle_raw':middle,'front_raw':front,'blocked':blocked,
                'door':door&0x7f,'door_offset':door_offset,'front_animation':animation,'front_animation_tick':tick,
                'front_library':front_lib+2,'light':light,'source_byte_offset':offset}
    return width,height,read

def component(cells,start):
    height=len(cells);width=len(cells[0])
    seen={start};queue=deque([start])
    while queue:
        x,y=queue.popleft()
        for dx,dy in [(0,-1),(1,0),(0,1),(-1,0)]:
            q=(x+dx,y+dy)
            if 0<=q[0]<width and 0<=q[1]<height and not cells[q[1]][q[0]]['blocked'] and q not in seen:
                seen.add(q);queue.append(q)
    return seen

def map_config():
    """读取唯一工作簿，不从历史生成物或来源提取快照补参数。"""
    sys.path.insert(0,str(ROOT/'tools/config'))
    import export_configs as config
    tables,errors,_,_=config.collect(config.load_json(ROOT/'config/schema/catalog.json'))
    if errors:raise ValueError('唯一Excel结构校验失败：'+'；'.join(errors))
    profile=next(row for row in tables['dev_first_loop_profile'] if row['id']==1)['parameters']
    definition=next(row for row in tables['game_maps'] if row['id']==profile['map_id'])
    collision=next(row for row in tables['game_map_collisions'] if row['id']==definition['collision_id'])
    geometry=definition.get('visual_geometry')
    fields={'projection','tile_width','tile_height','soil_thickness_px','source_origin'}
    if not isinstance(geometry,dict) or set(geometry)!=fields:raise ValueError('地图visual_geometry必须由唯一Excel显式维护完整字段')
    if geometry['projection']!='isometric_diamond':raise ValueError('当前资源转换仅支持显式isometric_diamond')
    for field in ['tile_width','tile_height']:
        if type(geometry[field]) is not int or geometry[field]<=0 or geometry[field]%2:raise ValueError('菱形格宽高必须为正偶数像素')
    if type(geometry['soil_thickness_px']) is not int or geometry['soil_thickness_px']<0:raise ValueError('土层厚度必须为非负整数像素')
    if not isinstance(geometry['source_origin'],dict) or set(geometry['source_origin'])!={'x','y'} or any(type(v) is not int or v<0 for v in geometry['source_origin'].values()):raise ValueError('必须显式给出源图非负整数裁剪原点')
    width,height=definition['width_cells'],definition['height_cells']
    if type(width) is not int or type(height) is not int or width<=0 or height<=0:raise ValueError('地图格数必须为正整数')
    if (collision['width_cells'],collision['height_cells'])!=(width,height):raise ValueError('地图与碰撞尺寸不一致')
    if collision.get('client_navigation')!={'geometry_reference':f'game_maps/{definition["id"]}.visual_geometry','collision_basis':'authored_edge_decorations'}:raise ValueError('本作地图碰撞必须显式登记为唯一Excel小型边缘装饰布局')
    spawns=[row for row in tables['game_monster_spawns'] if row['map_id']==definition['id']]
    spawns.sort(key=lambda row:row['slot_index'])
    return definition,collision,geometry,profile['pet_spawn'],[row['cell'] for row in spawns]

def project_floor(canvas,width,height,tile_width,tile_height):
    """仅将地表UV合成到菱形平面；前景物与原始WIL帧不参加变形。"""
    half_width,half_height=tile_width/2,tile_height/2
    size=(int((width+height)*half_width),int((width+height)*half_height))
    inverse=(TILE_WIDTH/(2*half_width),TILE_WIDTH/(2*half_height),-TILE_WIDTH*height/2,
             -TILE_HEIGHT/(2*half_width),TILE_HEIGHT/(2*half_height),TILE_HEIGHT*height/2)
    return canvas.transform(size,Image.Transform.AFFINE,inverse,resample=Image.Resampling.BICUBIC,fillcolor=(0,0,0,0))

def group_props(props,frames,read,ox,oy,source_width,source_height):
    """仅合并原地图完整模板位置已验证的连续切片，保留其原矩形像素间距。"""
    # 来源：map0右树(137,135)、岩石(135,132)、小树(128,136)的原坐标组合。
    # 树1384在本裁图之外仍只读取原cell核验；不补入裁图外PNG。1374属于另一棵裁图外树，独立保留。
    patterns={
        'tree_1376':[(1376,0,0),(1377,1,3),(1378,2,3),(1379,3,3),(1380,4,3),(1381,5,3),(1382,6,2),(1383,7,2),(1384,8,2)],
        'rock_1082':[(1082,0,0),(1083,1,0)],
        'tree_4132':[(4132,0,0),(4133,1,0),(4134,2,0)]}
    groups={}
    for prop in props:
        group_id='single_'+prop['id'];origin=point(ox+prop['x'],oy+prop['y'])
        raw=frames[prop['frame']]['raw_frame']
        for family,pattern in patterns.items():
            member=next((row for row in pattern if row[0]==raw),None)
            if member is None:continue
            bx,by=ox+prop['x']-member[1],oy+prop['y']-member[2]
            def matches(row):
                index,dx,dy=row;x,y=bx+dx,by+dy
                if not 0<=x<source_width or not 0<=y<source_height:return False
                cell=read(x,y)
                return cell['front_library']==2 and (cell['front_raw']&0x7fff)-1==index
            if all(matches(row) for row in pattern):
                group_id=f'objects_{family}_{bx}_{by}';origin=point(bx,by)
            break
        groups.setdefault(group_id,{'origin':origin,'parts':[]})['parts'].append(prop)
    for group_id,group in groups.items():
        parts=sorted(group['parts'],key=lambda p:(p['y'],p['x'],p['id']))
        left=min(p['x']*TILE_WIDTH for p in parts)
        right=max(p['x']*TILE_WIDTH+frames[p['frame']]['width'] for p in parts)
        bottom=max((p['y']+1)*TILE_HEIGHT for p in parts)
        anchor_x,anchor_y=(left+right)/2,bottom
        for index,prop in enumerate(parts):
            frame=frames[prop['frame']]
            prop.update({'group_id':group_id,'part_index':index,'group_source_origin':group['origin'],
                         'display_x':anchor_x/TILE_WIDTH-.5,'display_y':anchor_y/TILE_HEIGHT-.5,
                         'source_pixel_offset_x':prop['pixel_offset_x'],'source_pixel_offset_y':prop['pixel_offset_y'],
                         'pixel_offset_x':prop['x']*TILE_WIDTH+frame['width']/2-anchor_x,
                         'pixel_offset_y':anchor_y-(prop['y']+1)*TILE_HEIGHT,
                         'anchor_basis':'source_rectangular_verified_group_bottom_center'})
    return len(groups)

def authored_props(importer,definition,blocked):
    """仅显示用户选定的原生小装饰；布局和碰撞索引来自唯一Excel。"""
    decorations=definition.get('decorations')
    if not isinstance(decorations,list) or not 4<=len(decorations)<=6 or len(decorations)!=len(blocked):raise ValueError('当前地图必须提供4至6个独立小装饰及对应阻挡格')
    props=[];ids=set();indices=set()
    for row in decorations:
        if not isinstance(row,dict) or set(row)!={'id','frame','collision_cell_index'}:raise ValueError('小装饰必须显式填写id、原帧和唯一碰撞索引')
        id,key,index=row['id'],row['frame'],row['collision_cell_index']
        if not isinstance(id,str) or not id or id in ids or type(index) is not int or not 0<=index<len(blocked) or index in indices:raise ValueError('小装饰身份或碰撞索引重复/无效')
        if not isinstance(key,str) or not key.startswith('Objects:') or not key[8:].isdigit():raise ValueError('小装饰必须引用原始Objects帧')
        raw=int(key[8:])
        if importer.frame('Objects',raw)!=key:raise ValueError('小装饰原帧编号不是规范形式')
        frame=importer.frames[key]
        if frame['declared_empty'] or frame['width']>144 or frame['height']>144:raise ValueError('小装饰必须为非空原生小帧，不能裁切大物件代替')
        cell=blocked[index];source=importer.sources['Objects'];ids.add(id);indices.add(index)
        props.append({'id':id,'x':cell['x'],'y':cell['y'],'display_x':cell['x'],'display_y':cell['y'],
                      'frame':key,'anchor_x':0.5,'anchor_y':0,'pixel_offset_x':0,'pixel_offset_y':0,
                      'group_id':id,'part_index':0,'collision_cell_index':index,
                      'anchor_basis':'authored_collision_cell_center_native_sprite_bottom_center',
                      'source_ref':f"传奇1.80客户端/Data/Objects.wil 原帧{raw}；原字节位置{frame['source_offset']}；WIL SHA256 {source['wil_sha256']}；原图不裁切不缩放"})
    return props

def build_map(importer):
    definition,collision,geometry,pet_spawn,monster_spawns=map_config()
    width,height=definition['width_cells'],definition['height_cells']
    ox,oy=geometry['source_origin']['x'],geometry['source_origin']['y']
    sw,sh,read=parse_map()
    if ox+width>sw or oy+height>sh:raise ValueError('Excel裁剪超出原始地图范围')
    cells=[[read(ox+x,oy+y) for x in range(width)] for y in range(height)]
    source_blocked=[point(x,y) for y in range(height) for x in range(width) if cells[y][x]['blocked']]
    if sha(MAP_SOURCE)!=collision['source_hash']:raise ValueError('原始地图来源摘要与唯一Excel不一致')
    blocked=collision['blocked_cells']
    if not isinstance(blocked,list) or any(not isinstance(p,dict) or set(p)!={'x','y'} or any(type(v) is not int for v in p.values()) or not 0<=p['x']<width or not 0<=p['y']<height for p in blocked):raise ValueError('本作阻挡格格式错误或越界')
    runtime_blocked={(p['x'],p['y']) for p in blocked}
    if len(runtime_blocked)!=len(blocked):raise ValueError('本作阻挡格不能重复')
    for y,row in enumerate(cells):
        for x,cell in enumerate(row):
            cell['source_blocked']=cell['blocked'];cell['blocked']=(x,y) in runtime_blocked
    if any(c['door'] or c['front_animation'] or ((c['front_raw']&0x7fff) and c['front_library']!=2) for row in cells for c in row):raise ValueError('裁剪包含尚未核验支持的门、动画或前景库')
    player=definition['spawn_cell'];pet=pet_spawn;monsters=monster_spawns
    def cell_point(value):
        if not isinstance(value,dict) or set(value)!={'x','y'} or any(type(v) is not int for v in value.values()):raise ValueError('出生点必须为显式整数格坐标')
        x,y=value['x'],value['y']
        if not 0<=x<width or not 0<=y<height or cells[y][x]['blocked']:raise ValueError('出生点越界或位于阻挡格')
        return x,y
    player_point=cell_point(player);spawn_points=[player_point,cell_point(pet)]+[cell_point(p) for p in monsters]
    connected=component(cells,player_point)
    if len(monsters)!=8 or len(set(spawn_points))!=len(spawn_points) or any(p not in connected for p in spawn_points):raise ValueError('必须提供独立且静态可达的玩家、宠物和8怪槽位')
    canvas=Image.new('RGBA',(width*TILE_WIDTH,height*TILE_HEIGHT),(0,0,0,0));layers=[];props=[]
    for y in range(height):
        for x in range(width):
            c=cells[y][x]
            if (ox+x)%2==0 and (oy+y)%2==0 and c['back_raw']&0x7fff:
                i=(c['back_raw']&0x7fff)-1;key=importer.frame('Tiles',i);im=importer.image('Tiles',i)
                if im is not None:canvas.alpha_composite(im,(x*TILE_WIDTH,y*TILE_HEIGHT))
                layers.append({'x':x,'y':y,'layer':'back','frame':key})
            if c['middle_raw']&0x7fff:
                i=(c['middle_raw']&0x7fff)-1;key=importer.frame('SmTiles',i);im=importer.image('SmTiles',i)
                if im is not None:canvas.alpha_composite(im,(x*TILE_WIDTH,y*TILE_HEIGHT))
                layers.append({'x':x,'y':y,'layer':'middle','frame':key})
    for y in range(height):
        for x in range(width):
            c=cells[y][x]
            if c['front_raw']&0x7fff:
                i=(c['front_raw']&0x7fff)-1;key=importer.frame('Objects',i);frame=importer.frames[key]
                layers.append({'x':x,'y':y,'layer':'front','frame':key})
                if not frame['declared_empty']:
                    props.append({'id':f'prop_{x}_{y}','x':x,'y':y,'frame':key,'anchor_x':0.5,'anchor_y':0,
                                  'pixel_offset_x':frame['width']/2-TILE_WIDTH/2,'pixel_offset_y':-TILE_HEIGHT/2,
                                  'anchor_basis':'source_mir_cell_bottom_center'})
    source_prop_group_count=group_props(props,importer.frames,read,ox,oy,sw,sh)
    source_props=props;props=authored_props(importer,definition,blocked)
    top=project_floor(canvas,width,height,geometry['tile_width'],geometry['tile_height'])
    png='maps/first-map.png';png_sha=importer.write_image(png,top)
    result={'id':definition['id'],'name':definition['display_name'],'development_only':True,'record_status':'candidate','runtime_ready':False,
            'width':width,'height':height,'tile_width':geometry['tile_width'],'tile_height':geometry['tile_height'],
            'projection':geometry['projection'],'soil_thickness_px':geometry['soil_thickness_px'],
            'surface_width':top.width,'surface_height':top.height,'world_height':top.height+geometry['soil_thickness_px'],
            'background_offset_y':geometry['soil_thickness_px']/2,'background_layers':['back','middle'],'props':props,
            'prop_group_count':len(props),'source_props':source_props,'source_prop_group_count':source_prop_group_count,
            'source_path':str(MAP_SOURCE),'source_sha256':sha(MAP_SOURCE),'source_width':sw,'source_height':sh,'source_origin':point(ox,oy),
            'source_format':'mir2_v0_52_header_12_bytes_column_major','cell_order':'rows_y_then_x','cells':cells,'layers':layers,
            'blocked':blocked,'source_blocked':source_blocked,'collision_basis':'authored_edge_decorations',
            'player_spawn':player,'pet_spawn':pet,'monster_spawns':monsters,
            'spawn_selection':'unique_excel_explicit_connected_walkable_cells_not_legacy_spawn',
            'floor_projection':'source_back_and_middle_rgba_affine_uv_to_diamond_only_front_excluded',
            'prop_anchor_contract':'authored_small_native_sprites_at_collision_centers_original_fronts_retained_as_source_evidence_only',
            'connected_walkable_count':len(connected),'background':png,'background_sha256':png_sha,
            'collision_verified_against_source_bits':False,'source_collision_evidence_verified':True,
            'authored_collision_verified_against_excel':True,'source_fronts_rendered':False,'official_180_map_or_balance_acceptance':False}
    importer.write_json('maps/first-map.json',result)
    return {key:result[key] for key in ['id','width','height','tile_width','tile_height','projection','soil_thickness_px',
                                      'surface_width','surface_height','world_height','background_offset_y','source_origin','source_sha256','collision_basis']}|{
            'path':'maps/first-map.json','background':png}

def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--check',action='store_true');args=parser.parse_args()
    if OUTPUT.is_symlink():raise ValueError('Output must not be a symlink')
    importer=Importer(args.check);map_info=build_map(importer);profiles={}
    for name,layers,actions in PROFILES:
        profile={'id':name,'status':'candidate','development_only':True,'official_180_animation_acceptance':False,'layers':[],
                 'mapping_basis':'Crystal action order/count/timing + original WIL visible/padding runs and local contact-sheet review; compact Crystal indices are not raw WIL indices'}
        profile['layer_order_by_direction']={str(d):(['Weapon','Hum','Hair'] if d in [0,5,6,7] else ['Hum','Hair','Weapon']) if len(layers)==3 else [layers[0][0]] for d in range(8)}
        for package,base in layers:
            layer={'package':package,'base_index':base,'actions':{}}
            for action,(start,count,stride,interval) in actions.items():
                directions=[]
                for direction in range(8):
                    keys=[importer.frame(package,base+start+direction*stride+i) for i in range(count)]
                    directions.append({'direction':direction,'name':DIRECTIONS[direction],'frames':keys})
                layer['actions'][action]={'raw_start':base+start,'count':count,'direction_stride':stride,'duration_ms':interval,
                                          'loop':action in ['standing','walking'],'directions':directions}
            profile['layers'].append(layer)
        profiles[name]=profile
    # Magic原图含用于加色混合的暗色像素；保持原像素，由保存的特效Sprite选择加色混合。
    def effect(start,count,duration,package='Magic'):
        return {'frames':[importer.frame(package,start+i) for i in range(count)],'duration_ms':duration,'blend':'additive'}
    def directional_effect(start,count,duration,stride=10):
        return {'directions':[[importer.frame('Magic',start+d*stride+i) for i in range(count)] for d in range(8)],
                'duration_ms':duration,'blend':'additive'}
    def ranked_attack(start):
        # 刀光源索引本身按等级分四组；身体仍使用已核验的原WIL Attack1六帧，不套用Crystal紧凑帧号。
        return {'levels':[directional_effect(start+rank*90,6,600) for rank in range(4)],'duration_ms':600,'blend':'additive'}
    effects={
        '1':{'kind':'fireball','cast':effect(0,10,600),'hit':effect(170,10,600),
             'projectile':{'directions':[[importer.frame('Magic',10+d*10+i) for i in range(6)] for d in range(16)],
                           'frame_ms':30,'blend':'additive'}},
        '2':{'kind':'healing','cast':effect(200,10,600),'hit':effect(370,10,800)},
        '5':{'kind':'great_fireball','cast':effect(400,10,600),'hit':effect(570,10,600),
             'projectile':{'directions':[[importer.frame('Magic',410+d*10+i) for i in range(6)] for d in range(16)],
                           'frame_ms':30,'blend':'additive'}},
        '7':{'kind':'slaying','attack':ranked_attack(1820)},
        '9':{'kind':'hell_fire','cast':effect(920,10,600),'cast_anchor':'origin','step':effect(930,6,500),
             'step_anchor':'grid','step_trigger':'authoritative_cast_step',
             'visual_adaptation':'User-approved fixed forward line for every rank; source rank3 side lines are not displayed.'},
        '10':{'kind':'lightning','beam':directional_effect(970,6,600,20),'beam_anchor':'origin',
              # 原素材48×32方向基向量；只用于刚体转向与范围裁剪，不是技能格集参数。
              'beam_native_direction_vectors':[[0,TILE_HEIGHT],[TILE_WIDTH,TILE_HEIGHT],[TILE_WIDTH,0],[TILE_WIDTH,-TILE_HEIGHT],
                                               [0,-TILE_HEIGHT],[-TILE_WIDTH,-TILE_HEIGHT],[-TILE_WIDTH,0],[-TILE_WIDTH,TILE_HEIGHT]],
              'beam_alignment':'rigid_rotation_then_forward_range_mask'},
        '11':{'kind':'thunderbolt','cast':effect(20,3,300,'Magic2'),'hit':effect(10,5,400,'Magic2')},
        '12':{'kind':'thrusting','attack':ranked_attack(2190)},
        '23':{'kind':'firebang','cast':effect(1650,10,600),'hit':effect(1660,10,1000),'hit_anchor':'grid'},
        '24':{'kind':'thunder_storm','cast':effect(1680,10,600),'cast_anchor':'origin'},
        '25':{'kind':'half_moon','attack':ranked_attack(2560)},
        '26':{'kind':'flaming_sword','attack':directional_effect(3480,6,600)},
        '29':{'kind':'mass_healing','cast':effect(1790,10,600),'hit':effect(1800,10,1000),'hit_anchor':'grid'},
        '33':{'kind':'ice_storm','cast':effect(3840,10,600),'hit':effect(3850,20,1300),'hit_anchor':'grid'},
    }
    manifest={'schema_version':1,'purpose':'first_loop_development_candidates','development_only':True,'runtime_ready':False,
              'status':'candidate','official_180_restoration_verified':False,'pillow_version':PILLOW_VERSION,
              'converter':'tools/resources/build_development_assets.py','converter_sha256':sha(Path(__file__)),
              'wil_decoder_sha256':sha(ROOT/'tools/resources/inspect_wil.py'),'map':map_info,'direction_names':DIRECTIONS,
              'coordinate_contract':'Native frame PNGs remain uncropped and unscaled with original WIL offsets. Entity draw origin uses source offsets; authored small map props place the original image bottom center at their explicit collision cell. Historical source_props are not rendered. Only back/middle floor UVs are projected to the authored diamond; source logical y is down.',
              'sources':list(importer.sources.values()),'profiles':profiles,'effects':effects,'frames':importer.frames,
              'evidence':[evidence('资料参考/crystal/Client/MirObjects/Frames.cs',23,33,'monster compact action order/count/interval'),
                          evidence('资料参考/crystal/Client/MirObjects/Frames.cs',153,168,'player compact action order/count/interval'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',1643,1657,'fireball/healing cast Magic frame groups'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',2837,2883,'fireball projectile/hit and healing target effects'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',1688,1710,'great-fireball and thunderbolt caster effects'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',1791,1796,'firebang caster effect'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',1971,1976,'mass-healing caster effect'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',2954,2964,'thunderbolt target effect'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',2997,3003,'firebang ground effect'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',3073,3079,'mass-healing ground effect'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',4953,4975,'ranked Slaying/Thrusting/HalfMoon and FlamingSword directional Attack1 overlays'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',1077,1079,'FlamingSword Attack4 also selects Attack1 body frames'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',3222,3227,'Lightning eight-direction beam after spell body finishes'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',1697,1701,'HellFire caster Magic frame group'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',2907,2948,'HellFire per-cell effect pixels; source 50ms scheduling and rank3 side lines are not adopted'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',1962,1966,'ThunderStorm single caster-origin area effect'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',1980,1984,'IceStorm caster effect'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',3083,3088,'IceStorm single ground area effect'),
                          evidence('资料参考/crystal/Client/MirObjects/Effect.cs',152,216,'16-direction projectile frames and additive rendering'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',4887,4919,'weapon/body/hair order by direction'),
                          evidence('资料参考/crystal/Client/MirObjects/MapCode.cs',226,259,'v0 layers, 1-based images, column-major cells'),
                          evidence('资料参考/crystal/Server/MirEnvir/Map.cs',112,151,'three layer 0x8000 collision bits')],
              'limitations':['Cross-source action/appearance mapping remains candidate; no official 1.80 behavior acceptance.',
                             'Native frame PNGs use no guessed palette, deletion, reindexing, pixel scaling or fallback sprite. The separate floor composite is intentionally projected to a diamond.',
                             'Map geometry and explicit connected spawn coordinates are read from the unique Excel development fixture; they are not historical spawn coordinates.',
                             'Diamond floor, small authored edge decorations and client-rendered soil are project presentation adaptations, not source 1.80 map projection acceptance.',
                             'Runtime collision is authored with the small decorations. Original map bits, fronts and frame PNGs remain separate source evidence, not runtime obstacles or displayed large trees.']}
    importer.write_json('manifest.json',manifest)
    for record in importer.sources.values():
        if sha(Path(record['wil_path']))!=record['wil_sha256'] or sha(Path(record['wix_path']))!=record['wix_sha256']:raise ValueError('Source changed')
    if sha(MAP_SOURCE)!=map_info['source_sha256']:raise ValueError('Map source changed during import')
    print(json.dumps({'status':'checked' if args.check else 'generated','output':str(OUTPUT),'profiles':len(profiles),
                      'frames':len(importer.frames),'pngs':sum(x['png'] is not None for x in importer.frames.values()),
                      'map':map_info,'manifest_sha256':sha(OUTPUT/'manifest.json'),'runtime_ready':False},ensure_ascii=False))
    return 0
if __name__=='__main__':
    try:raise SystemExit(main())
    except (OSError,ValueError,KeyError,wil.UnsupportedFormat) as error:print(str(error),file=sys.stderr);raise SystemExit(2)
