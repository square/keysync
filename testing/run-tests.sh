#!/bin/bash

set -eu
set -o pipefail
set -x

# Start keywhiz (server)
java -jar /opt/keysync/testing/keywhiz-server.jar server /opt/keysync/testing/keywhiz-config.yaml &

# Start keysync (client)
keysync --config /opt/keysync/testing/keysync-config.yaml &

# Call the /sync endpoint which blocks until synced
sleep 5
curl --retry 20 -X POST http://localhost:31738/sync

# Diff content & permissions
function verify {
  pushd $1
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
  echo "Verified $1"
  popd
}

verify /secrets/client1

# Make a second client
sed 's/client1/client2/g' /opt/keysync/testing/clients/client.yaml > /opt/keysync/testing/clients/client2.yaml

curl --fail -X POST http://localhost:31738/sync/client2

verify /secrets/client2

rm /opt/keysync/testing/clients/client2.yaml

curl --fail -X POST http://localhost:31738/sync/client2

if [ -d /secrets/client2 ]; then
  echo "ERROR: Client 2 was not removed"
  exit 1
fi

# Subsequent try should 404
curl -X POST http://localhost:31738/sync/client2

# Make sure client 1 still works
curl --fail -X POST http://localhost:31738/sync/client1
verify /secrets/client1

echo "Test passed"
