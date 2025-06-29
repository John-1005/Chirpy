package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/John-1005/Chirpy/internal/auth"
	"github.com/John-1005/Chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)
UPDATE table_name 
SET column1 = value1, column2 = value2, column3 = value3
WHERE condition
RETURNING *;
type apiConfig struct {
	fileServerHits atomic.Int32
	db             *database.Queries
	platform       string
	secret         string
}

type User struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
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

	dbQueries := database.New(dbConn)

	apiCfg := &apiConfig{
		fileServerHits: atomic.Int32{},
		db:             dbQueries,
		platform:       os.Getenv("PLATFORM"),
		secret:         os.Getenv("SECRET"),
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
	params := database.RevokeRefreshTokenParams{
		Token:     refreshToken,
		RevokedAt: sql.NullTime{Time: time.Now(), Valid: true},
	}
	err = cfg.db.RevokeRefreshToken(r.Context(), params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't revoke session", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (cfg *config) handlerUsers(w http.ResponseWriter, r *http.Request) {
	type user struct {
		Password: `json:"password"`
		Email: `json:"email"`
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find token", err)
	}

	userID, err = auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "invaild token", err)
	}
	
	decoder := json.NewDecoder(r.Body)
	params := user{}
	err = decoder.Decode(&params)
	
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to decode Json", err)
	}

	hashedPassword, err = auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to hash password", err)
	}

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

	dbChirps, err := cfg.db.GetChirps(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "trouble accessing database", err)
		return
	}

	apiChirps := make([]Chirps, len(dbChirps))
	for i, dbChirp := range dbChirps {
		apiChirps[i] = databaseChirpToApi(dbChirp)
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
