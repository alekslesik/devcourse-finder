# Skypro React curriculum source

Epic: #170. The published React course's official product/payment contract
passes, but the generic course-text reader originally returned zero bytes:
Skypro's configured outcome heading belonged to Python, and its React curriculum
uses nested Tilda molecules. Reading only one heading record loses the modules;
deleting all molecules loses the lesson descriptions.

The additional reader is limited to the observed `react-developer` identity and
runs only after canonical/title/language/product/offer binding checks. It locates
one non-advertising `Программа курса` H2 in a Tilda record and reads adjacent
module records until the next H2 or the captured consultation CTA. It requires
an explicit ending boundary, at least one module, at most 24 records and 24,000
bytes. Duplicate headings, missing boundaries, unrelated record roots and changed
product mappings fail closed. Forms, scripts, styles and related-course content
are excluded. Salary, payment and resume sections do not enter this curriculum.

Skypro course documents are explicitly partial: selected learning content cannot
attest disappearance of previously verified prerequisites or conditions. The
existing primary offer document and native enrollment/pricing protections are
unchanged; no monthly payment becomes a total price or study duration.

The fixture is a source-derived projection with original-response SHA-256,
current native identity/payment fields and one nested module. It is not the full
public response, a data import or a model accuracy benchmark. Positive/negative
scope tests and PostgreSQL cache/queue/publication/identity controls exercise it.

Anonymous opt-in preparation makes no model calls or production writes:

```sh
cd backend
AI_SKYPRO_CURRENT_SOURCE_CHECK=1 go test ./updater -run '^TestSkyproCurrentOfficialCurriculumSource$' -count=1 -v
```

The 12 November 2026 full-source diagnostic prepared 15,885 bytes of course text
and one separately scoped offer document. This diagnostic derives a binding from
the native source; post-deployment `catalog-ai sources skypro-react-developer`
must also check the saved production mapping. Paid semantic acceptance and any
actual publication remain separate, under the existing 200-request daily cap.
