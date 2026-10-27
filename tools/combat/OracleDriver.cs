// 只驱动原公式，不在此重写GetDamage/GetPower；源片段由run_source_oracle.py逐字提取。
using System.Text.Json;

public sealed record Vector(int SkillId, byte Rank, int Base, int MPowerRoll, int PowerRoll);

public sealed class FixedRandom
{
    private readonly Queue<int> offsets;
    public FixedRandom(IEnumerable<int> values) { offsets = new Queue<int>(values); }
    public int Next(int min, int max)
    {
        int offset = offsets.Dequeue();
        if (offset < 0 || offset >= max - min) throw new InvalidDataException("固定抽值越界");
        return min + offset;
    }
}

public static class Envir
{
    public static FixedRandom Random = new FixedRandom(Array.Empty<int>());
}

public static class Program
{
    private static void Reset(MagicInfo info, Vector v)
    {
        var rolls = new List<int>();
        if (info.MPowerBonus > 0) rolls.Add(v.MPowerRoll);
        if (info.PowerBonus > 0) rolls.Add(v.PowerRoll);
        Envir.Random = new FixedRandom(rolls);
    }
    public static void Main(string[] args)
    {
        var options = new JsonSerializerOptions { PropertyNameCaseInsensitive = true };
        var vectors = JsonSerializer.Deserialize<Vector[]>(File.ReadAllText(args[0]), options)!;
        using var output = new StreamWriter(args[1]);
        foreach (var v in vectors)
        {
            var info = SourceFactory.Create(v.SkillId);
            var magic = new UserMagic { Info = info, Level = v.Rank };
            Reset(info, v);
            int damage = magic.GetDamage(v.Base);
            Reset(info, v);
            int power = magic.GetPower();
            output.WriteLine(JsonSerializer.Serialize(new {
                skill_id = v.SkillId, rank = v.Rank, @base = v.Base,
                mpower_roll = v.MPowerRoll, power_roll = v.PowerRoll,
                mpower_base = info.MPowerBase, mpower_bonus = info.MPowerBonus,
                power_base = info.PowerBase, power_bonus = info.PowerBonus,
                multiplier_base = info.MultiplierBase, multiplier_bonus = info.MultiplierBonus,
                multiplier_base_bits = BitConverter.SingleToUInt32Bits(info.MultiplierBase),
                multiplier_bonus_bits = BitConverter.SingleToUInt32Bits(info.MultiplierBonus),
                multiplier_bits = BitConverter.SingleToUInt32Bits(magic.GetMultiplier()),
                expected_power = power, expected_damage = damage, expected_delay_ms = magic.GetDelay()
            }));
        }
    }
}
