#!/usr/bin/env python3
"""逐字编译只读Pushed方法，生成独立.NET原语向量；不是运行配置或完整技能验收。"""
from pathlib import Path
import argparse
import gzip
import hashlib
import json
import sys
import subprocess

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / '.work/displacement-foundation/oracle'
LOCKS = {
    'human': ('资料参考/crystal/Server/MirObjects/HumanObject.cs', 'da406ed3f9e2cbc79d9759871410cfaf08b9b17a1e507631e68f2679c105958c'),
    'monster': ('资料参考/crystal/Server/MirObjects/MonsterObject.cs', '2cc4e673c589ad57452b4682c3bcb155b4b6b3027b1d3ee52a18a5ae1b7df98e'),
    'functions': ('资料参考/crystal/Shared/Functions/Functions.cs', '255e0ff5de9d9dec47e356f4049def61b5b607fd9664b74dd86944ecebee2278'),
    'enums': ('资料参考/crystal/Shared/Enums.cs', '63c8f63b436b27dc941e90e9eece792de2d4b89dcb932f2e23fb7428a993d398'),
}


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def method(source, signature):
    start = source.index(signature)
    body = source.index('{', start)
    depth = 1
    end = body + 1
    while depth:
        depth += (source[end] == '{') - (source[end] == '}')
        end += 1
    return source[start:end]


STUBS = r'''
using System;
using System.Drawing;
using System.Collections.Generic;
public enum ObjectType { Player, Monster, Spell }
public enum BuffType { Concentration }
public static class Envir { public static long Time = 10000; }
public static class Settings { public const long Second = 1000; }
public sealed class Buff { public void Set(string key, object value) {} public T Get<T>(string key)=>default!; }
public sealed class SafeZoneInfo {}
public sealed class MonsterInfo { public bool CanPush = true; }
public record Notice(string Channel, MirDirection Direction, Point Location);
public class Cell {
    public List<MapObject> Objects = new();
    public void Add(MapObject value)=>Objects.Add(value);
    public void Remove(MapObject value)=>Objects.Remove(value);
}
public sealed class Map {
    public int Width, Height;
    public HashSet<Point> Blocked = new();
    readonly Dictionary<Point, Cell> cells = new();
    public bool ValidPoint(Point p)=>p.X>=0&&p.Y>=0&&p.X<Width&&p.Y<Height&&!Blocked.Contains(p);
    public Cell GetCell(Point p) { if(!cells.TryGetValue(p,out var c)) cells[p]=c=new(); return c; }
    public SafeZoneInfo? GetSafeZone(Point p)=>null;
}
public class MapObject {
    public uint ObjectID=1;
    public bool Blocking=true;
    public ObjectType Race=ObjectType.Monster;
    public Point CurrentLocation;
    public Map CurrentMap=null!;
    public MirDirection Direction;
    public long ActionTime, MoveTime, AttackTime;
    public List<Notice> Notices = new();
    public virtual int Pushed(MapObject pusher, MirDirection dir, int distance)=>0;
    public void RemoveObjects(MirDirection dir,int distance) {}
    public void AddObjects(MirDirection dir,int distance) {}
    public void Broadcast(S.ObjectPushed p)=>Notices.Add(new("observer",p.Direction,p.Location));
    public void Enqueue(S.Pushed p)=>Notices.Add(new("owner",p.Direction,p.Location));
}
public class SpellObject:MapObject { public void ProcessSpell(MapObject value)=>throw new Exception("当前向量没有地面Spell"); }
namespace S {
    public sealed class ObjectPushed { public uint ObjectID; public MirDirection Direction; public Point Location; }
    public sealed class Pushed { public MirDirection Direction; public Point Location; }
}
'''

HUMAN_MEMBERS = r'''
    public bool InSafeZone;
    public bool HasBuff(BuffType kind,out Buff value) { value=null!; return false; }
    public void Moved() {}
    public void GetPlayerLocation() {}
    public void UpdateConcentration(bool first,bool second)=>throw new Exception("当前向量没有Concentration");
    public void SetBindSafeZone(SafeZoneInfo value)=>throw new Exception("当前向量没有SafeZone");
'''

