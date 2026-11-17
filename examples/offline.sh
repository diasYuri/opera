#!/bin/sh
set -eu
opera_bin=${OPERA_BIN:-opera}
"$opera_bin" demo chat
"$opera_bin" demo run --format jsonl
"$opera_bin" demo worker
"$opera_bin" demo headless --format jsonl
