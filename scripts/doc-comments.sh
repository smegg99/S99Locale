#!/usr/bin/env bash
# scripts/doc-comments.sh
# Report doc comments that carry a paragraph break.
set -euo pipefail

found=0
while IFS= read -r file; do
    hits="$(awk '
      /^[[:space:]]*\/\// {
        line = $0
        sub(/^[[:space:]]*/, "", line)
        if (!block) { block = 1; brk = 0; start = FNR }
        if (line == "//") brk = 1
        next
      }
      {
        if (block && brk && $0 ~ /^(func|type|var|const|package)/) print FILENAME ":" start
        block = 0; brk = 0
      }
    ' "$file")"
    if [ -n "$hits" ]; then
        echo "$hits"
        found=1
    fi
done < <(find . -name '*.go' -not -path './bin/*' -not -name '*_gen.go' | sort)

if [ "$found" -eq 1 ]; then
    echo "the doc comments above carry a paragraph break; collapse them to one line" >&2
    exit 1
fi
