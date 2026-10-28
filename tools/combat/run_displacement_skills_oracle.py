"""逐字编译抗拒、野蛮及Pushed来源方法；最小世界无PK、安全区、地面Spell或额外伤害属性。"""
from pathlib import Path
import argparse
import gzip
import hashlib
import json
import subprocess
import sys

import run_displacement_oracle as pushed
import run_source_oracle as magic_source

ROOT=pushed.ROOT
OUT=ROOT/'.work/displacement-skills-20261028/oracle'

STUBS=r'''
using System;
using System.Drawing;
using System.Collections.Generic;
public enum ObjectType {Player,Monster,Hero,Spell}
public enum BuffType {Concentration}
public enum DefenceType {None,MAC}
public enum Spell {Repulsion,ShoulderDash,FireWall}
public enum ChatType {System}
public enum ServerTextKeys {NoPushingInSafezone,NotEnoughPushingPower}
public sealed class TextMap {public string GetLocalization(ServerTextKeys key)=>key.ToString();}
public static class GameLanguage {public static TextMap ServerTextMap=new();}
public static class Settings {public const long Second=1000;}
public static class Envir {public static long Time=10000;public static FixedRandom Random=new();}
public sealed class FixedRandom:Random {
 public int Chance,Distance;public List<int> Draws=new();
 public override int Next(int max) {Draws.Add(max);if(max==20)return Chance;if(max==2)return Distance;throw new Exception("未经声明的随机边界："+max);}
}
public sealed class Buff {public void Set(string key,object value){} public T Get<T>(string key)=>default!;}
public sealed class SafeZoneInfo {public Point Location;}
public sealed class MonsterInfo {public bool CanPush=true;}
public record Notice(uint Actor,int Kind,Point Location);
public sealed class Cell {
 public bool Valid;public List<MapObject> Objects=new();
 public void Add(MapObject value)=>Objects.Add(value);public void Remove(MapObject value)=>Objects.Remove(value);
}
public sealed class Map {
 public int Width=17,Height=17;public HashSet<Point> Blocked=new();public List<Notice> Notices=new();
 readonly Dictionary<Point,Cell> cells=new();
 public bool ValidPoint(Point p)=>p.X>=0&&p.Y>=0&&p.X<Width&&p.Y<Height&&!Blocked.Contains(p);
 public Cell GetCell(Point p)=>GetCell(p.X,p.Y);
 public Cell GetCell(int x,int y){var p=new Point(x,y);if(!cells.TryGetValue(p,out var cell))cells[p]=cell=new(){Valid=ValidPoint(p)};return cell;}
 public SafeZoneInfo? GetSafeZone(Point p)=>null;
}
public class MapObject {
 public uint ObjectID;public int Level=1,HP=100,DamageCalls,DamageTotal;public bool Hostile=true,InSafeZone;
 public bool Blocking=>HP>0;
 public ObjectType Race=ObjectType.Monster;public Point CurrentLocation;public Map CurrentMap=null!;public MirDirection Direction;
 public long ActionTime=17000,MoveTime=19000,AttackTime=23000;
 public bool IsAttackTarget(MapObject source)=>Hostile&&source!=this&&HP>0;
 public virtual int Pushed(MapObject pusher,MirDirection dir,int distance)=>0;
 public virtual int Attacked(MapObject source,int damage,DefenceType defence,bool proc){if(defence!=DefenceType.None)throw new Exception("非None防御不属于此最小世界");DamageCalls++;var applied=Math.Min(HP,damage);HP-=applied;DamageTotal+=applied;return applied;}
 public void RemoveObjects(MirDirection dir,int length){}public void AddObjects(MirDirection dir,int length){}
 public void Broadcast(S.ObjectPushed p)=>CurrentMap.Notices.Add(new(ObjectID,2,p.Location));
 public void Broadcast(S.ObjectDash p)=>CurrentMap.Notices.Add(new(ObjectID,3,p.Location));
 public void Broadcast(S.ObjectDashFail p)=>CurrentMap.Notices.Add(new(ObjectID,5,p.Location));
 public void Enqueue(object message){}
}
public sealed class SpellObject:MapObject {
 public MapObject Caster=null!;public Spell Spell;public int Value;
 public void ProcessSpell(MapObject target)=>throw new Exception("此世界没有地面Spell");
}
public sealed class PlayerObject:Human {public Point BindLocation;public int BindMapIndex;}
public class Human:MapObject {
 public bool InTrapRock;public bool CanWalk=>HP>0&&Envir.Time>=ActionTime&&!InTrapRock;
 public const long MoveDelay=600;public long CellTime;public int _stepCounter,CurrentMapIndex;
 public void ReceiveChat(string text,ChatType kind){}public void LevelMagic(UserMagic magic){}
 public bool HasBuff(BuffType type,out Buff value){value=null!;return false;}
 public void Moved(){}public void GetPlayerLocation(){}
 public void UpdateConcentration(bool a,bool b)=>throw new Exception("此世界没有Concentration");
 public void SetBindSafeZone(SafeZoneInfo value)=>throw new Exception("此世界没有SafeZone");
 public void Run(string kind,UserMagic magic){if(kind=="repulsion")Repulsion(magic);else ShoulderDash(magic);}
 HUMAN_METHODS
}
public sealed class Monster:MapObject {public MonsterInfo Info=new();MONSTER_METHOD}
public sealed class MagicInfo {
 public string Name="";public Spell Spell;public int Icon,Level1,Level2,Level3,Need1,Need2,Need3,BaseCost,LevelCost,Range,MPowerBase,MPowerBonus,PowerBase,PowerBonus;
 public long DelayBase=1800;public float MultiplierBase=1,MultiplierBonus;
}
public sealed class UserMagic {
 public byte Level;public MagicInfo Info=new();public Spell Spell=>Info.Spell;public long CastTime;
 MAGIC_METHODS
}
public static class Defaults {public static MagicInfo Repulsion()=>REPULSION_DEFAULT;public static MagicInfo Dash()=>DASH_DEFAULT;}
public static class Functions {FUNCTION_METHODS}
namespace S {
 public sealed class ObjectPushed {public uint ObjectID;public MirDirection Direction;public Point Location;}
 public sealed class Pushed {public MirDirection Direction;public Point Location;}
 public sealed class UserDash {public MirDirection Direction;public Point Location;}
 public sealed class ObjectDash {public uint ObjectID;public MirDirection Direction;public Point Location;}
 public sealed class UserDashFail {public MirDirection Direction;public Point Location;}
 public sealed class ObjectDashFail {public uint ObjectID;public MirDirection Direction;public Point Location;}
 public sealed class MagicCast {public Spell Spell;}
}
'''

