#!/usr/bin/env bash
# Backup harian database rekapin, simpan 7 hari.
# Cron (user postgres):  15 2 * * * /opt/rekapin/backup.sh
set -euo pipefail
DIR=/var/backups/rekapin
mkdir -p "$DIR"
pg_dump -Fc rekapin > "$DIR/rekapin-$(date +%F).dump"
find "$DIR" -name 'rekapin-*.dump' -mtime +7 -delete
