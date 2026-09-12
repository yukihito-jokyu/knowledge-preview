import type { Page } from "@playwright/test";
import type { FixtureUser } from "./types";

export type AuthenticationInput = { page: Page; user: FixtureUser };

// 対応Issueで認可画面 → 実callback → 認証済み画面を実装する。
// Cookie注入・認証バイパス・storage stateの保存で代替しない。
export function authenticate(input: AuthenticationInput): Promise<void> {
  if (input.page.isClosed()) {
    return Promise.reject(new Error("Authentication requires an open browser page"));
  }
  return Promise.reject(
    new Error(
      "NOT_IMPLEMENTED: define OAuth flow and identity mapping in the authentication issue",
    ),
  );
}
