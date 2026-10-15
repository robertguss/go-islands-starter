-- +goose Up
ALTER TABLE app.exports ADD COLUMN job_id bigint;
GRANT UPDATE (job_id) ON app.exports TO islands_app;
ALTER TABLE app.audit_events ADD COLUMN user_id uuid REFERENCES app.users(id);
ALTER TABLE app.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE app.audit_events ADD CHECK (action IN ('request.created','request.approved','request.rejected','export.requested','export.completed','export.failed','user.updated'));
ALTER TABLE app.audit_events DROP CONSTRAINT audit_events_check;
ALTER TABLE app.audit_events ADD CHECK (num_nonnulls(request_id, export_id, user_id) = 1);
CREATE POLICY audit_admin_insert ON app.audit_events FOR INSERT WITH CHECK (actor_id = app.actor() AND user_id IS NOT NULL AND action = 'user.updated' AND app.permitted('access_admin'));
CREATE POLICY audit_admin_read ON app.audit_events FOR SELECT TO islands_app USING (user_id IS NOT NULL AND app.permitted('access_admin'));

-- Narrow privilege bridge: the runtime cannot directly change access assignments.
-- +goose StatementBegin
CREATE FUNCTION app.manage_user(address text, name text, can_request boolean, can_review boolean, can_admin boolean, is_disabled boolean, correlation text)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, app AS $$
DECLARE target uuid;
BEGIN
  IF NOT app.permitted('access_admin') THEN RAISE EXCEPTION 'access denied'; END IF;
  IF length(address) > 254 OR address !~ '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$' OR length(name) NOT BETWEEN 1 AND 120 THEN RAISE EXCEPTION 'invalid user'; END IF;
  IF EXISTS (SELECT 1 FROM app.users WHERE id = app.actor() AND email = lower(address)) THEN RAISE EXCEPTION 'cannot change own access'; END IF;
  INSERT INTO app.users(email, display_name, requester, reviewer, access_admin, disabled)
  VALUES(lower(address), name, can_request, can_review, can_admin, is_disabled)
  ON CONFLICT (email) DO UPDATE SET display_name = EXCLUDED.display_name, requester = EXCLUDED.requester, reviewer = EXCLUDED.reviewer, access_admin = EXCLUDED.access_admin, disabled = EXCLUDED.disabled
  RETURNING id INTO target;
  DELETE FROM app.sessions WHERE user_id = target;
  INSERT INTO app.audit_events(actor_id,action,user_id,correlation_id) VALUES(app.actor(),'user.updated',target,correlation);
END $$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION app.manage_user(text,text,boolean,boolean,boolean,boolean,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app.manage_user(text,text,boolean,boolean,boolean,boolean,text) TO islands_app;

-- +goose Down
DROP FUNCTION app.manage_user(text,text,boolean,boolean,boolean,boolean,text);
DROP POLICY audit_admin_read ON app.audit_events;
ALTER TABLE app.audit_events DROP COLUMN user_id CASCADE;
ALTER TABLE app.exports DROP COLUMN job_id;
