package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/John-1005/Chirpy/internal/auth"
	"github.com/John-1005/Chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileServerHits atomic.Int32
	db             *database.Queries
	platform       string
	secret         string
	polka_key      string
}

type User struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
	IsChirpyRed  bool      `json:"is_chirpy_red"`
}

type Chirps struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	User_ID   uuid.UUID `json:"user_id"`
}

func main() {
	godotenv.Load()

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Fatal("DB_URL must be set")
	}

	dbConn, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error opening database: %s", err)
	}

	secret := os.Getenv("SECRET")
	if secret == "" {
		log.Fatal("Secret must be set")
	}

	polka_key := os.Getenv("POLKA_KEY")
	if polka_key == "" {
		log.Fatal("polka key must be set")
	}

	dbQueries := database.New(dbConn)

	apiCfg := &apiConfig{
		fileServerHits: atomic.Int32{},
		db:             dbQueries,
		platform:       os.Getenv("PLATFORM"),
		secret:         os.Getenv("SECRET"),
		polka_key:      os.Getenv("POLKA_KEY"),
	}

	mux := http.NewServeMux()
	fileServer := http.StripPrefix("/app", http.FileServer(http.Dir(".")))

	mux.Handle("/app/", apiCfg.middlewareMetricsInc(fileServer))
	mux.HandleFunc("GET /api/healthz", handlerReadiness)
	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerCount)
	mux.HandleFunc("GET /api/chirps", apiCfg.handlerGetChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerGetChirpByID)

	mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
	mux.HandleFunc("POST /api/chirps", apiCfg.handlerSendChirp)
	mux.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)
	mux.HandleFunc("POST /api/login", apiCfg.handlerLogin)
	mux.HandleFunc("POST /api/refresh", apiCfg.handlerRefresh)
	mux.HandleFunc("POST /api/revoke", apiCfg.handlerRevoke)
	mux.HandleFunc("POST /api/polka/webhooks", apiCfg.handlerUserUpgraded)

	mux.HandleFunc("PUT /api/users", apiCfg.handlerUsers)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.handlerDelete)

	server := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}

	err = server.ListenAndServe()
	if err != nil {
		fmt.Println("Server Error:", err)
	}

}

func handlerReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	w.WriteHeader(http.StatusOK)

	w.Write([]byte("OK"))
}

func (cfg *apiConfig) handlerCount(w http.ResponseWriter, r *http.Request) {

	currentCount := cfg.fileServerHits.Load()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "Welcome, Chirpy Admin\n")
	fmt.Fprintf(w, "Chirpy has been visited %d times!", currentCount)

}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {

	if cfg.platform != "dev" {
		respondWithError(w, 403, "Forbidden", nil)
		return
	}

	err := cfg.db.DeleteUsers(r.Context())
	if err != nil {
		respondWithError(w, 500, "Failed to delete users", err)
		return
	}

	w.WriteHeader(http.StatusOK)

}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileServerHits.Add(1)
		next.ServeHTTP(w, r)
	})

}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {

	type user struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := user{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode message", err)
		return
	}

	hashedPassword, err := auth.HashPassword(params.Password)

	dbUserParams := database.CreateUserParams{
		HashedPassword: hashedPassword,
		Email:          params.Email,
	}

	dbUser, err := cfg.db.CreateUser(r.Context(), dbUserParams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create user", err)
		return
	}

	userResp := User{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}

	respondWithJSON(w, http.StatusCreated, userResp)
}

