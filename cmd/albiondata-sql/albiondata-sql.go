package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mssql"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/jinzhu/gorm/dialects/sqlite"

	adclib "github.com/broderickhyman/albiondata-client/lib"
	nats "github.com/nats-io/go-nats"
	"github.com/tikz/albiondata-sql/lib"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const secondsFrom0ToUnix = int64(62135596800)

var (
	version string
	cfgFile string
)

var rootCmd = &cobra.Command{
	Use:   "albiondata-sql",
	Short: "albiondata-sql is a NATS to SQL Bridge for the Albion Data Project",
	Long: `Reads data from NATS and pushes it to a SQL Database (MSSQL, MySQL, PostgreSQL and SQLite3 are supported),
creates one table per Market`,
	Run: doCmd,
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.albiondata-sql.yaml")
	rootCmd.PersistentFlags().StringP("dbType", "t", "mysql", "Database type must be one of mssql, mysql, postgresql, sqlite3")
	rootCmd.PersistentFlags().StringP("dbURI", "u", "", "Databse URI to connect to, see: http://jinzhu.me/gorm/database.html#connecting-to-a-database")
	rootCmd.PersistentFlags().StringP("natsURL", "n", "nats://public:thenewalbiondata@www.albion-data-online.com:4222", "NATS to connect to")
	rootCmd.PersistentFlags().Int64P("expireCheckEvery", "e", 3600, "every x seconds the db entries get checked if an order is expired")

	viper.BindPFlag("dbType", rootCmd.PersistentFlags().Lookup("dbType"))
	viper.BindPFlag("dbURI", rootCmd.PersistentFlags().Lookup("dbURI"))
	viper.BindPFlag("natsURL", rootCmd.PersistentFlags().Lookup("natsURL"))
	viper.BindPFlag("expireCheckEvery", rootCmd.PersistentFlags().Lookup("expireCheckEvery"))
}

func initConfig() {
	// Don't forget to read config either from cfgFile or from home directory!
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath("/etc")

		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".cobra" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName("albiondata-sql")

		// Add the executable path as
		ex, err := os.Executable()
		if err != nil {
			panic(err)
		}
		exPath := filepath.Dir(ex)
		viper.AddConfigPath(exPath)
	}

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("Can't read config:", err)
	}

	viper.SetEnvPrefix("ADS")
	viper.AutomaticEnv()
}

func updateOrCreateOrder(db *gorm.DB, io *adclib.MarketOrder) error {
	location, err := lib.NewLocationFromId(io.LocationID)
	if err != nil {
		return err
	}
	mo := lib.NewModelMarketOrder()

	//fmt.Printf("Importing: Price: %d - %d - %s at %d\n", io.Price, io.ID, io.ItemID, io.LocationID)

	if err := db.Unscoped().Where(&lib.ModelMarketOrder{AlbionID: uint(io.ID)}).First(&mo).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			fmt.Printf("ERROR: WHERE albion_id = %d, error was: %s\n", io.ID, err)
			return err
		}
	}
	if mo.ID == 0 {
		// Not found
		//fmt.Printf("Not Found\n")
		mo = lib.NewModelMarketOrder()
		mo.Location = location
		mo.AlbionID = uint(io.ID)
		mo.ItemID = io.ItemID
		mo.QualityLevel = int8(io.QualityLevel)
		mo.EnchantmentLevel = int8(io.EnchantmentLevel)
		price := strconv.Itoa(io.Price)
		if len(price) > 4 {
			price = price[:len(price)-4]
			i, _ := strconv.Atoi(price)
			mo.Price = i
		} else {
			mo.Price = 0
		}
		mo.InitialAmount = io.Amount
		mo.Amount = io.Amount
		mo.AuctionType = io.AuctionType
		t, err := time.Parse(time.RFC3339, io.Expires+"+00:00")
		if err != nil {
			return fmt.Errorf("while parsing the time of order id %d, error was: %s", io.ID, err)
		}

		// This is a workaround for strict datetime fields that do not support 1000 years from now
		maxTime := time.Now().AddDate(10, 0, 0)
		if t.After(maxTime) {
			t = time.Now().AddDate(0, 0, 7)
		}
		mo.Expires = t

		//fmt.Printf("Creating: %d - %s at %s\n", mo.AlbionID, mo.ItemID, mo.Location.String())
		if err := db.Create(&mo).Error; err != nil {
			return err
		}
	} else {
		// Found, set updatedAt
		//fmt.Printf("Updating %d - %s at %s\n", mo.AlbionID, mo.ItemID, mo.Location.String())
		mo.Amount = io.Amount
		mo.DeletedAt = nil
		mo.Location = location //TODO: Fix this workaround for when client sends the wrong location
		if err := db.Unscoped().Save(&mo).Error; err != nil {
			return err
		}
	}

	return nil
}

