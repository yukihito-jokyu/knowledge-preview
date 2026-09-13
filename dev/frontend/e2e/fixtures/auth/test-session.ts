import type { BrowserContext } from "@playwright/test";
import type { FixtureUser } from "./types";

export const actorCookieName = "__Host-kp_e2e_actor";
const appOrigin = process.env.E2E_APP_ORIGIN ?? "https://app.knowledge.test";

function actorValue(user: FixtureUser): "a" | "b" {
  return user.key === "owner-a" ? "a" : "b";
}

// 認証そのものは対象外にし、実SPA/APIへ接続するためのテスト専用sessionだけを注入する。
export async function authenticateTestSession(
  context: BrowserContext,
  user: FixtureUser,
): Promise<void> {
  await context.addCookies([
    {
      name: actorCookieName,
      value: actorValue(user),
      url: `${appOrigin}/`,
      secure: true,
      httpOnly: true,
      sameSite: "Lax",
    },
  ]);
}

export function authenticatedHeaders(user: FixtureUser): Record<string, string> {
  return {
    Cookie: `${actorCookieName}=${actorValue(user)}`,
    Origin: appOrigin,
  };
}

export async function assertCookieIsHostOnly(
  context: BrowserContext,
  user: FixtureUser,
): Promise<void> {
  const cookies = await context.cookies();
  const cookie = cookies.find((item) => item.name === actorCookieName);
  if (!cookie || cookie.domain !== "app.knowledge.test" || cookie.path !== "/") {
    throw new Error("E2E actor cookie must be host-only and scoped to /");
  }
  if (!cookie.secure || !cookie.httpOnly || cookie.sameSite !== "Lax") {
    throw new Error("E2E actor cookie attributes are unsafe");
  }
  if (cookie.value !== actorValue(user)) {
    throw new Error("E2E actor cookie value does not match the selected actor");
  }
}

export function requestOptions(user: FixtureUser) {
  return { extraHTTPHeaders: authenticatedHeaders(user) } as const;
}
