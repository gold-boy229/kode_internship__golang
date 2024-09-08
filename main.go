package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
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
id	username password
1 	One		 password
2	Two		 password
*/

type User struct {
	Id       int    `json:"id,omitempty"`
	Username string `json:"username"`
}

type Note struct {
	Id         int    `json:"id"`
	User_id    int    `json:"user_id,omitempty"`
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

var (
	logfile *os.File
	logger  *log.Logger
	db      *sql.DB
)

func init() {
	var err error
	logfile, err = os.OpenFile("./logs/0.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	logger = log.New(logfile, "INFO: ", log.LstdFlags|log.Lshortfile)

	db, err = dbConnection()
	if err != nil {
		logger.Printf("Error %s when getting db connection", err)
		return
	}
	logger.Print("Successfully connected to database")
}

func dsn(dbName string) string {
	return fmt.Sprintf("%s:%s@tcp(%s)/%s", username, password, hostname, dbName)
}

func dbConnection() (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn(""))
	if err != nil {
		logger.Printf("Error %s when opening DB\n", err)
		return nil, err
	}

	ctx, cancelfunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelfunc()

	res, err := db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS "+dbname)
	if err != nil {
		logger.Printf("Error %s when creating DB\n", err)
		return nil, err
	}
	no, err := res.RowsAffected()
	if err != nil {
		logger.Printf("Error %s when fetching rows", err)
		return nil, err
	}
	logger.Printf("rows affected: %d\n", no)

	db.Close()
	db, err = sql.Open("mysql", dsn(dbname))
	if err != nil {
		logger.Printf("Error %s when opening DB", err)
		return nil, err
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Minute * 5)

	ctx, cancelfunc = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelfunc()
	err = db.PingContext(ctx)
	if err != nil {
		logger.Printf("Errors %s pinging DB", err)
		return nil, err
	}
	logger.Printf("Connected to DB %s successfully\n", dbname)
	return db, nil
}

func main() {
	defer logfile.Close()
	defer db.Close()

	log.Print("Server is successfully started and now is running")

	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Get("/notes", getUserNotes)
	router.Post("/add-note", addNote)

	http.ListenAndServe(":3000", router)
}

func getUserNotes(res http.ResponseWriter, req *http.Request) {
	user_idStr := req.URL.Query().Get("user_id")
	user_id, err := strconv.Atoi(user_idStr)
	if err != nil {
		http.Error(res, "Invalid user_id parameter", http.StatusBadRequest)
		return
	}

	isUserValid, err := validateUser(req, user_id)
	if err != nil || !isUserValid {
		logger.Print("err = ", err, " isUserValid = ", isUserValid)
		http.Error(res, "Invalid user credentials", http.StatusBadRequest)
		return
	}

	notes, err := selectUserNotes(int(user_id))
	if err != nil {
		logger.Printf("Error %s while getting user notes", err)
		return
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(notes)
}

func selectUserNotes(user_id int) ([]Note, error) {
	query :=
		`SELECT id, content 
		FROM note
		WHERE user_id = ?`
	ctx, cancelfunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelfunc()

	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		logger.Printf("Error %s when preparing SQL statement", err)
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

func addNote(res http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Content-Type") != "application/json" {
		http.Error(res, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	var note Note
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(&note)
	if err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}

	isUserValid, err := validateUser(req, int(note.User_id))
	if err != nil || !isUserValid {
		logger.Print("err = ", err, " isUserValid = ", isUserValid)
		http.Error(res, "Invalid user credentials", http.StatusBadRequest)
		return
	}

	valid_content, err := YandexSpeller_API(note.Content)
	if err != nil {
		logger.Printf("Error in YandexSpeller_API happend %s", err)
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}
	logger.Print("valid_content = ", valid_content)

	note.Content = valid_content
	insertNote(note)
}

func insertNote(note Note) error {
	query := "INSERT INTO note(user_id, content) VALUES (?, ?)"
	ctx, cancelfunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelfunc()
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		logger.Printf("Error %s when preparing SQL statement", err)
		return err
	}
	defer stmt.Close()

	res, err := stmt.ExecContext(ctx, note.User_id, note.Content)
	if err != nil {
		logger.Printf("Error %s when inserting row into note table", err)
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		logger.Printf("Error %s when finding rows affected", err)
		return err
	}
	logger.Printf("%d note(s) created ", rows)
	return nil
}

func YandexSpeller_API(text_to_validate string) (string, error) {
	formData := url.Values{"text": {text_to_validate}}

	req, err := http.NewRequest("POST", YandexSpeller_apiURL, strings.NewReader(formData.Encode()))
	if err != nil {
		logger.Println(err)
		return "", errors.New("Cannot create POST-request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Println(err)
		return "", errors.New("Cannot receive response")
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)

	final_text, err := processYandexSpellerResponse(buf, text_to_validate)
	if err != nil {
		logger.Println(err)
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

func getCredentialsFromAuthHeader(req *http.Request) (string, string, error) {
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		return "", "", errors.New("Unauthorized")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Basic" {
		return "", "", errors.New("Unauthorized")
	}

	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", errors.New("Unauthorized")
	}

	credentials := strings.SplitN(string(decoded), ":", 2)
	if len(credentials) != 2 {
		return "", "", errors.New("Unauthorized")
	}

	username := credentials[0]
	password := credentials[1]

	return username, password, nil
}

func validateUser(req *http.Request, user_id int) (bool, error) {
	var isUserValid bool

	username, password, err := getCredentialsFromAuthHeader(req)
	if err != nil {
		return false, err
	}

	query := `
		SELECT EXISTS(
			SELECT 1
			FROM user
			WHERE (id = ? AND username = ? AND password = ?)
		)`

	if err := db.QueryRow(query, user_id, username, password).Scan(&isUserValid); err != nil {
		return false, err
	}

	return isUserValid, nil
}
