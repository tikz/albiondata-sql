albiondata-sql
==============

The [albiondata-client](https://github.com/broderickhyman/albiondata-client) pulls MarketOrders from the network traffic
and pushes them to NATS, albiondata-sql dumps those from NATS to your SQL Database (one of "mysql", "postgresql" or "sqlite3").


## Usage

Thanks to [viper](https://github.com/spf13/viper) and [cobra](https://github.com/spf13/cobra) you have 3 ways to configure albiondata-sql.

### 1.) Traditional by configfile 

Just copy albiondata-sql.yaml.tmpl to albiondata-sql.yaml and edit it.

### 2.) By commandline arguments

See the output of the help page ```./albiondata-sql -h```

### 3.) By environment variables

For example:

```
ADS_DBTYPE=sqlite3 ADS_DBURI=./sqlite.db ADS_NATSURL="nats://my-nats:4222" ./albiondata-sql 
```

## Authors

René Jochum <rene@jochums.at>


## LICENSE

MIT