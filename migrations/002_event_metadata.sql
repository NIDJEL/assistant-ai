ALTER TABLE events ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN priority TEXT NOT NULL DEFAULT 'normal' CHECK(priority IN ('normal','low','high'));
ALTER TABLE events ADD COLUMN external_calendar_id TEXT;
