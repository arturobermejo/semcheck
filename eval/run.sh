#!/usr/bin/env bash
#
# Runs semcheck, with the rules of .semcheck.yml, on the projects of
# eval/repos.txt, and leaves what it says of each in tmp/eval/results/<name>:
#
#   records.jsonl  every question and its answer (see the -record flag)
#   log.txt        findings, warnings and stats
#   run.txt        exit code and seconds
#
#   eval/run.sh -dry-run           count the questions of all, asking nothing
#   eval/run.sh caddy gotify       ask about these two (needs TYPESAFE_API_KEY)
#
# It only collects. "go run ./eval" reads the results.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
work=$root/tmp/eval
flags=(-stats)

if [[ ${1:-} == -dry-run ]]; then
	flags=(-dry-run)
	shift
fi

mkdir -p "$work"
go build -C "$root" -o "$work/semcheck" ./cmd/semcheck

while read -r name url commit packages; do
	[[ -z $name || $name == \#* ]] && continue
	[[ $# -gt 0 && " $* " != *" $name "* ]] && continue

	dir=$work/repos/$name
	out=$work/results/$name

	if [[ ! -d $dir/.git ]]; then
		git init -q "$dir"
		git -C "$dir" remote add origin "$url"
	fi

	if [[ $(git -C "$dir" rev-parse -q --verify HEAD || true) != "$commit" ]]; then
		git -C "$dir" fetch -q --depth 1 origin "$commit"
		git -C "$dir" checkout -q --detach FETCH_HEAD
	fi

	rm -rf "$out"
	mkdir -p "$out"

	echo "$name: semcheck ${flags[*]} $packages"

	start=$SECONDS
	code=0

	# Word splitting of $packages is wanted: it may be several patterns.
	# shellcheck disable=SC2086
	(cd "$dir" && "$work/semcheck" -config="$root/.semcheck.yml" "${flags[@]}" -record="$out/records.jsonl" $packages) \
		>"$out/log.txt" 2>&1 || code=$?

	echo "exit=$code seconds=$((SECONDS - start))" | tee "$out/run.txt"
done <"$root/eval/repos.txt"
