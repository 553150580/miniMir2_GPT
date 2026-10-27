using System;
public enum Spell {FireBall,Healing,Fencing,SpiritSword,GreatFireBall,Slaying,HellFire,Lightning,ThunderBolt,Thrusting,FireBang,ThunderStorm,HalfMoon,FlamingSword,MassHealing,IceStorm}
public sealed class MagicInfo {
        public string Name;
        public Spell Spell;
        public byte BaseCost, LevelCost, Icon;
        public byte Level1, Level2, Level3;
        public ushort Need1, Need2, Need3;
        public uint DelayBase = 1800, DelayReduction;
        public ushort PowerBase, PowerBonus;
        public ushort MPowerBase, MPowerBonus;
        public float MultiplierBase = 1.0f, MultiplierBonus;
        public byte Range = 9;

}
public sealed class UserMagic { public MagicInfo Info = null!; public byte Level;
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

        public int GetPower(int power)
        {
            return (int)Math.Round(power / 4F * (Level + 1) + DefPower());
        }

        public long GetDelay()
        {
            return Info.DelayBase - (Level * Info.DelayReduction);
        }

}
public static class SourceFactory { public static MagicInfo Create(int id) => id switch {
        1 => new MagicInfo
                {
                    Name = "FireBall",
                    Spell = Spell.FireBall,
                    Icon = 0,
                    Level1 = 7,
                    Level2 = 9,
                    Level3 = 11,
                    Need1 = 200,
                    Need2 = 350,
                    Need3 = 700,
                    BaseCost = 3,
                    LevelCost = 2,
                    MPowerBase = 8,
                    PowerBase = 2,
                    Range = 9
                },
        2 => new MagicInfo
                {
                    Name = "Healing",
                    Spell = Spell.Healing,
                    Icon = 1,
                    Level1 = 7,
                    Level2 = 11,
                    Level3 = 14,
                    Need1 = 150,
                    Need2 = 350,
                    Need3 = 700,
                    BaseCost = 3,
                    LevelCost = 2,
                    MPowerBase = 14,
                    Range = 9
                },
        3 => new MagicInfo { Name = "Fencing", Spell = Spell.Fencing, Icon = 2, Level1 = 7, Level2 = 9, Level3 = 12, Need1 = 270, Need2 = 600, Need3 = 1300, Range = 0 },
        4 => new MagicInfo
                { Name = "SpiritSword", Spell = Spell.SpiritSword, Icon = 3, Level1 = 9, Level2 = 12, Level3 = 15, Need1 = 350, Need2 = 1300, Need3 = 2700, Range = 0 },
        5 => new MagicInfo
                {
                    Name = "GreatFireBall",
                    Spell = Spell.GreatFireBall,
                    Icon = 4,
                    Level1 = 15,
                    Level2 = 18,
                    Level3 = 21,
                    Need1 = 2000,
                    Need2 = 2700,
                    Need3 = 3500,
                    BaseCost = 5,
                    LevelCost = 1,
                    MPowerBase = 6,
                    PowerBase = 10,
                    Range = 9
                },
        7 => new MagicInfo { Name = "Slaying", Spell = Spell.Slaying, Icon = 6, Level1 = 15, Level2 = 17, Level3 = 20, Need1 = 500, Need2 = 1100, Need3 = 1800, Range = 0 },
        9 => new MagicInfo
                {
                    Name = "HellFire",
                    Spell = Spell.HellFire,
                    Icon = 8,
                    Level1 = 16,
                    Level2 = 20,
                    Level3 = 24,
                    Need1 = 700,
                    Need2 = 2700,
                    Need3 = 3500,
                    BaseCost = 10,
                    LevelCost = 3,
                    MPowerBase = 14,
                    PowerBase = 6,
                    Range = 0
                },
        10 => new MagicInfo
                {
                    Name = "Lightning",
                    Spell = Spell.Lightning,
                    Icon = 9,
                    Level1 = 26,
                    Level2 = 29,
                    Level3 = 32,
                    Need1 = 3000,
                    Need2 = 6000,
                    Need3 = 12000,
                    BaseCost = 38,
                    LevelCost = 7,
                    MPowerBase = 12,
                    PowerBase = 12,
                    Range = 0
                },
        11 => new MagicInfo
                {
                    Name = "ThunderBolt",
                    Spell = Spell.ThunderBolt,
                    Icon = 10,
                    Level1 = 17,
                    Level2 = 20,
                    Level3 = 23,
                    Need1 = 500,
                    Need2 = 2000,
                    Need3 = 3500,
                    BaseCost = 9,
                    LevelCost = 2,
                    MPowerBase = 8,
                    MPowerBonus = 20,
                    PowerBase = 9,
                    Range = 9
                },
        12 => new MagicInfo
                {
                    Name = "Thrusting",
                    Spell = Spell.Thrusting,
                    Icon = 11,
                    Level1 = 22,
                    Level2 = 24,
                    Level3 = 27,
                    Need1 = 2000,
                    Need2 = 3500,
                    Need3 = 6000,
                    Range = 0,
                    MultiplierBase = 0.25f,
                    MultiplierBonus = 0.25f
                },
        23 => new MagicInfo
                {
                    Name = "FireBang",
                    Spell = Spell.FireBang,
                    Icon = 22,
                    Level1 = 22,
                    Level2 = 25,
                    Level3 = 28,
                    Need1 = 3000,
                    Need2 = 5000,
                    Need3 = 10000,
                    BaseCost = 14,
                    LevelCost = 4,
                    MPowerBase = 8,
                    PowerBase = 8,
                    Range = 9
                },
        24 => new MagicInfo
                {
                    Name = "ThunderStorm",
                    Spell = Spell.ThunderStorm,
                    Icon = 23,
                    Level1 = 30,
                    Level2 = 32,
                    Level3 = 34,
                    Need1 = 4000,
                    Need2 = 8000,
                    Need3 = 12000,
                    BaseCost = 29,
                    LevelCost = 9,
                    MPowerBase = 10,
                    MPowerBonus = 20,
                    PowerBase = 10,
                    PowerBonus = 20,
                    Range = 0
                },
        25 => new MagicInfo
                {
                    Name = "HalfMoon",
                    Spell = Spell.HalfMoon,
                    Icon = 24,
                    Level1 = 26,
                    Level2 = 28,
                    Level3 = 31,
                    Need1 = 5000,
                    Need2 = 8000,
                    Need3 = 14000,
                    BaseCost = 3,
                    Range = 0,
                    MultiplierBase = 0.3f,
                    MultiplierBonus = 0.1f
                },
        26 => new MagicInfo
                {
                    Name = "FlamingSword",
                    Spell = Spell.FlamingSword,
                    Icon = 25,
                    Level1 = 35,
                    Level2 = 37,
                    Level3 = 40,
                    Need1 = 2000,
                    Need2 = 4000,
                    Need3 = 6000,
                    BaseCost = 7,
                    Range = 0,
                    MultiplierBase = 1.4f,
                    MultiplierBonus = 0.4f
                },
        29 => new MagicInfo
                {
                    Name = "MassHealing",
                    Spell = Spell.MassHealing,
                    Icon = 28,
                    Level1 = 31,
                    Level2 = 33,
                    Level3 = 36,
                    Need1 = 2000,
                    Need2 = 4000,
                    Need3 = 8000,
                    BaseCost = 28,
                    LevelCost = 3,
                    MPowerBase = 10,
                    PowerBase = 4,
                    Range = 9
                },
        33 => new MagicInfo
                {
                    Name = "IceStorm",
                    Spell = Spell.IceStorm,
                    Icon = 32,
                    Level1 = 35,
                    Level2 = 37,
                    Level3 = 40,
                    Need1 = 4000,
                    Need2 = 8000,
                    Need3 = 12000,
                    BaseCost = 33,
                    LevelCost = 3,
                    MPowerBase = 12,
                    PowerBase = 14,
                    Range = 9
                },
_ => throw new ArgumentOutOfRangeException(nameof(id)) }; }
