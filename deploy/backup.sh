#!/bin/sh
set -eu
umask 077
exec 9>/run/lock/rhythm-backup.lock
flock -n 9 || exit 0
install -d -m 700 /var/backups/rhythm
cd /opt/rhythm/current
stamp=$(date -u +%Y%m%dT%H%M%SZ)
dump=/var/backups/rhythm/postgres-$stamp.dump
docker compose --env-file /etc/rhythm/production.env -f deploy/compose.production.yaml exec -T postgres sh -c 'PGPASSWORD="$POSTGRES_PASSWORD" pg_dump -h 127.0.0.1 -Fc -U "$POSTGRES_USER" -d "$POSTGRES_DB"' > "$dump.partial"
test -s "$dump.partial"
mv "$dump.partial" "$dump"
tar -czf "/var/backups/rhythm/config-$stamp.tar.gz" -C / etc/rhythm/production.env etc/nginx/conf.d/rhythm.conf etc/nginx/snippets/rhythm-proxy.conf etc/letsencrypt
find /var/backups/rhythm -maxdepth 1 -type f \( -name 'postgres-*.dump' -o -name 'config-*.tar.gz' \) -mtime +14 -delete
echo "Backup complete: $dump"
