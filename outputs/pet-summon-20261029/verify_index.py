"""核对Git实际待提交blob，覆盖本阶段源码、检查输入和原始证据字节。"""
from pathlib import Path
import hashlib,json,subprocess
ROOT=Path(__file__).resolve().parents[2]
read=lambda path:json.loads((ROOT/path).read_text(encoding='utf-8'))
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
stage=read('docs/召唤与跨图宠物规则阶段验收.json');build=read('docs/客户端构建检查.json');go=read('server/test-results/latest.json');client=read('.work/pet-summon-20261029/client-review/report.json')
expected={**build['asset_hashes'],**build['script_hashes'],**client['current'],**go['source_sha256'],**stage['evidence_sha256']}
for file,record in read('outputs/source-town-foundation-20261029/resources/artifact-inventory.json').items():expected[file]=record['sha256']
for base in ['资料参考','UI参考图']:
 for path in (ROOT/base).rglob('*'):
  if path.is_file():expected[path.relative_to(ROOT).as_posix()]=sha(path)
changed=subprocess.check_output(['git','diff','--cached','--name-only','-z'],cwd=ROOT).split(b'\0')
for raw in changed:
 if raw:
  path=raw.decode('utf-8');expected[path]=sha(ROOT/path)
indexed={}
for row in subprocess.check_output(['git','ls-files','--stage','-z'],cwd=ROOT).split(b'\0'):
 if not row:continue
 entry,path=row.split(b'\t',1);mode,oid,number=entry.split();assert number==b'0';indexed[path.decode('utf-8')]=oid
assert set(expected)<=set(indexed),sorted(set(expected)-set(indexed))
oids=sorted({indexed[path] for path in expected})
payload=subprocess.check_output(['git','cat-file','--batch'],cwd=ROOT,input=b''.join(oid+b'\n' for oid in oids))
pointer=0;hashes={}
for expected_oid in oids:
 end=payload.index(b'\n',pointer);oid,kind,size=payload[pointer:end].split();size=int(size);assert oid==expected_oid and kind==b'blob'
 pointer=end+1;hashes[oid]=hashlib.sha256(memoryview(payload)[pointer:pointer+size]).hexdigest();pointer+=size+1
assert pointer==len(payload)
mismatches=[{'path':path,'lf_normalization_matches_index':hashes[indexed[path]]==hashlib.sha256((ROOT/path).read_bytes().replace(b'\r\n',b'\n')).hexdigest()} for path,digest in expected.items() if hashes[indexed[path]]!=digest]
if mismatches:print(json.dumps({'byte_mismatches':mismatches},ensure_ascii=False))
assert not mismatches
for path,digest in build['asset_hashes'].items():assert hashlib.sha256(subprocess.check_output(['git','show','HEAD:'+path],cwd=ROOT)).hexdigest()==digest,path
assert client['stable'] and client['before']==client['after']==client['current']
assert sha(ROOT/'config/development/first-loop.json')==go['fixture']['sha256']==stage['runtime']['fixture_sha256']
print(json.dumps({'passed':True,'git_index_byte_hashes_verified':len(expected),'changed_paths':sum(bool(path) for path in changed),'go_inputs':len(go['source_sha256']),'client_inputs':len(client['current']),'saved_layouts_equal_to_previous_commit':len(build['asset_hashes']),'evidence_files':len(stage['evidence_sha256'])},ensure_ascii=False))
