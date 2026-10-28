#!/usr/bin/env python3
"""从唯一协议清单校验并生成显式Go/TS数值常量，不生成游戏实现。"""
import argparse
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    p = ROOT / 'shared/proto/protocol_manifest.json'
    manifest = json.loads(p.read_text(encoding='utf-8'))
    declared = set()
    proto_sources = {}
    for proto in (ROOT / 'shared/proto').glob('*.proto'):
        text = proto.read_text(encoding='utf-8')
        proto_sources[proto.name] = text
        for name in re.findall(r'\bmessage\s+(\w+)\s*\{', text):
            if name in declared:raise ValueError(f'重复PB消息 {name}')
            declared.add(name)
    scalars = {'double','float','int32','int64','uint32','uint64','sint32','sint64','fixed32','fixed64','sfixed32','sfixed64','bool','string','bytes'}
    for name,text in proto_sources.items():
        if 'syntax = "proto3";' not in text:raise ValueError(f'{name} 必须明确proto3')
        for imported in re.findall(r'import\s+"([^"]+)"',text):
            if imported not in proto_sources:raise ValueError(f'{name} 引用缺失 {imported}')
        for message,body in re.findall(r'\bmessage\s+(\w+)\s*\{([^}]*)\}',text,re.S):
            fields=set();numbers=set()
            for typ,field,number in re.findall(r'(?:repeated\s+)?(\w+)\s+(\w+)\s*=\s*(\d+)\s*;',body):
                number=int(number)
                if field in fields or number in numbers:raise ValueError(f'{message} 字段名或编号重复')
                if not 1<=number<2**29 or 19000<=number<=19999:raise ValueError(f'{message} 字段编号不合法')
                if typ not in scalars and typ not in declared:raise ValueError(f'{message} 未知字段类型 {typ}')
                fields.add(field);numbers.add(number)
    for family, prefix, upper in [('messages', 'Msg', 65534), ('error_codes', 'Code', 2**32-1)]:
        ids, names = set(), set()
        for row in manifest[family]:
            if not isinstance(row['value'], int) or not 0 <= row['value'] <= upper:
                raise ValueError(f'编号范围错误 {row}')
            if row['name'] in names or row['value'] in ids:
                raise ValueError(f'重复编号/名称 {row}')
            if not re.fullmatch(prefix+r'[A-Za-z0-9]+', row['name']):
                raise ValueError(f'非法常量名 {row}')
            if family == 'messages' and row['proto_message'] not in declared:
                raise ValueError(f"缺少PB消息 {row['proto_message']}")
            ids.add(row['value']); names.add(row['name'])
    reserved={r['value'] for r in manifest.get('reserved_messages',[])}
    if reserved & {r['value'] for r in manifest['messages']}:raise ValueError('消息编号占用了保留区段中的具体编号')
    if args.check:
        compiled='not_recorded'
        generation=ROOT/'conf/toolchain/go-pb-generation.json'
        if generation.exists():
            record=json.loads(generation.read_text(encoding='utf-8'))
            expected={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (ROOT/'shared/proto').glob('*.proto')}
            if record.get('sources')!=expected:raise ValueError('共享proto已变化，请先make protocol再校验')
            for relative,digest in record.get('outputs',{}).items():
                output=ROOT/relative
                if not output.exists() or hashlib.sha256(output.read_bytes()).hexdigest()!=digest:raise ValueError('生成Go PB与记录不一致：'+relative)
            compiled='verified_generated_sources_current'
        print(json.dumps({'valid':True,'messages':len(manifest['messages']),'error_codes':len(manifest['error_codes']),'proto_source_lint':'passed','protoc_compile':compiled,'game_business':'not_verified_by_generator'},ensure_ascii=False))
        return
    server_out=ROOT/'server/internal/protocol';server_out.mkdir(parents=True,exist_ok=True)
    client_out=ROOT/'client/assets/scripts/generated';client_out.mkdir(parents=True,exist_ok=True)
    go=['// 由protocol_manifest.json生成，草案未发布。','package protocol','']
    ts=['// 由protocol_manifest.json生成，草案未发布。','']
    for family,typ in [('messages','uint16'),('error_codes','uint32')]:
        go.append('const (')
        for row in manifest[family]:
            go.append(f"\t{row['name']} {typ} = {row['value']}")
            ts.append(f"export const {row['name']}: {row['value']} = {row['value']};")
        go.extend([')',''])
    (server_out/'protocol_ids.go').write_text('\n'.join(go),encoding='utf-8',newline='\n')
    (client_out/'protocol_ids.ts').write_text('\n'.join(ts)+'\n',encoding='utf-8',newline='\n')
    print(json.dumps({'server':str(server_out),'client':str(client_out),'implementation_status':'generated_constants_only'},ensure_ascii=False))


if __name__=='__main__':
    main()
