#!/bin/bash
# Cleanup transparent proxy iptables rules
IPTABLES=$(which iptables 2>/dev/null || which iptables-legacy 2>/dev/null)
if [ -z "$IPTABLES" ]; then echo "no iptables"; exit 1; fi

$IPTABLES -t nat -D PREROUTING -j KVN_TPROXY 2>/dev/null
$IPTABLES -t nat -D OUTPUT -j KVN_TPROXY 2>/dev/null
$IPTABLES -t nat -F KVN_TPROXY 2>/dev/null
$IPTABLES -t nat -X KVN_TPROXY 2>/dev/null
echo "KVN_TPROXY chain removed"
