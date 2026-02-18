// Package redmine provides a REST API client for the Redmine project management system.
// It supports paginated issue fetching, user validation, and project listing.
package redmine

// IDRef is a common Redmine reference type used for project, tracker, status,
// priority, author, and other named references.
type IDRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Issue represents a single Redmine issue as returned by the REST API.
type Issue struct {
	ID          int     `json:"id"`
	Subject     string  `json:"subject"`
	Description string  `json:"description"`
	Project     IDRef   `json:"project"`
	Tracker     IDRef   `json:"tracker"`
	Status      IDRef   `json:"status"`
	Priority    IDRef   `json:"priority"`
	Author      IDRef   `json:"author"`
	AssignedTo  *IDRef  `json:"assigned_to"`
	IsPrivate   bool    `json:"is_private"`
	CreatedOn   string  `json:"created_on"`
	UpdatedOn   string  `json:"updated_on"`
}

// IssueList is the paginated envelope returned by GET /issues.json.
type IssueList struct {
	Issues     []Issue `json:"issues"`
	TotalCount int     `json:"total_count"`
	Offset     int     `json:"offset"`
	Limit      int     `json:"limit"`
}

// Membership represents a project membership with associated roles.
type Membership struct {
	ID      int     `json:"id"`
	Project IDRef   `json:"project"`
	Roles   []IDRef `json:"roles"`
}

// User represents a Redmine user as returned by GET /users/current.json.
type User struct {
	ID          int          `json:"id"`
	Login       string       `json:"login"`
	FirstName   string       `json:"firstname"`
	LastName    string       `json:"lastname"`
	Mail        string       `json:"mail"`
	Admin       bool         `json:"admin"`
	Memberships []Membership `json:"memberships"`
}

// UserResponse is the JSON envelope wrapping a User from /users/current.json.
type UserResponse struct {
	User User `json:"user"`
}

// Project represents a Redmine project as returned by GET /projects.json.
type Project struct {
	ID         int    `json:"id"`
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
}

// ProjectList is the paginated envelope returned by GET /projects.json.
type ProjectList struct {
	Projects   []Project `json:"projects"`
	TotalCount int       `json:"total_count"`
	Offset     int       `json:"offset"`
	Limit      int       `json:"limit"`
}
