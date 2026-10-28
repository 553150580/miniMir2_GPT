
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
 public override int Pushed(MapObject pusher, MirDirection dir, int distance)
        {
            int result = 0;
            MirDirection reverse = Functions.ReverseDirection(dir);
            Cell cell;
            for (int i = 0; i < distance; i++)
            {
                Point location = Functions.PointMove(CurrentLocation, dir, 1);

                if (!CurrentMap.ValidPoint(location)) return result;

                cell = CurrentMap.GetCell(location);

                bool stop = false;
                if (cell.Objects != null)
                    for (int c = 0; c < cell.Objects.Count; c++)
                    {
                        MapObject ob = cell.Objects[c];
                        if (!ob.Blocking) continue;
                        stop = true;
                    }
                if (stop) break;

                CurrentMap.GetCell(CurrentLocation).Remove(this);

                Direction = reverse;
                RemoveObjects(dir, 1);
                CurrentLocation = location;
                CurrentMap.GetCell(CurrentLocation).Add(this);
                AddObjects(dir, 1);

                Moved();

                Enqueue(new S.Pushed { Direction = Direction, Location = CurrentLocation });
                Broadcast(new S.ObjectPushed { ObjectID = ObjectID, Direction = Direction, Location = CurrentLocation });
                GetPlayerLocation();
                result++;
            }

            if (result > 0)
            {
                if (HasBuff(BuffType.Concentration, out Buff concentration))
                {
                    concentration.Set("InterruptTime", Envir.Time + (Settings.Second * 3));

                    if (!concentration.Get<bool>("Interrupted"))
                    {
                        concentration.Set("Interrupted", true);
                        UpdateConcentration(true, true);
                    }
                }

                cell = CurrentMap.GetCell(CurrentLocation);

                for (int i = 0; i < cell.Objects.Count; i++)
                {
                    if (cell.Objects[i].Race != ObjectType.Spell) continue;
                    SpellObject ob = (SpellObject)cell.Objects[i];

                    ob.ProcessSpell(this);
                    //break;
                }

                SafeZoneInfo szi = CurrentMap.GetSafeZone(CurrentLocation);

                if (szi != null)
                {
                    SetBindSafeZone(szi);
                    InSafeZone = true;
                }
                else
                    InSafeZone = false;
            }

            ActionTime = Envir.Time + 500;
            return result;
        }
private void Repulsion(UserMagic magic)
        {
            bool result = false;
            for (int d = 0; d <= 1; d++)
            {
                for (int y = CurrentLocation.Y - d; y <= CurrentLocation.Y + d; y++)
                {
                    if (y < 0) continue;
                    if (y >= CurrentMap.Height) break;

                    for (int x = CurrentLocation.X - d; x <= CurrentLocation.X + d; x += Math.Abs(y - CurrentLocation.Y) == d ? 1 : d * 2)
                    {
                        if (x < 0) continue;
                        if (x >= CurrentMap.Width) break;

                        Cell cell = CurrentMap.GetCell(x, y);
                        if (!cell.Valid || cell.Objects == null) continue;

                        for (int i = 0; cell.Objects != null && i < cell.Objects.Count; i++)
                        {
                            MapObject ob = cell.Objects[i];
                            if (ob.Race != ObjectType.Monster && ob.Race != ObjectType.Player && ob.Race != ObjectType.Hero) continue;

                            if (!ob.IsAttackTarget(this) || ob.Level >= Level) continue;

                            if (Envir.Random.Next(20) >= 6 + magic.Level * 3 + Level - ob.Level) continue;

                            int distance = 1 + Math.Max(0, magic.Level - 1) + Envir.Random.Next(2);
                            MirDirection dir = Functions.DirectionFromPoint(CurrentLocation, ob.CurrentLocation);

                            if (ob.Pushed(this, dir, distance) == 0) continue;

                            if (ob.Race == ObjectType.Player)
                            {
                                SafeZoneInfo szi = CurrentMap.GetSafeZone(ob.CurrentLocation);

                                if (szi != null)
                                {
                                    ((PlayerObject)ob).BindLocation = szi.Location;
                                    ((PlayerObject)ob).BindMapIndex = CurrentMapIndex;
                                    ob.InSafeZone = true;

                                }
                                else
                                    ob.InSafeZone = false;

                                ob.Attacked(this, magic.GetDamage(0), DefenceType.None, false);
                            }
                            result = true;
                        }
                    }
                }
            }

            if (result) LevelMagic(magic);
        }
