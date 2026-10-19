# Template adjustments

The four report outputs preserve the supplied title page, section order, table
structure and styles. Adjustments are limited to the current report profile and
actual evidence.

| template | minimal adjustment |
| --- | --- |
| `0302-pz01-report-template.docx` | Filled requirements/MVP/security/refinement tables; replaced stale template status with `CONTENT_READY_IMAGES_PENDING`; resolved all tokens to actual paths/SHAs or explicit human-pending fields; retained 3 text-only image placeholders. |
| `0302-pz02-report-template.docx` | Filled equivalence/boundary, decision-table, fixtures, risk, coverage and M01-M05 mutation tables from actual logs; retained hidden-suite and human-gate limitations; retained 5 text-only image placeholders. |
| `0104-pz01-report-template.docx` | Filled seven-position toolchain, source-to-runtime path, local dev/release observations and sensitivity evidence; marked invalid-config slot `NOT_APPLICABLE` because this bounded CLI owns no runtime configuration; retained 7 text-only image placeholders. |
| `0104-pz02-report-template.docx` | Filled contract, application consumer, two replaceable dependency implementations, module map, complexity directions and ADR references; retained 6 text-only image placeholders and reciprocal-review pending state. |

No screenshot, image, score, teacher signature, group, interview, review,
opposition, merge or immutable submission tag was invented. The exact template
source files and their package SHA values remain unchanged in
`docs/labs/pr12/templates/template-manifest.json`.