func createGoldPrices(db *gorm.DB, gps *adclib.GoldPricesUpload) error {
	for i := range gps.Prices {
		m := lib.ModelGoldprices{}

		price := gps.Prices[i]
		time := time.Unix(int64(gps.TimeStamps[i])/int64(10000000)-int64(secondsFrom0ToUnix), 0)

		if err := db.Unscoped().Where("timestamp = ?", time).First(&m).Error; err != nil {
			// Not found
			m.Price = int(price / 10000)
			m.Timestamp = time

			if err := db.Create(&m).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func expireOrders(db *gorm.DB) {
	checkEvery := viper.GetInt64("expireCheckEvery")

	for {
		now := time.Now()
		updatedFrom := time.Now().AddDate(0, 0, -3)
		if err := db.Where("expires <= ? or updated_at <= ?", now, updatedFrom).Delete(&lib.ModelMarketOrder{}).Error; err != nil {
			fmt.Printf("ERROR: %v\n", err)
		}

		time.Sleep(time.Second * time.Duration(checkEvery))
	}
}

func doCmd(cmd *cobra.Command, args []string) {
	// Fix for MSSQL, see: https://github.com/jinzhu/gorm/issues/941
	if strings.ToLower(viper.GetString("dbType")) == "mssql" {
		gorm.DefaultCallback.Create().Remove("mssql:set_identity_insert")
	}

	fmt.Printf("Connecting to database: %s\n", viper.GetString("dbType"))
	db, err := gorm.Open(viper.GetString("dbType"), viper.GetString("dbURI"))
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
	defer db.Close()

	if viper.GetString("dbType") == "mysql" {
		model := lib.NewModelMarketOrder()
		err := db.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(&model).Error
		if err != nil {
			fmt.Printf("%v\n", err)
			return
		}

		gpmodel := lib.ModelGoldprices{}
		err = db.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(&gpmodel).Error
		if err != nil {
			fmt.Printf("%v\n", err)
			return
		}
	} else {
		model := lib.NewModelMarketOrder()
		if err := db.AutoMigrate(&model).Error; err != nil {
			fmt.Printf("ModelMarket AutoMigrate %v\n", err)
			return
		}

		gpmodel := lib.ModelGoldprices{}
		err = db.AutoMigrate(&gpmodel).Error
		if err != nil {
			fmt.Printf("ModelGold AutoMigrate %v\n", err)
			return
		}
	}

	// Expiration
	if viper.GetInt64("expireCheckEvery") > 0 {
		go expireOrders(db)
	}

	fmt.Printf("Connecting to nats: %s\n", viper.GetString("natsURL"))
	nc, _ := nats.Connect(viper.GetString("natsURL"))
	defer nc.Close()

	marketCh := make(chan *nats.Msg, 64)
	marketSub, err := nc.ChanSubscribe("*.deduped", marketCh)
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
	defer marketSub.Unsubscribe()

	for {
		select {
		case msg := <-marketCh:
			switch msg.Subject {
			case adclib.NatsMarketOrdersDeduped:
				order := &adclib.MarketOrder{}
				err := json.Unmarshal(msg.Data, order)
				if err != nil {
					fmt.Printf("ERROR MO Unmarshal: %v\n", err)
					continue
				}

				err = updateOrCreateOrder(db, order)
				if err != nil {
					fmt.Printf("ERROR MO UpdateCreate: %s\n", err)
				}

			case adclib.NatsGoldPricesDeduped:
				gprices := &adclib.GoldPricesUpload{}
				err := json.Unmarshal(msg.Data, gprices)
				if err != nil {
					fmt.Printf("ERROR G Unmarshal: %v\n", err)
					continue
				}

				err = createGoldPrices(db, gprices)
				if err != nil {
					fmt.Printf("ERROR G Create: %s\n", err)
				}

			default:
				continue
			}
		}
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
