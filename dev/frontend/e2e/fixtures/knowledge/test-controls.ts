import type { APIRequestContext } from "@playwright/test";
import { authenticatedHeaders } from "../auth/test-session";
import type { FixtureUser } from "../auth/types";

export type UploadFault = "database" | "object";

export async function failNextUpload(
  request: APIRequestContext,
  user: FixtureUser,
  kind: UploadFault,
) {
  const response = await request.post("/api/v1/__e2e/faults", {
    headers: authenticatedHeaders(user),
    data: { kind, count: 1 },
  });

  if (response.status() !== 204) throw new Error(`E2E fault setup failed: ${response.status()}`);
}

export async function draftState(request: APIRequestContext, user: FixtureUser) {
  const response = await request.get(
    `/api/v1/__e2e/draft-state?owner=${user.key === "owner-a" ? "a" : "b"}`,
    {
      headers: authenticatedHeaders(user),
    },
  );

  if (response.status() !== 200) throw new Error(`E2E draft state failed: ${response.status()}`);
  return (await response.json()) as { pending: number; total: number };
}
