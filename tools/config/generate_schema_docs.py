#!/usr/bin/env python3
"""从配置目录导出双端JSON数据结构契约，不读初始数值种子。"""
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]


def main():
    catalog=json.loads((ROOT/'config/schema/catalog.json').read_text(encoding='utf-8'))
    types={'int':'integer','number':'number','string':'string','bool':'boolean'}
    defs={}
    for target in ('server','client'):
        tables={}
        for table in catalog['tables']:
            fields=[f for f in table['fields'] if f['target'] in (target,'both')]
            if not fields:continue
            properties={}
            for f in fields:
                value={'type':types[f['type']]} if f['type'] in types else {}
                if 'enum_values' in f:value['enum']=f['enum_values']
                if 'minimum' in f:value['minimum']=f['minimum']
                if 'maximum' in f:value['maximum']=f['maximum']
                properties[f['name']]={'anyOf':[value,{'type':'null'}],'description':f['title']+(('，单位：'+f['unit']) if f.get('unit') else '')}
            key=target+'_'+table['table_id']
            defs[key]={'type':'object','properties':properties,'required':[f['name'] for f in fields],'additionalProperties':False}
            tables[table['table_id']]={'type':'array','items':{'$ref':'#/$defs/'+key}}
        defs[target+'_package']={'type':'object','properties':{'config_version':{'type':'string'},'runtime_ready':{'type':'boolean'},'tables':{'type':'object','properties':tables,'additionalProperties':False}},'required':['config_version','runtime_ready','tables'],'additionalProperties':False}
    schema={'$schema':'https://json-schema.org/draft/2020-12/schema','title':'双端配置研发数据结构','description':'nullable用于研发预览；正式发布还必须通过导出器领域规则、来源与验收门禁。json复合值以catalog.json的json_contracts为准。','$defs':defs}
    (ROOT/'config/schema/双端数据结构.schema.json').write_text(json.dumps(schema,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'definitions':len(defs),'generated':'config/schema/双端数据结构.schema.json'},ensure_ascii=False))


if __name__=='__main__':main()
