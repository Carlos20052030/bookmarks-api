-- Down migration for 002_bookmarks.up.sql
DROP TABLE IF EXISTS bookmark_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS bookmarks;