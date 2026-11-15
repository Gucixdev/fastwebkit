// mysql / mariadb backend (go-sql-driver/mysql). Single file: delete it and mysql leaves the
// build. DSN "mysql://user:pass@host:3306/dbname" (converted to the driver's own format).

package kit

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

var myDDL = strings.NewReplacer(
	"TEXT PRIMARY KEY", "VARCHAR(191) PRIMARY KEY",
	"TEXT UNIQUE", "VARCHAR(191) UNIQUE",
	"BLOB", "LONGBLOB",
)

func init() { register(backend{scheme: "mysql://", open: openMySQL, ddl: myDDL, mysql: true}) }

func openMySQL(dsn string) (*sql.DB, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: bad dsn")
	}
	cfg := mysql.NewConfig()
	cfg.User = u.User.Username()
	cfg.Passwd, _ = u.User.Password()
	cfg.Net, cfg.Addr = "tcp", u.Host
	cfg.DBName = strings.TrimPrefix(u.Path, "/")
	cfg.ParseTime = true // DATETIME -> time.Time
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetConnMaxLifetime(3 * time.Minute) // below typical server wait_timeout
	return db, nil
}
