# Plan: derived series (mean, geomean, min, max)

> Supersedes `DERIVED_SERIES.md` at the repo root (to be removed once this plan is agreed).

Status icons: 📝 planned · 🚧 in progress · ✅ done · ⏸️ deferred · ❌ dropped

## 1. Agreed semantics

Two distinct derivations, both computed **after** all real series are formed, both
**per metric**, never across metrics.

### 1.1 Derived context — "the extra summary bar"

A `Context` may be flagged derived. It aggregates, **for each (function, version)**,
over all *non-derived* contexts included in the category.

- Versions stay the series (same legend, same colours) → the comparison the chart
  exists to make is preserved.
- It adds one x-tick per function, labelled `[function title] - [agg]` via the
  existing `resolveLabels` logic (`function title` prefix only when the category
  spans >1 function, as today).
- This replaces the `Version.DerivedVersion` field currently declared in
  `config.go`: the original spec mistook Context for Version.

Swag example (versions `standard-library`/`easyjson`, contexts `small`/`medium`,
functions `ReadJSON`/`WriteJSON`), adding a derived context `geomean`:

```
x-ticks : ReadJSON-small  ReadJSON-medium  ReadJSON-geomean  WriteJSON-small  WriteJSON-medium  WriteJSON-geomean
series  : standard-library (colour 1), easyjson (colour 2)   ← unchanged, 2 legend entries
```

Every bar group stays fully populated; no new colour is introduced.

### 1.2 Derived category — "the bottom line chart"

A `Category` may be flagged derived. It produces its own chart(s) where, **for each
function**, a single measurement aggregates over all the contexts of the implied scope.

- One x-tick per function, labelled with the function title.
- **Versions stay the series**, exactly as in §1.1: aggregating them away would collapse
  the comparison the charts exist to make. *(Revised from the original spec, which
  aggregated over versions too — decision taken during Phase 2 review.)*
- Its `includes` clause is honoured **per dimension**: whatever it states narrows the
  aggregate, whatever it leaves out is implied from the other (non-derived) categories —
  the union of their functions, contexts, versions and metrics, in first-seen
  declaration order, minus the derived contexts.
- Narrowing matters because the implied union spans contexts that the other categories
  deliberately split across separate charts. On the swag example, the implied bottom
  line folds `large` in and reads 20738 ns/op for `ReadJSON`; narrowed to
  `contexts: [small, medium]` it reads 6859, matching the `ReadJSON - Geomean` tick of
  the R/W chart. Use it whenever the workloads are not really comparable.
- A derived context may not appear in that clause: an aggregate never aggregates
  another one (rejected at load time).
- Purpose: a simplified bar chart conveying the bottom-line message.

### 1.3 Aggregation functions

`mean` (arithmetic), `geomean`, `min`, `max`.

- Missing measurements are dropped from the input set.
- Zero values are dropped from the input set (matters for `allocsPerOp`, which is
  legitimately 0, and keeps `geomean` in its domain).
- If nothing is left after dropping, the derived cell is *missing* (renders as a
  gap, not as 0).

### 1.4 Rules and validation

| Rule | Behaviour |
| --- | --- |
| Derived entries carry no `match` / `notMatch` | error at load time |
| Derived entry without an explicit `id` | error |
| Every context derived (no real one left) | error |
| Every category derived | error |
| Unknown `formula` value | error |
| Derived entries in `includes` | must be listed **explicitly**; excluded from the "empty includes ⇒ inject all" auto-fill |
| Aggregation input | actual measurements only — never another derived series |
| Layout order | declaration order in `contexts:` / `categories:` decides tick / chart placement, even though values are computed last |
| Default title | derived context: titleized `formula` (⇒ x-tick `ReadJSON - Geomean`); derived category: `{metric} - [agg]` |

## 2. Prerequisite: fix the x-axis alignment hole

`chart.AddSeries` builds a positional `[]BarData` that ECharts aligns **by index**
against `SetXAxis(labels)`. Today it only works because every version series happens
to emit points in the same `includes.Functions × includes.Contexts` order — a single
missing measurement already shifts a whole series silently (latent bug, observed).

Any derived work makes this worse, so it is fixed first:

1. The organizer computes, per category, the ordered **column grid** of
   `(function, context)` pairs from the includes.
2. Every series is materialised against that full grid — one point per column, in
   column order, flagged missing where there is no measurement.
3. Columns that are missing for *every* version are pruned (preserves today's
   behaviour of not showing empty ticks).
4. `chart.AddSeries` emits `Value: nil` for missing points → ECharts draws a gap and
   alignment can no longer drift.

Derivation then operates on a dense, well-ordered grid — "first fill all data arrays
in each category, then only apply the derivation".

## 3. Type changes

### `internal/config`

```go
type Context struct {
    Object `mapstructure:",deep,squash"`

    DerivedContext Derived   // NEW
}

type Version struct {
    Object `mapstructure:",deep,squash"`
                             // DerivedVersion REMOVED
}

type Derived struct {
    Formula AggregationFunction
}

func (d Derived) IsDerived() bool { return d.Formula != AggregationFunctionNone }
func (f AggregationFunction) IsValid() bool
func AllAggregationFunctions() []AggregationFunction
```

`Category.DerivedCategory Derived` is unchanged. `Config` gains helpers to split
derived from real entries (`RealContexts()`, `DerivedContexts()`, …) so the organizer
does not re-test the flag everywhere.

### `internal/model`

