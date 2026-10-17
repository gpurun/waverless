#!/usr/bin/env sh
set -eu

docker compose up -d postgres redis
docker compose --profile test run --rm test
