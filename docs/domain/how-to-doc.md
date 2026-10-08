# How to write a domain model document

A domain model document defines the concepts, values, relationships, and rules
of a domain. It must explain what those concepts mean and what makes them valid
independently of the software used to implement them.

Use these conventions for domain guides in this directory. The
[Security](security.md), [Languages](languages.md), and [Budget](budget.md)
documents follow this structure.

## Scope and language

Write in domain language. Use the names understood by people working in the
domain, and use each name consistently throughout prose, tables, and diagrams.
Start with the domain's purpose and the scope of the model in a short paragraph.

Describe entities, value objects, aggregates, owned components, domain snapshots,
derived values, and their business rules. Include lifecycle behavior when it
affects identity, validity, ownership, or relationships.

Keep code definitions and examples out of domain documents. Do not include
programming-language types, field identifiers from code, method signatures,
constructors, interfaces, error names, or sample data. Do not describe
repositories, database schemas, storage engines, algorithms, transport
protocols, routes, forms, or build and deployment procedures. Those details
belong in the relevant architecture, technical, or operational guides.

Existing documentation and implementation may provide evidence for a rule,
but express the rule through its domain meaning. Preserve established
constraints without importing their enforcement mechanisms. Do not invent an
identity, field, default, relationship, or validation rule to complete a table.
If a model decision is unresolved, state that explicitly.

## Document structure

Organize the guide in this order, omitting sections that do not apply:

1. Domain title and purpose.
2. Model structure: a classification table and a relationship diagram.
3. Aggregates and entities: boundaries, identities, lifecycles, and field tables.
4. Owned components: their containing aggregate, composition, and field tables.
5. Value objects: their meaning, constituent fields, and validity rules.
6. Domain snapshots and derived values: their source, scope, and field tables.
7. Aggregate invariants and rules spanning multiple aggregates.

Use one top-level title. Give each substantial concept its own subsection.
Adapt section names to the domain while keeping these distinctions visible.

## Classify the model

Begin with a model structure table using these columns:

| Column | What to describe |
| --- | --- |
| Concept | The domain name of the concept. |
| Classification | Its role as an entity, aggregate root, value object, owned component, snapshot, derived value, or collection. |
| Boundary and relationships | What owns it, what it owns, what it references, and the scope of its identity or lifecycle. |

Apply the classifications by domain meaning:

| Classification | Defining property | What the document must establish |
| --- | --- | --- |
| Entity | Has an identity that persists through changes to its other fields. | Its identity, uniqueness scope, lifecycle, and containing aggregate. |
| Aggregate root | Is the entity responsible for an aggregate's consistency boundary. | Its owned entities and values, external references, and aggregate invariants. |
| Value object | Is defined by its constituent values and has no independent entity identity. | Its fields, validity rules, and how its values determine equality. |
| Owned component | Belongs to an aggregate without a separately established entity or value-object classification. | Its owner, fields, and whether a separate identity is defined; avoid assuming an independent lifecycle. |
| Domain snapshot | Describes domain state at a particular point or revision. | The state represented, its revision or time when relevant, and its relationship to the original entities. |
| Derived value | Is calculated or assembled from other domain state. | Its source, calculation or selection rules, and result scope. |
| Collection | Groups concepts under membership or ordering rules. | Its members, cardinality, ordering, and uniqueness rules; grouping alone does not make it an aggregate. |

An aggregate root is also an entity. An identity can itself be a value object
while identifying an entity. A reference to another aggregate does not make
that aggregate part of the owner's boundary.

## Show relationships schematically

Use a compact Mermaid relationship diagram to show the model's structure.
Diagram notation is permitted; it represents the domain model rather than code
definitions or sample data.

Label nodes with domain names. Label relationships to distinguish ownership,
containment, references, classification, and derivation. Show cardinality or
optionality where it matters. Use a grouped boundary when it helps distinguish
an aggregate's contents from external references.

Keep diagrams focused on concepts and their relationships. Describe fields in
tables rather than duplicating every field in a diagram. The diagram, model
structure table, and detailed descriptions must agree on ownership and scope.

## Describe every field

Introduce each concept with a short definition and its identity or ownership
semantics. Follow it with a field table:

| Column | What to describe |
| --- | --- |
| Field | A readable domain name for one constituent field. |
| Domain value | The named value object, entity reference, collection, or domain quantity the field represents. |
| Required | Whether the field is mandatory, optional, or conditional, including an established default when relevant. |
| Meaning and rules | Its purpose, allowed values, constraints, reference scope, and effect on the model. |

Use domain quantities and named concepts instead of programming-language types.
Give composite fields a linked or nearby definition of their constituent
fields. Do not leave a complex concept explained only by its name.

For each field, document the applicable details:

- Identity stability and the scope in which it must be unique.
- Whether a relationship is ownership or a reference, its cardinality, and
  which aggregate may contain the referenced concept.
- The meaning of absence, empty collections, zero values, and defaults.
- Conditions that make the field required.
- Allowed values, text validity, format, range, sign, and length limits.
- Units and precision, distinguishing characters from bytes where necessary.
- Collection ordering, membership constraints, and permitted duplicates.
- The meaning of time fields and the changes or deadlines they record.

A required collection may be empty. A default does not by itself make a field
optional. State both presence requirements and allowed content explicitly.

Scalar value objects may share a table with columns for Value object, Field,
and Meaning and rules. Give composite value objects their own field tables.
For snapshots and derived values whose fields are always present, the Required
column may be omitted; still describe any optional or conditional content.

## Describe rules at their scope

Keep a field's local constraints in its field table. Explain behavior involving
several fields or lifecycle changes in prose beside the owning concept.
Collect rules spanning owned entities and components under aggregate invariants.
Identify rules spanning independent aggregates separately.

State invariants as conditions that must hold, using precise cardinality,
uniqueness, ownership, compatibility, and lifecycle language. Describe deletion
restrictions and the effects of changes where they are part of the domain.
Express what remains valid without describing how software checks or enforces
it. If a rule is repeated in a summary, keep its wording and scope consistent.

## Separate declared state from derived results

Document supplied or declared fields separately from calculated results and
snapshots. A result does not acquire an independent entity lifecycle merely
because it has its own table.

For each derived field, name its source values and explain the calculation or
selection rule in domain terms. Specify the calculation scope, ordering,
carry-forward behavior, inclusions, exclusions, and precision where relevant.
Use relationships and formulas expressed with domain quantities; omit worked
examples and code expressions.

For snapshots, describe which state is captured and which state must remain
current. Explain validity limits without implying that possession of a snapshot
establishes any authority or lifecycle that the domain does not grant it.

## Link shared concepts

Give a shared value or concept one primary definition. Other domains should link
to that definition and describe only their additional constraints or meaning.
Use relative Markdown links, including heading anchors when useful. Keep
linked definitions consistent and check that the target files and headings
exist.

## Review the document

Before finishing, verify that:

- Each concept has a clear classification, ownership boundary, and definition.
- Every entity has an established identity and uniqueness scope.
- Every described field has a domain value, meaning, and applicable constraints.
- Required, optional, conditional, default, and empty states are distinguished.
- Composite values have field descriptions, not just labels.
- Diagrams, tables, prose, and invariant summaries agree.
- Derived values and snapshots are separated from declared state.
- Rules preserve known domain behavior without adding unsupported assumptions.
- No code definitions, implementation procedures, sample data, or worked
  examples appear.
- Relative links resolve, tables render consistently, and Markdown has no
  whitespace errors.
