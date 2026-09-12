FROM nginx:1.28.0-bookworm@sha256:552e7481ca93ffccd046aa658dbbed22caefbc09c66fa7cd247cbb90b8a5c609
RUN apt-get update && apt-get install -y --no-install-recommends openssl ca-certificates && rm -rf /var/lib/apt/lists/*
COPY frontend/e2e/environment/proxy/proxy.conf /etc/nginx/conf.d/default.conf
COPY frontend/e2e/environment/certificates/generate.sh /usr/local/bin/certificates.sh
