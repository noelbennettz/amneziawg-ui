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

# The same lookup setup_iptables.sh does, so the rules it wrote are the ones
# deleted here: an uplink read wrong would leave the NAT and forwarding rules
# behind for good.
wan_interface() {
    if [ "$1" = 6 ]; then
        ip -6 route show default 2>/dev/null
    else
        ip route show default 2>/dev/null
    fi | awk '{for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit }}'
}

WAN4=$(wan_interface 4)
[ -n "$WAN4" ] || WAN4="eth+"
WAN6=$(wan_interface 6)
[ -n "$WAN6" ] || WAN6="eth+"

# Every interface that could have been the uplink when the rules were written:
# the one the routing table names now, plus every interface that exists. The
# rules that name an uplink are scoped to this server's subnet, so trying them
# against each interface can only ever delete this server's own - and it means
# an uplink renamed, moved or re-plumbed between setup and cleanup does not
# leave a masquerade rule behind for good.
uplink_candidates() {
    printf '%s\n' "$1"
    ip -o link show 2>/dev/null | awk -F': ' '{print $2}' | sed 's/@.*//'
}

# Remove rules in reverse order
for wan in $(uplink_candidates "$WAN4"); do
    iptables -t nat -D POSTROUTING -s $SUBNET -o "$wan" -j MASQUERADE 2>/dev/null || true
    iptables -D FORWARD -i $INTERFACE -o "$wan" -s $SUBNET -j ACCEPT 2>/dev/null || true
done
iptables -D FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
iptables -D OUTPUT -o $INTERFACE -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -i $INTERFACE -j ACCEPT 2>/dev/null || true
iptables -D INPUT -i $INTERFACE -j ACCEPT 2>/dev/null || true

# The IPv6 rules SetupIPTables added for the same interface, if it added any.
if [ -n "$SUBNET6" ] && command -v ip6tables >/dev/null 2>&1; then
    for wan in $(uplink_candidates "$WAN6"); do
        ip6tables -t nat -D POSTROUTING -s $SUBNET6 -o "$wan" -j MASQUERADE 2>/dev/null || true
        ip6tables -D FORWARD -i $INTERFACE -o "$wan" -s $SUBNET6 -j ACCEPT 2>/dev/null || true
    done
    ip6tables -D FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
    ip6tables -D OUTPUT -o $INTERFACE -j ACCEPT 2>/dev/null || true
    ip6tables -D FORWARD -i $INTERFACE -j ACCEPT 2>/dev/null || true
    ip6tables -D INPUT -i $INTERFACE -j ACCEPT 2>/dev/null || true
fi

echo "iptables rules cleaned up successfully for $INTERFACE"
