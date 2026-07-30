package organizer

import (
	"github.com/fredbi/benchviz/internal/config"
	"github.com/fredbi/benchviz/internal/model"
)

// column identifies a single workload axis tick: one (function, context) pair.
//
// The columns of a category form an ordered grid against which every series is
// materialized. The chart aligns series data to the axis by index, so all the series
// of a category must expose exactly one point per column, in column order.
//
// A column sitting on a derived context holds no measurement: it aggregates the measured
// columns of the same function, within the same series.
type column struct {
	Function string
	Context  string
	Derived  config.Derived // zero value on a measured column

	// Summary marks the columns of a derived category, the only ones it displays: the
	// measured columns of such a grid exist only to be aggregated.
	Summary bool
}

// IsDerived reports whether the column aggregates other columns.
func (c column) IsDerived() bool {
	return c.Derived.IsDerived()
}

// columnsFor returns the ordered columns of a category, function-major then context,
// following the declaration order of the category includes.
func columnsFor(cfg *config.Config, filter config.Category) []column {
	includes := filter.Includes
	columns := make([]column, 0, len(includes.Functions)*len(includes.Contexts))

	for _, function := range includes.Functions {
		for _, contextID := range includes.Contexts {
			context, _ := cfg.GetContext(contextID)

			columns = append(columns, column{
				Function: function,
				Context:  contextID,
				Derived:  context.DerivedContext,
			})
		}
	}

	return columns
}

// derivedColumnsFor returns the columns of a derived category: one aggregated column per
// function, preceded by the measured columns it aggregates.
//
// The measured columns are transient — they exist so that [fillDerivedColumns] has
// something to aggregate, and [keepColumns] discards them right after, since they hold no
// aggregate of their own.
func derivedColumnsFor(scope config.Includes, derived config.Derived) []column {
	columns := make([]column, 0, len(scope.Functions)*(len(scope.Contexts)+1))

	for _, function := range scope.Functions {
		for _, context := range scope.Contexts {
			columns = append(columns, column{Function: function, Context: context})
		}

		columns = append(columns, column{
			Function: function,
			Context:  derived.Formula.String(),
			Derived:  derived,
			Summary:  true,
		})
	}

	return columns
}

// fillDerivedColumns computes the aggregated measurements of a series, once its measured
// columns have all been materialized.
//
// Each derived column aggregates the measured columns of its own function: the derived
// measurement of a version stays comparable with the other versions of the same series.
func fillDerivedColumns(series *model.MetricSeries, columns []column) {
	for i, col := range columns {
		if !col.IsDerived() {
			continue
		}

		value, ok := aggregate(col.Derived.Formula, measuredValues(series, columns, col.Function))
		if !ok {
			continue // nothing to aggregate: the column stays missing
		}

		series.Points[i].Value = value
		series.Points[i].Missing = false
	}
}

// measuredValues collects the measurements of one function within a series.
//
// Derived columns are skipped: an aggregate never feeds another aggregate. Missing and
// zero measurements are dropped too — a zero is a legitimate value for some metrics
// (allocsPerOp typically) but it carries no information in a summary, and it would sink
// a geometric mean.
func measuredValues(series *model.MetricSeries, columns []column, function string) []float64 {
	values := make([]float64, 0, len(columns))

	for i, col := range columns {
		if col.Function != function || col.IsDerived() {
			continue
		}

		point := series.Points[i]
		if point.Missing || point.Value == 0 {
			continue
		}

		values = append(values, point.Value)
	}

	return values
}

// keepColumns computes the mask of columns worth displaying: a column survives as soon
// as one series in the category holds a measurement for it.
//
// Fully empty columns would otherwise show as a labelled tick with no bar at all.
//
// A grid holding summary columns displays those only: the columns they aggregate were
// materialized for the sole purpose of feeding them.
func keepColumns(columns []column, series []model.MetricSeries) []bool {
	keep := make([]bool, len(columns))
	summaryOnly := hasSummary(columns)

	for _, s := range series {
		for i, point := range s.Points {
			if point.Missing || (summaryOnly && !columns[i].Summary) {
				continue
			}

			keep[i] = true
		}
	}

	return keep
}

// hasSummary reports whether the grid holds summary columns.
func hasSummary(columns []column) bool {
	for _, col := range columns {
		if col.Summary {
			return true
		}
	}

	return false
}

// pruneColumns drops the columns that are not kept by the mask.
func pruneColumns(columns []column, keep []bool) []column {
	pruned := make([]column, 0, len(columns))

	for i, col := range columns {
		if keep[i] {
			pruned = append(pruned, col)
		}
	}

	return pruned
}

// prunePoints drops from a series the points sitting on a discarded column.
func prunePoints(series *model.MetricSeries, keep []bool) {
	pruned := make([]model.MetricPoint, 0, len(series.Points))

	for i, point := range series.Points {
		if keep[i] {
			pruned = append(pruned, point)
		}
	}

	series.Points = pruned
}
