#!/bin/bash
# Wrapper para execução agendada do backup.
# Garante que o volume "Dados" esteja montado em /run/media/lucas/Dados
# (mesmo ponto usado pelo Nautilus/udisks2) antes de rodar o backup.
set -uo pipefail

MOUNTPOINT="/run/media/lucas/Dados"
DEVICE="/dev/disk/by-uuid/01DCF4F5AAE8F170"   # partição NTFS "Dados" (nvme0n1p1)
PROJECT_DIR="/home/lucas/Projects/GO/database-backup"
LOG_DIR="$PROJECT_DIR/logs"

mkdir -p "$LOG_DIR"
LOG_FILE="$LOG_DIR/backup_$(date +%F).log"

log() {
    echo "[$(date '+%F %T')] $*" >> "$LOG_FILE"
}

volume_writable() {
    findmnt -rn "$MOUNTPOINT" >/dev/null && \
    touch "$MOUNTPOINT/.backup-write-test" 2>/dev/null && \
    rm -f "$MOUNTPOINT/.backup-write-test"
}

if ! findmnt -rn "$MOUNTPOINT" >/dev/null; then
    log "Volume Dados não está montado. Montando via udisksctl..."
    udisksctl mount -b "$DEVICE" >> "$LOG_FILE" 2>&1
fi

# Montado não basta: uma montagem em estado inválido (ex.: NTFS dirty)
# aceita leituras mas devolve I/O error em toda escrita. Testa escrita
# e, se falhar, tenta remontar uma vez.
if ! volume_writable; then
    log "Volume montado mas sem escrita funcional. Tentando remontar..."
    udisksctl unmount -b "$DEVICE" >> "$LOG_FILE" 2>&1
    sleep 2
    udisksctl mount -b "$DEVICE" >> "$LOG_FILE" 2>&1
fi

if ! volume_writable; then
    log "ERRO: volume em $MOUNTPOINT indisponível ou sem escrita. Backup abortado."
    log "Dica: feche janelas do Nautilus usando o volume e/ou verifique o NTFS (chkdsk no Windows / ntfsfix)."
    exit 1
fi

cd "$PROJECT_DIR" || exit 1

log "Iniciando backup..."
./backup-tool-linux >> "$LOG_FILE" 2>&1
STATUS=$?

if [ $STATUS -eq 0 ]; then
    log "Backup finalizado com sucesso."
else
    log "Backup terminou com erro (exit code $STATUS)."
fi

# Remove logs com mais de 30 dias
find "$LOG_DIR" -name "backup_*.log" -mtime +30 -delete

exit $STATUS
