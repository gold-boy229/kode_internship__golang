package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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
	username             = "test_user"
	password             = "test_password"
	hostname             = "127.0.0.1:3306"
	dbname               = "kode_internship"
	YandexSpeller_apiURL = "https://speller.yandex.net/services/spellservice.json/checkText"
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

type SpellCheckResponse struct {
	Code        int      `json:"code"`
	Pos         int      `json:"pos"`
	Row         int      `json:"row"`
	Col         int      `json:"col"`
	Len         int      `json:"len"`
	Word        string   `json:"word"`
	Suggestions []string `json:"s"`
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

	isUserValid, err := validateUser(r, db, user_id)
	if err != nil || !isUserValid {
		log.Print("err = ", err, " isUserValid = ", isUserValid)
		http.Error(w, "Invalid user credentials", http.StatusBadRequest)
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

	isUserValid, err := validateUser(r, db, int(note.User_id))
	if err != nil || !isUserValid {
		log.Print("err = ", err, " isUserValid = ", isUserValid)
		http.Error(w, "Invalid user credentials", http.StatusBadRequest)
		return
	}

	valid_content, err := YandexSpeller_API(note.Content)
	if err != nil {
		log.Printf("Error in YandexSpeller_API happend %s", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Print("valid_content = ", valid_content)

	note.Content = valid_content
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

func YandexSpeller_API(text_to_validate string) (string, error) {
	formData := url.Values{"text": {text_to_validate}}

	req, err := http.NewRequest("POST", YandexSpeller_apiURL, strings.NewReader(formData.Encode()))
	if err != nil {
		log.Println(err)
		return "", errors.New("Cannot create POST-request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return "", errors.New("Cannot receive response")
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)

	final_text, err := processYandexSpellerResponse(buf, text_to_validate)
	if err != nil {
		log.Println(err)
		return "", errors.New("Error in processYandexSpellerResponse()")
	}

	return final_text, nil
}

func processYandexSpellerResponse(buf bytes.Buffer, input_text string) (string, error) {
	var errors []SpellCheckResponse
	json.Unmarshal(buf.Bytes(), &errors)

	final_text := ""
	ptr := 0

	for _, error_i := range errors {
		final_text += input_text[ptr:error_i.Pos]
		final_text += error_i.Suggestions[0]
		ptr = error_i.Pos + error_i.Len
	}

	final_text += input_text[ptr:]

	return final_text, nil
}

func getCredentialsFromAuthHeader(r *http.Request) (string, string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", "", errors.New("Unauthorized")
	}

	log.Print("authHeader = ", authHeader)

	// authHeader = Basic VHdvOnBhc3N3b3Jk
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Basic" {
		return "", "", errors.New("Unauthorized")
	}

	log.Print("parts = ", parts)

	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", errors.New("Unauthorized")
	}

	log.Print("decoded = ", decoded)

	credentials := strings.SplitN(string(decoded), ":", 2)
	if len(credentials) != 2 {
		return "", "", errors.New("Unauthorized")
	}

	log.Print("credentials = ", credentials)

	username := credentials[0]
	password := credentials[1]

	log.Print("username = ", username)
	log.Print("password = ", password)

	return username, password, nil
}

func validateUser(r *http.Request, db *sql.DB, user_id int) (bool, error) {
	var isUserValid bool

	username, password, err := getCredentialsFromAuthHeader(r)
	if err != nil {
		return false, err
	}

	query := `
		SELECT exists(
			SELECT 1
			FROM user
			WHERE id = ? AND username = ? AND password = ?
		)`

	if err := db.QueryRow(query, user_id, username, password).Scan(&isUserValid); err != nil {
		return false, err
	}

	return isUserValid, nil
}
