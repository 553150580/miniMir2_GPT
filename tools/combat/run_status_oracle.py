#!/usr/bin/env python3
"""编译逐字来源公式，以真实.NET输出防御技能独立回归向量。非运行配置。"""
import argparse,gzip,hashlib,json,re,subprocess,sys
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
OUT=ROOT/'.work/combat-status/oracle'
def save(p,v):p.write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n')
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--go-fixture',type=Path);args=p.parse_args()
 contract=json.loads((ROOT/'资料索引/防御技能与状态来源契约.json').read_text())
 src={}
 for name in ['human','map','magic_info']:
  lock=contract['sources'][name];path=ROOT/lock['path']
  if hashlib.sha256(path.read_bytes()).hexdigest()!=lock['sha256']:raise SystemExit('来源已变化：'+name)
  src[name]=path.read_text(encoding='utf-8-sig')
 lines=src['magic_info'].splitlines(keepends=True)
 power=''.join(lines[213:217])
 assert 'public int GetPower(int power)' in power and 'Math.Round' in power
 shield=re.search(r'if \(Stats\[Stat.DamageReductionPercent\] > 0\).*?if \(armour >= damage\).*?return 0;\s*}',src['human'],re.S).group()
 wear=re.search(r'var duration = \(int\)Math.Min\(int.MaxValue, magicShield.ExpireTime - \(\(damage - armour\) \* 60\)\);',src['human']).group()
 tao=re.search(r'GetAttackPower\(Stats\[Stat.MinSC\], Stats\[Stat.MaxSC\]\) \* 4 \+ \(magic.Level \+ 1\) \* 50',src['human']).group()
 bonus=re.search(r'target.Level / 7 \+ 4',src['map']).group()
 code='''using System;
public sealed class Magic {public byte Level; public int DefPower()=>0;
'''+power+'''
}
public enum Stat {DamageReductionPercent,MinSC,MaxSC}
public enum DamageType {Miss}
public sealed class Buff {public long ExpireTime;}
public sealed class Target {public int Level;}
public sealed class Source {
 public int[] Stats=new int[3]; public int FixedSC; public Magic magic=new();public Target target=new();
 public void BroadcastDamageIndicator(DamageType t){}
 public int GetAttackPower(int min,int max)=>FixedSC;
 public int Net(int damage,int armour,int percent){Stats[(int)Stat.DamageReductionPercent]=percent;
'''+shield.replace('Stats[Stat.DamageReductionPercent]','Stats[(int)Stat.DamageReductionPercent]')+'''
 return damage-armour;}
 public int Wear(int damage,int armour,long remaining){var magicShield=new Buff{ExpireTime=remaining};
'''+wear+'''
return duration;}
 public int Tao()=>'''+tao.replace('Stats[Stat.MinSC]','Stats[(int)Stat.MinSC]').replace('Stats[Stat.MaxSC]','Stats[(int)Stat.MaxSC]')+''';
 public int Bonus()=>'''+bonus+''';
}
'''
 # Stats的源索引保留原文：提供有类型索引器代替枚举到数组的改写。
 code=code.replace('public int[] Stats=new int[3];','public StatMap Stats=new();').replace('Stats[(int)Stat.', 'Stats[Stat.')
 code=code.replace('public sealed class Source {','public sealed class StatMap {readonly int[] values=new int[3];public int this[Stat key] {get=>values[(int)key];set=>values[(int)key]=value;}}\npublic sealed class Source {')
 assert all(fragment in code for fragment in [power,shield,wear,tao,bonus]),'编译片段必须保留来源原文'
 project=OUT/'project';project.mkdir(parents=True,exist_ok=True)
 (project/'SourceFormula.cs').write_text(code)
 (project/'Driver.cs').write_text('''using System.Text.Json;
var rows=new List<object>();var source=new Source();
int[] inputs=[0,1,2,3,4,5,6,7,8,9,10,15,20,45,100,255,1024,32767,65535,16777215,16777216];
foreach(byte rank in new byte[]{0,1,2,3})foreach(int stat in inputs){source.magic.Level=rank;source.FixedSC=stat;
rows.Add(new{kind="duration",rank=(int)rank,stat,power=stat+15,expected_shield_seconds=source.magic.GetPower(stat+15),expected_tao_seconds=source.Tao()});}
foreach(int level in new int[]{0,1,6,7,8,13,14,15,21,40,255,1024}){source.target.Level=level;rows.Add(new{kind="target_bonus",level,expected=source.Bonus()});}
foreach(int raw in new int[]{0,1,2,3,4,5,9,10,11,20,100,255,1024,32767})foreach(int armour in new int[]{0,1,2,3,5,10,100})foreach(int percent in new int[]{0,20,30,40,50}){
 int net=source.Net(raw,armour,percent);long remaining=1000;int reduced=raw-raw*percent/100;
 rows.Add(new{kind="damage",raw,armour,percent,remaining,expected_net=net,expected_remaining=net>0?source.Wear(reduced,armour,remaining):remaining});}
File.WriteAllText(args[0],JsonSerializer.Serialize(rows));
''')
 lock=json.loads((ROOT/'conf/toolchain/dotnet.lock.json').read_text())
 (project/'Oracle.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net10.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup></Project>\n')
 (project/'NuGet.Config').write_text('<configuration><packageSources><clear /></packageSources></configuration>\n')
 save(project/'global.json',{'sdk':{'version':lock['version'],'rollForward':'disable'}})
 commands=[['--cwd',str(project.relative_to(ROOT)),'dotnet','restore','--configfile','NuGet.Config'],['--cwd',str(project.relative_to(ROOT)),'dotnet','build','--no-restore','--configuration','Release'],['dotnet',str(project/'bin/Release/net10.0/Oracle.dll'),str(OUT/'vectors.json')]]
 with (OUT/'execution.log').open('w') as log:
  for command in commands:
   if subprocess.run([sys.executable,str(ROOT/'tools/dev')]+command,cwd=ROOT,stdout=log,stderr=subprocess.STDOUT).returncode:raise SystemExit('来源公式构建失败，见'+str(OUT/'execution.log'))
 vectors=json.loads((OUT/'vectors.json').read_text());encoded=gzip.compress(json.dumps(vectors,separators=(',',':')).encode(),mtime=0)
 if args.go_fixture:args.go_fixture.parent.mkdir(parents=True,exist_ok=True);args.go_fixture.write_bytes(encoded)
 report={'status':'passed','sdk_version':lock['version'],'vectors':len(vectors),'source_files':{name:contract['sources'][name] for name in src},'compiled_source_sha256':hashlib.sha256(code.encode()).hexdigest(),'go_fixture_sha256':hashlib.sha256(encoded).hexdigest(),'source_fragments':{'magic_getpower_int':power,'reduction_and_armour':shield,'remaining_loss':wear,'tao_duration_expression':tao,'target_level_expression':bonus},'scope':'逐字公式/表达式由.NET独立执行；最小索引器和确定性SC桩；不覆盖完整PRNG、完整伤害分支、对象调度或目标1.80认证','runtime_configuration':False}
 save(OUT/'report.json',report);print(json.dumps({k:report[k] for k in ['status','sdk_version','vectors','go_fixture_sha256']}))
if __name__=='__main__':main()
