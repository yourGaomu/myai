package model

import "time"

type Config struct {
	ID        string
	Name      string
	Provider  string
	BaseURL   string
	APIKey    string
	ModelName string
	Enabled   bool
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
