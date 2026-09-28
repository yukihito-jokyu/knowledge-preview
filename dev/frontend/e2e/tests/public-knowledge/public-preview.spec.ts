import { expect, test } from "../../fixtures/test";
import { assertCookieIsHostOnly, authenticateTestSession } from "../../fixtures/auth/test-session";
import type { FixtureUser } from "../../fixtures/auth/types";
import { commitAndPublish } from "../../fixtures/knowledge/prepare";
import { clearPublicReadFault, failNextPublicRead } from "../../fixtures/knowledge/test-controls";

const ownerA: FixtureUser = { key: "owner-a", displayName: "所有者A" };
const appOrigin = process.env.E2E_APP_ORIGIN ?? "https://app.knowledge.test";
const previewOrigin = process.env.E2E_PREVIEW_ORIGIN ?? "https://preview.knowledge.test";

// Issue #17 シナリオ1・5：未認証の公開Markdownを利用者経路で表示し、共有・目次・関連記事を確認する。
test("公開Markdownは未認証で表示され、共有・目次・関連記事を操作できる", async ({
  page,
  request,
}, testInfo) => {
  const relatedTitle = `関連記事-${testInfo.project.name}`;

  const source = [
    "---",
    "title: 公開画面記事",
    "tags:",
    "  - shared",
    "---",
    "",
    "# 本文見出し",
    "",
    "概要として表示される本文です。",
    "",
    "## 詳細見出し",
    "",
    "> 重要な要点",
    "",
    "<script>window.__e2e = true</script>",
    "",
    "```ts",
    'const code = "横スクロール対象";',
    "```",
  ].join("\n");

  const published = await commitAndPublish(request, ownerA, {
    owner: "a",
    format: "markdown",
    source,
  });

  const related = await commitAndPublish(request, ownerA, {
    owner: "a",
    format: "markdown",
    source: source.replace("公開画面記事", relatedTitle).replace("本文見出し", "関連見出し"),
  });

  expect(await page.context().cookies()).toHaveLength(0);
  await page.setViewportSize({ width: 320, height: 800 });
  await page.goto(published.publicUrl);

  await expect(page.getByRole("heading", { name: "公開画面記事", exact: true })).toBeVisible();
  await expect(
    page.locator(".knowledge-markdown").getByText("概要として表示される本文です。"),
  ).toBeVisible();
  await expect(page.getByRole("list", { name: "タグ" })).toContainText("#shared");
  await expect(page.locator(".knowledge-markdown script")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "関連記事", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: relatedTitle, exact: true })).toHaveAttribute(
    "href",
    `/public/knowledge/${related.publicId}`,
  );

  const tableOfContents = page.getByRole("navigation", { name: "このページ" });
  await expect(
    tableOfContents.getByRole("link", { name: "本文見出し", exact: true }),
  ).toBeVisible();
  await expect(
    tableOfContents.getByRole("link", { name: "詳細見出し", exact: true }),
  ).toBeVisible();

  const detailHeadingLink = tableOfContents.getByRole("link", {
    name: "詳細見出し",
    exact: true,
  });

  const detailHeadingHash = await detailHeadingLink.getAttribute("href");
  expect(detailHeadingHash).toMatch(/^#heading-\d+$/);
  await detailHeadingLink.focus();
  await expect(detailHeadingLink).toBeFocused();
  await detailHeadingLink.press("Enter");
  await expect(page).toHaveURL(new RegExp(`${detailHeadingHash!.replace("#", "\\#")}$`));
  await expect(page.locator(detailHeadingHash!)).toBeVisible();

  const shareInput = page.getByRole("textbox", { name: "共有URL" });
  await expect(shareInput).toHaveValue(published.publicUrl);
  const shareButton = page.getByRole("button", { name: "共有", exact: true });
  await shareButton.focus();
  await expect(shareButton).toBeFocused();
  await shareButton.press("Enter");
  await expect(page.getByText(/共有URLをコピー/)).toBeVisible();

  const relatedLink = page.getByRole("link", { name: relatedTitle, exact: true });
  await relatedLink.focus();
  await expect(relatedLink).toBeFocused();
  await Promise.all([page.waitForURL(related.publicUrl), relatedLink.press("Enter")]);
  await expect(page.getByRole("heading", { name: relatedTitle, exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "関連見出し", exact: true })).toBeVisible();
  await expect(
    page.locator(".knowledge-markdown").getByText("概要として表示される本文です。"),
  ).toBeVisible();

  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(320);
});

