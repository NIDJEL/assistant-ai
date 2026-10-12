CREATE TABLE IF NOT EXISTS events (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 kind TEXT NOT NULL CHECK(kind IN ('task','meeting','reminder')),
 title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 500),
 date TEXT, time TEXT,
 duration_minutes INTEGER NOT NULL DEFAULT 60 CHECK(duration_minutes BETWEEN 5 AND 1440),
 status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','done')),
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, source_key TEXT UNIQUE
);
CREATE INDEX IF NOT EXISTS events_period ON events(date,time);
CREATE INDEX IF NOT EXISTS events_kind_status ON events(kind,status);
CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY,state TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS exchanges (
 id INTEGER PRIMARY KEY AUTOINCREMENT,session_id TEXT NOT NULL,request_id TEXT NOT NULL,
 message TEXT NOT NULL,response TEXT NOT NULL,created_at TEXT NOT NULL,UNIQUE(session_id,request_id)
);
CREATE INDEX IF NOT EXISTS exchanges_session ON exchanges(session_id,id);
