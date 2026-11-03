#!/bin/bash

# Setup iptables/ip6tables for an AmneziaWG interface
# Usage: setup_iptables.sh <interface_name> <subnet> [subnet_v6]
#
# subnet_v6 is the server's ULA /64. When it is given, the same rule set is
# written for IPv6 - including the NAT66 masquerade that lets clients reach the
# IPv6 internet through the host's own uplink, which is what "NAT6" means here.

INTERFACE=$1
SUBNET=$2
SUBNET6=$3

if [ -z "$INTERFACE" ] || [ -z "$SUBNET" ]; then
    echo "Usage: $0 <interface_name> <subnet> [subnet_v6]"
    echo "Example: $0 wg0 10.8.1.0/24 fd42:ab12::/64"
    exit 1
fi

echo "Setting up iptables for interface $INTERFACE with subnet $SUBNET"

# Remove any existing rules for this interface to avoid duplicates
iptables -D INPUT -i $INTERFACE -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -i $INTERFACE -j ACCEPT 2>/dev/null || true
iptables -D OUTPUT -o $INTERFACE -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -i $INTERFACE -o eth+ -s $SUBNET -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -i $INTERFACE -o eth+ -s $SUBNET -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
iptables -t nat -D POSTROUTING -s $SUBNET -o eth+ -j MASQUERADE 2>/dev/null || true

# Allow traffic on the TUN interface
iptables -A INPUT -i $INTERFACE -j ACCEPT
iptables -A FORWARD -i $INTERFACE -j ACCEPT
iptables -A OUTPUT -o $INTERFACE -j ACCEPT

# Allow forwarding traffic only from the VPN
# Match any eth* interface
iptables -A FORWARD -i $INTERFACE -o eth+ -s $SUBNET -j ACCEPT

# Allow established and related connections
iptables -A FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT

# Enable NAT for VPN traffic
iptables -t nat -A POSTROUTING -s $SUBNET -o eth+ -j MASQUERADE

if [ $? -ne 0 ]; then
    echo "iptables set up failed for $INTERFACE"
    exit 1
fi

# The IPv6 half. A host without ip6tables, or one built without the nat table,
# keeps working: the tunnel itself stays up, only NAT6 is missing - which is
# what the panel's iptables check reports.
if [ -n "$SUBNET6" ]; then
    if ! command -v ip6tables >/dev/null 2>&1; then
        echo "ip6tables not found - skipping the IPv6 rules for $INTERFACE"
        exit 0
    fi

    echo "Setting up ip6tables for interface $INTERFACE with subnet $SUBNET6"

    ip6tables -D INPUT -i $INTERFACE -j ACCEPT 2>/dev/null || true
    ip6tables -D FORWARD -i $INTERFACE -j ACCEPT 2>/dev/null || true
    ip6tables -D OUTPUT -o $INTERFACE -j ACCEPT 2>/dev/null || true
    ip6tables -D FORWARD -i $INTERFACE -o eth+ -s $SUBNET6 -j ACCEPT 2>/dev/null || true
    ip6tables -t nat -D POSTROUTING -s $SUBNET6 -o eth+ -j MASQUERADE 2>/dev/null || true

    ip6tables -A INPUT -i $INTERFACE -j ACCEPT
    ip6tables -A FORWARD -i $INTERFACE -j ACCEPT
    ip6tables -A OUTPUT -o $INTERFACE -j ACCEPT
    ip6tables -A FORWARD -i $INTERFACE -o eth+ -s $SUBNET6 -j ACCEPT
    ip6tables -A FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT

    if ! ip6tables -t nat -A POSTROUTING -s $SUBNET6 -o eth+ -j MASQUERADE; then
        echo "ip6tables NAT66 setup failed for $INTERFACE (is the ip6table_nat module available?)"
        exit 1
    fi
fi

echo "iptables rules set up successfully for $INTERFACE"
