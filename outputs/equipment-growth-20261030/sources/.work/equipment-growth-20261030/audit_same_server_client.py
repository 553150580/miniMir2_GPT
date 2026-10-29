"""只读MYF同包美术清单/摘要/严格样例；不执行来源程序，不解析运营或登录器配置。"""
from datetime import datetime, timezone
import collections
import hashlib
import json
from pathlib import Path
import struct
import sys
import zlib

ROOT=Path(__file__).resolve().parents[2]
WORK=Path(__file__).resolve().parent
SOURCE=Path(r'D:\123DownLoad\20231121沉默世界【沐夜风】群服毕业版')
CLIENT=SOURCE/'沉默世界完整客户端（带单机登录器）'
DATA=CLIENT/'Data'
PATCH=CLIENT/'沉默世界补丁/data'
sys.path.insert(0,str(ROOT/'tools/resources'))
import inspect_wil as wil

def sha(path):return wil.sha256_file(path)
def save(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    path.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')

def native_probe(path,index):
    """独立审计0x19旧WIX签名变体；依据已复制Crystal默认48字节索引分支，不修改既有工具/源头。"""
    blob=path.read_bytes();offsets_raw=index.read_bytes()
    assert blob.startswith(b'#ILIB v1.0-WEMADE Entertainment inc.')
    assert offsets_raw.startswith(b'\x19WEMADE Entertainment inc.')
    count,palette_count,palette_bytes,version=struct.unpack_from('<IIII',blob,44)
    assert blob[40] not in (1,6) and blob[48]!=32 and (palette_count,palette_bytes,version)==(256,1024,0)
    assert (len(offsets_raw)-48)%4==0 and 0<count<=wil.MAX_FRAME_COUNT
    assert count==(len(offsets_raw)-48)//4==struct.unpack_from('<I',offsets_raw,44)[0]
    colors=struct.unpack('<255I',blob[60:1080]);assert all(c&0xff000000==0 for c in colors)
    palette=[(0,0,0,0)]+[((c>>16)&255,(c>>8)&255,c&255,255) for c in colors]
    offsets=struct.unpack_from(f'<{count}I',offsets_raw,48)
    distinct=sorted(set(offsets)-{0});following=dict(zip(distinct,distinct[1:]+[len(blob)]));frames=[]
    for id,offset in enumerate(offsets):
        if offset==0:
            frames.append({'frame_index':id,'source_offset':0,'width':0,'height':0,'offset_x':None,'offset_y':None,'pixel_bytes':0,'declared_empty':True})
            continue
        assert 1080<=offset and offset+8<=len(blob)
        width,height,x,y=struct.unpack_from('<hhhh',blob,offset)
        assert 0<=width<=wil.MAX_DIMENSION and 0<=height<=wil.MAX_DIMENSION and width*height<=wil.MAX_FRAME_PIXELS
        # Crystal CreateTexture在bytes.Length<=1时返回null；同包存在只有8字节头的1×1占位。
        # 保留声明尺寸/索引，不从下帧借像素或伪造PNG；其它不足尺寸一律拒绝。
        placeholder=width*height<=1 and following[offset]-offset-8<=1
        if not placeholder and offset+8+width*height>following[offset]:
            raise ValueError(f'{path.name} frame{id} payload exceeds next index: {width}x{height}, available={following[offset]-offset-8}')
        frames.append({'frame_index':id,'source_offset':offset,'width':width,'height':height,'offset_x':x,'offset_y':y,'pixel_bytes':0 if placeholder else width*height,'declared_empty':not bool(width*height) or placeholder,'placeholder_texture_null_by_crystal':placeholder})
    return {'frame_count':count,'palette':palette,'frames':frames,'palette_sha256':hashlib.sha256(blob[60:1080]).hexdigest()}

def main():
    assert CLIENT.is_dir() and DATA.is_dir() and PATCH.is_dir()
    selected=[p for p in DATA.iterdir() if p.stem.casefold() in {'items','stateitem','dnitems','weapon'} and p.suffix.lower() in {'.wil','.wix','.wzl','.wzx'}]
    selected += [p for p in PATCH.iterdir() if p.stem.casefold() in {'items','items1','items2','dnitems','dnitems1','dnitems2','hum','hum10','hum11','humeffect4','weapon','weapon10','weapon11'} and p.suffix.lower()=='.pak']
    source_before={str(p):sha(p) for p in selected};metadata=[]
    for path in selected:
        with path.open('rb') as stream:header=stream.read(64)
        metadata.append({'path':str(path),'bytes':path.stat().st_size,'sha256':source_before[str(path)],'header_hex':header.hex(),
            'header_family':'GEEPAK3' if header.startswith(b'\x07GEEPAK3') else 'shandagames_com_WZL_WZX' if header.startswith(b'\x13www.shandagames.com') else '#ILIB_v1_WIL' if header.startswith(b'#ILIB v1.0') else '0x19_WEMADE_WIX' if header.startswith(b'\x19WEMADE') else 'unknown'})
    probes=[];samples=[];wzl_parity=[];primary_parity=[]
    from PIL import Image,__version__ as pillow_version
    for package,ids in [('Items',[42,43]),('stateitem',[42,43]),('DnItems',[43]),('Weapon',[40800,41400])]:
        path=next(p for p in selected if p.parent==DATA and p.stem.casefold()==package.casefold() and p.suffix.lower()=='.wil')
        index=next(p for p in selected if p.parent==DATA and p.stem.casefold()==package.casefold() and p.suffix.lower()=='.wix')
        try:
            wil.probe(path,index)
            raise AssertionError('当前严格工具已改，需要重新审查本审计分支')
        except wil.UnsupportedFormat as error:tool_error=str(error)
        try:info=native_probe(path,index)
        except (AssertionError,ValueError) as error:
            probes.append({'package':package,'current_tool_supported':False,'current_tool_error':tool_error,'audit_variant_error':str(error),'mapping_certified':False})
            continue
        probes.append({'package':package,'frame_count':info['frame_count'],'current_tool_supported':False,'current_tool_error':tool_error,
            'audit_variant':'0x19_WEMADE_index48_wil_palette256_version0','all_index_frame_bounds_checked':True,'palette_sha256':info['palette_sha256'],
            'same_engine_numbering_or_action_mapping_certified':False})
        for id in ids:
            frame=info['frames'][id];rgba=wil.decode_frame(path,frame,info['palette'])
            png=WORK/'same-server-samples'/f'{package}_{id:06d}.png'
            image=Image.frombytes('RGBA',(frame['width'],frame['height']),rgba)
            if png.exists():
                with Image.open(png) as old:assert old.mode=='RGBA' and old.size==image.size and old.tobytes()==rgba
            else:png.parent.mkdir(parents=True,exist_ok=True);image.save(png)
            with Image.open(png) as decoded:assert decoded.tobytes()==rgba and decoded.size==image.size
            samples.append({'package':package,'raw_frame':id,**frame,'png':png.relative_to(ROOT).as_posix(),'png_sha256':sha(png),
                'rgba_sha256':hashlib.sha256(rgba).hexdigest(),'native_pixel_roundtrip':True,'crop_scale_recolor':False,
                'mapping_status':'Looks43_direct_index_hypothesis_not_consumer_proof' if id==43 else 'adjacent_comparison_only' if id==42 else 'Shape34_times1200_hypothesis_only_not_certified'})
            primary=WORK/'source-samples'/png.name
            if primary.is_file():
                with Image.open(primary) as old:
                    primary_parity.append({'package':package,'raw_frame':id,'primary_sample':primary.relative_to(ROOT).as_posix(),
                        'primary_sample_sha256':sha(primary),'same_dimensions':old.size==image.size,'same_rgba':old.size==image.size and old.tobytes()==rgba})
            compressed=next(p for p in selected if p.parent==DATA and p.stem.casefold()==package.casefold() and p.suffix.lower()=='.wzl')
            compressed_index=next(p for p in selected if p.parent==DATA and p.stem.casefold()==package.casefold() and p.suffix.lower()=='.wzx')
            cb,ci=compressed.read_bytes(),compressed_index.read_bytes()
            assert cb.startswith(b'\x13www.shandagames.com') and ci.startswith(b'\x13www.shandagames.com')
            offset=struct.unpack_from('<I',ci,48+4*id)[0]
            if offset==0:
                wzl_parity.append({'package':package,'raw_frame':id,'wzx_zero_index':True,'parity_checked':False})
                continue
            assert cb[offset]==3 and cb[offset+1:offset+4]==b'\x00\x00\x00'
            width,height,x,y,size=struct.unpack_from('<hhhhI',cb,offset+4)
            assert (width,height,x,y)==(frame['width'],frame['height'],frame['offset_x'],frame['offset_y'])
            indexes=struct.unpack_from(f'<{(len(ci)-48)//4}I',ci,48)
            end=min([n for n in indexes if n>offset]+[len(cb)])
            assert offset+16+size<=end
            decompressor=zlib.decompressobj();padded=decompressor.decompress(cb[offset+16:offset+16+size],(8192*8192)+1)
            assert decompressor.eof and not decompressor.unused_data and not decompressor.unconsumed_tail
            stride=((width+3)//4)*4;assert len(padded)==stride*height
            indices=b''.join(padded[row*stride:row*stride+width] for row in range(height))
            with path.open('rb') as stream:stream.seek(frame['source_offset']+8);native_indices=stream.read(width*height)
            wzl_parity.append({'package':package,'raw_frame':id,'wzx_zero_index':False,'parity_checked':True,
                'format_tag':3,'source_offset':offset,'compressed_bytes':size,'decoded_index_bytes':len(indices),
                'decoded_index_sha256':hashlib.sha256(indices).hexdigest(),'native_wil_index_sha256':hashlib.sha256(native_indices).hexdigest(),
                'same_raw_palette_indices':indices==native_indices,'palette_or_alpha_equivalence_certified':False})
    source_after={str(p):sha(p) for p in selected};assert source_before==source_after
    code=ROOT/'资料参考/crystal/LibraryEditor/Graphics/WeMadeLibrary.cs'
    report={'schema_version':1,'recorded_at_utc':datetime.now(timezone.utc).isoformat(),'status':'read_only_asset_audit_not_runtime_selection',
        'client_directory':str(CLIENT),'same_distribution_directory_as_server':True,'same_server_runtime_resource_precedence_certified':False,
        'archives':[{'name':p.name,'bytes':p.stat().st_size,'content_read':False,'executed':False} for p in SOURCE.iterdir() if p.is_file() and p.suffix.lower()=='.rar'],
        'inventory':{str(p):{'files':len([x for x in p.iterdir() if x.is_file()]),'extensions':dict(collections.Counter(x.suffix.lower() for x in p.iterdir() if x.is_file()))} for p in [DATA,PATCH]},
        'art_files':metadata,'source_before':source_before,'source_after':source_after,'source_stable':True,
        'probes':probes,'samples':samples,'same_server_wil_wzl_raw_index_comparison':wzl_parity,'named180_same_frame_rgba_comparison':primary_parity,'pillow_version':pillow_version,
        'manual_visual_inspection':{'method':'view_image原格，未裁切/缩放/重绘','frames':['Items:43','stateitem:43','Weapon:40800','Weapon:41400'],
            'observation':'Items/State43为棕色木剑形；Weapon两个假设帧为细长浅蓝/金属柄武器，不能据此认定乌木身体。两个Weapon帧原像素相同。'},
        'code_evidence':[{'path':code.relative_to(ROOT).as_posix(),'sha256':sha(code),'lines':[[42,47],[97,108],[112,148],[157,170],[223,235],[313,332],[377,394],[442,470]],
            'scope':'Crystal技术格式证据，非Apex/GXX运行编号、覆盖优先级或目标1.80认证'},
            {'path':'tools/resources/inspect_wil.py','sha256':sha(ROOT/'tools/resources/inspect_wil.py'),'lines':[[66,121],[124,136]],'scope':'当前生产解码器严格拒该WIX签名；未放宽生产工具，仅审计变体'}],
        'tool_sha256':sha(Path(__file__)),'production_converter_modified':False,'runtime_assets_modified':False,
        'remaining_unknowns':['MYF/GXX对同目录WIL/WZL/补丁GEEPAK3的真实资源优先级','Looks43实际消费编号是否直接等于原frame43','Shape34实际包分段/性别/动作步幅，不能用乘1200认证','GEEPAK3格式/密钥证据或原始资产导出，不猜密钥或绕过保护','同服候选与目标1.80外观是否匹配及完整方向/动作验收'],
        'forbidden_sources_read':False,'source_programs_executed':False,'network_or_database_used':False,
        'scope':'找到了同服完整客户端与补丁；不再称原件缺失。格式/原像素有限审计不等于编号、外观或目标版本认证；220仍未选入运行配置。'}
    save(WORK/'same-server-client-audit.json',report)
    print(json.dumps({'report':str(WORK/'same-server-client-audit.json'),'sha256':sha(WORK/'same-server-client-audit.json'),'source_files':len(selected),'native_samples':len(samples),'current_converter_changed':False,'source_stable':True},ensure_ascii=False))

if __name__=='__main__':main()
