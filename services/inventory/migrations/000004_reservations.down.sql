-- Migration 000004 (down): drop reservations

DROP TABLE IF EXISTS reservation_items;
DROP TABLE IF EXISTS reservations;
