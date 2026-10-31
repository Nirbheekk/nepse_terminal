-- 0002_reference_data: roles and permissions.
--
-- These rows are schema, not content: every account needs the `user` role to
-- exist, and the entitlement layer reads `role_permissions` when deciding what a
-- caller may retrieve. Shipping them as a migration means a fresh database is
-- usable immediately and every environment has the same role vocabulary.

INSERT INTO roles (code, name) VALUES
    ('user',    'Retail user'),
    ('analyst', 'Research analyst'),
    ('admin',   'Administrator')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name;

INSERT INTO permissions (code, description) VALUES
    ('market:read',       'Read market summaries, quotes and indices'),
    ('floorsheet:read',   'Read the trade-level floorsheet'),
    ('company:read',      'Read company profiles and price history'),
    ('export:write',      'Export datasets (requires a data licence)'),
    ('watchlist:write',   'Create and modify personal watchlists'),
    ('admin:manage',      'Manage users, jobs and platform settings')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description;

-- Retail users get read access to market data only. Export stays unassigned by
-- default: redistribution is prohibited by the current data source licence.
INSERT INTO role_permissions (role_code, permission) VALUES
    ('user',    'market:read'),
    ('user',    'floorsheet:read'),
    ('user',    'company:read'),
    ('user',    'watchlist:write'),
    ('analyst', 'market:read'),
    ('analyst', 'floorsheet:read'),
    ('analyst', 'company:read'),
    ('analyst', 'watchlist:write'),
    ('admin',   'market:read'),
    ('admin',   'floorsheet:read'),
    ('admin',   'company:read'),
    ('admin',   'watchlist:write'),
    ('admin',   'admin:manage')
ON CONFLICT (role_code, permission) DO NOTHING;
