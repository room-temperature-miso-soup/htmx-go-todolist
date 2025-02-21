// main.go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"htmx-go-todolist/internal/types"
	"htmx-go-todolist/view"
	"htmx-go-todolist/view/components/todo" // Import the todo components package
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var logger *log.Logger

func main() {
	logger = log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Lshortfile)

	db, err := sql.Open("sqlite3", "file:./todos.db?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		logger.Fatalf("Failed to open database: %v", err)
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
		logger.Fatalf("Failed to create table: %v", err)
	}

	mux := http.NewServeMux()

	// GET / - Initial Page Load
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		logger.Println("GET / request received")
		todos, err := loadTodos(db)
		if err != nil {
			logger.Printf("Error loading todos: %v\n", err)
			http.Error(w, "Failed to load todos", http.StatusInternalServerError)
			return
		}
		activeFilter := "all"
		view.Index(todos, activeFilter).Render(context.Background(), w)
	})

	// POST /add-todo - Add a New Todo
	mux.HandleFunc("POST /add-todo", func(w http.ResponseWriter, r *http.Request) {
		logger.Println("POST /add-todo request received")
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
		logger.Printf("Adding new todo: %+v\n", newTodo)

		err := insertTodo(db, newTodo)
		if err != nil {
			logger.Printf("Error inserting todo: %v\n", err)
			http.Error(w, "Failed to save todo", http.StatusInternalServerError)
			return
		}

		// After adding a todo, re-fetch all todos to update the list
		todos, err := loadTodos(db) // Or getFilteredTodos with current filter if you maintain filter state
		if err != nil {
			logger.Printf("Error reloading todos after add: %v\n", err)
			http.Error(w, "Failed to reload todos", http.StatusInternalServerError)
			return
		}
		todo.List(todos).Render(context.Background(), w) // Render only the todo list to update the UI
	})

	// POST /toggle-todo/{id} - Toggle Todo Completion
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

		logger.Printf("Toggled todo: %+v\n", updatedTodo)
		todo.Item(updatedTodo).Render(context.Background(), w) // Render just the updated item
	})

	// DELETE /delete-todo/{id} - Delete Todo
	mux.HandleFunc("DELETE /delete-todo/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		logger.Printf("DELETE /delete-todo request received for ID: %s\n", id)
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

		w.WriteHeader(http.StatusOK)

		// After deleting, re-fetch and re-render the todo list
		todos, err := loadTodos(db) // Or getFilteredTodos with current filter
		if err != nil {
			logger.Printf("Error reloading todos after delete: %v\n", err)
			http.Error(w, "Failed to reload todos", http.StatusInternalServerError)
			return
		}
		todo.List(todos).Render(context.Background(), w) // Re-render the todo list
	})

	// GET /todos - Filter Todos (HTMX Request)
	mux.HandleFunc("GET /todos", func(w http.ResponseWriter, r *http.Request) {
		logger.Println("GET /todos request received")

		filter := r.URL.Query().Get("filter")
		if filter == "" {
			filter = "all" // Default filter
		}
		logger.Printf("Filter applied: %s\n", filter)

		filteredTodos, err := getFilteredTodos(db, filter)
		if err != nil {
			logger.Printf("Error filtering todos: %v", err)
			http.Error(w, "Failed to filter todos", http.StatusInternalServerError)
			return
		}

		// *** RENDER ONLY THE TODO LIST COMPONENT ***
		todo.List(filteredTodos).Render(context.Background(), w) // Correct: Render only the list
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	logger.Printf("Server starting on %s\n", server.Addr)
	log.Fatal(server.ListenAndServe())
}

// --- Database Helper Functions ---

func insertTodo(db *sql.DB, todo types.Todo) error {
	logger.Printf("Inserting todo into database: %+v\n", todo)
	_, err := db.Exec("INSERT INTO todos (id, text, completed, created_at, priority, category) VALUES (?, ?, ?, ?, ?, ?)",
		todo.ID, todo.Text, todo.Completed, todo.CreatedAt, todo.Priority, todo.Category)
	if err != nil {
		logger.Printf("Error inserting todo: %v\n", err)
		return err
	}
	return nil
}

func loadTodos(db *sql.DB) ([]types.Todo, error) {
	logger.Println("Loading todos from database")
	rows, err := db.Query("SELECT id, text, completed, created_at, priority, category FROM todos ORDER BY created_at DESC")
	if err != nil {
		logger.Printf("Query Error: %v\n", err)
		return nil, err
	}
	defer rows.Close()

	var todos []types.Todo
	for rows.Next() {
		var todo types.Todo
		var completed int
		err := rows.Scan(&todo.ID, &todo.Text, &completed, &todo.CreatedAt, &todo.Priority, &todo.Category)
		if err != nil {
			logger.Printf("Scan Error: %v\n", err)
			return nil, err
		}
		todo.Completed = completed != 0
		todos = append(todos, todo)
	}
	if err = rows.Err(); err != nil {
		logger.Printf("Iteration error: %v\n", err)
		return nil, err
	}

	logger.Printf("Loaded %d todos from database\n", len(todos))
	return todos, nil
}

func toggleTodoCompletion(db *sql.DB, id string) error {
	logger.Printf("Toggling completion for todo ID: %s\n", id)
	_, err := db.Exec("UPDATE todos SET completed = NOT completed WHERE id = ?", id)
	if err != nil {
		logger.Printf("Error toggling todo: %v\n", err)
		return err
	}
	return nil
}

func deleteTodo(db *sql.DB, id string) error {
	logger.Printf("Deleting todo ID: %s\n", id)
	_, err := db.Exec("DELETE FROM todos WHERE id = ?", id)
	if err != nil {
		logger.Printf("Error deleting todo: %v\n", err)
		return err
	}
	return nil
}

func getTodoByID(db *sql.DB, id string) (types.Todo, error) {
	logger.Printf("Retrieving todo by ID: %s\n", id)
	row := db.QueryRow("SELECT id, text, completed, created_at, priority, category FROM todos WHERE id = ?", id)

	var todo types.Todo
	var completed int
	err := row.Scan(&todo.ID, &todo.Text, &completed, &todo.CreatedAt, &todo.Priority, &todo.Category)
	if err != nil {
		logger.Printf("Error retrieving todo by ID %v\n", err)
		return types.Todo{}, err
	}
	todo.Completed = completed != 0
	logger.Printf("Retrieved Todo: %+v\n", todo)
	return todo, nil
}

func generateTodoID() string {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	logger.Printf("Generated new Todo ID: %v\n", id)
	return id
}

func getFilteredTodos(db *sql.DB, filter string) ([]types.Todo, error) {
	logger.Printf("Retrieving todos with filter: %s\n", filter)

	var rows *sql.Rows
	var err error

	switch filter {
	case "active":
		rows, err = db.Query("SELECT id, text, completed, created_at, priority, category FROM todos WHERE completed = 0 ORDER BY created_at DESC")
	case "completed":
		rows, err = db.Query("SELECT id, text, completed, created_at, priority, category FROM todos WHERE completed = 1 ORDER BY created_at DESC")
	default: // "all" or any other value
		rows, err = db.Query("SELECT id, text, completed, created_at, priority, category FROM todos ORDER BY created_at DESC")
	}

	if err != nil {
		logger.Printf("Error querying database for filtered todos: %v\n", err)
		return nil, err
	}
	defer rows.Close()

	var todos []types.Todo
	for rows.Next() {
		var todo types.Todo
		var completed int
		if err := rows.Scan(&todo.ID, &todo.Text, &completed, &todo.CreatedAt, &todo.Priority, &todo.Category); err != nil {
			logger.Printf("Error scanning row: %v\n", err)
			return nil, err
		}
		todo.Completed = completed != 0
		todos = append(todos, todo)
	}

	if err = rows.Err(); err != nil {
		logger.Printf("Error during row iteration: %v\n", err)
		return nil, err
	}

	logger.Printf("Retrieved %d todos with filter: %s\n", len(todos), filter)
	return todos, nil
}
