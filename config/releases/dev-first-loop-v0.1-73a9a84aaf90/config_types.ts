// 由配置schema生成。研发预览允许null，正式发布须通过完整校验。

export interface ArchProtocolRanges {
  code: string | null;
  min_message_id: number | null;
  max_message_id: number | null;
  purpose: string | null;
  internal_only: boolean | null;
}

export interface GameCurrencies {
  id: number | null;
  currency_key: string | null;
  display_name: string | null;
  units_per_display: number | null;
  bound: boolean | null;
  shareable: boolean | null;
  market_payment: boolean | null;
}

export interface GameJobs {
  id: number | null;
  class_key: string | null;
  display_name: string | null;
  gender_id: number | null;
  appearance_key: string | null;
  select_gender: boolean | null;
  allow_class_change: boolean | null;
}

export interface GameEquipmentSlots {
  id: number | null;
  slot_key: string | null;
  display_name: string | null;
  strengthenable: boolean | null;
  oil_allowed: boolean | null;
  initial_luck_max: number | null;
}

export interface GameQualities {
  id: number | null;
  quality_key: string | null;
  display_name: string | null;
  affix_slots: number | null;
  random_drop_allowed: boolean | null;
  unbound_market_allowed: boolean | null;
}

export interface GameStages {
  id: number | null;
  stage_key: string | null;
  required_level_min: number | null;
  required_level_max: number | null;
  formula: string | null;
}

