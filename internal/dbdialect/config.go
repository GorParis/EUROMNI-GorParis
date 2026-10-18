package dbdialect

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

type Dialect string

const (
	PostgreSQL Dialect = "postgres"
	MariaDB    Dialect = "mariadb"
)

func IsUniqueConstraint(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return true
	}
	var mariaErr *mysql.MySQLError
	return errors.As(err, &mariaErr) && mariaErr.Number == 1062
}

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
			return mariaDBDriverName, driverDSN, MariaDB, nil
		}
	}
	if strings.Contains(dsn, "@tcp(") {
		driverDSN, err := mariaDBDriverDSN(dsn)
		if err != nil {
			return "", "", "", err
		}
		return mariaDBDriverName, driverDSN, MariaDB, nil
	}
	return "pgx", dsn, PostgreSQL, nil
}

func mariaDBDriverDSN(dsn string) (string, error) {
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("parse MariaDB driver DSN: %w", err)
	}
	setMariaDBDefaults(config)
	return config.FormatDSN(), nil
}

func setMariaDBDefaults(config *mysql.Config) {
	config.ParseTime = true
	config.Loc = time.UTC
	config.MultiStatements = true
	config.Collation = "utf8mb4_bin"
	if config.Params == nil {
		config.Params = make(map[string]string)
	}
	config.Params["time_zone"] = "'+00:00'"
}

func mariaDBDSN(parsed *url.URL) (string, error) {
	if parsed.Host == "" {
		return "", errors.New("MariaDB URL must include a host")
	}
	config := mysql.NewConfig()
	config.Net = "tcp"
	port := parsed.Port()
	if port == "" {
		port = "3306"
	}
	config.Addr = net.JoinHostPort(parsed.Hostname(), port)
	config.DBName = strings.TrimPrefix(parsed.EscapedPath(), "/")
	if database, err := url.PathUnescape(config.DBName); err != nil {
		return "", fmt.Errorf("decode MariaDB database name: %w", err)
	} else {
		config.DBName = database
	}
	if config.DBName == "" {
		return "", errors.New("MariaDB URL must include a database name")
	}
	setMariaDBDefaults(config)
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
		case "tls":
			useTLS, err := strconv.ParseBool(values[0])
			if err != nil {
				return "", fmt.Errorf("parse MariaDB TLS option: %w", err)
			}
			if useTLS {
				config.TLSConfig = "true"
			}
		default:
			return "", fmt.Errorf("unsupported MariaDB URL option %q", key)
		}
	}
	return config.FormatDSN(), nil
}