// Issue #17 シナリオ2：HTMLは別originの空sandbox iframeへ配信し、認証Cookieを送らない。
test("公開HTMLはCookieなし新規contextにも表示され、previewへ認証Cookieを送らない", async ({
  page,
  request,
}) => {
  const source =
    '<h1>公開HTML本文</h1><script src="https://evil.example/steal.js"></script><button onclick="window.top.location=\'https://evil.example\'">操作</button>';

  const published = await commitAndPublish(request, ownerA, {
    owner: "a",
    format: "html",
    source,
  });

  await authenticateTestSession(page.context(), ownerA);
  await assertCookieIsHostOnly(page.context(), ownerA);

  const externalRequests: string[] = [];
  page.on("request", (event) => {
    if (event.url().includes("evil.example")) externalRequests.push(event.url());
  });

  const appRequest = page.waitForRequest((event) =>
    event.url().includes(`/api/v1/public/knowledge/${published.publicId}`),
  );

  const previewRequest = page.waitForRequest(
    (event) =>
      event.url() ===
      `${previewOrigin}/public/${published.publicId}/html?version=${published.version}`,
  );

  const previewResponse = page.waitForResponse((response) =>
    response.url().includes(`/public/${published.publicId}/html?version=${published.version}`),
  );

  await page.goto(published.publicUrl);
  const iframe = page.locator('iframe[title="公開HTML本文"]');
  await expect(iframe).toHaveAttribute(
    "src",
    `${previewOrigin}/public/${published.publicId}/html?version=${published.version}`,
  );
  await expect(iframe).toHaveAttribute("sandbox", "");
  await expect(iframe).toHaveAttribute("referrerpolicy", "no-referrer");
  await expect(iframe).not.toHaveAttribute("allow", /.+/);
  await expect(iframe).not.toHaveAttribute("srcdoc", /.+/);

  const [app, preview, response] = await Promise.all([appRequest, previewRequest, previewResponse]);
  const appHeaders = await app.allHeaders();
  const previewHeaders = await preview.allHeaders();
  expect(appHeaders.cookie).toContain("__Host-kp_e2e_actor=a");
  expect(response.status()).toBe(200);
  expect(response.headers()["cache-control"]).toBe("no-store");
  expect(response.headers()["referrer-policy"]).toBe("no-referrer");
  expect(response.headers()["x-content-type-options"]).toBe("nosniff");
  expect(response.headers()["content-security-policy"]).toContain("script-src 'none'");
  await expect(page.frameLocator('iframe[title="公開HTML本文"]').locator("body")).toContainText(
    "公開HTML本文",
  );
  await expect(page.frameLocator('iframe[title="公開HTML本文"]').locator("script")).toHaveCount(0);
  await expect(page.frameLocator('iframe[title="公開HTML本文"]').locator("[onclick]")).toHaveCount(
    0,
  );
  expect(previewHeaders.cookie).toBeUndefined();
  expect(externalRequests).toEqual([]);
});

// Issue #17 シナリオ2：テストごとに分離されたCookieなしcontextでもHTMLを表示する。
test("公開HTMLは新規未認証contextから表示できる", async ({ page, request }) => {
  const published = await commitAndPublish(request, ownerA, {
    owner: "a",
    format: "html",
    source: "<h1>未認証HTML本文</h1>",
  });

  await expect(page.context().cookies()).resolves.toHaveLength(0);
  await page.goto(published.publicUrl);
  await expect(page.locator('iframe[title="公開HTML本文"]')).toHaveAttribute(
    "src",
    `${previewOrigin}/public/${published.publicId}/html?version=${published.version}`,
  );
  await expect(page.frameLocator('iframe[title="公開HTML本文"]').locator("body")).toContainText(
    "未認証HTML本文",
  );
});

