// 初次生成配置工作簿。已存在时拒绝覆盖；后续编辑Excel并运行export_configs.py。
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import crypto from 'node:crypto';
import { Workbook, SpreadsheetFile } from '@oai/artifact-tool';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const out = path.join(root, 'outputs/development-prep-20261026');
const previewDir = path.join(root, '.work/previews');
const fragmentDir = path.join(root, 'config/catalog/fragments');
const hash = b => crypto.createHash('sha256').update(b).digest('hex');
const col = n => { let s=''; for (;n>0;n=Math.floor((n-1)/26)) s=String.fromCharCode(65+(n-1)%26)+s; return s; };
const files = (await fs.readdir(fragmentDir)).filter(x=>x.endsWith('.json')).sort();
const fragments = await Promise.all(files.map(async x=>JSON.parse(await fs.readFile(path.join(fragmentDir,x),'utf8'))));
const groupLabels = {gameplay:'玩法配置',architecture:'架构配置'};
const schema = {schema_version:1, source_locks:[], workbooks:[], tables:[], blockers:[], json_contracts:[]};
function shapeOf(v) {
  if(v===null)return {type:'null'};
  if(Array.isArray(v)){
    const shapes=[...new Map(v.map(x=>{const s=shapeOf(x);return[JSON.stringify(s),s];})).values()];
    return {type:'array',items:shapes.length===1?shapes[0]:{any_of:shapes}};
  }
  if(typeof v==='object')return {type:'object',properties:Object.fromEntries(Object.entries(v).map(([k,x])=>[k,shapeOf(x)]))};
  return {type:typeof v==='number'?(Number.isInteger(v)?'int':'number'):typeof v,...(typeof v==='number'&&v>=0?{minimum:0}:{})};
}
function stable(v) {
  if(Array.isArray(v))return '['+v.map(stable).join(',')+']';
  if(v&&typeof v==='object')return '{'+Object.keys(v).sort().map(k=>JSON.stringify(k)+':'+stable(v[k])).join(',')+'}';
  return JSON.stringify(v);
}
for (const source of ['最终策划案.md','架构方案.md']) schema.source_locks.push({path:source,sha256:hash(await fs.readFile(path.join(root,source)))});
const groups = {};
for (const fragment of fragments) {
  const group = fragment.workbook_group;
  if (!groupLabels[group]) throw new Error(`未知配置分组 ${group}`);
  groups[group] ??= {tables:[],blockers:[]};
  groups[group].tables.push(...fragment.tables);
  groups[group].blockers.push(...fragment.blockers);
}
if (!groups.gameplay || !groups.architecture) throw new Error('必须同时具备玩法与架构数据');
await fs.mkdir(out,{recursive:true});
await fs.mkdir(previewDir,{recursive:true});
for (const group of Object.keys(groups)) {
  const target=path.join(out,`${groupLabels[group]}.xlsx`);
  try { await fs.access(target); throw new Error(`已存在 ${target}。请直接编辑工作簿，不使用初始种子覆盖`); }
  catch(e) { if(e.code!=='ENOENT') throw e; }
}
const allTableIds = new Set();
const qa = [];

function formatBody(s,range) {
  s.showGridLines=false;
  s.getRange(range).format={font:{name:'Arial',size:11,color:'#243247'},verticalAlignment:'center',rowHeight:26};
}
function header(s,range) {
  s.getRange(range).format={fill:'#243B53',font:{name:'Arial',size:11,bold:true,color:'#FFFFFF'},rowHeight:30,verticalAlignment:'center',horizontalAlignment:'center',wrapText:true};
}

