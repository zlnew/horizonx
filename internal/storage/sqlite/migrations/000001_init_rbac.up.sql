-- 1. Roles (Master)
CREATE TABLE IF NOT EXISTS roles (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

-- 2. Permissions (Master)
CREATE TABLE IF NOT EXISTS permissions (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

-- 3. Role Has Permissions (Pivot)
CREATE TABLE IF NOT EXISTS role_has_permissions (
    role_id INTEGER NOT NULL,
    permission_id INTEGER NOT NULL,
    PRIMARY KEY (role_id, permission_id),
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE
);

-- 4. Users (Child of Role)
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    role_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME DEFAULT NULL,
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE RESTRICT
);

-- Seeding Default Role
INSERT OR IGNORE INTO roles (id, name) VALUES 
(1, 'owner'),
(2, 'admin'),
(3, 'viewer');
