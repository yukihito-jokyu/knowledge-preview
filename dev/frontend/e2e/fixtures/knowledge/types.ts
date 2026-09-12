// DB・APIのDTOではなく、テスト準備用の入力モデル。
import type { FixtureUser } from "../auth/types";
export type FixtureKnowledge = {
  key: string;
  ownerKey: FixtureUser["key"];
  visibility: "public" | "private";
  title: string;
  file: "markdown/sample.md" | "html/sample.html";
};
export type ScenarioInput = {
  namespace: string;
  users: readonly FixtureUser[];
  knowledge: readonly FixtureKnowledge[];
};
export type PreparedScenario = {
  userIds: Readonly<Record<string, string>>;
  knowledgeIds: Readonly<Record<string, string>>;
  objectKeys: readonly string[];
};
