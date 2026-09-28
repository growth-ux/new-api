package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type RankingQuotaTotal struct {
	ModelName   string `json:"model_name"`
	TotalTokens int64  `json:"total_tokens"`
}

type RankingQuotaBucket struct {
	ModelName string `json:"model_name"`
	Bucket    int64  `json:"bucket"`
	Tokens    int64  `json:"tokens"`
}

func GetRankingQuotaTotals(startTime int64, endTime int64) ([]RankingQuotaTotal, error) {
	var rows []RankingQuotaTotal
	modelName := quoteModelIdentifier(DB, "model_name")
	tokenUsed := quoteModelIdentifier(DB, "token_used")
	totalTokens := quoteModelIdentifier(DB, "total_tokens")
	query := DB.Table("quota_data").
		Select(fmt.Sprintf("%s, sum(%s) as %s", modelName, tokenUsed, totalTokens)).
		Where(modelName + " <> ''").
		Group(modelName).
		Having("sum(" + tokenUsed + ") > 0").
		Order(totalTokens + " DESC")
	query = applyRankingQuotaTimeRange(query, startTime, endTime)
	err := query.Find(&rows).Error
	return rows, err
}

func GetRankingQuotaBuckets(startTime int64, endTime int64, bucketSize int64) ([]RankingQuotaBucket, error) {
	if bucketSize <= 0 {
		bucketSize = 3600
	}
	bucketExpr := rankingBucketExpr(bucketSize)
	var rows []RankingQuotaBucket
	modelName := quoteModelIdentifier(DB, "model_name")
	tokenUsed := quoteModelIdentifier(DB, "token_used")
	bucket := quoteModelIdentifier(DB, "bucket")
	tokens := quoteModelIdentifier(DB, "tokens")
	query := DB.Table("quota_data").
		Select(fmt.Sprintf("%s, %s as %s, sum(%s) as %s", modelName, bucketExpr, bucket, tokenUsed, tokens)).
		Where(modelName + " <> ''").
		Group(fmt.Sprintf("%s, %s", modelName, bucketExpr)).
		Having("sum(" + tokenUsed + ") > 0").
		Order(bucket + " ASC")
	query = applyRankingQuotaTimeRange(query, startTime, endTime)
	err := query.Find(&rows).Error
	return rows, err
}

func rankingBucketExpr(bucketSize int64) string {
	createdAt := quoteModelIdentifier(DB, "created_at")
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) ||
		common.UsingMainDatabase(common.DatabaseTypeDM) {
		return fmt.Sprintf("FLOOR(%s / %d) * %d", createdAt, bucketSize, bucketSize)
	}
	return fmt.Sprintf("(%s / %d) * %d", createdAt, bucketSize, bucketSize)
}

func applyRankingQuotaTimeRange(query *gorm.DB, startTime int64, endTime int64) *gorm.DB {
	createdAt := quoteModelIdentifier(query, "created_at")
	if startTime > 0 {
		query = query.Where(createdAt+" >= ?", startTime)
	}
	if endTime > 0 {
		query = query.Where(createdAt+" <= ?", endTime)
	}
	return query
}
