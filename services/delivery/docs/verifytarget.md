# VerifyTarget application module

VerifyTarget owns the application use case and depends on one consumer-owned
TargetReader port. The file-backed adapter and the test double both implement that
port. The consumer only receives a saved Observation and provenance; it never writes
provider state, emits a durable message, retries, or concludes that an absent target
proves no effect.

A read error is returned as unknown. The returned provenance is preserved only when it
is available from the adapter. This module is a candidate implementation for HIT-44;
human review, CI and merge remain pending.
