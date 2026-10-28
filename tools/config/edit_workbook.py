"""按显式单次编辑清单生成Excel候选副本；保留其它ZIP成员和单元格，不覆盖唯一源表。"""
import argparse
import hashlib
import json
from pathlib import Path
import re
from xml.dom import minidom
import zipfile

import export_configs as config

ROOT=config.ROOT
MAIN='http://schemas.openxmlformats.org/spreadsheetml/2006/main'


def column(number):
    result=''
    while number:
        number,rest=divmod(number-1,26);result=chr(65+rest)+result
    return result


def element(document,name):
    prefix=document.documentElement.prefix
    return document.createElementNS(MAIN,(prefix+':' if prefix else '')+name)


def children(node,name):
    return [c for c in node.childNodes if c.nodeType==c.ELEMENT_NODE and c.localName==name]


def has_row_data(row):
    for cell in children(row,'c'):
        if children(cell,'f'):return True
        for value in children(cell,'v')+children(cell,'is'):
            if any(node.data for descendant in value.getElementsByTagName('*') for node in descendant.childNodes if node.nodeType==node.TEXT_NODE):return True
            if any(node.nodeType==node.TEXT_NODE and node.data for node in value.childNodes):return True
    return False


def normalize_empty_duplicate_rows(document,allow):
    """只删除已审计的无值模板重复行；含值冲突或独有元数据一律拒绝。"""
    sheet_data=document.getElementsByTagNameNS(MAIN,'sheetData')[0];groups={};result=[]
    for row in children(sheet_data,'row'):
        addresses=[cell.getAttribute('r') for cell in children(row,'c')]
        if len(set(addresses))!=len(addresses):raise ValueError('同一行含重复单元格地址，禁止自动覆盖')
        if any(not re.fullmatch('[A-Z]+'+row.getAttribute('r'),address) for address in addresses):raise ValueError('单元格地址与所在行不一致')
        groups.setdefault(int(row.getAttribute('r')),[]).append(row)
    for number,rows in groups.items():
        if len(rows)<2:continue
        if not allow:raise ValueError('工作表存在重复行，须先审计并显式允许清理空模板：'+str(number))
        populated=[row for row in rows if has_row_data(row)]
        if len(populated)!=1:raise ValueError('重复行不是唯一含值行与空模板，禁止自动合并：'+str(number))
        winner=populated[0];winner_cells={cell.getAttribute('r'):cell for cell in children(winner,'c')}
        for empty in rows:
            if empty is winner:continue
            for key,value in empty.attributes.items():
                if key not in ('r','ht','customHeight') and winner.getAttribute(key)!=value:raise ValueError('重复空行有独有行元数据，禁止丢弃')
            if any(node.nodeType==node.ELEMENT_NODE and node.localName!='c' for node in empty.childNodes):raise ValueError('重复空行有未知内容，禁止丢弃')
            for cell in children(empty,'c'):
                target=winner_cells.get(cell.getAttribute('r'))
                if target is None:raise ValueError('重复空行有独有格式单元格，禁止丢弃')
                if any(node.nodeType==node.ELEMENT_NODE and node.localName not in ('v','is') for node in cell.childNodes):raise ValueError('重复空单元格有未知元数据')
                for key,value in cell.attributes.items():
                    if key not in ('r','t') and target.getAttribute(key)!=value:raise ValueError('重复空单元格有独有样式或元数据')
            sheet_data.removeChild(empty)
        result.append(dict(row=number,removed_empty_rows=len(rows)-1,retained_populated_row_unchanged=True))
    return result


