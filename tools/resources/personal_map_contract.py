"""普通挂机图与历史17格地台的资源、碰撞布局契约。"""

LEGACY_RESOURCE_KEY = 'development/maps/first-map'
OPEN_RESOURCE_KEY = 'development/maps/personal-map'
LEGACY_COLLISION = 'authored_edge_decorations'
OPEN_COLLISION = 'open_personal_field'


def collision_basis(definition, collision):
    navigation = collision.get('client_navigation')
    if (not isinstance(navigation, dict) or set(navigation) != {'geometry_reference', 'collision_basis'}
            or navigation['geometry_reference'] != f'game_maps/{definition["id"]}.visual_geometry'):
        raise ValueError('客户端地图碰撞须引用唯一基础表的完整投影约定')
    basis = navigation['collision_basis']
    key = definition.get('tile_resource_key')
    if basis == OPEN_COLLISION and key == OPEN_RESOURCE_KEY:
        if definition.get('decorations') != [] or collision.get('blocked_cells') != []:
            raise ValueError('开阔挂机图须显式为空装饰、空运行阻挡')
    elif basis != LEGACY_COLLISION or key != LEGACY_RESOURCE_KEY:
        raise ValueError('挂机图资源键与碰撞约定不一致')
    return basis
