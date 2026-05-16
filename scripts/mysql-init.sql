-- Runs on every MySQL node at first startup.
-- Creates the default schema and enables full-text search settings.

CREATE DATABASE IF NOT EXISTS `distdb`;
USE `distdb`;

-- Allow large packets for replication payloads
SET GLOBAL max_allowed_packet = 67108864;

-- Tune InnoDB for low-latency writes
SET GLOBAL innodb_flush_log_at_trx_commit = 2;