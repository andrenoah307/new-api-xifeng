package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	mysql "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// IsAffCodeUniqueViolation identifies only the users.aff_code unique constraint.
func IsAffCodeUniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062 && strings.Contains(mysqlErr.Message, "idx_users_aff_code")
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == "idx_users_aff_code"
	}

	return strings.Contains(err.Error(), "UNIQUE constraint failed: users.aff_code")
}

func (user *User) createWithAffCodeRetry(tx *gorm.DB) error {
	for range common.AffCodeGenerationMaxAttempts {
		affCode, err := common.GenerateAffCode()
		if err != nil {
			return err
		}
		user.AffCode = affCode

		err = tx.Transaction(func(createTx *gorm.DB) error {
			return createTx.Create(user).Error
		})
		if err == nil {
			return nil
		}
		if !IsAffCodeUniqueViolation(err) {
			return err
		}
	}

	return ErrAffCodeGenerationExhausted
}
