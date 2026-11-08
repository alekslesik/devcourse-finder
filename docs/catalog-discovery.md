# Diverse course discovery

The website defaults to `sort=discovery` and requests 18 courses per page. The public
API keeps its existing omitted-sort behaviour (recent data) and 12-item default;
clients can opt into discovery and any supported page size explicitly. Existing
price, duration and recent sorts retain their ordering semantics.

Discovery runs after all hard filters, exact-tariff selection and confidence
classification, over the complete result set before pagination. It never adds,
removes or repeats a course. Confirmed, advisory and incomplete-profile groups
remain in that order. An AI-subject quota cannot cross those confidence groups
or override language, budget, workload, enrollment or human-support requirements.
Page number and page size do not change the ranking, so later pages continue the
same order without duplicates.

Within each group, choose the least represented school, breaking ties by the
maintainer's priority: Practicum, OTUS, Hexlet and Stepik, followed by the other
connected schools. Normalize official platform domains and CodeBasics name
aliases so renamed labels do not gain extra places. Within each school's queue,
retain the existing freshness/date/course-ID order.

Reserve positions 3, 6, 9, ... for courses whose official title or explicitly listed
curriculum topics identify AI/ML/LLM subjects. Use ordinary courses elsewhere
while available. Choose the least represented eligible school in either queue.
If a queue is exhausted, use the other queue without duplication or filter
relaxation. Sparse filtered catalogs and confidence boundaries can prevent an
exact one-in-three cadence; do not invent or mislabel matches to fill a quota.

`Result.ai_course` describes the course subject, displayed as `Про ИИ`. It is
independent of AI-generated suitability recommendations. Generic summaries,
AI review/support features, recommendation quotes and data-direction advice do
not establish an AI subject. Conservative title/curriculum detection makes no
paid model calls and changes no catalog facts, prices or collection settings.

On the pre-change public snapshot of 784 visible courses, replay of the new order
put 13 distinct school labels in the first 18 results, with AI subjects at 3/6/9/12/15/18:
Practicum, OTUS, Stepik AI, Hexlet, Skillbox, OTUS AI, Netology, Skillfactory,
Stepik AI, Skypro, HTML Academy, Skillbox AI, PurpleSchool, CodeBasics, Stepik AI,
JavaRush, JavaScript.ru, Skillbox AI. This is a measured snapshot, not a promise
that all schools have matching courses under every filter.
