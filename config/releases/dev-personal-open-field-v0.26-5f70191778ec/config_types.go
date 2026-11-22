// 由配置schema生成。字段使用指针区分缺失值与0。
package configtypes

import "encoding/json"

var _ json.RawMessage

type ArchNodeRoles struct {
    Code *string `json:"code"`
    ProcessMode *string `json:"process_mode"`
    SingletonScope *string `json:"singleton_scope"`
    Responsibilities *string `json:"responsibilities"`
    AdaptationStatus *string `json:"adaptation_status"`
}

type ArchRuntimeBudgets struct {
    Code *string `json:"code"`
    Value *float64 `json:"value"`
    ValueUnit *string `json:"value_unit"`
    Applicability *string `json:"applicability"`
    AdaptationStatus *string `json:"adaptation_status"`
}

type ArchStorageGuards struct {
    Code *string `json:"code"`
    GuardGroupCount *int64 `json:"guard_group_count"`
    HashAlgorithm *string `json:"hash_algorithm"`
    GroupKey *string `json:"group_key"`
    ProtectedFields json.RawMessage `json:"protected_fields"`
}

type ArchProtocolRanges struct {
    Code *string `json:"code"`
    MinMessageId *int64 `json:"min_message_id"`
    MaxMessageId *int64 `json:"max_message_id"`
    Purpose *string `json:"purpose"`
    InternalOnly *bool `json:"internal_only"`
}

type ArchConfigClasses struct {
    Code *string `json:"code"`
    Contents *string `json:"contents"`
    ActivationMode *string `json:"activation_mode"`
    ReleaseContract *string `json:"release_contract"`
}

type ArchStageGates struct {
    Code *string `json:"code"`
    StageTitle *string `json:"stage_title"`
    Deliverables *string `json:"deliverables"`
    PassCondition *string `json:"pass_condition"`
}

type GameCurrencies struct {
    Id *int64 `json:"id"`
    CurrencyKey *string `json:"currency_key"`
    DisplayName *string `json:"display_name"`
    UnitsPerDisplay *int64 `json:"units_per_display"`
    Bound *bool `json:"bound"`
    Shareable *bool `json:"shareable"`
    MarketPayment *bool `json:"market_payment"`
    MintSources json.RawMessage `json:"mint_sources"`
}

type GameJobs struct {
    Id *int64 `json:"id"`
    ClassKey *string `json:"class_key"`
    DisplayName *string `json:"display_name"`
    GenderId *int64 `json:"gender_id"`
    AppearanceKey *string `json:"appearance_key"`
    SelectGender *bool `json:"select_gender"`
    AllowClassChange *bool `json:"allow_class_change"`
}

type GameEquipmentSlots struct {
    Id *int64 `json:"id"`
    SlotKey *string `json:"slot_key"`
    DisplayName *string `json:"display_name"`
    Strengthenable *bool `json:"strengthenable"`
    OilAllowed *bool `json:"oil_allowed"`
    InitialLuckMax *int64 `json:"initial_luck_max"`
}

type GameQualities struct {
    Id *int64 `json:"id"`
    QualityKey *string `json:"quality_key"`
    DisplayName *string `json:"display_name"`
    AffixSlots *int64 `json:"affix_slots"`
    RandomDropAllowed *bool `json:"random_drop_allowed"`
    UnboundMarketAllowed *bool `json:"unbound_market_allowed"`
}

type GameStages struct {
    Id *int64 `json:"id"`
    StageKey *string `json:"stage_key"`
    RequiredLevelMin *int64 `json:"required_level_min"`
    RequiredLevelMax *int64 `json:"required_level_max"`
    Formula *string `json:"formula"`
}

type GameProductRules struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameWarehouseRules struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameBindingEvents struct {
    Id *int64 `json:"id"`
    EventKey *string `json:"event_key"`
    DecisionTime *string `json:"decision_time"`
    BindingPolicy *string `json:"binding_policy"`
    AllActualInputs *bool `json:"all_actual_inputs"`
    RejudgeOnClaim *bool `json:"rejudge_on_claim"`
}

type GameBoundResourceUi struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameCardProducts struct {
    Id *int64 `json:"id"`
    CardKey *string `json:"card_key"`
    PaymentKey *string `json:"payment_key"`
    DurationDays *int64 `json:"duration_days"`
    CashFen *int64 `json:"cash_fen"`
    IngotU *int64 `json:"ingot_u"`
    RoleScoped *bool `json:"role_scoped"`
}

