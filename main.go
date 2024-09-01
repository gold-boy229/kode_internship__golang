package main

import (
	"net/http"

	"database/sql"

	_ "github.com/go-sql-driver/mysql"

	"log"
	"time"

	"context"
	"fmt"
)

const (
	username = "test_user"
	password = "test_password"
	hostname = "127.0.0.1:3306"
	dbname   = "kode_internship"
)

/* 	user
id	username
1 	One
2	Two
*/

type user struct {
	id       int32
	username string
}

/*	note
id	user_id	content	created_at
*/

type note struct {
	id         int32
	user_id    int32
	content    string
	created_at int64
}

func dsn(dbName string) string {
	return fmt.Sprintf("%s:%s@tcp(%s)/%s", username, password, hostname, dbName)
}

func dbConnection() (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn(""))
	if err != nil {
		log.Printf("Error %s when opening DB\n", err)
		return nil, err
	}

	ctx, cancelfunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelfunc()

	res, err := db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS "+dbname)
	if err != nil {
		log.Printf("Error %s when creating DB\n", err)
		return nil, err
	}
	no, err := res.RowsAffected()
	if err != nil {
		log.Printf("Error %s when fetching rows", err)
		return nil, err
	}
	log.Printf("rows affected: %d\n", no)

	db.Close()
	db, err = sql.Open("mysql", dsn(dbname))
	if err != nil {
		log.Printf("Error %s when opening DB", err)
		return nil, err
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Minute * 5)

	ctx, cancelfunc = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelfunc()
	err = db.PingContext(ctx)
	if err != nil {
		log.Printf("Errors %s pinging DB", err)
		return nil, err
	}
	log.Printf("Connected to DB %s successfully\n", dbname)
	return db, nil
}

func main() {
	db, err := dbConnection()
	if err != nil {
		log.Printf("Error %s when getting db connection", err)
		return
	}
	defer db.Close()
	log.Printf("Successfully connected to database")

	note := note{user_id: 1, content: "test_content"}
	noteInsert(db, note)

	// r := chi.NewRouter()
	// r.Use(middleware.Logger)
	// r.Get("/add-note", addNote)

	// http.ListenAndServe(":3000", r)
}

func addNote(responseWriter http.ResponseWriter, r *http.Request) {
	responseWriter.Write([]byte("add Note"))
	// note := note{user_id: 1, content: "test_content"}
	// noteInsert(db, note)
}

// INSERT INTO note(user_id, content, created_at) VALUES (?, ?, ?)

func noteInsert(db *sql.DB, note note) error {
	query := "INSERT INTO note(user_id, content) VALUES (?, ?)"
	ctx, cancelfunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelfunc()
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		log.Printf("Error %s when preparing SQL statement", err)
		return err
	}
	defer stmt.Close()

	res, err := stmt.ExecContext(ctx, note.user_id, note.content)
	if err != nil {
		log.Printf("Error %s when inserting row into note table", err)
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		log.Printf("Error %s when finding rows affected", err)
		return err
	}
	log.Printf("%d note(s) created ", rows)
	return nil
}
