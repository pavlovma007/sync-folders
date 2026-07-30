#!/bin/bash
# Оркестратор Docker-интеграционных тестов.
# Использование: ./run.sh <scenario-name> [--keep]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCENARIO="${1:?Usage: run.sh <scenario-name> [--keep]}"
KEEP="${2:-}"
SCENARIO_DIR="$SCRIPT_DIR/scenarios/$SCENARIO"

if [ ! -d "$SCENARIO_DIR" ]; then
    echo "Scenario not found: $SCENARIO"
    echo "Available scenarios:"
    ls "$SCRIPT_DIR/scenarios/"
    exit 1
fi

source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/topology.sh"
source "$SCENARIO_DIR/topology.sh"

PREFIX="sync-test-${SCENARIO}"
NET_A="${PREFIX}-a"
NET_B="${PREFIX}-b"
VOL="${PREFIX}-vol"

log_info "Starting scenario: $SCENARIO"

cleanup() {
    log_info "Cleaning up..."
    docker rm -f "${PREFIX}-a" "${PREFIX}-b" 2>/dev/null || true
    docker network rm "$NET_A" "$NET_B" 2>/dev/null || true
    docker volume rm "$VOL" 2>/dev/null || true
}

if [ -z "$KEEP" ]; then
    trap cleanup EXIT
fi

docker volume create "$VOL" >/dev/null
docker network create --subnet "10.${SCENARIO_ID}.1.0/24" "$NET_A" >/dev/null
docker network create --subnet "10.${SCENARIO_ID}.2.0/24" "$NET_B" >/dev/null

if ! docker image inspect sync-folders-test >/dev/null 2>&1; then
    log_info "Building image..."
    docker build -t sync-folders-test -f "$SCRIPT_DIR/Dockerfile" "$SCRIPT_DIR/.."
fi

run_peer() {
    local role="$1" net="$2"
    docker run -d --name "${PREFIX}-${role}" \
        --net "$net" \
        -e ROLE="$role" \
        -e SCENARIO="$SCENARIO" \
        -v "$SCENARIO_DIR:/scenario:ro" \
        -v "$SCRIPT_DIR/lib:/opt/sync-test/lib:ro" \
        -v "$VOL:/shared" \
        sync-folders-test \
        /bin/bash /scenario/test.sh
}

run_peer "a" "$NET_A"
run_peer "b" "$NET_B"

apply_nat "$NET_A" "$NET_B"

EXIT_CODE=0
for role in a b; do
    container="${PREFIX}-${role}"
    log_info "Waiting for $container..."
    if ! timeout 300 bash -c "docker wait '$container' >/dev/null 2>&1"; then
        log_fail "$container timeout"
        EXIT_CODE=1
    fi
    exit_code=$(docker inspect "$container" -f '{{.State.ExitCode}}')
    if [ "$exit_code" -ne 0 ]; then
        echo "--- $container output ---"
        docker logs "$container"
        echo "--- end ---"
        log_fail "$container failed with exit code $exit_code"
        EXIT_CODE=1
    else
        log_pass "$container completed"
    fi
done

if [ "$EXIT_CODE" -eq 0 ]; then
    log_pass "Scenario $SCENARIO PASSED"
else
    log_fail "Scenario $SCENARIO FAILED"
fi