func (cfg *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {

	type user struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := user{}
	err := decoder.Decode(&params)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode message", err)
		return
	}

	dbUser, err := cfg.db.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		respondWithError(w, 401, "incorrect email or password", err)
		return
	}

	err = auth.CheckPasswordHash(params.Password, dbUser.HashedPassword)
	if err != nil {
		respondWithError(w, 401, "incorrect email or password", err)
		return
	}

	accessToken, err := auth.MakeJWT(dbUser.ID, cfg.secret, 1*time.Hour)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to make token", err)
		return
	}

	refreshToken, _ := auth.MakeRefreshToken()

	userResp := User{
		ID:           dbUser.ID,
		CreatedAt:    dbUser.CreatedAt,
		UpdatedAt:    dbUser.UpdatedAt,
		Email:        dbUser.Email,
		Token:        accessToken,
		RefreshToken: refreshToken,
		IsChirpyRed:  dbUser.IsChirpyRed,
	}

	refreshResp := database.RefreshToken{
		Token:     refreshToken,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		UserID:    dbUser.ID,
		ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
		RevokedAt: sql.NullTime{Time: time.Time{}, Valid: false},
	}

	refreshParams := database.InsertRefreshTokenParams{
		Token:     refreshResp.Token,
		CreatedAt: refreshResp.CreatedAt,
		UpdatedAt: refreshResp.UpdatedAt,
		UserID:    refreshResp.UserID,
		ExpiresAt: refreshResp.ExpiresAt,
		RevokedAt: refreshResp.RevokedAt,
	}

	err = cfg.db.InsertRefreshToken(r.Context(), refreshParams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to insert refresh_token", err)
		return
	}

	respondWithJSON(w, http.StatusOK, userResp)
}

func (cfg *apiConfig) handlerRefresh(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")

	if authHeader == "" {
		respondWithError(w, 401, "authorization not found", nil)
		return
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		respondWithError(w, 401, "authorization header must start with Bearer", nil)
		return
	}

	refreshToken := strings.TrimPrefix(authHeader, prefix)
	refreshToken = strings.TrimSpace(refreshToken)

	dbToken, err := cfg.db.GetRefreshToken(r.Context(), refreshToken)
	if err != nil {
		respondWithError(w, 401, "invalid or expired token", err)
		return
	}

	if dbToken.ExpiresAt.Before(time.Now()) {
		respondWithError(w, 401, "refresh token expired", nil)
		return
	}

	if dbToken.RevokedAt.Valid {
		respondWithError(w, 401, "refresh token revoked", nil)
		return
	}

	accessToken, err := auth.MakeJWT(dbToken.UserID, cfg.secret, 1*time.Hour)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to create token", err)
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{"token": accessToken})
}

func (cfg *apiConfig) handlerRevoke(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find token", err)
		return
	}

	_, err = cfg.db.RevokeRefreshToken(r.Context(), refreshToken)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't revoke session", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) handlerUsers(w http.ResponseWriter, r *http.Request) {

	type user struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find token", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "invaild token", err)
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := user{}
	err = decoder.Decode(&params)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to decode Json", err)
		return
	}

	hashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to hash password", err)
		return
	}

	dbUserParams := database.UpdateUsersParams{
		HashedPassword: hashedPassword,
		Email:          params.Email,
		ID:             userID,
	}

	dbUser, err := cfg.db.UpdateUsers(r.Context(), dbUserParams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create user", err)
		return
	}

	userResp := User{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}

	respondWithJSON(w, http.StatusOK, userResp)
}

func (cfg *apiConfig) handlerSendChirp(w http.ResponseWriter, r *http.Request) {

	type message struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := message{}
	err := decoder.Decode(&params)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode message", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Unable to locate token", err)
	}

	claims, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, 401, "invalid token", err)
	}

	dbChirpParams := database.AddChirpParams{
		Body:   params.Body,
		UserID: claims,
	}
	const maxLength = 140

	if len(params.Body) > maxLength {
		respondWithError(w, http.StatusBadRequest, "Chirp is too long", nil)
		return
	}

	dbChirp, err := cfg.db.AddChirp(r.Context(), dbChirpParams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to add chirp", err)
		return
	}

	chirpResp := Chirps{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		User_ID:   dbChirp.UserID,
	}

	respondWithJSON(w, http.StatusCreated, chirpResp)
}

