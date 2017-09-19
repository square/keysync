#!/bin/bash

set -eu
set -o pipefail

# Start keywhiz (server)
java -jar /opt/keysync/testing/keywhiz-server.jar server /opt/keysync/testing/keywhiz-config.yaml &

# Start keysync (client)
keysync --config /opt/keysync/testing/keysync-config.yaml &

# Some time to finish sync
# TODO(cs): use curl and hit status endpoint when openjdk/alpine image updates curl package
sleep 20

# Diff content & permissions
cd /secrets/client1

for file in *; do
  perms_actual="$(stat -c '%U:%G:%a' "$file")"
  perms_expected="$(cat /opt/keysync/testing/expected/ownership/"$file")"
  content_actual="$(cat "$file")"
  content_expected="$(cat /opt/keysync/testing/expected/content/"$file")"

  if [ "$perms_actual" != "$perms_expected" ]; then
    echo "ERROR: Incorrect ownership on file $file (expecting $perms_expected, got $perms_actual)"
    exit 1
  fi
  if [ "$content_actual" != "$content_expected" ]; then
    echo "ERROR: Incorrect content in file $file (expecting $content_expected, got $content_actual)"
    exit 1
  fi
done
