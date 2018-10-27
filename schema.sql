-- sqlite3 database schema.

DROP TABLE IF EXISTS orders;

CREATE TABLE IF NOT EXISTS orders (
    id INTEGER NOT NULL PRIMARY KEY,
    distance REAL,
    status TEXT NOT NULL
);
