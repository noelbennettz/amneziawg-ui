#!/bin/bash

# Cleanup iptables/ip6tables for an AmneziaWG interface
# Usage: cleanup_iptables.sh <interface_name> <subnet> [subnet_v6]

INTERFACE=$1
SUBNET=$2
SUBNET6=$3

if [ -z "$INTERFACE" ] || [ -z "$SUBNET" ]; then
    echo "Usage: $0 <interface_name> <subnet> [subnet_v6]"
    echo "Example: $0 wg0 10.8.1.0/24 fd42:ab12::/64"
    exit 1
fi

echo "Cleaning up iptables for interface $INTERFACE with subnet $SUBNET"

# Remove rules in reverse order
iptables -t nat -D POSTROUTING -s $SUBNET -o eth+ -j MASQUERADE 2>/dev/null || true
iptables -D FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -i $INTERFACE -o eth+ -s $SUBNET -j ACCEPT 2>/dev/null || true
iptables -D OUTPUT -o $INTERFACE -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -i $INTERFACE -j ACCEPT 2>/dev/null || true
iptables -D INPUT -i $INTERFACE -j ACCEPT 2>/dev/null || true

# The IPv6 rules SetupIPTables added for the same interface, if it added any.
if [ -n "$SUBNET6" ] && command -v ip6tables >/dev/null 2>&1; then
    ip6tables -t nat -D POSTROUTING -s $SUBNET6 -o eth+ -j MASQUERADE 2>/dev/null || true
    ip6tables -D FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
    ip6tables -D FORWARD -i $INTERFACE -o eth+ -s $SUBNET6 -j ACCEPT 2>/dev/null || true
    ip6tables -D OUTPUT -o $INTERFACE -j ACCEPT 2>/dev/null || true
    ip6tables -D FORWARD -i $INTERFACE -j ACCEPT 2>/dev/null || true
    ip6tables -D INPUT -i $INTERFACE -j ACCEPT 2>/dev/null || true
fi

echo "iptables rules cleaned up successfully for $INTERFACE"
