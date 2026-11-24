package domain

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
	"portspendernote/internal/llm"
)

type CategorySummary struct {
	ID               string `gorm:"column:id" json:"id"`
	Name             string `gorm:"column:name" json:"name"`
	Icon             string `gorm:"column:icon" json:"icon"`
	IsSystem         int    `gorm:"column:is_system" json:"is_system"`
	SortOrder        int    `gorm:"column:sort_order" json:"sort_order"`
	TransactionCount int64  `gorm:"column:transaction_count" json:"transaction_count"`
	CreatedAtUtc     string `gorm:"column:created_at_utc" json:"created_at_utc"`
}

type CreateCategoryInput struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
}

type UpdateCategoryInput struct {
	Name *string `json:"name"`
	Icon *string `json:"icon"`
}

func ListCategories(gdb *gorm.DB) ([]CategorySummary, error) {
	rows := []CategorySummary{}
	if err := gdb.Raw(`
		SELECT
			c.id,
			c.name,
			c.icon,
			c.is_system,
			c.sort_order,
			c.created_at_utc,
			COUNT(t.id) AS transaction_count
		FROM categories c
		LEFT JOIN transactions t ON t.category = c.name
		GROUP BY c.id
		ORDER BY c.is_system ASC, c.sort_order ASC, LOWER(c.name) ASC`,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func ListCategoryNames(gdb *gorm.DB) ([]string, error) {
	var names []string
	if err := gdb.Raw(`
		SELECT name
		FROM categories
		ORDER BY is_system ASC, sort_order ASC, LOWER(name) ASC`,
	).Scan(&names).Error; err != nil {
		return nil, err
	}
	return llm.ResolveCategories(names), nil
}

func reassignCategoryReferences(tx *gorm.DB, fromName, toName, nowUtc string) error {
	if err := tx.Model(&db.Transaction{}).
		Where("category = ?", fromName).
		Updates(map[string]any{
			"category":       toName,
			"updated_at_utc": nowUtc,
		}).Error; err != nil {
		return err
	}
	return tx.Model(&db.EmailExtraction{}).
		Where("category = ?", fromName).
		Update("category", toName).Error
}

func lookupCategoryByName(gdb *gorm.DB, raw string) (*db.Category, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var cat db.Category
	if err := gdb.Where("normalized_name = ?", strings.ToLower(trimmed)).First(&cat).Error; err != nil {
		return nil, err
	}
	return &cat, nil
}

func ResolveKnownCategory(gdb *gorm.DB, raw string) string {
	cat, err := lookupCategoryByName(gdb, raw)
	if err == nil && cat != nil {
		return cat.Name
	}
	return DefaultCategory
}

func getCategorySummaryByID(gdb *gorm.DB, id string) (*CategorySummary, error) {
	var rows []CategorySummary
	if err := gdb.Raw(`
		SELECT
			c.id,
			c.name,
			c.icon,
			c.is_system,
			c.sort_order,
			c.created_at_utc,
			COUNT(t.id) AS transaction_count
		FROM categories c
		LEFT JOIN transactions t ON t.category = c.name
		WHERE c.id = ?
		GROUP BY c.id`,
		id,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("Category not found.")
	}
	return &rows[0], nil
}

func CreateCategory(gdb *gorm.DB, input CreateCategoryInput) (*CategorySummary, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("Category name is required.")
	}
	icon := strings.TrimSpace(input.Icon)
	if icon == "" {
		icon = "Tag"
	}
	norm := strings.ToLower(name)

	var createdID string
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var dupCount int64
		if err := tx.Model(&db.Category{}).Where("normalized_name = ?", norm).Count(&dupCount).Error; err != nil {
			return err
		}
		if dupCount > 0 {
			return errors.New("A category with this name already exists.")
		}

		var maxOrder int
		if err := tx.Raw(`SELECT COALESCE(MAX(sort_order), 0) FROM categories WHERE is_system = 0`).Scan(&maxOrder).Error; err != nil {
			return err
		}

		createdID = uuid.NewString()
		return tx.Create(&db.Category{
			ID:             createdID,
			Name:           name,
			NormalizedName: norm,
			Icon:           icon,
			IsSystem:       0,
			SortOrder:      maxOrder + 1,
			CreatedAtUtc:   NowUtcIsoSeconds(),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return getCategorySummaryByID(gdb, createdID)
}

func UpdateCategory(gdb *gorm.DB, id string, input UpdateCategoryInput) (*CategorySummary, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("Category ID is required.")
	}

	var updated *CategorySummary
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var current db.Category
		if err := tx.Where("id = ?", id).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("Category not found.")
			}
			return err
		}
		if current.IsSystem == 1 {
			return errors.New("The default 'Other' category is read-only and cannot be modified.")
		}

		nextName := current.Name
		if input.Name != nil {
			nextName = strings.TrimSpace(*input.Name)
		}
		if nextName == "" {
			return errors.New("Category name is required.")
		}
		nextNorm := strings.ToLower(nextName)

		nextIcon := current.Icon
		if input.Icon != nil {
			nextIcon = strings.TrimSpace(*input.Icon)
		}
		if nextIcon == "" {
			return errors.New("Category icon is required.")
		}

		if nextNorm != current.NormalizedName {
			var dupCount int64
			if err := tx.Model(&db.Category{}).
				Where("normalized_name = ? AND id <> ?", nextNorm, id).
				Count(&dupCount).Error; err != nil {
				return err
			}
			if dupCount > 0 {
				return errors.New("A category with this name already exists.")
			}
		}

		if nextName != current.Name {
			if err := reassignCategoryReferences(tx, current.Name, nextName, NowUtcIsoSeconds()); err != nil {
				return err
			}
		}

		if err := tx.Model(&db.Category{}).Where("id = ?", id).Updates(map[string]any{
			"name":            nextName,
			"normalized_name": nextNorm,
			"icon":            nextIcon,
		}).Error; err != nil {
			return err
		}

		summary, err := getCategorySummaryByID(tx, id)
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

func DeleteCategory(gdb *gorm.DB, id string, replaceWithCategory string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("Category ID is required.")
	}

	return gdb.Transaction(func(tx *gorm.DB) error {
		var current db.Category
		if err := tx.Where("id = ?", id).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("Category not found.")
			}
			return err
		}
		if current.IsSystem == 1 {
			return errors.New("The default 'Other' category cannot be deleted.")
		}

		targetName := strings.TrimSpace(replaceWithCategory)
		if targetName == "" {
			targetName = DefaultCategory
		}
		replacement, err := lookupCategoryByName(tx, targetName)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("Replacement category does not exist.")
			}
			return err
		}
		if replacement.ID == current.ID {
			return errors.New("Replacement category must be a different existing category.")
		}

		if err := reassignCategoryReferences(tx, current.Name, replacement.Name, NowUtcIsoSeconds()); err != nil {
			return err
		}

		return tx.Where("id = ?", id).Delete(&db.Category{}).Error
	})
}