func (cfg *apiConfig) handlerGetChirps(w http.ResponseWriter, r *http.Request) {

	s := r.URL.Query().Get("author_id")
	query := r.URL.Query().Get("sort")
	var apiChirps []Chirps

	if s != "" {
		authorID, err := uuid.Parse(s)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "no id found", err)
			return
		}

		authorChirps, err := cfg.db.GetChirpsByID(r.Context(), authorID)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "trouble accessing database", err)
			return
		}

		apiChirps = make([]Chirps, len(authorChirps))
		for i, authorChirp := range authorChirps {
			apiChirps[i] = databaseChirpToApi(authorChirp)
		}

	} else {
		dbChirps, err := cfg.db.GetChirps(r.Context())

		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "trouble accessing database", err)
			return
		}

		apiChirps = make([]Chirps, len(dbChirps))
		for i, dbChirp := range dbChirps {
			apiChirps[i] = databaseChirpToApi(dbChirp)
		}

	}

	if query == "desc" {
		sort.Slice(apiChirps, func(i, j int) bool {
			return apiChirps[i].CreatedAt.After(apiChirps[j].CreatedAt)
		})
	} else {
		sort.Slice(apiChirps, func(i, j int) bool {
			return apiChirps[i].CreatedAt.Before(apiChirps[j].CreatedAt)
		})
	}

	respondWithJSON(w, http.StatusOK, apiChirps)
}

func databaseChirpToApi(dbChirp database.Chirp) Chirps {
	return Chirps{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		User_ID:   dbChirp.UserID,
	}
}

func (cfg *apiConfig) handlerGetChirpByID(w http.ResponseWriter, r *http.Request) {

	userID := r.PathValue("chirpID")
	id, err := uuid.Parse(userID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "no id found", err)
	}
	dbChirp, err := cfg.db.GetChirpByID(r.Context(), id)
	if err != nil {
		respondWithError(w, 404, "trouble accessing database", err)
		return
	}

	chirpResp := Chirps{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		User_ID:   dbChirp.UserID,
	}

	respondWithJSON(w, http.StatusOK, chirpResp)
}

func (cfg *apiConfig) handlerDelete(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "couldn't find token", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, 401, "invalid token", err)
		return
	}

	id := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(id)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "no id found", err)
		return
	}

	dbChirp, err := cfg.db.GetChirpByID(r.Context(), chirpID)

	if err != nil {
		respondWithError(w, 404, "no chirp found", err)
		return
	}

	if dbChirp.UserID != userID {
		respondWithError(w, 403, "not authorized", nil)
		return
	}

	err = cfg.db.DeleteChirpByID(r.Context(), database.DeleteChirpByIDParams{
		ID:     chirpID,
		UserID: userID,
	})

	if err != nil {
		respondWithError(w, 404, "chirp not found", err)
		return
	}

	w.WriteHeader(204)

}

func (cfg *apiConfig) handlerUserUpgraded(w http.ResponseWriter, r *http.Request) {

	type user struct {
		Event string `json:"event"`
		Data  struct {
			UserID uuid.UUID `json:"user_id"`
		} `json:"data"`
	}

	apiKey, err := GetAPIKey(r.Header)

	if err != nil {
		w.WriteHeader(401)
		return
	}

	if apiKey != cfg.polka_key {
		w.WriteHeader(401)
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := user{}
	err = decoder.Decode(&params)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to decode json", err)
		return
	}

	if params.Event != "user.upgraded" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	err = cfg.db.ChirpyRedUpgrade(r.Context(), params.Data.UserID)
	if err != nil {
		respondWithError(w, 404, "no user found", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)

}

func GetAPIKey(headers http.Header) (string, error) {
	authHeader := headers.Get("Authorization")

	if authHeader == "" {
		return "", errors.New("authorization header not found")
	}

	const prefix = "ApiKey "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", errors.New("authorizaton header must start with 'ApiKey '")
	}

	apiKey := strings.TrimSpace(strings.TrimPrefix(authHeader, prefix))
	if apiKey == "" {
		return "", errors.New("missing apikey")
	}

	return apiKey, nil
}
