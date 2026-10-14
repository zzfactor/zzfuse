package content

import (
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/medxops/trazr-otel/pkg/util"
)

// NewContentFuncMap returns a FuncMap with sprig and custom helpers for content templates
func NewContentFuncMap() template.FuncMap {
	fm := sprig.TxtFuncMap()

	// Use the original functions from pkg/util to avoid duplication
	fm["formatArray"] = util.FormatArray
	fm["formatMap"] = util.FormatMap
	fm["formatLabels"] = util.FormatLabels

	// Add categorization helpers from pkg/util
	fm["categorizeLogCount"] = util.CategorizeLogCount
	fm["categorizeDuration"] = util.CategorizeDuration
	fm["categorizeErrorRate"] = util.CategorizeErrorRate
	fm["categorizeSpanCount"] = util.CategorizeSpanCount

	// Zero-allocation functions from pkg/util
	fm["fastJoin"] = util.FastJoin
	fm["fastConcat"] = util.FastConcat
	fm["fastReplace"] = util.FastReplace
	fm["fastContains"] = util.FastContains

	// Memory-efficient formatting from pkg/util
	fm["formatBytes"] = util.FormatBytesTemplate
	fm["formatDuration"] = util.FormatDurationTemplate
	fm["formatPercentage"] = util.FormatPercentageTemplate

	// Conditional helpers from pkg/util
	fm["ifThenElse"] = util.IfThenElse
	fm["defaultIfEmpty"] = util.DefaultIfEmpty
	fm["coalesce"] = util.Coalesce

	// Type-specific functions for zero-allocation (avoid interface{} boxing)
	fm["categorizeInt"] = util.CategorizeInt
	fm["categorizeInt64"] = util.CategorizeInt64
	fm["categorizeFloat64"] = util.CategorizeFloat64

	// Log and event counting helpers
	fm["countLogsBySeverity"] = util.CountLogsBySeverity
	fm["countEventsByName"] = util.CountEventsByName
	fm["formatLogCountsBySeverity"] = util.FormatLogCountsBySeverity
	fm["formatEventCountsByName"] = util.FormatEventCountsByName
	// TODO: Add formatExemplarCountsByType when the function is properly exported

	return fm
}