type GameEntitlementRules struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameItemMarketRules struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameGoldMarketRules struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameDropCategories struct {
    Id *int64 `json:"id"`
    CategoryKey *string `json:"category_key"`
    ProbPpm *int64 `json:"prob_ppm"`
    ExclusiveGroup *string `json:"exclusive_group"`
}

type GameDropGuarantees struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameAffixes struct {
    Id *int64 `json:"id"`
    AttrKey *string `json:"attr_key"`
    DisplayName *string `json:"display_name"`
    MinimumQuality *int64 `json:"minimum_quality"`
    SampleWeight *int64 `json:"sample_weight"`
    ValueCurveKey *string `json:"value_curve_key"`
}

type GameRefineQuality struct {
    Id *int64 `json:"id"`
    Quality *int64 `json:"quality"`
    SlotCount *int64 `json:"slot_count"`
    PoolSize *int64 `json:"pool_size"`
    StonesPerAttempt *int64 `json:"stones_per_attempt"`
    AllowDuplicateAttr *bool `json:"allow_duplicate_attr"`
}

type GameRefineTiers struct {
    Id *int64 `json:"id"`
    Tier *int64 `json:"tier"`
    MultiplierBp *int64 `json:"multiplier_bp"`
    WeightPpm *int64 `json:"weight_ppm"`
}

type GameRefineLockCosts struct {
    Id *int64 `json:"id"`
    LockedSlots *int64 `json:"locked_slots"`
    ExtraIngotUPerAttempt *int64 `json:"extra_ingot_u_per_attempt"`
}

type GameEnhancementLevels struct {
    Id *int64 `json:"id"`
    FromLevel *int64 `json:"from_level"`
    ToLevel *int64 `json:"to_level"`
    Stage *int64 `json:"stage"`
    SuccessPpm *int64 `json:"success_ppm"`
    StoneCost *int64 `json:"stone_cost"`
    BaseAttrIncrementBp *int64 `json:"base_attr_increment_bp"`
}

type GameBodyTracks struct {
    Id *int64 `json:"id"`
    TrackKey *string `json:"track_key"`
    DisplayName *string `json:"display_name"`
    CurrencyFamily *string `json:"currency_family"`
    CostBaseU *int64 `json:"cost_base_u"`
    BudgetGroup *string `json:"budget_group"`
    AttrCurveKey *string `json:"attr_curve_key"`
}

type GameBodyLevelSteps struct {
    Id *int64 `json:"id"`
    FromLevel *int64 `json:"from_level"`
    ToLevel *int64 `json:"to_level"`
    CostMultiplier *int64 `json:"cost_multiplier"`
    SuccessPpm *int64 `json:"success_ppm"`
}

type GameLuckInitial struct {
    Id *int64 `json:"id"`
    SourceKey *string `json:"source_key"`
    TriggerPpm *int64 `json:"trigger_ppm"`
    InitialValueWeights json.RawMessage `json:"initial_value_weights"`
    FixedInitialValue *int64 `json:"fixed_initial_value"`
}

type GameLuckOilSteps struct {
    Id *int64 `json:"id"`
    FromLuck *int64 `json:"from_luck"`
    TargetLuck *int64 `json:"target_luck"`
    SuccessPpm *int64 `json:"success_ppm"`
    FailureLuck *int64 `json:"failure_luck"`
    PitySegment *string `json:"pity_segment"`
    ThresholdUsedBottles *int64 `json:"threshold_used_bottles"`
    PityStartsAfterThreshold *bool `json:"pity_starts_after_threshold"`
}

type GameLuckContribution struct {
    Id *int64 `json:"id"`
    SourceGroup *string `json:"source_group"`
    Luck *int64 `json:"luck"`
    ContributionBp *int64 `json:"contribution_bp"`
}

type GameLuckSources struct {
    Id *int64 `json:"id"`
    SourceKey *string `json:"source_key"`
    MaximumLuck *int64 `json:"maximum_luck"`
    OilAllowed *bool `json:"oil_allowed"`
    ContributionGroup *string `json:"contribution_group"`
    ActivationRequirement *string `json:"activation_requirement"`
}