for (const [group, model] of Object.entries(groups)) {
  const wb=Workbook.create();
  const summary=wb.worksheets.add('配置目录');
  const tabs = new Set(['配置目录','发布缺口','使用说明']);
  for (const t of model.tables) {
    if(allTableIds.has(t.table_id)) throw new Error(`重复table_id ${t.table_id}`);
    allTableIds.add(t.table_id);
    if(t.sheet_name.length>31 || tabs.has(t.sheet_name)) throw new Error(`非法/重复表名 ${t.sheet_name}`);
    tabs.add(t.sheet_name);
    const fns=t.fields.map(f=>f.name);
    for(const n of ['id','record_status','source_ref','note']) if(!fns.includes(n)) throw new Error(`${t.table_id}缺少元数据${n}`);
    if(new Set(fns).size!==fns.length) throw new Error(`${t.table_id}重复字段`);
    for (const f of t.fields) {
      if (!/^[a-z][a-z0-9_]*$/.test(f.name)) throw new Error(`非法字段 ${f.name}`);
      if(!['int','number','bool','string','json'].includes(f.type)) throw new Error(`非法类型${f.type}`);
      if(!['server','client','both','none'].includes(f.target)) throw new Error(`非法导出目标${f.target}`);
    }
    wb.worksheets.add(t.sheet_name);
  }
  const missing=wb.worksheets.add('发布缺口');
  const guide=wb.worksheets.add('使用说明');
  const maxSummaryRow=7+model.tables.length;
  formatBody(summary,`A1:H${maxSummaryRow}`);
  summary.tabColor='#243B53';
  summary.getRange('A2').values=[[`${groupLabels[group]}目录`]];
  summary.getRange('A2').format.font={name:'Arial',size:16,bold:true,color:'#243B53'};
  summary.getRange('A3').values=[['2026-10-26　维护Excel数据区，导出时校验来源、类型和发布缺口']];
  summary.getRange('A3:H3').format.rowHeight=30;
  summary.getRange('A5:H5').values=[['配置表','table_id','记录数','已定','候选','待核验','历史','发布必需']];
  header(summary,'A5:H5');
  summary.getRange('A1:A1').format.columnWidth=29;
  summary.getRange(`B1:B${maxSummaryRow}`).format.columnWidth=36;
  summary.getRange(`C1:H${maxSummaryRow}`).format.columnWidth=13;
  summary.getRange(`A1:A${maxSummaryRow}`).format.columnWidth=29;
  summary.getRange(`H6:H${maxSummaryRow}`).format.horizontalAlignment='center';
  summary.freezePanes.freezeRows(5);

  for (let idx=0;idx<model.tables.length;idx++) {
    const t=model.tables[idx], s=wb.worksheets.getItem(t.sheet_name);
    const endCol=col(t.fields.length), endRow=Math.max(t.rows.length+3,9);
    formatBody(s,`A1:${endCol}${endRow}`);
    const matrix=[t.fields.map(f=>f.title),t.fields.map(f=>f.name),t.fields.map(f=>[f.type,f.required?'必填':'可空',f.unit||'',f.target].filter(Boolean).join(' / '))];
    for (const r of t.rows) matrix.push(t.fields.map(f=>{
      const v=r[f.name]??null;
      return f.type==='json'&&v!==null ? JSON.stringify(v) : v;
    }));
    s.getRange(`A1:${endCol}${matrix.length}`).values=matrix;
    header(s,`A1:${endCol}1`);
    s.getRange(`A2:${endCol}2`).format={fill:'#E7EDF3',font:{name:'Arial',size:11,color:'#344E68'},rowHeight:30,wrapText:true};
    s.getRange(`A3:${endCol}3`).format={fill:'#F3F5F7',font:{name:'Arial',size:11,color:'#526579'},rowHeight:48,wrapText:true,verticalAlignment:'center'};
    for(let i=0;i<t.fields.length;i++) {
      const f=t.fields[i], c=col(i+1), rg=s.getRange(`${c}1:${c}${endRow}`);
      const longestCode=f.name.length+2;
      let width=Math.max(18,Math.min(36,longestCode));
      if(f.type==='json')width=54;
      if(f.name==='source_ref')width=48;
      if(f.name==='note')width=70;
      if(f.name==='rule_key')width=62;
      rg.format.columnWidth=width;
      if(['string','json'].includes(f.type))s.getRange(`${c}4:${c}${endRow}`).format.wrapText=true;
      if (['int','number'].includes(f.type)) s.getRange(`${c}4:${c}${endRow}`).setNumberFormat(f.type==='int'?'#,##0':'General');
      if (f.enum_values && f.enum_values.join(',').length<250) s.getRange(`${c}4:${c}${Math.max(endRow,1003)}`).dataValidation={rule:{type:'list',values:f.enum_values.map(String)}};
      if (f.name==='record_status') {
        for(const [text,fill,color] of [['candidate','#FFF1CC','#825600'],['unverified','#FCE3E0','#9B2C2C'],['historical','#E9ECEF','#526579']]) s.getRange(`${c}4:${c}${Math.max(endRow,1003)}`).conditionalFormats.add('containsText',{text,format:{fill,font:{color}}});
      }
      if(f.required && !['record_status','source_ref'].includes(f.name)) {
        s.getRange(`${c}4:${c}${Math.max(endRow,1003)}`).conditionalFormats.addCustom(`AND($A4<>"",ISBLANK(${c}4))`,{fill:'#FFF1CC'});
      }
      if(['source_ref','note'].includes(f.name)) s.getRange(`${c}4:${c}${endRow}`).format.font={name:'Arial',size:11,color:'#526579'};
    }
    s.getRange(`A4:${endCol}${endRow}`).format.autofitRows();
    s.getRange(`A4:A${endRow}`).format.horizontalAlignment='center';
    s.freezePanes.freezeRows(3);s.freezePanes.freezeColumns(1);
    const sn=t.sheet_name.replaceAll("'","''"), sc=col(t.fields.findIndex(f=>f.name==='record_status')+1), row=idx+6;
    summary.getRange(`A${row}:B${row}`).values=[[t.title,t.table_id]];
    summary.getRange(`C${row}:G${row}`).formulas=[[
      `=COUNTA('${sn}'!$A$4:$A$10003)`,
      ...['confirmed','candidate','unverified','historical'].map(st=>`=COUNTIFS('${sn}'!$${sc}$4:$${sc}$10003,"${st}")`)
    ]];
    summary.getRange(`H${row}`).values=[[t.release_required?'是':'否']];
    const {rows,...definition}=t;
    schema.tables.push({...definition,workbook_group:group});
    for(const r of rows)for(const f of t.fields)if(f.type==='json' && (r[f.name]!=null||r.rule_key) && r.record_status!=='historical'){
      schema.json_contracts.push({table_id:t.table_id,id:r.id,field:f.name,rule_key:r.rule_key||null,shape:r[f.name]==null?{type:'unverified'}:shapeOf(r[f.name]),source_ref:r.source_ref,
        ...(r.record_status==='confirmed'&&r[f.name]!=null?{confirmed_sha256:hash(stable(r[f.name]))}:{})});
    }
  }
  const blockers=model.blockers.map((b,i)=>({...b,id:b.id||`${group.toUpperCase()}-${i+1}`}));
  schema.blockers.push(...blockers);
  const mr=Math.max(4,blockers.length+3);
  formatBody(missing,`A1:G${mr}`);
  missing.getRange('A1:G3').values=[
    ['缺口编号','领域','待补内容','完成条件','状态','验证证据或变更引用','来源'],
    ['id','domain','description','resolution','status','evidence','source_ref'],
    ['稳定编号','配置或资料域','缺失项；不以0代替','补齐并完成核验','open / resolved / not_applicable','关闭缺口必须填写','依据文件章节']
  ];
  header(missing,'A1:G1');
  missing.getRange('A2:G3').format={fill:'#E7EDF3',wrapText:true,rowHeight:40};
  if(blockers.length) missing.getRange(`A4:G${mr}`).values=blockers.map(b=>[b.id,b.domain,b.description,b.resolution,'open',null,b.source_ref]);
  for(const [c,w] of [['A',42],['B',23],['C',78],['D',78],['E',22],['F',72],['G',48]]) missing.getRange(`${c}1:${c}${mr}`).format.columnWidth=w;
  missing.getRange(`A4:A${mr}`).format.wrapText=true;
  missing.getRange(`C4:D${mr}`).format.wrapText=true;
  missing.getRange(`A4:G${mr}`).format.rowHeight=60;
  missing.getRange('E4:E1003').dataValidation={rule:{type:'list',values:['open','resolved','not_applicable']}};
  missing.getRange('E4:E1003').conditionalFormats.add('containsText',{text:'open',format:{fill:'#FFF1CC',font:{color:'#825600'}}});
  missing.freezePanes.freezeRows(3);missing.freezePanes.freezeColumns(2);
  formatBody(guide,'A1:C16');guide.tabColor='#8795A5';
  guide.getRange('A1:C12').values=[
    ['维护项目','执行规则','说明'],
    ['数据源','本工作簿第4行起','第1行为中文标题，第2行字段名，第3行类型/必填/单位/双端目标'],
    ['规则依据','最终策划案.md','玩法、数值、当前范围按最终策划；架构方案提供技术参考'],
    ['记录状态','confirmed / candidate / unverified / historical','已定 / 首测候选 / 待核验 / 历史样本；历史不进入运行配置'],
    ['缺少参数','保持空白','空白与0不同，必填缺失禁止正式发布'],
    ['导出目标','server / client / both / none','客户端不接收服务端专用字段，来源及备注均不下发'],
    ['元宝精度','显示1元宝 = 内部10','账本金钱用整数，来源架构百分精度已列适配'],
    ['改表方式','新增记录；保留稳定id','新增字段需同步schema；不重排/改写第2行字段名'],
    ['结构校验','python3 tools/config/export_configs.py --check','结构正确不等于实现和数值已验收'],
    ['研发预览','python3 tools/config/export_configs.py','预览含候选与空值，读取runtime_ready后决定是否可运行'],
    ['正式门禁','python3 tools/config/export_configs.py --check --mode release','必需配置、技能证据、资源映射和业务验收未齐时返回失败'],
    ['首次种子','config/catalog/fragments','仅保留本次来源提取记录，后续数值以本工作簿为唯一维护源']
  ];
  header(guide,'A1:C1');
  guide.getRange('A1:A16').format.columnWidth=22;guide.getRange('B1:B16').format.columnWidth=76;guide.getRange('C1:C16').format.columnWidth=102;
  guide.getRange('A1:C16').format.rowHeight=34;
  wb.recalculate();
  const err=await wb.inspect({kind:'match',searchTerm:'#REF!|#DIV/0!|#VALUE!|#NAME\\?|#N/A|#NUM!|#NULL!',options:{useRegex:true,maxResults:30},maxChars:4000});
  if(err.ndjson.includes('"kind":"cell"')) throw new Error(`公式错误 ${err.ndjson}`);
  qa.push({group,formula_scan:err.ndjson,inspection:(await wb.inspect({kind:'table',range:'配置目录!A5:H12',include:'values,formulas',tableMaxRows:8,tableMaxCols:8,maxChars:4500})).ndjson});
  // 每个工作表均渲染表头和前几条记录；宽表额外渲染后段，便于逐表视觉检查。
  for (let i=0;i<model.tables.length+3;i++) {
    const s=wb.worksheets.getItemAt(i);
    const t=model.tables.find(x=>x.sheet_name===s.name);
    const ranges=t?[`A1:${col(Math.min(t.fields.length,8))}${Math.min(t.rows.length+3,9)}`]:[s.name==='配置目录'?'A1:H13':s.name==='发布缺口'?'A1:E8':'A1:C12'];
    if(t&&t.fields.length>8)ranges.push(`${col(9)}1:${col(t.fields.length)}${Math.min(t.rows.length+3,9)}`);
    for(let j=0;j<ranges.length;j++){
      const png=await wb.render({sheetName:s.name,range:ranges[j],scale:1.2,format:'png'});
      await fs.writeFile(path.join(previewDir,`${group}-${String(i).padStart(2,'0')}-${j}.png`),new Uint8Array(await png.arrayBuffer()));
    }
  }
  const workbookPath=path.join(out,`${groupLabels[group]}.xlsx`);
  await (await SpreadsheetFile.exportXlsx(wb)).save(workbookPath);
  schema.workbooks.push({group,path:path.relative(root,workbookPath)});
  console.log(JSON.stringify({group,tables:model.tables.length,records:model.tables.reduce((n,t)=>n+t.rows.length,0),workbook:workbookPath}));
}
if(new Set(schema.blockers.map(x=>x.id)).size!==schema.blockers.length)throw new Error('缺口ID重复');
await fs.mkdir(path.join(root,'config/schema'),{recursive:true});
await fs.writeFile(path.join(root,'config/schema/catalog.json'),JSON.stringify(schema,null,2)+'\n');
await fs.writeFile(path.join(root,'config/reports/工作簿检查.json'),JSON.stringify(qa,null,2)+'\n');
console.log(JSON.stringify({schema_tables:schema.tables.length,blockers:schema.blockers.length}));
