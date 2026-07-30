#!/bin/bash
# Источнится из run.sh. Задаёт переменные для создания сетей и NAT.

SCENARIO_ID=""
NAT_A_ACTION=""
NAT_B_ACTION=""

apply_nat() {
    local net_a="$1" net_b="$2"

    if [ -n "$NAT_A_ACTION" ]; then
        log_info "Applying NAT to $net_a (action: $NAT_A_ACTION)"
        iptables -A FORWARD -i "$net_a" -j DROP 2>/dev/null || true
    fi

    if [ -n "$NAT_B_ACTION" ]; then
        log_info "Applying NAT to $net_b (action: $NAT_B_ACTION)"
        iptables -A FORWARD -i "$net_b" -j DROP 2>/dev/null || true
    fi
}
