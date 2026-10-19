# HIT-50 / NB-R2 — ambiguity decisions and blind-check handoff

Status: `NOT_SENT_TO_OTHER_STUDENT` for the human clarity gate. The following
decision record is the agent's bounded refinement; no independent student's
answer or opposition is invented.

## Decisions for the candidate contract

| ID | question | local decision | unresolved human/product point |
| --- | --- | --- | --- |
| A-01 | Which failure is reported when secret, clock, body and header are all bad? | secret/clock, then empty body, then header, then freshness/MAC; all reasons are safe | confirm operator-facing wording with product/Lead |
| A-02 | May duplicate timestamps differ? | no; equal duplicates are allowed, differing values reject independent of order | provider compatibility confirmation needed |
| A-03 | May v0 verify when v1 is absent? | no; v1 is required even when v0 is well formed | confirm provider migration policy |
| A-04 | Is 300 seconds inclusive? | yes, absolute distance 300 passes and 301 fails | none in local candidate; human review still required |
| A-05 | What does verified mean for the next component? | only authenticated raw bytes plus bounded metadata; no acceptance/receipt/payment claim | define future receipt/2xx contract outside this task |

## Blind clarity-check packet for another student

Lead should send the following without this decision table and record the real
answers, date, participant and discrepancies in the human gate record:

1. Given a valid body and two identical `t` values, should it verify? What if the
   second `t` differs by one second?
2. Given a valid v0 signature but no v1, should it verify?
3. Given a signature age of +300, -300, +301 and -301 seconds, list the four
   outcomes.
4. If secret lookup fails and the body is empty, which safe reason has priority?
5. Does `VERIFIED` authorize a receipt, HTTP 2xx or payment decision here?

Human completion fields (must not be filled by the agent):

```text
CHECK_STATUS: NOT_CONDUCTED
PARTICIPANT: [human fills]
DATE_TIME_TZ: [human fills]
RAW_ANSWERS: [human fills]
MISUNDERSTANDINGS_FOUND: [human fills]
REFINEMENT_FOLLOW_UP: [human fills]
EVIDENCE_LINK_OR_FILE: [human fills]
```

## Conflict route

If the blind answers conflict with the local refinement or with the pinned
Candidate clause, Lead records the exact clause and opens a new refinement or
escalates it. This package must not silently change canonical requirements,
formal status, ownership, or production behavior.