DRIVER=r'''
using System.Drawing;
using System.Text.Json;
var input=JsonDocument.Parse(File.ReadAllText(args[0])).RootElement;
var rows=new List<object>();
Point Read(JsonElement point)=>new(point.GetProperty("x").GetInt32(),point.GetProperty("y").GetInt32());
foreach(var test in input.EnumerateArray()) {
 var map=new Map();foreach(var point in test.GetProperty("blocked").EnumerateArray())map.Blocked.Add(Read(point));
 var human=new Human{ObjectID=1,Race=ObjectType.Player,Hostile=false,CurrentMap=map,CurrentLocation=Read(test.GetProperty("origin")),
   Direction=(MirDirection)test.GetProperty("direction").GetInt32(),Level=test.GetProperty("caster_level").GetInt32(),ActionTime=0};
 map.GetCell(human.CurrentLocation).Add(human);
 var objects=new List<MapObject>{human};
 foreach(var value in test.GetProperty("actors").EnumerateArray()) {
   var ob=new Monster{ObjectID=value.GetProperty("id").GetUInt32(),CurrentMap=map,CurrentLocation=Read(value.GetProperty("position")),
      Level=value.GetProperty("level").GetInt32(),HP=value.GetProperty("hp").GetInt32(),Hostile=value.GetProperty("hostile").GetBoolean(),
      Info=new MonsterInfo{CanPush=value.GetProperty("can_push").GetBoolean()},Direction=MirDirection.Down};
   objects.Add(ob);map.GetCell(ob.CurrentLocation).Add(ob);
 }
 var kind=test.GetProperty("kind").GetString()!;
 var magic=new UserMagic{Level=test.GetProperty("rank").GetByte(),Info=kind=="repulsion"?Defaults.Repulsion():Defaults.Dash()};
 Envir.Random=new(){Chance=test.GetProperty("chance").GetInt32(),Distance=test.GetProperty("distance_roll").GetInt32()};
 human.Run(kind,magic);
 rows.Add(new{input=test.Clone(),expected=objects.Select(ob=>new{id=ob.ObjectID,position=new{x=ob.CurrentLocation.X,y=ob.CurrentLocation.Y},direction=(int)ob.Direction,
    hp=ob.HP,damage_calls=ob.DamageCalls,damage=ob.DamageTotal,action_ms=ob.ActionTime,move_ms=ob.MoveTime,attack_ms=ob.AttackTime,
    path=map.Notices.Where(n=>n.Actor==ob.ObjectID&&n.Kind!=5).Select(n=>new{x=n.Location.X,y=n.Location.Y}),
    dash_failed=map.Notices.Any(n=>n.Actor==ob.ObjectID&&n.Kind==5)}),draws=Envir.Random.Draws});
}
File.WriteAllText(args[1],JsonSerializer.Serialize(rows));
'''

