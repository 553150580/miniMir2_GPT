"""将已定的领时、普通击杀加成和个人首领权益投影到开发服务端。"""


def compile_benefits(rows):
    """只使用唯一玩法工作簿的已确认规则，不从历史经济样本推导。"""
    fields = {
        'free_daily_total_seconds': ('free_daily_seconds', int, 2**63 - 1),
        'hosting_daily_total_seconds': ('card_daily_seconds', int, 2**63 - 1),
        'hosting_kill_exp_bonus_bp': ('kill_experience_bonus_bp', int, 2**32 - 1),
        'hosting_direct_kill_gold_bonus_bp': ('direct_gold_bonus_bp', int, 2**32 - 1),
        'hosting_auto_personal_boss': ('auto_personal_boss', bool, None),
    }
    benefits = {}
    for row in rows:
        key = row['rule_key']
        if key not in fields:
            continue
        field, value_type, largest = fields[key]
        value = row['rule_value']
        if (field in benefits or row['record_status'] != 'confirmed' or type(value) is not value_type
                or (largest is not None and not 0 <= value <= largest)):
            raise ValueError(f'托管权益须使用唯一且已确认的有效配置：{key}')
        benefits[field] = value
    if len(benefits) != len(fields) or not 0 < benefits['free_daily_seconds'] <= benefits['card_daily_seconds']:
        raise ValueError('托管权益缺少完整规则或领时总额度无效')
    return benefits
