#!/bin/sh

mkdir -p /var/log/amnezia
chmod 755 /var/log/amnezia

# The image ships both iptables backends. Which one to use is not a question
# about kernel modules - nf_tables can be built into the kernel, and
# /proc/modules is not readable in every runtime, so `lsmod` answers the wrong
# question and a host with no module list was pushed onto the legacy backend
# unconditionally. Ask iptables itself instead: can the nft-backed binary read
# the nat table? Only when it cannot does legacy take over.
#
# When neither backend can read it - no NET_ADMIN, for instance - nothing is
# switched, and the panel's own iptables test is where the operator sees why.
if iptables -t nat -L -n >/dev/null 2>&1; then
    echo "iptables: the nft-backed binary works"
else
    legacy=$(command -v iptables-legacy)
    if [ -n "$legacy" ] && "$legacy" -t nat -L -n >/dev/null 2>&1; then
        # Shadow the nft-backed iptables with the legacy one. The binaries live
        # in /sbin on older Alpine and in /usr/sbin since the /usr merge, so
        # resolve them through PATH and drop the symlink in /usr/local/sbin,
        # which comes first in PATH either way.
        mkdir -p /usr/local/sbin
        ln -sf "$legacy" /usr/local/sbin/iptables
        echo "iptables: nft backend unavailable, iptables-legacy set as default"
    else
        echo "iptables: neither backend can read the nat table (NET_ADMIN missing?) - leaving it alone" >&2
    fi
fi

exec /usr/bin/api
