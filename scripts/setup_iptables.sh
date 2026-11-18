#!/bin/bash

# Setup iptables/ip6tables for an AmneziaWG interface.
#
# Usage: setup_iptables.sh <interface> <subnet> [subnet_v6]
#        setup_iptables.sh --status
#
# Two different jobs, decided separately.
#
# The interface rules - INPUT, FORWARD and OUTPUT for the tunnel device - are
# what the tunnel itself needs: peers reach the server, the server reaches the
# peers, and the panel answers on the tunnel address. They are written for
# every family the kernel has, whatever the uplink looks like.
#
# The forwarding and masquerade rules are only written for a family that has a
# default route and a source address on it. A masquerade onto an interface
# with no address rewrites the source to nothing and drops the packet where no
# counter is read; the client sees a tunnel that answers and forwards nothing,
# which is indistinguishable from a dead uplink.
#
# What was decided is printed on stdout as one line per family, so the panel
# can repeat it in its own words and API clients can read it:
#
#   NAT4 state=ok uplink=eth0 source=203.0.113.9
#   NAT6 state=ula-only uplink=eth0 source=fd00:dead:beef::2
#
# state is one of:
#   ok           a default route and a routable source: the masquerade works
#   ula-only     IPv6 has only a unique local address - a Docker bridge network
#                hands out exactly that - so the rule is written, but nothing
#                on the internet can answer it
#   no-route     no default route in this family: nothing to masquerade onto
#   no-address   the default route's interface has no address of that family
#   unavailable  ip6tables, or its nat table, is missing
#   failed       iptables rejected the rules
#
# --status prints those two lines and changes nothing; that is how the panel
# asks the same question without touching the rules.

INTERFACE=$1
SUBNET=$2
SUBNET6=$3
STATUS_ONLY=

if [ "$1" = "--status" ]; then
    STATUS_ONLY=1
    INTERFACE=
    SUBNET=
    SUBNET6=
fi

if [ -z "$STATUS_ONLY" ] && { [ -z "$INTERFACE" ] || [ -z "$SUBNET" ]; }; then
    echo "Usage: $0 <interface_name> <subnet> [subnet_v6]" >&2
    echo "       $0 --status" >&2
    echo "Example: $0 wg0 10.8.1.0/24 fd42:ab12::/64" >&2
    exit 1
fi

# The interface a family's traffic leaves through, read off the routing table.
# "eth+" was what the container's bridge uplink happened to be called: on a
# host-network deployment, on an uplink with another name, or in a container
# whose interface is not eth0, no rule matched and the NAT silently never
# applied.
wan_interface() {
    if [ "$1" = 6 ]; then
        ip -6 route show default 2>/dev/null
    else
        ip route show default 2>/dev/null
    fi | awk '{for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit }}'
}

# first_v4 / first_global_v6 / first_ula_v6 print the first address of that
# kind the device carries, without the prefix length.
first_v4() {
    ip -4 -o addr show dev "$1" scope global 2>/dev/null \
        | awk '{print $4}' | cut -d/ -f1 | grep -v '^$' | head -1
}

# Only 2000::/3 is routable on the internet. "scope global" is not the test: a
# unique local address has global scope too, and a Docker bridge network gives
# the container nothing else.
first_global_v6() {
    ip -6 -o addr show dev "$1" scope global 2>/dev/null \
        | awk '{print $4}' | cut -d/ -f1 \
        | grep -E '^[23][0-9a-fA-F]{3}:' | head -1
}

first_ula_v6() {
    ip -6 -o addr show dev "$1" scope global 2>/dev/null \
        | awk '{print $4}' | cut -d/ -f1 \
        | grep -E '^f[cd][0-9a-fA-F]{2}:' | head -1
}

# family_state prints "state=... uplink=... source=..." for one family.
family_state() {
    family=$1
    wan=$(wan_interface "$family")
    if [ -z "$wan" ]; then
        echo "state=no-route"
        return
    fi
    if [ "$family" = 4 ]; then
        source=$(first_v4 "$wan")
        if [ -z "$source" ]; then
            echo "state=no-address uplink=$wan"
        else
            echo "state=ok uplink=$wan source=$source"
        fi
        return
    fi
    source=$(first_global_v6 "$wan")
    if [ -n "$source" ]; then
        echo "state=ok uplink=$wan source=$source"
        return
    fi
    source=$(first_ula_v6 "$wan")
    if [ -n "$source" ]; then
        echo "state=ula-only uplink=$wan source=$source"
    else
        echo "state=no-address uplink=$wan"
    fi
}

STATE4=$(family_state 4)
STATE6=$(family_state 6)

# An IPv6 subnet the panel asked for, on a host with no ip6tables at all: the
# tunnel still comes up, NAT6 does not, and the state line is how the operator
# finds out.
if [ -n "$SUBNET6" ] && ! command -v ip6tables >/dev/null 2>&1; then
    STATE6="state=unavailable"
fi

if [ -n "$STATUS_ONLY" ]; then
    echo "NAT4 $STATE4"
    echo "NAT6 $STATE6"
    exit 0
fi

