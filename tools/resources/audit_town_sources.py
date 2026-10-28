"""只读核对来源安全区与双侧地图原格；输出来源记录，不生成或启用运行地图。"""
import argparse
import hashlib
import json
from pathlib import Path
import re
import struct

ROOT=Path(__file__).resolve().parents[2]


def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()


def load_map(path):
    data=path.read_bytes()
    width,height=struct.unpack_from('<HH',data)
    if data[4:18]!=b'\rLegend of mir' or len(data)!=52+width*height*12:
        raise ValueError('不是已核验的52字节头+12字节列优先地图：'+str(path))
    def cell(x,y):
        offset=52+(x*height+y)*12
        raw=data[offset:offset+12]
        back,middle,front,door,door_offset,animation,tick,library,light=struct.unpack('<HHHBBBBBB',raw)
        return dict(raw=raw,blocked=bool((back|middle|front)&0x8000),front_library=library+2,
                    animated=animation!=0,door=door&0x7f,has_front=bool(front&0x7fff))
    return width,height,data,cell


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--server-root',required=True,type=Path)
    parser.add_argument('--client-root',required=True,type=Path)
    parser.add_argument('--report',default='.work/town-source-audit.json',type=Path)
    args=parser.parse_args()
    target=(ROOT/args.report).resolve()
    if not target.is_relative_to(ROOT):raise ValueError('报告须位于本项目内')
    start=args.server_root/'Envir/StartPoint.txt';local=ROOT/'资料参考/myf/Mir200/Envir/StartPoint.txt'
    if not local.is_file() or sha(local)!=sha(start):raise ValueError('安全区独立副本不存在或与指定原文件摘要不同')
    points=[];label='';source_text=local.read_bytes().decode('gb18030')
    for line_number,line in enumerate(source_text.splitlines(),1):
        stripped=line.strip()
        if not stripped:continue
        if stripped.startswith(';'):
            candidate=stripped[1:].strip()
            if candidate and not re.fullmatch(r'[-\s]+',candidate):label=candidate
            continue
        fields=stripped.split()
        if len(fields)!=8 or any(not re.fullmatch(r'\d+',value) for value in fields[1:]):raise ValueError('未核验的安全区行结构：'+str(line_number))
        map_code,x,y,no_chat,size,halo,pk_zone,pk_fire=fields
        points.append(dict(source_line=line_number,map_code=map_code,x=int(x),y=int(y),radius=int(size),
                           no_chat=int(no_chat),halo_type=int(halo),pk_zone=int(pk_zone),pk_fire=int(pk_fire),label=label))
    server_path=args.server_root/'Map/0.map';client_path=args.client_root/'Map/0.map'
    sw,sh,server_data,server_cell=load_map(server_path)
    cw,ch,client_data,client_cell=load_map(client_path)
    comparisons=[]
    for point in points:
        if point['map_code']!='0':continue
        x,y,r=point['x'],point['y'],point['radius']
        inside=lambda w,h:0<=x-r and 0<=y-r and x+r<w and y+r<h
        record=dict(source_line=point['source_line'],label=point['label'],center=dict(x=x,y=y),radius=r,
                    full_region_in_both_maps=inside(sw,sh) and inside(cw,ch))
        if record['full_region_in_both_maps']:
            cells=[(sx,sy) for sx in range(x-r,x+r+1) for sy in range(y-r,y+r+1)]
            record.update(cells=len(cells),server_blocked=sum(server_cell(*p)['blocked'] for p in cells),
                          client_blocked=sum(client_cell(*p)['blocked'] for p in cells),
                          raw_cell_differences=sum(server_cell(*p)['raw']!=client_cell(*p)['raw'] for p in cells),
                          collision_differences=sum(server_cell(*p)['blocked']!=client_cell(*p)['blocked'] for p in cells),
                          server_center_walkable=not server_cell(x,y)['blocked'],client_center_walkable=not client_cell(x,y)['blocked'],
                          front_libraries=sorted({client_cell(*p)['front_library'] for p in cells if client_cell(*p)['has_front']}),
                          animated_cells=sum(client_cell(*p)['animated'] for p in cells),door_cells=sum(bool(client_cell(*p)['door']) for p in cells))
        comparisons.append(record)
    report=dict(schema_version=1,source_record_only=True,runtime_configuration=False,runtime_ready=False,
                user_route='先接入来源城镇地图及安全区，再启用绑定与瞬移',
                sources=dict(start_points=dict(original=str(start),copy=local.relative_to(ROOT).as_posix(),sha256=sha(start),encoding='gb18030'),
                             server_map0=dict(path=str(server_path),sha256=sha(server_path),width=sw,height=sh),
                             client_map0=dict(path=str(client_path),sha256=sha(client_path),width=cw,height=ch)),
                whole_map_bytes_equal=server_data==client_data,
                map0_cell_bytes_equal=(sw,sh)==(cw,ch) and server_data[52:]==client_data[52:],
                source_safe_zone_rows=points,map0_safe_zone_comparisons=comparisons,
                limits=['StartPoint.txt注释定义安全区/复活点，不单凭文件名认证新角色初始选择或Crystal StartPoint标志',
                        '只核对0.map已核验格式，其它地图与版本未在本报告验收',
                        'MYF安全区半径映射Crystal Chebyshev语义仍属跨来源候选；未开放PK字段、收费或部署指令',
                        '原图摘要相同、局部格相同或来源记录生成均不等于城镇渲染、安全区业务、绑定或瞬移已实现'])
    target.parent.mkdir(parents=True,exist_ok=True);target.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
    print(json.dumps(dict(report=str(target),safe_zone_rows=len(points),whole_map_bytes_equal=report['whole_map_bytes_equal'],
                         map0_cell_bytes_equal=report['map0_cell_bytes_equal'],comparisons=comparisons),ensure_ascii=False))


if __name__=='__main__':main()
