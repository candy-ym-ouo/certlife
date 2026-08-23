#!/usr/bin/env bash
set -euo pipefail
FILES=$(find . -name '*.go' -not -name '*_test.go' -not -path './vendor/*' | sort)
LINES=$(cat $FILES | wc -l | tr -d ' ')
COUNT=$(echo "$FILES" | grep -c .)
echo "Go 文件数（不含测试）: $COUNT"
echo "Go 代码行数（不含测试）: $LINES"
[ "$COUNT" -gt 20 ] && [ "$COUNT" -lt 25 ] || { echo "文件数超出 (20,25)"; exit 1; }
[ "$LINES" -gt 2000 ] && [ "$LINES" -lt 2200 ] || { echo "行数超出 (2000,2200)"; exit 1; }
echo "约束满足"