echo "Setting up iptables for interface $INTERFACE with subnet $SUBNET"
case "$STATE4" in
    state=ok*) ;;
    *) echo "IPv4: ${STATE4#state=} - no forwarding or NAT rules are written" >&2 ;;
esac
if [ -n "$SUBNET6" ]; then
    case "$STATE6" in
        state=ok*) ;;
        state=ula-only*)
            echo "IPv6: ${STATE6#state=} - the masquerade is written, but the source is not routable on the internet" >&2
            ;;
        *) echo "IPv6: ${STATE6#state=} - no forwarding or NAT rules are written" >&2 ;;
    esac
fi

WAN4=$(wan_interface 4)
[ -n "$WAN4" ] || WAN4="eth+"
WAN6=$(wan_interface 6)
[ -n "$WAN6" ] || WAN6="eth+"

# Every exit path repeats the decision, so the caller reads the same two lines
# whether the rules went in or not; the exit status says which it was.
report_and_exit() {
    echo "NAT4 $STATE4"
    echo "NAT6 $STATE6"
    exit "$1"
}

# One failed command is a setup that did not happen. The message names the
# command, because "iptables set up failed" leaves the operator guessing.
fail() {
    echo "iptables $* failed for $INTERFACE" >&2
    report_and_exit 1
}

add4() {
    iptables "$@" || fail "$@"
}

add6() {
    ip6tables "$@" || fail "$@"
}

if ! command -v iptables >/dev/null 2>&1; then
    echo "iptables not found - cannot set up NAT for $INTERFACE" >&2
    report_and_exit 1
fi

# Remove any existing rules for this interface to avoid duplicates

iptables -D INPUT -i $INTERFACE -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -i $INTERFACE -j ACCEPT 2>/dev/null || true
iptables -D OUTPUT -o $INTERFACE -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -i $INTERFACE -o $WAN4 -s $SUBNET -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -o $INTERFACE -m state --state ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
iptables -t nat -D POSTROUTING -s $SUBNET -o $WAN4 -j MASQUERADE 2>/dev/null || true

# The interface rules, whatever the uplink is. Replies to what a client
# started are scoped to this server's interface: it used to be a blanket
# "established anywhere" rule, added once per server and removed once per
# server, so two servers left two copies and a create/delete cycle drifted the
# count.
add4 -A INPUT -i "$INTERFACE" -j ACCEPT
add4 -A FORWARD -i "$INTERFACE" -j ACCEPT
add4 -A OUTPUT -o "$INTERFACE" -j ACCEPT
add4 -A FORWARD -o "$INTERFACE" -m state --state ESTABLISHED,RELATED -j ACCEPT

# Forwarding and NAT only out of a family that has somewhere to go.
case "$STATE4" in
    state=ok*)
        add4 -A FORWARD -i "$INTERFACE" -o "$WAN4" -s "$SUBNET" -j ACCEPT
        add4 -t nat -A POSTROUTING -s "$SUBNET" -o "$WAN4" -j MASQUERADE
        ;;
esac

# The IPv6 half. A host without ip6tables, or one built without the nat table,
# keeps working: the tunnel itself stays up, only NAT6 is missing - which is
# what the state line and the panel's iptables check report.
if [ -n "$SUBNET6" ] && command -v ip6tables >/dev/null 2>&1; then
    ip6tables -D INPUT -i $INTERFACE -j ACCEPT 2>/dev/null || true
    ip6tables -D FORWARD -i $INTERFACE -j ACCEPT 2>/dev/null || true
    ip6tables -D OUTPUT -o $INTERFACE -j ACCEPT 2>/dev/null || true
    ip6tables -D FORWARD -i $INTERFACE -o $WAN6 -s $SUBNET6 -j ACCEPT 2>/dev/null || true
    ip6tables -D FORWARD -o $INTERFACE -m state --state ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
    ip6tables -t nat -D POSTROUTING -s $SUBNET6 -o $WAN6 -j MASQUERADE 2>/dev/null || true

    add6 -A INPUT -i "$INTERFACE" -j ACCEPT
    add6 -A FORWARD -i "$INTERFACE" -j ACCEPT
    add6 -A OUTPUT -o "$INTERFACE" -j ACCEPT
    add6 -A FORWARD -o "$INTERFACE" -m state --state ESTABLISHED,RELATED -j ACCEPT

    # A ULA-only uplink gets the rule as well: a host that SNATs the bridge
    # address on its own still works, and the state line already says the
    # source is not routable. A family with no route at all gets nothing.
    case "$STATE6" in
        state=ok*|state=ula-only*)
            add6 -A FORWARD -i "$INTERFACE" -o "$WAN6" -s "$SUBNET6" -j ACCEPT
            if ! ip6tables -t nat -A POSTROUTING -s $SUBNET6 -o $WAN6 -j MASQUERADE; then
                echo "ip6tables NAT66 setup failed for $INTERFACE (is the ip6table_nat module available?)" >&2
                STATE6="state=failed"
                report_and_exit 1
            fi
            ;;
    esac
fi

echo "iptables rules set up successfully for $INTERFACE"
report_and_exit 0
