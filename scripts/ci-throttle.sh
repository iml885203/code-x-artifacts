#!/bin/sh
# Runs its arguments and prints how often the container's CPU quota throttled it meanwhile.
cg="/sys/fs/cgroup$(sed -n 's/^0:://p' /proc/self/cgroup 2>/dev/null)"
stat() { awk -v k="$1" '$1 == k { print $2 }' "$cg/cpu.stat" 2>/dev/null; }
p0="$(stat nr_periods)"; t0="$(stat nr_throttled)"; u0="$(stat throttled_usec)"; c0="$(stat usage_usec)"; s="$(date +%s)"
"$@"; rc=$?
echo "THROTTLE $(echo "$*" | cut -c1-40): wall=$(($(date +%s) - s))s throttled_periods=$(($(stat nr_throttled) - t0))/$(($(stat nr_periods) - p0)) frozen_s=$((($(stat throttled_usec) - u0) / 1000000)) cpu_s=$((($(stat usage_usec) - c0) / 1000000))"
exit "$rc"
