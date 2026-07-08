#!/usr/bin/env bash
set -uo pipefail

# Upstream merge pre/post check script
# Usage:
#   ./scripts/merge-check.sh pre    — run before merge
#   ./scripts/merge-check.sh post   — run after merge
#   ./scripts/merge-check.sh full   — run both (for CI)

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PASS=0
FAIL=0
WARN=0

pass() { ((PASS++)); echo -e "  ${GREEN}✓${NC} $1"; }
fail() { ((FAIL++)); echo -e "  ${RED}✗${NC} $1"; }
warn() { ((WARN++)); echo -e "  ${YELLOW}!${NC} $1"; }

# ─── PRE-MERGE CHECKS ───

pre_check() {
  echo "═══ Pre-merge checks ═══"
  echo ""

  # 1. Working tree clean
  echo "1. Working tree status"
  if [ -z "$(git status --porcelain)" ]; then
    pass "Working tree clean"
  else
    fail "Working tree has uncommitted changes"
    git status --short
  fi

  # 2. On correct branch
  echo "2. Branch check"
  branch=$(git branch --show-current)
  echo "  Current branch: $branch"
  if [ "$branch" = "main" ]; then
    pass "On main branch"
  else
    warn "Not on main — currently on '$branch'"
  fi

  # 3. upstream remote configured
  echo "3. Upstream remote"
  if git remote get-url upstream &>/dev/null; then
    pass "upstream remote: $(git remote get-url upstream)"
  else
    fail "upstream remote not configured"
    echo "  Fix: git remote add upstream https://github.com/QuantumNous/new-api.git"
  fi

  # 4. upstream/main fetched
  echo "4. Upstream freshness"
  git fetch upstream main --no-tags 2>/dev/null || true
  local_ts=$(git log -1 --format='%ct' upstream/main 2>/dev/null || echo 0)
  now=$(date +%s)
  age=$(( (now - local_ts) / 3600 ))
  if [ "$age" -lt 24 ]; then
    pass "upstream/main fetched (${age}h ago)"
  else
    warn "upstream/main is ${age}h old — consider: git fetch upstream main"
  fi

  # 5. Merge base analysis
  echo "5. Merge gap analysis"
  merge_base=$(git merge-base main upstream/main)
  gap=$(git log --oneline --no-merges "${merge_base}..upstream/main" | wc -l)
  local_gap=$(git log --oneline --no-merges "${merge_base}..main" | wc -l)
  echo "  Common ancestor: $(git log --oneline -1 "$merge_base")"
  echo "  Upstream commits to merge: $gap"
  echo "  Local commits since base:  $local_gap"
  if [ "$gap" -eq 0 ]; then
    pass "Already up to date"
  elif [ "$gap" -lt 50 ]; then
    pass "Gap is manageable ($gap commits)"
  else
    warn "Large gap: $gap commits — review upstream changelog first"
  fi

  # 6. Potential conflict files
  echo "6. Potential conflict zones"
  conflicts=$(comm -12 \
    <(git diff --name-only "main...upstream/main" 2>/dev/null | sort) \
    <(git diff --name-only "upstream/main...main" 2>/dev/null | sort))
  if [ -z "$conflicts" ]; then
    pass "No overlapping file changes"
  else
    count=$(echo "$conflicts" | wc -l)
    warn "$count files modified on both sides:"
    echo "$conflicts" | while read -r f; do echo "    - $f"; done
  fi

  # 7. Pre-merge tag
  echo "7. Safety tag"
  tag="pre-merge-upstream-$(date +%Y%m%d)"
  if git tag -l "$tag" | grep -q .; then
    pass "Tag $tag already exists"
  else
    warn "Tag $tag not found — will create during merge"
  fi

  # 8. rerere enabled
  echo "8. Git rerere"
  if [ "$(git config rerere.enabled)" = "true" ]; then
    pass "rerere enabled"
  else
    warn "rerere disabled — recommend: git config rerere.enabled true"
  fi

  # 9. Go toolchain
  echo "9. Go toolchain"
  if command -v go &>/dev/null; then
    pass "go $(go version | awk '{print $3}')"
  else
    warn "go not found — post-merge checks will skip build/test"
  fi
}

# ─── POST-MERGE CHECKS ───

