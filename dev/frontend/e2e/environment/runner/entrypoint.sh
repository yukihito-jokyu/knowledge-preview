#!/bin/sh
set -eu
cp /certs/ca.crt /usr/local/share/ca-certificates/knowledge-e2e.crt
update-ca-certificates >/dev/null
mkdir -p "$HOME/.pki/nssdb"
certutil -N --empty-password -d "sql:$HOME/.pki/nssdb"
certutil -A -d "sql:$HOME/.pki/nssdb" -n knowledge-e2e -t 'C,,' -i /certs/ca.crt
# Firefoxはfixtures/test.tsがテスト専用profileのNSSへ登録する。
export E2E_CA_CERT=/certs/ca.crt
exec "$@"
