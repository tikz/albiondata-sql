package lib

import (
	"time"

	"github.com/jinzhu/gorm"
)

type ModelMarketOrder struct {
	AlbionID         uint   `gorm:"not null;unique_index"`
	ItemID           string `gorm:"index:item_id,main"`
	QualityLevel     int8   `gorm:"size:1"`
	EnchantmentLevel int8   `gorm:"size:1"`
	Price            int
	InitialAmount    int
	Amount           int
	AuctionType      string `gorm:"index:main"`
	Expires          time.Time
	Location         Location `gorm:"not null;index:main"`
	ID               uint     `gorm:"primary_key"`
	CreatedAt        time.Time
	UpdatedAt        time.Time  `gorm:"index:item_id,main"`
	DeletedAt        *time.Time `gorm:"index:item_id,main"`
}

func (m ModelMarketOrder) TableName() string {
	return "market_orders"
}

func NewModelMarketOrder() ModelMarketOrder {
	return ModelMarketOrder{}
}

type ModelMarketStats struct {
	ID        int      `gorm:"primary_key"`
	ItemID    string   `gorm:"index"`
	Location  Location `gorm:"index"`
	PriceMin  int
	PriceMax  int
	PriceAvg  float64
	Timestamp *time.Time `gorm:"index"`
}

func (m ModelMarketStats) TableName() string {
	return "market_stats"
}

type ModelGoldprices struct {
	gorm.Model
	Timestamp time.Time `gorm:"index"`
	Price     int
}

func (m ModelGoldprices) TableName() string {
	return "gold_prices"
}
