#!/usr/bin/env bash
# Guardrails for a Beach public cut (ThirdCoastInteractive/Beach).
#
#   scripts/check-release.sh --dev           # private-dev identity on HEAD
#   scripts/check-release.sh --commit REF    # public identity + denylist
#   scripts/check-release.sh --strip-index   # drop private paths from GIT_INDEX_FILE
#
# Source of truth for what a public commit may contain. Do not invent extra rules.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

DEV_NAME="Henry Slawniak"
DEV_EMAIL="henry@slawniak.com"
PUBLIC_NAME="ThirdCoast"
PUBLIC_EMAIL="git@thirdcoast.tv"

# Paths that must not appear on public/main. Examples ship. hush must already be gone.
# AGENTS.md and CLAUDE.md ship (agent-native). Cutter scripts stay private.
PRIVATE_PATH_GREP='^(\.grok/|\.claude/|cmd/hush/|cmd/askmuskie|cmd/saltbox/|cmd/lotophages\.com/|lotophages/|cmd/beach-demos/|internal/perf/|docs/master_|docs/HK|docs/templ\.md|scripts/release\.sh|scripts/check-release\.sh|scripts/copy-public-skills\.py)'

REQUIRED_PUBLIC_FILES=(
  AGENTS.md
  CLAUDE.md
  skills/beach/SKILL.md
  .github/copilot-instructions.md
)

fail() { echo "check-release: $*" >&2; exit 1; }

check_identity_dev() {
  local an ae cn ce
  an="$(git log -1 --format='%an')"
  ae="$(git log -1 --format='%ae')"
  cn="$(git log -1 --format='%cn')"
  ce="$(git log -1 --format='%ce')"
  [[ "$an" == "$DEV_NAME" && "$ae" == "$DEV_EMAIL" ]] || \
    fail "HEAD author is '$an <$ae>'; --dev wants $DEV_NAME <$DEV_EMAIL>"
  [[ "$cn" == "$DEV_NAME" && "$ce" == "$DEV_EMAIL" ]] || \
    fail "HEAD committer is '$cn <$ce>'; --dev wants $DEV_NAME <$DEV_EMAIL>"
  local msg
  msg="$(git log -1 --format=%B)"
  if echo "$msg" | grep -qiE 'Co-Authored-By|Made-with:|Generated-by:'; then
    fail "HEAD message has an agent trailer"
  fi
}

check_identity_public() {
  local ref="$1" line
  # Tip and every ancestor. Parenting onto leftover the-beach main would leak
  # Henry Slawniak into public git log.
  while IFS= read -r line; do
    [[ "$line" == "$PUBLIC_NAME <$PUBLIC_EMAIL>" ]] || \
      fail "$ref history author '$line'; every ancestor must be $PUBLIC_NAME <$PUBLIC_EMAIL>"
  done < <(git log --format='%an <%ae>' "$ref")
  while IFS= read -r line; do
    [[ "$line" == "$PUBLIC_NAME <$PUBLIC_EMAIL>" ]] || \
      fail "$ref history committer '$line'; every ancestor must be $PUBLIC_NAME <$PUBLIC_EMAIL>"
  done < <(git log --format='%cn <%ce>' "$ref")
}

check_private_files() {
  local ref="$1"
  local hits
  hits="$(git ls-tree -r --name-only "$ref" | grep -E "$PRIVATE_PATH_GREP" || true)"
  if [[ -n "$hits" ]]; then
    echo "$hits" >&2
    fail "$ref contains private paths. Strip them before pushing public/main."
  fi
}

check_required_public() {
  local ref="$1" f missing=""
  for f in "${REQUIRED_PUBLIC_FILES[@]}"; do
    git cat-file -e "$ref:$f" 2>/dev/null || missing+="$f "
  done
  [[ -z "$missing" ]] || fail "$ref missing required public files: $missing"
}

check_denylist_text() {
  local ref="$1"
  local tree
  tree="$(git ls-tree -r --name-only "$ref")"
  # Word-boundary: free-core datastar.js contains "datastar-prop-change".
  if echo "$tree" | grep -qiE '\bdatastar-pro\b'; then
    fail "$ref names datastar-pro"
  fi
  if echo "$tree" | grep -qiE 'the-beach'; then
    fail "$ref names the-beach"
  fi
  git grep -n -E '\bdatastar-pro\b|Y:\\\\|C:\\\\Users\\\\henry|slawniak\.com' "$ref" -- \
    ':!vendor/' ':!*.sum' >/tmp/beach-check-hits 2>/dev/null || true
  git grep -n -i -E 'the-beach' "$ref" -- \
    ':!vendor/' ':!*.sum' >>/tmp/beach-check-hits 2>/dev/null || true
  if [[ -s /tmp/beach-check-hits ]]; then
    cat /tmp/beach-check-hits >&2
    fail "$ref matches denylist (datastar-pro, the-beach, private paths, slawniak.com)"
  fi
  if git grep -l 'github.com/ThirdCoastInteractive/Repute' "$ref" -- go.mod >/dev/null 2>&1; then
    fail "$ref go.mod still requires Repute"
  fi
}

strip_index() {
  local f
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    git rm --cached -f -- "$f" >/dev/null
  done < <(git ls-files | grep -E "$PRIVATE_PATH_GREP" || true)
}

usage() {
  echo "Usage: $0 --dev | --commit REF | --strip-index" >&2
  exit 2
}

if [[ $# -eq 1 && "$1" == "--dev" ]]; then
  check_identity_dev
  echo "check-release --dev: ok ($DEV_NAME <$DEV_EMAIL>)"
  exit 0
fi

if [[ $# -eq 1 && "$1" == "--strip-index" ]]; then
  strip_index
  exit 0
fi

if [[ $# -eq 2 && "$1" == "--commit" ]]; then
  REF="$2"
  git rev-parse -q --verify "$REF^{commit}" >/dev/null || fail "not a commit: $REF"
  check_identity_public "$REF"
  check_private_files "$REF"
  check_required_public "$REF"
  check_denylist_text "$REF"
  echo "OK: $REF is a legal public commit ($PUBLIC_NAME <$PUBLIC_EMAIL>)"
  exit 0
fi

usage