export interface GameProductRules {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameWarehouseRules {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameBoundResourceUi {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameCardProducts {
  id: number | null;
  card_key: string | null;
  payment_key: string | null;
  duration_days: number | null;
  cash_fen: number | null;
  ingot_u: number | null;
  role_scoped: boolean | null;
}

export interface GameEntitlementRules {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameItemMarketRules {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameGoldMarketRules {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameDropCategories {
  id: number | null;
  category_key: string | null;
  prob_ppm: number | null;
  exclusive_group: string | null;
}

export interface GameDropGuarantees {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameAffixes {
  id: number | null;
  attr_key: string | null;
  display_name: string | null;
  minimum_quality: number | null;
  value_curve_key: string | null;
}

export interface GameRefineQuality {
  id: number | null;
  quality: number | null;
  slot_count: number | null;
  pool_size: number | null;
  stones_per_attempt: number | null;
  allow_duplicate_attr: boolean | null;
}

export interface GameRefineTiers {
  id: number | null;
  tier: number | null;
  multiplier_bp: number | null;
  weight_ppm: number | null;
}

export interface GameRefineLockCosts {
  id: number | null;
  locked_slots: number | null;
  extra_ingot_u_per_attempt: number | null;
}

export interface GameEnhancementLevels {
  id: number | null;
  from_level: number | null;
  to_level: number | null;
  stage: number | null;
  success_ppm: number | null;
  stone_cost: number | null;
  base_attr_increment_bp: number | null;
}

export interface GameBodyTracks {
  id: number | null;
  track_key: string | null;
  display_name: string | null;
  currency_family: string | null;
  cost_base_u: number | null;
  budget_group: string | null;
  attr_curve_key: string | null;
}

export interface GameBodyLevelSteps {
  id: number | null;
  from_level: number | null;
  to_level: number | null;
  cost_multiplier: number | null;
  success_ppm: number | null;
}

export interface GameLuckInitial {
  id: number | null;
  source_key: string | null;
  trigger_ppm: number | null;
  initial_value_weights: unknown | null;
  fixed_initial_value: number | null;
}

export interface GameLuckOilSteps {
  id: number | null;
  from_luck: number | null;
  target_luck: number | null;
  success_ppm: number | null;
  failure_luck: number | null;
  pity_segment: string | null;
  threshold_used_bottles: number | null;
  pity_starts_after_threshold: boolean | null;
}

export interface GameLuckContribution {
  id: number | null;
  source_group: string | null;
  luck: number | null;
  contribution_bp: number | null;
}

export interface GameLuckSources {
  id: number | null;
  source_key: string | null;
  maximum_luck: number | null;
  oil_allowed: boolean | null;
  contribution_group: string | null;
  activation_requirement: string | null;
}

export interface GameLuckRules {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameOreTypes {
  id: number | null;
  ore_key: string | null;
  display_name: string | null;
  layer_1_weight: number | null;
  layer_2_weight: number | null;
  price_gold_numerator: number | null;
  price_gold_denominator: number | null;
  integer_rounding_rule: string | null;
  craft_material: boolean | null;
}

export interface GameMineLayers {
  id: number | null;
  layer: number | null;
  minimum_level: number | null;
  map_config_id: number | null;
}

export interface GameOrePurity {
  id: number | null;
  purity_min: number | null;
  purity_max: number | null;
  weight_ppm: number | null;
  within_band: string | null;
}

export interface GameWorkshopRules {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameLevelBandEconomy {
  id: number | null;
  level_min: number | null;
  level_max: number | null;
  g_ref_gold_per_hour: number | null;
  hosting_gold_per_hour: number | null;
  refine_gold_per_attempt: number | null;
  rare_craft_gold: number | null;
  epic_craft_gold: number | null;
  legendary_craft_gold: number | null;
}

export interface GameBookRecycle {
  id: number | null;
  book_quality: number | null;
  pages: number | null;
  npc_basic_book_pages: number | null;
}

export interface GameShopPrices {
  id: number | null;
  product_key: string | null;
  display_name: string | null;
  payment_currency: string | null;
  price_u: number | null;
  quantity: number | null;
  purchased_bound: boolean | null;
  daily_limit: number | null;
}

export interface GameChallengeRules {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameDungeonUnlocks {
  id: number | null;
  required_level: number | null;
  dungeon_template_id: number | null;
}

export interface GameArenaRules {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameArenaStreakPoints {
  id: number | null;
  streak_wins: number | null;
  win_points: number | null;
}

export interface GameProgressionRules {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameSoulTypes {
  id: number | null;
  soul_key: string | null;
  display_name: string | null;
  max_star: number | null;
  max_owned: number | null;
  tradeable_record: boolean | null;
}

export interface GameSoulAffixes {
  id: number | null;
  pool_key: string | null;
  attr_key: string | null;
  display_name: string | null;
  unit_definition: string | null;
  star_values: unknown | null;
  trigger_and_target: unknown | null;
}

export interface GameCollectionRules {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameWingRanks {
  id: number | null;
  rank: number | null;
  feather_cost: number | null;
  attribute_effects: unknown | null;
  appearance_evolves: boolean | null;
}

export interface GameBattlepassTiers {
  id: number | null;
  tier_key: string | null;
  duration_days: number | null;
  cash_fen: number | null;
  cash_only: boolean | null;
  daily_total_not_stack: boolean | null;
}

export interface GameBattlepassRewards {
  id: number | null;
  tier_key: string | null;
  period_key: string | null;
  reward_key: string | null;
  quantity_u: number | null;
  bound: boolean | null;
}

export interface GameCommercialCandidates {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameAttributeBudgets {
  id: number | null;
  budget_key: string | null;
  limit_bp: number | null;
  constraint_kind: string | null;
  stacking: string | null;
}

export interface GameClientPresentation {
  id: number | null;
  rule_key: string | null;
  rule_value: unknown | null;
}

export interface GameJobGrowth {
  id: number | null;
  class_id: number | null;
  level: number | null;
  hp: number | null;
  mp: number | null;
  attack_min: number | null;
  attack_max: number | null;
  magic_min: number | null;
  magic_max: number | null;
  tao_min: number | null;
  tao_max: number | null;
  defence_min: number | null;
  defence_max: number | null;
  magic_defence_min: number | null;
  magic_defence_max: number | null;
  accuracy: number | null;
  agility: number | null;
  weight_limits: unknown | null;
}

export interface GameItems {
  id: number | null;
  item_key: string | null;
  display_name: string | null;
  item_type: string | null;
  max_stack: number | null;
  effect_list_id: number | null;
  weight: number | null;
  quality: number | null;
  description: string | null;
  icon_resource_key: string | null;
}

export interface GameEquipment {
  id: number | null;
  item_id: number | null;
  equipment_type: string | null;
  class_restriction: unknown | null;
  required_level: number | null;
  quality: number | null;
  stage: number | null;
  base_attributes: unknown | null;
  original_durability_u: number | null;
  weight: number | null;
  craft_bonus_strengthenable: boolean | null;
}

export interface GameAppearance {
  id: number | null;
  item_id: number | null;
  class_id: number | null;
  appearance_key: string | null;
  body_resource_key: string | null;
  weapon_resource_key: string | null;
  anchor_and_layers: unknown | null;
  action_map: unknown | null;
}

export interface GameMonsters {
  id: number | null;
  monster_key: string | null;
  display_name: string | null;
  level: number | null;
  category: string | null;
  hp: number | null;
  attack_interval_ms: number | null;
  move_interval_ms: number | null;
  exp: number | null;
  resource_key: string | null;
}

export interface GameMaps {
  id: number | null;
  map_key: string | null;
  display_name: string | null;
  scene_kind: string | null;
  width_cells: number | null;
  height_cells: number | null;
  required_level: number | null;
  collision_id: number | null;
  spawn_cell: unknown | null;
  tile_resource_key: string | null;
  map_version: string | null;
}

export interface GameMapCollisions {
  id: number | null;
  map_key: string | null;
  width_cells: number | null;
  height_cells: number | null;
  walkable_encoding: string | null;
  client_navigation: unknown | null;
  diagonal_rule: string | null;
}

export interface GameMapRegions {
  id: number | null;
  map_id: number | null;
  region_key: string | null;
  region_cells: unknown | null;
  npc_id: number | null;
  monster_pool: unknown | null;
  entry_rules: unknown | null;
}

export interface GameMonsterSpawns {
  id: number | null;
  map_id: number | null;
  slot_index: number | null;
  monster_id: number | null;
  cell: unknown | null;
  respawn_seconds: number | null;
  spawn_kind: string | null;
}

export interface GameNpcs {
  id: number | null;
  npc_key: string | null;
  display_name: string | null;
  map_id: number | null;
  cell: unknown | null;
  function_keys: unknown | null;
  resource_key: string | null;
}

export interface GameSkills {
  id: number | null;
  skill_key: string | null;
  display_name: string | null;
  class_id: number | null;
  learn_level: number | null;
  behavior_id: string | null;
  target_rule: unknown | null;
  range_pattern_id: number | null;
  cooldown_ms: number | null;
  cast_timing: unknown | null;
  resource_costs: unknown | null;
  damage_or_heal_path: string | null;
  effect_ids: unknown | null;
  action_resource_key: string | null;
}

export interface GameSkillUpgrades {
  id: number | null;
  skill_id: number | null;
  level: number | null;
  book_cost: unknown | null;
  page_cost: number | null;
  allowed_numeric_changes: unknown | null;
  preserve_behavior_shape: boolean | null;
}

export interface GameRangePatterns {
  id: number | null;
  pattern_key: string | null;
  relative_cells: unknown | null;
  direction_rotation: unknown | null;
  obstacle_policy: string | null;
  max_targets: number | null;
  line_of_sight: string | null;
}

export interface GameBuffs {
  id: number | null;
  buff_key: string | null;
  attribute_effects: unknown | null;
  duration_ms: number | null;
  tick_ms: number | null;
  stack_policy: string | null;
  remove_events: unknown | null;
  immunity_rules: unknown | null;
  budget_group: string | null;
}

export interface GameEffects {
  id: number | null;
  effect_key: string | null;
  effect_type: string | null;
  parameters: unknown | null;
  trigger_phase: string | null;
  target_rule: unknown | null;
  proc_limit: unknown | null;
  recursive_trigger_allowed: boolean | null;
}

export interface GamePetTemplates {
  id: number | null;
  pet_key: string | null;
  summon_skill_id: number | null;
  monster_template_id: number | null;
  base_attributes: unknown | null;
  attack_behavior: string | null;
  follow_and_return: unknown | null;
  lifetime_and_betrayal: unknown | null;
  resource_key: string | null;
}

export interface GamePetGrowth {
  id: number | null;
  pet_id: number | null;
  growth_level: number | null;
  attributes: unknown | null;
  required_conditions: unknown | null;
}

export interface GameExpLevels {
  id: number | null;
  level: number | null;
  next_level_exp: number | null;
  growth_target: string | null;
  measurement_version: string | null;
}

export interface GameRebirthCosts {
  id: number | null;
  rebirth_index: number | null;
  required_level: number | null;
  next_level_cap: number | null;
  challenge_token_id: number | null;
  gold_cost: number | null;
  bound_material_costs: unknown | null;
  bound_ingot_acceleration_cap: unknown | null;
}

export interface GameDropPools {
  id: number | null;
  pool_key: string | null;
  source_kind: string | null;
  category_key: string | null;
  equipment_stage_range: unknown | null;
  max_random_quality: number | null;
}

export interface GameDropEntries {
  id: number | null;
  pool_id: number | null;
  item_id: number | null;
  currency_key: string | null;
}

export interface GameRecycleValues {
  id: number | null;
  equipment_stage: number | null;
  quality: number | null;
  gold_distribution: unknown | null;
  broken_gold: number | null;
  stone_probability_ppm: number | null;
  broken_byproducts: unknown | null;
  oil_probability_formula: string | null;
  rewards_combinable: unknown | null;
}

export interface GameRefineValues {
  id: number | null;
  affix_id: number | null;
  equipment_stage: number | null;
  base_value_fixed: number | null;
  value_scale: number | null;
  stacking_group: string | null;
  per_item_cap: number | null;
  whole_role_cap: number | null;
}

export interface GamePickaxes {
  id: number | null;
  tool_key: string | null;
  required_layer: number | null;
  gold_price: number | null;
  durability_u: number | null;
  loss_per_attempt: number | null;
}

export interface GameCraftRecipes {
  id: number | null;
  blueprint_item_id: number | null;
  output_item_id: number | null;
  quality: number | null;
  required_level: number | null;
  equipment_type: string | null;
  ore_count: number | null;
  gold_cost: number | null;
  success_rule: string | null;
  failure_losses: unknown | null;
  extra_base_attribute: unknown | null;
}

export interface GameRepairPrices {
  id: number | null;
  repair_key: string | null;
  equipment_scope: unknown | null;
  currency_family: string | null;
  cost_formula: string | null;
  upper_durability_loss_bp: number | null;
  repair_amount_rule: string | null;
  material_costs: unknown | null;
}

export interface GameBossTemplates {
  id: number | null;
  boss_kind: string | null;
  monster_id: number | null;
  map_id: number | null;
  required_level: number | null;
  cooldown_seconds: number | null;
  failure_cooldown_seconds: number | null;
  rank_reward_group_id: number | null;
  participation_reward_group_id: number | null;
}

export interface GameDungeonTemplates {
  id: number | null;
  dungeon_key: string | null;
  required_level: number | null;
  segments: unknown | null;
  time_limit_sec: number | null;
  ai_hire_options: unknown | null;
  reward_group_id: number | null;
  disconnect_policy: string | null;
}

export interface GameArenaRewards {
  id: number | null;
  reward_kind: string | null;
  rank_min: number | null;
  rank_max: number | null;
  required_participation: number | null;
  exchange_point_cost: number | null;
  reward_group_id: number | null;
}

export interface GameQuests {
  id: number | null;
  quest_key: string | null;
  chapter: number | null;
  reward_group_id: number | null;
  next_quest_id: number | null;
}

export interface GameDailyTasks {
  id: number | null;
  task_key: string | null;
  objective_event: string | null;
  target_count: number | null;
  activity_points: number | null;
  reward_group_id: number | null;
  unlock_conditions: unknown | null;
}

export interface GameAchievements {
  id: number | null;
  achievement_key: string | null;
  required_progress: number | null;
  reward_group_id: number | null;
}

export interface GameCodex {
  id: number | null;
  monster_id: number | null;
  map_group_id: number | null;
  required_kills: number | null;
  qualification_rule: string | null;
  reward_group_id: number | null;
  budgeted_attributes: unknown | null;
}

export interface GameSoulStars {
  id: number | null;
  soul_type_id: number | null;
  star: number | null;
  decompose_value: number | null;
  upgrade_fragment_cost: number | null;
  synthesis_cost: number | null;
  duplicate_fragment_value: number | null;
  attribute_values: unknown | null;
}

export interface GameMedalLevels {
  id: number | null;
  level: number | null;
  fragment_cost: number | null;
  class_attributes: unknown | null;
  convenience_effects: unknown | null;
}

export interface GameRewardGroups {
  id: number | null;
  reward_group_key: string | null;
  source_kind: string | null;
  binding_policy: string | null;
}

export interface GameRewardEntries {
  id: number | null;
  reward_group_id: number | null;
  asset_key: string | null;
}

export interface GameSystemMail {
  id: number | null;
  template_key: string | null;
  title: string | null;
  body: string | null;
  reward_group_id: number | null;
}

export interface GameAttributePipeline {
  id: number | null;
  attr_key: string | null;
  unit: string | null;
  fixed_scale: number | null;
  source_scope: string | null;
  target_scope: string | null;
  calculation_phase: string | null;
  stacking_group: string | null;
  stacking_mode: string | null;
  cap_rule: unknown | null;
  rounding_phase: string | null;
  base_or_extension: string | null;
}

