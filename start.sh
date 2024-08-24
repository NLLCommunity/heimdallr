#!/bin/bash

START_COMMAND=./bin/heimdallr

set -e
set -u
set -x

litestream version
echo "DB_REPLICA_URL=${DB_REPLICA_URL}"

readonly DB_PATH='heimdallr.db'
export DB_PATH

if [[ -f "$DB_PATH" ]]; then
  echo "Existing database is $(stat -c %s "${DB_PATH}") bytes"
else
  echo "Restoring database from replica"
  litestream restore -config litestream.yml -if-replica-exists "${DB_PATH}"
fi

litestream replicate -config litestream.yml -exec "${START_COMMAND}"
