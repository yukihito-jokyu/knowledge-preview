#!/bin/sh
set -eu
umask 077
if [ -s /certs/server.crt ] && [ -s /certs/ca.crt ]; then
    openssl x509 -checkend 300 -noout -in /certs/server.crt
    openssl x509 -checkend 300 -noout -in /certs/ca.crt
    exit 0
fi
# 実行専用volumeに生成し、秘密鍵をリポジトリや成果物へ保存しない。
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -keyout /certs/ca.key -out /certs/ca.crt -subj '/CN=Knowledge E2E CA'
openssl req -newkey rsa:2048 -nodes -keyout /certs/server.key -out /certs/server.csr -subj '/CN=app.knowledge.test'
printf '%s\n' 'subjectAltName=DNS:app.knowledge.test,DNS:preview.knowledge.test,DNS:oauth.knowledge.test' 'extendedKeyUsage=serverAuth' > /certs/extensions
openssl x509 -req -in /certs/server.csr -CA /certs/ca.crt -CAkey /certs/ca.key -CAcreateserial -out /certs/server.crt -days 2 -extfile /certs/extensions
chmod 644 /certs/ca.crt /certs/server.crt
