import type { APIRequestContext } from "@playwright/test";
import { authenticatedHeaders } from "../auth/test-session";
import type { FixtureUser } from "../auth/types";
import type { PreparedScenario, ScenarioInput } from "./types";

export type FixtureInput = {
  owner: "a" | "b";
  format: "markdown" | "html";
  source: string;
  ageYears?: number;
};

export type PreparedFixture = { draftId: string };

export type PublishedFixture = {
  knowledgeId: string;
  publicId: string;
  version: number;
  publicUrl: string;
};

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

export async function commitAndPublish(
  request: APIRequestContext,
  user: FixtureUser,
  input: FixtureInput,
): Promise<PublishedFixture> {
  const fixture = await prepareFixture(request, input);
  const headers = authenticatedHeaders(user);

  const commit = await request.post(`/api/v1/knowledge-drafts/${fixture.draftId}/commit`, {
    headers,
    data: { version: 1, source: input.source },
  });

  if (commit.status() !== 201) throw new Error(`E2E commit failed with status ${commit.status()}`);
  const committed = (await commit.json()) as { knowledgeId?: unknown };
  if (typeof committed.knowledgeId !== "string") throw new Error("E2E commit returned no id");

  const publish = await request.put(`/api/v1/knowledge/${committed.knowledgeId}/visibility`, {
    headers,
    data: { version: 1, visibility: "unlisted" },
  });

  if (publish.status() !== 200)
    throw new Error(`E2E publish failed with status ${publish.status()}`);

  const body = (await publish.json()) as {
    publicId?: unknown;
    publicUrl?: unknown;
    version?: unknown;
  };

  if (
    typeof body.publicId !== "string" ||
    typeof body.publicUrl !== "string" ||
    typeof body.version !== "number"
  )
    throw new Error("E2E publish returned an incomplete public result");
  return {
    knowledgeId: committed.knowledgeId,
    publicId: body.publicId,
    version: body.version,
    publicUrl: body.publicUrl,
  };
}

// 旧fixture契約を使うspecへは、業務fixtureへ切り替えるよう明示的に失敗させる。
export function prepareScenario(input: ScenarioInput): Promise<PreparedScenario> {
  if (input.users.length === 0) {
    return Promise.reject(new Error("Fixture users must not be empty"));
  }
  return Promise.reject(new Error("Use prepareFixture with the test-only fixture API"));
}
