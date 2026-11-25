"""为规则30的三个普通怪物生成原创开发精灵和 Creator 导入信息。"""

import argparse
import io
import json
from pathlib import Path

from PIL import Image, ImageDraw

from build_hud_assets import base_meta, image_meta, stable_uuid
from prepare_shinsu_assets import DIRECTIONS, ROOT, sha


DIR = ROOT/'client/assets/resources/development/hunt'
ORIENTATIONS = ((0,-1),(1,-1),(1,0),(1,1),(0,1),(-1,1),(-1,0),(-1,-1))
POSES = ('idle','step','strike','down')
SIZE = (48,38)


def flower(draw, cx, cy, dx, dy, pose):
    """绘制独立的枝叶、花冠和伏地形态。"""
    line, leaf, light = '#182c29', '#407d47', '#82aa4b'
    stem = (cx+dx*2, cy-13 if pose!='down' else cy-5)
    draw.line((cx,cy,*stem), fill=line, width=5)
    draw.line((cx,cy,*stem), fill=leaf, width=2)
    for side in (-1,1):
        reach = 13 if pose=='step' else 10
        draw.polygon([(cx,cy-1),(cx+side*reach,cy-8),(cx+side*6,cy-10),(cx+side*2,cy-5)],leaf,line)
        draw.line((cx,cy-3,cx+side*(reach-2),cy-8),fill=light)
    x,y=stem
    y-=2 if pose=='strike' else 0
    for ox,oy in [(-6,1),(6,1),(-4,-5),(0,-7),(4,-5)]:
        draw.ellipse((x+ox-4,y+oy-4,x+ox+4,y+oy+4),fill='#a64b8a' if pose!='down' else '#733d68',outline='#3d253e')
        draw.point((x+ox-1,y+oy-2),fill='#f1a873')
    draw.ellipse((x-4,y-3,x+4,y+4),fill='#524020',outline='#3d253e')
    draw.arc((x-3,y-4,x+3,y+3),185,355,fill='#f3c05e',width=2)


def hook_cat(draw,cx,cy,dx,dy,pose):
    """给斑纹兽保留前后脚、圆耳和钩形前爪的独立轮廓。"""
    outline='#3b292e'
    stance=2 if pose=='step' else 0
    low=pose=='down'
    for side in (-1,1):
        x=cx+side*(5+stance)
        draw.line((x,cy-8,x+dx*2,cy-1),fill=outline,width=5)
        draw.line((x,cy-8,x+dx*2,cy-2),fill='#805541',width=2)
    draw.ellipse((cx-10,cy-(9 if low else 17),cx+9,cy-1), fill='#99705a',outline=outline,width=2)
    draw.arc((cx-15,cy-15,cx-4,cy-4),105,282,fill='#b1805a',width=3)
    x,y=cx+dx*5,cy-(7 if low else 15)+dy*2
    for side in (-1,1):
        draw.ellipse((x+side*5-3,y-7,x+side*5+3,y-2),fill='#b58059',outline=outline)
    draw.ellipse((x-7,y-5,x+7,y+5),fill='#d5a47a',outline=outline)
    draw.line((x-5,y-2,x-4,y+1),fill='#895540',width=2)
    draw.line((x+4,y-2,x+5,y+1),fill='#895540',width=2)
    if dy>=0:
        draw.point((x-2+dx,y),fill='#3b292e')
        draw.point((x+2+dx,y),fill='#3b292e')
    if not low:
        claw=x+dx*(9 if pose=='strike' else 5)+(-6 if dy<0 else 6)
        draw.arc((claw-4,y+1,claw+6,y+10),350,206,fill='#e7b96d',width=2)


