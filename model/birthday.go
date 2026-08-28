package model

import (
	"fmt"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LeapDayPolicy string

const (
	LeapDayFebruary28 LeapDayPolicy = "february_28"
	LeapDayMarch1     LeapDayPolicy = "march_1"
)

type Birthday struct {
	GuildID           snowflake.ID `gorm:"primaryKey;autoIncrement:false"`
	UserID            snowflake.ID `gorm:"primaryKey;autoIncrement:false"`
	Day               int
	Month             time.Month
	BirthYear         *int
	LeapDayPolicy     LeapDayPolicy
	LastAnnouncedYear int
}

func ValidateBirthday(day int, month time.Month, birthYear *int, policy LeapDayPolicy, currentYear int) error {
	validationYear := 2000
	if birthYear != nil {
		if *birthYear < 1000 || *birthYear > currentYear {
			return fmt.Errorf("birth year must be between 1000 and %d", currentYear)
		}
		validationYear = *birthYear
	}

	date := time.Date(validationYear, month, day, 0, 0, 0, 0, time.UTC)
	if date.Year() != validationYear || date.Month() != month || date.Day() != day {
		return fmt.Errorf("invalid birthday date")
	}

	isLeapDay := month == time.February && day == 29
	if isLeapDay {
		if policy != LeapDayFebruary28 && policy != LeapDayMarch1 {
			return fmt.Errorf("a February 29 birthday requires a non-leap-year policy")
		}
		return nil
	}
	if policy != "" {
		return fmt.Errorf("a leap-day policy is valid only for February 29")
	}
	return nil
}

func BirthdayObservedOn(birthday Birthday, date time.Time) bool {
	if birthday.Month != time.February || birthday.Day != 29 || isLeapYear(date.Year()) {
		return birthday.Month == date.Month() && birthday.Day == date.Day()
	}

	switch birthday.LeapDayPolicy {
	case LeapDayFebruary28:
		return date.Month() == time.February && date.Day() == 28
	case LeapDayMarch1:
		return date.Month() == time.March && date.Day() == 1
	default:
		return false
	}
}

func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

func SetBirthday(
	guildID snowflake.ID,
	userID snowflake.ID,
	day int,
	month time.Month,
	birthYear *int,
	policy LeapDayPolicy,
	currentYear int,
) (*Birthday, error) {
	if err := ValidateBirthday(day, month, birthYear, policy, currentYear); err != nil {
		return nil, err
	}

	birthday := Birthday{
		GuildID:       guildID,
		UserID:        userID,
		Day:           day,
		Month:         month,
		BirthYear:     birthYear,
		LeapDayPolicy: policy,
	}
	err := DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "guild_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"day",
			"month",
			"birth_year",
			"leap_day_policy",
		}),
	}).Create(&birthday).Error
	if err != nil {
		return nil, err
	}
	return GetBirthday(guildID, userID)
}

func GetBirthday(guildID, userID snowflake.ID) (*Birthday, error) {
	var birthday Birthday
	result := DB.Where("guild_id = ? AND user_id = ?", guildID, userID).Limit(1).Find(&birthday)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &birthday, nil
}

func DeleteBirthday(guildID, userID snowflake.ID) error {
	return DB.Where("guild_id = ? AND user_id = ?", guildID, userID).Delete(&Birthday{}).Error
}

func DueBirthdays(guildID snowflake.ID, localDate time.Time) ([]Birthday, error) {
	datePredicate := "(month = ? AND day = ?)"
	args := []any{localDate.Month(), localDate.Day()}

	if !isLeapYear(localDate.Year()) {
		switch {
		case localDate.Month() == time.February && localDate.Day() == 28:
			datePredicate += " OR (month = ? AND day = 29 AND leap_day_policy = ?)"
			args = append(args, time.February, LeapDayFebruary28)
		case localDate.Month() == time.March && localDate.Day() == 1:
			datePredicate += " OR (month = ? AND day = 29 AND leap_day_policy = ?)"
			args = append(args, time.February, LeapDayMarch1)
		}
	}

	var birthdays []Birthday
	err := DB.Where("guild_id = ? AND last_announced_year <> ?", guildID, localDate.Year()).
		Where("("+datePredicate+")", args...).
		Find(&birthdays).Error
	return birthdays, err
}

func MarkBirthdayAnnounced(guildID, userID snowflake.ID, year int) error {
	return DB.Model(&Birthday{}).
		Where("guild_id = ? AND user_id = ?", guildID, userID).
		Update("last_announced_year", year).Error
}
