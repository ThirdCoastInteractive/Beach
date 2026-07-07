-- Creates the per-app databases for the repo-root compose stack, where one
-- Postgres serves every example app. The image's POSTGRES_DB creates "pantry";
-- this script (mounted into /docker-entrypoint-initdb.d) creates the rest.
--
-- Init scripts run only when the pgdata volume is FIRST initialized: an
-- existing stack picks these up after `docker compose down -v` (drops data) or
-- by creating the databases by hand.
CREATE DATABASE boardwalk OWNER beach;
CREATE DATABASE driftbottle OWNER beach;
CREATE DATABASE booking OWNER beach;
