-- Trigram indexes for element search (SE-1, ADR-0003).
-- pg_trgm is a trusted extension: the database owner can enable it.

-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;
