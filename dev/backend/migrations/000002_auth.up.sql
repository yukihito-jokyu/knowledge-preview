CREATE TABLE auth_users (
 github_id text PRIMARY KEY,
 display_name text NOT NULL,
 email text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 last_login_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE auth_oauth_states (
 state_hash text PRIMARY KEY,
 verifier_hash text NOT NULL,
 return_to text NOT NULL,
 expires_at timestamptz NOT NULL,
 used_at timestamptz
);
CREATE INDEX auth_oauth_states_expiry ON auth_oauth_states (expires_at);

CREATE TABLE auth_refresh_families (
 family_id uuid PRIMARY KEY,
 session_id uuid NOT NULL,
 github_id text NOT NULL REFERENCES auth_users(github_id),
 revoked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_refresh_families_session ON auth_refresh_families (session_id);

CREATE TABLE auth_refresh_sessions (
 token_hash text PRIMARY KEY,
 session_id uuid NOT NULL,
 family_id uuid NOT NULL REFERENCES auth_refresh_families(family_id),
 github_id text NOT NULL REFERENCES auth_users(github_id),
 expires_at timestamptz NOT NULL,
 revoked_at timestamptz,
 replaced_by_hash text,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_refresh_active ON auth_refresh_sessions (session_id, github_id) WHERE revoked_at IS NULL;
CREATE INDEX auth_refresh_family ON auth_refresh_sessions (family_id);

CREATE TABLE auth_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 event text NOT NULL,
 github_id text,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_audit_expiry ON auth_audit (created_at);
