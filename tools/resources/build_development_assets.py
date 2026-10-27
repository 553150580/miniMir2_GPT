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
        if self.check:
            with Image.open(path) as old:
                if old.mode!=image.mode or old.size!=image.size or old.tobytes()!=image.tobytes():raise ValueError('image mismatch '+relative)
        else:
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
    seen={start};queue=deque([start])
    while queue:
        x,y=queue.popleft()
        for dx,dy in [(0,-1),(1,0),(0,1),(-1,0)]:
            q=(x+dx,y+dy)
            if 0<=q[0]<16 and 0<=q[1]<16 and not cells[q[1]][q[0]]['blocked'] and q not in seen:
                seen.add(q);queue.append(q)
    return seen

def build_map(importer):
    sw,sh,read=parse_map();selected=None
    # Fixture crop selection is a deterministic development choice, not an adopted historical spawn.
    for ox in range(128,min(sw-16,512),16):
        for oy in range(128,min(sh-16,512),16):
            cells=[[read(ox+x,oy+y) for x in range(16)] for y in range(16)]
            if cells[8][8]['blocked']:continue
            connected=component(cells,(8,8));blocked=sum(c['blocked'] for row in cells for c in row)
            if len(connected)<160 or not 16<=blocked<=80:continue
            if any(c['door'] or c['front_animation'] or ((c['front_raw']&0x7fff) and c['front_library']!=2) for row in cells for c in row):continue
            selected=(ox,oy,cells,connected);break
        if selected:break
    if selected is None:raise ValueError('No eligible bounded crop found; do not synthesize a replacement map')
    ox,oy,cells,connected=selected
    # Pick distinct connected walkable points, away from the border, in a stable geometric order.
    pool=sorted((p for p in connected if 1<=p[0]<=14 and 1<=p[1]<=14),key=lambda p:(abs(p[0]-8)+abs(p[1]-8),p[1],p[0]))
    player=(8,8);pet=next(p for p in pool if p!=player)
    monsters=[]
    for p in reversed(pool):
        if abs(p[0]-8)+abs(p[1]-8)<5 or p in [player,pet]:continue
        if all(abs(p[0]-q[0])+abs(p[1]-q[1])>=3 for q in monsters):monsters.append(p)
        if len(monsters)==8:break
    if len(monsters)!=8:raise ValueError('Need eight connected development spawn points')
    canvas=Image.new('RGBA',(16*TILE_WIDTH,16*TILE_HEIGHT),(0,0,0,0));layers=[]
    for y in range(16):
        for x in range(16):
            c=cells[y][x]
            if (ox+x)%2==0 and (oy+y)%2==0 and c['back_raw']&0x7fff:
                i=(c['back_raw']&0x7fff)-1;key=importer.frame('Tiles',i);im=importer.image('Tiles',i)
                if im is not None:canvas.alpha_composite(im,(x*TILE_WIDTH,y*TILE_HEIGHT))
                layers.append({'x':x,'y':y,'layer':'back','frame':key})
            if c['middle_raw']&0x7fff:
                i=(c['middle_raw']&0x7fff)-1;key=importer.frame('SmTiles',i);im=importer.image('SmTiles',i)
                if im is not None:canvas.alpha_composite(im,(x*TILE_WIDTH,y*TILE_HEIGHT))
                layers.append({'x':x,'y':y,'layer':'middle','frame':key})
    for y in range(16):
        for x in range(16):
            c=cells[y][x]
            if c['front_raw']&0x7fff:
                i=(c['front_raw']&0x7fff)-1;key=importer.frame('Objects',i);im=importer.image('Objects',i)
                if im is not None:canvas.alpha_composite(im,(x*TILE_WIDTH,(y+1)*TILE_HEIGHT-im.height))
                layers.append({'x':x,'y':y,'layer':'front','frame':key})
    png='maps/first-map.png';png_sha=importer.write_image(png,canvas)
    result={'id':1,'name':'开发地图0局部','development_only':True,'record_status':'candidate','runtime_ready':False,
            'width':16,'height':16,'tile_width':TILE_WIDTH,'tile_height':TILE_HEIGHT,'projection':'source_mir_grid_48x32',
            'source_path':str(MAP_SOURCE),'source_sha256':sha(MAP_SOURCE),'source_width':sw,'source_height':sh,'source_origin':point(ox,oy),
            'source_format':'mir2_v0_52_header_12_bytes_column_major','cell_order':'rows_y_then_x','cells':cells,'layers':layers,
            'blocked':[point(x,y) for y in range(16) for x in range(16) if cells[y][x]['blocked']],
            'player_spawn':point(*player),'pet_spawn':point(*pet),'monster_spawns':[point(*p) for p in monsters],
            'spawn_selection':'development_fixture_deterministic_connected_walkable_cells_not_legacy_spawn',
            'connected_walkable_count':len(connected),'background':png,'background_sha256':png_sha,
            'collision_verified_against_source_bits':True,'official_180_map_or_balance_acceptance':False}
    importer.write_json('maps/first-map.json',result)
    return {'id':1,'path':'maps/first-map.json','background':png,'width':16,'height':16,
            'tile_width':TILE_WIDTH,'tile_height':TILE_HEIGHT,'projection':result['projection'],'source_origin':result['source_origin']}

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
    effects={
        '1':{'kind':'fireball','cast':effect(0,10,600),'hit':effect(170,10,600),
             'projectile':{'directions':[[importer.frame('Magic',10+d*10+i) for i in range(6)] for d in range(16)],
                           'frame_ms':30,'blend':'additive'}},
        '2':{'kind':'healing','cast':effect(200,10,600),'hit':effect(370,10,800)},
        '5':{'kind':'great_fireball','cast':effect(400,10,600),'hit':effect(570,10,600),
             'projectile':{'directions':[[importer.frame('Magic',410+d*10+i) for i in range(6)] for d in range(16)],
                           'frame_ms':30,'blend':'additive'}},
        '11':{'kind':'thunderbolt','cast':effect(20,3,300,'Magic2'),'hit':effect(10,5,400,'Magic2')},
        '23':{'kind':'firebang','cast':effect(1650,10,600),'hit':effect(1660,10,1000),'hit_anchor':'grid'},
        '29':{'kind':'mass_healing','cast':effect(1790,10,600),'hit':effect(1800,10,1000),'hit_anchor':'grid'},
    }
    manifest={'schema_version':1,'purpose':'first_loop_development_candidates','development_only':True,'runtime_ready':False,
              'status':'candidate','official_180_restoration_verified':False,'pillow_version':PILLOW_VERSION,
              'converter':'tools/resources/build_development_assets.py','converter_sha256':sha(Path(__file__)),
              'wil_decoder_sha256':sha(ROOT/'tools/resources/inspect_wil.py'),'map':map_info,'direction_names':DIRECTIONS,
              'coordinate_contract':'Source image top-left = entity draw origin + WIL offset_x,offset_y. PNGs remain uncropped and unscaled; y is down in source grid.',
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
                          evidence('资料参考/crystal/Client/MirObjects/Effect.cs',152,216,'16-direction projectile frames and additive rendering'),
                          evidence('资料参考/crystal/Client/MirObjects/PlayerObject.cs',4887,4919,'weapon/body/hair order by direction'),
                          evidence('资料参考/crystal/Client/MirObjects/MapCode.cs',226,259,'v0 layers, 1-based images, column-major cells'),
                          evidence('资料参考/crystal/Server/MirEnvir/Map.cs',112,151,'three layer 0x8000 collision bits')],
              'limitations':['Cross-source action/appearance mapping remains candidate; no official 1.80 behavior acceptance.',
                             'No guessed palette, frame deletion, reindexing, pixel scaling or fallback sprite is used.',
                             'Map crop and connected spawn selection are development fixtures; they are not historical spawn coordinates.']}
    importer.write_json('manifest.json',manifest)
    for record in importer.sources.values():
        if sha(Path(record['wil_path']))!=record['wil_sha256'] or sha(Path(record['wix_path']))!=record['wix_sha256']:raise ValueError('Source changed')
    print(json.dumps({'status':'checked' if args.check else 'generated','output':str(OUTPUT),'profiles':len(profiles),
                      'frames':len(importer.frames),'pngs':sum(x['png'] is not None for x in importer.frames.values()),
                      'map':map_info,'manifest_sha256':sha(OUTPUT/'manifest.json'),'runtime_ready':False},ensure_ascii=False))
    return 0
if __name__=='__main__':
    try:raise SystemExit(main())
    except (OSError,ValueError,KeyError,wil.UnsupportedFormat) as error:print(str(error),file=sys.stderr);raise SystemExit(2)
