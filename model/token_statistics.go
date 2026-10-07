package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// tokenStatisticsSumSQL keeps aggregation exact beyond the billing quota domain.
// Raw logs are never rewritten: normalization applies only to their statistics.
func tokenStatisticsSumSQL() string {
	input := "CASE WHEN COALESCE(input_tokens_total, prompt_tokens) > 0 THEN COALESCE(input_tokens_total, prompt_tokens) ELSE 0 END"
	output := "CASE WHEN completion_tokens > 0 THEN completion_tokens ELSE 0 END"
	if common.UsingLogDatabase(common.DatabaseTypeSQLite) {
		// SQLite has no exact decimal aggregate. Its normal integer SUM is exact;
		// the rare integer-overflow error uses a bounded-memory Go fallback below.
		return sqliteTokenStatisticsSumSQL()
	}
	total := "COALESCE(sum(CAST(" + input + " AS DECIMAL(38, 0))), 0) + COALESCE(sum(CAST(" + output + " AS DECIMAL(38, 0))), 0)"
	bounded := "CASE WHEN (" + total + ") > 9223372036854775807 THEN 9223372036854775807 ELSE (" + total + ") END"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return "toInt64(" + bounded + ")"
	}
	return bounded
}

func sqliteTokenStatisticsOverflow(err error) bool {
	return err != nil && common.UsingLogDatabase(common.DatabaseTypeSQLite) && strings.Contains(strings.ToLower(err.Error()), "integer overflow")
}

// sumTokenStatisticsRows is only used when SQLite's integer aggregate overflows.
// It preserves the same query filters and does not load an unbounded log slice.
func sumTokenStatisticsRows(query *gorm.DB) (tokens int64, requests int, err error) {
	rows, err := query.Select("COALESCE(input_tokens_total, prompt_tokens, 0), COALESCE(completion_tokens, 0)").Rows()
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var input, output int64
		if err := rows.Scan(&input, &output); err != nil {
			return 0, 0, err
		}
		tokens = common.SumTokenCountsForStatistics(tokens, input, output)
		requests++
	}
	return tokens, requests, rows.Err()
}

// SQLite promotes integer addition to REAL on overflow even if each SUM fits.
// A scan of that REAL into int64 fails, so test the cross-sum boundary in SQL
// before adding; overflow within either SUM still follows the fallback above.
func sqliteTokenStatisticsSumSQL() string {
	input := "COALESCE(sum(CASE WHEN COALESCE(input_tokens_total, prompt_tokens) > 0 THEN COALESCE(input_tokens_total, prompt_tokens) ELSE 0 END), 0)"
	output := "COALESCE(sum(CASE WHEN completion_tokens > 0 THEN completion_tokens ELSE 0 END), 0)"
	return "CASE WHEN (" + input + ") > 9223372036854775807 - (" + output + ") THEN 9223372036854775807 ELSE (" + input + ") + (" + output + ") END"
}
