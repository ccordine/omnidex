package datasource

import (
	"fmt"
	"strings"
)

type Connection struct {
	Driver       string
	Host         string
	Port         int
	DatabaseName string
	Username     string
	Password     string
	SSLMode      string
	UseDSN       bool
	DSN          string
	ReadOnly     bool
}

func BuildPostgresDSN(conn Connection) (string, error) {
	if conn.UseDSN {
		dsn := strings.TrimSpace(conn.DSN)
		if dsn == "" {
			return "", fmt.Errorf("dsn is required")
		}
		return dsn, nil
	}
	host := strings.TrimSpace(conn.Host)
	db := strings.TrimSpace(conn.DatabaseName)
	user := strings.TrimSpace(conn.Username)
	if host == "" || db == "" || user == "" {
		return "", fmt.Errorf("host, database_name, and username are required")
	}
	port := conn.Port
	if port <= 0 {
		port = 5432
	}
	sslMode := strings.TrimSpace(conn.SSLMode)
	if sslMode == "" {
		sslMode = "prefer"
	}
	parts := []string{
		fmt.Sprintf("host=%s", host),
		fmt.Sprintf("port=%d", port),
		fmt.Sprintf("dbname=%s", db),
		fmt.Sprintf("user=%s", user),
		fmt.Sprintf("sslmode=%s", sslMode),
	}
	if pwd := strings.TrimSpace(conn.Password); pwd != "" {
		parts = append(parts, "password="+pwd)
	}
	return strings.Join(parts, " "), nil
}