private void ShoulderDash(UserMagic magic)
        {
            if (InTrapRock || !CanWalk)
            {
                return;
            }

            Point _nextLocation;
            MapObject _target = null;

            bool _blocking = false;
            bool _canDash = false;

            int _cellsTravelled = 0;
            int dist = Envir.Random.Next(2) + magic.Level + 2;

            ActionTime = Envir.Time + MoveDelay;

            for (int i = 0; i < dist; i++)
            {
                if (_blocking)
                {
                    break;
                }

                _nextLocation = Functions.PointMove(CurrentLocation, Direction, 1);

                if (!CurrentMap.ValidPoint(_nextLocation) || CurrentMap.GetSafeZone(_nextLocation) != null)
                {
                    break;
                }


                // acquire target
                if (i == 0)
                {
                    Cell targetCell = CurrentMap.GetCell(_nextLocation);

                    if (targetCell.Objects != null)
                    {
                        int cellCnt = targetCell.Objects.Count;

                        for (int j = 0; j < cellCnt; j++)
                        {
                            MapObject ob = targetCell.Objects[j];

                            if ((ob.Race == ObjectType.Player ||
                                ob.Race == ObjectType.Monster ||
                                ob.Race == ObjectType.Hero) &&
                                ob.IsAttackTarget(this) &&
                                ob.Level < Level)
                            {
                                _target = ob;
                                break;
                            }

                            if(ob.Blocking)
                            {
                                _blocking = true;
                                break;
                            }
                        }
                    }
                    
                    if (_blocking)
                    {
                        break;
                    }
                }

                // try to dash
                Cell dashCell = CurrentMap.GetCell(_nextLocation);
                _canDash = false;

                if (_target == null)
                {
                    if (dashCell.Objects != null)
                    {
                        int cellCnt = dashCell.Objects.Count;

                        for (int k = 0; k < cellCnt; k++)
                        {
                            MapObject ob = dashCell.Objects[k];

                            if (ob.Blocking)
                            {
                                _blocking = true;
                                break;
                            }
                        }

                        if(!_blocking)
                        {
                            _canDash = true;
                        }
                    }
                    else
                    {
                        _canDash = true;
                    }
                }
                else
                {
                    // try to push
                    if (_target.Pushed(this, Direction, 1) == 0)
                    {
                        _blocking = true;
                    }
                    else
                    {
                        _canDash = true;
                    }
                }

                if (_canDash)
                {
                    CurrentMap.GetCell(CurrentLocation).Remove(this);
                    RemoveObjects(Direction, 1);

                    Enqueue(new S.UserDash { Direction = Direction, Location = _nextLocation });
                    Broadcast(new S.ObjectDash { ObjectID = ObjectID, Direction = Direction, Location = _nextLocation });

                    CurrentMap.GetCell(_nextLocation).Add(this);
                    AddObjects(Direction, 1);

                    // dash interrupt
                    Cell cell = CurrentMap.GetCell(_nextLocation);
                    for (int l = 0; l < cell.Objects.Count; l++)
                    {
                        if (cell.Objects[l].Race == ObjectType.Spell)
                        {
                            SpellObject ob = (SpellObject)cell.Objects[l];

                            if (IsAttackTarget(ob.Caster))
                            {
                                switch (ob.Spell)
                                {
                                    case Spell.FireWall:
                                        if (Attacked((PlayerObject)ob.Caster, ob.Value, DefenceType.MAC, false) > 0)
                                        {
                                        _blocking = true;
                                        }
                                        break;
                                }
                            }
                        }
                    }

                    CurrentLocation = _nextLocation;
                    _cellsTravelled++;
                }
            }

            if (_cellsTravelled == 0)
            {
                Enqueue(new S.UserDashFail { Direction = Direction, Location = CurrentLocation });
                Broadcast(new S.ObjectDashFail { ObjectID = ObjectID, Direction = Direction, Location = CurrentLocation });

                if (InSafeZone)
                {
                    ReceiveChat(GameLanguage.ServerTextMap.GetLocalization(ServerTextKeys.NoPushingInSafezone), ChatType.System);
                }
                else
                {
                    ReceiveChat(GameLanguage.ServerTextMap.GetLocalization(ServerTextKeys.NotEnoughPushingPower), ChatType.System);
                }
            }
            else
            {
                _target?.Attacked(this, magic.GetDamage(0), DefenceType.None, false);
                LevelMagic(magic);

                // Broadcast(new S.ObjectDash { ObjectID = ObjectID, Direction = Direction, Location = Front });
            }

            long now = Envir.Time;

            magic.CastTime = now;
            Enqueue(new S.MagicCast { Spell = magic.Spell });

            CellTime = now + 500;
            _stepCounter = 0;
        }
}
public sealed class Monster:MapObject {public MonsterInfo Info=new();public override int Pushed(MapObject pusher, MirDirection dir, int distance)
        {
            if (!Info.CanPush) return 0;
            //if (!CanMove) return 0; //stops mobs that can't move (like cannibalplants) from being pushed