def factory(source,symbol):
    at=source.index('new MagicInfo',source.index('if (!MagicExists(Spell.'+symbol+'))'))
    opening=source.index('{',at);depth=1;end=opening+1
    while depth:
        depth+=(source[end]=='{')-(source[end]=='}');end+=1
    return source[at:end]


def scenarios(blocked):
    rows=[]
    point=lambda x,y:dict(x=x,y=y)
    directions=[(0,-1),(1,-1),(1,0),(1,1),(0,1),(-1,1),(-1,0),(-1,-1)]
    def actor(id,pos,level=8,hostile=True,can_push=True):return dict(id=id,position=pos,level=level,hostile=hostile,can_push=can_push,hp=100)
    def add(kind,origin,d,rank,chance,roll,actors,extra=()):rows.append(dict(kind=kind,origin=origin,direction=d,rank=rank,caster_level=12,chance=chance,distance_roll=roll,actors=actors,blocked=blocked+list(extra)))
    for rank in range(4):
      for d,(dx,dy) in enumerate(directions):
       for roll in range(2):
        ahead=lambda n:point(8+dx*n,8+dy*n)
        for chance in range(20):
         for level in [11,12,13,1]:add('repulsion',point(8,8),d,rank,chance,roll,[actor(10,ahead(1),level)])
        for special in ['immune','center_stack','ring_stack','center_and_ring','friendly','blocked_after_one']:
         actors=[actor(10,ahead(1),can_push=special!='immune')];extra=[]
         if special=='center_stack':actors=[actor(10,point(8,8)),actor(11,point(8,8)),actor(12,point(8,8))]
         if special=='ring_stack':actors.append(actor(11,ahead(1)))
         if special=='center_and_ring':actors=[actor(10,point(8,8)),actor(11,point(8,7))]
         if special=='friendly':actors=[actor(2,ahead(1),hostile=False)]
         if special=='blocked_after_one':extra=[ahead(3)]
         add('repulsion',point(8,8),d,rank,0,roll,actors,extra)
        for label in ['empty','target','equal','immune','later_enemy','wall_first','wall_later','friendly_first','target_stack','friend_then_target']:
         actors=[];extra=[]
         if label in ['target','equal','immune','target_stack']:actors=[actor(10,ahead(1),level=12 if label=='equal' else 8,can_push=label!='immune')]
         if label=='target_stack':actors.append(actor(2,ahead(1),hostile=False))
         if label=='friend_then_target':actors=[actor(2,ahead(1),hostile=False),actor(10,ahead(1))]
         if label=='later_enemy':actors=[actor(10,ahead(2))]
         if label=='friendly_first':actors=[actor(2,ahead(1),hostile=False)]
         if label=='wall_first':extra=[ahead(1)]
         if label=='wall_later':extra=[ahead(3)]
         add('shoulder_dash',point(8,8),d,rank,0,roll,actors,extra)
        for origin in [point(0,0),point(16,16),point(0,16),point(16,0)]:
         add('shoulder_dash',origin,d,rank,0,roll,[])
    return rows


