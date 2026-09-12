FROM mcr.microsoft.com/playwright:v1.63.0-noble@sha256:eff16c30e6f3f4af0a03fa4b706120d5e9b0891c344a27d64559aff5900a4a27
RUN npm install --global pnpm@10.31.0 && apt-get update && apt-get install -y --no-install-recommends libnss3-tools && rm -rf /var/lib/apt/lists/*
WORKDIR /workspace/frontend
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/tsconfig.node.json frontend/tsconfig.e2e.json frontend/playwright.config.ts ./
COPY frontend/e2e/ ./e2e/
COPY frontend/e2e/environment/runner/entrypoint.sh /usr/local/bin/test-entrypoint.sh
ENV CI=true NODE_EXTRA_CA_CERTS=/certs/ca.crt
ENTRYPOINT ["sh", "/usr/local/bin/test-entrypoint.sh"]
CMD ["pnpm", "run", "test:e2e"]
