
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
