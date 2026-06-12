package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func ConnectPostgres(
    host,
    port,
    user,
    password,
    dbname string,
) (*sql.DB, error) {

    psqlInfo := fmt.Sprintf(
        "host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
        host,
        port,
        user,
        password,
        dbname,
    )

    db, err := sql.Open("postgres", psqlInfo)
    if err != nil {
        return nil, err
    }

    db.SetMaxOpenConns(100)
    db.SetMaxIdleConns(25)
    db.SetConnMaxLifetime(time.Hour)
    db.SetConnMaxIdleTime(15 * time.Minute)

    if err := db.Ping(); err != nil {
        return nil, err
    }

    DB = db

    return db, nil
}