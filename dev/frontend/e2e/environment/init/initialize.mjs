// 既存SPA・API・DBの疎通だけを確認する。業務seedの成功を意味しない。
const targets = [
  ["https://app.knowledge.test/login", 200],
  ["https://app.knowledge.test/api/ready", 204],
  ["https://oauth.knowledge.test/health", 200],
  ["https://preview.knowledge.test/", 404],
];

for (const [url, expected] of targets) {
  const deadline = Date.now() + 60_000;
  let ready = false;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(3_000) });
      if (response.status === expected) {
        ready = true;
        break;
      }
    } catch {
      /* 起動途中の接続失敗は期限内だけ再試行する。 */
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  if (!ready) throw new Error(`Environment readiness failed: ${url}`);
}
console.log("SPA/API/DB/preview readiness passed. Business fixtures are created by the E2E tests.");
