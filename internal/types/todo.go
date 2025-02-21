// types/todo.go
package types

import "time"

type Todo struct {
	ID        string
	Text      string
	Completed bool
	CreatedAt time.Time
	Priority  string
	Category  string
}
