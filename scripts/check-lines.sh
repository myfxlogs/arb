#!/usr/bin/env bash
# check-lines — enforce Go file-size limits (AGENTS §7 / constraints §六).
#   soft > 300 lines → warning ; hard > 450 lines → fail (CI-blocking, exit 1)
# Exempt:
#   proto/    — generated stubs + .proto source (not hand-maintained)
#   docs/     — reference projects (docs/ant temp project, mtapi Go examples); not our code (AGENTS §11)
#   *_test.go — test files (constraints §六)
#   vendor/ .git/
#
# Single source of truth: CI (.github/workflows/ci.yml) and the Before Commit
# checklist (AGENTS §10) both invoke this script, so local and CI checks are
# identical — no bespoke Go tool, no logic drift between the two.
set -uo pipefail

hard=0
while IFS= read -r -d '' f; do
    n=$(wc -l < "$f")
    if [ "$n" -gt 450 ]; then
        echo "FAIL  (>450): $f = $n"
        hard=1
    elif [ "$n" -gt 300 ]; then
        echo "warn  (>300): $f = $n"
    fi
done < <(find . -name '*.go' \
    -not -path './vendor/*' \
    -not -path './proto/*' \
    -not -path './docs/*' \
    -not -path './.git/*' \
    -not -name '*_test.go' \
    -print0)

if [ "$hard" -eq 1 ]; then
    echo "ERROR: hard 450-line limit violated (constraints §六). Split the file."
    exit 1
fi
echo "OK: all Go files within the 450-line hard limit."
