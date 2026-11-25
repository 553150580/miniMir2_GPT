"""将已定的领时与普通击杀权益投影到开发服务端。"""


def compile_benefits(rows):
    """只使用唯一玩法工作簿的已确认规则，不从历史经济样本推导。"""
    fields = {
        'free_daily_total_seconds': 'free_daily_seconds',
        'hosting_daily_total_seconds': 'card_daily_seconds',
        'hosting_kill_exp_bonus_bp': 'kill_experience_bonus_bp',
        'hosting_direct_kill_gold_bonus_bp': 'direct_gold_bonus_bp',
    }
    benefits = {}
    for row in rows:
        key = row['rule_key']
        if key not in fields:
            continue
        field, value = fields[key], row['rule_value']
        largest = 2**32 - 1 if field.endswith('_bp') else 2**63 - 1
        if field in benefits or row['record_status'] != 'confirmed' or type(value) is not int or not 0 <= value <= largest:
            raise ValueError(f'托管权益须使用唯一且已确认的整数配置：{key}')
        benefits[field] = value
    if (benefits.keys() != set(fields.values()) or not 0 < benefits['free_daily_seconds'] <= benefits['card_daily_seconds']):
        raise ValueError('托管权益缺少完整领时或击杀加成规则')
    return benefits
