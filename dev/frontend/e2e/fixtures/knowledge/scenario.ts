import { createFixtureUsers } from "../auth/users";
import type { ScenarioInput } from "./types";

// namespaceは実行IDとテストIDから生成し、テスト間でデータを共有しない。
export function createScenarioInput(namespace: string): ScenarioInput {
  if (!/^[a-z0-9][a-z0-9_-]*$/.test(namespace)) {
    throw new Error(
      "Fixture namespace must contain only lowercase letters, digits, hyphens and underscores",
    );
  }
  return {
    namespace,
    users: createFixtureUsers(),
    knowledge: [
      {
        key: "public-markdown",
        ownerKey: "owner-a",
        visibility: "public",
        title: "公開Markdown",
        file: "markdown/sample.md",
      },
      {
        key: "private-html",
        ownerKey: "owner-a",
        visibility: "private",
        title: "非公開HTML",
        file: "html/sample.html",
      },
    ],
  };
}