type GameLuckRules struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameOreTypes struct {
    Id *int64 `json:"id"`
    OreKey *string `json:"ore_key"`
    DisplayName *string `json:"display_name"`
    Layer1Weight *int64 `json:"layer_1_weight"`
    Layer2Weight *int64 `json:"layer_2_weight"`
    PriceGoldNumerator *int64 `json:"price_gold_numerator"`
    PriceGoldDenominator *int64 `json:"price_gold_denominator"`
    IntegerRoundingRule *string `json:"integer_rounding_rule"`
    CraftMaterial *bool `json:"craft_material"`
}

type GameMineLayers struct {
    Id *int64 `json:"id"`
    Layer *int64 `json:"layer"`
    MinimumLevel *int64 `json:"minimum_level"`
    MapConfigId *int64 `json:"map_config_id"`
}

type GameOrePurity struct {
    Id *int64 `json:"id"`
    PurityMin *int64 `json:"purity_min"`
    PurityMax *int64 `json:"purity_max"`
    WeightPpm *int64 `json:"weight_ppm"`
    WithinBand *string `json:"within_band"`
}

type GameWorkshopRules struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameLevelBandEconomy struct {
    Id *int64 `json:"id"`
    LevelMin *int64 `json:"level_min"`
    LevelMax *int64 `json:"level_max"`
    GRefGoldPerHour *int64 `json:"g_ref_gold_per_hour"`
    HostingGoldPerHour *int64 `json:"hosting_gold_per_hour"`
    RefineGoldPerAttempt *int64 `json:"refine_gold_per_attempt"`
    RareCraftGold *int64 `json:"rare_craft_gold"`
    EpicCraftGold *int64 `json:"epic_craft_gold"`
    LegendaryCraftGold *int64 `json:"legendary_craft_gold"`
}

type GameBookRecycle struct {
    Id *int64 `json:"id"`
    BookQuality *int64 `json:"book_quality"`
    Pages *int64 `json:"pages"`
    NpcBasicBookPages *int64 `json:"npc_basic_book_pages"`
}

type GameShopPrices struct {
    Id *int64 `json:"id"`
    ProductKey *string `json:"product_key"`
    DisplayName *string `json:"display_name"`
    PaymentCurrency *string `json:"payment_currency"`
    PriceU *int64 `json:"price_u"`
    Quantity *int64 `json:"quantity"`
    PurchasedBound *bool `json:"purchased_bound"`
    DailyLimit *int64 `json:"daily_limit"`
    PriceGold *int64 `json:"price_gold"`
}

type GameChallengeRules struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameDungeonUnlocks struct {
    Id *int64 `json:"id"`
    RequiredLevel *int64 `json:"required_level"`
    DungeonTemplateId *int64 `json:"dungeon_template_id"`
}

type GameArenaRules struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameArenaStreakPoints struct {
    Id *int64 `json:"id"`
    StreakWins *int64 `json:"streak_wins"`
    WinPoints *int64 `json:"win_points"`
}

type GameProgressionRules struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameSoulTypes struct {
    Id *int64 `json:"id"`
    SoulKey *string `json:"soul_key"`
    DisplayName *string `json:"display_name"`
    MaxStar *int64 `json:"max_star"`
    MaxOwned *int64 `json:"max_owned"`
    TradeableRecord *bool `json:"tradeable_record"`
}

type GameSoulAffixes struct {
    Id *int64 `json:"id"`
    PoolKey *string `json:"pool_key"`
    AttrKey *string `json:"attr_key"`
    DisplayName *string `json:"display_name"`
    UnitDefinition *string `json:"unit_definition"`
    StarValues json.RawMessage `json:"star_values"`
    TriggerAndTarget json.RawMessage `json:"trigger_and_target"`
}

type GameCollectionRules struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameWingRanks struct {
    Id *int64 `json:"id"`
    Rank *int64 `json:"rank"`
    FeatherCost *int64 `json:"feather_cost"`
    AttributeEffects json.RawMessage `json:"attribute_effects"`
    AppearanceEvolves *bool `json:"appearance_evolves"`
}

type GameBattlepassTiers struct {
    Id *int64 `json:"id"`
    TierKey *string `json:"tier_key"`
    DurationDays *int64 `json:"duration_days"`
    CashFen *int64 `json:"cash_fen"`
    CashOnly *bool `json:"cash_only"`
    DailyTotalNotStack *bool `json:"daily_total_not_stack"`
}

