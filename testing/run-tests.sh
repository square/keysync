#!/bin/bash

set -eu
set -o pipefail
set -x

# Backup key for keysync, generated with `openssl rand -hex 16`
echo -n dd080878dbff297b06036bcc25f775f2 > /tmp/keysync-backup.key

# Start keywhiz (server)
java -jar /opt/keysync/testing/keywhiz-server.jar server /opt/keysync/testing/keywhiz-config.yaml &
keywhiz_pid=$!

# Start keysync (client)
keysync --config /opt/keysync/testing/keysync-config.yaml &
keysync_pid=$!

# Call the /sync endpoint which blocks until synced
sleep 5
curl --retry 20 -X POST http://localhost:31738/sync

# Diff content & permissions
function verify {
  pushd "$1"
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

# Create a backup
curl --fail -X POST http://localhost:31738/backup

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


# Stop keysync & keywhiz
kill $keywhiz_pid
kill $keysync_pid

sleep 1

# Keysync should have written a backup to this location
backup_file="/tmp/keysync-backup.tar.enc"
if [[ ! -f "$backup_file"  ]]; then
  echo "Backup file $backup_file is missing"
fi

rm -rf /secrets

# Restore the backup
keyrestore --config /opt/keysync/testing/keysync-config.yaml

# Make sure both clients are present in backup.  Client 2 was removed after backup was run.
verify /secrets/client1
verify /secrets/client2

echo "Keysync test passed"
