DROP INDEX IF EXISTS idx_env_app_id;
DROP INDEX IF EXISTS idx_apps_server_id;
DROP INDEX IF EXISTS idx_unique_apps_server_id_repo_name;
DROP TABLE IF EXISTS environment_variables CASCADE;
DROP TABLE IF EXISTS applications CASCADE;
