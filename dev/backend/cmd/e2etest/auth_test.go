package main

import "testing"

func TestRequireTestEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("E2E_RUN_ID", "run")

	if err := requireTestEnvironment(); err == nil {
		t.Fatal("development must not start the E2E server")
	}

	t.Setenv("APP_ENV", "test")
	t.Setenv("E2E_RUN_ID", "")

	if err := requireTestEnvironment(); err == nil {
		t.Fatal("an empty E2E_RUN_ID must not start the E2E server")
	}
}

func TestActorsUseTheSharedCookieContract(t *testing.T) {
	actors := newActors()
	for cookie, want := range map[string]struct {
		owner   string
		session string
	}{
		"a": {owner: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", session: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaab"},
		"b": {owner: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", session: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbc"},
	} {
		got, ok := actors.byCookie[cookie]
		if !ok || got.OwnerID != want.owner || got.SessionID != want.session {
			t.Fatalf("actor %q = %#v, want owner/session %q/%q", cookie, got, want.owner, want.session)
		}
	}
}
