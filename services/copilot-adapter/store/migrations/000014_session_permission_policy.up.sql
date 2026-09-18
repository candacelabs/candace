ALTER TABLE sessions
ADD COLUMN permission_policy TEXT NOT NULL DEFAULT 'ask'
CHECK (permission_policy IN ('ask', 'approveAll'));

ALTER TABLE session_creations
ADD COLUMN permission_policy TEXT NOT NULL DEFAULT 'ask'
CHECK (permission_policy IN ('ask', 'approveAll'));