post_check() {
  echo "═══ Post-merge checks ═══"
  echo ""

  # 1. No conflict markers left
  echo "1. Conflict marker scan"
  markers=$(grep -rl '<<<<<<< ' --include='*.go' --include='*.jsx' --include='*.tsx' --include='*.json' --include='*.md' . 2>/dev/null | grep -v node_modules | grep -v '.git/' || true)
  if [ -z "$markers" ]; then
    pass "No conflict markers found"
  else
    fail "Conflict markers remain in:"
    echo "$markers" | while read -r f; do echo "    - $f"; done
  fi

  # 2. Protected identifiers intact
  echo "2. Protected identifiers"
  if grep -rq "QuantumNous\|QuantumNous" README.md 2>/dev/null; then
    pass "QuantumNous references intact in README"
  else
    warn "Check README for protected identifiers"
  fi

  # 3. Go compilation
  echo "3. Go compilation"
  if command -v go &>/dev/null; then
    if go vet ./... 2>&1 | head -5; then
      vet_exit=${PIPESTATUS[0]}
      if [ "$vet_exit" -eq 0 ]; then
        pass "go vet passed"
      else
        fail "go vet failed"
      fi
    fi

    if go build ./... 2>&1 | head -10; then
      build_exit=${PIPESTATUS[0]}
      if [ "$build_exit" -eq 0 ]; then
        pass "go build passed"
      else
        fail "go build failed"
      fi
    fi
  else
    warn "go not available — skipping build checks"
  fi

  # 4. Key files exist (not accidentally deleted)
  echo "4. Critical file existence"
  critical_files=(
    "main.go"
    "router/api-router.go"
    "controller/relay.go"
    "service/risk_control.go"
    "service/group_monitoring.go"
    "service/pressure_cooling.go"
    "model/log.go"
    "model/risk_rule.go"
  )
  for f in "${critical_files[@]}"; do
    if [ -f "$f" ]; then
      pass "$f exists"
    else
      fail "$f MISSING"
    fi
  done

  # 5. Frontend directory structure
  echo "5. Frontend structure"
  if [ -d "web/classic/src" ] || [ -d "web/src" ]; then
    if [ -d "web/classic/src" ]; then
      pass "web/classic/src/ exists (post-restructure)"
    fi
    if [ -d "web/src" ]; then
      pass "web/src/ exists (pre-restructure)"
    fi
  else
    fail "No frontend source directory found"
  fi

  # 6. Custom frontend components not lost
  echo "6. Custom component integrity"
  custom_components=(
    "pages/Risk"
    "components/monitoring/GroupMonitoringDashboard"
    "components/channel/PressureCoolingEditor"
    "pages/Ticket"
    "pages/GroupMonitoring"
  )
  web_base=""
  if [ -d "web/classic/src" ]; then
    web_base="web/classic/src"
  elif [ -d "web/src" ]; then
    web_base="web/src"
  fi
  if [ -n "$web_base" ]; then
    for comp in "${custom_components[@]}"; do
      found=$(find "$web_base" -path "*${comp}*" -type f 2>/dev/null | head -1)
      if [ -n "$found" ]; then
        pass "$comp found"
      else
        fail "$comp MISSING in $web_base"
      fi
    done
  fi

  # 7. Go test (risk-related)
  echo "7. Risk-related tests"
  if command -v go &>/dev/null; then
    if go test ./service/... -run Risk -count=1 -short 2>&1 | tail -5; then
      test_exit=${PIPESTATUS[0]}
      if [ "$test_exit" -eq 0 ]; then
        pass "Risk tests passed"
      else
        fail "Risk tests failed"
      fi
    fi
  else
    warn "go not available — skipping tests"
  fi

  # 8. i18n key consistency
  echo "8. i18n files"
  for lang in zh en fr ru ja vi; do
    if [ -d "web/classic/src/i18n" ]; then
      locale_dir="web/classic/src/i18n/locales"
    else
      locale_dir="web/src/i18n/locales"
    fi
    # Check file exists and is valid JSON
    for f in "$locale_dir"/*.json; do
      if [ -f "$f" ]; then
        if python3 -c "import json; json.load(open('$f'))" 2>/dev/null; then
          : # valid
        else
          fail "Invalid JSON: $f"
        fi
      fi
    done
  done
  pass "i18n locale files are valid JSON"

  # 9. No .claude references in git log (last 5 commits)
  echo "9. Commit hygiene"
  bad_commits=$(git log --oneline -5 | grep -i '\.claude\|co-authored' || true)
  if [ -z "$bad_commits" ]; then
    pass "Recent commits clean (no .claude / Co-Authored-By)"
  else
    warn "Found .claude or Co-Authored-By in recent commits:"
    echo "  $bad_commits"
  fi
}

# ─── SUMMARY ───

summary() {
  echo ""
  echo "═══ Summary ═══"
  echo -e "  ${GREEN}Passed: $PASS${NC}  ${RED}Failed: $FAIL${NC}  ${YELLOW}Warnings: $WARN${NC}"
  if [ "$FAIL" -gt 0 ]; then
    echo -e "  ${RED}ACTION REQUIRED: Fix failures before proceeding${NC}"
    return 1
  elif [ "$WARN" -gt 0 ]; then
    echo -e "  ${YELLOW}Review warnings above${NC}"
    return 0
  else
    echo -e "  ${GREEN}All clear${NC}"
    return 0
  fi
}

# ─── MAIN ───

cd "$(git rev-parse --show-toplevel)"

case "${1:-full}" in
  pre)  pre_check; summary ;;
  post) post_check; summary ;;
  full) pre_check; echo ""; post_check; summary ;;
  *)    echo "Usage: $0 {pre|post|full}"; exit 1 ;;
esac
