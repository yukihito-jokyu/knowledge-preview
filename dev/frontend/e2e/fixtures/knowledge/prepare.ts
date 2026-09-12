import type { PreparedScenario, ScenarioInput } from "./types";

// 対応Issueで、テスト専用initサービスへの受渡しと結果の読取りを実装する。
// 製品APIへのseedエンドポイント追加や、ブラウザからのDB接続は行わない。
export function prepareScenario(input: ScenarioInput): Promise<PreparedScenario> {
  if (input.users.length === 0) {
    return Promise.reject(new Error("Fixture users must not be empty"));
  }
  return Promise.reject(
    new Error(
      "NOT_IMPLEMENTED: define business migration and fixture persistence in the feature issue",
    ),
  );
}
