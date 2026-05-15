# Post-Hardening — Data Model

## Phase Contract

Input: plan.md.
Output: no-change stub.
Stop if: data model changes are required — not applicable.

## Status: NO CHANGE

All data model entities (sessions, IP pool, BoltDB schema, routing rules, config) remain unchanged.
This spec addresses only technical debt — no new entities, fields, or migrations.
Exception: WSConfig gets an optional `WriteLimit int` field (0 = unlimited, backward compatible).
