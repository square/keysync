#!/bin/bash
set +ex
# Generate client fixtures, using certstrap.

# The 'CA' folder is taken from the `keywhiz` repo, and is required to work here.

mkdir -p clients

# Create four client certs
for client in client1 client2 client3 client4; do
     rm -f clients/${client}.csr clients/${client}.crt clients/${client}.key
     certstrap --depot-path clients request-cert --domain ${client} --passphrase ''
     certstrap --depot-path clients sign --years 30 --CA ../CA/cacert ${client}
     rm -f clients/${client}.csr
     git add clients/${client}.crt clients/${client}.key
done