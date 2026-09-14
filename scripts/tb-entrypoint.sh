#!/bin/sh
set -e

DATA_FILE="/data/0_0.tigerbeetle"
CLUSTER_ID=0
REPLICA=0
REPLICA_COUNT=1
TB_BIN="/tigerbeetle"

if [ ! -f "$DATA_FILE" ]; then
    echo "[tb-entrypoint] Datafile no existe. Formateando..."
    "$TB_BIN" format \
        --cluster="$CLUSTER_ID" \
        --replica="$REPLICA" \
        --replica-count="$REPLICA_COUNT" \
        --development \
        "$DATA_FILE"
    echo "[tb-entrypoint] Formateo completado."
else
    echo "[tb-entrypoint] Datafile ya existe. Saltando format."
fi

echo "[tb-entrypoint] Iniciando TigerBeetle en 0.0.0.0:3000..."
exec "$TB_BIN" start \
    --addresses=0.0.0.0:3000 \
    --development \
    "$DATA_FILE"