type GameBattlepassRewards struct {
    Id *int64 `json:"id"`
    TierKey *string `json:"tier_key"`
    PeriodKey *string `json:"period_key"`
    RewardKey *string `json:"reward_key"`
    QuantityU *int64 `json:"quantity_u"`
    Bound *bool `json:"bound"`
}

type GameCommercialCandidates struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameAttributeBudgets struct {
    Id *int64 `json:"id"`
    BudgetKey *string `json:"budget_key"`
    LimitBp *int64 `json:"limit_bp"`
    ConstraintKind *string `json:"constraint_kind"`
    Stacking *string `json:"stacking"`
}

type GameClientPresentation struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

type GameJobGrowth struct {
    Id *int64 `json:"id"`
    ClassId *int64 `json:"class_id"`
    Level *int64 `json:"level"`
    Hp *int64 `json:"hp"`
    Mp *int64 `json:"mp"`
    AttackMin *int64 `json:"attack_min"`
    AttackMax *int64 `json:"attack_max"`
    MagicMin *int64 `json:"magic_min"`
    MagicMax *int64 `json:"magic_max"`
    TaoMin *int64 `json:"tao_min"`
    TaoMax *int64 `json:"tao_max"`
    DefenceMin *int64 `json:"defence_min"`
    DefenceMax *int64 `json:"defence_max"`
    MagicDefenceMin *int64 `json:"magic_defence_min"`
    MagicDefenceMax *int64 `json:"magic_defence_max"`
    Accuracy *int64 `json:"accuracy"`
    Agility *int64 `json:"agility"`
    WeightLimits json.RawMessage `json:"weight_limits"`
}

type GameItems struct {
    Id *int64 `json:"id"`
    ItemKey *string `json:"item_key"`
    DisplayName *string `json:"display_name"`
    ItemType *string `json:"item_type"`
    MaxStack *int64 `json:"max_stack"`
    EffectListId *int64 `json:"effect_list_id"`
    Weight *int64 `json:"weight"`
    Quality *int64 `json:"quality"`
    Description *string `json:"description"`
    IconResourceKey *string `json:"icon_resource_key"`
}

type GameEquipment struct {
    Id *int64 `json:"id"`
    ItemId *int64 `json:"item_id"`
    EquipmentType *string `json:"equipment_type"`
    ClassRestriction json.RawMessage `json:"class_restriction"`
    RequiredLevel *int64 `json:"required_level"`
    Quality *int64 `json:"quality"`
    Stage *int64 `json:"stage"`
    BaseAttributes json.RawMessage `json:"base_attributes"`
    OriginalDurabilityU *int64 `json:"original_durability_u"`
    Weight *int64 `json:"weight"`
    CraftBonusStrengthenable *bool `json:"craft_bonus_strengthenable"`
}

type GameAppearance struct {
    Id *int64 `json:"id"`
    ItemId *int64 `json:"item_id"`
    ClassId *int64 `json:"class_id"`
    AppearanceKey *string `json:"appearance_key"`
    BodyResourceKey *string `json:"body_resource_key"`
    WeaponResourceKey *string `json:"weapon_resource_key"`
    AnchorAndLayers json.RawMessage `json:"anchor_and_layers"`
    ActionMap json.RawMessage `json:"action_map"`
}

type GameMonsters struct {
    Id *int64 `json:"id"`
    MonsterKey *string `json:"monster_key"`
    DisplayName *string `json:"display_name"`
    Level *int64 `json:"level"`
    Category *string `json:"category"`
    Hp *int64 `json:"hp"`
    Attributes json.RawMessage `json:"attributes"`
    AttackIntervalMs *int64 `json:"attack_interval_ms"`
    MoveIntervalMs *int64 `json:"move_interval_ms"`
    AttackBehaviorId *string `json:"attack_behavior_id"`
    AggroRangeCells *int64 `json:"aggro_range_cells"`
    ChaseRangeCells *int64 `json:"chase_range_cells"`
    Exp *int64 `json:"exp"`
    DropPoolId *int64 `json:"drop_pool_id"`
    ResourceKey *string `json:"resource_key"`
}

