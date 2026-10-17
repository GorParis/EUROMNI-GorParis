package dbdialect

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

type Dialect string

const (
	PostgreSQL Dialect = "postgres"
	MariaDB    Dialect = "mariadb"
)

func Resolve(dsn string) (driverName, driverDSN string, dialect Dialect, err error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return "", "", "", errors.New("database DSN is empty")
	}

	parsed, parseErr := url.Parse(dsn)
	if parseErr == nil {
		switch strings.ToLower(parsed.Scheme) {
		case "postgres", "postgresql":
			return "pgx", dsn, PostgreSQL, nil
		case "mysql", "mariadb":
			driverDSN, err = mariaDBDSN(parsed)
			if err != nil {
				return "", "", "", err
			}
			return "mysql", driverDSN, MariaDB, nil
		}
	}
	if strings.Contains(dsn, "@tcp(") {
		return "mysql", dsn, MariaDB, nil
	}
	return "pgx", dsn, PostgreSQL, nil
}

func mariaDBDSN(parsed *url.URL) (string, error) {
	if parsed.Host == "" {
		return "", errors.New("MariaDB URL must include a host")
	}
	config := mysql.NewConfig()
	config.Net = "tcp"
	config.Addr = parsed.Host
	config.DBName = strings.TrimPrefix(parsed.EscapedPath(), "/")
	if database, err := url.PathUnescape(config.DBName); err != nil {
		return "", fmt.Errorf("decode MariaDB database name: %w", err)
	} else {
		config.DBName = database
	}
	config.ParseTime = true
	config.Loc = time.UTC
	if parsed.User != nil {
		config.User = parsed.User.Username()
		if password, ok := parsed.User.Password(); ok {
			config.Passwd = password
		}
	}
	for key, values := range parsed.Query() {
		if len(values) == 0 {
			continue
		}
		switch strings.ToLower(key) {
		case "parsetime":
			parseTime, err := strconv.ParseBool(values[0])
			if err != nil {
				return "", fmt.Errorf("parse MariaDB parseTime option: %w", err)
			}
			config.ParseTime = parseTime
		case "loc":
			location, err := time.LoadLocation(values[0])
			if err != nil {
				return "", fmt.Errorf("load MariaDB time zone: %w", err)
			}
			config.Loc = location
		case "charset":
			if !strings.EqualFold(values[0], "utf8mb4") {
				return "", fmt.Errorf("unsupported MariaDB character set %q; only utf8mb4 is supported", values[0])
			}
		default:
			return "", fmt.Errorf("unsupported MariaDB URL option %q", key)
		}
	}
	return config.FormatDSN(), nil
}