def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--go-fixture',type=Path);args=parser.parse_args()
    locks={ROOT/path:expected for path,expected in pushed.LOCKS.values()}|magic_source.LOCKS
    for path,expected in locks.items():
        if hashlib.sha256(path.read_bytes()).hexdigest()!=expected:raise RuntimeError('来源摘要不符：'+str(path))
    text={key:(ROOT/value[0]).read_text(encoding='utf-8-sig') for key,value in pushed.LOCKS.items()}
    method=pushed.method
    human='\n'.join(method(text['human'],signature) for signature in ['public override int Pushed(MapObject pusher, MirDirection dir, int distance)','private void Repulsion(UserMagic magic)','private void ShoulderDash(UserMagic magic)'])
    monster=method(text['monster'],'public override int Pushed(MapObject pusher, MirDirection dir, int distance)')
    functions='\n'.join(method(text['functions'],signature) for signature in ['public static Point PointMove(Point p, MirDirection d, int i)','public static MirDirection ReverseDirection(MirDirection dir)','public static MirDirection DirectionFromPoint(Point source, Point dest)'])
    magic=magic_source.MAGIC.read_text(encoding='utf-8-sig')
    magic_methods='\n'.join(method(magic,signature) for signature in ['public int GetDamage(int DamageBase)','public float GetMultiplier()','public int GetPower()','public int MPower()','public int DefPower()'])
    defaults=magic_source.ENVIR.read_text(encoding='utf-8-sig')
    code=STUBS.replace('HUMAN_METHODS',human).replace('MONSTER_METHOD',monster).replace('FUNCTION_METHODS',functions).replace('MAGIC_METHODS',magic_methods).replace('REPULSION_DEFAULT',factory(defaults,'Repulsion')).replace('DASH_DEFAULT',factory(defaults,'ShoulderDash'))
    code+='\n'+method(text['enums'],'public enum MirDirection : byte')
    project=OUT/'project';project.mkdir(parents=True,exist_ok=True)
    (project/'Source.cs').write_text(code,encoding='utf-8',newline='\n');(project/'Driver.cs').write_text(DRIVER,encoding='utf-8',newline='\n')
    (project/'Oracle.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net10.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup></Project>\n',encoding='utf-8')
    (project/'NuGet.Config').write_text('<configuration><packageSources><clear /></packageSources></configuration>\n',encoding='utf-8')
    sdk=json.loads((ROOT/'conf/toolchain/dotnet.lock.json').read_text(encoding='utf-8'))['version']
    pushed.save(project/'global.json',dict(sdk=dict(version=sdk,rollForward='disable')))
    geometry=json.loads((ROOT/'client/assets/resources/development/maps/first-map.json').read_text(encoding='utf-8'))
    pushed.save(OUT/'input.json',scenarios(geometry['blocked']))
    commands=[['--cwd',project.relative_to(ROOT).as_posix(),'dotnet','restore','--configfile','NuGet.Config'],['--cwd',project.relative_to(ROOT).as_posix(),'dotnet','build','--no-restore','-c','Release'],['dotnet',str(project/'bin/Release/net10.0/Oracle.dll'),str(OUT/'input.json'),str(OUT/'vectors.json')]]
    with (OUT/'execution.log').open('wb') as log:
        for command in commands:
            if subprocess.run([sys.executable,str(ROOT/'tools/dev')]+command,cwd=ROOT,stdout=log,stderr=log).returncode:raise RuntimeError('来源编译或执行失败，见'+str(OUT/'execution.log'))
    vectors=json.loads((OUT/'vectors.json').read_text(encoding='utf-8'));encoded=gzip.compress(json.dumps(vectors,separators=(',',':')).encode(),mtime=0)
    if args.go_fixture:
        target=(ROOT/args.go_fixture).resolve()
        if not target.is_relative_to(ROOT/'server/internal/game/testdata'):raise ValueError('向量只能保存到当前game/testdata')
        target.write_bytes(encoded)
    report=dict(passed=True,vectors=len(vectors),compiled_source_sha256=hashlib.sha256(code.encode()).hexdigest(),go_fixture_sha256=hashlib.sha256(encoded).hexdigest(),
                sources={p.relative_to(ROOT).as_posix():digest for p,digest in locks.items()},
                scope='逐字Repulsion/ShoulderDash/Pushed与MagicInfo/默认值；无PK、安全区、Spell、额外伤害属性；比较每实体真实路径和时钟，不认证通知跨实体墙钟顺序',
                official_180_verified=False,runtime_configuration=False)
    pushed.save(OUT/'report.json',report);print(json.dumps(report,ensure_ascii=False))


if __name__=='__main__':main()
