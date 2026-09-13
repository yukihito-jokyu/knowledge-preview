// migrationとbucketはテスト専用Go serverの起動時に準備する。
// 各テストは同serverのfixture APIへ必要な入力だけを送り、テスト間でデータを共有しない。
console.log("Business fixtures are created by Playwright through the test-only fixture API.");
