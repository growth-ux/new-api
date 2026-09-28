package model

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PerfMetric stores aggregated relay performance metrics for the model square.
type PerfMetric struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	ModelName      string `json:"model_name" gorm:"size:128;uniqueIndex:idx_perf_model_group_bucket,priority:1"`
	Group          string `json:"group" gorm:"column:group;size:64;uniqueIndex:idx_perf_model_group_bucket,priority:2"`
	BucketTs       int64  `json:"bucket_ts" gorm:"uniqueIndex:idx_perf_model_group_bucket,priority:3;index:idx_perf_bucket_ts"`
	RequestCount   int64  `json:"-" gorm:"default:0"`
	SuccessCount   int64  `json:"-" gorm:"default:0"`
	TotalLatencyMs int64  `json:"-" gorm:"default:0"`
	TtftSumMs      int64  `json:"-" gorm:"default:0"`
	TtftCount      int64  `json:"-" gorm:"default:0"`
	OutputTokens   int64  `json:"-" gorm:"default:0"`
	GenerationMs   int64  `json:"-" gorm:"default:0"`
}

func (PerfMetric) TableName() string {
	return "perf_metrics"
}

func perfMetricSummarySelect(includeBucket bool) string {
	columns := []string{quoteModelIdentifier(DB, "model_name")}
	if includeBucket {
		columns = append(columns, quoteModelIdentifier(DB, "bucket_ts"))
	}
	for _, name := range []string{"request_count", "success_count", "total_latency_ms", "output_tokens", "generation_ms"} {
		column := quoteModelIdentifier(DB, name)
		columns = append(columns, fmt.Sprintf("SUM(%s) AS %s", column, column))
	}
	return strings.Join(columns, ", ")
}

func UpsertPerfMetric(metric *PerfMetric) error {
	if metric == nil || metric.RequestCount == 0 {
		return nil
	}
	add := func(name string, value int64) clause.Expr {
		column := quoteModelIdentifier(DB, name)
		table := quoteModelIdentifier(DB, "perf_metrics")
		return gorm.Expr(table+"."+column+" + ?", value)
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "model_name"},
			{Name: "group"},
			{Name: "bucket_ts"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"request_count":    add("request_count", metric.RequestCount),
			"success_count":    add("success_count", metric.SuccessCount),
			"total_latency_ms": add("total_latency_ms", metric.TotalLatencyMs),
			"ttft_sum_ms":      add("ttft_sum_ms", metric.TtftSumMs),
			"ttft_count":       add("ttft_count", metric.TtftCount),
			"output_tokens":    add("output_tokens", metric.OutputTokens),
			"generation_ms":    add("generation_ms", metric.GenerationMs),
		}),
	}).Create(metric).Error
}

func GetPerfMetrics(modelName string, group string, startTs int64, endTs int64) ([]PerfMetric, error) {
	var metrics []PerfMetric
	modelColumn, bucketColumn := quoteModelIdentifier(DB, "model_name"), quoteModelIdentifier(DB, "bucket_ts")
	query := DB.Model(&PerfMetric{}).
		Where(modelColumn+" = ? AND "+bucketColumn+" >= ? AND "+bucketColumn+" <= ?", modelName, startTs, endTs)
	if group != "" {
		query = query.Where(quoteModelIdentifier(DB, "group")+" = ?", group)
	}
	err := query.Order(bucketColumn + " ASC").Find(&metrics).Error
	return metrics, err
}

type PerfMetricSummary struct {
	ModelName      string `json:"model_name"`
	RequestCount   int64  `json:"request_count"`
	SuccessCount   int64  `json:"success_count"`
	TotalLatencyMs int64  `json:"total_latency_ms"`
	OutputTokens   int64  `json:"output_tokens"`
	GenerationMs   int64  `json:"generation_ms"`
}

type PerfMetricSummaryBucket struct {
	ModelName      string `json:"model_name"`
	BucketTs       int64  `json:"bucket_ts"`
	RequestCount   int64  `json:"request_count"`
	SuccessCount   int64  `json:"success_count"`
	TotalLatencyMs int64  `json:"total_latency_ms"`
	OutputTokens   int64  `json:"output_tokens"`
	GenerationMs   int64  `json:"generation_ms"`
}

func GetPerfMetricsSummaryAll(startTs int64, endTs int64, groups []string) ([]PerfMetricSummary, error) {
	var summaries []PerfMetricSummary
	bucketColumn := quoteModelIdentifier(DB, "bucket_ts")
	modelColumn := quoteModelIdentifier(DB, "model_name")
	requestCountColumn := quoteModelIdentifier(DB, "request_count")
	query := DB.Model(&PerfMetric{}).
		Select(perfMetricSummarySelect(false)).
		Where(bucketColumn+" >= ? AND "+bucketColumn+" <= ?", startTs, endTs)
	if groups != nil {
		if len(groups) == 0 {
			return summaries, nil
		}
		query = query.Where(quoteModelIdentifier(DB, "group")+" IN ?", groups)
	}
	err := query.
		Group(modelColumn).
		Having("SUM(" + requestCountColumn + ") > 0").
		Find(&summaries).Error
	return summaries, err
}

func GetPerfMetricsSummaryBucketsAll(startTs int64, endTs int64, groups []string) ([]PerfMetricSummaryBucket, error) {
	var summaries []PerfMetricSummaryBucket
	bucketColumn := quoteModelIdentifier(DB, "bucket_ts")
	modelColumn := quoteModelIdentifier(DB, "model_name")
	requestCountColumn := quoteModelIdentifier(DB, "request_count")
	query := DB.Model(&PerfMetric{}).
		Select(perfMetricSummarySelect(true)).
		Where(bucketColumn+" >= ? AND "+bucketColumn+" <= ?", startTs, endTs)
	if groups != nil {
		if len(groups) == 0 {
			return summaries, nil
		}
		query = query.Where(quoteModelIdentifier(DB, "group")+" IN ?", groups)
	}
	err := query.
		Group(modelColumn + ", " + bucketColumn).
		Having("SUM(" + requestCountColumn + ") > 0").
		Order(bucketColumn + " ASC").
		Find(&summaries).Error
	return summaries, err
}

func DeletePerfMetricsBefore(cutoffTs int64) error {
	if cutoffTs <= 0 {
		return nil
	}
	return DB.Where(quoteModelIdentifier(DB, "bucket_ts")+" < ?", cutoffTs).Delete(&PerfMetric{}).Error
}

func PerfMetricStartTime(hours int) int64 {
	if hours <= 0 {
		hours = 24
	}
	return time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
}
