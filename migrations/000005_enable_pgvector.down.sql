-- Do not drop the vector extension in production
-- as other tables might depend on it
-- DROP EXTENSION IF EXISTS vector;
SELECT 1; -- no-op down migration