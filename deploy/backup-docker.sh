#!/usr/bin/env bash
# Backup harian database rekapin (mode Docker), simpan 7 hari.
# Cron (root):  15 2 * * * /path/ke/rekapin/deploy/backup-docker.sh
set -euo pipefail
cd "$(dirname "$0")/.."
DIR=${BACKUP_DIR:-./backups}
mkdir -p "$DIR"
# Kredensial diambil dari env container db (tidak perlu source .env.prod).
docker compose -f compose.prod.yaml --env-file .env.prod exec -T db \
  sh -c 'pg_dump -Fc -U "$POSTGRES_USER" "$POSTGRES_DB"' \
  > "$DIR/rekapin-$(date +%F).dump"
find "$DIR" -name 'rekapin-*.dump' -mtime +7 -delete
