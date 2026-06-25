package main

import (
	"forum/database"
	"html/template"
	"log"
	"net/http"
)

var templates *template.Template

func init() {
	funcMap := template.FuncMap{
		"firstChar": func(s string) string {
			if len(s) == 0 {
				return "?"
			}
			runes := []rune(s)
			return string(runes[0])
		},
	}

	var err error
	templates, err = template.New("").Funcs(funcMap).ParseGlob("templates/*.html")
	if err != nil {
		log.Fatal("Error parsing templates:", err)
	}
}

func main() {
	database.InitDB()

	if err := EnsureUploadsDir(); err != nil {
		log.Fatal("Cannot create uploads directory:", err)
	}

	http.Handle("/static/",
		http.StripPrefix("/static/",
			http.FileServer(http.Dir("static"))))

	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/home", homeHandler)
	http.HandleFunc("/post", postHandler)
	http.HandleFunc("/profile", profileHandler)
	http.HandleFunc("/logout", logoutHandler)
	http.HandleFunc("/guest", guestHandler)
	http.HandleFunc("/auth/google", googleLoginHandler)
	http.HandleFunc("/auth/google/callback", googleCallbackHandler)
	http.HandleFunc("/auth/github", githubLoginHandler)
	http.HandleFunc("/auth/github/callback", githubCallbackHandler)
	http.HandleFunc("/create-post", createPostHandler)
	http.HandleFunc("/add-comment", addCommentHandler)
	http.HandleFunc("/like-post", likePostHandler)
	http.HandleFunc("/dislike-post", dislikePostHandler)
	http.HandleFunc("/delete-post", deletePostHandler)
	http.HandleFunc("/like-comment", likeCommentHandler)
	http.HandleFunc("/dislike-comment", dislikeCommentHandler)
	http.HandleFunc("/delete-comment", deleteCommentHandler)
	http.HandleFunc("/edit-username", editUsernameHandler)
	http.HandleFunc("/edit-email", editEmailHandler)
	http.HandleFunc("/edit-password", editPasswordHandler)

	log.Println("Server starting on http://localhost:8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("Error:", err)
	}
}