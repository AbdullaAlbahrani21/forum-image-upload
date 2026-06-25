package main

import (
	"fmt"
	"forum/database"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type ErrorData struct {
	Code    int
	Title   string
	Message string
}

var errorMessages = map[int]ErrorData{
	400: {Code: 400, Title: "Bad Request!", Message: "Sorry, we couldn't understand your request. There seems to be an error in the data sent."},
	403: {Code: 403, Title: "Access Forbidden!", Message: "Sorry, you don't have permission to access this page. Please contact the administrator if you believe this is an error."},
	404: {Code: 404, Title: "Page Not Found!", Message: "Sorry, the page you're looking for doesn't exist. It may have been deleted or moved to another location."},
	405: {Code: 405, Title: "Method Not Allowed!", Message: "Sorry, the request method used is not allowed for this resource."},
	500: {Code: 500, Title: "Internal Server Error!", Message: "Sorry, something went wrong on our server. We're working to fix the issue. Please try again later."},
}

func RenderError(w http.ResponseWriter, r *http.Request, statusCode int) {
	errData, exists := errorMessages[statusCode]
	if !exists {
		errData = ErrorData{Code: statusCode, Title: "Error!", Message: "An unexpected error occurred."}
	}
	w.WriteHeader(statusCode)
	if err := templates.ExecuteTemplate(w, "error.html", errData); err != nil {
		fmt.Fprintf(w, "<html><body><h1>%d - %s</h1><p>%s</p></body></html>",
			errData.Code, errData.Title, errData.Message)
	}
}

func sanitizeRedirect(redirect string) string {
	for _, prefix := range []string{"home", "profile", "post"} {
		if redirect == prefix || strings.HasPrefix(redirect, prefix+"?") {
			return "/" + redirect
		}
	}
	return "/home"
}

func getSession(r *http.Request) (database.Session, bool) {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		return database.Session{}, false
	}
	session, err := database.GetSession(cookie.Value)
	if err != nil {
		return database.Session{}, false
	}
	return session, true
}

func removeDuplicates(ids []int) []int {
	seen := make(map[int]bool)
	result := make([]int, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		RenderError(w, r, 404)
		return
	}
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		RenderError(w, r, 405)
		return
	}

	session, isLoggedIn := getSession(r)
	username := "Guest"
	userID := 0

	if isLoggedIn {
		var err error
		username, err = database.GetUserByID(session.UserID)
		if err != nil {
			RenderError(w, r, 500)
			return
		}
		userID = session.UserID
	}

	var filterCategory int
	if s := r.URL.Query().Get("filter_category"); s != "" {
		filterCategory, _ = strconv.Atoi(s)
	}

	var (
		posts []database.Post
		err   error
	)
	if filterCategory > 0 {
		posts, err = database.GetPostsByCategory(filterCategory)
	} else {
		posts, err = database.GetAllPosts()
	}
	if err != nil {
		log.Println("Error fetching posts:", err)
		RenderError(w, r, 500)
		return
	}

	categories, err := database.GetAllCategories()
	if err != nil {
		log.Println("Error fetching categories:", err)
		categories = []database.Category{}
	}

	data := map[string]interface{}{
		"Username":       username,
		"UserID":         userID,
		"IsLoggedIn":     isLoggedIn,
		"Posts":          posts,
		"Categories":     categories,
		"FilterCategory": filterCategory,
	}

	if err = templates.ExecuteTemplate(w, "home.html", data); err != nil {
		log.Println("Error executing home template:", err)
		RenderError(w, r, 500)
	}
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		RenderError(w, r, 405)
		return
	}

	postID, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || postID <= 0 {
		RenderError(w, r, 400)
		return
	}

	post, err := database.GetPostByID(postID)
	if err != nil {
		RenderError(w, r, 404)
		return
	}

	session, isLoggedIn := getSession(r)
	var username string
	if isLoggedIn {
		username, err = database.GetUserByID(session.UserID)
		if err != nil {
			RenderError(w, r, 500)
			return
		}
		post.UserLiked, post.UserDisliked = database.GetUserReaction(session.UserID, post.ID)
	}

	comments, err := database.GetCommentsByPostID(post.ID)
	if err != nil {
		comments = []database.Comment{}
	}
	if isLoggedIn {
		for i := range comments {
			comments[i].UserLiked, comments[i].UserDisliked = database.GetCommentUserReaction(session.UserID, comments[i].ID)
		}
	}

	data := map[string]interface{}{
		"Post":       post,
		"Comments":   comments,
		"IsLoggedIn": isLoggedIn,
		"Username":   username,
	}

	if err = templates.ExecuteTemplate(w, "post.html", data); err != nil {
		log.Println("Error executing post template:", err)
		RenderError(w, r, 500)
	}
}