// Issue #17 シナリオ3：更新・停止・同一publicId再公開後の再取得を画面で確認する。
test("公開画面は更新後の現在版だけを表示し、停止後は隠れ、同じURLの再公開で戻る", async ({
  page,
  request,
}) => {
  const published = await commitAndPublish(request, ownerA, {
    owner: "a",
    format: "markdown",
    source: "# ライフサイクル\n\n旧本文です。",
  });

  await expect(page.context().cookies()).resolves.toHaveLength(0);
  await page.goto(published.publicUrl);
  await expect(page.locator(".knowledge-markdown").getByText("旧本文です。")).toBeVisible();

  const update = await request.put(`/api/v1/knowledge/${published.knowledgeId}`, {
    headers: { Cookie: "__Host-kp_e2e_actor=a", Origin: appOrigin },
    data: { version: published.version, source: "# ライフサイクル\n\n新版本文です。" },
  });

  expect(update.status()).toBe(200);
  const updated = (await update.json()) as { version?: unknown };
  expect(updated.version).toBe(3);

  const reloadResponse = page.waitForResponse(
    (event) =>
      event.request().method() === "GET" &&
      event.url().includes(`/api/v1/public/knowledge/${published.publicId}`),
  );

  await page.reload();
  await reloadResponse;
  await expect(page.locator(".knowledge-markdown").getByText("新版本文です。")).toBeVisible();
  await expect(page.locator(".knowledge-markdown").getByText("旧本文です。")).toHaveCount(0);

  const stop = await request.put(`/api/v1/knowledge/${published.knowledgeId}/visibility`, {
    headers: { Cookie: "__Host-kp_e2e_actor=a", Origin: appOrigin },
    data: { version: 3, visibility: "private" },
  });

  expect(stop.status()).toBe(200);
  await page.reload();
  await expect(page.getByRole("heading", { name: "この知識は閲覧できません" })).toBeVisible();

  const reopen = await request.put(`/api/v1/knowledge/${published.knowledgeId}/visibility`, {
    headers: { Cookie: "__Host-kp_e2e_actor=a", Origin: appOrigin },
    data: { version: 4, visibility: "unlisted" },
  });

  expect(reopen.status()).toBe(200);
  await page.reload();
  await expect(page.locator(".knowledge-markdown").getByText("新版本文です。")).toBeVisible();
  await expect(page).toHaveURL(published.publicUrl);
});

// Issue #17 シナリオ3：停止後の新規未認証contextは公開本文を表示しない。
test("停止後の公開URLは新規未認証contextから拒否される", async ({ page, request }) => {
  const published = await commitAndPublish(request, ownerA, {
    owner: "a",
    format: "markdown",
    source: "# 停止後の新規context\n\n停止後は表示されない本文です。",
  });

  const stop = await request.put(`/api/v1/knowledge/${published.knowledgeId}/visibility`, {
    headers: { Cookie: "__Host-kp_e2e_actor=a", Origin: appOrigin },
    data: { version: published.version, visibility: "private" },
  });

  expect(stop.status()).toBe(200);
  await expect(page.context().cookies()).resolves.toHaveLength(0);
  await page.goto(published.publicUrl);
  await expect(page.getByRole("heading", { name: "この知識は閲覧できません" })).toBeVisible();
  await expect(page.getByText("停止後は表示されない本文です。")).toHaveCount(0);
});

// Issue #17 シナリオ4：公開JSONの503を画面の再試行で回復する。
test("公開JSONが503になっても再試行で公開本文を表示する", async ({ page, request }) => {
  const published = await commitAndPublish(request, ownerA, {
    owner: "a",
    format: "markdown",
    source: "# 再試行記事\n\n再試行で表示される本文です。",
  });

  await expect(page.context().cookies()).resolves.toHaveLength(0);
  await clearPublicReadFault(request, ownerA, "public");
  try {
    await failNextPublicRead(request, ownerA, "public");

    await page.goto(published.publicUrl);
    await expect(page.getByRole("heading", { name: "読み込みに失敗しました" })).toBeVisible();
    await expect(page.getByText("再試行で表示される本文です。")).toHaveCount(0);
    await page.getByRole("button", { name: "再試行", exact: true }).click();
    await expect(page.getByRole("heading", { name: "再試行記事", exact: true })).toBeVisible();
    await expect(
      page.locator(".knowledge-markdown").getByText("再試行で表示される本文です。"),
    ).toBeVisible();
  } finally {
    await clearPublicReadFault(request, ownerA, "public");
  }
});
