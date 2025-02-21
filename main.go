package main

import (
	"context"
	"database/sql"
	"fmt"
	"htmx-go-todolist/view/components/todo"
	"log"
	"net/http"
	"os"
	"time"

	"htmx-go-todolist/internal/types" // Your module path
	"htmx-go-todolist/view"           // Your module path

	_ "github.com/mattn/go-sqlite3"
)

var logger *log.Logger // Declare a global logger

func main() {
	// Initialize the logger
	logger = log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)

	// Database Setup
	db, err := sql.Open("sqlite3", "file:./todos.db?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		logger.Fatalf("Failed to open database: %v", err) // Use logger.Fatalf
	}
	defer db.Close()

	createTableSQL := `
		CREATE TABLE IF NOT EXISTS todos (
			id TEXT PRIMARY KEY,
			text TEXT NOT NULL,
			completed INTEGER NOT NULL,
			created_at DATETIME NOT NULL,
			priority TEXT,
			category TEXT
		);
	`
	_, err = db.Exec(createTableSQL)
	if err != nil {
		logger.Fatalf("Failed to create table: %v", err) // Use logger.Fatalf
	}

	mux := http.NewServeMux()

	// Load todos on startup
	todos, err := loadTodos(db)
	if err != nil {
		logger.Fatalf("Failed to load todos on startup: %v", err) // Use logger.Fatalf
	}

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		logger.Println("GET / request received") // Log the request
		todos, err = loadTodos(db)
		if err != nil {
			logger.Printf("Error loading todos: %v\n", err) // Use logger.Printf
			http.Error(w, "Failed to load todos", http.StatusInternalServerError)
			return
		}
		view.Index(todos).Render(context.Background(), w)
	})

	mux.HandleFunc("POST /add-todo", func(w http.ResponseWriter, r *http.Request) {
		logger.Println("POST /add-todo request received") // Log the request
		if err := r.ParseForm(); err != nil {
			logger.Printf("Error parsing form: %v\n", err)
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		newTodo := types.Todo{
			ID:        generateTodoID(),
			Text:      r.FormValue("todoText"),
			CreatedAt: time.Now(),
			Priority:  r.FormValue("priority"),
			Category:  r.FormValue("category"),
			Completed: false,
		}
		logger.Printf("Adding new todo: %+v\n", newTodo) // Log the new todo

		err := insertTodo(db, newTodo)
		if err != nil {
			logger.Printf("Error inserting todo: %v\n", err)
			http.Error(w, "Failed to save todo", http.StatusInternalServerError)
			return
		}

		todos = append([]types.Todo{newTodo}, todos...) // keep in memory for now
		todo.Item(newTodo).Render(context.Background(), w)
	})

	mux.HandleFunc("POST /toggle-todo/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		logger.Printf("POST /toggle-todo request received for ID: %s\n", id)
		if id == "" {
			logger.Println("Error: ID is required")
			http.Error(w, "ID is required", http.StatusBadRequest)
			return
		}

		err := toggleTodoCompletion(db, id)
		if err != nil {
			logger.Printf("Error toggling todo completion: %v\n", err)
			http.Error(w, "Failed to update todo", http.StatusInternalServerError)
			return
		}

		updatedTodo, err := getTodoByID(db, id)
		if err != nil {
			logger.Printf("Error getting updated todo: %v\n", err)
			http.Error(w, "Failed to retrieve updated todo", http.StatusInternalServerError)
			return
		}

		//  Update the todo IN-MEMORY.
		for i := range todos {
			if todos[i].ID == id {
				todos[i] = updatedTodo // Replace the old todo with the updated one.
				break
			}
		}
		logger.Printf("Toggled todo: %+v\n", updatedTodo)
		todo.Item(updatedTodo).Render(context.Background(), w)
	})

	mux.HandleFunc("DELETE /delete-todo/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		logger.Printf("DELETE /delete-todo request received for ID: %s\n", id) // log id
		if id == "" {
			logger.Println("Error: ID is required")
			http.Error(w, "ID is required", http.StatusBadRequest)
			return
		}

		err := deleteTodo(db, id)
		if err != nil {
			logger.Printf("Error deleting todo: %v\n", err)
			http.Error(w, "Failed to delete todo", http.StatusInternalServerError)
			return
		}

		// Remove from IN-MEMORY slice.
		for i := range todos {
			if todos[i].ID == id {
				todos = append(todos[:i], todos[i+1:]...)
				break
			}
		}

		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	logger.Printf("Server starting on %s\n", server.Addr) // log server
	log.Fatal(server.ListenAndServe())                    //  use standard log.Fatal
}

