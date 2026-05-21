CREATE TABLE users (
	id BIGSERIAL PRIMARY KEY,
	username TEXT NOT NULL UNIQUE,
	user_slug TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sessions (
	id BIGSERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	token_hash TEXT NOT NULL UNIQUE,
	expires_at TIMESTAMPTZ NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);

CREATE TABLE service_group_bindings (
	id BIGSERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
	service_group_name TEXT NOT NULL,
	encrypted_authorization_code TEXT NOT NULL,
	salt TEXT NOT NULL,
	algorithm TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE routes (
	id BIGSERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	enabled BOOLEAN NOT NULL DEFAULT TRUE,
	access_mode TEXT NOT NULL DEFAULT 'public' CHECK (access_mode IN ('public', 'caller_token', 'signed_link')),
	match_type TEXT NOT NULL CHECK (match_type IN ('prefix', 'exact')),
	path_pattern TEXT NOT NULL,
	methods TEXT[] NOT NULL DEFAULT ARRAY['ALL']::TEXT[],
	upstream_url TEXT NOT NULL,
	strip_prefix BOOLEAN NOT NULL DEFAULT FALSE,
	timeout_seconds INTEGER NOT NULL DEFAULT 30 CHECK (timeout_seconds > 0),
	retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
	priority INTEGER NOT NULL DEFAULT 0,
	auth_required BOOLEAN NOT NULL DEFAULT FALSE,
	auth_service_name TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT routes_auth_service_required CHECK (auth_required = FALSE OR auth_service_name <> ''),
	CONSTRAINT routes_access_mode_auth_service_required CHECK (access_mode = 'public' OR auth_service_name <> '')
);

CREATE INDEX routes_user_enabled_idx ON routes(user_id, enabled);
CREATE INDEX routes_user_path_idx ON routes(user_id, path_pattern);

CREATE TABLE route_header_rules (
	id BIGSERIAL PRIMARY KEY,
	route_id BIGINT NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
	phase TEXT NOT NULL CHECK (phase IN ('request', 'response')),
	operation TEXT NOT NULL CHECK (operation IN ('set', 'remove')),
	header_name TEXT NOT NULL,
	header_value TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX route_header_rules_route_id_idx ON route_header_rules(route_id);