def apply(plan_path,output):
    plan=json.loads(plan_path.read_text(encoding='utf-8'))
    schema=config.load_json(ROOT/'config/schema/catalog.json')
    book=ROOT/plan['workbook']
    if book.resolve()!=next((ROOT/w['path']).resolve() for w in schema['workbooks'] if w['group']==plan['group']):raise ValueError('编辑输入必须是唯一工作簿')
    if config.sha256(book)!=plan['input_workbook_sha256']:raise ValueError('输入Excel摘要已变化，不生成候选')
    if not output.resolve().is_relative_to(ROOT/'.work') or output.resolve()==book.resolve():raise ValueError('候选输出必须位于本项目.work内')
    with zipfile.ZipFile(book) as source:
        contents={info.filename:source.read(info.filename) for info in source.infolist()}
        infos=source.infolist()
    workbook=minidom.parseString(contents['xl/workbook.xml'])
    relationships=minidom.parseString(contents['xl/_rels/workbook.xml.rels'])
    targets={n.getAttribute('Id'):n.getAttribute('Target') for n in relationships.documentElement.childNodes if n.nodeType==n.ELEMENT_NODE}
    sheets={n.getAttribute('name'):targets[n.getAttributeNS('http://schemas.openxmlformats.org/officeDocument/2006/relationships','id')]
            for n in workbook.getElementsByTagNameNS(MAIN,'sheet')}
    raw=config.read_xlsx(book);documents={};edits=[];normalized=[]
    if plan.get('normalize_empty_duplicate_rows',False):
        for name,target in sheets.items():
            path=target.lstrip('/') if target.startswith('/') else 'xl/'+target
            document=minidom.parseString(contents[path]);repairs=normalize_empty_duplicate_rows(document,True)
            if repairs:
                documents[path]=document;normalized.extend(dict(sheet=name,**repair) for repair in repairs)
    tables={t['table_id']:t for t in schema['tables'] if t['workbook_group']==plan['group']}
    for operation in plan['operations']:
        table=tables[operation['table_id']];name=table['sheet_name']
        path=sheets[name].lstrip('/') if sheets[name].startswith('/') else 'xl/'+sheets[name]
        if path not in documents:
            documents[path]=minidom.parseString(contents[path]);normalize_empty_duplicate_rows(documents[path],False)
        document=documents[path]
        sheet_data=document.getElementsByTagNameNS(MAIN,'sheetData')[0]
        rows=children(sheet_data,'row')
        row_number=next((n for n,values in raw[name].items() if n>=4 and values.get(1)==operation['id']),None)
        append=operation['kind']=='append'
        if append and row_number is not None or not append and row_number is None:raise ValueError('记录存在状态与编辑种类不符')
        if append:
            if set(operation['values'])!={f['name'] for f in table['fields']}:raise ValueError('追加记录必须提供全部字段，未知值保持空白')
            row_number=max(raw[name])+1
            template=next(r for r in rows if int(r.getAttribute('r'))==max(n for n in raw[name] if n>=4))
            row=next((r for r in rows if int(r.getAttribute('r'))==row_number),None)
            if row is not None:
                if has_row_data(row):raise ValueError('追加目标已有未识别的内容，禁止覆盖')
                template=row
            else:
                row=element(document,'row');row.setAttribute('r',str(row_number))
                for key,value in template.attributes.items():
                    if key!='r':row.setAttribute(key,value)
                later=next((r for r in rows if int(r.getAttribute('r'))>row_number),None)
                if later:sheet_data.insertBefore(row,later)
                else:sheet_data.appendChild(row)
            raw[name][row_number]={1:operation['id']}
        else:
            row=next(r for r in rows if int(r.getAttribute('r'))==row_number);template=row
        for field,value in operation['values'].items():
            index=next((i+1 for i,f in enumerate(table['fields']) if f['name']==field),None)
            if index is None:raise ValueError('字段不在schema内：'+field)
            address=column(index)+str(row_number)
            cell=next((c for c in children(row,'c') if c.getAttribute('r')==address),None)
            if cell is None:
                cell=element(document,'c');cell.setAttribute('r',address)
                old_template=next((c for c in children(template,'c') if re.match('[A-Z]+',c.getAttribute('r')).group()==column(index)),None)
                if old_template is not None and old_template.hasAttribute('s'):cell.setAttribute('s',old_template.getAttribute('s'))
                later=next((c for c in children(row,'c') if len(re.match('[A-Z]+',c.getAttribute('r')).group())>len(column(index)) or len(re.match('[A-Z]+',c.getAttribute('r')).group())==len(column(index)) and re.match('[A-Z]+',c.getAttribute('r')).group()>column(index)),None)
                if later:row.insertBefore(cell,later)
                else:row.appendChild(cell)
            if children(cell,'f'):raise ValueError('不能直接覆盖公式单元格：'+address)
            for node in list(cell.childNodes):cell.removeChild(node)
            cell.removeAttribute('t') if cell.hasAttribute('t') else None
            if value is not None:
                if isinstance(value,(dict,list)):value=json.dumps(value,ensure_ascii=False,separators=(',',':'))
                if isinstance(value,str):
                    cell.setAttribute('t','inlineStr');node=element(document,'is');text=element(document,'t')
                    text.setAttribute('xml:space','preserve');text.appendChild(document.createTextNode(value));node.appendChild(text)
                else:
                    if isinstance(value,bool):cell.setAttribute('t','b');value=int(value)
                    node=element(document,'v');node.appendChild(document.createTextNode(str(value)))
                cell.appendChild(node)
            edits.append(dict(table=table['table_id'],sheet=name,cell=address,field=field,value=value))
        dimensions=document.getElementsByTagNameNS(MAIN,'dimension')
        if dimensions:
            old=dimensions[0].getAttribute('ref');end=old.split(':')[-1];last=int(re.search(r'\d+$',end).group())
            if row_number>last:dimensions[0].setAttribute('ref',old.split(':')[0]+':'+re.match('[A-Z]+',end).group()+str(row_number))
    for path,document in documents.items():contents[path]=document.toxml(encoding='utf-8')
    output.parent.mkdir(parents=True,exist_ok=True)
    with zipfile.ZipFile(output,'w') as target:
        for info in infos:target.writestr(info,contents[info.filename])
    after=config.read_xlsx(output)
    for operation in plan['operations']:
        table=tables[operation['table_id']];values=next(v for n,v in after[table['sheet_name']].items() if n>=4 and v.get(1)==operation['id'])
        for key,wanted in operation['values'].items():
            index=next(i+1 for i,f in enumerate(table['fields']) if f['name']==key)
            field=table['fields'][index-1]
            if config.parse_value(values.get(index),field)!=wanted:raise ValueError('Excel候选回读不符：'+key)
    with zipfile.ZipFile(book) as original,zipfile.ZipFile(output) as staged:
        untouched=[name for name in original.namelist() if name not in documents]
        if not all(original.read(name)==staged.read(name) for name in untouched):raise ValueError('未编辑ZIP成员发生变化')
    return dict(input_sha256=plan['input_workbook_sha256'],output_sha256=config.sha256(output),edited_worksheets=list(documents),
                untouched_zip_members_verified=len(untouched),all_requested_values_readback_equal=True,edited_cells=len(edits),
                formulas_overwritten=False,source_workbook_overwritten=False,normalized_empty_duplicate_rows=normalized)


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('plan',type=Path);parser.add_argument('output',type=Path)
    args=parser.parse_args();report=apply(args.plan,args.output)
    args.output.with_suffix('.review.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
    print(json.dumps(report,ensure_ascii=False))
