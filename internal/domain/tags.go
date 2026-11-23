package domain

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
)

type TagSummary struct {
	ID               string `gorm:"column:id" json:"id"`
	Name             string `gorm:"column:name" json:"name"`
	TransactionCount int64  `gorm:"column:transaction_count" json:"transaction_count"`
	CreatedAtUtc     string `gorm:"column:created_at_utc" json:"created_at_utc"`
}

type CreateTagInput struct {
	Name string `json:"name"`
}

type UpdateTagInput struct {
	Name string `json:"name"`
}

func ListTags(gdb *gorm.DB) ([]TagSummary, error) {
	rows := []TagSummary{}
	if err := gdb.Raw(`
		SELECT
			tg.id,
			tg.name,
			tg.created_at_utc,
			COUNT(tt.transaction_id) AS transaction_count
		FROM tags tg
		LEFT JOIN transaction_tags tt ON tt.tag_id = tg.id
		GROUP BY tg.id
		ORDER BY LOWER(tg.name) ASC, tg.created_at_utc ASC`,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func getTagSummaryByID(gdb *gorm.DB, id string) (*TagSummary, error) {
	var rows []TagSummary
	if err := gdb.Raw(`
		SELECT
			tg.id,
			tg.name,
			tg.created_at_utc,
			COUNT(tt.transaction_id) AS transaction_count
		FROM tags tg
		LEFT JOIN transaction_tags tt ON tt.tag_id = tg.id
		WHERE tg.id = ?
		GROUP BY tg.id`,
		id,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("Tag not found.")
	}
	return &rows[0], nil
}

func CreateTag(gdb *gorm.DB, input CreateTagInput) (*TagSummary, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("Tag name is required.")
	}
	norm := strings.ToLower(name)

	var createdID string
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var dupCount int64
		if err := tx.Model(&db.Tag{}).Where("normalized_name = ?", norm).Count(&dupCount).Error; err != nil {
			return err
		}
		if dupCount > 0 {
			return errors.New("A tag with this name already exists.")
		}

		createdID = uuid.NewString()
		return tx.Create(&db.Tag{
			ID:             createdID,
			Name:           name,
			NormalizedName: norm,
			CreatedAtUtc:   NowUtcIsoSeconds(),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return getTagSummaryByID(gdb, createdID)
}

func UpdateTag(gdb *gorm.DB, id string, input UpdateTagInput) (*TagSummary, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("Tag ID is required.")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("Tag name is required.")
	}
	norm := strings.ToLower(name)

	var updated *TagSummary
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var current db.Tag
		if err := tx.Where("id = ?", id).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("Tag not found.")
			}
			return err
		}

		var dupCount int64
		if err := tx.Model(&db.Tag{}).
			Where("normalized_name = ? AND id <> ?", norm, id).
			Count(&dupCount).Error; err != nil {
			return err
		}
		if dupCount > 0 {
			return errors.New("A tag with this name already exists.")
		}

		if err := tx.Model(&db.Tag{}).Where("id = ?", id).Updates(map[string]any{
			"name":            name,
			"normalized_name": norm,
		}).Error; err != nil {
			return err
		}

		summary, err := getTagSummaryByID(tx, id)
		if err != nil {
			return err
		}
		updated = summary
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func DeleteTag(gdb *gorm.DB, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("Tag ID is required.")
	}
	res := gdb.Where("id = ?", id).Delete(&db.Tag{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("Tag not found.")
	}
	return nil
}
