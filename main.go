package main


import (
    "fmt"
    "net/http"
    "sync/atomic"
)




type apiConfig struct {
    fileServerHits atomic.Int32
}














func main() {
  apiCfg := &apiConfig{}
  mux := http.NewServeMux()

  fileServer := http.StripPrefix("/app", http.FileServer(http.Dir(".")))

  mux.Handle("/app/", apiCfg.middlewareMetricsInc(fileServer))
  mux.HandleFunc("GET /api/healthz", readinessHandler)
  mux.HandleFunc("GET /admin/metrics", apiCfg.countHandler)
  mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
  mux.HandleFunc("POST /api/validate_chirp", checkChirpHandler)

  server := http.Server{
    Handler: mux,
    Addr: ":8080",
  }

  err := server.ListenAndServe()
  if err != nil {
    fmt.Println("Server Error:", err)
  }

}


func readinessHandler(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "text/plain; charset=utf-8")

  w.WriteHeader(http.StatusOK)

  w.Write([]byte("OK"))
}

func (cfg *apiConfig) countHandler(w http.ResponseWriter, r *http.Request) {

  currentCount := cfg.fileServerHits.Load()

  w.Header().Set("Content-Type", "text/plain; charset=utf-8")

  w.WriteHeader(http.StatusOK)

  fmt.Fprintf(w, "Welcome, Chirpy Admin\n")
  fmt.Fprintf(w, "Chirpy has been visited %d times!", currentCount)

}


func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {

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