// --- Database Helper Functions ---

func insertTodo(db *sql.DB, todo types.Todo) error {
	logger.Printf("Inserting todo into database: %+v\n", todo) // Log before insert
	_, err := db.Exec("INSERT INTO todos (id, text, completed, created_at, priority, category) VALUES (?, ?, ?, ?, ?, ?)",
		todo.ID, todo.Text, todo.Completed, todo.CreatedAt, todo.Priority, todo.Category)
	if err != nil {
		logger.Printf("Error inserting todo: %v\n", err) // log on error
	}
	return err
}

func loadTodos(db *sql.DB) ([]types.Todo, error) {
	logger.Println("Loading todos from database")
	rows, err := db.Query("SELECT id, text, completed, created_at, priority, category FROM todos ORDER BY created_at DESC")
	if err != nil {
		logger.Printf("Query Error: %v\n", err) // Log the error.
		return nil, err
	}
	defer rows.Close()

	var todos []types.Todo
	for rows.Next() {
		var todo types.Todo
		var completed int
		err := rows.Scan(&todo.ID, &todo.Text, &completed, &todo.CreatedAt, &todo.Priority, &todo.Category)
		if err != nil {
			logger.Printf("Scan Error: %v\n", err) // Log the error
			return nil, err
		}
		todo.Completed = completed != 0
		todos = append(todos, todo)
	}
	if err = rows.Err(); err != nil { // Check for errors during iteration
		logger.Printf("Iteration error: %v\n", err)
		return nil, err
	}

	logger.Printf("Loaded %d todos from database\n", len(todos)) // logitud
	return todos, nil
}

func toggleTodoCompletion(db *sql.DB, id string) error {
	logger.Printf("Toggling completion for todo ID: %s\n", id)
	_, err := db.Exec("UPDATE todos SET completed = NOT completed WHERE id = ?", id)
	if err != nil {
		logger.Printf("Error toggling todo: %v\n", err) // log on error
	}
	return err
}

func deleteTodo(db *sql.DB, id string) error {
	logger.Printf("Deleting todo ID: %s\n", id) // logitud
	_, err := db.Exec("DELETE FROM todos WHERE id = ?", id)
	if err != nil {
		logger.Printf("Error deleting todo: %v\n", err) // log on error
	}
	return err
}

func getTodoByID(db *sql.DB, id string) (types.Todo, error) {
	logger.Printf("Retrieving todo by ID: %s\n", id) // log
	row := db.QueryRow("SELECT id, text, completed, created_at, priority, category FROM todos WHERE id = ?", id)

	var todo types.Todo
	var completed int
	err := row.Scan(&todo.ID, &todo.Text, &completed, &todo.CreatedAt, &todo.Priority, &todo.Category)
	if err != nil {
		logger.Printf("Error retrieving todo by ID %v\n", err) // log error
		return types.Todo{}, err
	}
	todo.Completed = completed != 0
	logger.Printf("Retrieved Todo: %+v\n", todo) // Log the todo.
	return todo, nil
}

func generateTodoID() string {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	logger.Printf("Generated new Todo ID: %v\n", id) // Log ID.
	return id
}
