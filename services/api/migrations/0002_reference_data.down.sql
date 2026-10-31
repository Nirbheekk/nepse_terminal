-- Reverse 0002: remove the reference rows. Roles are referenced by user_roles
-- with ON DELETE CASCADE, so reversing this on a populated database detaches
-- every account from its role. That is the intended (and loud) behaviour of a
-- down-migration, not a safe production operation.
DELETE FROM role_permissions
 WHERE role_code IN ('user', 'analyst', 'admin');

DELETE FROM permissions
 WHERE code IN ('market:read', 'floorsheet:read', 'company:read',
                'export:write', 'watchlist:write', 'admin:manage');

DELETE FROM roles WHERE code IN ('user', 'analyst', 'admin');
