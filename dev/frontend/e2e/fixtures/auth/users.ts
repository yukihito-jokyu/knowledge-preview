import type { FixtureUser } from "./types";

export function createFixtureUsers(): FixtureUser[] {
  return [
    { key: "owner-a", displayName: "所有者A" },
    { key: "other-b", displayName: "別ユーザーB" },
  ];
}
