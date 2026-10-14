package trazrextension

// LEGACY HANDLERS AND MIDDLEWARE - COMMENTED OUT TO AVOID CONFUSION
/*
// getDB returns a new DB connection using the default config.
func getDB() (*sql.DB, error) {
	// TODO: Get the database config from the config file
	return sql.Open("pgx", "")
}

// AdminOnly is middleware that checks for a valid JWT and admin role.
func AdminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeJSONError(w, http.StatusUnauthorized, "missing or invalid token", zap.NewNop())
			return
		}
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := auth.ValidateJWT(tokenString)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid token", zap.NewNop())
			return
		}
		db, err := getDB()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "db error", zap.NewNop())
			return
		}
		defer db.Close()
		isAdmin, err := auth.HasRole(r.Context(), db, claims.UserID, "admin")
		if err != nil || !isAdmin {
			writeJSONError(w, http.StatusForbidden, "admin access required", zap.NewNop())
			return
		}
		next(w, r)
	}
}

// RegisterHandler handles user registration.
func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
		return
	}
	db, err := getDB()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
		return
	}
	defer db.Close()
	user, err := auth.Register(r.Context(), db, req.Email, req.Password, "", "", "", "", "", "", "", "", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error(), zap.NewNop())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": user.ID, "email": user.Email, "created_at": user.CreatedAt})
}

// LoginHandler handles user login.
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
		return
	}
	db, err := getDB()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
		return
	}
	defer db.Close()
	token, err := auth.Login(r.Context(), db, req.Email, req.Password)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, err.Error(), zap.NewNop())
		return
	}
	user, err := auth.GetUserByEmail(r.Context(), db, req.Email)
	if err != nil || user == nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to fetch user info", zap.NewNop())
		return
	}
	roles, err := auth.ListRoles(r.Context(), db, user.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to fetch user roles", zap.NewNop())
		return
	}
	if len(roles) == 0 {
		if user.ID == 1 {
			roles = []string{"admin", "user"}
		} else {
			roles = []string{"user"}
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": token,
		"user":  user,
		"roles": roles,
	})
}

// RequestPasswordResetHandler handles password reset token generation.
func RequestPasswordResetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
		return
	}
	db, err := getDB()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
		return
	}
	defer db.Close()
	token, err := auth.CreatePasswordResetToken(r.Context(), db, req.Email, 1*time.Hour)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error(), zap.NewNop())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"reset_token": token})
}

// ResetPasswordHandler handles password reset using a valid token.
func ResetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
		return
	}
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
		return
	}
	db, err := getDB()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
		return
	}
	defer db.Close()
	userID, err := auth.VerifyPasswordResetToken(r.Context(), db, req.Token)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error(), zap.NewNop())
		return
	}
	if err := auth.ResetPassword(r.Context(), db, userID, req.NewPassword); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
		return
	}
	if err := auth.MarkPasswordResetUsed(r.Context(), db, req.Token); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password reset successful"})
}

// ListUsersHandler handles listing all auth_users (admin only).
func ListUsersHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Add JWT authentication and admin check here
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
		return
	}
	db, err := getDB()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
		return
	}
	defer db.Close()
	users, err := auth.ListUsers(r.Context(), db)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
		return
	}
	var result []map[string]interface{}
	for _, u := range users {
		roles, _ := auth.ListRoles(r.Context(), db, u.ID)
		result = append(result, map[string]interface{}{
			"id":       u.ID,
			"email":    u.Email,
			"roles":    roles,
			"metadata": u.Metadata,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

// Helper to get actorID from JWT claims in context
func getActorID(r *http.Request) int64 {
	claims, ok := r.Context().Value("user").(*auth.Claims)
	if ok && claims != nil {
		return claims.UserID
	}
	return 0
}

// DeleteUserHandler handles deleting a user by ID (admin only).
func DeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Add JWT authentication and admin check here
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
		return
	}
	var req struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
		return
	}
	db, err := getDB()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
		return
	}
	defer db.Close()
	actorID := getActorID(r)
	if !auth.AllowAction(actorID, "delete_user") {
		writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded", zap.NewNop())
		return
	}
	if err := auth.DeleteUser(r.Context(), db, req.UserID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
		return
	}
	_ = auth.LogAuditEvent(r.Context(), db, actorID, "delete_user", &req.UserID, "", r.RemoteAddr)
	writeJSON(w, http.StatusOK, map[string]string{"status": "user deleted"})
}

// EditUserHandler handles updating a user's email and name (admin only).
func EditUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
		return
	}
	// TODO: Add JWT authentication and admin check here if not wrapped
	var req struct {
		UserID int64  `json:"user_id"`
		Email  string `json:"email"`
		Name   string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
		return
	}
	db, err := getDB()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
		return
	}
	defer db.Close()
	if err := auth.UpdateUserProfile(r.Context(), db, req.UserID, req.Email, req.Name); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "user updated"})
}

// ResetUserPasswordHandler handles setting a new password for a user (admin only).
func ResetUserPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
		return
	}
	// TODO: Add JWT authentication and admin check here if not wrapped
	var req struct {
		UserID      int64  `json:"user_id"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
		return
	}
	db, err := getDB()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
		return
	}
	defer db.Close()
	if err := auth.ResetPassword(r.Context(), db, req.UserID, req.NewPassword); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password reset"})
}

// DeleteUserByIDHandler handles DELETE /api/users/{id} (admin only, RESTful)
func DeleteUserByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
		return
	}
	// Parse user ID from URL path
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		writeJSONError(w, http.StatusBadRequest, "missing user id in path", zap.NewNop())
		return
	}
	idStr := parts[len(parts)-1]
	userID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || userID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid user id", zap.NewNop())
		return
	}
	db, err := getDB()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
		return
	}
	defer db.Close()
	actorID := getActorID(r)
	if !auth.AllowAction(actorID, "delete_user") {
		writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded", zap.NewNop())
		return
	}
	if err := auth.DeleteUser(r.Context(), db, userID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
		return
	}
	_ = auth.LogAuditEvent(r.Context(), db, actorID, "delete_user", &userID, "", r.RemoteAddr)
	writeJSON(w, http.StatusOK, map[string]string{"status": "user deleted"})
}
*/