func clearSession(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session_id"); err == nil {
		database.DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		if _, isLoggedIn := getSession(r); isLoggedIn {
			http.Redirect(w, r, "/home", http.StatusSeeOther)
			return
		}
		if err := templates.ExecuteTemplate(w, "login.html", nil); err != nil {
			log.Println("Error executing login template:", err)
			RenderError(w, r, 500)
		}

	case "POST":
		clearSession(w, r)

		usernameLower := strings.ToLower(r.FormValue("username"))
		password := r.FormValue("password")

		userID, err := database.AuthenticateUser(usernameLower, password)
		if err != nil {
			templates.ExecuteTemplate(w, "login.html", map[string]string{"Error": "Invalid username or password"})
			return
		}

		database.DeleteSessionsByUserID(userID)

		sessionID, err := database.CreateSession(userID)
		if err != nil {
			log.Println("Error creating session:", err)
			RenderError(w, r, 500)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    sessionID,
			Path:     "/",
			MaxAge:   3600 * 24,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/home", http.StatusSeeOther)

	default:
		RenderError(w, r, 405)
	}
}

func guestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		RenderError(w, r, 405)
		return
	}
	clearSession(w, r)
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	renderErr := func(msg string) {
		templates.ExecuteTemplate(w, "register.html", map[string]string{"Error": msg})
	}

	switch r.Method {
	case "GET":
		if _, isLoggedIn := getSession(r); isLoggedIn {
			http.Redirect(w, r, "/home", http.StatusSeeOther)
			return
		}
		if err := templates.ExecuteTemplate(w, "register.html", nil); err != nil {
			log.Println("Error executing register template:", err)
			RenderError(w, r, 500)
		}

	case "POST":
		username := r.FormValue("username")
		email := r.FormValue("email")
		password := r.FormValue("password")

		if len(username) > 30 || len(email) > 254 || len(password) > 64 {
			RenderError(w, r, 400)
			return
		}

		if err := validateEmail(email); err != nil {
			renderErr("Invalid email format")
			return
		}
		if err := validateUsername(username); err != nil {
			renderErr("Invalid username format")
			return
		}
		if err := validatePassword(password, username, email); err != nil {
			renderErr(err.Error())
			return
		}

		usernameLower := strings.ToLower(username)
		usernameExists, emailExists, err := database.CheckUserExists(usernameLower, email)
		if err != nil {
			renderErr("Something went wrong, please try again")
			return
		}

		switch {
		case usernameExists && emailExists:
			renderErr("Username and Email already exist")
			return
		case usernameExists:
			renderErr("Username already exists, please choose another one")
			return
		case emailExists:
			renderErr("Email already exists, please use another one")
			return
		}

		if err = database.CreateUser(usernameLower, email, password); err != nil {
			log.Println("Error creating user:", err)
			renderErr("Failed to create account, please try again")
			return
		}

		http.Redirect(w, r, "/login", http.StatusSeeOther)

	default:
		RenderError(w, r, 405)
	}
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		RenderError(w, r, 405)
		return
	}

	if cookie, err := r.Cookie("session_id"); err == nil {
		database.DeleteSession(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func profileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		RenderError(w, r, 405)
		return
	}

	session, exists := getSession(r)
	if !exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	username, err := database.GetUserByID(session.UserID)
	if err != nil {
		RenderError(w, r, 500)
		return
	}

	email, err := database.GetUserEmailByID(session.UserID)
	if err != nil {
		RenderError(w, r, 500)
		return
	}

	activeTab := r.URL.Query().Get("tab")
	if activeTab == "" {
		activeTab = "posts"
	}

	var posts []database.Post
	if activeTab == "likes" {
		posts, err = database.GetLikedPostsByUserID(session.UserID)
	} else {
		posts, err = database.GetPostsByUserID(session.UserID)
	}
	if err != nil {
		log.Println("Error fetching posts:", err)
		RenderError(w, r, 500)
		return
	}

	postsWithComments := make([]database.PostWithComments, 0, len(posts))
	for _, post := range posts {
		post.UserLiked, post.UserDisliked = database.GetUserReaction(session.UserID, post.ID)
		comments, err := database.GetCommentsByPostID(post.ID)
		if err != nil {
			log.Println("Error fetching comments for post", post.ID, ":", err)
			comments = []database.Comment{}
		}
		if exists {
			for i := range comments {
				comments[i].UserLiked, comments[i].UserDisliked = database.GetCommentUserReaction(session.UserID, comments[i].ID)
			}
		}
		postsWithComments = append(postsWithComments, database.PostWithComments{
			Post:     post,
			Comments: comments,
		})
	}

	showCommentsForPost, _ := strconv.Atoi(r.URL.Query().Get("show_comments"))

	data := map[string]interface{}{
		"Username":            username,
		"Email":               email,
		"PostsWithComments":   postsWithComments,
		"ActiveTab":           activeTab,
		"ShowCommentsForPost": showCommentsForPost,
		"EditError":           r.URL.Query().Get("edit_error"),
		"EditSuccess":         r.URL.Query().Get("edit_success"),
	}

	if err = templates.ExecuteTemplate(w, "profile.html", data); err != nil {
		log.Println("Error executing profile template:", err)
		RenderError(w, r, 500)
	}
}

func createPostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		RenderError(w, r, 405)
		return
	}

	session, exists := getSession(r)
	if !exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Do NOT reject early based on request size
	// Let the multipart upload finish
	// then validate the actual file size using header.Size.
	const maxMultipartSize = 32 << 20 // 32 MB request allowance for testing

	if err := r.ParseMultipartForm(maxMultipartSize); err != nil {
		RenderError(w, r, 400)
		return
	}

	title := r.FormValue("title")
	content := r.FormValue("content")
	categoryIDStrs := r.Form["category_id"]

	if strings.TrimSpace(title) == "" || len(categoryIDStrs) == 0 {
		RenderError(w, r, 400)
		return
	}

	if len([]rune(title)) > 50 || len([]rune(content)) > 500 {
		RenderError(w, r, 400)
		return
	}

	var categoryIDs []int
	for _, idStr := range categoryIDStrs {
		if id, err := strconv.Atoi(idStr); err == nil {
			categoryIDs = append(categoryIDs, id)
		}
	}
	if len(categoryIDs) == 0 {
		http.Redirect(w, r, "/home", http.StatusSeeOther)
		return
	}
	categoryIDs = removeDuplicates(categoryIDs)

	// ── Image upload (optional) ──────────────────────────────────────────────
	// r.FormFile returns multipart.ErrMissingFile (or another error) when the
	// user did not select a file, which is perfectly valid — the image is optional.
	imagePath := ""
	file, header, fileErr := r.FormFile("image")
	if fileErr == nil {
		defer file.Close()

		// Skip empty file inputs (browser sends a zero-size part when no file is chosen).
		if header.Size > 0 || header.Filename != "" {
			mimeType, valErr := ValidateImageFile(file, header)
			if valErr != nil {
				renderHomeError(w, r, session.UserID, valErr.Error())
				return
			}

			savedPath, saveErr := SaveImageFile(file, mimeType)
			if saveErr != nil {
				log.Println("Error saving image:", saveErr)
				RenderError(w, r, 500)
				return
			}
			imagePath = savedPath
		}
	}
	// fileErr != nil means no file field was present — that is fine.

	// Must provide at least title + (content OR image)
	if strings.TrimSpace(content) == "" && imagePath == "" {
		renderHomeError(w, r, session.UserID, "Please add text content or attach an image — a title alone is not enough.")
		return
	}

	if err := database.CreatePost(session.UserID, categoryIDs, title, content, imagePath); err != nil {
		log.Println("Error creating post:", err)
		RenderError(w, r, 500)
		return
	}

	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

