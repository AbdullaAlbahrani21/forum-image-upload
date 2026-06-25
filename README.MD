# Forum

A full-stack web forum built entirely in Go — no frontend frameworks. Users can register, post, comment, and interact with content through likes and dislikes, with full authentication and Docker support.


## Description

Forum is a web application that enables users to communicate through posts and comments, organize content with categories, and filter what they see. It uses SQLite for persistent storage, session-based authentication with encrypted passwords, and runs inside a Docker container for easy deployment.

## Features

- **Authentication** — Register with email, username, and password. Passwords are hashed with bcrypt. Each user gets one session at a time via cookies with an expiration date.
- **Posts & Comments** — Registered users can create posts assigned to one or more categories. All users (including guests) can read posts and comments.
- **Likes & Dislikes** — Registered users can like or dislike any post or comment. Counts are visible to everyone.
- **Filtering** — Filter posts by category, by posts you created, or by posts you liked (last two require login).
- **Error Handling** — All HTTP errors are handled correctly (400, 404, 500).
- **Docker** — The entire application is containerized and runs with a single command.

## Tech Stack

| Layer | Technology |
|-------|------------|
| Backend | Go (standard library only) |
| Database | SQLite (`mattn/go-sqlite3`) |
| Auth | Sessions, Cookies, bcrypt, UUID |
| Frontend | HTML Templates (Go `html/template`) |
| Deployment | Docker |

## Usage

### Run with Docker

```bash
git clone https://learn.reboot01.com/git/aalbraik/forum
cd forum
docker build -t forum .
docker run -p 8080:8080 forum
```

Then open `http://localhost:8080` in your browser.

### Run without Docker

```bash
go run .
```

## Implementation Details

### Database

The app uses SQLite with the following core tables:

- `users` — stores credentials and session tokens
- `posts` — stores post content and category associations
- `comments` — linked to posts and users
- `likes` — tracks likes/dislikes per user per post/comment
- `sessions` — manages active login sessions with expiry

### Authentication Flow

1. User registers → password hashed with bcrypt → stored in DB
2. User logs in → credentials verified → UUID session token created → stored in cookie
3. Each request checks the session cookie against the DB
4. Session expires after a set duration

### SQL Queries Used

- `SELECT` — fetch posts, comments, users, session data
- `INSERT` — register users, create posts/comments, record likes
- `CREATE` — table creation on first run

## Allowed Packages

- All standard Go packages
- `github.com/mattn/go-sqlite3`
- `golang.org/x/crypto/bcrypt`
- `github.com/gofrs/uuid` or `github.com/google/uuid`