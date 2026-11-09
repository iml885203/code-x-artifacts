#!/bin/sh
# make's SHELL for CI timing: runs the recipe exactly as make handed it over and
# prints one PHASE line on stderr afterwards, so a verify run says which recipes
# carry its time and CPU stall (cpu.stat throttling, cpu.pressure) instead of
# only the two steps scripts/ci-throttle.sh wraps. Used only through
# VERIFY_RUNNER in the verify job (.gitlab-ci.yml): make SHELL=scripts/ci-phase.sh.
#
# Measuring never changes the job: the recipe's own argv, stdout, stderr and
# exit status pass through untouched, and an unreadable cgroup file or a failed
# write leaves the exit status alone. The line carries no recipe text: only a
# class from the fixed table below and an opaque checksum that tells two recipes
# of the same class apart. Nested makes are wrapped too (SHELL travels in
# MAKEFLAGS); MAKELEVEL says which line contains which. Few forks on purpose:
# it runs once per recipe line.

last=
for a in "$@"; do last=$a; done

line=
{ read -r line </proc/self/cgroup; } 2>/dev/null
cg="${CI_PHASE_CGROUP:-/sys/fs/cgroup${line#0::}}"

# Prints five fields, "-" for any that is missing or not a number, so each metric
# stands alone: usage_usec nr_throttled nr_periods throttled_usec cpu_pressure_some_total_usec.
snap() {
	files=
	[ -r "$cg/cpu.stat" ] && files="$cg/cpu.stat"
	[ -r "$cg/cpu.pressure" ] && files="$files $cg/cpu.pressure"
	if [ -z "$files" ]; then
		echo "- - - - -"
		return
	fi
	# shellcheck disable=SC2086
	awk 'function num(x) { return x ~ /^[0-9]+$/ }
		BEGIN { u = t = p = f = s = "-" }
		FILENAME ~ /cpu.stat$/ && $1 == "usage_usec" && num($2) { u = $2 }
		FILENAME ~ /cpu.stat$/ && $1 == "nr_throttled" && num($2) { t = $2 }
		FILENAME ~ /cpu.stat$/ && $1 == "nr_periods" && num($2) { p = $2 }
		FILENAME ~ /cpu.stat$/ && $1 == "throttled_usec" && num($2) { f = $2 }
		FILENAME ~ /cpu.pressure$/ && $1 == "some" { for (i = 2; i <= NF; i++) if ($i ~ /^total=[0-9]+$/) { s = substr($i, 7) } }
		END { print u, t, p, f, s }' $files 2>/dev/null
}

start=$(date +%s 2>/dev/null)
before=$(snap)

/bin/sh "$@" 3>&-
rc=$?

(
	# fd 3 is the job's stderr; this block's own errors go nowhere.
	exec 3>&2 2>/dev/null
	end=$(date +%s)
	after=$(snap)
	# shellcheck disable=SC2086
	set -- $before $after
	num() { case "$1" in '' | *[!0-9]*) return 1 ;; esac; }
	# delta <before> <after> <divisor>: sets R to the difference, or n/a.
	delta() {
		if num "$1" && num "$2" && [ "$2" -ge "$1" ]; then R=$((($2 - $1) / $3)); else R=n/a; fi
	}
	# tenths: turns R, a count of tenths of a second, into "S.T"; n/a stays.
	tenths() { if [ "$R" != n/a ]; then R="$((R / 10)).$((R % 10))"; fi; }
	cpu=n/a thr=n/a frozen=n/a psi=n/a
	if [ "$#" -eq 10 ]; then
		delta "$1" "$6" 1000000
		cpu=$R
		delta "$4" "$9" 100000
		tenths
		frozen=$R
		delta "$5" "${10}" 100000
		tenths
		psi=$R
		delta "$2" "$7" 1
		t=$R
		delta "$3" "$8" 1
		if [ "$t" != n/a ] && [ "$R" != n/a ]; then thr="$t/$R"; fi
	fi
	m="cpu_s=$cpu thr=$thr frozen_s=$frozen psi_s=$psi"
	case "$last" in
	*"go test"*) class=gotest ;;
	*golangci-lint*) class=golint ;;
	*"go vet"*) class=govet ;;
	*"go build"*) class=gobuild ;;
	*"go mod"*) class=gomod ;;
	*vitest* | *"run test"*) class=vitest ;;
	*"run typecheck"* | *"run lint"* | *npm*) class=npm ;;
	*node* | *.mjs*) class=node ;;
	*verify-*) class=check ;;
	*) class=other ;;
	esac
	set -- $(printf %s "$last" | cksum)
	printf 'PHASE class=%s id=%s lvl=%s wall=%ss %s\n' "$class" "$1" "${MAKELEVEL:-0}" "$((end - start))" "$m" >&3
) || :

exit "$rc"