            int result = 0;
            MirDirection reverse = Functions.ReverseDirection(dir);
            for (int i = 0; i < distance; i++)
            {
                Point location = Functions.PointMove(CurrentLocation, dir, 1);

                if (!CurrentMap.ValidPoint(location)) return result;

                Cell cell = CurrentMap.GetCell(location);

                bool stop = false;
                if (cell.Objects != null)
                    for (int c = 0; c < cell.Objects.Count; c++)
                    {
                        MapObject ob = cell.Objects[c];
                        if (!ob.Blocking) continue;
                        stop = true;
                    }
                if (stop) break;

                CurrentMap.GetCell(CurrentLocation).Remove(this);

                Direction = reverse;
                RemoveObjects(dir, 1);
                CurrentLocation = location;
                CurrentMap.GetCell(CurrentLocation).Add(this);
                AddObjects(dir, 1);

                Broadcast(new S.ObjectPushed { ObjectID = ObjectID, Direction = Direction, Location = CurrentLocation });

                result++;
            }

            ActionTime = Envir.Time + 300 * result;
            MoveTime = Envir.Time + 500 * result;

            if (result > 0)
            {
                Cell cell = CurrentMap.GetCell(CurrentLocation);

                for (int i = 0; i < cell.Objects.Count; i++)
                {
                    if (cell.Objects[i].Race != ObjectType.Spell) continue;
                    SpellObject ob = (SpellObject)cell.Objects[i];

                    ob.ProcessSpell(this);
                    //break;
                }
            }

            return result;
        }}
