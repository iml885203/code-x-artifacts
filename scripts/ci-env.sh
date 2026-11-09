#!/bin/sh
# Prints what the job's pod really has, so a slow run can be read against it:
# the CPUs the kernel shows, the quota the cgroup enforces (and how much it
# throttled), Go's GOMAXPROCS, load, and how long a synced write takes here.
# Usage: scripts/ci-env.sh <label>      (never fails the job)
label="${1:-env}"
echo "== ci-env $label =="
echo "nproc: $(nproc 2>/dev/null)  GOMAXPROCS(env): ${GOMAXPROCS:-unset}"
if command -v go >/dev/null 2>&1; then
	go run scripts/ci-env/main.go 2>&1 | sed 's/^/go: /'
	GODEBUG=containermaxprocs=0 go run scripts/ci-env/main.go 2>&1 | sed 's/^/go (no container awareness): /'
fi
echo "/proc/self/cgroup: $(cat /proc/self/cgroup 2>/dev/null | tr '\n' ' ')"
grep cgroup /proc/self/mountinfo 2>/dev/null | sed 's/^/mountinfo: /'
# The files of this process's own cgroup and of every parent up to the mount:
# the quota may sit on the pod's cgroup, not the container's.
cg="$(sed -n 's/^0:://p' /proc/self/cgroup 2>/dev/null)"
d="/sys/fs/cgroup$cg"
while [ -n "$d" ] && [ "$d" != "/sys/fs" ]; do
	for n in cpu.max cpu.weight cpu.stat; do
		[ -r "$d/$n" ] && { echo "$d/$n:"; sed 's/^/  /' "$d/$n"; }
	done
	d="${d%/*}"
done
for f in /sys/fs/cgroup/cpu.max /sys/fs/cgroup/cpu/cpu.cfs_quota_us /sys/fs/cgroup/cpu/cpu.cfs_period_us /sys/fs/cgroup/cpu.weight /sys/fs/cgroup/cpu/cpu.shares /sys/fs/cgroup/memory.max; do
	[ -r "$f" ] && echo "$f: $(cat "$f")"
done
for f in /sys/fs/cgroup/cpu.stat /sys/fs/cgroup/cpu/cpu.stat /sys/fs/cgroup/cpu.pressure /sys/fs/cgroup/io.pressure /proc/pressure/cpu /proc/pressure/io; do
	[ -r "$f" ] && { echo "$f:"; sed 's/^/  /' "$f"; }
done
echo "processes in this pod: $(ps -e 2>/dev/null | wc -l)"
echo "loadavg: $(cat /proc/loadavg 2>/dev/null)"
echo "cpu model: $(grep -m1 'model name' /proc/cpuinfo 2>/dev/null)"
echo "mounts:"; df -T "${TMPDIR:-/tmp}" . /dev/shm 2>/dev/null | sed 's/^/  /'
s="$(date +%s%N)"; i=0
while [ "$i" -lt 50 ]; do
	dd if=/dev/zero of=.ci-env-probe bs=4k count=1 conv=fsync 2>/dev/null; i=$((i + 1))
done
e="$(date +%s%N)"; rm -f .ci-env-probe
echo "50 synced 4k writes in cwd: $(((e - s) / 1000000)) ms"
s="$(date +%s%N)"; i=0
while [ "$i" -lt 50 ]; do
	dd if=/dev/zero of="${TMPDIR:-/tmp}/.ci-env-probe" bs=4k count=1 conv=fsync 2>/dev/null; i=$((i + 1))
done
e="$(date +%s%N)"; rm -f "${TMPDIR:-/tmp}/.ci-env-probe"
echo "50 synced 4k writes in tmp: $(((e - s) / 1000000)) ms"
if [ -d /dev/shm ]; then
	s="$(date +%s%N)"; i=0
	while [ "$i" -lt 50 ]; do
		dd if=/dev/zero of=/dev/shm/.ci-env-probe bs=4k count=1 conv=fsync 2>/dev/null; i=$((i + 1))
	done
	e="$(date +%s%N)"; rm -f /dev/shm/.ci-env-probe
	echo "50 synced 4k writes in /dev/shm: $(((e - s) / 1000000)) ms"
fi
exit 0
