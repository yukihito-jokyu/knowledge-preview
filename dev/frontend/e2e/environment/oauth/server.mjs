import { createServer } from "node:http";

// 今回はプロセス疎通のみ。認可・code交換・PKCE・ユーザー取得は対応Issueで実装する。
// 認証ヘッダーやcodeをログに出力しない。
createServer((request, response) => {
  response.setHeader("Content-Type", "application/json");
  if (request.method === "GET" && request.url === "/health") {
    response.writeHead(200).end(JSON.stringify({ status: "scaffold" }));
    return;
  }
  response.writeHead(501).end(
    JSON.stringify({
      error: {
        code: "not_implemented",
        message: "OAuth behavior must be implemented in the authentication issue",
      },
    }),
  );
}).listen(80, "0.0.0.0");