```go
type MetricPoint struct {
    SeriesKey
    Name    string
    Label   string
    Value   float64
    Missing bool   // NEW: no measurement — renders as a gap, distinct from 0
}

type Category struct {
    ID, Title, Environment string
    XLabels []string   // NEW: authoritative, ordered column grid
    Data    []CategoryData
}
```

`Category.Labels()` (which currently re-derives labels by walking the points) is
dropped in favour of the `XLabels` field; the single caller is `chart/builder.go`.

## 4. Work breakdown

### Phase 0 — grid refactor (no user-visible feature) ✅

- [x] ✅ `model`: add `MetricPoint.Missing`, replace `Category.Labels()` with `XLabels`
- [x] ✅ `organizer`: build the column grid per category (`grid.go`); materialise every
      series against it; prune all-missing columns
- [x] ✅ `organizer`: index measurements by series key; fold repeated samples
      (`-count=N`, same benchmark across files) into their mean instead of piling up
      extra points — another source of misalignment
- [x] ✅ `chart`: emit the ECharts empty marker `"-"` for missing points in `AddSeries`;
      use `XLabels`
- [x] ✅ tests: series with a hole stays aligned; empty columns pruned; samples averaged;
      `AddSeries` emits the empty marker
- [x] ✅ `testintegration`: the fixture was being read as plain text (`cfg.IsJSON` is a
      runtime-only field, never unmarshalled from the yaml), so the test exercised
      nothing past the parser — fixed, and it now asserts axis alignment end to end
- [x] ✅ verified: `examples/swag` and `examples/testify` render byte-identical chart
      data before and after the refactor

### Phase 1 — config surface ✅

- [x] ✅ moved `Derived` from `Version` to `Context`; added `Context.IsDerived`,
      `Category.IsDerived`, `Derived.IsDerived`, `AggregationFunction.IsValid`,
      `AllAggregationFunctions`
- [x] ✅ validation rules from §1.4, each with its own error message and test:
      unknown formula, `match`/`notMatch` on a derived context, all contexts derived,
      all categories derived, a category including only derived contexts
- [x] ✅ default titles: derived context ⇒ titleized formula (`Geomean`), derived
      category ⇒ `{metric} - Geomean`
- [x] ✅ derived contexts excluded from the includes auto-fill (opt-in per category);
      derived categories get their `Includes` cleared
- [x] ✅ `FindContext` skips derived contexts (belt and braces — no matcher anyway)
- [x] ✅ round-trip test: `EncodeYAML` preserves derived entries

YAML surface:

```yaml
contexts:
  - id: summary
    derivedContext:
      formula: geomean   # mean | geomean | min | max

categories:
  - id: bottom-line
    derivedCategory:
      formula: mean
```

### Phase 2 — derived contexts ✅

- [x] ✅ `internal/organizer/aggregate.go`: the four formulas with the drop rules of §1.3.
      `geomean` works in the log domain — benchmark timings span several orders of
      magnitude and a plain product overflows before the root is taken
- [x] ✅ `column` carries its `Derived` settings; `fillDerivedColumns` fills the derived
      cells from the measured cells of the same (function, version) row
- [x] ✅ tests: the four formulas end to end, per metric; zeros and holes dropped;
      declaration-order placement; an aggregate never feeds another aggregate;
      nothing to aggregate leaves the column missing
- [x] ✅ visual check on the swag example with a `geomean` context — validated:
      6 ticks, 2 series in their existing colours, every bar group fully populated
      (`√(2910 × 16170) = 6859.6` on `ReadJSON - Geomean`)

### Phase 3 — derived categories ✅

- [x] ✅ `impliedScope`: union of the non-derived categories' includes, first-seen
      declaration order, derived contexts excluded
- [x] ✅ `scopeFor`: an explicit `includes` narrows the aggregate one dimension at a
      time, the rest stays implied (supersedes the Phase 1 decision to clear it);
      a derived context in that clause is rejected at load time
- [x] ✅ `derivedColumnsFor` builds one aggregated column per function, preceded by the
      measured columns feeding it; `column.Summary` marks the only ones displayed, so
      `keepColumns` drops the transient ones
- [x] ✅ one series per version, one point per function; labels resolved by a pluggable
      `labeller` (context-based for regular categories, function-based here)
- [x] ✅ `SeriesFor` no longer looks up a measurement for a derived column — it can only
      be filled by aggregation
- [x] ✅ tests: values and labels, implied scope, aggregation spanning contexts split
      across several charts, unmeasured function dropped
- [x] ✅ visual check on the swag example: three bottom-line charts (one per metric),
      one tick per function, versions side by side in their existing colours

### Phase 4 — docs and examples ✅

- [x] ✅ `docs/configuration.md`: "Derived contexts", "Derived categories" and
      "Aggregation" sections — the reference for the rules of §1
- [x] ✅ README: a "Derived series" subsection under Concepts
- [x] ✅ `default_config.yaml`: commented examples next to `contexts:` and `categories:`
- [x] ✅ `examples/swag/benchviz.yaml`: a `geomean` context on the two multi-workload
      categories, and a `bottom-line` derived category narrowed to `small, medium`
- [x] ✅ regenerated `examples/swag/benchmark-swag.{html,png}`
- [x] ✅ deleted `DERIVED_SERIES.md`

## 5. Open points

- ~~**Derived-category rendering**~~ ✅ resolved: versions stay the series, one tick per
  function. See §1.2.
- **Multiple aggregations in one derived category** ⏸️ — explicitly deferred; the
  `Derived` struct stays a single `Formula` for now, but nothing in the above blocks
  turning it into a list later.
- ~~**Explicit `includes` on a derived category**~~ ✅ implemented: it narrows the
  implied scope, per dimension. See §1.2.
