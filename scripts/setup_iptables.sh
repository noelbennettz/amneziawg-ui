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

# The interface a family's traffic leaves through, read off the routing table.
# "eth+" was what the container's bridge uplink happened to be called: on a
# host-network deployment, on an uplink with another name, or in a container
# whose interface is not eth0, no rule matched and the NAT silently never
# applied - a server whose clients reach the tunnel and nothing beyond it.
wan_interface() {
    if [ "$1" = 6 ]; then
        ip -6 route show default 2>/dev/null
    else
        ip route show default 2>/dev/null
    fi | awk '{for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit }}'
}

# has_public_v6 reports whether an interface carries an address outside
# fc00::/7 and fe80::/10 - what a masquerade rule needs as a source. "scope
# global" alone is not the test: a unique local address is global scope too.
has_public_v6() {
    ip -6 -o addr show dev "$1" scope global 2>/dev/null \
        | awk '{print $4}' | cut -d/ -f1 \
        | grep -qE '^[23][0-9a-fA-F]{3}:'
}

WAN4=$(wan_interface 4)
if [ -z "$WAN4" ]; then
    echo "no IPv4 default route - NAT4 has nothing to masquerade onto" >&2
    WAN4="eth+"
fi
WAN6=$(wan_interface 6)
if [ -n "$SUBNET6" ]; then
    if [ -z "$WAN6" ]; then
        # The rules are written anyway, so a host that gains IPv6 later is
        # covered, but the operator should know why v6 traffic goes nowhere now.
        echo "no IPv6 default route - NAT66 has nothing to masquerade onto, so IPv6 clients get the tunnel but not the IPv6 internet" >&2
    elif ! has_public_v6 "$WAN6"; then
        # A default route is not enough: MASQUERADE takes its source from the
        # outgoing interface, and a ULA or link-local address there is not
        # routable. A Docker network with enable_ipv6 and fd00::/64 looks
        # exactly like this.
        echo "$WAN6 has no global IPv6 address (2000::/3) - NAT66 has nothing routable to masquerade onto" >&2
    fi
fi
[ -n "$WAN6" ] || WAN6="eth+"

# Remove any existing rules for this interface to avoid duplicates
iptables -D INPUT -i $INTERFACE -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -i $INTERFACE -j ACCEPT 2>/dev/null || true
iptables -D OUTPUT -o $INTERFACE -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -i $INTERFACE -o $WAN4 -s $SUBNET -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
iptables -t nat -D POSTROUTING -s $SUBNET -o $WAN4 -j MASQUERADE 2>/dev/null || true

# Allow traffic on the TUN interface
iptables -A INPUT -i $INTERFACE -j ACCEPT
iptables -A FORWARD -i $INTERFACE -j ACCEPT
iptables -A OUTPUT -o $INTERFACE -j ACCEPT

# Allow forwarding traffic only from the VPN
# Match the uplink the routing table named above
iptables -A FORWARD -i $INTERFACE -o $WAN4 -s $SUBNET -j ACCEPT

# Allow established and related connections
iptables -A FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT

# Enable NAT for VPN traffic
iptables -t nat -A POSTROUTING -s $SUBNET -o $WAN4 -j MASQUERADE

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
    ip6tables -D FORWARD -i $INTERFACE -o $WAN6 -s $SUBNET6 -j ACCEPT 2>/dev/null || true
    ip6tables -t nat -D POSTROUTING -s $SUBNET6 -o $WAN6 -j MASQUERADE 2>/dev/null || true

    ip6tables -A INPUT -i $INTERFACE -j ACCEPT
    ip6tables -A FORWARD -i $INTERFACE -j ACCEPT
    ip6tables -A OUTPUT -o $INTERFACE -j ACCEPT
    ip6tables -A FORWARD -i $INTERFACE -o $WAN6 -s $SUBNET6 -j ACCEPT
    ip6tables -A FORWARD -m state --state ESTABLISHED,RELATED -j ACCEPT

    if ! ip6tables -t nat -A POSTROUTING -s $SUBNET6 -o $WAN6 -j MASQUERADE; then
        echo "ip6tables NAT66 setup failed for $INTERFACE (is the ip6table_nat module available?)"
        exit 1
    fi
fi

echo "iptables rules set up successfully for $INTERFACE"
