#!/bin/bash
set -eu
cd "$(dirname "$0")"

go test -count=1 -run .
