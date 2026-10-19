# Delivery TargetReadback module

This service-local document materializes the candidate contract from HIT-42 for the
reusable internal/domain/readback module. The domain package is pure: it accepts a
pinned expectation and a saved observation, returns a typed result, and performs no
network, provider write, state transition, or implicit retry.

The delivery-contractcheck command is a synthetic diagnostic consumer of the same
package. It has no token input, arbitrary fetch URL, or provider POST path. It is not
a production transport and does not prove live destination permissions or external state.

Verification routes: build, dependency listing, tests, static vet, dynamic fixture
smoke, profiling (not required for this pure bounded comparator), and CI. Their
results must be recorded only after the corresponding command actually runs.
Human review, merge, acceptance, and live provider validation remain outside scope.
