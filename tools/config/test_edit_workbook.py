"""真实ZIP读写覆盖空模板追加/再次编辑，防止重复物理行遮蔽数据。"""
import hashlib
import json
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest.mock import patch
from xml.dom import minidom
import zipfile

import edit_workbook as editor
import export_configs as config


class WorkbookEditTests(unittest.TestCase):
    def setUp(self):
        self.directory=tempfile.TemporaryDirectory();self.addCleanup(self.directory.cleanup)
        self.root=Path(self.directory.name);(self.root/'config/schema').mkdir(parents=True);(self.root/'outputs').mkdir();(self.root/'.work').mkdir()
        self.book=self.root/'outputs/book.xlsx'
        schema={'workbooks':[{'group':'gameplay','path':'outputs/book.xlsx'}],
            'tables':[{'table_id':'sample','sheet_name':'样例','workbook_group':'gameplay','fields':[{'name':'id','type':'int'},{'name':'payload','type':'string'}]}]}
        (self.root/'config/schema/catalog.json').write_text(json.dumps(schema),encoding='utf-8')
        self.scope=patch.object(editor,'ROOT',self.root);self.scope.start();self.addCleanup(self.scope.stop)

    def write_book(self,extra_rows):
        members={'xl/workbook.xml':f'<workbook xmlns="{editor.MAIN}" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="样例" sheetId="1" r:id="rId1"/></sheets></workbook>',
          'xl/_rels/workbook.xml.rels':'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>',
          'xl/worksheets/sheet1.xml':f'<worksheet xmlns="{editor.MAIN}"><sheetData><row r="2"><c r="A2" t="inlineStr"><is><t>id</t></is></c><c r="B2" t="inlineStr"><is><t>payload</t></is></c></row><row r="4" ht="20"><c r="A4" s="0"><v>1</v></c><c r="B4" s="0" t="inlineStr"><is><t>原记录</t></is></c><c r="C4"><f>A4+1</f><v>2</v></c></row>{extra_rows}</sheetData></worksheet>',
          'preserved/unknown.xml':'<other xmlns="urn:preserved" attribute="不应改写"/>'}
        with zipfile.ZipFile(self.book,'w') as archive:
            for name,value in members.items():archive.writestr(name,value.encode('utf-8'))

    def apply(self,kind,id,payload,repair=False):
        plan={'workbook':'outputs/book.xlsx','group':'gameplay','input_workbook_sha256':config.sha256(self.book),
            'normalize_empty_duplicate_rows':repair,'operations':[{'kind':kind,'table_id':'sample','id':id,'values':{'id':id,'payload':payload} if kind=='append' else {'payload':payload}}]}
        path=self.root/'.work/plan.json';path.write_text(json.dumps(plan),encoding='utf-8')
        output=self.root/'.work/candidate.xlsx';report=editor.apply(path,output)
        return output,report

    def test_append_reuses_blank_template_then_update_hits_same_physical_row(self):
        self.write_book('<row r="5" ht="15"><c r="A5" s="0"/><c r="B5" s="0"/></row>')
        before=self.book.read_bytes();output,report=self.apply('append',2,'新增记录')
        self.assertEqual(self.book.read_bytes(),before)
        with zipfile.ZipFile(output) as archive:
            document=minidom.parseString(archive.read('xl/worksheets/sheet1.xml'))
            rows=[row for row in document.getElementsByTagNameNS(editor.MAIN,'row') if row.getAttribute('r')=='5']
            self.assertEqual(len(rows),1);self.assertEqual(rows[0].getAttribute('ht'),'15')
            self.assertEqual(archive.read('preserved/unknown.xml'),'<other xmlns="urn:preserved" attribute="不应改写"/>'.encode())
        shutil.copyfile(output,self.book)
        output,report=self.apply('update',2,'再次编辑')
        self.assertEqual(config.read_xlsx(output)['样例'][5][2],'再次编辑')
        self.assertEqual(config.read_xlsx(output)['样例'][4][3],2)

    def test_explicit_repair_keeps_populated_row_style_and_all_values(self):
        self.write_book('<row r="5" ht="15"><c r="A5" s="0"/><c r="B5" s="0"/></row><row r="5" ht="196"><c r="A5" s="0"><v>2</v></c><c r="B5" s="0" t="inlineStr"><is><t>已有数据</t></is></c></row>')
        before=self.book.read_bytes();output,report=self.apply('update',2,'小地图字段',repair=True)
        self.assertEqual(len(report['normalized_empty_duplicate_rows']),1)
        self.assertEqual(self.book.read_bytes(),before)
        with zipfile.ZipFile(output) as archive:
            document=minidom.parseString(archive.read('xl/worksheets/sheet1.xml'))
            rows=[row for row in document.getElementsByTagNameNS(editor.MAIN,'row') if row.getAttribute('r')=='5']
            self.assertEqual(len(rows),1);self.assertEqual(rows[0].getAttribute('ht'),'196')
        self.assertEqual(config.read_xlsx(output)['样例'][5][2],'小地图字段')

    def test_repair_is_explicit_and_conflicting_data_is_never_merged(self):
        for repair,second in [(False,'<row r="5"/>'),(True,'<row r="5"><c r="B5" t="inlineStr"><is><t>冲突</t></is></c></row>')]:
            self.write_book('<row r="5"><c r="A5"><v>2</v></c><c r="B5" t="inlineStr"><is><t>原值</t></is></c></row>'+second)
            before=self.book.read_bytes()
            with self.assertRaises(ValueError):self.apply('update',2,'新值',repair)
            self.assertEqual(self.book.read_bytes(),before)

    def test_duplicate_cell_addresses_are_rejected(self):
        self.write_book('<row r="5"><c r="A5"><v>2</v></c><c r="B5" t="inlineStr"><is><t>原值</t></is></c><c r="B5" t="inlineStr"><is><t>隐藏覆盖</t></is></c></row>')
        with self.assertRaisesRegex(ValueError,'重复单元格'):self.apply('update',2,'新值',True)

    def test_unique_metadata_on_empty_duplicate_is_not_discarded(self):
        self.write_book('<row r="5"><c r="A5" s="7"/><c r="B5"/></row><row r="5"><c r="A5"><v>2</v></c><c r="B5" t="inlineStr"><is><t>原值</t></is></c></row>')
        with self.assertRaisesRegex(ValueError,'独有样式'):self.apply('update',2,'新值',True)


if __name__=='__main__':unittest.main()
