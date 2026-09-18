ALTER TABLE session_event_versions
ADD COLUMN permission_policy TEXT NOT NULL DEFAULT 'ask'
CHECK (permission_policy IN ('ask', 'approveAll'));
