ALTER TABLE routes
	ADD COLUMN access_mode TEXT NOT NULL DEFAULT 'public';

UPDATE routes
SET access_mode = CASE
	WHEN auth_required THEN 'caller_token'
	ELSE 'public'
END;

ALTER TABLE routes
	ADD CONSTRAINT routes_access_mode_check CHECK (access_mode IN ('public', 'caller_token', 'signed_link'));

ALTER TABLE routes
	ADD CONSTRAINT routes_access_mode_auth_service_required CHECK (access_mode = 'public' OR auth_service_name <> '');
