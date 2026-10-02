#!/bin/sh
set -eu
# The runtime role owns only its database and schema; it cannot administer the cluster.
psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set=ON_ERROR_STOP=1 \
  --set=app_password="$(cat /run/secrets/postgres_password)" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %L', 'flowforge', :'app_password') \gexec
CREATE DATABASE flowforge OWNER flowforge;
SQL