type GameMaps struct {
    Id *int64 `json:"id"`
    MapKey *string `json:"map_key"`
    DisplayName *string `json:"display_name"`
    SceneKind *string `json:"scene_kind"`
    WidthCells *int64 `json:"width_cells"`
    HeightCells *int64 `json:"height_cells"`
    RequiredLevel *int64 `json:"required_level"`
    CollisionId *int64 `json:"collision_id"`
    SpawnCell json.RawMessage `json:"spawn_cell"`
    TileResourceKey *string `json:"tile_resource_key"`
    MapVersion *string `json:"map_version"`
}

type GameMapCollisions struct {
    Id *int64 `json:"id"`
    MapKey *string `json:"map_key"`
    WidthCells *int64 `json:"width_cells"`
    HeightCells *int64 `json:"height_cells"`
    BlockedCells json.RawMessage `json:"blocked_cells"`
    WalkableEncoding *string `json:"walkable_encoding"`
    DiagonalRule *string `json:"diagonal_rule"`
}

type GameMapRegions struct {
    Id *int64 `json:"id"`
    MapId *int64 `json:"map_id"`
    RegionKey *string `json:"region_key"`
    RegionCells json.RawMessage `json:"region_cells"`
    NpcId *int64 `json:"npc_id"`
    MonsterPool json.RawMessage `json:"monster_pool"`
    EntryRules json.RawMessage `json:"entry_rules"`
}

type GameMonsterSpawns struct {
    Id *int64 `json:"id"`
    MapId *int64 `json:"map_id"`
    SlotIndex *int64 `json:"slot_index"`
    MonsterId *int64 `json:"monster_id"`
    Cell json.RawMessage `json:"cell"`
    RespawnSeconds *int64 `json:"respawn_seconds"`
    SpawnKind *string `json:"spawn_kind"`
    SlotResizePolicy *string `json:"slot_resize_policy"`
}

type GameNpcs struct {
    Id *int64 `json:"id"`
    NpcKey *string `json:"npc_key"`
    DisplayName *string `json:"display_name"`
    MapId *int64 `json:"map_id"`
    Cell json.RawMessage `json:"cell"`
    FunctionKeys json.RawMessage `json:"function_keys"`
    ResourceKey *string `json:"resource_key"`
}

type GameSkills struct {
    Id *int64 `json:"id"`
    SkillKey *string `json:"skill_key"`
    DisplayName *string `json:"display_name"`
    ClassId *int64 `json:"class_id"`
    LearnLevel *int64 `json:"learn_level"`
    BehaviorId *string `json:"behavior_id"`
    TargetRule json.RawMessage `json:"target_rule"`
    RangePatternId *int64 `json:"range_pattern_id"`
    CooldownMs *int64 `json:"cooldown_ms"`
    CastTiming json.RawMessage `json:"cast_timing"`
    ResourceCosts json.RawMessage `json:"resource_costs"`
    DamageOrHealPath *string `json:"damage_or_heal_path"`
    EffectIds json.RawMessage `json:"effect_ids"`
    ActionResourceKey *string `json:"action_resource_key"`
    BookItemId *int64 `json:"book_item_id"`
    NativeStartLevel *int64 `json:"native_start_level"`
    NativeMaxLevel *int64 `json:"native_max_level"`
    DamageParameters json.RawMessage `json:"damage_parameters"`
}

type GameSkillUpgrades struct {
    Id *int64 `json:"id"`
    SkillId *int64 `json:"skill_id"`
    Level *int64 `json:"level"`
    BookCost json.RawMessage `json:"book_cost"`
    PageCost *int64 `json:"page_cost"`
    AllowedNumericChanges json.RawMessage `json:"allowed_numeric_changes"`
    PreserveBehaviorShape *bool `json:"preserve_behavior_shape"`
    RequiredRoleLevel *int64 `json:"required_role_level"`
    PageItemId *int64 `json:"page_item_id"`
}

type GameRangePatterns struct {
    Id *int64 `json:"id"`
    PatternKey *string `json:"pattern_key"`
    RelativeCells json.RawMessage `json:"relative_cells"`
    DirectionRotation json.RawMessage `json:"direction_rotation"`
    ObstaclePolicy *string `json:"obstacle_policy"`
    MaxTargets *int64 `json:"max_targets"`
    LineOfSight *string `json:"line_of_sight"`
}

