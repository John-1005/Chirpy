package main


import (
    "fmt"
    "net/http"
    "sync/atomic"
    "os"
    "database/sql"
    "log"
    "time"
    "encoding/json"


    "github.com/John-1005/Chirpy/internal/database"
  _ "github.com/lib/pq"
    "github.com/joho/godotenv"
    "github.com/google/uuid"
)


type apiConfig struct {
    fileServerHits atomic.Int32
    db *database.Queries
    platform string
}

type User struct {
  ID uuid.UUID        `json:"id"`
  CreatedAt time.Time `json:"created_at"`
  UpdatedAt time.Time `json:"updated_at"`
  Email string        `json:"email"`
}


type Chirps struct {
  ID uuid.UUID        `json:"id"`
  CreatedAt time.Time `json:"created_at"`
  UpdatedAt time.Time `json:"updated_at"`
  Body string         `json:"body"`
  User_ID uuid.UUID   `json:"user_id"`
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


  dbQueries := database.New(dbConn)

  apiCfg := &apiConfig{
      fileServerHits: atomic.Int32{},
      db:             dbQueries,
      platform:       os.Getenv("PLATFORM"),
  }




  mux := http.NewServeMux()
  fileServer := http.StripPrefix("/app", http.FileServer(http.Dir(".")))

  mux.Handle("/app/", apiCfg.middlewareMetricsInc(fileServer))
  mux.HandleFunc("GET /api/healthz", handlerReadiness)
  mux.HandleFunc("GET /admin/metrics", apiCfg.handlerCount)
  mux.HandleFunc("GET /api/chirps", apiCfg.handlerGetChirps)
  mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
  mux.HandleFunc("POST /api/chirps", apiCfg.handlerSendChirp)
  mux.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)

  server := http.Server{
    Handler: mux,
    Addr: ":8080",
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
    respondWithError(w, 500, "Failed to delete users", err )
    return
  }

  w.WriteHeader(http.StatusOK)

}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
  cfg.fileServerHits.Add(1)
  next.ServeHTTP(w, r)
  })

}


func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {

  type email struct {
    Email string `json:"email"`
  }

  decoder := json.NewDecoder(r.Body)
  params := email{}
  err := decoder.Decode(&params)
  if err != nil {
    respondWithError(w, http.StatusInternalServerError, "Couldn't decode message", err)
    return
  }

  dbUser, err := cfg.db.CreateUser(r.Context(), params.Email)
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





func (cfg *apiConfig) handlerSendChirp(w http.ResponseWriter, r *http.Request) {

  type message struct {
    Body string `json:"body"`
    User_ID uuid.UUID `json:"user_id"`
  }


  decoder := json.NewDecoder(r.Body)
  params := message{}
  err := decoder.Decode(&params)

  if err != nil {
    respondWithError(w, http.StatusInternalServerError, "Couldn't decode message", err)
    return
  }

  dbChirpParams := database.AddChirpParams {
    Body: params.Body,
    UserID: params.User_ID,
  }
  const maxLength = 140

  if len(params.Body) > maxLength {
    respondWithError(w, http.StatusBadRequest, "Chirp is too long", nil )
    return
  }

  dbChirp, err := cfg.db.AddChirp(r.Context(), dbChirpParams)
  if err != nil {
    respondWithError(w, http.StatusInternalServerError, "Failed to add chirp", err )
  }

  chirpResp := Chirps {
    ID:        dbChirp.ID,
    CreatedAt: dbChirp.CreatedAt,
    UpdatedAt: dbChirp.UpdatedAt,
    Body:      dbChirp.Body,
    User_ID:   dbChirp.UserID,
  }

  respondWithJSON(w, http.StatusCreated, chirpResp)
}


func (cfg *apiConfig) handlerGetChirps (w http.ResponseWriter, r *http.Request) {


  dbChirps, err := cfg.db.GetChirps(r.Context())
  if err != nil {
    respondWithError(w, http.StatusInternalServerError, "trouble accessing database", err)
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
