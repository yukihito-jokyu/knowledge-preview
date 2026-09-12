#!/usr/bin/env bash
set -euo pipefail

# 起動途中の失敗でも当該実行の環境を片付け、最初の失敗を保持する。
cleanup() {
  result=$?
  trap - EXIT
  cleanup_result=0
  task e2e:env:down || cleanup_result=$?
  if [ "$result" -eq 0 ]; then
    result=$cleanup_result
  fi
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

task e2e:env:up
task e2e:env:init
task e2e:test