type GameBuffs struct {
    Id *int64 `json:"id"`
    BuffKey *string `json:"buff_key"`
    AttributeEffects json.RawMessage `json:"attribute_effects"`
    DurationMs *int64 `json:"duration_ms"`
    TickMs *int64 `json:"tick_ms"`
    StackPolicy *string `json:"stack_policy"`
    RemoveEvents json.RawMessage `json:"remove_events"`
    ImmunityRules json.RawMessage `json:"immunity_rules"`
    BudgetGroup *string `json:"budget_group"`
    DurationRule json.RawMessage `json:"duration_rule"`
    DisplayName *string `json:"display_name"`
}

type GameEffects struct {
    Id *int64 `json:"id"`
    EffectKey *string `json:"effect_key"`
    EffectType *string `json:"effect_type"`
    Parameters json.RawMessage `json:"parameters"`
    TriggerPhase *string `json:"trigger_phase"`
    TargetRule json.RawMessage `json:"target_rule"`
    ProcLimit json.RawMessage `json:"proc_limit"`
    RecursiveTriggerAllowed *bool `json:"recursive_trigger_allowed"`
}

type GamePetTemplates struct {
    Id *int64 `json:"id"`
    PetKey *string `json:"pet_key"`
    SummonSkillId *int64 `json:"summon_skill_id"`
    MonsterTemplateId *int64 `json:"monster_template_id"`
    BaseAttributes json.RawMessage `json:"base_attributes"`
    AttackBehavior *string `json:"attack_behavior"`
    FollowAndReturn json.RawMessage `json:"follow_and_return"`
    LifetimeAndBetrayal json.RawMessage `json:"lifetime_and_betrayal"`
    ResourceKey *string `json:"resource_key"`
}

type GamePetGrowth struct {
    Id *int64 `json:"id"`
    PetId *int64 `json:"pet_id"`
    GrowthLevel *int64 `json:"growth_level"`
    Attributes json.RawMessage `json:"attributes"`
    RequiredConditions json.RawMessage `json:"required_conditions"`
}

type GameExpLevels struct {
    Id *int64 `json:"id"`
    Level *int64 `json:"level"`
    NextLevelExp *int64 `json:"next_level_exp"`
    GrowthTarget *string `json:"growth_target"`
    MeasurementVersion *string `json:"measurement_version"`
}

type GameRebirthCosts struct {
    Id *int64 `json:"id"`
    RebirthIndex *int64 `json:"rebirth_index"`
    RequiredLevel *int64 `json:"required_level"`
    NextLevelCap *int64 `json:"next_level_cap"`
    ChallengeTokenId *int64 `json:"challenge_token_id"`
    GoldCost *int64 `json:"gold_cost"`
    BoundMaterialCosts json.RawMessage `json:"bound_material_costs"`
    BoundIngotAccelerationCap json.RawMessage `json:"bound_ingot_acceleration_cap"`
}

type GameDropPools struct {
    Id *int64 `json:"id"`
    PoolKey *string `json:"pool_key"`
    SourceKind *string `json:"source_kind"`
    CategoryKey *string `json:"category_key"`
    SelectionMode *string `json:"selection_mode"`
    EquipmentStageRange json.RawMessage `json:"equipment_stage_range"`
    QualityWeights json.RawMessage `json:"quality_weights"`
    MaxRandomQuality *int64 `json:"max_random_quality"`
}

type GameDropEntries struct {
    Id *int64 `json:"id"`
    PoolId *int64 `json:"pool_id"`
    ItemId *int64 `json:"item_id"`
    CurrencyKey *string `json:"currency_key"`
    Weight *int64 `json:"weight"`
    QuantityMin *int64 `json:"quantity_min"`
    QuantityMax *int64 `json:"quantity_max"`
    BindingPolicy *string `json:"binding_policy"`
}

type GameDropQuotas struct {
    Id *int64 `json:"id"`
    Stage *int64 `json:"stage"`
    Quality *int64 `json:"quality"`
    SourceKind *string `json:"source_kind"`
    WindowSeconds *int64 `json:"window_seconds"`
    QuotaUnits *int64 `json:"quota_units"`
    RefillSchedule json.RawMessage `json:"refill_schedule"`
    ExhaustedDowngradeWeights json.RawMessage `json:"exhausted_downgrade_weights"`
}

