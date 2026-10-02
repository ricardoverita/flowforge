#!/bin/sh
set -eu
umask 077
mkdir -p /db-secrets /nats-secrets /grafana-secrets /admin-secrets
create_secret() {
  secret_dir=$2
  if [ ! -s "$secret_dir/$1" ]; then
    head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n' > "$secret_dir/$1"
  fi
  chmod 0444 "$secret_dir/$1"
}
create_secret postgres_password /db-secrets
create_secret postgres_admin_password /admin-secrets
create_secret nats_token /nats-secrets
create_secret grafana_password /grafana-secrets
if [ ! -s /nats-secrets/nats_auth.conf ]; then
  printf 'token: "%s"\n' "$(cat /nats-secrets/nats_token)" > /nats-secrets/nats_auth.conf
fi
chmod 0444 /nats-secrets/nats_auth.conf
# Only this short-lived bootstrap container needs root to initialize named volumes.
chown 70:70 /postgres-data
chown 1000:1000 /nats-data
chown 472:472 /grafana-data
chown 65534:65534 /prometheus-data
