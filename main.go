package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"database/sql"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

type User struct {
	Id       int32  `json:"id,omitempty"`
	Username string `json:"username"`
}

/*	note
id	user_id	content	created_at
*/

type Note struct {
	Id         int32  `json:"id"`
	User_id    int32  `json:"user_id,omitempty"`
	Content    string `json:"content"`
	Created_at int64  `json:"created_at,omitempty"`
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

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Get("/notes", func(w http.ResponseWriter, r *http.Request) {
		getUserNotes(w, r, db)
	})
	r.Post("/add-note", func(w http.ResponseWriter, r *http.Request) {
		addNote(w, r, db)
	})

	http.ListenAndServe(":3000", r)
}

func getUserNotes(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	user_idStr := r.URL.Query().Get("user_id")
	user_id, err := strconv.Atoi(user_idStr)
	if err != nil {
		// Handle the error (e.g., return a 400 Bad Request response)
		http.Error(w, "Invalid user_id parameter", http.StatusBadRequest)
		return
	}

	notes, err := selectUserNotes(db, int32(user_id))
	if err != nil {
		log.Printf("Error %s while getting user notes", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notes)
}

func selectUserNotes(db *sql.DB, user_id int32) ([]Note, error) {
	query :=
		`SELECT id, content 
		FROM note
		WHERE user_id = ?`
	ctx, cancelfunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelfunc()

	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		log.Printf("Error %s when preparing SQL statement", err)
		return []Note{}, err
	}
	defer stmt.Close()

	rows, err := stmt.QueryContext(ctx, user_id)
	if err != nil {
		return []Note{}, err
	}
	defer rows.Close()
	var notes = []Note{}
	for rows.Next() {
		var row Note
		if err := rows.Scan(&row.Id, &row.Content); err != nil {
			return []Note{}, err
		}
		notes = append(notes, row)
	}
	if err := rows.Err(); err != nil {
		return []Note{}, err
	}
	return notes, nil
}

func addNote(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	var note Note
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	insertNote(db, note)
}

func insertNote(db *sql.DB, note Note) error {
	query := "INSERT INTO note(user_id, content) VALUES (?, ?)"
	ctx, cancelfunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelfunc()
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		log.Printf("Error %s when preparing SQL statement", err)
		return err
	}
	defer stmt.Close()

	res, err := stmt.ExecContext(ctx, note.User_id, note.Content)
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
