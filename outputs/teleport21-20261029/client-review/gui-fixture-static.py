import ast
from pathlib import Path
root=Path(__file__).resolve().parents[3]
p=root/'tools/client/prepare_teleport_qa.py'
tree=ast.parse(p.read_text(encoding='utf-8'))
writes=[n for n in ast.walk(tree) if isinstance(n,ast.Call) and isinstance(n.func,ast.Attribute) and n.func.attr in {'update_one','update_many','replace_one','insert_one','insert_many','delete_one','delete_many'}]
assert len(writes)==1 and writes[0].func.attr=='update_one'
setter=writes[0].args[1]
assert [k.value for k in setter.keys]==['$set']
assert {k.value for k in setter.values[0].keys}=={'level','wallet'}
assert p.read_bytes().count(bytes([13]))==0
print('fixture语法及唯一level/wallet写白名单通过；未连接数据库')
