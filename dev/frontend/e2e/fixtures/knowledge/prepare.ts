import type { APIRequestContext } from "@playwright/test";
import type { PreparedScenario, ScenarioInput } from "./types";

export type FixtureInput = {
  owner: "a" | "b";
  format: "markdown" | "html";
  source: string;
  ageYears?: number;
};

export type PreparedFixture = { draftId: string };

// 製品APIへseedを追加せず、テスト専用serverのfixture入口だけを利用する。
export async function prepareFixture(
  request: APIRequestContext,
  input: FixtureInput,
): Promise<PreparedFixture> {
  const response = await request.post("/api/v1/__e2e/fixtures", { data: input });
  if (response.status() !== 201) {
    throw new Error(`E2E fixture creation failed with status ${response.status()}`);
  }
  const body: unknown = await response.json();
  if (
    typeof body !== "object" ||
    body === null ||
    !("draftId" in body) ||
    typeof body.draftId !== "string"
  ) {
    throw new Error("E2E fixture response did not contain a draft id");
  }
  return { draftId: body.draftId };
}

// 旧fixture契約を使うspecへは、業務fixtureへ切り替えるよう明示的に失敗させる。
export function prepareScenario(input: ScenarioInput): Promise<PreparedScenario> {
  if (input.users.length === 0) {
    return Promise.reject(new Error("Fixture users must not be empty"));
  }
  return Promise.reject(new Error("Use prepareFixture with the test-only fixture API"));
}
