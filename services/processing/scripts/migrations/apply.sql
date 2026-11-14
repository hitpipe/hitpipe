\set ON_ERROR_STOP on
-- The migrator is the only runtime client allowed to SET ROLE to schema owner.
SET ROLE hp_processing_owner;
SELECT pg_advisory_lock(760001);
SELECT to_regclass('hp_processing.schema_version') IS NOT NULL AS installed \gset
\if :installed
 SELECT count(*)=1 AND bool_and(version=1) AS supported FROM hp_processing.schema_version \gset
 \if :supported
  \echo ALREADY_INSTALLED processing schema version 1
 \else
  \echo UNSUPPORTED processing schema version
  \quit 1
 \endif
\else
 \ir 001_processing.sql
 \echo INSTALLED processing schema version 1
\endif
SELECT pg_advisory_unlock(760001);
RESET ROLE;
