#!/usr/bin/env python3
"""Build the terminal-bible SQLite database from KJV source JSON.

Reproducible builder: reads data/kjv.json (UTF-8 BOM tolerant) and produces a
SQLite database with a `verses` table plus an FTS5 virtual table, matching the
schema expected by `tbible`.

Usage:
    build_db.py [--json data/kjv.json] [--db /path/to/bible.db]

Output defaults to $DB_FILE, otherwise $XDG_DATA_HOME/terminal-bible/bible.db,
otherwise ~/.local/share/terminal-bible/bible.db.
"""

import argparse
import json
import os
import sqlite3
import sys

SCHEMA = """
CREATE TABLE IF NOT EXISTS verses (
    book    TEXT,
    chapter INTEGER,
    verse   INTEGER,
    text    TEXT
);
CREATE VIRTUAL TABLE IF NOT EXISTS verses_fts USING fts5(book, chapter, verse, text);
"""


def default_db_path():
    env = os.environ.get("DB_FILE")
    if env:
        return env
    data_home = os.environ.get("XDG_DATA_HOME") or os.path.expanduser("~/.local/share")
    return os.path.join(data_home, "terminal-bible", "bible.db")


def main():
    parser = argparse.ArgumentParser(description="Build the tBible SQLite database.")
    parser.add_argument("--json", default=None, help="Path to KJV source JSON")
    parser.add_argument("--db", default=None, help="Path to output SQLite database")
    args = parser.parse_args()

    json_path = args.json or os.path.join(
        os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "data", "kjv.json"
    )
    db_path = args.db or default_db_path()

    if not os.path.exists(json_path):
        sys.exit(f"error: source JSON not found: {json_path}")

    with open(json_path, encoding="utf-8-sig") as fh:
        books = json.load(fh)

    if os.path.dirname(db_path):
        os.makedirs(os.path.dirname(db_path), exist_ok=True)

    conn = sqlite3.connect(db_path)
    cur = conn.cursor()

    cur.execute("DROP VIEW IF EXISTS verses_fts")
    cur.execute("DROP TABLE IF EXISTS verses_fts")
    cur.execute("DROP TABLE IF EXISTS verses")
    cur.executescript(SCHEMA)

    total = 0
    for book in books:
        name = book["name"]
        for chapter_no, verses in enumerate(book["chapters"], start=1):
            for verse_no, text in enumerate(verses, start=1):
                cur.execute(
                    "INSERT INTO verses (book, chapter, verse, text) VALUES (?, ?, ?, ?)",
                    (name, chapter_no, verse_no, text),
                )
                cur.execute(
                    "INSERT INTO verses_fts (book, chapter, verse, text) VALUES (?, ?, ?, ?)",
                    (name, chapter_no, verse_no, text),
                )
                total += 1

    conn.commit()
    conn.execute("PRAGMA optimize")
    conn.commit()
    conn.close()

    print(f"books: {len(books)}, verses: {total}")
    print(f"db written: {db_path}")


if __name__ == "__main__":
    main()