type GameRecycleValues struct {
    Id *int64 `json:"id"`
    EquipmentStage *int64 `json:"equipment_stage"`
    Quality *int64 `json:"quality"`
    GoldDistribution json.RawMessage `json:"gold_distribution"`
    BrokenGold *int64 `json:"broken_gold"`
    StoneProbabilityPpm *int64 `json:"stone_probability_ppm"`
    BrokenByproducts json.RawMessage `json:"broken_byproducts"`
    OilProbabilityFormula *string `json:"oil_probability_formula"`
    RewardsCombinable json.RawMessage `json:"rewards_combinable"`
}

type GameRefineValues struct {
    Id *int64 `json:"id"`
    AffixId *int64 `json:"affix_id"`
    EquipmentStage *int64 `json:"equipment_stage"`
    BaseValueFixed *int64 `json:"base_value_fixed"`
    ValueScale *int64 `json:"value_scale"`
    StackingGroup *string `json:"stacking_group"`
    PerItemCap *int64 `json:"per_item_cap"`
    WholeRoleCap *int64 `json:"whole_role_cap"`
}

type GamePickaxes struct {
    Id *int64 `json:"id"`
    ToolKey *string `json:"tool_key"`
    RequiredLayer *int64 `json:"required_layer"`
    GoldPrice *int64 `json:"gold_price"`
    DurabilityU *int64 `json:"durability_u"`
    LossPerAttempt *int64 `json:"loss_per_attempt"`
}

type GameCraftRecipes struct {
    Id *int64 `json:"id"`
    BlueprintItemId *int64 `json:"blueprint_item_id"`
    OutputItemId *int64 `json:"output_item_id"`
    Quality *int64 `json:"quality"`
    RequiredLevel *int64 `json:"required_level"`
    EquipmentType *string `json:"equipment_type"`
    OreCount *int64 `json:"ore_count"`
    GoldCost *int64 `json:"gold_cost"`
    SuccessRule *string `json:"success_rule"`
    FailureLosses json.RawMessage `json:"failure_losses"`
    ExtraBaseAttribute json.RawMessage `json:"extra_base_attribute"`
}

type GameRepairPrices struct {
    Id *int64 `json:"id"`
    RepairKey *string `json:"repair_key"`
    EquipmentScope json.RawMessage `json:"equipment_scope"`
    CurrencyFamily *string `json:"currency_family"`
    CostFormula *string `json:"cost_formula"`
    UpperDurabilityLossBp *int64 `json:"upper_durability_loss_bp"`
    RepairAmountRule *string `json:"repair_amount_rule"`
    MaterialCosts json.RawMessage `json:"material_costs"`
}

type GameBossTemplates struct {
    Id *int64 `json:"id"`
    BossKind *string `json:"boss_kind"`
    MonsterId *int64 `json:"monster_id"`
    MapId *int64 `json:"map_id"`
    RequiredLevel *int64 `json:"required_level"`
    CooldownSeconds *int64 `json:"cooldown_seconds"`
    FailureCooldownSeconds *int64 `json:"failure_cooldown_seconds"`
    ParticipationRule json.RawMessage `json:"participation_rule"`
    RankRewardGroupId *int64 `json:"rank_reward_group_id"`
    ParticipationRewardGroupId *int64 `json:"participation_reward_group_id"`
}

type GameDungeonTemplates struct {
    Id *int64 `json:"id"`
    DungeonKey *string `json:"dungeon_key"`
    RequiredLevel *int64 `json:"required_level"`
    Segments json.RawMessage `json:"segments"`
    TimeLimitSec *int64 `json:"time_limit_sec"`
    AiHireOptions json.RawMessage `json:"ai_hire_options"`
    RewardGroupId *int64 `json:"reward_group_id"`
    DisconnectPolicy *string `json:"disconnect_policy"`
}

type GameArenaRewards struct {
    Id *int64 `json:"id"`
    RewardKind *string `json:"reward_kind"`
    RankMin *int64 `json:"rank_min"`
    RankMax *int64 `json:"rank_max"`
    RequiredParticipation *int64 `json:"required_participation"`
    ExchangePointCost *int64 `json:"exchange_point_cost"`
    RewardGroupId *int64 `json:"reward_group_id"`
}

