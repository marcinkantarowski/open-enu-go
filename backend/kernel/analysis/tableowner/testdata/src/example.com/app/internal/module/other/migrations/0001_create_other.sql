-- +goose Up
CREATE TABLE other_account (id UUID PRIMARY KEY, name TEXT NOT NULL);
