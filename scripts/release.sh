#!/usr/bin/env bash
# Squash master into a single public commit on main (Rewind-style).
#
#   origin  ThirdCoastInteractive/the-beach  (private, master)
#   public  ThirdCoastInteractive/Beach      (public, main) — optional until created
#
# Usage:
#   scripts/release.sh "summary of the finished work"
#   scripts/release.sh "summary" --dry-run    # commit-tree + check-release, no push
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

msg="${1:-}"
if [ -z "$msg" ]; then
  echo "usage: scripts/release.sh \"summary of finished work\" [--dry-run]" >&2
  exit 1
fi
dry=0
if [[ "${2:-}" == "--dry-run" ]]; then
  dry=1
fi

if ! git diff-index --quiet HEAD --; then
  echo "release: working tree not clean — commit or stash first" >&2
  exit 1
fi

export GIT_AUTHOR_NAME="ThirdCoast"
export GIT_AUTHOR_EMAIL="git@thirdcoast.tv"
export GIT_COMMITTER_NAME="ThirdCoast"
export GIT_COMMITTER_EMAIL="git@thirdcoast.tv"

echo "==> Reading master tree..."
MASTER_TREE=$(git rev-parse master^{tree})
TMPINDEX="$(mktemp)"
export GIT_INDEX_FILE="$TMPINDEX"
git read-tree "$MASTER_TREE"
bash scripts/check-release.sh --strip-index
RELEASE_TREE=$(git write-tree)
rm -f "$TMPINDEX"
unset GIT_INDEX_FILE

if git show-ref --verify --quiet refs/heads/main; then
  MAIN_TREE=$(git rev-parse main^{tree})
  if [ "$RELEASE_TREE" = "$MAIN_TREE" ]; then
    echo "Nothing to release — main already matches stripped master."
    exit 0
  fi
  PARENT=$(git rev-parse main)
  COMMIT=$(git commit-tree "$RELEASE_TREE" -p "$PARENT" -m "$msg")
else
  echo "==> First public commit (no parent)..."
  COMMIT=$(git commit-tree "$RELEASE_TREE" -m "$msg")
fi

echo "==> Checking public commit $COMMIT..."
bash scripts/check-release.sh --commit "$COMMIT"

if [[ "$dry" -eq 1 ]]; then
  echo "dry-run: public commit $(git rev-parse --short "$COMMIT") — not pointing main, not pushing"
  echo "inspect: git ls-tree -r --name-only $COMMIT | head"
  exit 0
fi

git update-ref refs/heads/main "$COMMIT"
echo "==> main is $(git rev-parse --short main)"

if git remote get-url public >/dev/null 2>&1; then
  git push --force origin main
  git push --force public main
  echo "pushed main to origin and public"
else
  echo "no public remote — left main local only"
fi
