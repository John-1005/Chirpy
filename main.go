package main


import (
    "fmt"
    "net/http"
    "sync/atomic"
    "os"
    "database/sql"
    "log"
    "context"


    "github.com/John-1005/Chirpy/internal/database"
  _ "github.com/lib/pq"
    "github.com/joho/godotenv"
)





type apiConfig struct {
    fileServerHits atomic.Int32
    db *database.Queries
}

type User struct {
  ID uuid.UUID        `json:"id"`
  CreatedAt time.Time `json:"created_at"`
  UpdatedAt time.Time `json:"updated_at"`
  Email string        `json:"email"`
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
  }




  mux := http.NewServeMux()
  fileServer := http.StripPrefix("/app", http.FileServer(http.Dir(".")))

  mux.Handle("/app/", apiCfg.middlewareMetricsInc(fileServer))
  mux.HandleFunc("GET /api/healthz", handlerReadiness)
  mux.HandleFunc("GET /admin/metrics", apiCfg.handlerCount)
  mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
  mux.HandleFunc("POST /api/validate_chirp", handlerCheckChirp)
  mux.Handlefunc("POST /api/users", handlerCreateUser)

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

  cfg.fileServerHits.Store(0)

  w.Header().Set("Content-Type", "text/plain; charset=utf-8")

  w.WriteHeader(http.StatusOK)

  fmt.Fprintf(w, "Hit count reset to 0")

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

  user, err := cfg.db.CreateUser(r.Context(), params.Email)
  if err != nil {
    fmt.Println("Failed to create user: %s", err)
    os.Exit(1)
  }

}


