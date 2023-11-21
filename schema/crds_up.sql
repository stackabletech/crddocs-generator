CREATE TABLE tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    repo TEXT NOT NULL,
    time TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
    UNIQUE(name, repo)
);

CREATE TABLE crds (
    "group" TEXT NOT NULL,
    version TEXT NOT NULL,
    kind TEXT NOT NULL,
    tag_id INTEGER NOT NULL,
    filename TEXT NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY(tag_id, "group", version, kind),
    FOREIGN KEY (tag_id) REFERENCES tags (id) ON DELETE CASCADE
);