package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var DB *sql.DB


func InitDB() {
	db, err := sql.Open("sqlite", "forum.db")
	if err != nil {
		log.Fatal(err)
	}

	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		log.Fatal("Failed to enable foreign keys:", err)
	}

	DB = db
	CreateTables(db)
	migrateImagePath(db)
	insertDefaultCategories(db)
}

func CreateTables(db *sql.DB) {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			username   TEXT NOT NULL UNIQUE,
			email      TEXT NOT NULL UNIQUE,
			password   TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS categories (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT NOT NULL UNIQUE CHECK(name IN ('Education','Sports','Religious','Technology','Entertainment','Health','Other')),
			description TEXT,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS posts (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id     INTEGER NOT NULL,
			category_id INTEGER NOT NULL,
			title       TEXT NOT NULL,
			content     TEXT NOT NULL,
			image_path  TEXT NOT NULL DEFAULT '',
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id)     REFERENCES users(id)      ON DELETE CASCADE,
			FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE
		);`,

		`CREATE TABLE IF NOT EXISTS post_categories (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			post_id     INTEGER NOT NULL,
			category_id INTEGER NOT NULL,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (post_id)     REFERENCES posts(id)      ON DELETE CASCADE,
			FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE,
			UNIQUE(post_id, category_id)
		);`,
		`CREATE TABLE IF NOT EXISTS comments (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			post_id    INTEGER NOT NULL,
			user_id    INTEGER NOT NULL,
			content    TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (post_id) REFERENCES posts(id)  ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id)  ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS post_reactions (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id    INTEGER NOT NULL,
			post_id    INTEGER NOT NULL,
			value      INTEGER NOT NULL CHECK(value IN (1, -1)),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
			UNIQUE(user_id, post_id)
		);`,
		`CREATE TABLE IF NOT EXISTS comment_reactions (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id    INTEGER NOT NULL,
			comment_id INTEGER NOT NULL,
			value      INTEGER NOT NULL CHECK(value IN (1, -1)),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id)    REFERENCES users(id)    ON DELETE CASCADE,
			FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE CASCADE,
			UNIQUE(user_id, comment_id)
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT PRIMARY KEY,
			user_id    INTEGER NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS oauth_accounts (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id     INTEGER NOT NULL,
			provider    TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			UNIQUE(provider, provider_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_posts_user              ON posts(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_posts_category          ON posts(category_id);`,
		`CREATE INDEX IF NOT EXISTS idx_post_categories_post    ON post_categories(post_id);`,
		`CREATE INDEX IF NOT EXISTS idx_post_categories_cat     ON post_categories(category_id);`,
		`CREATE INDEX IF NOT EXISTS idx_comments_post           ON comments(post_id);`,
		`CREATE INDEX IF NOT EXISTS idx_comments_user           ON comments(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_post_reactions_post     ON post_reactions(post_id);`,
		`CREATE INDEX IF NOT EXISTS idx_comment_reactions       ON comment_reactions(comment_id);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user           ON sessions(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires        ON sessions(expires_at);`,
		`CREATE TRIGGER IF NOT EXISTS update_post_timestamp
		AFTER UPDATE ON posts
		BEGIN
			UPDATE posts SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
		END;`,
		`CREATE TRIGGER IF NOT EXISTS update_comment_timestamp
		AFTER UPDATE ON comments
		BEGIN
			UPDATE comments SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
		END;`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			log.Fatal(err)
		}
	}
}

// migrateImagePath safely adds image_path column to existing databases.
func migrateImagePath(db *sql.DB) {
	// Ignore error — column already exists in fresh DBs created by CreateTables
	db.Exec(`ALTER TABLE posts ADD COLUMN image_path TEXT NOT NULL DEFAULT ''`)
}

func insertDefaultCategories(db *sql.DB) {
	_, err := db.Exec(`
		INSERT OR IGNORE INTO categories (name, description) VALUES
		('Education',     'Educational and academic topics'),
		('Sports',        'Sports news and discussions'),
		('Religious',     'Religious and Islamic topics'),
		('Technology',    'Everything related to technology and programming'),
		('Entertainment', 'Movies, music, games, and entertainment'),
		('Health',        'Health, fitness, and wellness topics'),
		('Other',         'General and miscellaneous topics');
	`)
	if err != nil {
		log.Fatal(err)
	}
}

//session
type Session struct {
	ID     string
	UserID int
}

func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func CreateSession(userID int) (string, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().Add(24 * time.Hour)
	_, err = DB.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		sessionID, userID, expiresAt)
	if err != nil {
		return "", err
	}
	return sessionID, nil
}

func GetSession(sessionID string) (Session, error) {
	var s Session
	err := DB.QueryRow(
		`SELECT id, user_id FROM sessions WHERE id = ? AND expires_at > ?`,
		sessionID, time.Now(),
	).Scan(&s.ID, &s.UserID)
	if err != nil {
		return Session{}, err
	}
	return s, nil
}
//logout
func DeleteSession(sessionID string) {
	DB.Exec(`DELETE FROM sessions WHERE id = ?`, sessionID)
}
//اذا سجل دخول من جديد
func DeleteSessionsByUserID(userID int) {
	DB.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
}

func CleanExpiredSessions() {
	DB.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now())
}

//login page
func AuthenticateUser(usernameOrEmail, password string) (int, error) {
	var userID int
	var hashedPassword string
	err := DB.QueryRow(
		`SELECT id, password FROM users WHERE username = ? OR email = ?`,
		usernameOrEmail, usernameOrEmail,
	).Scan(&userID, &hashedPassword)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, errors.New("invalid username or password")
		}
		return 0, err
	}
	if err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)); err != nil {
		return 0, errors.New("invalid username or password")
	}
	return userID, nil
}

func CheckUserExists(username, email string) (usernameExists, emailExists bool, err error) {
	var count int
	if err = DB.QueryRow(`SELECT COUNT(*) FROM users WHERE username = ?`, username).Scan(&count); err != nil {
		return
	}
	usernameExists = count > 0

	if err = DB.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ?`, email).Scan(&count); err != nil {
		return
	}
	emailExists = count > 0
	return
}

func CheckUserExistsExcluding(username, email string, excludeUserID int) (usernameExists, emailExists bool, err error) {
	var count int
	if err = DB.QueryRow(`SELECT COUNT(*) FROM users WHERE username = ? AND id != ?`, username, excludeUserID).Scan(&count); err != nil {
		return
	}
	usernameExists = count > 0

	if err = DB.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ? AND id != ?`, email, excludeUserID).Scan(&count); err != nil {
		return
	}
	emailExists = count > 0
	return
}

func CreateUser(username, email, password string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = DB.Exec(`INSERT INTO users (username, email, password) VALUES (?, ?, ?)`,
		username, email, string(hashed))
	return err
}

func GetUserByID(userID int) (string, error) {
	var username string
	err := DB.QueryRow(`SELECT username FROM users WHERE id = ?`, userID).Scan(&username)
	return username, err
}

func GetUserEmailByID(userID int) (string, error) {
	var email string
	err := DB.QueryRow(`SELECT email FROM users WHERE id = ?`, userID).Scan(&email)
	return email, err
}

func VerifyUserPassword(userID int, password string) error {
	var hashed string
	if err := DB.QueryRow(`SELECT password FROM users WHERE id = ?`, userID).Scan(&hashed); err != nil {
		return err
	}
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password))
}

func UpdateUsername(userID int, newUsername string) error {
	_, err := DB.Exec(`UPDATE users SET username = ? WHERE id = ?`, newUsername, userID)
	return err
}

func UpdateEmail(userID int, newEmail string) error {
	_, err := DB.Exec(`UPDATE users SET email = ? WHERE id = ?`, newEmail, userID)
	return err
}

func UpdatePassword(userID int, newPassword string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = DB.Exec(`UPDATE users SET password = ? WHERE id = ?`, string(hashed), userID)
	return err
}


type Post struct {
	ID           int
	UserID       int
	Username     string
	CategoryID   int
	CategoryName string
	Categories   []string
	Title        string
	Content      string
	ImagePath    string
	CreatedAt    string
	LikeCount    int
	DislikeCount int
	CommentCount int
	UserLiked    bool
	UserDisliked bool
}

const basePostQuery = `
	SELECT
		p.id,
		p.user_id,
		u.username,
		p.category_id,
		c.name,
		p.title,
		p.content,
		p.image_path,
		p.created_at,
		COALESCE((SELECT COUNT(*) FROM post_reactions WHERE post_id = p.id AND value =  1), 0),
		COALESCE((SELECT COUNT(*) FROM post_reactions WHERE post_id = p.id AND value = -1), 0),
		COALESCE((SELECT COUNT(*) FROM comments       WHERE post_id = p.id), 0)
	FROM posts p
	LEFT JOIN users      u ON p.user_id     = u.id
	LEFT JOIN categories c ON p.category_id = c.id
`

func scanPost(row interface{ Scan(...interface{}) error }) (Post, error) {
	var post Post
	var username, categoryName sql.NullString
	err := row.Scan(
		&post.ID, &post.UserID, &username,
		&post.CategoryID, &categoryName,
		&post.Title, &post.Content, &post.ImagePath, &post.CreatedAt,
		&post.LikeCount, &post.DislikeCount, &post.CommentCount,
	)
	post.Username = username.String
	post.CategoryName = categoryName.String
	return post, err
}

func scanPosts(rows *sql.Rows) ([]Post, error) {
	defer rows.Close()
	var posts []Post
	for rows.Next() {
		post, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		post.Categories, err = GetCategoriesForPost(post.ID)
		if err != nil {
			post.Categories = []string{post.CategoryName}
		}
		post.CreatedAt = FormatRelativeTime(post.CreatedAt)
		posts = append(posts, post)
	}
	return posts, nil
}

func GetPostByID(postID int) (Post, error) {
	post, err := scanPost(DB.QueryRow(basePostQuery+` WHERE p.id = ?`, postID))
	if err != nil {
		return Post{}, err
	}
	post.Categories, err = GetCategoriesForPost(post.ID)
	if err != nil {
		post.Categories = []string{post.CategoryName}
	}
	post.CreatedAt = FormatRelativeTime(post.CreatedAt)
	return post, nil
}

func GetAllPosts() ([]Post, error) {
	rows, err := DB.Query(basePostQuery + ` ORDER BY p.created_at DESC`)
	if err != nil {
		return nil, err
	}
	return scanPosts(rows)
}

func GetPostsByCategory(categoryID int) ([]Post, error) {
	rows, err := DB.Query(basePostQuery+`
		INNER JOIN post_categories pc ON p.id = pc.post_id
		WHERE pc.category_id = ?
		GROUP BY p.id
		ORDER BY p.created_at DESC
	`, categoryID)
	if err != nil {
		return nil, err
	}
	return scanPosts(rows)
}

func GetPostsByUserID(userID int) ([]Post, error) {
	rows, err := DB.Query(basePostQuery+` WHERE p.user_id = ? ORDER BY p.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return scanPosts(rows)
}

func GetLikedPostsByUserID(userID int) ([]Post, error) {
	rows, err := DB.Query(basePostQuery+`
		INNER JOIN post_reactions pr ON p.id = pr.post_id
		WHERE pr.user_id = ? AND pr.value = 1
		ORDER BY pr.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	return scanPosts(rows)
}

func GetCategoriesForPost(postID int) ([]string, error) {
	rows, err := DB.Query(`
		SELECT c.name
		FROM post_categories pc
		INNER JOIN categories c ON pc.category_id = c.id
		WHERE pc.post_id = ?
		ORDER BY c.name
	`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		categories = append(categories, name)
	}
	return categories, nil
}

func CreatePost(userID int, categoryIDs []int, title, content, imagePath string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO posts (user_id, category_id, title, content, image_path) VALUES (?, ?, ?, ?, ?)`,
		userID, categoryIDs[0], title, content, imagePath,
	)
	if err != nil {
		return err
	}

	postID, err := result.LastInsertId()
	if err != nil {
		return err
	}

	for _, categoryID := range categoryIDs {
		if _, err = tx.Exec(
			`INSERT INTO post_categories (post_id, category_id) VALUES (?, ?)`,
			postID, categoryID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func DeletePost(postID, userID int) error {
	var postUserID int
	err := DB.QueryRow(`SELECT user_id FROM posts WHERE id = ?`, postID).Scan(&postUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("post not found")
		}
		return err
	}
	if postUserID != userID {
		return fmt.Errorf("unauthorized: you don't own this post")
	}

	_, err = DB.Exec(`DELETE FROM posts WHERE id = ?`, postID)
	return err
}


type Category struct {
	ID          int
	Name        string
	Description string
}

func GetAllCategories() ([]Category, error) {
	rows, err := DB.Query(`SELECT id, name, description FROM categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []Category
	for rows.Next() {
		var c Category
		if err = rows.Scan(&c.ID, &c.Name, &c.Description); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, nil
}


type Comment struct {
	ID           int
	PostID       int
	UserID       int
	Username     string
	Content      string
	CreatedAt    string
	Likes        int
	Dislikes     int
	UserLiked    bool
	UserDisliked bool
}

type PostWithComments struct {
	Post     Post
	Comments []Comment
}

func CreateComment(postID, userID int, content string) error {
	_, err := DB.Exec(`INSERT INTO comments (post_id, user_id, content) VALUES (?, ?, ?)`,
		postID, userID, content)
	return err
}

func GetCommentsByPostID(postID int) ([]Comment, error) {
	rows, err := DB.Query(`
		SELECT c.id, c.post_id, c.user_id, u.username, c.content, c.created_at,
		       COALESCE(SUM(CASE WHEN cr.value = 1  THEN 1 ELSE 0 END), 0) AS likes,
		       COALESCE(SUM(CASE WHEN cr.value = -1 THEN 1 ELSE 0 END), 0) AS dislikes
		FROM comments c
		INNER JOIN users u ON c.user_id = u.id
		LEFT JOIN comment_reactions cr ON cr.comment_id = c.id
		WHERE c.post_id = ?
		GROUP BY c.id
		ORDER BY c.created_at ASC
	`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err = rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Username, &c.Content, &c.CreatedAt, &c.Likes, &c.Dislikes); err != nil {
			return nil, err
		}
		c.CreatedAt = FormatRelativeTime(c.CreatedAt)
		comments = append(comments, c)
	}
	return comments, nil
}

func DeleteComment(commentID, userID int) error {
	var commentUserID int
	err := DB.QueryRow(`SELECT user_id FROM comments WHERE id = ?`, commentID).Scan(&commentUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("comment not found")
		}
		return err
	}
	if commentUserID != userID {
		return fmt.Errorf("unauthorized: you don't own this comment")
	}

	_, err = DB.Exec(`DELETE FROM comments WHERE id = ?`, commentID)
	return err
}


func TogglePostReaction(userID, postID, value int) error {
	var existing int
	err := DB.QueryRow(
		`SELECT value FROM post_reactions WHERE user_id = ? AND post_id = ?`,
		userID, postID,
	).Scan(&existing)

	switch {
	case err == sql.ErrNoRows:
		_, err = DB.Exec(`INSERT INTO post_reactions (user_id, post_id, value) VALUES (?, ?, ?)`,
			userID, postID, value)
	case err != nil:
		return err
	case existing == value:
		_, err = DB.Exec(`DELETE FROM post_reactions WHERE user_id = ? AND post_id = ?`, userID, postID)
	default:
		_, err = DB.Exec(`UPDATE post_reactions SET value = ? WHERE user_id = ? AND post_id = ?`,
			value, userID, postID)
	}
	return err
}

func GetUserReaction(userID, postID int) (liked, disliked bool) {
	var value int
	if err := DB.QueryRow(
		`SELECT value FROM post_reactions WHERE user_id = ? AND post_id = ?`,
		userID, postID,
	).Scan(&value); err != nil {
		return false, false
	}
	return value == 1, value == -1
}

func ToggleCommentReaction(userID, commentID, value int) error {
	var existing int
	err := DB.QueryRow(
		`SELECT value FROM comment_reactions WHERE user_id = ? AND comment_id = ?`,
		userID, commentID,
	).Scan(&existing)

	switch {
	case err == sql.ErrNoRows:
		_, err = DB.Exec(`INSERT INTO comment_reactions (user_id, comment_id, value) VALUES (?, ?, ?)`,
			userID, commentID, value)
	case err != nil:
		return err
	case existing == value:
		_, err = DB.Exec(`DELETE FROM comment_reactions WHERE user_id = ? AND comment_id = ?`, userID, commentID)
	default:
		_, err = DB.Exec(`UPDATE comment_reactions SET value = ? WHERE user_id = ? AND comment_id = ?`,
			value, userID, commentID)
	}
	return err
}

func GetCommentUserReaction(userID, commentID int) (liked, disliked bool) {
	var value int
	if err := DB.QueryRow(
		`SELECT value FROM comment_reactions WHERE user_id = ? AND comment_id = ?`,
		userID, commentID,
	).Scan(&value); err != nil {
		return false, false
	}
	return value == 1, value == -1
}


func FormatRelativeTime(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05", timestamp)
		if err != nil {
			return timestamp
		}
	}

	diff := time.Since(t)
	seconds := int(diff.Seconds())

	switch {
	case seconds < 60:
		return "just now"
	case seconds < 3600:
		return fmt.Sprintf("%dm", seconds/60)
	case seconds < 86400:
		return fmt.Sprintf("%dh", seconds/3600)
	case seconds < 604800:
		return fmt.Sprintf("%dd", seconds/86400)
	case seconds < 2592000:
		return fmt.Sprintf("%dw", seconds/604800)
	}

	if t.Year() == time.Now().Year() {
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2, 2006")
}
// ── OAuth database functions ──────────────────────────────────────────────────

// GetUserByProviderID returns userID for an existing OAuth account.
func GetUserByProviderID(provider, providerID string) (int, error) {
	var userID int
	err := DB.QueryRow(
		`SELECT user_id FROM oauth_accounts WHERE provider = ? AND provider_id = ?`,
		provider, providerID,
	).Scan(&userID)
	return userID, err
}

// GetUserIDByEmail returns the userID for an existing user with the given email.
func GetUserIDByEmail(email string) (int, error) {
	var userID int
	err := DB.QueryRow(
		`SELECT id FROM users WHERE email = ?`, email,
	).Scan(&userID)
	return userID, err
}

// LinkOAuthProvider links an OAuth provider to an existing user account.
func LinkOAuthProvider(userID int, provider, providerID string) error {
	_, err := DB.Exec(
		`INSERT OR IGNORE INTO oauth_accounts (user_id, provider, provider_id) VALUES (?, ?, ?)`,
		userID, provider, providerID,
	)
	return err
}

// CreateOAuthUser creates a new user (no password) and links the OAuth account.
func CreateOAuthUser(username, email, provider, providerID string) (int, error) {
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO users (username, email, password) VALUES (?, ?, '')`,
		username, email,
	)
	if err != nil {
		return 0, err
	}

	userID64, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	userID := int(userID64)

	_, err = tx.Exec(
		`INSERT INTO oauth_accounts (user_id, provider, provider_id) VALUES (?, ?, ?)`,
		userID, provider, providerID,
	)
	if err != nil {
		return 0, err
	}

	return userID, tx.Commit()
}

// EnsureUniqueUsername appends a number suffix until the username is unique.
func EnsureUniqueUsername(base string) string {
	candidate := base
	for i := 2; i <= 9999; i++ {
		var count int
		if err := DB.QueryRow(
			`SELECT COUNT(*) FROM users WHERE username = ?`, candidate,
		).Scan(&count); err != nil || count == 0 {
			return candidate
		}
		candidate = fmt.Sprintf("%s%d", base, i)
		if len(candidate) > 30 {
			candidate = fmt.Sprintf("%s%d", base[:25], i)
		}
	}
	return candidate
}