// renderHomeError re-renders the home page with an upload error message.
func renderHomeError(w http.ResponseWriter, r *http.Request, userID int, errMsg string) {
	username, _ := database.GetUserByID(userID)
	categories, _ := database.GetAllCategories()
	posts, _ := database.GetAllPosts()

	data := map[string]interface{}{
		"Username":       username,
		"UserID":         userID,
		"IsLoggedIn":     true,
		"Posts":          posts,
		"Categories":     categories,
		"FilterCategory": 0,
		"UploadError":    errMsg,
	}
	w.WriteHeader(http.StatusBadRequest)
	templates.ExecuteTemplate(w, "home.html", data)
}

func addCommentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		RenderError(w, r, 405)
		return
	}

	session, exists := getSession(r)
	if !exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	postID, err := strconv.Atoi(r.FormValue("post_id"))
	if err != nil {
		http.Redirect(w, r, "/home", http.StatusSeeOther)
		return
	}

	content := r.FormValue("content")
	redirectURL := sanitizeRedirect(r.FormValue("redirect"))

	if strings.TrimSpace(content) == "" {
		RenderError(w, r, 400)
		return
	}

	if len([]rune(content)) > 300 {
		RenderError(w, r, 400)
		return
	}

	if err = database.CreateComment(postID, session.UserID, content); err != nil {
		log.Println("Error creating comment:", err)
		RenderError(w, r, 500)
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func likePostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		RenderError(w, r, 405)
		return
	}

	session, exists := getSession(r)
	if !exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	postID, err := strconv.Atoi(r.FormValue("post_id"))
	if err != nil {
		http.Redirect(w, r, "/home", http.StatusSeeOther)
		return
	}

	if err = database.TogglePostReaction(session.UserID, postID, 1); err != nil {
		log.Println("Error toggling like:", err)
		RenderError(w, r, 500)
		return
	}

	http.Redirect(w, r, sanitizeRedirect(r.FormValue("redirect")), http.StatusSeeOther)
}

func dislikePostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		RenderError(w, r, 405)
		return
	}

	session, exists := getSession(r)
	if !exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	postID, err := strconv.Atoi(r.FormValue("post_id"))
	if err != nil {
		http.Redirect(w, r, "/home", http.StatusSeeOther)
		return
	}

	if err = database.TogglePostReaction(session.UserID, postID, -1); err != nil {
		log.Println("Error toggling dislike:", err)
		RenderError(w, r, 500)
		return
	}

	http.Redirect(w, r, sanitizeRedirect(r.FormValue("redirect")), http.StatusSeeOther)
}

func deletePostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		RenderError(w, r, 405)
		return
	}

	session, exists := getSession(r)
	if !exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	postID, err := strconv.Atoi(r.FormValue("post_id"))
	if err != nil {
		http.Redirect(w, r, "/home", http.StatusSeeOther)
		return
	}

	if err = database.DeletePost(postID, session.UserID); err != nil {
		log.Println("Error deleting post:", err)
		RenderError(w, r, 500)
		return
	}

	http.Redirect(w, r, sanitizeRedirect(r.FormValue("redirect")), http.StatusSeeOther)
}

func likeCommentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		RenderError(w, r, 405)
		return
	}

	session, exists := getSession(r)
	if !exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	commentID, err := strconv.Atoi(r.FormValue("comment_id"))
	if err != nil {
		http.Redirect(w, r, "/home", http.StatusSeeOther)
		return
	}

	if err = database.ToggleCommentReaction(session.UserID, commentID, 1); err != nil {
		log.Println("Error toggling comment like:", err)
		RenderError(w, r, 500)
		return
	}

	http.Redirect(w, r, sanitizeRedirect(r.FormValue("redirect")), http.StatusSeeOther)
}

func dislikeCommentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		RenderError(w, r, 405)
		return
	}

	session, exists := getSession(r)
	if !exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	commentID, err := strconv.Atoi(r.FormValue("comment_id"))
	if err != nil {
		http.Redirect(w, r, "/home", http.StatusSeeOther)
		return
	}

	if err = database.ToggleCommentReaction(session.UserID, commentID, -1); err != nil {
		log.Println("Error toggling comment dislike:", err)
		RenderError(w, r, 500)
		return
	}

	http.Redirect(w, r, sanitizeRedirect(r.FormValue("redirect")), http.StatusSeeOther)
}

func deleteCommentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		RenderError(w, r, 405)
		return
	}

	session, exists := getSession(r)
	if !exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	commentID, err := strconv.Atoi(r.FormValue("comment_id"))
	if err != nil {
		http.Redirect(w, r, "/home", http.StatusSeeOther)
		return
	}

	if err = database.DeleteComment(commentID, session.UserID); err != nil {
		log.Println("Error deleting comment:", err)
		RenderError(w, r, 500)
		return
	}

	http.Redirect(w, r, sanitizeRedirect(r.FormValue("redirect")), http.StatusSeeOther)
}

func editUsernameHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		RenderError(w, r, 405)
		return
	}

	session, exists := getSession(r)
	if !exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	newUsername := r.FormValue("new_username")
	currentPassword := r.FormValue("current_password")

	if len(newUsername) > 30 || len(currentPassword) > 64 {
		RenderError(w, r, 400)
		return
	}

	if err := database.VerifyUserPassword(session.UserID, currentPassword); err != nil {
		http.Redirect(w, r, "/profile?edit_error=Incorrect+password", http.StatusSeeOther)
		return
	}

	if err := validateUsername(newUsername); err != nil {
		http.Redirect(w, r, "/profile?edit_error=Invalid+username+format", http.StatusSeeOther)
		return
	}

	newUsernameLower := strings.ToLower(newUsername)
	usernameExists, _, err := database.CheckUserExistsExcluding(newUsernameLower, "", session.UserID)
	if err != nil {
		RenderError(w, r, 500)
		return
	}
	if usernameExists {
		http.Redirect(w, r, "/profile?edit_error=Username+already+exists", http.StatusSeeOther)
		return
	}

	if err = database.UpdateUsername(session.UserID, newUsernameLower); err != nil {
		log.Println("Error updating username:", err)
		RenderError(w, r, 500)
		return
	}

	http.Redirect(w, r, "/profile?edit_success=Username+updated+successfully", http.StatusSeeOther)
}

func editEmailHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		RenderError(w, r, 405)
		return
	}

	session, exists := getSession(r)
	if !exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	newEmail := r.FormValue("new_email")
	currentPassword := r.FormValue("current_password")

	if len(newEmail) > 254 || len(currentPassword) > 64 {
		RenderError(w, r, 400)
		return
	}

	if err := database.VerifyUserPassword(session.UserID, currentPassword); err != nil {
		http.Redirect(w, r, "/profile?edit_error=Incorrect+password", http.StatusSeeOther)
		return
	}

	if err := validateEmail(newEmail); err != nil {
		http.Redirect(w, r, "/profile?edit_error=Invalid+email+format", http.StatusSeeOther)
		return
	}

	_, emailExists, err := database.CheckUserExistsExcluding("", newEmail, session.UserID)
	if err != nil {
		RenderError(w, r, 500)
		return
	}
	if emailExists {
		http.Redirect(w, r, "/profile?edit_error=Email+already+exists", http.StatusSeeOther)
		return
	}

	if err = database.UpdateEmail(session.UserID, newEmail); err != nil {
		log.Println("Error updating email:", err)
		RenderError(w, r, 500)
		return
	}

	http.Redirect(w, r, "/profile?edit_success=Email+updated+successfully", http.StatusSeeOther)
}

func editPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		RenderError(w, r, 405)
		return
	}

	session, exists := getSession(r)
	if !exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if len(currentPassword) > 64 || len(newPassword) > 64 || len(confirmPassword) > 64 {
		RenderError(w, r, 400)
		return
	}

	if err := database.VerifyUserPassword(session.UserID, currentPassword); err != nil {
		http.Redirect(w, r, "/profile?edit_error=Incorrect+current+password", http.StatusSeeOther)
		return
	}

	if newPassword != confirmPassword {
		http.Redirect(w, r, "/profile?edit_error=Passwords+do+not+match", http.StatusSeeOther)
		return
	}

	username, _ := database.GetUserByID(session.UserID)
	email, _ := database.GetUserEmailByID(session.UserID)

	if err := validatePassword(newPassword, username, email); err != nil {
		http.Redirect(w, r, "/profile?edit_error=Invalid+password+format", http.StatusSeeOther)
		return
	}

	if err := database.UpdatePassword(session.UserID, newPassword); err != nil {
		log.Println("Error updating password:", err)
		RenderError(w, r, 500)
		return
	}

	http.Redirect(w, r, "/profile?edit_success=Password+updated+successfully", http.StatusSeeOther)
}

func validateEmail(email string) error {
	if len(email) < 6 || len(email) > 254 {
		return fmt.Errorf("invalid email")
	}

	atCount, atIndex := 0, -1
	for i, ch := range email {
		if ch == '@' {
			atCount++
			atIndex = i
		}
	}
	if atCount != 1 {
		return fmt.Errorf("invalid email")
	}

	localPart := email[:atIndex]
	domainPart := email[atIndex+1:]

	if len(localPart) == 0 || len(localPart) > 64 || len(domainPart) == 0 {
		return fmt.Errorf("invalid email")
	}

	dotIndex := -1
	for i := len(domainPart) - 1; i >= 0; i-- {
		if domainPart[i] == '.' {
			dotIndex = i
			break
		}
	}
	if dotIndex <= 0 || dotIndex == len(domainPart)-1 {
		return fmt.Errorf("invalid email")
	}
	if len(domainPart[dotIndex+1:]) < 2 {
		return fmt.Errorf("invalid email")
	}

	for _, ch := range email {
		if ch == ' ' {
			return fmt.Errorf("invalid email")
		}
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '.' || ch == '-' ||
			ch == '_' || ch == '@' || ch == '+' || ch == '%') {
			return fmt.Errorf("invalid email")
		}
	}
	return nil
}

func validateUsername(username string) error {
	if len(username) < 3 || len(username) > 30 {
		return fmt.Errorf("invalid username")
	}
	if strings.Contains(username, " ") {
		return fmt.Errorf("invalid username")
	}
	for _, ch := range username {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			return nil
		}
	}
	return fmt.Errorf("invalid username")
}

func validatePassword(password, username, email string) error {
	if len(password) < 8 {
		return fmt.Errorf("Password must be at least 8 characters long")
	}
	if len(password) > 64 {
		return fmt.Errorf("Password must not exceed 64 characters")
	}
	if password[0] == ' ' || password[len(password)-1] == ' ' {
		return fmt.Errorf("Password must not start or end with a space")
	}

	hasLetter, hasNumber := false, false
	for _, ch := range password {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			hasLetter = true
		}
		if ch >= '0' && ch <= '9' {
			hasNumber = true
		}
	}
	if !hasLetter {
		return fmt.Errorf("Password must contain at least one letter")
	}
	if !hasNumber {
		return fmt.Errorf("Password must contain at least one number")
	}

	passwordLower := strings.ToLower(password)

	if u := strings.ToLower(username); u != "" && strings.Contains(passwordLower, u) {
		return fmt.Errorf("Password must not contain your username")
	}
	if e := strings.ToLower(email); e != "" && strings.Contains(passwordLower, e) {
		return fmt.Errorf("Password must not contain your email")
	}
	if i := strings.Index(email, "@"); i > 0 {
		if local := strings.ToLower(email[:i]); local != "" && strings.Contains(passwordLower, local) {
			return fmt.Errorf("Password must not contain your email")
		}
	}
	return nil
}