type GameQuests struct {
    Id *int64 `json:"id"`
    QuestKey *string `json:"quest_key"`
    Chapter *int64 `json:"chapter"`
    Prerequisites json.RawMessage `json:"prerequisites"`
    ObjectiveEvents json.RawMessage `json:"objective_events"`
    RewardGroupId *int64 `json:"reward_group_id"`
    NextQuestId *int64 `json:"next_quest_id"`
}

type GameDailyTasks struct {
    Id *int64 `json:"id"`
    TaskKey *string `json:"task_key"`
    ObjectiveEvent *string `json:"objective_event"`
    TargetCount *int64 `json:"target_count"`
    ActivityPoints *int64 `json:"activity_points"`
    RewardGroupId *int64 `json:"reward_group_id"`
    UnlockConditions json.RawMessage `json:"unlock_conditions"`
}

type GameAchievements struct {
    Id *int64 `json:"id"`
    AchievementKey *string `json:"achievement_key"`
    ObjectiveEvents json.RawMessage `json:"objective_events"`
    RequiredProgress *int64 `json:"required_progress"`
    RewardGroupId *int64 `json:"reward_group_id"`
}

type GameCodex struct {
    Id *int64 `json:"id"`
    MonsterId *int64 `json:"monster_id"`
    MapGroupId *int64 `json:"map_group_id"`
    RequiredKills *int64 `json:"required_kills"`
    QualificationRule *string `json:"qualification_rule"`
    RewardGroupId *int64 `json:"reward_group_id"`
    BudgetedAttributes json.RawMessage `json:"budgeted_attributes"`
}

type GameSoulStars struct {
    Id *int64 `json:"id"`
    SoulTypeId *int64 `json:"soul_type_id"`
    Star *int64 `json:"star"`
    DecomposeValue *int64 `json:"decompose_value"`
    UpgradeFragmentCost *int64 `json:"upgrade_fragment_cost"`
    SynthesisCost *int64 `json:"synthesis_cost"`
    DuplicateFragmentValue *int64 `json:"duplicate_fragment_value"`
    AttributeValues json.RawMessage `json:"attribute_values"`
}

type GameMedalLevels struct {
    Id *int64 `json:"id"`
    Level *int64 `json:"level"`
    FragmentCost *int64 `json:"fragment_cost"`
    ClassAttributes json.RawMessage `json:"class_attributes"`
    ConvenienceEffects json.RawMessage `json:"convenience_effects"`
}

type GameRewardGroups struct {
    Id *int64 `json:"id"`
    RewardGroupKey *string `json:"reward_group_key"`
    SourceKind *string `json:"source_kind"`
    BindingPolicy *string `json:"binding_policy"`
    Qualification json.RawMessage `json:"qualification"`
    BudgetWindow json.RawMessage `json:"budget_window"`
}

type GameRewardEntries struct {
    Id *int64 `json:"id"`
    RewardGroupId *int64 `json:"reward_group_id"`
    AssetKey *string `json:"asset_key"`
    QuantityDistribution json.RawMessage `json:"quantity_distribution"`
    WeightOrProbability json.RawMessage `json:"weight_or_probability"`
    BindingOverride *string `json:"binding_override"`
}

type GameSystemMail struct {
    Id *int64 `json:"id"`
    TemplateKey *string `json:"template_key"`
    Title *string `json:"title"`
    Body *string `json:"body"`
    RewardGroupId *int64 `json:"reward_group_id"`
    ExpirySeconds *int64 `json:"expiry_seconds"`
}

type GameAttributePipeline struct {
    Id *int64 `json:"id"`
    AttrKey *string `json:"attr_key"`
    Unit *string `json:"unit"`
    FixedScale *int64 `json:"fixed_scale"`
    SourceScope *string `json:"source_scope"`
    TargetScope *string `json:"target_scope"`
    CalculationPhase *string `json:"calculation_phase"`
    StackingGroup *string `json:"stacking_group"`
    StackingMode *string `json:"stacking_mode"`
    CapRule json.RawMessage `json:"cap_rule"`
    RoundingPhase *string `json:"rounding_phase"`
    BaseOrExtension *string `json:"base_or_extension"`
}

type GameServerRules struct {
    Id *int64 `json:"id"`
    RuleKey *string `json:"rule_key"`
    RuleValue json.RawMessage `json:"rule_value"`
}

