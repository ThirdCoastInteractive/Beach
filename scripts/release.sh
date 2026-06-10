#!/usr/bin/env bash
# Release: squash finished work from master (dev) into main (public release).
#
#   master / remote `origin` — dev branch, messy WIP commits welcome
#   main   / remote `public` — release branch, ONLY squashed summary commits
#
# Usage:
#   scripts/release.sh "summary line of the finished work"
#
# The summary becomes the single squashed commit message on main. Write it for
# the public changelog: what shipped, not how the sausage was made.
set -euo pipefail

msg="${1:-}"
if [ -z "$msg" ]; then
  echo "usage: scripts/release.sh \"summary of finished work\"" >&2
  exit 1
fi

# Refuse on a dirty tree — release only committed work.
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "release: working tree not clean — commit or stash first" >&2
  exit 1
fi

current=$(git rev-parse --abbrev-ref HEAD)
if [ "$current" != "master" ]; then
  echo "release: run from master (currently on $current)" >&2
  exit 1
fi

git checkout main
git merge --squash master
git commit -m "$msg"

# Push the release; push dev too so origin stays current.
git push public main:main
git push origin master:master

git checkout master
echo "release: main is now $(git rev-parse --short main) — \"$msg\""
