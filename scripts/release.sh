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

# Public/main is always one parentless squash. Never parent onto a previous
# public commit — git log on Beach is a single ThirdCoast commit.
PUBLIC_REMOTE_URL="$(git remote get-url public 2>/dev/null || true)"
PUBLIC_IS_BEACH=0
if [[ "$PUBLIC_REMOTE_URL" == *ThirdCoastInteractive/Beach* ]]; then
  PUBLIC_IS_BEACH=1
elif [[ -n "$PUBLIC_REMOTE_URL" ]]; then
  echo "release: warning: public remote is $PUBLIC_REMOTE_URL (not ThirdCoastInteractive/Beach)" >&2
fi

if [[ "$PUBLIC_IS_BEACH" -eq 1 ]] && git show-ref --verify --quiet refs/remotes/public/main; then
  PARENT="$(git rev-parse refs/remotes/public/main)"
  PARENT_TREE="$(git rev-parse "$PARENT^{tree}")"
  # Skip only when the tree already matches AND public/main is already parentless.
  if [ "$RELEASE_TREE" = "$PARENT_TREE" ] && [ "$(git rev-list --count "$PARENT")" = "1" ]; then
    echo "Nothing to release — public/main is already the parentless stripped tree."
    exit 0
  fi
fi

echo "==> Public commit (no parent)..."
COMMIT=$(git commit-tree "$RELEASE_TREE" -m "$msg")

echo "==> Checking public commit $COMMIT..."
bash scripts/check-release.sh --commit "$COMMIT"

if [[ "$dry" -eq 1 ]]; then
  echo "dry-run: public commit $(git rev-parse --short "$COMMIT") — not pointing main, not pushing"
  echo "inspect: git ls-tree -r --name-only $COMMIT | head"
  exit 0
fi

if [[ "$PUBLIC_IS_BEACH" -eq 0 ]]; then
  echo "release: refusing to update-ref/push — point public at ThirdCoastInteractive/Beach first (got ${PUBLIC_REMOTE_URL:-none})" >&2
  echo "release: dangling public commit is $COMMIT" >&2
  exit 1
fi

# Local main becomes the public cut. Do not force-push origin/main — that ref
# on the-beach is leftover v0, not the Beach public lineage.
git update-ref refs/heads/main "$COMMIT"
echo "==> main is $(git rev-parse --short main)"
git push --force public main
echo "pushed main to public"
