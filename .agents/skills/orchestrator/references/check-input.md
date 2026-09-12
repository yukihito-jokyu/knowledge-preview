# 任意の形式検査

Python 3標準ライブラリのみ。入力ファイルは読み取り専用。失敗時は非ゼロ終了し、planやlogsを変更しない。

```sh
python3 <skill-root>/scripts/check_plan.py <proposal.json>
python3 <skill-root>/scripts/check_plan.py --trace <events.json>
python3 -B <skill-root>/scripts/test_check_plan.py
```

## 計画入力

issue（正の整数）、requirements（id/text）、tasks（id/status/phase/skills/depends）、scenarios（id/requirements/initial/input/action/expected/method）を持つJSONオブジェクト。3配列のidはそれぞれ1からの連番。dependsは作業idの配列、scenarios.requirementsは受け入れ基準idの配列。phaseが「調査」の場合skillsは「スキルなし」。statusは計画テンプレートの作業状況値。

## 記録入力

イベントのJSON配列。workは元の実装作業ID、agentsは実際の新規agent ID。例は模擬記録であり実績を示さない。

```json
[
  {"event":"approve"},
  {"event":"implement","work":4,"agents":["implement-1"]},
  {"event":"review_pass","work":4,"agents":["review-1"]},
  {"event":"final","agents":["audit-1","debt-1"],"outcomes":{"4":"pass"}},
  {"event":"e2e","work":12,"agents":["e2e-1"]},
  {"event":"review_pass","work":12,"agents":["e2e-review-1"]}
]
```

通常レビュー不合格はreview_return。finalは同一回のaudit/debtの2担当を1回だけ記録し、全製品作業のIDをoutcomesに含め、それぞれpass/returnを指定する。異なる対象作業に同じ担当IDを重複登録しない。通常レビュー・最終チェックは各作業で別々に2回目の差し戻しでhuman状態となる。人間への報告はevent=humanとworkを記録する。

e2eは新規担当によるテスト作成・修正と実行を表し、新規review_pass/review_returnで確認する。E2Eでの製品不具合はevent=product_failure、works=元の製品作業IDの配列を記録する。同じE2E実行で判明した複数の製品不具合は1イベントへまとめる。この観測イベントは直前のE2E実行によるもので、新しいagent起動を意味しない。製品implement→review→finalを経ないE2E再試行を拒否する。

この検査は記録の整合確認に限る。実際の承認の真正性、部分再承認、未記録の担当、環境復旧、人間の追加承認後の再開は検査・制御しない。ログから記録する場合はagent IDを仮名に置き換えず、未実行イベントを追加しない。網羅的なスケジューラとして利用しない。