def spider(draw,cx,cy,dx,dy,pose):
    """绘制带相对方向和折腿动作的原创节肢怪物。"""
    down=pose=='down'
    y=cy-(4 if down else 8)
    for side in (-1,1):
        for leg in range(4):
            stride=(2 if leg%2 else -1) if pose=='step' else 0
            footx=cx+side*((8 if down else 17)-abs(leg-1)*2)
            footy=y+(leg-1)*4+stride
            points=[(cx+side*4,y+(leg-1)*2),(cx+side*(8 if down else 12),footy-5),(footx,footy)]
            draw.line(points, fill='#351f2a',width=4)
            draw.line(points, fill='#82606b' if down else '#a4646a',width=2)
    draw.ellipse((cx-8,y-7,cx+8,y+6),fill='#644651' if down else '#784243',outline='#341f2c',width=2)
    draw.arc((cx-6,y-6,cx+4,y+4),170,305,fill='#ce786b',width=2)
    x2,y2=cx+dx*(8 if pose=='strike' else 5),y+dy*4
    draw.ellipse((x2-4,y2-4,x2+4,y2+4),fill='#4b2935',outline='#341f2c')
    if not down:
        draw.line((x2-2,y2+1,x2+2,y2+1),fill='#e8b25f',width=1)


DESIGNS={'flower':flower,'cat':hook_cat,'spider':spider}


def encode(value):
    """序列化稳定的 UTF-8 资源和导入清单。"""
    return (json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode('utf-8')


def compile_assets():
    """生成场景渲染所需的有界动作帧，重复相同位姿只引用同一 PNG。"""
    outputs, profiles, frames = {}, {}, {}
    for kind, draw_actor in DESIGNS.items():
        profile=f'monster_m1_{kind}'
        actions={}
        for side,(dx,dy) in enumerate(ORIENTATIONS):
            for pose in POSES:
                image=Image.new('RGBA',SIZE)
                draw_actor(ImageDraw.Draw(image),24,30,dx,dy,pose)
                stream=io.BytesIO()
                image.save(stream,format='PNG')
                name=f'{kind}_{side}_{pose}'
                key=f'Hunt:{len(frames)}'
                data=stream.getvalue()
                frames[key]={'png':f'hunt/frames/{name}.png','png_sha256':sha(data),'width':SIZE[0],'height':SIZE[1],
                             'offset_x':0,'offset_y':-14,'declared_empty':False}
                outputs[f'frames/{name}.png']=data
                metadata=image_meta(f'hunt-{name}',*SIZE,0)
                for sub in metadata['subMetas'].values():
                    sub['displayName']=name
                outputs[f'frames/{name}.png.meta']=encode(metadata)
                for action,pattern,ms,loop in [
                    ('standing',('idle',),500,True),('walking',('idle','step','idle','step'),150,False),
                    ('attack',('idle','strike','strike','idle'),150,False),('struck',('strike','idle'),100,False),
                    ('die',('strike','down','down'),200,False),
                ]:
                    record=actions.setdefault(action,dict(count=len(pattern),duration_ms=ms,loop=loop,directions=[]))
                    if len(record['directions'])==side:
                        record['directions'].append(dict(direction=side,name=DIRECTIONS[side],frames=[None]*len(pattern)))
                    for index,recorded_pose in enumerate(pattern):
                        if pose==recorded_pose:
                            record['directions'][side]['frames'][index]=key
        profiles[profile]={'id':profile,'layers':[{'package':'Hunt','actions':actions}]}
    outputs['actors.json']=encode({'schema_version':1,'profiles':profiles,'frames':frames})
    outputs['actors.json.meta']=encode(base_meta('json','2.0.1',stable_uuid('hunt-actors'),['.json']))
    for folder in ('.','frames'):
        name='hunt' if folder=='.' else 'hunt-frames'
        outputs['../hunt.meta' if folder=='.' else 'frames.meta']=encode(base_meta('directory','1.2.0',stable_uuid(name),[]))
    return outputs


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check',action='store_true')
    args=parser.parse_args()
    for name,data in compile_assets().items():
        path=DIR/name
        if path.is_file() and path.read_bytes()==data:
            continue
        if args.check:
            raise ValueError(f'原创普通怪物资源与生成合同不符 {name}')
        path.parent.mkdir(parents=True,exist_ok=True)
        path.write_bytes(data)


if __name__=='__main__':
    main()