public sealed class MagicInfo {
 public string Name="";public Spell Spell;public int Icon,Level1,Level2,Level3,Need1,Need2,Need3,BaseCost,LevelCost,Range,MPowerBase,MPowerBonus,PowerBase,PowerBonus;
 public long DelayBase=1800;public float MultiplierBase=1,MultiplierBonus;
}
public sealed class UserMagic {
 public byte Level;public MagicInfo Info=new();public Spell Spell=>Info.Spell;public long CastTime;
 public int GetDamage(int DamageBase)
        {
            return (int)((DamageBase + GetPower()) * GetMultiplier());
        }
public float GetMultiplier()
        {
            return (Info.MultiplierBase + (Level * Info.MultiplierBonus));
        }
public int GetPower()
        {
            return (int)Math.Round((MPower() / 4F) * (Level + 1) + DefPower());
        }
public int MPower()
        {
            if (Info.MPowerBonus > 0)
            {
                return Envir.Random.Next(Info.MPowerBase, Info.MPowerBonus + Info.MPowerBase);
            }
            else
                return Info.MPowerBase;
        }
public int DefPower()
        {
            if (Info.PowerBonus > 0)
            {
                return Envir.Random.Next(Info.PowerBase, Info.PowerBonus + Info.PowerBase);
            }
            else
                return Info.PowerBase;
        }
}
public static class Defaults {public static MagicInfo Repulsion()=>new MagicInfo
                {
                    Name = "Repulsion",
                    Spell = Spell.Repulsion,
                    Icon = 7,
                    Level1 = 12,
                    Level2 = 15,
                    Level3 = 19,
                    Need1 = 500,
                    Need2 = 1300,
                    Need3 = 2200,
                    BaseCost = 2,
                    LevelCost = 2,
                    Range = 0,
                    MPowerBase = 4
                };public static MagicInfo Dash()=>new MagicInfo
                {
                    Name = "ShoulderDash",
                    Spell = Spell.ShoulderDash,
                    Icon = 26,
                    Level1 = 30,
                    Level2 = 32,
                    Level3 = 34,
                    Need1 = 3000,
                    Need2 = 4000,
                    Need3 = 6000,
                    BaseCost = 4,
                    LevelCost = 4,
                    DelayBase = 2500,
                    Range = 0,
                    MPowerBase = 4
                };}
public static class Functions {public static Point PointMove(Point p, MirDirection d, int i)
    {
        switch (d)
        {
            case MirDirection.Up:
                p.Offset(0, -i);
                break;
            case MirDirection.UpRight:
                p.Offset(i, -i);
                break;
            case MirDirection.Right:
                p.Offset(i, 0);
                break;
            case MirDirection.DownRight:
                p.Offset(i, i);
                break;
            case MirDirection.Down:
                p.Offset(0, i);
                break;
            case MirDirection.DownLeft:
                p.Offset(-i, i);
                break;
            case MirDirection.Left:
                p.Offset(-i, 0);
                break;
            case MirDirection.UpLeft:
                p.Offset(-i, -i);
                break;
        }
        return p;
    }
public static MirDirection ReverseDirection(MirDirection dir)
    {
        switch (dir)
        {
            case MirDirection.Up:
                return MirDirection.Down;
            case MirDirection.UpRight:
                return MirDirection.DownLeft;
            case MirDirection.Right:
                return MirDirection.Left;
            case MirDirection.DownRight:
                return MirDirection.UpLeft;
            case MirDirection.Down:
                return MirDirection.Up;
            case MirDirection.DownLeft:
                return MirDirection.UpRight;
            case MirDirection.Left:
                return MirDirection.Right;
            case MirDirection.UpLeft:
                return MirDirection.DownRight;
            default:
                return dir;
        }
    }
public static MirDirection DirectionFromPoint(Point source, Point dest)
    {
        if (source.X < dest.X)
        {
            if (source.Y < dest.Y)
                return MirDirection.DownRight;
            if (source.Y > dest.Y)
                return MirDirection.UpRight;
            return MirDirection.Right;
        }

        if (source.X > dest.X)
        {
            if (source.Y < dest.Y)
                return MirDirection.DownLeft;
            if (source.Y > dest.Y)
                return MirDirection.UpLeft;
            return MirDirection.Left;
        }

        return source.Y < dest.Y ? MirDirection.Down : MirDirection.Up;
    }}
namespace S {
 public sealed class ObjectPushed {public uint ObjectID;public MirDirection Direction;public Point Location;}
 public sealed class Pushed {public MirDirection Direction;public Point Location;}
 public sealed class UserDash {public MirDirection Direction;public Point Location;}
 public sealed class ObjectDash {public uint ObjectID;public MirDirection Direction;public Point Location;}
 public sealed class UserDashFail {public MirDirection Direction;public Point Location;}
 public sealed class ObjectDashFail {public uint ObjectID;public MirDirection Direction;public Point Location;}
 public sealed class MagicCast {public Spell Spell;}
}

public enum MirDirection : byte
{
    Up = 0,
    UpRight = 1,
    Right = 2,
    DownRight = 3,
    Down = 4,
    DownLeft = 5,
    Left = 6,
    UpLeft = 7
}