DRIVER = r'''
using System.Drawing;
using System.Text.Json;
var input=JsonDocument.Parse(File.ReadAllText(args[0])).RootElement;
int width=input.GetProperty("width").GetInt32(),height=input.GetProperty("height").GetInt32();
var baseBlocked=input.GetProperty("blocked").EnumerateArray().Select(p=>new Point(p.GetProperty("x").GetInt32(),p.GetProperty("y").GetInt32())).ToArray();
var origins=new[]{new Point(8,8),new Point(0,0),new Point(16,16),new Point(0,16),new Point(16,0),new Point(1,6),new Point(6,1),new Point(14,9),new Point(7,14)};
var rows=new List<object>();
void Run(string kind,bool canPush,Point origin,int direction,int distance,int occupiedDistance,bool corner) {
    var map=new Map{Width=width,Height=height,Blocked=new(baseBlocked)};
    if(corner) {
        var next=Functions.PointMove(origin,(MirDirection)direction,1);
        map.Blocked.Add(new(next.X,origin.Y));map.Blocked.Add(new(origin.X,next.Y));
    }
    var occupied=new List<Point>();
    if(occupiedDistance>0) {
        var p=Functions.PointMove(origin,(MirDirection)direction,occupiedDistance);
        if(map.ValidPoint(p)) {occupied.Add(p);map.GetCell(p).Add(new MapObject{CurrentMap=map,CurrentLocation=p});}
    }
    MapObject target=kind=="human"?new Human():new Monster{Info=new MonsterInfo{CanPush=canPush}};
    target.CurrentMap=map;target.CurrentLocation=origin;target.Direction=MirDirection.Down;
    target.ActionTime=17000;target.MoveTime=19000;target.AttackTime=23000;map.GetCell(origin).Add(target);
    int steps=target.Pushed(new MapObject(),(MirDirection)direction,distance);
    var path=target.Notices.Where(n=>n.Channel=="observer").Select(n=>new{x=n.Location.X,y=n.Location.Y}).ToArray();
    var owner=target.Notices.Where(n=>n.Channel=="owner").Select(n=>new{x=n.Location.X,y=n.Location.Y}).ToArray();
    rows.Add(new{kind,can_push=canPush,width,height,blocked=map.Blocked.OrderBy(p=>p.X).ThenBy(p=>p.Y).Select(p=>new{x=p.X,y=p.Y}),
        origin=new{x=origin.X,y=origin.Y},direction,distance,occupied_distance=occupiedDistance,occupied=occupied.Select(p=>new{x=p.X,y=p.Y}),corner,
        now_ms=Envir.Time,initial_action_ms=17000,initial_move_ms=19000,initial_attack_ms=23000,
        expected_steps=steps,expected_position=new{x=target.CurrentLocation.X,y=target.CurrentLocation.Y},expected_direction=(int)target.Direction,
        expected_action_ms=target.ActionTime,source_move_ms=target.MoveTime,
        project_expected_move_ms=kind=="human"?(target.ActionTime==17000?19000:target.ActionTime):target.MoveTime,
        expected_attack_ms=target.AttackTime,expected_path=path,expected_owner_path=owner});
}
foreach(var origin in origins)foreach(int direction in Enumerable.Range(0,8))foreach(int distance in new[]{0,1,2,3,6,20})foreach(int occupied in new[]{0,1,2}) {
    Run("human",true,origin,direction,distance,occupied,false);
    Run("monster",true,origin,direction,distance,occupied,false);
    Run("monster",false,origin,direction,distance,occupied,false);
}
foreach(int direction in new[]{1,3,5,7})Run("monster",true,new Point(8,8),direction,1,0,true);
File.WriteAllText(args[1],JsonSerializer.Serialize(rows));
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--go-fixture', type=Path)
    args = parser.parse_args()
    OUT.mkdir(parents=True, exist_ok=True)
    texts = {}
    for name, (relative, expected) in LOCKS.items():
        path = ROOT / relative
        if hashlib.sha256(path.read_bytes()).hexdigest() != expected:
            raise SystemExit('来源摘要变化，先审定再更新：' + relative)
        texts[name] = path.read_text(encoding='utf-8-sig')
    signature = 'public override int Pushed(MapObject pusher, MirDirection dir, int distance)'
    human = method(texts['human'], signature)
    monster = method(texts['monster'], signature)
    point = method(texts['functions'], 'public static Point PointMove(Point p, MirDirection d, int i)')
    reverse = method(texts['functions'], 'public static MirDirection ReverseDirection(MirDirection dir)')
    direction = method(texts['enums'], 'public enum MirDirection : byte')
    code = STUBS + '\n' + direction + '\npublic static class Functions {\n' + point + '\n' + reverse + '\n}\n'
    code += 'public sealed class Human:MapObject {\n' + HUMAN_MEMBERS + human + '\n}\n'
    code += 'public sealed class Monster:MapObject {public MonsterInfo Info=new();\n' + monster + '\n}\n'
    assert all(fragment in code for fragment in (human, monster, point, reverse, direction))
    project = OUT / 'project'
    project.mkdir(exist_ok=True)
    (project / 'Source.cs').write_text(code, encoding='utf-8', newline='\n')
    (project / 'Driver.cs').write_text(DRIVER, encoding='utf-8', newline='\n')
    (project / 'Oracle.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net10.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup></Project>\n', encoding='utf-8', newline='\n')
    (project / 'NuGet.Config').write_text('<configuration><packageSources><clear /></packageSources></configuration>\n', encoding='utf-8', newline='\n')
    sdk = json.loads((ROOT / 'conf/toolchain/dotnet.lock.json').read_text())['version']
    save(project / 'global.json', {'sdk': {'version': sdk, 'rollForward': 'disable'}})
    geometry = json.loads((ROOT / 'client/assets/resources/development/maps/first-map.json').read_text())
    if (geometry['width'], geometry['height']) != (17, 17):
        raise SystemExit('当前来源向量场景为17×17，地图范围变化时须先更新对照输入')
    save(OUT / 'map-input.json', {key: geometry[key] for key in ('width', 'height', 'blocked')})
    commands = [
        ['--cwd', str(project.relative_to(ROOT)), 'dotnet', 'restore', '--configfile', 'NuGet.Config'],
        ['--cwd', str(project.relative_to(ROOT)), 'dotnet', 'build', '--no-restore', '--configuration', 'Release'],
        ['dotnet', str(project / 'bin/Release/net10.0/Oracle.dll'), str(OUT / 'map-input.json'), str(OUT / 'vectors.json')],
    ]
    with (OUT / 'execution.log').open('w') as log:
        for command in commands:
            if subprocess.run([sys.executable,str(ROOT / 'tools/dev')] + command, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT).returncode:
                raise SystemExit('独立来源原语构建/执行失败，见' + str(OUT / 'execution.log'))
    vectors = json.loads((OUT / 'vectors.json').read_text())
    encoded = gzip.compress(json.dumps(vectors, separators=(',', ':')).encode(), mtime=0)
    if args.go_fixture:
        args.go_fixture.parent.mkdir(parents=True, exist_ok=True)
        if not args.go_fixture.is_file() or args.go_fixture.read_bytes() != encoded:
            staged = args.go_fixture.with_suffix(args.go_fixture.suffix + '.tmp')
            staged.write_bytes(encoded)
            staged.replace(args.go_fixture)
    report = {'status': 'passed', 'sdk_version': sdk, 'vectors': len(vectors),
              'sources': {name: {'path': value[0], 'sha256': value[1]} for name, value in LOCKS.items()},
              'compiled_source_sha256': hashlib.sha256(code.encode()).hexdigest(),
              'go_fixture_sha256': hashlib.sha256(encoded).hexdigest(),
              'source_fragments': {'human_pushed': human, 'monster_pushed': monster, 'point_move': point, 'reverse_direction': reverse, 'direction_enum': direction},
              'scope': '逐字来源Pushed/PointMove/ReverseDirection；当前无Concentration、Spell、安全区的最小世界；Human.NextMove按登记动作锁映射单列',
              'not_proven': ['目标1.80归属', 'Repulsion/ShoulderDash/Teleport完整技能', '源完整对象调度、绑定及地面Spell', 'scene/life/授权/导航的项目门禁'],
              'runtime_configuration': False}
    save(OUT / 'report.json', report)
    print(json.dumps({key: report[key] for key in ('status', 'sdk_version', 'vectors', 'go_fixture_sha256')}, ensure_ascii=False))


if __name__ == '__main__':
    main()
